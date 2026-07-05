// Package appapi is the Wails-bound surface the frontend talks to.
//
// Architecture (post-refactor): the route engine runs IN the GUI process —
// appapi holds a *core.Core directly and calls its methods. No Windows
// service, no named-pipe IPC. The GUI process must be elevated to modify
// routes (route.exe needs admin); if it isn't, appapi runs read-only and the
// frontend offers to relaunch elevated. Auto-start at login is configured via
// a scheduled task (Task Scheduler, "highest privileges") so a normal login
// relaunches the elevated GUI without a UAC prompt.
package appapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/netswitcher/netswitcher/internal/config"
	"github.com/netswitcher/netswitcher/internal/conflict"
	"github.com/netswitcher/netswitcher/internal/core"
	"github.com/netswitcher/netswitcher/internal/diag"
	"github.com/netswitcher/netswitcher/internal/logging"
	"github.com/netswitcher/netswitcher/internal/paths"
	"github.com/netswitcher/netswitcher/internal/routeengine"
	"github.com/netswitcher/netswitcher/internal/routeread"
	"github.com/netswitcher/netswitcher/internal/tray"
	"github.com/netswitcher/netswitcher/pkg/winutil"
)

// Event names emitted to the frontend.
const (
	EventDiagLine  = "diag:line"
	EventDiagEnd   = "diag:end"
	EventDiagError = "diag:error"
	EventLogLine   = "log:line"
	EventLogEnd    = "log:end"
	EventStatus    = "status:changed"
)

// AutoStartTaskName is the Task Scheduler task used for "launch elevated at
// logon without a UAC prompt".
const AutoStartTaskName = "NetSwitcher"

// API is constructed once and bound to the Wails app.
type API struct {
	ctx       context.Context
	core      *core.Core
	log       *slog.Logger
	elevated  bool
	logFan    *logFanout
	IconBytes []byte      // tray icon (.ico); set by the GUI layer before OnStartup
	Version   string      // build version; set by the GUI layer
	quitting  atomic.Bool // true once a real quit is in progress (tray → 退出 / RelaunchElevated)

	mu     sync.Mutex
	cancel context.CancelFunc // cancels any active diag stream
}

// IsQuitting reports whether the app is in the middle of a real shutdown.
// OnBeforeClose uses this to distinguish "user clicked X → hide" from
// "tray → 退出 → let the close proceed".
func (a *API) IsQuitting() bool { return a.quitting.Load() }

// New returns an API. Whether the engine can actually modify routes depends
// on IsElevated() — the frontend must guard non-elevated runs.
func New() *API {
	return &API{
		log:      slog.Default(),
		elevated: winutil.IsElevated(),
		logFan:   newLogFanout(),
	}
}

// OnStartup is called by Wails with the runtime context.
func (a *API) OnStartup(ctx context.Context) {
	a.ctx = ctx
	// File + stdout logging always (so logs work even when non-elevated).
	logDir, _ := paths.LogDir()
	_, _ = logging.Configure("info", logDir)
	logging.SetPipeSink(a.logFan)

	go a.subscribeStatusLoop(ctx)
	// Second-instance signal: when another copy launches, it sets the show
	// event; bring this window to the front.
	go func() {
		ch := winutil.WaitSingletonShow()
		for range ch {
			if a.ctx != nil {
				runtime.WindowShow(a.ctx)
			}
		}
	}()
	if len(a.IconBytes) > 0 {
		go tray.Run(a.IconBytes, a.showWindow, a.applyNow, a.quitApp)
	}
	if a.elevated {
		a.startEngine()
	}
}

// startEngine brings up the in-process route engine (core). Idempotent.
func (a *API) startEngine() {
	cfgPath, _ := paths.ConfigPath()
	statePath, _ := paths.StatePath()
	logDir, _ := paths.LogDir()

	c, err := core.New(core.Options{
		ConfigPath: cfgPath,
		StatePath:  statePath,
		LogLevel:   "info",
		LogDir:     logDir,
	}, a.log)
	if err != nil {
		a.log.Error("core init failed", "err", err)
		return
	}
	if err := c.Start(); err != nil {
		a.log.Error("core start failed", "err", err)
		return
	}
	a.core = c
	// Apply the persisted log level from config (if any) so the Settings
	// choice survives restarts.
	if lvl := c.Config().LogLevel; lvl != "" {
		logging.SetLevel(lvl)
	}
	a.log.Info("embedded engine started (elevated)", "pid", os.Getpid())
}

// ---------- Status / elevation ----------

// IsElevated reports whether the GUI process has admin rights (and thus can
// modify routes). The frontend shows a relaunch prompt when false.
func (a *API) IsElevated() bool { return a.elevated }

// EngineActive reports whether the embedded route engine is running.
func (a *API) EngineActive() bool { return a.core != nil }

// RelaunchElevated re-launches the binary (GUI mode) with a UAC prompt and
// quits the current (non-elevated) instance so there's only one window.
func (a *API) RelaunchElevated() error {
	if err := winutil.RelaunchElevated(""); err != nil {
		return err
	}
	// The elevated instance is starting via UAC; let this one close so we
	// don't end up with two windows. Arming quitting bypasses OnBeforeClose.
	a.quitting.Store(true)
	if a.ctx != nil {
		go func() {
			<-time.After(500 * time.Millisecond)
			runtime.Quit(a.ctx)
		}()
	}
	return nil
}

// ---------- Single-shot methods (all require the engine) ----------

// GetStatus returns the current snapshot + last apply + conflicts.
func (a *API) GetStatus() (core.StatusResponse, error) {
	if a.core == nil {
		return core.StatusResponse{}, errEngine
	}
	return a.core.Status(), nil
}

// GetConfig returns the full config document.
func (a *API) GetConfig() (config.Config, error) {
	if a.core == nil {
		return config.Config{}, errEngine
	}
	cfg := *a.core.Config()
	if cfg.Profiles == nil {
		cfg.Profiles = []config.Profile{} // non-null so the frontend gets [] not null
	}
	return cfg, nil
}

// SaveConfig replaces the whole config (with validation) and re-applies.
func (a *API) SaveConfig(cfg config.Config) error {
	if a.core == nil {
		return errEngine
	}
	return a.core.SaveConfig(&cfg)
}

// SaveProfile is the GUI convenience: load → replace/insert profile → save.
func (a *API) SaveProfile(p config.Profile) error {
	if a.core == nil {
		return errEngine
	}
	cfg := *a.core.Config()
	replaced := false
	for i := range cfg.Profiles {
		if cfg.Profiles[i].ID == p.ID {
			cfg.Profiles[i] = p
			replaced = true
			break
		}
	}
	if !replaced {
		cfg.Profiles = append(cfg.Profiles, p)
	}
	return a.core.SaveConfig(&cfg)
}

// DeleteProfile removes a profile by id.
func (a *API) DeleteProfile(id string) error {
	if a.core == nil {
		return errEngine
	}
	cfg := *a.core.Config()
	out := cfg.Profiles[:0]
	for _, p := range cfg.Profiles {
		if p.ID != id {
			out = append(out, p)
		}
	}
	cfg.Profiles = out
	if cfg.ActiveProfile == id && len(cfg.Profiles) > 0 {
		cfg.ActiveProfile = cfg.Profiles[0].ID
	} else if cfg.ActiveProfile == id {
		cfg.ActiveProfile = ""
	}
	return a.core.SaveConfig(&cfg)
}

// SetActiveProfile switches the active profile and triggers an apply.
func (a *API) SetActiveProfile(id string) error {
	if a.core == nil {
		return errEngine
	}
	return a.core.SetActiveProfile(id)
}

// ApplyNow forces a re-apply and returns the result.
func (a *API) ApplyNow() (routeengine.ApplyResult, error) {
	if a.core == nil {
		return routeengine.ApplyResult{}, errEngine
	}
	return a.core.ApplyOnce("gui"), nil
}

// RouteRow is one row of the Routes page, tagged with its likely source.
type RouteRow struct {
	DestinationPrefix string `json:"destinationPrefix"`
	NextHop           string `json:"nextHop"`
	InterfaceIndex    int    `json:"interfaceIndex"`
	InterfaceAlias    string `json:"interfaceAlias"`
	RouteMetric       int    `json:"routeMetric"`
	InterfaceMetric   int    `json:"interfaceMetric"`
	Source            string `json:"source"`
}

// GetRouteTable returns the live route table with per-row source tags.
func (a *API) GetRouteTable() ([]RouteRow, error) {
	rows, err := routeread.Read(a.ctx)
	if err != nil {
		return nil, fmt.Errorf("read route table: %w", err)
	}
	var managed []struct{ Destination string }
	if a.core != nil {
		for _, e := range a.core.ManagedRoutes() {
			managed = append(managed, struct{ Destination string }{e.Destination})
		}
	}
	managedSet := make(map[string]bool, len(managed))
	for _, m := range managed {
		managedSet[m.Destination] = true
	}

	st := core.StatusResponse{}
	if a.core != nil {
		st = a.core.Status()
	}
	vpnIdx := make(map[int]bool)
	for _, ifc := range st.Interfaces {
		if conflict.IsVPNInterface(ifc) {
			vpnIdx[ifc.Index] = true
		}
	}

	out := make([]RouteRow, 0, len(rows))
	for _, r := range rows {
		src := string(routeread.SourceSystem)
		if managedSet[r.DestinationPrefix] {
			src = string(routeread.SourceManaged)
		} else if vpnIdx[r.InterfaceIndex] {
			src = string(routeread.SourceSuspect)
		}
		out = append(out, RouteRow{
			DestinationPrefix: r.DestinationPrefix,
			NextHop:           r.NextHop,
			InterfaceIndex:    r.InterfaceIndex,
			InterfaceAlias:    r.InterfaceAlias,
			RouteMetric:       r.RouteMetric,
			InterfaceMetric:   r.InterfaceMetric,
			Source:            src,
		})
	}
	return out, nil
}

// ---------- Streaming diagnostics ----------

// Ping streams ping.exe output as EventDiagLine, then EventDiagEnd.
func (a *API) Ping(target string) error { return a.runDiag(true, target) }

// Tracert streams tracert.exe output, same event protocol as Ping.
func (a *API) Tracert(target string) error { return a.runDiag(false, target) }

// StopDiag cancels an in-flight ping/tracert.
func (a *API) StopDiag() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
}

func (a *API) runDiag(isPing bool, target string) error {
	a.StopDiag()
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()

	run := diag.Tracert
	if isPing {
		run = diag.Ping
	}
	go func() {
		defer cancel()
		a.mu.Lock()
		a.cancel = nil
		a.mu.Unlock()
		err := run(ctx, target, func(line string) error {
			return a.emitErr(EventDiagLine, line)
		})
		if err != nil {
			a.emit(EventDiagError, err.Error())
		}
		a.emit(EventDiagEnd, nil)
	}()
	return nil
}

// ---------- Log streaming ----------

// SubscribeLogs pushes each matching log line as EventLogLine, forever.
func (a *API) SubscribeLogs(level string) error {
	go a.subscribeLogsLoop(level)
	return nil
}

func (a *API) subscribeLogsLoop(level string) {
	lvl := logging.LevelFromString(level)
	id, ch := a.logFan.subscribe(lvl)
	defer a.logFan.unsubscribe(id)
	for line := range ch {
		if a.emitErr(EventLogLine, string(line)) != nil {
			return
		}
	}
}

// ---------- Status push ----------

func (a *API) subscribeStatusLoop(ctx context.Context) {
	if a.core == nil {
		return
	}
	events := make(chan core.StatusResponse, 32)
	unsub := a.core.SubscribeStatus(func(st core.StatusResponse) {
		select {
		case events <- st:
		default:
		}
	})
	defer unsub()
	for {
		select {
		case <-ctx.Done():
			return
		case st, ok := <-events:
			if !ok {
				return
			}
			a.emit(EventStatus, st)
		}
	}
}

// ---------- Tray callbacks ----------

func (a *API) showWindow() {
	if a.ctx != nil {
		runtime.WindowShow(a.ctx)
	}
}

func (a *API) applyNow() {
	if _, err := a.ApplyNow(); err != nil {
		a.log.Warn("tray apply-now failed", "err", err)
	}
}

// quitApp is the tray "退出" handler: arm the quitting flag so OnBeforeClose
// stops blocking, then ask Wails to shut down. The process actually exits
// when wails.Run returns in gui.Run.
func (a *API) quitApp() {
	a.quitting.Store(true)
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}

// ---------- Auto-start (Task Scheduler) ----------

// AppInfo describes the running build for the Settings/About section.
type AppInfo struct {
	Version    string `json:"version"`
	Elevated   bool   `json:"elevated"`
	EngineOn   bool   `json:"engineOn"`
	ConfigPath string `json:"configPath"`
	StatePath  string `json:"statePath"`
	LogDir     string `json:"logDir"`
	PipeName   string `json:"pipeName"`
}

// GetAppInfo returns build + runtime paths for the Settings page.
func (a *API) GetAppInfo() AppInfo {
	cfgPath, _ := paths.ConfigPath()
	statePath, _ := paths.StatePath()
	logDir, _ := paths.LogDir()
	return AppInfo{
		Version:    a.Version,
		Elevated:   a.elevated,
		EngineOn:   a.core != nil,
		ConfigPath: cfgPath,
		StatePath:  statePath,
		LogDir:     logDir,
	}
}

// GetLogLevel returns the active log level (debug/info/warn/error).
func (a *API) GetLogLevel() string {
	return logging.ActiveLevel()
}

// SetLogLevel changes the log level at runtime AND persists it to config.json
// so it survives restarts.
func (a *API) SetLogLevel(level string) error {
	logging.SetLevel(level)
	if a.core == nil {
		return nil // can't persist without core; runtime change still applied
	}
	cfg := *a.core.Config()
	cfg.LogLevel = level
	return a.core.SaveConfig(&cfg)
}

// OpenLogFolder opens Explorer at the log directory.
func (a *API) OpenLogFolder() error {
	logDir, err := paths.LogDir()
	if err != nil {
		return err
	}
	cmd := exec.Command("explorer.exe", logDir)
	winutil.HideWindow(cmd)
	return cmd.Start()
}

// ---------- Auto-start (Task Scheduler) ----------

// schtasksPath is the absolute path to schtasks.exe.
const schtasksPath = "schtasks.exe"

// AutoStartInstalled reports whether the logon auto-start task exists.
func (a *API) AutoStartInstalled() bool {
	cmd := exec.Command(schtasksPath, "/Query", "/TN", AutoStartTaskName)
	winutil.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return !containsAny(string(out), []string{"cannot find", "找不到", "无法找到"})
}

// InstallAutoStart creates the logon auto-start task (must be elevated).
// The task runs the current exe (no args → GUI) at user logon with highest
// privileges, so the GUI comes back elevated after login without a UAC prompt.
func (a *API) InstallAutoStart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if !a.elevated {
		return fmt.Errorf("需要管理员权限来配置开机自启")
	}
	// /rl HIGHEST → run elevated; /sc ONLOGON → at logon; /f → overwrite.
	cmd := exec.Command(schtasksPath,
		"/Create", "/F",
		"/TN", AutoStartTaskName,
		"/TR", "\""+exe+"\"",
		"/SC", "ONLOGON",
		"/RL", "HIGHEST",
	)
	winutil.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks: %s: %w", string(out), err)
	}
	return nil
}

// UninstallAutoStart removes the auto-start task.
func (a *API) UninstallAutoStart() error {
	cmd := exec.Command(schtasksPath, "/Delete", "/F", "/TN", AutoStartTaskName)
	winutil.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil && !containsAny(string(out), []string{"cannot find", "找不到", "无法找到"}) {
		return fmt.Errorf("schtasks: %s: %w", string(out), err)
	}
	return nil
}

// ---------- helpers ----------

var errEngine = fmt.Errorf("路由引擎未运行：请以管理员身份重启 NetSwitcher")

// emit publishes an event to the frontend. Safe before OnStartup (drops).
func (a *API) emit(name string, data any) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, name, data)
}

// emitErr emits and returns the write error (used to stop a stream when the
// window is closed).
func (a *API) emitErr(name string, data any) error {
	if a.ctx == nil {
		return fmt.Errorf("no context")
	}
	runtime.EventsEmit(a.ctx, name, data)
	return nil
}

func containsAny(s string, markers []string) bool {
	for _, m := range markers {
		if len(m) > 0 && len(s) >= len(m) {
			for i := 0; i+len(m) <= len(s); i++ {
				if s[i:i+len(m)] == m {
					return true
				}
			}
		}
	}
	return false
}

// logFanout fans slog records to SubscribeLogs subscribers. Same shape as the
// IPC server's fanout, but in-process.
type logFanout struct {
	mu     sync.RWMutex
	subs   map[int]*logSub
	nextID atomic.Int64
}

type logSub struct {
	level slog.Level
	ch    chan []byte
}

func newLogFanout() *logFanout { return &logFanout{subs: make(map[int]*logSub)} }

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
	delete(f.subs, id)
	f.mu.Unlock()
}

func extractLevel(p []byte) slog.Level {
	var probe struct {
		Level string `json:"level"`
	}
	if json.Unmarshal(p, &probe) != nil {
		return slog.LevelInfo
	}
	return logging.LevelFromString(probe.Level)
}
