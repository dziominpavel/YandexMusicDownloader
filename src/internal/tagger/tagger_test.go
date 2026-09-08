package tagger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bogem/id3v2/v2"
	"github.com/go-flac/flacvorbis"
	flac "github.com/go-flac/go-flac"
	"github.com/tommyo123/mtag"

	"yamdl/internal/downloader"
)

func TestMetaForTrack(t *testing.T) {
	tr := &downloader.Track{
		ID: "8", Title: "Song", Version: "Live", Artists: []string{"A", "B"},
		Album: "Alb", AlbumID: "7", Year: "2000", Genre: "rusrock",
		TrackNum: 8, TrackTotal: 14, DiscNum: 1,
	}
	m := MetaForTrack(tr)
	if m.Title != "Song Live" {
		t.Fatalf("title: %q", m.Title)
	}
	if len(m.Artists) != 2 || m.Artists[1] != "B" {
		t.Fatalf("artists: %q", m.Artists)
	}
	if m.SourceURL != "https://music.yandex.ru/album/7/track/8" {
		t.Fatalf("url: %q", m.SourceURL)
	}
	if m.TrackTotal != 14 || m.DiscNum != 1 || m.Genre != "rusrock" {
		t.Fatalf("meta: %+v", m)
	}
}

func TestCoverURL(t *testing.T) {
	got := CoverURL("avatars.yandex.net/get-music-content/x/%%", "400x400")
	want := "https://avatars.yandex.net/get-music-content/x/400x400"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if CoverURL("", "400x400") != "" {
		t.Fatal("empty uri must give empty url")
	}
}

func TestWriteBadExt(t *testing.T) {
	if err := Write(filepath.Join(t.TempDir(), "x.ogg"), Meta{}); err == nil {
		t.Fatal("must fail on unknown extension")
	}
}

// Fixture roundtrips: copy a real file, tag it, read tags back.
// Fixtures: mtag testdata (MIT), cover.png from the same set.
func testMeta() Meta {
	cover, _ := os.ReadFile(filepath.Join("testdata", "cover.png"))
	return Meta{Title: "Test Title", Artists: []string{"Artist One", "Artist Two"},
		Album: "Test Album", Year: "2001", Genre: "rusrock",
		TrackNum: 8, TrackTotal: 14, DiscNum: 1,
		TrackID: "99", SourceURL: "https://music.yandex.ru/album/7/track/99",
		Cover: cover, CoverMIME: "image/png"}
}

func copyFixture(t *testing.T, name, ext string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "song"+ext)
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMP3Roundtrip(t *testing.T) {
	p := copyFixture(t, "mp3-id3v24.mp3", ".mp3")
	if err := Write(p, testMeta()); err != nil {
		t.Fatalf("write: %v", err)
	}
	tag, err := id3v2.Open(p, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tag.Close()
	if tag.Title() != "Test Title" {
		t.Fatalf("title: %q", tag.Title())
	}
	if tag.Album() != "Test Album" || tag.Year() != "2001" {
		t.Fatalf("album/year: %q/%q", tag.Album(), tag.Year())
	}
	artists := frameTexts(tag, "TPE1")
	if len(artists) != 1 {
		t.Fatalf("want 1 TPE1 frame, got %d", len(artists))
	}
	if parts := strings.Split(artists[0], "\x00"); len(parts) != 2 || parts[0] != "Artist One" || parts[1] != "Artist Two" {
		t.Fatalf("TPE1 multi-value: %q", artists[0])
	}
	if trck := frameTexts(tag, "TRCK"); len(trck) != 1 || trck[0] != "8/14" {
		t.Fatalf("TRCK: %q", trck)
	}
	if pics := tag.GetFrames(tag.CommonID("Attached picture")); len(pics) == 0 {
		t.Fatal("no cover attached")
	}
}

func frameTexts(tag *id3v2.Tag, id string) []string {
	var out []string
	for _, f := range tag.GetFrames(id) {
		if tf, ok := f.(id3v2.TextFrame); ok {
			out = append(out, tf.Text)
		}
	}
	return out
}

func TestFLACRoundtrip(t *testing.T) {
	p := copyFixture(t, "flac.flac", ".flac")
	if err := Write(p, testMeta()); err != nil {
		t.Fatalf("write: %v", err)
	}
	file, err := flac.ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var comments *flacvorbis.MetaDataBlockVorbisComment
	var pictures int
	for _, b := range file.Meta {
		switch b.Type {
		case flac.VorbisComment:
			c, err := flacvorbis.ParseFromMetaDataBlock(*b)
			if err != nil {
				t.Fatal(err)
			}
			comments = c
		case flac.Picture:
			pictures++
		}
	}
	if comments == nil {
		t.Fatal("no vorbis comments")
	}
	check := map[string][]string{
		"TITLE": {"Test Title"}, "ARTIST": {"Artist One", "Artist Two"},
		"TRACKTOTAL": {"14"}, "YANDEX_TRACK_ID": {"99"},
	}
	for k, want := range check {
		got, err := comments.Get(k)
		if err != nil || len(got) != len(want) {
			t.Fatalf("%s: got %q err %v", k, got, err)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s: got %q want %q", k, got, want)
			}
		}
	}
	if pictures == 0 {
		t.Fatal("no cover picture")
	}
}

func TestM4ARoundtrip(t *testing.T) {
	p := copyFixture(t, "mp4-aac.m4a", ".m4a")
	if err := Write(p, testMeta()); err != nil {
		t.Fatalf("write: %v", err)
	}
	file, err := mtag.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if file.Title() != "Test Title" || file.Album() != "Test Album" {
		t.Fatalf("title/album: %q/%q", file.Title(), file.Album())
	}
	if file.Track() != 8 || file.TrackTotal() != 14 {
		t.Fatalf("track: %d/%d", file.Track(), file.TrackTotal())
	}
	if len(file.Images()) == 0 {
		t.Fatal("no cover image")
	}
}
