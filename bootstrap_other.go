//go:build !windows

package gui

// EnsureWebView2 is a no-op on non-Windows.
func EnsureWebView2(_ bool) error { return nil }
