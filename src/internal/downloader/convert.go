package downloader

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ffmpegFile is the sidecar converter binary, placed next to the app exe.
const ffmpegFile = "ffmpeg.exe"

// findFFmpeg locates the ALAC→FLAC converter: FFMPEG_PATH env first,
// then ffmpeg.exe next to the running exe. Empty string = unavailable,
// M4A downloads stay M4A.
func findFFmpeg() string {
	if p := strings.TrimSpace(os.Getenv("FFMPEG_PATH")); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	p := filepath.Join(filepath.Dir(exe), ffmpegFile)
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	return ""
}

// alacToFLAC decodes an ALAC-in-MP4 temp file and encodes plain FLAC
// (lossless→lossless, bit-identical audio) into a new temp file in dir.
// Metadata is NOT carried over: the caller tags the result via runTagHook.
// The source temp file is left for the caller to remove.
func alacToFLAC(ffmpeg, src, dir string) (string, error) {
	dstFile, err := os.CreateTemp(dir, ".yamdl-*.flac")
	if err != nil {
		return "", fmt.Errorf("downloader: convert temp: %w", err)
	}
	dst := dstFile.Name()
	_ = dstFile.Close()
	cmd := exec.Command(ffmpeg, "-y", "-v", "error", "-i", src,
		"-map", "0:a:0", "-c:a", "flac", dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(dst)
		return "", fmt.Errorf("downloader: ffmpeg alac→flac: %v: %s",
			err, strings.TrimSpace(string(out)))
	}
	return dst, nil
}
