package ipc

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// streamCommand runs an OS command and forwards each stdout line as a stream
// data frame, then writes the stream:end marker. Returns nil on clean exit,
// or an error if the client disconnected (write failed) or the target was
// rejected up front.
func streamCommand(ctx context.Context, w io.Writer, id, name string, args []string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = nil // tracert/ping write diagnostics to stdout on Windows
	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		// ping.exe / tracert.exe emit GBK on Chinese Windows (§14); decode so
		// the JSON string we forward is valid UTF-8 for the GUI.
		text := decodeLocal(scanner.Bytes())
		if err := WriteStreamData(w, id, text); err != nil {
			// Client gone. Kill the process and bail.
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return err
		}
	}
	werr := cmd.Wait()
	if werr != nil && ctx.Err() == nil {
		// Non-zero exit (e.g. ping found no route). Surface as a final line.
		_ = WriteStreamData(w, id, "("+name+" exited "+werr.Error()+")")
	}
	return WriteStreamEnd(w, id)
}

// decodeLocal decodes GBK to UTF-8 when the input is not already valid UTF-8.
func decodeLocal(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	if s, err := simplifiedchinese.GBK.NewDecoder().Bytes(b); err == nil {
		return string(s)
	}
	return string(b)
}

// runPing streams Windows ping.exe output for the given target.
func runPing(ctx context.Context, w io.Writer, id, target string) error {
	if strings.TrimSpace(target) == "" {
		return errors.New("empty target")
	}
	// Windows ping.exe: -n count, -w timeout per reply (ms). 4 echoes is
	// the default and matches what users expect.
	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := streamCommand(pingCtx, w, id, "ping", []string{target}); err != nil {
		slog.Warn("ping stream ended", "target", target, "err", err)
		return err
	}
	return nil
}

// runTracert streams Windows tracert.exe output. tracert can be slow; cap at
// 90s and limit hops to keep the GUI responsive.
func runTracert(ctx context.Context, w io.Writer, id, target string) error {
	if strings.TrimSpace(target) == "" {
		return errors.New("empty target")
	}
	tracertCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	args := []string{"-d", "-h", "20", target} // -d = no DNS, -h 20 max hops
	if err := streamCommand(tracertCtx, w, id, "tracert", args); err != nil {
		slog.Warn("tracert stream ended", "target", target, "err", err)
		return err
	}
	return nil
}
