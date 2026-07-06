//go:build !windows

package winutil

// AcquireSingleton is Windows-only; on other platforms allow always.
func AcquireSingleton() (bool, error) { return true, nil }

// SignalSingletonShow is Windows-only.
func SignalSingletonShow() error { return nil }

// ReleaseSingleton is Windows-only.
func ReleaseSingleton() {}

// WaitSingletonShow returns a never-firing channel on non-Windows.
func WaitSingletonShow() <-chan struct{} { return make(chan struct{}) }
