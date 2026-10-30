// Package ui is the desktop window backend: it wires config, resolver
// and downloader together and streams log events to the frontend.
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
	"yamdl/internal/resolver"
	"yamdl/internal/tagger"
)

// App is bound to the frontend (window.go.main.App).
type App struct {
	ctx       context.Context
	client    *downloader.Client
	outputDir string
	ready     string // startup status line
	mu        sync.Mutex

	// index is the collection index; nil means duplicate checks are off
	// (LIBRARY_DIR unset or invalid).
	index *library.Index
	// readyNote is logged right after the startup status line.
	readyNote string
	// confirm asks the user about a probable duplicate; nil = native
	// Wails dialog. Tests substitute it.
	confirm func(msg string) bool
	// logSink, when set, captures log lines instead of emitting events.
	logSink func(text, kind string)
	// eventSink, when set, captures named frontend events (job-start,
	// job-end, progress, track-status, tracks) instead of emitting them.
	eventSink func(name string, args ...any)
	// busy marks an active Download call so settings stay locked.
	busy bool
	// downloadOne runs the single-track pipeline; nil means
	// client.DownloadTrack. Tests substitute it (same pattern as
	// downloader.Client.downloadOne).
	downloadOne func(*downloader.Track, string, func(downloader.Event), downloader.ProgressFunc) error
	// downloadBatch runs the playlist pool; nil means
	// client.DownloadPlaylist. Tests substitute it (the pool itself is
	// covered by downloader tests).
	downloadBatch func([]*downloader.Track, string, func(downloader.Event), downloader.ProgressFunc) downloader.Summary
	// fetchPlaylist resolves a playlist UUID; nil means client.Playlist.
	// Tests substitute it.
	fetchPlaylist func(string) (*downloader.Playlist, error)
	// envPath is the .env file the settings window edits (empty when
	// the executable path is unresolvable).
	envPath string
	// cfg is the last applied configuration; source for GetSettings.
	cfg config.Config
	// logFile is the diagnostic file log; nil means file logging is off.
	logFile *applog.Logger
	// logFileErr explains why file logging is off (nil when it works).
	logFileErr error
}

// SetLogFile attaches the diagnostic file log. Called by main before the
// window starts; nil keeps logging in the window only.
func (a *App) SetLogFile(l *applog.Logger) {
	a.logFile = l
}

// SetLogFileError remembers why the diagnostic log could not be opened,
// so Startup can tell the user in the window.
func (a *App) SetLogFileError(err error) {
	a.logFileErr = err
}

// NewApp loads config and prepares the downloader.
func NewApp() *App {
	a := &App{outputDir: "./downloads"}
	a.cfg = config.Config{
		OutputDir: "./downloads", ConvertM4A: true, EmbedCover: true,
		EmbedLyrics: true, SubdirTemplate: "%artist%/%album%",
		Workers: config.DefaultWorkers,
	}
	envPath, err := config.DefaultPath()
	if err != nil {
		a.ready = "config: " + err.Error()
		return a
	}
	a.envPath = envPath
	cfg, err := config.Load(envPath)
	if err != nil {
		a.ready = "config: " + err.Error()
		return a
	}
	a.apply(cfg)
	return a
}

// apply pushes a loaded configuration into the running app: output dir,
// downloader client and the collection index. It is the single path used
// both at startup and by SaveSettings (hot reload, no restart). Status
// lines land in a.ready/a.readyNote for the caller to log.
func (a *App) apply(cfg config.Config) {
	a.cfg = cfg
	a.addLogSecret(cfg.Token)
	a.ready = ""
	a.readyNote = ""
	a.outputDir = "./downloads"
	if cfg.OutputDir != "" {
		a.outputDir = cfg.OutputDir
	}
	// Пересобираем с нуля: LIBRARY_DIR мог стать пустым или смениться.
	a.index = nil
	a.setupLibrary(a.envPath, cfg.LibraryDir)
	if cfg.Token == "" {
		a.client = nil
		a.ready = "токен пуст — доступно только 30-секундное превью; открой «⚙ Настройки → Как получить токен?»"
		return
	}
	who, err := config.Validate(cfg.Token)
	if err != nil {
		a.client = nil
		a.ready = "токен НЕ принят: " + err.Error()
		return
	}
	c := downloader.NewClient(cfg.Token)
	c.ConvertM4A = cfg.ConvertM4A
	c.EmbedLyrics = cfg.EmbedLyrics
	c.SaveLRC = cfg.SaveLRC
	c.Subdirs = cfg.Subdirs
	c.SubdirTemplate = cfg.SubdirTemplate
	c.PlaylistWorkers = cfg.Workers
	c.Tag = tagger.Adapter(cfg.EmbedCover)
	a.client = c
	a.attachIndex(c)
	a.ready = "токен ок: " + who
}

// attachIndex makes every freshly published file part of the collection
// immediately — no rescan needed for the next duplicate check.
func (a *App) attachIndex(c *downloader.Client) {
	if a.index == nil {
		return
	}
	c.OnPublished = a.index.AddFile
}

// setupLibrary builds the collection index when LIBRARY_DIR is set and
// points at a real folder. An invalid path disables the feature with a
// Russian note in the log; an empty parameter keeps everything off.
func (a *App) setupLibrary(envPath, libraryDir string) {
	libraryDir = strings.TrimSpace(libraryDir)
	if libraryDir == "" {
		return
	}
	if st, err := os.Stat(libraryDir); err != nil || !st.IsDir() {
		a.readyNote = fmt.Sprintf("[index] папка коллекции не найдена: %s — проверка дублей выключена", libraryDir)
		return
	}
	cachePath := filepath.Join(filepath.Dir(envPath), "library-index.json")
	a.index = library.New(cachePath, libraryDir, a.outputDir)
}

// addLogSecret tells the file logger to hide token in every message.
// Called from apply, so a hot-reloaded token is covered too.
func (a *App) addLogSecret(token string) {
	if a.logFile == nil {
		return
	}
	a.logFile.AddSecret(token)
}

// writeLogHeader writes the build and path block at the top of every
// run. It bypasses a.log on purpose: the header belongs to the file only.
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

// logFileLine tells the user where the diagnostic file is, or why it is
// off. The line goes to both sinks through the regular log path.
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

// buildSummary describes the running binary: module version, VCS
// revision and Go version. Missing build info degrades to "unknown".
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
	if a.index != nil {
		// Полное сканирование — в фоне: окно живое, ход в логе.
		go a.index.Refresh(func(s string) { a.log(s, "info") })
	}
}

// Default window size for a first launch, i.e. when .env carries no
// saved WINDOW_WIDTH/WINDOW_HEIGHT (see spec desktop-app).
const (
	DefaultWindowWidth  = 950
	DefaultWindowHeight = 634
)

// InitialSize returns the size the window must open with: the last
// saved one when both dimensions survived Load, the default otherwise.
// Called before wails.Run, so it must not touch the Wails runtime.
func (a *App) InitialSize() (int, int) {
	if a.cfg.WindowWidth > 0 && a.cfg.WindowHeight > 0 {
		return a.cfg.WindowWidth, a.cfg.WindowHeight
	}
	return DefaultWindowWidth, DefaultWindowHeight
}

// BeforeClose is the Wails OnBeforeClose hook. It remembers the current
// window size and always lets the window close (returns false). A
// maximised/fullscreen window is skipped: only the last normal size is
// worth restoring.
func (a *App) BeforeClose(ctx context.Context) bool {
	if !runtime.WindowIsNormal(ctx) {
		return false
	}
	w, h := runtime.WindowGetSize(ctx)
	a.saveWindowSize(w, h)
	return false
}

// saveWindowSize writes the new size into .env. Errors are swallowed —
// there is no one left to report them to while the window closes.
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

// windowSizeUpdate builds the config.Write payload for a new window
// size, or nil when the size is invalid or unchanged — so a window that
// never moved does not rewrite .env on every close. Pure: no runtime,
// no filesystem.
func windowSizeUpdate(lastW, lastH, w, h int) map[string]string {
	if w <= 0 || h <= 0 || (w == lastW && h == lastH) {
		return nil
	}
	return map[string]string{
		"WINDOW_WIDTH":  strconv.Itoa(w),
		"WINDOW_HEIGHT": strconv.Itoa(h),
	}
}

// log sends one line to the diagnostic file and the window. Secrets are
// redacted before both sinks: a screenshot of the window is safe too.
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
// Safe for concurrent worker goroutines.
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

// progress forwards a byte counter to the frontend, where the track's
// row is updated in place. Safe for concurrent worker goroutines.
func (a *App) progress(p downloader.Progress) {
	a.emit("progress", p.TrackID, p.Label, p.Percent, p.Done, p.Total)
}

func formatEvent(e downloader.Event) (string, string) {
	switch e.Kind {
	case downloader.KindDownloading:
		return "[downloading] " + e.Label, "info"
	case downloader.KindDone:
		s := "[done] " + e.Label + " (" + e.Format + ")"
		if e.Fallback {
			s += " [fallback MP3"
			if e.FallbackReason != "" {
				s += ": " + e.FallbackReason
			}
			s += "]"
		}
		if e.Converted {
			s += " [ALAC→FLAC]"
		}
		return s, "done"
	case downloader.KindSkipped:
		switch e.Detail {
		case downloader.SkipDuplicate:
			return "[skip] " + e.Label + " (duplicate)", "info"
		case downloader.SkipUnavailable:
			return "[skip] " + e.Label + " (unavailable)", "info"
		default:
			return "[skip] " + e.Label + " (already exists)", "info"
		}
	case downloader.KindStopped:
		return "[stopped] " + e.Label, "stopped"
	default:
		return "[error] " + e.Label + ": " + e.Detail, "err"
	}
}

// checkDuplicate asks the user when the incoming track looks like a file
// already in the collection. True = the download may proceed. Runs for
// single tracks and per track of a playlist.
func (a *App) checkDuplicate(t *downloader.Track) bool {
	if a.index == nil || t == nil {
		return true
	}
	if a.index.Scanning() {
		// Честно: проверяем только уже проиндексированную часть.
		done, total := a.index.Progress()
		a.log(fmt.Sprintf("[warn] индексация ещё идёт (%d/%d) — проверка дублей неполная", done, total), "info")
	}
	matches := a.index.FindMatches(t.Artists, t.FullTitle())
	if len(matches) == 0 {
		return true
	}
	return a.askConfirm(duplicateMessage(t, matches[0]))
}

// duplicateMessage is the warning text: a probable duplicate, never a
// verdict — the percentage and both files are shown so the user decides.
func duplicateMessage(t *downloader.Track, m library.Match) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Возможен дубль — совпадение %d%%.\n\n", m.Percent)
	fmt.Fprintf(&b, "Сейчас скачивается: %s\n", t.Label())
	fmt.Fprintf(&b, "В коллекции: %s\n", m.Path)
	if ext := strings.TrimPrefix(strings.ToLower(m.Ext), "."); ext != "" && ext != "flac" && ext != "m4a" {
		fmt.Fprintf(&b, "Внимание: найденный файл в формате %s — может получиться один трек в двух форматах.\n", ext)
	}
	b.WriteString("\nСкачать всё равно?")
	return b.String()
}

// askConfirm shows the native question dialog (or the test substitute).
// The answer goes through confirmAnswer — never a label comparison here:
// Windows ignores our Buttons and answers Yes/No (see confirmAnswer).
// Errors fall back to "proceed": the check must not block downloads.
func (a *App) askConfirm(msg string) bool {
	if a.confirm != nil {
		return a.confirm(msg)
	}
	if a.ctx == nil {
		return true
	}
	res, err := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Возможен дубль",
		Message:       msg,
		Buttons:       []string{"Скачать", "Отмена"},
		DefaultButton: "Скачать",
		CancelButton:  "Отмена",
	})
	if err != nil {
		return true
	}
	return confirmAnswer(res)
}

// confirmAnswer interprets a native dialog answer. The platforms Wails
// runs on disagree: Windows discards the custom Buttons option and
// answers "Yes"/"No" (MB_YESNO), Linux answers "Yes"/"No"/"Cancel" or ""
// for a dismissed dialog, macOS echoes the labels we set ("Скачать"/
// "Отмена"). So only a definite "no" declines; every other answer —
// including an unexpected one — proceeds, because a dialog quirk must
// never turn into a silently skipped download.
func confirmAnswer(res string) bool {
	switch strings.ToLower(strings.TrimSpace(res)) {
	case "", "no", "n", "нет", "отмена", "cancel":
		return false
	default:
		return true
	}
}

// filterDuplicates applies the per-track confirmation to a playlist.
// Declined tracks are logged and dropped; the rest (including the
// unavailable ones, which need no question) go through as before.
func (a *App) filterDuplicates(tracks []*downloader.Track) []*downloader.Track {
	if a.index == nil {
		return tracks
	}
	var out []*downloader.Track
	for _, t := range tracks {
		if t == nil {
			continue
		}
		if !t.Available || a.checkDuplicate(t) {
			out = append(out, t)
			continue
		}
		a.log("[skip] "+t.Label()+" (дубль, отменено пользователем)", "info")
	}
	return out
}

func splitLinks(input string) []string {
	// split by whitespace and comma/semicolon
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

// Download fetches track/album/playlist links. It blocks; the frontend
// awaits it while live log and progress events arrive. Supports multiple
// links separated by whitespace/comma/semicolon.
func (a *App) Download(link string) string {
	if a.client == nil {
		return "[error] нет токена: доступно только 30-секундное превью. Открой «⚙ Настройки → Как получить токен?»"
	}
	links := splitLinks(link)
	if len(links) == 0 {
		return "[error] пустая ссылка"
	}
	if len(links) == 1 {
		return a.downloadOneLink(links[0])
	}
	var first string
	for i, l := range links {
		if i == 0 {
			first = a.downloadOneLink(l)
			continue
		}
		res := a.downloadOneLink(l)
		if res != "" && first == "" {
			first = res
		}
	}
	return first
}

func (a *App) downloadOneLink(link string) string {
	a.setBusy(true)
	defer a.setBusy(false)
	// Новая job'а: сбрасываем остановку предыдущей, чтобы «Остановить»
	// не отравил следующее скачивание (stop.go).
	a.client.StartJob()

	ref, err := resolver.Parse(link)
	if err != nil {
		return "[error] " + err.Error()
	}
	if ref.Kind == resolver.KindPlaylist {
		return a.downloadPlaylist(ref.PlaylistUUID)
	}
	if ref.Kind == resolver.KindAlbum {
		return a.downloadAlbum(ref.AlbumID)
	}
	track, err := a.client.ResolveTrack(ref.TrackID)
	if err != nil {
		if a.stopRequested() {
			return "[stopped] прервано пользователем"
		}
		return "[error] " + err.Error()
	}
	if !a.checkDuplicate(track) {
		a.log("[skip] "+track.Label()+" (дубль, отменено пользователем)", "info")
		return ""
	}
	return a.downloadSingle(track)
}

func (a *App) downloadAlbum(id string) string {
	alb, err := a.client.Album(id)
	if err != nil {
		if a.stopRequested() {
			return "[stopped] прервано пользователем"
		}
		return fmt.Sprintf("[error] альбом недоступен: %v", err)
	}
	if len(alb.Tracks) == 0 {
		return "[error] альбом пуст"
	}
	tracks := a.filterDuplicates(alb.Tracks)
	if len(tracks) == 0 {
		return "[skip] все треки отклонены как возможные дубли"
	}
	title := alb.Title
	if title == "" {
		title = "Альбом"
	}
	a.jobStart(title, len(tracks))
	a.trackRows(tracks)
	sum, paths := a.runBatchCollect(tracks)
	a.exportM3U(title, paths)
	stopped := sum.Stopped > 0 || a.stopRequested()
	if stopped {
		a.log(fmt.Sprintf("[stopped] прервано: %d ok, %d skip, %d error", sum.OK, sum.Skipped, sum.Failed), "stopped")
	} else {
		a.log(fmt.Sprintf("[finished] %d ok, %d skip, %d error", sum.OK, sum.Skipped, sum.Failed), "info")
	}
	a.jobEnd(sum.OK, sum.Skipped, sum.Failed, stopped)
	return ""
}

// downloadSingle runs one track as a job of 1: the same lifecycle as a
// playlist (job-start → row status/progress → job-end), plus the full
// log lines the window spec requires for single downloads.
func (a *App) downloadSingle(track *downloader.Track) string {
	a.jobStart("", 1) // no header line: the row itself shows the label
	a.trackRows([]*downloader.Track{track})
	var shown bool
	sum := downloader.Summary{}
	run := a.downloadOne
	if run == nil {
		run = func(t *downloader.Track, dir string, emit func(downloader.Event), onProgress downloader.ProgressFunc) error {
			_, err := a.client.DownloadTrack(t, dir, emit, onProgress)
			return err
		}
	}
	err := run(track, a.outputDir, func(e downloader.Event) {
		text, kind := formatEvent(e)
		a.log(text, kind)
		status, skind := formatTrackStatus(e)
		a.trackStatus(e.TrackID, status, skind)
		countOutcome(&sum, e)
		if e.Kind == downloader.KindDone || e.Kind == downloader.KindSkipped ||
			e.Kind == downloader.KindFailed || e.Kind == downloader.KindStopped {
			shown = true
		}
	}, func(p downloader.Progress) {
		a.progress(p)
	})
	a.jobEnd(sum.OK, sum.Skipped, sum.Failed, sum.Stopped > 0 || a.stopRequested())
	if err != nil {
		if shown {
			return ""
		}
		if errors.Is(err, downloader.ErrStopped) {
			return "[stopped] прервано пользователем"
		}
		return fmt.Sprintf("[error] %v", err)
	}
	if !shown {
		return "[done]"
	}
	return ""
}

// countOutcome folds one terminal lifecycle event into the job summary.
func countOutcome(sum *downloader.Summary, e downloader.Event) {
	switch e.Kind {
	case downloader.KindDone:
		sum.OK++
	case downloader.KindSkipped:
		sum.Skipped++
	case downloader.KindFailed:
		sum.Failed++
	case downloader.KindStopped:
		sum.Stopped++
	}
}

// formatTrackStatus renders the short in-row status for one track:
// the same vocabulary as the log, minus the repeated label.
// The frontend shows exactly one row per track and only this part changes.
func formatTrackStatus(e downloader.Event) (string, string) {
	switch e.Kind {
	case downloader.KindDownloading:
		return "[downloading]", "info"
	case downloader.KindDone:
		s := "[done] (" + e.Format + ")"
		if e.Fallback {
			s += " [fallback MP3]"
		}
		if e.Converted {
			s += " [ALAC→FLAC]"
		}
		return s, "done"
	case downloader.KindSkipped:
		switch e.Detail {
		case downloader.SkipDuplicate:
			return "[skip] (duplicate)", "info"
		case downloader.SkipUnavailable:
			return "[skip] (unavailable)", "info"
		default:
			return "[skip] (already exists)", "info"
		}
	case downloader.KindStopped:
		return "[stopped]", "stopped"
	default:
		return "[error] " + e.Detail, "err"
	}
}

// trackStatus forwards one track's status to its frontend row.
// Safe for concurrent worker goroutines.
func (a *App) trackStatus(id, status, kind string) {
	a.emit("track-status", id, status, kind)
}

// trackRows sends the ordered row scaffolding: one [id, label] pair per
// unique track, so the frontend shows exactly len rows in playlist order
// before any worker reports back.
func (a *App) trackRows(tracks []*downloader.Track) {
	seen := make(map[string]bool)
	var rows [][2]string
	for _, t := range tracks {
		if t == nil || seen[t.ID] {
			continue
		}
		seen[t.ID] = true
		rows = append(rows, [2]string{t.ID, t.Label()})
	}
	if len(rows) == 0 {
		return
	}
	a.emit("tracks", rows)
}

// fetchPlaylistUUID resolves a playlist via the injectable hook (tests)
// or the real client.
func (a *App) fetchPlaylistUUID(uuid string) (*downloader.Playlist, error) {
	if a.fetchPlaylist != nil {
		return a.fetchPlaylist(uuid)
	}
	return a.client.Playlist(uuid)
}

// jobCallbacks is the shared event/progress wiring for every batch job:
// one row status per track, a fallback reason line when lossless was
// unavailable, and byte counters forwarded to the frontend. Single-track
// downloads use their own callback (formatEvent writes the full line).
func (a *App) jobCallbacks() (func(downloader.Event), downloader.ProgressFunc) {
	emit := func(e downloader.Event) {
		status, kind := formatTrackStatus(e)
		a.trackStatus(e.TrackID, status, kind)
		if e.Kind == downloader.KindDone && e.Fallback && e.FallbackReason != "" {
			a.log("[fallback] "+e.Label+": "+e.FallbackReason, "info")
		}
	}
	return emit, func(p downloader.Progress) { a.progress(p) }
}

// runBatch downloads the playlist tracks through the injectable hook or
// the real worker pool, wiring the shared job callbacks.
func (a *App) runBatch(tracks []*downloader.Track) downloader.Summary {
	sum, _ := a.runBatchCollect(tracks)
	return sum
}

// runBatchCollect is runBatch plus the paths of successfully published
// files, used by the .m3u export.
func (a *App) runBatchCollect(tracks []*downloader.Track) (downloader.Summary, []string) {
	emit, onProgress := a.jobCallbacks()
	var paths []string
	collect := func(e downloader.Event) {
		if e.Kind == downloader.KindDone && e.Path != "" {
			paths = append(paths, e.Path)
		}
		emit(e)
	}
	var sum downloader.Summary
	if a.downloadBatch != nil {
		sum = a.downloadBatch(tracks, a.outputDir, collect, onProgress)
	} else {
		sum = a.client.DownloadPlaylist(tracks, a.outputDir, collect, onProgress)
	}
	return sum, paths
}

// exportM3U writes a playlist file next to the downloaded tracks when
// EXPORT_M3U is enabled. Failures are silent: audio files are already
// valid and the export is a convenience.
func (a *App) exportM3U(title string, paths []string) {
	if !a.cfg.ExportM3U || len(paths) == 0 {
		return
	}
	dir := a.outputDir
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

// downloadPlaylist resolves a playlist UUID into tracks, asks about the
// probable duplicates per track and downloads the rest with the fixed
// worker pool. One track never stops the rest.
// The frontend shows exactly one row per track (status updates in place);
// the log only gets the header and the [finished] summary line.
func (a *App) downloadPlaylist(uuid string) string {
	pl, err := a.fetchPlaylistUUID(uuid)
	if err != nil {
		if a.stopRequested() {
			return "[stopped] прервано пользователем"
		}
		return fmt.Sprintf("[error] плейлист недоступен: %v", err)
	}
	if len(pl.Tracks) == 0 {
		return "[error] плейлист пуст"
	}
	tracks := a.filterDuplicates(pl.Tracks)
	if len(tracks) == 0 {
		return "[skip] все треки отклонены как возможные дубли"
	}
	title := pl.Title
	if title == "" {
		title = "Плейлист"
	}
	a.jobStart(title, len(tracks))
	a.trackRows(tracks)
	sum, paths := a.runBatchCollect(tracks)
	a.exportM3U(title, paths)
	stopped := sum.Stopped > 0 || a.stopRequested()
	if stopped {
		a.log(fmt.Sprintf("[stopped] прервано: %d ok, %d skip, %d error", sum.OK, sum.Skipped, sum.Failed), "stopped")
	} else {
		a.log(fmt.Sprintf("[finished] %d ok, %d skip, %d error", sum.OK, sum.Skipped, sum.Failed), "info")
	}
	a.jobEnd(sum.OK, sum.Skipped, sum.Failed, stopped)
	return ""
}

// jobStart announces a new job: the frontend clears the previous run,
// shows the optional header ("" for a single track — its row carries the
// label) and opens count rows for status updates.
func (a *App) jobStart(title string, count int) {
	a.emit("job-start", title, count)
}

// jobEnd freezes the statusbar on the outcome of the finished job.
// stopped marks a user abort: the frontend must not pull the bar to
// 100% — an unfinished job stays at its real percentage.
func (a *App) jobEnd(ok, skipped, failed int, stopped bool) {
	a.emit("job-end", ok, skipped, failed, stopped)
}

// Stop aborts the running download: in-flight requests are cancelled
// and the playlist pool stops launching queued tracks. Frontend side:
// the «Скачать» button becomes «■ Остановить» while busy.
func (a *App) Stop() {
	a.mu.Lock()
	c := a.client
	a.mu.Unlock()
	if c != nil {
		c.Stop()
	}
}

// stopRequested reports whether the user pressed «Остановить» in the
// current job.
func (a *App) stopRequested() bool {
	a.mu.Lock()
	c := a.client
	a.mu.Unlock()
	return c != nil && c.Stopped()
}

// Preview is the compact link preview shown above the input field.
type Preview struct {
	Kind     string // "track", "album", "playlist"
	Title    string
	Subtitle string
	CoverURL string
}

// PreviewLink resolves a pasted link just enough to show a cover and a
// title. It never downloads audio and never blocks «Скачать».
func (a *App) PreviewLink(link string) (Preview, error) {
	if a.client == nil {
		return Preview{}, fmt.Errorf("нет токена")
	}
	ref, err := resolver.Parse(strings.TrimSpace(link))
	if err != nil {
		return Preview{}, err
	}
	switch ref.Kind {
	case resolver.KindPlaylist:
		pl, err := a.fetchPlaylistUUID(ref.PlaylistUUID)
		if err != nil {
			return Preview{}, err
		}
		return previewOfList("playlist", pl.Title, pl.Tracks), nil
	case resolver.KindAlbum:
		alb, err := a.client.Album(ref.AlbumID)
		if err != nil {
			return Preview{}, err
		}
		return previewOfList("album", alb.Title, alb.Tracks), nil
	default:
		t, err := a.client.ResolveTrack(ref.TrackID)
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
	dir := a.outputDir
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

// PickFolder opens the native directory picker and returns the chosen
// path ("" when cancelled). Used by the settings window.
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

// setBusy marks the download state; the settings window stays locked
// while busy (spec: настройки закрыты на время скачивания).
func (a *App) setBusy(v bool) {
	a.mu.Lock()
	a.busy = v
	a.mu.Unlock()
}

func (a *App) isBusy() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.busy
}

// Settings is the settings-window form: everything the UI exposes,
// mapped 1:1 to .env keys (see config.Load).
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
}

// GetSettings returns the currently applied configuration with defaults
// filled in, so the form never shows empty required fields. Locked while
// a download runs.
func (a *App) GetSettings() (Settings, error) {
	if a.isBusy() {
		return Settings{}, fmt.Errorf("идёт скачивание — настройки недоступны")
	}
	cfg := a.cfg
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
	return Settings{
		Token:          cfg.Token,
		OutputDir:      out,
		LibraryDir:     cfg.LibraryDir,
		Workers:        workers,
		ConvertM4A:     cfg.ConvertM4A,
		EmbedCover:     cfg.EmbedCover,
		EmbedLyrics:    cfg.EmbedLyrics,
		SaveLRC:        cfg.SaveLRC,
		Subdirs:        cfg.Subdirs,
		SubdirTemplate: subdirTemplate,
		ExportM3U:      cfg.ExportM3U,
	}, nil
}

// CheckToken validates a token against account/status without saving it.
// Returns "ok: login (uid)" or a Russian error message.
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

// SaveSettings validates the form, writes it into .env (preserving
// comments and unknown keys) and hot-applies it without a restart.
// On any failure the in-memory state stays untouched.
func (a *App) SaveSettings(s Settings) error {
	if a.isBusy() {
		return fmt.Errorf("идёт скачивание — настройки недоступны")
	}
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
	}
	a.apply(cfg)
	if a.ready != "" {
		a.log(a.ready, "info")
	}
	if a.readyNote != "" {
		a.log(a.readyNote, "info")
	}
	if a.index != nil {
		// Новый LIBRARY_DIR: полное сканирование в фоне, как при старте.
		go a.index.Refresh(func(s string) { a.log(s, "info") })
	}
	return nil
}
