//go:build !windows

package winutil

import "errors"

// IsElevated is Windows-only.
func IsElevated() bool { return false }

// StartServiceElevated is Windows-only; NetSwitcher targets Windows.
func StartServiceElevated(_ string) error {
	return errors.New("elevation only supported on Windows")
}

// RelaunchElevated is Windows-only.
func RelaunchElevated(_ string) error { return errors.New("windows only") }

// RunElevated is Windows-only.
func RunElevated(_, _ string) error { return errors.New("windows only") }

// ShellOpen is Windows-only.
func ShellOpen(_ string) error { return errors.New("windows only") }
