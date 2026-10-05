package library

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bogem/id3v2/v2"
	"github.com/go-flac/flacvorbis"
	flac "github.com/go-flac/go-flac"
	"github.com/tommyo123/mtag"

	"yamdl/internal/norm"
)

// audioExts is the set of containers the indexer cares about — the same
// set the downloader can produce.
var audioExts = map[string]bool{
	".mp3":  true,
	".flac": true,
	".m4a":  true,
}

// readTags extracts artists and title from the file's own tags (ID3v2 for
// MP3, Vorbis comments for FLAC, MP4 atoms for M4A). Any read failure or
// missing value yields empty strings: the caller falls back to the name.
func readTags(path, ext string) (artists []string, title string) {
	switch ext {
	case ".mp3":
		return readMP3Tags(path)
	case ".flac":
		return readFLACTags(path)
	case ".m4a":
		return readM4ATags(path)
	}
	return nil, ""
}

func readMP3Tags(path string) ([]string, string) {
	// id3v2.Open leaks the file handle when the tag body fails to parse,
	// so only open files that actually carry an ID3v2 header. Everything
	// else is treated as "no tags" and falls back to the file name.
	if !hasID3v2Header(path) {
		return nil, ""
	}
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return nil, ""
	}
	defer tag.Close()
	var artists []string
	for _, f := range tag.GetFrames("TPE1") {
		if tf, ok := f.(id3v2.TextFrame); ok {
			// ID3v2.4 keeps multi-value frames NUL-separated.
			for _, part := range strings.Split(tf.Text, "\x00") {
				if part = strings.TrimSpace(part); part != "" {
					artists = append(artists, part)
				}
			}
		}
	}
	return artists, strings.TrimSpace(tag.Title())
}

// hasID3v2Header reports whether the file starts with an ID3 tag.
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

func readFLACTags(path string) ([]string, string) {
	file, err := flac.ParseFile(path)
	if err != nil {
		return nil, ""
	}
	for _, b := range file.Meta {
		if b.Type != flac.VorbisComment {
			continue
		}
		c, err := flacvorbis.ParseFromMetaDataBlock(*b)
		if err != nil {
			return nil, ""
		}
		var artists []string
		if vals, err := c.Get("ARTIST"); err == nil {
			for _, v := range vals {
				if v = strings.TrimSpace(v); v != "" {
					artists = append(artists, v)
				}
			}
		}
		title := ""
		if vals, err := c.Get("TITLE"); err == nil && len(vals) > 0 {
			title = strings.TrimSpace(vals[0])
		}
		return artists, title
	}
	return nil, ""
}

func readM4ATags(path string) ([]string, string) {
	f, err := mtag.Open(path)
	if err != nil {
		return nil, ""
	}
	defer func() { _ = f.Close() }()
	var artists []string
	for _, part := range strings.Split(f.Artist(), ",") {
		if part = strings.TrimSpace(part); part != "" {
			artists = append(artists, part)
		}
	}
	return artists, strings.TrimSpace(f.Title())
}

// fromFilename is the fallback parser for files without usable tags.
// It only trusts the "Artist - Title" shape (optionally after a track
// number): "01 - Песня" → title "Песня", "Исполнитель - Название" → both.
// A name without the separator ("track001") gives nothing — such a file
// stays out of matching rather than producing false warnings.
func fromFilename(base string) ([]string, string) {
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	i := strings.Index(stem, " - ")
	if i < 0 {
		return nil, ""
	}
	left := norm.StripTrackNumber(strings.TrimSpace(stem[:i]))
	right := strings.TrimSpace(stem[i+3:])
	right = strings.Trim(right, ". ")
	if right == "" {
		return nil, ""
	}
	if left == "" {
		return nil, right
	}
	return []string{left}, right
}
