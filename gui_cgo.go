//go:build cgo

package gui

import (
	"context"
	"embed"
	"log/slog"

	"github.com/netswitcher/netswitcher/appapi"
	"github.com/netswitcher/netswitcher/internal/paths"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/windows/icon.ico
var trayIcon []byte

// Run opens the desktop window and blocks until the user closes it.
func Run(opts Options) error {
	if opts.Title == "" {
		opts.Title = "NetSwitcher"
	}
	if opts.Width == 0 {
		opts.Width = 1024
	}
	if opts.Height == 0 {
		opts.Height = 700
	}

	api := appapi.New()
	api.IconBytes = trayIcon // tray icon for "minimize to tray" behavior
	api.Version = opts.Version
	bindings := append([]any{api}, opts.Bindings...)

	err := wails.Run(&options.App{
		Title:     opts.Title,
		Width:     opts.Width,
		Height:    opts.Height,
		MinWidth:  760,
		MinHeight: 520,
		// Frameless: we draw our own title bar (App.svelte .topbar) with
		// custom min/max/close buttons. This removes the native title bar,
		// so there's no more blank/default window icon in the title bar.
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: func(ctx context.Context) {
			dir, _ := paths.ProgramDataDir()
			slog.Info("GUI starting", "programdata", dir)
			api.OnStartup(ctx) // wire event context + tray
		},
		// X button hides the window instead of quitting — the system tray
		// menu's "退出" item is the real quit path. When a real quit is in
		// progress (tray → 退出, or RelaunchElevated), OnBeforeClose must let
		// the close proceed, otherwise the window never closes and the process
		// becomes an orphan with no UI and no tray.
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			if api.IsQuitting() {
				return false // allow the close — real shutdown
			}
			runtime.WindowHide(ctx)
			slog.Info("window hidden to tray (X clicked); use tray → 退出 to quit")
			return true
		},
		Bind:    bindings,
		Windows: &windows.Options{},
	})
	return err
}
