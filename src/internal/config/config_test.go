package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFile(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), ".env"))
	if err != nil {
		t.Fatalf("missing file must not fail: %v", err)
	}
	if cfg.Token != "" {
		t.Fatalf("token must be empty, got %q", cfg.Token)
	}
	if cfg.OutputDir != "./downloads" {
		t.Fatalf("default output dir, got %q", cfg.OutputDir)
	}
}

func TestLoadParsesEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	body := "# comment\n\nYANDEX_TOKEN='tok123'\nOUTPUT_DIR=\"D:/music\"\nJUNK\n"
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "tok123" {
		t.Fatalf("token, got %q", cfg.Token)
	}
	if cfg.OutputDir != "D:/music" {
		t.Fatalf("output dir, got %q", cfg.OutputDir)
	}
}

func TestLoadLibraryDir(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"задан", "LIBRARY_DIR=D:/Music\n", "D:/Music"},
		{"в кавычках", `LIBRARY_DIR="D:/Музыка"`, "D:/Музыка"},
		{"пустой", "LIBRARY_DIR=\n", ""},
		{"нет параметра", "OUTPUT_DIR=./x\n", ""},
	}
	for _, c := range cases {
		dir := t.TempDir()
		p := filepath.Join(dir, ".env")
		if err := os.WriteFile(p, []byte(c.body), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(p)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if cfg.LibraryDir != c.want {
			t.Errorf("%s: LibraryDir = %q, want %q", c.name, cfg.LibraryDir, c.want)
		}
	}
}

func TestLoadEmbedCover(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		{"", true},                   // missing → default: embed
		{"EMBED_COVER=true", true},   // plain true
		{"EMBED_COVER=TRUE", true},   // case-insensitive
		{"EMBED_COVER=yes", true},    // synonym
		{"EMBED_COVER=1", true},      // numeric true
		{"EMBED_COVER=false", false}, // explicit off
		{"EMBED_COVER=FALSE", false}, // case-insensitive off
		{"EMBED_COVER=0", false},     // numeric off
		{"EMBED_COVER=no", false},    // synonym off
		{"EMBED_COVER=off", false},   // synonym off
		{"EMBED_COVER=maybe", true},  // garbage → ignored, stays on
		{"SKIP_COVER=false", true},   // old key is ignored entirely
		{"EMBED_COVER=\n", true},     // empty value → default
	}
	for _, c := range cases {
		dir := t.TempDir()
		p := filepath.Join(dir, ".env")
		if err := os.WriteFile(p, []byte(c.body), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(p)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.EmbedCover != c.want {
			t.Errorf("body %q: EmbedCover = %v, want %v", c.body, cfg.EmbedCover, c.want)
		}
	}
}

func TestLoadWindowDim(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		wantW int
		wantH int
	}{
		{"нет ключей", "", 0, 0},
		{"оба заданы", "WINDOW_WIDTH=1000\nWINDOW_HEIGHT=700\n", 1000, 700},
		{"пробелы", "WINDOW_WIDTH= 1000 \n", 1000, 0},
		{"мусор", "WINDOW_WIDTH=abc\nWINDOW_HEIGHT=\n", 0, 0},
		{"слишком мало", "WINDOW_WIDTH=100\nWINDOW_HEIGHT=50\n", 0, 0},
		{"слишком много", "WINDOW_WIDTH=99999\nWINDOW_HEIGHT=99999\n", 0, 0},
		{"неполный набор", "WINDOW_WIDTH=1000\n", 1000, 0},
	}
	for _, c := range cases {
		dir := t.TempDir()
		p := filepath.Join(dir, ".env")
		if err := os.WriteFile(p, []byte(c.body), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(p)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.WindowWidth != c.wantW || cfg.WindowHeight != c.wantH {
			t.Errorf("%s: size = %dx%d, want %dx%d",
				c.name, cfg.WindowWidth, cfg.WindowHeight, c.wantW, c.wantH)
		}
	}
}

func TestWritePreservesCommentsAndUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	body := "# header comment\nYANDEX_TOKEN=old\n\nUNKNOWN_KEY=keepme\nOUTPUT_DIR=./downloads\n"
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	err := Write(p, map[string]string{
		"YANDEX_TOKEN": "newtok",
		"EMBED_COVER":  "false",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	want := "# header comment\nYANDEX_TOKEN=newtok\n\nUNKNOWN_KEY=keepme\nOUTPUT_DIR=./downloads\nEMBED_COVER=false\n"
	if string(got) != want {
		t.Fatalf("file mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "newtok" || cfg.EmbedCover {
		t.Fatalf("round-trip: token=%q embedCover=%v", cfg.Token, cfg.EmbedCover)
	}
}

func TestWriteWindowSizeRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	if err := Write(p, map[string]string{
		"WINDOW_WIDTH":  "1000",
		"WINDOW_HEIGHT": "700",
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WindowWidth != 1000 || cfg.WindowHeight != 700 {
		t.Fatalf("round-trip: size = %dx%d, want 1000x700", cfg.WindowWidth, cfg.WindowHeight)
	}
	// Повторная запись того же размера не должна дублировать ключи.
	if err := Write(p, map[string]string{
		"WINDOW_WIDTH":  "1000",
		"WINDOW_HEIGHT": "700",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(got), "WINDOW_WIDTH=") != 1 {
		t.Fatalf("WINDOW_WIDTH must appear once:\n%s", got)
	}
}

func TestWriteCreatesMissingFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	if err := Write(p, map[string]string{"OUTPUT_DIR": "D:/x"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OutputDir != "D:/x" {
		t.Fatalf("output dir, got %q", cfg.OutputDir)
	}
}

func TestWriteNoUpdates(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	if err := Write(p, nil); err != nil {
		t.Fatalf("empty updates must not fail: %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("empty updates must not create a file")
	}
}

func TestWriteReplacesQuotedValue(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte("LIBRARY_DIR=\"D:/Old\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Write(p, map[string]string{"LIBRARY_DIR": "D:/New"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LibraryDir != "D:/New" {
		t.Fatalf("library dir, got %q", cfg.LibraryDir)
	}
}

func TestLoadPlaylistWorkers(t *testing.T) {
	cases := []struct {
		body string
		want int
	}{
		{"", DefaultWorkers},                      // missing → default
		{"PLAYLIST_WORKERS=10", 10},               // max allowed
		{"PLAYLIST_WORKERS=1", 1},                 // min allowed
		{"PLAYLIST_WORKERS=0", DefaultWorkers},    // out of range → default
		{"PLAYLIST_WORKERS=99", DefaultWorkers},   // out of range → default
		{"PLAYLIST_WORKERS=many", DefaultWorkers}, // garbage → default
		{"PLAYLIST_WORKERS= 5 ", 5},               // surrounding spaces ok
	}
	for _, c := range cases {
		dir := t.TempDir()
		p := filepath.Join(dir, ".env")
		if err := os.WriteFile(p, []byte(c.body), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(p)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Workers != c.want {
			t.Errorf("body %q: workers = %d, want %d", c.body, cfg.Workers, c.want)
		}
	}
}
