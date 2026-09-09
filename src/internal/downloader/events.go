package downloader

// EventKind is a download lifecycle stage for the UI log.
type EventKind string

const (
	// KindDownloading: "[downloading] Artist — Title".
	KindDownloading EventKind = "downloading"
	// KindDone: "[done] Artist — Title (FORMAT)".
	KindDone EventKind = "done"
	// KindSkipped: "[skip] Artist — Title (already exists)".
	KindSkipped EventKind = "skipped"
	// KindFailed: "[error] Artist — Title: reason".
	KindFailed EventKind = "failed"
)

// Event is one lifecycle fact the UI renders as a log line.
type Event struct {
	Kind      EventKind
	Label     string // "Artist — Title"
	TrackID   string // stable key; the UI updates one row per TrackID
	Detail    string // error text or extra info
	Format    string // "FLAC", "M4A", "MP3" (for KindDone)
	Fallback  bool   // true when MP3 was used instead of lossless
	Converted bool   // true when ALAC-in-M4A was converted to FLAC
}

// emitEvent delivers e unless emit is nil.
func emitEvent(emit func(Event), e Event) {
	if emit != nil {
		emit(e)
	}
}

// Progress is a live byte counter for one downloading track.
// It travels on its own channel (not the log), so the UI can update
// the track's row in place instead of appending lines.
type Progress struct {
	TrackID string // stable key for the UI row
	Label   string // "Artist — Title" for display
	Done    int64  // bytes received so far
	Total   int64  // bytes expected, -1 when the server hides the size
	Percent int    // 0..100, -1 when Total is unknown
}

// ProgressFunc receives progress updates; nil means no reporting.
type ProgressFunc func(Progress)

// emitProgress delivers p unless fn is nil.
func emitProgress(fn ProgressFunc, p Progress) {
	if fn != nil {
		fn(p)
	}
}

// percentOf maps done/total to 0..100, or -1 when total is unknown.
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
