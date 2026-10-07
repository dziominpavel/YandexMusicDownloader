package ui

import (
	"strings"
	"testing"

	"yamdl/internal/downloader"
)

// Остановленный одиночный трек: лог и строка — [stopped] вместо [error],
// job-end уходит с флагом остановки (статусбар не доводится до 100%).
func TestDownloadSingleStopped(t *testing.T) {
	var rec eventRecorder
	var logs []string
	tr := &downloader.Track{ID: "9", Artists: []string{"A"}, Title: "Solo", Available: true}
	a := &App{
		client:    downloader.NewClient(""),
		eventSink: rec.sink,
		logSink:   func(text, kind string) { logs = append(logs, text) },
		downloadOne: func(got *downloader.Track, _ string, emit func(downloader.Event), _ downloader.ProgressFunc) error {
			emit(downloader.Event{Kind: downloader.KindDownloading, Label: got.Label(), TrackID: got.ID})
			emit(downloader.Event{Kind: downloader.KindStopped, Label: got.Label(), TrackID: got.ID})
			return downloader.ErrStopped
		},
	}
	if got := a.downloadSingle(tr); got != "" {
		t.Fatalf("stopped outcome must be shown by events, got %q", got)
	}
	iEnd := rec.find("job-end")
	if iEnd < 0 {
		t.Fatalf("job-end missing: %v", rec.names)
	}
	args := rec.arg(iEnd)
	if len(args) != 4 {
		t.Fatalf("job-end args = %v, want 4 values", args)
	}
	if args[0] != 0 || args[1] != 0 || args[2] != 0 {
		t.Errorf("job-end outcome = %v, want all zeroes", args)
	}
	if args[3] != true {
		t.Errorf("job-end stopped flag = %v, want true", args[3])
	}
	// Строка трека: последний статус — [stopped], не [error].
	statuses := rec.all("track-status")
	if len(statuses) == 0 {
		t.Fatalf("track-status missing: %v", rec.names)
	}
	last := statuses[len(statuses)-1]
	if rec.arg(last)[1] != "[stopped]" || rec.arg(last)[2] != "stopped" {
		t.Errorf("last track-status = %v, want [stopped]/stopped", rec.arg(last))
	}
	var sawStoppedLog bool
	for _, l := range logs {
		if l == "[stopped] A — Solo" {
			sawStoppedLog = true
		}
		if strings.HasPrefix(l, "[error]") {
			t.Errorf("a stop must not be logged as an error: %q", l)
		}
	}
	if !sawStoppedLog {
		t.Errorf("log must show [stopped] A — Solo, got %v", logs)
	}
}

// Стоп биндинга поднимает флаг клиента; новая job'а его сбрасывает.
func TestStopFlagResetOnNewJob(t *testing.T) {
	var rec eventRecorder
	a := &App{
		client:    downloader.NewClient(""),
		eventSink: rec.sink,
		logSink:   func(text, kind string) {},
	}
	a.downloadOne = func(*downloader.Track, string, func(downloader.Event), downloader.ProgressFunc) error {
		return nil
	}
	a.Stop()
	if !a.stopRequested() {
		t.Fatal("Stop must set the client stop flag")
	}
	// Плохая ссылка: job'а не стартует, но флаг уже должен быть сброшен
	// — иначе следующее скачивание не начнётся.
	if got := a.Download("not a link"); !strings.HasPrefix(got, "[error]") {
		t.Fatalf("want [error], got %q", got)
	}
	if a.stopRequested() {
		t.Fatal("a new Download must reset the previous stop")
	}
}

// Плейлист, остановленный на середине: [stopped]-итог вместо [finished].
func TestDownloadPlaylistStopped(t *testing.T) {
	var rec eventRecorder
	var logs []string
	a := &App{
		client:    downloader.NewClient(""),
		eventSink: rec.sink,
		logSink:   func(text, kind string) { logs = append(logs, text) },
		fetchPlaylist: func(string) (*downloader.Playlist, error) {
			return &downloader.Playlist{
				Title: "Микс",
				Tracks: []*downloader.Track{
					{ID: "1", Artists: []string{"A"}, Title: "T1", Available: true},
					{ID: "2", Artists: []string{"A"}, Title: "T2", Available: true},
				},
			}, nil
		},
		downloadBatch: func(tracks []*downloader.Track, _ string, emit func(downloader.Event), _ downloader.ProgressFunc) downloader.Summary {
			emit(downloader.Event{Kind: downloader.KindStopped, Label: tracks[0].Label(), TrackID: tracks[0].ID})
			return downloader.Summary{Stopped: 2}
		},
	}
	if got := a.Download("https://music.yandex.ru/playlists/11111111-2222-3333-4444-555555555555"); got != "" {
		t.Fatalf("stopped outcome must be in the log, got %q", got)
	}
	iEnd := rec.find("job-end")
	if iEnd < 0 {
		t.Fatalf("job-end missing: %v", rec.names)
	}
	if rec.arg(iEnd)[3] != true {
		t.Errorf("job-end stopped flag = %v, want true", rec.arg(iEnd))
	}
	var summary string
	for _, l := range logs {
		if strings.HasPrefix(l, "[finished]") {
			t.Errorf("a stopped job must not log [finished]: %q", l)
		}
		if strings.HasPrefix(l, "[stopped] прервано:") {
			summary = l
		}
	}
	if summary == "" {
		t.Errorf("want a [stopped] summary line, got %v", logs)
	}
}
