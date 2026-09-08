package downloader

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestBestOptionPicksMaxBitrate(t *testing.T) {
	best, err := bestOption([]DownloadInfo{
		{BitrateInKbps: 128, Codec: "mp3"},
		{BitrateInKbps: 320, Codec: "mp3"},
		{BitrateInKbps: 192, Codec: "aac"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if best.BitrateInKbps != 320 {
		t.Fatalf("want 320, got %d", best.BitrateInKbps)
	}
}

func TestBestOptionEmpty(t *testing.T) {
	if _, err := bestOption(nil); !errors.Is(err, ErrNoDownloadOptions) {
		t.Fatalf("want ErrNoDownloadOptions, got %v", err)
	}
}

func TestBuildDirectLinkFormat(t *testing.T) {
	got := buildDirectLink(directLinkXML{Host: "h.example", Path: "/get-mp3/x", Ts: "1", S: "s"})
	if !strings.HasPrefix(got, "https://h.example/get-mp3/") || !strings.HasSuffix(got, "/1/get-mp3/x") {
		t.Fatalf("bad link: %s", got)
	}
	if len(strings.Split(got, "/")[4]) != 32 {
		t.Fatalf("sign is not md5 hex: %s", got)
	}
}

func TestTrackFilenameSanitized(t *testing.T) {
	tr := &Track{Artists: []string{"A/B"}, Title: `T:"<>?*|`, Version: ""}
	got := destPath("out", tr, ".mp3")
	// Latin-only artist + title = foreign: sanitized but not transformed.
	want := filepath.Join("out", "A_B - T_______.mp3")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDestPathFlat(t *testing.T) {
	tr := &Track{ID: "8", Title: "Song.", Artists: []string{"Auth/or"},
		Album: "Alb:um", Year: "2000", TrackNum: 8}
	got := destPath("out", tr, ".flac")
	// No folders, no track number, no album: flat "Artist - Title".
	// Latin-only = foreign, no first-letter transform.
	want := filepath.Join("out", "Auth_or - Song.flac")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSortArtistName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Амирчик", "Амирчик"},         // Cyrillic stays
		{"Mari Sa", "Мari Sa"},         // Latin M → Cyrillic М
		{"HEXXENMIND", "НEXXENMIND"},   // Latin H → Cyrillic Н (first char only)
		{"Noize MC", sortPrefix + "Noize MC"},     // N has no twin → prefix
		{"VAVAN", sortPrefix + "VAVAN"},           // V has no twin → prefix
		{"7Б", sortPrefix + "7Б"},                 // digit → prefix
		{"JANAGA", sortPrefix + "JANAGA"},         // J has no twin → prefix
		{"", ""},                       // empty stays empty
	}
	for _, c := range cases {
		if got := sortArtistName(c.in); got != c.want {
			t.Errorf("sortArtistName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSortPrefixOrder(t *testing.T) {
	// Byte-wise (codepoint) order must be: Latin < prefix < Cyrillic.
	// sortPrefix itself must be a single invisible rune.
	if sortPrefix != "\u03b9 " {
		t.Fatalf("prefix changed: %q", sortPrefix)
	}
	if !("zzz" < sortPrefix+"Noize MC" && sortPrefix+"Noize MC" < "Амирчик") {
		t.Fatal("order must be: latin < prefixed < cyrillic")
	}
}

func TestDisplayArtistRussianDetection(t *testing.T) {
	cases := []struct {
		artists []string
		title   string
		want    string
	}{
		// Foreign: no Cyrillic in artist or title → untouched.
		{[]string{"Metallica"}, "Enter Sandman", "Metallica"},
		{[]string{"ABBA"}, "Dancing Queen", "ABBA"},
		{[]string{"Nirvana"}, "Smells Like Teen Spirit", "Nirvana"},
		// Russian via Cyrillic title → transform applies.
		{[]string{"Mari Sa"}, "Моя Любовь", "Мari Sa"},
		{[]string{"Noize MC"}, "Светлая полоса", sortPrefix + "Noize MC"},
		{[]string{"HEXXENMIND"}, "Камин", "НEXXENMIND"},
		{[]string{"JANAGA"}, "Солнце, море и смех", sortPrefix + "JANAGA"},
		// Russian via Cyrillic artist → transform applies.
		{[]string{"7Б"}, "Знаю! Будет!", sortPrefix + "7Б"},
		{[]string{"Амирчик"}, "Не верю", "Амирчик"},
		// No artist at all.
		{nil, "Трек", ""},
	}
	for _, c := range cases {
		if got := displayArtist(c.artists, c.title); got != c.want {
			t.Errorf("displayArtist(%q, %q) = %q, want %q", c.artists, c.title, got, c.want)
		}
	}
}

func TestDestPathUserExamples(t *testing.T) {
	cases := []struct {
		track *Track
		ext   string
		want  string
	}{
		{&Track{Artists: []string{"Mari Sa"}, Title: "Моя Любовь"}, ".m4a", "Мari Sa - Моя Любовь.m4a"},
		{&Track{Artists: []string{"Noize MC"}, Title: "Светлая полоса"}, ".flac", sortPrefix + "Noize MC - Светлая полоса.flac"},
		{&Track{Artists: []string{"7Б"}, Title: "Знаю! Будет!"}, ".flac", sortPrefix + "7Б - Знаю! Будет!.flac"},
		{&Track{Artists: []string{"Амирчик"}, Title: "Не верю"}, ".flac", "Амирчик - Не верю.flac"},
		{&Track{Artists: []string{"Metallica"}, Title: "Enter Sandman"}, ".mp3", "Metallica - Enter Sandman.mp3"},
	}
	for _, c := range cases {
		got := destPath("out", c.track, c.ext)
		want := filepath.Join("out", c.want)
		if got != want {
			t.Errorf("got %q want %q", got, want)
		}
	}
}

func TestDestPathMissingMeta(t *testing.T) {
	tr := &Track{ID: "42"}
	got := destPath("out", tr, ".mp3")
	want := filepath.Join("out", "42.mp3")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
