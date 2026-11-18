// Package ui holds what the TUI and the web UI share: the execution tree both
// draw, built from an execution's preview and events, and formatting.
package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/preview"
	"github.com/tinkerbell/wendy/internal/vocab"
)

// Node kinds.
const (
	KindBuild       = "build"
	KindCombination = "combination"
	KindSuite       = "suite"
	KindTest        = "test"
	KindStep        = "step"
)

// maxLogs bounds the log lines kept per step in memory; step.log has all.
const maxLogs = 2000

// Node is one row of the execution tree.
type Node struct {
	Key         string
	Name        string
	Kind        string
	Depth       int
	Combination string
	Path        []string
	Status      event.Status
	Ms          int64
	Started     time.Time
	Msg         string
	Reused      bool
	Summary     string
	Dir         string
	Logs        []event.Event
	Cases       []event.Event
	Files       []string
	Children    []*Node
	parent      *Node
}

// Label is the node's display name with any qualifier. The phases read as
// vocab names them; their Name stays the engine's id.
func (n *Node) Label() string {
	switch {
	case n.Reused:
		return n.Name + "  (reused)"
	case n.Kind == KindBuild:
		return vocab.Phase("build")
	case n.Kind == KindSuite && n.Name == "setup":
		return fmt.Sprintf("%s  (%s)", vocab.Phase("setup"), vocab.Count(len(n.Children), "step", "steps"))
	case n.Kind == KindSuite:
		return vocab.Phase(n.Name)
	}
	return n.Name
}

// Parent is the node's parent in the tree, nil at the top.
func (n *Node) Parent() *Node { return n.parent }

// Elapsed is the node's duration, live while it runs.
func (n *Node) Elapsed() int64 {
	if n.Status == event.Running && !n.Started.IsZero() {
		return time.Since(n.Started).Milliseconds()
	}
	return n.Ms
}

// ExecutionView is everything the UIs draw for one execution.
type ExecutionView struct {
	ID       string
	Summary  engine.Summary
	Root     []*Node
	ByKey    map[string]*Node
	LastSeq  int64
	Started  time.Time
	Finished bool
	Status   event.Status
	Total    int
	Counts   map[event.Status]int
	// Paused holds each paused combination's failure.
	Paused map[string]*engine.Failure
	// Running is the most recently started step that has not finished.
	Running string
	Dir     string
}

// Load builds the view of a run from its directory.
func Load(dir string) (*ExecutionView, error) {
	sum, err := engine.ReadSummary(dir)
	if err != nil {
		return nil, err
	}
	v := New(*sum, dir)
	if evs, err := event.ReadFile(filepath.Join(dir, "events.jsonl")); err == nil {
		for _, e := range evs {
			v.Apply(e)
		}
	}
	return v, nil
}

// New is the view of a run before any events, laid out from its preview.
func New(sum engine.Summary, dir string) *ExecutionView {
	v := &ExecutionView{
		ID: sum.ID, Summary: sum, ByKey: map[string]*Node{}, Counts: map[event.Status]int{},
		Paused: map[string]*engine.Failure{}, Dir: dir, Status: sum.Status, Started: sum.Started,
	}
	v.Total = sum.Steps.Total
	var p preview.Preview
	if b, err := os.ReadFile(filepath.Join(dir, "preview.json")); err == nil && json.Unmarshal(b, &p) == nil {
		if len(p.Builds) > 0 {
			b := v.node("", []string{"build"}, KindBuild, "Build")
			for _, bd := range p.Builds {
				k := bd.Component
				if bd.Source.Ref != "" {
					k += "@" + bd.Source.Ref
				}
				v.node("", []string{"build", "build", k}, KindStep, k)
			}
			_ = b
		}
		for _, c := range p.Combinations {
			v.combination(c.Name)
			for _, st := range c.Setup {
				v.node(c.Name, []string{"setup", "setup", st.Key}, KindStep, st.Key)
			}
			for _, s := range c.Suites {
				for _, sc := range s.Tests {
					v.node(c.Name, []string{s.Name, sc.Name}, KindTest, sc.Name)
					for _, st := range sc.Steps {
						v.node(c.Name, []string{s.Name, sc.Name, st.Key}, KindStep, st.Key)
					}
				}
			}
		}
	}
	return v
}

func key(combination string, path []string) string {
	return combination + "\x00" + strings.Join(path, "\x00")
}

func (v *ExecutionView) combination(name string) *Node {
	k := key(name, nil)
	if n, ok := v.ByKey[k]; ok {
		return n
	}
	n := &Node{Key: k, Name: name, Kind: KindCombination, Combination: name, Status: event.Waiting}
	v.ByKey[k] = n
	v.Root = append(v.Root, n)
	return n
}

// node finds or creates the node at path, creating its ancestors.
func (v *ExecutionView) node(combination string, path []string, kind, name string) *Node {
	k := key(combination, path)
	if n, ok := v.ByKey[k]; ok {
		return n
	}
	var parent *Node
	switch {
	case combination == "" && len(path) > 0 && path[0] == "build":
		if len(path) == 1 {
			n := &Node{Key: k, Name: "Build", Kind: KindBuild, Path: path, Status: event.Waiting}
			v.ByKey[k] = n
			v.Root = append([]*Node{n}, v.Root...)
			return n
		}
		parent = v.node("", []string{"build"}, KindBuild, "Build")
	case len(path) == 1:
		parent = v.combination(combination)
	case path[0] == "setup" || path[0] == "teardown" || path[0] == "diagnostics" || path[0] == "adhoc":
		// These have no test level worth showing.
		parent = v.node(combination, path[:1], KindSuite, path[0])
	case len(path) == 2:
		parent = v.node(combination, path[:1], KindSuite, path[0])
	default:
		parent = v.node(combination, path[:2], KindTest, path[1])
	}
	n := &Node{Key: k, Name: name, Kind: kind, Combination: combination, Path: path, Status: event.Waiting, Depth: parent.Depth + 1, parent: parent}
	if kind == KindSuite && combination != "" {
		n.Depth = 1
	}
	v.ByKey[k] = n
	parent.Children = append(parent.Children, n)
	return n
}

func (v *ExecutionView) stepNode(e event.Event) *Node {
	p := e.Path
	if len(p) == 2 && p[0] == p[1] && (p[0] == "setup" || p[0] == "teardown" || p[0] == "diagnostics" || p[0] == "adhoc") {
		// The test-level events of Setup and the like are the node itself.
		return v.node(e.Combination, p[:1], KindSuite, p[0])
	}
	if len(p) == 3 && (p[0] == "setup" || p[0] == "teardown" || p[0] == "diagnostics" || p[0] == "build" || p[0] == "adhoc") {
		// Shown one level up: Setup › step.
		k := key(e.Combination, p)
		if n, ok := v.ByKey[k]; ok {
			return n
		}
		if p[0] == "build" {
			return v.node("", p, KindStep, p[2])
		}
		parent := v.node(e.Combination, p[:1], KindSuite, p[0])
		n := &Node{Key: k, Name: p[2], Kind: KindStep, Combination: e.Combination, Path: p, Status: event.Waiting, Depth: parent.Depth + 1, parent: parent}
		v.ByKey[k] = n
		parent.Children = append(parent.Children, n)
		return n
	}
	kind := KindStep
	switch len(p) {
	case 1:
		return v.node(e.Combination, p, KindSuite, p[0])
	case 2:
		kind = KindTest
	}
	return v.node(e.Combination, p, kind, p[len(p)-1])
}

// Apply updates the view with an event. It is one switch over event types,
// so every way the tree changes reads in one place.
func (v *ExecutionView) Apply(e event.Event) { //nolint:cyclop // See above.
	if e.Seq <= v.LastSeq && e.Seq != 0 {
		return
	}
	v.LastSeq = e.Seq
	switch e.Type {
	case event.ExecutionStarted:
		v.Status, v.Started = event.Running, e.TS
		if t, ok := e.Attrs["steps"].(float64); ok {
			v.Total = int(t)
		}
	case event.ExecutionFinished:
		v.Status, v.Finished = e.Status, true
		v.Running = ""
	case event.CombinationStarted:
		c := v.combination(e.Combination)
		c.Status, c.Started = event.Running, e.TS
		c.Dir, _ = e.Attrs["dir"].(string)
	case event.CombinationFinished:
		c := v.combination(e.Combination)
		c.Status, c.Ms = e.Status, e.Ms
		delete(v.Paused, e.Combination)
		if f := failureOf(e); f != nil {
			c.Msg = f.Msg
		}
	case event.CombinationPaused:
		v.combination(e.Combination).Status = event.Paused
		v.Status = event.Paused
		if f := failureOf(e); f != nil {
			v.Paused[e.Combination] = f
		}
	case event.CombinationResumed:
		v.combination(e.Combination).Status = event.Running
		delete(v.Paused, e.Combination)
		if len(v.Paused) == 0 {
			v.Status = event.Running
		}
	case event.StepStarted:
		n := v.stepNode(e)
		n.Status, n.Started, n.Msg = event.Running, e.TS, ""
		if d, ok := e.Attrs["dir"].(string); ok {
			n.Dir = d
		}
		for p := n.parent; p != nil; p = p.parent {
			if !p.Status.Done() || p.Status == event.Skipped {
				p.Status = event.Running
				if p.Started.IsZero() {
					p.Started = e.TS
				}
			}
		}
		if n.Kind == KindStep {
			v.Running = n.Key
		}
	case event.StepFinished:
		n := v.stepNode(e)
		if n.Kind == KindStep && n.Status == event.Running && e.Status.Done() && e.Path[0] != "teardown" && e.Path[0] != "diagnostics" {
			v.Counts[e.Status]++
		}
		n.Status, n.Ms, n.Msg = e.Status, e.Ms, e.Msg
		n.Reused, _ = e.Attrs["reused"].(bool)
		if v.Running == n.Key {
			v.Running = ""
		}
		if n.Kind == KindStep && n.parent != nil && n.parent.Kind == KindSuite {
			n.parent.Status, n.parent.Ms = aggregate(n.parent)
		}
		if n.Kind == KindStep && n.parent != nil && n.parent.Kind == KindBuild {
			n.parent.Status, n.parent.Ms = aggregate(n.parent)
		}
	case event.Log:
		n := v.stepNode(e)
		n.Logs = append(n.Logs, e)
		if len(n.Logs) > maxLogs {
			n.Logs = n.Logs[len(n.Logs)-maxLogs:]
		}
		if n.Summary == "" && e.Attrs == nil && n.Kind == KindStep && looksLikeSummary(e.Msg) {
			n.Summary = e.Msg
		}
	case event.Case:
		if len(e.Path) > 3 {
			n := v.stepNode(event.Event{Combination: e.Combination, Path: e.Path[:3]})
			n.Cases = append(n.Cases, e)
		}
	case event.Artifact:
		n := v.stepNode(e)
		n.Files = append(n.Files, e.Msg)
	}
}

// looksLikeSummary picks a step's "in human terms" line, such as
// "MachineDeployment md-0 replicas 1 → 2".
func looksLikeSummary(msg string) bool {
	return strings.Contains(msg, " → ")
}

func aggregate(n *Node) (event.Status, int64) {
	var st event.Status
	var ms int64
	running := false
	for _, c := range n.Children {
		ms += c.Ms
		switch c.Status {
		case event.Running:
			running = true
		case event.Waiting:
		default:
			st = event.Worst(st, c.Status)
		}
	}
	if running {
		return event.Running, ms
	}
	if st == "" {
		st = event.Waiting
	}
	return st, ms
}

func failureOf(e event.Event) *engine.Failure {
	raw, ok := e.Attrs["failure"]
	if !ok {
		return nil
	}
	b, _ := json.Marshal(raw)
	var f engine.Failure
	if json.Unmarshal(b, &f) != nil {
		return nil
	}
	return &f
}

// Done is how many steps have a verdict.
func (v *ExecutionView) Done() int {
	n := 0
	for _, c := range v.Counts {
		n += c
	}
	return n
}

// Row is a visible tree row.
type Row struct {
	*Node
	Expanded bool
	HasKids  bool
}

// Rows flattens the tree for drawing, honouring collapsed nodes. Nodes
// collapse by default once they passed, so the tree shows where things are.
func (v *ExecutionView) Rows(expanded map[string]bool) []Row {
	var out []Row
	var walk func(ns []*Node, depth int)
	walk = func(ns []*Node, depth int) {
		for _, n := range ns {
			open, set := expanded[n.Key]
			if !set {
				open = n.Kind == KindCombination || (n.Status != event.Passed && n.Status != event.Skipped && n.Status != event.Waiting) ||
					(n.Kind == KindSuite && n.Name != "setup" && n.Name != "teardown" && n.Name != "diagnostics")
			}
			n.Depth = depth
			out = append(out, Row{Node: n, Expanded: open, HasKids: len(n.Children) > 0})
			if open {
				walk(n.Children, depth+1)
			}
		}
	}
	walk(v.Root, 0)
	return out
}

// Dur formats milliseconds the way the mockups do: 0m02, 14m59, 1h52.
func Dur(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	if d >= time.Hour {
		return fmt.Sprintf("%dh%02d", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dm%02d", int(d.Minutes()), int(d.Seconds())%60)
}

// Ago formats a time relative to now: "2h ago", "yesterday", "22:00 today".
func Ago(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
