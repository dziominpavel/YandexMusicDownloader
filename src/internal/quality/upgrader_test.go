package quality

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"yamdl/internal/downloader"
	"yamdl/internal/library"
)

type fakeUpgradeClient struct {
	stopped    bool
	track      *downloader.Track
	resolveErr error
}

func (f *fakeUpgradeClient) StartJob()                   {}
func (f *fakeUpgradeClient) Stop()                       { f.stopped = true }
func (f *fakeUpgradeClient) Stopped() bool               { return f.stopped }
func (f *fakeUpgradeClient) JobContext() context.Context { return context.Background() }
func (f *fakeUpgradeClient) ResolveTrack(string) (*downloader.Track, error) {
	if f.resolveErr != nil {
		return nil, f.resolveErr
	}
	return f.track, nil
}

func newTestUpgrader(t *testing.T, dir string, upgrade UpgradeFunc) (*Upgrader, *library.Index, *Journal) {
	t.Helper()
	ix := library.New(filepath.Join(t.TempDir(), "cache.json"), dir)
	ix.Refresh(nil)
	j := NewJournal("")
	client := &fakeUpgradeClient{track: &downloader.Track{ID: "42", Title: "Song", Artists: []string{"Artist"}}}
	u := &Upgrader{
		Client: client, Upgrade: upgrade, Index: ix, Journal: j,
		OutputDir: dir, Workers: 1,
		Now: func() time.Time { return testTime() },
	}
	return u, ix, j
}

func foundItem(dir, name string) FoundItem {
	p := filepath.Join(dir, name)
	return FoundItem{
		Key: p, Path: p, FileName: filepath.Base(p), Label: "Artist — Song",
		CurrentFormat: "MP3", FoundFormat: "FLAC", MatchSource: "id", TrackID: "42", Selected: true,
	}
}

func writeOld(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUpgraderSuccessMovesOld(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "Artist - Song.mp3")
	writeOld(t, old)
	var upgrades int
	var logs []string
	u, ix, j := newTestUpgrader(t, dir, func(_ context.Context, _ *downloader.Track, oldPath string, _ downloader.ProgressFunc) (downloader.Result, error) {
		upgrades++
		newPath := strings.TrimSuffix(oldPath, ".mp3") + ".flac"
		if err := os.WriteFile(newPath, []byte("new"), 0o644); err != nil {
			return downloader.Result{}, err
		}
		return downloader.Result{Path: newPath, Format: "FLAC"}, nil
	})
	u.Log = func(s string) { logs = append(logs, s) }
	ix.AddFile(old)

	sum := u.Run([]FoundItem{foundItem(dir, "Artist - Song.mp3")}, nil, nil)
	if sum.OK != 1 || upgrades != 1 {
		t.Fatalf("summary=%+v upgrades=%d", sum, upgrades)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("old MP3 must be moved")
	}
	moved := filepath.Join(dir, "_old", "Artist - Song.mp3")
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("old file must be in _old: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Artist - Song.flac")); err != nil {
		t.Fatalf("new FLAC missing: %v", err)
	}
	for _, e := range ix.Snapshot() {
		if e.Path == old {
			t.Fatal("index must drop the moved MP3")
		}
	}
	if e, ok := j.Get("42"); !ok || e.Status != StatusLossless {
		t.Fatalf("journal must mark the track lossless: %+v ok=%v", e, ok)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "[upgrade]") {
		t.Fatalf("upgrade must be logged: %v", logs)
	}
}

func TestUpgraderFallbackDoesNotMove(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "Artist - Song.mp3")
	writeOld(t, old)
	u, _, j := newTestUpgrader(t, dir, func(_ context.Context, _ *downloader.Track, _ string, _ downloader.ProgressFunc) (downloader.Result, error) {
		return downloader.Result{Fallback: true, Format: "MP3"}, nil
	})
	sum := u.Run([]FoundItem{foundItem(dir, "Artist - Song.mp3")}, nil, nil)
	if sum.Failed != 1 {
		t.Fatalf("fallback must fail the item: %+v", sum)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatal("old file must stay on fallback")
	}
	if _, err := os.Stat(filepath.Join(dir, "_old")); !os.IsNotExist(err) {
		t.Fatal("_old must not be created on fallback")
	}
	if e, _ := j.Get("42"); e.Status != StatusNoLossless {
		t.Fatalf("journal must store no_lossless: %+v", e)
	}
}

func TestUpgraderNoFLACDoesNotMove(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "Artist - Song.mp3")
	writeOld(t, old)
	u, _, j := newTestUpgrader(t, dir, func(_ context.Context, _ *downloader.Track, _ string, _ downloader.ProgressFunc) (downloader.Result, error) {
		return downloader.Result{}, downloader.ErrNoFLAC
	})
	sum := u.Run([]FoundItem{foundItem(dir, "Artist - Song.mp3")}, nil, nil)
	if sum.Failed != 1 {
		t.Fatalf("no FLAC must fail the item: %+v", sum)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatal("old file must stay when lossless disappeared")
	}
	if e, _ := j.Get("42"); e.Status != StatusNoLossless {
		t.Fatalf("journal must store no_lossless: %+v", e)
	}
}

func TestUpgraderAlreadyFlacOnlyMoves(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "Artist - Song.mp3")
	flac := filepath.Join(dir, "Artist - Song.flac")
	writeOld(t, old)
	writeOld(t, flac)
	var upgrades int
	var logs []string
	u, _, _ := newTestUpgrader(t, dir, func(_ context.Context, _ *downloader.Track, _ string, _ downloader.ProgressFunc) (downloader.Result, error) {
		upgrades++
		return downloader.Result{}, nil
	})
	u.Log = func(s string) { logs = append(logs, s) }
	it := foundItem(dir, "Artist - Song.mp3")
	it.AlreadyFlac = flac
	sum := u.Run([]FoundItem{it}, nil, nil)
	if sum.Skipped != 1 || upgrades != 0 {
		t.Fatalf("existing FLAC must only archive the MP3: summary=%+v upgrades=%d", sum, upgrades)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "[skip]") {
		t.Fatalf("archive-only action must be logged as skip: %v", logs)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("old MP3 must be moved")
	}
	if _, err := os.Stat(flac); err != nil {
		t.Fatal("existing FLAC must stay")
	}
	if _, err := os.Stat(filepath.Join(dir, "_old", "Artist - Song.mp3")); err != nil {
		t.Fatalf("old MP3 must be in _old: %v", err)
	}
}

func TestUpgraderOldCollision(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "Artist - Song.mp3")
	writeOld(t, old)
	existing := filepath.Join(dir, "_old", "Artist - Song.mp3")
	writeOld(t, existing)
	if err := os.WriteFile(existing, []byte("previous"), 0o644); err != nil {
		t.Fatal(err)
	}
	u, _, _ := newTestUpgrader(t, dir, func(_ context.Context, _ *downloader.Track, oldPath string, _ downloader.ProgressFunc) (downloader.Result, error) {
		newPath := strings.TrimSuffix(oldPath, ".mp3") + ".flac"
		_ = os.WriteFile(newPath, []byte("new"), 0o644)
		return downloader.Result{Path: newPath, Format: "FLAC"}, nil
	})
	sum := u.Run([]FoundItem{foundItem(dir, "Artist - Song.mp3")}, nil, nil)
	if sum.OK != 1 {
		t.Fatalf("collision must not fail the upgrade: %+v", sum)
	}
	got, err := os.ReadFile(existing)
	if err != nil || string(got) != "previous" {
		t.Fatalf("existing _old file must stay untouched: %q %v", got, err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "_old", "Artist - Song.*.mp3"))
	if len(matches) != 1 {
		t.Fatalf("timestamped old file expected, got %v", matches)
	}
}

func TestUpgraderStopLeavesOld(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "Artist - Song.mp3")
	writeOld(t, old)
	u, _, _ := newTestUpgrader(t, dir, func(_ context.Context, _ *downloader.Track, _ string, _ downloader.ProgressFunc) (downloader.Result, error) {
		return downloader.Result{}, downloader.ErrStopped
	})
	it := foundItem(dir, "Artist - Song.mp3")
	sum := u.Run([]FoundItem{it}, nil, nil)
	if sum.Stopped != 1 {
		t.Fatalf("stopped item must be counted separately: %+v", sum)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatal("old file must stay after stop")
	}
}

func TestUpgraderStopKeepsPendingForResume(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "Artist - Song.mp3")
	writeOld(t, old)
	u, ix, j := newTestUpgrader(t, dir, func(_ context.Context, _ *downloader.Track, _ string, _ downloader.ProgressFunc) (downloader.Result, error) {
		return downloader.Result{}, downloader.ErrStopped
	})
	ix.AddFile(old)
	j.Put("file:"+old, Entry{Status: StatusLossless, Codec: "flac", MatchSource: "id"})
	it := foundItem(dir, "Artist - Song.mp3")
	if sum := u.Run([]FoundItem{it}, nil, nil); sum.Stopped != 1 {
		t.Fatalf("first run must stop: %+v", sum)
	}
	if pending := Pending(ix.Snapshot(), j, testTime(), FormatOptions{}); len(pending) != 1 {
		t.Fatalf("stopped item must stay pending: %+v", pending)
	}

	u.Upgrade = func(_ context.Context, _ *downloader.Track, oldPath string, _ downloader.ProgressFunc) (downloader.Result, error) {
		newPath := strings.TrimSuffix(oldPath, ".mp3") + ".flac"
		_ = os.WriteFile(newPath, []byte("new"), 0o644)
		return downloader.Result{Path: newPath, Format: "FLAC"}, nil
	}
	if sum := u.Run([]FoundItem{it}, nil, nil); sum.OK != 1 {
		t.Fatalf("second run must finish: %+v", sum)
	}
	if pending := Pending(ix.Snapshot(), j, testTime(), FormatOptions{}); len(pending) != 0 {
		t.Fatalf("upgraded item must leave pending: %+v", pending)
	}
}

func TestUniqueOldPath(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "a.mp3")
	writeOld(t, existing)
	got := uniqueOldPath(existing, testTime())
	if got == existing {
		t.Fatal("collision must produce a different name")
	}
	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Fatalf("unique path must not exist yet: %s", got)
	}
	if !strings.Contains(got, ".mp3") || !strings.Contains(got, "20261103-180000") {
		t.Fatalf("unexpected unique name: %s", got)
	}
}
