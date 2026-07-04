// Package netwatch detects local network-configuration changes by polling
// the interface snapshot every couple of seconds (spec §7.6).
//
// Polling — not NotifyAddrChange — is deliberate: it is simpler, robust
// across Windows versions, and reuses ifaceMgr.Snapshot. The diff detector
// fires on any interface up/down flip, IPv4 address change, or gateway
// change. core wraps the callback in a 1500ms debouncer so a flapping
// Wi-Fi reconnect produces one apply, not five.
package netwatch

import (
	"log/slog"
	"time"

	"github.com/netswitcher/netswitcher/internal/ifacemgr"
)

// Watcher polls the interface snapshot and invokes cb with a human-readable
// description of what changed ("" if nothing did this tick).
type Watcher struct {
	mgr      *ifacemgr.Manager
	cb       func(description string)
	interval time.Duration
	log      *slog.Logger

	stop    chan struct{}
	stopped chan struct{}
}

// New constructs a Watcher. interval defaults to 2s when <= 0 (spec §7.6).
func New(mgr *ifacemgr.Manager, cb func(description string), interval time.Duration, log *slog.Logger) *Watcher {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if log == nil {
		log = slog.Default()
	}
	return &Watcher{
		mgr:      mgr,
		cb:       cb,
		interval: interval,
		log:      log,
		stop:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}
}

// Start spawns the poll loop. Returns immediately.
func (w *Watcher) Start() {
	prev, err := w.mgr.Snapshot()
	if err != nil {
		w.log.Warn("netwatch initial snapshot failed", "err", err)
	}
	go w.loop(prev)
}

// Stop signals the poll loop to exit and blocks until it has.
func (w *Watcher) Stop() {
	select {
	case <-w.stop:
		// already closed
	default:
		close(w.stop)
	}
	<-w.stopped
}

func (w *Watcher) loop(prev ifacemgr.Snapshot) {
	defer close(w.stopped)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-ticker.C:
			cur, err := w.mgr.Snapshot()
			if err != nil {
				w.log.Warn("netwatch snapshot failed", "err", err)
				continue
			}
			desc, changed := Diff(prev, cur)
			if changed {
				w.log.Info("network change detected", "desc", desc)
				w.cb(desc)
			}
			prev = cur
		}
	}
}

// Diff describes the difference between two snapshots. Returns changed=true
// when any of the §7.6 conditions hold: up/down flip, IPv4 change, gateway
// change, or an interface appeared/disappeared.
func Diff(prev, cur ifacemgr.Snapshot) (description string, changed bool) {
	prevByID := indexInterfaces(prev)
	curByID := indexInterfaces(cur)

	var parts []string

	for id, p := range prevByID {
		c, ok := curByID[id]
		if !ok {
			parts = append(parts, p.Name+" disappeared")
			continue
		}
		if p.IsUp != c.IsUp {
			state := "down"
			if c.IsUp {
				state = "up"
			}
			parts = append(parts, c.Name+" went "+state)
		}
		if !sameSlice(p.IPv4, c.IPv4) {
			parts = append(parts, c.Name+" IPv4 changed")
		}
		if !sameSlice(p.Gateways, c.Gateways) {
			parts = append(parts, c.Name+" gateway changed")
		}
	}
	for id, c := range curByID {
		if _, ok := prevByID[id]; !ok {
			parts = append(parts, c.Name+" appeared")
		}
	}

	if len(parts) == 0 {
		return "", false
	}
	return joinParts(parts), true
}

func indexInterfaces(s ifacemgr.Snapshot) map[int]ifacemgr.Interface {
	m := make(map[int]ifacemgr.Interface, len(s.Interfaces))
	for _, ifc := range s.Interfaces {
		// Prefer the lowest index if duplicates (shouldn't happen); stable.
		if _, exists := m[ifc.Index]; !exists {
			m[ifc.Index] = ifc
		}
	}
	return m
}

func sameSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	// Order-independent compare: addresses/gateways may reorder between polls.
	seen := make(map[string]int, len(a))
	for _, x := range a {
		seen[x]++
	}
	for _, y := range b {
		if seen[y]--; seen[y] < 0 {
			return false
		}
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

func joinParts(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "; "
		}
		out += p
	}
	return out
}
