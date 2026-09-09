package downloader

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DownloadInfo is one audio source option for a track.
type DownloadInfo struct {
	BitrateInKbps   int    `json:"bitrateInKbps"`
	Codec           string `json:"codec"`
	DownloadInfoURL string `json:"downloadInfoUrl"`
	Preview         bool   `json:"preview"`
}

type downloadInfoResult struct {
	Result []DownloadInfo `json:"result"`
}

// downloadOptions fetches all audio source options for a track.
func (c *Client) downloadOptions(trackID string) ([]DownloadInfo, error) {
	var data downloadInfoResult
	if err := c.get(apiBase+"/tracks/"+trackID+"/download-info", &data); err != nil {
		return nil, err
	}
	return data.Result, nil
}

// bestOption picks the highest-bitrate source.
func bestOption(infos []DownloadInfo) (DownloadInfo, error) {
	if len(infos) == 0 {
		return DownloadInfo{}, ErrNoDownloadOptions
	}
	opts := append([]DownloadInfo(nil), infos...)
	sort.Slice(opts, func(i, j int) bool { return opts[i].BitrateInKbps > opts[j].BitrateInKbps })
	return opts[0], nil
}

type directLinkXML struct {
	Host string `xml:"host"`
	Path string `xml:"path"`
	Ts   string `xml:"ts"`
	S    string `xml:"s"`
}

// directLink resolves a downloadInfoUrl into a time-limited direct file URL.
func (c *Client) directLink(infoURL string) (string, error) {
	req, err := c.newRequest(http.MethodGet, infoURL)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloader: direct link: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloader: direct link: http %d", resp.StatusCode)
	}
	var info directLinkXML
	if err := xml.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("downloader: parse direct link: %w", err)
	}
	return buildDirectLink(info), nil
}

// buildDirectLink signs path+ts+s into a get-mp3 URL.
func buildDirectLink(info directLinkXML) string {
	sum := md5.Sum([]byte(signSalt + strings.TrimPrefix(info.Path, "/") + info.S))
	return fmt.Sprintf("https://%s/get-mp3/%s/%s%s", info.Host, hex.EncodeToString(sum[:]), info.Ts, info.Path)
}

// DownloadMP3 downloads the best MP3 for a track ID into outputDir.
// Pipeline: temp download → tag hook → atomic rename. Emits lifecycle events.
func (c *Client) DownloadMP3(trackID, outputDir string, emit func(Event)) (dest string, err error) {
	return c.DownloadMP3WithProgress(trackID, outputDir, emit, nil)
}

// DownloadMP3WithProgress is DownloadMP3 with live byte progress.
func (c *Client) DownloadMP3WithProgress(trackID, outputDir string, emit func(Event), onProgress ProgressFunc) (dest string, err error) {
	track, err := c.fetchTrack(trackID)
	if err != nil {
		emitEvent(emit, Event{Kind: KindFailed, Label: trackID, TrackID: trackID, Detail: err.Error()})
		return "", err
	}
	return c.downloadMP3Track(track, outputDir, emit, onProgress)
}

// downloadMP3Track runs the MP3 pipeline for an already-resolved track.
func (c *Client) downloadMP3Track(track *Track, outputDir string, emit func(Event), onProgress ProgressFunc) (dest string, err error) {
	trackID := track.ID
	label := track.Label()
	emitEvent(emit, Event{Kind: KindDownloading, Label: label, TrackID: trackID})
	defer func() {
		if err == nil {
			emitEvent(emit, Event{Kind: KindDone, Label: label, TrackID: trackID, Format: "MP3"})
		} else if errors.Is(err, ErrAlreadyExists) {
			emitEvent(emit, Event{Kind: KindSkipped, Label: label, TrackID: trackID, Detail: dest})
		} else {
			emitEvent(emit, Event{Kind: KindFailed, Label: label, TrackID: trackID, Detail: err.Error()})
		}
	}()

	dest = destPath(outputDir, track, ".mp3")
	if _, serr := os.Stat(dest); serr == nil {
		return dest, fmt.Errorf("%w: %s", ErrAlreadyExists, dest)
	}

	opts, err := c.downloadOptions(trackID)
	if err != nil {
		return "", err
	}
	best, err := bestOption(opts)
	if err != nil {
		return "", err
	}
	link, err := c.directLink(best.DownloadInfoURL)
	if err != nil {
		return "", err
	}
	tmp, err := c.downloadToTemp(link, outputDir, ".mp3", track.ID, label, onProgress)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp) // no-op after successful publish
	if err = c.runTagHook(tmp, track); err != nil {
		return "", err
	}
	if err = publishTemp(tmp, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// downloadToTemp streams url into a temp file in dir. The temp file carries
// ext so format dispatch (tagging) works before publish. No full file in RAM.
// Received bytes are reported via onProgress (nil = silent).
func (c *Client) downloadToTemp(url, dir, ext, trackID, label string, onProgress ProgressFunc) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("downloader: mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".yamdl-*"+ext)
	if err != nil {
		return "", fmt.Errorf("downloader: temp: %w", err)
	}
	tmpName := tmp.Name()

	req, err := c.newRequest(http.MethodGet, url)
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", fmt.Errorf("downloader: GET file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		os.Remove(tmpName)
		return "", fmt.Errorf("downloader: GET file: http %d", resp.StatusCode)
	}
	total := resp.ContentLength
	src := newProgressReader(resp.Body, total, func(done, total int64) {
		emitProgress(onProgress, Progress{TrackID: trackID, Label: label, Done: done, Total: total, Percent: percentOf(done, total)})
	})
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", fmt.Errorf("downloader: write: %w", err)
	}
	emitProgress(onProgress, Progress{TrackID: trackID, Label: label, Done: src.done, Total: total, Percent: percentOf(src.done, total)})
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", fmt.Errorf("downloader: close: %w", err)
	}
	return tmpName, nil
}

// publishTemp atomically moves a finished temp file to its destination.
func publishTemp(tmp, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("downloader: mkdir: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("downloader: publish: %w", err)
	}
	return nil
}
