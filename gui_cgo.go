//go:build cgo

package gui

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/netswitcher/netswitcher/appapi"
	"github.com/netswitcher/netswitcher/internal/logging"
	"github.com/netswitcher/netswitcher/internal/paths"
	"github.com/netswitcher/netswitcher/pkg/winutil"

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

	// Configure file logging BEFORE Wails starts so startup-time messages
	// (pre-OnStartup) land in netswitcher.log. This is the key diagnostic for
	// the elevated-relaunch loop: we can see whether the new instance reached
	// Wails at all, and with what args/elevation.
	if dir, err := paths.LogDir(); err == nil {
		_, _ = logging.Configure("info", dir)
	}
	slog.Info("gui.Run start", "elevated", api.IsElevated(), "args", os.Args)

	// Ensure WebView2 runtime is installed before launching the GUI.
	if err := EnsureWebView2(api.IsElevated()); err != nil {
		slog.Error("ensure webview2 failed", "err", err)
		return fmt.Errorf("ensure webview2: %w", err)
	}
	slog.Info("webview2 ready, calling wails.Run")

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
		OnDomReady: func(ctx context.Context) {
			slog.Info("on-dom-ready (frontend loaded)", "pid", os.Getpid())
		},
		// X button hides the window instead of quitting — the system tray
		// menu's "退出" item is the real quit path. When a real quit is in
		// progress (tray → 退出, or RelaunchElevated), OnBeforeClose must let
		// the close proceed, otherwise the window never closes and the process
		// becomes an orphan with no UI and no tray.
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			slog.Info("on-before-close", "pid", os.Getpid(), "quitting", api.IsQuitting())
			if api.IsQuitting() {
				return false // allow the close — real shutdown
			}
			runtime.WindowHide(ctx)
			slog.Info("window hidden to tray (X clicked); use tray → 退出 to quit")
			return true
		},
		OnShutdown: func(ctx context.Context) {
			slog.Info("on-shutdown", "pid", os.Getpid())
			// Kill webview2/route/ping children explicitly — the job can't use
			// KILL_ON_JOB_CLOSE (would also kill the elevated-replacement).
			winutil.KillChildProcesses()
		},
		Bind:      bindings,
		Windows:   windowsOptions(opts.Takeover),
	})
	slog.Info("wails.Run returned", "err", err)
	return err
}

// windowsOptions builds Wails' Windows-specific options. When Takeover is set
// (elevated relaunch), WebView2's user-data folder is redirected to an isolated
// path so it doesn't race the previous (non-elevated) instance's WebView2
// children for the default folder lock — that race crashed the new window
// mid-render right after on-dom-ready.
func windowsOptions(takeover bool) *windows.Options {
	o := &windows.Options{}
	if takeover {
		if dir, err := paths.ProgramDataDir(); err == nil {
			o.WebviewUserDataPath = filepath.Join(dir, "webview2-takeover")
			slog.Info("using isolated WebView2 user-data dir", "path", o.WebviewUserDataPath)
		}
	}
	return o
}
