package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"yamdl/internal/downloader"
)

// testSink collects events so tests can assert on the observable stream.
type testSink struct {
	mu       sync.Mutex
	added    []Task
	updated  map[string]Task
	removed  []string
	sources  []Source
	snaps    []Snapshot
	logs     []string
	existing []string
}

func newTestSink() *testSink {
	return &testSink{updated: make(map[string]Task)}
}

func (s *testSink) TaskAdded(t Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.added = append(s.added, t)
}

func (s *testSink) TaskUpdated(t Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updated[t.Key] = t
}

func (s *testSink) TaskRemoved(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removed = append(s.removed, key)
}

func (s *testSink) Sources(src []Source) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sources = append([]Source(nil), src...)
}

func (s *testSink) Queue(snap Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snaps = append(s.snaps, snap)
}

func (s *testSink) Log(text, kind string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, kind+": "+text)
}

func (s *testSink) Existing(trackID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.existing = append(s.existing, trackID)
}

func (s *testSink) hasLog(sub string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.logs {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

func track(id string) *downloader.Track {
	return &downloader.Track{ID: id, Title: id, Artists: []string{"A"}, Available: true}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func taskByTrack(s Snapshot, trackID string) Task {
	for _, t := range s.Tasks {
		if t.TrackID == trackID {
			return t
		}
	}
	return Task{}
}

// 2.1: каждая ссылка — источник, треки разворачиваются по порядку.
func TestSubmitExpandsLinksInOrder(t *testing.T) {
	sink := newTestSink()
	resolve := func(_ context.Context, link string) (Resolved, error) {
		switch link {
		case "album":
			return Resolved{Label: "Альбом", Log: "[album] Album (3 треков)", Tracks: []*downloader.Track{track("a1"), track("a2"), track("a3")}}, nil
		case "single":
			return Resolved{Label: "Трек", Tracks: []*downloader.Track{track("b1")}}, nil
		}
		return Resolved{}, errors.New("bad link")
	}
	m := New(sink, resolve, func(_ context.Context, tr *downloader.Track, _ any, _ func(int64, int64)) (Result, error) {
		return Result{Format: "FLAC", Path: tr.ID + ".flac"}, nil
	}, nil, func() int { return 3 })

	m.Submit([]string{"album", "single"})
	waitFor(t, "four expanded tasks", func() bool { return m.Snapshot().Total == 4 })

	s := m.Snapshot()
	want := []string{"a1", "a2", "a3", "b1"}
	for i, id := range want {
		if s.Tasks[i].TrackID != id {
			t.Fatalf("task %d = %q, want %q", i, s.Tasks[i].TrackID, id)
		}
	}
	if len(s.Sources) != 2 {
		t.Fatalf("sources = %d, want 2", len(s.Sources))
	}
	if !sink.hasLog("[album] Album (3 треков)") {
		t.Fatal("source log line missing")
	}
}

// 2.2: общий семафор, лишние ждут с позицией, освободившийся слот стартует следующий.
func TestPoolLimitAndWaitingPositions(t *testing.T) {
	sink := newTestSink()
	resolve := func(_ context.Context, _ string) (Resolved, error) {
		return Resolved{Label: "Альбом", Tracks: []*downloader.Track{track("1"), track("2"), track("3"), track("4"), track("5")}}, nil
	}
	var mu sync.Mutex
	gates := map[string]chan struct{}{}
	for _, id := range []string{"1", "2", "3", "4", "5"} {
		gates[id] = make(chan struct{})
	}
	defer func() {
		for _, g := range gates {
			select {
			case <-g:
			default:
				close(g)
			}
		}
	}()
	started := make(chan string, 5)
	release := func(id string) { close(gates[id]) }
	download := func(ctx context.Context, tr *downloader.Track, _ any, _ func(int64, int64)) (Result, error) {
		mu.Lock()
		gate := gates[tr.ID]
		mu.Unlock()
		started <- tr.ID
		select {
		case <-gate:
			return Result{Format: "FLAC"}, nil
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}
	m := New(sink, resolve, download, nil, func() int { return 3 })
	m.Submit([]string{"album"})

	waitFor(t, "3 active and 2 waiting", func() bool {
		s := m.Snapshot()
		return s.Active == 3 && s.Waiting == 2
	})
	s := m.Snapshot()
	var positions []int
	for _, task := range s.Tasks {
		if task.State == StateQueued {
			positions = append(positions, task.Position)
		}
	}
	if fmt.Sprint(positions) != "[1 2]" {
		t.Fatalf("waiting positions = %v, want [1 2]", positions)
	}

	first := <-started
	release(first)
	waitFor(t, "next queued task starts", func() bool {
		s := m.Snapshot()
		return s.Waiting == 1 && s.Active == 3
	})
}

// 2.3: отмена одной задачи не трогает соседние.
func TestCancelOneOfThree(t *testing.T) {
	sink := newTestSink()
	resolve := func(_ context.Context, _ string) (Resolved, error) {
		return Resolved{Label: "Альбом", Tracks: []*downloader.Track{track("1"), track("2"), track("3")}}, nil
	}
	gates := map[string]chan struct{}{}
	for _, id := range []string{"1", "2", "3"} {
		gates[id] = make(chan struct{})
	}
	download := func(ctx context.Context, tr *downloader.Track, _ any, _ func(int64, int64)) (Result, error) {
		select {
		case <-gates[tr.ID]:
			return Result{Format: "FLAC"}, nil
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}
	m := New(sink, resolve, download, nil, func() int { return 3 })
	m.Submit([]string{"album"})
	waitFor(t, "all three downloading", func() bool { return m.Snapshot().Active == 3 })

	target := ""
	for _, task := range m.Snapshot().Tasks {
		if task.TrackID == "2" {
			target = task.Key
		}
	}
	m.CancelTask(target)
	waitFor(t, "one canceled", func() bool {
		s := m.Snapshot()
		return s.Canceled == 1 && s.Active == 2 && s.Waiting == 0
	})
	// The other two keep downloading until their gates open.
	s := m.Snapshot()
	if s.Done != 0 {
		t.Fatalf("neighbours must keep running, done = %d", s.Done)
	}
	for _, id := range []string{"1", "3"} {
		close(gates[id])
	}
	waitFor(t, "two done", func() bool { return m.Snapshot().Done == 2 })
}

// 2.3: повтор упавшей задачи ставит её в конец очереди и она скачивается.
func TestRetryFailedTask(t *testing.T) {
	sink := newTestSink()
	resolve := func(_ context.Context, _ string) (Resolved, error) {
		return Resolved{Label: "Трек", Tracks: []*downloader.Track{track("bad")}}, nil
	}
	var attempts atomic.Int32
	download := func(_ context.Context, tr *downloader.Track, _ any, _ func(int64, int64)) (Result, error) {
		if tr.ID == "bad" && attempts.Load() == 0 {
			attempts.Add(1)
			return Result{}, errors.New("сеть упала")
		}
		return Result{Format: "MP3"}, nil
	}
	m := New(sink, resolve, download, nil, func() int { return 1 })
	m.Submit([]string{"track"})
	waitFor(t, "first attempt fails", func() bool { return m.Snapshot().Failed == 1 })
	key := m.Snapshot().Tasks[0].Key
	m.RetryTask(key)
	waitFor(t, "retry succeeds", func() bool { return m.Snapshot().Done == 1 })
}

// 2.3: одинаковый трек не ставится в очередь дважды.
func TestDedupeAcrossSources(t *testing.T) {
	sink := newTestSink()
	resolve := func(_ context.Context, link string) (Resolved, error) {
		if link == "one" {
			return Resolved{Label: "Один", Tracks: []*downloader.Track{track("x"), track("x")}}, nil
		}
		return Resolved{Label: "Два", Tracks: []*downloader.Track{track("x")}}, nil
	}
	gate := make(chan struct{})
	download := func(ctx context.Context, _ *downloader.Track, _ any, _ func(int64, int64)) (Result, error) {
		select {
		case <-gate:
			return Result{Format: "FLAC"}, nil
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}
	m := New(sink, resolve, download, nil, func() int { return 1 })
	m.Submit([]string{"one", "two"})
	waitFor(t, "all sources expanded", func() bool {
		s := m.Snapshot()
		return len(s.Sources) == 2 && s.Total == 3
	})
	close(gate)
	waitFor(t, "single download", func() bool { return m.Snapshot().Done == 1 })
	s := m.Snapshot()
	skipped := 0
	for _, task := range s.Tasks {
		if task.TrackID == "x" && task.State == StateSkipped {
			skipped++
			if task.Reason != "уже в очереди" {
				t.Fatalf("skip reason = %q", task.Reason)
			}
		}
	}
	if skipped != 2 {
		t.Fatalf("skipped duplicates = %d, want 2", skipped)
	}
	if !sink.hasLog("(уже в очереди)") {
		t.Fatal("duplicate log line missing")
	}
}

// 2.4: два дубля ждут собственного решения, остальные качаются.
func TestDuplicateReviewDoesNotBlockQueue(t *testing.T) {
	sink := newTestSink()
	resolve := func(_ context.Context, _ string) (Resolved, error) {
		return Resolved{Label: "Альбом", Tracks: []*downloader.Track{track("dup1"), track("dup2"), track("ok")}}, nil
	}
	duplicate := func(tr *downloader.Track) (Candidate, bool) {
		if tr.ID == "dup1" || tr.ID == "dup2" {
			return Candidate{Percent: 93, FilePath: `C:\Music\x.mp3`, FileName: "x.mp3"}, true
		}
		return Candidate{}, false
	}
	gate := make(chan struct{})
	download := func(ctx context.Context, _ *downloader.Track, _ any, _ func(int64, int64)) (Result, error) {
		select {
		case <-gate:
			return Result{Format: "FLAC"}, nil
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}
	m := New(sink, resolve, download, duplicate, func() int { return 2 })
	m.Submit([]string{"album"})
	waitFor(t, "duplicates wait, ok downloads", func() bool {
		s := m.Snapshot()
		return s.Review == 2 && s.Active == 1
	})
	var dup1, dup2 string
	for _, task := range m.Snapshot().Tasks {
		switch task.TrackID {
		case "dup1":
			dup1 = task.Key
			if task.Candidate == nil || task.Candidate.Percent != 93 {
				t.Fatalf("candidate missing: %+v", task.Candidate)
			}
		case "dup2":
			dup2 = task.Key
		}
	}
	m.ResolveDuplicate(dup1, true)
	waitFor(t, "accepted duplicate starts", func() bool {
		s := m.Snapshot()
		return s.Active == 2 && s.Review == 1
	})
	m.ResolveDuplicate(dup2, false)
	waitFor(t, "declined duplicate is skipped", func() bool {
		return taskByTrack(m.Snapshot(), "dup2").State == StateSkipped
	})
	close(gate)
	waitFor(t, "queue settles", func() bool {
		s := m.Snapshot()
		return s.Done == 2 && s.Active == 0
	})
}

// 2.5: процент очереди = (завершённые + доли активных) / всего.
func TestAggregatePercent(t *testing.T) {
	sink := newTestSink()
	var tracks []*downloader.Track
	for i := 0; i < 20; i++ {
		tracks = append(tracks, track(fmt.Sprintf("t%02d", i)))
	}
	resolve := func(_ context.Context, _ string) (Resolved, error) {
		return Resolved{Label: "Альбом", Tracks: tracks}, nil
	}
	gate := make(chan struct{})
	download := func(ctx context.Context, tr *downloader.Track, _ any, onProgress func(int64, int64)) (Result, error) {
		// The 10 first tracks finish immediately; the next two report a
		// half and wait, everything else stays queued.
		switch {
		case tr.ID < "t10":
			return Result{Format: "FLAC"}, nil
		case tr.ID == "t10" || tr.ID == "t11":
			onProgress(50, 100)
			select {
			case <-gate:
				return Result{Format: "FLAC"}, nil
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
		}
		return Result{Format: "FLAC"}, nil
	}
	m := New(sink, resolve, download, nil, func() int { return 2 })
	m.Submit([]string{"album"})
	waitFor(t, "10 done and 2 active at 50%", func() bool {
		s := m.Snapshot()
		return s.Done == 10 && s.Active == 2 && s.Percent == 55
	})
	s := m.Snapshot()
	if s.Percent != 55 {
		t.Fatalf("percent = %d, want 55", s.Percent)
	}
	if s.Waiting != 8 {
		t.Fatalf("waiting = %d, want 8", s.Waiting)
	}
	if s.BytesTotal != 200 || s.BytesDone != 100 {
		t.Fatalf("bytes = %d/%d, want 100/200", s.BytesDone, s.BytesTotal)
	}
	close(gate)
	waitFor(t, "queue settles", func() bool {
		s := m.Snapshot()
		return s.Active == 0 && s.Done+s.Skipped+s.Failed+s.Canceled == s.Total
	})
}

// 2.3/2.5: «Остановить всё» замораживает очередь, новая постановка работает.
func TestCancelAllAndRearm(t *testing.T) {
	sink := newTestSink()
	resolve := func(_ context.Context, link string) (Resolved, error) {
		if link == "many" {
			return Resolved{Label: "Альбом", Tracks: []*downloader.Track{track("1"), track("2"), track("3")}}, nil
		}
		return Resolved{Label: "Трек", Tracks: []*downloader.Track{track("new")}}, nil
	}
	gate := make(chan struct{})
	download := func(ctx context.Context, _ *downloader.Track, _ any, _ func(int64, int64)) (Result, error) {
		select {
		case <-gate:
			return Result{Format: "FLAC"}, nil
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}
	m := New(sink, resolve, download, nil, func() int { return 1 })
	m.Submit([]string{"many"})
	waitFor(t, "one active", func() bool { return m.Snapshot().Active == 1 })
	m.CancelAll()
	waitFor(t, "all canceled", func() bool {
		s := m.Snapshot()
		return s.Canceled == 3 && s.Active == 0
	})
	// A new submission re-arms the queue and downloads normally.
	m.Submit([]string{"single"})
	waitFor(t, "new task starts", func() bool { return m.Snapshot().Active == 1 })
	close(gate)
	waitFor(t, "new task done", func() bool { return m.Snapshot().Done == 1 })
}

// 2.2: изменение лимита воркеров применяется к ожидающим задачам сразу.
func TestWorkersChangeAppliesImmediately(t *testing.T) {
	sink := newTestSink()
	resolve := func(_ context.Context, _ string) (Resolved, error) {
		return Resolved{Label: "Альбом", Tracks: []*downloader.Track{track("1"), track("2"), track("3")}}, nil
	}
	var mu sync.Mutex
	workers := 1
	gate := make(chan struct{})
	download := func(ctx context.Context, _ *downloader.Track, _ any, _ func(int64, int64)) (Result, error) {
		select {
		case <-gate:
			return Result{Format: "FLAC"}, nil
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}
	m := New(sink, resolve, download, nil, func() int {
		mu.Lock()
		defer mu.Unlock()
		return workers
	})
	m.Submit([]string{"album"})
	waitFor(t, "one active and two waiting", func() bool {
		s := m.Snapshot()
		return s.Active == 1 && s.Waiting == 2
	})

	mu.Lock()
	workers = 2
	mu.Unlock()
	m.Touch()
	waitFor(t, "second task starts without waiting for a finish", func() bool {
		s := m.Snapshot()
		return s.Active == 2 && s.Waiting == 1
	})

	mu.Lock()
	workers = 3
	mu.Unlock()
	m.Touch()
	waitFor(t, "third task starts", func() bool {
		s := m.Snapshot()
		return s.Active == 3 && s.Waiting == 0
	})
	if got := m.Snapshot().Workers; got != 3 {
		t.Fatalf("snapshot workers = %d, want 3", got)
	}
	close(gate)
	waitFor(t, "all done", func() bool { return m.Snapshot().Done == 3 })
}

// 2.5: список сгруппирован — активные сверху, ожидающие, затем завершённые.
func TestSnapshotGroupsActiveFirst(t *testing.T) {
	sink := newTestSink()
	resolve := func(_ context.Context, _ string) (Resolved, error) {
		return Resolved{Label: "Альбом", Tracks: []*downloader.Track{
			track("1"), track("2"), track("3"), track("4"),
		}}, nil
	}
	gates := map[string]chan struct{}{}
	for _, id := range []string{"1", "2", "3", "4"} {
		gates[id] = make(chan struct{})
	}
	download := func(ctx context.Context, tr *downloader.Track, _ any, _ func(int64, int64)) (Result, error) {
		select {
		case <-gates[tr.ID]:
			return Result{Format: "FLAC"}, nil
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}
	m := New(sink, resolve, download, nil, func() int { return 2 })
	m.Submit([]string{"album"})
	waitFor(t, "first two downloading", func() bool {
		s := m.Snapshot()
		return s.Active == 2 && s.Waiting == 2
	})

	// Первый трек завершается, освободившийся слот занимает третий.
	close(gates["1"])
	waitFor(t, "third task starts", func() bool {
		s := m.Snapshot()
		return s.Done == 1 && s.Active == 2 && s.Waiting == 1
	})
	s := m.Snapshot()
	got := make([]string, 0, len(s.Tasks))
	for _, task := range s.Tasks {
		got = append(got, task.TrackID)
	}
	want := []string{"2", "3", "4", "1"} // активные, ожидание, завершённые
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("display order = %v, want %v", got, want)
	}
	if s.Tasks[2].Position != 1 {
		t.Fatalf("waiting position = %d, want 1", s.Tasks[2].Position)
	}
	close(gates["2"])
	close(gates["3"])
	close(gates["4"])
	waitFor(t, "all done", func() bool { return m.Snapshot().Done == 4 })
}

// 2.5: очистка завершённых не трогает активные строки.
func TestClearFinishedKeepsActive(t *testing.T) {
	sink := newTestSink()
	resolve := func(_ context.Context, _ string) (Resolved, error) {
		return Resolved{Label: "Альбом", Tracks: []*downloader.Track{track("done"), track("slow")}}, nil
	}
	gate := make(chan struct{})
	download := func(ctx context.Context, tr *downloader.Track, _ any, _ func(int64, int64)) (Result, error) {
		if tr.ID == "done" {
			return Result{Format: "FLAC"}, nil
		}
		select {
		case <-gate:
			return Result{Format: "FLAC"}, nil
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}
	m := New(sink, resolve, download, nil, func() int { return 2 })
	m.Submit([]string{"album"})
	waitFor(t, "one done and one active", func() bool {
		s := m.Snapshot()
		return s.Done == 1 && s.Active == 1
	})
	m.ClearFinished()
	s := m.Snapshot()
	if s.Total != 1 || s.Done != 0 || s.Active != 1 {
		t.Fatalf("after clear: total=%d done=%d active=%d", s.Total, s.Done, s.Active)
	}
	close(gate)
	waitFor(t, "active finishes", func() bool { return m.Snapshot().Done == 1 })
}
