package routeengine

import (
	"fmt"
	"net/netip"
)

// parsePrefix parses a canonical IPv4 CIDR. IPv6 is rejected (v1 is IPv4-only
// per spec §2 non-targets).
func parsePrefix(cidr string) (netip.Prefix, error) {
	pfx, err := netip.ParsePrefix(cidr)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("parse CIDR %q: %w", cidr, err)
	}
	if !pfx.Addr().Is4() {
		return netip.Prefix{}, fmt.Errorf("IPv6 CIDR %q not supported in v1", cidr)
	}
	return pfx, nil
}
