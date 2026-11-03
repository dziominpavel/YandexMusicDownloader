package downloader

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func accountJSON(plus bool) string {
	if plus {
		return `{"result":{"account":{"uid":42},"plus":{"hasPlus":true}}}`
	}
	return `{"result":{"account":{"uid":42},"plus":{"hasPlus":false}}}`
}

func qualityServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/account/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(accountJSON(true)))
		case "/get-file-info":
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestProbeLosslessClassification(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		wantOK    bool
		wantCodec string
		wantErr   error
	}{
		{
			name: "flac", status: 200,
			body:      `{"result":{"downloadInfo":{"quality":"lossless","codec":"flac","bitrate":0}}}`,
			wantOK:    true,
			wantCodec: "flac",
		},
		{
			name: "flac-mp4", status: 200,
			body:      `{"result":{"downloadInfo":{"quality":"lossless","codec":"flac-mp4","bitrate":0}}}`,
			wantOK:    true,
			wantCodec: "flac-mp4",
		},
		{
			name: "mp3", status: 200,
			body:      `{"result":{"downloadInfo":{"quality":"lossless","codec":"mp3","bitrate":320}}}`,
			wantCodec: "mp3",
		},
		{
			name: "aac", status: 200,
			body:      `{"result":{"downloadInfo":{"quality":"lossless","codec":"aac-mp4","bitrate":192}}}`,
			wantCodec: "aac-mp4",
		},
		{
			name: "preview", status: 200,
			body:    `{"result":{"downloadInfo":{"quality":"smart_preview","codec":"mp3","bitrate":192}}}`,
			wantErr: ErrPreview,
		},
		{name: "auth", status: 401, wantErr: ErrAuth},
		{name: "forbidden", status: 403, wantErr: ErrForbidden},
		{name: "unavailable", status: 404, wantErr: ErrUnavailable},
		{name: "server error", status: 500, wantErr: ErrCheckFailed},
		{
			name: "empty info", status: 200,
			body:    `{"result":{}}`,
			wantErr: ErrCheckFailed,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := qualityServer(t, c.status, c.body)
			defer srv.Close()
			client := NewClient("t")
			client.BaseURL = srv.URL
			got, err := client.ProbeLossless(context.Background(), "123")
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("want %v, got %v", c.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Available != c.wantOK || got.Codec != c.wantCodec {
				t.Fatalf("got %+v, want available=%v codec=%q", got, c.wantOK, c.wantCodec)
			}
		})
	}
}

func TestProbeRetriesTransientOnly(t *testing.T) {
	var fileInfoCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/account/status":
			_, _ = w.Write([]byte(accountJSON(true)))
		case "/get-file-info":
			fileInfoCalls++
			if fileInfoCalls == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`{"result":{"downloadInfo":{"quality":"lossless","codec":"flac"}}}`))
		}
	}))
	defer srv.Close()
	client := NewClient("t")
	client.BaseURL = srv.URL
	if _, err := client.ProbeLossless(context.Background(), "1"); err != nil {
		t.Fatal(err)
	}
	if fileInfoCalls != 2 {
		t.Fatalf("transient 500 must be retried once, got %d calls", fileInfoCalls)
	}

	// A business 404 must not be retried.
	fileInfoCalls = 0
	srv404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/account/status":
			_, _ = w.Write([]byte(accountJSON(true)))
		case "/get-file-info":
			fileInfoCalls++
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv404.Close()
	client = NewClient("t")
	client.BaseURL = srv404.URL
	if _, err := client.ProbeLossless(context.Background(), "1"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
	if fileInfoCalls != 1 {
		t.Fatalf("404 must not be retried, got %d calls", fileInfoCalls)
	}
}

func TestProbeLosslessNetworkFailure(t *testing.T) {
	srv := qualityServer(t, 200, `{}`)
	srv.Close() // closed port → retries → ErrCheckFailed
	client := NewClient("t")
	client.BaseURL = srv.URL
	if _, err := client.ProbeLossless(context.Background(), "1"); !errors.Is(err, ErrCheckFailed) {
		t.Fatalf("want ErrCheckFailed, got %v", err)
	}
}

func TestProbeLosslessUsesCachedAccount(t *testing.T) {
	var accountCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/account/status":
			accountCalls++
			_, _ = w.Write([]byte(accountJSON(true)))
		case "/get-file-info":
			_, _ = w.Write([]byte(`{"result":{"downloadInfo":{"quality":"lossless","codec":"flac"}}}`))
		}
	}))
	defer srv.Close()
	client := NewClient("t")
	client.BaseURL = srv.URL
	for i := 0; i < 3; i++ {
		if _, err := client.ProbeLossless(context.Background(), "123"); err != nil {
			t.Fatal(err)
		}
	}
	if accountCalls != 1 {
		t.Fatalf("account/status must be cached, got %d calls", accountCalls)
	}
}

func TestSearchTracks(t *testing.T) {
	const body = `{"result":{"tracks":{"results":[
		{"id":42,"title":"Song","available":true,"coverUri":"avatars.yandex.net/x/%%",
		 "artists":[{"name":"Artist A"},{"name":"Artist B"}],
		 "albums":[{"id":7,"title":"Album","year":2020,"genre":"rock","trackCount":10,
		            "trackPosition":{"volume":1,"index":2}}]},
		{"id":43,"title":"Hidden","available":false,"artists":[{"name":"Artist A"}]}
	]}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("text"); got != "Artist A Song" {
			t.Errorf("search text = %q", got)
		}
		if got := r.URL.Query().Get("type"); got != "track" {
			t.Errorf("search type = %q", got)
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	client := NewClient("t")
	client.BaseURL = srv.URL
	tracks, err := client.SearchTracks(context.Background(), "Artist A Song")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 {
		t.Fatalf("unavailable track must be filtered, got %d", len(tracks))
	}
	tr := tracks[0]
	if tr.ID != "42" || tr.Title != "Song" || tr.Album != "Album" || tr.AlbumID != "7" ||
		tr.Year != "2020" || tr.TrackNum != 2 || tr.TrackTotal != 10 ||
		strings.Join(tr.Artists, ", ") != "Artist A, Artist B" {
		t.Fatalf("unexpected track: %+v", tr)
	}
}

func TestSearchTracksEmptyAndErrors(t *testing.T) {
	client := NewClient("t")
	client.BaseURL = "http://unused"
	if tracks, err := client.SearchTracks(context.Background(), "  "); err != nil || tracks != nil {
		t.Fatalf("empty query must be a no-op, got %v %v", tracks, err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	client.BaseURL = srv.URL
	if _, err := client.SearchTracks(context.Background(), "x"); !errors.Is(err, ErrAuth) {
		t.Fatalf("want ErrAuth, got %v", err)
	}
}
