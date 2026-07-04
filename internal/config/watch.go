package config

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher fires a callback when the config file changes on disk. It watches
// the parent directory (file-level watches break across atomic rename) and
// filters to the target file.
//
// The service's own Save() writes call SuppressFor so we don't feed our own
// write back to the core as a "config changed" event (spec §7.2). The
// suppression holds for a short window after the write, because fsnotify
// delivers the rename/write event asynchronously — possibly after Save has
// already returned.
type Watcher struct {
	path    string
	dir     string
	fw      *fsnotify.Watcher
	cb      func()
	silent  atomic.Bool
	stop    chan struct{}
	stopped atomic.Bool

	// debounce window: a single editor save often produces CREATE+WRITE+...
	// we collapse bursts into one callback.
	debounce time.Duration
	mu       sync.Mutex
	timer    *time.Timer

	// silent-clear timer: cleared by SuppressFor after the suppression window
	// so async fsnotify events from our own write are absorbed.
	clearMu     sync.Mutex
	clearTimer  *time.Timer
	clearWindow time.Duration
}

// NewWatcher constructs a watcher for path. Call Start to begin.
func NewWatcher(path string, cb func()) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		path:        filepath.Clean(path),
		dir:         filepath.Clean(filepath.Dir(path)),
		fw:          fw,
		cb:          cb,
		debounce:    400 * time.Millisecond,
		clearWindow: 800 * time.Millisecond,
		stop:        make(chan struct{}),
	}, nil
}

// Start begins watching. It is safe to call once.
func (w *Watcher) Start() error {
	if err := w.fw.Add(w.dir); err != nil {
		return err
	}
	go w.loop()
	return nil
}

// Close stops watching and releases the fsnotify watcher.
func (w *Watcher) Close() error {
	if w.stopped.CompareAndSwap(false, true) {
		close(w.stop)
	}
	return w.fw.Close()
}

// SuppressFor arms a time window during which on-disk writes are ignored,
// so the service's own Save (which uses atomic rename) does not feed back
// through the watcher as a "config changed" event. The window is longer than
// fsnotify's typical delivery latency and the debounce window combined.
func (w *Watcher) SuppressFor() {
	w.silent.Store(true)
	w.clearMu.Lock()
	defer w.clearMu.Unlock()
	if w.clearTimer != nil {
		w.clearTimer.Stop()
	}
	w.clearTimer = time.AfterFunc(w.clearWindow, func() { w.silent.Store(false) })
}

// IsSilent reports whether writes from this process are currently being
// suppressed. Used in tests.
func (w *Watcher) IsSilent() bool { return w.silent.Load() }

func (w *Watcher) loop() {
	for {
		select {
		case <-w.stop:
			return
		case ev, ok := <-w.fw.Events:
			if !ok {
				return
			}
			if !w.matchesTarget(ev.Name) {
				continue
			}
			// Ignore our own writes.
			if w.silent.Load() {
				continue
			}
			// Only react to write/create/rename/remove (chmod is irrelevant).
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			w.bounce()
		case <-w.fw.Errors:
			// fsnotify surfaces errors; keep going.
		}
	}
}

func (w *Watcher) matchesTarget(name string) bool {
	if name == "" {
		return false
	}
	a, errA := filepath.Abs(name)
	b, errB := filepath.Abs(w.path)
	if errA != nil || errB != nil {
		return filepath.Clean(name) == w.path
	}
	return a == b
}

// bounce debounces bursts into a single cb invocation.
func (w *Watcher) bounce() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(w.debounce, func() {
		// Re-check silent in case Save() flipped it on during the window.
		if w.silent.Load() {
			return
		}
		if w.cb != nil {
			w.cb()
		}
	})
}
