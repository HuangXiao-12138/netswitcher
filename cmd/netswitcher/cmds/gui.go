package cmds

import (
	"fmt"

	"github.com/spf13/cobra"

	gui "github.com/netswitcher/netswitcher" // root package gui
	"github.com/netswitcher/netswitcher/pkg/winutil"
)

// runGUI launches the Wails desktop window and is shared by the `gui`
// subcommand and the bare-invocation default (double-clicking the exe).
//
// Single-instance: if another GUI is already running, this instance signals
// it to show its window (it may be hidden to tray) and exits. CLI subcommands
// (dump/apply/ipc) bypass this and can run alongside the GUI.
func runGUI(version string) error {
	if owned, err := winutil.AcquireSingleton(); err == nil && !owned {
		// Another instance owns the GUI; nudge it forward and bail out.
		_ = winutil.SignalSingletonShow()
		infof("NetSwitcher 已在运行，已唤出主窗口。")
		return nil
	}

	infof("NetSwitcher GUI 启动")
	err := gui.Run(gui.Options{Title: "NetSwitcher", Width: 1024, Height: 700, Version: version})
	if err == nil {
		return nil
	}
	if err == gui.ErrGUINotCompiled {
		return fmt.Errorf("%w\n  重新构建: 安装 MinGW-w64 (gcc) 后执行 `make build`", err)
	}
	return err
}

// newGUICmd launches the Wails desktop window.
func newGUICmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "gui",
		Short: "启动桌面 GUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGUI(version)
		},
	}
}
