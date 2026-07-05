package routeengine

import (
	"bytes"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/netswitcher/netswitcher/pkg/winutil"
)

// NetshMetric is the production MetricSetter, backed by netsh.
type NetshMetric struct {
	DryRun bool
}

// SetInterfaceMetric runs `netsh interface ipv4 set interface name=.. metric=N`.
func (m *NetshMetric) SetInterfaceMetric(ifaceName string, metric int) error {
	if strings.TrimSpace(ifaceName) == "" {
		return fmt.Errorf("set metric: empty interface name")
	}
	args := []string{"interface", "ipv4", "set", "interface", fmt.Sprintf("name=%s", ifaceName), fmt.Sprintf("metric=%d", metric)}
	if m.DryRun {
		slog.Info("dry-run netsh set metric", "args", args)
		return nil
	}
	cmd := exec.Command("netsh", args...)
	winutil.HideWindow(cmd) // no console flash from netsh.exe
	var out, errB bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errB
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("netsh set metric %s=%d: %s: %w", ifaceName, metric, strings.TrimSpace(decode(out.Bytes())+decode(errB.Bytes())), err)
	}
	return nil
}

// SetAutomaticMetric restores an interface to automatic metric (§11.2).
func (m *NetshMetric) SetAutomaticMetric(ifaceName string) error {
	if strings.TrimSpace(ifaceName) == "" {
		return fmt.Errorf("set automatic: empty interface name")
	}
	args := []string{"interface", "ipv4", "set", "interface", fmt.Sprintf("name=%s", ifaceName), "metric=automatic"}
	if m.DryRun {
		slog.Info("dry-run netsh metric=automatic", "args", args)
		return nil
	}
	cmd := exec.Command("netsh", args...)
	winutil.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("netsh metric=automatic %s: %s: %w", ifaceName, strings.TrimSpace(decode(out)), err)
	}
	return nil
}
