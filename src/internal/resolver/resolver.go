// Package resolver parses Yandex Music URLs into track references.
// v0.1 supports track links only:
// https://music.yandex.ru/album/<albumID>/track/<trackID>
package resolver

import (
	"fmt"
	"regexp"
)

// Ref is a parsed track link.
type Ref struct {
	TrackID string
	AlbumID string
}

var trackURL = regexp.MustCompile(`^(?:https?://)?music\.yandex\.(?:ru|com|kz|by|uz)/album/(\d+)/track/(\d+)(?:\?.*)?$`)

// Parse extracts track and album IDs from a Yandex Music track URL.
// Non-track links fail: albums/playlists arrive in v0.2.
func Parse(input string) (*Ref, error) {
	m := trackURL.FindStringSubmatch(input)
	if m == nil {
		return nil, fmt.Errorf("resolver: not a track link (v0.1 supports tracks only): %q", input)
	}
	return &Ref{AlbumID: m[1], TrackID: m[2]}, nil
}
