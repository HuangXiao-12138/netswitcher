//go:build windows

package nrpt

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/netswitcher/netswitcher/pkg/winutil"
)

// PowerShellSetter is the production Setter, backed by the Add/Remove-DnsClient
// NrptRule cmdlets. Per-rule Add/Remove each spawn their own powershell; prefer
// Sync for batches (one spawn for the whole batch).
type PowerShellSetter struct {
	DryRun bool
}

// Add runs `Add-DnsClientNrptRule -Namespace <ns> -NameServers <ip,ip,...>`.
func (s *PowerShellSetter) Add(namespace string, nameServers []string) error {
	if namespace == "" || len(nameServers) == 0 {
		return nil
	}
	cmdText := fmt.Sprintf(
		"Add-DnsClientNrptRule -Namespace '%s' -NameServers '%s'",
		namespace, strings.Join(nameServers, ","),
	)
	return s.run(cmdText, "Add", namespace)
}

// Remove runs `Remove-DnsClientNrptRule -Namespace <ns> -Force`. A missing rule
// is treated as success (SilentlyContinue).
func (s *PowerShellSetter) Remove(namespace string) error {
	if namespace == "" {
		return nil
	}
	cmdText := fmt.Sprintf(
		"Remove-DnsClientNrptRule -Namespace '%s' -Force -ErrorAction SilentlyContinue",
		namespace,
	)
	return s.run(cmdText, "Remove", namespace)
}

// Sync runs all adds and removes in a single powershell process (commands
// joined by "; ") instead of one process per rule — per-rule spawns were the
// main cause of the slow engine startup.
func (s *PowerShellSetter) Sync(add []Rule, remove []string) error {
	if s.DryRun {
		return nil
	}
	var cmds []string
	for _, ns := range remove {
		cmds = append(cmds, fmt.Sprintf("Remove-DnsClientNrptRule -Namespace '%s' -Force -ErrorAction SilentlyContinue", ns))
	}
	for _, r := range add {
		if r.Namespace == "" || len(r.NameServers) == 0 {
			continue
		}
		cmds = append(cmds, fmt.Sprintf("Add-DnsClientNrptRule -Namespace '%s' -NameServers '%s'", r.Namespace, strings.Join(r.NameServers, ",")))
	}
	if len(cmds) == 0 {
		return nil
	}
	cmd := exec.Command("powershell", "-NoProfile", "-Command", strings.Join(cmds, "; "))
	winutil.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nrpt sync: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (s *PowerShellSetter) run(cmdText, op, namespace string) error {
	if s.DryRun {
		return nil
	}
	cmd := exec.Command("powershell", "-NoProfile", "-Command", cmdText)
	winutil.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s-DnsClientNrptRule %s: %s: %w", op, namespace, strings.TrimSpace(string(out)), err)
	}
	return nil
}
