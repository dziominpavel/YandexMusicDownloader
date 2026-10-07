package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yamdl/internal/config"
)

func TestInitialSize(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		w, h int
	}{
		{"нет ключей → дефолт", config.Config{}, DefaultWindowWidth, DefaultWindowHeight},
		{"заданы → их", config.Config{WindowWidth: 1000, WindowHeight: 700}, 1000, 700},
		{"неполный набор → дефолт", config.Config{WindowWidth: 1000}, DefaultWindowWidth, DefaultWindowHeight},
		{"ноль → дефолт", config.Config{WindowWidth: 1000, WindowHeight: 0}, DefaultWindowWidth, DefaultWindowHeight},
	}
	for _, c := range cases {
		a := &App{cfg: c.cfg}
		w, h := a.InitialSize()
		if w != c.w || h != c.h {
			t.Errorf("%s: got %dx%d, want %dx%d", c.name, w, h, c.w, c.h)
		}
	}
}

func TestWindowSizeUpdate(t *testing.T) {
	cases := []struct {
		name         string
		lastW, lastH int
		w, h         int
		wantNil      bool
		wantW, wantH string
	}{
		{"первое сохранение", 0, 0, 800, 600, false, "800", "600"},
		{"изменился", 864, 576, 1000, 700, false, "1000", "700"},
		{"без изменений", 864, 576, 864, 576, true, "", ""},
		{"нулевая ширина", 864, 576, 0, 576, true, "", ""},
		{"отрицательная высота", 864, 576, 800, -5, true, "", ""},
	}
	for _, c := range cases {
		got := windowSizeUpdate(c.lastW, c.lastH, c.w, c.h)
		if c.wantNil {
			if got != nil {
				t.Errorf("%s: want nil, got %v", c.name, got)
			}
			continue
		}
		if got == nil {
			t.Errorf("%s: want %sx%s, got nil", c.name, c.wantW, c.wantH)
			continue
		}
		if got["WINDOW_WIDTH"] != c.wantW || got["WINDOW_HEIGHT"] != c.wantH {
			t.Errorf("%s: got %v, want %sx%s", c.name, got, c.wantW, c.wantH)
		}
	}
}

func TestSaveWindowSizeWritesOnce(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	a := &App{envPath: p, cfg: config.Config{}}

	a.saveWindowSize(800, 600)
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, "WINDOW_WIDTH=800") || !strings.Contains(text, "WINDOW_HEIGHT=600") {
		t.Fatalf("size not written:\n%s", text)
	}

	// Тот же размер при следующем закрытии файл не трогает.
	a.cfg.WindowWidth, a.cfg.WindowHeight = 800, 600
	a.saveWindowSize(800, 600)
	got, err = os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(got), "WINDOW_WIDTH=") != 1 {
		t.Fatalf("unchanged size must not duplicate keys:\n%s", got)
	}
}

func TestSaveWindowSizeWithoutEnvPath(t *testing.T) {
	a := &App{cfg: config.Config{}}
	a.saveWindowSize(800, 600) // без .env — тихий пропуск, паники нет
}
