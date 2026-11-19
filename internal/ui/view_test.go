package ui

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/event"
)

// Until the summary has a verdict, a combination's progress comes from its
// events.
func TestLiveCombination(t *testing.T) {
	v := New(engine.Summary{ID: "1"}, t.TempDir())
	started := time.Now().Add(-2 * time.Minute)
	apply := func(typ string, st event.Status, path ...string) {
		v.Apply(event.Event{Type: typ, Combination: "c", Status: st, Path: path, TS: started})
	}
	check := func(what string, got, want event.Status) {
		t.Helper()
		if got != want {
			t.Errorf("%s = %q, want %q", what, got, want)
		}
	}
	c := engine.CombinationSummary{Name: "c", Status: event.Running, Nodes: map[string]event.Status{}}
	apply(event.CombinationStarted, event.Running)
	apply(event.StepStarted, event.Running, "setup", "setup", "a")
	apply(event.StepFinished, event.Passed, "setup", "setup", "a")

	check("setup between two steps", v.Phase(c, "setup"), event.Running)
	check("a suite not started", v.Phase(c, "smoke"), event.Waiting)
	check("the build of a combination that started without one", v.Phase(c, "build"), event.Waiting)
	if n := v.Now(c); n != nil {
		t.Errorf("between two steps, Now = %s", n.Name)
	}
	if ms := v.Elapsed(c); ms < (2 * time.Minute).Milliseconds() {
		t.Errorf("Elapsed = %d, want at least two minutes", ms)
	}

	apply(event.StepStarted, event.Running, "setup", "setup", "b")
	if n := v.Now(c); n == nil || n.Name != "b" {
		t.Errorf("Now = %v, want b", n)
	}

	c.Nodes["setup"] = event.Passed
	apply(event.StepStarted, event.Running, "smoke", "deploy", "x")
	apply(event.StepFinished, event.Failed, "smoke", "deploy", "x")
	c.Paused, c.Failure = true, &engine.Failure{Path: []string{"smoke", "deploy", "x"}, Status: event.Failed}
	apply(event.CombinationPaused, event.Paused)
	check("setup with a verdict", v.Phase(c, "setup"), event.Passed)
	check("a suite paused at a failure", v.Phase(c, "smoke"), event.Failed)
	check("a paused combination", CombinationStatus(c), event.Paused)
	if n := v.Now(c); n == nil || n.Name != "x" {
		t.Errorf("paused, Now = %v, want x", n)
	}

	c.Status, c.Paused, c.Ms = event.Failed, false, 1000
	check("a suite that never got a verdict", v.Phase(c, "smoke"), event.Skipped)
	if n := v.Now(c); n != nil {
		t.Errorf("done, Now = %s", n.Name)
	}
	if ms := v.Elapsed(c); ms != 1000 {
		t.Errorf("done, Elapsed = %d, want the summary's 1000", ms)
	}
	if ms := v.Elapsed(engine.CombinationSummary{Name: "w", Status: event.Waiting}); ms != 0 {
		t.Errorf("waiting, Elapsed = %d", ms)
	}
}

// A combination's tree is the shared build and that combination's rows, all
// of them open, whatever they passed.
func TestTreeIsOneCombinationWhole(t *testing.T) {
	v := New(engine.Summary{ID: "1"}, t.TempDir())
	for _, e := range []event.Event{
		{Type: event.StepStarted, Path: []string{"build", "build", "tinkerbell"}},
		{Type: event.StepFinished, Status: event.Passed, Path: []string{"build", "build", "tinkerbell"}},
		{Type: event.StepStarted, Combination: "a", Path: []string{"setup", "setup", "x"}},
		{Type: event.StepFinished, Combination: "a", Status: event.Passed, Path: []string{"setup", "setup", "x"}},
		{Type: event.StepStarted, Combination: "a", Path: []string{"smoke", "deploy", "y"}},
		{Type: event.StepFinished, Combination: "a", Status: event.Passed, Path: []string{"smoke", "deploy", "y"}},
		{Type: event.StepStarted, Combination: "b", Path: []string{"smoke", "deploy", "z"}},
	} {
		v.Apply(e)
	}
	var got []string
	for _, r := range v.Tree("a") {
		if !r.Expanded {
			t.Errorf("%s is folded", r.Name)
		}
		got = append(got, fmt.Sprintf("%d %s", r.Depth, r.Name))
	}
	want := []string{"0 Build", "1 tinkerbell", "0 setup", "1 x", "0 smoke", "1 deploy", "2 y"}
	if !slices.Equal(got, want) {
		t.Errorf("Tree(a) = %q, want %q", got, want)
	}
}

func TestNodeLabelNamesPhases(t *testing.T) {
	steps := []*Node{{Name: "management.create"}, {Name: "bmc.start"}}
	for _, c := range []struct {
		node *Node
		want string
	}{
		{&Node{Kind: KindBuild, Name: "Build"}, "Build images"},
		{&Node{Kind: KindSuite, Name: "setup", Children: steps}, "Set up testbed  (2 steps)"},
		{&Node{Kind: KindSuite, Name: "diagnostics"}, "Collect diagnostics"},
		{&Node{Kind: KindSuite, Name: "teardown"}, "Tear down"},
		{&Node{Kind: KindSuite, Name: "smoke"}, "smoke"},
		{&Node{Kind: KindTest, Name: "deploy", Reused: true}, "deploy  (reused)"},
	} {
		if got := c.node.Label(); got != c.want {
			t.Errorf("%s %q: Label() = %q, want %q", c.node.Kind, c.node.Name, got, c.want)
		}
	}
}
