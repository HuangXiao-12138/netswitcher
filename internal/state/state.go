// Package state persists the set of routes NetSwitcher last successfully
// applied. It is the diff baseline for the next Apply (spec §7.4.1, §7.8) and
// what the GUI shows as "current applied state".
//
// Only successfully-applied routes are saved, so a transient add failure
// (e.g. gateway not yet assigned) is retried on the next apply rather than
// recorded as done.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Entry is one managed route. This is the diff key (Destination + IfIndex) and
// the full record needed to re-emit or delete it.
type Entry struct {
	Destination string `json:"destination"` // canonical CIDR, e.g. "168.168.0.0/16"
	Gateway     string `json:"gateway"`
	Interface   string `json:"interface"` // human name, display only
	IfIndex     int    `json:"ifIndex"`
	Metric      int    `json:"metric"`
}

// Snapshot is the timestamped last-applied set.
type Snapshot struct {
	Entries        []Entry   `json:"entries"`
	NrptNamespaces []string  `json:"nrptNamespaces,omitempty"` // domain-suffix NRPT rules currently applied
	At             time.Time `json:"at"`
	Reason         string    `json:"reason,omitempty"`
}

// Store reads and writes state.json atomically.
type Store struct {
	path string
}

// New returns a Store backed at path.
func New(path string) *Store { return &Store{path: path} }

// Path returns the configured state file path.
func (s *Store) Path() string { return s.path }

// Load reads the last-applied entry set. A missing file is treated as empty
// (spec §14: state.json corruption/absence → empty state).
func (s *Store) Load() (Snapshot, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return Snapshot{}, nil
		}
		return Snapshot{}, fmt.Errorf("read state: %w", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		// Corrupt state: start empty rather than failing the whole service
		// (spec §14). Caller logs a warning.
		return Snapshot{}, fmt.Errorf("parse state (will reset): %w", err)
	}
	return snap, nil
}

// Save atomically writes the entry set.
func (s *Store) Save(snap Snapshot) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	if snap.At.IsZero() {
		snap.At = time.Now()
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	data = append(data, '\n')
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write state tmp: %w", err)
	}
	if f, err := os.Open(tmp); err == nil {
		_ = f.Sync()
		_ = f.Close()
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}
