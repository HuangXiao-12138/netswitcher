// Package winutil holds small Windows-only helpers (elevation check,
// ShellExecute runas for the GUI's "start service" button).
package winutil

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// StartServiceElevated launches `<exe> service start` with a UAC prompt via
// the "runas" verb. Returns nil on accepted elevation. ERROR_CANCELLED is
// surfaced as a friendly error when the user clicks No.
func StartServiceElevated(exePath string) error {
	if exePath == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("locate executable: %w", err)
		}
		exePath = exe
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exePath)
	args, _ := windows.UTF16PtrFromString("service start")
	if err := windows.ShellExecute(0, verb, file, args, nil, windows.SW_HIDE); err != nil {
		// ERROR_CANCELLED (1223) → user declined the UAC prompt.
		return fmt.Errorf("runas: %w (用户可能取消了提权)", err)
	}
	return nil
}
