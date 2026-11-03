package quality

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"yamdl/internal/downloader"
	"yamdl/internal/tagread"
)

// UpgradeClient is the part of downloader.Client the upgrader needs.
type UpgradeClient interface {
	StartJob()
	Stop()
	Stopped() bool
	JobContext() context.Context
	ResolveTrack(id string) (*downloader.Track, error)
}

// IndexUpdater keeps the collection index fresh after a moved file.
type IndexUpdater interface {
	RemoveFile(path string)
	AddFile(path string)
}

// UpgradeFunc downloads lossless next to oldPath.
type UpgradeFunc func(ctx context.Context, track *downloader.Track, oldPath string, onProgress downloader.ProgressFunc) (downloader.Result, error)

// Upgrader runs the selected upgrades as a normal stoppable job.
type Upgrader struct {
	Client     UpgradeClient
	Upgrade    UpgradeFunc
	Index      IndexUpdater
	Journal    *Journal
	LibraryDir string
	OutputDir  string
	Workers    int
	Now        func() time.Time
	Log        func(string)

	logMu sync.Mutex
}

// Run upgrades items with a small pool and returns the job summary.
func (u *Upgrader) Run(items []FoundItem, emit func(downloader.Event), onProgress downloader.ProgressFunc) downloader.Summary {
	var sum downloader.Summary
	if u.Client == nil || u.Upgrade == nil || len(items) == 0 {
		return sum
	}
	u.Client.StartJob()

	workers := u.Workers
	if workers <= 0 {
		workers = 2
	}
	if workers > 4 {
		workers = 4
	}

	var emitMu, progressMu, sumMu sync.Mutex
	safeEmit := func(e downloader.Event) {
		if emit == nil {
			return
		}
		emitMu.Lock()
		emit(e)
		emitMu.Unlock()
	}
	safeProgress := func(p downloader.Progress) {
		if onProgress == nil {
			return
		}
		progressMu.Lock()
		onProgress(p)
		progressMu.Unlock()
	}

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, it := range items {
		if u.Client.Stopped() {
			sumMu.Lock()
			sum.Stopped++
			sumMu.Unlock()
			safeEmit(downloader.Event{Kind: downloader.KindStopped, Label: it.Label, TrackID: it.Key})
			continue
		}
		wg.Add(1)
		go func(it FoundItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if u.Client.Stopped() {
				sumMu.Lock()
				sum.Stopped++
				sumMu.Unlock()
				safeEmit(downloader.Event{Kind: downloader.KindStopped, Label: it.Label, TrackID: it.Key})
				return
			}
			outcome := u.upgradeOne(it, safeEmit, safeProgress)
			sumMu.Lock()
			switch outcome {
			case outcomeOK:
				sum.OK++
			case outcomeSkip:
				sum.Skipped++
			case outcomeStopped:
				sum.Stopped++
			default:
				sum.Failed++
			}
			sumMu.Unlock()
		}(it)
	}
	wg.Wait()
	return sum
}

type outcome int

const (
	outcomeOK outcome = iota
	outcomeSkip
	outcomeStopped
	outcomeError
)

func (u *Upgrader) upgradeOne(it FoundItem, emit func(downloader.Event), onProgress downloader.ProgressFunc) outcome {
	now := u.now()
	if strings.TrimSpace(it.Label) == "" {
		it.Label = it.FileName
	}

	// The FLAC is already next to the MP3: only archive the lossy copy.
	if it.AlreadyFlac != "" {
		target, err := u.moveToOld(it.Path)
		if err != nil {
			u.logf("[warn] не удалось переместить %s в _old: %v", it.Label, err)
		} else {
			u.logf("[skip] %s: FLAC уже есть, MP3 → _old: %s", it.Label, target)
		}
		u.removeIndex(it.Path)
		u.addIndex(it.AlreadyFlac)
		emit(downloader.Event{
			Kind: downloader.KindSkipped, Label: it.Label, TrackID: it.Key,
			Path: it.AlreadyFlac,
		})
		return outcomeSkip
	}

	if strings.TrimSpace(it.TrackID) == "" {
		emit(downloader.Event{Kind: downloader.KindFailed, Label: it.Label, TrackID: it.Key, Detail: "нет Yandex ID"})
		return outcomeError
	}
	track, err := u.Client.ResolveTrack(it.TrackID)
	if err != nil {
		if u.Client.Stopped() {
			emit(downloader.Event{Kind: downloader.KindStopped, Label: it.Label, TrackID: it.Key})
			return outcomeStopped
		}
		emit(downloader.Event{Kind: downloader.KindFailed, Label: it.Label, TrackID: it.Key, Detail: err.Error()})
		return outcomeError
	}

	emit(downloader.Event{Kind: downloader.KindDownloading, Label: track.Label(), TrackID: it.Key})
	progress := func(p downloader.Progress) {
		p.TrackID = it.Key
		onProgress(p)
	}
	res, err := u.Upgrade(u.Client.JobContext(), track, it.Path, progress)
	if err != nil {
		if errors.Is(err, downloader.ErrStopped) || u.Client.Stopped() {
			emit(downloader.Event{Kind: downloader.KindStopped, Label: track.Label(), TrackID: it.Key})
			return outcomeStopped
		}
		if errors.Is(err, downloader.ErrAlreadyExists) {
			// A race or a pre-existing FLAC; treat it as an archive-only
			// action when the file really is the same track.
			if id := tagread.TrackIDOf(res.Path); id == track.ID {
				target, mergeErr := u.moveToOld(it.Path)
				if mergeErr != nil {
					u.logf("[warn] не удалось переместить %s в _old: %v", it.Label, mergeErr)
				} else {
					u.logf("[skip] %s: FLAC уже есть, MP3 → _old: %s", it.Label, target)
				}
				u.removeIndex(it.Path)
				u.addIndex(res.Path)
				emit(downloader.Event{
					Kind: downloader.KindSkipped, Label: track.Label(), TrackID: it.Key,
					Path: res.Path,
				})
				return outcomeSkip
			}
		}
		u.recordUpgradeError(it, now, err)
		emit(downloader.Event{
			Kind: downloader.KindFailed, Label: track.Label(), TrackID: it.Key,
			Detail: HumanError(err),
		})
		return outcomeError
	}

	if res.Fallback {
		u.recordNoLossless(it, now)
		emit(downloader.Event{
			Kind: downloader.KindFailed, Label: track.Label(), TrackID: it.Key,
			Detail: "lossless снова недоступен",
		})
		return outcomeError
	}
	if res.Path != it.Path {
		if target, moveErr := u.moveToOld(it.Path); moveErr != nil {
			u.logf("[warn] не удалось переместить %s в _old: %v", it.Label, moveErr)
		} else {
			u.logf("[upgrade] %s: MP3 → %s; старый файл: %s", track.Label(), res.Format, target)
		}
	}
	u.removeIndex(it.Path)
	u.addIndex(res.Path)
	u.markUpgraded(it, res, now)
	emit(downloader.Event{
		Kind: downloader.KindDone, Label: track.Label(), TrackID: it.Key,
		Format: res.Format, Converted: res.Converted, Path: res.Path,
	})
	return outcomeOK
}

func (u *Upgrader) markUpgraded(it FoundItem, res downloader.Result, now time.Time) {
	if u.Journal == nil {
		return
	}
	entry := Entry{
		Status: StatusLossless, Codec: codecFromFormat(res.Format), CheckedAt: now,
		MatchSource: it.MatchSource, MatchedTrackID: it.TrackID,
		MatchedArtist: it.MatchedArtist, MatchedTitle: it.MatchedTitle,
		MatchedAlbum: it.Album, MatchedYear: it.Year,
		Confidence: it.Confidence, Exact: it.Selected,
	}
	for _, key := range journalKeys(it) {
		u.Journal.Put(key, entry)
	}
	u.Journal.Save()
}

func (u *Upgrader) recordNoLossless(it FoundItem, now time.Time) {
	if u.Journal == nil {
		return
	}
	entry := Entry{
		Status: StatusNoLossless, CheckedAt: now, NextCheck: now.Add(NoLosslessTTL),
		MatchSource: it.MatchSource, MatchedTrackID: it.TrackID,
	}
	for _, key := range journalKeys(it) {
		u.Journal.Put(key, entry)
	}
	u.Journal.Save()
}

func (u *Upgrader) recordUpgradeError(it FoundItem, now time.Time, err error) {
	if u.Journal == nil {
		return
	}
	e := Entry{Status: StatusError, CheckedAt: now, NextCheck: now.Add(ErrorTTL)}
	if errors.Is(err, downloader.ErrUnavailable) {
		e.Status = StatusUnavailable
		e.NextCheck = now.Add(UnavailableTTL)
	}
	if errors.Is(err, downloader.ErrNoFLAC) {
		e.Status = StatusNoLossless
		e.NextCheck = now.Add(NoLosslessTTL)
	}
	for _, key := range journalKeys(it) {
		u.Journal.Put(key, e)
	}
	u.Journal.Save()
}

func journalKeys(it FoundItem) []string {
	var keys []string
	if it.TrackID != "" {
		keys = append(keys, it.TrackID)
	}
	if it.MatchSource == "search" || len(keys) == 0 {
		keys = append(keys, "file:"+it.Path)
	}
	return keys
}

func (u *Upgrader) moveToOld(oldPath string) (string, error) {
	root := u.rootFor(oldPath)
	rel, err := filepath.Rel(root, oldPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		root = filepath.Dir(oldPath)
		rel = filepath.Base(oldPath)
	}
	target := filepath.Join(root, "_old", rel)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(target); err == nil {
		target = uniqueOldPath(target, u.now())
	}
	if err := os.Rename(oldPath, target); err != nil {
		return "", err
	}
	return target, nil
}

func (u *Upgrader) rootFor(path string) string {
	for _, root := range []string{u.LibraryDir, u.OutputDir} {
		if strings.TrimSpace(root) == "" {
			continue
		}
		rel, err := filepath.Rel(root, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return root
		}
	}
	return filepath.Dir(path)
}

func uniqueOldPath(target string, now time.Time) string {
	ext := filepath.Ext(target)
	stem := strings.TrimSuffix(target, ext)
	candidate := fmt.Sprintf("%s.%s%s", stem, now.Format("20060102-150405"), ext)
	if _, err := os.Stat(candidate); err != nil {
		return candidate
	}
	for i := 1; ; i++ {
		candidate = fmt.Sprintf("%s.%s-%d%s", stem, now.Format("20060102-150405"), i, ext)
		if _, err := os.Stat(candidate); err != nil {
			return candidate
		}
	}
}

func (u *Upgrader) removeIndex(path string) {
	if u.Index != nil {
		u.Index.RemoveFile(path)
	}
}

func (u *Upgrader) addIndex(path string) {
	if u.Index != nil {
		u.Index.AddFile(path)
	}
}

func (u *Upgrader) now() time.Time {
	if u.Now != nil {
		return u.Now()
	}
	return time.Now()
}

func (u *Upgrader) logf(format string, args ...any) {
	if u.Log == nil {
		return
	}
	u.logMu.Lock()
	u.Log(fmt.Sprintf(format, args...))
	u.logMu.Unlock()
}

func codecFromFormat(format string) string {
	switch strings.ToUpper(strings.TrimSpace(format)) {
	case "FLAC":
		return "flac"
	case "M4A":
		return "flac-mp4"
	case "MP3":
		return "mp3"
	default:
		return ""
	}
}
