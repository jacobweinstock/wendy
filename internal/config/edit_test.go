package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goodParts are a hardware part and a testbed that names it.
func goodParts(spec Spec) {
	spec.Set(KindHardware, "duo", map[string]any{"machines": map[string]any{"worker": map[string]any{"count": int64(2)}}})
	spec.Set(KindTestbed, "mine", map[string]any{"hardware": "duo"})
}

func TestSpecRoundTrip(t *testing.T) {
	dir := t.TempDir()
	spec := Spec{}
	goodParts(spec)
	spec.Set(KindTest, "my-smoke", map[string]any{
		"description": "ours",
		"steps": []any{
			map[string]any{"name": "deploy-app", "container": map[string]any{"image": "ghcr.io/acme/e2e:v1"}},
		},
	})
	if err := WriteSpec(dir, spec); err != nil {
		t.Fatal(err)
	}
	got, err := ReadSpec(filepath.Join(dir, UIFile))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Has(KindTestbed, "mine") || !got.Has(KindHardware, "duo") || !got.Has(KindTest, "my-smoke") {
		t.Fatalf("lost entries: %v", got)
	}
	if err := WriteSpec(dir, got); err != nil {
		t.Fatal(err)
	}
	cat := (&Loader{Dirs: []string{dir}, Builtins: testBuiltins()}).Load()
	if errs := cat.AllErrors(); len(errs) > 0 {
		t.Fatalf("written file does not load: %v", errs)
	}
	tb := cat.Testbeds["mine"]
	if tb == nil || tb.Resolved == nil || tb.Resolved.Hardware.Machines["worker"].Count != 2 {
		t.Fatalf("testbed did not load as written: %+v", tb)
	}
	if cat.Parts(KindHardware)["duo"] == nil || tb.Value.PartNames()[KindHardware] != "duo" {
		t.Error("part or reference did not load as written")
	}
	if tb.Origin == BuiltIn {
		t.Error("origin is the built-in")
	}
}

func TestSpecOfBuiltinsLoads(t *testing.T) {
	cat := (&Loader{Builtins: testBuiltins()}).Load()
	spec := Spec{}
	for _, c := range []struct{ kind, name string }{
		{KindTestbed, "quickstart"},
		{KindWorkloadCluster, "k8s-1.36-kube-router"},
		{KindTinkerbell, "chart-latest"},
		{KindClusterAPI, "capi-1.14-capt-0.7"},
		{KindManagementCluster, "kind"},
		{KindHardware, "small"},
		{KindTest, "scale-out"},
		{KindSuite, "lifecycle"},
	} {
		v, err := SpecOf(c.kind, cat, c.name)
		if err != nil {
			t.Fatal(err)
		}
		spec.Set(c.kind, c.name+"-copy", v)
	}
	errs, err := Check(spec, testBuiltins())
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) > 0 {
		t.Fatalf("a copy of built-ins is rejected: %v", errs)
	}
}

func TestCheckRejects(t *testing.T) {
	spec := Spec{}
	spec.Set(KindClusterAPI, "bad", map[string]any{"core": "1.14", "capt": map[string]any{"version": "v0.7.0"}})
	errs, err := Check(spec, testBuiltins())
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) == 0 || errs[0].Path != "parts.clusterAPI.bad.core" {
		t.Fatalf("errors = %v", errs)
	}

	spec = Spec{}
	spec.Set(KindSuite, "s", map[string]any{"tests": []any{"nope"}})
	spec.Set(KindTestbed, "e", map[string]any{"hardware": "defined-elsewhere"})
	if errs, _ := Check(spec, testBuiltins()); len(errs) > 0 {
		t.Errorf("references are the catalog's to check: %v", errs)
	}
}

func TestPackageNameFollowsTheDir(t *testing.T) {
	dir := writeConfig(t, map[string]string{"acme.cue": "package acme\n"})
	if got := PackageName(dir); got != "acme" {
		t.Fatalf("PackageName = %q", got)
	}
	spec := Spec{}
	spec.Set(KindTestbed, "mine", map[string]any{})
	if err := WriteSpec(dir, spec); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, UIFile))
	if !strings.Contains(string(b), "package acme") {
		t.Fatalf("UI file is in the wrong package:\n%s", b)
	}
}

func TestReadSpecRefusesHandWrittenFile(t *testing.T) {
	dir := writeConfig(t, map[string]string{UIFile: "package local\nfoo: 1\n"})
	if _, err := ReadSpec(filepath.Join(dir, UIFile)); err == nil {
		t.Fatal("a file in another shape was read as a spec")
	}
}
