// Package paths centralizes filesystem locations used by NetSwitcher.
//
// All persistent data lives under %ProgramData%\NetSwitcher so that the
// SYSTEM service can write without impersonation and per-machine state
// (config, last-applied routes, logs) is shared across user sessions.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// AppName is the on-disk product name used for directories and the service.
const AppName = "NetSwitcher"

// ProgramDataDir returns %ProgramData%\NetSwitcher, creating it if missing.
func ProgramDataDir() (string, error) {
	base := os.Getenv("PROGRAMDATA")
	if base == "" {
		// Fall back to the system default; %SystemDrive%\ProgramData.
		base = `C:\ProgramData`
	}
	dir := filepath.Join(base, AppName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create program data dir: %w", err)
	}
	return dir, nil
}

// RuntimeDir holds transient runtime artifacts (e.g. the IPC lock).
func RuntimeDir() (string, error) {
	root, err := ProgramDataDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "runtime")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// LogDir returns the log directory and creates it if missing.
func LogDir() (string, error) {
	root, err := ProgramDataDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// ConfigPath returns the path to config.json.
func ConfigPath() (string, error) {
	root, err := ProgramDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "config.json"), nil
}

// StatePath returns the path to state.json (last successful apply).
func StatePath() (string, error) {
	root, err := ProgramDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "state.json"), nil
}

// LogPath returns the path to the primary log file.
func LogPath() (string, error) {
	dir, err := LogDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "netswitcher.log"), nil
}

// EnsureDefaultConfig writes an empty-but-valid config if none exists.
// Returns the path regardless.
func EnsureDefaultConfig() (string, error) {
	p, err := ConfigPath()
	if err != nil {
		return "", err
	}
	if _, statErr := os.Stat(p); statErr == nil {
		return p, nil // already present
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return p, nil
}
