// Package resolver parses Yandex Music URLs into track and playlist references:
// https://music.yandex.ru/album/<albumID>/track/<trackID>
// https://music.yandex.by/playlists/{uuid} (with optional xx. prefix)
package resolver

import (
	"fmt"
	"regexp"
	"strings"
)

// Kind tells what a Ref points to.
type Kind int

const (
	// KindTrack is album/{albumID}/track/{trackID}.
	KindTrack Kind = iota
	// KindPlaylist is playlists/{uuid}.
	KindPlaylist
)

// Ref is a parsed Yandex Music link.
type Ref struct {
	Kind Kind
	// Track links:
	TrackID string
	AlbumID string
	// Playlist links: full ID with optional xx. prefix preserved for the API.
	PlaylistUUID string
}

var trackURL = regexp.MustCompile(`^(?:https?://)?music\.yandex\.(?:ru|com|kz|by|uz)/album/(\d+)/track/(\d+)(?:\?.*)?$`)

var playlistURL = regexp.MustCompile(`^(?:https?://)?music\.yandex\.(?:ru|com|kz|by|uz)/playlists/((?:[a-z]{2}\.)?[0-9a-fA-F-]{36})(?:\?.*)?$`)

// isUUID reports whether s is a canonical 8-4-4-4-12 hex UUID.
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// Parse extracts a track or playlist reference from a Yandex Music URL.
func Parse(input string) (*Ref, error) {
	s := strings.TrimSpace(input)
	if m := trackURL.FindStringSubmatch(s); m != nil {
		return &Ref{Kind: KindTrack, AlbumID: m[1], TrackID: m[2]}, nil
	}
	if m := playlistURL.FindStringSubmatch(s); m != nil {
		raw := m[1]
		uuidPart := raw
		if prefix, rest, found := strings.Cut(raw, "."); found {
			if len(prefix) != 2 {
				return nil, fmt.Errorf("resolver: bad playlist id %q", input)
			}
			uuidPart = rest
		}
		if !isUUID(uuidPart) {
			return nil, fmt.Errorf("resolver: bad playlist uuid %q", input)
		}
		return &Ref{Kind: KindPlaylist, PlaylistUUID: raw}, nil
	}
	return nil, fmt.Errorf("resolver: not a track or playlist link: %q", input)
}
