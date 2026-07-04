package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Save writes the config atomically: serialize, write to a sibling .tmp file,
// fsync, then rename over the target. Validation runs first; on any failure
// the on-disk file is untouched (§6.4.6).
//
// If watcher is non-nil, Save marks it silent around the write so the file
// watcher does not feed our own write back to core as a "config changed"
// event (§7.2 silent suppression).
func Save(path string, c *Config, watcher *Watcher) error {
	if c == nil {
		return fmt.Errorf("save: config is nil")
	}
	if verrs := Validate(c); len(verrs) > 0 {
		return verrs
	}

	// Always serialize a canonical, defaults-applied document.
	c.applyDefaults()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write tmp config: %w", err)
	}
	// fsync the tmp file before rename so a crash never leaves a half-written
	// file in place of the target.
	if f, err := os.Open(tmp); err == nil {
		_ = f.Sync()
		_ = f.Close()
	}

	if watcher != nil {
		// Arm suppression BEFORE the rename so the async fsnotify event is
		// absorbed. Holds longer than fsnotify delivery + watcher debounce.
		watcher.SuppressFor()
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename tmp config: %w", err)
	}
	c.loadedPath = path
	return nil
}

// SaveSimple is Save without watcher suppression, for one-off CLI writes.
func SaveSimple(path string, c *Config) error { return Save(path, c, nil) }
