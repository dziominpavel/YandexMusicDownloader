package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"yamdl/internal/downloader"
	"yamdl/internal/quality"
	"yamdl/internal/queue"
)

// QualityStatusView is the status line shown in the settings window.
type QualityStatusView struct {
	Text      string `json:"text"`
	Checking  bool   `json:"checking"`
	Pending   int    `json:"pending"`
	Found     int    `json:"found"`
	OutputDir string `json:"outputDir"`
	Error     string `json:"error,omitempty"`
}

// QualityStatus returns the current monitor state. The frontend calls it
// once on load; later updates arrive as quality-check-finished events.
func (a *App) QualityStatus() QualityStatusView {
	return a.qualityStatusView()
}

func (a *App) qualityStatusView() QualityStatusView {
	a.qualityMu.Lock()
	checking := a.checking
	pending := len(a.found)
	a.qualityMu.Unlock()

	st := quality.Status{}
	if a.qualityJournal != nil {
		st = a.qualityJournal.Status()
	}
	view := QualityStatusView{
		Checking:  checking,
		Pending:   pending,
		Found:     st.Found,
		OutputDir: a.outputDir,
		Error:     st.Error,
	}
	last := st.LastAttempt
	if last.IsZero() {
		last = st.LastCompleted
	}
	when := last.Format("02.01.2006 15:04")
	switch {
	case checking:
		view.Text = "FLAC: проверка идёт…"
	case last.IsZero():
		view.Text = "FLAC: проверок не было"
	case st.Error != "":
		view.Text = fmt.Sprintf("FLAC: %s — ошибка", when)
	case st.Found > 0:
		view.Text = fmt.Sprintf("FLAC: %s — найдено %d", when, st.Found)
	default:
		view.Text = fmt.Sprintf("FLAC: %s — ничего не найдено", when)
	}
	return view
}

func (a *App) qualityOpts() quality.FormatOptions {
	return quality.FormatOptions{
		ConvertM4A: a.cfg.ConvertM4A,
		FFmpeg:     downloader.HasFFmpeg(),
	}
}

func (a *App) qualityChecker() *quality.Checker {
	c := a.clientSnapshot()
	if a.index == nil || c == nil || a.qualityJournal == nil {
		return nil
	}
	return &quality.Checker{
		Index: a.index, Prober: c, Searcher: c, Journal: a.qualityJournal,
		Log:        func(s string) { a.log(s, "info") },
		Throttle:   200 * time.Millisecond,
		ConvertM4A: a.cfg.ConvertM4A,
		FFmpeg:     downloader.HasFFmpeg(),
	}
}

// CheckQuality runs the manual check. It is read-only and does not touch
// the download queue: checks and downloads may run at the same time.
func (a *App) CheckQuality() (quality.Result, error) {
	if a.clientSnapshot() == nil {
		return quality.Result{}, fmt.Errorf("нет токена: проверка FLAC недоступна")
	}
	if a.isChecking() {
		return quality.Result{}, fmt.Errorf("проверка уже идёт")
	}
	return a.runQualityCheck()
}

func (a *App) runQualityCheck() (quality.Result, error) {
	if a.clientSnapshot() == nil {
		return quality.Result{}, fmt.Errorf("нет токена: проверка FLAC недоступна")
	}
	chk := a.qualityChecker()
	if chk == nil {
		return quality.Result{}, fmt.Errorf("индекс коллекции не готов — проверка недоступна")
	}

	a.qualityMu.Lock()
	if a.checking {
		a.qualityMu.Unlock()
		return quality.Result{}, fmt.Errorf("проверка уже идёт")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.checking = true
	a.checkCancel = cancel
	a.qualityMu.Unlock()
	finish := func() {
		cancel()
		a.qualityMu.Lock()
		a.checking = false
		a.checkCancel = nil
		a.qualityMu.Unlock()
	}
	defer finish()

	chk.Progress = func(done, total int) { a.emit("quality-check-progress", done, total) }
	a.emit("quality-check-started")
	res, err := chk.Check(ctx)
	a.storeFound(res.Found)

	// Clear the running flag before rendering the result: otherwise the
	// finished event carries "проверка идёт…" forever.
	finish()
	status := a.qualityStatusView()
	if errors.Is(err, context.Canceled) {
		a.log("[flac] проверка отменена", "info")
		a.emit("quality-check-finished", "FLAC: проверка отменена", len(res.Found), "", len(res.Found))
		a.emit("quality-pending", res.Found)
		return res, nil
	}
	if err != nil {
		errText := quality.HumanError(err)
		a.emit("quality-check-finished", status.Text, len(res.Found), errText, len(res.Found))
		a.emit("quality-pending", res.Found)
		return res, err
	}
	if len(res.Found) > 0 {
		a.emit("quality-found", res.Found)
	} else {
		a.emit("quality-pending", res.Found)
	}
	a.emit("quality-check-finished", status.Text, len(res.Found), "", len(res.Found))
	return res, nil
}

func (a *App) maybeAutoCheck() {
	if !a.cfg.FlacMonitor || a.clientSnapshot() == nil || a.index == nil || a.qualityJournal == nil {
		return
	}
	if a.isChecking() {
		return
	}
	if !a.qualityJournal.DueCheck(time.Now()) {
		return
	}
	go func() { _, _ = a.runQualityCheck() }()
}

func (a *App) autoCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.maybeAutoCheck()
		}
	}
}

func (a *App) isChecking() bool {
	a.qualityMu.Lock()
	defer a.qualityMu.Unlock()
	return a.checking
}

func (a *App) cancelQualityCheck() {
	a.qualityMu.Lock()
	cancel := a.checkCancel
	a.qualityMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *App) storeFound(items []quality.FoundItem) {
	a.qualityMu.Lock()
	a.found = items
	a.qualityMu.Unlock()
}

// refreshPendingQuality rebuilds the offer list from the index and the
// journal. It is called after the startup index scan, after a check and
// after an upgrade.
func (a *App) refreshPendingQuality() {
	var found []quality.FoundItem
	if a.index != nil && a.qualityJournal != nil {
		found = quality.Pending(a.index.Snapshot(), a.qualityJournal, time.Now(), a.qualityOpts())
	}
	a.storeFound(found)
}

// PendingFound returns the current offer list for the badge/modal.
func (a *App) PendingFound() []quality.FoundItem {
	a.qualityMu.Lock()
	defer a.qualityMu.Unlock()
	out := make([]quality.FoundItem, len(a.found))
	copy(out, a.found)
	return out
}

// RejectFound explicitly rejects a search match. The local MP3 is not
// touched: its lossless check entry is removed so the next check can look
// for another version.
func (a *App) RejectFound(item quality.FoundItem) ([]quality.FoundItem, error) {
	if a.qualityJournal == nil {
		return nil, fmt.Errorf("журнал проверок недоступен")
	}
	if item.MatchSource != "search" {
		return nil, fmt.Errorf("ID-совпадение нельзя отклонить")
	}
	if strings.TrimSpace(item.Path) == "" || strings.TrimSpace(item.TrackID) == "" {
		return nil, fmt.Errorf("не удалось определить совпадение")
	}

	// Keep qualityMu across validation and the journal update: starting a
	// check would otherwise race with the rejection.
	a.qualityMu.Lock()
	if a.checking {
		a.qualityMu.Unlock()
		return nil, fmt.Errorf("идёт проверка — дождитесь завершения")
	}
	current := false
	for _, it := range a.found {
		if it.Path == item.Path && it.TrackID == item.TrackID && it.MatchSource == "search" {
			current = true
			break
		}
	}
	if !current {
		a.qualityMu.Unlock()
		return nil, fmt.Errorf("совпадение уже изменилось — обновите список")
	}
	d, ok := a.declineFound(item, time.Now())
	if ok {
		a.qualityJournal.Save()
	}
	a.qualityMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("не удалось сохранить отклонение")
	}
	a.log(fmt.Sprintf("[reject] %s: %s — больше не предлагать", d.LocalLabel, declinedCandidate(d)), "info")

	a.refreshPendingQuality()
	remaining := a.PendingFound()
	a.emit("quality-pending", remaining)
	return remaining, nil
}

// UndoReject removes one rejection. When the stored metadata is complete,
// the original lossless offer is restored immediately without a network
// call; otherwise the next check will find it again.
func (a *App) UndoReject(path, trackID string) ([]quality.FoundItem, error) {
	if a.qualityJournal == nil {
		return nil, fmt.Errorf("журнал проверок недоступен")
	}
	if strings.TrimSpace(path) == "" || strings.TrimSpace(trackID) == "" {
		return nil, fmt.Errorf("не удалось определить совпадение")
	}

	a.qualityMu.Lock()
	if a.checking {
		a.qualityMu.Unlock()
		return nil, fmt.Errorf("идёт проверка — дождитесь завершения")
	}
	d, ok := a.qualityJournal.Undecline(path, trackID)
	if ok {
		if d.MatchedArtist != "" || d.MatchedTitle != "" || d.Codec != "" {
			a.qualityJournal.Put("file:"+path, quality.Entry{
				Status: quality.StatusLossless, Codec: d.Codec, CheckedAt: d.DeclinedAt,
				MatchSource: "search", MatchedTrackID: d.TrackID,
				MatchedArtist: d.MatchedArtist, MatchedTitle: d.MatchedTitle,
				MatchedAlbum: d.MatchedAlbum, MatchedYear: d.MatchedYear,
				Confidence: d.Confidence, Exact: d.Exact,
			})
		}
		a.qualityJournal.Save()
	}
	a.qualityMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("отклонение не найдено")
	}
	a.log(fmt.Sprintf("[reject] %s: %s — возвращено", d.LocalLabel, declinedCandidate(d)), "info")

	a.refreshPendingQuality()
	remaining := a.PendingFound()
	a.emit("quality-pending", remaining)
	return remaining, nil
}

// declineFound stores one rejection and clears the file's lossless entry.
// The caller saves the journal.
func (a *App) declineFound(it quality.FoundItem, now time.Time) (quality.Declined, bool) {
	if a.qualityJournal == nil || it.MatchSource != "search" || strings.TrimSpace(it.TrackID) == "" {
		return quality.Declined{}, false
	}
	local := strings.TrimSpace(it.Label)
	if local == "" {
		local = strings.TrimSpace(it.FileName)
	}
	if local == "" {
		local = it.Path
	}
	d := quality.Declined{
		TrackID: it.TrackID, LocalLabel: local,
		MatchedArtist: it.MatchedArtist, MatchedTitle: it.MatchedTitle,
		MatchedAlbum: it.Album, MatchedYear: it.Year,
		Confidence: it.Confidence, Exact: it.Selected,
		Codec: it.Codec, DeclinedAt: now,
	}
	if je, ok := a.qualityJournal.Get("file:" + it.Path); ok {
		d.Exact = je.Exact
		if d.Codec == "" {
			d.Codec = je.Codec
		}
	}
	a.qualityJournal.Decline(it.Path, d)
	a.qualityJournal.Forget("file:" + it.Path)
	return d, true
}

func declinedCandidate(d quality.Declined) string {
	label := strings.TrimSpace(strings.TrimSpace(d.MatchedArtist) + " — " + strings.TrimSpace(d.MatchedTitle))
	if label == "" || label == "—" {
		return d.TrackID
	}
	return label
}

// DownloadFound enqueues the selected upgrades as tasks of the
// «Обновление FLAC» source. The queue owns the pool, cancellation and the
// rows; the upgrader only owns one item at a time.
func (a *App) DownloadFound(items []quality.FoundItem) error {
	if len(items) == 0 {
		return fmt.Errorf("ничего не выбрано")
	}
	if a.clientSnapshot() == nil {
		return fmt.Errorf("нет токена: обновление недоступно")
	}
	a.cancelQualityCheck()

	entries := make([]queue.Entry, 0, len(items))
	for _, it := range items {
		label := strings.TrimSpace(it.Label)
		if label == "" {
			label = strings.TrimSpace(it.FileName)
		}
		entries = append(entries, queue.Entry{TrackID: it.Key, Label: label, Item: it})
	}
	a.ensureQueue().SubmitTasks("Обновление FLAC", fmt.Sprintf("[flac] обновление: %d треков", len(items)), "upgrade", entries)

	// Unchecked search matches are explicit rejections; unchecked ID
	// matches stay pending for the next time.
	selected := map[string]bool{}
	for _, it := range items {
		selected[it.Path] = true
	}
	var declined []quality.FoundItem
	a.qualityMu.Lock()
	for _, it := range a.found {
		if !selected[it.Path] && it.MatchSource == "search" && it.TrackID != "" {
			declined = append(declined, it)
		}
	}
	a.qualityMu.Unlock()
	if a.qualityJournal != nil {
		now := time.Now()
		for _, it := range declined {
			if d, ok := a.declineFound(it, now); ok {
				a.log(fmt.Sprintf("[reject] %s: %s — больше не предлагать", d.LocalLabel, declinedCandidate(d)), "info")
			}
		}
		a.qualityJournal.Save()
	}
	a.refreshPendingQuality()
	remaining := a.PendingFound()
	a.emit("quality-pending", remaining)
	status := a.qualityStatusView()
	a.emit("quality-check-finished", status.Text, len(remaining), "", len(remaining))
	return nil
}
