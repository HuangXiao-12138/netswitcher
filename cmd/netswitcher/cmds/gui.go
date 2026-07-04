package cmds

import (
	"fmt"

	"github.com/spf13/cobra"

	gui "github.com/netswitcher/netswitcher" // root package gui
)

// newGUICmd launches the Wails desktop window. Real API bindings are attached
// in Phase 6; Phase 0 just opens the window with a placeholder binding.
func newGUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gui",
		Short: "启动桌面 GUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			infof("[phase0] launching GUI")
			err := gui.Run(gui.Options{Title: "NetSwitcher", Width: 1024, Height: 700})
			if err == nil {
				return nil
			}
			if err == gui.ErrGUINotCompiled {
				return fmt.Errorf("%w\n  重新构建: 安装 MinGW-w64 (gcc) 后执行 `make build`", err)
			}
			return err
		},
	}
}
