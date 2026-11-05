package downloader

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Отменённый контекст задачи помечает трек [stopped] и не трогает
// состояние остановки клиента (его использует legacy-путь).
func TestDownloadTrackCtxCancelledBeforeStart(t *testing.T) {
	c := NewClient("")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var got []Event
	_, err := c.DownloadTrackCtx(
		ctx,
		&Track{ID: "1", Artists: []string{"A"}, Title: "T", Available: true},
		t.TempDir(),
		func(e Event) { got = append(got, e) },
		nil,
	)
	if !errors.Is(err, ErrStopped) {
		t.Fatalf("err = %v, want ErrStopped", err)
	}
	if len(got) != 1 || got[0].Kind != KindStopped {
		t.Fatalf("want a single KindStopped event, got %+v", got)
	}
	if c.Stopped() {
		t.Fatal("a cancelled task must not mark the whole client stopped")
	}
}

// Отмена одной задачи не мешает соседней: у каждой свой контекст.
func TestTaskContextDoesNotStopNeighbor(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			close(started)
			<-r.Context().Done()
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient("")
	dir := t.TempDir()

	slowCtx, cancelSlow := context.WithCancel(context.Background())
	slowDone := make(chan error, 1)
	go func() {
		_, err := c.downloadToTempCtx(slowCtx, srv.URL+"/slow", dir, ".mp3", "a", "A", nil)
		slowDone <- err
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("slow request did not start")
	}
	cancelSlow()
	if err := <-slowDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled task err = %v, want context.Canceled", err)
	}

	if _, err := c.downloadToTempCtx(context.Background(), srv.URL+"/fast", dir, ".mp3", "b", "B", nil); err != nil {
		t.Fatalf("neighbour task must complete, got %v", err)
	}
}
