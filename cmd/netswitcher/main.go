// NetSwitcher is a single-binary, dual-role Windows application: a Windows
// service that maintains routing for split internal/external networks, and a
// desktop GUI for configuration and diagnostics.
//
// All roles share this entry point; subcommands select behavior (§3.2):
//
//	netswitcher service install|uninstall|start|stop|run
//	netswitcher gui
//	netswitcher apply
//	netswitcher dump
//	netswitcher ipc call <method> <json>
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/netswitcher/netswitcher/cmd/netswitcher/cmds"
	"github.com/netswitcher/netswitcher/pkg/winutil"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func init() {
	// Cobra ships a "mousetrap" that intercepts explorer.exe (double-click)
	// launches on Windows and prints "This is a command line tool. You need to
	// open cmd.exe…". For NetSwitcher the *intended* double-click behavior is to
	// open the GUI, so disable it.
	cobra.MousetrapHelpText = ""
	cobra.MousetrapDisplayDuration = 0
}

func main() {
	// The binary is built GUI-subsystem (-H windowsgui) so double-clicking
	// doesn't pop a black console. When invoked from a real terminal with
	// args (dump/apply/ipc/--help), attach to the parent's console so CLI
	// output is visible. Bare invocation = GUI, no console needed.
	if len(os.Args) > 1 {
		_ = winutil.AttachParentConsole()
	}

	root := cmds.NewRoot(version)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
