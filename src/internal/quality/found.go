package quality

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"yamdl/internal/library"
)

// FormatOptions tells the UI what a lossless codec will become on disk.
type FormatOptions struct {
	ConvertM4A bool
	FFmpeg     bool
}

// FoundFormat maps a server codec to the published container.
func (o FormatOptions) FoundFormat(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "flac":
		return "FLAC"
	case "flac-mp4":
		if o.ConvertM4A && o.FFmpeg {
			return "FLAC"
		}
		return "M4A"
	default:
		return "FLAC"
	}
}

// FoundItem is one pending upgrade shown to the user and passed back to
// the upgrader. Key is the local path: it keeps rows unique when two
// MP3 copies map to the same Yandex ID.
type FoundItem struct {
	Key           string `json:"key"`
	Path          string `json:"path"`
	FileName      string `json:"fileName"`
	Label         string `json:"label"`
	CurrentFormat string `json:"currentFormat"`
	FoundFormat   string `json:"foundFormat"`
	Codec         string `json:"codec,omitempty"`
	MatchSource   string `json:"matchSource"` // "id" or "search"
	TrackID       string `json:"trackId,omitempty"`
	MatchedArtist string `json:"matchedArtist,omitempty"`
	MatchedTitle  string `json:"matchedTitle,omitempty"`
	Album         string `json:"album,omitempty"`
	Year          string `json:"year,omitempty"`
	Confidence    int    `json:"confidence,omitempty"`
	Selected      bool   `json:"selected"`
	AlreadyFlac   string `json:"alreadyFlac,omitempty"`
}

// Pending rebuilds the list of upgrade offers from the index and the
// journal. It is used after a check and after a restart, so the badge
// and the list survive without another API request.
func Pending(entries []library.Entry, j *Journal, now time.Time, opts FormatOptions) []FoundItem {
	flacByID := map[string]string{}
	for _, e := range entries {
		if strings.EqualFold(e.Ext, ".flac") && e.TrackID != "" {
			flacByID[e.TrackID] = e.Path
		}
	}

	var out []FoundItem
	for _, e := range entries {
		if !strings.EqualFold(e.Ext, ".mp3") {
			continue
		}
		key := e.TrackID
		if key == "" {
			key = "file:" + e.Path
		}
		je, has := Entry{}, false
		if j != nil {
			je, has = j.Get(key)
		}
		// A rejected search match must stay hidden even when a FLAC with
		// the same Yandex ID already exists in the collection.
		if has && j != nil && j.IsDeclined(e.Path, je.MatchedTrackID) {
			continue
		}

		matchID := e.TrackID
		if matchID == "" && has {
			matchID = je.MatchedTrackID
		}
		if flacPath, ok := flacByID[matchID]; ok && matchID != "" {
			item := baseItem(e, opts)
			item.AlreadyFlac = flacPath
			item.Codec = "flac"
			item.FoundFormat = "FLAC"
			item.MatchSource = "id"
			item.TrackID = matchID
			item.Selected = true
			item.Confidence = 100
			if has && je.MatchSource != "" {
				item.MatchSource = je.MatchSource
				item.MatchedArtist = je.MatchedArtist
				item.MatchedTitle = je.MatchedTitle
				item.Album = je.MatchedAlbum
				item.Year = je.MatchedYear
				item.Confidence = je.Confidence
			}
			out = append(out, item)
			continue
		}
		if !has || je.Status != StatusLossless {
			continue
		}

		item := baseItem(e, opts)
		item.MatchSource = je.MatchSource
		if item.MatchSource == "" {
			if e.TrackID != "" {
				item.MatchSource = "id"
			} else {
				item.MatchSource = "search"
			}
		}
		item.TrackID = e.TrackID
		if item.TrackID == "" {
			item.TrackID = je.MatchedTrackID
		}
		item.Codec = je.Codec
		item.FoundFormat = opts.FoundFormat(je.Codec)
		item.MatchedArtist = je.MatchedArtist
		item.MatchedTitle = je.MatchedTitle
		item.Album = je.MatchedAlbum
		item.Year = je.MatchedYear
		item.Confidence = je.Confidence
		if item.Confidence == 0 && item.MatchSource == "id" {
			item.Confidence = 100
		}
		item.Selected = item.MatchSource == "id" || je.Exact
		out = append(out, item)
	}

	sort.Slice(out, func(i, k int) bool { return out[i].Path < out[k].Path })
	return out
}

func baseItem(e library.Entry, opts FormatOptions) FoundItem {
	return FoundItem{
		Key:           e.Path,
		Path:          e.Path,
		FileName:      filepath.Base(e.Path),
		Label:         localLabel(e),
		CurrentFormat: "MP3",
		FoundFormat:   opts.FoundFormat("flac"),
	}
}

func localLabel(e library.Entry) string {
	artists := strings.TrimSpace(e.Artists)
	title := strings.TrimSpace(e.Title)
	switch {
	case artists != "" && title != "":
		return artists + " — " + title
	case title != "":
		return title
	case artists != "":
		return artists
	default:
		return filepath.Base(e.Path)
	}
}
