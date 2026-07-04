// Package app wires core + the IPC server into a single startable stack so
// the foreground `service run` entrypoint and the SCM-launched program share
// identical setup.
package app

import (
	"fmt"
	"log/slog"

	"github.com/netswitcher/netswitcher/internal/core"
	"github.com/netswitcher/netswitcher/internal/ipc"
)

// Stack is the running core + IPC server pair.
type Stack struct {
	Core *core.Core
	IPC  *ipc.Server
}

// Start builds the core, applies on startup, and opens the IPC pipe.
// Stop reverses the order: IPC first (drain GUI clients), then core.
func Start(opts core.Options, log *slog.Logger) (*Stack, error) {
	if log == nil {
		log = slog.Default()
	}
	c, err := core.New(opts, log)
	if err != nil {
		return nil, fmt.Errorf("core: %w", err)
	}
	if err := c.Start(); err != nil {
		return nil, fmt.Errorf("core start: %w", err)
	}
	srv := ipc.New(c, log)
	if err := srv.Start(); err != nil {
		c.Stop()
		return nil, fmt.Errorf("ipc: %w", err)
	}
	return &Stack{Core: c, IPC: srv}, nil
}

// Stop tears the stack down gracefully.
func (s *Stack) Stop() {
	if s.IPC != nil {
		s.IPC.Stop()
	}
	if s.Core != nil {
		s.Core.Stop()
	}
}
