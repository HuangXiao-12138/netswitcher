package cmds

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/netswitcher/netswitcher/internal/app"
	"github.com/netswitcher/netswitcher/internal/core"
	"github.com/netswitcher/netswitcher/internal/logging"
	"github.com/netswitcher/netswitcher/internal/paths"
	svcwrap "github.com/netswitcher/netswitcher/internal/service"
)

// newServiceCmd groups the Windows-service management subcommands. install/
// uninstall/start/stop wrap kardianos/service (Phase 4). run is the
// foreground debug entrypoint. scm is the hidden SCM entrypoint.
func newServiceCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Windows 服务管理 (install/uninstall/start/stop/run)",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "install",
			Short: "安装为 Windows 服务（需管理员）",
			RunE: func(cmd *cobra.Command, args []string) error {
				opts, err := serviceOpts()
				if err != nil {
					return err
				}
				if err := svcwrap.Install(opts); err != nil {
					return fmt.Errorf("install: %w", err)
				}
				infof("NetSwitcher 服务已安装 (自动启动, 失败重启)")
				return nil
			},
		},
		&cobra.Command{
			Use:   "uninstall",
			Short: "卸载 Windows 服务（需管理员）",
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := svcwrap.Uninstall(); err != nil {
					return fmt.Errorf("uninstall: %w", err)
				}
				infof("NetSwitcher 服务已卸载")
				return nil
			},
		},
		&cobra.Command{
			Use:   "start",
			Short: "启动 Windows 服务（需管理员）",
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := svcwrap.Start(); err != nil {
					return fmt.Errorf("start: %w", err)
				}
				infof("NetSwitcher 服务已启动")
				return nil
			},
		},
		&cobra.Command{
			Use:   "stop",
			Short: "停止 Windows 服务（需管理员）",
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := svcwrap.Stop(); err != nil {
					return fmt.Errorf("stop: %w", err)
				}
				infof("NetSwitcher 服务已停止")
				return nil
			},
		},
		newServiceRunCmd(version),
		newServiceSCMCmd(),
		newServiceEnsureCmd(),
	)
	return cmd
}

// serviceOpts builds service.Options from the global flags.
func serviceOpts() (svcwrap.Options, error) {
	cfgPath, err := gflags.configPathOrDefault()
	if err != nil {
		return svcwrap.Options{}, err
	}
	statePath, err := gflags.statePathOrDefault()
	if err != nil {
		return svcwrap.Options{}, err
	}
	return svcwrap.Options{
		ConfigPath: cfgPath,
		StatePath:  statePath,
		LogLevel:   gflags.logLevel,
	}, nil
}

// errNotImplemented is a clear sentinel so it's obvious which placeholder
// still needs wiring at each phase boundary.
func errNotImplemented(name, phase string) error {
	return fmt.Errorf("%s not implemented yet (lands in %s)", name, phase)
}

// newServiceRunCmd runs the service logic in the foreground (no SCM install),
// which is the primary debug and dev entry point. Blocks until Ctrl-C.
func newServiceRunCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "前台运行服务逻辑（调试用，无需安装）",
		RunE: func(cmd *cobra.Command, args []string) error {
			logDir, _ := paths.LogDir()
			cleanup, _ := logging.Configure(gflags.logLevel, logDir)
			defer cleanup()
			slog.Info("NetSwitcher service (foreground)", "version", version)

			cfgPath, err := gflags.configPathOrDefault()
			if err != nil {
				return err
			}
			statePath, err := gflags.statePathOrDefault()
			if err != nil {
				return err
			}

			stack, err := app.Start(core.Options{
				ConfigPath: cfgPath,
				StatePath:  statePath,
				LogLevel:   gflags.logLevel,
				LogDir:     logDir,
			}, slog.Default())
			if err != nil {
				return err
			}
			defer stack.Stop()

			// Block until interrupted. Windows services get SCM stop; the
			// foreground path uses Ctrl-C / window-close.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			sig := <-sigCh
			slog.Info("shutting down", "signal", sig.String())
			return nil
		},
	}
}

// newServiceSCMCmd is the hidden entrypoint invoked by the Service Control
// Manager. It calls into kardianos svc.Run() which registers with SCM and
// dispatches Start/Stop to the program wrapper around core.
func newServiceSCMCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "scm",
		Short:  "（SCM 内部入口，由服务控制管理器调用）",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := serviceOpts()
			if err != nil {
				return err
			}
			return svcwrap.RunSCM(opts)
		},
	}
}

// newServiceEnsureCmd is the one-click setup the GUI's "start service" button
// elevates to: install if absent, then start. Hidden because it's an internal
// helper invoked via ShellExecute runas from the GUI banner.
func newServiceEnsureCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "ensure",
		Short:  "（GUI 内部）若未安装则安装，然后启动服务",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := serviceOpts()
			if err != nil {
				return err
			}
			if err := svcwrap.Ensure(opts); err != nil {
				return fmt.Errorf("ensure: %w", err)
			}
			infof("NetSwitcher 服务已就绪")
			return nil
		},
	}
}
