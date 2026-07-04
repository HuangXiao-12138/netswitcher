package core

import (
	"sync"
	"time"
)

// Debouncer collapses a burst of Call invocations into one execution of the
// latest callback after a quiet period (spec §16.2). Wi-Fi reconnects flap
// several events within ~hundreds of ms; we want one apply at the end.
type Debouncer struct {
	d      time.Duration
	mu     sync.Mutex
	timer  *time.Timer
	target func()
}

// NewDebouncer constructs a Debouncer with quiet period d.
func NewDebouncer(d time.Duration) *Debouncer {
	return &Debouncer{d: d}
}

// Call schedules f to run after d, replacing any pending call. The most
// recent f wins.
func (db *Debouncer) Call(f func()) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.timer != nil {
		db.timer.Stop()
	}
	db.target = f
	db.timer = time.AfterFunc(db.d, func() {
		db.mu.Lock()
		t := db.target
		db.target = nil
		db.timer = nil
		db.mu.Unlock()
		if t != nil {
			t()
		}
	})
}

// Flush executes any pending call immediately and cancels the timer. Used on
// graceful shutdown so a final pending apply is not lost.
func (db *Debouncer) Flush() {
	db.mu.Lock()
	t := db.target
	db.target = nil
	if db.timer != nil {
		db.timer.Stop()
		db.timer = nil
	}
	db.mu.Unlock()
	if t != nil {
		t()
	}
}

// Stop cancels the pending call without executing it.
func (db *Debouncer) Stop() {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.timer != nil {
		db.timer.Stop()
		db.timer = nil
	}
	db.target = nil
}
