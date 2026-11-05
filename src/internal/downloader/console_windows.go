//go:build windows

package downloader

import (
	"os/exec"
	"syscall"
)

// createNoWindow is CREATE_NO_WINDOW: the child console app gets no console
// at all, which avoids the black window that otherwise flashes for a few
// seconds when ffmpeg runs from the Windows GUI build.
const createNoWindow = 0x08000000

// hideConsoleWindow keeps console tools (ffmpeg.exe) from showing a black
// window. HideWindow covers the STARTUPINFO path, CREATE_NO_WINDOW covers
// console allocation; both are set so no flash appears on any Windows build.
func hideConsoleWindow(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
