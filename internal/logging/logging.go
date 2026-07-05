// Package logging configures structured logging (log/slog) with multi-sink
// output: stdout (for foreground/debug runs) and a rotating log file under
// %ProgramData%\NetSwitcher\logs.
//
// Per the spec (§7.10, §17.8) this is the first module implemented so every
// other module can record diagnostics from the start. The named-pipe sink
// for live GUI tailing is wired up in Phase 5 (IPC) via SetPipeSink.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RotatingFileWriter is a minimal size-based + age-based rotator sufficient
// for NetSwitcher's volume (apply events, network changes, IPC requests).
// One file per day; if a file exceeds MaxBytes a numeric suffix is appended.
type RotatingFileWriter struct {
	mu       sync.Mutex
	dir      string
	prefix   string // e.g. "netswitcher"
	ext      string // e.g. ".log"
	MaxBytes int64
	KeepDays int

	f *os.File
}

// NewRotatingFileWriter creates a writer that writes to <dir>/<prefix><ext>.
func NewRotatingFileWriter(dir, prefix, ext string) (*RotatingFileWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	w := &RotatingFileWriter{
		dir:      dir,
		prefix:   prefix,
		ext:      ext,
		MaxBytes: 50 * 1024 * 1024, // 50 MB
		KeepDays: 7,
	}
	if err := w.openCurrent(); err != nil {
		return nil, err
	}
	w.pruneOld()
	return w, nil
}

func (w *RotatingFileWriter) currentPath() string {
	return filepath.Join(w.dir, w.prefix+w.ext)
}

func (w *RotatingFileWriter) openCurrent() error {
	f, err := os.OpenFile(w.currentPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w.f = f
	return nil
}

// Write implements io.Writer with size-based rotation.
func (w *RotatingFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		if err := w.openCurrent(); err != nil {
			return 0, err
		}
	}
	if fi, err := w.f.Stat(); err == nil && fi.Size()+int64(len(p)) > w.MaxBytes {
		// rotate: close, rename with timestamp, reopen.
		_ = w.f.Close()
		w.f = nil
		stamp := time.Now().Format("20060102-150405")
		_ = os.Rename(w.currentPath(), filepath.Join(w.dir, fmt.Sprintf("%s-%s%s", w.prefix, stamp, w.ext)))
		if err := w.openCurrent(); err != nil {
			return 0, err
		}
	}
	return w.f.Write(p)
}

// pruneOld removes rotated files older than KeepDays.
func (w *RotatingFileWriter) pruneOld() {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -w.KeepDays)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(w.dir, e.Name()))
		}
	}
}

// Close releases the underlying file.
func (w *RotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f != nil {
		err := w.f.Close()
		w.f = nil
		return err
	}
	return nil
}

// pipeSink is the live named-pipe writer used by the IPC layer to push log
// lines to subscribed GUI clients. It is set via SetPipeSink in Phase 5.
type pipeSink struct {
	mu sync.Mutex
	w  io.Writer
}

var globalPipeSink pipeSink

// SetPipeSink installs (or removes, when nil) the live log subscriber sink.
// After this is called, every emitted log record is also written to w.
func SetPipeSink(w io.Writer) {
	globalPipeSink.mu.Lock()
	defer globalPipeSink.mu.Unlock()
	globalPipeSink.w = w
}

// LevelFromString parses slog level names; defaults to Info.
func LevelFromString(s string) slog.Level {
	switch s {
	case "debug", "DEBUG":
		return slog.LevelDebug
	case "info", "INFO":
		return slog.LevelInfo
	case "warn", "warning", "WARN", "WARNING":
		return slog.LevelWarn
	case "error", "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Configure sets up slog with JSON output to file (rotating) and, if the
// process has an interactive stdout (a real console), stdout too. GUI-
// subsystem builds (-H windowsgui) have no console attached when double-
// clicked; in that case stdout is skipped so we never write to an invalid
// handle, and logging goes to the file + the pipe sink only.
//
// If logDir is empty, only stdout (when available) is used.
// Returns a cleanup function the caller defers.
func Configure(level string, logDir string) (cleanup func(), err error) {
	var sinks []io.Writer
	if stdoutIsConsole() {
		sinks = append(sinks, os.Stdout)
	}

	cleanup = func() {}

	if logDir != "" {
		rf, ferr := NewRotatingFileWriter(logDir, "netswitcher", ".log")
		if ferr != nil {
			return cleanup, fmt.Errorf("open log file: %w", ferr)
		}
		sinks = append(sinks, rf)
		cleanup = func() { _ = rf.Close() }
	}

	// Add the live pipe sink if one is registered.
	multi := io.MultiWriter(sinks...)
	tee := &teeWriter{base: multi}

	logger := slog.New(slog.NewJSONHandler(tee, &slog.HandlerOptions{
		Level: LevelFromString(level),
	}))
	slog.SetDefault(logger)
	return cleanup, nil
}

// stdoutIsConsole reports whether os.Stdout is a real terminal/console. On
// Windows GUI-subsystem binaries launched by explorer, os.Stdout is an
// invalid handle and Stat() returns an error or a non-char-device mode.
func stdoutIsConsole() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// teeWriter forwards bytes to the base writer and, if a pipe sink is
// registered, to it as well. The pipe sink is best-effort: write errors are
// ignored so a disconnected GUI never breaks logging.
type teeWriter struct {
	base io.Writer
}

func (t *teeWriter) Write(p []byte) (int, error) {
	n, err := t.base.Write(p)
	globalPipeSink.mu.Lock()
	w := globalPipeSink.w
	globalPipeSink.mu.Unlock()
	if w != nil {
		_, _ = w.Write(p)
	}
	return n, err
}
