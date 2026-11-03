// Package quality monitors the local collection for tracks that could be
// upgraded to lossless. It owns the check journal, candidate matching and
// the upgrade orchestration. It knows nothing about Wails.
package quality

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const journalVersion = 2

// Check statuses stored per track (or per local file for search matches).
const (
	StatusLossless    = "lossless"
	StatusNoLossless  = "no_lossless"
	StatusNoMatch     = "no_match"
	StatusUnavailable = "unavailable"
	StatusError       = "error"
)

// TTLs and the auto-check interval. DeclinedTTL survives only to
// approximate the date of v1 rejections during migration.
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

// Declined remembers a search match the user explicitly rejected. Multiple
// candidates are kept per local file; a rejection never expires by itself.
type Declined struct {
	TrackID       string    `json:"track_id"`
	LocalLabel    string    `json:"local_label,omitempty"`
	MatchedArtist string    `json:"matched_artist,omitempty"`
	MatchedTitle  string    `json:"matched_title,omitempty"`
	MatchedAlbum  string    `json:"matched_album,omitempty"`
	MatchedYear   string    `json:"matched_year,omitempty"`
	Confidence    int       `json:"confidence,omitempty"`
	Exact         bool      `json:"exact,omitempty"`
	Codec         string    `json:"codec,omitempty"`
	DeclinedAt    time.Time `json:"declined_at,omitempty"`
}

// DeclinedItem is the settings-window view of one rejected match.
type DeclinedItem struct {
	Path           string    `json:"path"`
	FileName       string    `json:"fileName"`
	LocalLabel     string    `json:"localLabel"`
	TrackID        string    `json:"trackId"`
	MatchedArtist  string    `json:"matchedArtist,omitempty"`
	MatchedTitle   string    `json:"matchedTitle,omitempty"`
	Album          string    `json:"album,omitempty"`
	Year           string    `json:"year,omitempty"`
	Confidence     int       `json:"confidence,omitempty"`
	DeclinedAt     time.Time `json:"declinedAt"`
	DeclinedAtText string    `json:"declinedAtText"`
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
	Version       int                   `json:"version"`
	AccountUID    int64                 `json:"account_uid,omitempty"`
	HasPlus       bool                  `json:"has_plus,omitempty"`
	LastAttempt   time.Time             `json:"last_attempt,omitempty"`
	LastCompleted time.Time             `json:"last_completed,omitempty"`
	LastResult    LastResult            `json:"last_result"`
	NextCheck     time.Time             `json:"next_check,omitempty"`
	Entries       map[string]Entry      `json:"entries"`
	Declined      map[string][]Declined `json:"declined"`
}

type declinedV1 struct {
	TrackID string    `json:"track_id"`
	Until   time.Time `json:"until"`
}

type journalDocV1 struct {
	Version       int                   `json:"version"`
	AccountUID    int64                 `json:"account_uid,omitempty"`
	HasPlus       bool                  `json:"has_plus,omitempty"`
	LastAttempt   time.Time             `json:"last_attempt,omitempty"`
	LastCompleted time.Time             `json:"last_completed,omitempty"`
	LastResult    LastResult            `json:"last_result"`
	NextCheck     time.Time             `json:"next_check,omitempty"`
	Entries       map[string]Entry      `json:"entries"`
	Declined      map[string]declinedV1 `json:"declined"`
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
		Declined: map[string][]Declined{},
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
	var head struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return
	}
	switch head.Version {
	case journalVersion:
		var doc journalDoc
		if err := json.Unmarshal(raw, &doc); err != nil {
			return
		}
		j.doc = normalizeDoc(doc)
	case 1:
		var old journalDocV1
		if err := json.Unmarshal(raw, &old); err != nil {
			return
		}
		j.doc = migrateV1(old)
		// Persist the migration right away so a crash before the first
		// check does not repeat it (and does not lose the migrated list).
		j.Save()
	}
}

func normalizeDoc(doc journalDoc) journalDoc {
	doc.Version = journalVersion
	if doc.Entries == nil {
		doc.Entries = map[string]Entry{}
	}
	if doc.Declined == nil {
		doc.Declined = map[string][]Declined{}
	}
	return doc
}

// migrateV1 converts the one-rejection-per-path journal into the v2 list.
// Expired rejections are dropped: v1 treated them as forgotten. Metadata
// is copied from the matching "file:" entry when it is still present.
func migrateV1(old journalDocV1) journalDoc {
	now := time.Now()
	doc := journalDoc{
		Version:       journalVersion,
		AccountUID:    old.AccountUID,
		HasPlus:       old.HasPlus,
		LastAttempt:   old.LastAttempt,
		LastCompleted: old.LastCompleted,
		LastResult:    old.LastResult,
		NextCheck:     old.NextCheck,
		Entries:       old.Entries,
		Declined:      map[string][]Declined{},
	}
	if doc.Entries == nil {
		doc.Entries = map[string]Entry{}
	}
	for path, d := range old.Declined {
		if d.TrackID == "" {
			continue
		}
		if !d.Until.IsZero() && !now.Before(d.Until) {
			continue
		}
		rec := Declined{
			TrackID:    d.TrackID,
			LocalLabel: filepath.Base(path),
			DeclinedAt: now,
		}
		if !d.Until.IsZero() {
			rec.DeclinedAt = d.Until.Add(-DeclinedTTL)
		}
		if e, ok := doc.Entries["file:"+path]; ok {
			rec.MatchedArtist = e.MatchedArtist
			rec.MatchedTitle = e.MatchedTitle
			rec.MatchedAlbum = e.MatchedAlbum
			rec.MatchedYear = e.MatchedYear
			rec.Confidence = e.Confidence
			rec.Exact = e.Exact
			rec.Codec = e.Codec
		}
		doc.Declined[path] = append(doc.Declined[path], rec)
	}
	return doc
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

// Decline remembers a rejected search match for a local file. Rejecting
// the same candidate again replaces its metadata; another candidate is
// appended.
func (j *Journal) Decline(path string, d Declined) {
	if path == "" || d.TrackID == "" {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.doc.Declined == nil {
		j.doc.Declined = map[string][]Declined{}
	}
	if d.DeclinedAt.IsZero() {
		d.DeclinedAt = time.Now()
	}
	list := j.doc.Declined[path]
	for i := range list {
		if list[i].TrackID == d.TrackID {
			list[i] = d
			j.doc.Declined[path] = list
			return
		}
	}
	j.doc.Declined[path] = append(list, d)
}

// IsDeclined reports whether this exact local file + candidate was rejected.
// Rejections have no automatic expiry.
func (j *Journal) IsDeclined(path, trackID string) bool {
	if path == "" || trackID == "" {
		return false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, d := range j.doc.Declined[path] {
		if d.TrackID == trackID {
			return true
		}
	}
	return false
}

// Undecline removes one rejection and returns its stored metadata.
func (j *Journal) Undecline(path, trackID string) (Declined, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	list := j.doc.Declined[path]
	for i, d := range list {
		if d.TrackID != trackID {
			continue
		}
		if len(list) == 1 {
			delete(j.doc.Declined, path)
		} else {
			j.doc.Declined[path] = append(list[:i], list[i+1:]...)
		}
		return d, true
	}
	return Declined{}, false
}

// DeclinedList returns every rejection sorted by newest first.
func (j *Journal) DeclinedList() []DeclinedItem {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]DeclinedItem, 0)
	for path, list := range j.doc.Declined {
		for _, d := range list {
			label := d.LocalLabel
			if label == "" {
				label = filepath.Base(path)
			}
			out = append(out, DeclinedItem{
				Path: path, FileName: filepath.Base(path), LocalLabel: label,
				TrackID: d.TrackID, MatchedArtist: d.MatchedArtist,
				MatchedTitle: d.MatchedTitle, Album: d.MatchedAlbum,
				Year: d.MatchedYear, Confidence: d.Confidence,
				DeclinedAt: d.DeclinedAt, DeclinedAtText: declinedAtText(d.DeclinedAt),
			})
		}
	}
	sort.Slice(out, func(i, k int) bool {
		if !out[i].DeclinedAt.Equal(out[k].DeclinedAt) {
			return out[i].DeclinedAt.After(out[k].DeclinedAt)
		}
		if out[i].Path != out[k].Path {
			return out[i].Path < out[k].Path
		}
		return out[i].TrackID < out[k].TrackID
	})
	return out
}

func declinedAtText(t time.Time) string {
	if t.IsZero() {
		return "дата неизвестна"
	}
	return t.Format("02.01.2006 15:04")
}

// Forget drops one check entry, e.g. "file:"+path after a rejection.
func (j *Journal) Forget(key string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	delete(j.doc.Entries, key)
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
