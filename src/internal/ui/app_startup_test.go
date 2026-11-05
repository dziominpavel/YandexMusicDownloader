package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"yamdl/internal/config"
)

func TestApplyStartupDoesNotValidateToken(t *testing.T) {
	a := &App{}
	a.validateToken = func(string) (string, error) {
		t.Fatal("startup apply must not validate the token")
		return "", nil
	}
	a.apply(config.Config{Token: "t", OutputDir: t.TempDir()}, false)
	if a.client == nil {
		t.Fatal("client must be created immediately")
	}
	if a.ready != "проверяю токен…" {
		t.Fatalf("startup status must say the token is being checked, got %q", a.ready)
	}
	if got := a.authError(); got != "" {
		t.Fatalf("no auth error expected before the check, got %q", got)
	}
}

func TestVerifyStartupTokenSuccess(t *testing.T) {
	a := &App{}
	var logs []string
	a.logSink = func(text, kind string) { logs = append(logs, kind+": "+text) }
	a.validateToken = func(string) (string, error) { return "user (uid 1)", nil }
	a.mu.Lock()
	a.tokenGen = 1
	a.mu.Unlock()

	a.verifyStartupToken("t", 1)

	if got := a.authError(); got != "" {
		t.Fatalf("valid token must not set an auth error, got %q", got)
	}
	if !hasLog(logs, "info: токен ок: user (uid 1)") {
		t.Fatalf("success must be logged, got %v", logs)
	}
}

func TestVerifyStartupTokenFailure(t *testing.T) {
	a := &App{}
	var logs []string
	a.logSink = func(text, kind string) { logs = append(logs, kind+": "+text) }
	a.validateToken = func(string) (string, error) { return "", errors.New("http 401") }
	a.mu.Lock()
	a.tokenGen = 1
	a.mu.Unlock()

	a.verifyStartupToken("t", 1)

	if got := a.authError(); !strings.Contains(got, "токен НЕ принят") {
		t.Fatalf("invalid token must set a Russian auth error, got %q", got)
	}
	if !hasLog(logs, "error: токен НЕ принят: http 401") {
		t.Fatalf("failure must be logged, got %v", logs)
	}
}

func TestVerifyStartupTokenIgnoresStaleResult(t *testing.T) {
	a := &App{}
	var logs []string
	a.logSink = func(text, kind string) { logs = append(logs, kind+": "+text) }
	a.validateToken = func(string) (string, error) { return "", errors.New("old") }
	a.mu.Lock()
	a.tokenGen = 2
	a.mu.Unlock()

	a.verifyStartupToken("old", 1)

	if got := a.authError(); got != "" {
		t.Fatalf("stale result must not set an auth error, got %q", got)
	}
	if len(logs) != 0 {
		t.Fatalf("stale result must not be logged, got %v", logs)
	}
}

func TestSubmitRejectsInvalidToken(t *testing.T) {
	a := &App{authErr: "токен НЕ принят: http 401"}
	got := a.Submit("https://music.yandex.ru/track/1")
	if got != "[error] токен НЕ принят: http 401" {
		t.Fatalf("unexpected submit result: %q", got)
	}
}

func TestPreviewLinkRejectsInvalidToken(t *testing.T) {
	a := &App{authErr: "токен НЕ принят: http 401"}
	_, err := a.PreviewLink("https://music.yandex.ru/track/1")
	if err == nil || !strings.Contains(err.Error(), "токен НЕ принят") {
		t.Fatalf("unexpected preview error: %v", err)
	}
}

func TestStartupChecksTokenInBackground(t *testing.T) {
	a := &App{cfg: config.Config{Token: "t"}}
	a.validateToken = func(string) (string, error) { return "user", nil }
	lines := make(chan string, 16)
	a.logSink = func(text, kind string) { lines <- kind + ": " + text }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.Startup(ctx)

	want := "info: токен ок: user"
	deadline := time.After(2 * time.Second)
	for {
		select {
		case line := <-lines:
			if line == want {
				return
			}
		case <-deadline:
			t.Fatalf("startup token check did not log %q", want)
		}
	}
}

func hasLog(logs []string, want string) bool {
	for _, line := range logs {
		if line == want {
			return true
		}
	}
	return false
}
