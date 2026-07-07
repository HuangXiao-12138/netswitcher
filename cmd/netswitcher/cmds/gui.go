package cmds

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	gui "github.com/netswitcher/netswitcher" // root package gui
	"github.com/netswitcher/netswitcher/internal/logging"
	"github.com/netswitcher/netswitcher/internal/paths"
	"github.com/netswitcher/netswitcher/pkg/winutil"
)

// runGUI launches the Wails desktop window and is shared by the `gui`
// subcommand and the bare-invocation default (double-clicking the exe).
//
// Single-instance: if another GUI is already running, this instance signals
// it to show its window (it may be hidden to tray) and exits. CLI subcommands
// (dump/apply/ipc) bypass this and can run alongside the GUI.
func runGUI(version string, takeover bool, minimized bool) error {
	if dir, err := paths.LogDir(); err == nil {
		_, _ = logging.Configure("info", dir)
	}
	slog.Info("runGUI start", "takeover", takeover, "args", os.Args, "inJob", winutil.InJob())
	if takeover {
		// Elevated replacement. Pulse-log every 500ms so the log shows exactly
		// if/when this process is killed during the wait — the previous
		// instance is quitting in parallel, and if we inherited its
		// KILL_ON_JOB_CLOSE job, its exit takes us with it.
		for i := 0; i < 4; i++ {
			time.Sleep(500 * time.Millisecond)
			slog.Info("takeover: pausing", "elapsed_ms", (i+1)*500)
		}
	}
	owned, err := winutil.AcquireSingleton()
	slog.Info("acquire singleton", "owned", owned, "err", err)
	if err == nil && !owned {
		if takeover {
			// Elevated relaunch: the previous (non-elevated) instance is
			// quitting but still holds the single-instance mutex. Wait for it
			// to release instead of misidentifying ourselves as a second
			// instance and exiting — which previously left NO instance running
			// (the "以管理员身份重启不生效" bug).
			slog.Info("takeover: waiting for previous instance to exit")
			deadline := time.Now().Add(8 * time.Second)
			for time.Now().Before(deadline) {
				time.Sleep(150 * time.Millisecond)
				if owned, err = winutil.AcquireSingleton(); err == nil && owned {
					break
				}
			}
			if err != nil || !owned {
				slog.Error("takeover timed out: previous instance did not release the lock")
				return fmt.Errorf("接管超时:旧实例未退出,请右键以管理员身份运行")
			}
			slog.Info("takeover: lock acquired, proceeding")
		} else {
			// Another instance owns the GUI; nudge it forward and bail out.
			_ = winutil.SignalSingletonShow()
			infof("NetSwitcher 已在运行，已唤出主窗口。")
			return nil
		}
	}

	infof("NetSwitcher GUI 启动")
	// Put this process in a kill-on-close Job Object so webview2 children die
	// with us on crash / taskkill (no orphans).
	if err := winutil.AssignSelfToKillOnCloseJob(); err != nil {
		infof("warning: job object not applied: %v", err)
	}
	err = gui.Run(gui.Options{Title: "NetSwitcher", Width: 1024, Height: 700, Version: version, Takeover: takeover, Minimized: minimized})
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
	cmd := &cobra.Command{
		Use:   "gui",
		Short: "启动桌面 GUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGUI(version, takeoverFlag, minimizedFlag)
		},
	}
	cmd.Flags().BoolVar(&takeoverFlag, "takeover", false, "内部:提权重启接管模式")
	cmd.Flags().BoolVar(&minimizedFlag, "minimized", false, "内部:启动隐藏到托盘(开机自启用)")
	_ = cmd.Flags().MarkHidden("takeover")
	_ = cmd.Flags().MarkHidden("minimized")
	return cmd
}
