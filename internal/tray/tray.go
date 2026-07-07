// Package tray provides the NetSwitcher system-tray icon: clicking the
// window's X button hides the window (Wails OnBeforeClose), and this tray
// icon is the always-visible entry point to show it again or quit.
//
// fyne.io/systray v1.12.2 runs its own hidden-window message pump; on Windows
// it works from a goroutine, so Run blocks and is started in a goroutine by
// the GUI layer.
package tray

import (
	"log/slog"

	"fyne.io/systray"
)

// Run starts the tray. Blocks until systray.Quit is called. iconBytes is the
// .ico content. The callbacks are invoked from the tray menu goroutine.
func Run(iconBytes []byte, onShow, onApply, onQuit func()) {
	systray.Run(func() { onReady(iconBytes, onShow, onApply, onQuit) }, func() {})
}

func onReady(iconBytes []byte, onShow, onApply, onQuit func()) {
	systray.SetIcon(iconBytes)
	systray.SetTitle("")
	systray.SetTooltip("NetSwitcher — 内外网路由管理")

	// Left-click → show window; right-click → context menu (default).
	systray.SetOnTapped(func() { safe(onShow, "show window (left-click)") })

	mShow := systray.AddMenuItem("显示主窗口", "显示 NetSwitcher 主界面")
	systray.AddSeparator()
	mApply := systray.AddMenuItem("立即重新应用路由", "触发一次 apply")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出 NetSwitcher", "退出程序（服务继续运行）")

	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				safe(onShow, "show window")
			case <-mApply.ClickedCh:
				safe(onApply, "apply now")
			case <-mQuit.ClickedCh:
				systray.Quit()
				safe(onQuit, "quit")
				return
			}
		}
	}()
}

func safe(f func(), what string) {
	if f == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("tray callback panicked", "what", what, "err", r)
		}
	}()
	f()
}
