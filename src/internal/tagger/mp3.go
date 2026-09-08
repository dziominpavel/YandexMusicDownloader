package tagger

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bogem/id3v2/v2"
)

const (
	yandexUFIDOwner = "music.yandex.ru"
	sourceURLDesc   = "yandex-music-downloader:source-url"
)

// writeMP3 writes ID3v2.4 tags. Each artist gets its own TPE1 frame.
func writeMP3(path string, m Meta) error {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return fmt.Errorf("tagger: open mp3: %w", err)
	}
	defer tag.Close()

	tag.SetVersion(4)
	tag.SetDefaultEncoding(id3v2.EncodingUTF8)

	if m.Title != "" {
		tag.SetTitle(m.Title)
	}
	if len(m.Artists) > 0 {
		// id3v2 keeps a single frame per ID, so multi-artist is one TPE1
		// with v2.4 null separators — players split it into artists.
		tag.DeleteFrames("TPE1")
		tag.AddTextFrame("TPE1", id3v2.EncodingUTF8, strings.Join(m.Artists, "\x00"))
	}
	if m.Album != "" {
		tag.SetAlbum(m.Album)
	}
	if len(m.Artists) > 0 {
		// TPE2 doubles as Album Artist; no dedicated setter in id3v2 v2.1.4.
		tag.DeleteFrames("TPE2")
		tag.AddTextFrame("TPE2", id3v2.EncodingUTF8, m.Artists[0])
	}
	if m.Genre != "" {
		tag.SetGenre(m.Genre)
	}
	if m.TrackNum > 0 {
		trck := strconv.Itoa(m.TrackNum)
		if m.TrackTotal > 0 {
			trck += "/" + strconv.Itoa(m.TrackTotal)
		}
		tag.DeleteFrames("TRCK")
		tag.AddTextFrame("TRCK", id3v2.EncodingUTF8, trck)
	}
	if m.Year != "" {
		tag.SetYear(m.Year)
	}
	if m.TrackID != "" {
		tag.DeleteFrames(tag.CommonID("Unique file identifier"))
		tag.AddUFIDFrame(id3v2.UFIDFrame{OwnerIdentifier: yandexUFIDOwner, Identifier: []byte(m.TrackID)})
	}
	if m.SourceURL != "" {
		tag.AddCommentFrame(id3v2.CommentFrame{
			Encoding: id3v2.EncodingUTF8, Language: "eng",
			Description: sourceURLDesc, Text: m.SourceURL,
		})
	}
	if len(m.Cover) > 0 && strings.HasPrefix(m.CoverMIME, "image/") {
		tag.DeleteFrames(tag.CommonID("Attached picture"))
		tag.AddAttachedPicture(id3v2.PictureFrame{
			Encoding: id3v2.EncodingUTF8, MimeType: m.CoverMIME,
			PictureType: id3v2.PTFrontCover, Description: "Cover", Picture: m.Cover,
		})
	}
	if err := tag.Save(); err != nil {
		return fmt.Errorf("tagger: save mp3: %w", err)
	}
	return nil
}
