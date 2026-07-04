package routeengine

import (
	"bytes"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/netswitcher/netswitcher/internal/state"
)

// Executor is the route-mutation interface the engine depends on. The real
// implementation calls route.exe; tests inject a mock (spec §13.1).
type Executor interface {
	Add(r state.Entry) error
	Delete(destination string, ifIndex int) error
}

// MetricSetter is the interface for interface-metric changes (§7.5). Real
// impl shells out to netsh; tests mock it.
type MetricSetter interface {
	SetInterfaceMetric(ifaceName string, metric int) error
	SetAutomaticMetric(ifaceName string) error
}

// Exec is the route.exe-backed Executor.
type Exec struct {
	// If non-nil, commands are recorded for diagnostics without being run.
	// Set only in dry-run mode.
	DryRun bool
}

// idempotence substrings for both locales.
var (
	addExistsMarkers     = []string{"已存在", "exists", "already exists", "对象已存在"}
	deleteMissingMarkers = []string{"找不到", "could not find", "no matching", "not found"}
)

// Add runs `route add <dest> mask <mask> <gateway> IF <ifIndex> metric <m>`.
// Runtime-only (never -p; spec §17.3). Re-adding an existing route is
// treated as success (idempotent).
func (e *Exec) Add(r state.Entry) error {
	dest, mask, err := splitCIDR(r.Destination)
	if err != nil {
		return fmt.Errorf("route add %s: %w", r.Destination, err)
	}
	if r.Gateway == "" {
		return fmt.Errorf("route add %s: empty gateway", r.Destination)
	}
	if r.IfIndex <= 0 {
		return fmt.Errorf("route add %s: invalid ifIndex %d", r.Destination, r.IfIndex)
	}

	args := []string{"add", dest, "mask", mask, r.Gateway, "IF", fmt.Sprint(r.IfIndex), "metric", fmt.Sprint(r.Metric)}
	if e.DryRun {
		slog.Info("dry-run route add", "args", args)
		return nil
	}

	cmd := exec.Command("route", args...)
	var out, errB bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errB
	runErr := cmd.Run()
	msg := decode(out.Bytes()) + decode(errB.Bytes())
	if runErr != nil && containsAny(msg, addExistsMarkers) {
		// Idempotent: route already present.
		return nil
	}
	if runErr != nil {
		return fmt.Errorf("route add %s: %s: %w", r.Destination, strings.TrimSpace(msg), runErr)
	}
	return nil
}

// Delete runs `route delete <dest>` (with optional `IF <ifIndex>`).
// Deleting a route that isn't there is treated as success.
func (e *Exec) Delete(destination string, ifIndex int) error {
	dest, _, err := splitCIDR(destination)
	if err != nil {
		// destination may already be a bare network; fall back to using it
		// directly so deletes never wedge on a parse error.
		dest = destination
	}
	args := []string{"delete", dest}
	if ifIndex > 0 {
		args = append(args, "IF", fmt.Sprint(ifIndex))
	}
	if e.DryRun {
		slog.Info("dry-run route delete", "args", args)
		return nil
	}

	cmd := exec.Command("route", args...)
	out, runErr := cmd.CombinedOutput()
	msg := decode(out)
	if runErr != nil && containsAny(msg, deleteMissingMarkers) {
		return nil
	}
	if runErr != nil {
		return fmt.Errorf("route delete %s: %s: %w", dest, strings.TrimSpace(msg), runErr)
	}
	return nil
}

// decode handles GBK output on Chinese Windows (spec §14, §11.1).
func decode(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if utf8.Valid(b) {
		return string(b)
	}
	if s, err := simplifiedchinese.GBK.NewDecoder().Bytes(b); err == nil {
		return string(s)
	}
	return string(b)
}

func containsAny(s string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// splitCIDR turns "168.168.0.0/16" into ("168.168.0.0", "255.255.0.0").
func splitCIDR(cidr string) (dest, mask string, err error) {
	pfx, perr := parsePrefix(cidr)
	if perr != nil {
		return "", "", perr
	}
	bits := pfx.Bits()
	if bits < 0 || bits > 32 {
		return "", "", fmt.Errorf("invalid prefix length %d", bits)
	}
	return pfx.Masked().Addr().String(), ipv4Mask(bits), nil
}

// ipv4Mask converts a prefix length to dotted-decimal mask.
func ipv4Mask(bits int) string {
	m := uint32(0xFFFFFFFF << (32 - bits))
	return fmt.Sprintf("%d.%d.%d.%d", byte(m>>24), byte(m>>16), byte(m>>8), byte(m))
}
