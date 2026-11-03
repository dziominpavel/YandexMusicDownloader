package quality

import (
	"path/filepath"
	"testing"

	"yamdl/internal/library"
)

func TestFormatOptions(t *testing.T) {
	cases := []struct {
		codec   string
		convert bool
		ffmpeg  bool
		want    string
	}{
		{"flac", false, false, "FLAC"},
		{"flac-mp4", true, true, "FLAC"},
		{"flac-mp4", true, false, "M4A"},
		{"flac-mp4", false, true, "M4A"},
	}
	for _, c := range cases {
		got := (FormatOptions{ConvertM4A: c.convert, FFmpeg: c.ffmpeg}).FoundFormat(c.codec)
		if got != c.want {
			t.Errorf("FoundFormat(%q, convert=%v, ffmpeg=%v) = %q, want %q",
				c.codec, c.convert, c.ffmpeg, got, c.want)
		}
	}
}

func TestPendingPreselectionAndAlreadyFlac(t *testing.T) {
	now := testTime()
	j := NewJournal("")
	entries := []library.Entry{
		{Path: `c:\music\id.mp3`, Ext: ".mp3", TrackID: "1", Artists: "Artist", Title: "ID Song", Matchable: true},
		{Path: `c:\music\exact.mp3`, Ext: ".mp3", Artists: "Artist", Title: "Exact", Matchable: true},
		{Path: `c:\music\fuzzy.mp3`, Ext: ".mp3", Artists: "Artist", Title: "Fuzzy", Matchable: true},
		{Path: `c:\music\paired.mp3`, Ext: ".mp3", TrackID: "2", Artists: "Artist", Title: "Paired", Matchable: true},
		{Path: `c:\music\paired.flac`, Ext: ".flac", TrackID: "2", Artists: "Artist", Title: "Paired", Matchable: true},
	}
	j.Put("1", Entry{Status: StatusLossless, Codec: "flac", MatchSource: "id"})
	j.Put("file:"+filepath.Join(`c:\music`, "exact.mp3"), Entry{
		Status: StatusLossless, Codec: "flac", MatchSource: "search",
		MatchedTrackID: "42", MatchedArtist: "Artist", MatchedTitle: "Exact",
		MatchedAlbum: "Album", MatchedYear: "2020", Confidence: 100, Exact: true,
	})
	j.Put("file:"+filepath.Join(`c:\music`, "fuzzy.mp3"), Entry{
		Status: StatusLossless, Codec: "flac", MatchSource: "search",
		MatchedTrackID: "43", MatchedArtist: "Artist", MatchedTitle: "Fuzzy!",
		Confidence: 93,
	})

	got := Pending(entries, j, now, FormatOptions{ConvertM4A: true, FFmpeg: true})
	if len(got) != 4 {
		t.Fatalf("want 4 pending items, got %d: %+v", len(got), got)
	}
	byPath := map[string]FoundItem{}
	for _, it := range got {
		byPath[it.Path] = it
	}
	if !byPath[`c:\music\id.mp3`].Selected || byPath[`c:\music\id.mp3`].Confidence != 100 {
		t.Fatalf("ID match must be preselected: %+v", byPath[`c:\music\id.mp3`])
	}
	if !byPath[`c:\music\exact.mp3`].Selected {
		t.Fatalf("exact search match must be preselected: %+v", byPath[`c:\music\exact.mp3`])
	}
	if byPath[`c:\music\fuzzy.mp3`].Selected {
		t.Fatalf("fuzzy search match must not be preselected: %+v", byPath[`c:\music\fuzzy.mp3`])
	}
	if byPath[`c:\music\paired.mp3`].AlreadyFlac != `c:\music\paired.flac` {
		t.Fatalf("existing FLAC must be linked: %+v", byPath[`c:\music\paired.mp3`])
	}
}

func TestPendingDeclinedIsSkipped(t *testing.T) {
	now := testTime()
	path := `c:\music\b.mp3`
	j := NewJournal("")
	j.Put("file:"+path, Entry{
		Status: StatusLossless, MatchSource: "search",
		MatchedTrackID: "42", Exact: true,
	})
	j.Decline(path, "42", now)
	entries := []library.Entry{{Path: path, Ext: ".mp3", Artists: "Artist", Title: "Song", Matchable: true}}
	if got := Pending(entries, j, now, FormatOptions{}); len(got) != 0 {
		t.Fatalf("declined match must not be pending: %+v", got)
	}
}
