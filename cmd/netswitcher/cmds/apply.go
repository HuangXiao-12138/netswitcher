package cmds

import (
	"github.com/spf13/cobra"
)

// newApplyCmd loads the config and applies routing once, then exits. Useful for
// CLI validation. Phase 2 replaces the stub with routeengine.Apply.
func newApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply",
		Short: "读 config 应用一次后退出（调试/CLI 验证用）",
		RunE: func(cmd *cobra.Command, args []string) error {
			infof("[phase0] apply (stub) — Phase 2 wires routeengine.Apply")
			return errNotImplemented("apply", "Phase 2")
		},
	}
}
