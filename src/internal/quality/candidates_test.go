package quality

import (
	"path/filepath"
	"testing"

	"yamdl/internal/library"
)

func TestCandidatesFilterAndID(t *testing.T) {
	entries := []library.Entry{
		{Path: `c:\music\a.mp3`, Ext: ".mp3", TrackID: "1", Title: "A", Artists: "Artist", Matchable: true},
		{Path: `c:\music\b.mp3`, Ext: ".mp3", Title: "B", Artists: "Artist", Matchable: true},
		{Path: `c:\music\c.mp3`, Ext: ".mp3", Title: "C", Matchable: false},
		{Path: `c:\music\d.flac`, Ext: ".flac", Title: "D", Artists: "Artist", Matchable: true},
		{Path: `c:\music\e.m4a`, Ext: ".m4a", Title: "E", Artists: "Artist", Matchable: true},
		{Path: `c:\music\_old\f.mp3`, Ext: ".mp3", TrackID: "2", Title: "F", Artists: "Artist", Matchable: true},
	}
	got := Candidates(entries)
	if len(got) != 2 {
		t.Fatalf("want two candidates (id and search), got %+v", got)
	}
	if got[0].TrackID != "1" || got[1].TrackID != "" {
		t.Fatalf("unexpected candidates: %+v", got)
	}
}

func TestCandidatesNestedOld(t *testing.T) {
	entries := []library.Entry{
		{Path: filepath.Join("music", "Rock", "_old", "a.mp3"), Ext: ".mp3", TrackID: "1", Title: "A", Artists: "X", Matchable: true},
	}
	if got := Candidates(entries); len(got) != 0 {
		t.Fatalf("nested _old must be skipped, got %+v", got)
	}
}

func TestSearchQueryCleaning(t *testing.T) {
	cases := []struct {
		c    Candidate
		want string
	}{
		{Candidate{Artists: []string{"Artist"}, Title: "01 - Song [320]"}, "Artist Song"},
		{Candidate{Artists: []string{"A", "B"}, Title: "Song (m4a)"}, "A B Song"},
		{Candidate{Artists: nil, Title: "Song [flac]"}, "Song"},
		{Candidate{Artists: []string{"Artist"}, Title: "Song (Live)"}, "Artist Song (Live)"},
	}
	for _, c := range cases {
		if got := SearchQuery(c.c); got != c.want {
			t.Errorf("SearchQuery(%+v) = %q, want %q", c.c, got, c.want)
		}
	}
}
