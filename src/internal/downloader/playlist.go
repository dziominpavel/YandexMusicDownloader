package downloader

import (
	"fmt"
)

// Playlist is a resolved Yandex Music playlist: title plus tracks in order.
type Playlist struct {
	Title      string
	Tracks     []*Track
	TrackCount int
	// Total mirrors the API trackCount/pager total when present.
	// Used to detect truncated (paginated) responses.
	Total int
}

// playlistResult mirrors GET /playlist/{uuid} (singular, like Stmol).
type playlistResult struct {
	Result struct {
		Title      string `json:"title"`
		TrackCount int    `json:"trackCount"`
		Pager      *struct {
			Total   int `json:"total"`
			Page    int `json:"page"`
			PerPage int `json:"perPage"`
		} `json:"pager"`
		Tracks []struct {
			ID    any `json:"id"`
			Track struct {
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
			} `json:"track"`
		} `json:"tracks"`
	} `json:"result"`
}

// mapPlaylist converts a decoded API answer into a Playlist.
// Entries with an empty inner track keep the outer id and Available=false
// so the batch layer can emit [skip] instead of failing.
func mapPlaylist(data playlistResult) *Playlist {
	p := &Playlist{Title: data.Result.Title, TrackCount: data.Result.TrackCount}
	// Проверено на плейлисте 100+ треков (Universe, trackCount=123):
	// один GET отдает все треки (len == trackCount), а pager.total (193)
	// считает что-то другое и для длины не используется.
	p.Total = data.Result.TrackCount
	for _, e := range data.Result.Tracks {
		t := &Track{ID: fmt.Sprint(e.ID)}
		in := e.Track
		if fmt.Sprint(in.ID) != "" && fmt.Sprint(in.ID) != "<nil>" {
			t.ID = fmt.Sprint(in.ID)
		}
		t.Title = in.Title
		t.Version = in.Version
		t.Available = in.Available
		t.CoverURI = in.CoverURI
		for _, a := range in.Artists {
			t.Artists = append(t.Artists, a.Name)
		}
		if len(in.Albums) > 0 {
			a := in.Albums[0]
			t.Album = a.Title
			t.AlbumID = fmt.Sprint(a.ID)
			if a.Year > 0 {
				t.Year = fmt.Sprintf("%d", a.Year)
			}
			t.Genre = a.Genre
			t.TrackNum = a.Position.Index
			t.TrackTotal = a.TrackCnt
			t.DiscNum = a.Position.Volume
		}
		p.Tracks = append(p.Tracks, t)
	}
	return p
}

// Playlist fetches a playlist by UUID (full ID with optional xx. prefix)
// via GET /playlist/{uuid}. Tracks come embedded in the answer, so no
// per-track requests are needed.
func (c *Client) Playlist(uuid string) (*Playlist, error) {
	if uuid == "" {
		return nil, fmt.Errorf("downloader: empty playlist uuid")
	}
	var data playlistResult
	if err := c.get(apiBase+"/playlist/"+uuid, &data); err != nil {
		return nil, err
	}
	p := mapPlaylist(data)
	if p.Title == "" && len(p.Tracks) == 0 {
		return nil, fmt.Errorf("downloader: playlist %s not found", uuid)
	}
	return p, nil
}

type albumWithTracksResult struct {
	Result struct {
		ID    any    `json:"id"`
		Title string `json:"title"`
		Volumes [][]struct {
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
		} `json:"volumes"`
		TrackCount int `json:"trackCount"`
	} `json:"result"`
}

// Album fetches an album by ID via GET /albums/{id}/with-tracks and returns
// tracks in order (flattening volumes). Mapped into *Playlist for reuse of
// existing playlist download pipeline.
func (c *Client) Album(id string) (*Playlist, error) {
	if id == "" {
		return nil, fmt.Errorf("downloader: empty album id")
	}
	var data albumWithTracksResult
	if err := c.get(apiBase+"/albums/"+id+"/with-tracks", &data); err != nil {
		return nil, err
	}
	p := &Playlist{Title: data.Result.Title, TrackCount: data.Result.TrackCount}
	p.Total = data.Result.TrackCount
	for _, vol := range data.Result.Volumes {
		for _, in := range vol {
			t := &Track{ID: fmt.Sprint(in.ID), Title: in.Title, Version: in.Version, Available: in.Available, CoverURI: in.CoverURI}
			for _, a := range in.Artists {
				t.Artists = append(t.Artists, a.Name)
			}
			if len(in.Albums) > 0 {
				a := in.Albums[0]
				t.Album = a.Title
				if fmt.Sprint(a.ID) != "" {
					t.AlbumID = fmt.Sprint(a.ID)
				} else if fmt.Sprint(data.Result.ID) != "" {
					t.AlbumID = fmt.Sprint(data.Result.ID)
				}
				if a.Year > 0 {
					t.Year = fmt.Sprintf("%d", a.Year)
				}
				t.Genre = a.Genre
				t.TrackNum = a.Position.Index
				if a.TrackCnt > 0 {
					t.TrackTotal = a.TrackCnt
				}
				t.DiscNum = a.Position.Volume
			} else if fmt.Sprint(data.Result.ID) != "" {
				t.AlbumID = fmt.Sprint(data.Result.ID)
				t.Album = data.Result.Title
			}
			p.Tracks = append(p.Tracks, t)
		}
	}
	if p.Title == "" && len(p.Tracks) == 0 {
		return nil, fmt.Errorf("downloader: album %s not found", id)
	}
	if p.TrackCount == 0 {
		p.TrackCount = len(p.Tracks)
		p.Total = len(p.Tracks)
	}
	return p, nil
}
