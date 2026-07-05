//go:build !windows

package winutil

import "os/exec"

// HideWindow is a no-op on non-Windows.
func HideWindow(_ *exec.Cmd) {}
