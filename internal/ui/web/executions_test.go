package web

import (
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/preview"
	"github.com/tinkerbell/wendy/internal/service"
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
			`<span class="mute">Collected so far:</span>`, `<button class="btn primary" hx-post="/api/executions/5/retry?combination=a-net">Retry step</button>`,
			`Carry on <span class="mute">(record the failure, run the rest)</span>`,
			// Plan nightly keeps the testbed if something fails, so Stop keeps it.
			`hx-confirm="Stop #5? Its testbed is kept so you can look around; tear it down when you're done.">Stop</button>`,
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

var changeAndRun = regexp.MustCompile(`<a class="btn" href="(/run\?[^"]*)">Change and run…</a>`)

// A Tinkerbell on its own cluster has its kubeconfig on its row, and a
// cluster with no kubeconfig yet has no buttons.
func TestKeptRowsReachTinkerbellsOwnCluster(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "kubeconfigs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "kubeconfigs", engine.TinkerbellCluster), []byte("kind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := service.KeptTestbed{Execution: "9", Dir: dir, State: engine.State{Clusters: map[string]string{
		engine.TinkerbellCluster: filepath.Join(dir, "kubeconfigs", engine.TinkerbellCluster),
		engine.ManagementCluster: filepath.Join(dir, "kubeconfigs", engine.ManagementCluster),
	}}}
	clusters := map[string]string{}
	for _, r := range keptRows(e) {
		if r.Cluster != nil {
			clusters[r.Label] = clusterLabel(r.Cluster.Cluster)
		}
	}
	if len(clusters) != 1 || clusters["Tinkerbell"] != "Tinkerbell cluster" {
		t.Errorf("rows with a cluster to reach = %v, want only Tinkerbell's", clusters)
	}
}

// Stopping a paused combination says first whether its testbed is kept.
func TestStopConfirmSaysWhatHappensToTheTestbed(t *testing.T) {
	for keep, want := range map[string]string{
		"never":  "Stop #7 and tear down its testbed?",
		"failed": "Stop #7? Its testbed is kept",
		"always": "Stop #7? Its testbed is kept",
	} {
		sum := &engine.Summary{ID: "7", Effective: preview.Selection{Keep: keep}}
		if got := stopConfirm(sum); !strings.HasPrefix(got, want) {
			t.Errorf("keep %s: %q, want %q…", keep, got, want)
		}
	}
}

// A live execution's grid says which combination is running, where it is and
// for how long.
func TestGridShowsWhatIsRunning(t *testing.T) {
	s, svc := fixtureServer(t)
	addExecution(t, svc, "6", preview.Selection{Plan: "nightly"}, event.Running, func(i int, c *engine.CombinationSummary, _ *preview.Combination, _ string) {
		if i == 0 {
			c.Nodes["setup"] = event.Passed
			return
		}
		c.Status = event.Waiting
	})
	stream, err := event.Create(filepath.Join(svc.ExecutionDir("6"), "events.jsonl"), "6")
	if err != nil {
		t.Fatal(err)
	}
	stream.Emit(event.Event{Type: event.StepStarted, Combination: "a-net", Path: []string{"smoke", "deploy", "capt.cluster.create"}, Status: event.Running})
	_ = stream.Close()

	page := shownPage(t, s, "/executions/6?view=grid")
	status := func(st event.Status) string {
		return `<span class="` + cls(st) + `" title="` + word(st) + `">` + glyph(st) + `</span> <a href=`
	}
	for _, want := range []string{
		status(event.Running) + `"/executions/6?combination=a-net&view=tree">a-net</a>`,
		`<td class="c ` + cls(event.Running) + `" title="smoke: ` + word(event.Running) + `">` + glyph(event.Running) + `</td>`,
		// The execution started four hours ago, and so did a-net.
		`<td class="mono">4h00</td>`,
		`<a href="/executions/6?node=a-net%00smoke%00deploy%00capt.cluster.create">smoke › deploy › capt.cluster.create</a>`,
		status(event.Waiting) + `"/executions/6?combination=b-iso&view=tree">b-iso</a>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the grid lacks %s", want)
		}
	}
	if n := strings.Count(page, `<td class="mono"></td>`); n != 3 {
		t.Errorf("%d combinations show no time, want the 3 waiting ones", n)
	}
}

var (
	pickedCombination = regexp.MustCompile(`<details id="combination-picker" class="dropdown">\s*<summary class="btn sm"><span [^>]*>[^<]*</span> ([^<]+)</summary>`)
	pickerEntry       = regexp.MustCompile(`<a href="/executions/\d+\?combination=([^&"]+)&view=tree"(?: class="([^"]*)")?><span class="[^"]*" title="([^"]+)">`)
)

// treeOf is the tree's rows on an execution's page.
func treeOf(page string) string {
	_, tree, _ := strings.Cut(page, `class="card tree"`)
	tree, _, _ = strings.Cut(tree, `id="follow"`)
	return tree
}

// The tree shows one combination: the one the URL names, else the one that
// most needs looking at. A dropdown lists them all with their statuses.
func TestTreeShowsOneCombination(t *testing.T) {
	s, svc := fixtureServer(t)
	passed := func(c *engine.CombinationSummary) { c.Status = event.Passed }
	running := func(c *engine.CombinationSummary) { c.Status = event.Running }
	waiting := func(c *engine.CombinationSummary) { c.Status = event.Waiting }
	failed := func(c *engine.CombinationSummary) {
		c.Status, c.Failure = event.Failed, &engine.Failure{Path: []string{"smoke", "nodes"}, Status: event.Failed}
	}
	paused := func(c *engine.CombinationSummary) { failed(c); c.Status, c.Paused = event.Paused, true }
	// Each of nightly's combinations in turn: a-net, a-iso, b-net, b-iso.
	add := func(id string, status event.Status, each ...func(*engine.CombinationSummary)) {
		addExecution(t, svc, id, preview.Selection{Plan: "nightly"}, status, func(i int, c *engine.CombinationSummary, _ *preview.Combination, _ string) {
			each[i](c)
		})
	}
	add("6", event.Running, passed, failed, running, paused)
	add("7", event.Running, passed, failed, running, waiting)
	add("8", event.Failed, passed, failed, passed, passed)
	add("9", event.Passed, passed, passed, passed, passed)

	setup := func(c string) string { return url.QueryEscape(c + "\x00setup") }
	for path, want := range map[string]string{
		"/executions/6?view=tree":                                "b-iso",
		"/executions/7?view=tree":                                "b-net",
		"/executions/8?view=tree":                                "a-iso",
		"/executions/9?view=tree":                                "a-net",
		"/executions/6?view=tree&combination=a-net":              "a-net",
		"/executions/6?view=tree&combination=nope":               "b-iso",
		"/executions/6?node=" + setup("a-iso"):                   "a-iso",
		"/executions/6?combination=b-net&node=" + setup("a-iso"): "b-net",
	} {
		page := shownPage(t, s, path)
		if m := pickedCombination.FindStringSubmatch(page); m == nil || m[1] != want {
			t.Errorf("%s shows %v, want %s", path, m, want)
			continue
		}
		tree := treeOf(page)
		for _, c := range []string{"a-net", "a-iso", "b-net", "b-iso"} {
			if has := strings.Contains(tree, "node="+c+"%00"); has != (c == want) {
				t.Errorf("%s: the tree has %s's rows: %v", path, c, has)
			}
		}
		if strings.Contains(tree, "node="+want+`%00"`) {
			t.Errorf("%s: the tree has a row for the combination itself", path)
		}
	}
	if tree := treeOf(shownPage(t, s, "/executions/6?node="+setup("a-iso"))); !strings.Contains(tree, "node="+setup("a-iso")+`" class="sel"`) {
		t.Error("the row ?node= names is not picked")
	}

	page := shownPage(t, s, "/executions/6?view=tree&combination=b-net")
	var entries []string
	for _, m := range pickerEntry.FindAllStringSubmatch(page, -1) {
		entries = append(entries, strings.TrimSpace(m[1]+" "+m[3]+" "+m[2]))
	}
	if want := []string{
		"a-net " + word(event.Passed), "a-iso " + word(event.Failed), "b-net " + word(event.Running) + " on", "b-iso " + word(event.Paused),
	}; !slices.Equal(entries, want) {
		t.Errorf("the dropdown lists %q, want %q", entries, want)
	}
	tree := treeOf(page)
	for _, want := range []string{"<span>Set up testbed</span>", "<span>suite smoke</span>"} {
		if !strings.Contains(tree, want) {
			t.Errorf("the tree lacks %s", want)
		}
	}
	if strings.Contains(tree, "steps)") {
		t.Error("Set up testbed still counts its steps")
	}

	page = shownPage(t, s, "/executions/1")
	if strings.Contains(page, "combination-picker") {
		t.Error("a single combination has a dropdown")
	}
	if tree := treeOf(page); !strings.Contains(tree, "node="+setup("quickstart")) || strings.Contains(tree, `node=quickstart%00"`) {
		t.Error("a single combination's tree should show its rows and no row for itself")
	}
}

// Following stays in the combination the URL names, even while another
// started a step more recently.
func TestFollowStaysInTheNamedCombination(t *testing.T) {
	s, svc := fixtureServer(t)
	addExecution(t, svc, "6", preview.Selection{Plan: "nightly"}, event.Running, func(i int, c *engine.CombinationSummary, _ *preview.Combination, _ string) {
		if i > 1 {
			c.Status = event.Waiting
		}
	})
	stream, err := event.Create(filepath.Join(svc.ExecutionDir("6"), "events.jsonl"), "6")
	if err != nil {
		t.Fatal(err)
	}
	step := []string{"smoke", "deploy", "capt.cluster.create"}
	stream.Emit(event.Event{Type: event.StepStarted, Combination: "a-iso", Path: step, Status: event.Running, TS: time.Now().Add(-2 * time.Minute)})
	stream.Emit(event.Event{Type: event.StepStarted, Combination: "a-net", Path: step, Status: event.Running, TS: time.Now().Add(-time.Minute)})
	_ = stream.Close()

	page := shownPage(t, s, "/executions/6?view=tree&combination=a-iso")
	if !strings.Contains(treeOf(page), "node=a-iso%00smoke%00deploy%00capt.cluster.create\" class=\"sel\"") {
		t.Error("Follow left a-iso for the step a-net started later")
	}
	if !strings.Contains(page, "☑ Follow the running step") {
		t.Error("the page is not following")
	}
}

// An execution's page says what it ran with, defaults marked, and Change and
// run… opens the run form filled in with it.
func TestExecutionSettings(t *testing.T) {
	s, svc := fixtureServer(t)
	unsaved := shownPage(t, s, "/executions/3")
	for _, want := range []string{
		`<details id="settings" class="adv" style="margin:0 0 14px" hx-preserve><summary>Settings</summary>`,
		`<th>Plan</th><td><span class="mute">Unsaved plan: run from a form</span>`,
		`<th>Testbed</th><td><a href="/testbeds/testbeds/big">big</a>`,
		`<th>Combinations</th><td><span class="mute">All (default)</span>`,
		`<th>Suites</th><td> <a href="/suites/suites/smoke">smoke</a>`,
		`<th>Only tests with labels</th><td><span class="mute">Every test (default)</span>`,
		`<th>When it finishes</th><td><span class="mute">Tear down the testbed (default)</span>`,
		`<th>When a step fails</th><td><span class="mute">Stop (default)</span>`,
		`<th>Machine consoles</th><td><span class="mute">Recorded (default)</span>`,
	} {
		if !strings.Contains(unsaved, want) {
			t.Errorf("#3's settings lack %s", want)
		}
	}

	nightly := shownPage(t, s, "/executions/2")
	for _, want := range []string{
		`<th>Plan</th><td><a href="/plans/nightly">nightly</a>`,
		`<th>Testbed</th><td><a href="/testbeds/testbeds/m">m</a>`,
		`<th>When it finishes</th><td><span style="white-space:pre-wrap">Keep the testbed if something fails</span>`,
	} {
		if !strings.Contains(nightly, want) {
			t.Errorf("#2's settings lack %s", want)
		}
	}

	// Change and run… previews what the execution ran.
	m := changeAndRun.FindStringSubmatch(nightly)
	if m == nil {
		t.Fatal("#2 has no Change and run…")
	}
	run := shownPage(t, s, m[1])
	ran, err := preview.New(svc.Catalog(), preview.Selection{Plan: "nightly"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<option value="nightly" selected>nightly</option>`, `<option value="m" selected>m</option>`,
		`name="suites" value="smoke" checked`, `<input type="radio" name="keep" value="failed" checked>`,
		`<input type="radio" name="pauseOnFailure" value="false" checked>`, "<b>" + previewHeadline(ran) + "</b>",
	} {
		if !strings.Contains(run, want) {
			t.Errorf("Change and run… lacks %s", want)
		}
	}
	if strings.Contains(run, `<p class="fail"`) {
		t.Error("Change and run… previews a problem")
	}

	// An execution recorded before executions kept their settings shows them
	// from its selection when it ran an unsaved plan, and names only the
	// plan otherwise.
	for _, id := range []string{"1", "3"} {
		dir := svc.ExecutionDir(id)
		sum, err := engine.ReadSummary(dir)
		if err != nil {
			t.Fatal(err)
		}
		sum.Effective = preview.Selection{}
		if err := engine.WriteSummary(dir, sum); err != nil {
			t.Fatal(err)
		}
	}
	if old := shownPage(t, s, "/executions/3"); !strings.Contains(old, `<th>When it finishes</th><td><span class="mute">Tear down the testbed (default)</span>`) {
		t.Error("an older unsaved execution does not show its settings")
	}
	old := shownPage(t, s, "/executions/1")
	if !strings.Contains(old, "recorded before Wendy kept an execution's settings") || !strings.Contains(old, `<a class="btn" href="/run/quickstart">Change and run…</a>`) {
		t.Error("an older plan execution does not say its settings weren't recorded, or offer its plan")
	}
}

func TestExecutionKeptTestbedCard(t *testing.T) {
	s, _ := fixtureServer(t)
	page := shownPage(t, s, "/executions/4")
	if open, folded := keptCards(page); !slices.Equal(open, []string{"4/quickstart"}) || len(folded) > 0 {
		t.Fatalf("cards: open %v, folded %v", open, folded)
	}
	for _, want := range []string{
		`<span class="chip pass">✓ passed</span>`,
		// Each part is a row, and each value says what it is.
		`<tr><th>Tinkerbell</th><td><div class="facts"><span><span class="mute">Address</span> <span class="mono">172.20.0.100</span></span>`,
		`<tr><th>Workload cluster</th><td><div class="facts"><span><span class="mute">Kubernetes</span> <span class="mono">v1.36.4</span></span>`,
		`<th scope="col">Name</th><th scope="col">Role</th><th scope="col">IP address</th></tr><tr><td class="mono">cp-1</td><td>Control plane</td><td class="mono">172.20.0.11</td></tr>`,
		`<span class="mute">Protocol</span> <span>Redfish</span></span> <span><span class="mute">Address</span> <span class="mono">172.20.0.2:8000</span>`,
		`<span class="mute">Name</span> <span class="mono">wendy-pt4</span></span> <span><span class="mute">IPv4</span> <span class="mono">172.20.0.0/24</span>`,
		`hx-post="/api/executions/4/quickstart/teardown" hx-confirm="Tear down 4/quickstart? This deletes its clusters, VMs and network.">Tear down…</button>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the kept testbed card lacks %s", want)
		}
	}
	if strings.Contains(page, `<span class="mono">/`) || strings.Contains(page, `<div class="facts"></div>`) {
		t.Error("the card shows a kubeconfig's path, or an empty row of facts")
	}
	if strings.Contains(page, "Copy export KUBECONFIG (management cluster)") {
		t.Error("the node pane repeats the open card's kubeconfig buttons")
	}
	// Each cluster's row copies and downloads that cluster's kubeconfig, and
	// the buttons' names say which cluster.
	for _, c := range []string{"management", "workload"} {
		line := regexp.MustCompile(`<tr><th>` + capital(c) + ` cluster</th>.*?</tr>`).FindString(page)
		for _, want := range []string{
			`/quickstart/kubeconfigs/` + c + `" aria-label="Copy export KUBECONFIG for the ` + c + ` cluster">Copy export KUBECONFIG</button>`,
			`href="/executions/4/files/quickstart/kubeconfigs/` + c + `" download aria-label="Download kubeconfig of the ` + c + ` cluster">Download kubeconfig</a>`,
		} {
			if !strings.Contains(line, want) {
				t.Errorf("the %s cluster's row %q lacks %s", c, line, want)
			}
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
	link := regexp.MustCompile(`<td>\s*<span [^>]*>[^<]*</span>\s*<a href="([^"]*)">` + regexp.QuoteMeta(second) + `</a>\s*</td>`).FindStringSubmatch(shownPage(t, s, "/executions/6"))
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
