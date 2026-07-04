//go:build cgo

package gui

import (
	"context"
	"embed"
	"log/slog"

	"github.com/netswitcher/netswitcher/internal/paths"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

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

	bindings := opts.Bindings
	if len(bindings) == 0 {
		bindings = []any{&noopAPI{}}
	}

	err := wails.Run(&options.App{
		Title:     opts.Title,
		Width:     opts.Width,
		Height:    opts.Height,
		MinWidth:  760,
		MinHeight: 520,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: func(ctx context.Context) {
			dir, _ := paths.ProgramDataDir()
			slog.Info("GUI starting", "programdata", dir)
		},
		Bind:    bindings,
		Windows: &windows.Options{},
	})
	return err
}

// noopAPI is a placeholder bound to the frontend so `wails build` succeeds in
// Phase 0. Phase 6 replaces it with the real IPC-backed API.
type noopAPI struct{}

func (a *noopAPI) Ping() string { return "pong" }
