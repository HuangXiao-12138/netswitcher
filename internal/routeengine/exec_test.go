package routeengine

import (
	"testing"
)

func TestSplitCIDR(t *testing.T) {
	cases := []struct {
		cidr    string
		dest    string
		mask    string
		wantErr bool
	}{
		{"168.168.0.0/16", "168.168.0.0", "255.255.0.0", false},
		{"10.0.0.0/8", "10.0.0.0", "255.0.0.0", false},
		{"192.168.1.0/24", "192.168.1.0", "255.255.255.0", false},
		{"0.0.0.0/0", "0.0.0.0", "0.0.0.0", false},
		{"32-bit", "", "", true},    // not a CIDR
		{"fe80::/64", "", "", true}, // IPv6 rejected in v1
	}
	for _, c := range cases {
		dest, mask, err := splitCIDR(c.cidr)
		if c.wantErr {
			if err == nil {
				t.Errorf("%q: expected error, got %s %s", c.cidr, dest, mask)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error: %v", c.cidr, err)
			continue
		}
		if dest != c.dest || mask != c.mask {
			t.Errorf("%q: got %s %s, want %s %s", c.cidr, dest, mask, c.dest, c.mask)
		}
	}
}

func TestDecode_UTF8AndGBK(t *testing.T) {
	// ASCII/UTF-8 passes through.
	got := decode([]byte("OK"))
	if got != "OK" {
		t.Errorf("decode ASCII = %q", got)
	}
	// GBK byte sequence for "已存在" (the idempotence marker on Chinese Windows).
	// 已 = 0xD2D1, 存 = 0xB4E6, 在 = 0xD4DA.
	gbk := []byte{0xD2, 0xD1, 0xB4, 0xE6, 0xD4, 0xDA}
	got = decode(gbk)
	if got != "已存在" {
		t.Errorf("decode GBK = %q, want 已存在", got)
	}
}

func TestContainsAny(t *testing.T) {
	if !containsAny("路由 对象已存在。", addExistsMarkers) {
		t.Error("expected to find 已存在 marker")
	}
	if !containsAny("The object already exists.", addExistsMarkers) {
		t.Error("expected to find 'already exists' marker")
	}
	if containsAny("some unrelated text", addExistsMarkers) {
		t.Error("false positive on unrelated text")
	}
	if !containsAny("找不到元素。", deleteMissingMarkers) {
		t.Error("expected to find 找不到 marker")
	}
}
