//go:build !windows

package winutil

// AttachParentConsole is a no-op on non-Windows.
func AttachParentConsole() error { return nil }
