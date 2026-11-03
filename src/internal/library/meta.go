package library

import (
	"path/filepath"
	"strings"

	"yamdl/internal/norm"
	"yamdl/internal/tagread"
)

// audioExts is the set of containers the indexer cares about — the same
// set the downloader can produce.
var audioExts = map[string]bool{
	".mp3":  true,
	".flac": true,
	".m4a":  true,
}

// fileTags is kept as an alias so the index code stays readable.
type fileTags = tagread.Tags

// readTags is a thin wrapper over the shared tag reader.
func readTags(path, ext string) fileTags {
	return tagread.Read(path, ext)
}

// TrackIDOf returns the Yandex track id stored in a file's tags, or "".
func TrackIDOf(path string) string {
	return tagread.TrackIDOf(path)
}

// fromFilename is the fallback parser for files without usable tags.
// It only trusts the "Artist - Title" shape (optionally after a track
// number): "01 - Песня" → title "Песня", "Исполнитель - Название" → both.
// A name without the separator ("track001") gives nothing — such a file
// stays out of matching rather than producing false warnings.
func fromFilename(base string) ([]string, string) {
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	i := strings.Index(stem, " - ")
	if i < 0 {
		return nil, ""
	}
	left := norm.StripTrackNumber(strings.TrimSpace(stem[:i]))
	right := strings.TrimSpace(stem[i+3:])
	right = strings.Trim(right, ". ")
	if right == "" {
		return nil, ""
	}
	if left == "" {
		return nil, right
	}
	return []string{left}, right
}
