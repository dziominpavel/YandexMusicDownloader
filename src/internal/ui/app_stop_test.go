package ui

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"yamdl/internal/downloader"
	"yamdl/internal/queue"
)

func TestCancelAllStopsQueue(t *testing.T) {
	logs := &logRecorder{}
	a := newJobApp(t, nil, logs)
	a.cfg.Workers = 1
	a.resolveHook = func(_ context.Context, link string) (queue.Resolved, error) {
		if link == "many" {
			return queue.Resolved{Label: "Альбом · Many", Kind: "album", Tracks: []*downloader.Track{trk("1", "T1"), trk("2", "T2"), trk("3", "T3")}}, nil
		}
		return queue.Resolved{Label: "Трек · New", Kind: "track", Tracks: []*downloader.Track{trk("4", "T4")}}, nil
	}
	gate := make(chan struct{})
	a.downloadHook = func(ctx context.Context, _ *downloader.Track, _ any, _ func(int64, int64)) (queue.Result, error) {
		select {
		case <-gate:
			return queue.Result{Format: "FLAC"}, nil
		case <-ctx.Done():
			return queue.Result{}, ctx.Err()
		}
	}
	a.ensureQueue()
	a.Submit("many")
	waitApp(t, "one active and two waiting", func() bool {
		s := a.GetQueue()
		return s.Active == 1 && s.Waiting == 2
	})
	a.CancelAll()
	waitApp(t, "everything canceled", func() bool {
		s := a.GetQueue()
		return s.Canceled == 3 && s.Active == 0
	})
	if !logs.has("[stopped]") {
		t.Fatalf("a stop must be logged as [stopped]: %v", logs.all())
	}
	// Новая постановка после остановки работает: очередь не «залипает».
	if got := a.Submit("new"); got != "" {
		t.Fatalf("submit after cancel must be accepted, got %q", got)
	}
	waitApp(t, "new task starts", func() bool {
		s := a.GetQueue()
		return s.Active == 1
	})
	close(gate)
	waitApp(t, "new task done", func() bool { return a.GetQueue().Done == 1 })
}

func TestCancelOneTaskKeepsNeighbour(t *testing.T) {
	a := newJobApp(t, nil, nil)
	a.cfg.Workers = 2
	a.resolveHook = func(_ context.Context, _ string) (queue.Resolved, error) {
		return queue.Resolved{Label: "Альбом · Two", Kind: "album", Tracks: []*downloader.Track{trk("1", "T1"), trk("2", "T2")}}, nil
	}
	gate := make(chan struct{})
	a.downloadHook = func(ctx context.Context, _ *downloader.Track, _ any, _ func(int64, int64)) (queue.Result, error) {
		select {
		case <-gate:
			return queue.Result{Format: "FLAC"}, nil
		case <-ctx.Done():
			return queue.Result{}, ctx.Err()
		}
	}
	a.ensureQueue()
	a.Submit("two")
	waitApp(t, "both active", func() bool { return a.GetQueue().Active == 2 })

	var key string
	for _, task := range a.GetQueue().Tasks {
		if task.TrackID == "1" {
			key = task.Key
		}
	}
	a.CancelTask(key)
	waitApp(t, "one canceled, one active", func() bool {
		s := a.GetQueue()
		return s.Canceled == 1 && s.Active == 1
	})
	close(gate)
	waitApp(t, "neighbour done", func() bool { return a.GetQueue().Done == 1 })
}

func TestRetryFailedTask(t *testing.T) {
	a := newJobApp(t, nil, nil)
	a.cfg.Workers = 1
	a.resolveHook = func(_ context.Context, _ string) (queue.Resolved, error) {
		return queue.Resolved{Label: "Трек · Bad", Kind: "track", Tracks: []*downloader.Track{trk("bad", "Bad")}}, nil
	}
	var attempts atomic.Int32
	a.downloadHook = func(_ context.Context, _ *downloader.Track, _ any, _ func(int64, int64)) (queue.Result, error) {
		if attempts.Load() == 0 {
			attempts.Add(1)
			return queue.Result{}, errors.New("сеть упала")
		}
		return queue.Result{Format: "MP3"}, nil
	}
	a.ensureQueue()
	a.Submit("bad")
	waitApp(t, "error", func() bool { return a.GetQueue().Failed == 1 })
	var key string
	for _, task := range a.GetQueue().Tasks {
		key = task.Key
	}
	a.RetryTask(key)
	waitApp(t, "retry succeeds", func() bool {
		s := a.GetQueue()
		return s.Done == 1 && s.Failed == 0
	})
}
