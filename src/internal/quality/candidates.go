package quality

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"yamdl/internal/library"
	"yamdl/internal/norm"
)

// Candidate is one local MP3 the checker may probe or search for.
type Candidate struct {
	Path      string
	TrackID   string
	SourceURL string
	Artists   []string
	Title     string
}

// Candidates filters an index snapshot down to MP3 files. Files with a
// Yandex ID are probed directly; files without one need a search and
// must have a usable artist and title.
func Candidates(entries []library.Entry) []Candidate {
	var out []Candidate
	for _, e := range entries {
		if !strings.EqualFold(e.Ext, ".mp3") || isOldPath(e.Path) {
			continue
		}
		c := Candidate{
			Path:      e.Path,
			TrackID:   e.TrackID,
			SourceURL: e.SourceURL,
			Title:     e.Title,
		}
		if artists := strings.TrimSpace(e.Artists); artists != "" {
			for _, a := range strings.Split(artists, ", ") {
				if a = strings.TrimSpace(a); a != "" {
					c.Artists = append(c.Artists, a)
				}
			}
		}
		if c.TrackID == "" {
			if len(c.Artists) == 0 || strings.TrimSpace(c.Title) == "" || !e.Matchable {
				continue
			}
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// SearchQuery builds the artist+title query used for files without an ID.
func SearchQuery(c Candidate) string {
	title := cleanQuery(c.Title)
	if len(c.Artists) == 0 {
		return title
	}
	artists := make([]string, 0, len(c.Artists))
	for _, a := range c.Artists {
		if a = strings.TrimSpace(a); a != "" {
			artists = append(artists, a)
		}
	}
	return strings.TrimSpace(strings.Join(artists, " ") + " " + title)
}

var qualityMarker = regexp.MustCompile(`(?i)\s*[\[(](?:320|128|192|256|m4a|flac|mp3)(?:\s*kbps)?[\])]`)

// cleanQuery removes leading track numbers and obvious quality/format
// markers, while leaving Remix/Live/Remaster as meaningful version text.
func cleanQuery(s string) string {
	s = norm.StripTrackNumber(strings.TrimSpace(s))
	s = qualityMarker.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

func isOldPath(path string) bool {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		if strings.EqualFold(filepath.Base(dir), "_old") {
			return true
		}
		if next := filepath.Dir(dir); next == dir {
			return false
		}
	}
}
