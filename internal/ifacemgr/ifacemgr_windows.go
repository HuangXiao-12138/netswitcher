//go:build windows

package ifacemgr

import (
	"fmt"
	"net"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// GAA flags (spec §7.3; the windows package only declares INCLUDE_PREFIX).
const (
	gaaFlagIncludePrefix   uint32 = 0x00000010
	gaaFlagIncludeGateways uint32 = 0x00000080
)

// Snapshot enumerates IPv4 adapters via GetAdaptersAddresses. Two passes:
// probe for size, then read. Gateways come from FirstGatewayAddress (requires
// GAA_FLAG_INCLUDE_GATEWAYS); unicast IPv4 + prefix length from
// FirstUnicastAddress (OnLinkPrefixLength, requires INCLUDE_PREFIX).
func (m *Manager) Snapshot() (Snapshot, error) {
	flags := gaaFlagIncludePrefix | gaaFlagIncludeGateways

	var size uint32
	if err := windows.GetAdaptersAddresses(windows.AF_INET, flags, 0, nil, &size); err != nil &&
		err != windows.ERROR_BUFFER_OVERFLOW && err != syscall.ERROR_MORE_DATA {
		return Snapshot{}, fmt.Errorf("GetAdaptersAddresses probe: %w", err)
	}

	if size == 0 {
		return Snapshot{TakenAt: time.Now()}, nil
	}

	buf := make([]byte, size)
	head := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
	if err := windows.GetAdaptersAddresses(windows.AF_INET, flags, 0, head, &size); err != nil {
		return Snapshot{}, fmt.Errorf("GetAdaptersAddresses read: %w", err)
	}

	out := Snapshot{TakenAt: time.Now()}
	for a := head; a != nil; a = a.Next {
		// Skip loopback in the user-facing list; routing for loopback is
		// irrelevant and clutters the GUI.
		if a.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK {
			continue
		}

		ifc := Interface{
			Index:        int(a.IfIndex),
			Name:         windows.UTF16PtrToString(a.FriendlyName),
			FriendlyName: windows.UTF16PtrToString(a.Description),
			IfType:       a.IfType,
			MediaType:    MediaTypeFromIfType(a.IfType),
			IsUp:         a.OperStatus == windows.IfOperStatusUp,
			MAC:          formatMAC(a.PhysicalAddress[:a.PhysicalAddressLength]),
			Metric:       int(a.Ipv4Metric),
		}

		for ua := a.FirstUnicastAddress; ua != nil; ua = ua.Next {
			ip, ok := ipv4FromSockaddr(ua.Address.Sockaddr)
			if !ok {
				continue
			}
			ifc.IPv4 = append(ifc.IPv4, fmt.Sprintf("%s/%d", ip.String(), ua.OnLinkPrefixLength))
		}

		for ga := a.FirstGatewayAddress; ga != nil; ga = ga.Next {
			ip, ok := ipv4FromSockaddr(ga.Address.Sockaddr)
			if !ok {
				continue
			}
			ifc.Gateways = append(ifc.Gateways, ip.String())
		}

		out.Interfaces = append(out.Interfaces, ifc)
	}
	return out, nil
}

// ipv4FromSockaddr extracts an IPv4 address from a sockaddr if it is AF_INET.
func ipv4FromSockaddr(sa *syscall.RawSockaddrAny) (net.IP, bool) {
	if sa == nil {
		return nil, false
	}
	family := (*syscall.RawSockaddr)(unsafe.Pointer(sa)).Family
	if family != syscall.AF_INET {
		return nil, false
	}
	inet := (*syscall.RawSockaddrInet4)(unsafe.Pointer(sa))
	out := make(net.IP, 4)
	copy(out, inet.Addr[:])
	return out, true
}

// formatMAC renders a MAC as aa:bb:cc:dd:ee:ff.
func formatMAC(b []byte) string {
	if len(b) < 6 {
		return ""
	}
	return net.HardwareAddr(b[:6]).String()
}
