package downloader

import (
	"encoding/json"
	"testing"
)

func TestMapPlaylist(t *testing.T) {
	body := `{"result":{"title":"Мой плейлист","trackCount":3,"tracks":[
		{"id":"11","track":{"id":11,"title":"T1","artists":[{"name":"A1"}],"available":true,"coverUri":"cover1","albums":[{"id":100,"title":"Alb","year":2020,"genre":"rock","trackCount":10,"trackPosition":{"volume":1,"index":2}}]}},
		{"id":"22","track":{"id":22,"title":"T2","version":"live","artists":[{"name":"A2"},{"name":"A3"}],"available":false,"albums":[]}},
		{"id":"33","track":{}}
	]}}`
	var data playlistResult
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		t.Fatal(err)
	}
	p := mapPlaylist(data)
	if p.Title != "Мой плейлист" {
		t.Fatalf("title = %q", p.Title)
	}
	if len(p.Tracks) != 3 {
		t.Fatalf("tracks = %d", len(p.Tracks))
	}
	t1 := p.Tracks[0]
	if t1.ID != "11" || t1.Title != "T1" || !t1.Available || len(t1.Artists) != 1 || t1.Album != "Alb" || t1.TrackNum != 2 || t1.TrackTotal != 10 {
		t.Fatalf("t1 = %+v", t1)
	}
	if p.Tracks[1].Available {
		t.Fatalf("t2 must be unavailable: %+v", p.Tracks[1])
	}
	if len(p.Tracks[1].Artists) != 2 {
		t.Fatalf("t2 artists = %+v", p.Tracks[1].Artists)
	}
	if p.Tracks[2].ID != "33" || p.Tracks[2].Available {
		t.Fatalf("empty inner track must keep outer id as unavailable: %+v", p.Tracks[2])
	}
}

func TestMapPlaylistEmpty(t *testing.T) {
	var data playlistResult
	p := mapPlaylist(data)
	if len(p.Tracks) != 0 {
		t.Fatalf("want no tracks, got %+v", p)
	}
}
