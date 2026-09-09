package downloader

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// losslessSignKey signs get-file-info requests (part of the public API protocol).
const losslessSignKey = "7tvSmFbyf5hJnIHhCimDDD"

// ErrNoFLAC means the API has no lossless source for this track (fallback to MP3).
var ErrNoFLAC = errors.New("no lossless source available")

// losslessCodecs advertises containers we can handle, flac first.
func losslessCodecs() []string {
	return []string{"flac", "aac", "he-aac", "mp3", "flac-mp4", "aac-mp4", "he-aac-mp4"}
}

// signLossless builds the HMAC-SHA256 signature for get-file-info.
func signLossless(timestamp int64, trackID string) string {
	data := fmt.Sprintf("%d%slossless%sraw", timestamp, trackID, strings.Join(losslessCodecs(), ""))
	mac := hmac.New(sha256.New, []byte(losslessSignKey))
	mac.Write([]byte(data))
	return strings.TrimRight(base64.StdEncoding.EncodeToString(mac.Sum(nil)), "=")
}

// fileInfoURL builds the get-file-info request URL.
func fileInfoURL(trackID string, timestamp int64) string {
	q := url.Values{}
	q.Set("ts", fmt.Sprintf("%d", timestamp))
	q.Set("trackId", trackID)
	q.Set("quality", "lossless")
	q.Set("codecs", strings.Join(losslessCodecs(), ","))
	q.Set("transports", "raw")
	q.Set("sign", signLossless(timestamp, trackID))
	return apiBase + "/get-file-info?" + q.Encode()
}

type fileInfoPayload struct {
	Quality string   `json:"quality"`
	Codec   string   `json:"codec"`
	URLs    []string `json:"urls"`
	Key     string   `json:"key"`
	Bitrate int      `json:"bitrate"`
}

// losslessInfo is a parsed get-file-info answer.
type losslessInfo struct {
	Codec   string
	URLs    []string
	Key     string
	Bitrate int
}

// parseLosslessInfo tolerates snake_case, camelCase and result-wrapped payloads.
func parseLosslessInfo(body []byte) (losslessInfo, error) {
	var resp struct {
		Snake  *fileInfoPayload `json:"download_info"`
		Camel  *fileInfoPayload `json:"downloadInfo"`
		Result *struct {
			Snake *fileInfoPayload `json:"download_info"`
			Camel *fileInfoPayload `json:"downloadInfo"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return losslessInfo{}, fmt.Errorf("downloader: parse file info: %w", err)
	}
	p := resp.Snake
	if p == nil {
		p = resp.Camel
	}
	if p == nil && resp.Result != nil {
		p = resp.Result.Snake
		if p == nil {
			p = resp.Result.Camel
		}
	}
	if p == nil {
		return losslessInfo{}, ErrNoFLAC
	}
	return losslessInfo{Codec: p.Codec, URLs: p.URLs, Key: p.Key, Bitrate: p.Bitrate}, nil
}

// isFLACCodec reports containers we save as lossless.
func isFLACCodec(codec string) bool {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "flac", "flac-mp4":
		return true
	default:
		return false
	}
}

// losslessExt maps codec to file extension.
func losslessExt(codec string) string {
	if strings.EqualFold(strings.TrimSpace(codec), "flac-mp4") {
		return ".m4a"
	}
	return ".flac"
}

// accountUID resolves the numeric user id needed for lossless requests.
func (c *Client) accountUID() (int64, error) {
	var data struct {
		Result struct {
			Account struct {
				UID int64 `json:"uid"`
			} `json:"account"`
		} `json:"result"`
	}
	if err := c.get(apiBase+"/account/status", &data); err != nil {
		return 0, err
	}
	if data.Result.Account.UID == 0 {
		return 0, fmt.Errorf("downloader: no account uid (authorization required)")
	}
	return data.Result.Account.UID, nil
}

// losslessFileInfo fetches and validates the lossless source for a track.
func (c *Client) losslessFileInfo(trackID string, uid int64) (losslessInfo, error) {
	req, err := c.newRequest(http.MethodGet, fileInfoURL(trackID, time.Now().Unix()))
	if err != nil {
		return losslessInfo{}, err
	}
	req.Header.Set("x-yandex-music-client", "YandexMusicWebNext/1.0.0")
	req.Header.Set("Referer", "https://music.yandex.ru/")
	req.Header.Set("Origin", "https://music.yandex.ru")
	if uid > 0 {
		req.Header.Set("x-yandex-music-multi-auth-user-id", fmt.Sprintf("%d", uid))
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return losslessInfo{}, fmt.Errorf("downloader: file info: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return losslessInfo{}, fmt.Errorf("downloader: file info: http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return losslessInfo{}, fmt.Errorf("downloader: file info body: %w", err)
	}
	info, err := parseLosslessInfo(body)
	if err != nil {
		return losslessInfo{}, err
	}
	if !isFLACCodec(info.Codec) {
		return losslessInfo{}, fmt.Errorf("%w: codec %q", ErrNoFLAC, info.Codec)
	}
	if len(info.URLs) == 0 {
		return losslessInfo{}, fmt.Errorf("%w: no urls", ErrNoFLAC)
	}
	return info, nil
}

// Result describes a finished download.
type Result struct {
	Path      string
	Format    string // "FLAC", "M4A" or "MP3"
	Fallback  bool   // true when MP3 was used instead of lossless
	Converted bool   // true when ALAC-in-M4A was converted to FLAC via ffmpeg
	Label     string // "Artist — Title" for logs
}

// Download fetches a track in the best available quality:
// lossless first, MP3 fallback. Emits lifecycle events.
func (c *Client) Download(trackID, outputDir string, emit func(Event)) (res Result, err error) {
	return c.DownloadWithProgress(trackID, outputDir, emit, nil)
}

// DownloadWithProgress is Download with live byte progress.
func (c *Client) DownloadWithProgress(trackID, outputDir string, emit func(Event), onProgress ProgressFunc) (res Result, err error) {
	track, err := c.fetchTrack(trackID)
	if err != nil {
		emitEvent(emit, Event{Kind: KindFailed, Label: trackID, TrackID: trackID, Detail: err.Error()})
		return Result{}, err
	}
	return c.DownloadTrack(track, outputDir, emit, onProgress)
}

// DownloadTrack fetches an already-resolved track in the best quality.
// The playlist batch uses it to avoid an extra metadata request per track.
func (c *Client) DownloadTrack(track *Track, outputDir string, emit func(Event), onProgress ProgressFunc) (res Result, err error) {
	label := track.Label()
	tid := track.ID
	emitEvent(emit, Event{Kind: KindDownloading, Label: label, TrackID: tid})
	defer func() {
		if err == nil {
			res.Label = label
			emitEvent(emit, Event{Kind: KindDone, Label: label, TrackID: tid, Format: res.Format, Fallback: res.Fallback, Converted: res.Converted})
		} else if errors.Is(err, ErrAlreadyExists) {
			emitEvent(emit, Event{Kind: KindSkipped, Label: label, TrackID: tid, Detail: res.Path})
		} else {
			emitEvent(emit, Event{Kind: KindFailed, Label: label, TrackID: tid, Detail: err.Error()})
		}
	}()

	if uid, uerr := c.accountUID(); uerr == nil {
		if res, lerr := c.downloadLossless(track, outputDir, uid, onProgress); lerr == nil || errors.Is(lerr, ErrAlreadyExists) {
			return res, lerr
		}
		// Any other lossless failure falls back to MP3 below.
	}

	path, err := c.downloadMP3Track(track, outputDir, nil, onProgress) // events emitted by outer defer
	if err != nil {
		if errors.Is(err, ErrAlreadyExists) {
			return Result{Path: path, Format: "MP3", Fallback: true}, err
		}
		return Result{}, err
	}
	// MP3 after a lossless attempt (or without uid) is a fallback.
	return Result{Path: path, Format: "MP3", Fallback: true}, nil
}

// downloadLossless streams, decrypts, tags and publishes a lossless file.
func (c *Client) downloadLossless(track *Track, outputDir string, uid int64, onProgress ProgressFunc) (Result, error) {
	info, err := c.losslessFileInfo(track.ID, uid)
	if err != nil {
		return Result{}, err
	}
	streamFormat := "FLAC"
	if strings.EqualFold(info.Codec, "flac-mp4") {
		streamFormat = "M4A"
	}
	format := streamFormat
	// ALAC-in-M4A becomes plain FLAC when the sidecar converter is
	// available: better player support, bit-identical audio.
	ffmpeg := ""
	if streamFormat == "M4A" && c.ConvertM4A {
		ffmpeg = findFFmpeg()
		if ffmpeg != "" {
			format = "FLAC"
		}
	}
	dest := destPath(outputDir, track, losslessExt(info.Codec))
	if ffmpeg != "" {
		dest = destPath(outputDir, track, ".flac")
	}
	if _, err := os.Stat(dest); err == nil {
		return Result{Path: dest, Format: format}, fmt.Errorf("%w: %s", ErrAlreadyExists, dest)
	}

	var stream cipher.Stream
	if strings.TrimSpace(info.Key) != "" {
		key, err := hex.DecodeString(strings.TrimSpace(info.Key))
		if err != nil {
			return Result{}, fmt.Errorf("downloader: decode key: %w", err)
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return Result{}, fmt.Errorf("downloader: cipher: %w", err)
		}
		stream = cipher.NewCTR(block, make([]byte, aes.BlockSize))
	}

	var lastErr error
	for _, rawURL := range info.URLs {
		tmp, err := c.streamLosslessTemp(rawURL, outputDir, stream, streamFormat, losslessExt(info.Codec), track.ID, track.Label(), onProgress)
		if err != nil {
			lastErr = err
			continue
		}
		converted := false
		if ffmpeg != "" {
			flacTmp, cerr := alacToFLAC(ffmpeg, tmp, outputDir)
			os.Remove(tmp)
			if cerr != nil {
				lastErr = cerr
				continue
			}
			tmp = flacTmp
			converted = true
		}
		if err := c.runTagHook(tmp, track); err != nil {
			os.Remove(tmp)
			lastErr = err
			continue
		}
		if err := publishTemp(tmp, dest); err != nil {
			lastErr = err
			continue
		}
		return Result{Path: dest, Format: format, Converted: converted}, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: no urls", ErrNoFLAC)
	}
	return Result{}, lastErr
}

// streamLosslessURL downloads one URL with streaming decrypt and flac magic check.
// streamLosslessTemp downloads one URL into a temp file with streaming
// decrypt and flac magic check. Returns the temp path.
// Network bytes are reported via onProgress (nil = silent).
func (c *Client) streamLosslessTemp(rawURL, dir string, stream cipher.Stream, format, ext, trackID, label string, onProgress ProgressFunc) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".yamdl-*"+ext)
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()

	req, err := c.newRequest(http.MethodGet, rawURL)
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		tmp.Close()
		os.Remove(tmpName)
		return "", fmt.Errorf("downloader: GET file: http %d", resp.StatusCode)
	}

	total := resp.ContentLength
	counter := newProgressReader(resp.Body, total, func(done, total int64) {
		emitProgress(onProgress, Progress{TrackID: trackID, Label: label, Done: done, Total: total, Percent: percentOf(done, total)})
	})
	var src io.Reader = counter
	if stream != nil {
		src = &cipher.StreamReader{S: stream, R: counter}
	}
	// Peek magic without loading the file: fLaC for FLAC.
	head := make([]byte, 4)
	if _, err := io.ReadFull(src, head); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", fmt.Errorf("downloader: peek: %w", err)
	}
	if format == "FLAC" && !bytes.Equal(head, []byte("fLaC")) {
		tmp.Close()
		os.Remove(tmpName)
		return "", fmt.Errorf("downloader: not a flac stream")
	}
	if _, err := tmp.Write(head); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	emitProgress(onProgress, Progress{TrackID: trackID, Label: label, Done: counter.done, Total: total, Percent: percentOf(counter.done, total)})
	return tmpName, nil
}
