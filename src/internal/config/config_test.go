package config

import (
	"os"
	"path/filepath"
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

func TestLoadPlaylistWorkers(t *testing.T) {
	cases := []struct {
		body string
		want int
	}{
		{"", DefaultWorkers},                       // missing → default
		{"PLAYLIST_WORKERS=10", 10},                // max allowed
		{"PLAYLIST_WORKERS=1", 1},                  // min allowed
		{"PLAYLIST_WORKERS=0", DefaultWorkers},     // out of range → default
		{"PLAYLIST_WORKERS=99", DefaultWorkers},    // out of range → default
		{"PLAYLIST_WORKERS=many", DefaultWorkers},  // garbage → default
		{"PLAYLIST_WORKERS= 5 ", 5},                // surrounding spaces ok
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
