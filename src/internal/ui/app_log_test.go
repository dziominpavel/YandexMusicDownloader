package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yamdl/internal/applog"
)

func testLogFile(t *testing.T) *applog.Logger {
	t.Helper()
	l, err := applog.Open(filepath.Join(t.TempDir(), "app.log"), 0)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func readLog(t *testing.T, l *applog.Logger) string {
	t.Helper()
	data, err := os.ReadFile(l.Path())
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	return string(data)
}

func TestLogWritesFile(t *testing.T) {
	l := testLogFile(t)
	var lines []string
	a := &App{logFile: l, logSink: func(text, kind string) { lines = append(lines, text) }}

	a.log("привет", "error")

	if len(lines) != 1 || lines[0] != "привет" {
		t.Fatalf("window log = %q", lines)
	}
	if body := readLog(t, l); !strings.Contains(body, "[error] привет") {
		t.Fatalf("file log missing line: %q", body)
	}
}

func TestStartupLogsFilePath(t *testing.T) {
	l := testLogFile(t)
	envPath := filepath.Join(t.TempDir(), ".env")
	outDir := t.TempDir()
	var lines []string
	a := &App{
		logFile:   l,
		envPath:   envPath,
		outputDir: outDir,
		ready:     "токен ок",
		logSink:   func(text, kind string) { lines = append(lines, text) },
	}

	a.Startup(context.Background())

	wantPath := "[log] файл: " + l.Path()
	found := false
	for _, line := range lines {
		if line == wantPath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("window log lacks %q: %q", wantPath, lines)
	}

	body := readLog(t, l)
	for _, want := range []string{"запуск: ", "система: ", "exe: ", ".env: " + envPath, "загрузки: " + outDir, "лог: " + l.Path()} {
		if !strings.Contains(body, want) {
			t.Errorf("header missing %q:\n%s", want, body)
		}
	}
	if !strings.Contains(body, "[info] токен ок") {
		t.Errorf("ready line missing from file:\n%s", body)
	}
}

func TestStartupLogsDisabledFileLog(t *testing.T) {
	var lines []string
	a := &App{
		logFileErr: errors.New("нет доступа к папке"),
		logSink:    func(text, kind string) { lines = append(lines, text) },
	}

	a.Startup(context.Background())

	if len(lines) == 0 {
		t.Fatal("window log is empty")
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "отключён") || !strings.Contains(joined, "нет доступа к папке") {
		t.Fatalf("disabled log must be reported, got %q", joined)
	}
}

func TestLogRedactsToken(t *testing.T) {
	l := testLogFile(t)
	var lines []string
	a := &App{logFile: l, logSink: func(text, kind string) { lines = append(lines, text) }}

	a.addLogSecret("secret-token-123")
	a.log("Authorization: OAuth abcdef123 and secret-token-123", "error")

	if len(lines) != 1 {
		t.Fatalf("window log = %q", lines)
	}
	for _, secret := range []string{"abcdef123", "secret-token-123"} {
		if strings.Contains(lines[0], secret) {
			t.Fatalf("window leaked %q: %q", secret, lines[0])
		}
	}
	body := readLog(t, l)
	for _, secret := range []string{"abcdef123", "secret-token-123"} {
		if strings.Contains(body, secret) {
			t.Fatalf("file leaked %q:\n%s", secret, body)
		}
	}
	if !strings.Contains(lines[0], "OAuth ***") {
		t.Fatalf("OAuth value not redacted: %q", lines[0])
	}
}
