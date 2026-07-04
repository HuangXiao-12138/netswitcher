// Package core is the service's heart (spec §7.1). It owns all subsystems
// (config, iface manager, route engine, conflict detector, watchers),
// serializes applies, and exposes the operations the IPC layer proxies.
//
// Lifecycle:
//
//	c := New(opts)
//	c.Start()   // non-blocking: load config, start watchers, applyOnce("startup")
//	… run …
//	c.Stop()    // stop watchers; routes are NOT torn down (§7.1)
//
// onNetworkChange and onConfigChange funnel through a 1500ms debouncer into
// applyOnce so a flappy reconnect yields one apply.
package core

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/netswitcher/netswitcher/internal/config"
	"github.com/netswitcher/netswitcher/internal/conflict"
	"github.com/netswitcher/netswitcher/internal/ifacemgr"
	"github.com/netswitcher/netswitcher/internal/netwatch"
	"github.com/netswitcher/netswitcher/internal/routeengine"
	"github.com/netswitcher/netswitcher/internal/state"
)

// Options configures a Core.
type Options struct {
	ConfigPath string
	StatePath  string
	LogLevel   string
	LogDir     string
	// PollInterval overrides the default 2s netwatch interval (mostly for tests).
	PollInterval time.Duration
	// Debounce overrides the default 1500ms apply debounce (mostly for tests).
	Debounce time.Duration
	// ApplyExec lets tests inject a mock route executor. Nil → real route.exe.
	ApplyExec routeengine.Executor
	// MetricSetter lets tests inject a mock. Nil → real netsh.
	MetricSetter routeengine.MetricSetter
}

// StatusResponse is the data the IPC GetStatus / SubscribeStatus push to GUIs.
type StatusResponse struct {
	Interfaces    []ifacemgr.Interface    `json:"interfaces"`
	ActiveProfile *config.Profile         `json:"activeProfile"`
	LastResult    routeengine.ApplyResult `json:"lastResult"`
	Conflicts     []conflict.Conflict     `json:"conflicts"`
	SnapshotAt    time.Time               `json:"snapshotAt"`
}

// Core owns the live service state.
type Core struct {
	opts Options
	log  *slog.Logger

	mu  sync.Mutex // serializes applyOnce / config mutation
	cfg *config.Config

	cfgWatcher *config.Watcher
	ifaceMgr   *ifacemgr.Manager
	store      *state.Store
	engine     *routeengine.Engine
	conflict   *conflict.Detector
	netwatch   *netwatch.Watcher
	debounce   *Debouncer

	statusMu     sync.RWMutex
	statusCbs    map[int64]func(StatusResponse)
	statusNextID int64
	lastStatus   StatusResponse

	started bool
}

// New constructs a Core. Call Start to begin operating.
func New(opts Options, log *slog.Logger) (*Core, error) {
	if opts.ConfigPath == "" {
		return nil, errors.New("core: ConfigPath required")
	}
	if opts.StatePath == "" {
		return nil, errors.New("core: StatePath required")
	}
	if log == nil {
		log = slog.Default()
	}

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		// Spec §14: a bad config on disk is not fatal; start with empty and
		// log loudly. The GUI can re-save a corrected config.
		log.Error("config load failed; starting empty", "err", err, "path", opts.ConfigPath)
		cfg = &config.Config{Version: config.SchemaVersion, LogLevel: opts.LogLevel}
	}

	c := &Core{
		opts:     opts,
		log:      log,
		cfg:      cfg,
		ifaceMgr: ifacemgr.New(),
		store:    state.New(opts.StatePath),
		conflict: conflict.New(log),
	}

	debounce := opts.Debounce
	if debounce <= 0 {
		debounce = 1500 * time.Millisecond
	}
	c.debounce = NewDebouncer(debounce)

	exec := opts.ApplyExec
	if exec == nil {
		exec = &routeengine.Exec{}
	}
	ms := opts.MetricSetter
	if ms == nil {
		ms = &routeengine.NetshMetric{}
	}
	c.engine = routeengine.New(exec, ms, c.store, log)

	return c, nil
}

// Start loads config, spins up watchers, and emits an initial apply.
func (c *Core) Start() error {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return errors.New("core already started")
	}
	c.started = true
	c.mu.Unlock()

	// Config watcher → reload + apply on external edits.
	w, err := config.NewWatcher(c.opts.ConfigPath, c.onConfigChange)
	if err != nil {
		c.log.Error("config watcher setup failed", "err", err)
	} else if err := w.Start(); err != nil {
		c.log.Error("config watcher start failed", "err", err)
	} else {
		c.cfgWatcher = w
	}

	// Network watcher → debounced apply.
	c.netwatch = netwatch.New(c.ifaceMgr, c.onNetworkChange, c.opts.PollInterval, c.log)
	c.netwatch.Start()

	// Initial apply.
	c.applyOnce("startup")
	return nil
}

// Stop tears down watchers. Per spec §7.1, routes are NOT cleaned up — a
// service stop must not sever the user's connectivity. The debouncer is
// flushed so a pending change-driven apply completes first.
func (c *Core) Stop() {
	if c.netwatch != nil {
		c.netwatch.Stop()
	}
	c.debounce.Flush()
	if c.cfgWatcher != nil {
		_ = c.cfgWatcher.Close()
	}
}

// ApplyOnce is the IPC ApplyNow entrypoint. It is serialized against config
// changes and other applies.
func (c *Core) ApplyOnce(reason string) routeengine.ApplyResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.applyOnce(reason).LastResult
}

// applyOnce is the un-locked inner; caller holds c.mu. It snapshots,
// reconciles, checks cheap conflicts, and emits a status event.
func (c *Core) applyOnce(reason string) StatusResponse {
	snap, err := c.ifaceMgr.Snapshot()
	if err != nil {
		c.log.Error("interface snapshot failed; proceeding with empty", "err", err)
	}

	profile := c.cfg.ActiveProfileOrDefault()
	result := c.engine.Apply(profile, snap, reason)

	wantsDefault := profile != nil && profile.AutoManage() &&
		(profile.DefaultRouteInterface != "" ||
			(profile.MetricPolicy != nil && profile.MetricPolicy.PreferredInterface != ""))
	conflicts := c.conflict.CheckVPN(snap, wantsDefault)

	st := StatusResponse{
		Interfaces:    snap.Interfaces,
		ActiveProfile: profile,
		LastResult:    result,
		Conflicts:     conflicts,
		SnapshotAt:    snap.TakenAt,
	}
	c.setStatus(st)
	c.emitStatus(st)
	return st
}

func (c *Core) onNetworkChange(desc string) {
	c.debounce.Call(func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.applyOnce("network_change: " + desc)
	})
}

func (c *Core) onConfigChange() {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Reload; on failure keep the cached config (§14).
	loaded, err := config.Load(c.opts.ConfigPath)
	if err != nil {
		c.log.Error("config reload failed; keeping previous", "err", err)
		return
	}
	c.cfg = loaded
	c.log.Info("config reloaded", "activeProfile", loaded.ActiveProfile)
	c.applyOnce("config_change")
}

// Config returns a pointer to the current config (read-only for callers).
func (c *Core) Config() *config.Config {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg
}

// ManagedRoutes returns the routes currently recorded in state.json (the set
// NetSwitcher last applied). Used by the IPC GetRouteTable handler to label
// rows as "managed" vs system/VPN.
func (c *Core) ManagedRoutes() []state.Entry {
	snap, err := c.store.Load()
	if err != nil {
		return nil
	}
	return snap.Entries
}

// SetActiveProfile switches the active profile and triggers an apply.
// Returns an error if the id does not exist.
func (c *Core) SetActiveProfile(id string) error {
	c.mu.Lock()
	cfg := c.cfg
	c.mu.Unlock()

	found := false
	for _, p := range cfg.Profiles {
		if p.ID == id {
			found = true
			break
		}
	}
	if !found {
		return errors.New("unknown profile id: " + id)
	}

	// Save (suppressing the watcher echo) then apply.
	cfg.ActiveProfile = id
	if err := config.Save(c.opts.ConfigPath, cfg, c.cfgWatcher); err != nil {
		return err
	}
	c.mu.Lock()
	c.cfg = cfg
	c.mu.Unlock()
	c.ApplyOnce("set_active:" + id)
	return nil
}

// SaveConfig persists a new config (used by IPC SaveConfig). The watcher is
// suppressed around the write so this does not echo back as config_change;
// we still apply immediately since the caller wants the change now.
func (c *Core) SaveConfig(newCfg *config.Config) error {
	if err := config.Save(c.opts.ConfigPath, newCfg, c.cfgWatcher); err != nil {
		return err
	}
	c.mu.Lock()
	c.cfg = newCfg
	c.mu.Unlock()
	c.ApplyOnce("save_config")
	return nil
}

// Status returns the latest snapshot + apply result + conflicts. If stale or
// never applied, triggers a fresh snapshot (cheap) without applying.
func (c *Core) Status() StatusResponse {
	c.statusMu.RLock()
	st := c.lastStatus
	c.statusMu.RUnlock()
	if !st.SnapshotAt.IsZero() {
		return st
	}
	// No status yet (e.g. Start failed before initial apply). Build one.
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastStatus.SnapshotAt.IsZero() {
		snap, _ := c.ifaceMgr.Snapshot()
		st = StatusResponse{
			Interfaces:    snap.Interfaces,
			ActiveProfile: c.cfg.ActiveProfileOrDefault(),
			SnapshotAt:    snap.TakenAt,
		}
		c.setStatus(st)
	}
	return c.lastStatus
}

// SubscribeStatus registers cb for status events (apply completions). The
// returned function unsubscribes. Used by the IPC layer (§9 SubscribeStatus).
func (c *Core) SubscribeStatus(cb func(StatusResponse)) func() {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	if c.statusCbs == nil {
		c.statusCbs = make(map[int64]func(StatusResponse))
	}
	id := c.statusNextID
	c.statusNextID++
	c.statusCbs[id] = cb
	return func() {
		c.statusMu.Lock()
		delete(c.statusCbs, id)
		c.statusMu.Unlock()
	}
}

func (c *Core) setStatus(st StatusResponse) {
	c.statusMu.Lock()
	c.lastStatus = st
	c.statusMu.Unlock()
}

func (c *Core) emitStatus(st StatusResponse) {
	c.statusMu.RLock()
	cbs := make([]func(StatusResponse), 0, len(c.statusCbs))
	for _, cb := range c.statusCbs {
		cbs = append(cbs, cb)
	}
	c.statusMu.RUnlock()
	for _, cb := range cbs {
		go func(f func(StatusResponse)) { f(st) }(cb)
	}
}
