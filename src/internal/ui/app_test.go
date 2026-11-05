package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yamdl/internal/config"
	"yamdl/internal/downloader"
	"yamdl/internal/library"
	"yamdl/internal/queue"
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

func TestSubmitWithoutToken(t *testing.T) {
	a := &App{}
	got := a.Submit("https://music.yandex.ru/album/1/track/2")
	if !strings.HasPrefix(got, "[error]") {
		t.Fatalf("want [error], got %q", got)
	}
	for _, want := range []string{"Настройки", "Как получить токен"} {
		if !strings.Contains(got, want) {
			t.Errorf("error must point to %q, got %q", want, got)
		}
	}
	if strings.Contains(got, ".env") {
		t.Errorf("error must not send the user to .env, got %q", got)
	}
}

func TestReadyWithoutTokenMentionsHelp(t *testing.T) {
	a := &App{envPath: filepath.Join(t.TempDir(), ".env")}
	a.apply(config.Config{OutputDir: t.TempDir()}, false)
	for _, want := range []string{"30-секундное превью", "Настройки", "Как получить токен"} {
		if !strings.Contains(a.ready, want) {
			t.Errorf("startup status must contain %q, got %q", want, a.ready)
		}
	}
	if strings.Contains(a.ready, ".env") {
		t.Errorf("startup status must not send the user to .env, got %q", a.ready)
	}
}

// Невалидная ссылка не блокирует очередь: строка получает ошибку.
func TestBadLinkBecomesErrorRow(t *testing.T) {
	a := newJobApp(t, nil, nil)
	a.resolveHook = func(_ context.Context, _ string) (queue.Resolved, error) {
		return queue.Resolved{}, errors.New("ссылка не распознана")
	}
	a.ensureQueue()
	if got := a.Submit("badlink"); got != "" {
		t.Fatalf("submit is accepted and resolved in the queue, got %q", got)
	}
	waitApp(t, "error row", func() bool { return a.GetQueue().Failed == 1 })
	for _, task := range a.GetQueue().Tasks {
		if !strings.Contains(task.Reason, "не распознана") {
			t.Fatalf("row must carry the reason, got %q", task.Reason)
		}
	}
}

// --- проверка возможного дубля как данные строки, а не диалог ---

func TestDuplicateCandidate(t *testing.T) {
	ix := testIndex(t, "Исполнитель A - Песня.flac")
	a := &App{index: ix, duplicatesEnabled: true}

	cand, ok := a.duplicateCandidate(&downloader.Track{Artists: []string{"Исполнитель A"}, Title: "Песня"})
	if !ok {
		t.Fatal("expected a probable duplicate")
	}
	if cand.Percent != 100 {
		t.Errorf("candidate must carry the match percent, got %d", cand.Percent)
	}
	if cand.FileName != "Исполнитель A - Песня.flac" {
		t.Errorf("candidate must name the found file, got %q", cand.FileName)
	}
	if !strings.HasSuffix(cand.FilePath, cand.FileName) {
		t.Errorf("candidate must carry the full path, got %q", cand.FilePath)
	}
	if cand.DifferentFormat {
		t.Errorf("flac must not be reported as a different format: %+v", cand)
	}

	cand, ok = a.duplicateCandidate(&downloader.Track{Artists: []string{"Другой"}, Title: "Совсем иное"})
	if ok {
		t.Fatalf("non-duplicate must not be a candidate: %+v", cand)
	}
}

func TestDuplicateCandidateDifferentFormat(t *testing.T) {
	ix := testIndex(t, "Исполнитель A - Песня.mp3")
	a := &App{index: ix, duplicatesEnabled: true}
	cand, ok := a.duplicateCandidate(&downloader.Track{Artists: []string{"Исполнитель A"}, Title: "Песня"})
	if !ok {
		t.Fatal("expected a candidate")
	}
	if !cand.DifferentFormat || cand.FileExt != "mp3" {
		t.Fatalf("format mismatch must be called out, got %+v", cand)
	}
}

func TestNoIndexNoQuestions(t *testing.T) {
	a := &App{
		logSink: func(text, kind string) { t.Errorf("unexpected log: %s", text) },
	}
	if _, ok := a.duplicateCandidate(&downloader.Track{ID: "1", Artists: []string{"A"}, Title: "T"}); ok {
		t.Fatal("without an index nothing is a duplicate")
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

func TestSetupLibraryForFlacMonitor(t *testing.T) {
	downloads := t.TempDir()
	end := filepath.Join(t.TempDir(), ".env")

	// Empty LIBRARY_DIR + monitor on: index OUTPUT_DIR, no duplicate checks.
	a := &App{outputDir: downloads, cfg: config.Config{FlacMonitor: true}}
	a.setupLibrary(end, "")
	if a.index == nil {
		t.Fatal("monitor must build an index over OUTPUT_DIR")
	}
	if a.duplicatesEnabled {
		t.Fatal("monitor alone must not enable duplicate checks")
	}

	// Empty LIBRARY_DIR + monitor off: exactly the old behaviour.
	a = &App{outputDir: downloads}
	a.setupLibrary(end, "")
	if a.index != nil || a.duplicatesEnabled {
		t.Fatal("without monitor an empty LIBRARY_DIR must keep indexing off")
	}

	// Valid LIBRARY_DIR: index and duplicate checks are on.
	collection := t.TempDir()
	a = &App{outputDir: downloads}
	a.setupLibrary(end, collection)
	if a.index == nil || !a.duplicatesEnabled {
		t.Fatalf("valid LIBRARY_DIR must enable indexing and duplicates: index=%v dup=%v", a.index != nil, a.duplicatesEnabled)
	}
}

// --- 3.2: предупреждение о неполной проверке во время индексации ---

func TestDuplicateCandidateWarnsWhileScanning(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "A - Song.flac"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ix := library.New(filepath.Join(t.TempDir(), "cache.json"), dir)

	logs := &logRecorder{}
	a := &App{
		index:             ix,
		duplicatesEnabled: true,
		logSink:           logs.add,
	}
	// Проверка внутри лог-колбэка Refresh: scanning == true ровно там.
	var checked bool
	ix.Refresh(func(string) {
		if checked {
			return
		}
		checked = true
		a.duplicateCandidate(&downloader.Track{Artists: []string{"A"}, Title: "Song"})
	})
	if !checked {
		t.Fatal("test never ran the check during the scan")
	}
	var warned bool
	for _, l := range logs.all() {
		if strings.Contains(l, "[warn] индексация ещё идёт") && strings.Contains(l, "неполная") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("incomplete check must be reported, got %v", logs.all())
	}
	// После завершения индексации предупреждений больше нет.
	before := len(logs.all())
	a.duplicateCandidate(&downloader.Track{Artists: []string{"A"}, Title: "Song"})
	for _, l := range logs.all()[before:] {
		if strings.Contains(l, "[warn]") {
			t.Fatalf("no warn after the scan, got %q", l)
		}
	}
}

func TestSplitLinks(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"https://a/1", []string{"https://a/1"}},
		{"https://a/1 https://b/2", []string{"https://a/1", "https://b/2"}},
		{"https://a/1\nhttps://b/2\r\nhttps://c/3", []string{"https://a/1", "https://b/2", "https://c/3"}},
		{"https://a/1,https://b/2;https://c/3", []string{"https://a/1", "https://b/2", "https://c/3"}},
		{"  https://a/1  \n\n ", []string{"https://a/1"}},
		{"", nil},
	}
	for _, c := range cases {
		got := splitLinks(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("splitLinks(%q) = %q, want %q", c.in, got, c.want)
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Fatalf("splitLinks(%q) = %q, want %q", c.in, got, c.want)
			}
		}
	}
}

func TestExportM3U(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "Artist - One.flac")
	p2 := filepath.Join(dir, "Artist - Two.mp3")
	for _, p := range []string{p1, p2} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := &App{outputDir: dir, cfg: config.Config{ExportM3U: true}}
	a.exportM3U("Мой: плейлист", []string{p1, p2})

	data, err := os.ReadFile(filepath.Join(dir, "Мой_ плейлист.m3u"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{"#EXTM3U", "Artist - One.flac", "Artist - Two.mp3"} {
		if !strings.Contains(body, want) {
			t.Fatalf("m3u must contain %q:\n%s", want, body)
		}
	}

	// Выключенный экспорт не создаёт файл.
	a = &App{outputDir: dir, cfg: config.Config{ExportM3U: false}}
	a.exportM3U("No", []string{p1})
	if _, err := os.Stat(filepath.Join(dir, "No.m3u")); !os.IsNotExist(err) {
		t.Fatal("disabled export must not write a file")
	}
}
