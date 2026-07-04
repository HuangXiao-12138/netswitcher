package netwatch

import (
	"testing"

	"github.com/netswitcher/netswitcher/internal/ifacemgr"
)

func TestDiff_NoChange(t *testing.T) {
	s := snap(
		ifacemgr.Interface{Index: 7, Name: "WLAN", IsUp: true, IPv4: []string{"192.168.1.5/24"}, Gateways: []string{"192.168.1.1"}},
	)
	if _, changed := Diff(s, s); changed {
		t.Fatal("identical snapshots should not report change")
	}
}

func TestDiff_UpFlip(t *testing.T) {
	prev := snap(ifacemgr.Interface{Index: 7, Name: "WLAN", IsUp: false})
	cur := snap(ifacemgr.Interface{Index: 7, Name: "WLAN", IsUp: true})
	desc, changed := Diff(prev, cur)
	if !changed {
		t.Fatal("expected change on up/down flip")
	}
	if !contains(desc, "up") {
		t.Errorf("desc = %q, want up indication", desc)
	}
}

func TestDiff_IPv4Change(t *testing.T) {
	prev := snap(ifacemgr.Interface{Index: 7, Name: "WLAN", IsUp: true, IPv4: []string{"192.168.1.5/24"}})
	cur := snap(ifacemgr.Interface{Index: 7, Name: "WLAN", IsUp: true, IPv4: []string{"192.168.1.6/24"}})
	desc, changed := Diff(prev, cur)
	if !changed {
		t.Fatal("expected change on IPv4 change")
	}
	if !contains(desc, "IPv4") {
		t.Errorf("desc = %q, want IPv4 mention", desc)
	}
}

func TestDiff_GatewayChange(t *testing.T) {
	prev := snap(ifacemgr.Interface{Index: 7, Name: "WLAN", IsUp: true, Gateways: []string{"192.168.1.1"}})
	cur := snap(ifacemgr.Interface{Index: 7, Name: "WLAN", IsUp: true, Gateways: []string{"10.0.0.1"}})
	desc, changed := Diff(prev, cur)
	if !changed || !contains(desc, "gateway") {
		t.Errorf("desc = %q changed=%v", desc, changed)
	}
}

func TestDiff_InterfaceAppeared(t *testing.T) {
	prev := snap()
	cur := snap(ifacemgr.Interface{Index: 12, Name: "以太网", IsUp: true})
	desc, changed := Diff(prev, cur)
	if !changed || !contains(desc, "appeared") {
		t.Errorf("desc = %q changed=%v", desc, changed)
	}
}

func TestDiff_InterfaceDisappeared(t *testing.T) {
	prev := snap(ifacemgr.Interface{Index: 12, Name: "以太网", IsUp: true})
	cur := snap()
	desc, changed := Diff(prev, cur)
	if !changed || !contains(desc, "disappeared") {
		t.Errorf("desc = %q changed=%v", desc, changed)
	}
}

func TestDiff_GatewayOrderInsensitive(t *testing.T) {
	prev := snap(ifacemgr.Interface{Index: 7, Name: "WLAN", IsUp: true, Gateways: []string{"1.1.1.1", "2.2.2.2"}})
	cur := snap(ifacemgr.Interface{Index: 7, Name: "WLAN", IsUp: true, Gateways: []string{"2.2.2.2", "1.1.1.1"}})
	if _, changed := Diff(prev, cur); changed {
		t.Fatal("gateway reorder should not count as a change")
	}
}

func snap(ifcs ...ifacemgr.Interface) ifacemgr.Snapshot {
	return ifacemgr.Snapshot{Interfaces: ifcs}
}

func contains(hay, needle string) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
