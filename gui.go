// Package gui launches the Wails desktop front-end.
//
// The actual wails.Run call requires CGO (the Windows WebView2 runtime is
// linked via cgo). To keep `go build` usable for service/CLI-only builds
// without a C toolchain, the cgo implementation lives in gui_cgo.go and a
// stub lives in gui_nocgo.go.
package gui

import "errors"

// ErrGUINotCompiled is returned by Run when the binary was built without CGO
// (i.e. no C compiler was available). The CLI surfaces this to the user.
var ErrGUINotCompiled = errors.New("GUI not compiled into this build; rebuild with CGO enabled (MinGW-w64/gcc required)")

// Options configures the GUI launcher.
type Options struct {
	Title     string
	Width     int
	Height    int
	Version   string // build version, surfaced on the Settings page
	Bindings  []any  // objects whose exported methods are exposed to the frontend
	Takeover  bool   // elevated-relaunch replacement: isolate WebView2 data dir
	Minimized bool   // auto-start: start hidden in the tray (no window flash on login)
}
