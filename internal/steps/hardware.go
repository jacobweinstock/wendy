package steps

import (
	"context"
	"crypto/md5" //nolint:gosec // For stable machine addresses, not security.
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/onsi/gomega"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/provider/bmc"
	"github.com/tinkerbell/wendy/internal/provider/hardware"
	"github.com/tinkerbell/wendy/internal/provider/network"
)

func init() {
	engine.Register("hardware.create",
		"Creates the machines: libvirt domains on the combination's bridge, powered off until Tinkerbell boots them.",
		"{}", hardwareCreate)
	engine.Register("hardware.register",
		"Registers every machine with the virtual BMC and creates its Hardware, BMC Machine and credentials Secret.",
		"{}", hardwareRegister)
}

// Machine roles every testbed has. Config may add others.
const (
	roleControlPlane = "controlPlane"
	roleWorker       = "worker"
	roleSpare        = "spare"
)

// roleOrder puts the roles every testbed has first, then any others by name.
func roleOrder(ms map[string]config.MachineSet) []string {
	known := []string{roleControlPlane, roleWorker, roleSpare}
	var other []string
	for r := range ms {
		if !slices.Contains(known, r) {
			other = append(other, r)
		}
	}
	sort.Strings(other)
	return append(known, other...)
}

// roleLabel is the tinkerbell.org/role value a role's machines carry, which
// the machine templates' hardware affinity selects on.
func roleLabel(role string) string {
	if role == roleControlPlane {
		return "control-plane"
	}
	return role
}

func roleShort(role string) string {
	if role == roleControlPlane {
		return "cp"
	}
	return strings.ToLower(role)
}

// mac is stable for a name, so a machine keeps its address from one
// execution to the next.
func mac(name string) string {
	sum := md5.Sum([]byte(name + "\n")) //nolint:gosec // An address derived from a name, not a secret.
	return fmt.Sprintf("02:%02x:%02x:%02x:%02x:%02x", sum[0], sum[1], sum[2], sum[3], sum[4])
}

func hardwareCreate(ctx context.Context, s *engine.Step, _ struct{}) error {
	tb := s.Testbed
	cfg := tb.Config.Hardware
	n := tb.State().Network

	var ms []hardware.Machine
	i := 0
	for _, role := range roleOrder(cfg.Machines) {
		set, ok := cfg.Machines[role]
		if !ok {
			continue
		}
		for k := 1; k <= set.Count; k++ {
			name := tb.HostName(fmt.Sprintf("%s%d", roleShort(role), k))
			ip, gw, err := network.MachineIP(n, i)
			if err != nil {
				return err
			}
			i++
			labels := map[string]string{}
			for lk, lv := range set.Labels {
				labels[lk] = lv
			}
			ms = append(ms, hardware.Machine{
				Machine: engine.Machine{
					Name: name, Role: roleLabel(role), MAC: mac(name), IP: ip, Gateway: gw, Arch: tb.Config.Arch,
					Labels: labels, Console: filepath.Join("consoles", name+".log"),
				},
				CPUs: set.CPUs, MemMiB: set.MemMiB, DiskGiB: set.DiskGiB,
			})
		}
	}
	var names []string
	for _, m := range ms {
		names = append(names, m.Name)
	}
	if err := tb.Track(engine.Resource{Kind: "bmc-ports", Name: tb.ID}); err != nil {
		return err
	}
	ports, err := bmc.Ports(s.Data, tb.ID, names)
	if err != nil {
		return err
	}
	var state []engine.Machine
	for i := range ms {
		ms[i].BMCPort = ports[ms[i].Name]
		state = append(state, ms[i].Machine)
		s.Log.Info(fmt.Sprintf("%s  %-13s %s  %s  bmc :%d", ms[i].Name, ms[i].Role, ms[i].MAC, ms[i].IP, ms[i].BMCPort))
	}
	if err := tb.Update(func(st *engine.State) { st.Machines = state }); err != nil {
		return err
	}

	hw, err := hardware.For(cfg.Backend)
	if err != nil {
		return err
	}
	if err := hw.Create(ctx, tb, ms); err != nil {
		return err
	}

	if tb.Config.Diagnostics.Consoles {
		// Streamed into the testbed's directory for as long as it lives, so
		// a machine that never boots still leaves its firmware's output.
		dir := filepath.Join(tb.Dir, "consoles")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		tctx, cancel := context.WithCancel(context.Background())
		if err := hardware.StreamConsoles(tctx, tb, cfg.Backend.DiskPath, dir, names, logf(s)); err != nil {
			cancel()
			return err
		}
		tb.Defer("stop console capture", func(context.Context) error {
			cancel()
			return nil
		})
	}
	s.Outputs["machines"] = names
	return nil
}

func hardwareRegister(ctx context.Context, s *engine.Step, _ struct{}) error {
	tb := s.Testbed
	st := tb.State()
	for _, m := range st.Machines {
		if err := tb.Track(engine.Resource{Kind: "vbmc-machine", Name: m.Name}); err != nil {
			return err
		}
		if err := bmc.Register(ctx, m.Name, m.BMCPort); err != nil {
			return err
		}
	}
	rt, err := runtimeOf(ctx, s)
	if err != nil {
		return err
	}
	var secret map[string]any
	var machines, hw []map[string]any
	if err := tb.Render(rt, "bmcSecret", &secret, nil); err != nil {
		return err
	}
	if err := tb.Render(rt, "bmcMachines", &machines, nil); err != nil {
		return err
	}
	if err := tb.Render(rt, "hardware", &hw, nil); err != nil {
		return err
	}
	c, err := kube(s, engine.TinkerbellCluster)
	if err != nil {
		return err
	}
	ns := tb.Config.Tinkerbell.Namespace
	if err := ensureNamespace(ctx, c, ns); err != nil {
		return err
	}
	if err := apply(ctx, c, secret); err != nil {
		return err
	}
	if err := apply(ctx, c, machines...); err != nil {
		return err
	}
	if err := apply(ctx, c, hw...); err != nil {
		return err
	}
	s.Log.Info(fmt.Sprintf("applied %d Hardware and BMC Machines; waiting for every BMC to be contactable", len(hw)))

	wctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	eventually(wctx, 5*time.Second, func(g gomega.Gomega) {
		objs, err := list(wctx, c, "bmc.tinkerbell.org", "Machine", ns)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		var down []string
		for _, o := range objs {
			if condition(o, "Contactable") != "True" {
				down = append(down, o.GetName())
			}
		}
		g.Expect(objs).To(gomega.HaveLen(len(st.Machines)))
		g.Expect(down).To(gomega.BeEmpty(), "BMCs not contactable: %v", down)
	})
	return nil
}
