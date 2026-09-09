package resolver

import "testing"

func TestParseTrack(t *testing.T) {
	cases := []struct {
		in      string
		trackID string
		albumID string
	}{
		{"https://music.yandex.ru/album/10733135/track/12345678", "12345678", "10733135"},
		{"https://music.yandex.ru/album/10555/track/32988399?utm_source=copy", "32988399", "10555"},
		{"http://music.yandex.kz/album/1/track/2", "2", "1"},
		{"music.yandex.com/album/99/track/100", "100", "99"},
		{"https://music.yandex.by/album/7/track/8?from=share", "8", "7"},
	}
	for _, c := range cases {
		ref, err := Parse(c.in)
		if err != nil {
			t.Errorf("Parse(%q) failed: %v", c.in, err)
			continue
		}
		if ref.TrackID != c.trackID || ref.AlbumID != c.albumID {
			t.Errorf("Parse(%q) = %+v, want track %s album %s", c.in, ref, c.trackID, c.albumID)
		}
	}
}

func TestParseRejectsNonTrack(t *testing.T) {
	for _, in := range []string{
		"https://music.yandex.ru/album/10733135",
		"https://music.yandex.ru/chart",
		"https://example.com/album/1/track/2",
		"not a url",
		"",
	} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) must fail", in)
		}
	}
}

func TestParsePlaylistUUID(t *testing.T) {
	uuid := "4dc94b2f-e96b-2daf-a53c-ce71846901b3"
	cases := []struct{ in, want string }{
		{"https://music.yandex.by/playlists/" + uuid, uuid},
		{"https://music.yandex.ru/playlists/" + uuid + "?utm_source=web&utm_medium=copy_link", uuid},
		{"https://music.yandex.ru/playlists/lk." + uuid, "lk." + uuid},
		{"https://music.yandex.com/playlists/ps." + uuid + "?from=share", "ps." + uuid},
		{"  https://music.yandex.kz/playlists/" + uuid + "  ", uuid},
	}
	for _, c := range cases {
		ref, err := Parse(c.in)
		if err != nil {
			t.Errorf("Parse(%q) failed: %v", c.in, err)
			continue
		}
		if ref.Kind != KindPlaylist || ref.PlaylistUUID != c.want {
			t.Errorf("Parse(%q) = %+v, want playlist %q", c.in, ref, c.want)
		}
	}
}

func TestParsePlaylistRejectsBadUUID(t *testing.T) {
	for _, in := range []string{
		"https://music.yandex.ru/playlists/not-a-uuid",
		"https://music.yandex.ru/playlists/123",
		"https://music.yandex.ru/playlists/p.4dc94b2f-e96b-2daf-a53c-ce71846901b3",
		"https://music.yandex.ru/playlists/lk.------------------------------------",
	} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) must fail", in)
		}
	}
}
