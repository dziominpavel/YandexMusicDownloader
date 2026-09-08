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
