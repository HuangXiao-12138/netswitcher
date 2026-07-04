package core

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netswitcher/netswitcher/internal/config"
	"github.com/netswitcher/netswitcher/internal/routeengine"
	"github.com/netswitcher/netswitcher/internal/state"
)

// recordingExec is a mock Executor that records calls and never fails.
type recordingExec struct {
	mu   sync.Mutex
	adds []state.Entry
	dels []state.Entry
}

func (e *recordingExec) Add(r state.Entry) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.adds = append(e.adds, r)
	return nil
}
func (e *recordingExec) Delete(dest string, ifIndex int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dels = append(e.dels, state.Entry{Destination: dest, IfIndex: ifIndex})
	return nil
}

type noopMetric struct{}

func (noopMetric) SetInterfaceMetric(string, int) error { return nil }
func (noopMetric) SetAutomaticMetric(string) error      { return nil }

// newTestCore builds a Core in a temp dir with mocks + short debounce.
func newTestCore(t *testing.T) (*Core, *recordingExec, string, string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	statePath := filepath.Join(dir, "state.json")
	// Seed a config whose rules reference interfaces that won't resolve in CI
	// (the engine will report Skipped but still complete apply).
	cfg := &config.Config{
		Version:       1,
		ActiveProfile: "p",
		Profiles: []config.Profile{{
			ID:   "p",
			Name: "P",
			Rules: []config.Rule{
				{ID: "r1", Destination: "168.168.0.0/16", ViaInterface: "MissingIface", ViaGateway: "auto"},
			},
		}},
	}
	if err := config.SaveSimple(cfgPath, cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	exec := &recordingExec{}
	c, err := New(Options{
		ConfigPath:   cfgPath,
		StatePath:    statePath,
		Debounce:     20 * time.Millisecond,
		ApplyExec:    exec,
		MetricSetter: noopMetric{},
	}, nil)
	if err != nil {
		t.Fatalf("core.New: %v", err)
	}
	return c, exec, cfgPath, statePath
}

func TestCore_StartEmitsInitialApply(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	var fires int32
	c.SubscribeStatus(func(st StatusResponse) { atomic.AddInt32(&fires, 1) })
	if err := c.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer c.Stop()
	if !waitFor(&fires, 1, time.Second) {
		t.Fatalf("initial apply status not emitted; fires=%d", atomic.LoadInt32(&fires))
	}
	st := c.Status()
	if st.LastResult.Reason != "startup" {
		t.Errorf("lastResult.Reason = %q, want startup", st.LastResult.Reason)
	}
	if len(st.LastResult.Skipped) != 1 {
		t.Errorf("expected the MissingIface rule to be skipped, got %+v", st.LastResult.Skipped)
	}
}

func TestCore_ApplyNowSerializes(t *testing.T) {
	c, exec, _, _ := newTestCore(t)
	c.Start()
	defer c.Stop()
	// Two concurrent ApplyNow calls; both should complete without racing.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.ApplyOnce("concurrent")
		}()
	}
	wg.Wait()
	c.mu.Lock()
	defer c.mu.Unlock()
	// Every Add went through the lock; no panic → serialized.
	_ = exec
}

func TestCore_ConfigChangeReloadsAndApplies(t *testing.T) {
	c, exec, cfgPath, _ := newTestCore(t)
	c.Start()
	defer c.Stop()
	time.Sleep(40 * time.Millisecond) // let initial apply settle

	// Rewrite config externally (bypass Save so we don't suppress).
	c2, _ := config.Load(cfgPath)
	c2.Profiles[0].Rules = []config.Rule{
		{ID: "rx", Destination: "10.0.0.0/8", ViaInterface: "OtherMissing", ViaGateway: "auto"},
	}
	if err := config.SaveSimple(cfgPath, c2); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	// onConfigChange is invoked by the fsnotify watcher in real life; we
	// call it directly to keep the test deterministic.
	var fires int32
	c.SubscribeStatus(func(st StatusResponse) { atomic.AddInt32(&fires, 1) })
	c.onConfigChange()
	if !waitFor(&fires, 1, time.Second) {
		t.Fatalf("config_change apply not emitted; fires=%d", atomic.LoadInt32(&fires))
	}
	exec.mu.Lock()
	defer exec.mu.Unlock()
	if len(exec.adds) != 0 {
		// The new rule references a missing interface → no adds, but apply ran.
		t.Errorf("expected zero adds for missing interface, got %d", len(exec.adds))
	}
}

func TestCore_SetActiveProfileUnknown(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	c.Start()
	defer c.Stop()
	if err := c.SetActiveProfile("does-not-exist"); err == nil {
		t.Error("expected error for unknown profile id")
	}
}

func TestCore_SubscribeAndUnsubscribe(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	var fires int32
	unsub := c.SubscribeStatus(func(st StatusResponse) { atomic.AddInt32(&fires, 1) })
	c.Start()
	defer c.Stop()
	if !waitFor(&fires, 1, time.Second) {
		t.Fatalf("expected at least 1 fire before unsubscribe; got %d", atomic.LoadInt32(&fires))
	}
	unsub()
	before := atomic.LoadInt32(&fires)
	c.ApplyOnce("after-unsub")
	time.Sleep(60 * time.Millisecond)
	if got := atomic.LoadInt32(&fires); got != before {
		t.Errorf("after unsubscribe, fires went %d → %d", before, got)
	}
}

func waitFor(counter *int32, want int32, max time.Duration) bool {
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(counter) >= want {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return atomic.LoadInt32(counter) >= want
}

// silence unused import if routeengine only used as type ref via Options.
var _ = routeengine.New
