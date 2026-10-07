package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yamdl/internal/config"
	"yamdl/internal/downloader"
)

// eventRecorder captures named frontend events in order.
type eventRecorder struct {
	names []string
	args  []any
}

func (r *eventRecorder) sink(name string, args ...any) {
	r.names = append(r.names, name)
	r.args = append(r.args, args)
}

func (r *eventRecorder) find(name string) int {
	for i, n := range r.names {
		if n == name {
			return i
		}
	}
	return -1
}

func (r *eventRecorder) all(name string) []int {
	var out []int
	for i, n := range r.names {
		if n == name {
			out = append(out, i)
		}
	}
	return out
}

// arg returns the arguments captured for event number i.
func (r *eventRecorder) arg(i int) []any {
	return r.args[i].([]any)
}

// --- 2.1/2.3: жизненный цикл плейлиста: job-start -> строки -> job-end ---

func TestPlaylistJobLifecycle(t *testing.T) {
	var rec eventRecorder
	a := &App{
		client:    downloader.NewClient(""),
		eventSink: rec.sink,
		logSink:   func(text, kind string) {},
		fetchPlaylist: func(string) (*downloader.Playlist, error) {
			return &downloader.Playlist{
				Title: "Микс",
				Tracks: []*downloader.Track{
					{ID: "1", Artists: []string{"A"}, Title: "T1", Available: true},
					{ID: "2", Artists: []string{"B"}, Title: "T2", Available: true},
				},
			}, nil
		},
		downloadBatch: func(tracks []*downloader.Track, _ string, emit func(downloader.Event), onProgress downloader.ProgressFunc) downloader.Summary {
			var sum downloader.Summary
			for _, tr := range tracks {
				emit(downloader.Event{Kind: downloader.KindDownloading, Label: tr.Label(), TrackID: tr.ID})
				if onProgress != nil {
					onProgress(downloader.Progress{TrackID: tr.ID, Label: tr.Label(), Done: 50, Total: 100, Percent: 50})
				}
				emit(downloader.Event{Kind: downloader.KindDone, Label: tr.Label(), TrackID: tr.ID, Format: "FLAC"})
				sum.OK++
			}
			return sum
		},
	}
	got := a.Download("https://music.yandex.ru/playlists/11111111-2222-3333-4444-555555555555")
	if got != "" {
		t.Fatalf("playlist outcome must be in the log, got %q", got)
	}
	iStart := rec.find("job-start")
	iEnd := rec.find("job-end")
	iTracks := rec.find("tracks")
	if iStart < 0 || iEnd < 0 || iTracks < 0 {
		t.Fatalf("lifecycle events missing: %v", rec.names)
	}
	if iStart > iTracks {
		t.Errorf("job-start must come before tracks: %v", rec.names)
	}
	if iEnd < iTracks {
		t.Errorf("job-end must come after tracks: %v", rec.names)
	}
	if rec.arg(iStart)[0] != "Микс" || rec.arg(iStart)[1] != 2 {
		t.Errorf("job-start args = %v, want [Микс 2]", rec.arg(iStart))
	}
	// Итог: 2 ok, 0 skip, 0 error.
	if rec.arg(iEnd)[0] != 2 || rec.arg(iEnd)[1] != 0 || rec.arg(iEnd)[2] != 0 {
		t.Errorf("job-end args = %v, want [2 0 0]", rec.arg(iEnd))
	}
	if len(rec.all("job-start")) != 1 || len(rec.all("job-end")) != 1 {
		t.Errorf("exactly one job per run, got %v", rec.names)
	}
}

// --- 2.2: одиночный трек — та же job'а из одного трека ---

func TestSingleTrackJobLifecycle(t *testing.T) {
	var rec eventRecorder
	var logs []string
	tr := &downloader.Track{ID: "9", Artists: []string{"A"}, Title: "Solo", Available: true}
	a := &App{
		client:    downloader.NewClient(""),
		eventSink: rec.sink,
		logSink:   func(text, kind string) { logs = append(logs, text) },
		downloadOne: func(got *downloader.Track, _ string, emit func(downloader.Event), onProgress downloader.ProgressFunc) error {
			if got.ID != "9" {
				t.Errorf("wrong track: %s", got.ID)
			}
			emit(downloader.Event{Kind: downloader.KindDownloading, Label: got.Label(), TrackID: got.ID})
			if onProgress == nil {
				t.Error("single track must receive a progress callback, got nil")
			} else {
				onProgress(downloader.Progress{TrackID: got.ID, Label: got.Label(), Done: 5, Total: 10, Percent: 50})
			}
			emit(downloader.Event{Kind: downloader.KindDone, Label: got.Label(), TrackID: got.ID, Format: "FLAC"})
			return nil
		},
	}
	// DownloadTrack идёт через ResolveTrack — подменяем fetch через downloadOne,
	// поэтому зовём внутренний путь после резолва: используем Download c подменой
	// клиента недоступной... Проще: зовём downloadSingle напрямую.
	got := a.downloadSingle(tr)
	if got != "" {
		t.Fatalf("done outcome must be shown in the log, got %q", got)
	}
	iStart := rec.find("job-start")
	iEnd := rec.find("job-end")
	iTracks := rec.find("tracks")
	iStatus := rec.find("track-status")
	iProgress := rec.find("progress")
	if iStart < 0 || iEnd < 0 || iTracks < 0 || iStatus < 0 || iProgress < 0 {
		t.Fatalf("single-track events missing: %v", rec.names)
	}
	if !(iStart < iTracks && iTracks < iStatus && iStatus < iEnd) {
		t.Errorf("event order wrong: %v", rec.names)
	}
	if rec.arg(iStart)[0] != "" || rec.arg(iStart)[1] != 1 {
		t.Errorf("job-start args = %v, want empty title and count 1", rec.arg(iStart))
	}
	if rec.arg(iEnd)[0] != 1 || rec.arg(iEnd)[1] != 0 || rec.arg(iEnd)[2] != 0 {
		t.Errorf("job-end args = %v, want [1 0 0]", rec.arg(iEnd))
	}
	// Лог по-прежнему получает полные строки для одиночного трека.
	var sawDownloading, sawDone bool
	for _, l := range logs {
		if l == "[downloading] A — Solo" {
			sawDownloading = true
		}
		if l == "[done] A — Solo (FLAC)" {
			sawDone = true
		}
	}
	if !sawDownloading || !sawDone {
		t.Fatalf("log lines required by desktop-app spec missing: %v", logs)
	}
}

// --- 2.1: счёт итогов по событиям ---

func TestCountOutcome(t *testing.T) {
	var sum downloader.Summary
	countOutcome(&sum, downloader.Event{Kind: downloader.KindDone})
	countOutcome(&sum, downloader.Event{Kind: downloader.KindDone})
	countOutcome(&sum, downloader.Event{Kind: downloader.KindSkipped})
	countOutcome(&sum, downloader.Event{Kind: downloader.KindFailed})
	countOutcome(&sum, downloader.Event{Kind: downloader.KindStopped})
	countOutcome(&sum, downloader.Event{Kind: downloader.KindDownloading}) // не терминальное
	if sum.OK != 2 || sum.Skipped != 1 || sum.Failed != 1 || sum.Stopped != 1 {
		t.Fatalf("summary = %+v, want {OK:2 Skipped:1 Failed:1 Stopped:1}", sum)
	}
}

// --- 4.1/4.2: настройки ---

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
	if a.client != nil {
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

// --- 4.3: настройки закрыты на время скачивания ---

func TestSettingsLockedWhileBusy(t *testing.T) {
	a := NewApp()
	a.setBusy(true)
	if _, err := a.GetSettings(); err == nil || !strings.Contains(err.Error(), "скачивание") {
		t.Errorf("GetSettings must be locked while busy, got %v", err)
	}
	if err := a.SaveSettings(Settings{OutputDir: "./x", Workers: 3}); err == nil ||
		!strings.Contains(err.Error(), "скачивание") {
		t.Errorf("SaveSettings must be locked while busy, got %v", err)
	}
	a.setBusy(false)
	if _, err := a.GetSettings(); err != nil {
		t.Errorf("unlocked after download: %v", err)
	}
}

// --- 2.3: busy поднимается на время Download и опускается после ---

func TestBusyAroundDownload(t *testing.T) {
	var rec eventRecorder
	a := &App{
		client:    downloader.NewClient(""),
		eventSink: rec.sink,
		logSink:   func(text, kind string) {},
	}
	a.downloadOne = func(*downloader.Track, string, func(downloader.Event), downloader.ProgressFunc) error {
		if !a.isBusy() {
			t.Error("busy must be set while downloading")
		}
		return nil
	}
	if got := a.Download("not a link"); !strings.HasPrefix(got, "[error]") {
		t.Fatalf("bad link must fail, got %q", got)
	}
	if a.isBusy() {
		t.Error("busy must be cleared after a failed download")
	}
}
