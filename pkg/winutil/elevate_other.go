//go:build !windows

package winutil

import "errors"

// IsElevated is Windows-only.
func IsElevated() bool { return false }

// StartServiceElevated is Windows-only; NetSwitcher targets Windows.
func StartServiceElevated(_ string) error {
	return errors.New("elevation only supported on Windows")
}
