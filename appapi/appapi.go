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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/netswitcher/netswitcher/internal/updater"
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
	Minimized bool        // auto-start: start hidden in the tray (no window flash on login)
	quitting  atomic.Bool // true once a real quit is in progress (tray → 退出 / RelaunchElevated)

	mu     sync.Mutex
	cancel context.CancelFunc // cancels any active diag stream

	updateCancel context.CancelFunc // cancels an in-flight PerformUpdate (nil if none)
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
	// (Window hiding on --minimized is handled by Wails' StartHidden option in
	// gui_cgo.go — runtime.WindowHide here gets overridden by Wails' ShowWindow
	// which runs AFTER OnStartup.)
	// File + stdout logging always (so logs work even when non-elevated).
	logDir, _ := paths.LogDir()
	_, _ = logging.Configure("info", logDir)
	logging.SetPipeSink(a.logFan)
	// Record how this instance was launched — the key diagnostic for the
	// "以管理员身份重启不生效" loop: shows whether --takeover was forwarded and
	// whether the process is elevated.
	a.log.Info("onstartup", "elevated", a.elevated, "pid", os.Getpid(), "args", os.Args)

	go func() {
		// Second-instance signal: when another copy launches, it sets the show
		// event; bring this window to the front.
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
	// Silent startup update check — surfaces a top-bar badge if a newer
	// release exists. Network failures/dev builds/no-update stay quiet.
	go a.startUpdateCheck()
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
	// Subscribe to status pushes now that core exists (must run AFTER a.core
	// is set, else the goroutine races OnStartup and sees nil).
	if a.ctx != nil {
		go a.subscribeStatusLoop(a.ctx)
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
	// Drop the single-instance lock IMMEDIATELY. We've committed to exit; the
	// elevated instance starting via UAC must be able to acquire it without
	// racing our shutdown. Without this the new process starts its takeover
	// loop, then disappears when we exit — leaving no GUI.
	winutil.ReleaseSingleton()
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
	// Build a NEW slice — don't reuse the backing array (cfg.Profiles[:0]
	// would share memory with the live config, risking a race if applyOnce
	// reads it concurrently).
	var out []config.Profile
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

// DeactivateProfile clears the active profile — no profile active means the
// engine removes any previously-managed routes and adds nothing. System/DHCP/
// VPN routes are left as-is. The frontend "停用" button uses this.
func (a *API) DeactivateProfile() error {
	if a.core == nil {
		return errEngine
	}
	return a.core.Deactivate()
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

// GetDefaultRouteInterface returns the interface alias of the system's current
// default route (0.0.0.0/0 with lowest combined metric). Used by the Profiles
// overview to show "系统默认（WLAN）" when the user hasn't configured
// defaultRouteInterface. Returns "" if none found.
func (a *API) GetDefaultRouteInterface() string {
	rows, err := routeread.Read(a.ctx)
	if err != nil {
		return ""
	}
	var bestAlias string
	var bestMetric int = 1<<31 - 1
	for _, r := range rows {
		if r.DestinationPrefix == "0.0.0.0/0" && r.NextHop != "0.0.0.0" && r.NextHop != "" {
			total := r.RouteMetric + r.InterfaceMetric
			if total < bestMetric {
				bestMetric = total
				bestAlias = r.InterfaceAlias
			}
		}
	}
	return bestAlias
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

// IsMaximised reports the window's maximize state so the frontend can show a
// restore icon instead of maximize when already maximized.
func (a *API) IsMaximised() bool {
	if a.ctx == nil {
		return false
	}
	return runtime.WindowIsMaximised(a.ctx)
}

func (a *API) applyNow() {
	if _, err := a.ApplyNow(); err != nil {
		a.log.Warn("tray apply-now failed", "err", err)
	}
}

// Quit is the frontend-facing exit (the elevation modal's "退出" button).
// Same as the tray quit: arm quitting so OnBeforeClose doesn't minimize to
// tray, then runtime.Quit.
func (a *API) Quit() {
	a.quitApp()
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

// ---------- Update check ----------

// UpdateInfo is the result of a check-for-updates probe against GitHub
// Releases. Surfaced on the Settings page's "关于" section.
type UpdateInfo struct {
	CurrentVersion string `json:"currentVersion"` // build version (main.version)
	LatestVersion  string `json:"latestVersion"`  // latest release tag, e.g. "v1.2.0"
	HasUpdate      bool   `json:"hasUpdate"`      // true only for release builds behind latest
	IsDevBuild     bool   `json:"isDevBuild"`     // true when current isn't a clean vX.Y.Z
	ReleaseURL     string `json:"releaseURL"`     // HTML page for the release
	ReleaseNotes   string `json:"releaseNotes"`   // release body (markdown)
	PublishedAt    string `json:"publishedAt"`    // RFC3339, empty if unknown
	ZipURL         string `json:"zipURL"`         // portable-zip asset URL (for the future one-click upgrade)
	// Error is empty on success; on failure a user-facing message. ErrorKind
	// categorizes the cause. Empty ErrorKind == success.
	Error     string `json:"error"`
	ErrorKind string `json:"errorKind"` // "" | "network" | "notfound" | "http" | "parse" | "unknown"
}

// CheckUpdate queries GitHub for the latest release and compares it to the
// running build. Dev builds (anything that isn't a clean vX.Y.Z tag — e.g.
// "v0.1.0-3-gabcdef", "v0.1.0-dirty", "dev", a bare hash) report
// IsDevBuild=true and skip the comparison, so the frontend shows "开发版本"
// instead of nagging on every Settings open. Network-bound; bounded to 15s.
//
// Never returns a Go error: on failure the cause is categorized (see
// updater.ErrorKind) and surfaced inside UpdateInfo, so the frontend shows a
// localized message in the 关于 card rather than a raw English error string.
func (a *API) CheckUpdate() (UpdateInfo, error) {
	info := UpdateInfo{
		CurrentVersion: a.Version,
		IsDevBuild:     !updater.IsReleaseBuild(a.Version),
	}

	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	rel, err := updater.FetchLatest(ctx, updater.RepoAPI)
	if err != nil {
		kind := kindOf(err)
		info.ErrorKind = string(kind)
		info.Error = friendlyUpdateMessage(kind)
		a.log.Warn("check update failed", "kind", kind, "err", err)
		return info, nil
	}

	info.LatestVersion = rel.TagName
	info.ReleaseURL = rel.HTMLURL
	info.ReleaseNotes = rel.Body
	info.ZipURL = rel.ZipURL
	if !rel.PublishedAt.IsZero() {
		info.PublishedAt = rel.PublishedAt.Format(time.RFC3339)
	}
	// Only compare for release builds; dev builds always read HasUpdate=false so
	// developers running local builds aren't told their build is "behind".
	if !info.IsDevBuild {
		info.HasUpdate = updater.HasNewer(a.Version, rel.TagName)
	}
	return info, nil
}

// startUpdateCheck runs once at startup to surface a "new version available"
// top-bar badge without user action. Silent: on network error, dev build, or
// no update it emits nothing (no nag). Only emits "update:available" when a
// real release build is behind latest.
func (a *API) startUpdateCheck() {
	// Let the frontend wire up its event listener before we emit.
	time.Sleep(3 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rel, err := updater.FetchLatest(ctx, updater.RepoAPI)
	if err != nil {
		return
	}
	info := UpdateInfo{
		CurrentVersion: a.Version,
		LatestVersion:  rel.TagName,
		ReleaseURL:     rel.HTMLURL,
		ReleaseNotes:   rel.Body,
		ZipURL:         rel.ZipURL,
		IsDevBuild:     !updater.IsReleaseBuild(a.Version),
	}
	if !info.IsDevBuild {
		info.HasUpdate = updater.HasNewer(a.Version, rel.TagName)
	}
	if info.HasUpdate {
		a.emit("update:available", info)
	}
}

// friendlyUpdateMessage maps an updater error kind to a user-facing string.
// Kept in the backend so the localized message is consistent and the raw
// English error never reaches the UI.
func friendlyUpdateMessage(kind updater.ErrorKind) string {
	switch kind {
	case updater.ErrNetwork:
		return "无法连接 GitHub，请检查网络后重试。"
	case updater.ErrNotFound:
		return "尚未发布任何版本（或仓库地址有误）。"
	case updater.ErrParse:
		return "版本信息解析失败，请稍后重试。"
	case updater.ErrHTTP:
		return "GitHub 服务异常，请稍后重试。"
	default:
		return "检查更新失败，请稍后重试。"
	}
}

// kindOf extracts the ErrorKind from a FetchError, defaulting to ErrUnknown
// for anything else.
func kindOf(err error) updater.ErrorKind {
	if fe, ok := err.(*updater.FetchError); ok {
		return fe.Kind
	}
	return updater.ErrUnknown
}

// PerformUpdate kicks off a background upgrade: download the latest release,
// extract the new exe, and arm a detached helper batch to swap it in once this
// process exits. Requires elevation.
//
// It returns immediately — the actual work runs in a goroutine (see
// performUpdateAsync) because Wails buffers events emitted during a bound
// method's synchronous run until that method returns. Progress + completion
// are streamed via the "update:progress" event (stages: preparing →
// downloading [with bytes] → installing → armed | failed). The frontend
// listens for "armed" to know it should Quit() so the swap can complete.
func (a *API) PerformUpdate() error {
	if !a.elevated {
		return fmt.Errorf("需要管理员权限才能自动升级，请以管理员身份重启后再试")
	}
	if a.ctx == nil {
		return fmt.Errorf("尚未就绪，请稍后重试")
	}
	go a.performUpdateAsync()
	return nil
}

// CancelUpdate aborts an in-flight upgrade (download phase). No-op if none is
// running or the helper batch is already armed — once armed, the process must
// quit for the swap to complete, so there's no calling it off.
func (a *API) CancelUpdate() {
	a.mu.Lock()
	if a.updateCancel != nil {
		a.updateCancel()
	}
	a.mu.Unlock()
}

// performUpdateAsync does the real work on a background goroutine so emitted
// progress events reach the frontend live (not buffered behind a bound call).
// Terminal events: "armed" (success, frontend should Quit) or "failed" (with
// an "error" field).
func (a *API) performUpdateAsync() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	a.mu.Lock()
	a.updateCancel = cancel
	a.mu.Unlock()
	defer func() {
		cancel()
		a.mu.Lock()
		a.updateCancel = nil
		a.mu.Unlock()
	}()

	emit := func(stage string, extra map[string]any) {
		if a.ctx == nil {
			return
		}
		payload := map[string]any{"stage": stage}
		for k, v := range extra {
			payload[k] = v
		}
		runtime.EventsEmit(a.ctx, "update:progress", payload)
	}
	fail := func(msg string) {
		a.log.Warn("update failed", "msg", msg)
		emit("failed", map[string]any{"error": msg})
	}

	emit("preparing", nil)
	rel, err := updater.FetchLatest(ctx, updater.RepoAPI)
	if err != nil {
		if ctx.Err() != nil {
			return // cancelled
		}
		fail(friendlyUpdateMessage(kindOf(err)))
		return
	}
	if rel.ZipURL == "" {
		fail("该版本未提供可下载的升级包")
		return
	}
	// Guard against direct IPC calls that bypass the UI's hasUpdate gate: only
	// upgrade a release build that's actually behind latest.
	if !updater.IsReleaseBuild(a.Version) {
		fail("开发版本无法自动升级，请前往发布页手动下载")
		return
	}
	if !updater.HasNewer(a.Version, rel.TagName) {
		fail("当前已是最新版本，无需升级")
		return
	}

	emit("downloading", nil)
	tmpDir, err := os.MkdirTemp("", "ns-update-*")
	if err != nil {
		fail("创建临时目录失败")
		return
	}
	newExe, err := updater.DownloadAndExtract(ctx, rel.ZipURL, tmpDir, func(downloaded, total int64) {
		emit("downloading", map[string]any{
			"downloaded": downloaded,
			"total":      total,
		})
	})
	if err != nil {
		if ctx.Err() != nil {
			return // cancelled
		}
		fail("下载升级包失败，请检查网络后重试")
		return
	}
	// Sanity-check: a sub-MB exe means the download is broken or malicious.
	if info, e := os.Stat(newExe); e != nil || info.Size() < 1<<20 {
		fail("下载的升级包异常，请稍后重试")
		return
	}

	emit("installing", nil)
	if err := updater.ReplaceAndRestart(newExe); err != nil {
		fail("启动升级失败：" + err.Error())
		return
	}
	a.log.Info("update armed; quitting to let helper complete the swap", "new", newExe)
	emit("armed", nil)
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
	return winutil.ShellOpen(logDir)
}

// OpenURL opens a URL in the system's default browser. Used by the update
// check to send the user to the GitHub release page.
func (a *API) OpenURL(url string) error {
	return winutil.ShellOpen(url)
}

// RecentLogs returns up to the last n JSON log lines from the log file, oldest
// first. Used by the Logs page to show history on open (live subscription only
// covers logs emitted AFTER opening). Reads backward in chunks so a 50MB log
// doesn't get loaded whole.
func (a *API) RecentLogs(n int) ([]string, error) {
	if n <= 0 {
		n = 500
	}
	p, err := paths.LogPath()
	if err != nil {
		return nil, err
	}
	return tailLines(p, n)
}

// tailLines reads the last <=n newline-delimited records from path, returning
// them in chronological order (oldest of the set first).
func tailLines(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err // file may not exist yet — caller treats as empty
	}
	defer f.Close()

	const chunk = 8192
	var collected []string // newest-first as we walk backward
	tail := []byte(nil)    // bytes carried into the previous (earlier) chunk
	offset, _ := f.Seek(0, io.SeekEnd)

	for offset > 0 {
		read := int64(chunk)
		if offset < read {
			read = offset
		}
		offset -= read
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil, err
		}
		buf := make([]byte, read)
		if _, err := io.ReadFull(f, buf); err != nil {
			return nil, err
		}
		data := append(buf, tail...)
		// Split on newlines. parts[0] is the partial line continuing into an
		// earlier chunk; parts[1:] are complete lines (in file order).
		parts := bytes.Split(data, []byte("\n"))
		tail = parts[0]
		for i := len(parts) - 1; i >= 1; i-- {
			if len(bytes.TrimSpace(parts[i])) > 0 {
				collected = append(collected, string(parts[i]))
				if len(collected) >= n {
					out := make([]string, len(collected))
					for i, s := range collected {
						out[len(out)-1-i] = s // reverse to chronological
					}
					return out, nil
				}
			}
		}
	}
	if len(bytes.TrimSpace(tail)) > 0 {
		collected = append(collected, string(tail))
	}
	out := make([]string, len(collected))
	for i, s := range collected {
		out[len(out)-1-i] = s
	}
	return out, nil
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
		"/TR", "\""+exe+"\" --minimized",
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
