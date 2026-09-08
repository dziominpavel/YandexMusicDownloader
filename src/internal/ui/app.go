// Package ui is the desktop window backend: it wires config, resolver
// and downloader together and streams log events to the frontend.
package ui

import (
	"context"
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"yamdl/internal/config"
	"yamdl/internal/downloader"
	"yamdl/internal/resolver"
	"yamdl/internal/tagger"
)

// App is bound to the frontend (window.go.main.App).
type App struct {
	ctx       context.Context
	client    *downloader.Client
	outputDir string
	ready     string // startup status line
}

// NewApp loads config and prepares the downloader.
func NewApp() *App {
	a := &App{outputDir: "./downloads"}
	envPath, err := config.DefaultPath()
	if err != nil {
		a.ready = "config: " + err.Error()
		return a
	}
	cfg, err := config.Load(envPath)
	if err != nil {
		a.ready = "config: " + err.Error()
		return a
	}
	if cfg.OutputDir != "" {
		a.outputDir = cfg.OutputDir
	}
	if cfg.Token == "" {
		a.ready = "токен пуст — доступно только 30-секундное превью"
		return a
	}
	who, err := config.Validate(cfg.Token)
	if err != nil {
		a.ready = "токен НЕ принят: " + err.Error()
		return a
	}
	a.client = downloader.NewClient(cfg.Token)
	a.client.ConvertM4A = cfg.ConvertM4A
	a.client.Tag = tagger.Adapter(false)
	a.ready = "токен ок: " + who
	return a
}

// Startup is called by Wails when the window is ready.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.log(a.ready, "info")
}

func (a *App) log(text, kind string) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "log", text, kind)
	}
}

func formatEvent(e downloader.Event) (string, string) {
	switch e.Kind {
	case downloader.KindDownloading:
		return "[downloading] " + e.Label, "info"
	case downloader.KindDone:
		s := "[done] " + e.Label + " (" + e.Format + ")"
		if e.Fallback {
			s += " [fallback MP3]"
		}
		if e.Converted {
			s += " [ALAC→FLAC]"
		}
		return s, "done"
	case downloader.KindSkipped:
		return "[skip] " + e.Label + " (already exists)", "info"
	default:
		return "[error] " + e.Label + ": " + e.Detail, "err"
	}
}

// Download fetches one track link. It blocks; the frontend awaits it
// while live progress arrives via log events. Terminal lifecycle events
// are already shown in the log, so Download returns "" when the outcome
// was displayed and a message only when there is nothing in the log yet
// (no token, bad link, failure before any event).
func (a *App) Download(link string) string {
	if a.client == nil {
		return "[error] нет токена: заполни YANDEX_TOKEN в .env рядом с app.exe"
	}
	ref, err := resolver.Parse(link)
	if err != nil {
		return "[error] " + err.Error()
	}
	var shown bool
	_, err = a.client.Download(ref.TrackID, a.outputDir, func(e downloader.Event) {
		text, kind := formatEvent(e)
		a.log(text, kind)
		if e.Kind == downloader.KindDone || e.Kind == downloader.KindSkipped || e.Kind == downloader.KindFailed {
			shown = true
		}
	})
	if err != nil {
		if shown {
			return ""
		}
		return fmt.Sprintf("[error] %v", err)
	}
	if !shown {
		return "[done]"
	}
	return ""
}
