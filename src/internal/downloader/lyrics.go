package downloader

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// lyricsSignKey signs the /tracks/{id}/lyrics request (Android app key,
// the same one the reference clients use).
const lyricsSignKey = "p93jhgh689SBReK6ghtw62"

// lrcTimeTag matches "[mm:ss.xx]" / "[mm:ss]" prefixes in LRC text.
var lrcTimeTag = regexp.MustCompile(`^\[\d{1,3}:\d{2}(?:[.:]\d{1,3})?\]`)

type lyricsInfo struct {
	Result struct {
		DownloadURL string `json:"downloadUrl"`
	} `json:"result"`
}

// fetchLyricsText asks the API for lyrics in TEXT or LRC format and
// downloads the payload. An absent lyrics entry is a normal error the
// callers ignore.
func (c *Client) fetchLyricsText(trackID, format string) (string, error) {
	ts := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte(lyricsSignKey))
	fmt.Fprintf(mac, "%s%d", trackID, ts)
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	u := fmt.Sprintf("%s/tracks/%s/lyrics?format=%s&timeStamp=%d&sign=%s",
		apiBase, url.PathEscape(trackID), format, ts, url.QueryEscape(sign))
	var data lyricsInfo
	if err := c.get(u, &data); err != nil {
		return "", err
	}
	if strings.TrimSpace(data.Result.DownloadURL) == "" {
		return "", fmt.Errorf("downloader: lyrics: empty download url")
	}

	req, err := http.NewRequestWithContext(c.reqCtx(), http.MethodGet, data.Result.DownloadURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloader: lyrics download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloader: lyrics download: http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", fmt.Errorf("downloader: lyrics read: %w", err)
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "", fmt.Errorf("downloader: lyrics: empty")
	}
	return text, nil
}

// lrcToText strips LRC time tags so the plain-text tag still reads well.
func lrcToText(lrc string) string {
	lines := strings.Split(strings.ReplaceAll(lrc, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = lrcTimeTag.ReplaceAllString(strings.TrimSpace(line), "")
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// prepareLyrics loads song text once per track. Errors are silent: a
// missing lyrics entry must never fail a download.
func (c *Client) prepareLyrics(t *Track) {
	if t == nil || t.lyricsLoaded || (!c.EmbedLyrics && !c.SaveLRC) {
		return
	}
	t.lyricsLoaded = true

	if c.SaveLRC {
		if lrc, err := c.fetchLyricsText(t.ID, "LRC"); err == nil {
			t.LyricsLRC = lrc
			if c.EmbedLyrics {
				if text := lrcToText(lrc); text != "" {
					t.Lyrics = text
					return
				}
			}
			return
		}
	}
	if c.EmbedLyrics {
		if text, err := c.fetchLyricsText(t.ID, "TEXT"); err == nil {
			t.Lyrics = text
		}
	}
}

// prepareAndTag loads lyrics (when enabled) and applies the tag hook.
func (c *Client) prepareAndTag(path string, track *Track) error {
	c.prepareLyrics(track)
	return c.runTagHook(path, track)
}

// saveLyricsLRC writes the synchronized lyrics next to the published
// audio file. Failures are ignored: the audio file is already valid.
func (c *Client) saveLyricsLRC(dest string, track *Track) {
	if !c.SaveLRC || track == nil || strings.TrimSpace(track.LyricsLRC) == "" {
		return
	}
	lrcPath := strings.TrimSuffix(dest, filepath.Ext(dest)) + ".lrc"
	_ = os.WriteFile(lrcPath, []byte(track.LyricsLRC), 0o644)
}
