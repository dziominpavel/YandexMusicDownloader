// Package ui is the desktop window backend: it wires config, resolver,
// the download queue and the quality monitor together and streams events
// to the frontend.
package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"yamdl/internal/applog"
	"yamdl/internal/config"
	"yamdl/internal/downloader"
	"yamdl/internal/library"
	"yamdl/internal/quality"
	"yamdl/internal/queue"
	"yamdl/internal/resolver"
	"yamdl/internal/tagger"
)

// App is bound to the frontend (window.go.main.App).
type App struct {
	ctx       context.Context
	mu        sync.Mutex
	client    *downloader.Client
	outputDir string
	ready     string // startup status line

	// index is the collection index; nil means it is not built at all.
	index *library.Index
	// duplicatesEnabled is true only when LIBRARY_DIR is set and valid.
	duplicatesEnabled bool
	// readyNote is logged right after the startup status line.
	readyNote string
	// logSink, when set, captures log lines instead of emitting events.
	logSink func(text, kind string)
	// eventSink, when set, captures named frontend events instead of emitting.
	eventSink func(name string, args ...any)
	// envPath is the .env file the settings window edits.
	envPath string
	// cfg is the last applied configuration.
	cfg config.Config
	// validateToken checks a token against account/status.
	validateToken func(string) (string, error)
	// tokenGen invalidates an in-flight startup check when settings are saved.
	tokenGen int
	// authErr is the last startup token error.
	authErr string
	// logFile is the diagnostic file log; nil means file logging is off.
	logFile *applog.Logger
	// logFileErr explains why file logging is off.
	logFileErr error

	// qualityJournal is the persistent FLAC-monitor cache.
	qualityJournal *quality.Journal
	qualityPath    string
	qualityMu      sync.Mutex
	checking       bool
	checkCancel    context.CancelFunc
	found          []quality.FoundItem

	// q is the download queue: the single owner of download state.
	q *queue.Manager
	// exported marks sources whose .m3u was already written.
	exported map[string]bool

	// resolveHook, downloadHook and duplicateHook are test seams for the
	// queue; nil means the real implementations below.
	resolveHook   queue.ResolveFunc
	downloadHook  queue.DownloadFunc
	duplicateHook queue.DuplicateFunc
}

// SetLogFile attaches the diagnostic file log. Called by main before the
// window starts; nil keeps logging in the window only.
func (a *App) SetLogFile(l *applog.Logger) {
	a.logFile = l
}

// SetLogFileError remembers why the diagnostic log could not be opened.
func (a *App) SetLogFileError(err error) {
	a.logFileErr = err
}

// NewApp loads config and prepares the downloader and the queue. It never
// validates the token over the network: the window and the splash must
// appear first, and Startup checks the token in the background.
func NewApp() *App {
	a := &App{outputDir: "./downloads", validateToken: config.Validate}
	a.cfg = config.Config{
		OutputDir: "./downloads", ConvertM4A: true, EmbedCover: true,
		EmbedLyrics: true, SubdirTemplate: "%artist%/%album%",
		Workers: config.DefaultWorkers,
	}
	envPath, err := config.DefaultPath()
	if err != nil {
		a.ready = "config: " + err.Error()
		a.ensureQueue()
		return a
	}
	a.envPath = envPath
	cfg, err := config.Load(envPath)
	if err != nil {
		a.ready = "config: " + err.Error()
		a.ensureQueue()
		return a
	}
	a.apply(cfg, false)
	a.ensureQueue()
	return a
}

// ensureQueue creates the queue once; all bindings call it so a partially
// failed startup still yields an empty, working queue.
func (a *App) ensureQueue() *queue.Manager {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.q == nil {
		a.q = queue.New(a.queueSink(), a.resolveFunc(), a.downloadFunc(), a.duplicateFunc(), a.workers)
	}
	return a.q
}

func (a *App) queueSink() queue.Sink { return &appQueueSink{a: a} }

// resolveFunc returns the test seam or the real link resolver.
func (a *App) resolveFunc() queue.ResolveFunc {
	if a.resolveHook != nil {
		return a.resolveHook
	}
	return a.resolveLink
}

func (a *App) downloadFunc() queue.DownloadFunc {
	if a.downloadHook != nil {
		return a.downloadHook
	}
	return a.downloadTask
}

func (a *App) duplicateFunc() queue.DuplicateFunc {
	if a.duplicateHook != nil {
		return a.duplicateHook
	}
	return a.duplicateCandidate
}

// workers reports the configured pool size; read at every dispatch so a
// settings change applies to the next free slot.
func (a *App) workers() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.Workers
}

// appQueueSink forwards queue state to the window as named events.
type appQueueSink struct {
	a *App
}

func (s *appQueueSink) TaskAdded(t queue.Task)     { s.a.emit("task-added", t) }
func (s *appQueueSink) TaskRemoved(key string)     { s.a.emit("task-removed", key) }
func (s *appQueueSink) Sources(src []queue.Source) { s.a.emit("sources", src) }
func (s *appQueueSink) Queue(snap queue.Snapshot)  { s.a.emit("queue", snap) }
func (s *appQueueSink) Log(text, kind string)      { s.a.log(text, kind) }

func (s *appQueueSink) TaskUpdated(t queue.Task) {
	s.a.emit("task-updated", t)
	if !t.State.Terminal() {
		return
	}
	switch t.Kind {
	case "upgrade":
		go s.a.afterUpgradeTask()
	default:
		go s.a.maybeExportM3U(t.SourceKey)
	}
}

func (s *appQueueSink) Existing(trackID string) {
	s.a.emit("existing", trackID)
}

// apply pushes a loaded configuration into the running app. The queue is
// intentionally kept: its workers closure sees the new limit and new tasks
// capture the new client and output dir, while running downloads finish
// with the parameters they started with.
func (a *App) apply(cfg config.Config, validate bool) {
	a.cancelQualityCheck()
	a.mu.Lock()
	a.cfg = cfg
	a.tokenGen++
	a.authErr = ""
	a.mu.Unlock()
	a.addLogSecret(cfg.Token)
	a.ready = ""
	a.readyNote = ""
	a.outputDir = "./downloads"
	if cfg.OutputDir != "" {
		a.outputDir = cfg.OutputDir
	}
	if a.envPath != "" {
		jp := filepath.Join(filepath.Dir(a.envPath), "flac-quality.json")
		if a.qualityJournal == nil || a.qualityPath != jp {
			a.qualityJournal = quality.NewJournal(jp)
			a.qualityPath = jp
		}
	} else {
		a.qualityJournal = nil
		a.qualityPath = ""
	}
	// Пересобираем с нуля: LIBRARY_DIR мог стать пустым или смениться.
	a.index = nil
	a.duplicatesEnabled = false
	a.setupLibrary(a.envPath, cfg.LibraryDir)
	a.refreshPendingQuality()
	if cfg.Token == "" {
		a.mu.Lock()
		a.client = nil
		a.mu.Unlock()
		a.ready = "токен пуст — доступно только 30-секундное превью; открой «⚙ Настройки → Как получить токен?»"
		return
	}
	if validate {
		who, err := config.Validate(cfg.Token)
		if err != nil {
			a.mu.Lock()
			a.client = nil
			a.authErr = "токен НЕ принят: " + err.Error()
			a.mu.Unlock()
			a.ready = a.authErr
			return
		}
		c := a.newClient(cfg)
		a.mu.Lock()
		a.client = c
		a.mu.Unlock()
		a.attachIndex(c)
		a.ready = "токен ок: " + who
		return
	}
	c := a.newClient(cfg)
	a.mu.Lock()
	a.client = c
	a.mu.Unlock()
	a.attachIndex(c)
	a.ready = "проверяю токен…"
}

// newClient builds the downloader client from the applied configuration.
// It never touches the network; token validation is a separate step.
func (a *App) newClient(cfg config.Config) *downloader.Client {
	c := downloader.NewClient(cfg.Token)
	c.ConvertM4A = cfg.ConvertM4A
	c.EmbedLyrics = cfg.EmbedLyrics
	c.SaveLRC = cfg.SaveLRC
	c.Subdirs = cfg.Subdirs
	c.SubdirTemplate = cfg.SubdirTemplate
	c.PlaylistWorkers = cfg.Workers
	c.Tag = tagger.Adapter(cfg.EmbedCover)
	return c
}

// verifyStartupToken validates the token after the window is visible.
func (a *App) verifyStartupToken(token string, gen int) {
	check := a.validateToken
	if check == nil {
		check = config.Validate
	}
	who, err := check(token)

	a.mu.Lock()
	if gen != a.tokenGen {
		a.mu.Unlock()
		return
	}
	if err != nil {
		a.authErr = "токен НЕ принят: " + err.Error()
		msg := a.authErr
		a.mu.Unlock()
		a.log(msg, "error")
		return
	}
	a.authErr = ""
	a.mu.Unlock()
	a.log("токен ок: "+who, "info")
}

// authError returns the last startup token error, if any.
func (a *App) authError() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.authErr
}

// clientSnapshot returns the current client under the lock.
func (a *App) clientSnapshot() *downloader.Client {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.client
}

// attachIndex makes every freshly published file part of the collection.
func (a *App) attachIndex(c *downloader.Client) {
	if a.index == nil {
		return
	}
	c.OnPublished = a.index.AddFile
}

// setupLibrary builds the collection index when LIBRARY_DIR is set and
// points at a real folder, or when the FLAC monitor needs OUTPUT_DIR.
func (a *App) setupLibrary(envPath, libraryDir string) {
	libraryDir = strings.TrimSpace(libraryDir)
	a.duplicatesEnabled = libraryDir != ""
	if libraryDir == "" && !a.cfg.FlacMonitor {
		return
	}

	var roots []string
	if libraryDir != "" {
		if st, err := os.Stat(libraryDir); err != nil || !st.IsDir() {
			a.duplicatesEnabled = false
			a.readyNote = fmt.Sprintf("[index] папка коллекции не найдена: %s — проверка дублей выключена", libraryDir)
			if !a.cfg.FlacMonitor {
				return
			}
			a.readyNote += "; мониторинг FLAC работает по папке загрузок"
		} else {
			roots = append(roots, libraryDir)
		}
	}
	roots = append(roots, a.outputDir)
	cachePath := filepath.Join(filepath.Dir(envPath), "library-index.json")
	a.index = library.New(cachePath, roots...)
}

// addLogSecret tells the file logger to hide the token in every message.
func (a *App) addLogSecret(token string) {
	if a.logFile == nil {
		return
	}
	a.logFile.AddSecret(token)
}

// writeLogHeader writes the build and path block at the top of every run.
func (a *App) writeLogHeader() {
	if a.logFile == nil {
		return
	}
	exe := ""
	if p, err := os.Executable(); err == nil {
		exe = p
	}
	env := a.envPath
	if env == "" {
		env = "(не найден)"
	}
	lines := []string{
		"запуск: " + buildSummary(),
		"система: " + goruntime.GOOS + "/" + goruntime.GOARCH,
		"exe: " + exe,
		".env: " + env,
		"загрузки: " + a.outputDir,
		"лог: " + a.logFile.Path(),
	}
	for _, line := range lines {
		_ = a.logFile.Write(line, "info")
	}
}

// logFileLine tells the user where the diagnostic file is.
func (a *App) logFileLine() {
	switch {
	case a.logFile != nil:
		a.log("[log] файл: "+a.logFile.Path(), "info")
	case a.logFileErr != nil:
		a.log("[log] файл: отключён — "+a.logFileErr.Error(), "info")
	default:
		a.log("[log] файл: отключён", "info")
	}
}

// buildSummary describes the running binary.
func buildSummary() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "версия: unknown"
	}
	version := info.Main.Version
	if version == "" {
		version = "unknown"
	}
	parts := []string{"версия: " + version}
	var rev, at string
	modified := false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.time":
			at = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if rev != "" {
		if len(rev) > 12 {
			rev = rev[:12]
		}
		parts = append(parts, "ревизия: "+rev)
	}
	if at != "" {
		parts = append(parts, "время: "+at)
	}
	if rev != "" {
		parts = append(parts, fmt.Sprintf("изменено: %t", modified))
	}
	parts = append(parts, "go: "+goruntime.Version())
	return strings.Join(parts, ", ")
}

// Startup is called by Wails when the window is ready.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.writeLogHeader()
	a.logFileLine()
	a.log(a.ready, "info")
	if a.readyNote != "" {
		a.log(a.readyNote, "info")
	}
	if token := a.cfg.Token; token != "" {
		a.mu.Lock()
		gen := a.tokenGen
		a.mu.Unlock()
		go a.verifyStartupToken(token, gen)
	}
	if a.index != nil {
		go func() {
			a.index.Refresh(func(s string) { a.log(s, "info") })
			a.refreshPendingQuality()
			a.maybeAutoCheck()
		}()
	} else {
		a.refreshPendingQuality()
		a.maybeAutoCheck()
	}
	go a.autoCheckLoop(ctx)
	go func() {
		<-ctx.Done()
		a.cancelQualityCheck()
	}()
}

// DomReady shows the window once the frontend is loaded.
func (a *App) DomReady(ctx context.Context) {
	runtime.WindowShow(ctx)
	runtime.EventsEmit(ctx, "window-shown")
}

// BootReady shows the window after the frontend has armed the splash.
func (a *App) BootReady() {
	if a.ctx != nil {
		runtime.WindowShow(a.ctx)
	}
}

// Default window size for a first launch (see spec desktop-app).
const (
	DefaultWindowWidth  = 950
	DefaultWindowHeight = 634
	MinWindowWidth      = 860
	MinWindowHeight     = 600
)

// InitialSize returns the size the window must open with.
func (a *App) InitialSize() (int, int) {
	if a.cfg.WindowWidth > 0 && a.cfg.WindowHeight > 0 {
		return a.cfg.WindowWidth, a.cfg.WindowHeight
	}
	return DefaultWindowWidth, DefaultWindowHeight
}

// BeforeClose remembers the last normal window size and always closes.
func (a *App) BeforeClose(ctx context.Context) bool {
	a.cancelQualityCheck()
	if !runtime.WindowIsNormal(ctx) {
		return false
	}
	w, h := runtime.WindowGetSize(ctx)
	a.saveWindowSize(w, h)
	return false
}

// saveWindowSize writes the new size into .env.
func (a *App) saveWindowSize(w, h int) {
	if a.envPath == "" {
		return
	}
	updates := windowSizeUpdate(a.cfg.WindowWidth, a.cfg.WindowHeight, w, h)
	if updates == nil {
		return
	}
	_ = config.Write(a.envPath, updates)
}

// windowSizeUpdate builds the config.Write payload for a new window size.
func windowSizeUpdate(lastW, lastH, w, h int) map[string]string {
	if w <= 0 || h <= 0 || (w == lastW && h == lastH) {
		return nil
	}
	return map[string]string{
		"WINDOW_WIDTH":  strconv.Itoa(w),
		"WINDOW_HEIGHT": strconv.Itoa(h),
	}
}

// log sends one line to the diagnostic file and the window.
func (a *App) log(text, kind string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.logFile != nil {
		text = a.logFile.Redact(text)
		_ = a.logFile.Write(text, kind)
	}
	if a.logSink != nil {
		a.logSink(text, kind)
		return
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "log", text, kind)
	}
}

// emit sends one named frontend event; tests capture it via eventSink.
func (a *App) emit(name string, args ...any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.eventSink != nil {
		a.eventSink(name, args...)
		return
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, name, args...)
	}
}

// Submit puts pasted links into the queue and returns immediately. An
// empty return means accepted; otherwise the string is a [error] line.
func (a *App) Submit(link string) string {
	if msg := a.authError(); msg != "" {
		return "[error] " + msg
	}
	if a.clientSnapshot() == nil {
		return "[error] нет токена: доступно только 30-секундное превью. Открой «⚙ Настройки → Как получить токен?»"
	}
	links := splitLinks(link)
	if len(links) == 0 {
		return "[error] пустая ссылка"
	}
	a.ensureQueue().Submit(links)
	return ""
}

// GetQueue returns the current queue snapshot (first paint / redraw).
func (a *App) GetQueue() queue.Snapshot {
	return a.ensureQueue().Snapshot()
}

// CancelTask cancels one task (queued or active).
func (a *App) CancelTask(key string) {
	a.ensureQueue().CancelTask(key)
}

// RetryTask re-queues one failed or cancelled task.
func (a *App) RetryTask(key string) {
	a.ensureQueue().RetryTask(key)
}

// RemoveTask drops a finished row.
func (a *App) RemoveTask(key string) {
	a.ensureQueue().RemoveTask(key)
}

// CancelAll stops every active and waiting task.
func (a *App) CancelAll() {
	a.ensureQueue().CancelAll()
}

// ClearFinished removes finished rows from the list.
func (a *App) ClearFinished() {
	a.ensureQueue().ClearFinished()
}

// CancelSource cancels every task that came from one link.
func (a *App) CancelSource(key string) {
	a.ensureQueue().CancelSource(key)
}

// ResolveDuplicate answers a "possible duplicate" row.
func (a *App) ResolveDuplicate(key string, accept bool) {
	a.ensureQueue().ResolveDuplicate(key, accept)
}

// resolveLink expands one pasted link into tracks for the queue.
func (a *App) resolveLink(ctx context.Context, link string) (queue.Resolved, error) {
	c := a.clientSnapshot()
	if c == nil {
		return queue.Resolved{}, fmt.Errorf("нет токена: доступно только 30-секундное превью")
	}
	ref, err := resolver.Parse(strings.TrimSpace(link))
	if err != nil {
		return queue.Resolved{}, err
	}
	switch ref.Kind {
	case resolver.KindPlaylist:
		pl, err := c.PlaylistCtx(ctx, ref.PlaylistUUID)
		if err != nil {
			return queue.Resolved{}, err
		}
		return queue.Resolved{
			Label:  "Плейлист · " + pl.Title,
			Kind:   "playlist",
			Title:  pl.Title,
			Log:    fmt.Sprintf("[playlist] %s (%d треков)", pl.Title, len(pl.Tracks)),
			Tracks: pl.Tracks,
		}, nil
	case resolver.KindAlbum:
		alb, err := c.AlbumCtx(ctx, ref.AlbumID)
		if err != nil {
			return queue.Resolved{}, err
		}
		return queue.Resolved{
			Label:  "Альбом · " + alb.Title,
			Kind:   "album",
			Title:  alb.Title,
			Log:    fmt.Sprintf("[album] %s (%d треков)", alb.Title, len(alb.Tracks)),
			Tracks: alb.Tracks,
		}, nil
	default:
		t, err := c.ResolveTrackCtx(ctx, ref.TrackID)
		if err != nil {
			return queue.Resolved{}, err
		}
		return queue.Resolved{Label: "Трек · " + t.Label(), Kind: "track", Tracks: []*downloader.Track{t}}, nil
	}
}

// downloadTask downloads one queue task with the parameters current at the
// moment the worker starts: a settings change applies to new tasks only.
func (a *App) downloadTask(ctx context.Context, track *downloader.Track, item any, onProgress func(done, total int64)) (queue.Result, error) {
	if it, ok := item.(quality.FoundItem); ok {
		return a.upgradeItemTask(ctx, it, onProgress)
	}
	c := a.clientSnapshot()
	if c == nil {
		return queue.Result{}, fmt.Errorf("нет токена")
	}
	a.mu.Lock()
	dir := a.outputDir
	a.mu.Unlock()
	var progress downloader.ProgressFunc
	if onProgress != nil {
		progress = func(p downloader.Progress) { onProgress(p.Done, p.Total) }
	}
	res, err := c.DownloadTrackCtx(ctx, track, dir, nil, progress)
	return queue.Result{
		Path:           res.Path,
		Format:         res.Format,
		Fallback:       res.Fallback,
		FallbackReason: res.FallbackReason,
		Converted:      res.Converted,
	}, err
}

// upgradeItemTask runs one FLAC upgrade inside the shared queue pool.
func (a *App) upgradeItemTask(ctx context.Context, it quality.FoundItem, onProgress func(done, total int64)) (queue.Result, error) {
	a.cancelQualityCheck()
	c := a.clientSnapshot()
	if c == nil {
		return queue.Result{}, fmt.Errorf("нет токена")
	}
	a.mu.Lock()
	cfg := a.cfg
	dir := a.outputDir
	ix := a.index
	journal := a.qualityJournal
	a.mu.Unlock()
	up := &quality.Upgrader{
		Client: c, Upgrade: c.Upgrade, Index: ix, Journal: journal,
		LibraryDir: cfg.LibraryDir, OutputDir: dir,
		Log: func(s string) { a.log(s, "info") },
	}
	var progress downloader.ProgressFunc
	if onProgress != nil {
		progress = func(p downloader.Progress) { onProgress(p.Done, p.Total) }
	}
	res, err := up.UpgradeItem(ctx, it, nil, progress)
	return queue.Result{Path: res.Path, Format: res.Format, Converted: res.Converted}, err
}

// duplicateCandidate looks for a probable duplicate in the collection.
func (a *App) duplicateCandidate(t *downloader.Track) (queue.Candidate, bool) {
	if !a.duplicatesEnabled || a.index == nil || t == nil {
		return queue.Candidate{}, false
	}
	if a.index.Scanning() {
		done, total := a.index.Progress()
		a.log(fmt.Sprintf("[warn] индексация ещё идёт (%d/%d) — проверка дублей неполная", done, total), "info")
	}
	matches := a.index.FindMatches(t.Artists, t.FullTitle())
	if len(matches) == 0 {
		return queue.Candidate{}, false
	}
	m := matches[0]
	p := queue.Candidate{
		Percent:  m.Percent,
		FileName: filepath.Base(m.Path),
		FilePath: m.Path,
		FileExt:  strings.TrimPrefix(strings.ToLower(m.Ext), "."),
	}
	p.DifferentFormat = p.FileExt != "" && p.FileExt != "flac" && p.FileExt != "m4a"
	return p, true
}

// afterUpgradeTask refreshes the FLAC offer list once no upgrade task is
// left running. It runs in its own goroutine from the queue sink.
func (a *App) afterUpgradeTask() {
	qm := a.ensureQueue()
	for _, t := range qm.Snapshot().Tasks {
		if t.Kind == "upgrade" && !t.State.Terminal() {
			return
		}
	}
	a.refreshPendingQuality()
	remaining := a.PendingFound()
	a.emit("quality-pending", remaining)
	status := a.qualityStatusView()
	a.emit("quality-check-finished", status.Text, len(remaining), "", len(remaining))
}

// maybeExportM3U writes the playlist/album .m3u once its source is fully
// finished and EXPORT_M3U is on. It is idempotent per source.
func (a *App) maybeExportM3U(sourceKey string) {
	a.mu.Lock()
	if a.exported != nil && a.exported[sourceKey] {
		a.mu.Unlock()
		return
	}
	cfg := a.cfg
	a.mu.Unlock()
	if !cfg.ExportM3U || sourceKey == "" {
		return
	}
	snap := a.ensureQueue().Snapshot()
	var src queue.Source
	for _, s := range snap.Sources {
		if s.Key == sourceKey {
			src = s
		}
	}
	if src.Kind != "album" && src.Kind != "playlist" {
		return
	}
	var paths []string
	for _, t := range snap.Tasks {
		if t.SourceKey != sourceKey {
			continue
		}
		if !t.State.Terminal() {
			return
		}
		if t.Path != "" {
			paths = append(paths, t.Path)
		}
	}
	if len(paths) == 0 {
		return
	}
	a.mu.Lock()
	if a.exported == nil {
		a.exported = make(map[string]bool)
	}
	a.exported[sourceKey] = true
	a.mu.Unlock()
	a.exportM3U(src.Title, paths)
}

// exportM3U writes a playlist file next to the downloaded tracks. Failures
// are silent: audio files are already valid and the export is a convenience.
func (a *App) exportM3U(title string, paths []string) {
	if len(paths) == 0 {
		return
	}
	a.mu.Lock()
	cfg := a.cfg
	dir := a.outputDir
	a.mu.Unlock()
	if !cfg.ExportM3U {
		return
	}
	if dir == "" {
		dir = filepath.Dir(paths[0])
	}
	name := m3uSafeName(title)
	if name == "" {
		name = "playlist"
	}
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for _, p := range paths {
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			rel = p
		}
		b.WriteString(strings.ReplaceAll(rel, `\`, "/"))
		b.WriteByte('\n')
	}
	_ = os.WriteFile(filepath.Join(dir, name+".m3u"), []byte(b.String()), 0o644)
}

// m3uSafeName sanitizes a playlist title for use as a file name.
func m3uSafeName(title string) string {
	repl := strings.NewReplacer(
		"/", "_", `\`, "_", ":", "_", "*", "_",
		"?", "_", `"`, "_", "<", "_", ">", "_", "|", "_",
	)
	return strings.Trim(strings.TrimSpace(repl.Replace(title)), ". ")
}

func splitLinks(input string) []string {
	var out []string
	var cur strings.Builder
	for _, r := range input {
		switch r {
		case '\n', '\r', '\t', ' ', ',', ';':
			s := strings.TrimSpace(cur.String())
			cur.Reset()
			if s != "" {
				out = append(out, s)
			}
			continue
		default:
			cur.WriteRune(r)
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		out = append(out, s)
	}
	return out
}

// Preview is the compact link preview shown under the input field.
type Preview struct {
	Kind     string // "track", "album", "playlist"
	Title    string
	Subtitle string
	CoverURL string
}

// PreviewLink resolves a pasted link just enough to show a cover and a
// title. It never downloads audio and never blocks «Скачать».
func (a *App) PreviewLink(link string) (Preview, error) {
	if msg := a.authError(); msg != "" {
		return Preview{}, errors.New(msg)
	}
	c := a.clientSnapshot()
	if c == nil {
		return Preview{}, fmt.Errorf("нет токена")
	}
	ref, err := resolver.Parse(strings.TrimSpace(link))
	if err != nil {
		return Preview{}, err
	}
	ctx := context.Background()
	switch ref.Kind {
	case resolver.KindPlaylist:
		pl, err := c.PlaylistCtx(ctx, ref.PlaylistUUID)
		if err != nil {
			return Preview{}, err
		}
		return previewOfList("playlist", pl.Title, pl.Tracks), nil
	case resolver.KindAlbum:
		alb, err := c.AlbumCtx(ctx, ref.AlbumID)
		if err != nil {
			return Preview{}, err
		}
		return previewOfList("album", alb.Title, alb.Tracks), nil
	default:
		t, err := c.ResolveTrackCtx(ctx, ref.TrackID)
		if err != nil {
			return Preview{}, err
		}
		p := Preview{Kind: "track", Title: t.FullTitle(), Subtitle: strings.Join(t.Artists, ", ")}
		if t.Album != "" {
			p.Subtitle = strings.TrimSpace(p.Subtitle + " · " + t.Album)
		}
		p.CoverURL = tagger.CoverURL(t.CoverURI, tagger.DefaultCoverSize)
		return p, nil
	}
}

func previewOfList(kind, title string, tracks []*downloader.Track) Preview {
	p := Preview{Kind: kind, Title: title, Subtitle: fmt.Sprintf("%d треков", len(tracks))}
	if len(tracks) > 0 {
		p.CoverURL = tagger.CoverURL(tracks[0].CoverURI, tagger.DefaultCoverSize)
		if len(tracks[0].Artists) > 0 {
			p.Subtitle = strings.Join(tracks[0].Artists, ", ") + " · " + p.Subtitle
		}
	}
	return p
}

// OpenOutputDir opens the downloads folder in the system file manager.
func (a *App) OpenOutputDir() error {
	a.mu.Lock()
	dir := a.outputDir
	a.mu.Unlock()
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	return cmd.Start()
}

// PickFolder opens the native directory picker and returns the chosen path.
func (a *App) PickFolder() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("окно ещё не готово")
	}
	res, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Выбери папку",
	})
	if err != nil {
		return "", err
	}
	return res, nil
}

// Settings is the settings-window form.
type Settings struct {
	Token          string
	OutputDir      string
	LibraryDir     string
	Workers        int
	ConvertM4A     bool
	EmbedCover     bool
	EmbedLyrics    bool
	SaveLRC        bool
	Subdirs        bool
	SubdirTemplate string
	ExportM3U      bool
	// FLAC monitor read-only fields.
	FlacMonitor     bool
	QualityText     string
	QualityChecking bool
	QualityPending  int
	QualityRejected []quality.DeclinedItem
}

// GetSettings returns the applied configuration with defaults filled in.
// It stays available while downloads run (see capability app-settings).
func (a *App) GetSettings() (Settings, error) {
	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()
	out := cfg.OutputDir
	if out == "" {
		out = "./downloads"
	}
	workers := cfg.Workers
	if workers < 1 || workers > config.MaxWorkers {
		workers = config.DefaultWorkers
	}
	subdirTemplate := cfg.SubdirTemplate
	if subdirTemplate == "" {
		subdirTemplate = "%artist%/%album%"
	}
	qs := a.qualityStatusView()
	var rejected []quality.DeclinedItem
	if a.qualityJournal != nil {
		rejected = a.qualityJournal.DeclinedList()
	}
	return Settings{
		Token:           cfg.Token,
		OutputDir:       out,
		LibraryDir:      cfg.LibraryDir,
		Workers:         workers,
		ConvertM4A:      cfg.ConvertM4A,
		EmbedCover:      cfg.EmbedCover,
		EmbedLyrics:     cfg.EmbedLyrics,
		SaveLRC:         cfg.SaveLRC,
		Subdirs:         cfg.Subdirs,
		SubdirTemplate:  subdirTemplate,
		ExportM3U:       cfg.ExportM3U,
		FlacMonitor:     cfg.FlacMonitor,
		QualityText:     qs.Text,
		QualityChecking: qs.Checking,
		QualityPending:  qs.Pending,
		QualityRejected: rejected,
	}, nil
}

// CheckToken validates a token against account/status without saving it.
func (a *App) CheckToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("токен пуст")
	}
	who, err := config.Validate(token)
	if err != nil {
		return "", err
	}
	return "ок: " + who, nil
}

// SaveSettings validates the form, writes it into .env and hot-applies it
// without a restart. Active downloads keep their captured parameters.
func (a *App) SaveSettings(s Settings) error {
	if a.envPath == "" {
		return fmt.Errorf("путь к .env неизвестен — настройки не сохранены")
	}
	s.Token = strings.TrimSpace(s.Token)
	s.OutputDir = strings.TrimSpace(s.OutputDir)
	s.LibraryDir = strings.TrimSpace(s.LibraryDir)
	if s.OutputDir == "" {
		return fmt.Errorf("папка загрузок не может быть пустой")
	}
	if s.Workers < 1 || s.Workers > config.MaxWorkers {
		return fmt.Errorf("число параллельных скачиваний: целое от 1 до %d", config.MaxWorkers)
	}

	subdirTemplate := strings.TrimSpace(s.SubdirTemplate)
	if subdirTemplate == "" {
		subdirTemplate = "%artist%/%album%"
	}
	updates := map[string]string{
		"YANDEX_TOKEN":     s.Token,
		"OUTPUT_DIR":       s.OutputDir,
		"PLAYLIST_WORKERS": strconv.Itoa(s.Workers),
		"CONVERT_M4A":      strconv.FormatBool(s.ConvertM4A),
		"EMBED_COVER":      strconv.FormatBool(s.EmbedCover),
		"LIBRARY_DIR":      s.LibraryDir,
		"EMBED_LYRICS":     strconv.FormatBool(s.EmbedLyrics),
		"SAVE_LRC":         strconv.FormatBool(s.SaveLRC),
		"SUBDIRS":          strconv.FormatBool(s.Subdirs),
		"SUBDIR_TEMPLATE":  subdirTemplate,
		"EXPORT_M3U":       strconv.FormatBool(s.ExportM3U),
		"FLAC_MONITOR":     strconv.FormatBool(s.FlacMonitor),
	}
	// Сначала файл, потом память: ошибка записи ничего не меняет.
	if err := config.Write(a.envPath, updates); err != nil {
		return err
	}
	cfg := config.Config{
		Token:          s.Token,
		OutputDir:      s.OutputDir,
		LibraryDir:     s.LibraryDir,
		Workers:        s.Workers,
		ConvertM4A:     s.ConvertM4A,
		EmbedCover:     s.EmbedCover,
		EmbedLyrics:    s.EmbedLyrics,
		SaveLRC:        s.SaveLRC,
		Subdirs:        s.Subdirs,
		SubdirTemplate: subdirTemplate,
		ExportM3U:      s.ExportM3U,
		FlacMonitor:    s.FlacMonitor,
	}
	a.apply(cfg, true)
	a.ensureQueue()
	// Новый лимит воркеров должен быть виден и применён сразу: Touch
	// пересчитывает сводку и запускает ожидающие задачи по новому лимиту.
	a.ensureQueue().Touch()
	if a.ready != "" {
		a.log(a.ready, "info")
	}
	if a.readyNote != "" {
		a.log(a.readyNote, "info")
	}
	if a.index != nil {
		go a.index.Refresh(func(s string) { a.log(s, "info") })
	}
	a.maybeAutoCheck()
	return nil
}
