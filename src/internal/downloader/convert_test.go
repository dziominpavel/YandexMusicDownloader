package downloader

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindFFmpegAbsent(t *testing.T) {
	t.Setenv("FFMPEG_PATH", "")
	if got := findFFmpeg(); got != "" {
		t.Fatalf("test env must have no ffmpeg, got %q", got)
	}
}

func TestFindFFmpegEnvOverride(t *testing.T) {
	f := filepath.Join(t.TempDir(), "ffmpeg.exe")
	if err := os.WriteFile(f, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FFMPEG_PATH", f)
	if got := findFFmpeg(); got != f {
		t.Fatalf("got %q want %q", got, f)
	}
}

// repoFFmpeg locates the sidecar binary in the repo root for integration tests.
func repoFFmpeg(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "..", "ffmpeg.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("no repo ffmpeg at %s", p)
	}
	return p
}

func audioMD5(t *testing.T, ffmpeg, path string) string {
	t.Helper()
	out, err := exec.Command(ffmpeg, "-v", "error", "-i", path,
		"-map", "0:a:0", "-f", "md5", "-").Output()
	if err != nil {
		t.Fatalf("md5 of %s: %v", path, err)
	}
	return strings.TrimSpace(string(out))
}

func TestAlacToFLACRoundtrip(t *testing.T) {
	ffmpeg := repoFFmpeg(t)
	dir := t.TempDir()

	src, err := filepath.Abs(filepath.Join("..", "tagger", "testdata", "flac.flac"))
	if err != nil {
		t.Fatal(err)
	}
	// FLAC testdata → ALAC m4a (simulates a server flac-mp4 download).
	m4a := filepath.Join(dir, "in.m4a")
	cmd := exec.Command(ffmpeg, "-y", "-v", "error", "-i", src,
		"-map", "0:a:0", "-c:a", "alac", m4a)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make alac: %v: %s", err, out)
	}

	got, err := alacToFLAC(ffmpeg, m4a, dir)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if !strings.HasSuffix(got, ".flac") {
		t.Fatalf("want .flac temp, got %q", got)
	}
	if audioMD5(t, ffmpeg, src) != audioMD5(t, ffmpeg, got) {
		t.Fatal("audio differs after ALAC→FLAC roundtrip")
	}
}

func TestAlacToFLACBadInput(t *testing.T) {
	ffmpeg := repoFFmpeg(t)
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.m4a")
	if err := os.WriteFile(bad, []byte("not audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := alacToFLAC(ffmpeg, bad, dir); err == nil {
		t.Fatal("garbage input must fail")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".flac") {
			t.Fatalf("failed convert left %s behind", e.Name())
		}
	}
}
