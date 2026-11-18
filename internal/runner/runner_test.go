package runner

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/preview"
	"github.com/tinkerbell/wendy/internal/provider/docker"
)

const script = `#!/bin/sh
set -e
[ "$WENDY_CONTRACT" = 1 ]
[ "$WENDY_LABELS" = "colocated,ipv4" ]
[ -f "$WENDY_BUNDLE/state.json" ]
[ -f "$WENDY_BUNDLE/params.json" ]
echo "hello from $1"
cat > "$WENDY_ARTIFACTS_DIR/junit.xml" <<EOF
<testsuites><testsuite name="s">
<testcase name="good" time="0.5"/>
<testcase name="bad"><failure message="nope">details</failure></testcase>
</testsuite></testsuites>
EOF
exit "$2"
`

func execStep(key string, code string) preview.Step {
	return preview.Step{Key: key, Step: config.Step{Timeout: "30s", Exec: &config.Exec{Command: []string{"./t.sh", key, code}}}}
}

func runSteps(t *testing.T, steps ...preview.Step) []event.Event {
	t.Helper()
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, "t.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	p := &preview.Preview{Parallel: 1, Keep: "never", Combinations: []*preview.Combination{{
		Name: "c", Labels: []string{"colocated", "ipv4"},
		Testbed: config.Testbed{Tinkerbell: config.Tinkerbell{Topology: "colocated"}},
		Suites:  []preview.Suite{{Name: "s", Tests: []preview.Test{{Name: "byo", Steps: steps}}}},
	}}}
	e := &engine.Engine{Data: t.TempDir(), Work: work, Runners: Runners()}
	r, err := e.Start(p, "")
	if err != nil {
		t.Fatal(err)
	}
	r.Wait()
	evs, err := event.ReadFile(filepath.Join(r.Dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func status(evs []event.Event, path ...string) event.Status {
	var s event.Status
	for _, e := range evs {
		if (e.Type == event.StepFinished || e.Type == event.Case) && slices.Equal(e.Path, path) {
			s = e.Status
		}
	}
	return s
}

func TestExecContract(t *testing.T) {
	pass := execStep("pass", "0")
	pass.ContinueOnFailure = true
	fail := execStep("fail", "3")
	fail.ContinueOnFailure = true
	evs := runSteps(t, pass, execStep("skip", "77"), fail)

	if got := status(evs, "s", "byo", "pass"); got != event.Passed {
		t.Errorf("exit 0 = %s", got)
	}
	if got := status(evs, "s", "byo", "skip"); got != event.Skipped {
		t.Errorf("exit 77 = %s", got)
	}
	if got := status(evs, "s", "byo", "fail"); got != event.Failed {
		t.Errorf("exit 3 = %s", got)
	}
	if got := status(evs, "s", "byo", "pass", "bad"); got != event.Failed {
		t.Errorf("junit case = %s", got)
	}
	var logged bool
	for _, e := range evs {
		if e.Type == event.Log && e.Msg == "hello from pass" {
			logged = true
		}
	}
	if !logged {
		t.Error("stdout was not logged")
	}
}

func TestContainer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, ok, err := docker.Inspect(ctx, "wendy-no-such-container"); err != nil || ok {
		t.Skipf("docker unavailable: %v", err)
	}
	if err := docker.EnsureImage(ctx, "alpine:3.22", nil); err != nil {
		t.Skipf("no alpine image: %v", err)
	}
	st := preview.Step{Key: "ctr", Step: config.Step{Timeout: "60s", Container: &config.Container{
		Image:   "alpine:3.22",
		Command: []string{"sh", "-c"},
		Args:    []string{`test "$WENDY_BUNDLE" = /wendy/bundle && test -f /wendy/bundle/state.json && echo ok > /wendy/artifacts/out.txt && exit 77`},
	}}}
	evs := runSteps(t, st)
	if got := status(evs, "s", "byo", "ctr"); got != event.Skipped {
		for _, e := range evs {
			t.Logf("%s %v %s %s", e.Type, e.Path, e.Status, e.Msg)
		}
		t.Fatalf("container = %s", got)
	}
	var artifact bool
	for _, e := range evs {
		if e.Type == event.Artifact && filepath.Base(e.Msg) == "out.txt" {
			artifact = true
		}
	}
	if !artifact {
		t.Error("artifact written in the container was not reported")
	}
}
