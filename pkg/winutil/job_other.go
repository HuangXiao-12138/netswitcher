//go:build !windows

package winutil

// AssignSelfToKillOnCloseJob is Windows-only.
func AssignSelfToKillOnCloseJob() error { return nil }

// InJob is Windows-only.
func InJob() bool { return false }

// KillChildProcesses is Windows-only.
func KillChildProcesses() {}
