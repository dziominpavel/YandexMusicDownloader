// Package ui is the desktop window backend: it wires config, resolver
// and downloader together and streams log events to the frontend.
package ui

import (
	"context"
	"fmt"
	"sync"

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
	mu        sync.Mutex
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
	a.client.PlaylistWorkers = cfg.Workers
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
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "log", text, kind)
	}
}

// progress forwards a byte counter to the frontend, where the track's
// row is updated in place. Safe for concurrent worker goroutines.
func (a *App) progress(p downloader.Progress) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "progress", p.TrackID, p.Label, p.Percent, p.Done, p.Total)
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
		switch e.Detail {
		case downloader.SkipDuplicate:
			return "[skip] " + e.Label + " (duplicate)", "info"
		case downloader.SkipUnavailable:
			return "[skip] " + e.Label + " (unavailable)", "info"
		default:
			return "[skip] " + e.Label + " (already exists)", "info"
		}
	default:
		return "[error] " + e.Label + ": " + e.Detail, "err"
	}
}

// Download fetches a track or playlist link. It blocks; the frontend
// awaits it while live log and progress events arrive. Terminal lifecycle
// events are already shown in the log, so Download returns "" when the
// outcome was displayed and a message only when there is nothing in the
// log yet (no token, bad link, failure before any event).
func (a *App) Download(link string) string {
	if a.client == nil {
		return "[error] нет токена: заполни YANDEX_TOKEN в .env рядом с app.exe"
	}
	ref, err := resolver.Parse(link)
	if err != nil {
		return "[error] " + err.Error()
	}
	if ref.Kind == resolver.KindPlaylist {
		return a.downloadPlaylist(ref.PlaylistUUID)
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

// formatTrackStatus renders the short in-row status for one track:
// the same vocabulary as the log, minus the repeated label.
// The frontend shows exactly one row per track and only this part changes.
func formatTrackStatus(e downloader.Event) (string, string) {
	switch e.Kind {
	case downloader.KindDownloading:
		return "[downloading]", "info"
	case downloader.KindDone:
		s := "[done] (" + e.Format + ")"
		if e.Fallback {
			s += " [fallback MP3]"
		}
		if e.Converted {
			s += " [ALAC→FLAC]"
		}
		return s, "done"
	case downloader.KindSkipped:
		switch e.Detail {
		case downloader.SkipDuplicate:
			return "[skip] (duplicate)", "info"
		case downloader.SkipUnavailable:
			return "[skip] (unavailable)", "info"
		default:
			return "[skip] (already exists)", "info"
		}
	default:
		return "[error] " + e.Detail, "err"
	}
}

// trackStatus forwards one track's status to its frontend row.
// Safe for concurrent worker goroutines.
func (a *App) trackStatus(id, status, kind string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "track-status", id, status, kind)
	}
}

// trackRows sends the ordered row scaffolding: one [id, label] pair per
// unique track, so the frontend shows exactly len rows in playlist order
// before any worker reports back.
func (a *App) trackRows(tracks []*downloader.Track) {
	seen := make(map[string]bool)
	var rows [][2]string
	for _, t := range tracks {
		if t == nil || seen[t.ID] {
			continue
		}
		seen[t.ID] = true
		rows = append(rows, [2]string{t.ID, t.Label()})
	}
	if len(rows) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "tracks", rows)
	}
}

// downloadPlaylist resolves a playlist UUID into tracks and downloads
// them with the fixed worker pool. One track never stops the rest.
// The frontend shows exactly one row per track (status updates in place);
// the log only gets the header and the [finished] summary line.
func (a *App) downloadPlaylist(uuid string) string {
	pl, err := a.client.Playlist(uuid)
	if err != nil {
		return fmt.Sprintf("[error] плейлист недоступен: %v", err)
	}
	if len(pl.Tracks) == 0 {
		return "[error] плейлист пуст"
	}
	title := pl.Title
	if title == "" {
		title = "Плейлист"
	}
	a.playlistStart(title, len(pl.Tracks))
	a.trackRows(pl.Tracks)
	sum := a.client.DownloadPlaylist(pl.Tracks, a.outputDir, func(e downloader.Event) {
		status, kind := formatTrackStatus(e)
		a.trackStatus(e.TrackID, status, kind)
	}, func(p downloader.Progress) {
		a.progress(p)
	})
	a.log(fmt.Sprintf("[finished] %d ok, %d skip, %d error", sum.OK, sum.Skipped, sum.Failed), "info")
	return ""
}

// playlistStart announces a new playlist run: the frontend clears old
// rows and shows the header on top of the row block.
func (a *App) playlistStart(title string, count int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "playlist-start", title, count)
	}
}
