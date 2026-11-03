// Package tagread extracts artists, title, Yandex ID and source URL from
// audio tags. It is shared by the collection index and the upgrader so
// both read the same fields without a package cycle.
package tagread

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bogem/id3v2/v2"
	"github.com/go-flac/flacvorbis"
	flac "github.com/go-flac/go-flac"
	"github.com/tommyo123/mtag"
)

const (
	yandexUFIDOwner = "music.yandex.ru"
	sourceURLDesc   = "yandex-music-downloader:source-url"
	m4aSourceField  = "----:com.yandex-music-downloader:source-url"
)

var trackIDPattern = regexp.MustCompile(`(?i)(?:album/\d+/)?track/(\d+)`)

// Tags is everything the index and upgrader need from a file's tags.
type Tags struct {
	Artists   []string
	Title     string
	TrackID   string
	SourceURL string
}

// Read extracts tags by extension. Any failure yields an empty result.
func Read(path, ext string) Tags {
	switch ext {
	case ".mp3":
		return readMP3Tags(path)
	case ".flac":
		return readFLACTags(path)
	case ".m4a":
		return readM4ATags(path)
	}
	return Tags{}
}

// TrackIDOf returns the Yandex track id stored in a file's tags, or "".
func TrackIDOf(path string) string {
	return Read(path, strings.ToLower(filepath.Ext(path))).TrackID
}

func readMP3Tags(path string) Tags {
	if !hasID3v2Header(path) {
		return Tags{}
	}
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return Tags{}
	}
	defer tag.Close()

	out := Tags{Title: strings.TrimSpace(tag.Title())}
	for _, f := range tag.GetFrames("TPE1") {
		if tf, ok := f.(id3v2.TextFrame); ok {
			for _, part := range strings.Split(tf.Text, "\x00") {
				if part = strings.TrimSpace(part); part != "" {
					out.Artists = append(out.Artists, part)
				}
			}
		}
	}
	for _, f := range tag.GetFrames("UFID") {
		if uf, ok := f.(id3v2.UFIDFrame); ok && strings.EqualFold(uf.OwnerIdentifier, yandexUFIDOwner) {
			if id := normalizeTrackID(string(uf.Identifier)); id != "" {
				out.TrackID = id
			}
		}
	}
	for _, f := range tag.GetFrames("COMM") {
		if cf, ok := f.(id3v2.CommentFrame); ok && looksLikeYandexURL(cf.Text) {
			if out.SourceURL == "" {
				out.SourceURL = strings.TrimSpace(cf.Text)
			}
		}
	}
	if out.TrackID == "" {
		out.TrackID = parseTrackID(out.SourceURL)
	}
	return out
}

func hasID3v2Header(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var h [3]byte
	if _, err := io.ReadFull(f, h[:]); err != nil {
		return false
	}
	return string(h[:]) == "ID3"
}

func readFLACTags(path string) Tags {
	// ParseMetadata reads only the FLAC header and metadata blocks. The
	// generic ParseFile reads the whole audio stream, which turns a
	// collection scan into gigabytes of disk I/O for no reason.
	f, err := os.Open(path)
	if err != nil {
		return Tags{}
	}
	defer f.Close()
	file, err := flac.ParseMetadata(f)
	if err != nil {
		return Tags{}
	}
	for _, b := range file.Meta {
		if b.Type != flac.VorbisComment {
			continue
		}
		c, err := flacvorbis.ParseFromMetaDataBlock(*b)
		if err != nil {
			return Tags{}
		}
		out := Tags{}
		if vals, err := c.Get("ARTIST"); err == nil {
			for _, v := range vals {
				if v = strings.TrimSpace(v); v != "" {
					out.Artists = append(out.Artists, v)
				}
			}
		}
		if vals, err := c.Get("TITLE"); err == nil && len(vals) > 0 {
			out.Title = strings.TrimSpace(vals[0])
		}
		if vals, err := c.Get("YANDEX_TRACK_ID"); err == nil && len(vals) > 0 {
			out.TrackID = normalizeTrackID(vals[0])
		}
		if vals, err := c.Get("COMMENT"); err == nil {
			for _, v := range vals {
				if looksLikeYandexURL(v) {
					out.SourceURL = strings.TrimSpace(v)
					break
				}
			}
		}
		if out.TrackID == "" {
			out.TrackID = parseTrackID(out.SourceURL)
		}
		return out
	}
	return Tags{}
}

func readM4ATags(path string) Tags {
	f, err := mtag.Open(path)
	if err != nil {
		return Tags{}
	}
	defer func() { _ = f.Close() }()

	out := Tags{Title: strings.TrimSpace(f.Title())}
	for _, part := range strings.Split(f.Artist(), ",") {
		if part = strings.TrimSpace(part); part != "" {
			out.Artists = append(out.Artists, part)
		}
	}
	out.SourceURL = strings.TrimSpace(f.CustomValue(m4aSourceField))
	if out.SourceURL == "" {
		if comment := strings.TrimSpace(f.Comment()); looksLikeYandexURL(comment) {
			out.SourceURL = comment
		}
	}
	out.TrackID = parseTrackID(out.SourceURL)
	if out.TrackID == "" {
		out.TrackID = normalizeTrackID(out.SourceURL)
	}
	return out
}

func parseTrackID(s string) string {
	m := trackIDPattern.FindStringSubmatch(strings.TrimSpace(s))
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

// NormalizeTrackID accepts a bare numeric id or a Yandex Music URL and
// returns the numeric id (tests/observability).
func NormalizeTrackID(s string) string {
	return normalizeTrackID(s)
}

func normalizeTrackID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if id := parseTrackID(s); id != "" {
		return id
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return s
}

func looksLikeYandexURL(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "music.yandex.") && strings.Contains(s, "/track/")
}
