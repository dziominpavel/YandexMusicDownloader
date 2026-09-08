package downloader

import (
	"path/filepath"
	"strings"
	"unicode/utf8"
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

// cyrillicTwin maps a Latin first letter to its Cyrillic look-alike.
// Only these pairs convert; any other first character keeps its form
// and the name gets sortPrefix instead (see sortArtistName).
var cyrillicTwin = map[rune]rune{
	'A': 'А', 'B': 'В', 'C': 'С', 'E': 'Е', 'H': 'Н',
	'K': 'К', 'M': 'М', 'O': 'О', 'P': 'Р', 'T': 'Т', 'X': 'Х',
	'a': 'а', 'c': 'с', 'e': 'е', 'h': 'н', 'k': 'к',
	'm': 'м', 'o': 'о', 'p': 'р', 't': 'т', 'x': 'х',
}

// sortPrefix pushes names that cannot start with Cyrillic (Latin letters
// without a Cyrillic twin, digits, symbols) into the gap between the
// Latin and Cyrillic blocks: U+03B9 sorts after Latin (ends U+007A)
// and before Cyrillic (starts U+0400), both in byte order and in
// Windows linguistic sort. (Invisible U+00AD was tried: linguistic
// sort ignores it, so prefixed names floated before all Latin.)
const sortPrefix = "\u03b9 "

// isCyrillic reports Cyrillic block letters (U+0400–U+04FF, incl. Ё/ё).
func isCyrillic(r rune) bool {
	return r >= 0x0400 && r <= 0x04FF
}

// sortArtistName applies the first-letter sorting rule so Russian artists
// sort apart from foreign ones:
//   - Cyrillic first letter: unchanged ("Амирчик");
//   - Latin first letter with a Cyrillic twin: replaced (first char only)
//     ("Mari Sa" → "Мari Sa", "HEXXENMIND" → "НEXXENMIND");
//   - anything else (G, digits, symbols): iota prefix (U+03B9 + space),
//     so the name sorts after Latin and before Cyrillic.
func sortArtistName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	r, _ := utf8.DecodeRuneInString(name)
	switch {
	case isCyrillic(r):
		return name
	case cyrillicTwin[r] != 0:
		return string(cyrillicTwin[r]) + name[len(string(r)):]
	default:
		return sortPrefix + name
	}
}

// hasCyrillic reports whether s contains at least one Cyrillic letter.
func hasCyrillic(s string) bool {
	for _, r := range s {
		if isCyrillic(r) {
			return true
		}
	}
	return false
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
	if !hasCyrillic(name) && !hasCyrillic(title) {
		return name
	}
	return sortArtistName(name)
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
