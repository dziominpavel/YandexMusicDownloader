package tagger

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tommyo123/mtag"
)

const m4aSourceField = "----:com.yandex-music-downloader:source-url"

// writeM4A writes MP4 atoms. Yandex files usually lack the ilst subtree,
// so it is bootstrapped first. Best-effort: callers decide whether a tag
// failure blocks publish (see downloader policies).
func writeM4A(path string, m Meta) error {
	probe, err := mtag.Open(path)
	if err != nil {
		return fmt.Errorf("tagger: open m4a: %w", err)
	}
	if probe.Container() != mtag.ContainerMP4 {
		_ = probe.Close()
		return fmt.Errorf("tagger: unsupported m4a container: %s", probe.Container())
	}
	_ = probe.Close()
	if err := ensureMetaTree(path); err != nil {
		return err
	}

	file, err := mtag.Open(path)
	if err != nil {
		return fmt.Errorf("tagger: reopen m4a: %w", err)
	}
	defer func() { _ = file.Close() }()

	if m.Title != "" {
		file.SetTitle(m.Title)
	}
	if len(m.Artists) > 0 {
		// ©ART is single-valued: join, players split on ", ".
		file.SetArtist(strings.Join(m.Artists, ", "))
		file.SetAlbumArtist(m.Artists[0])
	}
	if m.Album != "" {
		file.SetAlbum(m.Album)
	}
	if m.Genre != "" {
		file.SetGenre(m.Genre)
	}
	if year, err := strconv.Atoi(m.Year); err == nil && year > 0 {
		file.SetYear(year)
	}
	if m.TrackNum > 0 || m.TrackTotal > 0 {
		file.SetTrack(m.TrackNum, m.TrackTotal)
	}
	if m.DiscNum > 0 {
		file.SetDisc(m.DiscNum, 0)
	}
	if m.SourceURL != "" {
		file.SetCustomValues(m4aSourceField, m.SourceURL)
	}
	if len(m.Cover) > 0 && (m.CoverMIME == "image/jpeg" || m.CoverMIME == "image/png") {
		file.AddImage(mtag.Picture{MIME: m.CoverMIME, Type: mtag.PictureCoverFront, Data: m.Cover})
	}

	if err := file.Save(); err != nil {
		return fmt.Errorf("tagger: save m4a: %w", err)
	}
	return nil
}
