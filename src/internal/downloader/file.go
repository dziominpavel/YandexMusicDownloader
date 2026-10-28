package downloader

import (
	"path/filepath"
	"strings"

	"yamdl/internal/norm"
)

// filenameReplacer sanitizes names for Windows/Unix filesystems.
var filenameReplacer = strings.NewReplacer(
	"/", "_", "\\", "_", ":", "_", "*", "_",
	"?", "_", `"`, "_", "<", "_", ">", "_", "|", "_",
)

// cleanSegment sanitizes one name part (artist or title).
func cleanSegment(s string) string {
	s = strings.TrimSpace(filenameReplacer.Replace(s))
	return strings.Trim(s, ". ")
}

// displayArtist joins the artists and applies the sorting rule — but only
// for Russian acts: if neither the artist name nor the track title contains
// Cyrillic, the act is considered foreign and the name is left untouched
// ("Metallica" stays "Metallica", never "Мetallica").
// Empty result means the track has no usable artist name.
func displayArtist(artists []string, title string) string {
	var names []string
	for _, a := range artists {
		if a = strings.TrimSpace(a); a != "" {
			names = append(names, a)
		}
	}
	if len(names) == 0 {
		return ""
	}
	name := cleanSegment(strings.Join(names, ", "))
	if !norm.HasCyrillic(name) && !norm.HasCyrillic(title) {
		return name
	}
	return norm.SortArtistName(name)
}

// trackStem is the file base name without extension: "Artist - Title".
// No track number, no album folders — flat layout, see destPath.
func trackStem(t *Track) string {
	title := cleanSegment(t.FullTitle())
	if title == "" {
		title = cleanSegment(t.ID)
	}
	artist := displayArtist(t.Artists, title)
	switch {
	case artist != "" && title != "":
		return artist + " - " + title
	case title != "":
		return title
	case artist != "":
		return artist
	default:
		return "track"
	}
}

// destPath builds the flat layout: "outputDir/Artist - Title.ext".
func destPath(outputDir string, t *Track, ext string) string {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return filepath.Join(outputDir, trackStem(t)+ext)
}

// destPath builds the destination for one track, honouring optional
// subdirectories. Subdirs=false keeps the flat layout.
func (c *Client) destPath(outputDir string, t *Track, ext string) string {
	if !c.Subdirs {
		return destPath(outputDir, t, ext)
	}
	sub := subdirFor(t, c.SubdirTemplate)
	if sub == "" {
		return destPath(outputDir, t, ext)
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return filepath.Join(outputDir, sub, trackStem(t)+ext)
}

// subdirFor expands %artist%, %album% and %year% into sanitized path
// segments. Empty template falls back to Artist/Album.
func subdirFor(t *Track, tmpl string) string {
	tmpl = strings.TrimSpace(tmpl)
	if tmpl == "" {
		tmpl = "%artist%/%album%"
	}
	artist := displayArtist(t.Artists, t.FullTitle())
	album := cleanSegment(t.Album)
	year := cleanSegment(t.Year)

	var parts []string
	for _, raw := range strings.FieldsFunc(tmpl, func(r rune) bool { return r == '/' || r == '\\' }) {
		part := raw
		part = strings.ReplaceAll(part, "%artist%", artist)
		part = strings.ReplaceAll(part, "%album%", album)
		part = strings.ReplaceAll(part, "%year%", year)
		if part = cleanSegment(part); part != "" {
			parts = append(parts, part)
		}
	}
	return filepath.Join(parts...)
}
