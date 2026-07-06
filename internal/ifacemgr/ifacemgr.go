// Package ifacemgr enumerates local network interfaces and resolves the
// human-readable names used in config (e.g. "以太网", "WLAN") to the
// interface index + gateway that route.exe needs.
//
// Snapshot() is the live call (Windows-specific). ResolveByName is a pure
// function over a Snapshot so the matching rules in §7.3 are unit-testable
// without touching the OS.
package ifacemgr

import (
	"errors"
	"strings"
	"time"
)

// Snapshot is a point-in-time view of all local interfaces.
type Snapshot struct {
	Interfaces []Interface
	TakenAt    time.Time
}

// Interface mirrors the fields route.exe / netsh need plus what the GUI shows.
// Field names follow the spec §7.3 shape.
type Interface struct {
	Index        int      // route.exe "IF" argument
	Name         string   // NetConnectionID, e.g. "以太网" / "WLAN"
	FriendlyName string   // adapter Description (driver/driver-desc)
	MAC          string   // "aa:bb:cc:dd:ee:ff"
	IPv4         []string // "172.16.5.10/24"
	Gateways     []string // current IPv4 default gateways, e.g. ["172.16.5.1"]
	IsUp         bool
	Metric       int    // interface metric (lower wins the default route on 0.0.0.0/0)
	MediaType    string // "Ethernet" | "WiFi" | "Loopback" | "VPN" | ...
	IfType       uint32 // raw IF_TYPE_*
}

// Errors.
var (
	// ErrInterfaceNotFound is returned by ResolveByName when no interface
	// matches per the §7.3 priority rules. The route engine treats this as a
	// Skipped rule, not a hard failure.
	ErrInterfaceNotFound = errors.New("interface not found")
)

// MediaType constants derived from Windows IF_TYPE (IF-MIB). Only the kinds
// NetSwitcher cares about are named.
const (
	IFTypeEthernet         uint32 = 6
	IFTypeSoftwareLoopback uint32 = 24
	IFTypeWiFi             uint32 = 71
	IFTypeTunnel           uint32 = 131
)

// MediaTypeFromIfType maps the numeric IfType to a human label.
func MediaTypeFromIfType(t uint32) string {
	switch t {
	case IFTypeEthernet:
		return "Ethernet"
	case IFTypeWiFi:
		return "WiFi"
	case IFTypeSoftwareLoopback:
		return "Loopback"
	case IFTypeTunnel:
		return "Tunnel"
	default:
		return "Other"
	}
}

// Manager is the live interface enumerator. Construct once, reuse.
type Manager struct {
	// no state for now; Snapshot always reads live. Keeps a struct (not bare
	// functions) so we can add caching later without changing call sites.
}

// New returns a Manager.
func New() *Manager { return &Manager{} }

// FindByPredicate returns the first interface matching pred, or nil.
func (s Snapshot) FindByPredicate(pred func(Interface) bool) *Interface {
	for i := range s.Interfaces {
		if pred(s.Interfaces[i]) {
			return &s.Interfaces[i]
		}
	}
	return nil
}

// ResolveByName resolves a config interface reference to a live interface
// using the §7.3 priority rules:
//
//  1. exact match on Name (NetConnectionID)
//  2. exact match on FriendlyName (Description)
//  3. case-insensitive contains match on either
//
// Returns ErrInterfaceNotFound if nothing matches.
func ResolveByName(name string, snap Snapshot) (*Interface, error) {
	target := strings.TrimSpace(name)
	if target == "" {
		return nil, ErrInterfaceNotFound
	}

	// 1. exact Name.
	if ifc := snap.FindByPredicate(func(i Interface) bool {
		return strings.TrimSpace(i.Name) == target
	}); ifc != nil {
		return ifc, nil
	}

	// 2. exact FriendlyName (Description).
	if ifc := snap.FindByPredicate(func(i Interface) bool {
		return strings.TrimSpace(i.FriendlyName) == target
	}); ifc != nil {
		return ifc, nil
	}

	// 3. case-insensitive contains on either field.
	lower := strings.ToLower(target)
	if ifc := snap.FindByPredicate(func(i Interface) bool {
		return strings.Contains(strings.ToLower(i.Name), lower) ||
			strings.Contains(strings.ToLower(i.FriendlyName), lower)
	}); ifc != nil {
		return ifc, nil
	}

	return nil, ErrInterfaceNotFound
}

// PrimaryGateway returns the first IPv4 gateway of the interface or "".
func (i Interface) PrimaryGateway() string {
	if len(i.Gateways) == 0 {
		return ""
	}
	return i.Gateways[0]
}

// FirstIPv4Prefix returns the first IPv4 "addr/prefix" or "".
func (i Interface) FirstIPv4Prefix() string {
	if len(i.IPv4) == 0 {
		return ""
	}
	return i.IPv4[0]
}
