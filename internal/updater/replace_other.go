//go:build !windows

package updater

import "fmt"

// ReplaceAndRestart is Windows-only — the self-replace relies on a cmd batch
// to swap the running exe after exit. On other platforms the caller should
// fall back to opening the release page for manual download.
func ReplaceAndRestart(newExePath string) error {
	return fmt.Errorf("self-update is only supported on Windows")
}
