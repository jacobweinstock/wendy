package ui

import (
	"reflect"
	"slices"
	"testing"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/vocab"
)

func field(t *testing.T, fields []Field, path string) Field {
	t.Helper()
	for _, f := range fields {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("no field %s", path)
	return Field{}
}

// A form names its default once, as the choice that leaves the field unset.
func TestDefaultsAreListedOnce(t *testing.T) {
	for _, tc := range []struct {
		f     Field
		label string
		rest  []string
	}{
		{field(t, TestbedFields, "bootMode"), "Network boot (default)", []string{"isoboot"}},
		{field(t, TestbedFields, "network.mirror.enabled"), "No (default)", []string{"true"}},
		{field(t, PlanDiagnosticsFields, "consoles"), "Yes (default)", []string{"false"}},
		{field(t, TestbedFields, "network.nat64"), "Default (on with IPv6)", []string{"true", "false"}},
		{field(t, StepFields, "uses"), "—", nil},
	} {
		if got := DefaultLabel(tc.f); got != tc.label {
			t.Errorf("%s: DefaultLabel = %q, want %q", tc.f.Path, got, tc.label)
		}
		if got := Choices(tc.f); !slices.Equal(got, tc.rest) {
			t.Errorf("%s: Choices = %v, want %v", tc.f.Path, got, tc.rest)
		}
	}
}

func TestSetUnsetsAndPrunes(t *testing.T) {
	v := map[string]any{}
	f := field(t, TinkerbellFields, "chart.version")
	if err := Set(v, f, "v0.22.0"); err != nil {
		t.Fatal(err)
	}
	if got := Get(v, f); got != "v0.22.0" {
		t.Fatalf("Get = %q", got)
	}
	if err := Set(v, f, "  "); err != nil {
		t.Fatal(err)
	}
	if len(v) != 0 {
		t.Errorf("blank text left %v behind", v)
	}
}

func TestSetKinds(t *testing.T) {
	hw, tb := map[string]any{}, map[string]any{}
	for _, c := range []struct {
		v      map[string]any
		fields []Field
		path   string
		text   string
	}{
		{hw, HardwareFields, "machines.worker.count", "3"},
		{tb, TestbedFields, "network.mirror.upstreams", "ghcr.io, quay.io\n\n gcr.io "},
		{tb, TinkerbellFields, "values", "a:\n  b: 1"},
	} {
		if err := Set(c.v, field(t, c.fields, c.path), c.text); err != nil {
			t.Fatalf("%s: %v", c.path, err)
		}
	}
	if got, _ := lookup(hw, "machines.worker.count"); got != int64(3) {
		t.Errorf("count = %#v, want the integer the schema requires", got)
	}
	if got, _ := lookup(tb, "network.mirror.upstreams"); !reflect.DeepEqual(got, []any{"ghcr.io", "quay.io", "gcr.io"}) {
		t.Errorf("upstreams = %#v", got)
	}
	if got, _ := lookup(tb, "values.a.b"); got != int64(1) {
		t.Errorf("values.a.b = %#v", got)
	}

	for _, c := range []struct {
		fields []Field
		path   string
		bad    string
	}{
		{HardwareFields, "machines.worker.count", "two"},
		{TestbedFields, "bootMode", "pxe"},
		{TinkerbellFields, "values", "- a list"},
	} {
		if err := Set(map[string]any{}, field(t, c.fields, c.path), c.bad); err == nil {
			t.Errorf("%s accepted %q", c.path, c.bad)
		}
	}
}

func TestMissingHonoursRequiredUnless(t *testing.T) {
	v := map[string]any{}
	_ = Set(v, field(t, ClusterAPIFields, "core"), "v1.14.4")
	if got := Missing(v, ClusterAPIFields); len(got) != 1 || got[0] != "CAPT version" {
		t.Fatalf("Missing = %v", got)
	}
	_ = Set(v, field(t, ClusterAPIFields, "capt.source.repo"), "~/src/capt")
	if got := Missing(v, ClusterAPIFields); len(got) != 0 {
		t.Errorf("a source build still needs a version: %v", got)
	}
}

// A form folds Advanced fields away, so a field that must be set can never be
// one, and the machine table (what a hardware part is mostly about) never is.
// A form of one field folds nothing, or it would show nothing.
func TestAdvancedFields(t *testing.T) {
	for _, kind := range append(slices.Clone(config.PartKinds), config.KindTestbed) {
		var advanced int
		for _, f := range PartFields(kind) {
			if !f.Advanced {
				continue
			}
			advanced++
			if f.Required || f.RequiredUnless != "" {
				t.Errorf("%s %s is Advanced but required", kind, f.Path)
			}
			if IsMachineField(f) {
				t.Errorf("%s %s is a machine setting and Advanced", kind, f.Path)
			}
		}
		if n := len(PartFields(kind)); n > 1 && advanced == 0 {
			t.Errorf("%s folds nothing away", kind)
		} else if n == 1 && advanced == 1 {
			t.Errorf("%s folds away its only field", kind)
		}
	}
}

func TestPlanDiagnosticsFieldsRoundTrip(t *testing.T) {
	v := map[string]any{}
	if err := Set(v, field(t, PlanDiagnosticsFields, "diagnostics"), "always"); err != nil {
		t.Fatal(err)
	}
	if err := Set(v, field(t, PlanDiagnosticsFields, "consoles"), "false"); err != nil {
		t.Fatal(err)
	}
	if got := Get(v, field(t, PlanDiagnosticsFields, "diagnostics")); got != "always" {
		t.Errorf("diagnostics = %q", got)
	}
	if got := Get(v, field(t, PlanDiagnosticsFields, "consoles")); got != "false" {
		t.Errorf("consoles = %q", got)
	}
	if err := Set(v, field(t, PlanDiagnosticsFields, "diagnostics"), "sometimes"); err == nil {
		t.Error("accepted a diagnostics setting that does not exist")
	}
}

// TestEveryTestbedFieldHasOneHome holds the settings the single form offered
// before parts, so splitting them into parts and a plan cannot lose or
// duplicate one. Each part's paths are relative to the part; the flat path is
// where the value lands in a resolved testbed, or on the plan.
func TestEveryTestbedFieldHasOneHome(t *testing.T) {
	want := []string{
		"arch",
		"hardware.backend.uri", "hardware.backend.diskPath", "hardware.bmc.image", "hardware.bmc.protocol",
		"hardware.machines.controlPlane.count", "hardware.machines.controlPlane.cpus", "hardware.machines.controlPlane.memMiB", "hardware.machines.controlPlane.diskGiB",
		"hardware.machines.worker.count", "hardware.machines.worker.cpus", "hardware.machines.worker.memMiB", "hardware.machines.worker.diskGiB",
		"hardware.machines.spare.count", "hardware.machines.spare.cpus", "hardware.machines.spare.memMiB", "hardware.machines.spare.diskGiB",
		"consoles", "diagnostics",
		"managementCluster.image",
		"tinkerbell.topology", "tinkerbell.bootMode", "tinkerbell.namespace",
		"tinkerbell.chart.location", "tinkerbell.chart.version", "tinkerbell.source.repo", "tinkerbell.source.ref",
		"tinkerbell.values", "tinkerbell.captainos.flavor", "tinkerbell.captainos.image",
		"network.family", "network.nat64", "network.mirror.enabled", "network.mirror.upstreams",
		"clusterAPI.core", "clusterAPI.capt.version", "clusterAPI.capt.repository", "clusterAPI.capt.source.repo", "clusterAPI.capt.source.ref",
		"workloadCluster.name", "workloadCluster.kubernetesVersion", "workloadCluster.osVersion", "workloadCluster.cni", "workloadCluster.kubeVipVersion",
	}
	// Where a part's path lands in a resolved testbed: under its kind, except
	// the testbed's own settings and hardware's arch.
	flat := func(kind string, f Field) string {
		switch {
		case kind == config.KindTestbed:
			if f.Path == "topology" || f.Path == "bootMode" {
				return "tinkerbell." + f.Path
			}
			return f.Path
		case kind == config.KindHardware && f.Path == "arch":
			return f.Path
		}
		return kind + "." + f.Path
	}
	var got []string
	for _, kind := range append(slices.Clone(config.PartKinds), config.KindTestbed) {
		for _, f := range PartFields(kind) {
			if kind == config.KindTestbed && f.Path == "description" {
				continue
			}
			got = append(got, flat(kind, f))
		}
	}
	for _, f := range PlanDiagnosticsFields {
		got = append(got, f.Path)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("fields by part do not match the testbed's.\ngot:  %v\nwant: %v", got, want)
	}
}

func TestEveryChoiceHasALabel(t *testing.T) {
	lists := [][]Field{PlanDiagnosticsFields, TestFields, SuiteFields, StepFields}
	for _, kind := range append(slices.Clone(config.PartKinds), config.KindTestbed) {
		lists = append(lists, PartFields(kind))
	}
	for _, fields := range lists {
		for _, f := range fields {
			// A step's built-in steps come from the catalog and read as their IDs.
			if f.Kind != FSelect || f.Path == "uses" {
				continue
			}
			for _, o := range append(slices.Clone(f.Options), f.Default) {
				if o != "" && !vocab.HasChoice(f.Path, o) {
					t.Errorf("%s offers %q with no vocab.Choice label", f.Path, o)
				}
			}
		}
	}
	for _, k := range StepKinds {
		if !vocab.HasChoice("step.kind", k) {
			t.Errorf("step kind %q has no vocab.Choice label", k)
		}
	}
}
