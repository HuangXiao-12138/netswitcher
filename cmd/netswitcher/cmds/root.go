// Package cmds defines the cobra subcommands that route a single binary into
// its service / GUI / CLI-debug roles (spec §3.2).
package cmds

import (
	"fmt"

	"github.com/spf13/cobra"
)

// globalFlags holds flags common to most subcommands. Each command reads it
// via the package-level shared instance set in PersistentPreRunE.
type globalFlags struct {
	configPath string
	logLevel   string
	statePath  string
	foreground bool // service run: run core logic in foreground even if not under SCM
}

var gflags = &globalFlags{}

// NewRoot builds the root command with all subcommands attached.
func NewRoot(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "netswitcher",
		Short: "NetSwitcher — 内外网路由管理工具",
		Long: `NetSwitcher 维护 Windows 双网卡（内网/外网）路由：让指定网段走指定网卡，
其余流量（含默认路由）走另一块网卡。常驻服务监听网络变化，自动重新下发路由。

单二进制多角色（详见 --help 子命令）：
  service install/uninstall/start/stop/run   Windows 服务管理
  gui                                         桌面 GUI
  apply                                       读 config 应用一次后退出（调试）
  dump                                        打印接口、配置、路由表（调试）
  ipc call <method> <json>                    命名管道自测`,
		Version: version,
	}
	root.PersistentFlags().StringVar(&gflags.configPath, "config", "",
		"config.json 路径（默认 %ProgramData%\\NetSwitcher\\config.json）")
	root.PersistentFlags().StringVar(&gflags.statePath, "state", "",
		"state.json 路径（默认 %ProgramData%\\NetSwitcher\\state.json）")
	root.PersistentFlags().StringVar(&gflags.logLevel, "log-level", "info",
		"日志级别: debug|info|warn|error")

	root.AddCommand(
		newServiceCmd(version),
		newGUICmd(),
		newApplyCmd(),
		newDumpCmd(),
		newIPCCmd(),
	)
	return root
}

// resolveConfigPath returns the effective config path (flag or default).
func (g *globalFlags) configPathOrDefault() (string, error) {
	if g.configPath != "" {
		return g.configPath, nil
	}
	return defaultConfigPath()
}

func (g *globalFlags) statePathOrDefault() (string, error) {
	if g.statePath != "" {
		return g.statePath, nil
	}
	return defaultStatePath()
}

func infof(format string, a ...any) {
	fmt.Printf(format+"\n", a...)
}
