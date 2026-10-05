// Package norm holds the Cyrillic sorting rule for file names (the single
// source of truth used by downloader naming) and the reversible
// normalization used to compare track metadata when looking for duplicates.
package norm

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// SortPrefix pushes names that cannot start with Cyrillic (Latin letters
// without a Cyrillic twin, digits, symbols) into the gap between the
// Latin and Cyrillic blocks: U+03B9 sorts after Latin (ends U+007A)
// and before Cyrillic (starts U+0400), both in byte order and in
// Windows linguistic sort. (Invisible U+00AD was tried: linguistic
// sort ignores it, so prefixed names floated before all Latin.)
const SortPrefix = "ι "

// cyrillicTwin maps a Latin first letter to its Cyrillic look-alike.
// Only these pairs convert; any other first character keeps its form
// and the name gets SortPrefix instead (see SortArtistName).
var cyrillicTwin = map[rune]rune{
	'A': 'А', 'B': 'В', 'C': 'С', 'E': 'Е', 'H': 'Н',
	'K': 'К', 'M': 'М', 'O': 'О', 'P': 'Р', 'T': 'Т', 'X': 'Х',
	'a': 'а', 'c': 'с', 'e': 'е', 'h': 'н', 'k': 'к',
	'm': 'м', 'o': 'о', 'p': 'р', 't': 'т', 'x': 'х',
}

// SortArtistName applies the first-letter sorting rule so Russian artists
// sort apart from foreign ones:
//   - Cyrillic first letter: unchanged ("Амирчик");
//   - Latin first letter with a Cyrillic twin: replaced (first char only)
//     ("Mari Sa" → "Мari Sa", "HEXXENMIND" → "НEXXENMIND");
//   - anything else (G, digits, symbols): iota prefix (U+03B9 + space),
//     so the name sorts after Latin and before Cyrillic.
func SortArtistName(name string) string {
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
		return SortPrefix + name
	}
}

// isCyrillic reports Cyrillic block letters (U+0400–U+04FF, incl. Ё/ё).
func isCyrillic(r rune) bool {
	return r >= 0x0400 && r <= 0x04FF
}

// HasCyrillic reports whether s contains at least one Cyrillic letter.
func HasCyrillic(s string) bool {
	for _, r := range s {
		if isCyrillic(r) {
			return true
		}
	}
	return false
}

// canonTwin folds Latin/Cyrillic look-alike letters into the Latin form so
// "Мari Sa" and "Mari Sa" normalize to the same key regardless of which
// side carries the sort transform. Built from cyrillicTwin in both
// directions; letters without a twin pass through unchanged.
var canonTwin = func() map[rune]rune {
	m := make(map[rune]rune, len(cyrillicTwin)*2)
	for latin, cyr := range cyrillicTwin {
		m[cyr] = latin
		m[latin] = latin
	}
	return m
}()

// dashReplacer folds every dash flavour to a space, quoteReplacer drops
// quotes/apostrophes entirely (so "don't" and "dont" stay equal).
var (
	dashReplacer = strings.NewReplacer(
		"-", " ", "–", " ", "—", " ", "−", " ", "‐", " ", "‑", " ",
		"‒", " ", "―", " ",
	)
	quoteReplacer = strings.NewReplacer(
		`"`, "", "'", "", "«", "", "»", "", "‘", "", "’", "",
		"„", "", "“", "", "”", "",
	)
	spaceReplacer = strings.NewReplacer("\t", " ")
)

// Comparable normalizes s for duplicate matching. Both sides of a
// comparison (files in the collection and metadata from the API) MUST go
// through this function:
//
//  1. the sort prefix "ι " is stripped;
//  2. letters are lowercased, ё → е;
//  3. look-alike letters fold to the Latin form (А→A, с→c, ...);
//  4. dashes become spaces, quotes are dropped, spaces collapse.
func Comparable(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.TrimPrefix(s, SortPrefix)
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "ё", "е")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if c, ok := canonTwin[r]; ok {
			b.WriteRune(c)
			continue
		}
		b.WriteRune(r)
	}
	s = dashReplacer.Replace(b.String())
	s = quoteReplacer.Replace(s)
	s = spaceReplacer.Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// ArtistsKey normalizes an artist list into an order-independent key:
// every name through Comparable (comma-joined names split into tokens,
// so "B, A" from a file name equals ["A", "B"] from the API), tokens
// sorted, joined with ",". "B, A" and "A, B" produce the same key.
func ArtistsKey(artists []string) string {
	var tokens []string
	for _, a := range artists {
		for _, part := range strings.Split(a, ",") {
			if k := Comparable(part); k != "" {
				tokens = append(tokens, k)
			}
		}
	}
	sort.Strings(tokens)
	return strings.Join(tokens, ",")
}

// StripTrackNumber removes a leading track number from a file-name
// segment: "01 - Title" → "Title", "01" → "", "2Pac" → "2Pac".
func StripTrackNumber(s string) string {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 || i > 3 {
		return s
	}
	if i == len(s) {
		// A bare "01" carries no name: it is a pure track number.
		return ""
	}
	// The number must end at a separator to count as a track number.
	switch s[i] {
	case ' ', '.', ')', ']', '-':
		rest := strings.TrimLeft(s[i+1:], " .-)][")
		return rest
	default:
		return s
	}
}
