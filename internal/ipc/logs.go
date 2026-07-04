package ipc

import (
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/netswitcher/netswitcher/internal/logging"
)

// logFanout fans slog JSON records out to many SubscribeLogs subscribers,
// each optionally filtered by level. It registers itself once with the
// logging package (logging.SetPipeSink) so every emitted record reaches
// every live subscriber.
type logFanout struct {
	mu     sync.RWMutex
	subs   map[int]*logSub
	nextID atomic.Int64
}

type logSub struct {
	level  slog.Level
	ch     chan []byte
	closed atomic.Bool
}

func newLogFanout() *logFanout {
	f := &logFanout{subs: make(map[int]*logSub)}
	logging.SetPipeSink(f) // register as the global log sink
	return f
}

// Write implements io.Writer; logging hands one complete JSON record per call.
// We parse the level field and dispatch to matching subscribers without
// blocking the logger: each subscriber has a small buffered channel; if full,
// the record is dropped for that subscriber (better than stalling apply).
func (f *logFanout) Write(p []byte) (int, error) {
	lvl := extractLevel(p)
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, s := range f.subs {
		if s.level > lvl {
			continue
		}
		select {
		case s.ch <- append([]byte(nil), p...):
		default:
			// drop on slow consumer
		}
	}
	return len(p), nil
}

func (f *logFanout) subscribe(level slog.Level) (int, <-chan []byte) {
	id := int(f.nextID.Add(1))
	s := &logSub{level: level, ch: make(chan []byte, 256)}
	f.mu.Lock()
	f.subs[id] = s
	f.mu.Unlock()
	return id, s.ch
}

func (f *logFanout) unsubscribe(id int) {
	f.mu.Lock()
	if s, ok := f.subs[id]; ok {
		s.closed.Store(true)
		delete(f.subs, id)
	}
	f.mu.Unlock()
}

// stopAll closes all subscriber channels (server shutdown).
func (f *logFanout) stopAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, s := range f.subs {
		s.closed.Store(true)
		close(s.ch)
		delete(f.subs, id)
	}
	logging.SetPipeSink(nil)
}

// extractLevel parses just the "level" field out of a slog JSON record
// without decoding the whole thing into a typed struct.
func extractLevel(p []byte) slog.Level {
	var probe struct {
		Level string `json:"level"`
	}
	if json.Unmarshal(p, &probe) != nil {
		return slog.LevelInfo
	}
	return logging.LevelFromString(probe.Level)
}

var _ io.Writer = (*logFanout)(nil)
