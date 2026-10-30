package downloader

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSignLosslessStable(t *testing.T) {
	a := signLossless(1700000000, "123")
	b := signLossless(1700000000, "123")
	if a != b || a == "" {
		t.Fatalf("sign not stable: %q", a)
	}
	if strings.HasSuffix(a, "=") {
		t.Fatalf("sign must be unpadded: %q", a)
	}
	if signLossless(1700000001, "123") == a || signLossless(1700000000, "124") == a {
		t.Fatal("sign must depend on ts and track id")
	}
}

func TestFileInfoURLParams(t *testing.T) {
	u := fileInfoURL("123", 1700000000)
	for _, want := range []string{"trackId=123", "quality=lossless", "transports=raw", "codecs=", "sign=", "ts=1700000000"} {
		if !strings.Contains(u, want) {
			t.Fatalf("url %q lacks %q", u, want)
		}
	}
}

func TestParseLosslessInfoVariants(t *testing.T) {
	snake := `{"download_info":{"quality":"lossless","codec":"flac","urls":["http://x"],"key":"00","bitrate":941}}`
	camel := `{"downloadInfo":{"quality":"lossless","codec":"flac-mp4","urls":["http://x"],"bitrate":256}}`
	wrapped := `{"result":{"download_info":{"quality":"lossless","codec":"flac","urls":["http://x"]}}}`
	for i, body := range []string{snake, camel, wrapped} {
		info, err := parseLosslessInfo([]byte(body))
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if len(info.URLs) != 1 {
			t.Fatalf("case %d: urls %+v", i, info)
		}
		if info.Quality != "lossless" {
			t.Fatalf("case %d: quality %q", i, info.Quality)
		}
	}
	if _, err := parseLosslessInfo([]byte(`{"result":{}}`)); err == nil {
		t.Fatal("empty payload must fail")
	}
}

func TestLosslessFallbackReason(t *testing.T) {
	noFLAC := &noFLACError{Quality: "nq", Codec: "aac-mp4", Bitrate: 192}
	if !errors.Is(noFLAC, ErrNoFLAC) {
		t.Fatal("noFLACError must unwrap to ErrNoFLAC")
	}
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"server offered aac", noFLAC, "Яндекс вернул nq/aac-mp4 (192 kbps) вместо FLAC"},
		{"no urls", fmt.Errorf("%w: no urls", ErrNoFLAC), "Яндекс не вернул FLAC"},
		{"network", fmt.Errorf("downloader: GET file: http 403"), "ошибка lossless: downloader: GET file: http 403"},
	}
	for _, c := range cases {
		if got := losslessFallbackReason(c.err); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
	if got := losslessFallbackReason(nil); got != "" {
		t.Errorf("nil error: got %q", got)
	}
}

func TestCodecMapping(t *testing.T) {
	if !isFLACCodec("flac") || !isFLACCodec("FLAC-MP4") || isFLACCodec("mp3") {
		t.Fatal("codec filter wrong")
	}
	if losslessExt("flac-mp4") != ".m4a" || losslessExt("flac") != ".flac" {
		t.Fatal("ext mapping wrong")
	}
}

func TestAESCTRRoundtrip(t *testing.T) {
	key := make([]byte, 16)
	for i := range key {
		key[i] = byte(i)
	}
	plain := []byte("fLaC-test-payload-1234567890")
	block, _ := aes.NewCipher(key)
	enc := make([]byte, len(plain))
	cipher.NewCTR(block, make([]byte, aes.BlockSize)).XORKeyStream(enc, plain)

	// decrypt path mirrors streamLosslessURL: StreamReader over CTR
	dec := make([]byte, len(enc))
	stream := cipher.NewCTR(block, make([]byte, aes.BlockSize))
	r := &cipher.StreamReader{S: stream, R: bytes.NewReader(enc)}
	n := 0
	for n < len(enc) {
		m, err := r.Read(dec[n:])
		n += m
		if err != nil {
			break
		}
	}
	if string(dec) != string(plain) {
		t.Fatalf("roundtrip failed: %q", dec)
	}
}
