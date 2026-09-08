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
