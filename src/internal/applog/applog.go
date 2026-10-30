// Package applog writes the application's diagnostic log file.
//
// The logger appends timestamped lines, rotates the file by size and
// hides OAuth tokens. It is safe for concurrent use: the download pool
// reports progress from several goroutines.
package applog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// MaxBytes is the default size after which app.log is rotated to
// app.log.1.
const MaxBytes int64 = 5 << 20

// oauthRe matches a token after the OAuth scheme without swallowing
// trailing punctuation.
var oauthRe = regexp.MustCompile(`(?i)(OAuth\s+)[A-Za-z0-9._~+/=-]+`)

// Logger appends log lines to a file. The zero value is not usable;
// call Open.
type Logger struct {
	mu      sync.Mutex
	path    string
	max     int64
	size    int64
	f       *os.File
	secrets []string
}

// Open opens path for appending, creating the file when missing.
// maxBytes <= 0 means MaxBytes.
func Open(path string, maxBytes int64) (*Logger, error) {
	if maxBytes <= 0 {
		maxBytes = MaxBytes
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("applog: open %s: %w", path, err)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("applog: stat %s: %w", path, err)
	}
	return &Logger{path: path, max: maxBytes, size: st.Size(), f: f}, nil
}

// OpenDefault opens app.log next to the executable. When that folder is
// not writable it falls back to the user cache directory
// (%LOCALAPPDATA%\YandexMusicDownloader on Windows).
func OpenDefault(maxBytes int64) (*Logger, error) {
	primary := ""
	if exe, err := os.Executable(); err == nil {
		primary = filepath.Join(filepath.Dir(exe), "app.log")
	}
	fallback := ""
	if dir, err := os.UserCacheDir(); err == nil {
		fallback = filepath.Join(dir, "YandexMusicDownloader", "app.log")
	}
	if primary == "" && fallback == "" {
		return nil, fmt.Errorf("applog: no writable location")
	}
	if primary == "" {
		return openFallback("", fallback, maxBytes)
	}
	return openFallback(primary, fallback, maxBytes)
}

// openFallback opens primary, then fallback. An empty path is skipped.
func openFallback(primary, fallback string, maxBytes int64) (*Logger, error) {
	primaryErr := fmt.Errorf("applog: no primary path")
	if primary != "" {
		if l, err := Open(primary, maxBytes); err == nil {
			return l, nil
		} else {
			primaryErr = err
		}
	}
	if fallback != "" {
		if err := os.MkdirAll(filepath.Dir(fallback), 0o755); err == nil {
			if l, err := Open(fallback, maxBytes); err == nil {
				return l, nil
			}
		}
	}
	return nil, fmt.Errorf("applog: no writable log file: %w", primaryErr)
}

// Path returns the file path the logger writes to.
func (l *Logger) Path() string {
	return l.path
}

// AddSecret registers a literal secret that Write replaces with ***.
// Empty values are ignored.
func (l *Logger) AddSecret(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, have := range l.secrets {
		if have == s {
			return
		}
	}
	l.secrets = append(l.secrets, s)
}

// Redact hides OAuth tokens and registered secrets in text.
func (l *Logger) Redact(text string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.redactLocked(text)
}

func (l *Logger) redactLocked(text string) string {
	text = oauthRe.ReplaceAllString(text, "${1}***")
	for _, s := range l.secrets {
		text = strings.ReplaceAll(text, s, "***")
	}
	return text
}

// Write appends one record: local RFC 3339 time, level in brackets and
// the message. Embedded newlines are preserved as-is.
func (l *Logger) Write(text, kind string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return fmt.Errorf("applog: closed")
	}
	text = l.redactLocked(text)
	kind = strings.TrimSpace(kind)
	rec := time.Now().Format(time.RFC3339) + " [" + kind + "] " + text
	if !strings.HasSuffix(rec, "\n") {
		rec += "\n"
	}
	if l.size > 0 && l.size+int64(len(rec)) > l.max {
		if err := l.rotateLocked(); err != nil {
			return err
		}
	}
	n, err := io.WriteString(l.f, rec)
	l.size += int64(n)
	if err != nil {
		return fmt.Errorf("applog: write %s: %w", l.path, err)
	}
	return nil
}

// Close flushes and closes the file. Later writes fail.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	if err != nil {
		return fmt.Errorf("applog: close %s: %w", l.path, err)
	}
	return nil
}

// rotateLocked closes the current file, moves it to app.log.1 (replacing
// the previous backup) and opens a fresh app.log. On failure the old file
// is reopened so logging can continue.
func (l *Logger) rotateLocked() error {
	if err := l.f.Close(); err != nil {
		l.f = nil
		return fmt.Errorf("applog: close for rotate: %w", err)
	}
	l.f = nil
	backup := l.path + ".1"
	if err := os.Rename(l.path, backup); err != nil && !os.IsNotExist(err) {
		l.reopenLocked()
		return fmt.Errorf("applog: rotate %s: %w", l.path, err)
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("applog: reopen after rotate %s: %w", l.path, err)
	}
	l.f = f
	l.size = 0
	return nil
}

// reopenLocked restores the file handle after a failed rotation.
func (l *Logger) reopenLocked() {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return
	}
	l.f = f
	l.size = st.Size()
}
