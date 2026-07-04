package cmds

import (
	"github.com/spf13/cobra"
)

// newDumpCmd prints interfaces, the loaded config, and the routing table.
// Phase 1 replaces the stub with real output from ifacemgr + config.
func newDumpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dump",
		Short: "打印当前接口、配置、路由表（调试用）",
		RunE: func(cmd *cobra.Command, args []string) error {
			infof("[phase0] dump (stub) — Phase 1 wires ifacemgr + config")
			return errNotImplemented("dump", "Phase 1")
		},
	}
}
