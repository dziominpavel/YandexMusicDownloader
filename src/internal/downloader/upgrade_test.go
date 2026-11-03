package downloader

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUpgradeTarget(t *testing.T) {
	cases := []struct {
		codec            string
		convert, ffmpeg  bool
		wantExt, wantFmt string
		wantStreamFormat string
	}{
		{"flac", false, false, ".flac", "FLAC", "FLAC"},
		{"flac-mp4", true, true, ".flac", "FLAC", "M4A"},
		{"flac-mp4", true, false, ".m4a", "M4A", "M4A"},
		{"flac-mp4", false, true, ".m4a", "M4A", "M4A"},
	}
	for _, c := range cases {
		ext, format, stream := upgradeTarget(c.codec, c.convert, c.ffmpeg)
		if ext != c.wantExt || format != c.wantFmt || stream != c.wantStreamFormat {
			t.Errorf("upgradeTarget(%q, %v, %v) = (%q,%q,%q), want (%q,%q,%q)",
				c.codec, c.convert, c.ffmpeg, ext, format, stream,
				c.wantExt, c.wantFmt, c.wantStreamFormat)
		}
	}
}

func TestUpgradeDownloadsNextToOldFile(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "tagger", "testdata", "flac.flac"))
	if err != nil {
		t.Fatal(err)
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/account/status":
			_, _ = w.Write([]byte(accountJSON(true)))
		case "/get-file-info":
			_, _ = fmt.Fprintf(w, `{"result":{"downloadInfo":{"quality":"lossless","codec":"flac","urls":[%q]}}}`, srv.URL+"/audio")
		case "/audio":
			_, _ = w.Write(raw)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	oldPath := filepath.Join(dir, "Artist - Song.mp3")
	if err := os.WriteFile(oldPath, []byte("old mp3"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewClient("t")
	client.BaseURL = srv.URL
	client.EmbedLyrics = false
	track := &Track{ID: "42", Title: "Song", Artists: []string{"Artist"}}
	res, err := client.Upgrade(context.Background(), track, oldPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "Artist - Song.flac")
	if res.Path != want || res.Format != "FLAC" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("new FLAC missing: %v", err)
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("Upgrade must not move the old file itself: %v", err)
	}
}

func TestUpgradeDestConflict(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/account/status":
			_, _ = w.Write([]byte(accountJSON(true)))
		case "/get-file-info":
			_, _ = fmt.Fprintf(w, `{"result":{"downloadInfo":{"quality":"lossless","codec":"flac","urls":[%q]}}}`, srv.URL+"/audio")
		case "/audio":
			_, _ = w.Write([]byte("fLaC"))
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	oldPath := filepath.Join(dir, "Artist - Song.mp3")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Artist - Song.flac"), []byte("not a flac"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewClient("t")
	client.BaseURL = srv.URL
	client.EmbedLyrics = false
	_, err := client.Upgrade(context.Background(), &Track{ID: "42", Title: "Song", Artists: []string{"Artist"}}, oldPath, nil)
	if !errors.Is(err, ErrDestConflict) {
		t.Fatalf("want ErrDestConflict, got %v", err)
	}
}

func TestUpgradeNoLossless(t *testing.T) {
	srv := qualityServer(t, 200, `{"result":{"downloadInfo":{"quality":"lossless","codec":"mp3"}}}`)
	defer srv.Close()
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "Artist - Song.mp3")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewClient("t")
	client.BaseURL = srv.URL
	client.EmbedLyrics = false
	_, err := client.Upgrade(context.Background(), &Track{ID: "42", Title: "Song", Artists: []string{"Artist"}}, oldPath, nil)
	if !errors.Is(err, ErrNoFLAC) {
		t.Fatalf("want ErrNoFLAC, got %v", err)
	}
}
