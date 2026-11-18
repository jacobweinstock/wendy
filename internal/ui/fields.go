package ui

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/vocab"
)

// FieldKind says how a field is typed and parsed.
type FieldKind int

// The field kinds.
const (
	FText   FieldKind = iota
	FInt              // a whole number
	FBool             // unset, true or false
	FSelect           // one of Options, or unset
	FLines            // a list, one item per line, order kept
	FWords            // a list of words separated by spaces or commas
	FMap              // key=value per line
	FYAML             // free-form data
)

// Field is one input of an editor form. The TUI and the web UI both build
// their forms from these, so they offer the same settings.
type Field struct {
	// Path is where the value sits in the entry, dotted.
	Path  string
	Label string
	Kind  FieldKind
	// Options are the choices of a FSelect.
	Options []string
	// Default is what applies when the field is unset.
	Default  string
	Help     string
	Required bool
	// RequiredUnless names a path that, when set, makes the field optional.
	RequiredUnless string
	// Advanced marks a setting a typical entry leaves alone, so a form can
	// fold it away. A Required field is never Advanced.
	Advanced bool
	// Only limits a step field to these step kinds.
	Only []string
}

// MachineRoles are the machine sets every hardware part has.
var MachineRoles = []struct{ Key, Label string }{
	{"controlPlane", "Control plane"},
	{"worker", "Worker"},
	{"spare", "Spare"},
}

// MachineColumns are the settings of each machine set.
var MachineColumns = []struct{ Key, Label, Default string }{
	{"count", "Machines", "1"},
	{"cpus", "CPUs", "2"},
	{"memMiB", "Memory MiB", "2048"},
	{"diskGiB", "Disk GiB", "10"},
}

// IsMachineField reports whether f is a setting of a machine set.
func IsMachineField(f Field) bool { return strings.HasPrefix(f.Path, machinePrefix) }

const machinePrefix = "machines."

// MachineField is the field for one setting of one machine set.
func MachineField(role, column string) Field {
	for _, f := range HardwareFields {
		if f.Path == machinePrefix+role+"."+column {
			return f
		}
	}
	return Field{}
}

// TestbedFields are the settings a testbed keeps for itself, beside the parts
// it names: those that more than one part reads, so none owns them. Paths are
// relative to the testbed.
var TestbedFields = []Field{
	{Path: "description", Label: "Description"},
	{Path: "topology", Label: "Where Tinkerbell runs", Kind: FSelect, Options: []string{"colocated", "external"}, Default: "colocated"},
	{Path: "bootMode", Label: "How machines boot", Kind: FSelect, Options: []string{"netboot", "isoboot"}, Default: "netboot"},
	{Path: "network.family", Label: "IP version", Kind: FSelect, Options: []string{"ipv4", "ipv6"}, Default: "ipv4"},
	{Path: "network.nat64", Label: "NAT64", Kind: FBool, Advanced: true, Default: "on with IPv6", Help: "lets IPv6 machines reach registries that are IPv4 only"},
	{Path: "network.mirror.enabled", Label: "Registry mirror", Kind: FBool, Advanced: true, Default: "false", Help: "machines pull container images through a registry mirror that Wendy runs"},
	{Path: "network.mirror.upstreams", Label: "Mirror upstreams", Kind: FLines, Advanced: true, Default: "ghcr.io, quay.io, registry.k8s.io, gcr.io, docker.io", Help: "the registries the mirror serves, one per line"},
}

// PlanDiagnosticsFields are the diagnostics settings of an execution: when to
// collect diagnostics, and whether to capture the machines' consoles. A plan
// holds them, and Run tests can override them for one execution. Paths are
// relative to the plan.
var PlanDiagnosticsFields = []Field{
	{Path: "diagnostics", Label: "Collect diagnostics", Kind: FSelect, Options: []string{"onFailure", "always", "never"}, Default: "onFailure"},
	{Path: "consoles", Label: "Capture consoles", Kind: FBool, Default: "true"},
}

// TinkerbellFields are the settings of a Tinkerbell part: how Tinkerbell is
// installed. Paths are relative to the part.
var TinkerbellFields = []Field{
	{Path: "namespace", Label: "Namespace", Advanced: true, Default: "tinkerbell"},
	{Path: "chart.location", Label: "Chart", Advanced: true, Default: "oci://ghcr.io/tinkerbell/charts/tinkerbell"},
	{Path: "chart.version", Label: "Chart version", Default: "latest"},
	{Path: "source.repo", Label: "Build from repo", Advanced: true, Help: "a git URL or local path; building wins over the chart version"},
	{Path: "source.ref", Label: "Build from ref", Advanced: true, Help: "branch, tag or commit; empty with a local path builds the working tree"},
	{Path: "values", Label: "Chart values", Kind: FYAML, Advanced: true, Help: "YAML applied over wendy's own values"},
	{Path: "captainos.flavor", Label: "CaptainOS flavor", Advanced: true, Default: "trixie-slim"},
	{Path: "captainos.image", Label: "CaptainOS image", Advanced: true, Default: "ghcr.io/tinkerbell/captain/artifacts:v0.1.0-slim"},
}

// ClusterAPIFields are the settings of a Cluster API part: the Cluster API and
// CAPT releases, or CAPT built from source.
var ClusterAPIFields = []Field{
	{Path: "core", Label: "Cluster API version", Required: true, Default: "v1.14.4", Help: "e.g. v1.14.4"},
	{Path: "capt.version", Label: "CAPT version", RequiredUnless: "capt.source", Default: "v0.7.0", Help: "a release, e.g. v0.7.0"},
	{Path: "capt.repository", Label: "CAPT repository", Advanced: true, Default: "https://github.com/tinkerbell/cluster-api-provider-tinkerbell/releases", Help: "a releases URL or a directory"},
	{Path: "capt.source.repo", Label: "Build CAPT from repo", Advanced: true, Help: "a git URL or local path; building wins over the version"},
	{Path: "capt.source.ref", Label: "Build CAPT from ref", Advanced: true, Help: "branch, tag or commit; empty with a local path builds the working tree"},
}

// ManagementClusterFields are the settings of a Management cluster part: the
// cluster Cluster API is installed on. kind is its only type, so only the
// node image varies.
var ManagementClusterFields = []Field{
	{Path: "image", Label: "Node image", Help: "the kind node image; empty uses kind's own"},
}

// HardwareFields are the settings of a Hardware part: the machines and what
// runs them.
var HardwareFields = hardwareFields()

func hardwareFields() []Field {
	f := []Field{
		{Path: "arch", Label: "Architecture", Kind: FSelect, Options: []string{"amd64", "arm64"}, Default: "amd64"},
		{Path: "backend.uri", Label: "libvirt URI", Advanced: true, Default: "qemu:///system"},
		{Path: "backend.diskPath", Label: "Disk directory", Advanced: true, Default: "/tmp", Help: "where the VMs' disk images are created"},
		{Path: "bmc.image", Label: "Virtual BMC image", Advanced: true, Default: "ghcr.io/jacobweinstock/virtualbmc:latest"},
		{Path: "bmc.protocol", Label: "BMC protocol", Kind: FSelect, Advanced: true, Options: []string{"redfish", "ipmi"}, Default: "redfish"},
	}
	for _, r := range MachineRoles {
		for _, c := range MachineColumns {
			f = append(f, Field{
				Path: machinePrefix + r.Key + "." + c.Key, Label: r.Label + " " + strings.ToLower(c.Label),
				Kind: FInt, Default: c.Default,
			})
		}
	}
	return f
}

// WorkloadClusterFields are the settings of a Workload cluster part: the
// cluster to deploy.
var WorkloadClusterFields = []Field{
	{Path: "name", Label: "Workload cluster name", Advanced: true, Default: "wendy"},
	{Path: "kubernetesVersion", Label: "Kubernetes version", Required: true, Default: "v1.36.4", Help: "e.g. v1.36.4"},
	{Path: "osVersion", Label: "OS version", Advanced: true, Default: "2404"},
	{Path: "cni", Label: "CNI", Kind: FSelect, Options: []string{"cilium", "kube-router"}, Default: "cilium"},
	{Path: "kubeVipVersion", Label: "kube-vip version", Advanced: true, Default: "1.1.2"},
}

// PartFields are the settings of a kind of part, by the config kind
// (config.KindTinkerbell and so on).
func PartFields(kind string) []Field {
	switch kind {
	case config.KindTinkerbell:
		return TinkerbellFields
	case config.KindClusterAPI:
		return ClusterAPIFields
	case config.KindManagementCluster:
		return ManagementClusterFields
	case config.KindHardware:
		return HardwareFields
	case config.KindWorkloadCluster:
		return WorkloadClusterFields
	case config.KindTestbed:
		return TestbedFields
	}
	return nil
}

// TestFields are the settings of a test other than its steps.
var TestFields = []Field{
	{Path: "description", Label: "Description"},
	{Path: "labels", Label: "Labels", Kind: FWords, Help: "select tests with a label filter"},
	{Path: "requires.spare", Label: "Spare machines", Kind: FInt, Default: "0", Help: "machines the test takes from the spare pool"},
	{Path: "once", Label: "Run once per testbed", Kind: FBool, Default: "false", Help: "later suites that list it reuse the first result"},
}

// SuiteFields are the settings of a suite.
var SuiteFields = []Field{
	{Path: "description", Label: "Description"},
	{Path: "tests", Label: "Tests", Kind: FLines, Help: "one per line, run in this order"},
}

// Step kinds: a step runs exactly one of these.
const (
	StepUses      = "uses"
	StepExec      = "exec"
	StepContainer = "container"
	StepChainsaw  = "chainsaw"
	StepGinkgo    = "ginkgo"
)

// StepKinds are the ways a step can run, in display order.
var StepKinds = []string{StepUses, StepExec, StepContainer, StepChainsaw, StepGinkgo}

// StepKindHelp says in one line what each step kind does, adding to the
// kind's label.
var StepKindHelp = map[string]string{
	StepUses:      "A step by name: one that comes with Wendy, such as creating or scaling the workload cluster, or one of yours.",
	StepExec:      "Runs a program on this host, with the testbed's kubeconfigs in its environment variables.",
	StepContainer: "Runs a container image, with the testbed's kubeconfigs mounted.",
	StepChainsaw:  "Runs declarative Kubernetes tests, written for Kyverno Chainsaw, from a directory in your config.",
	StepGinkgo:    "Runs a compiled Go test binary written with Ginkgo, and records its results.",
}

// StepFields are the settings of a step. The ones with Only are specific to
// a step kind; `uses` has its Options filled in from the named steps.
var StepFields = []Field{
	{Path: "name", Label: "Name"},
	{Path: "timeout", Label: "Timeout", Default: "15m", Help: "e.g. 90s, 15m, 1h"},
	{Path: "retries", Label: "Retries", Kind: FInt, Default: "0"},
	{Path: "labels", Label: "Labels", Kind: FWords},
	{Path: "when", Label: "Only on combinations with labels", Help: "skips this step unless the combination's labels match, e.g. ipv6"},
	{Path: "continueOnFailure", Label: "Continue on failure", Kind: FBool, Default: "false"},

	{Path: "uses", Label: "Named step", Kind: FSelect, Required: true, Only: []string{StepUses}},
	{Path: "with", Label: "Parameters", Kind: FYAML, Only: []string{StepUses}, Help: "YAML, for a built-in step only; see its parameters under Steps"},

	{Path: "exec.command", Label: "Command", Kind: FLines, Required: true, Only: []string{StepExec}, Help: "one argument per line; the first is the program"},
	{Path: "exec.env", Label: "Environment variables", Kind: FMap, Only: []string{StepExec}, Help: "KEY=value per line"},

	{Path: "container.image", Label: "Image", Required: true, Only: []string{StepContainer}},
	{Path: "container.command", Label: "Entrypoint", Kind: FLines, Only: []string{StepContainer}, Help: "one argument per line"},
	{Path: "container.args", Label: "Arguments", Kind: FLines, Only: []string{StepContainer}, Help: "one argument per line"},
	{Path: "container.env", Label: "Environment variables", Kind: FMap, Only: []string{StepContainer}, Help: "KEY=value per line"},

	{Path: "chainsaw.path", Label: "Test directory", Required: true, Only: []string{StepChainsaw}, Help: "relative to the config directory"},
	{Path: "chainsaw.selector", Label: "Selectors", Kind: FLines, Only: []string{StepChainsaw}, Help: "one label selector per line"},
	{Path: "chainsaw.image", Label: "Chainsaw image", Only: []string{StepChainsaw}, Default: "ghcr.io/kyverno/chainsaw:v0.2.15"},

	{Path: "ginkgo.binary", Label: "Test binary", Required: true, Only: []string{StepGinkgo}},
	{Path: "ginkgo.labelFilter", Label: "Label filter", Only: []string{StepGinkgo}},
	{Path: "ginkgo.args", Label: "Arguments", Kind: FLines, Only: []string{StepGinkgo}, Help: "one argument per line"},
}

// StepKindOf is the kind a step runs as. Presets count as their own kind,
// since that is how the user wrote them; a step from the catalog has expanded
// to exec or container.
func StepKindOf(step map[string]any) string {
	for _, k := range StepKinds {
		if _, ok := step[k]; ok {
			return k
		}
	}
	return ""
}

// FieldsFor are the step fields that apply to a kind.
func FieldsFor(kind string) []Field {
	var out []Field
	for _, f := range StepFields {
		if len(f.Only) == 0 || slices.Contains(f.Only, kind) {
			out = append(out, f)
		}
	}
	return out
}

// ChoiceLabel is how a form names value of a select or yes/no field: the
// choice's label, or Yes or No.
func ChoiceLabel(f Field, value string) string {
	if f.Kind == FBool {
		switch value {
		case "true":
			return "Yes"
		case "false":
			return "No"
		}
	}
	return vocab.Choice(f.Path, value)
}

// DefaultLabel is how a form names the choice that leaves f unset, so its
// default applies: "Network boot (default)", or "Default (on with IPv6)" for
// a default that is none of the choices. A field with no default reads "—".
func DefaultLabel(f Field) string {
	switch {
	case f.Default == "":
		return "—"
	case f.Kind == FBool && f.Default != "true" && f.Default != "false":
		return "Default (" + f.Default + ")"
	}
	return ChoiceLabel(f, f.Default) + " (default)"
}

// Choices are the values a select or yes/no field lists after its default.
// The default is not among them: the choice that leaves the field unset
// stands for it, so it is listed once.
func Choices(f Field) []string {
	opts := f.Options
	if f.Kind == FBool {
		opts = []string{"true", "false"}
	}
	out := make([]string, 0, len(opts))
	for _, o := range opts {
		if o != f.Default {
			out = append(out, o)
		}
	}
	return out
}

// Get is a field's value as the text a form shows. An unset field is "".
func Get(v map[string]any, f Field) string {
	raw, ok := lookup(v, f.Path)
	if !ok {
		return ""
	}
	switch f.Kind {
	case FInt:
		if n, ok := asInt(raw); ok {
			return strconv.FormatInt(n, 10)
		}
	case FBool:
		if b, ok := raw.(bool); ok {
			return strconv.FormatBool(b)
		}
	case FLines:
		return strings.Join(strs(raw), "\n")
	case FWords:
		return strings.Join(strs(raw), " ")
	case FMap:
		m, _ := raw.(map[string]any)
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		lines := make([]string, 0, len(keys))
		for _, k := range keys {
			lines = append(lines, fmt.Sprintf("%s=%v", k, m[k]))
		}
		return strings.Join(lines, "\n")
	case FYAML:
		b, err := yaml.Marshal(raw)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	default:
		if s, ok := raw.(string); ok {
			return s
		}
	}
	return fmt.Sprint(raw)
}

// Set parses text into a field. Blank text unsets the field, so the schema's
// default applies again.
func Set(v map[string]any, f Field, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		unset(v, f.Path)
		return nil
	}
	var val any
	switch f.Kind {
	case FInt:
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil || n < 0 {
			return fmt.Errorf("%s: a whole number, 0 or more", f.Label)
		}
		val = n
	case FBool:
		b, err := strconv.ParseBool(text)
		if err != nil {
			return fmt.Errorf("%s: true or false", f.Label)
		}
		val = b
	case FSelect:
		if len(f.Options) > 0 && !slices.Contains(f.Options, text) {
			return fmt.Errorf("%s: one of %s", f.Label, strings.Join(f.Options, ", "))
		}
		val = text
	case FLines:
		val = toAny(lines(text))
	case FWords:
		val = toAny(strings.FieldsFunc(text, func(r rune) bool { return r == ' ' || r == ',' || r == '\n' || r == '\t' }))
	case FMap:
		m := map[string]any{}
		for _, l := range lines(text) {
			k, e, ok := strings.Cut(l, "=")
			if !ok || strings.TrimSpace(k) == "" {
				return fmt.Errorf("%s: %q is not KEY=value", f.Label, l)
			}
			m[strings.TrimSpace(k)] = e
		}
		val = m
	case FYAML:
		d, err := config.FromYAML(text)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Label, err)
		}
		if _, ok := d.(map[string]any); !ok {
			return fmt.Errorf("%s: a mapping of keys to values", f.Label)
		}
		val = d
	default:
		val = text
	}
	assign(v, f.Path, val)
	return nil
}

// Missing lists the required fields of fields that v leaves unset.
func Missing(v map[string]any, fields []Field) []string {
	var out []string
	for _, f := range fields {
		if !f.Required && f.RequiredUnless == "" {
			continue
		}
		if _, ok := lookup(v, f.Path); ok {
			continue
		}
		if _, ok := lookup(v, f.RequiredUnless); f.RequiredUnless != "" && ok {
			continue
		}
		out = append(out, f.Label)
	}
	return out
}

func lookup(v map[string]any, path string) (any, bool) {
	var cur any = v
	for _, k := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = m[k]; !ok {
			return nil, false
		}
	}
	return cur, true
}

func assign(v map[string]any, path string, val any) {
	keys := strings.Split(path, ".")
	cur := v
	for _, k := range keys[:len(keys)-1] {
		next, ok := cur[k].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[k] = next
		}
		cur = next
	}
	cur[keys[len(keys)-1]] = val
}

// unset removes a value and the parents it leaves empty.
func unset(v map[string]any, path string) {
	keys := strings.Split(path, ".")
	var parents []map[string]any
	cur := v
	for _, k := range keys[:len(keys)-1] {
		next, ok := cur[k].(map[string]any)
		if !ok {
			return
		}
		parents = append(parents, cur)
		cur = next
	}
	delete(cur, keys[len(keys)-1])
	for i := len(parents) - 1; i >= 0 && len(cur) == 0; i-- {
		delete(parents[i], keys[i])
		cur = parents[i]
	}
}

func asInt(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), n == float64(int64(n))
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	}
	return 0, false
}

func strs(v any) []string {
	l, _ := v.([]any)
	out := make([]string, 0, len(l))
	for _, e := range l {
		out = append(out, fmt.Sprint(e))
	}
	return out
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func lines(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// NewValue is the start of a new entry. A part with fields the schema has no
// default for starts from the versions the built-in parts use.
func NewValue(kind string) map[string]any {
	v := map[string]any{}
	switch kind {
	case config.KindClusterAPI, config.KindWorkloadCluster:
		for _, f := range PartFields(kind) {
			if f.Required || f.Path == "capt.version" {
				_ = Set(v, f, f.Default)
			}
		}
	}
	return v
}

// Steps returns a test's steps as plain data.
func Steps(test map[string]any) []map[string]any {
	l, _ := test["steps"].([]any)
	out := make([]map[string]any, 0, len(l))
	for _, e := range l {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// SetSteps stores steps in a test.
func SetSteps(test map[string]any, steps []map[string]any) {
	l := make([]any, len(steps))
	for i, s := range steps {
		l[i] = s
	}
	test["steps"] = l
}

// Valid is whether a step names what it runs, with what, as far as forms can
// tell; the schema has the final say.
func Valid(step map[string]any) []string {
	kind := StepKindOf(step)
	if kind == "" {
		return []string{"a step needs one of " + strings.Join(StepKinds, ", ")}
	}
	return Missing(step, FieldsFor(kind))
}
