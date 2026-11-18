package web

import (
	"strings"
	"testing"

	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/preview"
)

func TestHomeWaitingForYou(t *testing.T) {
	s, _ := fixtureServer(t)
	page := shownPage(t, s, homePath)
	for _, want := range []string{
		`<b>Waiting for you:</b> <a href="/executions/5?combination=a-net">#5 nightly</a> paused after a failure. The testbed is kept so you can look around.`,
		"✗ Failed after 0m04: smoke › nodes",
		`<a class="btn primary" href="/executions/5?combination=a-net">Open execution</a>`,
		`hx-post="/api/executions/5/retry?combination=a-net">Retry step</button>`,
		`hx-post="/api/executions/5/continue?combination=a-net" title="Record the failure and run the rest">Continue</button>`,
		`hx-post="/api/executions/5/abort?combination=a-net" hx-confirm="Abort #5 and tear down its testbed?`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Home lacks %s", want)
		}
	}
	// Waiting is not running: #5 is only in its banner.
	if !strings.Contains(page, `<h3>Running</h3><div class="mute">Nothing is running.</div>`) {
		t.Error("an execution waiting for you shows as running")
	}

	quiet := shownPage(t, partsServer(t), homePath)
	if strings.Contains(quiet, `class="banner"`) || strings.Contains(quiet, "Waiting for you") {
		t.Error("Home shows a banner with nothing paused")
	}
}

func TestHomeKeptTestbeds(t *testing.T) {
	s, _ := fixtureServer(t)
	page := shownPage(t, s, homePath)
	for _, want := range []string{
		"<h3>Kept testbeds</h3>",
		"They use this host's memory and disk until you tear them down.",
		`<a href="/executions/4?combination=quickstart"><b class="mono">4/quickstart</b></a>`,
		"quickstart · kept ",
		">Copy export KUBECONFIG</button>",
		`hx-post="/api/executions/4/quickstart/teardown" hx-confirm="Tear down 4/quickstart?`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Home lacks %s", want)
		}
	}
	// The paused testbed is in its banner, not listed again.
	if strings.Contains(page, "<b class=\"mono\">5/a-net</b>") {
		t.Error("a paused testbed is listed with the kept ones")
	}
	if quiet := shownPage(t, partsServer(t), homePath); strings.Contains(quiet, "Kept testbeds") {
		t.Error("Home shows Kept testbeds with none kept")
	}
}

func TestHomePlansAndRunning(t *testing.T) {
	s, svc := fixtureServer(t)
	addExecution(t, svc, "6", preview.Selection{Plan: "quickstart"}, event.Running, nil)
	page := shownPage(t, s, homePath)
	for _, want := range []string{
		`<a class="name" href="/plans/nightly" style="text-decoration:none">nightly</a>`,
		`href="/executions/2" title="Last execution #2, failed">✗ 10m00</a>`,
		`<a class="btn" href="/run/nightly">Change and run…</a>`,
		`<span class="name">Run a plan…</span> <span class="desc">Pick a testbed, suites and options</span>`,
		`<a href="/executions/6"><b>#6</b></a> <span>quickstart</span>`,
		"0/0 steps",
		`<a class="btn sm" href="/executions">All executions</a>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Home lacks %s", want)
		}
	}

	fresh := shownPage(t, partsServer(t), homePath)
	if !strings.Contains(fresh, `<span class="chip skip">Not run yet</span>`) || !strings.Contains(fresh, "No finished executions yet.") {
		t.Error("a plan never run, or a Home with no executions, does not say so")
	}
}
