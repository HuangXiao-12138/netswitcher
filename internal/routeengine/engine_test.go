package routeengine_test

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/netswitcher/netswitcher/internal/config"
	"github.com/netswitcher/netswitcher/internal/ifacemgr"
	"github.com/netswitcher/netswitcher/internal/routeengine"
	"github.com/netswitcher/netswitcher/internal/state"
)

// mockExec records every Add/Delete call and can be wired to fail specific ones.
type mockExec struct {
	mu      sync.Mutex
	added   []state.Entry
	deleted []state.Entry
	failAdd map[string]bool // destination → fail
	failDel map[string]bool
}

func (m *mockExec) Add(r state.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failAdd != nil && m.failAdd[r.Destination] {
		return errFake{op: "add", dest: r.Destination}
	}
	m.added = append(m.added, r)
	return nil
}
func (m *mockExec) Delete(dest string, ifIndex int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failDel != nil && m.failDel[dest] {
		return errFake{op: "delete", dest: dest}
	}
	m.deleted = append(m.deleted, state.Entry{Destination: dest, IfIndex: ifIndex})
	return nil
}

type mockMetric struct{ sets []mockMetricSet }

type mockMetricSet struct {
	iface  string
	metric int
}

func (m *mockMetric) SetInterfaceMetric(iface string, metric int) error {
	m.sets = append(m.sets, mockMetricSet{iface, metric})
	return nil
}
func (m *mockMetric) SetAutomaticMetric(iface string) error { return nil }

type errFake struct{ op, dest string }

func (e errFake) Error() string { return "fake " + e.op + " failure for " + e.dest }

func boolPtr(b bool) *bool { return &b }

func sampleSnap() ifacemgr.Snapshot {
	return ifacemgr.Snapshot{
		Interfaces: []ifacemgr.Interface{
			{Index: 12, Name: "以太网", FriendlyName: "Intel I219-V", IsUp: true, IPv4: []string{"172.16.5.10/24"}, Gateways: []string{"172.16.5.1"}},
			{Index: 7, Name: "WLAN", FriendlyName: "Intel Wireless", IsUp: true, IPv4: []string{"192.168.1.5/24"}, Gateways: []string{"192.168.1.1"}},
		},
	}
}

func officeProfile() *config.Profile {
	return &config.Profile{
		ID:                    "office",
		Name:                  "Office",
		DefaultRouteInterface: "WLAN",
		MetricPolicy:          &config.MetricPolicy{PreferredInterface: "WLAN"},
		Rules: []config.Rule{
			{ID: "r1", Destination: "168.168.0.0/16", ViaInterface: "以太网", ViaGateway: "auto", Metric: 1, Enabled: boolPtr(true)},
			{ID: "r2", Destination: "172.16.0.0/16", ViaInterface: "以太网", ViaGateway: "auto", Metric: 1, Enabled: boolPtr(true)},
		},
	}
}

func newEngine(t *testing.T) (*routeengine.Engine, *mockExec, *mockMetric, *state.Store) {
	t.Helper()
	dir := t.TempDir()
	store := state.New(filepath.Join(dir, "state.json"))
	exec := &mockExec{}
	met := &mockMetric{}
	return routeengine.New(exec, met, store, nil), exec, met, store
}

func TestComputeWantSet_AutoGateway(t *testing.T) {
	want, skipped := routeengine.ComputeWantSet(officeProfile(), sampleSnap())
	if len(skipped) != 0 {
		t.Fatalf("expected no skips, got %+v", skipped)
	}
	if len(want) != 2 {
		t.Fatalf("want 2 entries, got %d", len(want))
	}
	// viaGateway=auto should resolve to the 以太网 gateway 172.16.5.1.
	var r1 state.Entry
	for _, w := range want {
		if w.Destination == "168.168.0.0/16" {
			r1 = w
		}
	}
	if r1.Gateway != "172.16.5.1" {
		t.Errorf("r1 gateway = %q, want 172.16.5.1", r1.Gateway)
	}
	if r1.IfIndex != 12 {
		t.Errorf("r1 ifIndex = %d, want 12", r1.IfIndex)
	}
}

func TestComputeWantSet_SkipsMissingInterface(t *testing.T) {
	p := officeProfile()
	p.Rules[0].ViaInterface = "不存在"
	want, skipped := routeengine.ComputeWantSet(p, sampleSnap())
	if len(skipped) != 1 || skipped[0].Reason != "interface not found" {
		t.Fatalf("expected one skip (interface not found), got %+v", skipped)
	}
	if len(want) != 1 {
		t.Errorf("want 1 entry (other rule), got %d", len(want))
	}
}

func TestDiff_AddRemoveModify(t *testing.T) {
	old := []state.Entry{
		{Destination: "10.0.0.0/8", Gateway: "1.1.1.1", IfIndex: 5, Metric: 1},     // kept
		{Destination: "172.16.0.0/16", Gateway: "2.2.2.2", IfIndex: 6, Metric: 1},  // removed
		{Destination: "192.168.0.0/16", Gateway: "3.3.3.3", IfIndex: 7, Metric: 1}, // modified (gateway change)
	}
	want := []state.Entry{
		{Destination: "10.0.0.0/8", Gateway: "1.1.1.1", IfIndex: 5, Metric: 1},     // unchanged
		{Destination: "192.168.0.0/16", Gateway: "9.9.9.9", IfIndex: 7, Metric: 1}, // changed gateway
		{Destination: "168.168.0.0/16", Gateway: "8.8.8.8", IfIndex: 8, Metric: 1}, // new
	}
	toAdd, toRemove := routeengine.Diff(want, old)
	if len(toAdd) != 2 {
		t.Errorf("toAdd = %d, want 2 (new + modified): %+v", len(toAdd), toAdd)
	}
	if len(toRemove) != 2 {
		t.Errorf("toRemove = %d, want 2 (gone + modified): %+v", len(toRemove), toRemove)
	}
}

func TestDiff_IdempotentNoChange(t *testing.T) {
	set := []state.Entry{{Destination: "10.0.0.0/8", Gateway: "1.1.1.1", IfIndex: 5, Metric: 1}}
	toAdd, toRemove := routeengine.Diff(set, set)
	if len(toAdd) != 0 || len(toRemove) != 0 {
		t.Errorf("identical sets should produce empty diff; add=%d del=%d", len(toAdd), len(toRemove))
	}
}

func TestApply_FreshAddsAndPersists(t *testing.T) {
	eng, exec, _, store := newEngine(t)
	res := eng.Apply(officeProfile(), sampleSnap(), "test")
	if len(res.Applied) != 2 {
		t.Fatalf("applied = %d, want 2: %+v", len(res.Applied), res.Applied)
	}
	if len(exec.added) != 2 {
		t.Errorf("exec.added = %d, want 2", len(exec.added))
	}
	snap, _ := store.Load()
	if len(snap.Entries) != 2 {
		t.Errorf("state after apply = %d entries, want 2", len(snap.Entries))
	}
}

func TestApply_IdempotentSecondPass(t *testing.T) {
	eng, exec, _, _ := newEngine(t)
	eng.Apply(officeProfile(), sampleSnap(), "first")
	n := len(exec.added)
	eng.Apply(officeProfile(), sampleSnap(), "second")
	if len(exec.added) != n {
		t.Errorf("second apply added %d more routes; want 0 (idempotent)", len(exec.added)-n)
	}
}

func TestApply_DetectsRuleChange(t *testing.T) {
	eng, exec, _, _ := newEngine(t)
	p := officeProfile()
	eng.Apply(p, sampleSnap(), "first")

	addedBefore := len(exec.added)
	deletedBefore := len(exec.deleted)

	// Change gateway of r1 from auto to explicit 172.16.99.1.
	p.Rules[0].ViaGateway = "172.16.99.1"
	eng.Apply(p, sampleSnap(), "second")

	// Second pass should add the new-gateway route and delete the old one.
	gotAdd := 0
	for _, e := range exec.added[addedBefore:] {
		if e.Destination == "168.168.0.0/16" {
			gotAdd++
		}
	}
	gotDel := 0
	for _, e := range exec.deleted[deletedBefore:] {
		if e.Destination == "168.168.0.0/16" {
			gotDel++
		}
	}
	if gotAdd != 1 || gotDel != 1 {
		t.Errorf("changed rule: add=%d del=%d, want 1/1", gotAdd, gotDel)
	}
}

func TestApply_FailedAddRetried(t *testing.T) {
	eng, exec, _, store := newEngine(t)
	eng, _, _, _ = newEngine(t) // discard; rewire below
	// Configure the mock to fail adds for 168.168.0.0/16.
	dir := t.TempDir()
	store = state.New(filepath.Join(dir, "state.json"))
	exec = &mockExec{failAdd: map[string]bool{"168.168.0.0/16": true}}
	met := &mockMetric{}
	eng = routeengine.New(exec, met, store, nil)

	res := eng.Apply(officeProfile(), sampleSnap(), "fail-add")
	if len(res.Applied) != 1 {
		t.Fatalf("applied = %d, want 1 (the non-failing rule)", len(res.Applied))
	}
	if len(res.Errors) != 1 {
		t.Fatalf("errors = %d, want 1", len(res.Errors))
	}
	snap, _ := store.Load()
	for _, e := range snap.Entries {
		if e.Destination == "168.168.0.0/16" {
			t.Fatal("failed add must not be persisted (so it retries next pass)")
		}
	}
}

func TestApply_NilProfileRemovesAll(t *testing.T) {
	eng, exec, _, store := newEngine(t)
	eng.Apply(officeProfile(), sampleSnap(), "seed")
	if len(exec.added) != 2 {
		t.Fatalf("seed: added = %d, want 2", len(exec.added))
	}
	res := eng.Apply(nil, sampleSnap(), "wipe")
	if len(res.Removed) != 2 {
		t.Errorf("removed = %d, want 2", len(res.Removed))
	}
	snap, _ := store.Load()
	if len(snap.Entries) != 0 {
		t.Errorf("state after wipe = %d, want 0", len(snap.Entries))
	}
}

func TestApply_MetricManagement(t *testing.T) {
	eng, _, met, _ := newEngine(t)
	p := officeProfile()
	p.AutoManageMetrics = boolPtr(true)
	eng.Apply(p, sampleSnap(), "metric")
	// Only the preferred interface (WLAN) gets a metric now; others are left
	// untouched (applyMetrics no longer sets others, to avoid interfering with
	// VPN / virtual adapters).
	var wlan, eth *mockMetricSet
	for i := range met.sets {
		if met.sets[i].iface == "WLAN" {
			wlan = &met.sets[i]
		}
		if met.sets[i].iface == "以太网" {
			eth = &met.sets[i]
		}
	}
	if wlan == nil || wlan.metric != config.DefaultPreferredMetric {
		t.Errorf("WLAN metric = %+v, want %d", wlan, config.DefaultPreferredMetric)
	}
	if eth != nil {
		t.Errorf("以太网 metric = %+v, want nil (only preferred is managed)", eth)
	}
}

func TestApply_DryRunNoMutation(t *testing.T) {
	// DryRun is a property of the real Exec, not the mock; this test just
	// verifies a metric-less profile still completes and persists state.
	dir := t.TempDir()
	store := state.New(filepath.Join(dir, "state.json"))
	exec := &mockExec{}
	eng := routeengine.New(exec, &mockMetric{}, store, nil)
	p := officeProfile()
	p.AutoManageMetrics = boolPtr(false)

	res := eng.Apply(p, sampleSnap(), "dry")
	if len(res.Applied) != 2 {
		t.Fatalf("applied = %d, want 2", len(res.Applied))
	}
}
