package downloader

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
)

// Стоп до старта: ни одного обращения к сети, трек помечен [stopped].
func TestDownloadTrackStoppedBeforeStart(t *testing.T) {
	c := NewClient("")
	c.StartJob()
	c.Stop()

	var got []Event
	_, err := c.DownloadTrack(
		&Track{ID: "1", Artists: []string{"A"}, Title: "T", Available: true},
		t.TempDir(),
		func(e Event) { got = append(got, e) },
		nil,
	)
	if !errors.Is(err, ErrStopped) {
		t.Fatalf("err = %v, want ErrStopped", err)
	}
	if len(got) != 1 || got[0].Kind != KindStopped {
		t.Fatalf("want a single KindStopped event, got %+v", got)
	}
}

// Стоп на середине пула: очередь не стартует, итог — Stopped, не Failed.
func TestStopSkipsPlaylistQueue(t *testing.T) {
	c := NewClient("")
	c.PlaylistWorkers = 1
	c.StartJob()

	var started int32
	c.downloadOne = func(tr *Track, _ string, emit func(Event), _ ProgressFunc) (Result, error) {
		atomic.AddInt32(&started, 1)
		c.Stop() // первый же трек останавливает job'у
		emitEvent(emit, Event{Kind: KindStopped, Label: tr.Label(), TrackID: tr.ID})
		return Result{}, ErrStopped
	}
	tracks := []*Track{
		{ID: "1", Title: "T1", Artists: []string{"A"}, Available: true},
		{ID: "2", Title: "T2", Artists: []string{"A"}, Available: true},
		{ID: "3", Title: "T3", Artists: []string{"A"}, Available: true},
		{ID: "4", Title: "T4", Artists: []string{"A"}, Available: true},
		{ID: "5", Title: "T5", Artists: []string{"A"}, Available: true},
	}
	var stoppedEvents int32
	sum := c.DownloadPlaylist(tracks, t.TempDir(), func(e Event) {
		if e.Kind == KindStopped {
			atomic.AddInt32(&stoppedEvents, 1)
		}
	}, nil)

	if n := atomic.LoadInt32(&started); n != 1 {
		t.Errorf("started downloads = %d, want 1 (pool must not launch the rest)", n)
	}
	if sum.OK != 0 || sum.Skipped != 0 || sum.Failed != 0 {
		t.Errorf("summary = %+v, want only Stopped", sum)
	}
	if sum.Stopped != 5 {
		t.Errorf("Stopped = %d, want 5", sum.Stopped)
	}
	if n := atomic.LoadInt32(&stoppedEvents); n != 5 {
		t.Errorf("KindStopped events = %d, want 5", n)
	}
}

// Запрос несёт контекст job'ы: «Остановить» обрывает его, новая job'а —
// новый живой контекст.
func TestRequestContextFollowsStop(t *testing.T) {
	c := NewClient("")
	c.StartJob()

	req, err := c.newRequest(http.MethodGet, "https://example.com/x")
	if err != nil {
		t.Fatal(err)
	}
	if req.Context().Err() != nil {
		t.Fatal("a fresh job must have a live request context")
	}

	c.Stop()
	req, err = c.newRequest(http.MethodGet, "https://example.com/x")
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(req.Context().Err(), context.Canceled) {
		t.Fatalf("after Stop the context must be cancelled, got %v", req.Context().Err())
	}
	// Сам запрос падает мгновенно и без обращения к сети.
	if _, err := c.http.Do(req); !errors.Is(err, context.Canceled) {
		t.Fatalf("Do on a stopped job = %v, want context.Canceled", err)
	}

	c.StartJob()
	if c.Stopped() {
		t.Fatal("StartJob must clear the stop flag")
	}
	req, err = c.newRequest(http.MethodGet, "https://example.com/x")
	if err != nil {
		t.Fatal(err)
	}
	if req.Context().Err() != nil {
		t.Fatalf("a new job must have a live context, got %v", req.Context().Err())
	}
}
