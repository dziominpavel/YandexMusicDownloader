package applog

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestWriteFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	l, err := Open(path, 0)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := l.Write("текст", "error"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	got := readFile(t, path)
	re := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:Z|[+-]\d{2}:\d{2}) \[error\] текст\n$`)
	if !re.MatchString(got) {
		t.Fatalf("unexpected format: %q", got)
	}
}

func TestWriteMultiline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	l, err := Open(path, 0)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := l.Write("first\nsecond\nthird", "info"); err != nil {
		t.Fatalf("write: %v", err)
	}
	l.Close()
	got := readFile(t, path)
	if strings.Count(got, "[info]") != 1 {
		t.Fatalf("prefix must appear once, got %q", got)
	}
	if !strings.Contains(got, "[info] first\nsecond\nthird\n") {
		t.Fatalf("multiline message lost: %q", got)
	}
}

func TestRedactOAuth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	l, err := Open(path, 0)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := l.Write("Authorization: OAuth abcdef123, done", "error"); err != nil {
		t.Fatalf("write: %v", err)
	}
	l.Close()
	got := readFile(t, path)
	if strings.Contains(got, "abcdef123") {
		t.Fatalf("token leaked: %q", got)
	}
	if !strings.Contains(got, "Authorization: OAuth ***, done") {
		t.Fatalf("unexpected redaction: %q", got)
	}
}

func TestRedactSecretValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	l, err := Open(path, 0)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	l.AddSecret("secret-token-123")
	if err := l.Write("leaked secret-token-123 inside", "error"); err != nil {
		t.Fatalf("write: %v", err)
	}
	l.Close()
	got := readFile(t, path)
	if strings.Contains(got, "secret-token-123") {
		t.Fatalf("secret leaked: %q", got)
	}
	if !strings.Contains(got, "leaked *** inside") {
		t.Fatalf("unexpected redaction: %q", got)
	}
	if redacted := l.Redact("secret-token-123"); redacted != "***" {
		t.Fatalf("Redact returned %q", redacted)
	}
}

func TestAppendAcrossOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	first, err := Open(path, 0)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := first.Write("first", "info"); err != nil {
		t.Fatalf("write: %v", err)
	}
	first.Close()
	second, err := Open(path, 0)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if err := second.Write("second", "info"); err != nil {
		t.Fatalf("write: %v", err)
	}
	second.Close()
	got := readFile(t, path)
	if !strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Fatalf("append lost lines: %q", got)
	}
}

func TestRotate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	l, err := Open(path, 120)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	line := strings.Repeat("a", 60)
	if err := l.Write(line, "info"); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	if err := l.Write(line, "info"); err != nil {
		t.Fatalf("write 2: %v", err)
	}
	l.Close()
	if got := readFile(t, path+".1"); !strings.Contains(got, line) {
		t.Fatalf("backup lost first record: %q", got)
	}
	cur := readFile(t, path)
	if !strings.Contains(cur, line) {
		t.Fatalf("current file lost second record: %q", cur)
	}
	if n := strings.Count(cur, "[info]"); n != 1 {
		t.Fatalf("current file must hold exactly the new record, got %d", n)
	}
}

func TestRotateReplacesBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	l, err := Open(path, 120)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for i := 1; i <= 3; i++ {
		line := fmt.Sprintf("record-%d-%s", i, strings.Repeat("a", 40))
		if err := l.Write(line, "info"); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	l.Close()

	backup := readFile(t, path+".1")
	if !strings.Contains(backup, "record-2-") {
		t.Fatalf("backup must hold the second record: %q", backup)
	}
	if strings.Contains(backup, "record-1-") {
		t.Fatalf("backup kept an older record: %q", backup)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected app.log and app.log.1 only, got %d entries", len(entries))
	}
}

func TestOpenFallback(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	primary := filepath.Join(blocker, "app.log")
	fallback := filepath.Join(dir, "cache", "app.log")
	l, err := openFallback(primary, fallback, 0)
	if err != nil {
		t.Fatalf("openFallback: %v", err)
	}
	if l.Path() != fallback {
		t.Fatalf("path = %q, want %q", l.Path(), fallback)
	}
	if err := l.Write("fallback", "info"); err != nil {
		t.Fatalf("write: %v", err)
	}
	l.Close()
	if got := readFile(t, fallback); !strings.Contains(got, "fallback") {
		t.Fatalf("fallback file missing record: %q", got)
	}
}

func TestOpenAllFail(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	l, err := openFallback(filepath.Join(blocker, "app.log"), filepath.Join(blocker, "cache", "app.log"), 0)
	if err == nil {
		l.Close()
		t.Fatal("expected an error when both paths are unavailable")
	}
}

func TestConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	l, err := Open(path, 0)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	const workers, perWorker = 8, 25
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				if err := l.Write(fmt.Sprintf("worker %d line %d", w, i), "info"); err != nil {
					t.Errorf("write: %v", err)
				}
			}
		}(w)
	}
	wg.Wait()
	l.Close()

	got := readFile(t, path)
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != workers*perWorker {
		t.Fatalf("got %d lines, want %d", len(lines), workers*perWorker)
	}
	re := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:Z|[+-]\d{2}:\d{2}) \[info\] worker \d+ line \d+$`)
	seen := make(map[string]bool, len(lines))
	for _, line := range lines {
		if !re.MatchString(line) {
			t.Fatalf("interleaved or malformed line: %q", line)
		}
		if seen[line] {
			t.Fatalf("duplicate line: %q", line)
		}
		seen[line] = true
	}
}
