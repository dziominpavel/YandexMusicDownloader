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
	return newQualityAppWithServer(t, qualityTestServer(t, codec))
}

func newQualityAppWithServer(t *testing.T, srv *httptest.Server) (*App, *capturedEvents) {
	t.Helper()
	dir := t.TempDir()
	ix := qualityIndex(t, dir, "1")
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

func (c *capturedEvents) finishedText() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.text
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
	if got := ev.finishedText(); !strings.Contains(got, "найдено 1") || strings.Contains(got, "идёт") {
		t.Fatalf("finished event must carry the result, got %q", got)
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
	if got := ev.finishedText(); !strings.Contains(got, "ничего не найдено") || strings.Contains(got, "идёт") {
		t.Fatalf("finished event must carry the result, got %q", got)
	}
}

func TestQualityFinishedEventTextError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/account/status":
			_, _ = w.Write([]byte(`{"result":{"account":{"uid":42},"plus":{"hasPlus":true}}}`))
		case "/get-file-info":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			http.NotFound(w, r)
		}
	}))
	a, ev := newQualityAppWithServer(t, srv)
	if _, err := a.CheckQuality(); err == nil {
		t.Fatal("auth error must be returned")
	}
	if got := ev.finishedText(); !strings.Contains(got, "ошибка") || strings.Contains(got, "идёт") {
		t.Fatalf("finished event must report an error, got %q", got)
	}
}

func TestQualityFinishedEventTextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/account/status":
			_, _ = w.Write([]byte(`{"result":{"account":{"uid":42},"plus":{"hasPlus":true}}}`))
		case "/get-file-info":
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	a, ev := newQualityAppWithServer(t, srv)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = a.CheckQuality()
	}()
	deadline := time.Now().Add(3 * time.Second)
	for !ev.has("quality-check-started") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	a.cancelQualityCheck()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("check did not stop after cancel")
	}
	if got := ev.finishedText(); !strings.Contains(got, "отменена") || strings.Contains(got, "идёт") {
		t.Fatalf("finished event must report cancellation, got %q", got)
	}
}

func TestQualityStatusStates(t *testing.T) {
	a := &App{qualityJournal: quality.NewJournal("")}
	if got := a.QualityStatus().Text; got != "FLAC: проверок не было" {
		t.Fatalf("fresh status = %q", got)
	}
	a.outputDir = "C:/downloads"
	if got := a.QualityStatus().OutputDir; got != "C:/downloads" {
		t.Fatalf("status must expose the output dir, got %q", got)
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
	// SaveSettings starts the library index refresh in the background; wait for
	// its atomic cache write before t.TempDir cleanup removes the directory.
	t.Cleanup(func() {
		cache := filepath.Join(dir, "library-index.json")
		for i := 0; i < 200; i++ {
			if _, err := os.Stat(cache); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
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
	var finishedText string
	a.eventSink = func(name string, args ...any) {
		events = append(events, name)
		if name == "quality-check-finished" && len(args) > 0 {
			finishedText, _ = args[0].(string)
		}
	}
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
	if strings.Contains(finishedText, "идёт") || finishedText == "" {
		t.Fatalf("finished event must carry the final status, got %q", finishedText)
	}
}

func TestDownloadFoundKeepsFinalStatusAfterPartialFailure(t *testing.T) {
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
	okItem := quality.FoundItem{
		Key: old, Path: old, FileName: filepath.Base(old), Label: "Artist — Song",
		CurrentFormat: "MP3", FoundFormat: "FLAC", MatchSource: "id", TrackID: "42",
		Selected: true, AlreadyFlac: flac,
	}
	badItem := quality.FoundItem{
		Key: "bad", Path: filepath.Join(dir, "bad.mp3"), FileName: "bad.mp3",
		Label: "Bad", CurrentFormat: "MP3", FoundFormat: "FLAC", Selected: true,
	}
	a := &App{
		client: downloader.NewClient(""), index: ix, qualityJournal: j,
		outputDir: dir, found: []quality.FoundItem{okItem, badItem},
	}
	var finishedText string
	a.eventSink = func(name string, args ...any) {
		if name == "quality-check-finished" && len(args) > 0 {
			finishedText, _ = args[0].(string)
		}
	}
	a.logSink = func(string, string) {}

	if err := a.DownloadFound([]quality.FoundItem{okItem, badItem}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "_old", "Artist - Song.mp3")); err != nil {
		t.Fatalf("successful item must be archived: %v", err)
	}
	if strings.Contains(finishedText, "идёт") || finishedText == "" {
		t.Fatalf("finished event must carry the final status, got %q", finishedText)
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

func newSearchQualityApp(t *testing.T) (*App, *capturedEvents, quality.FoundItem, string) {
	t.Helper()
	dir := t.TempDir()
	ix := qualityIndex(t, dir, "")
	path := filepath.Join(dir, "Artist - Song.mp3")
	j := quality.NewJournal(filepath.Join(t.TempDir(), "flac-quality.json"))
	item := quality.FoundItem{
		Key: path, Path: path, FileName: filepath.Base(path), Label: "Artist — Song",
		CurrentFormat: "MP3", FoundFormat: "FLAC", MatchSource: "search", TrackID: "42",
		MatchedArtist: "Artist", MatchedTitle: "Song (Remix)", Album: "Album", Year: "2020",
		Confidence: 100, Selected: true, Codec: "flac",
	}
	j.Put("file:"+path, quality.Entry{
		Status: quality.StatusLossless, Codec: "flac", MatchSource: "search",
		MatchedTrackID: "42", MatchedArtist: "Artist", MatchedTitle: "Song (Remix)",
		MatchedAlbum: "Album", MatchedYear: "2020", Confidence: 100, Exact: true,
	})
	a := &App{
		index: ix, qualityJournal: j, outputDir: dir,
		found: []quality.FoundItem{item},
		cfg:   config.Config{OutputDir: dir},
	}
	ev := &capturedEvents{}
	a.eventSink = ev.emit
	a.logSink = func(string, string) {}
	return a, ev, item, path
}

func TestRejectFoundKeepsLocalFileAndClearsLossless(t *testing.T) {
	a, ev, item, path := newSearchQualityApp(t)
	remaining, err := a.RejectFound(item)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 || len(a.PendingFound()) != 0 {
		t.Fatalf("rejected item must leave the list: %+v / %+v", remaining, a.PendingFound())
	}
	if !a.qualityJournal.IsDeclined(path, "42") {
		t.Fatal("rejection must be stored")
	}
	if _, ok := a.qualityJournal.Get("file:" + path); ok {
		t.Fatal("lossless entry must be cleared so the file is rechecked")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("local MP3 must not be touched: %v", err)
	}
	list := a.qualityJournal.DeclinedList()
	if len(list) != 1 || list[0].MatchedTitle != "Song (Remix)" || list[0].LocalLabel != "Artist — Song" {
		t.Fatalf("rejection metadata lost: %+v", list)
	}
	if !ev.has("quality-pending") || len(ev.found) != 0 {
		t.Fatalf("pending event must be emitted with the remaining list: %+v", ev.found)
	}
	s := a.GetSettingsOrFail(t)
	if len(s.QualityRejected) != 1 || s.QualityRejected[0].TrackID != "42" {
		t.Fatalf("settings must expose the rejected list: %+v", s.QualityRejected)
	}
}

func TestUndoRejectRestoresImmediateOffer(t *testing.T) {
	a, ev, item, path := newSearchQualityApp(t)
	if _, err := a.RejectFound(item); err != nil {
		t.Fatal(err)
	}
	remaining, err := a.UndoReject(path, "42")
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].TrackID != "42" {
		t.Fatalf("undo must restore the offer: %+v", remaining)
	}
	if a.qualityJournal.IsDeclined(path, "42") {
		t.Fatal("rejection must be removed")
	}
	e, ok := a.qualityJournal.Get("file:" + path)
	if !ok || e.Status != quality.StatusLossless || e.MatchedTrackID != "42" {
		t.Fatalf("lossless entry must be restored: %+v ok=%v", e, ok)
	}
	if len(a.GetSettingsOrFail(t).QualityRejected) != 0 {
		t.Fatal("settings list must be empty after undo")
	}
	if !ev.has("quality-pending") || len(ev.found) != 1 {
		t.Fatalf("undo must emit the restored list: %+v", ev.found)
	}
}

func TestUndoRejectWithoutMetadataDoesNotInventOffer(t *testing.T) {
	a, _, _, path := newSearchQualityApp(t)
	a.found = nil
	a.qualityJournal.Forget("file:" + path)
	a.qualityJournal.Decline(path, quality.Declined{TrackID: "42", LocalLabel: "Artist — Song"})
	remaining, err := a.UndoReject(path, "42")
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("metadata-less undo must wait for the next check: %+v", remaining)
	}
	if _, ok := a.qualityJournal.Get("file:" + path); ok {
		t.Fatal("no lossless entry may be invented")
	}
}

func TestRejectFoundGuards(t *testing.T) {
	a, _, item, path := newSearchQualityApp(t)
	if _, err := (&App{}).RejectFound(item); err == nil {
		t.Fatal("missing journal must fail")
	}
	if _, err := (&App{busy: true}).RejectFound(item); err == nil {
		t.Fatal("busy app must reject")
	}
	a.qualityMu.Lock()
	a.checking = true
	a.qualityMu.Unlock()
	if _, err := a.RejectFound(item); err == nil {
		t.Fatal("checking app must reject")
	}
	a.qualityMu.Lock()
	a.checking = false
	a.qualityMu.Unlock()
	idItem := item
	idItem.MatchSource = "id"
	if _, err := a.RejectFound(idItem); err == nil {
		t.Fatal("ID match must not be rejectable")
	}
	stale := item
	stale.TrackID = "99"
	if _, err := a.RejectFound(stale); err == nil {
		t.Fatal("stale item must be rejected")
	}
	if _, err := a.UndoReject(path, "99"); err == nil {
		t.Fatal("missing rejection must be reported")
	}
}

func TestQualityRejectAndNextCandidateIntegration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/account/status":
			_, _ = w.Write([]byte(`{"result":{"account":{"uid":42},"plus":{"hasPlus":true}}}`))
		case "/search":
			_, _ = w.Write([]byte(`{"result":{"tracks":{"results":[
				{"id":42,"title":"Song","available":true,"artists":[{"name":"Artist"}],"albums":[{"id":1,"title":"Album","year":2020}]},
				{"id":43,"title":"Song","available":true,"artists":[{"name":"Artist"}],"albums":[{"id":1,"title":"Album","year":2020}]}
			]}}}`))
		case "/get-file-info":
			_, _ = w.Write([]byte(`{"result":{"downloadInfo":{"quality":"lossless","codec":"flac"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	ix := qualityIndex(t, dir, "")
	client := downloader.NewClient("t")
	client.BaseURL = srv.URL
	j := quality.NewJournal(filepath.Join(t.TempDir(), "flac-quality.json"))
	a := &App{
		client: client, index: ix, qualityJournal: j, outputDir: dir,
		cfg: config.Config{OutputDir: dir, FlacMonitor: true},
	}
	a.eventSink = func(string, ...any) {}
	a.logSink = func(string, string) {}

	res, err := a.CheckQuality()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Found) != 1 || res.Found[0].TrackID != "42" {
		t.Fatalf("first check must find 42: %+v", res.Found)
	}
	item := res.Found[0]
	remaining, err := a.RejectFound(item)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("rejected item must disappear: %+v", remaining)
	}

	res, err = a.CheckQuality()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Found) != 1 || res.Found[0].TrackID != "43" {
		t.Fatalf("next check must find the other version: %+v", res.Found)
	}
	if !j.IsDeclined(item.Path, "42") {
		t.Fatal("42 must stay rejected")
	}

	remaining, err = a.UndoReject(item.Path, "42")
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].TrackID != "42" {
		t.Fatalf("undo must restore 42: %+v", remaining)
	}
	if j.IsDeclined(item.Path, "42") {
		t.Fatal("42 must be allowed again")
	}
}

func timeNow() time.Time {
	return time.Now()
}
