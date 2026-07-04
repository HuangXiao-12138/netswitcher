// Package conflict detects other programs (VPN clients, virtual adapters)
// that are also managing the route table, and reports them without
// auto-resolving (spec §7.7).
//
// Two checks live here:
//
//   - CheckVPN: a cheap scan of the interface snapshot (IfType + name
//     heuristics). Safe in the apply hot path (§11.3).
//   - CheckExternalOverride: reads the live route table (PowerShell,
//     ~500ms) and flags destinations whose next hop / interface disagree
//     with what NetSwitcher manages. GUI / dump only — never called from
//     applyOnce.
package conflict

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/netswitcher/netswitcher/internal/ifacemgr"
	"github.com/netswitcher/netswitcher/internal/state"
)

// Conflict describes one detected conflict.
type Conflict struct {
	Type        string `json:"type"` // "vpn_present" | "external_override"
	Description string `json:"description"`
	Interface   string `json:"interface,omitempty"`
	Destination string `json:"destination,omitempty"`
}

// Conflict types.
const (
	TypeVPN              = "vpn_present"
	TypeExternalOverride = "external_override"
)

// Name heuristics for VPN / tunnel adapters (§7.7). Lower-cased for matching.
var vpnKeywords = []string{
	"wireguard", "openvpn", "wintun", "tun ", "tap ",
	"cisco", "anyconnect", "fortinet", "forticlient",
	"pulse secure", "globalprotect", "nordvpn",
	"sangfor", "easyconnect", "vpn", "tunnel",
}

// IfType values commonly used by software/VPN adapters.
const (
	ifTypePPP         uint32 = 23
	ifTypePropVirtual uint32 = 53
	ifTypeTunnel      uint32 = 131
)

// Detector reports conflicts. Construct once.
type Detector struct {
	log *slog.Logger
}

// New returns a Detector.
func New(log *slog.Logger) *Detector {
	if log == nil {
		log = slog.Default()
	}
	return &Detector{log: log}
}

// IsVPNInterface applies the §7.7 heuristics to a single interface.
func IsVPNInterface(i ifacemgr.Interface) bool {
	switch i.IfType {
	case ifTypePPP, ifTypePropVirtual, ifTypeTunnel:
		// Most software-loopback tunnels are VPNs, butWiFi/etc. are type 71,
		// Ethernet 6 — so these tunnel types are a strong signal.
		return true
	}
	hay := strings.ToLower(i.FriendlyName + " " + i.Name)
	for _, kw := range vpnKeywords {
		if strings.Contains(hay, kw) {
			return true
		}
	}
	return false
}

// CheckVPN returns one vpn_present conflict per detected VPN adapter. If
// the profile is trying to own the default route (autoManageMetrics with a
// preferred interface), the conflict is highlighted as competing.
func (d *Detector) CheckVPN(snap ifacemgr.Snapshot, profileWantsDefault bool) []Conflict {
	var out []Conflict
	for _, ifc := range snap.Interfaces {
		if !ifc.IsUp {
			continue
		}
		if !IsVPNInterface(ifc) {
			continue
		}
		c := Conflict{
			Type:        TypeVPN,
			Interface:   ifc.Name,
			Description: fmt.Sprintf("VPN 适配器在线: %s (%s)", ifc.Name, ifc.FriendlyName),
		}
		if profileWantsDefault {
			c.Description += "；可能与默认路由管理冲突"
		}
		out = append(out, c)
	}
	return out
}

// CheckExternalOverride flags want destinations whose live route differs from
// what NetSwitcher manages, or which are served by a VPN interface. Expensive
// (PowerShell) — call from GUI only. managed is the state.json set.
func (d *Detector) CheckExternalOverride(rows []RouteRow, want []state.Entry, managed []state.Entry) []Conflict {
	managedByDest := make(map[string]bool, len(managed))
	for _, e := range managed {
		managedByDest[e.Destination] = true
	}
	wantByDest := make(map[string]bool, len(want))
	for _, e := range want {
		wantByDest[e.Destination] = true
	}

	var out []Conflict
	for _, row := range rows {
		if !wantByDest[row.DestinationPrefix] {
			continue
		}
		if managedByDest[row.DestinationPrefix] {
			continue
		}
		out = append(out, Conflict{
			Type:        TypeExternalOverride,
			Destination: row.DestinationPrefix,
			Interface:   row.InterfaceAlias,
			Description: fmt.Sprintf("目标 %s 已被外部覆盖 (下一跳 %s, 接口 %s)",
				row.DestinationPrefix, row.NextHop, row.InterfaceAlias),
		})
	}
	return out
}

// RouteRow is one IPv4 route as returned by Get-NetRoute (§11.3).
type RouteRow struct {
	DestinationPrefix string `json:"DestinationPrefix"` // "168.168.0.0/16"
	NextHop           string `json:"NextHop"`
	InterfaceIndex    int    `json:"InterfaceIndex"`
	InterfaceAlias    string `json:"InterfaceAlias"`
	RouteMetric       int    `json:"RouteMetric"`
	InterfaceMetric   int    `json:"InterfaceMetric"`
}
