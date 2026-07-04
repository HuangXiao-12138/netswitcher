package cmds

import (
	"github.com/spf13/cobra"
)

// newIPCCmd exposes a hidden `ipc call <method> <json>` helper for manual
// named-pipe testing. Phase 5 wires the real client.
func newIPCCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "ipc",
		Short:  "命名管道自测",
		Hidden: true,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "call [method] [json]",
		Short: "调用一个 IPC 方法并打印结果",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			infof("[phase0] ipc call (stub) — Phase 5 wires ipc.Client")
			return errNotImplemented("ipc call", "Phase 5")
		},
	})
	return cmd
}
