package downloader

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func stubClient() *Client {
	c := NewClient("")
	c.fetchTrack = func(id string) (*Track, error) {
		return &Track{ID: id, Title: "Title", Artists: []string{"Artist"}}, nil
	}
	return c
}

func TestPublishTempRenames(t *testing.T) {
	dir := t.TempDir()
	tmp, err := os.CreateTemp(dir, ".yamdl-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString("audio"); err != nil {
		t.Fatal(err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "out.mp3")
	if err := publishTemp(tmpName, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "audio" {
		t.Fatalf("dest content: %q, err %v", got, err)
	}
	if _, err := os.Stat(tmpName); !os.IsNotExist(err) {
		t.Fatal("temp must be gone after publish")
	}
}

func TestDownloadToTempFailureLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	c := NewClient("")
	if _, err := c.downloadToTemp("http://127.0.0.1:1/nope", dir, ".mp3", "1", "Artist — Title", nil); err == nil {
		t.Fatal("must fail")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("dir must be empty, has %d entries", len(entries))
	}
}

func TestTagHookFailureBlocksPublish(t *testing.T) {
	c := stubClient()
	if err := c.runTagHook("x", &Track{}); err != nil {
		t.Fatalf("nil hook must be no-op, got %v", err)
	}
	c.Tag = func(path string, track *Track) error { return errors.New("tag boom") }
	if err := c.runTagHook("x", &Track{}); err == nil {
		t.Fatal("failing hook must propagate")
	}
}

func TestEventsSkippedOnExists(t *testing.T) {
	c := stubClient()
	dir := t.TempDir()
	dest := destPath(dir, &Track{ID: "1", Title: "Title", Artists: []string{"Artist"}}, ".mp3")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	var got []Event
	_, err := c.DownloadMP3("1", dir, func(e Event) { got = append(got, e) })
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("want ErrAlreadyExists, got %v", err)
	}
	if len(got) != 2 || got[0].Kind != KindDownloading || got[1].Kind != KindSkipped {
		t.Fatalf("want [downloading skipped], got %+v", got)
	}
	// Existing file untouched.
	content, _ := os.ReadFile(dest)
	if string(content) != "old" {
		t.Fatal("existing file must not be touched")
	}
}

func TestEventsFailedOnResolveError(t *testing.T) {
	c := NewClient("")
	c.fetchTrack = func(id string) (*Track, error) { return nil, errors.New("boom") }
	var got []Event
	if _, err := c.Download("1", t.TempDir(), func(e Event) { got = append(got, e) }); err == nil {
		t.Fatal("must fail")
	}
	if len(got) != 1 || got[0].Kind != KindFailed {
		t.Fatalf("want single [failed], got %+v", got)
	}
}
