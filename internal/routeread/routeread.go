// Package routeread reads the live IPv4 route table via PowerShell Get-NetRoute
// (spec §11.3). Slow (~500ms) — used only by the GUI Routes page and the
// conflict detector's external-override scan, never in the apply hot path.
package routeread

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// Row is one IPv4 route. JSON tags match the Property names returned by
// `Get-NetRoute ... | ConvertTo-Json`.
type Row struct {
	DestinationPrefix string `json:"DestinationPrefix"`
	NextHop           string `json:"NextHop"`
	InterfaceIndex    int    `json:"InterfaceIndex"`
	InterfaceAlias    string `json:"InterfaceAlias"`
	RouteMetric       int    `json:"RouteMetric"`
	InterfaceMetric   int    `json:"InterfaceMetric"`
	ifType            uint32
}

// script returns the PowerShell that emits one JSON array of IPv4 routes.
// -NoProfile avoids loading the user profile (§14 PowerShell policy).
// -ExecutionPolicy Bypass overrides restrictive policies.
func script() string {
	return `Get-NetRoute -AddressFamily IPv4 ` +
		`| Select-Object DestinationPrefix,NextHop,InterfaceIndex,InterfaceAlias,RouteMetric,InterfaceMetric ` +
		`| ConvertTo-Json -AsArray`
}

// Read returns the current IPv4 route table. Times out after 8s.
func Read(ctx context.Context) ([]Row, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx,
		"powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-Command", script())
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Get-NetRoute: %w: %s", err, string(out))
	}

	// PowerShell emits a single object (not array) when there's exactly one
	// route; normalize to an array.
	var rows []Row
	if err := json.Unmarshal(out, &rows); err != nil {
		var single Row
		if err2 := json.Unmarshal(out, &single); err2 == nil {
			return []Row{single}, nil
		}
		return nil, fmt.Errorf("parse Get-NetRoute json: %w", err)
	}
	return rows, nil
}

// Source classifies who likely owns a row, for the Routes page coloring.
type Source string

const (
	SourceManaged Source = "managed" // present in state.json
	SourceSystem  Source = "system"  // non-managed, non-VPN
	SourceSuspect Source = "suspect" // VPN / virtual adapter
)
