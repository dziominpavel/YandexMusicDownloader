package tagger

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-flac/flacpicture"
	"github.com/go-flac/flacvorbis"
	flac "github.com/go-flac/go-flac"
)

// writeFLAC rewrites Vorbis comments (one ARTIST per artist) and front cover.
func writeFLAC(path string, m Meta) error {
	file, err := flac.ParseFile(path)
	if err != nil {
		return fmt.Errorf("tagger: parse flac: %w", err)
	}

	comments := flacvorbis.New()
	addComment(comments, "TITLE", m.Title)
	for _, a := range m.Artists {
		addComment(comments, "ARTIST", a)
	}
	addComment(comments, "ALBUM", m.Album)
	if len(m.Artists) > 0 {
		addComment(comments, "ALBUMARTIST", m.Artists[0])
	}
	addComment(comments, "GENRE", m.Genre)
	if m.TrackNum > 0 {
		addComment(comments, "TRACKNUMBER", strconv.Itoa(m.TrackNum))
	}
	if m.TrackTotal > 0 {
		addComment(comments, "TRACKTOTAL", strconv.Itoa(m.TrackTotal))
	}
	if m.DiscNum > 0 {
		addComment(comments, "DISCNUMBER", strconv.Itoa(m.DiscNum))
	}
	addComment(comments, "DATE", m.Year)
	addComment(comments, "YANDEX_TRACK_ID", m.TrackID)
	addComment(comments, "COMMENT", m.SourceURL)

	vorbisBlock := comments.Marshal()
	meta := make([]*flac.MetaDataBlock, 0, len(file.Meta)+2)
	for _, block := range file.Meta {
		if block.Type == flac.VorbisComment || block.Type == flac.Picture {
			continue
		}
		meta = append(meta, block)
	}
	meta = append(meta, &vorbisBlock)

	if len(m.Cover) > 0 && strings.HasPrefix(m.CoverMIME, "image/") {
		pic, err := flacpicture.NewFromImageData(flacpicture.PictureTypeFrontCover, "Cover", m.Cover, m.CoverMIME)
		if err != nil {
			return fmt.Errorf("tagger: flac cover: %w", err)
		}
		block := pic.Marshal()
		meta = append(meta, &block)
	}

	file.Meta = meta
	if err := file.Save(path); err != nil {
		return fmt.Errorf("tagger: save flac: %w", err)
	}
	return nil
}

func addComment(c *flacvorbis.MetaDataBlockVorbisComment, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	_ = c.Add(key, value)
}
