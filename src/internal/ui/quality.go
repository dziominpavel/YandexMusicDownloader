package ui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"yamdl/internal/downloader"
	"yamdl/internal/quality"
)

// QualityStatusView is the compact line shown in the main window and the
// status section of the settings window.
type QualityStatusView struct {
	Text     string `json:"text"`
	Checking bool   `json:"checking"`
	Pending  int    `json:"pending"`
	Found    int    `json:"found"`
	Error    string `json:"error,omitempty"`
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
		Checking: checking,
		Pending:  pending,
		Found:    st.Found,
		Error:    st.Error,
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
	if a.index == nil || a.client == nil || a.qualityJournal == nil {
		return nil
	}
	return &quality.Checker{
		Index: a.index, Prober: a.client, Searcher: a.client, Journal: a.qualityJournal,
		Log:        func(s string) { a.log(s, "info") },
		Throttle:   200 * time.Millisecond,
		ConvertM4A: a.cfg.ConvertM4A,
		FFmpeg:     downloader.HasFFmpeg(),
	}
}

// CheckQuality runs the manual check. It is read-only and does not turn
// the main button into «Остановить».
func (a *App) CheckQuality() (quality.Result, error) {
	if a.client == nil {
		return quality.Result{}, fmt.Errorf("нет токена: проверка FLAC недоступна")
	}
	if a.isBusy() {
		return quality.Result{}, fmt.Errorf("идёт скачивание — проверка недоступна")
	}
	if a.isChecking() {
		return quality.Result{}, fmt.Errorf("проверка уже идёт")
	}
	return a.runQualityCheck()
}

func (a *App) runQualityCheck() (quality.Result, error) {
	if a.client == nil {
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
	defer func() {
		cancel()
		a.qualityMu.Lock()
		a.checking = false
		a.checkCancel = nil
		a.qualityMu.Unlock()
	}()

	chk.Progress = func(done, total int) { a.emit("quality-check-progress", done, total) }
	a.emit("quality-check-started")
	res, err := chk.Check(ctx)
	a.storeFound(res.Found)

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
	if !a.cfg.FlacMonitor || a.client == nil || a.index == nil || a.qualityJournal == nil {
		return
	}
	if a.isBusy() || a.isChecking() {
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

// DownloadFound runs the selected upgrades as a normal stoppable job.
func (a *App) DownloadFound(items []quality.FoundItem) error {
	if len(items) == 0 {
		return fmt.Errorf("ничего не выбрано")
	}
	if a.isBusy() {
		return fmt.Errorf("идёт скачивание — обновление недоступно")
	}
	if a.client == nil {
		return fmt.Errorf("нет токена: обновление недоступно")
	}
	a.cancelQualityCheck()
	a.setBusy(true)
	defer a.setBusy(false)

	a.client.StartJob()
	a.jobStart("Обновление FLAC", len(items))
	tracks := make([]*downloader.Track, 0, len(items))
	for _, it := range items {
		tracks = append(tracks, &downloader.Track{ID: it.Key, Title: it.Label})
	}
	a.trackRows(tracks)

	up := &quality.Upgrader{
		Client: a.client, Upgrade: a.client.Upgrade, Index: a.index,
		Journal: a.qualityJournal, LibraryDir: a.cfg.LibraryDir, OutputDir: a.outputDir,
		Workers: 2, Log: func(s string) { a.log(s, "info") },
	}
	sum := up.Run(items, func(e downloader.Event) {
		text, kind := formatEvent(e)
		a.log(text, kind)
		status, skind := formatTrackStatus(e)
		a.trackStatus(e.TrackID, status, skind)
	}, func(p downloader.Progress) {
		a.progress(p)
	})

	stopped := sum.Stopped > 0 || a.stopRequested()
	a.jobEnd(sum.OK, sum.Skipped, sum.Failed, stopped)
	if stopped {
		a.log(fmt.Sprintf("[stopped] обновление: %d ok, %d skip, %d error", sum.OK, sum.Skipped, sum.Failed), "stopped")
	} else {
		a.log(fmt.Sprintf("[finished] %d ok, %d skip, %d error", sum.OK, sum.Skipped, sum.Failed), "info")
	}

	// Unchecked search matches are explicit rejections; unchecked ID
	// matches stay pending for the next time.
	selected := map[string]bool{}
	for _, it := range items {
		selected[it.Path] = true
	}
	a.qualityMu.Lock()
	for _, it := range a.found {
		if !selected[it.Path] && it.MatchSource == "search" && it.TrackID != "" && a.qualityJournal != nil {
			a.qualityJournal.Decline(it.Path, it.TrackID, time.Now())
		}
	}
	a.qualityMu.Unlock()
	if a.qualityJournal != nil {
		a.qualityJournal.Save()
	}
	a.refreshPendingQuality()
	remaining := a.PendingFound()
	a.emit("quality-pending", remaining)
	status := a.qualityStatusView()
	a.emit("quality-check-finished", status.Text, len(remaining), "", len(remaining))
	return nil
}
