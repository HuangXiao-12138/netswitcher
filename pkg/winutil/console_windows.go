//go:build windows

package winutil

import (
	"os"

	"golang.org/x/sys/windows"
)

var (
	kernel32          = windows.NewLazySystemDLL("kernel32.dll")
	procAttachConsole = kernel32.NewProc("AttachConsole")
)

// ATTACH_PARENT_PROCESS == (DWORD)-1 == 0xFFFFFFFF.
const attachParentProcess = ^uint32(0)

// AttachParentConsole attaches this process to its parent's console so that
// stdout/stderr reach the terminal that launched us. Used by CLI subcommands
// (dump/apply/ipc/--help) when the binary is built GUI-subsystem (-H
// windowsgui), which prevents double-click from spawning a black console.
//
// Returns nil if there is no parent console to attach (e.g. explorer.exe
// double-click) — in that case the GUI runs without a console, which is fine.
func AttachParentConsole() error {
	r, _, err := procAttachConsole.Call(uintptr(attachParentProcess))
	if r == 0 {
		return err
	}
	// Re-point os.Stdout / os.Stderr at the attached console.
	if conout, err := os.OpenFile("CONOUT$", os.O_RDWR, 0); err == nil {
		os.Stdout = conout
		os.Stderr = conout
	}
	return nil
}
