package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"yamdl/internal/config"
	"yamdl/internal/downloader"
	"yamdl/internal/queue"
)

// eventRecorder captures named frontend events in order. Queue events come
// from worker goroutines, so every access is locked.
type eventRecorder struct {
	mu    sync.Mutex
	names []string
	args  []any
}

func (r *eventRecorder) sink(name string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.names = append(r.names, name)
	r.args = append(r.args, args)
}

func (r *eventRecorder) namesCopy() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.names...)
}

func (r *eventRecorder) find(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, n := range r.names {
		if n == name {
			return i
		}
	}
	return -1
}

func (r *eventRecorder) has(name string) bool { return r.find(name) >= 0 }

// lastQueueWorkers returns the worker limit from the last queue event.
func (r *eventRecorder) lastQueueWorkers() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.names) - 1; i >= 0; i-- {
		if r.names[i] != "queue" {
			continue
		}
		args, ok := r.args[i].([]any)
		if !ok || len(args) == 0 {
			continue
		}
		if snap, ok := args[0].(queue.Snapshot); ok {
			return snap.Workers
		}
	}
	return -1
}

func (r *eventRecorder) all(name string) []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []int
	for i, n := range r.names {
		if n == name {
			out = append(out, i)
		}
	}
	return out
}

// logRecorder captures journal lines.
type logRecorder struct {
	mu   sync.Mutex
	text []string
}

func (l *logRecorder) add(text, _ string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.text = append(l.text, text)
}

func (l *logRecorder) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.text...)
}

func (l *logRecorder) has(sub string) bool {
	for _, s := range l.all() {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func waitApp(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func trk(id, title string) *downloader.Track {
	return &downloader.Track{ID: id, Title: title, Artists: []string{"A"}, Available: true}
}

// newJobApp builds an App with a queue and test seams but no network.
func newJobApp(t *testing.T, rec *eventRecorder, logs *logRecorder) *App {
	t.Helper()
	a := &App{client: downloader.NewClient(""), logSink: func(string, string) {}}
	a.cfg = config.Config{Workers: 3, OutputDir: t.TempDir(), SubdirTemplate: "%artist%/%album%"}
	if rec != nil {
		a.eventSink = rec.sink
	}
	if logs != nil {
		a.logSink = logs.add
	}
	return a
}

func TestQueueLifecycleEvents(t *testing.T) {
	rec := &eventRecorder{}
	logs := &logRecorder{}
	a := newJobApp(t, rec, logs)
	a.resolveHook = func(_ context.Context, _ string) (queue.Resolved, error) {
		return queue.Resolved{
			Label: "Альбом · Микс", Kind: "album", Title: "Микс",
			Log:    "[album] Микс (2 треков)",
			Tracks: []*downloader.Track{trk("1", "T1"), trk("2", "T2")},
		}, nil
	}
	a.downloadHook = func(_ context.Context, tr *downloader.Track, _ any, _ func(int64, int64)) (queue.Result, error) {
		return queue.Result{Format: "FLAC", Path: "/x/" + tr.ID + ".flac"}, nil
	}
	a.ensureQueue()
	if got := a.Submit("https://music.yandex.ru/albums/1"); got != "" {
		t.Fatalf("submit must be accepted, got %q", got)
	}
	waitApp(t, "both tasks done", func() bool { return a.GetQueue().Done == 2 })

	for _, want := range []string{"sources", "task-added", "task-updated", "queue"} {
		if !rec.has(want) {
			t.Fatalf("event %q missing: %v", want, rec.namesCopy())
		}
	}
	if rec.has("job-start") || rec.has("job-end") {
		t.Fatalf("legacy job events must be gone: %v", rec.namesCopy())
	}
	if !logs.has("[downloading] A — T1") || !logs.has("[done] A — T1 (FLAC)") {
		t.Fatalf("journal lines missing: %v", logs.all())
	}
	if !logs.has("[album] Микс (2 треков)") {
		t.Fatalf("source line missing: %v", logs.all())
	}
}

func TestSecondSubmitDoesNotWait(t *testing.T) {
	rec := &eventRecorder{}
	a := newJobApp(t, rec, nil)
	a.cfg.Workers = 2
	a.resolveHook = func(_ context.Context, link string) (queue.Resolved, error) {
		switch link {
		case "https://music.yandex.ru/albums/1":
			return queue.Resolved{Label: "Альбом · One", Kind: "album", Tracks: []*downloader.Track{trk("1", "T1"), trk("2", "T2")}}, nil
		default:
			return queue.Resolved{Label: "Трек · Three", Kind: "track", Tracks: []*downloader.Track{trk("3", "T3")}}, nil
		}
	}
	gate := make(chan struct{})
	a.downloadHook = func(ctx context.Context, tr *downloader.Track, _ any, _ func(int64, int64)) (queue.Result, error) {
		select {
		case <-gate:
			return queue.Result{Format: "FLAC"}, nil
		case <-ctx.Done():
			return queue.Result{}, ctx.Err()
		}
	}
	a.ensureQueue()
	a.Submit("https://music.yandex.ru/albums/1")
	waitApp(t, "first two downloading", func() bool { return a.GetQueue().Active == 2 })

	// Вторая ссылка принимается, пока первая ещё качается.
	if got := a.Submit("https://music.yandex.ru/album/2/track/3"); got != "" {
		t.Fatalf("second submit must be accepted, got %q", got)
	}
	waitApp(t, "third task queued", func() bool {
		s := a.GetQueue()
		return s.Total == 3 && s.Waiting == 1 && s.Active == 2
	})
	close(gate)
	waitApp(t, "queue settles", func() bool { return a.GetQueue().Done == 3 })
}

func TestFallbackReasonReachesJournal(t *testing.T) {
	logs := &logRecorder{}
	a := newJobApp(t, nil, logs)
	a.resolveHook = func(_ context.Context, _ string) (queue.Resolved, error) {
		return queue.Resolved{Label: "Трек · T", Kind: "track", Tracks: []*downloader.Track{trk("1", "T")}}, nil
	}
	a.downloadHook = func(_ context.Context, _ *downloader.Track, _ any, _ func(int64, int64)) (queue.Result, error) {
		return queue.Result{
			Format: "MP3", Fallback: true,
			FallbackReason: "Яндекс вернул nq/aac-mp4 (192 kbps) вместо FLAC",
		}, nil
	}
	a.ensureQueue()
	a.Submit("https://music.yandex.ru/track/1")
	waitApp(t, "done", func() bool { return a.GetQueue().Done == 1 })
	if !logs.has("[done] A — T (MP3) [fallback MP3: Яндекс вернул nq/aac-mp4 (192 kbps) вместо FLAC]") {
		t.Fatalf("fallback line missing: %v", logs.all())
	}
}

// --- настройки ---

func TestGetSettingsDefaults(t *testing.T) {
	a := NewApp() // cfg уже с дефолтами
	s, err := a.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.OutputDir != "./downloads" {
		t.Errorf("OutputDir = %q, want ./downloads", s.OutputDir)
	}
	if s.Workers != 3 {
		t.Errorf("Workers = %d, want 3", s.Workers)
	}
	if !s.ConvertM4A {
		t.Errorf("ConvertM4A must default to true")
	}
	if !s.EmbedCover {
		t.Errorf("EmbedCover must default to true")
	}
}

func TestGetSettingsNormalized(t *testing.T) {
	a := &App{cfg: config.Config{OutputDir: "", Workers: 99}}
	s, err := a.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.OutputDir != "./downloads" || s.Workers != 3 {
		t.Errorf("invalid values must normalize to defaults, got %+v", s)
	}
}

func TestCheckTokenEmpty(t *testing.T) {
	a := &App{}
	if _, err := a.CheckToken("   "); err == nil || !strings.Contains(err.Error(), "пуст") {
		t.Errorf("empty token must fail in Russian, got %v", err)
	}
}

func TestSaveSettingsValidation(t *testing.T) {
	a := NewApp()
	a.envPath = filepath.Join(t.TempDir(), ".env")

	if err := a.SaveSettings(Settings{OutputDir: "  ", Workers: 3}); err == nil ||
		!strings.Contains(err.Error(), "папка") {
		t.Errorf("empty output dir must be rejected, got %v", err)
	}
	if err := a.SaveSettings(Settings{OutputDir: "./d", Workers: 0}); err == nil ||
		!strings.Contains(err.Error(), "1") {
		t.Errorf("workers out of range must be rejected, got %v", err)
	}
	if err := a.SaveSettings(Settings{OutputDir: "./d", Workers: 11}); err == nil {
		t.Errorf("workers above max must be rejected")
	}
}

func TestSaveSettingsWritesAndApplies(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	body := "# комментарий пользователя\nYANDEX_TOKEN=old\nSTRANGE_KEY=keep\n"
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.envPath = p

	err := a.SaveSettings(Settings{
		Token:      "", // пустой токен — без сетевой валидации
		OutputDir:  "D:/Музыка",
		LibraryDir: "",
		Workers:    7,
		ConvertM4A: false,
		EmbedCover: false, // сняли галку — обложку не встраиваем
	})
	if err != nil {
		t.Fatal(err)
	}
	// Файл сохранил комментарий и чужой ключ.
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, "# комментарий пользователя") || !strings.Contains(text, "STRANGE_KEY=keep") {
		t.Fatalf("comments and unknown keys must survive:\n%s", text)
	}
	if !strings.Contains(text, "PLAYLIST_WORKERS=7") || !strings.Contains(text, "EMBED_COVER=false") {
		t.Fatalf("new values missing:\n%s", text)
	}
	// Хот-релоад: состояние применено без перезапуска.
	if a.outputDir != "D:/Музыка" {
		t.Errorf("outputDir not applied: %q", a.outputDir)
	}
	if a.cfg.Workers != 7 || a.cfg.EmbedCover || a.cfg.ConvertM4A {
		t.Errorf("cfg not applied: %+v", a.cfg)
	}
	if a.clientSnapshot() != nil {
		t.Errorf("empty token must leave the app in preview-only mode")
	}
	// Повторное чтение видит те же значения.
	s, err := a.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.Workers != 7 || s.EmbedCover || s.OutputDir != "D:/Музыка" {
		t.Errorf("round-trip mismatch: %+v", s)
	}
}

// Настройки доступны и применяются, пока очередь работает: активная
// задача продолжает качаться, новые берут новые значения.
func TestSettingsAvailableWhileQueueRuns(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("OUTPUT_DIR="+dir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rec := &eventRecorder{}
	a := newJobApp(t, rec, nil)
	a.envPath = env
	a.resolveHook = func(_ context.Context, _ string) (queue.Resolved, error) {
		return queue.Resolved{Label: "Трек · T", Kind: "track", Tracks: []*downloader.Track{trk("1", "T")}}, nil
	}
	gate := make(chan struct{})
	a.downloadHook = func(ctx context.Context, _ *downloader.Track, _ any, _ func(int64, int64)) (queue.Result, error) {
		select {
		case <-gate:
			return queue.Result{Format: "FLAC"}, nil
		case <-ctx.Done():
			return queue.Result{}, ctx.Err()
		}
	}
	a.ensureQueue()
	a.Submit("https://music.yandex.ru/track/1")
	waitApp(t, "active", func() bool { return a.GetQueue().Active == 1 })

	if _, err := a.GetSettings(); err != nil {
		t.Fatalf("settings must open during a download: %v", err)
	}
	if err := a.SaveSettings(Settings{OutputDir: dir, Workers: 2, ConvertM4A: true, EmbedCover: true, EmbedLyrics: true, SubdirTemplate: "%artist%/%album%"}); err != nil {
		t.Fatalf("settings must save during a download: %v", err)
	}
	if got := a.GetQueue(); got.Active != 1 {
		t.Fatalf("active download must survive a settings save, got %+v", got)
	}
	if got := rec.lastQueueWorkers(); got != 2 {
		t.Fatalf("settings save must emit the new worker limit, got %d", got)
	}
	close(gate)
	waitApp(t, "done", func() bool { return a.GetQueue().Done == 1 })
}
