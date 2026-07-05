// Package appapi is the Wails-bound surface the frontend talks to. Per the
// spec (§8.2) the GUI process never calls route.exe directly — every method
// proxies to the running service via the IPC client.
//
// Streaming methods (Ping / Tracert / SubscribeLogs) launch a background
// reader that pushes lines to the frontend via Wails events; the frontend
// subscribes with the events API. This avoids returning Go channels across
// the JS bridge.
package appapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/netswitcher/netswitcher/internal/config"
	"github.com/netswitcher/netswitcher/internal/core"
	"github.com/netswitcher/netswitcher/internal/ipc"
	"github.com/netswitcher/netswitcher/internal/logging"
	"github.com/netswitcher/netswitcher/internal/routeengine"
	svcwrap "github.com/netswitcher/netswitcher/internal/service"
	"github.com/netswitcher/netswitcher/internal/tray"
	"github.com/netswitcher/netswitcher/pkg/winutil"
)

// Event names emitted to the frontend.
const (
	EventDiagLine  = "diag:line"  // one output line from ping/tracert
	EventDiagEnd   = "diag:end"   // stream finished
	EventDiagError = "diag:error" // stream-level error
	EventLogLine   = "log:line"   // one slog JSON record
	EventLogEnd    = "log:end"
	EventStatus    = "status:changed" // pushed status snapshot
)

// API is constructed once and bound to the Wails app.
type API struct {
	ctx       context.Context
	client    *ipc.Client
	log       *slog.Logger
	mu        sync.Mutex
	cancel    context.CancelFunc // cancels any active diag/log stream
	IconBytes []byte             // tray icon (.ico); set by the GUI layer before Start
}

// New returns an API bound to the default named pipe.
func New() *API {
	return &API{
		client: ipc.NewClient(),
		log:    slog.Default(),
	}
}

// OnStartup is called by Wails with the runtime context. Required so we can
// emit events and use the runtime API.
func (a *API) OnStartup(ctx context.Context) {
	a.ctx = ctx
	// Push status updates from the service to the frontend in the background.
	go a.subscribeStatusLoop(ctx)
	// System tray (X button hides the window; tray icon is the way back).
	if len(a.IconBytes) > 0 {
		go tray.Run(a.IconBytes, a.showWindow, a.applyNow, a.quitApp)
	}
}

// showWindow brings the hidden window back to the foreground.
func (a *API) showWindow() {
	if a.ctx == nil {
		return
	}
	runtime.WindowShow(a.ctx)
}

// applyNow triggers a re-apply (tray menu convenience).
func (a *API) applyNow() {
	if _, err := a.ApplyNow(); err != nil {
		a.log.Warn("tray apply-now failed", "err", err)
	}
}

// quitApp exits the whole GUI process. The service is independent and keeps
// running (this only quits the desktop app).
func (a *API) quitApp() {
	if a.ctx == nil {
		return
	}
	runtime.Quit(a.ctx)
}

// ---------- Service availability ----------

// ServiceAvailable returns whether the IPC service is reachable. The frontend
// uses this to toggle the "service not running" banner.
func (a *API) ServiceAvailable() bool {
	_, err := a.client.Call(ipc.MethodGetStatus, struct{}{})
	return err == nil
}

// ServiceInstalled reports whether the service is registered with SCM
// (independent of whether it's currently running). Used by the banner to pick
// the right button label: "安装并启动" vs "启动". Querying SCM status does not
// require elevation.
func (a *API) ServiceInstalled() bool {
	st, err := svcwrap.Query()
	if err != nil {
		return false
	}
	return st.Installed
}

// ---------- Service control ----------

// StartServiceElevated triggers a UAC prompt to run `netswitcher.exe service
// start`. The frontend calls this from the "service not running" banner.
func (a *API) StartServiceElevated() error {
	return winutil.StartServiceElevated("")
}

// ---------- Single-shot methods ----------

// GetStatus returns the current snapshot + last apply + conflicts.
func (a *API) GetStatus() (core.StatusResponse, error) {
	var st core.StatusResponse
	if err := a.client.CallJSON(ipc.MethodGetStatus, struct{}{}, &st); err != nil {
		return core.StatusResponse{}, err
	}
	return st, nil
}

// GetConfig returns the full config document.
func (a *API) GetConfig() (config.Config, error) {
	var cfg config.Config
	if err := a.client.CallJSON(ipc.MethodGetConfig, struct{}{}, &cfg); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

// SaveConfig replaces the whole config (with validation).
func (a *API) SaveConfig(cfg config.Config) error {
	_, err := a.client.Call(ipc.MethodSaveConfig, map[string]any{"config": cfg})
	return err
}

// SaveProfile is the GUI convenience (§8.2): load → replace/insert profile → save.
func (a *API) SaveProfile(p config.Profile) error {
	cfg, err := a.GetConfig()
	if err != nil {
		return err
	}
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
	return a.SaveConfig(cfg)
}

// DeleteProfile removes a profile by id.
func (a *API) DeleteProfile(id string) error {
	cfg, err := a.GetConfig()
	if err != nil {
		return err
	}
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
	return a.SaveConfig(cfg)
}

// SetActiveProfile switches the active profile and triggers an apply.
func (a *API) SetActiveProfile(id string) error {
	_, err := a.client.Call(ipc.MethodSetActiveProfile, map[string]any{"id": id})
	return err
}

// ApplyNow forces a re-apply and returns the result.
func (a *API) ApplyNow() (routeengine.ApplyResult, error) {
	var r routeengine.ApplyResult
	if err := a.client.CallJSON(ipc.MethodApplyNow, struct{}{}, &r); err != nil {
		return routeengine.ApplyResult{}, err
	}
	return r, nil
}

// RouteRow is one row of the Routes page, tagged with its likely source.
type RouteRow struct {
	DestinationPrefix string `json:"destinationPrefix"`
	NextHop           string `json:"nextHop"`
	InterfaceIndex    int    `json:"interfaceIndex"`
	InterfaceAlias    string `json:"interfaceAlias"`
	RouteMetric       int    `json:"routeMetric"`
	InterfaceMetric   int    `json:"interfaceMetric"`
	Source            string `json:"source"` // managed | system | suspect
}

// GetRouteTable returns the live route table with per-row source tags.
func (a *API) GetRouteTable() ([]RouteRow, error) {
	raw, err := a.client.Call(ipc.MethodGetRouteTable, struct{}{})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Rows []RouteRow `json:"rows"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode route table: %w", err)
	}
	return resp.Rows, nil
}

// ---------- Streaming diagnostics ----------

// Ping streams ping.exe output to the frontend via EventDiagLine events,
// then emits EventDiagEnd. Cancels any prior active diagnostic.
func (a *API) Ping(target string) error {
	return a.runDiag(ipc.MethodPing, target)
}

// Tracert streams tracert.exe output, same event protocol as Ping.
func (a *API) Tracert(target string) error {
	return a.runDiag(ipc.MethodTracert, target)
}

// StopDiag cancels an in-flight ping/tracert.
func (a *API) StopDiag() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
}

func (a *API) runDiag(method, target string) error {
	a.StopDiag()
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()

	frames, errCh, err := a.client.Stream(method, map[string]any{"target": target})
	if err != nil {
		cancel()
		return err
	}
	go func() {
		defer cancel()
		a.mu.Lock()
		a.cancel = nil
		a.mu.Unlock()
		for f := range frames {
			a.emit(EventDiagLine, trimQuotes(string(f.Data)))
		}
		if e := <-errCh; e != nil {
			a.emit(EventDiagError, e.Error())
		}
		a.emit(EventDiagEnd, nil)
		_ = ctx
	}()
	return nil
}

// ---------- Log streaming ----------

// SubscribeLogs pushes each matching log line as EventLogLine. Pushes until
// the frontend calls UnsubscribeLogs or the window closes.
func (a *API) SubscribeLogs(level string) error {
	go a.subscribeLogsLoop(level)
	return nil
}

func (a *API) subscribeLogsLoop(level string) {
	frames, errCh, err := a.client.Stream(ipc.MethodSubscribeLogs, map[string]any{"level": level})
	if err != nil {
		a.emit(EventLogLine, mustJSON(map[string]string{
			"level": "ERROR", "msg": "无法订阅日志: " + err.Error(),
		}))
		return
	}
	for f := range frames {
		a.emit(EventLogLine, string(f.Data))
	}
	if e := <-errCh; e != nil {
		_ = e
	}
	a.emit(EventLogEnd, nil)
}

// ---------- Status push ----------

func (a *API) subscribeStatusLoop(ctx context.Context) {
	// The service's SubscribeStatus stream pushes a frame on every apply.
	frames, errCh, err := a.client.Stream(ipc.MethodSubscribeStatus, struct{}{})
	if err != nil {
		return
	}
	go func() {
		defer func() { <-errCh }()
		for f := range frames {
			a.emit(EventStatus, json.RawMessage(f.Data))
		}
	}()
	_ = ctx
}

// emit publishes an event to the frontend. Safe to call before OnStartup
// (drops silently if there is no context yet).
func (a *API) emit(name string, data any) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, name, data)
}

// ---------- helpers ----------

func trimQuotes(s string) string {
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"') {
		var out string
		if err := json.Unmarshal([]byte(s), &out); err == nil {
			return out
		}
	}
	return s
}

func mustJSON(v any) string {
	bs, _ := json.Marshal(v)
	return string(bs)
}

// silence unused import warning when level parsing is referenced elsewhere
var _ = logging.LevelFromString
