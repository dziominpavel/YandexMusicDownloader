package downloader

import (
	"errors"
	"sync"
)

// Playlist pool bounds. The live value is Client.PlaylistWorkers
// (PLAYLIST_WORKERS in .env, same 3/10 contract as config).
const (
	defaultPlaylistWorkers = 3
	maxPlaylistWorkers     = 10
)

// workers normalizes the pool size: unset → default, over max → capped.
func (c *Client) workers() int {
	w := c.PlaylistWorkers
	if w <= 0 {
		return defaultPlaylistWorkers
	}
	if w > maxPlaylistWorkers {
		return maxPlaylistWorkers
	}
	return w
}

// Skip reasons carried in Event.Detail for KindSkipped.
const (
	SkipDuplicate   = "duplicate"
	SkipUnavailable = "unavailable"
)

// Summary counts batch outcomes for the final [finished] line.
type Summary struct {
	OK      int
	Skipped int
	Failed  int
}

// DownloadPlaylist downloads resolved tracks with a fixed pool of
// PlaylistWorkers. One track's error never stops the rest: every track
// goes through the standard pipeline (lossless first, MP3 fallback,
// tags, temp → rename). Duplicates (same ID twice) and unavailable
// tracks are skipped without network. emit/onProgress may be called
// concurrently from workers; both are serialized inside.
func (c *Client) DownloadPlaylist(tracks []*Track, outputDir string, emit func(Event), onProgress ProgressFunc) Summary {
	var mu sync.Mutex
	safeEmit := func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		emitEvent(emit, e)
	}
	safeProgress := ProgressFunc(nil)
	if onProgress != nil {
		safeProgress = func(p Progress) {
			mu.Lock()
			defer mu.Unlock()
			onProgress(p)
		}
	}

	dl := c.downloadOne
	if dl == nil {
		dl = c.DownloadTrack
	}

	var sum Summary
	seen := make(map[string]bool)
	type job struct{ t *Track }
	var queue []job
	for _, t := range tracks {
		if t == nil {
			continue
		}
		if seen[t.ID] {
			sum.Skipped++
			safeEmit(Event{Kind: KindSkipped, Label: t.Label(), TrackID: t.ID, Detail: SkipDuplicate})
			continue
		}
		seen[t.ID] = true
		if !t.Available {
			sum.Skipped++
			safeEmit(Event{Kind: KindSkipped, Label: t.Label(), TrackID: t.ID, Detail: SkipUnavailable})
			continue
		}
		queue = append(queue, job{t: t})
	}

	sem := make(chan struct{}, c.workers())
	var wg sync.WaitGroup
	for _, j := range queue {
		wg.Add(1)
		go func(t *Track) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			_, err := dl(t, outputDir, safeEmit, safeProgress)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				sum.OK++
			case errors.Is(err, ErrAlreadyExists):
				sum.Skipped++
			default:
				sum.Failed++
			}
		}(j.t)
	}
	wg.Wait()
	return sum
}
