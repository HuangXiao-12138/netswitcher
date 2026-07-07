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

// Add runs `Add-DnsClientNrptRule -Namespace <ns> -NameServers <ip>,<ip>,...`.
func (s *PowerShellSetter) Add(namespace string, nameServers []string) error {
	if namespace == "" || len(nameServers) == 0 {
		return nil
	}
	cmdText := fmt.Sprintf(
		"Add-DnsClientNrptRule -Namespace '%s' -NameServers %s",
		namespace, psNameServers(nameServers),
	)
	return s.run(cmdText, "Add", namespace)
}

// Remove runs `Get-DnsClientNrptRule -Namespace <ns> | Remove-DnsClientNrptRule`.
// Remove-DnsClientNrptRule has NO -Namespace parameter (only -Name/GUID), so we
// pipe through Get to delete every rule matching the namespace.
func (s *PowerShellSetter) Remove(namespace string) error {
	if namespace == "" {
		return nil
	}
	cmdText := fmt.Sprintf(
		"Get-DnsClientNrptRule -ErrorAction SilentlyContinue | Where-Object { $_.Namespace -eq '%s' } | Remove-DnsClientNrptRule -Force -ErrorAction SilentlyContinue",
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
		// Remove-DnsClientNrptRule has no -Namespace param; pipe via Get.
		cmds = append(cmds, fmt.Sprintf("Get-DnsClientNrptRule -ErrorAction SilentlyContinue | Where-Object { $_.Namespace -eq '%s' } | Remove-DnsClientNrptRule -Force -ErrorAction SilentlyContinue", ns))
	}
	for _, r := range add {
		if r.Namespace == "" || len(r.NameServers) == 0 {
			continue
		}
		// NRPT allows multiple rules per namespace; clear any existing first so
		// re-applies don't accumulate duplicates (the old Remove bug left 12
		// copies of the same rule behind).
		cmds = append(cmds, fmt.Sprintf("Get-DnsClientNrptRule -ErrorAction SilentlyContinue | Where-Object { $_.Namespace -eq '%s' } | Remove-DnsClientNrptRule -Force -ErrorAction SilentlyContinue", r.Namespace))
		cmds = append(cmds, fmt.Sprintf("Add-DnsClientNrptRule -Namespace '%s' -NameServers %s", r.Namespace, psNameServers(r.NameServers)))
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

// psNameServers formats a name-server list as a PowerShell string array,
// 'ip1','ip2' — Add-DnsClientNrptRule -NameServers takes String[], NOT a
// comma-joined single string (the old "'a,b'" made the whole thing one invalid
// server, which broke resolution with 2+ DNS servers configured).
func psNameServers(ips []string) string {
	quoted := make([]string, len(ips))
	for i, ip := range ips {
		quoted[i] = fmt.Sprintf("'%s'", ip)
	}
	return strings.Join(quoted, ",")
}
