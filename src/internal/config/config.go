// Package config reads runtime configuration from a .env file
// located next to the executable (see DefaultPath).
package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DefaultWorkers is the playlist pool size when PLAYLIST_WORKERS is
// missing or invalid. MaxWorkers caps it to bound API pressure.
const (
	DefaultWorkers = 3
	MaxWorkers     = 10

	// Saved window dimensions outside these bounds are read as zero
	// ("size not saved") — see parseWindowDim.
	minWindowWidth  = 320
	maxWindowWidth  = 16384
	minWindowHeight = 240
	maxWindowHeight = 16384
)

// Config holds runtime settings. Token may be empty (preview-only mode).
type Config struct {
	Token     string
	OutputDir string
	// ConvertM4A turns ALAC-in-M4A lossless into plain FLAC via the
	// ffmpeg.exe sidecar. Default true; CONVERT_M4A=false disables.
	ConvertM4A bool
	// EmbedCover embeds cover art while text tags always apply.
	// Default true; EMBED_COVER=false (0/no/off) disables the
	// picture. Garbage is ignored: the flag stays enabled.
	EmbedCover bool
	// WindowWidth and WindowHeight are the last normal window size in
	// pixels, written on close (WINDOW_WIDTH / WINDOW_HEIGHT). Zero
	// means "not saved yet"; malformed or out-of-range values are
	// ignored and read as zero.
	WindowWidth  int
	WindowHeight int
	// Workers caps parallel playlist downloads (PLAYLIST_WORKERS, 1..MaxWorkers).
	Workers int
	// LibraryDir is the user's music collection root (LIBRARY_DIR).
	// Empty = collection indexing and duplicate checks are off.
	LibraryDir string
}

// DefaultPath returns the .env path next to the running executable.
func DefaultPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("config: exe path: %w", err)
	}
	return filepath.Join(filepath.Dir(exe), ".env"), nil
}

// Load reads KEY=VALUE pairs from path. Missing file is not an error
// (empty config = preview-only mode). OUTPUT_DIR defaults to ./downloads,
// CONVERT_M4A and EMBED_COVER default to true.
func Load(path string) (Config, error) {
	cfg := Config{OutputDir: "./downloads", ConvertM4A: true, EmbedCover: true, Workers: DefaultWorkers}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("config: open %s: %w", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch k {
		case "YANDEX_TOKEN":
			cfg.Token = v
		case "OUTPUT_DIR":
			if v != "" {
				cfg.OutputDir = v
			}
		case "CONVERT_M4A":
			switch strings.ToLower(v) {
			case "0", "false", "no", "off":
				cfg.ConvertM4A = false
			}
		case "EMBED_COVER":
			// Default true: only explicit off-values disable the
			// picture, garbage leaves the flag enabled.
			switch strings.ToLower(v) {
			case "0", "false", "no", "off":
				cfg.EmbedCover = false
			}
		case "WINDOW_WIDTH":
			cfg.WindowWidth = parseWindowDim(v, minWindowWidth, maxWindowWidth)
		case "WINDOW_HEIGHT":
			cfg.WindowHeight = parseWindowDim(v, minWindowHeight, maxWindowHeight)
		case "LIBRARY_DIR":
			cfg.LibraryDir = v
		case "PLAYLIST_WORKERS":
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 1 && n <= MaxWorkers {
				cfg.Workers = n
			}
		}
	}
	if err := sc.Err(); err != nil {
		return cfg, fmt.Errorf("config: read %s: %w", path, err)
	}
	return cfg, nil
}

// parseWindowDim parses a saved window dimension. Malformed or
// out-of-range values return 0, i.e. "no saved size" — the caller then
// falls back to the default window size.
func parseWindowDim(s string, min, max int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < min || n > max {
		return 0
	}
	return n
}

// Write saves updates (KEY=VALUE) into the .env file at path without
// rewriting it: existing lines for known keys are replaced in place,
// comments, unknown keys and ordering survive, missing keys are
// appended at the end. Missing file is created. Values are written
// bare; surrounding quotes are stripped by Load anyway.
func Write(path string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	var lines []string
	seen := make(map[string]bool, len(updates))

	if f, err := os.Open(path); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			t := strings.TrimSpace(line)
			if t != "" && !strings.HasPrefix(t, "#") {
				if k, _, ok := strings.Cut(t, "="); ok {
					k = strings.TrimSpace(k)
					if v, has := updates[k]; has {
						lines = append(lines, k+"="+v)
						seen[k] = true
						continue
					}
				}
			}
			lines = append(lines, line)
		}
		scanErr := sc.Err()
		f.Close()
		if scanErr != nil {
			return fmt.Errorf("config: read %s: %w", path, scanErr)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("config: open %s: %w", path, err)
	}

	// Append keys that were absent from the file, in deterministic
	// lexical order.
	for _, k := range sortedKeys(updates) {
		if !seen[k] {
			lines = append(lines, k+"="+updates[k])
		}
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		return fmt.Errorf("config: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("config: replace %s: %w", path, err)
	}
	return nil
}

// sortedKeys returns map keys in lexical order for deterministic output.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// accountStatus is the subset of /account/status we need.
type accountStatus struct {
	Result struct {
		Account struct {
			UID      int64  `json:"uid"`
			Login    string `json:"login"`
			Username string `json:"username"`
		} `json:"account"`
	} `json:"result"`
}

// Validate checks the token against /account/status.
// Returns "login (uid)" on success.
func Validate(token string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, "https://api.music.yandex.net/account/status", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "OAuth "+token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("config: account/status: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("config: account/status: http %d", resp.StatusCode)
	}

	var st accountStatus
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return "", fmt.Errorf("config: decode status: %w", err)
	}
	name := st.Result.Account.Login
	if name == "" {
		name = st.Result.Account.Username
	}
	return fmt.Sprintf("%s (uid %d)", name, st.Result.Account.UID), nil
}
