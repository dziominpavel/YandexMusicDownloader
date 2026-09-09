package downloader

import (
	"bytes"
	"io"
	"testing"
	"time"
)

func TestPercentOf(t *testing.T) {
	if got := percentOf(0, -1); got != -1 {
		t.Fatalf("unknown total = %d, want -1", got)
	}
	if got := percentOf(0, 0); got != -1 {
		t.Fatalf("zero total = %d, want -1", got)
	}
	if got := percentOf(50, 100); got != 50 {
		t.Fatalf("got %d want 50", got)
	}
	if got := percentOf(200, 100); got != 100 {
		t.Fatalf("overflow clamps to 100, got %d", got)
	}
}

func TestProgressReaderCountsAndFlushes(t *testing.T) {
	data := bytes.Repeat([]byte("a"), 1000)
	var calls [][2]int64
	r := newProgressReader(bytes.NewReader(data), 1000, func(done, total int64) {
		calls = append(calls, [2]int64{done, total})
	})
	// No throttle in test: every read reports.
	r.interval = 0
	now := time.Now()
	r.now = func() time.Time { now = now.Add(time.Hour); return now }
	buf := make([]byte, 100)
	for {
		_, err := r.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if r.done != 1000 {
		t.Fatalf("done = %d, want 1000", r.done)
	}
	if len(calls) == 0 || calls[len(calls)-1][0] != 1000 {
		t.Fatalf("final flush missing: %+v", calls)
	}
}

func TestProgressReaderUnknownTotal(t *testing.T) {
	r := newProgressReader(bytes.NewReader([]byte("hello")), -1, nil)
	r.interval = 0
	if _, err := io.ReadAll(r); err != nil {
		t.Fatal(err)
	}
	if got := percentOf(r.done, -1); got != -1 {
		t.Fatalf("percent = %d, want -1", got)
	}
}
