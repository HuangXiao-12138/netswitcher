// Package winutil holds small Windows-only helpers (elevation check,
// ShellExecute runas for the GUI's "start service" button).
package winutil

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// StartServiceElevated launches `<exe> service ensure` with a UAC prompt via
// the "runas" verb. `service ensure` installs (if absent) AND starts the
// service, so this is a true one-click setup from the GUI banner.
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
	args, _ := windows.UTF16PtrFromString("service ensure")
	if err := windows.ShellExecute(0, verb, file, args, nil, windows.SW_HIDE); err != nil {
		// ERROR_CANCELLED (1223) → user declined the UAC prompt.
		return fmt.Errorf("runas: %w (用户可能取消了提权)", err)
	}
	return nil
}
