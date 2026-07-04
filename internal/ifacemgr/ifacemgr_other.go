//go:build !windows

package ifacemgr

import (
	"errors"
	"runtime"
	"time"
)

// Snapshot on non-Windows returns an error; NetSwitcher is Windows-only but
// keeping a stub lets the package compile anywhere for refactoring/tests.
func (m *Manager) Snapshot() (Snapshot, error) {
	return Snapshot{TakenAt: time.Now()}, errors.New("ifacemgr.Snapshot not implemented on " + runtime.GOOS)
}
