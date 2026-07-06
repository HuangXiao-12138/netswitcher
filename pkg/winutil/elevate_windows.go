// Package winutil holds small Windows-only helpers (elevation check,
// ShellExecute runas for the GUI's "start service" button).
package winutil

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// IsElevated reports whether the current process is running with an elevated
// (administrator) token. Used to decide whether the embedded route engine can
// run route.exe/netsh directly (it can't without elevation).
func IsElevated() bool {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}

// StartServiceElevated launches `<exe> service ensure` with a UAC prompt via
// the "runas" verb. `service ensure` installs (if absent) AND starts the
// service, so this is a true one-click setup from the GUI banner.
func StartServiceElevated(exePath string) error {
	return relaunchRunas(exePath, "service ensure", windows.SW_HIDE)
}

// RelaunchElevated launches `<exe> --takeover` (GUI mode) with a UAC prompt.
// Used when the GUI was launched non-elevated and needs admin to modify routes.
// The --takeover flag makes the new instance wait for this one to release the
// single-instance mutex instead of mistaking itself for a second instance and
// exiting — without it, both instances end up dead (relaunch does nothing).
func RelaunchElevated(exePath string) error {
	// SW_SHOWNORMAL (not SW_HIDE): the new instance is a GUI window the user
	// expects to see. With SW_HIDE the elevated restarts invisibly and looks
	// like "重启没生效" — the process is actually running (engine online) but
	// its window never appears.
	return relaunchRunas(exePath, "--takeover", windows.SW_SHOWNORMAL)
}

// RunElevated runs an arbitrary exe with args under a UAC prompt (runas verb).
func RunElevated(exePath, args string) error {
	return relaunchRunas(exePath, args, windows.SW_HIDE)
}

// ShellOpen opens a file or folder with its default handler via the shell
// "open" verb (Explorer for folders, the associated app for files). More
// reliable than exec.Command("explorer.exe", path) — explorer is a GUI
// process that opens folders by IPC-ing the running shell, which
// CREATE_NO_WINDOW/detached spawns can break silently.
func ShellOpen(path string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	file, _ := windows.UTF16PtrFromString(path)
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}

func relaunchRunas(exePath, args string, showCmd int32) error {
	if exePath == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("locate executable: %w", err)
		}
		exePath = exe
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exePath)
	var argsPtr *uint16
	if args != "" {
		argsPtr, _ = windows.UTF16PtrFromString(args)
	}
	if err := windows.ShellExecute(0, verb, file, argsPtr, nil, showCmd); err != nil {
		return fmt.Errorf("runas: %w (用户可能取消了提权)", err)
	}
	return nil
}
