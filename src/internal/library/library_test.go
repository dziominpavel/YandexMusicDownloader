package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yamdl/internal/tagger"
	"yamdl/internal/tagread"
)

// writeRaw creates a file with the given (non-tag) content.
func writeRaw(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func refresh(t *testing.T, ix *Index) []string {
	t.Helper()
	var lines []string
	ix.Refresh(func(s string) { lines = append(lines, s) })
	return lines
}

// --- 2.1: рекурсивный обход, подкаталоги, фильтр не-аудио ---

func TestWalkCoversSubdirsAndSkipsNonAudio(t *testing.T) {
	collection := t.TempDir()
	downloads := t.TempDir()
	writeRaw(t, collection, "root.flac", "x")
	writeRaw(t, collection, filepath.Join("Рок", "sub", "deep.mp3"), "x")
	writeRaw(t, collection, "cover.jpg", "not audio")
	writeRaw(t, collection, "playlist.m3u", "not audio")
	writeRaw(t, collection, "readme.txt", "not audio")
	writeRaw(t, downloads, "fresh.m4a", "x")

	ix := New(filepath.Join(t.TempDir(), "cache.json"), collection, downloads)
	refresh(t, ix)

	if got := ix.Len(); got != 3 {
		t.Fatalf("want 3 audio files indexed, got %d", got)
	}
	// Подкаталоги попадают в индекс.
	if _, ok := ix.entries[filepath.Join(collection, "Рок", "sub", "deep.mp3")]; !ok {
		t.Fatal("subdirectory file must be indexed")
	}
	// Не-аудио файлы не попадают.
	for p := range ix.entries {
		switch filepath.Ext(p) {
		case ".flac", ".mp3", ".m4a":
		default:
			t.Fatalf("non-audio file indexed: %s", p)
		}
	}
}

// --- 2.2: теги первичны, имя файла — фолбэк ---

// tagFixture copies a real audio fixture, tags it and returns the path.
func tagFixture(t *testing.T, fixture, ext string, m tagger.Meta) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "tagger", "testdata", fixture))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	p := filepath.Join(t.TempDir(), "unrecognizable"+ext)
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tagger.Write(p, m); err != nil {
		t.Fatalf("tag fixture: %v", err)
	}
	return p
}

func TestTagsBeatFilename(t *testing.T) {
	m := tagger.Meta{Title: "Название из тега", Artists: []string{"Исполнитель из тега"}}
	cases := []struct{ fixture, ext string }{
		{"mp3-id3v24.mp3", ".mp3"},
		{"flac.flac", ".flac"},
		{"mp4-aac.m4a", ".m4a"},
	}
	for _, c := range cases {
		p := tagFixture(t, c.fixture, c.ext, m)
		e := readEntry(p, fileMeta{ext: c.ext})
		if e.Title != "Название из тега" {
			t.Errorf("%s: title from tags, got %q", c.ext, e.Title)
		}
		if e.Artists != "Исполнитель из тега" {
			t.Errorf("%s: artists from tags, got %q", c.ext, e.Artists)
		}
		if !e.Matchable {
			t.Errorf("%s: tagged file must be matchable", c.ext)
		}
	}
}

func TestReadsYandexIDAndSourceURL(t *testing.T) {
	const source = "https://music.yandex.ru/album/7/track/99"
	m := tagger.Meta{
		Title: "Название", Artists: []string{"Исполнитель"},
		TrackID: "99", SourceURL: source,
	}
	for _, c := range []struct{ fixture, ext string }{
		{"mp3-id3v24.mp3", ".mp3"},
		{"flac.flac", ".flac"},
		{"mp4-aac.m4a", ".m4a"},
	} {
		p := tagFixture(t, c.fixture, c.ext, m)
		e := readEntry(p, fileMeta{ext: c.ext})
		if e.TrackID != "99" {
			t.Errorf("%s: TrackID = %q, want 99", c.ext, e.TrackID)
		}
		if e.SourceURL != source {
			t.Errorf("%s: SourceURL = %q, want %q", c.ext, e.SourceURL, source)
		}
	}
}

func TestReadsTrackIDFromSourceURLFallback(t *testing.T) {
	// Older files may carry only the source URL, without YANDEX_TRACK_ID
	// or UFID. The index still has to recover the numeric id.
	const source = "https://music.yandex.ru/album/7/track/123"
	m := tagger.Meta{Title: "Название", Artists: []string{"Исполнитель"}, SourceURL: source}
	for _, c := range []struct{ fixture, ext string }{
		{"mp3-id3v24.mp3", ".mp3"},
		{"flac.flac", ".flac"},
		{"mp4-aac.m4a", ".m4a"},
	} {
		p := tagFixture(t, c.fixture, c.ext, m)
		e := readEntry(p, fileMeta{ext: c.ext})
		if e.TrackID != "123" {
			t.Errorf("%s: TrackID = %q, want 123", c.ext, e.TrackID)
		}
	}
}

func TestExternalFileHasNoYandexID(t *testing.T) {
	p := writeRaw(t, t.TempDir(), "Внешний - Трек.mp3", "junk")
	e := readEntry(p, fileMeta{ext: ".mp3"})
	if e.TrackID != "" || e.SourceURL != "" {
		t.Fatalf("external file must have no Yandex tags, got %+v", e)
	}
	if e.Artists != "Внешний" || e.Title != "Трек" {
		t.Fatalf("filename fallback must still work, got %+v", e)
	}
}

func TestParseTrackID(t *testing.T) {
	cases := map[string]string{
		"https://music.yandex.ru/album/7/track/99":   "99",
		"https://music.yandex.by/track/123?utm=1":    "123",
		"https://music.yandex.com/album/1/track/456": "456",
		"99":                          "99",
		"not a track id":              "",
		"https://example.com/track/x": "",
	}
	for in, want := range cases {
		if got := tagread.NormalizeTrackID(in); got != want {
			t.Errorf("NormalizeTrackID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFilenameFallback(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name        string
		wantArtists string
		wantTitle   string
		matchable   bool
	}{
		{"Исполнитель - Название.flac", "Исполнитель", "Название", true},
		{"01 - Песня.mp3", "", "Песня", true},
		{"ι Noize MC - Светлая полоса.flac", "ι Noize MC", "Светлая полоса", true},
		{"track001.flac", "", "", false}, // ни тегов, ни смысла в имени
		{"sflkj.mp3", "", "", false},
	}
	for _, c := range cases {
		p := writeRaw(t, dir, c.name, "junk")
		e := readEntry(p, fileMeta{ext: filepath.Ext(c.name)})
		if e.Artists != c.wantArtists || e.Title != c.wantTitle || e.Matchable != c.matchable {
			t.Errorf("%q: got (%q, %q, %v), want (%q, %q, %v)",
				c.name, e.Artists, e.Title, e.Matchable,
				c.wantArtists, c.wantTitle, c.matchable)
		}
	}
}

// --- 2.3 + 2.5: нормализация и поиск по схожести ---

// buildIndex scans a tree of tagless files named "Artist - Title.ext".
func buildIndex(t *testing.T, names ...string) *Index {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		writeRaw(t, dir, n, "junk")
	}
	ix := New(filepath.Join(t.TempDir(), "cache.json"), dir)
	refresh(t, ix)
	return ix
}

func TestFindMatchesExactTitle(t *testing.T) {
	ix := buildIndex(t, "Исполнитель A - Песня.flac")
	m := ix.FindMatches([]string{"Исполнитель A"}, "Песня")
	if len(m) != 1 || m[0].Percent != 100 || m[0].Ext != ".flac" {
		t.Fatalf("want single 100%% flac match, got %+v", m)
	}
}

func TestFindMatchesNinetyThreePercent(t *testing.T) {
	ix := buildIndex(t, "Noize MC - Светлая полоса.flac")
	// 1 правка в 14 символах → ~93%.
	m := ix.FindMatches([]string{"Noize MC"}, "Светлая полосы")
	if len(m) != 1 {
		t.Fatalf("want a match, got %+v", m)
	}
	if m[0].Percent != 93 {
		t.Fatalf("want 93%%, got %d", m[0].Percent)
	}
}

func TestFindMatchesBelowThreshold(t *testing.T) {
	ix := buildIndex(t, "Исполнитель A - Песня.flac")
	if m := ix.FindMatches([]string{"Исполнитель A"}, "Совсем иное название"); len(m) != 0 {
		t.Fatalf("below-threshold titles must not match, got %+v", m)
	}
	if m := ix.FindMatches([]string{"Другой"}, "Ничего похожего"); len(m) != 0 {
		t.Fatalf("want no matches, got %+v", m)
	}
}

func TestFindMatchesCoverSameTitleDifferentArtists(t *testing.T) {
	ix := buildIndex(t, "Исполнитель A - Песня.flac")
	// Кавер: заголовок тот же, исполнитель другой — всё равно дубль-кандидат.
	m := ix.FindMatches([]string{"Совсем другой"}, "Песня")
	if len(m) != 1 || m[0].Percent != 100 {
		t.Fatalf("cover must match, got %+v", m)
	}
}

func TestFindMatchesArtistOrderAndLooseTitle(t *testing.T) {
	ix := buildIndex(t, "Исполнитель B, Исполнитель A - Песня.flac")
	// Порядок исполнителей не важен; заголовок ниже 90%, но >= 80% —
	// срабатывает второе правило при совпадении исполнителей.
	m := ix.FindMatches([]string{"Исполнитель A", "Исполнитель B"}, "Песняа")
	if len(m) != 1 {
		t.Fatalf("artist order must not matter, got %+v", m)
	}
	if m[0].Percent < 80 || m[0].Percent > 90 {
		t.Fatalf("want loose-title score in 80..90, got %d", m[0].Percent)
	}
}

func TestFindMatchesSortPrefixAndTwin(t *testing.T) {
	ix := buildIndex(t, "ι Noize MC - Светлая полоса.flac", "Мari Sa - Моя Любовь.flac")
	if m := ix.FindMatches([]string{"Noize MC"}, "Светлая полоса"); len(m) != 1 {
		t.Fatalf("sort prefix must be stripped, got %+v", m)
	}
	if m := ix.FindMatches([]string{"Mari Sa"}, "Моя Любовь"); len(m) != 1 {
		t.Fatalf("latin twin must fold, got %+v", m)
	}
}

func TestFindMatchesIgnoresUnmatchable(t *testing.T) {
	ix := buildIndex(t, "track001.flac", "sflkj.mp3")
	if m := ix.FindMatches([]string{"X"}, "track001"); len(m) != 0 {
		t.Fatalf("files without usable names must not match, got %+v", m)
	}
	if m := ix.FindMatches([]string{"X"}, ""); len(m) != 0 {
		t.Fatalf("empty query must not match, got %+v", m)
	}
}

func TestScoreCandidate(t *testing.T) {
	cases := []struct {
		name         string
		localArtists []string
		localTitle   string
		candArtists  []string
		candTitle    string
		wantOK       bool
		wantPercent  int
		wantExact    bool
	}{
		{"точное совпадение", []string{"Artist"}, "Song", []string{"Artist"}, "Song", true, 100, true},
		{"93 процента", []string{"Noize MC"}, "Светлая полосы", []string{"Noize MC"}, "Светлая полоса", true, 93, false},
		{"ё и е", []string{"Artist"}, "Всё", []string{"Artist"}, "Все", true, 100, true},
		{"порядок исполнителей", []string{"B", "A"}, "Song", []string{"A", "B"}, "Song", true, 100, true},
		{"другая версия", []string{"Artist"}, "Song", []string{"Artist"}, "Song (Live)", false, 0, false},
		{"другой исполнитель", []string{"Artist"}, "Song", []string{"Other"}, "Song", false, 0, false},
		{"нет исполнителя", nil, "Song", []string{"Artist"}, "Song", false, 0, false},
	}
	for _, c := range cases {
		score, ok := ScoreCandidate(c.localArtists, c.localTitle, c.candArtists, c.candTitle)
		if ok != c.wantOK {
			t.Errorf("%s: ok = %v, want %v (%+v)", c.name, ok, c.wantOK, score)
			continue
		}
		if !ok {
			continue
		}
		if score.Percent != c.wantPercent || score.Exact != c.wantExact {
			t.Errorf("%s: got %+v, want percent=%d exact=%v", c.name, score, c.wantPercent, c.wantExact)
		}
	}
}

// --- 2.4: кэш и инкремент ---

func TestCacheIncrementalRefresh(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(t.TempDir(), "cache.json")
	writeRaw(t, dir, "A - Song.flac", "v1")

	ix := New(cache, dir)
	lines := refresh(t, ix)
	if !hasLine(lines, "перечитано 1") {
		t.Fatalf("first scan must read the file, got %v", lines)
	}

	// Повторный запуск: кэш есть, файлы не менялись — без чтения тегов.
	ix2 := New(cache, dir)
	if ix2.Len() != 1 {
		t.Fatalf("cache must load %d entries, got %d", 1, ix2.Len())
	}
	lines = refresh(t, ix2)
	if !hasLine(lines, "без изменений") {
		t.Fatalf("unchanged files must not be re-read, got %v", lines)
	}

	// Изменённый файл перечитывается.
	writeRaw(t, dir, "A - Song.flac", "v2 — content is different")
	lines = refresh(t, ix2)
	if !hasLine(lines, "перечитано 1") {
		t.Fatalf("changed file must be re-read, got %v", lines)
	}

	// Удалённый файл уходит из индекса.
	if err := os.Remove(filepath.Join(dir, "A - Song.flac")); err != nil {
		t.Fatal(err)
	}
	refresh(t, ix2)
	if ix2.Len() != 0 {
		t.Fatalf("deleted file must leave the index, got %d entries", ix2.Len())
	}
}

func TestCacheVersionMismatchRescans(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(t.TempDir(), "cache.json")
	writeRaw(t, dir, "A - Song.flac", "v1")
	refresh(t, New(cache, dir))

	// Битая/чужая версия кэша → полное пересканирование, не паника.
	if err := os.WriteFile(cache, []byte(`{"version":99,"entries":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ix := New(cache, dir)
	if ix.Len() != 0 {
		t.Fatalf("unknown cache version must be ignored, got %d", ix.Len())
	}
	lines := refresh(t, ix)
	if !hasLine(lines, "перечитано 1") {
		t.Fatalf("must rescan from scratch, got %v", lines)
	}
}

func TestOldDirectoriesAreSkipped(t *testing.T) {
	dir := t.TempDir()
	writeRaw(t, dir, "_old/replaced.mp3", "old")
	writeRaw(t, dir, filepath.Join("Рок", "_old", "nested.flac"), "old")
	writeRaw(t, dir, "kept.mp3", "new")

	ix := New(filepath.Join(t.TempDir(), "cache.json"), dir)
	refresh(t, ix)
	if ix.Len() != 1 {
		t.Fatalf("only the non-_old file must be indexed, got %d", ix.Len())
	}
	if _, ok := ix.entries[filepath.Join(dir, "kept.mp3")]; !ok {
		t.Fatal("regular file must survive the _old filter")
	}
}

func TestSnapshotIsACopy(t *testing.T) {
	dir := t.TempDir()
	writeRaw(t, dir, "A - Song.mp3", "x")
	ix := New(filepath.Join(t.TempDir(), "cache.json"), dir)
	refresh(t, ix)

	snap := ix.Snapshot()
	if len(snap) != 1 || snap[0].Title != "Song" {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
	snap[0].Title = "changed"
	if ix.Snapshot()[0].Title != "Song" {
		t.Fatal("Snapshot must not expose the internal entry map")
	}
}

func TestRemoveFile(t *testing.T) {
	dir := t.TempDir()
	p := writeRaw(t, dir, "A - Song.mp3", "x")
	ix := New(filepath.Join(t.TempDir(), "cache.json"), dir)
	refresh(t, ix)
	ix.RemoveFile(p)
	if ix.Len() != 0 {
		t.Fatalf("removed file must leave the index, got %d", ix.Len())
	}
}

func TestCacheVersionOneIsIgnored(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(t.TempDir(), "cache.json")
	writeRaw(t, dir, "A - Song.mp3", "x")
	refresh(t, New(cache, dir))

	// The previous schema (v1) has no track id fields; it must be rebuilt.
	if err := os.WriteFile(cache, []byte(`{"version":1,"entries":[{"path":"stale.mp3"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ix := New(cache, dir)
	if ix.Len() != 0 {
		t.Fatalf("v1 cache must be ignored, got %d", ix.Len())
	}
	lines := refresh(t, ix)
	if !hasLine(lines, "перечитано 1") {
		t.Fatalf("v1 cache must trigger a rescan, got %v", lines)
	}
}

func TestAddFileWithoutRescan(t *testing.T) {
	dir := t.TempDir()
	ix := New(filepath.Join(t.TempDir(), "cache.json"), dir)
	refresh(t, ix)

	p := writeRaw(t, dir, "Исполнитель - Свежий.flac", "x")
	ix.AddFile(p)
	if ix.Len() != 1 {
		t.Fatalf("AddFile must index the file, got %d", ix.Len())
	}
	if m := ix.FindMatches([]string{"Исполнитель"}, "Свежий"); len(m) != 1 {
		t.Fatalf("freshly downloaded file must be found, got %+v", m)
	}
	// Не-аудио файлы не индексируются.
	ix.AddFile(writeRaw(t, dir, "cover.jpg", "x"))
	if ix.Len() != 1 {
		t.Fatalf("non-audio must be ignored, got %d", ix.Len())
	}
}

// Scanning/Progress отражают ход фонового сканирования — на них опирается
// предупреждение о неполной проверке.
func TestScanningAndProgressDuringRefresh(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.flac", "b.mp3"} {
		writeRaw(t, dir, n, "x")
	}
	ix := New(filepath.Join(t.TempDir(), "cache.json"), dir)

	var sawScanning, sawCounter bool
	ix.Refresh(func(string) {
		// Первый же лог приходит уже при выставленном флаге и ненулевом счётчике.
		if ix.Scanning() {
			sawScanning = true
		}
		if done, total := ix.Progress(); total == 2 && done >= 0 {
			sawCounter = true
		}
	})
	if !sawScanning || !sawCounter {
		t.Fatalf("scan state must be observable: scanning=%v counter=%v", sawScanning, sawCounter)
	}
	if ix.Scanning() {
		t.Fatal("scanning must stop when Refresh returns")
	}
	if done, total := ix.Progress(); done != 2 || total != 2 {
		t.Fatalf("final progress must be 2/2, got %d/%d", done, total)
	}
}

func hasLine(lines []string, sub string) bool {
	for _, l := range lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}
