//go:build !windows

package winutil

// AssignSelfToKillOnCloseJob is Windows-only.
func AssignSelfToKillOnCloseJob() error { return nil }
