//go:build windows

package winutil

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW prevents Windows from allocating a console window for a
// console-subsystem child (route.exe, netsh.exe, powershell.exe, …). Without
// it, a GUI-subsystem parent flashes a black console each time it spawns one.
const CREATE_NO_WINDOW = 0x08000000

// HideWindow marks cmd's child process as hidden + no-console. Call before
// Run/Start. Safe to call when cmd.SysProcAttr is already set.
func HideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= CREATE_NO_WINDOW
}
