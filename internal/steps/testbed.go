package steps

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/provider/bmc"
	"github.com/tinkerbell/wendy/internal/provider/distro"
	"github.com/tinkerbell/wendy/internal/provider/network"
	"github.com/tinkerbell/wendy/internal/provider/registry"
)

type clusterParam struct {
	Cluster string `json:"cluster"`
}

func init() {
	engine.Register("testbed.network",
		"Creates the combination's Docker bridge; for IPv6 also Router Advertisements, DNS64 and NAT64; starts registry mirrors.",
		"{}", testbedNetwork)
	engine.Register("testbed.registry",
		"Joins the local registry that carries source-built images to the combination's network.",
		"{}", testbedRegistry)
	engine.Register("testbed.distro",
		"Creates the management or Tinkerbell cluster on the combination's network.",
		`{cluster: *"management" | "tinkerbell"}`, testbedDistro)
	engine.Register("testbed.bmc",
		"Joins the shared virtual BMC to the combination's network.",
		"{}", testbedBMC)
}

func logf(s *engine.Step) func(string) { return func(m string) { s.Log.Info(m) } }

func testbedNetwork(ctx context.Context, s *engine.Step, _ struct{}) error {
	n, err := network.Create(ctx, s.Data, s.Testbed)
	if err != nil {
		return err
	}
	vips, dns64, err := network.Addresses(n)
	if err != nil {
		return err
	}
	n.DNS64 = dns64
	if err := s.Testbed.Update(func(st *engine.State) { st.Network, st.VIPs = n, vips }); err != nil {
		return err
	}
	s.Log.Info("network ready", "name", n.Name, "bridge", n.Bridge, "subnet", n.Subnet, "subnet6", n.Subnet6)

	if n.Family == "ipv6" {
		if err := network.EnsureIPv6(ctx, s.Data, s.Testbed, n, logf(s)); err != nil {
			return err
		}
	}
	if m := s.Testbed.Config.Network.Mirror; m.Enabled {
		mirrors := map[string]string{}
		for _, up := range m.Upstreams {
			addr, err := registry.EnsureMirror(ctx, up, n.Name, n.Family, logf(s))
			if err != nil {
				return fmt.Errorf("mirror for %s: %w", up, err)
			}
			if err := s.Testbed.Track(engine.Resource{Kind: "mirror-net", Name: registry.MirrorName(up), Attrs: map[string]string{"network": n.Name}}); err != nil {
				return err
			}
			mirrors[up] = addr
		}
		if err := s.Testbed.Update(func(st *engine.State) { st.Registry.Mirrors = mirrors }); err != nil {
			return err
		}
	}
	s.Outputs["network"] = n.Name
	s.Outputs["vips"] = vips
	return nil
}

func testbedRegistry(ctx context.Context, s *engine.Step, _ struct{}) error {
	if len(s.Builds.Snapshot()) == 0 {
		return engine.Skip("nothing is built from source")
	}
	n := s.Testbed.State().Network
	if err := registry.Ensure(ctx, logf(s)); err != nil {
		return err
	}
	pull, err := registry.Join(ctx, n.Name, n.Family)
	if err != nil {
		return err
	}
	if err := s.Testbed.Track(engine.Resource{Kind: "registry-net", Name: n.Name}); err != nil {
		return err
	}
	s.Outputs["pull"] = pull
	return s.Testbed.Update(func(st *engine.State) { st.Registry.Pull, st.Registry.Push = pull, registry.PushEndpoint })
}

func testbedDistro(ctx context.Context, s *engine.Step, p clusterParam) error {
	cfg := s.Testbed.Config
	d, name := cfg.ManagementCluster, s.Testbed.HostName("")
	if p.Cluster == engine.TinkerbellCluster {
		if cfg.Tinkerbell.Cluster == nil {
			return engine.Skip("Tinkerbell is colocated")
		}
		d, name = *cfg.Tinkerbell.Cluster, s.Testbed.HostName("tink")
	}
	prov, err := distro.For(d)
	if err != nil {
		return err
	}
	rt, err := runtimeOf(ctx, s)
	if err != nil {
		return err
	}
	if err := writeCertsDir(rt); err != nil {
		return err
	}
	if err := s.Testbed.Track(engine.Resource{Kind: d.Type, Name: name}); err != nil {
		return err
	}
	kc, err := prov.Up(ctx, s.Testbed, name, d, rt, logf(s))
	if err != nil {
		return err
	}
	if err := s.Testbed.SetKubeconfig(p.Cluster, kc); err != nil {
		return err
	}
	if dns64 := s.Testbed.State().Network.DNS64; dns64 != "" {
		if err := repointCoreDNS(ctx, s, p.Cluster, dns64); err != nil {
			return fmt.Errorf("pointing CoreDNS at DNS64: %w", err)
		}
	}
	s.Outputs["cluster"] = name
	return nil
}

// repointCoreDNS sends a cluster's off-cluster lookups to DNS64. kind points
// CoreDNS at the node's IPv4 resolver, which an IPv6-only pod cannot reach.
func repointCoreDNS(ctx context.Context, s *engine.Step, cluster, dns64 string) error {
	c, err := kube(s, cluster)
	if err != nil {
		return err
	}
	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: "kube-system", Name: "coredns"}, cm); err != nil {
		return err
	}
	// A bare address: CoreDNS rejects a bracketed one without a port.
	cm.Data["Corefile"] = strings.Replace(cm.Data["Corefile"], "forward . /etc/resolv.conf", "forward . "+dns64, 1)
	if err := c.Update(ctx, cm); err != nil {
		return err
	}
	dep := &appsv1.Deployment{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: "kube-system", Name: "coredns"}, dep); err != nil {
		return err
	}
	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = map[string]string{}
	}
	dep.Spec.Template.Annotations["wendy/restartedAt"] = time.Now().Format(time.RFC3339)
	return c.Update(ctx, dep)
}

func testbedBMC(ctx context.Context, s *engine.Step, _ struct{}) error {
	cfg := s.Testbed.Config.Hardware
	n := s.Testbed.State().Network
	if err := bmc.Ensure(ctx, s.Data, cfg.BMC.Image, n.Name, cfg.Backend.Firmware, logf(s)); err != nil {
		return err
	}
	if err := s.Testbed.Track(engine.Resource{Kind: "vbmc-net", Name: n.Name}); err != nil {
		return err
	}
	host, err := bmc.Host(ctx, n.Name, n.Family)
	if err != nil {
		return err
	}
	if s.Testbed.Config.Tinkerbell.BootMode == "isoboot" {
		if err := bmc.EnsurePool(s.Data, cfg.Backend.URI, s.Testbed.ID); err != nil {
			return err
		}
		if err := s.Testbed.Track(engine.Resource{Kind: "pool", Name: s.Testbed.ID, Attrs: map[string]string{"uri": cfg.Backend.URI}}); err != nil {
			return err
		}
	}
	s.Outputs["host"] = host
	return s.Testbed.Update(func(st *engine.State) {
		st.BMC = engine.BMC{Host: host, Container: bmc.Container, Protocol: cfg.BMC.Protocol}
	})
}
