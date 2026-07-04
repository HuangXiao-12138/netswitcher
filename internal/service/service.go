// Package service wraps core as a Windows service via kardianos/service
// (spec §3.2, §7.10, §10.1, Phase 4).
//
// The same netswitcher.exe plays every role:
//
//   - `service install` / `uninstall` / `start` / `stop` — service control
//     (elevated). Writes/removes the SCM registration.
//   - `service run` — foreground debug loop (Phase 3, no SCM).
//   - `service scm` (hidden) — invoked by the Service Control Manager; calls
//     svc.Run() which registers with SCM and runs core.
//
// Config.Arguments is set to ["service","scm"] so SCM launches us into the
// scm subcommand. The service is auto-start, restart on failure.
package service

import (
	"errors"
	"log/slog"

	"github.com/kardianos/service"

	"github.com/netswitcher/netswitcher/internal/core"
	"github.com/netswitcher/netswitcher/internal/logging"
	"github.com/netswitcher/netswitcher/internal/paths"
)

// ServiceName is the SCM service key (and display name).
const ServiceName = "NetSwitcher"

// Config returns the kardianos service configuration.
func Config() *service.Config {
	return &service.Config{
		Name:        ServiceName,
		DisplayName: "NetSwitcher",
		Description: "内外网路由管理工具 — 自动维护双网卡路由",
		// SCM launches this binary into the hidden scm subcommand.
		Arguments: []string{"service", "scm"},
		// Windows-specific options: auto-start at boot (routes correct before
		// login, §15.2) and restart on failure (§14) with a 10s back-off.
		Option: service.KeyValue{
			"StartType":              service.ServiceStartAutomatic,
			"OnFailure":              "restart",
			"OnFailureDelayDuration": "10s",
		},
	}
}

// Options are passed from the CLI to the service entrypoint.
type Options struct {
	ConfigPath string
	StatePath  string
	LogLevel   string
}

// program implements service.Interface; it owns a Core once started.
type program struct {
	opts   Options
	core   *core.Core
	stopCh chan struct{}
}

func newProgram(opts Options) *program {
	return &program{opts: opts, stopCh: make(chan struct{})}
}

// Start is called by kardianos under SCM. It must return quickly, so the
// actual work runs in a goroutine.
func (p *program) Start(s service.Service) error {
	go p.run()
	return nil
}

// run sets up logging + core and blocks until Stop signals.
func (p *program) run() {
	logDir, _ := paths.LogDir()
	cleanup, _ := logging.Configure(p.opts.LogLevel, logDir)
	defer cleanup()
	slog.Info("NetSwitcher service (SCM) starting")

	c, err := core.New(core.Options{
		ConfigPath: p.opts.ConfigPath,
		StatePath:  p.opts.StatePath,
		LogLevel:   p.opts.LogLevel,
		LogDir:     logDir,
	}, slog.Default())
	if err != nil {
		slog.Error("core init failed", "err", err)
		return
	}
	p.core = c
	if err := c.Start(); err != nil {
		slog.Error("core start failed", "err", err)
		return
	}
	<-p.stopCh // wait for Stop
}

// Stop is called by kardianos when the service is stopping.
func (p *program) Stop(s service.Service) error {
	if p.core != nil {
		p.core.Stop()
	}
	select {
	case <-p.stopCh:
	default:
		close(p.stopCh)
	}
	return nil
}

// new constructs the kardianos service with the program.
func new(opts Options) (service.Service, *program, error) {
	p := newProgram(opts)
	svc, err := service.New(p, Config())
	if err != nil {
		return nil, nil, err
	}
	return svc, p, nil
}

// Install registers the service with SCM and ensures the data directory exists.
func Install(opts Options) error {
	if err := ensureDataDir(); err != nil {
		return err
	}
	svc, _, err := new(opts)
	if err != nil {
		return err
	}
	if err := svc.Install(); err != nil {
		return err
	}
	// The service runs as SYSTEM; ensure it can reach the binary path.
	return nil
}

// Uninstall removes the service. Config/state files are left in place so a
// reinstall resumes (Phase 7 installer asks whether to delete them).
func Uninstall() error {
	svc, _, err := new(Options{})
	if err != nil {
		return err
	}
	return svc.Uninstall()
}

// Start asks SCM to start the service.
func Start() error {
	svc, _, err := new(Options{})
	if err != nil {
		return err
	}
	return svc.Start()
}

// Stop asks SCM to stop the service.
func Stop() error {
	svc, _, err := new(Options{})
	if err != nil {
		return err
	}
	return svc.Stop()
}

// RunSCM is the SCM entrypoint. Blocks until the service stops.
func RunSCM(opts Options) error {
	if !service.Interactive() {
		// Running under SCM (or launched as a service). kardianos requires
		// svc.Run() to register with the SCM and dispatch Start/Stop.
		svc, _, err := new(opts)
		if err != nil {
			return err
		}
		return svc.Run()
	}
	return errors.New("service scm must be launched by the Service Control Manager; use `netswitcher service run` for foreground")
}

// Status reports whether the service is installed and its current state.
type Status struct {
	Installed bool
	State     string // service.State* stringer
}

// Query returns the current install / running state.
func Query() (*Status, error) {
	svc, _, err := new(Options{})
	if err != nil {
		return nil, err
	}
	st, err := svc.Status()
	if err != nil {
		// Most likely "not installed".
		return &Status{Installed: false, State: err.Error()}, nil
	}
	return &Status{Installed: true, State: stateString(st)}, nil
}

func stateString(st service.Status) string {
	switch st {
	case service.StatusRunning:
		return "running"
	case service.StatusStopped:
		return "stopped"
	case service.StatusUnknown:
		return "unknown"
	}
	return "unknown"
}

// ensureDataDir creates %ProgramData%\NetSwitcher\ (with logs/runtime) so the
// SYSTEM service can write immediately on first start. The default ProgramData
// ACL (SYSTEM full / Admins full / Users read) is appropriate; we don't
// re-ACL an existing dir.
func ensureDataDir() error {
	for _, fn := range []func() (string, error){
		paths.ProgramDataDir,
		paths.LogDir,
		paths.RuntimeDir,
	} {
		if _, err := fn(); err != nil {
			return err
		}
	}
	return nil
}
