package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-logr/logr"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/preview"
)

var syscallZero = syscall.Signal(0)

// Decisions a paused combination waits for.
const (
	Retry    = "retry"
	Continue = "continue"
	Abort    = "abort"
)

// Phases are the first element of the event path of a step that is not part
// of a suite. Their paths are [phase, phase, step].
const (
	PhaseBuild       = "build"
	PhaseSetup       = "setup"
	PhaseTeardown    = "teardown"
	PhaseDiagnostics = "diagnostics"
	PhaseAdhoc       = "adhoc"
)

// teardownTimeout bounds teardown, which runs even after a cancel.
const teardownTimeout = 15 * time.Minute

// Engine executes previews.
type Engine struct {
	Data     string
	Work     string
	Renderer *config.Renderer
	// Runners execute exec and container steps, by kind.
	Runners map[string]func(ctx context.Context, s *Step) error
	// Finish runs after an execution's last event, e.g. to write its report.
	Finish func(dir string) error
}

// Execution is one start of a preview: its combinations, each on its own
// testbed.
type Execution struct {
	ID      string
	Dir     string
	Preview *preview.Preview
	Stream  *event.Stream

	engine *Engine
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	builds *Builds

	mu     sync.Mutex
	sum    Summary
	pauses map[string]chan string
}

// Start begins executing p in the background.
func (e *Engine) Start(p *preview.Preview, configDigest string) (*Execution, error) {
	id, dir, err := newExecutionDir(e.Data)
	if err != nil {
		return nil, err
	}
	stream, err := event.Create(filepath.Join(dir, "events.jsonl"), id)
	if err != nil {
		return nil, err
	}
	if b, err := json.MarshalIndent(p, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "preview.json"), b, 0o644)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &Execution{
		ID: id, Dir: dir, Preview: p, Stream: stream, engine: e, ctx: ctx, cancel: cancel,
		done: make(chan struct{}), builds: &Builds{}, pauses: map[string]chan string{},
	}
	r.sum = Summary{
		ID: id, Label: p.Label, Key: p.Key, Edited: p.Edited, Selection: p.Selection, Effective: p.Effective, ConfigDigest: configDigest,
		Status: event.Running, Started: time.Now(), PID: os.Getpid(),
		Steps: StepCounts{Total: p.StepCount() + len(p.Builds)},
	}
	for _, c := range p.Combinations {
		r.sum.Combinations = append(r.sum.Combinations, CombinationSummary{
			Name: c.Name, Status: event.Waiting, Nodes: map[string]event.Status{}, Dir: combinationDirName(c.Name),
		})
	}
	if err := WriteSummary(dir, &r.sum); err != nil {
		return nil, err
	}
	go r.execute()
	return r, nil
}

// Wait blocks until the execution finishes and returns its verdict.
func (r *Execution) Wait() event.Status {
	<-r.done
	return r.Summary().Status
}

// Done is closed when the execution finishes.
func (r *Execution) Done() <-chan struct{} { return r.done }

// Cancel stops the execution. Each combination's testbed is then kept or torn
// down as the preview's Keep says, paused ones included.
func (r *Execution) Cancel() {
	r.cancel()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ch := range r.pauses {
		select {
		case ch <- Abort:
		default:
		}
	}
}

// Decide answers a paused combination: Retry, Continue or Abort. Abort stops
// the combination, whose testbed is then kept or torn down as the preview's
// Keep says. With combination empty, it answers whichever combination is
// paused.
func (r *Execution) Decide(combination, decision string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, ch := range r.pauses {
		if combination == "" || combination == name {
			select {
			case ch <- decision:
				return nil
			default:
				return fmt.Errorf("%s already has a decision", name)
			}
		}
	}
	return fmt.Errorf("nothing in %s is paused", r.ID)
}

// Summary returns a copy of execution.json as it stands.
func (r *Execution) Summary() Summary {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, _ := json.Marshal(r.sum)
	var s Summary
	_ = json.Unmarshal(b, &s)
	return s
}

func (r *Execution) update(fn func(*Summary)) {
	// The file is written under the lock too: combinations update the summary
	// concurrently, and a copy would share their slices and maps.
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(&r.sum)
	_ = WriteSummary(r.Dir, &r.sum)
}

func (r *Execution) combination(name string, fn func(*CombinationSummary)) {
	r.update(func(s *Summary) {
		for i := range s.Combinations {
			if s.Combinations[i].Name == name {
				fn(&s.Combinations[i])
			}
		}
	})
}

func (r *Execution) emit(e event.Event) { r.Stream.Emit(e) }

func (r *Execution) execute() {
	defer close(r.done)
	start := time.Now()
	var combinations []string
	for _, c := range r.Preview.Combinations {
		combinations = append(combinations, c.Name)
	}
	r.emit(event.Event{Type: event.ExecutionStarted, Status: event.Running, Msg: r.Preview.Label, Attrs: map[string]any{
		"label": r.Preview.Label, "key": r.Preview.Key, "combinations": combinations, "steps": r.sum.Steps.Total,
		"parallel": r.Preview.Parallel, "builds": len(r.Preview.Builds),
	}})

	r.build()

	// Slots are taken here, in the order the combinations are listed, and each
	// combination has started before the next one waits for a slot.
	sem := make(chan struct{}, r.Preview.Parallel)
	var wg sync.WaitGroup
	for _, c := range r.Preview.Combinations {
		select {
		case sem <- struct{}{}:
		case <-r.ctx.Done():
		}
		if r.ctx.Err() != nil {
			r.combination(c.Name, func(cs *CombinationSummary) { cs.Status = event.Cancelled })
			continue
		}
		r.combination(c.Name, func(cs *CombinationSummary) { cs.Status = event.Running })
		r.emit(event.Event{Type: event.CombinationStarted, Combination: c.Name, Status: event.Running, Attrs: map[string]any{"labels": c.Labels, "dir": combinationDirName(c.Name)}})
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			r.runCombination(c)
		}()
	}
	wg.Wait()

	verdict := event.Status("")
	for _, b := range r.Summary().Builds {
		verdict = event.Worst(verdict, b.Status)
	}
	for _, c := range r.Summary().Combinations {
		verdict = event.Worst(verdict, c.Status)
	}
	if r.ctx.Err() != nil {
		verdict = event.Worst(verdict, event.Cancelled)
	}
	if verdict == "" {
		verdict = event.Skipped
	}
	ms := time.Since(start).Milliseconds()
	r.update(func(s *Summary) {
		s.Status, s.Finished, s.Ms = verdict, time.Now(), ms
		s.Builds = r.sum.Builds
	})
	sum := r.Summary()
	r.emit(event.Event{Type: event.ExecutionFinished, Status: verdict, Ms: ms, Attrs: map[string]any{
		"passed": countCombinations(sum, event.Passed), "failed": countCombinations(sum, event.Failed),
		"errored": countCombinations(sum, event.Errored) + countCombinations(sum, event.TimedOut),
	}})
	if r.engine.Finish != nil {
		if err := r.engine.Finish(r.Dir); err != nil {
			r.emit(event.Event{Type: event.Log, Level: "error", Msg: "writing the report: " + err.Error()})
		}
	}
	_ = r.Stream.Close()
	r.cancel()
}

func countCombinations(s Summary, st event.Status) int {
	n := 0
	for _, c := range s.Combinations {
		if c.Status == st {
			n++
		}
	}
	return n
}

// build runs every distinct source build once, before any combination.
func (r *Execution) build() {
	for i, b := range r.Preview.Builds {
		key := b.Component
		if b.Source.Ref != "" {
			key += "@" + b.Source.Ref
		}
		st := preview.Step{Key: key, Step: config.Step{
			Uses: "source." + b.Component, Timeout: "60m",
			With: map[string]any{"repo": b.Source.Repo, "ref": b.Source.Ref},
		}}
		dir := filepath.Join(r.Dir, "build", fmt.Sprintf("%02d-%s", i+1, safe(key)))
		res := r.runStep(r.ctx, nil, "", []string{"build", "build", key}, st, dir, stepOpts{})
		var err error
		if res.status != event.Passed {
			err = fmt.Errorf("building %s %s: %s", b.Component, b.Source.Repo, res.msg)
		}
		r.builds.set(b.Component, b.Source, res.outputs, err)
		bs := BuildSummary{Component: b.Component, Repo: b.Source.Repo, Ref: b.Source.Ref, Status: res.status}
		if res.outputs != nil {
			bs.Commit, _ = res.outputs["commit"].(string)
			bs.Version, _ = res.outputs["version"].(string)
			bs.Dirty, _ = res.outputs["dirty"].(bool)
		}
		r.update(func(s *Summary) { s.Builds = append(s.Builds, bs) })
	}
}

// combinationExecution is one combination's part of an execution.
type combinationExecution struct {
	r         *Execution
	c         *preview.Combination
	testbed   *Testbed
	failed    bool
	diagnosed bool
	aborted   bool
	// related and links are what collecting diagnostics found, for the
	// failure card.
	related string
	links   map[string]string
	scIndex int
}

// runCombination runs a combination that has started: its builds' verdict,
// setup, suites, diagnostics and teardown.
func (r *Execution) runCombination(c *preview.Combination) {
	start := time.Now()
	dir := filepath.Join(r.Dir, combinationDirName(c.Name))

	finish := func(st event.Status, kept bool) {
		ms := time.Since(start).Milliseconds()
		cx := CombinationSummary{}
		r.combination(c.Name, func(cs *CombinationSummary) {
			cs.Status, cs.Ms, cs.Kept, cs.Paused = st, ms, kept, false
			cx = *cs
		})
		attrs := map[string]any{"kept": kept, "dir": combinationDirName(c.Name)}
		if cx.Failure != nil {
			attrs["failure"] = cx.Failure
		}
		r.emit(event.Event{Type: event.CombinationFinished, Combination: c.Name, Status: st, Ms: ms, Attrs: attrs})
	}

	buildStatus := event.Status("")
	for _, b := range r.Preview.Builds {
		if !slices.Contains(b.UsedBy, c.Name) {
			continue
		}
		if _, err := r.builds.Get(b.Component, b.Source); err != nil {
			buildStatus = event.Errored
		} else {
			buildStatus = event.Worst(buildStatus, event.Passed)
		}
	}
	if buildStatus != "" {
		r.combination(c.Name, func(cs *CombinationSummary) { cs.Nodes["build"] = buildStatus })
	}
	if buildStatus == event.Errored {
		r.skipAll(c, "a build it needs failed")
		finish(event.Errored, false)
		return
	}

	tb, err := NewTestbed(r.ID, c.Name, shortID(r.ID, c.Name), dir, c.Testbed, c.Labels, r.engine.Renderer, r.builds)
	if err != nil {
		r.emit(event.Event{Type: event.Log, Combination: c.Name, Level: "error", Msg: err.Error()})
		finish(event.Errored, false)
		return
	}
	cx := &combinationExecution{r: r, c: c, testbed: tb}

	verdict := cx.runSteps("setup", "setup", c.Setup, true)
	r.combination(c.Name, func(cs *CombinationSummary) { cs.Nodes["setup"] = verdict })
	if failing(verdict) || verdict == event.Cancelled {
		cx.stop()
	}

	passed := map[string]bool{}
	for _, s := range c.Suites {
		suiteVerdict := event.Status("")
		suiteStart := time.Now()
		for _, sc := range s.Tests {
			var st event.Status
			switch {
			case cx.failed || cx.aborted || r.ctx.Err() != nil:
				st = cx.skipTest(s.Name, sc, "the combination stopped")
			case sc.Skip != "":
				st = cx.skipTest(s.Name, sc, sc.Skip)
			case sc.Once && passed[sc.Name]:
				cx.scIndex++
				st = event.Passed
				r.emit(event.Event{Type: event.StepFinished, Combination: c.Name, Path: []string{s.Name, sc.Name}, Status: st, Msg: "reused", Attrs: map[string]any{"reused": true}})
			default:
				st = cx.runSteps(s.Name, sc.Name, sc.Steps, false)
				if st == event.Passed {
					passed[sc.Name] = true
				}
			}
			suiteVerdict = event.Worst(suiteVerdict, st)
		}
		if suiteVerdict == "" {
			suiteVerdict = event.Skipped
		}
		r.combination(c.Name, func(cs *CombinationSummary) { cs.Nodes[s.Name] = suiteVerdict })
		r.emit(event.Event{Type: event.StepFinished, Combination: c.Name, Path: []string{s.Name}, Status: suiteVerdict, Ms: time.Since(suiteStart).Milliseconds()})
	}

	if c.Testbed.Diagnostics.Collect == "always" && !cx.diagnosed {
		cx.collectDiagnostics()
	}

	status := cx.verdict()
	keep := r.Preview.Keep == "always" || (r.Preview.Keep == "failed" && failing(status))
	if !keep {
		cx.teardown()
	}
	finish(status, keep)
}

func (cx *combinationExecution) stop() { cx.failed = true }

// verdict is the worst of everything the combination ran.
func (cx *combinationExecution) verdict() event.Status {
	s := cx.r.Summary()
	v := event.Status("")
	for _, c := range s.Combinations {
		if c.Name != cx.c.Name {
			continue
		}
		for _, st := range c.Nodes {
			v = event.Worst(v, st)
		}
	}
	if cx.aborted && v != event.Failed && v != event.Errored {
		v = event.Worst(v, event.Cancelled)
	}
	if cx.r.ctx.Err() != nil {
		v = event.Worst(v, event.Cancelled)
	}
	if v == "" {
		v = event.Skipped
	}
	return v
}

func (cx *combinationExecution) skipTest(suite string, sc preview.Test, reason string) event.Status {
	cx.scIndex++
	for _, st := range sc.Steps {
		cx.r.emit(event.Event{Type: event.StepFinished, Combination: cx.c.Name, Path: []string{suite, sc.Name, st.Key}, Status: event.Skipped, Msg: reason})
	}
	cx.r.emit(event.Event{Type: event.StepFinished, Combination: cx.c.Name, Path: []string{suite, sc.Name}, Status: event.Skipped, Msg: reason})
	return event.Skipped
}

func (r *Execution) skipAll(c *preview.Combination, reason string) {
	for _, st := range c.Setup {
		r.emit(event.Event{Type: event.StepFinished, Combination: c.Name, Path: []string{"setup", "setup", st.Key}, Status: event.Skipped, Msg: reason})
	}
	for _, s := range c.Suites {
		for _, sc := range s.Tests {
			r.emit(event.Event{Type: event.StepFinished, Combination: c.Name, Path: []string{s.Name, sc.Name}, Status: event.Skipped, Msg: reason})
		}
	}
}

// runSteps runs a test's steps, or setup's, in order and returns their verdict.
func (cx *combinationExecution) runSteps(suite, name string, steps []preview.Step, setup bool) event.Status {
	r := cx.r
	idx := cx.scIndex
	cx.scIndex++
	path := []string{suite, name}
	start := time.Now()
	r.emit(event.Event{Type: event.StepStarted, Combination: cx.c.Name, Path: path, Status: event.Running})

	verdict := event.Status("")
	stopped := ""
	for i := 0; i < len(steps); i++ {
		st := steps[i]
		stepPath := []string{suite, name, st.Key}
		if stopped != "" {
			r.emit(event.Event{Type: event.StepFinished, Combination: cx.c.Name, Path: stepPath, Status: event.Skipped, Msg: stopped})
			continue
		}
		if st.Skip != "" {
			r.emit(event.Event{Type: event.StepFinished, Combination: cx.c.Name, Path: stepPath, Status: event.Skipped, Msg: st.Skip})
			continue
		}
		dir := filepath.Join(cx.testbed.Dir, "steps", fmt.Sprintf("%02d-%s", idx, safe(name)), fmt.Sprintf("%02d-%s", i+1, safe(st.Key)))
		res := r.runStep(r.ctx, cx.testbed, cx.c.Name, stepPath, st, dir, stepOpts{setup: setup})
		if r.ctx.Err() != nil {
			verdict = event.Worst(verdict, res.status)
			stopped = "cancelled"
			continue
		}
		if !failing(res.status) {
			verdict = event.Worst(verdict, res.status)
			continue
		}
		if st.ContinueOnFailure {
			verdict = event.Worst(verdict, res.status)
			continue
		}
		switch cx.onFailure(stepPath, res, setup) {
		case Retry:
			r.update(func(s *Summary) {
				s.Steps.Done--
				if res.status == event.Failed {
					s.Steps.Failed--
				} else {
					s.Steps.Errored--
				}
			})
			r.combination(cx.c.Name, func(cs *CombinationSummary) { cs.Failure = nil })
			i--
			continue
		case Continue:
			// Later tests run; the suite's verdict still records this.
			verdict = event.Worst(verdict, res.status)
			stopped = "an earlier step failed"
		default:
			verdict = event.Worst(verdict, res.status)
			stopped = "an earlier step failed"
			cx.failed = true
		}
	}
	if verdict == "" {
		verdict = event.Skipped
	}
	r.emit(event.Event{Type: event.StepFinished, Combination: cx.c.Name, Path: path, Status: verdict, Ms: time.Since(start).Milliseconds()})
	return verdict
}

func failing(s event.Status) bool {
	return s == event.Failed || s == event.Errored || s == event.TimedOut
}

// onFailure collects diagnostics once, then pauses if the preview asks to,
// and returns what to do next.
func (cx *combinationExecution) onFailure(path []string, res stepResult, setup bool) string {
	r := cx.r
	if !cx.diagnosed {
		cx.collectDiagnostics()
	}
	f := Failure{Path: path, Status: res.status, Msg: res.msg, Ms: res.ms, Related: cx.related, Diagnostics: cx.links}
	r.combination(cx.c.Name, func(cs *CombinationSummary) { cs.Failure = &f })

	if !r.Preview.PauseOnFailure || r.ctx.Err() != nil {
		return Abort
	}
	ch := make(chan string, 1)
	r.mu.Lock()
	r.pauses[cx.c.Name] = ch
	r.mu.Unlock()
	r.combination(cx.c.Name, func(cs *CombinationSummary) { cs.Paused = true })
	r.update(func(s *Summary) { s.Status = event.Paused })
	r.emit(event.Event{Type: event.CombinationPaused, Combination: cx.c.Name, Path: path, Status: event.Paused, Msg: res.msg, Attrs: map[string]any{
		"failure": f, "kubeconfigs": filepath.Join(cx.testbed.Dir, "kubeconfigs"),
	}})

	var d string
	select {
	case d = <-ch:
	case <-r.ctx.Done():
		d = Abort
	}
	r.mu.Lock()
	delete(r.pauses, cx.c.Name)
	paused := len(r.pauses) > 0
	r.mu.Unlock()
	r.combination(cx.c.Name, func(cs *CombinationSummary) { cs.Paused = false })
	r.update(func(s *Summary) {
		if !paused {
			s.Status = event.Running
		}
	})
	r.emit(event.Event{Type: event.CombinationResumed, Combination: cx.c.Name, Path: path, Status: event.Running, Msg: d})
	switch {
	case d == Continue && setup:
		// Nothing can run without its setup.
		d = Abort
	case d == Abort:
		cx.aborted = true
	}
	return d
}

// diagnosticsSteps are what a combination collects about a failure, before
// teardown: the clusters' state, Tinkerbell's objects, the BMC logs and the
// machines' consoles. Each runs whether or not the ones before it failed.
var diagnosticsSteps = []preview.Step{
	diagnosticsStep("collect.cluster"),
	diagnosticsStep("collect.tinkerbell"),
	diagnosticsStep("collect.bmc"),
	diagnosticsStep("collect.consoles"),
}

func diagnosticsStep(uses string) preview.Step {
	return preview.Step{Key: uses, Step: config.Step{Uses: uses, Timeout: "5m", ContinueOnFailure: true}}
}

// collectDiagnostics runs the diagnostics phase into the combination's
// diagnostics directory, unless its diagnostics say never.
func (cx *combinationExecution) collectDiagnostics() {
	cx.diagnosed = true
	if cx.c.Testbed.Diagnostics.Collect == "never" {
		return
	}
	r := cx.r
	dir := filepath.Join(cx.testbed.Dir, PhaseDiagnostics)
	related := ""
	links := map[string]string{}
	for i, st := range diagnosticsSteps {
		sdir := filepath.Join(dir, ".steps", fmt.Sprintf("%02d-%s", i+1, safe(st.Key)))
		res := r.runStep(r.ctx, cx.testbed, cx.c.Name, []string{PhaseDiagnostics, PhaseDiagnostics, st.Key}, st, sdir, stepOpts{artifacts: dir, uncounted: true})
		if s, ok := res.outputs["related"].(string); ok && s != "" {
			related = s
		}
		if m, ok := res.outputs["diagnostics"].(map[string]any); ok {
			for k, v := range m {
				if p, ok := v.(string); ok {
					if rel, err := filepath.Rel(r.Dir, p); err == nil {
						p = rel
					}
					links[k] = p
				}
			}
		}
	}
	cx.related = related
	cx.links = links
}

func (cx *combinationExecution) teardown() {
	r := cx.r
	ctx, cancel := context.WithTimeout(context.Background(), teardownTimeout)
	defer cancel()
	for _, err := range RunTeardown(ctx, cx.testbed, func(name string, status event.Status, msg string, ms int64) {
		r.emit(event.Event{Type: event.StepFinished, Combination: cx.c.Name, Path: []string{"teardown", "teardown", name}, Status: status, Msg: msg, Ms: ms})
	}) {
		r.update(func(s *Summary) { s.TeardownErrors = append(s.TeardownErrors, cx.c.Name+": "+err.Error()) })
	}
}

// RunTeardown runs a testbed's cleanup stack, reporting each entry. It
// keeps going past errors, which it returns.
func RunTeardown(ctx context.Context, tb *Testbed, report func(name string, status event.Status, msg string, ms int64)) []error {
	var errs []error
	for _, td := range tb.Teardown() {
		start := time.Now()
		err := td.Run(ctx)
		st, msg := event.Passed, ""
		if err != nil {
			st, msg = event.Errored, err.Error()
			errs = append(errs, fmt.Errorf("%s: %w", td.Name, err))
		}
		if report != nil {
			report(td.Name, st, msg, time.Since(start).Milliseconds())
		}
	}
	return errs
}

type stepOpts struct {
	setup     bool
	artifacts string
	uncounted bool
}

type stepResult struct {
	status  event.Status
	msg     string
	ms      int64
	outputs map[string]any
}

// runStep runs one step with its timeout and retries, and reports it.
func (r *Execution) runStep(ctx context.Context, tb *Testbed, combination string, path []string, st preview.Step, dir string, o stepOpts) stepResult {
	artifacts := o.artifacts
	if artifacts == "" {
		artifacts = filepath.Join(dir, "artifacts")
	}
	_ = os.MkdirAll(artifacts, 0o755)
	_ = os.MkdirAll(dir, 0o755)
	logf, err := os.OpenFile(filepath.Join(dir, "step.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	w := io.Discard
	if err == nil {
		defer logf.Close()
		w = logf
	}
	rel, _ := filepath.Rel(r.Dir, dir)
	r.emit(event.Event{Type: event.StepStarted, Combination: combination, Path: path, Status: event.Running, Attrs: map[string]any{
		"kind": st.Kind(), "uses": st.Uses, "dir": rel, "timeout": st.Timeout,
	}})
	logger := newLogger(w, func(e event.Event) { r.emit(e) }, combination, path)

	start := time.Now()
	var res stepResult
	for attempt := 0; ; attempt++ {
		res = r.attempt(ctx, tb, combination, path, st, dir, artifacts, logger)
		if !failing(res.status) || attempt >= st.Retries || ctx.Err() != nil {
			break
		}
		logger.Warn(fmt.Sprintf("attempt %d of %d %s: %s; retrying", attempt+1, st.Retries+1, res.status, res.msg))
	}
	if o.setup && res.status == event.Failed {
		// Setup builds the testbed; nothing in it is under test yet.
		res.status = event.Errored
	}
	res.ms = time.Since(start).Milliseconds()

	if tb != nil && len(res.outputs) > 0 {
		key := strings.Join(path[1:], "/")
		_ = tb.Update(func(s *State) { s.Outputs[key] = res.outputs })
	}
	r.artifacts(combination, path, artifacts)
	r.emit(event.Event{Type: event.StepFinished, Combination: combination, Path: path, Status: res.status, Ms: res.ms, Msg: res.msg})
	if !o.uncounted {
		r.update(func(s *Summary) {
			s.Steps.Done++
			switch res.status {
			case event.Passed:
				s.Steps.Passed++
			case event.Failed:
				s.Steps.Failed++
			case event.Errored, event.TimedOut:
				s.Steps.Errored++
			default:
			}
		})
	}
	return res
}

func (r *Execution) attempt(ctx context.Context, tb *Testbed, combination string, path []string, st preview.Step, dir, artifacts string, logger *slog.Logger) stepResult {
	timeout, err := time.ParseDuration(st.Timeout)
	if err != nil || timeout <= 0 {
		timeout = 15 * time.Minute
	}
	sctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// Libraries that log through the context, such as controller-runtime with
	// the API server's warnings, write to the step's log; see StepFunc.
	sctx = logr.NewContext(sctx, logr.FromSlogHandler(logger.Handler()))

	s := &Step{
		Testbed: tb, Log: logger, Artifacts: artifacts, Dir: dir, Outputs: map[string]any{},
		Data: r.engine.Data, Work: r.engine.Work, Config: st.Step, Builds: r.builds,
		Defer: func(name string, fn func(context.Context) error) {
			if tb != nil {
				tb.Defer(name, fn)
			}
		},
		Case: func(name string, status Status, msg string, ms int64) {
			r.emit(event.Event{Type: event.Case, Combination: combination, Path: append(slices.Clone(path), name), Status: event.Status(status), Msg: msg, Ms: ms})
		},
	}
	err = r.call(sctx, s, st)
	res := stepResult{outputs: s.Outputs}
	var sk *skip
	switch {
	case err == nil:
		res.status = event.Passed
	case errors.As(err, &sk):
		res.status, res.msg = event.Skipped, sk.reason
	case ctx.Err() != nil:
		res.status, res.msg = event.Cancelled, "cancelled"
	case errors.Is(sctx.Err(), context.DeadlineExceeded):
		res.status, res.msg = event.TimedOut, fmt.Sprintf("timed out after %s: %v", timeout, err)
	case IsFail(err):
		res.status, res.msg = event.Failed, err.Error()
	default:
		res.status, res.msg = event.Errored, err.Error()
	}
	return res
}

// call runs the step, turning panics into verdicts: a Gomega failure is a
// product failure, anything else is wendy's.
func (r *Execution) call(ctx context.Context, s *Step, st preview.Step) (err error) {
	defer func() {
		if p := recover(); p != nil {
			if gf, ok := p.(gomegaFailure); ok {
				err = Fail(errors.New(gf.msg))
				return
			}
			err = fmt.Errorf("panic: %v\n%s", p, debug.Stack())
		}
	}()
	switch st.Kind() {
	case "uses":
		b, ok := lookup(st.Uses)
		if !ok {
			return fmt.Errorf("unknown step %q", st.Uses)
		}
		params, _ := json.Marshal(st.With)
		return b.run(ctx, s, params)
	case "exec", "container":
		run, ok := r.engine.Runners[st.Kind()]
		if !ok {
			return fmt.Errorf("no %s runner", st.Kind())
		}
		return run(ctx, s)
	}
	return fmt.Errorf("step %q says nothing to run", st.Key)
}

func (r *Execution) artifacts(combination string, path []string, dir string) {
	// An unreadable entry is skipped rather than ending the walk.
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(r.Dir, p)
			r.emit(event.Event{Type: event.Artifact, Combination: combination, Path: path, Msg: rel})
		}
		return nil
	})
}

// shortID is a short, host-unique name for a combination of an execution.
func shortID(execution, combination string) string {
	sum := sha256.Sum256([]byte(execution + "/" + combination))
	return hex.EncodeToString(sum[:3])
}

func combinationDirName(name string) string { return safe(name) }

func safe(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.', r == '_', r == '@':
			return r
		}
		return '_'
	}, s)
}
