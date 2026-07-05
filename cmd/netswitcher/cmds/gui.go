package cmds

import (
	"fmt"

	"github.com/spf13/cobra"

	gui "github.com/netswitcher/netswitcher" // root package gui
)

// runGUI launches the Wails desktop window and is shared by the `gui`
// subcommand and the bare-invocation default (double-clicking the exe).
func runGUI() error {
	infof("NetSwitcher GUI 启动")
	err := gui.Run(gui.Options{Title: "NetSwitcher", Width: 1024, Height: 700})
	if err == nil {
		return nil
	}
	if err == gui.ErrGUINotCompiled {
		return fmt.Errorf("%w\n  重新构建: 安装 MinGW-w64 (gcc) 后执行 `make build`", err)
	}
	return err
}

// newGUICmd launches the Wails desktop window.
func newGUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gui",
		Short: "启动桌面 GUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGUI()
		},
	}
}
