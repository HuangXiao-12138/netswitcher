package cmds

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/netswitcher/netswitcher/internal/config"
	"github.com/netswitcher/netswitcher/internal/ifacemgr"
	"github.com/netswitcher/netswitcher/internal/logging"
	"github.com/netswitcher/netswitcher/internal/paths"
	"github.com/netswitcher/netswitcher/internal/routeengine"
	"github.com/netswitcher/netswitcher/internal/state"
)

// newApplyCmd loads the config and applies routing once, then exits.
// Useful for CLI validation. Add --dry-run to compute the diff without
// touching route.exe / netsh.exe.
func newApplyCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "读 config 应用一次后退出（调试/CLI 验证用）",
		RunE: func(cmd *cobra.Command, args []string) error {
			logDir, _ := paths.LogDir()
			_, _ = logging.Configure(gflags.logLevel, logDir)

			cfgPath, err := gflags.configPathOrDefault()
			if err != nil {
				return err
			}
			statePath, err := gflags.statePathOrDefault()
			if err != nil {
				return err
			}

			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			profile := cfg.ActiveProfileOrDefault()
			if profile == nil {
				slog.Warn("no active profile; will remove any managed routes")
			}

			mgr := ifacemgr.New()
			snap, err := mgr.Snapshot()
			if err != nil {
				return fmt.Errorf("interface snapshot: %w", err)
			}

			var exec routeengine.Executor = &routeengine.Exec{DryRun: dryRun}
			var metrics routeengine.MetricSetter = &routeengine.NetshMetric{DryRun: dryRun}
			store := state.New(statePath)
			engine := routeengine.New(exec, metrics, store, slog.Default())

			result := engine.Apply(profile, snap, "cli:apply")
			bs, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(bs))
			if len(result.Errors) > 0 {
				return fmt.Errorf("apply completed with %d error(s)", len(result.Errors))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "计算 diff 但不执行 route/netsh 命令")
	return cmd
}
