package cmds

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/netswitcher/netswitcher/internal/config"
	"github.com/netswitcher/netswitcher/internal/ifacemgr"
	"github.com/netswitcher/netswitcher/internal/logging"
	"github.com/netswitcher/netswitcher/internal/paths"
)

// newDumpCmd prints the loaded config and the live interface snapshot.
func newDumpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dump",
		Short: "打印当前接口、配置、路由表（调试用）",
		RunE: func(cmd *cobra.Command, args []string) error {
			logDir, _ := paths.LogDir()
			_, _ = logging.Configure(gflags.logLevel, logDir)
			defer slog.Info("dump complete")

			cfgPath, err := gflags.configPathOrDefault()
			if err != nil {
				return err
			}

			cfg, err := config.Load(cfgPath)
			if err != nil {
				fmt.Fprintln(os.Stderr, "config load error:", err)
			}

			mgr := ifacemgr.New()
			snap, snapErr := mgr.Snapshot()

			out := map[string]any{
				"configPath":     cfgPath,
				"config":         cfg,
				"interfaces":     snap.Interfaces,
				"interfaceError": nilIfEmpty(snapErr),
			}
			bs, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(bs))
			return nil
		},
	}
}

func nilIfEmpty(err error) any {
	if err == nil {
		return nil
	}
	return err.Error()
}
