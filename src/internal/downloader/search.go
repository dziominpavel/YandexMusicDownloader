package downloader

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

type searchResponse struct {
	Result struct {
		Tracks struct {
			Results []struct {
				ID        any    `json:"id"`
				Title     string `json:"title"`
				Version   string `json:"version"`
				Available bool   `json:"available"`
				CoverURI  string `json:"coverUri"`
				Artists   []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Albums []struct {
					ID       any    `json:"id"`
					Title    string `json:"title"`
					Year     int    `json:"year"`
					Genre    string `json:"genre"`
					TrackCnt int    `json:"trackCount"`
					Position struct {
						Volume int `json:"volume"`
						Index  int `json:"index"`
					} `json:"trackPosition"`
				} `json:"albums"`
			} `json:"results"`
		} `json:"tracks"`
	} `json:"result"`
}

// SearchTracks asks Yandex Music for tracks matching query. It is used
// by the quality monitor for files that carry no Yandex ID. Only
// available tracks are returned; the caller still compares artist and
// title before offering anything.
func (c *Client) SearchTracks(ctx context.Context, query string) ([]*Track, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	q := url.Values{}
	q.Set("text", query)
	q.Set("type", "track")
	q.Set("page", "0")

	var data searchResponse
	if err := c.doJSON(ctx, c.baseURL()+"/search?"+q.Encode(), &data); err != nil {
		return nil, classifyStatus(err)
	}

	var out []*Track
	for _, r := range data.Result.Tracks.Results {
		if !r.Available {
			continue
		}
		t := &Track{
			ID:        idString(r.ID),
			Title:     r.Title,
			Version:   r.Version,
			Available: r.Available,
			CoverURI:  r.CoverURI,
		}
		for _, a := range r.Artists {
			if name := strings.TrimSpace(a.Name); name != "" {
				t.Artists = append(t.Artists, name)
			}
		}
		if len(r.Albums) > 0 {
			a := r.Albums[0]
			t.Album = a.Title
			t.AlbumID = idString(a.ID)
			if a.Year > 0 {
				t.Year = fmt.Sprintf("%d", a.Year)
			}
			t.Genre = a.Genre
			t.TrackNum = a.Position.Index
			t.TrackTotal = a.TrackCnt
			t.DiscNum = a.Position.Volume
		}
		if t.ID == "" {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}
