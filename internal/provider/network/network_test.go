package network

import (
	"testing"

	"github.com/tinkerbell/wendy/internal/engine"
)

func TestAddresses(t *testing.T) {
	cases := []struct {
		prefix string
		n      int
		want   string
	}{
		{"172.20.0.0/16", 1, "172.20.255.254"},
		{"172.20.0.0/16", 16, "172.20.255.239"},
		{"192.168.48.0/20", 3, "192.168.63.252"},
		{"fd00:ca7e:1::/64", 1, "fd00:ca7e:1:0:ffff:ffff:ffff:fffe"},
	}
	for _, c := range cases {
		got, err := FromTop(c.prefix, c.n)
		if err != nil || got != c.want {
			t.Errorf("FromTop(%s, %d) = %s, %v; want %s", c.prefix, c.n, got, err, c.want)
		}
	}
	if _, err := FromTop("10.0.0.0/30", 8); err == nil {
		t.Error("offset beyond a /30 should fail")
	}
	if m := Netmask("192.168.48.0/20"); m != "255.255.240.0" {
		t.Errorf("netmask = %s", m)
	}

	n := engine.Network{Family: "ipv4", Subnet: "172.20.0.0/16", Gateway: "172.20.0.1"}
	vips, dns64, err := Addresses(n)
	if err != nil || vips.Tinkerbell != "172.20.255.254" || vips.ControlPlane != "172.20.255.252" || dns64 != "" {
		t.Errorf("vips = %+v dns64 = %q err = %v", vips, dns64, err)
	}
	ip, gw, err := MachineIP(n, 2)
	if err != nil || ip != "172.20.255.237" || gw != "172.20.0.1" {
		t.Errorf("machine 2 = %s via %s, %v", ip, gw, err)
	}
}
