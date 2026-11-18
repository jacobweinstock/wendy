package web

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/preview"
)

var listedExecution = regexp.MustCompile(`<td><a href="/executions/(\d+)">#`)

// listedExecutions are the numbers of the executions a list shows, sorted.
func listedExecutions(page string) []string {
	var out []string
	for _, m := range listedExecution.FindAllStringSubmatch(page, -1) {
		out = append(out, m[1])
	}
	slices.Sort(out)
	return out
}

func TestExecutionsFilters(t *testing.T) {
	s, _ := fixtureServer(t)
	for query, want := range map[string][]string{
		"":                {"1", "2", "3", "4", "5"},
		"?status=running": nil,
		"?status=paused":  {"5"},
		"?status=passed":  {"1", "4"},
		"?status=failed":  {"2"},
		"?status=errored": {"3"},
		// A paused execution's testbed is up too.
		"?status=kept": {"4", "5"},
		// A number matches with or without its #.
		"?q=2":    {"2"},
		"?q=%232": {"2"},
		// Plans and testbeds match by name, including a plan's testbed.
		"?q=nightly":             {"2", "5"},
		"?q=big":                 {"3"},
		"?q=m":                   {"2", "5"},
		"?q=nightly&status=kept": {"5"},
	} {
		code, page := get(t, s, executionsPath+query)
		if code != http.StatusOK {
			t.Errorf("GET %s = %d", query, code)
			continue
		}
		if got := listedExecutions(page); !slices.Equal(got, want) {
			t.Errorf("%s lists %v, want %v", query, got, want)
		}
	}
	_, page := get(t, s, executionsPath+"?status=kept")
	if !strings.Contains(page, `<a class="on" href="/executions?status=kept" aria-current="true">Kept testbeds</a>`) {
		t.Error("the Kept testbeds segment is not marked current")
	}
}

func TestExecutionsRows(t *testing.T) {
	s, svc := fixtureServer(t)
	addExecution(t, svc, "6", preview.Selection{Plan: "nightly", Keep: "never"}, event.Passed, nil)
	for id, wants := range map[string][]string{
		// A plan's execution links the plan and its testbed, and counts its combinations.
		"2": {`<a href="/plans/nightly">nightly</a>`, `<a href="/testbeds/testbeds/m">m</a>`, `title="4 combinations">× 4</span>`, ">Run again</button>", ">Delete</button>"},
		// Tests picked in Run tests have no plan.
		"3": {`title="Run from a form, not saved as a plan">Unsaved plan</span>`, `<a href="/testbeds/testbeds/big">big</a>`},
		"4": {`href="/executions/4" title="The testbed is still up">kept</a>`},
		"5": {`hx-confirm="Cancel #5? Its testbeds are torn down unless kept.">Cancel execution</button>`, "‖ paused"},
		// Run tests changed the plan's keep setting.
		"6": {`<a href="/plans/nightly">nightly</a>`, ">(changed)</span>"},
	} {
		_, page := get(t, s, executionsPath+"?q=%23"+id)
		if got := listedExecutions(page); !slices.Equal(got, []string{id}) {
			t.Fatalf("#%s: the filter lists %v", id, got)
		}
		for _, want := range wants {
			if !strings.Contains(page, want) {
				t.Errorf("#%s's row lacks %s", id, want)
			}
		}
	}
}

func TestExecutionsEmptyStates(t *testing.T) {
	s, _ := fixtureServer(t)
	_, page := get(t, s, executionsPath+"?status=running")
	if !strings.Contains(page, "<p>No executions match.</p>") || !strings.Contains(page, `href="/executions">Show all executions</a>`) {
		t.Error("a filter that hides every execution does not say so, or offers no way back")
	}

	none, _ := editServer(t)
	_, page = get(t, none, executionsPath)
	if !strings.Contains(page, "<p>No executions yet.</p>") || !strings.Contains(page, `href="/run">Run a plan…</a>`) {
		t.Error("with no executions, the list does not say so, or offers no way to start one")
	}
}

// shownPage is a page with its whitespace collapsed and its entities decoded,
// as a browser shows it.
func shownPage(t *testing.T, s *Server, path string) string {
	t.Helper()
	code, page := get(t, s, path)
	if code != http.StatusOK {
		t.Fatalf("GET %s = %d", path, code)
	}
	return html.UnescapeString(strings.Join(strings.Fields(page), " "))
}

func TestExecutionSummaryAndFailureCards(t *testing.T) {
	s, _ := fixtureServer(t)
	for path, wants := range map[string][]string{
		"/executions/2": {
			`<a href="/executions">Executions</a> › <b>#2 nightly</b>`,
			`plan <a href="/plans/nightly">nightly</a> · testbed <a href="/testbeds/testbeds/m">m</a> · 4 combinations`,
			">Run failed again (1)</button>", ">Run again</button>",
			`<div class="mute">combination a-net</div>`, `<span class="mute">Diagnostics:</span>`,
			"means Wendy or the host failed, not Tinkerbell or CAPT.",
		},
		"/executions/3": {
			"testbed <a href=\"/testbeds/testbeds/big\">big</a>",
			"Errored means Wendy or the host failed, not Tinkerbell or CAPT. Running it again usually helps.",
		},
		// Paused on a failure: the chip says why, and the card offers what to do next.
		"/executions/5": {
			`<span class="chip pause">‖ paused after a failure</span>`,
			`hx-confirm="Cancel #5? Its testbeds are torn down unless kept.">Cancel execution</button>`,
			`<span class="mute">Collected so far:</span>`, ">Retry step</button>",
			`Continue <span class="mute">(record the failure, run the rest)</span>`,
			`hx-confirm="Abort #5 and tear down its testbed? This deletes its clusters, VMs and network.">Abort and tear down</button>`,
		},
	} {
		page := shownPage(t, s, path)
		for _, want := range wants {
			if !strings.Contains(page, want) {
				t.Errorf("%s lacks %s", path, want)
			}
		}
	}
	if page := shownPage(t, s, "/executions/5"); strings.Contains(page, "Diagnostics:") || strings.Contains(page, "Run again") {
		t.Error("a paused execution offers what only a finished one has")
	}
}

var (
	openKept   = regexp.MustCompile(`<h3>Kept testbed <span class="mono"[^>]*>([^<]+)</span>\s*</h3>`)
	foldedKept = regexp.MustCompile(`<a href="([^"]*)">Kept testbed <span class="mono">([^<]+)</span></a>`)
)

// keptCards are the address of a page's open kept testbed card, and the
// addresses and links of its folded ones.
func keptCards(page string) (open []string, folded map[string]string) {
	for _, m := range openKept.FindAllStringSubmatch(page, -1) {
		open = append(open, m[1])
	}
	folded = map[string]string{}
	for _, m := range foldedKept.FindAllStringSubmatch(page, -1) {
		folded[m[2]] = m[1]
	}
	return open, folded
}

func TestExecutionKeptTestbedCard(t *testing.T) {
	s, _ := fixtureServer(t)
	page := shownPage(t, s, "/executions/4")
	if open, folded := keptCards(page); !slices.Equal(open, []string{"4/quickstart"}) || len(folded) > 0 {
		t.Fatalf("cards: open %v, folded %v", open, folded)
	}
	for _, want := range []string{
		`<span class="chip pass">✓ passed</span>`,
		"<th>Machines</th>", "cp-1 control-plane 172.20.0.11", "Redfish 172.20.0.2:8000", "wendy-pt4 · 172.20.0.0/24",
		">Copy export KUBECONFIG</button>",
		`href="/executions/4/files/quickstart/kubeconfigs/management" download>management cluster kubeconfig</a>`,
		`href="/executions/4/files/quickstart/kubeconfigs/workload" download>workload cluster kubeconfig</a>`,
		`hx-post="/api/executions/4/quickstart/teardown" hx-confirm="Tear down 4/quickstart? This deletes its clusters, VMs and network.">Tear down…</button>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the kept testbed card lacks %s", want)
		}
	}
	if open, _ := keptCards(shownPage(t, s, "/executions/5")); !slices.Equal(open, []string{"5/a-net"}) {
		t.Errorf("a paused execution's cards: open %v", open)
	}
	if open, _ := keptCards(shownPage(t, s, "/executions/1")); len(open) > 0 {
		t.Error("an execution that kept nothing shows a kept testbed")
	}
}

// With several kept testbeds, the one in view is open and the others fold to
// a line that opens them.
func TestExecutionFoldsOtherKeptTestbeds(t *testing.T) {
	s, svc := fixtureServer(t)
	addExecution(t, svc, "6", preview.Selection{Plan: "nightly"}, event.Passed, func(i int, c *engine.CombinationSummary, pc *preview.Combination, dir string) {
		if i < 2 {
			c.Kept = true
			keepTestbed(t, svc, "6", pc, dir)
		}
	})
	sum, err := svc.Execution("6")
	if err != nil {
		t.Fatal(err)
	}
	first, second := sum.Combinations[0].Name, sum.Combinations[1].Name

	check := func(path, wantOpen, wantFolded, wantLink string) {
		t.Helper()
		open, folded := keptCards(shownPage(t, s, path))
		if !slices.Equal(open, []string{"6/" + wantOpen}) || len(folded) != 1 {
			t.Fatalf("%s: open %v, folded %v", path, open, folded)
		}
		if link := folded["6/"+wantFolded]; link != wantLink {
			t.Errorf("%s: 6/%s opens with %q, want %q", path, wantFolded, link, wantLink)
		}
	}
	check("/executions/6", first, second, "/executions/6?combination="+second+"&view=grid")
	check("/executions/6?combination="+second, second, first, "/executions/6?combination="+first+"&view=grid")
	check("/executions/6?view=tree&combination="+second, second, first, "/executions/6?combination="+first+"&view=tree")
	check("/executions/6?combination=nope", first, second, "/executions/6?combination="+second+"&view=grid")

	// Picking a combination in the grid opens its card.
	link := regexp.MustCompile(`<td>\s*<a href="([^"]*)">` + regexp.QuoteMeta(second) + `</a>\s*</td>`).FindStringSubmatch(shownPage(t, s, "/executions/6"))
	if link == nil {
		t.Fatal("the grid does not link the second combination")
	}
	if open, _ := keptCards(shownPage(t, s, link[1])); !slices.Equal(open, []string{"6/" + second}) {
		t.Errorf("after picking %s: open %v", second, open)
	}
}

func TestTearDownFromTheExecutionPage(t *testing.T) {
	s, svc := fixtureServer(t)
	rec := post(t, s, teardownAPI("4", "quickstart"), url.Values{}, nil)
	if got := rec.Header().Get("HX-Redirect"); got != "/executions/4" {
		t.Fatalf("teardown = %d, redirect %q", rec.Code, got)
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if sum, err := svc.Execution("4"); err == nil && !sum.Combinations[0].Kept {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the testbed is still kept")
		}
	}
	if open, folded := keptCards(shownPage(t, s, "/executions/4")); len(open)+len(folded) > 0 {
		t.Error("a torn-down testbed still has a card")
	}
}
