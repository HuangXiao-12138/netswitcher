// Package routeengine is the declarative routing reconciler (spec §7.4).
//
// Apply makes the system route table match the active profile: it computes
// the desired route set from the profile + current interface snapshot, diffs
// against the last-applied set in state.json, and emits only the add/delete
// delta. Routes are runtime-only (never -p); only routes NetSwitcher itself
// recorded in state.json are ever deleted, so system / VPN routes are never
// touched (§17.4).
package routeengine

import (
	"log/slog"
	"strings"
	"time"

	"github.com/netswitcher/netswitcher/internal/config"
	"github.com/netswitcher/netswitcher/internal/ifacemgr"
	"github.com/netswitcher/netswitcher/internal/state"
)

// SkippedRule captures a rule that could not be turned into a route this
// pass (interface missing, no gateway, etc.). It is retried on the next apply.
type SkippedRule struct {
	RuleID       string `json:"ruleId"`
	Destination  string `json:"destination"`
	ViaInterface string `json:"viaInterface"`
	Reason       string `json:"reason"`
}

// RuleError is a route add/delete that failed against route.exe.
type RuleError struct {
	RuleID      string `json:"ruleId,omitempty"`
	Destination string `json:"destination,omitempty"`
	Op          string `json:"op"` // "add" | "delete" | "metric" | "state"
	Message     string `json:"message"`
}

// MetricChange records an interface metric adjustment.
type MetricChange struct {
	Interface string `json:"interface"`
	NewMetric int    `json:"newMetric"`
}

// ApplyResult is the outcome of one Apply pass. Applied/Removed are routes
// actually changed; Skipped/Errors are advisories. Conflicts come from the
// conflict detector (§7.7) and are attached by the caller (core).
type ApplyResult struct {
	Applied []state.Entry  `json:"applied"`
	Removed []state.Entry  `json:"removed"`
	Skipped []SkippedRule  `json:"skipped"`
	Errors  []RuleError    `json:"errors"`
	Metrics []MetricChange `json:"metrics"`
	At      time.Time      `json:"at"`
	Reason  string         `json:"reason"`
}

// Engine is the route reconciler.
type Engine struct {
	exec    Executor
	metrics MetricSetter
	store   *state.Store
	log     *slog.Logger
}

// New constructs an Engine. log may be nil (defaults to slog.Default()).
func New(exec Executor, ms MetricSetter, store *state.Store, log *slog.Logger) *Engine {
	if log == nil {
		log = slog.Default()
	}
	return &Engine{exec: exec, metrics: ms, store: store, log: log}
}

// Apply reconciles the system route table to the active profile. With a nil
// profile (no active selection) every previously-applied route is removed.
func (e *Engine) Apply(profile *config.Profile, snap ifacemgr.Snapshot, reason string) ApplyResult {
	res := ApplyResult{Reason: reason, At: time.Now()}
	e.log.Info("apply start", "reason", reason)

	// Always start from the last-applied set.
	prev, err := e.store.Load()
	if err != nil {
		e.log.Warn("state load failed; starting from empty", "err", err)
	}

	if profile == nil {
		// Nothing wanted → tear down everything we manage.
		for _, r := range prev.Entries {
			if delErr := e.exec.Delete(r.Destination, r.IfIndex); delErr != nil {
				res.Errors = append(res.Errors, RuleError{Destination: r.Destination, Op: "delete", Message: delErr.Error()})
			} else {
				res.Removed = append(res.Removed, r)
			}
		}
		e.saveState(state.Snapshot{}, reason, &res)
		res.At = time.Now()
		e.log.Info("apply done (no profile)", "removed", len(res.Removed), "errors", len(res.Errors))
		return res
	}

	want, skipped := ComputeWantSet(profile, snap)
	res.Skipped = skipped

	toAdd, toRemove := Diff(want, prev.Entries)

	// Delete obsolete first so interface index reuse is clean.
	for _, r := range toRemove {
		if delErr := e.exec.Delete(r.Destination, r.IfIndex); delErr != nil {
			res.Errors = append(res.Errors, RuleError{Destination: r.Destination, Op: "delete", Message: delErr.Error()})
			continue
		}
		res.Removed = append(res.Removed, r)
	}

	// Add new / changed.
	applied := make([]state.Entry, 0, len(toAdd))
	for _, r := range toAdd {
		if addErr := e.exec.Add(r); addErr != nil {
			res.Errors = append(res.Errors, RuleError{Destination: r.Destination, Op: "add", Message: addErr.Error()})
			continue
		}
		res.Applied = append(res.Applied, r)
		applied = append(applied, r)
	}

	if profile.AutoManage() {
		res.Metrics = e.applyMetrics(profile, snap)
	}

	// Persist the new baseline: (prev - removed) ∪ applied. Failed adds are
	// NOT included so they retry next pass; failed removes stay in baseline.
	e.saveState(state.Snapshot{Entries: mergeBaseline(prev.Entries, res.Removed, applied)}, reason, &res)

	res.At = time.Now()
	e.log.Info("apply done",
		"reason", reason,
		"added", len(res.Applied),
		"removed", len(res.Removed),
		"skipped", len(res.Skipped),
		"errors", len(res.Errors),
		"metrics", len(res.Metrics))
	return res
}

// ComputeWantSet turns the profile rules into the desired route set using the
// current interface snapshot. Skipped rules (interface missing, no gateway)
// are returned alongside. Exported for unit testing.
func ComputeWantSet(profile *config.Profile, snap ifacemgr.Snapshot) ([]state.Entry, []SkippedRule) {
	want := make([]state.Entry, 0, len(profile.Rules))
	var skipped []SkippedRule

	for i := range profile.Rules {
		r := profile.Rules[i]
		if !r.IsEnabled() {
			continue
		}
		ifc, err := ifacemgr.ResolveByName(r.ViaInterface, snap)
		if err != nil {
			skipped = append(skipped, SkippedRule{RuleID: r.ID, Destination: r.Destination, ViaInterface: r.ViaInterface, Reason: "interface not found"})
			continue
		}
		gw := r.ViaGateway
		if strings.EqualFold(gw, "auto") {
			gw = ifc.PrimaryGateway()
			if gw == "" {
				skipped = append(skipped, SkippedRule{RuleID: r.ID, Destination: r.Destination, ViaInterface: r.ViaInterface, Reason: "no IPv4 gateway on " + ifc.Name})
				continue
			}
		}
		want = append(want, state.Entry{
			Destination: r.Destination,
			Gateway:     gw,
			Interface:   ifc.Name,
			IfIndex:     ifc.Index,
			Metric:      r.EffectiveMetric(),
		})
	}
	return want, skipped
}

// routeKey is the diff identity: same destination on the same interface is
// the same managed route.
type routeKey struct {
	Destination string
	IfIndex     int
}

// Diff returns the routes to add (in want, not in old, or content changed)
// and to remove (in old, not in want, or replaced). Gateway/metric changes
// produce a delete+add pair.
func Diff(want, old []state.Entry) (toAdd, toRemove []state.Entry) {
	wantIdx := make(map[routeKey]state.Entry, len(want))
	for _, r := range want {
		wantIdx[routeKey{r.Destination, r.IfIndex}] = r
	}
	oldIdx := make(map[routeKey]state.Entry, len(old))
	for _, r := range old {
		oldIdx[routeKey{r.Destination, r.IfIndex}] = r
	}

	for k, wr := range wantIdx {
		or, exists := oldIdx[k]
		if !exists {
			toAdd = append(toAdd, wr)
			continue
		}
		if wr.Gateway != or.Gateway || wr.Metric != or.Metric {
			toRemove = append(toRemove, or)
			toAdd = append(toAdd, wr)
		}
	}
	for k, or := range oldIdx {
		if _, exists := wantIdx[k]; !exists {
			toRemove = append(toRemove, or)
		}
	}
	return toAdd, toRemove
}

// applyMetrics adjusts interface metrics so the preferred interface wins the
// default route (§7.5). If the preferred interface can't be resolved we warn
// and skip — never block the apply.
func (e *Engine) applyMetrics(profile *config.Profile, snap ifacemgr.Snapshot) []MetricChange {
	preferredName := ""
	if profile.MetricPolicy != nil && strings.TrimSpace(profile.MetricPolicy.PreferredInterface) != "" {
		preferredName = profile.MetricPolicy.PreferredInterface
	} else if strings.TrimSpace(profile.DefaultRouteInterface) != "" {
		preferredName = profile.DefaultRouteInterface
	}
	if preferredName == "" {
		e.log.Warn("autoManageMetrics enabled but no preferred interface configured; skipping metric management")
		return nil
	}

	preferredMetric := config.DefaultPreferredMetric
	othersMetric := config.DefaultOthersMetric
	if profile.MetricPolicy != nil {
		if profile.MetricPolicy.PreferredMetric > 0 {
			preferredMetric = profile.MetricPolicy.PreferredMetric
		}
		if profile.MetricPolicy.OthersMetric > 0 {
			othersMetric = profile.MetricPolicy.OthersMetric
		}
	}

	preferredIfc, err := ifacemgr.ResolveByName(preferredName, snap)
	if err != nil {
		e.log.Warn("preferred interface not present; skipping metric management", "name", preferredName)
		return nil
	}
	preferredIdx := preferredIfc.Index

	var changes []MetricChange
	for _, ifc := range snap.Interfaces {
		if !ifc.IsUp || len(ifc.IPv4) == 0 {
			continue
		}
		target := othersMetric
		if ifc.Index == preferredIdx {
			target = preferredMetric
		}
		if e.metrics == nil {
			continue
		}
		if err := e.metrics.SetInterfaceMetric(ifc.Name, target); err != nil {
			e.log.Warn("set metric failed", "iface", ifc.Name, "err", err)
			continue
		}
		changes = append(changes, MetricChange{Interface: ifc.Name, NewMetric: target})
	}
	return changes
}

// saveState persists the new baseline, logging but not failing the apply on
// save error.
func (e *Engine) saveState(snap state.Snapshot, reason string, res *ApplyResult) {
	snap.Reason = reason
	if err := e.store.Save(snap); err != nil {
		e.log.Error("state save failed", "err", err)
		res.Errors = append(res.Errors, RuleError{Op: "state", Message: err.Error()})
	}
}

// mergeBaseline returns (old - removed) ∪ added, keyed by routeKey.
func mergeBaseline(old, removed, added []state.Entry) []state.Entry {
	rm := make(map[routeKey]bool, len(removed))
	for _, r := range removed {
		rm[routeKey{r.Destination, r.IfIndex}] = true
	}
	out := make([]state.Entry, 0, len(old)-len(removed)+len(added))
	for _, r := range old {
		if !rm[routeKey{r.Destination, r.IfIndex}] {
			out = append(out, r)
		}
	}
	out = append(out, added...)
	return out
}
