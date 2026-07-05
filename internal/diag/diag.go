// Package diag runs diagnostic subprocesses (ping.exe, tracert.exe) and
// streams their stdout as UTF-8 lines. Used by:
//   - the GUI's embedded diagnostics (appapi → Wails events)
//   - the IPC server's Ping/Tracert methods (writes stream frames)
//
// Output is GBK-decoded on Chinese Windows so callers always see UTF-8.
package diag

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// Run executes name with args and invokes onLine for each stdout line. It
// blocks until the command finishes (or ctx is cancelled), handling GBK
// decoding. onLine must be safe to call from the goroutine; return an error
// from onLine to stop the stream early.
func Run(ctx context.Context, onLine func(string) error, name string, args ...string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("diag.Run: empty command")
	}
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if e := onLine(Decode(scanner.Bytes())); e != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return e
		}
	}
	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		// Surface a final synthetic line so the user sees non-zero exits.
		_ = onLine("(" + name + " exited: " + err.Error() + ")")
	}
	return nil
}

// Ping runs Windows ping.exe (4 echoes, 30s cap) streaming each line.
func Ping(ctx context.Context, target string, onLine func(string) error) error {
	if strings.TrimSpace(target) == "" {
		return errors.New("empty target")
	}
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return Run(c, onLine, "ping", target)
}

// Tracert runs Windows tracert.exe (-d no DNS, -h 20 max hops, 90s cap).
func Tracert(ctx context.Context, target string, onLine func(string) error) error {
	if strings.TrimSpace(target) == "" {
		return errors.New("empty target")
	}
	c, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	return Run(c, onLine, "tracert", "-d", "-h", "20", target)
}

// Decode decodes GBK to UTF-8 when the input is not already valid UTF-8.
// ping.exe / tracert.exe emit GBK on Chinese Windows (spec §14).
func Decode(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	if s, err := simplifiedchinese.GBK.NewDecoder().Bytes(b); err == nil {
		return string(s)
	}
	return string(b)
}

// Ensure io is referenced for future helpers (avoids unused-import churn).
var _ = io.Discard
