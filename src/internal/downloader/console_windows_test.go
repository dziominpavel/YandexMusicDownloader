//go:build windows

package downloader

import (
	"os/exec"
	"testing"
)

// Консольный ffmpeg не должен показывать чёрное окно из GUI-сборки:
// проверяем, что запуск идёт со скрытым окном и без нового консольного
// окна (CREATE_NO_WINDOW).
func TestHideConsoleWindow(t *testing.T) {
	cmd := exec.Command("ffmpeg.exe", "-version")
	hideConsoleWindow(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr must be set for a console child")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Error("HideWindow must be true")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Errorf("CreationFlags = %#x, want CREATE_NO_WINDOW bit", cmd.SysProcAttr.CreationFlags)
	}
}
