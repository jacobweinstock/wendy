package preview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinkerbell/wendy/internal/config"
)

func builtins() []config.Builtin {
	var out []config.Builtin
	for _, id := range strings.Fields(`testbed.network testbed.registry testbed.distro testbed.bmc tinkerbell.install
		tinkerbell.upgrade capi.init capi.upgrade hardware.create hardware.register capt.cluster.create
		capt.cluster.delete capt.scale capt.upgrade.kubernetes capt.move capt.remediate workloadCluster.cni
		assert.workflows assert.apiserver assert.nodes assert.capi-ready assert.pods assert.components assert.healthy
		collect.cluster collect.tinkerbell collect.bmc collect.consoles`) {
		out = append(out, config.Builtin{ID: id, Schema: "{...}"})
	}
	return out
}

const cfg = `package t

import "github.com/tinkerbell/wendy/cue/wendy"

wendy.#Config & {
	testbeds: default: {}
	parts: tinkerbell: dev: source: repo: "/src/tinkerbell"
	testbeds: dev: tinkerbell: "dev"
	tests: labelled: {
		labels: ["health"]
		steps: [
			{name: "v6 only", labels: ["ipv6"], exec: command: ["true"]},
			{name: "iso only", when: "isoboot", exec: command: ["true"]},
			{name: "always", exec: command: ["true"]},
			{name: "always", exec: command: ["true"]},
		]
	}
	tests: greedy: {requires: spare: 2, steps: [{exec: command: ["true"]}]}
	tests: up: steps: [{uses: "tinkerbell.upgrade", with: to: source: repo: "/src/tinkerbell"}]
	suites: t: tests: ["labelled", "greedy", "up"]
	testbeds: m: matrix: {
		order: ["ipFamily", "bootMode"]
		axes: {
			ipFamily: {ipv4: network: family: "ipv4", ipv6: network: family: "ipv6"}
			bootMode: {netboot: bootMode: "netboot", isoboot: bootMode: "isoboot"}
		}
	}
	plans: p: {testbed: "m", combinations: ["ipv4-*", "ipv6-isoboot"], suites: ["t"], parallel: 2}
	plans: d: {testbed: "dev", suites: ["t"]}
	plans: debug: {testbed: "default", suites: ["t"], diagnostics: "always", consoles: false}
	plans: quiet: {testbed: "m", suites: ["t"], diagnostics: "never"}
	parts: workloadCluster: next: kubernetesVersion: "v1.37.0"
	parts: hardware: ha: machines: {controlPlane: count: 3, worker: count: 3}
	testbeds: sized: matrix: {
		order: ["size"]
		axes: size: {small: {}, ha: hardware: "ha", next: workloadCluster: "next"}
	}
	plans: sized: {testbed: "sized", suites: ["t"]}
}
`

func catalog(t *testing.T) *config.Catalog {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "c.cue"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	cat := (&config.Loader{Dirs: []string{dir}, Builtins: builtins()}).Load()
	if errs := cat.AllErrors(); len(errs) > 0 {
		t.Fatal(errs)
	}
	return cat
}

func testNamed(t *testing.T, c *Combination, name string) Test {
	t.Helper()
	for _, s := range c.Suites[0].Tests {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no test %s in %s", name, c.Name)
	return Test{}
}

func TestPlanMatrix(t *testing.T) {
	cat := catalog(t)
	p, err := New(cat, Selection{Plan: "p"})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range p.Combinations {
		names = append(names, c.Name)
	}
	if got := strings.Join(names, ","); got != "ipv4-netboot,ipv4-isoboot,ipv6-isoboot" {
		t.Fatalf("combinations = %s", got)
	}
	if p.Key != "plan:p" || p.Edited {
		t.Errorf("key = %s edited = %v", p.Key, p.Edited)
	}

	skips := func(c *Combination) []string {
		var out []string
		for _, st := range testNamed(t, c, "labelled").Steps {
			out = append(out, st.Key+"="+st.Skip)
		}
		return out
	}
	if got := strings.Join(skips(p.Combinations[0]), "|"); got != "v6 only=filtered by labels|iso only=when: isoboot|always=|always#2=" {
		t.Errorf("ipv4-netboot steps: %s", got)
	}
	if got := strings.Join(skips(p.Combinations[2]), "|"); got != "v6 only=|iso only=|always=|always#2=" {
		t.Errorf("ipv6-isoboot steps: %s", got)
	}
	if s := testNamed(t, p.Combinations[0], "greedy").Skip; !strings.Contains(s, "needs 2 spare") {
		t.Errorf("greedy skip = %q", s)
	}
	if p.Needs.VMs != 6 {
		t.Errorf("needs %d VMs at parallel 2, want 6", p.Needs.VMs)
	}
}

func TestUserLabelFilterAndEdits(t *testing.T) {
	cat := catalog(t)
	p, err := New(cat, Selection{Plan: "p", LabelFilter: "!health"})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Edited || !strings.HasPrefix(p.Key, "selection:") {
		t.Errorf("edited = %v key = %s", p.Edited, p.Key)
	}
	if s := testNamed(t, p.Combinations[0], "labelled").Skip; s != "filtered by labels" {
		t.Errorf("labelled skip = %q", s)
	}
	if s := testNamed(t, p.Combinations[0], "up").Skip; s != "" {
		t.Errorf("unlabelled test skipped: %q", s)
	}
}

func TestBuildsDeduplicated(t *testing.T) {
	cat := catalog(t)
	p, err := New(cat, Selection{Plan: "d"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Builds) != 1 || p.Builds[0].Component != "tinkerbell" || p.Builds[0].Source.Repo != "/src/tinkerbell" {
		t.Fatalf("builds = %+v", p.Builds)
	}
}

// Diagnostics are an execution's setting, so a plan decides them for every
// combination and a testbed without a plan gets the defaults.
func TestDiagnosticsComeFromThePlan(t *testing.T) {
	cat := catalog(t)

	debug, err := New(cat, Selection{Plan: "debug"})
	if err != nil {
		t.Fatal(err)
	}
	if got := debug.Combinations[0].Testbed.Diagnostics; got.Collect != "always" || got.Consoles {
		t.Errorf("debug diagnostics = %+v", got)
	}

	quiet, err := New(cat, Selection{Plan: "quiet"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range quiet.Combinations {
		if got := c.Testbed.Diagnostics; got.Collect != "never" || !got.Consoles {
			t.Errorf("%s: diagnostics = %+v", c.Name, got)
		}
	}

	plain, err := New(cat, Selection{Testbed: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if got := plain.Combinations[0].Testbed.Diagnostics; got != config.DefaultDiagnostics() {
		t.Errorf("no plan: diagnostics = %+v", got)
	}
}

// A combination records the parts it was built from, and the host footprint
// follows the hardware part an axis swaps in.
func TestCombinationsRecordTheirParts(t *testing.T) {
	cat := catalog(t)
	p, err := New(cat, Selection{Plan: "sized"})
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]map[string]string{}
	for _, c := range p.Combinations {
		parts[c.Name] = c.Parts
	}
	if parts["ha"][config.KindHardware] != "ha" || parts["small"][config.KindHardware] != "small" || parts["next"][config.KindWorkloadCluster] != "next" {
		t.Errorf("parts = %v", parts)
	}
	// One at a time, the biggest combination decides: 3+3+1 machines plus a kind cluster.
	if p.Needs.VMs != 7 || p.Needs.RAMMiB != 7*2048+kindOverheadMiB {
		t.Errorf("needs = %+v", p.Needs)
	}

	plain, err := New(cat, Selection{Testbed: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if got := plain.Combinations[0].Parts; got[config.KindTinkerbell] != "dev" || got[config.KindHardware] != "small" {
		t.Errorf("testbed parts = %v", got)
	}
}

// A testbed without a matrix has no combinations to pick. Picking another
// testbed than a plan's drops the plan's combinations, which name the plan's
// testbed's.
func TestCombinationsNeedAMatrix(t *testing.T) {
	cat := catalog(t)
	if _, err := New(cat, Selection{Testbed: "default", Combinations: []string{"x-*"}}); err == nil || !strings.Contains(err.Error(), "testbed default has no matrix") {
		t.Errorf("err = %v", err)
	}
	p, err := New(cat, Selection{Plan: "p", Testbed: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Combinations) != 1 || p.Combinations[0].Name != "default" || !p.Edited {
		t.Errorf("combinations = %+v, edited = %v", p.Combinations, p.Edited)
	}
}

// An override changes diagnostics for one execution: it marks the plan
// edited, and an override equal to the plan's does not.
func TestDiagnosticsOverride(t *testing.T) {
	cat := catalog(t)
	never := config.Diagnostics{Collect: "never", Consoles: true}

	p, err := New(cat, Selection{Plan: "debug", Diagnostics: &never})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Edited || !strings.HasPrefix(p.Key, "selection:") {
		t.Errorf("edited = %v, key = %s", p.Edited, p.Key)
	}
	if got := p.Combinations[0].Testbed.Diagnostics; got != never {
		t.Errorf("diagnostics = %+v", got)
	}

	same := config.Diagnostics{Collect: "always", Consoles: false} // what the debug plan says
	p, err = New(cat, Selection{Plan: "debug", Diagnostics: &same})
	if err != nil {
		t.Fatal(err)
	}
	if p.Edited || p.Key != "plan:debug" {
		t.Errorf("the plan's own diagnostics marked it edited: %v %s", p.Edited, p.Key)
	}

	p, err = New(cat, Selection{Testbed: "default", Diagnostics: &never})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Combinations[0].Testbed.Diagnostics; got != never {
		t.Errorf("without a plan: diagnostics = %+v", got)
	}
}

func TestUnknownSuite(t *testing.T) {
	_, err := New(catalog(t), Selection{Testbed: "default", Suites: []string{"smok"}})
	if err == nil || !strings.Contains(err.Error(), `did you mean "smoke"`) {
		t.Fatalf("err = %v", err)
	}
}

// A test that uses a step of yours runs that step, under its name, with
// the test's own settings over it.
func TestTestsUseYourSteps(t *testing.T) {
	dir := t.TempDir()
	src := `package t

steps: wait: {exec: command: ["kubectl", "wait"], timeout: "30m"}
tests: one: steps: [{uses: "wait"}]
tests: two: steps: [{uses: "wait", timeout: "5m"}]
suites: s: tests: ["one", "two"]
`
	if err := os.WriteFile(filepath.Join(dir, "c.cue"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cat := (&config.Loader{Dirs: []string{dir}, Builtins: builtins()}).Load()
	p, err := New(cat, Selection{Testbed: "quickstart", Suites: []string{"s"}})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, tc := range p.Combinations[0].Suites[0].Tests {
		st := tc.Steps[0]
		got[tc.Name] = st.Key + " " + st.Timeout
		if st.Exec == nil {
			t.Errorf("%s's step does not run the command: %+v", tc.Name, st)
		}
	}
	if got["one"] != "wait 30m" || got["two"] != "wait 5m" {
		t.Errorf("steps = %v", got)
	}
}

func TestImplicitFilter(t *testing.T) {
	got := ImplicitFilter(config.IntrinsicAxes, map[string]string{"ipFamily": "ipv6", "topology": "colocated"})
	if got != "!ipv4 && !external" {
		t.Errorf("filter = %q", got)
	}
}
