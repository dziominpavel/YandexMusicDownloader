package quality

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"yamdl/internal/downloader"
	"yamdl/internal/library"
)

type fakeIndex struct{ entries []library.Entry }

func (f *fakeIndex) Snapshot() []library.Entry { return f.entries }

type fakeProber struct {
	uid    int64
	plus   bool
	probes map[string]downloader.Probe
	errs   map[string]error
	calls  []string
}

func (f *fakeProber) AccountState(context.Context) (int64, bool, error) {
	return f.uid, f.plus, nil
}

func (f *fakeProber) ProbeLossless(_ context.Context, id string) (downloader.Probe, error) {
	f.calls = append(f.calls, id)
	if err := f.errs[id]; err != nil {
		return downloader.Probe{}, err
	}
	return f.probes[id], nil
}

type fakeSearcher struct {
	tracks []*downloader.Track
	err    error
	calls  []string
}

func (f *fakeSearcher) SearchTracks(_ context.Context, q string) ([]*downloader.Track, error) {
	f.calls = append(f.calls, q)
	if f.err != nil {
		return nil, f.err
	}
	return f.tracks, nil
}

func newTestChecker(t *testing.T, entries []library.Entry, p *fakeProber, s *fakeSearcher) (*Checker, *Journal, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flac-quality.json")
	j := NewJournal(path)
	c := &Checker{
		Index: &fakeIndex{entries: entries}, Prober: p, Searcher: s,
		Journal: j, Now: func() time.Time { return testTime() },
	}
	return c, j, path
}

func idEntry() library.Entry {
	return library.Entry{
		Path: `c:\music\a.mp3`, Ext: ".mp3", TrackID: "1",
		Artists: "Artist", Title: "Song", Matchable: true,
	}
}

func searchEntry() library.Entry {
	return library.Entry{
		Path: `c:\music\b.mp3`, Ext: ".mp3",
		Artists: "Artist", Title: "Song", Matchable: true,
	}
}

func TestCheckerIDLossless(t *testing.T) {
	p := &fakeProber{uid: 42, plus: true, probes: map[string]downloader.Probe{
		"1": {Available: true, Codec: "flac"},
	}}
	var logs []string
	c, j, path := newTestChecker(t, []library.Entry{idEntry()}, p, &fakeSearcher{})
	c.Log = func(s string) { logs = append(logs, s) }
	res, err := c.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Found) != 1 || !res.Found[0].Selected || res.Found[0].TrackID != "1" {
		t.Fatalf("unexpected found: %+v", res.Found)
	}
	e, ok := j.Get("1")
	if !ok || e.Status != StatusLossless || e.Codec != "flac" {
		t.Fatalf("journal entry: %+v ok=%v", e, ok)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "проверено 1, найдено 1") {
		t.Fatalf("missing summary: %v", logs)
	}
	if loaded := NewJournal(path); loaded.Status().Found != 1 {
		t.Fatalf("status must survive a reload: %+v", loaded.Status())
	}
}

func TestCheckerNoLossless(t *testing.T) {
	p := &fakeProber{uid: 42, plus: true, probes: map[string]downloader.Probe{
		"1": {Available: false, Codec: "mp3", Bitrate: 320},
	}}
	c, j, _ := newTestChecker(t, []library.Entry{idEntry()}, p, &fakeSearcher{})
	res, err := c.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Found) != 0 {
		t.Fatalf("mp3 must not be offered: %+v", res.Found)
	}
	e, _ := j.Get("1")
	if e.Status != StatusNoLossless || e.Codec != "mp3" {
		t.Fatalf("journal entry: %+v", e)
	}
}

func TestCheckerFatalPreviewAborts(t *testing.T) {
	p := &fakeProber{uid: 42, plus: true, errs: map[string]error{"1": downloader.ErrPreview}}
	c, j, _ := newTestChecker(t, []library.Entry{idEntry()}, p, &fakeSearcher{})
	_, err := c.Check(context.Background())
	if !errors.Is(err, downloader.ErrPreview) {
		t.Fatalf("want ErrPreview, got %v", err)
	}
	if _, ok := j.Get("1"); ok {
		t.Fatal("preview must not poison the journal")
	}
}

func TestCheckerUnavailableAndNetwork(t *testing.T) {
	p := &fakeProber{uid: 42, plus: true, errs: map[string]error{
		"1": downloader.ErrUnavailable,
	}}
	c, j, _ := newTestChecker(t, []library.Entry{idEntry()}, p, &fakeSearcher{})
	res, err := c.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Found) != 0 {
		t.Fatalf("unavailable must not be offered: %+v", res.Found)
	}
	if e, _ := j.Get("1"); e.Status != StatusUnavailable {
		t.Fatalf("want unavailable, got %+v", e)
	}

	p = &fakeProber{uid: 42, plus: true, errs: map[string]error{
		"1": downloader.ErrCheckFailed,
	}}
	c, j, _ = newTestChecker(t, []library.Entry{idEntry()}, p, &fakeSearcher{})
	if _, err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e, _ := j.Get("1"); e.Status != StatusError {
		t.Fatalf("want error status, got %+v", e)
	}

	// A single 403 is a per-track rights answer: mark unavailable and
	// keep the check going.
	p = &fakeProber{uid: 42, plus: true, errs: map[string]error{
		"1": downloader.ErrForbidden,
	}}
	c, j, _ = newTestChecker(t, []library.Entry{idEntry()}, p, &fakeSearcher{})
	if _, err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e, _ := j.Get("1"); e.Status != StatusUnavailable {
		t.Fatalf("single 403 must be unavailable, got %+v", e)
	}
}

func TestCheckerForbiddenStreakAborts(t *testing.T) {
	entries := []library.Entry{
		{Path: `c:\music\a.mp3`, Ext: ".mp3", TrackID: "1", Artists: "A", Title: "A", Matchable: true},
		{Path: `c:\music\b.mp3`, Ext: ".mp3", TrackID: "2", Artists: "B", Title: "B", Matchable: true},
		{Path: `c:\music\c.mp3`, Ext: ".mp3", TrackID: "3", Artists: "C", Title: "C", Matchable: true},
	}
	p := &fakeProber{uid: 42, plus: true, errs: map[string]error{
		"1": downloader.ErrForbidden, "2": downloader.ErrForbidden, "3": downloader.ErrForbidden,
	}}
	c, _, _ := newTestChecker(t, entries, p, &fakeSearcher{})
	if _, err := c.Check(context.Background()); !errors.Is(err, downloader.ErrForbidden) {
		t.Fatalf("three 403s in a row must abort, got %v", err)
	}
}

func TestCheckerNoPlus(t *testing.T) {
	p := &fakeProber{uid: 42, plus: false}
	c, _, _ := newTestChecker(t, []library.Entry{idEntry()}, p, &fakeSearcher{})
	_, err := c.Check(context.Background())
	if !errors.Is(err, ErrNoPlus) {
		t.Fatalf("want ErrNoPlus, got %v", err)
	}
	if len(p.calls) != 0 {
		t.Fatalf("no probes without Plus, got %v", p.calls)
	}
}

func TestCheckerSearchExact(t *testing.T) {
	p := &fakeProber{uid: 42, plus: true, probes: map[string]downloader.Probe{
		"42": {Available: true, Codec: "flac"},
	}}
	s := &fakeSearcher{tracks: []*downloader.Track{{
		ID: "42", Title: "Song", Artists: []string{"Artist"},
		Album: "Album", Year: "2020", Available: true,
	}}}
	c, j, _ := newTestChecker(t, []library.Entry{searchEntry()}, p, s)
	res, err := c.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Found) != 1 {
		t.Fatalf("want one search match, got %+v", res.Found)
	}
	it := res.Found[0]
	if it.MatchSource != "search" || it.TrackID != "42" || !it.Selected || it.Album != "Album" {
		t.Fatalf("unexpected item: %+v", it)
	}
	if e, _ := j.Get("file:" + searchEntry().Path); e.Status != StatusLossless || e.MatchedTrackID != "42" {
		t.Fatalf("file journal entry: %+v", e)
	}
}

func TestCheckerSearchFuzzyNotSelected(t *testing.T) {
	p := &fakeProber{uid: 42, plus: true, probes: map[string]downloader.Probe{
		"42": {Available: true, Codec: "flac"},
	}}
	s := &fakeSearcher{tracks: []*downloader.Track{{
		ID: "42", Title: "Светлая полосы", Artists: []string{"Artist"}, Available: true,
	}}}
	e := searchEntry()
	e.Title = "Светлая полоса"
	c, _, _ := newTestChecker(t, []library.Entry{e}, p, s)
	res, err := c.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Found) != 1 || res.Found[0].Selected {
		t.Fatalf("fuzzy match must be offered but not preselected: %+v", res.Found)
	}
}

func TestCheckerSearchNoMatch(t *testing.T) {
	p := &fakeProber{uid: 42, plus: true}
	s := &fakeSearcher{tracks: []*downloader.Track{{
		ID: "42", Title: "Completely different", Artists: []string{"Other"}, Available: true,
	}}}
	c, j, _ := newTestChecker(t, []library.Entry{searchEntry()}, p, s)
	res, err := c.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Found) != 0 {
		t.Fatalf("no match expected: %+v", res.Found)
	}
	if e, _ := j.Get("file:" + searchEntry().Path); e.Status != StatusNoMatch {
		t.Fatalf("want no_match, got %+v", e)
	}
}

func TestCheckerTTLAndPendingSkip(t *testing.T) {
	now := testTime()
	p := &fakeProber{uid: 42, plus: true}
	c, j, _ := newTestChecker(t, []library.Entry{idEntry()}, p, &fakeSearcher{})
	j.Put("1", Entry{Status: StatusNoLossless, NextCheck: now.Add(NoLosslessTTL)})
	if _, err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(p.calls) != 0 {
		t.Fatalf("TTL must skip the probe, got %v", p.calls)
	}

	j.Put("1", Entry{Status: StatusLossless, Codec: "flac"})
	res, err := c.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Found) != 1 || len(p.calls) != 0 {
		t.Fatalf("pending lossless must be shown without a probe: %+v calls=%v", res.Found, p.calls)
	}
}

func TestCheckerRechecksDeclinedLossless(t *testing.T) {
	now := testTime()
	p := &fakeProber{uid: 42, plus: true}
	s := &fakeSearcher{tracks: []*downloader.Track{{
		ID: "42", Title: "Song", Artists: []string{"Artist"}, Available: true,
	}}}
	c, j, _ := newTestChecker(t, []library.Entry{searchEntry()}, p, s)
	j.Put("file:"+searchEntry().Path, Entry{
		Status: StatusLossless, Codec: "flac", MatchSource: "search",
		MatchedTrackID: "42", MatchedArtist: "Artist", MatchedTitle: "Song", Exact: true,
	})
	j.Decline(searchEntry().Path, Declined{TrackID: "42", DeclinedAt: now})

	res, err := c.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Found) != 0 {
		t.Fatalf("declined lossless must not be offered: %+v", res.Found)
	}
	if len(s.calls) != 1 {
		t.Fatalf("declined file must be searched again, got %v", s.calls)
	}
	e, ok := j.Get("file:" + searchEntry().Path)
	if !ok || e.Status != StatusNoMatch {
		t.Fatalf("want no_match after recheck, got %+v ok=%v", e, ok)
	}
	if !e.NextCheck.Equal(now.Add(AutoCheckInterval)) {
		t.Fatalf("all-declined retry must be 7 days, got %v", e.NextCheck)
	}
}

func TestCheckerDeclinedCandidateFallsThrough(t *testing.T) {
	now := testTime()
	p := &fakeProber{uid: 42, plus: true, probes: map[string]downloader.Probe{
		"43": {Available: true, Codec: "flac"},
	}}
	s := &fakeSearcher{tracks: []*downloader.Track{
		{ID: "42", Title: "Song", Artists: []string{"Artist"}, Available: true},
		{ID: "43", Title: "Song", Artists: []string{"Artist"}, Available: true},
	}}
	c, j, _ := newTestChecker(t, []library.Entry{searchEntry()}, p, s)
	j.Decline(searchEntry().Path, Declined{TrackID: "42", DeclinedAt: now})

	res, err := c.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Found) != 1 || res.Found[0].TrackID != "43" {
		t.Fatalf("next candidate must be offered: %+v", res.Found)
	}
	if len(p.calls) != 1 || p.calls[0] != "43" {
		t.Fatalf("only the non-rejected candidate must be probed: %v", p.calls)
	}
	if e, _ := j.Get("file:" + searchEntry().Path); e.MatchedTrackID != "43" {
		t.Fatalf("journal must point at the new candidate: %+v", e)
	}
}

func TestCheckerNoCandidatesUsesThirtyDayTTL(t *testing.T) {
	now := testTime()
	p := &fakeProber{uid: 42, plus: true}
	s := &fakeSearcher{tracks: []*downloader.Track{{
		ID: "42", Title: "Completely different", Artists: []string{"Other"}, Available: true,
	}}}
	c, j, _ := newTestChecker(t, []library.Entry{searchEntry()}, p, s)
	if _, err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	e, _ := j.Get("file:" + searchEntry().Path)
	if e.Status != StatusNoMatch || !e.NextCheck.Equal(now.Add(NoLosslessTTL)) {
		t.Fatalf("no-candidate retry must stay 30 days: %+v", e)
	}
}

func TestCheckerIDMatchIgnoresDecline(t *testing.T) {
	now := testTime()
	p := &fakeProber{uid: 42, plus: true}
	c, j, _ := newTestChecker(t, []library.Entry{idEntry()}, p, &fakeSearcher{})
	j.Put("1", Entry{Status: StatusLossless, Codec: "flac", MatchSource: "id"})
	j.Decline(idEntry().Path, Declined{TrackID: "1", DeclinedAt: now})

	res, err := c.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Found) != 1 || res.Found[0].MatchSource != "id" {
		t.Fatalf("ID match must ignore the rejection filter: %+v", res.Found)
	}
	if len(p.calls) != 0 {
		t.Fatalf("ID lossless must be served from the journal, got probes %v", p.calls)
	}
}

func TestCheckerProgressAndThrottle(t *testing.T) {
	p := &fakeProber{uid: 42, plus: true, probes: map[string]downloader.Probe{
		"1": {Available: true, Codec: "flac"},
	}}
	c, _, _ := newTestChecker(t, []library.Entry{idEntry()}, p, &fakeSearcher{})
	c.Throttle = 10 * time.Millisecond
	var done, total int
	c.Progress = func(d, tt int) { done, total = d, tt }
	start := time.Now()
	if _, err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if done != 1 || total != 1 {
		t.Fatalf("progress = %d/%d, want 1/1", done, total)
	}
	if elapsed := time.Since(start); elapsed < 8*time.Millisecond {
		t.Fatalf("throttle not applied, elapsed %v", elapsed)
	}
}

func TestHumanError(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{downloader.ErrPreview, "токен"},
		{downloader.ErrAuth, "токен"},
		{downloader.ErrForbidden, "403"},
		{ErrNoPlus, "Плюс"},
		{context.Canceled, "отменена"},
		{errors.New("boom"), "boom"},
	}
	for _, c := range cases {
		if got := HumanError(c.err); !strings.Contains(got, c.want) {
			t.Errorf("HumanError(%v) = %q, want substring %q", c.err, got, c.want)
		}
	}
}

func TestCheckerCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &fakeProber{uid: 42, plus: true}
	c, _, _ := newTestChecker(t, []library.Entry{idEntry()}, p, &fakeSearcher{})
	res, err := c.Check(ctx)
	if !errors.Is(err, context.Canceled) || !res.Canceled {
		t.Fatalf("want canceled result, got %+v err=%v", res, err)
	}
}
