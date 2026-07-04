//go:build !windows

package winutil

import "errors"

// StartServiceElevated is Windows-only; NetSwitcher targets Windows.
func StartServiceElevated(_ string) error {
	return errors.New("elevation only supported on Windows")
}
