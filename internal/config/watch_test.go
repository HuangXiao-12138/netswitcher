package config_test

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netswitcher/netswitcher/internal/config"
)

// TestWatcher_FiresOnExternalWrite verifies that an external write to the
// watched config file triggers the callback.
func TestWatcher_FiresOnExternalWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := config.SaveSimple(path, validOffice()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var fires int32
	w, err := config.NewWatcher(path, func() { atomic.AddInt32(&fires, 1) })
	if err != nil {
		t.Fatalf("watcher: %v", err)
	}
	if err := w.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer w.Close()

	// External (non-Save) write: bypass silent suppression.
	if err := overwrite(path); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	if !waitForFires(&fires, 1, time.Second) {
		t.Fatalf("callback did not fire; fires=%d", atomic.LoadInt32(&fires))
	}
}

// TestWatcher_SilentSuppressesOwnWrite verifies that Save() with the watcher
// suppresses the callback for the service's own write (spec §7.2).
func TestWatcher_SilentSuppressesOwnWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := config.SaveSimple(path, validOffice()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var fires int32
	w, err := config.NewWatcher(path, func() { atomic.AddInt32(&fires, 1) })
	if err != nil {
		t.Fatalf("watcher: %v", err)
	}
	if err := w.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer w.Close()

	// Save through the watcher-aware path: should NOT fire.
	c := validOffice()
	c.Profiles[0].Rules = append(c.Profiles[0].Rules, config.Rule{
		ID: "r3", Destination: "10.20.0.0/16", ViaInterface: "以太网", ViaGateway: "auto",
	})
	if err := config.Save(path, c, w); err != nil {
		t.Fatalf("save: %v", err)
	}

	time.Sleep(600 * time.Millisecond) // > debounce window
	if got := atomic.LoadInt32(&fires); got != 0 {
		t.Fatalf("own write should be suppressed; fires=%d", got)
	}
}

// overwrite writes bytes directly, bypassing config.Save (no silent flag).
func overwrite(path string) error {
	c := validOffice()
	c.Profiles[0].Rules[0].Metric = 7
	return config.SaveSimple(path, c)
}

func waitForFires(counter *int32, want int32, max time.Duration) bool {
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(counter) >= want {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return atomic.LoadInt32(counter) >= want
}
