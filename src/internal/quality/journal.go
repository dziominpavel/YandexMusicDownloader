// Package quality monitors the local collection for tracks that could be
// upgraded to lossless. It owns the check journal, candidate matching and
// the upgrade orchestration. It knows nothing about Wails.
package quality

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const journalVersion = 1

// Check statuses stored per track (or per local file for search matches).
const (
	StatusLossless    = "lossless"
	StatusNoLossless  = "no_lossless"
	StatusNoMatch     = "no_match"
	StatusUnavailable = "unavailable"
	StatusError       = "error"
)

// TTLs and the auto-check interval.
const (
	NoLosslessTTL     = 30 * 24 * time.Hour
	ErrorTTL          = 24 * time.Hour
	UnavailableTTL    = 90 * 24 * time.Hour
	DeclinedTTL       = 180 * 24 * time.Hour
	AutoCheckInterval = 7 * 24 * time.Hour
	RetryAfterError   = 24 * time.Hour
)

// Entry is one probe result. For files with a Yandex ID the map key is
// that ID; for search matches it is "file:" + local path.
type Entry struct {
	Status         string    `json:"status"`
	Codec          string    `json:"codec,omitempty"`
	CheckedAt      time.Time `json:"checked_at,omitempty"`
	NextCheck      time.Time `json:"next_check,omitempty"`
	MatchSource    string    `json:"match_source,omitempty"`
	MatchedTrackID string    `json:"matched_track_id,omitempty"`
	MatchedArtist  string    `json:"matched_artist,omitempty"`
	MatchedTitle   string    `json:"matched_title,omitempty"`
	MatchedAlbum   string    `json:"matched_album,omitempty"`
	MatchedYear    string    `json:"matched_year,omitempty"`
	Confidence     int       `json:"confidence,omitempty"`
	Exact          bool      `json:"exact,omitempty"`
}

// Declined remembers a search match the user explicitly rejected.
type Declined struct {
	TrackID string    `json:"track_id"`
	Until   time.Time `json:"until"`
}

// LastResult is shown by the main window and settings.
type LastResult struct {
	Found int    `json:"found"`
	Error string `json:"error,omitempty"`
}

// Status is a read-only snapshot for the UI.
type Status struct {
	LastAttempt   time.Time
	LastCompleted time.Time
	Found         int
	Error         string
	NextCheck     time.Time
}

type journalDoc struct {
	Version       int                 `json:"version"`
	AccountUID    int64               `json:"account_uid,omitempty"`
	HasPlus       bool                `json:"has_plus,omitempty"`
	LastAttempt   time.Time           `json:"last_attempt,omitempty"`
	LastCompleted time.Time           `json:"last_completed,omitempty"`
	LastResult    LastResult          `json:"last_result"`
	NextCheck     time.Time           `json:"next_check,omitempty"`
	Entries       map[string]Entry    `json:"entries"`
	Declined      map[string]Declined `json:"declined"`
}

// Journal is a thread-safe, atomically persisted check cache.
type Journal struct {
	path string
	mu   sync.Mutex
	doc  journalDoc
}

// NewJournal loads a journal from path. Missing, corrupt or old files
// are treated as an empty journal.
func NewJournal(path string) *Journal {
	j := &Journal{path: path, doc: emptyDoc()}
	j.load()
	return j
}

func emptyDoc() journalDoc {
	return journalDoc{
		Version:  journalVersion,
		Entries:  map[string]Entry{},
		Declined: map[string]Declined{},
	}
}

func (j *Journal) load() {
	if j.path == "" {
		return
	}
	raw, err := os.ReadFile(j.path)
	if err != nil {
		return
	}
	var doc journalDoc
	if err := json.Unmarshal(raw, &doc); err != nil || doc.Version != journalVersion {
		return
	}
	if doc.Entries == nil {
		doc.Entries = map[string]Entry{}
	}
	if doc.Declined == nil {
		doc.Declined = map[string]Declined{}
	}
	j.doc = doc
}

// Save writes the journal atomically (temp → rename).
func (j *Journal) Save() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.saveLocked()
}

func (j *Journal) saveLocked() {
	if j.path == "" {
		return
	}
	raw, err := json.Marshal(j.doc)
	if err != nil {
		return
	}
	tmp := j.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, j.path); err != nil {
		_ = os.Remove(tmp)
	}
}

// SetAccount updates the cached account. A changed uid clears everything;
// a Plus transition false→true clears the "no lossless" results so the
// collection is checked again. It reports whether a reset happened.
func (j *Journal) SetAccount(uid int64, hasPlus bool) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	reset := false
	switch {
	case j.doc.AccountUID != 0 && j.doc.AccountUID != uid:
		j.doc = emptyDoc()
		reset = true
	case hasPlus && !j.doc.HasPlus && j.doc.AccountUID != 0:
		j.doc.Entries = map[string]Entry{}
		j.doc.NextCheck = time.Time{}
		reset = true
	}
	j.doc.AccountUID = uid
	j.doc.HasPlus = hasPlus
	return reset
}

// Get returns one entry.
func (j *Journal) Get(key string) (Entry, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, ok := j.doc.Entries[key]
	return e, ok
}

// Put stores one entry.
func (j *Journal) Put(key string, e Entry) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.doc.Entries == nil {
		j.doc.Entries = map[string]Entry{}
	}
	j.doc.Entries[key] = e
}

// Due reports whether the entry is missing or its next check has arrived.
func (j *Journal) Due(key string, now time.Time) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, ok := j.doc.Entries[key]
	if !ok || e.NextCheck.IsZero() {
		return true
	}
	return !now.Before(e.NextCheck)
}

// Begin marks the start of a check attempt.
func (j *Journal) Begin(now time.Time) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.doc.LastAttempt = now
}

// Finish records the completed check. A successful check (even with zero
// findings) schedules the next auto-check in 7 days; an error retries in
// 24 hours and does not overwrite LastCompleted.
func (j *Journal) Finish(now time.Time, found int, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.doc.LastAttempt = now
	j.doc.LastResult = LastResult{Found: found}
	if err != nil {
		j.doc.LastResult.Error = err.Error()
		j.doc.NextCheck = now.Add(RetryAfterError)
		return
	}
	j.doc.LastCompleted = now
	j.doc.NextCheck = now.Add(AutoCheckInterval)
}

// DueCheck reports whether the whole collection check is due.
func (j *Journal) DueCheck(now time.Time) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.doc.NextCheck.IsZero() || !now.Before(j.doc.NextCheck)
}

// Status returns a copy for the UI.
func (j *Journal) Status() Status {
	j.mu.Lock()
	defer j.mu.Unlock()
	return Status{
		LastAttempt:   j.doc.LastAttempt,
		LastCompleted: j.doc.LastCompleted,
		Found:         j.doc.LastResult.Found,
		Error:         j.doc.LastResult.Error,
		NextCheck:     j.doc.NextCheck,
	}
}

// Decline remembers a rejected search match for a local file.
func (j *Journal) Decline(path, trackID string, now time.Time) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.doc.Declined == nil {
		j.doc.Declined = map[string]Declined{}
	}
	j.doc.Declined[path] = Declined{TrackID: trackID, Until: now.Add(DeclinedTTL)}
}

// IsDeclined reports whether this exact local file + candidate was rejected.
func (j *Journal) IsDeclined(path, trackID string, now time.Time) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	d, ok := j.doc.Declined[path]
	if !ok {
		return false
	}
	if !now.Before(d.Until) {
		delete(j.doc.Declined, path)
		return false
	}
	return d.TrackID == trackID
}

// Prune drops entries for tracks/files that are no longer in the index.
// existingKeys contains track IDs and "file:"+path keys; existingPaths
// contains local paths used by Declined.
func (j *Journal) Prune(existingKeys, existingPaths map[string]bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for k := range j.doc.Entries {
		if !existingKeys[k] {
			delete(j.doc.Entries, k)
		}
	}
	for p := range j.doc.Declined {
		if !existingPaths[p] {
			delete(j.doc.Declined, p)
		}
	}
}

// Path returns the backing file path (tests/observability).
func (j *Journal) Path() string {
	if j.path == "" {
		return ""
	}
	return filepath.Clean(j.path)
}
