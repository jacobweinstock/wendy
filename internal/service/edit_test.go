package service

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
	_ "github.com/tinkerbell/wendy/internal/steps" // registers the built-in steps
)

func editService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	s := New(&config.Loader{Dirs: []string{dir}, Builtins: engine.Builtins()}, nil)
	s.StarterDir = dir
	return s
}

func newTest(timeout string) map[string]any {
	step := map[string]any{"container": map[string]any{"image": "ghcr.io/acme/e2e:v1"}}
	if timeout != "" {
		step["timeout"] = timeout
	}
	return map[string]any{"description": "ours", "steps": []any{step}}
}

func TestSaveEntryLifecycle(t *testing.T) {
	s := editService(t)

	if err := s.SaveEntry(config.KindTest, "", "mine", newTest("")); err != nil {
		t.Fatalf("create: %v", err)
	}
	it := s.Catalog().Tests["mine"]
	if it == nil || it.Value.Description != "ours" {
		t.Fatalf("test not in the catalog as saved: %+v", it)
	}

	if err := s.SaveEntry(config.KindTest, "", "mine", newTest("")); err == nil {
		t.Error("created an entry that exists")
	}
	if err := s.SaveEntry(config.KindTest, "", "deploy", newTest("")); err == nil || !strings.Contains(err.Error(), "already defined") {
		t.Errorf("took a built-in's name: %v", err)
	}

	before, _ := os.ReadFile(s.UIFilePath())
	err := s.SaveEntry(config.KindTest, "mine", "mine", newTest("15 min"))
	var ee *EditError
	if !errors.As(err, &ee) {
		t.Fatalf("a bad timeout was accepted: %v", err)
	}
	if after, _ := os.ReadFile(s.UIFilePath()); string(after) != string(before) {
		t.Error("a rejected change was written")
	}

	if err := s.SaveEntry(config.KindTest, "mine", "renamed", newTest("")); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, ok := s.Catalog().Tests["mine"]; ok {
		t.Error("the old name is still in the catalog")
	}
}

// Parts and testbeds are edited like tests and suites: a testbed names parts,
// and what uses a part blocks deleting or renaming it.
func TestPartsAndTestbedsLifecycle(t *testing.T) {
	s := editService(t)
	ha := map[string]any{"machines": map[string]any{"worker": map[string]any{"count": int64(3)}}}
	if err := s.SaveEntry(config.KindHardware, "", "ha", ha); err != nil {
		t.Fatalf("hardware: %v", err)
	}
	next := map[string]any{"kubernetesVersion": "v1.37.0"}
	if err := s.SaveEntry(config.KindWorkloadCluster, "", "next", next); err != nil {
		t.Fatalf("workload cluster: %v", err)
	}
	if err := s.SaveEntry(config.KindTestbed, "", "big", map[string]any{"hardware": "ha", "workloadCluster": "next"}); err != nil {
		t.Fatalf("testbed: %v", err)
	}
	tb := s.Catalog().Testbeds["big"]
	if tb == nil || tb.Resolved == nil || tb.Resolved.Hardware.Machines["worker"].Count != 3 || tb.Resolved.WorkloadCluster.KubernetesVersion != "v1.37.0" {
		t.Fatalf("testbed did not resolve from its parts: %+v", tb)
	}

	if got := s.UsedBy(config.KindHardware, "ha"); len(got) != 1 || got[0] != "testbed big" {
		t.Errorf("ha is used by %v", got)
	}
	for _, name := range []string{"ha", "next"} {
		kind := config.KindHardware
		if name == "next" {
			kind = config.KindWorkloadCluster
		}
		if err := s.DeleteEntry(kind, name); err == nil || !strings.Contains(err.Error(), "testbed big") {
			t.Errorf("deleted %s while a testbed names it: %v", name, err)
		}
	}
	if err := s.SaveEntry(config.KindHardware, "ha", "ha2", ha); err == nil || !strings.Contains(err.Error(), "testbed big") {
		t.Errorf("renamed a part a testbed names: %v", err)
	}

	if err := s.DeleteEntry(config.KindTestbed, "big"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteEntry(config.KindHardware, "ha"); err != nil {
		t.Errorf("deleting an unused part: %v", err)
	}
}

func TestTestbedWithAnUnknownPartIsRejected(t *testing.T) {
	s := editService(t)
	if err := s.SaveEntry(config.KindHardware, "", "tiny", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.UIFilePath())
	err := s.SaveEntry(config.KindTestbed, "", "x", map[string]any{"hardware": "tiyn"})
	var ee *EditError
	if !errors.As(err, &ee) || !strings.Contains(err.Error(), `unknown hardware part "tiyn" (did you mean "tiny"?)`) {
		t.Fatalf("err = %v", err)
	}
	if after, _ := os.ReadFile(s.UIFilePath()); string(after) != string(before) {
		t.Error("a rejected testbed was written")
	}
}

// A part may be named in the same change that creates it, or live in a file
// the UI does not own.
func TestTestbedMayNameBuiltinParts(t *testing.T) {
	s := editService(t)
	if err := s.SaveEntry(config.KindTestbed, "", "x", map[string]any{"workloadCluster": "k8s-1.36-kube-router"}); err != nil {
		t.Fatal(err)
	}
	if tb := s.Catalog().Testbeds["x"]; tb == nil || tb.Resolved == nil || tb.Resolved.WorkloadCluster.CNI != "kube-router" {
		t.Errorf("testbed x = %+v, want the built-in k8s-1.36-kube-router workload cluster's CNI", tb)
	}
}

// A part an axis picks is used by the axis's testbed, and a testbed by the
// plans that name it.
func TestPartsAreUsedByTestbedsAndTestbedsByPlans(t *testing.T) {
	s := editService(t)
	hand := "package local\n\n" +
		"testbeds: m: matrix: {order: [\"k\"], axes: k: a: workloadCluster: \"next\"}\n" +
		"plans: p: {testbed: \"big\", suites: [\"smoke\"]}\n"
	if err := os.WriteFile(filepath.Join(s.StarterDir, "hand.cue"), []byte(hand), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveEntry(config.KindWorkloadCluster, "", "next", map[string]any{"kubernetesVersion": "v1.37.0"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveEntry(config.KindTestbed, "", "big", map[string]any{"workloadCluster": "next"}); err != nil {
		t.Fatal(err)
	}
	if got := s.UsedBy(config.KindWorkloadCluster, "next"); !slices.Equal(got, []string{"testbed big", "testbed m"}) {
		t.Errorf("next is used by %v", got)
	}
	if got := s.UsedBy(config.KindTestbed, "big"); !slices.Equal(got, []string{"plan p"}) {
		t.Errorf("big is used by %v", got)
	}
	if got := s.UsedBy(config.KindTestbed, "quickstart"); !slices.Equal(got, []string{"plan quickstart"}) {
		t.Errorf("quickstart is used by %v", got)
	}
}

func TestPartsNeedTheirRequiredFields(t *testing.T) {
	s := editService(t)
	for kind, bad := range map[string]map[string]any{
		config.KindClusterAPI:      {"capt": map[string]any{"version": "v0.7.0"}}, // no core
		config.KindWorkloadCluster: {"cni": "cilium"},                             // no Kubernetes version
	} {
		if err := s.SaveEntry(kind, "", "bad", bad); err == nil || !strings.Contains(err.Error(), "required") {
			t.Errorf("%s: %v", kind, err)
		}
	}
}

func TestCopiesOfBuiltinPartsAndTestbedsLoad(t *testing.T) {
	s := editService(t)
	for _, c := range []struct{ kind, name string }{
		{config.KindWorkloadCluster, "k8s-1.36-kube-router"},
		{config.KindHardware, "small"},
		{config.KindTinkerbell, "chart-latest"},
		{config.KindClusterAPI, "capi-1.14-capt-0.7"},
		{config.KindTestbed, "quickstart"},
	} {
		e, err := s.Entry(c.kind, c.name)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.SaveEntry(c.kind, "", c.name+"-copy", e.Value); err != nil {
			t.Errorf("copy of %s %s: %v", c.kind, c.name, err)
		}
	}
	if errs := s.Catalog().AllErrors(); len(errs) > 0 {
		t.Errorf("errors after the copies: %v", errs)
	}
}

func TestSuiteAndTestReferences(t *testing.T) {
	s := editService(t)
	sc := map[string]any{"steps": []any{map[string]any{"container": map[string]any{"image": "ghcr.io/acme/e2e:v1"}}}}
	if err := s.SaveEntry(config.KindTest, "", "acme", sc); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveEntry(config.KindSuite, "", "bad", map[string]any{"tests": []any{"acmee"}}); err == nil || !strings.Contains(err.Error(), `did you mean "acme"`) {
		t.Fatalf("unknown test: %v", err)
	}
	if err := s.SaveEntry(config.KindSuite, "", "ours", map[string]any{"tests": []any{"deploy", "acme"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteEntry(config.KindTest, "acme"); err == nil || !strings.Contains(err.Error(), "suite ours") {
		t.Errorf("deleted a test a suite uses: %v", err)
	}
	if err := s.DeleteEntry(config.KindSuite, "ours"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteEntry(config.KindTest, "acme"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.UIFilePath()); !os.IsNotExist(err) {
		t.Errorf("the UI file outlived its last entry: %v", err)
	}
}

func TestStepWithoutWhatToRunIsRejected(t *testing.T) {
	s := editService(t)
	sc := map[string]any{"steps": []any{map[string]any{"name": "nothing"}}}
	if err := s.SaveEntry(config.KindTest, "", "empty", sc); err == nil {
		t.Fatal("a step that runs nothing was saved")
	}
}

// The user's own steps are entries like the rest: named, used by tests, and
// kept while a test uses them.
func TestYourStepsLifecycle(t *testing.T) {
	s := editService(t)
	wait := map[string]any{"exec": map[string]any{"command": []any{"kubectl", "wait"}}}
	if err := s.SaveEntry(config.KindStep, "", "wait", wait); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.SaveEntry(config.KindStep, "", "assert.nodes", wait); err == nil || !strings.Contains(err.Error(), "already defined in built-in") {
		t.Errorf("took a built-in step's ID: %v", err)
	}
	usesWait := map[string]any{"uses": "wait"}
	if err := s.SaveEntry(config.KindStep, "", "again", usesWait); err == nil || !strings.Contains(err.Error(), "can use only those") {
		t.Errorf("a step of yours used another: %v", err)
	}
	if err := s.SaveEntry(config.KindTest, "", "bad", map[string]any{"steps": []any{map[string]any{"uses": "wiat"}}}); err == nil || !strings.Contains(err.Error(), `unknown step "wiat" (did you mean "wait"?)`) {
		t.Errorf("a test used an unknown step: %v", err)
	}
	if err := s.SaveEntry(config.KindTest, "", "t", map[string]any{"steps": []any{usesWait}}); err != nil {
		t.Fatalf("a test using wait: %v", err)
	}

	if got := s.UsedBy(config.KindStep, "wait"); !slices.Equal(got, []string{"test t"}) {
		t.Errorf("wait is used by %v", got)
	}
	if err := s.DeleteEntry(config.KindStep, "wait"); err == nil || !strings.Contains(err.Error(), "used by test t") {
		t.Errorf("deleted a used step: %v", err)
	}
	if err := s.SaveEntry(config.KindStep, "wait", "settle", wait); err == nil || !strings.Contains(err.Error(), "used by test t") {
		t.Errorf("renamed a used step: %v", err)
	}
	if err := s.DeleteEntry(config.KindTest, "t"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteEntry(config.KindStep, "wait"); err != nil {
		t.Errorf("deleting an unused step: %v", err)
	}
}

// Plans are entries like the rest, checked against the testbeds and suites
// they name before anything is written.
func TestPlansLifecycle(t *testing.T) {
	s := editService(t)
	plan := func(testbed string, suites []any, combinations ...any) map[string]any {
		v := map[string]any{"testbed": testbed, "suites": suites}
		if len(combinations) > 0 {
			v["combinations"] = combinations
		}
		return v
	}
	if err := s.SaveEntry(config.KindPlan, "", "nightly", plan("quickstart", []any{"lifecycle"})); err != nil {
		t.Fatalf("create: %v", err)
	}
	if p := s.Catalog().Plans["nightly"]; p == nil || p.Value.Testbed != "quickstart" || !slices.Equal(p.Value.Suites, []string{"lifecycle"}) {
		t.Fatalf("plan = %+v", p)
	}

	before, _ := os.ReadFile(s.UIFilePath())
	for _, c := range []struct {
		v    map[string]any
		want string
	}{
		{plan("quickstrat", []any{"smoke"}), `unknown testbed "quickstrat" (did you mean "quickstart"?)`},
		{plan("quickstart", []any{"smok"}), `unknown suite "smok" (did you mean "smoke"?)`},
		{plan("quickstart", []any{"smoke"}, "a-*"), `testbed "quickstart" has no matrix`},
		{plan("quickstart", []any{}), "at least one suite"},
	} {
		if err := s.SaveEntry(config.KindPlan, "", "bad", c.v); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: err = %v, want %q", c.v, err, c.want)
		}
	}
	if after, _ := os.ReadFile(s.UIFilePath()); string(after) != string(before) {
		t.Error("a rejected plan was written")
	}

	e, err := s.Entry(config.KindPlan, "quickstart")
	if err != nil || e.Editable {
		t.Fatalf("the built-in plan: %+v, %v", e, err)
	}
	if err := s.SaveEntry(config.KindPlan, "", "quickstart-copy", e.Value); err != nil {
		t.Errorf("copying the built-in plan: %v", err)
	}
	if err := s.SaveEntry(config.KindPlan, "nightly", "weekly", plan("quickstart", []any{"lifecycle"})); err != nil {
		t.Errorf("rename: %v", err)
	}
	if err := s.DeleteEntry(config.KindPlan, "weekly"); err != nil {
		t.Errorf("delete: %v", err)
	}
	if _, ok := s.Catalog().Plans["weekly"]; ok {
		t.Error("a deleted plan is still there")
	}
}

// What the UI file writes may name entries a hand-written file defines.
func TestEntriesMayNameWhatOtherFilesDefine(t *testing.T) {
	s := editService(t)
	hand := "package local\n\nsteps: settle: exec: command: [\"true\"]\ntestbeds: lab: {}\n"
	if err := os.WriteFile(filepath.Join(s.StarterDir, "lab.cue"), []byte(hand), 0o644); err != nil {
		t.Fatal(err)
	}
	s.Reload()
	if err := s.SaveEntry(config.KindTest, "", "t", map[string]any{"steps": []any{map[string]any{"uses": "settle"}}}); err != nil {
		t.Errorf("a test using a step from another file: %v", err)
	}
	if err := s.SaveEntry(config.KindPlan, "", "p", map[string]any{"testbed": "lab", "suites": []any{"smoke"}}); err != nil {
		t.Errorf("a plan naming a testbed from another file: %v", err)
	}
}
