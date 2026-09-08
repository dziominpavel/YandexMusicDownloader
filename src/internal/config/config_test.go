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
