// Package downloader fetches audio from Yandex Music.
// v0.1: MP3 path here; lossless + fallback + events land in tasks 2.3-2.4.
package downloader

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const apiBase = "https://api.music.yandex.net"

// signSalt signs direct-link requests (part of the public API protocol).
const signSalt = "XGRlBW9FXlekgbPrRHuSiA"

// ErrAlreadyExists reports a skipped download: the file is already on disk.
var ErrAlreadyExists = errors.New("track file already exists")

// ErrNoDownloadOptions means the API returned no usable audio source.
var ErrNoDownloadOptions = errors.New("no download options available")

// Track is the metadata subset needed for download and tagging.
type Track struct {
	ID         string
	Title      string
	Version    string
	Artists    []string
	Album      string
	AlbumID    string
	Year       string
	Genre      string
	TrackNum   int
	TrackTotal int
	DiscNum    int
	CoverURI   string
	Available  bool
}

// FullTitle is "Title Version" trimmed.
func (t Track) FullTitle() string {
	return strings.TrimSpace(strings.TrimSpace(t.Title) + " " + strings.TrimSpace(t.Version))
}

// Label is "Artist — Title" for logs.
func (t Track) Label() string {
	a := strings.TrimSpace(strings.Join(t.Artists, ", "))
	title := strings.TrimSpace(t.FullTitle())
	switch {
	case a != "" && title != "":
		return a + " — " + title
	case title != "":
		return title
	default:
		return strings.TrimSpace(t.ID)
	}
}

// userAgent identifies us like the other API clients do.
const userAgent = "Yandex-Music-API"

// TagFunc writes metadata into a downloaded temp file before publish.
// Wired by the tagger in task 3.1.
type TagFunc func(path string, track *Track) error

// Client talks to the Yandex Music API.
type Client struct {
	token string
	http  *http.Client
	// Tag runs between download and publish; nil skips tagging.
	Tag TagFunc
	// SkipCover disables cover-art embedding (text tags still apply).
	SkipCover bool
	// ConvertM4A turns ALAC-in-M4A lossless into plain FLAC via the
	// ffmpeg.exe sidecar (see convert.go). No ffmpeg = M4A stays M4A.
	ConvertM4A bool
	// fetchTrack resolves metadata; override in tests to avoid network.
	fetchTrack func(string) (*Track, error)
	// downloadOne runs the per-track pipeline; override in tests.
	// Nil means DownloadTrack.
	downloadOne func(*Track, string, func(Event), ProgressFunc) (Result, error)
	// PlaylistWorkers caps parallel playlist downloads (PLAYLIST_WORKERS).
	// Zero or negative means DefaultWorkers (3); above MaxWorkers is capped.
	PlaylistWorkers int
}

// NewClient builds a downloader. Empty token = 30-second previews only.
func NewClient(token string) *Client {
	c := &Client{token: token, http: &http.Client{Timeout: 30 * time.Second}, ConvertM4A: true, PlaylistWorkers: defaultPlaylistWorkers}
	c.fetchTrack = c.TrackInfo
	return c
}

// runTagHook applies the tag hook (no-op when unset).
func (c *Client) runTagHook(path string, track *Track) error {
	if c.Tag == nil {
		return nil
	}
	if err := c.Tag(path, track); err != nil {
		return fmt.Errorf("downloader: tag: %w", err)
	}
	return nil
}

// newRequest builds an API request with the required headers.
func (c *Client) newRequest(method, url string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	if c.token != "" {
		req.Header.Set("Authorization", "OAuth "+c.token)
	}
	return req, nil
}

func (c *Client) get(url string, out any) error {
	req, err := c.newRequest(http.MethodGet, url)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("downloader: GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloader: GET %s: http %d", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("downloader: decode %s: %w", url, err)
	}
	return nil
}

type trackResult struct {
	Result []struct {
		ID        any    `json:"id"`
		Title     string `json:"title"`
		Version   string `json:"version"`
		Available bool   `json:"available"`
		CoverURI  string `json:"coverUri"`
		Artists   []struct {
			Name string `json:"name"`
		} `json:"artists"`
		Albums []struct {
			ID       any    `json:"id"`
			Title    string `json:"title"`
			Year     int    `json:"year"`
			Genre    string `json:"genre"`
			TrackCnt int    `json:"trackCount"`
			Position struct {
				Volume int `json:"volume"`
				Index  int `json:"index"`
			} `json:"trackPosition"`
		} `json:"albums"`
	} `json:"result"`
}

// TrackInfo fetches metadata for a track ID.
func (c *Client) TrackInfo(id string) (*Track, error) {
	var data trackResult
	if err := c.get(apiBase+"/tracks/"+id, &data); err != nil {
		return nil, err
	}
	if len(data.Result) == 0 {
		return nil, fmt.Errorf("downloader: track %s not found", id)
	}
	r := data.Result[0]
	t := &Track{ID: fmt.Sprint(r.ID), Title: r.Title, Version: r.Version, Available: r.Available, CoverURI: r.CoverURI}
	for _, a := range r.Artists {
		t.Artists = append(t.Artists, a.Name)
	}
	if len(r.Albums) > 0 {
		a := r.Albums[0]
		t.Album = a.Title
		t.AlbumID = fmt.Sprint(a.ID)
		if a.Year > 0 {
			t.Year = fmt.Sprintf("%d", a.Year)
		}
		t.Genre = a.Genre
		t.TrackNum = a.Position.Index
		t.TrackTotal = a.TrackCnt
		t.DiscNum = a.Position.Volume
	}
	return t, nil
}
