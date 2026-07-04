package cmds

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/netswitcher/netswitcher/internal/core"
	"github.com/netswitcher/netswitcher/internal/logging"
	"github.com/netswitcher/netswitcher/internal/paths"
)

// newServiceCmd groups the Windows-service management subcommands. Phase 4
// wires install/uninstall/start/stop into kardianos/service; run is the
// foreground debug entry point and is fully functional from Phase 3.
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
				infof("[stub] service install — Phase 4")
				return errNotImplemented("service install", "Phase 4")
			},
		},
		&cobra.Command{
			Use:   "uninstall",
			Short: "卸载 Windows 服务（需管理员）",
			RunE: func(cmd *cobra.Command, args []string) error {
				infof("[stub] service uninstall — Phase 4")
				return errNotImplemented("service uninstall", "Phase 4")
			},
		},
		&cobra.Command{
			Use:   "start",
			Short: "启动 Windows 服务（需管理员）",
			RunE: func(cmd *cobra.Command, args []string) error {
				infof("[stub] service start — Phase 4")
				return errNotImplemented("service start", "Phase 4")
			},
		},
		&cobra.Command{
			Use:   "stop",
			Short: "停止 Windows 服务（需管理员）",
			RunE: func(cmd *cobra.Command, args []string) error {
				infof("[stub] service stop — Phase 4")
				return errNotImplemented("service stop", "Phase 4")
			},
		},
		newServiceRunCmd(version),
	)
	return cmd
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

			c, err := core.New(core.Options{
				ConfigPath: cfgPath,
				StatePath:  statePath,
				LogLevel:   gflags.logLevel,
				LogDir:     logDir,
			}, slog.Default())
			if err != nil {
				return fmt.Errorf("init core: %w", err)
			}
			if err := c.Start(); err != nil {
				return fmt.Errorf("start core: %w", err)
			}
			defer c.Stop()

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
