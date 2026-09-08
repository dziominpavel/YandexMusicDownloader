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
	"strings"
	"time"
)

// Config holds runtime settings. Token may be empty (preview-only mode).
type Config struct {
	Token     string
	OutputDir string
	// ConvertM4A turns ALAC-in-M4A lossless into plain FLAC via the
	// ffmpeg.exe sidecar. Default true; CONVERT_M4A=false disables.
	ConvertM4A bool
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
// CONVERT_M4A defaults to true.
func Load(path string) (Config, error) {
	cfg := Config{OutputDir: "./downloads", ConvertM4A: true}

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
		}
	}
	if err := sc.Err(); err != nil {
		return cfg, fmt.Errorf("config: read %s: %w", path, err)
	}
	return cfg, nil
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
