package steps

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/provider/bmc"
	"github.com/tinkerbell/wendy/internal/provider/network"
	"github.com/tinkerbell/wendy/internal/provider/registry"
)

// Images the workflow and Smee use, before any mirror rewrites them.
const (
	osRegistry      = "ghcr.io/tinkerbell/cluster-api-provider-tinkerbell/ubuntu"
	agentImage      = "ghcr.io/tinkerbell/tink-agent"
	imageOCI2Disk   = "quay.io/tinkerbell/actions/oci2disk"
	imageWritefile  = "quay.io/tinkerbell/actions/writefile"
	imageKexec      = "quay.io/tinkerbell/actions/kexec"
	imageWaitdaemon = "ghcr.io/jacobweinstock/waitdaemon:latest"
)

func metaName(n string) metav1.ObjectMeta { return metav1.ObjectMeta{Name: n} }

// runtimeOf is #Runtime for a testbed, from its state.
func runtimeOf(ctx context.Context, s *engine.Step) (map[string]any, error) {
	tb := s.Testbed
	st := tb.State()
	v6 := st.Network.Family == "ipv6"

	prefix := st.Network.Subnet
	if v6 {
		prefix = st.Network.Subnet6
	}
	plen := 0
	if p, err := netip.ParsePrefix(prefix); err == nil {
		plen = p.Bits()
	}

	machines := []map[string]any{}
	for _, m := range st.Machines {
		labels := m.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		machines = append(machines, map[string]any{
			"name": m.Name, "role": m.Role, "mac": m.MAC, "ip": m.IP, "gateway": m.Gateway,
			"arch": m.Arch, "bmcPort": m.BMCPort, "labels": labels,
		})
	}

	key, err := sshKey(tb)
	if err != nil {
		return nil, err
	}

	hosts := map[string]string{}
	rewrite := func(ref string) string { return mirrored(ref, st.Registry.Mirrors) }
	rt := map[string]any{
		"vips":           map[string]any{"tinkerbell": st.VIPs.Tinkerbell, "hookos": st.VIPs.HookOS, "controlPlane": st.VIPs.ControlPlane},
		"prefixLen":      plen,
		"podCIDR":        "10.244.0.0/16",
		"serviceCIDR":    "10.96.0.0/12",
		"nameServers":    []string{"8.8.8.8", "1.1.1.1"},
		"trustedProxies": []string{},
		"machines":       machines,
		"bmc":            map[string]any{"host": st.BMC.Host, "user": bmc.User, "pass": bmc.Pass},
		"sshKey":         key,
		"agentImage":     rewrite(agentImage),
		"images": map[string]any{
			"oci2disk": rewrite(imageOCI2Disk), "writefile": rewrite(imageWritefile),
			"kexec": rewrite(imageKexec), "waitdaemon": rewrite(imageWaitdaemon),
		},
		"osRegistry":    rewrite(osRegistry),
		"registryHosts": hosts,
		"bridge":        st.Network.Bridge,
		"nat64Prefix":   network.NAT64Prefix,
	}
	if !v6 {
		rt["netmask"] = network.Netmask(prefix)
	} else {
		rt["subnet6"] = st.Network.Subnet6
		rt["podCIDR"] = "fd00:10:244::/56"
		rt["serviceCIDR"] = "fd00:10:96::/112"
		if st.Network.DNS64 != "" {
			rt["nameServers"] = []string{st.Network.DNS64}
			rt["dns64IP"] = st.Network.DNS64
		} else {
			rt["nameServers"] = []string{"2606:4700:4700::1111", "2001:4860:4860::8888"}
		}
	}

	insecure := []string{}
	for up, addr := range st.Registry.Mirrors {
		hosts[up] = mirrorHostsTOML(up, addr)
		insecure = append(insecure, addr)
	}
	if st.Registry.Pull != "" {
		hosts[st.Registry.Pull] = registry.HostsTOML(st.Registry.Pull)
		insecure = append(insecure, st.Registry.Pull)
	}
	slices.Sort(insecure)
	rt["insecureRegistries"] = insecure
	if len(hosts) > 0 {
		rt["certsDir"] = filepath.Join(tb.Dir, "certs.d")
	}

	if src := tb.Config.Tinkerbell.Source; src != nil {
		out, err := tb.Builds.Get("tinkerbell", *src)
		if err != nil {
			return nil, err
		}
		if img, _ := out["image"].(string); img != "" {
			rt["tinkerbellImage"] = pullRef(img, st.Registry.Pull)
		}
		if img, _ := out["agentImage"].(string); img != "" {
			rt["agentImage"] = pullRef(img, st.Registry.Pull)
		}
	}
	capt, err := captTarget(tb, tb.Config.ClusterAPI.CAPT, st.Registry.Pull)
	if err != nil {
		return nil, err
	}
	rt["capt"] = capt

	if c, err := kube(s, engine.TinkerbellCluster); err == nil {
		nodes := &corev1.NodeList{}
		if err := c.List(ctx, nodes); err == nil {
			var cidrs []string
			for _, n := range nodes.Items {
				cidrs = append(cidrs, n.Spec.PodCIDRs...)
			}
			if len(cidrs) > 0 {
				rt["trustedProxies"] = cidrs
			}
		}
	}
	return rt, nil
}

// captTarget is #Runtime.capt for a CAPT release or build.
func captTarget(tb *engine.Testbed, c config.CAPT, pull string) (map[string]any, error) {
	if c.Source == nil {
		return map[string]any{"version": c.Version, "repository": c.Repository}, nil
	}
	out, err := tb.Builds.Get("capt", *c.Source)
	if err != nil {
		return nil, err
	}
	version, _ := out["version"].(string)
	repo, _ := out["repository"].(string)
	t := map[string]any{"version": version, "repository": repo}
	if img, _ := out["image"].(string); img != "" {
		// clusterctl replaces everything before the image's own name.
		ref := pullRef(img, pull)
		if i := strings.LastIndex(ref, "/"); i > 0 {
			t["imageRepository"] = ref[:i]
		}
	}
	return t, nil
}

// pullRef swaps the push endpoint of a built image for the address the
// testbed pulls from.
func pullRef(ref, pull string) string {
	if pull == "" {
		return ref
	}
	return strings.Replace(ref, registry.PushEndpoint, pull, 1)
}

// mirrored points an image at its upstream's pull-through cache. Only
// images pulled by something without containerd's certs.d need this.
func mirrored(ref string, mirrors map[string]string) string {
	for up, addr := range mirrors {
		if strings.HasPrefix(ref, up+"/") {
			return addr + strings.TrimPrefix(ref, up)
		}
	}
	return ref
}

// mirrorHostsTOML points containerd at a pull-through cache for an upstream,
// falling back to the upstream itself.
func mirrorHostsTOML(upstream, addr string) string {
	server := "https://" + upstream
	if upstream == "docker.io" {
		// docker.io itself answers the v2 API with a redirect to a web page.
		server = "https://registry-1.docker.io"
	}
	return fmt.Sprintf("server = %q\n\n[host.\"http://%s\"]\n  capabilities = [\"pull\", \"resolve\"]\n", server, addr)
}

// writeCertsDir writes the hosts.toml files kind nodes mount.
func writeCertsDir(rt map[string]any) error {
	dir, ok := rt["certsDir"].(string)
	if !ok {
		return nil
	}
	hosts, _ := rt["registryHosts"].(map[string]string)
	for host, body := range hosts {
		d := filepath.Join(dir, strings.NewReplacer("[", "", "]", "").Replace(host))
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(d, "hosts.toml"), []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// sshKey is the testbed's public key for the machines' tink user, generated
// on first use. The private key stays in the testbed's directory for
// `ssh -i`.
func sshKey(tb *engine.Testbed) (string, error) {
	dir := filepath.Join(tb.Dir, "ssh")
	pubPath := filepath.Join(dir, "id_ed25519.pub")
	if b, err := os.ReadFile(pubPath); err == nil {
		return strings.TrimSpace(string(b)), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		return "", err
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "id_ed25519"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sp))) + " wendy"
	return line, os.WriteFile(pubPath, []byte(line+"\n"), 0o644)
}
