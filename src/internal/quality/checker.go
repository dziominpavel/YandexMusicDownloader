package quality

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"yamdl/internal/downloader"
	"yamdl/internal/library"
)

// ErrNoPlus is a check-level stop: the account cannot receive lossless.
var ErrNoPlus = errors.New("нет активной подписки Плюс")

// Snapshotter is the read-only slice of the collection index.
type Snapshotter interface {
	Snapshot() []library.Entry
}

// Prober is implemented by downloader.Client.
type Prober interface {
	AccountState(ctx context.Context) (int64, bool, error)
	ProbeLossless(ctx context.Context, trackID string) (downloader.Probe, error)
}

// Searcher is implemented by downloader.Client.
type Searcher interface {
	SearchTracks(ctx context.Context, query string) ([]*downloader.Track, error)
}

// Checker probes candidates and writes the journal. It never writes
// audio files.
type Checker struct {
	Index    Snapshotter
	Prober   Prober
	Searcher Searcher
	Journal  *Journal
	// Now defaults to time.Now (tests use a fixed clock).
	Now      func() time.Time
	Log      func(string)
	Progress func(done, total int)
	Throttle time.Duration
	// ConvertM4A/FFmpeg decide whether flac-mp4 will become FLAC or M4A
	// in the found-item label. They do not change probing.
	ConvertM4A bool
	FFmpeg     bool
	// MaxSearchCandidates caps the number of search results probed per
	// local file (default 3).
	MaxSearchCandidates int
}

// Result is one check run.
type Result struct {
	Found         []FoundItem
	Candidates    int
	Checked       int
	LastAttempt   time.Time
	LastCompleted time.Time
	Error         string
	Canceled      bool
}

// Check runs one collection scan. Fatal token/permission answers abort
// the scan; per-track failures are recorded and do not stop the rest.
func (c *Checker) Check(ctx context.Context) (Result, error) {
	now := c.now()
	if c.Index == nil || c.Prober == nil {
		return Result{}, fmt.Errorf("%w: checker is not configured", downloader.ErrCheckFailed)
	}
	entries := c.Index.Snapshot()
	candidates := Candidates(entries)
	if c.Journal != nil {
		c.Journal.Begin(now)
	}
	c.logf("[flac] проверка: %d кандидатов", len(candidates))
	if len(candidates) == 0 {
		return c.finish(now, entries, 0, nil)
	}

	uid, hasPlus, err := c.Prober.AccountState(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return c.finishCanceled(now, entries, 0)
		}
		return c.finish(now, entries, 0, err)
	}
	if !hasPlus {
		return c.finish(now, entries, 0, ErrNoPlus)
	}
	if c.Journal != nil && c.Journal.SetAccount(uid, hasPlus) {
		c.Journal.Save()
	}

	flacByID := map[string]string{}
	for _, e := range entries {
		if strings.EqualFold(e.Ext, ".flac") && e.TrackID != "" {
			flacByID[e.TrackID] = e.Path
		}
	}

	processed := 0
	forbiddenStreak := 0
	for i, cand := range candidates {
		if ctx.Err() != nil {
			return c.finishCanceled(now, entries, processed)
		}
		key := cand.TrackID
		if key == "" {
			key = "file:" + cand.Path
		}

		if c.Journal != nil {
			if je, ok := c.Journal.Get(key); ok && je.Status == StatusLossless {
				processed++
				c.progress(i+1, len(candidates))
				continue
			}
			if !c.Journal.Due(key, now) {
				processed++
				c.progress(i+1, len(candidates))
				continue
			}
		}

		if cand.TrackID != "" {
			if flacPath := flacByID[cand.TrackID]; flacPath != "" {
				c.put(key, Entry{Status: StatusLossless, Codec: "flac", CheckedAt: now, MatchSource: "id"})
				c.logf("[found] %s (FLAC уже есть: %s)", candidateLabel(cand), flacPath)
			} else {
				probe, perr := c.Prober.ProbeLossless(ctx, cand.TrackID)
				if perr != nil {
					if ctx.Err() != nil {
						return c.finishCanceled(now, entries, processed)
					}
					if fatal(perr) {
						return c.finish(now, entries, processed, perr)
					}
					if errors.Is(perr, downloader.ErrForbidden) {
						forbiddenStreak++
						if forbiddenStreak >= 3 {
							return c.finish(now, entries, processed, perr)
						}
					} else {
						forbiddenStreak = 0
					}
					c.recordProbeError(key, now, perr)
					c.logf("[warn] %s: %s", candidateLabel(cand), HumanError(perr))
				} else if probe.Available {
					forbiddenStreak = 0
					c.put(key, Entry{Status: StatusLossless, Codec: probe.Codec, CheckedAt: now, MatchSource: "id"})
					c.logf("[found] %s (%s)", candidateLabel(cand), c.opts().FoundFormat(probe.Codec))
				} else {
					forbiddenStreak = 0
					c.put(key, Entry{
						Status: StatusNoLossless, Codec: probe.Codec, CheckedAt: now,
						NextCheck: now.Add(NoLosslessTTL), MatchSource: "id",
					})
				}
				c.throttle(ctx)
			}
		} else if err := c.searchCandidate(ctx, cand, now, flacByID); err != nil {
			if ctx.Err() != nil {
				return c.finishCanceled(now, entries, processed)
			}
			return c.finish(now, entries, processed, err)
		}

		processed++
		c.progress(i+1, len(candidates))
	}
	return c.finish(now, entries, processed, nil)
}

func (c *Checker) searchCandidate(ctx context.Context, cand Candidate, now time.Time, flacByID map[string]string) error {
	if c.Searcher == nil {
		return fmt.Errorf("%w: searcher is not configured", downloader.ErrCheckFailed)
	}
	key := "file:" + cand.Path
	title := cleanQuery(cand.Title)
	query := SearchQuery(cand)
	tracks, err := c.Searcher.SearchTracks(ctx, query)
	if err != nil {
		if fatal(err) {
			return err
		}
		c.recordSearchError(key, now, err)
		c.logf("[warn] %s: %s", candidateLabel(cand), HumanError(err))
		return nil
	}
	// A title-only retry helps when the artist spelling differs from the
	// one in Yandex. The artist is still checked by ScoreCandidate.
	if len(tracks) == 0 && len(cand.Artists) > 0 && title != "" && title != query {
		tracks, err = c.Searcher.SearchTracks(ctx, title)
		if err != nil {
			if fatal(err) {
				return err
			}
			c.recordSearchError(key, now, err)
			c.logf("[warn] %s: %s", candidateLabel(cand), HumanError(err))
			return nil
		}
	}
	if len(tracks) == 0 {
		c.put(key, Entry{Status: StatusNoMatch, CheckedAt: now, NextCheck: now.Add(NoLosslessTTL)})
		return nil
	}

	type scored struct {
		t     *downloader.Track
		score library.CandidateScore
	}
	var list []scored
	for _, t := range tracks {
		if t == nil || !t.Available {
			continue
		}
		score, ok := library.ScoreCandidate(cand.Artists, title, t.Artists, t.FullTitle())
		if !ok {
			continue
		}
		if c.Journal != nil && c.Journal.IsDeclined(cand.Path, t.ID, now) {
			continue
		}
		list = append(list, scored{t: t, score: score})
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].score.Exact != list[j].score.Exact {
			return list[i].score.Exact
		}
		if list[i].score.Percent != list[j].score.Percent {
			return list[i].score.Percent > list[j].score.Percent
		}
		return list[i].t.ID < list[j].t.ID
	})
	if len(list) == 0 {
		c.put(key, Entry{Status: StatusNoMatch, CheckedAt: now, NextCheck: now.Add(NoLosslessTTL)})
		return nil
	}

	max := 3
	if c.MaxSearchCandidates > 0 && c.MaxSearchCandidates < max {
		max = c.MaxSearchCandidates
	}
	for i := 0; i < len(list) && i < max; i++ {
		s := &list[i]
		if flacPath := flacByID[s.t.ID]; flacPath != "" {
			entry := c.searchEntry(now, s.t, s.score, "flac")
			c.put(key, entry)
			c.put(s.t.ID, entry)
			c.logf("[found] %s (FLAC уже есть: %s; поиск: %s, %d%%)",
				candidateLabel(cand), flacPath, s.t.Label(), s.score.Percent)
			return nil
		}
		if c.Journal != nil && !c.Journal.Due(s.t.ID, now) {
			if je, ok := c.Journal.Get(s.t.ID); ok && je.Status == StatusLossless {
				codec := je.Codec
				if codec == "" {
					codec = "flac"
				}
				c.put(key, c.searchEntry(now, s.t, s.score, codec))
				return nil
			}
			continue
		}
		probe, perr := c.Prober.ProbeLossless(ctx, s.t.ID)
		if perr != nil {
			if ctx.Err() != nil || fatal(perr) {
				return perr
			}
			c.recordProbeError(s.t.ID, now, perr)
			c.logf("[warn] %s: %s", s.t.Label(), HumanError(perr))
			continue
		}
		c.throttle(ctx)
		if probe.Available {
			entry := c.searchEntry(now, s.t, s.score, probe.Codec)
			c.put(key, entry)
			c.put(s.t.ID, entry)
			c.logf("[found] %s (%s; поиск: %s, %d%%)",
				candidateLabel(cand), c.opts().FoundFormat(probe.Codec), s.t.Label(), s.score.Percent)
			return nil
		}
		c.put(s.t.ID, Entry{
			Status: StatusNoLossless, Codec: probe.Codec, CheckedAt: now,
			NextCheck: now.Add(NoLosslessTTL),
		})
	}

	c.put(key, Entry{Status: StatusNoMatch, CheckedAt: now, NextCheck: now.Add(NoLosslessTTL)})
	return nil
}

func (c *Checker) searchEntry(now time.Time, t *downloader.Track, score library.CandidateScore, codec string) Entry {
	return Entry{
		Status: StatusLossless, Codec: codec, CheckedAt: now, MatchSource: "search",
		MatchedTrackID: t.ID, MatchedArtist: strings.Join(t.Artists, ", "),
		MatchedTitle: t.FullTitle(), MatchedAlbum: t.Album, MatchedYear: t.Year,
		Confidence: score.Percent, Exact: score.Exact,
	}
}

func (c *Checker) finish(now time.Time, entries []library.Entry, processed int, checkErr error) (Result, error) {
	found := Pending(entries, c.Journal, now, c.opts())
	if c.Journal != nil {
		c.prune(entries)
		c.Journal.Finish(now, len(found), checkErr)
		c.Journal.Save()
	}
	if checkErr != nil {
		c.logf("[flac] %s", HumanError(checkErr))
	} else {
		c.logf("[flac] проверено %d, найдено %d", processed, len(found))
	}
	return c.result(entries, processed, found, checkErr, false), checkErr
}

func (c *Checker) finishCanceled(now time.Time, entries []library.Entry, processed int) (Result, error) {
	found := Pending(entries, c.Journal, now, c.opts())
	if c.Journal != nil {
		c.prune(entries)
		c.Journal.Save()
	}
	c.logf("[flac] проверка отменена")
	return c.result(entries, processed, found, nil, true), context.Canceled
}

func (c *Checker) result(entries []library.Entry, processed int, found []FoundItem, checkErr error, canceled bool) Result {
	res := Result{
		Found:      found,
		Candidates: len(Candidates(entries)),
		Checked:    processed,
		Canceled:   canceled,
	}
	if checkErr != nil {
		res.Error = HumanError(checkErr)
	}
	if c.Journal != nil {
		st := c.Journal.Status()
		res.LastAttempt = st.LastAttempt
		res.LastCompleted = st.LastCompleted
		if st.Error != "" {
			res.Error = st.Error
		}
	}
	return res
}

func (c *Checker) prune(entries []library.Entry) {
	if c.Journal == nil {
		return
	}
	keys := map[string]bool{}
	paths := map[string]bool{}
	for _, e := range entries {
		paths[e.Path] = true
		if e.TrackID != "" {
			keys[e.TrackID] = true
		}
		if strings.EqualFold(e.Ext, ".mp3") {
			keys["file:"+e.Path] = true
		}
	}
	c.Journal.Prune(keys, paths)
}

func (c *Checker) recordProbeError(key string, now time.Time, err error) {
	e := Entry{Status: StatusError, CheckedAt: now, NextCheck: now.Add(ErrorTTL)}
	if errors.Is(err, downloader.ErrUnavailable) || errors.Is(err, downloader.ErrForbidden) {
		e.Status = StatusUnavailable
		e.NextCheck = now.Add(UnavailableTTL)
	}
	c.put(key, e)
}

func (c *Checker) recordSearchError(key string, now time.Time, err error) {
	e := Entry{Status: StatusError, CheckedAt: now, NextCheck: now.Add(ErrorTTL)}
	if errors.Is(err, downloader.ErrUnavailable) || errors.Is(err, downloader.ErrForbidden) {
		e.Status = StatusUnavailable
		e.NextCheck = now.Add(UnavailableTTL)
	}
	c.put(key, e)
}

func (c *Checker) put(key string, e Entry) {
	if c.Journal != nil {
		c.Journal.Put(key, e)
	}
}

func (c *Checker) opts() FormatOptions {
	return FormatOptions{ConvertM4A: c.ConvertM4A, FFmpeg: c.FFmpeg}
}

func (c *Checker) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Checker) logf(format string, args ...any) {
	if c.Log != nil {
		c.Log(fmt.Sprintf(format, args...))
	}
}

func (c *Checker) progress(done, total int) {
	if c.Progress != nil {
		c.Progress(done, total)
	}
}

func (c *Checker) throttle(ctx context.Context) {
	if c.Throttle <= 0 {
		return
	}
	t := time.NewTimer(c.Throttle)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func fatal(err error) bool {
	return errors.Is(err, downloader.ErrPreview) ||
		errors.Is(err, downloader.ErrAuth) ||
		errors.Is(err, context.Canceled)
}

// HumanError renders the check errors in Russian for the window/log.
func HumanError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.Canceled):
		return "проверка отменена"
	case errors.Is(err, downloader.ErrPreview):
		return "токен не даёт полный доступ — проверь токен и подписку"
	case errors.Is(err, downloader.ErrAuth):
		return "токен не принят — проверь токен"
	case errors.Is(err, downloader.ErrForbidden):
		return "доступ к треку запрещён (403)"
	case errors.Is(err, ErrNoPlus):
		return "нет активной подписки Плюс — lossless недоступен"
	default:
		return err.Error()
	}
}

func candidateLabel(c Candidate) string {
	artists := strings.TrimSpace(strings.Join(c.Artists, ", "))
	title := strings.TrimSpace(c.Title)
	switch {
	case artists != "" && title != "":
		return artists + " — " + title
	case title != "":
		return title
	case artists != "":
		return artists
	default:
		return c.Path
	}
}
