// Package tagger writes metadata and cover art into downloaded files.
// Dispatch is by file extension: .mp3 (ID3v2.4), .flac (Vorbis), .m4a (MP4 atoms).
package tagger

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"yamdl/internal/downloader"
)

// DefaultCoverSize is the artwork resolution embedded into tags.
const DefaultCoverSize = "400x400"

// Meta is the tag content for one audio file.
type Meta struct {
	Title      string
	Artists    []string // each artist separately (no "A, B" folding except M4A)
	Album      string
	Year       string
	Genre      string
	TrackNum   int
	TrackTotal int
	DiscNum    int
	TrackID    string
	SourceURL  string
	Cover      []byte
	CoverMIME  string
}

// Write dispatches to the format writer by file extension.
func Write(path string, m Meta) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		return writeMP3(path, m)
	case ".flac":
		return writeFLAC(path, m)
	case ".m4a":
		return writeM4A(path, m)
	default:
		return fmt.Errorf("tagger: unsupported extension: %s", path)
	}
}

// Adapter builds a downloader.TagFunc: cover fetch (unless skipCover)
// + meta mapping + Write. Cover failures never fail the track.
func Adapter(skipCover bool) downloader.TagFunc {
	return func(path string, track *downloader.Track) error {
		m := MetaForTrack(track)
		if !skipCover {
			if url := CoverURL(track.CoverURI, DefaultCoverSize); url != "" {
				if data, mime, err := FetchCover(url); err == nil {
					m.Cover, m.CoverMIME = data, mime
				}
			}
		}
		return Write(path, m)
	}
}

// MetaForTrack maps a downloader track to tag metadata.
func MetaForTrack(t *downloader.Track) Meta {
	m := Meta{TrackID: strings.TrimSpace(t.ID)}
	m.Title = strings.TrimSpace(t.FullTitle())
	for _, a := range t.Artists {
		if a = strings.TrimSpace(a); a != "" {
			m.Artists = append(m.Artists, a)
		}
	}
	m.Album = strings.TrimSpace(t.Album)
	m.Year = strings.TrimSpace(t.Year)
	m.Genre = strings.TrimSpace(t.Genre)
	m.TrackNum = t.TrackNum
	m.TrackTotal = t.TrackTotal
	m.DiscNum = t.DiscNum
	if t.AlbumID != "" {
		m.SourceURL = fmt.Sprintf("https://music.yandex.ru/album/%s/track/%s", t.AlbumID, m.TrackID)
	} else if m.TrackID != "" {
		m.SourceURL = fmt.Sprintf("https://music.yandex.ru/track/%s", m.TrackID)
	}
	return m
}

// CoverURL expands a cover URI template (%% → size) into an https URL.
func CoverURL(uri, size string) string {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return ""
	}
	uri = strings.ReplaceAll(uri, "%%", size)
	switch {
	case strings.HasPrefix(uri, "http://"), strings.HasPrefix(uri, "https://"):
		return uri
	case strings.HasPrefix(uri, "//"):
		return "https:" + uri
	default:
		return "https://" + uri
	}
}

// FetchCover downloads artwork, returning bytes + detected MIME.
func FetchCover(url string) ([]byte, string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Yandex-Music-API")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("tagger: cover: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("tagger: cover: http %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil || len(data) == 0 {
		return nil, "", fmt.Errorf("tagger: cover: empty body")
	}
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		return nil, "", fmt.Errorf("tagger: cover: not an image: %s", mime)
	}
	return data, mime, nil
}
