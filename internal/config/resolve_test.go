package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestQuickstartGolden holds the built-in quickstart testbed as the engine
// receives it, so reshaping the config cannot change what an execution is
// built from. Regenerate with WENDY_UPDATE_GOLDEN=1 only when the resolved
// shape is meant to change.
func TestQuickstartGolden(t *testing.T) {
	cat := (&Loader{Builtins: testBuiltins()}).Load()
	if errs := cat.AllErrors(); len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	got, err := json.MarshalIndent(resolved(t, cat, "quickstart"), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "quickstart.golden.json")
	if os.Getenv("WENDY_UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("quickstart resolved differently.\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func loadOne(t *testing.T, src string) *Catalog {
	t.Helper()
	return (&Loader{Dirs: []string{writeConfig(t, map[string]string{"a.cue": src})}, Builtins: testBuiltins()}).Load()
}

func TestTestbedUsesParts(t *testing.T) {
	cat := loadOne(t, `package a

parts: tinkerbell: stable: chart: version: "v0.19.0"
parts: hardware: ha: machines: {controlPlane: count: 3, worker: count: 3}
parts: workloadCluster: next: kubernetesVersion: "v1.37.0"

testbeds: {
	big:    {hardware: "ha", workloadCluster: "next", tinkerbell: "stable", bootMode: "isoboot"}
	inline: {hardware: {machines: worker: count: 5}}
	plain:  {}
}
`)
	if errs := cat.AllErrors(); len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	big, inline, plain := resolved(t, cat, "big"), resolved(t, cat, "inline"), resolved(t, cat, "plain")

	if big.Hardware.Machines["controlPlane"].Count != 3 || big.WorkloadCluster.KubernetesVersion != "v1.37.0" || big.Tinkerbell.BootMode != "isoboot" {
		t.Errorf("named parts: %+v", big)
	}
	if inline.Hardware.Machines["worker"].Count != 5 {
		t.Errorf("inline hardware worker count = %d", inline.Hardware.Machines["worker"].Count)
	}
	if inline.WorkloadCluster.KubernetesVersion != "v1.36.4" || inline.Tinkerbell.BootMode != "netboot" {
		t.Errorf("omitted parts are not the defaults: %+v", inline)
	}
	// Changing one part must leave a testbed that does not name it alone.
	if plain.Hardware.Machines["worker"].Count != 1 || plain.Hardware.Machines["controlPlane"].Count != 1 {
		t.Errorf("plain hardware = %+v", plain.Hardware.Machines)
	}
	if got := cat.Testbeds["big"].Value.PartNames(); got[KindHardware] != "ha" || got[KindTinkerbell] != "stable" {
		t.Errorf("part names = %v", got)
	}
	// A testbed without a matrix is one combination, named after it.
	if c := cat.Testbeds["big"].Combinations; len(c) != 1 || c[0].Name != "big" || !reflect.DeepEqual(c[0].Testbed, big) || c[0].Parts[KindHardware] != "ha" {
		t.Errorf("combinations = %+v", c)
	}
}

func TestUnknownPartIsReported(t *testing.T) {
	cat := loadOne(t, `package a

parts: hardware: small: machines: worker: count: 1
testbeds: x: hardware: "smal"
`)
	for _, e := range cat.AllErrors() {
		if e.Path == "testbeds.x.hardware" {
			if e.Msg != `unknown hardware part "smal" (did you mean "small"?)` || e.Line == 0 || !strings.HasSuffix(e.File, "a.cue") {
				t.Errorf("error = %+v", e)
			}
			return
		}
	}
	t.Fatalf("no error for the reference: %v", cat.AllErrors())
}

func TestFaultyPartIsReportedOnceAtThePart(t *testing.T) {
	cat := loadOne(t, `package a

parts: hardware: bad: machine: {}
parts: workloadCluster: unused: kubernetesVersion: "1.36"
testbeds: {
	a: hardware: "bad"
	b: hardware: "bad"
}
`)
	counts := map[string]int{}
	for _, e := range cat.AllErrors() {
		counts[e.Path]++
		if e.Path == "parts.hardware.bad.machine" && e.Line != 3 {
			t.Errorf("faulty part reported at line %d, want 3", e.Line)
		}
	}
	if counts["parts.hardware.bad.machine"] != 1 {
		t.Errorf("faulty part reported %d times: %v", counts["parts.hardware.bad.machine"], cat.AllErrors())
	}
	if counts["parts.workloadCluster.unused.kubernetesVersion"] != 1 {
		t.Errorf("an unused part was not checked: %v", cat.AllErrors())
	}
}

func TestTestbedUsesPartFromAnotherDirectory(t *testing.T) {
	lib := writeConfig(t, map[string]string{"lib.cue": "package lib\n\nparts: hardware: ha: machines: worker: count: 3\n"})
	use := writeConfig(t, map[string]string{"use.cue": "package use\n\ntestbeds: big: hardware: \"ha\"\n"})
	cat := (&Loader{Dirs: []string{lib, use}, Builtins: testBuiltins()}).Load()
	if errs := cat.AllErrors(); len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if got := resolved(t, cat, "big").Hardware.Machines["worker"].Count; got != 3 {
		t.Errorf("worker count = %d", got)
	}
}

// An axis value is written as the testbed writes its fields, so a field the
// testbed does not have, like the plan's diagnostics, is the schema's to
// reject, at the axis value.
func TestAxisValueOutsideTheTestbedIsRejected(t *testing.T) {
	src := "package a\n\ntestbeds: t: matrix: {order: [\"x\"], axes: x: {a: diagnostics: \"never\", b: {}}}\n"
	for _, e := range loadOne(t, src).AllErrors() {
		if strings.HasSuffix(e.Path, "testbeds.t.matrix.axes.x.a.diagnostics") {
			if !strings.Contains(e.Msg, "not allowed") || e.Line != 3 {
				t.Errorf("error = %+v", e)
			}
			return
		}
	}
	t.Fatal("diagnostics in an axis value was accepted")
}

func TestTestbedSettings(t *testing.T) {
	cat := loadOne(t, `package a

parts: tinkerbell: stable: chart: version: "v0.19.0"
testbeds: {
	iso:   {tinkerbell: "stable", bootMode: "isoboot", topology: "external", network: {family: "ipv6", mirror: enabled: true}}
	net:   {tinkerbell: "stable"}
}
`)
	if errs := cat.AllErrors(); len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	iso, net := resolved(t, cat, "iso"), resolved(t, cat, "net")
	if iso.Tinkerbell.BootMode != "isoboot" || iso.Tinkerbell.Topology != "external" || iso.Network.Family != "ipv6" || !iso.Network.NAT64 || !iso.Network.Mirror.Enabled {
		t.Errorf("settings not applied: tinkerbell %+v network %+v", iso.Tinkerbell, iso.Network)
	}
	if iso.Tinkerbell.Chart.Version != "v0.19.0" || net.Tinkerbell.Chart.Version != "v0.19.0" {
		t.Errorf("one part should serve both testbeds")
	}
	if net.Tinkerbell.BootMode != "netboot" || net.Network.Family != "ipv4" {
		t.Errorf("a testbed that sets nothing should take the defaults: %+v", net)
	}
}

func TestInvalidTestbedSettingIsReportedAtTheTestbed(t *testing.T) {
	cat := loadOne(t, "package a\n\ntestbeds: e: {bootMode: \"pxe\", network: family: \"ipx\"}\n")
	paths := map[string]bool{}
	for _, e := range cat.AllErrors() {
		paths[e.Path] = true
		if e.Line != 3 {
			t.Errorf("%s reported at line %d, want 3", e.Path, e.Line)
		}
	}
	for _, want := range []string{"testbeds.e.bootMode", "testbeds.e.network.family"} {
		if !paths[want] {
			t.Errorf("no error at %s: %v", want, cat.AllErrors())
		}
	}
}

func TestMovedTinkerbellKeysAreReported(t *testing.T) {
	cat := loadOne(t, `package a

parts: tinkerbell: old: {topology: "external", bootMode: "isoboot", network: family: "ipv6"}
testbeds: e: tinkerbell: {bootMode: "isoboot"}
testbeds: m: matrix: {order: ["boot"], axes: boot: {iso: tinkerbell: bootMode: "isoboot", net: {}}}
`)
	got := map[string]Error{}
	for _, e := range cat.AllErrors() {
		got[e.Path] = e
	}
	for path, want := range map[string]string{
		"parts.tinkerbell.old.topology":                       "set it on the testbed",
		"parts.tinkerbell.old.bootMode":                       "set it on the testbed",
		"parts.tinkerbell.old.network":                        "set it on the testbed",
		"testbeds.e.tinkerbell.bootMode":                      "set it on the testbed",
		"testbeds.m.matrix.axes.boot.iso.tinkerbell.bootMode": "beside tinkerbell",
	} {
		e, ok := got[path]
		if !ok || !strings.Contains(e.Msg, want) || e.Line == 0 {
			t.Errorf("%s: %+v, want a message saying %q", path, e, want)
		}
	}
}

func TestManagementClusterAndClusterAPIAreSeparateParts(t *testing.T) {
	cat := loadOne(t, `package a

parts: managementCluster: pinned: image: "kindest/node:v1.34.0"
parts: clusterAPI: edge: {core: "v1.15.0", capt: version: "v0.8.0"}
testbeds: {
	mc:  managementCluster: "pinned"
	api: {
		clusterAPI: "edge"
		matrix: {order: ["mc"], axes: mc: {own: {}, pinned: managementCluster: "pinned"}}
	}
}
`)
	if errs := cat.AllErrors(); len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	mc, api := resolved(t, cat, "mc"), resolved(t, cat, "api")
	if mc.ManagementCluster.Image != "kindest/node:v1.34.0" || mc.ClusterAPI.Core != "v1.14.4" {
		t.Errorf("swapping the management cluster changed Cluster API: %+v %+v", mc.ManagementCluster, mc.ClusterAPI)
	}
	if api.ClusterAPI.Core != "v1.15.0" || api.ManagementCluster.Image != "" {
		t.Errorf("swapping Cluster API changed the management cluster: %+v %+v", api.ClusterAPI, api.ManagementCluster)
	}
	for _, c := range cat.Testbeds["api"].Combinations {
		if c.Name != "pinned" {
			continue
		}
		if c.Testbed.ManagementCluster.Image != "kindest/node:v1.34.0" || c.Testbed.ClusterAPI.Core != "v1.15.0" || c.Parts[KindManagementCluster] != "pinned" {
			t.Errorf("axis pick: %+v, parts %v", c.Testbed, c.Parts)
		}
		return
	}
	t.Fatalf("no pinned combination: %+v", cat.Testbeds["api"].Combinations)
}

// partsMatrix is a versions-style matrix: one axis picks a part, the other
// picks or patches one.
const partsMatrix = `package a

parts: workloadCluster: "k8s-1.37": kubernetesVersion: "v1.37.0"
parts: hardware: ha: machines: {controlPlane: count: 3, worker: count: 3}

testbeds: ref: {hardware: "ha", workloadCluster: "k8s-1.37"}

testbeds: versions: matrix: {
	order: ["k8s", "size"]
	axes: {
		k8s: {
			"1.36": workloadCluster: "k8s-1.36"
			"1.37": workloadCluster: "k8s-1.37"
		}
		size: {
			small: {}
			ha:    hardware: "ha"
			big:   hardware: machines: worker: count: 5
		}
	}
}
`

func TestAxisSelectsParts(t *testing.T) {
	cat := loadOne(t, partsMatrix)
	if errs := cat.AllErrors(); len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	m := cat.Testbeds["versions"]
	// The combinations, their names and their axis values are the ones the
	// matrix made before it moved into the testbed.
	var names []string
	byName := map[string]Combination{}
	for _, c := range m.Combinations {
		if c.Err != "" {
			t.Errorf("%s: %s", c.Name, c.Err)
		}
		names = append(names, c.Name)
		byName[c.Name] = c
	}
	if want := []string{"1.36-small", "1.36-ha", "1.36-big", "1.37-small", "1.37-ha", "1.37-big"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("combinations = %v, want %v", names, want)
	}
	if got := byName["1.37-big"].Axes; !reflect.DeepEqual(got, map[string]string{"k8s": "1.37", "size": "big"}) {
		t.Errorf("1.37-big axes = %v", got)
	}

	// Picking the parts equals a testbed that names them.
	if got, want := byName["1.37-ha"].Testbed, resolved(t, cat, "ref"); !reflect.DeepEqual(got, want) {
		t.Errorf("1.37-ha differs from the testbed naming the same parts.\ngot:  %+v\nwant: %+v", got, want)
	}
	if got := byName["1.37-ha"].Parts; got[KindWorkloadCluster] != "k8s-1.37" || got[KindHardware] != "ha" {
		t.Errorf("parts = %v", got)
	}

	// A part's fields are merged over the testbed's part, beside a pick on
	// another axis. The patched part is no longer the named one.
	big := byName["1.36-big"]
	if big.Testbed.WorkloadCluster.KubernetesVersion != "v1.36.4" || big.Testbed.Hardware.Machines["worker"].Count != 5 || big.Testbed.Hardware.Machines["controlPlane"].Count != 1 {
		t.Errorf("1.36-big = %+v", big.Testbed)
	}
	if _, named := big.Parts[KindHardware]; named {
		t.Errorf("1.36-big names its patched hardware: %v", big.Parts)
	}
	// What no axis sets is the testbed's.
	if got := byName["1.36-small"].Parts; got[KindHardware] != "small" || got[KindTinkerbell] != "chart-latest" {
		t.Errorf("1.36-small parts = %v", got)
	}
	if m.Resolved == nil || m.Resolved.WorkloadCluster.KubernetesVersion != "v1.36.4" {
		t.Errorf("the testbed without its matrix = %+v", m.Resolved)
	}
}

// An axis value overrides what the testbed itself sets, and a part's fields
// merge over the part a combination uses, whichever axis picks it.
func TestAxisValuesOverrideTheTestbed(t *testing.T) {
	cat := loadOne(t, `package a

parts: hardware: ha: machines: {controlPlane: count: 3, worker: count: 3}

testbeds: t: {
	bootMode: "isoboot"
	hardware: machines: worker: count: 2
	network: family: "ipv6"
	matrix: {
		order: ["boot", "size", "hw"]
		axes: {
			boot: {net: bootMode: "netboot", iso: {}}
			size: {big: hardware: machines: worker: count: 5, same: {}}
			hw: {own: {}, ha: hardware: "ha"}
		}
	}
}
`)
	if errs := cat.AllErrors(); len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	byName := map[string]Combination{}
	for _, c := range cat.Testbeds["t"].Combinations {
		if c.Err != "" {
			t.Errorf("%s: %s", c.Name, c.Err)
		}
		byName[c.Name] = c
	}
	check := func(name, boot string, controlPlane, workers int) {
		t.Helper()
		got := byName[name].Testbed
		if got.Tinkerbell.BootMode != boot || got.Hardware.Machines["controlPlane"].Count != controlPlane || got.Hardware.Machines["worker"].Count != workers || got.Network.Family != "ipv6" {
			t.Errorf("%s: boot %s, machines %+v, network %+v; want boot %s, %d control plane, %d workers, ipv6",
				name, got.Tinkerbell.BootMode, got.Hardware.Machines, got.Network, boot, controlPlane, workers)
		}
	}
	check("iso-same-own", "isoboot", 1, 2)
	check("net-same-own", "netboot", 1, 2)
	check("net-big-own", "netboot", 1, 5)
	check("iso-same-ha", "isoboot", 3, 3)
	check("iso-big-ha", "isoboot", 3, 5)
	if got := byName["iso-same-ha"].Parts[KindHardware]; got != "ha" {
		t.Errorf("iso-same-ha hardware = %q, want ha", got)
	}
}

func TestPartsLine(t *testing.T) {
	parts := map[string]string{KindHardware: "ha", KindWorkloadCluster: "k8s-1.37", KindTinkerbell: "stable"}
	if got, want := PartsLine(parts, []string{KindHardware, KindWorkloadCluster}), "Hardware ha · Workload cluster k8s-1.37"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
	if got := PartsLine(parts, []string{KindClusterAPI}); got != "Cluster API inline" {
		t.Errorf("a part with no name reads %q", got)
	}
}

func TestUnknownPartInAnAxisIsReportedOnce(t *testing.T) {
	src := `package a

testbeds: m: matrix: {
	order: ["a", "b"]
	axes: {
		a: {
			x: workloadCluster: "k8s-1.3"
			y: {}
		}
		b: {p: {}, q: {}, r: {}}
	}
}
`
	var hits []Error
	for _, e := range loadOne(t, src).AllErrors() {
		if strings.HasSuffix(e.Path, "axes.a.x.workloadCluster") {
			hits = append(hits, e)
		}
	}
	if len(hits) != 1 {
		t.Fatalf("reported %d times: %v", len(hits), hits)
	}
	e := hits[0]
	if e.Path != "testbeds.m.matrix.axes.a.x.workloadCluster" || e.Msg != `unknown workload cluster part "k8s-1.3" (did you mean "k8s-1.36"?)` {
		t.Errorf("error = %+v", e)
	}
	if want := strings.Index(src, "x: workloadCluster"); want < 0 || e.Line != 1+strings.Count(src[:want], "\n") {
		t.Errorf("reported at line %d", e.Line)
	}
}

func TestTwoAxesSettingOneField(t *testing.T) {
	cat := loadOne(t, `package a

parts: workloadCluster: "k8s-1.37": kubernetesVersion: "v1.37.0"

testbeds: m: matrix: {
	order: ["a", "b"]
	axes: {
		a: x: {workloadCluster: "k8s-1.36", bootMode: "isoboot"}
		b: {
			y: workloadCluster: "k8s-1.37"
			z: workloadCluster: "k8s-1.36"
			w: bootMode: "netboot"
			v: bootMode: "isoboot"
		}
	}
}
`)
	combinations := cat.Testbeds["m"].Combinations
	if errs := cat.AllErrors(); len(errs) > 0 || len(combinations) != 4 {
		t.Fatalf("errors = %v, combinations = %d, want none and 4", errs, len(combinations))
	}
	for _, c := range combinations {
		switch c.Name {
		case "x-y":
			if !strings.Contains(c.Err, "axes a and b both select a workload cluster part") {
				t.Errorf("x-y err = %q", c.Err)
			}
		case "x-w":
			if !strings.Contains(c.Err, `axes a and b both set bootMode ("isoboot" and "netboot")`) {
				t.Errorf("x-w err = %q", c.Err)
			}
		case "x-z", "x-v":
			if c.Err != "" || c.Testbed.WorkloadCluster.KubernetesVersion != "v1.36.4" || c.Testbed.Tinkerbell.BootMode != "isoboot" {
				t.Errorf("%s: setting one field the same twice: %+v", c.Name, c)
			}
		}
	}
}

// A plan picks combinations only of a testbed that has them.
func TestPlanCombinationsNeedAMatrix(t *testing.T) {
	cat := loadOne(t, "package a\n\nplans: p: {testbed: \"quickstart\", combinations: [\"x-*\"], suites: [\"smoke\"]}\n")
	for _, e := range cat.AllErrors() {
		if e.Path == "plans.p.combinations" {
			if !strings.Contains(e.Msg, `testbed "quickstart" has no matrix`) || !strings.Contains(e.Msg, "plan p") || e.Line != 3 {
				t.Errorf("error = %+v", e)
			}
			return
		}
	}
	t.Fatalf("no error for combinations without a matrix: %v", cat.AllErrors())
}
