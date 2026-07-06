//go:build !windows

package nrpt

import "fmt"

// PowerShellSetter is Windows-only on non-Windows builds.
type PowerShellSetter struct {
	DryRun bool
}

func (s *PowerShellSetter) Add(_ string, _ []string) error {
	return fmt.Errorf("nrpt: Windows only")
}

func (s *PowerShellSetter) Remove(_ string) error {
	return fmt.Errorf("nrpt: Windows only")
}

func (s *PowerShellSetter) Sync(_ []Rule, _ []string) error {
	return fmt.Errorf("nrpt: Windows only")
}
