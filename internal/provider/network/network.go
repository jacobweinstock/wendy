// Package network provides each combination's L2: a Docker bridge that the
// kind nodes, the shared services and the libvirt machines all join.
package network

import (
	"context"
	"fmt"
	"math/big"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"

	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/provider/docker"
	"github.com/tinkerbell/wendy/internal/provider/hostalloc"
)

// Labels on every network wendy creates.
const (
	LabelExecution   = "wendy.execution"
	LabelCombination = "wendy.combination"
	LabelID          = "wendy.id"
)

// Offsets from the top of the subnet. Docker hands out container addresses
// from the bottom, so the top is free for addresses wendy picks.
const (
	offsetTinkerbell   = 1
	offsetHookOS       = 2
	offsetControlPlane = 3
	offsetDNS64        = 4
	offsetMachines     = 16
)

// IPv6 translation, shared by every IPv6 combination on the host.
const (
	NAT64Container = "wendy-nat64"
	NAT64Prefix    = "fd00:64::/96"
	tunDev         = "wendy64"
	taygaIPv4      = "192.168.254.1"
	taygaPool      = "192.168.254.0/24"
	netImage       = "docker.io/library/debian:stable-slim"
	cleanupImage   = "docker.io/library/alpine:3.22"
	corednsImage   = "docker.io/coredns/coredns:1.14.7"
	nat64Service   = "nat64"
)

// Create makes the combination's network and records it in the state.
func Create(ctx context.Context, data string, tb *engine.Testbed) (engine.Network, error) {
	c, err := docker.Client()
	if err != nil {
		return engine.Network{}, err
	}
	name := tb.HostName("")
	family := tb.Config.Network.Family
	out := engine.Network{Name: name, Family: family}

	if _, err := c.NetworkInspect(ctx, name, client.NetworkInspectOptions{}); docker.IsNotFound(err) {
		opts := client.NetworkCreateOptions{
			Driver: "bridge",
			Options: map[string]string{
				"com.docker.network.bridge.enable_ip_masquerade": "true",
			},
			Labels: map[string]string{LabelExecution: tb.Execution, LabelCombination: tb.Name, LabelID: tb.ID},
		}
		if family == "ipv6" {
			var sub6 string
			err := hostalloc.Update(data, func(h *hostalloc.Host) error {
				var err error
				sub6, err = h.Subnet6(tb.ID, subnetInUse(ctx))
				return err
			})
			if err != nil {
				return out, err
			}
			p := netip.MustParsePrefix(sub6)
			on := true
			opts.EnableIPv6 = &on
			// Asked for by name: some engines report an auto-assigned IPv6
			// gateway as empty.
			opts.IPAM = &network.IPAM{Config: []network.IPAMConfig{{Subnet: p, Gateway: p.Addr().Next()}}}
		}
		if _, err := c.NetworkCreate(ctx, name, opts); err != nil {
			return out, fmt.Errorf("creating network %s: %w", name, err)
		}
	} else if err != nil {
		return out, err
	}
	if err := tb.Track(engine.Resource{Kind: "network", Name: name}); err != nil {
		return out, err
	}

	ins, err := c.NetworkInspect(ctx, name, client.NetworkInspectOptions{})
	if err != nil {
		return out, err
	}
	out.Bridge = "br-" + ins.Network.ID[:12]
	for _, cfg := range ins.Network.IPAM.Config {
		if cfg.Subnet.Addr().Is4() {
			out.Subnet, out.Gateway = cfg.Subnet.String(), cfg.Gateway.String()
		} else if cfg.Subnet.IsValid() {
			out.Subnet6, out.Gateway6 = cfg.Subnet.String(), cfg.Gateway.String()
		}
	}
	if out.Subnet == "" && out.Subnet6 == "" {
		return out, fmt.Errorf("network %s has no subnet", name)
	}
	return out, nil
}

// subnetInUse reports subnets other Docker networks already have.
func subnetInUse(ctx context.Context) func(string) bool {
	c, err := docker.Client()
	if err != nil {
		return nil
	}
	used := map[string]bool{}
	if nets, err := c.NetworkList(ctx, client.NetworkListOptions{}); err == nil {
		for _, n := range nets.Items {
			if ins, err := c.NetworkInspect(ctx, n.ID, client.NetworkInspectOptions{}); err == nil {
				for _, cfg := range ins.Network.IPAM.Config {
					used[cfg.Subnet.String()] = true
				}
			}
		}
	}
	return func(s string) bool { return used[s] }
}

// Delete removes a network; a missing one is not an error.
func Delete(ctx context.Context, name string) error {
	c, err := docker.Client()
	if err != nil {
		return err
	}
	_, err = c.NetworkRemove(ctx, name, client.NetworkRemoveOptions{})
	if docker.IsNotFound(err) {
		return nil
	}
	return err
}

// FromTop is the address n below the top of the prefix: the broadcast
// address for IPv4, the last address for IPv6.
func FromTop(prefix string, n int) (string, error) {
	p, err := netip.ParsePrefix(prefix)
	if err != nil {
		return "", err
	}
	p = p.Masked()
	bits := p.Addr().BitLen()
	host := new(big.Int).Lsh(big.NewInt(1), uint(bits-p.Bits()))
	if big.NewInt(int64(n)).Cmp(host) >= 0 {
		return "", fmt.Errorf("%s is too small for offset %d", prefix, n)
	}
	base := new(big.Int).SetBytes(p.Addr().AsSlice())
	v := new(big.Int).Add(base, host)
	v.Sub(v, big.NewInt(int64(n+1)))
	b := v.FillBytes(make([]byte, bits/8))
	a, _ := netip.AddrFromSlice(b)
	return a.String(), nil
}

// Netmask is the dotted mask of an IPv4 prefix.
func Netmask(prefix string) string {
	p, err := netip.ParsePrefix(prefix)
	if err != nil || !p.Addr().Is4() {
		return ""
	}
	return net.IP(net.CIDRMask(p.Bits(), 32)).String()
}

// Addresses picks the VIPs and the DNS64 address of a network.
func Addresses(n engine.Network) (engine.VIPs, string, error) {
	prefix := n.Subnet
	if n.Family == "ipv6" {
		prefix = n.Subnet6
	}
	var v engine.VIPs
	var err error
	pick := func(off int) string {
		if err != nil {
			return ""
		}
		var a string
		a, err = FromTop(prefix, off)
		return a
	}
	v.Tinkerbell, v.HookOS, v.ControlPlane = pick(offsetTinkerbell), pick(offsetHookOS), pick(offsetControlPlane)
	dns64 := ""
	if n.Family == "ipv6" {
		dns64 = pick(offsetDNS64)
	}
	return v, dns64, err
}

// MachineIP is the address of the i-th machine (0-based).
func MachineIP(n engine.Network, i int) (string, string, error) {
	prefix, gw := n.Subnet, n.Gateway
	if n.Family == "ipv6" {
		prefix, gw = n.Subnet6, n.Gateway6
	}
	ip, err := FromTop(prefix, offsetMachines+i)
	return ip, gw, err
}

// EnsureIPv6 starts the IPv6 layer for a combination: forwarding, Router
// Advertisements on its bridge, a DNS64 resolver on its network and the
// shared NAT64 translator.
func EnsureIPv6(ctx context.Context, data string, tb *engine.Testbed, n engine.Network, log func(string)) error {
	// Router Advertisements only go out on forwarding interfaces, and NAT64
	// forwards between the bridge and its TUN device.
	if err := hostShell(ctx, netImage, "echo 1 > /proc/sys/net/ipv6/conf/all/forwarding; echo 1 > /proc/sys/net/ipv4/ip_forward", log); err != nil {
		return fmt.Errorf("enabling forwarding: %w", err)
	}
	// A network that went through a daemon restart can lose its bridge
	// address, and NAT64 replies then have no way home.
	if p, err := netip.ParsePrefix(n.Subnet6); err == nil {
		_ = hostShell(ctx, netImage, fmt.Sprintf("%s; ip -6 addr add %s/%d dev %s 2>/dev/null || true", aptIPRoute, n.Gateway6, p.Bits(), n.Bridge), log)
	}

	dir := filepath.Join(tb.Dir, "net")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var radvd, corefile string
	rt := map[string]any{"bridge": n.Bridge, "subnet6": n.Subnet6, "nat64Prefix": NAT64Prefix}
	if err := tb.Render(rt, "radvd", &radvd, nil); err != nil {
		return err
	}
	if err := tb.Render(rt, "dns64Corefile", &corefile, nil); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "radvd.conf"), []byte(radvd), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "Corefile"), []byte(corefile), 0o644); err != nil {
		return err
	}

	radvdName := tb.HostName("radvd")
	if err := docker.Ensure(ctx, docker.Spec{
		Name: radvdName, Image: netImage, HostNet: true, CapAdd: []string{"NET_ADMIN", "NET_RAW"},
		Binds:      []string{filepath.Join(dir, "radvd.conf") + ":/etc/radvd.conf:ro"},
		Entrypoint: []string{"/bin/sh", "-c"},
		Cmd: []string{"set -e; export DEBIAN_FRONTEND=noninteractive; apt-get update -qq >/dev/null && " +
			"apt-get install -y -qq --no-install-recommends radvd >/dev/null 2>&1; " +
			"exec radvd --nodaemon --logmethod stderr --config /etc/radvd.conf"},
	}, log); err != nil {
		return err
	}
	if err := tb.Track(engine.Resource{Kind: "container", Name: radvdName}); err != nil {
		return err
	}

	dnsName := tb.HostName("dns64")
	if err := docker.Ensure(ctx, docker.Spec{
		Name: dnsName, Image: corednsImage, Network: n.Name, IPv6: n.DNS64,
		Binds: []string{filepath.Join(dir, "Corefile") + ":/Corefile:ro"},
		Cmd:   []string{"-conf", "/Corefile"},
	}, log); err != nil {
		return err
	}
	if err := tb.Track(engine.Resource{Kind: "container", Name: dnsName}); err != nil {
		return err
	}

	if !tb.Config.Network.NAT64 {
		return nil
	}
	if err := hostalloc.Update(data, func(h *hostalloc.Host) error { h.AddUser(nat64Service, tb.ID); return nil }); err != nil {
		return err
	}
	if err := tb.Track(engine.Resource{Kind: "nat64", Name: tb.ID}); err != nil {
		return err
	}
	if err := docker.Ensure(ctx, docker.Spec{
		Name: NAT64Container, Image: netImage, HostNet: true, Privileged: true,
		Entrypoint: []string{"/bin/sh", "-c"},
		Cmd: []string{"set -e; " + aptTayga + "; " + iptPick + "; mkdir -p /var/spool/tayga; " +
			fmt.Sprintf("printf 'tun-device %s\\nipv4-addr %s\\nprefix %s\\ndynamic-pool %s\\ndata-dir /var/spool/tayga\\n' > /etc/tayga.conf; ", tunDev, taygaIPv4, NAT64Prefix, taygaPool) +
			fmt.Sprintf("tayga --mktun -c /etc/tayga.conf; ip link set %[1]s up; ip addr add %[2]s dev %[1]s 2>/dev/null || true; "+
				"ip route add %[3]s dev %[1]s 2>/dev/null || true; ip -6 route add %[4]s dev %[1]s 2>/dev/null || true; ", tunDev, taygaIPv4, taygaPool, NAT64Prefix) +
			fmt.Sprintf("$IPT -t nat -C POSTROUTING -s %[1]s ! -o %[2]s -j MASQUERADE 2>/dev/null || $IPT -t nat -A POSTROUTING -s %[1]s ! -o %[2]s -j MASQUERADE; ", taygaPool, tunDev) +
			fmt.Sprintf("for ipt in $IPT $IPT6; do for dir in -i -o; do $ipt -C FORWARD $dir %[1]s -j ACCEPT 2>/dev/null || $ipt -A FORWARD $dir %[1]s -j ACCEPT; done; done; ", tunDev) +
			"exec tayga -d -c /etc/tayga.conf --nodetach"},
	}, log); err != nil {
		return err
	}
	// The route appears once tayga has installed its packages and started.
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		if out, _ := os.ReadFile("/proc/net/ipv6_route"); strings.Contains(string(out), tunDev) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	logs, _ := docker.Logs(ctx, NAT64Container, "20")
	return fmt.Errorf("NAT64 did not come up:\n%s", logs)
}

// ReleaseNAT64 drops an environment's use of the shared translator, and
// removes it, with what it left in the host's tables, once nobody uses it.
func ReleaseNAT64(ctx context.Context, data, envID string) error {
	still := true
	if err := hostalloc.Update(data, func(h *hostalloc.Host) error {
		still = h.RemoveUser(nat64Service, envID)
		return nil
	}); err != nil {
		return err
	}
	if still {
		return nil
	}
	if err := docker.Remove(ctx, NAT64Container); err != nil {
		return err
	}
	// The TUN device and the rules outlive the container.
	return hostShell(ctx, cleanupImage, "apk add --no-cache iproute2-minimal iptables ip6tables iptables-legacy >/dev/null 2>&1 || true; "+iptPick+"; "+
		fmt.Sprintf("while $IPT -t nat -D POSTROUTING -s %[1]s ! -o %[2]s -j MASQUERADE 2>/dev/null; do :; done; ", taygaPool, tunDev)+
		fmt.Sprintf("for ipt in $IPT $IPT6; do for dir in -i -o; do while $ipt -D FORWARD $dir %[1]s -j ACCEPT 2>/dev/null; do :; done; done; done; ", tunDev)+
		fmt.Sprintf("ip link del %s 2>/dev/null || true", tunDev), nil)
}

const (
	aptIPRoute = "export DEBIAN_FRONTEND=noninteractive; apt-get update -qq >/dev/null && apt-get install -y -qq --no-install-recommends iproute2 >/dev/null 2>&1"
	aptTayga   = "export DEBIAN_FRONTEND=noninteractive; apt-get update -qq >/dev/null && apt-get install -y -qq --no-install-recommends tayga iptables iproute2 >/dev/null 2>&1"
	// The masquerade rule has to land in the backend Docker uses.
	iptPick = `IPT=iptables; iptables-legacy -t nat -S 2>/dev/null | grep -q DOCKER && IPT=iptables-legacy; IPT6=ip6tables; case "$IPT" in *-legacy) IPT6=ip6tables-legacy ;; esac`
)

// hostShell runs a shell snippet in a privileged container sharing the
// host's network namespace, so wendy never needs sudo.
func hostShell(ctx context.Context, image, script string, log func(string)) error {
	name := fmt.Sprintf("wendy-hostsh-%d", time.Now().UnixNano())
	if err := docker.Ensure(ctx, docker.Spec{
		Name: name, Image: image, HostNet: true, Privileged: true, OneShot: true,
		Entrypoint: []string{"/bin/sh", "-c"}, Cmd: []string{script},
	}, log); err != nil {
		return err
	}
	defer docker.Remove(context.Background(), name) //nolint:errcheck // A leftover one-shot container is harmless and does not change the result.
	c, err := docker.Client()
	if err != nil {
		return err
	}
	res := c.ContainerWait(ctx, name, client.ContainerWaitOptions{})
	select {
	case r := <-res.Result:
		if r.StatusCode != 0 {
			out, _ := docker.Logs(ctx, name, "20")
			return fmt.Errorf("exit %d: %s", r.StatusCode, out)
		}
		return nil
	case err := <-res.Error:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func init() {
	engine.RegisterTeardown("network", func(ctx context.Context, _ *engine.Testbed, r engine.Resource) error {
		// Endpoints left by a crashed step would block removal.
		if c, err := docker.Client(); err == nil {
			if ins, err := c.NetworkInspect(ctx, r.Name, client.NetworkInspectOptions{}); err == nil {
				for id := range ins.Network.Containers {
					_ = docker.Disconnect(ctx, r.Name, id)
				}
			}
		}
		return Delete(ctx, r.Name)
	})
	engine.RegisterTeardown("container", func(ctx context.Context, _ *engine.Testbed, r engine.Resource) error {
		return docker.Remove(ctx, r.Name)
	})
	engine.RegisterTeardown("nat64", func(ctx context.Context, tb *engine.Testbed, r engine.Resource) error {
		return ReleaseNAT64(ctx, tb.DataDir(), r.Name)
	})
}
