package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/preview"
	"github.com/tinkerbell/wendy/internal/service"
)

// fixtureConfig adds to the built-ins a testbed with parts of its own, a
// testbed with a four-combination matrix, and a plan that runs it.
const fixtureConfig = `package local

parts: hardware: ha: machines: {controlPlane: {count: 3, cpus: 4}, worker: count: 3}
parts: workloadCluster: next: kubernetesVersion: "v1.37.0"

testbeds: big: {hardware: "ha", workloadCluster: "next"}
testbeds: m: {
	hardware: "ha"
	matrix: {
		order: ["k", "boot"]
		axes: {
			k: {a: workloadCluster: "next", b: {}}
			boot: {net: bootMode: "netboot", iso: bootMode: "isoboot"}
		}
	}
}

plans: nightly: {testbed: "m", suites: ["smoke"], keep: "failed"}
`

// fixtureServer serves fixtureConfig and five executions, one of each kind a
// page shows differently: #1 passed, #2 failed with diagnostics, #3 errored,
// #4 kept its testbed, and #5 is paused on a failure.
func fixtureServer(t *testing.T) (*Server, *service.Service) {
	t.Helper()
	s, svc := editServer(t)
	if err := os.WriteFile(filepath.Join(svc.StarterDir, "fixture.cue"), []byte(fixtureConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	svc.Reload()
	if errs := svc.Catalog().AllErrors(); len(errs) > 0 {
		t.Fatalf("fixture config: %v", errs)
	}

	quickstart := preview.Selection{Plan: "quickstart"}
	nightly := preview.Selection{Plan: "nightly"}
	addExecution(t, svc, "1", quickstart, event.Passed, nil)
	addExecution(t, svc, "2", nightly, event.Passed, func(i int, c *engine.CombinationSummary, _ *preview.Combination, dir string) {
		if i > 0 {
			return
		}
		c.Status = event.Failed
		c.Failure = &engine.Failure{
			Path: []string{"smoke", "nodes"}, Status: event.Failed, Msg: "1 of 3 nodes is not ready", Ms: 4200,
			Diagnostics: map[string]string{"workflow": filepath.Join(c.Dir, "workflow.yaml")},
		}
		if err := os.WriteFile(filepath.Join(dir, "workflow.yaml"), []byte("kind: Workflow\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	addExecution(t, svc, "3", preview.Selection{Testbed: "big", Suites: []string{"smoke"}}, event.Errored, func(_ int, c *engine.CombinationSummary, _ *preview.Combination, _ string) {
		c.Failure = &engine.Failure{Path: []string{"setup", "testbed.network"}, Status: event.Errored, Msg: "docker: connection refused", Ms: 900}
	})
	addExecution(t, svc, "4", quickstart, event.Passed, func(_ int, c *engine.CombinationSummary, pc *preview.Combination, dir string) {
		c.Kept = true
		keepTestbed(t, svc, "4", pc, dir)
	})
	addExecution(t, svc, "5", nightly, event.Running, func(i int, c *engine.CombinationSummary, pc *preview.Combination, dir string) {
		if i > 0 {
			c.Status = event.Waiting
			return
		}
		c.Status, c.Paused = event.Paused, true
		c.Failure = &engine.Failure{
			Path: []string{"smoke", "nodes"}, Status: event.Failed, Msg: "1 of 3 nodes is not ready", Ms: 4200,
			Diagnostics: map[string]string{"console": filepath.Join(c.Dir, "console-worker-1.log")},
		}
		if err := os.WriteFile(filepath.Join(dir, "console-worker-1.log"), []byte("login:\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		keepTestbed(t, svc, "5", pc, dir)
	})
	return s, svc
}

// addExecution writes what an execution of sel leaves on disk, as if it ended
// with status, with each combination given status unless edit changes it.
// Unfinished executions belong to this process, so they read as running.
func addExecution(t *testing.T, svc *service.Service, id string, sel preview.Selection, status event.Status, edit func(i int, c *engine.CombinationSummary, pc *preview.Combination, dir string)) {
	t.Helper()
	p, err := preview.New(svc.Catalog(), sel)
	if err != nil {
		t.Fatal(err)
	}
	dir := svc.ExecutionDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "preview.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	stream, err := event.Create(filepath.Join(dir, "events.jsonl"), id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stream.Close() }()

	n, _ := strconv.Atoi(id)
	started := time.Now().Add(-time.Duration(10-n) * time.Hour)
	sum := &engine.Summary{
		ID: id, Label: p.Label, Key: p.Key, Edited: p.Edited, Selection: p.Selection, Effective: p.Effective,
		Status: status, Started: started, PID: os.Getpid(),
	}
	stream.Emit(event.Event{Type: event.ExecutionStarted, TS: started})
	for i, pc := range p.Combinations {
		c := engine.CombinationSummary{Name: pc.Name, Status: status, Ms: 60_000, Nodes: map[string]event.Status{}, Dir: pc.Name}
		cdir := filepath.Join(dir, c.Dir)
		if err := os.MkdirAll(cdir, 0o755); err != nil {
			t.Fatal(err)
		}
		if edit != nil {
			edit(i, &c, pc, cdir)
		}
		stream.Emit(event.Event{Type: event.CombinationStarted, TS: started, Combination: c.Name, Attrs: map[string]any{"dir": c.Dir}})
		switch {
		case c.Paused:
			stream.Emit(event.Event{Type: event.CombinationPaused, Combination: c.Name, Status: event.Paused})
		case c.Status.Done():
			stream.Emit(event.Event{Type: event.CombinationFinished, Combination: c.Name, Status: c.Status, Ms: c.Ms})
			sum.Status = event.Worst(sum.Status, c.Status)
		}
		sum.Combinations = append(sum.Combinations, c)
	}
	if status.Done() {
		sum.Finished, sum.Ms = started.Add(10*time.Minute), (10 * time.Minute).Milliseconds()
		stream.Emit(event.Event{Type: event.ExecutionFinished, Status: sum.Status, Ms: sum.Ms})
	}
	if err := engine.WriteSummary(dir, sum); err != nil {
		t.Fatal(err)
	}
}

// keepTestbed leaves a combination's testbed in dir, as keeping or pausing
// does, with what setup records of its clusters, machines and network.
func keepTestbed(t *testing.T, svc *service.Service, execution string, pc *preview.Combination, dir string) {
	t.Helper()
	tb, err := engine.NewTestbed(execution, pc.Name, "pt"+execution, dir, pc.Testbed, pc.Labels, svc.Engine.Renderer, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{engine.ManagementCluster, engine.WorkloadCluster} {
		if err := tb.SetKubeconfig(c, []byte("apiVersion: v1\nkind: Config\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := tb.Update(func(s *engine.State) {
		s.Network = engine.Network{Name: "wendy-" + tb.ID, Subnet: "172.20.0.0/24"}
		s.VIPs.Tinkerbell = "172.20.0.100"
		s.Machines = []engine.Machine{{Name: "cp-1", Role: "control-plane", IP: "172.20.0.11"}}
		s.BMC = engine.BMC{Protocol: "redfish", Host: "172.20.0.2:8000"}
	}); err != nil {
		t.Fatal(err)
	}
}

// The fixture is only useful if the service reads it the way the pages do.
func TestFixtureReadsAsWritten(t *testing.T) {
	_, svc := fixtureServer(t)
	want := map[string]event.Status{"1": event.Passed, "2": event.Failed, "3": event.Errored, "4": event.Passed, "5": event.Running}
	got := map[string]event.Status{}
	for _, sum := range svc.Executions() {
		got[sum.ID] = sum.Status
	}
	for id, st := range want {
		if got[id] != st {
			t.Errorf("#%s is %q, want %q", id, got[id], st)
		}
	}
	kept := map[string]bool{}
	for _, k := range svc.KeptTestbeds() {
		kept[k.Execution] = true
		if k.Execution == "5" && !k.Paused {
			t.Error("#5's testbed is not paused")
		}
	}
	if len(kept) != 2 || !kept["4"] || !kept["5"] {
		t.Errorf("kept testbeds are of %v, want #4 and #5", kept)
	}
}
