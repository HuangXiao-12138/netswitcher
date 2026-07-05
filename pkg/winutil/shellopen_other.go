//go:build !windows

package winutil

// ShellOpen is Windows-only.
func ShellOpen(_ string) error { return nil }
