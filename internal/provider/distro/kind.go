// Package distro provides management and Tinkerbell clusters. v1 has kind.
package distro

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kind/pkg/cluster"
	kindlog "sigs.k8s.io/kind/pkg/log"
	"sigs.k8s.io/yaml"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/provider/docker"
)

// Distro creates and destroys a cluster.
type Distro interface {
	// Up creates the cluster unless it exists, and returns a kubeconfig that
	// works from the host and from containers on the environment's network.
	Up(ctx context.Context, tb *engine.Testbed, name string, d config.Distro, rt map[string]any, log func(string)) ([]byte, error)
	Down(ctx context.Context, name string) error
}

// For returns the provider for a distro type.
func For(d config.Distro) (Distro, error) {
	if d.Type == "kind" {
		return Kind{}, nil
	}
	return nil, fmt.Errorf("unsupported distro %q", d.Type)
}

// Kind provides clusters with kind.
type Kind struct{}

// kindMu serialises cluster creation: kind reads the network to use from a
// process-wide environment variable.
var kindMu sync.Mutex

func (Kind) Up(ctx context.Context, tb *engine.Testbed, name string, d config.Distro, rt map[string]any, log func(string)) ([]byte, error) {
	net := tb.State().Network
	p := cluster.NewProvider(cluster.ProviderWithDocker(), cluster.ProviderWithLogger(logger{log: log}))
	existing, err := p.List()
	if err != nil {
		return nil, err
	}
	if !slices.Contains(existing, name) {
		var cfg map[string]any
		if err := tb.Render(rt, "kind", &cfg, nil); err != nil {
			return nil, err
		}
		raw, err := yaml.Marshal(cfg)
		if err != nil {
			return nil, err
		}
		opts := []cluster.CreateOption{
			cluster.CreateWithRawConfig(raw),
			cluster.CreateWithWaitForReady(5 * time.Minute),
			cluster.CreateWithKubeconfigPath(scratchKubeconfig(name)),
			cluster.CreateWithDisplayUsage(false),
			cluster.CreateWithDisplaySalutation(false),
		}
		if d.Image != "" {
			opts = append(opts, cluster.CreateWithNodeImage(d.Image))
		}
		log("creating kind cluster " + name + " on " + net.Name)
		done := make(chan error, 1)
		go func() {
			kindMu.Lock()
			defer kindMu.Unlock()
			os.Setenv("KIND_EXPERIMENTAL_DOCKER_NETWORK", net.Name)
			defer os.Unsetenv("KIND_EXPERIMENTAL_DOCKER_NETWORK")
			done <- p.Create(name, opts...)
		}()
		select {
		case err = <-done:
		case <-ctx.Done():
			// kind has no context; let it finish and let teardown remove it.
			return nil, ctx.Err()
		}
		if err != nil {
			return nil, fmt.Errorf("creating kind cluster %s: %w", name, err)
		}
		_ = os.Remove(scratchKubeconfig(name))
	}
	return Kubeconfig(ctx, name, net)
}

// Kubeconfig is a kind cluster's kubeconfig with its server set to the
// control-plane node's address on the network. kind's own points at a port
// published on the host's loopback, which no container can reach.
func Kubeconfig(ctx context.Context, name string, net engine.Network) ([]byte, error) {
	p := cluster.NewProvider(cluster.ProviderWithDocker())
	raw, err := p.KubeConfig(name, false)
	if err != nil {
		return nil, err
	}
	ip, err := docker.IP(ctx, name+"-control-plane", net.Name, net.Family)
	if err != nil {
		return nil, err
	}
	server := "https://" + ip + ":6443"
	if net.Family == "ipv6" {
		server = "https://[" + ip + "]:6443"
	}
	kc, err := clientcmd.Load([]byte(raw))
	if err != nil {
		return nil, err
	}
	for _, c := range kc.Clusters {
		c.Server = server
	}
	return clientcmd.Write(*kc)
}

func (Kind) Down(_ context.Context, name string) error {
	path := scratchKubeconfig(name)
	defer os.Remove(path)
	return cluster.NewProvider(cluster.ProviderWithDocker()).Delete(name, path)
}

// scratchKubeconfig is where kind writes the kubeconfig it insists on
// writing. wendy's own comes from Kubeconfig, so this is thrown away; it is
// not $KUBECONFIG, which kind would otherwise edit.
func scratchKubeconfig(name string) string {
	return filepath.Join(os.TempDir(), "wendy-kind-"+name+".kubeconfig")
}

func init() {
	engine.RegisterTeardown("kind", func(ctx context.Context, _ *engine.Testbed, r engine.Resource) error {
		return Kind{}.Down(ctx, r.Name)
	})
}

type logger struct{ log func(string) }

func (l logger) out(m string) {
	if m = strings.TrimSpace(m); m != "" {
		l.log(m)
	}
}

func (l logger) Warn(m string)                        { l.out(m) }
func (l logger) Warnf(f string, a ...interface{})     { l.out(fmt.Sprintf(f, a...)) }
func (l logger) Error(m string)                       { l.out(m) }
func (l logger) Errorf(f string, a ...interface{})    { l.out(fmt.Sprintf(f, a...)) }
func (l logger) V(v kindlog.Level) kindlog.InfoLogger { return info{l: l, on: v <= 0} }

type info struct {
	l  logger
	on bool
}

func (i info) Info(m string) {
	if i.on {
		i.l.out(m)
	}
}

func (i info) Infof(f string, a ...interface{}) {
	if i.on {
		i.l.out(fmt.Sprintf(f, a...))
	}
}

func (i info) Enabled() bool { return i.on }
