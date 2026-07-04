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

	"github.com/netswitcher/netswitcher/cmd/netswitcher/cmds"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	root := cmds.NewRoot(version)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
