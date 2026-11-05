//go:build !windows

package downloader

import "os/exec"

// hideConsoleWindow is a no-op outside Windows: no console window is
// created for a child process there.
func hideConsoleWindow(cmd *exec.Cmd) {}
