package config

import (
	"path"
	"strings"
)

// Testbed is a concrete #ResolvedTestbed: a testbed's parts assembled, with
// every default applied.
type Testbed struct {
	Arch              string          `json:"arch"`
	Network           Network         `json:"network"`
	ManagementCluster Distro          `json:"managementCluster"`
	Tinkerbell        Tinkerbell      `json:"tinkerbell"`
	ClusterAPI        ClusterAPI      `json:"clusterAPI"`
	Hardware          Hardware        `json:"hardware"`
	WorkloadCluster   WorkloadCluster `json:"workloadCluster"`
	Diagnostics       Diagnostics     `json:"diagnostics"`
	Setup             []Step          `json:"setup"`
}

type Network struct {
	Family string `json:"family"`
	NAT64  bool   `json:"nat64"`
	Mirror Mirror `json:"mirror"`
}

type Mirror struct {
	Enabled   bool     `json:"enabled"`
	Upstreams []string `json:"upstreams"`
}

type Distro struct {
	Type  string `json:"type"`
	Image string `json:"image,omitempty"`
}

// Source is a git repository or local checkout to build from.
type Source struct {
	Repo string `json:"repo,omitempty"`
	Ref  string `json:"ref,omitempty"`
}

type Chart struct {
	Location string `json:"location"`
	Version  string `json:"version"`
}

type Tinkerbell struct {
	Topology  string         `json:"topology"`
	Cluster   *Distro        `json:"cluster,omitempty"`
	Chart     *Chart         `json:"chart,omitempty"`
	Source    *Source        `json:"source,omitempty"`
	Values    map[string]any `json:"values"`
	BootMode  string         `json:"bootMode"`
	Namespace string         `json:"namespace"`
	CaptainOS CaptainOS      `json:"captainos"`
}

type CaptainOS struct {
	Flavor string `json:"flavor"`
	Image  string `json:"image"`
}

type ClusterAPI struct {
	Core string `json:"core"`
	CAPT CAPT   `json:"capt"`
}

// CAPT is a release (Version, Repository) or a build (Source).
type CAPT struct {
	Version    string  `json:"version,omitempty"`
	Repository string  `json:"repository,omitempty"`
	Source     *Source `json:"source,omitempty"`
}

type Hardware struct {
	Backend  Backend               `json:"backend"`
	BMC      BMC                   `json:"bmc"`
	Machines map[string]MachineSet `json:"machines"`
}

type Backend struct {
	Type     string    `json:"type"`
	URI      string    `json:"uri"`
	DiskPath string    `json:"diskPath"`
	Firmware *Firmware `json:"firmware,omitempty"`
}

type Firmware struct {
	Code string `json:"code"`
	Vars string `json:"vars"`
}

type BMC struct {
	Image    string `json:"image"`
	Protocol string `json:"protocol"`
}

type MachineSet struct {
	Count   int               `json:"count"`
	CPUs    int               `json:"cpus"`
	MemMiB  int               `json:"memMiB"`
	DiskGiB int               `json:"diskGiB"`
	Labels  map[string]string `json:"labels"`
}

type WorkloadCluster struct {
	Name              string `json:"name"`
	KubernetesVersion string `json:"kubernetesVersion"`
	OSVersion         string `json:"osVersion"`
	CNI               string `json:"cni"`
	KubeVipVersion    string `json:"kubeVipVersion"`
}

// Diagnostics is what an execution collects about a failure: when to collect
// the clusters' state, Tinkerbell's objects and the BMC logs (onFailure,
// always or never), and whether to capture the machines' consoles.
type Diagnostics struct {
	Collect  string `json:"collect"`
	Consoles bool   `json:"consoles"`
}

// DefaultDiagnostics are the diagnostics of an execution that has no plan.
// They match the schema's defaults.
func DefaultDiagnostics() Diagnostics { return Diagnostics{Collect: "onFailure", Consoles: true} }

// TinkerbellTarget is a chart release or a build to install or upgrade to.
type TinkerbellTarget struct {
	Chart  *Chart  `json:"chart,omitempty"`
	Source *Source `json:"source,omitempty"`
}

// Step is a concrete #Step. Presets have already expanded to Exec or
// Container by the time CUE hands it over.
type Step struct {
	Name              string         `json:"name,omitempty"`
	Labels            []string       `json:"labels"`
	Timeout           string         `json:"timeout"`
	Retries           int            `json:"retries"`
	When              string         `json:"when,omitempty"`
	ContinueOnFailure bool           `json:"continueOnFailure"`
	Uses              string         `json:"uses,omitempty"`
	With              map[string]any `json:"with,omitempty"`
	Exec              *Exec          `json:"exec,omitempty"`
	Container         *Container     `json:"container,omitempty"`
}

type Exec struct {
	Command []string          `json:"command"`
	Env     map[string]string `json:"env"`
}

type Container struct {
	Image   string            `json:"image"`
	Command []string          `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env"`
}

// DisplayName is how a step is shown: its name, else what it runs.
func (s Step) DisplayName() string {
	switch {
	case s.Name != "":
		return s.Name
	case s.Uses != "":
		return s.Uses
	case s.Exec != nil && len(s.Exec.Command) > 0:
		return path.Base(s.Exec.Command[0])
	case s.Container != nil:
		img := s.Container.Image
		if i := strings.LastIndex(img, "/"); i >= 0 {
			img = img[i+1:]
		}
		return img
	}
	return "step"
}

// Kind reports which runner executes the step: "uses", "exec" or "container".
func (s Step) Kind() string {
	switch {
	case s.Uses != "":
		return "uses"
	case s.Exec != nil:
		return "exec"
	case s.Container != nil:
		return "container"
	}
	return ""
}

type Test struct {
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
	Requires    struct {
		Spare int `json:"spare"`
	} `json:"requires"`
	Once  bool   `json:"once"`
	Steps []Step `json:"steps"`
}

type Suite struct {
	Description string   `json:"description"`
	Tests       []string `json:"tests"`
}

// Plan says what to test, where and how: the suites, the testbed, and how to
// execute them.
type Plan struct {
	Description string `json:"description,omitempty"`
	Testbed     string `json:"testbed"`
	// Combinations are glob patterns over the testbed's combination names;
	// empty means all.
	Combinations []string `json:"combinations"`
	Suites       []string `json:"suites"`
	LabelFilter  string   `json:"labelFilter"`
	Parallel     int      `json:"parallel"`
	// Keep says when the testbed stays up after an execution: never, failed
	// or always.
	Keep           string `json:"keep"`
	PauseOnFailure bool   `json:"pauseOnFailure"`
	// Diagnostics says when to collect them: onFailure, always or never.
	Diagnostics string `json:"diagnostics"`
	Consoles    bool   `json:"consoles"`
}

// Collects is what the plan's executions collect about a failure.
func (p Plan) Collects() Diagnostics {
	return Diagnostics{Collect: p.Diagnostics, Consoles: p.Consoles}
}

// Matrix is a testbed's matrix: its axes, each a set of named values that
// override the testbed's fields, and the combinations it leaves out.
type Matrix struct {
	Order   []string                  `json:"order"`
	Axes    map[string]map[string]any `json:"axes"`
	Exclude []map[string]string       `json:"exclude"`
	// Values are each axis's value names in the order the config lists them.
	Values map[string][]string `json:"-"`
	// AxisPos is where each axis value is written, by axis then value, for
	// errors about it.
	AxisPos map[string]map[string]string `json:"-"`
}

// Excluded reports why a combination with these axis values is excluded, if
// it is.
func (m Matrix) Excluded(values map[string]string) (string, bool) {
	for _, ex := range m.Exclude {
		match := true
		for axis, want := range ex {
			if axis == "reason" {
				continue
			}
			if values[axis] != want {
				match = false
				break
			}
		}
		if match {
			return ex["reason"], true
		}
	}
	return "", false
}

// MachineCount is how many machines the testbed builds.
func (e Testbed) MachineCount() int {
	n := 0
	for _, m := range e.Hardware.Machines {
		n += m.Count
	}
	return n
}

// UsesSource reports whether Tinkerbell or CAPT is built rather than released.
func (e Testbed) UsesSource() bool {
	return e.Tinkerbell.Source != nil || e.ClusterAPI.CAPT.Source != nil
}

// archKey is the name of the architecture, which is both a field of a
// resolved testbed and one of its axes.
const archKey = "arch"

// IntrinsicAxes are the axes every testbed has a value on, whether or not it
// has a matrix, so labelled steps behave the same either way.
var IntrinsicAxes = map[string][]string{
	"topology": {"colocated", "external"},
	"ipFamily": {"ipv4", "ipv6"},
	"bootMode": {"netboot", "isoboot"},
	"registry": {"direct", "mirror"},
	archKey:    {"amd64", "arm64"},
}

// AxisValues are the testbed's values on IntrinsicAxes.
func (e Testbed) AxisValues() map[string]string {
	registry := "direct"
	if e.Network.Mirror.Enabled {
		registry = "mirror"
	}
	return map[string]string{
		"topology": e.Tinkerbell.Topology,
		"ipFamily": e.Network.Family,
		"bootMode": e.Tinkerbell.BootMode,
		"registry": registry,
		archKey:    e.Arch,
	}
}
