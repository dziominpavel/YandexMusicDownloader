package ui

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"yamdl/internal/config"
	"yamdl/internal/downloader"
	"yamdl/internal/library"
	"yamdl/internal/quality"
	"yamdl/internal/tagger"
)

func qualityIndex(t *testing.T, dir, trackID string) *library.Index {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "tagger", "testdata", "mp3-id3v24.mp3"))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "Artist - Song.mp3")
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tagger.Write(p, tagger.Meta{
		Title: "Song", Artists: []string{"Artist"}, TrackID: trackID,
		SourceURL: "https://music.yandex.ru/album/1/track/" + trackID,
	}); err != nil {
		t.Fatal(err)
	}
	ix := library.New(filepath.Join(t.TempDir(), "cache.json"), dir)
	ix.Refresh(nil)
	return ix
}

func qualityTestServer(t *testing.T, codec string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/account/status":
			_, _ = w.Write([]byte(`{"result":{"account":{"uid":42},"plus":{"hasPlus":true}}}`))
		case "/get-file-info":
			_, _ = w.Write([]byte(`{"result":{"downloadInfo":{"quality":"lossless","codec":"` + codec + `"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func newQualityApp(t *testing.T, codec string) (*App, *capturedEvents) {
	t.Helper()
	dir := t.TempDir()
	ix := qualityIndex(t, dir, "1")
	srv := qualityTestServer(t, codec)
	t.Cleanup(srv.Close)
	client := downloader.NewClient("t")
	client.BaseURL = srv.URL
	j := quality.NewJournal(filepath.Join(t.TempDir(), "flac-quality.json"))
	a := &App{
		client: client, index: ix, qualityJournal: j, outputDir: dir,
		cfg: config.Config{OutputDir: dir, FlacMonitor: true},
	}
	ev := &capturedEvents{}
	a.eventSink = ev.emit
	a.logSink = func(string, string) {}
	return a, ev
}

type capturedEvents struct {
	mu    sync.Mutex
	names []string
	found []quality.FoundItem
	text  string
	err   string
}

func (c *capturedEvents) emit(name string, args ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.names = append(c.names, name)
	switch name {
	case "quality-found", "quality-pending":
		if len(args) > 0 {
			if items, ok := args[0].([]quality.FoundItem); ok {
				c.found = items
			}
		}
	case "quality-check-finished":
		if len(args) > 0 {
			c.text, _ = args[0].(string)
		}
		if len(args) > 2 {
			c.err, _ = args[2].(string)
		}
	}
}

func (c *capturedEvents) has(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, n := range c.names {
		if n == name {
			return true
		}
	}
	return false
}

func TestCheckQualityFindsLossless(t *testing.T) {
	a, ev := newQualityApp(t, "flac")
	res, err := a.CheckQuality()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Found) != 1 || res.Found[0].TrackID != "1" || !res.Found[0].Selected {
		t.Fatalf("unexpected found: %+v", res.Found)
	}
	if !ev.has("quality-found") {
		t.Fatalf("quality-found event missing: %v", ev.names)
	}
	if len(a.PendingFound()) != 1 {
		t.Fatalf("pending list must contain the find: %+v", a.PendingFound())
	}
	st := a.QualityStatus()
	if st.Pending != 1 || st.Found != 1 || !strings.Contains(st.Text, "найдено 1") {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestCheckQualityNoFindings(t *testing.T) {
	a, ev := newQualityApp(t, "mp3")
	res, err := a.CheckQuality()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Found) != 0 {
		t.Fatalf("mp3 must not be offered: %+v", res.Found)
	}
	if ev.has("quality-found") {
		t.Fatalf("empty check must not open the found window: %v", ev.names)
	}
	st := a.QualityStatus()
	if st.Found != 0 || !strings.Contains(st.Text, "ничего не найдено") {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestQualityStatusStates(t *testing.T) {
	a := &App{qualityJournal: quality.NewJournal("")}
	if got := a.QualityStatus().Text; got != "FLAC: проверок не было" {
		t.Fatalf("fresh status = %q", got)
	}

	a.qualityJournal.Finish(timeNow(), 3, nil)
	if got := a.QualityStatus().Text; !strings.Contains(got, "найдено 3") {
		t.Fatalf("found status = %q", got)
	}
	a.qualityJournal.Finish(timeNow(), 0, errors.New("boom"))
	if got := a.QualityStatus().Text; !strings.Contains(got, "ошибка") {
		t.Fatalf("error status = %q", got)
	}

	a.qualityMu.Lock()
	a.checking = true
	a.qualityMu.Unlock()
	if got := a.QualityStatus().Text; !strings.Contains(got, "проверка идёт") {
		t.Fatalf("checking status = %q", got)
	}
}

func TestMaybeAutoCheckRunsWhenDue(t *testing.T) {
	a, ev := newQualityApp(t, "flac")
	a.maybeAutoCheck()
	deadline := time.Now().Add(3 * time.Second)
	for !ev.has("quality-check-finished") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !ev.has("quality-check-finished") {
		t.Fatalf("auto-check did not finish: %v", ev.names)
	}
	if len(a.PendingFound()) != 1 {
		t.Fatalf("auto-check must store the find: %+v", a.PendingFound())
	}
}

func TestMaybeAutoCheckSkipsBusyAndNotDue(t *testing.T) {
	dir := t.TempDir()
	ix := qualityIndex(t, dir, "1")
	j := quality.NewJournal("")
	a := &App{
		busy: true, index: ix, qualityJournal: j,
		client: downloader.NewClient(""),
		cfg:    config.Config{FlacMonitor: true},
	}
	a.maybeAutoCheck()
	if a.isChecking() {
		t.Fatal("busy app must not start an auto-check")
	}

	a.busy = false
	j.Finish(timeNow(), 0, nil) // next check in 7 days
	a.maybeAutoCheck()
	if a.isChecking() {
		t.Fatal("auto-check must wait for the 7-day interval")
	}
}

func TestSaveSettingsFlacMonitor(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("# keep\nOUTPUT_DIR="+dir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &App{envPath: env, outputDir: dir, cfg: config.Config{
		OutputDir: dir, Workers: 3, ConvertM4A: true, EmbedCover: true,
		EmbedLyrics: true, SubdirTemplate: "%artist%/%album%",
	}}
	err := a.SaveSettings(Settings{
		OutputDir: dir, Workers: 3, ConvertM4A: true, EmbedCover: true,
		EmbedLyrics: true, SubdirTemplate: "%artist%/%album%", FlacMonitor: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(env)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.FlacMonitor {
		t.Fatal("FLAC_MONITOR must be written and reloaded")
	}
	if got := a.GetSettingsOrFail(t); !got.FlacMonitor {
		t.Fatal("settings must expose the enabled monitor")
	}
}

func (a *App) GetSettingsOrFail(t *testing.T) Settings {
	t.Helper()
	s, err := a.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDownloadFoundArchivesExistingFlac(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "Artist - Song.mp3")
	flac := filepath.Join(dir, "Artist - Song.flac")
	for _, p := range []string{old, flac} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ix := library.New(filepath.Join(t.TempDir(), "cache.json"), dir)
	ix.Refresh(nil)
	j := quality.NewJournal("")
	item := quality.FoundItem{
		Key: old, Path: old, FileName: filepath.Base(old), Label: "Artist — Song",
		CurrentFormat: "MP3", FoundFormat: "FLAC", MatchSource: "id", TrackID: "42",
		Selected: true, AlreadyFlac: flac,
	}
	a := &App{
		client: downloader.NewClient(""), index: ix, qualityJournal: j,
		outputDir: dir, found: []quality.FoundItem{item},
	}
	var events []string
	a.eventSink = func(name string, args ...any) { events = append(events, name) }
	a.logSink = func(string, string) {}

	if err := a.DownloadFound([]quality.FoundItem{item}); err != nil {
		t.Fatal(err)
	}
	if a.isBusy() {
		t.Fatal("busy must be cleared after the job")
	}
	found := false
	for _, e := range events {
		if e == "job-start" {
			found = true
		}
	}
	if !found {
		t.Fatalf("job-start event missing: %v", events)
	}
	if _, err := os.Stat(filepath.Join(dir, "_old", "Artist - Song.mp3")); err != nil {
		t.Fatalf("old MP3 must be archived: %v", err)
	}
	if len(a.PendingFound()) != 0 {
		t.Fatalf("nothing must remain pending: %+v", a.PendingFound())
	}
}

func TestDownloadFoundGuards(t *testing.T) {
	if err := (&App{}).DownloadFound(nil); err == nil {
		t.Fatal("empty selection must fail")
	}
	if err := (&App{busy: true}).DownloadFound([]quality.FoundItem{{Path: "x", Key: "x"}}); err == nil {
		t.Fatal("busy app must reject the upgrade")
	}
	if err := (&App{}).DownloadFound([]quality.FoundItem{{Path: "x", Key: "x"}}); err == nil {
		t.Fatal("missing client must reject the upgrade")
	}
}

func timeNow() time.Time {
	return time.Now()
}
