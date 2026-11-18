// Package preview turns a selection into the exact work an execution will do.
package preview

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2/types"

	"github.com/tinkerbell/wendy/internal/config"
)

// Selection is what to run: a plan, optionally edited, or a testbed with its
// own options.
type Selection struct {
	Plan    string `json:"plan,omitempty"`
	Testbed string `json:"testbed,omitempty"`
	// Combinations are glob patterns over the testbed's combination names;
	// empty means all. Only a testbed with a matrix has combinations to pick.
	Combinations   []string `json:"combinations,omitempty"`
	Suites         []string `json:"suites,omitempty"`
	LabelFilter    string   `json:"labelFilter,omitempty"`
	Parallel       int      `json:"parallel,omitempty"`
	Keep           string   `json:"keep,omitempty"`
	PauseOnFailure *bool    `json:"pauseOnFailure,omitempty"`
	// Diagnostics overrides what a failed execution collects, for this
	// execution only: the plan's setting, or the defaults without a plan.
	Diagnostics *config.Diagnostics `json:"diagnostics,omitempty"`
	// SetupOnly builds the testbed and runs no suites: `wendy env up`.
	SetupOnly bool `json:"setupOnly,omitempty"`
}

// Preview is a resolved selection.
type Preview struct {
	Selection      Selection      `json:"selection"`
	Label          string         `json:"label"`
	Edited         bool           `json:"edited,omitempty"`
	Key            string         `json:"key"`
	Combinations   []*Combination `json:"combinations"`
	Builds         []Build        `json:"builds,omitempty"`
	Parallel       int            `json:"parallel"`
	Keep           string         `json:"keep"`
	PauseOnFailure bool           `json:"pauseOnFailure"`
	Needs          Needs          `json:"needs"`
	Warnings       []string       `json:"warnings,omitempty"`
}

// Combination is one combination of the testbed and the suites run against
// it.
type Combination struct {
	Name   string            `json:"name"`
	Labels []string          `json:"labels"`
	Axes   map[string]string `json:"axes"`
	// Parts names the part the combination uses of each kind; a kind built
	// from an inline part, or from a part an axis patched, has no entry.
	Parts   map[string]string `json:"parts,omitempty"`
	Testbed config.Testbed    `json:"testbed"`
	Setup   []Step            `json:"setup"`
	Suites  []Suite           `json:"suites"`
}

type Suite struct {
	Name  string `json:"name"`
	Tests []Test `json:"tests"`
}

type Test struct {
	Name   string   `json:"name"`
	Labels []string `json:"labels,omitempty"`
	Once   bool     `json:"once,omitempty"`
	Steps  []Step   `json:"steps"`
	Skip   string   `json:"skip,omitempty"`
}

// Step is a configured step and, when it will not run, why.
type Step struct {
	config.Step
	// Key is unique within its test; it is the last element of the
	// step's event path.
	Key  string `json:"key"`
	Skip string `json:"skip,omitempty"`
}

// Build is a source referenced by the preview. The engine resolves it to a
// commit and builds each distinct one once.
type Build struct {
	Component string        `json:"component"`
	Source    config.Source `json:"source"`
	UsedBy    []string      `json:"usedBy"`
}

// Needs is what the preview asks of the host at its busiest.
type Needs struct {
	VMs      int           `json:"vms"`
	RAMMiB   int           `json:"ramMiB"`
	DiskGiB  int           `json:"diskGiB"`
	Estimate time.Duration `json:"estimate,omitempty"`
	Samples  int           `json:"samples,omitempty"`
}

// kindOverheadMiB and kindDiskGiB approximate one kind cluster.
const (
	kindOverheadMiB = 2048
	kindDiskGiB     = 4
)

// New resolves sel against the catalog.
func New(cat *config.Catalog, sel Selection) (*Preview, error) {
	eff, label, edited, err := effective(cat, sel)
	if err != nil {
		return nil, err
	}
	p := &Preview{Selection: sel, Label: label, Edited: edited, Parallel: max(eff.Parallel, 1), Keep: eff.Keep}
	if p.Keep == "" {
		p.Keep = "never"
	}
	if eff.PauseOnFailure != nil {
		p.PauseOnFailure = *eff.PauseOnFailure
	}
	p.Key = key(sel.Plan, edited, eff)

	userFilter, err := types.ParseLabelFilter(eff.LabelFilter)
	if err != nil {
		return nil, fmt.Errorf("label filter %q: %w", eff.LabelFilter, err)
	}

	var raw []config.Combination
	axes := map[string][]string{}
	for k, v := range config.IntrinsicAxes {
		axes[k] = v
	}
	if eff.Testbed == "" {
		return nil, fmt.Errorf("select a plan or a testbed")
	}
	tb, ok := cat.Testbeds[eff.Testbed]
	if !ok {
		return nil, unknown("testbed", eff.Testbed, config.Names(cat.Testbeds))
	}
	if tb.HasMatrix() {
		for axis, values := range tb.Value.Matrix.Axes {
			axes[axis] = config.Names(values)
		}
		for _, c := range tb.Combinations {
			if !matches(c.Name, eff.Combinations) || c.Excluded != "" {
				continue
			}
			if c.Err != "" {
				return nil, fmt.Errorf("combination %s: %s", c.Name, c.Err)
			}
			raw = append(raw, c)
		}
		if len(raw) == 0 {
			return nil, fmt.Errorf("no combination of %s matches %v", eff.Testbed, eff.Combinations)
		}
	} else {
		switch {
		case len(eff.Combinations) > 0 && sel.Combinations == nil && sel.Plan != "":
			return nil, fmt.Errorf("plan %s picks combinations, but testbed %s has no matrix", sel.Plan, eff.Testbed)
		case len(eff.Combinations) > 0:
			return nil, fmt.Errorf("testbed %s has no matrix, so it has no combinations to pick", eff.Testbed)
		case len(tb.Combinations) == 0:
			return nil, fmt.Errorf("testbed %s does not resolve; `wendy vet` shows why", eff.Testbed)
		}
		raw = tb.Combinations
	}

	suites := eff.Suites
	if len(suites) == 0 {
		suites = []string{"smoke"}
	}
	if sel.SetupOnly {
		suites = nil
	}
	for _, s := range suites {
		if _, ok := cat.Suites[s]; !ok {
			return nil, unknown("suite", s, config.Names(cat.Suites))
		}
	}

	diagnostics := config.DefaultDiagnostics()
	if eff.Diagnostics != nil {
		diagnostics = *eff.Diagnostics
	}
	for _, rc := range raw {
		c, err := buildCombination(cat, rc, axes, suites, userFilter, diagnostics)
		if err != nil {
			return nil, err
		}
		p.Combinations = append(p.Combinations, c)
	}
	p.Builds = collectBuilds(p.Combinations)
	p.Needs = needs(p.Combinations, p.Parallel)
	return p, nil
}

// effective applies a selection's own fields over its plan's. A selection that
// picks another testbed drops the plan's combinations, which name the plan's
// testbed's.
func effective(cat *config.Catalog, sel Selection) (Selection, string, bool, error) {
	if sel.Plan == "" {
		return sel, sel.Testbed, false, nil
	}
	pi, ok := cat.Plans[sel.Plan]
	if !ok {
		return sel, "", false, unknown("plan", sel.Plan, config.Names(cat.Plans))
	}
	pl := pi.Value
	pause, planDiagnostics := pl.PauseOnFailure, pl.Collects()
	eff := Selection{
		Testbed: pl.Testbed, Combinations: pl.Combinations, Suites: pl.Suites,
		LabelFilter: pl.LabelFilter, Parallel: pl.Parallel, Keep: pl.Keep, PauseOnFailure: &pause, Diagnostics: &planDiagnostics,
	}
	edited := false
	set := func(changed bool) {
		edited = edited || changed
	}
	if sel.Testbed != "" && sel.Testbed != eff.Testbed {
		set(true)
		eff.Testbed, eff.Combinations = sel.Testbed, nil
	}
	if sel.Combinations != nil {
		set(!slices.Equal(sel.Combinations, eff.Combinations))
		eff.Combinations = sel.Combinations
	}
	if sel.Suites != nil {
		set(!slices.Equal(sel.Suites, eff.Suites))
		eff.Suites = sel.Suites
	}
	if sel.LabelFilter != "" {
		set(sel.LabelFilter != eff.LabelFilter)
		eff.LabelFilter = sel.LabelFilter
	}
	if sel.Parallel != 0 {
		set(sel.Parallel != eff.Parallel)
		eff.Parallel = sel.Parallel
	}
	if sel.Keep != "" {
		set(sel.Keep != eff.Keep)
		eff.Keep = sel.Keep
	}
	if sel.PauseOnFailure != nil {
		set(*sel.PauseOnFailure != pause)
		eff.PauseOnFailure = sel.PauseOnFailure
	}
	if sel.Diagnostics != nil {
		set(*sel.Diagnostics != planDiagnostics)
		eff.Diagnostics = sel.Diagnostics
	}
	return eff, sel.Plan, edited, nil
}

// key identifies a selection across executions, so estimates come from
// executions of the same thing: an unedited plan by name, anything else by
// content.
func key(plan string, edited bool, eff Selection) string {
	if plan != "" && !edited {
		return PlanKey(plan)
	}
	b, _ := json.Marshal(eff)
	sum := sha256.Sum256(b)
	return "selection:" + hex.EncodeToString(sum[:6])
}

// PlanKey is the key of the executions of an unedited plan, by which its past
// durations and last result are found.
func PlanKey(plan string) string { return "plan:" + plan }

func buildCombination(cat *config.Catalog, rc config.Combination, axes map[string][]string, suites []string, user types.LabelFilter, diagnostics config.Diagnostics) (*Combination, error) {
	// A testbed says what is built; what a failure collects belongs to the
	// execution.
	rc.Testbed.Diagnostics = diagnostics
	values := rc.Testbed.AxisValues()
	for k, v := range rc.Axes {
		values[k] = v
	}
	c := &Combination{Name: rc.Name, Axes: values, Parts: rc.Parts, Testbed: rc.Testbed}
	for _, axis := range config.Names(values) {
		if !slices.Contains(c.Labels, values[axis]) {
			c.Labels = append(c.Labels, values[axis])
		}
	}
	implicit := ImplicitFilter(axes, values)
	implicitFn, err := types.ParseLabelFilter(implicit)
	if err != nil {
		return nil, fmt.Errorf("combination %s: implicit filter %q: %w", rc.Name, implicit, err)
	}
	selected := func(labels []string) bool { return implicitFn(labels) && user(labels) }

	c.Setup = keyed(rc.Testbed.Setup, func(config.Step) string { return "" })
	spares := rc.Testbed.Hardware.Machines["spare"].Count
	for _, sn := range suites {
		suite := Suite{Name: sn}
		for _, tn := range cat.Suites[sn].Value.Tests {
			tc := cat.Tests[tn].Value
			test := Test{Name: tn, Labels: tc.Labels, Once: tc.Once}
			steps := make([]config.Step, len(tc.Steps))
			for i, st := range tc.Steps {
				steps[i] = cat.Expand(st)
			}
			test.Steps = keyed(steps, func(st config.Step) string {
				if !selected(union(tc.Labels, st.Labels)) {
					return "filtered by labels"
				}
				if st.When != "" {
					when, err := types.ParseLabelFilter(st.When)
					if err != nil {
						return fmt.Sprintf("bad when %q: %v", st.When, err)
					}
					if !when(c.Labels) {
						return "when: " + st.When
					}
				}
				return ""
			})
			switch {
			case tc.Requires.Spare > spares:
				test.Skip = fmt.Sprintf("needs %d spare machine(s), the testbed has %d", tc.Requires.Spare, spares)
			case allSkipped(test.Steps):
				test.Skip = "filtered by labels"
			}
			suite.Tests = append(suite.Tests, test)
		}
		c.Suites = append(c.Suites, suite)
	}
	return c, nil
}

// ImplicitFilter excludes every other value of every axis the combination
// has a value on, so a step labelled for one value runs only there.
func ImplicitFilter(axes map[string][]string, values map[string]string) string {
	var parts []string
	for _, axis := range config.Names(axes) {
		v, ok := values[axis]
		if !ok {
			continue
		}
		for _, other := range axes[axis] {
			if other != v && !slices.Contains(parts, "!"+other) && !containsValue(values, other) {
				parts = append(parts, "!"+other)
			}
		}
	}
	return strings.Join(parts, " && ")
}

func containsValue(values map[string]string, v string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}

func keyed(steps []config.Step, skip func(config.Step) string) []Step {
	seen := map[string]int{}
	out := make([]Step, 0, len(steps))
	for _, st := range steps {
		k := st.DisplayName()
		seen[k]++
		if seen[k] > 1 {
			k = fmt.Sprintf("%s#%d", k, seen[k])
		}
		out = append(out, Step{Step: st, Key: k, Skip: skip(st)})
	}
	return out
}

func allSkipped(steps []Step) bool {
	for _, s := range steps {
		if s.Skip == "" {
			return false
		}
	}
	return len(steps) > 0
}

func union(a, b []string) []string {
	out := slices.Clone(a)
	for _, x := range b {
		if !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}

func matches(name string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// collectBuilds finds every source the preview refers to, including upgrade
// targets in step params, so each is built once before any combination
// starts.
func collectBuilds(combinations []*Combination) []Build {
	byKey := map[string]*Build{}
	var order []string
	add := func(component string, src *config.Source, combination string) {
		if src == nil {
			return
		}
		k := component + "|" + src.Repo + "|" + src.Ref
		b, ok := byKey[k]
		if !ok {
			b = &Build{Component: component, Source: *src}
			byKey[k] = b
			order = append(order, k)
		}
		if !slices.Contains(b.UsedBy, combination) {
			b.UsedBy = append(b.UsedBy, combination)
		}
	}
	for _, c := range combinations {
		tb := c.Testbed
		add("tinkerbell", tb.Tinkerbell.Source, c.Name)
		add("capt", tb.ClusterAPI.CAPT.Source, c.Name)
		for _, s := range c.Suites {
			for _, sc := range s.Tests {
				for _, st := range sc.Steps {
					if sc.Skip != "" || st.Skip != "" {
						continue
					}
					switch st.Uses {
					case "tinkerbell.upgrade":
						add("tinkerbell", paramSource(st.With, "to"), c.Name)
					case "capi.upgrade":
						add("capt", paramSource(st.With, "capt"), c.Name)
					}
				}
			}
		}
	}
	out := make([]Build, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
}

func paramSource(with map[string]any, field string) *config.Source {
	target, _ := with[field].(map[string]any)
	src, ok := target["source"].(map[string]any)
	if !ok {
		return nil
	}
	repo, _ := src["repo"].(string)
	ref, _ := src["ref"].(string)
	return &config.Source{Repo: repo, Ref: ref}
}

// needs is the host footprint of the parallel combinations with the most
// machines.
func needs(combinations []*Combination, parallel int) Needs {
	type fp struct{ vms, ram, disk int }
	var fps []fp
	for _, c := range combinations {
		var f fp
		for _, m := range c.Testbed.Hardware.Machines {
			f.vms += m.Count
			f.ram += m.Count * m.MemMiB
			f.disk += m.Count * m.DiskGiB
		}
		clusters := 1
		if c.Testbed.Tinkerbell.Topology == "external" {
			clusters = 2
		}
		f.ram += clusters * kindOverheadMiB
		f.disk += clusters * kindDiskGiB
		fps = append(fps, f)
	}
	sort.Slice(fps, func(i, j int) bool { return fps[i].ram > fps[j].ram })
	var n Needs
	for i := 0; i < len(fps) && i < parallel; i++ {
		n.VMs += fps[i].vms
		n.RAMMiB += fps[i].ram
		n.DiskGiB += fps[i].disk
	}
	return n
}

// Estimate sets the preview's expected duration from past executions of the
// same selection: their median.
func (p *Preview) Estimate(past []time.Duration) {
	if len(past) == 0 {
		return
	}
	s := slices.Clone(past)
	slices.Sort(s)
	p.Needs.Estimate = s[len(s)/2]
	p.Needs.Samples = len(s)
}

// StepCount is how many steps an execution of the preview runs, setup
// included.
func (p *Preview) StepCount() int {
	n := 0
	for _, c := range p.Combinations {
		n += len(c.Setup)
		for _, s := range c.Suites {
			for _, sc := range s.Tests {
				if sc.Skip != "" {
					continue
				}
				for _, st := range sc.Steps {
					if st.Skip == "" {
						n++
					}
				}
			}
		}
	}
	return n
}

func unknown(kind, name string, candidates []string) error {
	msg := fmt.Sprintf("unknown %s %q", kind, name)
	if s := config.Suggest(name, candidates); s != "" {
		msg += fmt.Sprintf(" (did you mean %q?)", s)
	}
	return fmt.Errorf("%s", msg)
}
