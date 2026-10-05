package library

import (
	"math"
	"sort"

	"yamdl/internal/norm"
)

// Matching thresholds (see design.md §4). A candidate qualifies when the
// normalized titles are at least minTitleSim similar — regardless of the
// artists, so covers and alternate versions are caught — or when the
// titles are at least minTitleLoose similar AND the artist sets are at
// least minArtistSim similar, which catches rewordings of the same song.
const (
	minTitleSim   = 0.90
	minTitleLoose = 0.80
	minArtistSim  = 0.90
	maxMatches    = 5
	artistWeight  = 0.3
)

// Match is one duplicate candidate found in the index.
type Match struct {
	Path    string // full path of the found file
	Artists string // as recorded in the index (raw)
	Title   string // as recorded in the index (raw)
	Ext     string // ".flac", ".m4a", ".mp3"
	Percent int    // title similarity, 0..100
}

// FindMatches returns up to maxMatches candidates for the given track,
// best first. An empty result means "no probable duplicate".
func (ix *Index) FindMatches(artists []string, title string) []Match {
	titleKey := norm.Comparable(title)
	if titleKey == "" {
		return nil
	}
	artistsKey := norm.ArtistsKey(artists)
	tRunes := []rune(titleKey)

	ix.mu.Lock()
	defer ix.mu.Unlock()

	var out []Match
	for _, e := range ix.entries {
		if !e.Matchable {
			continue
		}
		simT := similarityCapped(tRunes, []rune(e.TitleKey), minTitleLoose)
		if simT < 0 {
			continue // too different to ever qualify
		}
		simA := 0.0
		if artistsKey != "" && e.ArtistsKey != "" {
			simA = similarityCapped([]rune(artistsKey), []rune(e.ArtistsKey), minArtistSim)
			if simA < 0 {
				simA = 0
			}
		}
		if simT < minTitleSim && !(simT >= minTitleLoose && simA >= minArtistSim) {
			continue
		}
		out = append(out, Match{
			Path:    e.Path,
			Artists: e.Artists,
			Title:   e.Title,
			Ext:     e.Ext,
			Percent: percentOf(simT, simA),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Percent != out[j].Percent {
			return out[i].Percent > out[j].Percent
		}
		return out[i].Path < out[j].Path
	})
	if len(out) > maxMatches {
		out = out[:maxMatches]
	}
	return out
}

// percentOf renders the displayed similarity. A title at or above
// minTitleSim is the whole score (a cover is a 100% title match even
// with another artist); a match that only qualified through the artist
// rule reports the blend, so the number never pretends to be higher
// than the evidence.
func percentOf(simT, simA float64) int {
	score := simT
	if simT < minTitleSim {
		score = (1-artistWeight)*simT + artistWeight*simA
	}
	p := int(math.Round(score * 100))
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// similarityCapped returns 1 - levenshtein/maxLen, or -1 when the
// distance certainly exceeds what min allows (d >= |len diff| gives a
// free early reject; the DP itself aborts as soon as its row minimum
// passes the cutoff).
func similarityCapped(a, b []rune, min float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return -1
	}
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	cutoff := int(math.Ceil((1 - min) * float64(maxLen)))
	diff := len(a) - len(b)
	if diff < 0 {
		diff = -diff
	}
	if diff > cutoff {
		return -1
	}
	d, ok := levenshteinCapped(a, b, cutoff)
	if !ok {
		return -1
	}
	return 1 - float64(d)/float64(maxLen)
}

// levenshteinCapped computes the edit distance up to cutoff; ok=false
// means the distance is known to exceed cutoff.
func levenshteinCapped(a, b []rune, cutoff int) (int, bool) {
	if len(a) == 0 {
		return len(b), len(b) <= cutoff
	}
	if len(b) == 0 {
		return len(a), len(a) <= cutoff
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		rowMin := curr[0]
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			v := del
			if ins < v {
				v = ins
			}
			if sub < v {
				v = sub
			}
			curr[j] = v
			if v < rowMin {
				rowMin = v
			}
		}
		if rowMin > cutoff {
			return 0, false // rows only grow: distance exceeds cutoff
		}
		prev, curr = curr, prev
	}
	d := prev[len(b)]
	return d, d <= cutoff
}
