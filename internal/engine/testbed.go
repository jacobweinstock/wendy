package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/tinkerbell/wendy/internal/config"
)

// Cluster names used for kubeconfigs and state.
const (
	ManagementCluster = "management"
	TinkerbellCluster = "tinkerbell"
	WorkloadCluster   = "workload"
)

// Testbed is one combination's testbed as it runs or is kept: its resolved
// config, its state on disk and the stack that tears it down.
type Testbed struct {
	Execution string
	// Name is the combination's name.
	Name string
	// ID is short and unique on the host; host-global names derive from it.
	ID       string
	Dir      string
	Config   config.Testbed
	Labels   []string
	Renderer *config.Renderer
	Builds   *Builds

	mu      sync.Mutex
	state   State
	cleanup []cleanupEntry
}

// State is what state.json holds: everything BYO tests and later commands
// need to find this testbed. It never holds BMC credentials.
type State struct {
	Execution       string               `json:"execution"`
	Combination     string               `json:"combination"`
	ID              string               `json:"id"`
	Arch            string               `json:"arch"`
	Distro          string               `json:"distro"`
	Labels          []string             `json:"labels"`
	Namespace       string               `json:"namespace"`
	Clusters        map[string]string    `json:"clusters"`
	Network         Network              `json:"network"`
	VIPs            VIPs                 `json:"vips"`
	Machines        []Machine            `json:"machines"`
	BMC             BMC                  `json:"bmc"`
	Registry        Registry             `json:"registry"`
	Versions        map[string]string    `json:"versions"`
	WorkloadCluster WorkloadClusterState `json:"workloadCluster"`
	Outputs         map[string]any       `json:"outputs"`
	Resources       []Resource           `json:"resources"`
}

type Network struct {
	Name     string `json:"name"`
	Bridge   string `json:"bridge"`
	Family   string `json:"family"`
	Subnet   string `json:"subnet"`
	Gateway  string `json:"gateway"`
	Subnet6  string `json:"subnet6,omitempty"`
	Gateway6 string `json:"gateway6,omitempty"`
	DNS64    string `json:"dns64,omitempty"`
}

type VIPs struct {
	Tinkerbell   string `json:"tinkerbell"`
	HookOS       string `json:"hookos"`
	ControlPlane string `json:"controlPlane"`
}

type Machine struct {
	Name    string            `json:"name"`
	Role    string            `json:"role"`
	MAC     string            `json:"mac"`
	IP      string            `json:"ip"`
	Gateway string            `json:"gateway"`
	Arch    string            `json:"arch"`
	BMCPort int               `json:"bmcPort"`
	Labels  map[string]string `json:"labels"`
	Console string            `json:"console,omitempty"`
}

type BMC struct {
	Host      string `json:"host"`
	Container string `json:"container"`
	Protocol  string `json:"protocol"`
}

type Registry struct {
	Pull string `json:"pull,omitempty"`
	Push string `json:"push,omitempty"`
	// Mirrors maps an upstream registry to its pull-through cache's address.
	Mirrors map[string]string `json:"mirrors,omitempty"`
	// MirrorCA is the PEM certificate the mirrors' certificates chain to.
	MirrorCA string `json:"mirrorCA,omitempty"`
}

type WorkloadClusterState struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	KubernetesVersion string `json:"kubernetesVersion"`
}

// Resource is something on the host the testbed created. Resources are
// destroyed in reverse order of creation, by the teardown registered for
// their kind, whether in this process or a later one that tears down a kept
// testbed.
type Resource struct {
	Kind  string            `json:"kind"`
	Name  string            `json:"name"`
	Attrs map[string]string `json:"attrs,omitempty"`
}

type cleanupEntry struct {
	name     string
	fn       func(context.Context) error
	resource *Resource
}

var (
	teardownMu sync.RWMutex
	teardowns  = map[string]func(ctx context.Context, tb *Testbed, r Resource) error{}
)

// RegisterTeardown sets how resources of a kind are destroyed.
func RegisterTeardown(kind string, fn func(ctx context.Context, tb *Testbed, r Resource) error) {
	teardownMu.Lock()
	defer teardownMu.Unlock()
	teardowns[kind] = fn
}

// NewTestbed creates a combination's testbed in dir, writing its resolved
// config to testbed.json and its first state.json.
func NewTestbed(execution, name, id, dir string, cfg config.Testbed, labels []string, r *config.Renderer, b *Builds) (*Testbed, error) {
	if err := os.MkdirAll(filepath.Join(dir, "kubeconfigs"), 0o755); err != nil {
		return nil, err
	}
	tb := &Testbed{Execution: execution, Name: name, ID: id, Dir: dir, Config: cfg, Labels: labels, Renderer: r, Builds: b}
	tb.state = State{
		Execution: execution, Combination: name, ID: id, Arch: cfg.Arch, Distro: cfg.ManagementCluster.Type, Labels: labels,
		Namespace: cfg.Tinkerbell.Namespace, Clusters: map[string]string{}, Versions: map[string]string{},
		Outputs: map[string]any{},
		WorkloadCluster: WorkloadClusterState{
			Name: cfg.WorkloadCluster.Name, Namespace: cfg.Tinkerbell.Namespace, KubernetesVersion: cfg.WorkloadCluster.KubernetesVersion,
		},
	}
	b2, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "testbed.json"), b2, 0o644); err != nil {
		return nil, err
	}
	return tb, tb.Save()
}

// LoadTestbed reopens a testbed from its directory, for teardown and for
// running a single step against a kept testbed.
func LoadTestbed(dir string, r *config.Renderer) (*Testbed, error) {
	var cfg config.Testbed
	b, err := os.ReadFile(filepath.Join(dir, "testbed.json"))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	var st State
	b, err = os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	if st.Outputs == nil {
		st.Outputs = map[string]any{}
	}
	if st.Clusters == nil {
		st.Clusters = map[string]string{}
	}
	if st.Versions == nil {
		st.Versions = map[string]string{}
	}
	return &Testbed{Execution: st.Execution, Name: st.Combination, ID: st.ID, Dir: dir, Config: cfg, Labels: st.Labels, Renderer: r, state: st}, nil
}

// State returns a copy of the testbed's state.
func (tb *Testbed) State() State {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	b, _ := json.Marshal(tb.state)
	var out State
	_ = json.Unmarshal(b, &out)
	return out
}

// Update changes the state and saves it.
func (tb *Testbed) Update(fn func(*State)) error {
	tb.mu.Lock()
	fn(&tb.state)
	tb.mu.Unlock()
	return tb.Save()
}

// Save writes state.json atomically.
func (tb *Testbed) Save() error {
	tb.mu.Lock()
	b, err := json.MarshalIndent(tb.state, "", "  ")
	tb.mu.Unlock()
	if err != nil {
		return err
	}
	tmp := filepath.Join(tb.Dir, ".state.json.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(tb.Dir, "state.json"))
}

// Kubeconfig is the path of a cluster's kubeconfig. The Tinkerbell cluster is
// the management cluster unless the topology is external.
func (tb *Testbed) Kubeconfig(cluster string) string {
	if cluster == TinkerbellCluster && tb.Config.Tinkerbell.Topology != "external" {
		cluster = ManagementCluster
	}
	return filepath.Join(tb.Dir, "kubeconfigs", cluster)
}

// SetKubeconfig writes a cluster's kubeconfig into the bundle.
func (tb *Testbed) SetKubeconfig(cluster string, data []byte) error {
	path := filepath.Join(tb.Dir, "kubeconfigs", cluster)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	if cluster == ManagementCluster && tb.Config.Tinkerbell.Topology != "external" {
		if err := os.WriteFile(filepath.Join(tb.Dir, "kubeconfigs", TinkerbellCluster), data, 0o600); err != nil {
			return err
		}
	}
	return tb.Update(func(s *State) { s.Clusters[cluster] = path })
}

// Render evaluates a field of #Render for this testbed.
func (tb *Testbed) Render(rt any, field string, out any, fill map[string]any) error {
	return tb.Renderer.Render(tb.Config, rt, field, out, fill)
}

// Track records a resource and pushes its teardown onto the cleanup stack.
// A resource already tracked (by kind and name) is not added twice.
func (tb *Testbed) Track(r Resource) error {
	tb.mu.Lock()
	for _, x := range tb.state.Resources {
		if x.Kind == r.Kind && x.Name == r.Name {
			tb.mu.Unlock()
			return nil
		}
	}
	tb.state.Resources = append(tb.state.Resources, r)
	tb.cleanup = append(tb.cleanup, cleanupEntry{name: r.Kind + " " + r.Name, resource: &r})
	tb.mu.Unlock()
	return tb.Save()
}

// Defer pushes an in-process cleanup onto the stack.
func (tb *Testbed) Defer(name string, fn func(context.Context) error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.cleanup = append(tb.cleanup, cleanupEntry{name: name, fn: fn})
}

// TeardownStep is one entry of a teardown, for reporting.
type TeardownStep struct {
	Name string
	Run  func(context.Context) error
}

// Teardown is the cleanup stack in the order it runs: last in, first out.
// When this process created nothing (a kept testbed torn down later), it is
// the persisted resources, newest first.
func (tb *Testbed) Teardown() []TeardownStep {
	tb.mu.Lock()
	entries := tb.cleanup
	if len(entries) == 0 {
		for i := range tb.state.Resources {
			r := tb.state.Resources[i]
			entries = append(entries, cleanupEntry{name: r.Kind + " " + r.Name, resource: &r})
		}
	}
	tb.mu.Unlock()

	var out []TeardownStep
	for i := len(entries) - 1; i >= 0; i-- {
		ent := entries[i]
		run := ent.fn
		if ent.resource != nil {
			r := *ent.resource
			run = func(ctx context.Context) error {
				teardownMu.RLock()
				fn, ok := teardowns[r.Kind]
				teardownMu.RUnlock()
				if !ok {
					return fmt.Errorf("no teardown for %s", r.Kind)
				}
				if err := fn(ctx, tb, r); err != nil {
					return err
				}
				return tb.forget(r)
			}
		}
		out = append(out, TeardownStep{Name: ent.name, Run: run})
	}
	return out
}

func (tb *Testbed) forget(r Resource) error {
	return tb.Update(func(s *State) {
		for i, x := range s.Resources {
			if x.Kind == r.Kind && x.Name == r.Name {
				s.Resources = append(s.Resources[:i], s.Resources[i+1:]...)
				return
			}
		}
	})
}

// HostName derives a host-unique name for something this testbed owns.
func (tb *Testbed) HostName(suffix string) string {
	if suffix == "" {
		return "wendy-" + tb.ID
	}
	return "wendy-" + tb.ID + "-" + suffix
}

// DataDir is wendy's data directory: a testbed lives at
// <data>/executions/<execution>/<combination>.
func (tb *Testbed) DataDir() string {
	return filepath.Dir(filepath.Dir(filepath.Dir(tb.Dir)))
}
