package cmds

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newServiceCmd groups the Windows-service management subcommands.
//
// install/uninstall/start/stop require elevation; run is the foreground debug
// entry point. Phase 0 logs a placeholder line; Phase 4 wires kardianos/service.
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
				infof("[phase0] service install (stub) version=%s", version)
				return errNotImplemented("service install", "Phase 4")
			},
		},
		&cobra.Command{
			Use:   "uninstall",
			Short: "卸载 Windows 服务（需管理员）",
			RunE: func(cmd *cobra.Command, args []string) error {
				infof("[phase0] service uninstall (stub)")
				return errNotImplemented("service uninstall", "Phase 4")
			},
		},
		&cobra.Command{
			Use:   "start",
			Short: "启动 Windows 服务（需管理员）",
			RunE: func(cmd *cobra.Command, args []string) error {
				infof("[phase0] service start (stub)")
				return errNotImplemented("service start", "Phase 4")
			},
		},
		&cobra.Command{
			Use:   "stop",
			Short: "停止 Windows 服务（需管理员）",
			RunE: func(cmd *cobra.Command, args []string) error {
				infof("[phase0] service stop (stub)")
				return errNotImplemented("service stop", "Phase 4")
			},
		},
		&cobra.Command{
			Use:   "run",
			Short: "前台运行服务逻辑（调试用，无需安装）",
			RunE: func(cmd *cobra.Command, args []string) error {
				infof("[phase0] service run (stub) — Phase 3 wires the core loop")
				return errNotImplemented("service run", "Phase 3")
			},
		},
	)
	return cmd
}

// errNotImplemented is a clear sentinel so it's obvious which placeholder
// still needs wiring at each phase boundary.
func errNotImplemented(name, phase string) error {
	return fmt.Errorf("%s not implemented yet (lands in %s)", name, phase)
}
