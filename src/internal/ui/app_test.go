package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yamdl/internal/downloader"
	"yamdl/internal/library"
)

// testIndex builds an index over tagless files named "Artist - Title.ext".
func testIndex(t *testing.T, names ...string) *library.Index {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ix := library.New(filepath.Join(t.TempDir(), "cache.json"), dir)
	ix.Refresh(nil)
	return ix
}

func TestFormatEvent(t *testing.T) {
	cases := []struct {
		in   downloader.Event
		text string
		kind string
	}{
		{downloader.Event{Kind: downloader.KindDownloading, Label: "A — T"}, "[downloading] A — T", "info"},
		{downloader.Event{Kind: downloader.KindDone, Label: "A — T", Format: "FLAC"}, "[done] A — T (FLAC)", "done"},
		{downloader.Event{Kind: downloader.KindDone, Label: "A — T", Format: "MP3", Fallback: true}, "[done] A — T (MP3) [fallback MP3]", "done"},
		{downloader.Event{Kind: downloader.KindDone, Label: "A — T", Format: "FLAC", Converted: true}, "[done] A — T (FLAC) [ALAC→FLAC]", "done"},
		{downloader.Event{Kind: downloader.KindSkipped, Label: "A — T"}, "[skip] A — T (already exists)", "info"},
		{downloader.Event{Kind: downloader.KindSkipped, Label: "A — T", Detail: downloader.SkipDuplicate}, "[skip] A — T (duplicate)", "info"},
		{downloader.Event{Kind: downloader.KindSkipped, Label: "A — T", Detail: downloader.SkipUnavailable}, "[skip] A — T (unavailable)", "info"},
		{downloader.Event{Kind: downloader.KindFailed, Label: "A — T", Detail: "boom"}, "[error] A — T: boom", "err"},
	}
	for _, c := range cases {
		text, kind := formatEvent(c.in)
		if text != c.text || kind != c.kind {
			t.Errorf("got (%q,%q) want (%q,%q)", text, kind, c.text, c.kind)
		}
	}
}

func TestFormatTrackStatus(t *testing.T) {
	cases := []struct {
		in   downloader.Event
		text string
		kind string
	}{
		{downloader.Event{Kind: downloader.KindDownloading}, "[downloading]", "info"},
		{downloader.Event{Kind: downloader.KindDone, Format: "FLAC"}, "[done] (FLAC)", "done"},
		{downloader.Event{Kind: downloader.KindDone, Format: "MP3", Fallback: true}, "[done] (MP3) [fallback MP3]", "done"},
		{downloader.Event{Kind: downloader.KindDone, Format: "FLAC", Converted: true}, "[done] (FLAC) [ALAC→FLAC]", "done"},
		{downloader.Event{Kind: downloader.KindSkipped}, "[skip] (already exists)", "info"},
		{downloader.Event{Kind: downloader.KindSkipped, Detail: downloader.SkipDuplicate}, "[skip] (duplicate)", "info"},
		{downloader.Event{Kind: downloader.KindSkipped, Detail: downloader.SkipUnavailable}, "[skip] (unavailable)", "info"},
		{downloader.Event{Kind: downloader.KindFailed, Detail: "boom"}, "[error] boom", "err"},
	}
	for _, c := range cases {
		text, kind := formatTrackStatus(c.in)
		if text != c.text || kind != c.kind {
			t.Errorf("got (%q,%q) want (%q,%q)", text, kind, c.text, c.kind)
		}
	}
}

func TestDownloadWithoutToken(t *testing.T) {
	a := &App{}
	got := a.Download("https://music.yandex.ru/album/1/track/2")
	if !strings.HasPrefix(got, "[error]") {
		t.Fatalf("want [error], got %q", got)
	}
}

func TestDownloadBadLink(t *testing.T) {
	a := &App{client: downloader.NewClient("")}
	got := a.Download("not a link")
	if !strings.HasPrefix(got, "[error]") {
		t.Fatalf("want [error], got %q", got)
	}
}

// --- 4.1: ветвление подтверждения через подмену диалога ---

func TestCheckDuplicateBranching(t *testing.T) {
	ix := testIndex(t, "Исполнитель A - Песня.flac")
	var asked []string
	a := &App{index: ix, confirm: func(msg string) bool {
		asked = append(asked, msg)
		return false
	}}

	match := &downloader.Track{Artists: []string{"Исполнитель A"}, Title: "Песня"}
	if a.checkDuplicate(match) {
		t.Fatal("declined duplicate must not proceed")
	}
	if len(asked) != 1 {
		t.Fatalf("want exactly one dialog, got %d", len(asked))
	}
	for _, want := range []string{"Возможен дубль", "100%", "Песня.flac", "Скачать всё равно"} {
		if !strings.Contains(asked[0], want) {
			t.Errorf("dialog text must contain %q, got:\n%s", want, asked[0])
		}
	}

	// Подтверждение → скачивание продолжается.
	a.confirm = func(string) bool { return true }
	if !a.checkDuplicate(match) {
		t.Fatal("confirmed duplicate must proceed")
	}

	// Нет совпадений → диалога нет вообще.
	asked = nil
	cold := &downloader.Track{Artists: []string{"Другой"}, Title: "Совсем иное"}
	if !a.checkDuplicate(cold) {
		t.Fatal("non-duplicate must proceed")
	}
	if len(asked) != 0 {
		t.Fatalf("no dialog expected, got %d", len(asked))
	}
}

func TestDuplicateMessageMentionsFormat(t *testing.T) {
	ix := testIndex(t, "Исполнитель A - Песня.mp3")
	var msg string
	a := &App{index: ix, confirm: func(m string) bool { msg = m; return true }}
	tr := &downloader.Track{Artists: []string{"Исполнитель A"}, Title: "Песня"}
	a.checkDuplicate(tr)
	if !strings.Contains(msg, "в формате mp3") || !strings.Contains(msg, "двух форматах") {
		t.Fatalf("format mismatch must be called out, got:\n%s", msg)
	}
}

// --- 4.2: подтверждение на каждый трек плейлиста ---

func TestFilterDuplicatesPlaylist(t *testing.T) {
	ix := testIndex(t,
		"Исполнитель A - Песня 1.flac",
		"Исполнитель A - Песня 3.flac",
	)
	var logs []string
	// Первый совпавший трек отклоняем, второй подтверждаем.
	a := &App{
		index:   ix,
		logSink: func(text, kind string) { logs = append(logs, text) },
		confirm: func(msg string) bool { return !strings.Contains(msg, "Песня 1") },
	}
	tracks := []*downloader.Track{
		{ID: "1", Artists: []string{"Исполнитель A"}, Title: "Песня 1", Available: true},
		{ID: "2", Artists: []string{"Другой"}, Title: "Ничего похожего", Available: true},
		{ID: "3", Artists: []string{"Исполнитель A"}, Title: "Песня 3", Available: true},
	}
	kept := a.filterDuplicates(tracks)
	if len(kept) != 2 || kept[0].ID != "2" || kept[1].ID != "3" {
		t.Fatalf("want tracks 2 and 3 kept, got %+v", kept)
	}
	declined := 0
	for _, l := range logs {
		if strings.Contains(l, "(дубль, отменено пользователем)") {
			declined++
		}
	}
	if declined != 1 {
		t.Fatalf("want one decline log line, got %d: %v", declined, logs)
	}
}

// --- 3.3: скачанное сразу попадает в индекс (temp → rename → индекс) ---

func TestOnPublishedAddsDownloadedFile(t *testing.T) {
	dir := t.TempDir()
	ix := library.New(filepath.Join(t.TempDir(), "cache.json"), dir)
	a := &App{index: ix, outputDir: dir}

	c := downloader.NewClient("")
	a.attachIndex(c)
	if c.OnPublished == nil {
		t.Fatal("published files must be indexed without a rescan")
	}
	p := filepath.Join(dir, "Исполнитель A - Песня.flac")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.OnPublished(p) // как это делает пайплайн сразу после rename
	if m := ix.FindMatches([]string{"Исполнитель A"}, "Песня"); len(m) != 1 {
		t.Fatalf("just downloaded file must be a candidate, got %+v", m)
	}

	// Без индекса колбэк не ставится — поведение прежнее.
	plain := downloader.NewClient("")
	(&App{outputDir: dir}).attachIndex(plain)
	if plain.OnPublished != nil {
		t.Fatal("no index means no indexing hook")
	}
}

// --- 4.3: без LIBRARY_DIR проверки нет вообще ---

func TestNoIndexNoQuestions(t *testing.T) {
	a := &App{
		logSink: func(text, kind string) { t.Errorf("unexpected log: %s", text) },
		confirm: func(string) bool { t.Error("unexpected dialog"); return false },
	}
	tracks := []*downloader.Track{
		{ID: "1", Artists: []string{"A"}, Title: "T", Available: true},
	}
	if !a.checkDuplicate(tracks[0]) {
		t.Fatal("without an index every track must proceed")
	}
	if kept := a.filterDuplicates(tracks); len(kept) != 1 {
		t.Fatalf("without an index nothing is filtered, got %d", len(kept))
	}
}

func TestSetupLibraryInvalidPath(t *testing.T) {
	a := &App{outputDir: "./downloads"}
	a.setupLibrary(filepath.Join(t.TempDir(), ".env"), filepath.Join(t.TempDir(), "missing"))
	if a.index != nil {
		t.Fatal("missing collection folder must disable the feature")
	}
	if !strings.Contains(a.readyNote, "не найдена") || !strings.Contains(a.readyNote, "выключена") {
		t.Fatalf("user must be told in Russian, got %q", a.readyNote)
	}
}

// --- 3.2: предупреждение о неполной проверке во время индексации ---

func TestCheckDuplicateWarnsWhileScanning(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "A - Song.flac"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ix := library.New(filepath.Join(t.TempDir(), "cache.json"), dir)

	var logs []string
	a := &App{
		index:   ix,
		logSink: func(text, kind string) { logs = append(logs, text) },
		confirm: func(string) bool { return true },
	}
	// Проверка внутри лог-колбэка Refresh: scanning == true ровно там.
	var checked bool
	ix.Refresh(func(string) {
		if checked {
			return
		}
		checked = true
		a.checkDuplicate(&downloader.Track{Artists: []string{"A"}, Title: "Song"})
	})
	if !checked {
		t.Fatal("test never ran the check during the scan")
	}
	var warned bool
	for _, l := range logs {
		if strings.Contains(l, "[warn] индексация ещё идёт") && strings.Contains(l, "неполная") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("incomplete check must be reported, got %v", logs)
	}
	// После завершения индексации предупреждений больше нет.
	logs = nil
	a.checkDuplicate(&downloader.Track{Artists: []string{"A"}, Title: "Song"})
	for _, l := range logs {
		if strings.Contains(l, "[warn]") {
			t.Fatalf("no warn after the scan, got %q", l)
		}
	}
}
