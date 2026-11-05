// Package queue is the flat download queue: every track is a task, all
// tasks share one worker pool, and each task can be cancelled, retried or
// held for a duplicate decision without blocking the rest.
//
// The manager owns the state; the UI only renders snapshots and sends
// commands. Sink callbacks always run outside the manager lock, so the UI
// may call back into the manager from an event handler.
package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"yamdl/internal/downloader"
)

// State is the observable state of one task.
type State string

const (
	// StateResolving means the link was accepted and its tracks are being resolved.
	StateResolving State = "resolving"
	// StateQueued means the task waits for a free worker slot.
	StateQueued State = "queued"
	// StateDownloading means a worker is downloading the track right now.
	StateDownloading State = "downloading"
	// StateDone means the file was published.
	StateDone State = "done"
	// StateSkipped means the download was intentionally not performed.
	StateSkipped State = "skipped"
	// StateNeedsReview means a probable duplicate waits for the user's answer.
	StateNeedsReview State = "needs-review"
	// StateError means the download failed.
	StateError State = "error"
	// StateCanceled means the user cancelled the task.
	StateCanceled State = "canceled"
)

// Terminal reports whether the task cannot change without a retry.
func (s State) Terminal() bool {
	switch s {
	case StateDone, StateSkipped, StateError, StateCanceled:
		return true
	default:
		return false
	}
}

// Source is the link a group of tasks came from.
type Source struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Link  string `json:"link"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Order int    `json:"order"`
}

// Candidate describes a probable duplicate found in the collection.
type Candidate struct {
	Percent         int    `json:"percent"`
	FileName        string `json:"fileName"`
	FilePath        string `json:"filePath"`
	FileExt         string `json:"fileExt"`
	DifferentFormat bool   `json:"differentFormat"`
}

// Task is one queue row: a track to download, or a link still resolving.
type Task struct {
	Key            string     `json:"key"`
	TrackID        string     `json:"trackId"`
	Label          string     `json:"label"`
	Kind           string     `json:"kind"`
	SourceKey      string     `json:"sourceKey"`
	SourceLabel    string     `json:"sourceLabel"`
	State          State      `json:"state"`
	Percent        int        `json:"percent"`
	Done           int64      `json:"done"`
	Total          int64      `json:"total"`
	Format         string     `json:"format"`
	FallbackReason string     `json:"fallbackReason"`
	Reason         string     `json:"reason"`
	Path           string     `json:"path"`
	Candidate      *Candidate `json:"candidate,omitempty"`
	// Position is the 1-based place among waiting tasks (0 when not queued).
	Position int `json:"position"`
}

// Snapshot is the aggregate state the window renders.
type Snapshot struct {
	Tasks      []Task   `json:"tasks"`
	Sources    []Source `json:"sources"`
	Total      int      `json:"total"`
	Done       int      `json:"done"`
	Skipped    int      `json:"skipped"`
	Failed     int      `json:"failed"`
	Canceled   int      `json:"canceled"`
	Active     int      `json:"active"`
	Waiting    int      `json:"waiting"`
	Review     int      `json:"review"`
	Percent    int      `json:"percent"`
	BytesDone  int64    `json:"bytesDone"`
	BytesTotal int64    `json:"bytesTotal"`
	Workers    int      `json:"workers"`
}

// Result is the outcome of one download step.
type Result struct {
	Format         string
	Fallback       bool
	FallbackReason string
	Converted      bool
	Path           string
}

// Resolved is a link expanded into tracks.
type Resolved struct {
	// Label is the human source name for the queue chips, e.g. "Альбом · RAM".
	Label string
	// Kind is "track", "album" or "playlist" (m3u export decision).
	Kind string
	// Title is the raw album/playlist title (m3u file name).
	Title string
	// Log is the journal line written once per source, e.g. "[album] RAM (13 треков)".
	Log string
	// Tracks are the tasks in source order.
	Tracks []*downloader.Track
}

// ResolveFunc turns a pasted link into tracks. It must be safe to call
// concurrently (the manager resolves at most two links at a time).
type ResolveFunc func(ctx context.Context, link string) (Resolved, error)

// DownloadFunc downloads one track (or upgrades one item) for a task.
// item is nil for regular tracks and carries the source payload for
// quality-upgrade tasks. onProgress reports received bytes.
type DownloadFunc func(ctx context.Context, track *downloader.Track, item any, onProgress func(done, total int64)) (Result, error)

// DuplicateFunc looks for a probable duplicate in the collection; nil
// disables the check.
type DuplicateFunc func(track *downloader.Track) (Candidate, bool)

// Sink receives state changes. Implementations must not call back into the
// manager synchronously except through the public commands.
type Sink interface {
	TaskAdded(Task)
	TaskUpdated(Task)
	TaskRemoved(key string)
	Sources([]Source)
	Queue(Snapshot)
	Log(text, kind string)
	// Existing is called when a submitted track is already in the queue;
	// the UI highlights the existing row and shows a short notice.
	Existing(trackID string)
}

// Manager is the queue. Create with New; all methods are safe for
// concurrent use.
type Manager struct {
	sink      Sink
	resolve   ResolveFunc
	download  DownloadFunc
	duplicate DuplicateFunc
	workers   func() int
	now       func() time.Time

	mu        sync.Mutex
	seq       int
	sourceSeq int
	tasks     []*taskState
	byKey     map[string]*taskState
	sources   []*Source
	active    int
	stopped   bool
	runCtx    context.Context
	runCancel context.CancelFunc
	lastQueue time.Time

	resolveSem chan struct{}
}

type taskState struct {
	Task
	track    *downloader.Track
	item     any
	cancel   context.CancelFunc
	lastEmit time.Time
}

// New builds a queue. workers is read at every dispatch, so a settings
// change applies to the next free slot without a restart.
func New(sink Sink, resolve ResolveFunc, download DownloadFunc, duplicate DuplicateFunc, workers func() int) *Manager {
	runCtx, runCancel := context.WithCancel(context.Background())
	return &Manager{
		sink:       sink,
		resolve:    resolve,
		download:   download,
		duplicate:  duplicate,
		workers:    workers,
		now:        time.Now,
		byKey:      make(map[string]*taskState),
		runCtx:     runCtx,
		runCancel:  runCancel,
		resolveSem: make(chan struct{}, 2),
	}
}

// workersCount normalizes the configured pool size.
func (m *Manager) workersCount() int {
	n := 3
	if m.workers != nil {
		n = m.workers()
	}
	if n < 1 {
		n = 1
	}
	if n > 10 {
		n = 10
	}
	return n
}

func (m *Manager) nextKeyLocked() string {
	m.seq++
	return fmt.Sprintf("t%d", m.seq)
}

// shortLink makes a readable source label before the link is resolved.
func shortLink(link string) string {
	s := strings.TrimSpace(link)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "music.yandex.")
	if s == "" {
		return "ссылка"
	}
	runes := []rune(s)
	if len(runes) > 42 {
		return string(runes[:39]) + "..."
	}
	return s
}

// Submit accepts pasted links and puts them into the queue without waiting
// for the current downloads. Every link becomes its own source.
func (m *Manager) Submit(links []string) []Source {
	clean := cleanLinks(links)
	if len(clean) == 0 {
		return nil
	}
	m.mu.Lock()
	if m.stopped {
		// A cancelled queue re-arms on the next submission: a stop must
		// not poison later downloads.
		m.runCancel()
		m.runCtx, m.runCancel = context.WithCancel(context.Background())
		m.stopped = false
	}
	var placeholders []*taskState
	for _, link := range clean {
		m.sourceSeq++
		src := &Source{Key: fmt.Sprintf("s%d", m.sourceSeq), Label: shortLink(link), Link: link, Order: m.sourceSeq}
		m.sources = append(m.sources, src)
		t := &taskState{
			Task: Task{
				Key:         m.nextKeyLocked(),
				Label:       "Определяется…",
				Kind:        "download",
				SourceKey:   src.Key,
				SourceLabel: src.Label,
				State:       StateResolving,
			},
		}
		m.tasks = append(m.tasks, t)
		m.byKey[t.Key] = t
		placeholders = append(placeholders, t)
	}
	taskCopies := make([]Task, 0, len(placeholders))
	for _, t := range placeholders {
		taskCopies = append(taskCopies, t.Task)
	}
	sources := m.sourcesCopyLocked()
	m.mu.Unlock()

	for _, task := range taskCopies {
		m.sink.TaskAdded(task)
	}
	m.sink.Sources(sources)
	m.pushQueue(true)
	for i, t := range placeholders {
		go m.resolveSource(t, clean[i])
	}
	return sources
}

// cleanLinks trims and drops empty links, keeping the input order.
func cleanLinks(links []string) []string {
	out := make([]string, 0, len(links))
	for _, l := range links {
		if s := strings.TrimSpace(l); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Entry is a prebuilt task for SubmitTasks: the quality upgrader already
// knows the track and carries the FoundItem as an opaque payload.
type Entry struct {
	TrackID string
	Label   string
	Track   *downloader.Track
	Item    any
}

// SubmitTasks enqueues already-resolved tasks (FLAC upgrades) as one
// source. Unlike link submissions it skips the collection duplicate check:
// for an upgrade the local file is the expected duplicate.
func (m *Manager) SubmitTasks(sourceLabel, logLine, kind string, entries []Entry) Source {
	if sourceLabel == "" {
		sourceLabel = "Задачи"
	}
	m.mu.Lock()
	if m.stopped {
		m.runCancel()
		m.runCtx, m.runCancel = context.WithCancel(context.Background())
		m.stopped = false
	}
	m.sourceSeq++
	src := &Source{Key: fmt.Sprintf("s%d", m.sourceSeq), Label: sourceLabel, Kind: kind, Order: m.sourceSeq}
	m.sources = append(m.sources, src)
	var copies []Task
	for _, e := range entries {
		track := e.Track
		if track == nil {
			track = &downloader.Track{ID: e.TrackID, Title: e.Label}
		}
		label := strings.TrimSpace(e.Label)
		if label == "" {
			label = track.Label()
		}
		t := &taskState{
			Task: Task{
				Key:         m.nextKeyLocked(),
				TrackID:     e.TrackID,
				Label:       label,
				Kind:        kind,
				SourceKey:   src.Key,
				SourceLabel: sourceLabel,
			},
			track: track,
			item:  e.Item,
		}
		if t.TrackID != "" {
			if existing := m.liveTaskLocked(t.TrackID, nil); existing != nil {
				t.State = StateSkipped
				t.Percent = 100
				t.Reason = "уже в очереди"
			}
		}
		if t.State == "" {
			t.State = StateQueued
		}
		m.tasks = append(m.tasks, t)
		m.byKey[t.Key] = t
		copies = append(copies, t.Task)
	}
	copiedSource := *src
	sources := m.sourcesCopyLocked()
	m.mu.Unlock()

	for _, task := range copies {
		m.sink.TaskAdded(task)
	}
	m.sink.Sources(sources)
	if logLine != "" {
		m.sink.Log(logLine, "info")
	}
	m.pushQueue(true)
	m.pump()
	return copiedSource
}

// resolveSource expands one link and replaces its placeholder task with
// track tasks, keeping the source order.
func (m *Manager) resolveSource(placeholder *taskState, link string) {
	m.resolveSem <- struct{}{}
	defer func() { <-m.resolveSem }()

	m.mu.Lock()
	runCtx := m.runCtx
	key := placeholder.Key
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(runCtx, 60*time.Second)
	defer cancel()
	res, err := m.resolve(ctx, link)

	m.mu.Lock()
	t, ok := m.byKey[key]
	if !ok || t.State != StateResolving {
		// Cancelled (or removed) while resolving: drop the result.
		m.mu.Unlock()
		return
	}
	if err != nil {
		t.State = StateError
		t.Reason = resolveReason(err)
		copied := t.Task
		m.mu.Unlock()
		m.sink.TaskUpdated(copied)
		m.sink.Log("[error] "+copied.SourceLabel+": "+copied.Reason, "err")
		m.pushQueue(true)
		return
	}
	label := strings.TrimSpace(res.Label)
	if label == "" {
		label = t.SourceLabel
	}
	// Update the source label before the rows appear.
	for _, s := range m.sources {
		if s.Key == t.SourceKey {
			s.Label = label
			if res.Kind != "" {
				s.Kind = res.Kind
			}
			if res.Title != "" {
				s.Title = res.Title
			}
		}
	}
	if len(res.Tracks) == 0 {
		t.State = StateError
		t.Reason = "не найдено треков"
		copied := t.Task
		sources := m.sourcesCopyLocked()
		m.mu.Unlock()
		m.sink.TaskUpdated(copied)
		m.sink.Sources(sources)
		m.sink.Log("[error] "+label+": не найдено треков", "err")
		m.pushQueue(true)
		return
	}
	pending := m.expandLocked(t, label, res.Tracks)
	sources := m.sourcesCopyLocked()
	placeholderKey := t.Key
	m.mu.Unlock()

	m.sink.TaskRemoved(placeholderKey)
	m.sink.Sources(sources)
	if res.Log != "" {
		m.sink.Log(res.Log, "info")
	}
	for _, p := range pending {
		m.sink.TaskAdded(p.Task)
		if p.dupSkip {
			m.sink.Existing(p.Task.TrackID)
		}
		if p.log != nil {
			m.sink.Log(p.log.text, p.log.kind)
		}
	}
	m.pushQueue(true)
	m.pump()
}

// expandLocked replaces the placeholder with track tasks. It returns the
// new tasks (with their pending log lines) for the caller to emit.
func (m *Manager) expandLocked(placeholder *taskState, label string, tracks []*downloader.Track) []*pendingTask {
	idx := -1
	for i, t := range m.tasks {
		if t == placeholder {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	delete(m.byKey, placeholder.Key)

	var expanded []*taskState
	var out []*pendingTask
	for _, track := range tracks {
		if track == nil || strings.TrimSpace(track.ID) == "" {
			continue
		}
		t := &taskState{
			Task: Task{
				Key:         m.nextKeyLocked(),
				TrackID:     track.ID,
				Label:       track.Label(),
				Kind:        "download",
				SourceKey:   placeholder.SourceKey,
				SourceLabel: label,
			},
			track: track,
		}
		m.byKey[t.Key] = t

		if !track.Available {
			t.State = StateSkipped
			t.Percent = 100
			t.Reason = "недоступно"
			expanded = append(expanded, t)
			out = append(out, &pendingTask{Task: t.Task, log: &logLine{text: "[skip] " + t.Label + " (unavailable)", kind: "info"}})
			continue
		}
		// A track already waiting or downloading is not queued twice.
		if m.liveTaskLocked(track.ID, expanded) != nil {
			t.State = StateSkipped
			t.Percent = 100
			t.Reason = "уже в очереди"
			expanded = append(expanded, t)
			out = append(out, &pendingTask{Task: t.Task, dupSkip: true, log: &logLine{text: "[skip] " + t.Label + " (уже в очереди)", kind: "info"}})
			continue
		}
		if m.duplicate != nil {
			if cand, ok := m.duplicate(track); ok {
				c := cand
				t.State = StateNeedsReview
				t.Candidate = &c
				expanded = append(expanded, t)
				out = append(out, &pendingTask{Task: t.Task})
				continue
			}
		}
		t.State = StateQueued
		expanded = append(expanded, t)
		out = append(out, &pendingTask{Task: t.Task})
	}

	// Put the expanded tasks where the placeholder was, keeping order.
	rest := make([]*taskState, 0, len(m.tasks)-1)
	rest = append(rest, m.tasks[:idx]...)
	rest = append(rest, m.tasks[idx+1:]...)
	merged := make([]*taskState, 0, len(rest)+len(expanded))
	merged = append(merged, rest[:idx]...)
	merged = append(merged, expanded...)
	merged = append(merged, rest[idx:]...)
	m.tasks = merged
	return out
}

// liveTaskLocked finds a non-terminal task with the same track ID, also
// considering the tasks being added by the current expansion.
func (m *Manager) liveTaskLocked(trackID string, extra []*taskState) *taskState {
	for _, t := range m.tasks {
		if t.TrackID == trackID && !t.State.Terminal() {
			return t
		}
	}
	for _, t := range extra {
		if t.TrackID == trackID && !t.State.Terminal() {
			return t
		}
	}
	return nil
}

func resolveReason(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// pump starts queued tasks while slots are free, in queue order.
func (m *Manager) pump() {
	for {
		m.mu.Lock()
		if m.stopped {
			m.mu.Unlock()
			return
		}
		if m.active >= m.workersCount() {
			m.mu.Unlock()
			return
		}
		var next *taskState
		for _, t := range m.tasks {
			if t.State == StateQueued {
				next = t
				break
			}
		}
		if next == nil {
			m.mu.Unlock()
			return
		}
		ctx, cancel := context.WithCancel(m.runCtx)
		next.State = StateDownloading
		next.Percent = 0
		next.Done = 0
		next.Total = 0
		next.lastEmit = time.Time{}
		next.cancel = cancel
		m.active++
		taskCopy := next.Task
		track := next.track
		item := next.item
		state := next
		m.mu.Unlock()

		m.sink.TaskUpdated(taskCopy)
		m.pushQueue(true)
		m.sink.Log("[downloading] "+taskCopy.Label, "info")
		go m.runTask(state, ctx, track, item)
	}
}

func (m *Manager) runTask(t *taskState, ctx context.Context, track *downloader.Track, item any) {
	if m.download == nil {
		m.finish(t, Result{}, fmt.Errorf("скачивание недоступно"), ctx)
		return
	}
	res, err := m.download(ctx, track, item, func(done, total int64) {
		m.progress(t, done, total)
	})
	m.finish(t, res, err, ctx)
}

// progress updates one task and emits a throttled row update.
func (m *Manager) progress(t *taskState, done, total int64) {
	m.mu.Lock()
	if t.State != StateDownloading {
		m.mu.Unlock()
		return
	}
	t.Done = done
	t.Total = total
	if total > 0 {
		t.Percent = percentOf(done, total)
	} else {
		t.Percent = -1
	}
	now := m.now()
	emit := now.Sub(t.lastEmit) >= 200*time.Millisecond
	if emit {
		t.lastEmit = now
	}
	copied := t.Task
	m.mu.Unlock()
	if emit {
		m.sink.TaskUpdated(copied)
		m.pushQueue(false)
	}
}

// percentOf maps done/total to 0..100.
func percentOf(done, total int64) int {
	if total <= 0 {
		return -1
	}
	if done < 0 {
		done = 0
	}
	p := int(done * 100 / total)
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// finish records the outcome of one completed worker and starts the next task.
func (m *Manager) finish(t *taskState, res Result, err error, ctx context.Context) {
	ctxErr := ctx.Err()
	m.mu.Lock()
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	if m.active > 0 {
		m.active--
	}
	var text, kind string
	switch {
	case err == nil:
		t.State = StateDone
		t.Percent = 100
		if t.Total > 0 {
			t.Done = t.Total
		}
		t.Format = res.Format
		t.FallbackReason = res.FallbackReason
		t.Path = res.Path
		text = "[done] " + t.Label + " (" + res.Format + ")"
		if res.Fallback {
			text += " [fallback MP3"
			if res.FallbackReason != "" {
				text += ": " + res.FallbackReason
			}
			text += "]"
		}
		if res.Converted {
			text += " [ALAC→FLAC]"
		}
		kind = "done"
	case isAlreadyExists(err):
		t.State = StateSkipped
		t.Percent = 100
		t.Path = res.Path
		t.Reason = "уже есть"
		text = "[skip] " + t.Label + " (already exists)"
		kind = "info"
	case ctxErr != nil || isStopped(err):
		t.State = StateCanceled
		text = "[stopped] " + t.Label
		kind = "stopped"
	default:
		t.State = StateError
		t.Reason = err.Error()
		text = "[error] " + t.Label + ": " + err.Error()
		kind = "err"
	}
	copied := t.Task
	m.mu.Unlock()

	m.sink.TaskUpdated(copied)
	m.sink.Log(text, kind)
	m.pushQueue(true)
	m.pump()
}

// CancelTask cancels one task without touching the others.
func (m *Manager) CancelTask(key string) {
	m.mu.Lock()
	t := m.byKey[key]
	if t == nil || t.State.Terminal() {
		m.mu.Unlock()
		return
	}
	t.State = StateCanceled
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	copied := t.Task
	m.mu.Unlock()
	m.sink.TaskUpdated(copied)
	m.sink.Log("[stopped] "+copied.Label, "stopped")
	m.pushQueue(true)
}

// CancelAll cancels every active and waiting task. The queue re-arms on
// the next Submit, so a cancelled queue never poisons later downloads.
func (m *Manager) CancelAll() {
	m.mu.Lock()
	m.stopped = true
	m.runCancel()
	var copies []Task
	for _, t := range m.tasks {
		if t.State.Terminal() {
			continue
		}
		t.State = StateCanceled
		if t.cancel != nil {
			t.cancel()
			t.cancel = nil
		}
		copies = append(copies, t.Task)
	}
	m.mu.Unlock()
	if len(copies) == 0 {
		return
	}
	for _, c := range copies {
		m.sink.TaskUpdated(c)
		m.sink.Log("[stopped] "+c.Label, "stopped")
	}
	m.pushQueue(true)
}

// CancelSource cancels all non-terminal tasks of one source.
func (m *Manager) CancelSource(key string) {
	m.mu.Lock()
	var copies []Task
	for _, t := range m.tasks {
		if t.SourceKey != key || t.State.Terminal() {
			continue
		}
		t.State = StateCanceled
		if t.cancel != nil {
			t.cancel()
			t.cancel = nil
		}
		copies = append(copies, t.Task)
	}
	m.mu.Unlock()
	for _, c := range copies {
		m.sink.TaskUpdated(c)
	}
	if len(copies) > 0 {
		m.sink.Log(fmt.Sprintf("[stopped] источник отменён: %d задач", len(copies)), "stopped")
	}
	m.pushQueue(true)
}

// RetryTask puts an error or cancelled task back into the queue at the end.
func (m *Manager) RetryTask(key string) {
	m.mu.Lock()
	t := m.byKey[key]
	if t == nil || (t.State != StateError && t.State != StateCanceled) {
		m.mu.Unlock()
		return
	}
	t.State = StateQueued
	t.Percent = 0
	t.Done = 0
	t.Total = 0
	t.Format = ""
	t.Reason = ""
	t.Path = ""
	t.FallbackReason = ""
	t.Candidate = nil
	t.cancel = nil
	t.lastEmit = time.Time{}
	// Move to the end so a retry never jumps ahead of waiting tasks.
	m.tasks = removeTask(m.tasks, t)
	m.tasks = append(m.tasks, t)
	copied := t.Task
	m.mu.Unlock()
	m.sink.TaskUpdated(copied)
	m.pushQueue(true)
	m.pump()
}

// RemoveTask drops a finished row from the list.
func (m *Manager) RemoveTask(key string) {
	m.mu.Lock()
	t := m.byKey[key]
	if t == nil || !t.State.Terminal() {
		m.mu.Unlock()
		return
	}
	delete(m.byKey, key)
	m.tasks = removeTask(m.tasks, t)
	if !m.sourceHasTasksLocked(t.SourceKey) {
		m.sources = removeSource(m.sources, t.SourceKey)
	}
	sources := m.sourcesCopyLocked()
	m.mu.Unlock()
	m.sink.TaskRemoved(key)
	m.sink.Sources(sources)
	m.pushQueue(true)
}

// ClearFinished removes every terminal row and the sources left empty.
func (m *Manager) ClearFinished() {
	m.mu.Lock()
	var removed []string
	keep := make([]*taskState, 0, len(m.tasks))
	for _, t := range m.tasks {
		if t.State.Terminal() {
			removed = append(removed, t.Key)
			delete(m.byKey, t.Key)
			continue
		}
		keep = append(keep, t)
	}
	m.tasks = keep
	keptSources := make([]*Source, 0, len(m.sources))
	for _, s := range m.sources {
		if m.sourceHasTasksLocked(s.Key) {
			keptSources = append(keptSources, s)
		}
	}
	m.sources = keptSources
	sources := m.sourcesCopyLocked()
	m.mu.Unlock()
	for _, key := range removed {
		m.sink.TaskRemoved(key)
	}
	m.sink.Sources(sources)
	m.pushQueue(true)
}

// ResolveDuplicate answers a "possible duplicate" row. accept=true starts
// the download, accept=false skips the task.
func (m *Manager) ResolveDuplicate(key string, accept bool) {
	m.mu.Lock()
	t := m.byKey[key]
	if t == nil || t.State != StateNeedsReview {
		m.mu.Unlock()
		return
	}
	t.Candidate = nil
	if accept {
		t.State = StateQueued
	} else {
		t.State = StateSkipped
		t.Percent = 0
		t.Reason = "дубль, отменено пользователем"
	}
	copied := t.Task
	m.mu.Unlock()
	m.sink.TaskUpdated(copied)
	if accept {
		m.pushQueue(true)
		m.pump()
		return
	}
	m.sink.Log("[skip] "+copied.Label+" (дубль, отменено пользователем)", "info")
	m.pushQueue(true)
}

// Touch re-emits the aggregate and dispatches waiting tasks. Call it after
// a settings change: the window picks up the new worker limit immediately
// and the pool starts more tasks without waiting for a task to finish.
func (m *Manager) Touch() {
	m.pushQueue(true)
	m.pump()
}

// Snapshot returns the current queue state.
func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshotLocked()
}

// snapshotLocked builds the aggregate view. Positions are assigned to
// waiting tasks in queue order.
func (m *Manager) snapshotLocked() Snapshot {
	snap := Snapshot{
		Total:   len(m.tasks),
		Workers: m.workersCount(),
		Tasks:   make([]Task, 0, len(m.tasks)),
		Sources: m.sourcesCopyLocked(),
	}
	// Список показывается тремя группами: активные (в порядке запуска),
	// ожидающие и завершённые. Планировщик по-прежнему берёт задачи строго
	// из FIFO m.tasks, поэтому видимый порядок = порядок запуска, а
	// «качается N» всегда одной пачкой сверху.
	var active, pending, terminal []Task
	var share float64
	pos := 0
	for _, t := range m.tasks {
		copied := t.Task
		if copied.Candidate != nil {
			c := *copied.Candidate
			copied.Candidate = &c
		}
		switch t.State {
		case StateDone:
			snap.Done++
			share++
			terminal = append(terminal, copied)
		case StateSkipped:
			snap.Skipped++
			share++
			terminal = append(terminal, copied)
		case StateError:
			snap.Failed++
			share++
			terminal = append(terminal, copied)
		case StateCanceled:
			snap.Canceled++
			if t.Percent > 0 {
				share += float64(t.Percent) / 100
			}
			terminal = append(terminal, copied)
		case StateDownloading:
			snap.Active++
			if t.Percent > 0 {
				share += float64(t.Percent) / 100
			}
			active = append(active, copied)
		case StateNeedsReview:
			snap.Review++
			pending = append(pending, copied)
		case StateQueued:
			snap.Waiting++
			pos++
			copied.Position = pos
			pending = append(pending, copied)
		default:
			pending = append(pending, copied)
		}
		if t.Total > 0 {
			snap.BytesDone += t.Done
			snap.BytesTotal += t.Total
		}
	}
	snap.Tasks = append(append(active, pending...), terminal...)
	if snap.Total > 0 {
		snap.Percent = int(share*100/float64(snap.Total) + 0.5)
		if snap.Percent > 100 {
			snap.Percent = 100
		}
	}
	return snap
}

// pushQueue emits the aggregate; force bypasses the 250 ms throttle used
// for byte-progress updates.
func (m *Manager) pushQueue(force bool) {
	m.mu.Lock()
	now := m.now()
	if !force && now.Sub(m.lastQueue) < 250*time.Millisecond {
		m.mu.Unlock()
		return
	}
	m.lastQueue = now
	snap := m.snapshotLocked()
	m.mu.Unlock()
	m.sink.Queue(snap)
}

// SetClock replaces the time source (tests).
func (m *Manager) SetClock(now func() time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if now != nil {
		m.now = now
	}
}

func (m *Manager) sourcesCopyLocked() []Source {
	out := make([]Source, 0, len(m.sources))
	for _, s := range m.sources {
		out = append(out, *s)
	}
	return out
}

func (m *Manager) sourceHasTasksLocked(key string) bool {
	for _, t := range m.tasks {
		if t.SourceKey == key {
			return true
		}
	}
	return false
}

func removeTask(tasks []*taskState, target *taskState) []*taskState {
	out := make([]*taskState, 0, len(tasks))
	for _, t := range tasks {
		if t == target {
			continue
		}
		out = append(out, t)
	}
	return out
}

func removeSource(sources []*Source, key string) []*Source {
	out := make([]*Source, 0, len(sources))
	for _, s := range sources {
		if s.Key == key {
			continue
		}
		out = append(out, s)
	}
	return out
}

func isAlreadyExists(err error) bool { return errors.Is(err, downloader.ErrAlreadyExists) }

func isStopped(err error) bool { return errors.Is(err, downloader.ErrStopped) }

// pendingTask is an expanded task plus the lines the UI still needs.
type pendingTask struct {
	Task
	log     *logLine
	dupSkip bool
}

type logLine struct {
	text string
	kind string
}
