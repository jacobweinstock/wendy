package engine

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/onsi/gomega"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/preview"
)

var (
	torn    []string
	tornMu  sync.Mutex
	flakies atomic.Int32
)

type nameParam struct {
	Name string `json:"name"`
}

func init() {
	Register("t.ok", "", "{}", func(context.Context, *Step, struct{}) error { return nil })
	Register("t.fail", "", "{}", func(context.Context, *Step, struct{}) error { return Fail(errors.New("assertion")) })
	Register("t.err", "", "{}", func(context.Context, *Step, struct{}) error { return errors.New("host broke") })
	Register("t.skip", "", "{}", func(context.Context, *Step, struct{}) error { return Skip("nothing to do") })
	Register("t.slow", "", "{}", func(ctx context.Context, _ *Step, _ struct{}) error {
		<-ctx.Done()
		return ctx.Err()
	})
	Register("t.gomega", "", "{}", func(context.Context, *Step, struct{}) error {
		gomega.Expect(1).To(gomega.Equal(2))
		return nil
	})
	Register("t.flaky", "", "{}", func(context.Context, *Step, struct{}) error {
		if flakies.Add(1) < 3 {
			return Fail(errors.New("not yet"))
		}
		return nil
	})
	Register("t.track", "", "{}", func(_ context.Context, s *Step, p nameParam) error {
		s.Outputs["tracked"] = p.Name
		return s.Testbed.Track(Resource{Kind: "t.res", Name: p.Name})
	})
	RegisterTeardown("t.res", func(_ context.Context, _ *Testbed, r Resource) error {
		tornMu.Lock()
		defer tornMu.Unlock()
		torn = append(torn, r.Name)
		return nil
	})
	Register("collect.cluster", "", "{}", func(_ context.Context, s *Step, _ struct{}) error {
		s.Outputs["diagnostics"] = map[string]any{"dump": filepath.Join(s.Artifacts, "dump")}
		return nil
	})
	for _, uses := range []string{"collect.tinkerbell", "collect.bmc", "collect.consoles"} {
		Register(uses, "", "{}", func(context.Context, *Step, struct{}) error { return nil })
	}
}

func step(uses string, with ...string) preview.Step {
	s := preview.Step{Key: uses, Step: config.Step{Uses: uses, Timeout: "5s"}}
	if len(with) == 1 {
		s.Key = uses + "-" + with[0]
		s.With = map[string]any{"name": with[0]}
	}
	return s
}

func testPreview(setup []preview.Step, tests ...preview.Test) *preview.Preview {
	tb := config.Testbed{Tinkerbell: config.Tinkerbell{Topology: "colocated"}, Diagnostics: config.Diagnostics{Collect: "onFailure"}}
	return &preview.Preview{Label: "t", Key: "plan:t", Parallel: 1, Keep: "never", Combinations: []*preview.Combination{{
		Name: "c", Testbed: tb, Setup: setup,
		Suites: []preview.Suite{{Name: "s", Tests: tests}},
	}}}
}

func run(t *testing.T, p *preview.Preview) (*Execution, []event.Event) {
	t.Helper()
	tornMu.Lock()
	torn = nil
	tornMu.Unlock()
	e := &Engine{Data: t.TempDir()}
	r, err := e.Start(p, "")
	if err != nil {
		t.Fatal(err)
	}
	sub := r.Stream.Subscribe(0)
	var evs []event.Event
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		ev, ok := sub.Next(ctx)
		if !ok {
			break
		}
		evs = append(evs, ev)
		if ev.Type == event.CombinationPaused {
			decide(t, r, ev)
		}
	}
	r.Wait()
	return r, evs
}

// decisions answers pauses in order, for the pause tests.
var decisions []string

func decide(t *testing.T, r *Execution, _ event.Event) {
	t.Helper()
	d := decisions[0]
	decisions = decisions[1:]
	if err := r.Decide("", d); err != nil {
		t.Error(err)
	}
}

func finished(evs []event.Event, path ...string) []event.Status {
	var out []event.Status
	for _, e := range evs {
		if e.Type == event.StepFinished && slices.Equal(e.Path, path) {
			out = append(out, e.Status)
		}
	}
	return out
}

func last(evs []event.Event, path ...string) event.Status {
	s := finished(evs, path...)
	if len(s) == 0 {
		return ""
	}
	return s[len(s)-1]
}

func TestVerdicts(t *testing.T) {
	slow := step("t.slow")
	slow.Timeout = "50ms"
	r, evs := run(t, testPreview(
		[]preview.Step{step("t.track", "net"), step("t.track", "cluster")},
		preview.Test{Name: "a", Steps: []preview.Step{step("t.ok"), step("t.skip")}},
		preview.Test{Name: "b", Steps: []preview.Step{func() preview.Step { s := step("t.err"); s.ContinueOnFailure = true; return s }(), step("t.gomega"), step("t.ok")}},
		preview.Test{Name: "c", Steps: []preview.Step{slow}},
	))
	if got := last(evs, "s", "a", "t.skip"); got != event.Skipped {
		t.Errorf("skip = %s", got)
	}
	if got := last(evs, "s", "b", "t.err"); got != event.Errored {
		t.Errorf("continueOnFailure step = %s", got)
	}
	if got := last(evs, "s", "b", "t.gomega"); got != event.Failed {
		t.Errorf("gomega step = %s", got)
	}
	if got := last(evs, "s", "b", "t.ok"); got != event.Skipped {
		t.Errorf("step after failure = %s, want skipped", got)
	}
	if got := last(evs, "s", "c"); got != event.Skipped {
		t.Errorf("test after failure = %s", got)
	}
	sum := r.Summary()
	if sum.Status != event.Errored || sum.Combinations[0].Status != event.Errored {
		t.Errorf("run = %s combo = %s", sum.Status, sum.Combinations[0].Status)
	}
	if f := sum.Combinations[0].Failure; f == nil || !strings.Contains(f.Msg, "Expected") || f.Status != event.Failed {
		t.Errorf("failure = %+v", f)
	}
	if strings.Join(torn, ",") != "cluster,net" {
		t.Errorf("teardown order = %v", torn)
	}
	if sum.Combinations[0].Kept {
		t.Error("kept with keep: never")
	}
}

func TestCombinationsStartInOrder(t *testing.T) {
	for _, parallel := range []int{1, 2} {
		p := testPreview(nil, preview.Test{Name: "a", Steps: []preview.Step{step("t.ok")}})
		p.Parallel = parallel
		first := *p.Combinations[0]
		p.Combinations = nil
		var want []string
		for i := range 6 {
			c := first
			c.Name = "c" + strconv.Itoa(i)
			p.Combinations = append(p.Combinations, &c)
			want = append(want, c.Name)
		}
		_, evs := run(t, p)
		var got []string
		for _, e := range evs {
			if e.Type == event.CombinationStarted {
				got = append(got, e.Combination)
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("%d at once started %v, want %v", parallel, got, want)
		}
	}
}

func TestTimeout(t *testing.T) {
	slow := step("t.slow")
	slow.Timeout = "50ms"
	_, evs := run(t, testPreview(nil, preview.Test{Name: "a", Steps: []preview.Step{slow}}))
	if got := last(evs, "s", "a", "t.slow"); got != event.TimedOut {
		t.Errorf("slow = %s", got)
	}
}

// A failure collects diagnostics into the combination's diagnostics
// directory before teardown, and its failure card links what they saved.
func TestFailureCollectsDiagnostics(t *testing.T) {
	failing := preview.Test{Name: "a", Steps: []preview.Step{step("t.fail")}}

	r, evs := run(t, testPreview(nil, failing))
	for _, uses := range []string{"collect.cluster", "collect.tinkerbell", "collect.bmc", "collect.consoles"} {
		if got := last(evs, PhaseDiagnostics, PhaseDiagnostics, uses); got != event.Passed {
			t.Errorf("%s = %q", uses, got)
		}
	}
	f := r.Summary().Combinations[0].Failure
	if f == nil {
		t.Fatal("no failure")
	}
	if dump := filepath.ToSlash(f.Diagnostics["dump"]); filepath.IsAbs(dump) || !strings.HasSuffix(dump, "/diagnostics/dump") {
		t.Errorf("dump link = %q, want one relative to the execution, in diagnostics/", dump)
	}

	never := testPreview(nil, failing)
	never.Combinations[0].Testbed.Diagnostics.Collect = "never"
	r, evs = run(t, never)
	if got := finished(evs, PhaseDiagnostics, PhaseDiagnostics, "collect.cluster"); len(got) != 0 {
		t.Errorf("never collected diagnostics: %v", got)
	}
	if f := r.Summary().Combinations[0].Failure; f == nil || len(f.Diagnostics) != 0 {
		t.Errorf("never: failure = %+v", f)
	}

	always := testPreview(nil, preview.Test{Name: "a", Steps: []preview.Step{step("t.ok")}})
	always.Combinations[0].Testbed.Diagnostics.Collect = "always"
	if _, evs = run(t, always); last(evs, PhaseDiagnostics, PhaseDiagnostics, "collect.cluster") != event.Passed {
		t.Error("always did not collect diagnostics from a passing combination")
	}
}

func TestSetupFailureIsErrored(t *testing.T) {
	r, evs := run(t, testPreview([]preview.Step{step("t.fail")}, preview.Test{Name: "a", Steps: []preview.Step{step("t.ok")}}))
	if got := last(evs, "setup", "setup", "t.fail"); got != event.Errored {
		t.Errorf("setup step = %s", got)
	}
	if got := last(evs, "s", "a", "t.ok"); got != event.Skipped {
		t.Errorf("suite step after setup failure = %s", got)
	}
	if got := r.Summary().Combinations[0].Nodes["setup"]; got != event.Errored {
		t.Errorf("setup node = %s", got)
	}
}

func TestRetriesAndOnce(t *testing.T) {
	flakies.Store(0)
	flaky := step("t.flaky")
	flaky.Retries = 2
	p := testPreview(nil,
		preview.Test{Name: "deploy", Once: true, Steps: []preview.Step{flaky}},
	)
	p.Combinations[0].Suites = append(p.Combinations[0].Suites, preview.Suite{Name: "s2", Tests: []preview.Test{{Name: "deploy", Once: true, Steps: []preview.Step{flaky}}}})
	r, evs := run(t, p)
	if got := r.Summary().Status; got != event.Passed {
		t.Fatalf("run = %s", got)
	}
	var reused bool
	for _, e := range evs {
		if v, _ := e.Attrs["reused"].(bool); v && e.Type == event.StepFinished && slices.Equal(e.Path, []string{"s2", "deploy"}) {
			reused = true
		}
	}
	if !reused {
		t.Error("once test was not reused")
	}
	if flakies.Load() != 3 {
		t.Errorf("flaky ran %d times, want 3", flakies.Load())
	}
}

func TestKeepFailed(t *testing.T) {
	p := testPreview([]preview.Step{step("t.track", "net")}, preview.Test{Name: "a", Steps: []preview.Step{step("t.fail")}})
	p.Keep = "failed"
	r, _ := run(t, p)
	if !r.Summary().Combinations[0].Kept || len(torn) != 0 {
		t.Errorf("kept = %v torn = %v", r.Summary().Combinations[0].Kept, torn)
	}
	tb, err := LoadTestbed(r.Dir+"/c", nil)
	if err != nil {
		t.Fatal(err)
	}
	if errs := RunTeardown(context.Background(), tb, nil); len(errs) > 0 || strings.Join(torn, ",") != "net" {
		t.Errorf("later teardown: errs=%v torn=%v", errs, torn)
	}
	if len(tb.State().Resources) != 0 {
		t.Errorf("resources left: %v", tb.State().Resources)
	}
}

func TestPauseRetryContinue(t *testing.T) {
	flakies.Store(1)
	p := testPreview(nil,
		preview.Test{Name: "a", Steps: []preview.Step{step("t.flaky"), step("t.fail"), step("t.ok")}},
		preview.Test{Name: "b", Steps: []preview.Step{step("t.ok")}},
	)
	p.PauseOnFailure = true
	// flaky fails once more, then passes on retry; t.fail is continued past.
	decisions = []string{Retry, Continue}
	r, evs := run(t, p)
	if got := finished(evs, "s", "a", "t.flaky"); !slices.Equal(got, []event.Status{event.Failed, event.Passed}) {
		t.Errorf("flaky = %v", got)
	}
	if got := last(evs, "s", "a", "t.ok"); got != event.Skipped {
		t.Errorf("rest of the failed test = %s", got)
	}
	if got := last(evs, "s", "b", "t.ok"); got != event.Passed {
		t.Errorf("next test after continue = %s", got)
	}
	sum := r.Summary()
	if sum.Status != event.Failed || sum.Steps.Failed != 1 {
		t.Errorf("run = %s steps = %+v", sum.Status, sum.Steps)
	}
}

// Stopping a paused combination skips the rest, and its testbed is then kept
// or torn down as Keep says, as when a step fails with no pause.
func TestPauseStop(t *testing.T) {
	for keep, wantKept := range map[string]bool{"always": true, "failed": true, "never": false} {
		t.Run(keep, func(t *testing.T) {
			p := testPreview([]preview.Step{step("t.track", "net")},
				preview.Test{Name: "a", Steps: []preview.Step{step("t.fail")}},
				preview.Test{Name: "b", Steps: []preview.Step{step("t.ok")}},
			)
			p.PauseOnFailure = true
			p.Keep = keep
			decisions = []string{Abort}
			r, evs := run(t, p)
			if got := last(evs, "s", "b", "t.ok"); got != event.Skipped {
				t.Errorf("after stop = %s", got)
			}
			kept := r.Summary().Combinations[0].Kept
			if kept != wantKept || (strings.Join(torn, ",") == "net") == wantKept {
				t.Errorf("keep %s: kept=%v torn=%v", keep, kept, torn)
			}
		})
	}
}

// Execution IDs are plain numbers, so the next one and the newest-first list
// go by number: 10 comes after 9, not between 1 and 2.
func TestExecutionIDsPastNine(t *testing.T) {
	data := t.TempDir()
	for want := 1; want <= 11; want++ {
		id, dir, err := newExecutionDir(data)
		if err != nil {
			t.Fatal(err)
		}
		if id != strconv.Itoa(want) || dir != filepath.Join(data, ExecutionsDir, id) {
			t.Fatalf("allocated %s at %s, want %d", id, dir, want)
		}
		if err := WriteSummary(dir, &Summary{ID: id, Status: event.Passed}); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for _, s := range ListExecutions(data) {
		got = append(got, s.ID)
	}
	if want := []string{"11", "10", "9", "8", "7", "6", "5", "4", "3", "2", "1"}; !slices.Equal(got, want) {
		t.Errorf("listed %v, want %v", got, want)
	}
}

// A library that logs through the step's context, as controller-runtime
// does with the API server's warnings, writes to that step's log.
func TestStepContextCarriesItsLogger(t *testing.T) {
	Register("t.ctxlog", "", "{}", func(ctx context.Context, _ *Step, _ struct{}) error {
		logr.FromContextOrDiscard(ctx).Info("v1beta1 is deprecated")
		return nil
	})
	_, evs := run(t, testPreview(nil, preview.Test{Name: "a", Steps: []preview.Step{step("t.ctxlog")}}))
	for _, e := range evs {
		if e.Type == event.Log && e.Msg == "v1beta1 is deprecated" && slices.Equal(e.Path, []string{"s", "a", "t.ctxlog"}) {
			return
		}
	}
	t.Error("a message logged through the step's context is not in its log")
}
