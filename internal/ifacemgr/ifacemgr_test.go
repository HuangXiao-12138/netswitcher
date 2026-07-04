package ifacemgr_test

import (
	"errors"
	"testing"

	"github.com/netswitcher/netswitcher/internal/ifacemgr"
)

func sampleSnapshot() ifacemgr.Snapshot {
	return ifacemgr.Snapshot{
		Interfaces: []ifacemgr.Interface{
			{Index: 12, Name: "以太网", FriendlyName: "Intel I219-V", MediaType: "Ethernet"},
			{Index: 7, Name: "WLAN", FriendlyName: "Intel Wireless 9560", MediaType: "WiFi"},
			{Index: 22, Name: "万兆网", FriendlyName: "X520-1", Gateways: []string{"10.101.31.1"}},
		},
	}
}

func TestResolveByName_ExactName(t *testing.T) {
	ifc, err := ifacemgr.ResolveByName("以太网", sampleSnapshot())
	if err != nil {
		t.Fatalf("expected match, got %v", err)
	}
	if ifc.Index != 12 {
		t.Errorf("index = %d, want 12", ifc.Index)
	}
}

func TestResolveByName_ExactDescription(t *testing.T) {
	ifc, err := ifacemgr.ResolveByName("X520-1", sampleSnapshot())
	if err != nil {
		t.Fatalf("expected match on Description, got %v", err)
	}
	if ifc.Index != 22 {
		t.Errorf("index = %d, want 22", ifc.Index)
	}
}

func TestResolveByName_ContainsCaseInsensitive(t *testing.T) {
	// "intel wireless 9560" lower-cased substring of Description.
	ifc, err := ifacemgr.ResolveByName("intel wireless", sampleSnapshot())
	if err != nil {
		t.Fatalf("expected fuzzy match, got %v", err)
	}
	if ifc.Index != 7 {
		t.Errorf("index = %d, want 7", ifc.Index)
	}
}

func TestResolveByName_NotFound(t *testing.T) {
	_, err := ifacemgr.ResolveByName("does-not-exist", sampleSnapshot())
	if !errors.Is(err, ifacemgr.ErrInterfaceNotFound) {
		t.Fatalf("expected ErrInterfaceNotFound, got %v", err)
	}
}

func TestResolveByName_Empty(t *testing.T) {
	_, err := ifacemgr.ResolveByName("   ", sampleSnapshot())
	if !errors.Is(err, ifacemgr.ErrInterfaceNotFound) {
		t.Fatalf("expected ErrInterfaceNotFound for blank input, got %v", err)
	}
}

// Spec §7.3 priority: exact Name must win over a fuzzy match that would
// otherwise match Description first.
func TestResolveByName_PriorityExactNameWins(t *testing.T) {
	snap := ifacemgr.Snapshot{
		Interfaces: []ifacemgr.Interface{
			{Index: 1, Name: "WLAN", FriendlyName: "WLAN adapter"},      // exact Name
			{Index: 2, Name: "other", FriendlyName: "WLAN description"}, // Description contains WLAN
		},
	}
	ifc, err := ifacemgr.ResolveByName("WLAN", snap)
	if err != nil {
		t.Fatalf("expected match, got %v", err)
	}
	if ifc.Index != 1 {
		t.Errorf("priority broken: index = %d, want 1 (exact Name)", ifc.Index)
	}
}

func TestMediaTypeFromIfType(t *testing.T) {
	cases := []struct {
		t    uint32
		want string
	}{
		{ifacemgr.IFTypeEthernet, "Ethernet"},
		{ifacemgr.IFTypeWiFi, "WiFi"},
		{ifacemgr.IFTypeSoftwareLoopback, "Loopback"},
		{ifacemgr.IFTypeTunnel, "Tunnel"},
		{999, "Other"},
	}
	for _, c := range cases {
		if got := ifacemgr.MediaTypeFromIfType(c.t); got != c.want {
			t.Errorf("IfType %d = %q, want %q", c.t, got, c.want)
		}
	}
}
