package downloader

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadPlaylistSummary(t *testing.T) {
	c := NewClient("")
	c.downloadOne = func(tr *Track, _ string, emit func(Event), _ ProgressFunc) (Result, error) {
		emitEvent(emit, Event{Kind: KindDownloading, Label: tr.Label()})
		switch tr.ID {
		case "fail":
			emitEvent(emit, Event{Kind: KindFailed, Label: tr.Label(), Detail: "boom"})
			return Result{}, errors.New("boom")
		case "exists":
			emitEvent(emit, Event{Kind: KindSkipped, Label: tr.Label()})
			return Result{Path: "x", Format: "MP3", Fallback: true}, ErrAlreadyExists
		default:
			emitEvent(emit, Event{Kind: KindDone, Label: tr.Label(), Format: "FLAC"})
			return Result{Path: "x", Format: "FLAC"}, nil
		}
	}
	tracks := []*Track{
		{ID: "1", Title: "T1", Artists: []string{"A"}, Available: true},
		{ID: "2", Title: "T2", Artists: []string{"A"}, Available: true},
		{ID: "1", Title: "T1", Artists: []string{"A"}, Available: true},   // duplicate
		{ID: "3", Title: "T3", Artists: []string{"A"}, Available: false},  // unavailable
		{ID: "fail", Title: "TF", Artists: []string{"A"}, Available: true}, // error
	}
	var skippedDetails []string
	sum := c.DownloadPlaylist(tracks, t.TempDir(), func(e Event) {
		if e.Kind == KindSkipped {
			skippedDetails = append(skippedDetails, e.Detail)
		}
	}, nil)
	if sum.OK != 2 || sum.Skipped != 2 || sum.Failed != 1 {
		t.Fatalf("summary = %+v, want {2 2 1}", sum)
	}
	foundDup, foundUnav := false, false
	for _, d := range skippedDetails {
		if d == SkipDuplicate {
			foundDup = true
		}
		if d == SkipUnavailable {
			foundUnav = true
		}
	}
	if !foundDup || !foundUnav {
		t.Fatalf("skip reasons missing: %q", skippedDetails)
	}
}

func TestDownloadPlaylistCapsWorkers(t *testing.T) {
	c := NewClient("")
	var cur, max int64
	c.downloadOne = func(tr *Track, _ string, emit func(Event), _ ProgressFunc) (Result, error) {
		n := atomic.AddInt64(&cur, 1)
		for {
			m := atomic.LoadInt64(&max)
			if n <= m || atomic.CompareAndSwapInt64(&max, m, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt64(&cur, -1)
		return Result{Format: "FLAC"}, nil
	}
	var tracks []*Track
	for i := 0; i < 9; i++ {
		tracks = append(tracks, &Track{ID: string(rune('a' + i)), Title: "T", Available: true})
	}
	sum := c.DownloadPlaylist(tracks, t.TempDir(), nil, nil)
	if sum.OK != 9 {
		t.Fatalf("summary = %+v", sum)
	}
	if max > int64(c.workers()) {
		t.Fatalf("max parallel = %d, want <= %d", max, c.workers())
	}
}

func TestDownloadPlaylistCustomWorkers(t *testing.T) {
	c := NewClient("")
	c.PlaylistWorkers = 5
	var cur, max int64
	c.downloadOne = func(tr *Track, _ string, emit func(Event), _ ProgressFunc) (Result, error) {
		n := atomic.AddInt64(&cur, 1)
		for {
			m := atomic.LoadInt64(&max)
			if n <= m || atomic.CompareAndSwapInt64(&max, m, n) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt64(&cur, -1)
		return Result{Format: "FLAC"}, nil
	}
	var tracks []*Track
	for i := 0; i < 10; i++ {
		tracks = append(tracks, &Track{ID: string(rune('a' + i)), Title: "T", Available: true})
	}
	if sum := c.DownloadPlaylist(tracks, t.TempDir(), nil, nil); sum.OK != 10 {
		t.Fatalf("summary = %+v", sum)
	}
	if max <= 1 || max > 5 {
		t.Fatalf("max parallel = %d, want 2..5", max)
	}

	// Unset and over-max values normalize to default and cap.
	c.PlaylistWorkers = 0
	if got := c.workers(); got != 3 {
		t.Fatalf("workers(0) = %d, want 3", got)
	}
	c.PlaylistWorkers = 99
	if got := c.workers(); got != 10 {
		t.Fatalf("workers(99) = %d, want 10", got)
	}
}
