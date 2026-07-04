package cmds

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/netswitcher/netswitcher/internal/ipc"
)

// newIPCCmd exposes a hidden `ipc call <method> [json]` helper for manual
// named-pipe testing against the running service (Phase 5 self-test).
func newIPCCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "ipc",
		Short:  "命名管道自测",
		Hidden: true,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "call [method] [json]",
		Short: "调用一个 IPC 方法并打印结果",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			method := args[0]
			var params any
			if len(args) == 2 {
				if err := json.Unmarshal([]byte(args[1]), &params); err != nil {
					return fmt.Errorf("parse params json: %w", err)
				}
			}
			client := ipc.NewClient()

			if ipc.IsStreaming(method) {
				frames, errCh, err := client.Stream(method, params)
				if err != nil {
					return err
				}
				for f := range frames {
					fmt.Println(string(f.Data))
				}
				if err := <-errCh; err != nil {
					return err
				}
				return nil
			}

			raw, err := client.Call(method, params)
			if err != nil {
				return err
			}
			if len(raw) == 0 {
				fmt.Println("{}")
				return nil
			}
			var pretty any
			if err := json.Unmarshal(raw, &pretty); err == nil {
				bs, _ := json.MarshalIndent(pretty, "", "  ")
				fmt.Println(string(bs))
			} else {
				fmt.Println(string(raw))
			}
			return nil
		},
	})
	return cmd
}
