package ipc

import (
	"context"
	"io"
	"log/slog"

	"github.com/netswitcher/netswitcher/internal/diag"
)

// runPing streams Windows ping.exe output via the IPC stream protocol.
func runPing(ctx context.Context, w io.Writer, id, target string) error {
	if err := diag.Ping(ctx, target, func(line string) error {
		return WriteStreamData(w, id, line)
	}); err != nil {
		slog.Warn("ping stream ended", "target", target, "err", err)
		return err
	}
	return WriteStreamEnd(w, id)
}

// runTracert streams Windows tracert.exe output via the IPC stream protocol.
func runTracert(ctx context.Context, w io.Writer, id, target string) error {
	if err := diag.Tracert(ctx, target, func(line string) error {
		return WriteStreamData(w, id, line)
	}); err != nil {
		slog.Warn("tracert stream ended", "target", target, "err", err)
		return err
	}
	return WriteStreamEnd(w, id)
}
