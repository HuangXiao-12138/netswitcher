// Package nrpt manages Windows Name Resolution Policy Table rules
// (Add/Remove-DnsClientNrptRule). An NRPT rule sends DNS queries for a domain
// suffix (e.g. .luculent.vip) to a specified DNS server, so internal domains
// resolve to internal IPs — combined with an IP route for that range, traffic
// for the domain exits the desired interface.
package nrpt

import "strings"

// Rule is one NRPT rule passed to Setter.Sync (namespace + its DNS servers).
type Rule struct {
	Namespace   string
	NameServers []string
}

// Setter is the interface the route engine depends on for NRPT mutation. The
// real implementation shells out to PowerShell; tests inject a mock.
type Setter interface {
	// Add creates (or re-creates) an NRPT rule: queries for namespace (a leading
	// "." suffix like ".luculent.vip") resolve via nameServers.
	Add(namespace string, nameServers []string) error
	// Remove deletes the NRPT rule for namespace. Removing a non-existent rule
	// is treated as success.
	Remove(namespace string) error
	// Sync applies a batch of add/remove in one shot. Implementations spawn at
	// most one process for the whole batch — per-rule Add/Remove is too slow
	// (each spawns its own powershell), which delayed engine startup.
	Sync(add []Rule, remove []string) error
}

// NamespaceFor converts a user-entered domain to an NRPT namespace.
//   - "*.demo.com" → ".demo.com"  (wildcard: matches only sub-domains *.demo.com)
//   - "demo.com"   → "demo.com"   (matches demo.com and all its sub-domains)
//   - ".demo.com"  → ".demo.com"  (explicit wildcard, same as *.demo.com)
// Leading "*." is the user-facing way to request "wildcard sub-domains only";
// without it the rule covers the domain AND its sub-domains (NRPT is
// suffix-based, there's no true "exact host" — use a leaf FQDN for that).
func NamespaceFor(domain string) string {
	d := strings.TrimSpace(domain)
	if strings.HasPrefix(d, "*.") {
		return d[1:] // "*.demo.com" → ".demo.com"
	}
	return d
}
