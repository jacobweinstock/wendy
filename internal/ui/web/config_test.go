package web

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/preview"
)

// Every subtab must be usable: a unique kind, a known top-bar tab, a blurb
// that says what it holds, and a way to list its entries.
func TestSubtabTable(t *testing.T) {
	seen := map[string]bool{}
	for _, tab := range subtabs {
		if seen[tab.ID] {
			t.Errorf("subtab %q is listed twice", tab.ID)
		}
		seen[tab.ID] = true
		if tab.Label == "" || tab.Blurb == "" || tab.entries == nil {
			t.Errorf("subtab %q has no label, blurb or entries", tab.ID)
		}
		if !slices.Contains(configTabs, tab.Top) {
			t.Errorf("subtab %q is in unknown top-bar tab %q", tab.ID, tab.Top)
		}
	}
	for _, g := range configTabs {
		if len(subtabsOf(g)) == 0 {
			t.Errorf("top-bar tab %q has no subtabs", g)
		}
	}
}

func TestEverySubtabRendersWithItsBlurb(t *testing.T) {
	s, _ := editServer(t)
	for _, tab := range subtabs {
		u := string(entryURL(tab.ID, ""))
		code, body := get(t, s, u)
		if code != http.StatusOK {
			t.Errorf("%s = %d", u, code)
			continue
		}
		if !strings.Contains(body, `<p class="lead">`+tab.Blurb+`</p>`) {
			t.Errorf("%s lacks its blurb", u)
		}
	}
}

func TestTestbedsTabListsItsSubtabsAndParts(t *testing.T) {
	s, _ := editServer(t)
	_, body := get(t, s, "/testbeds/hardware")
	if !strings.Contains(body, `<a class="on" href="/testbeds/hardware" aria-current="true">Hardware</a>`) {
		t.Errorf("the Hardware subtab is not marked current")
	}
	// The Testbeds tab has the testbeds and their five kinds of part, and
	// none of the Suites tab's subtabs.
	for _, want := range []string{`href="/testbeds/testbeds">Testbeds</a>`, ">Tinkerbell</a>", ">Cluster API</a>", ">Management cluster</a>", ">Workload cluster</a>", ">Hardware</a>", "＋ New hardware part"} {
		if !strings.Contains(body, want) {
			t.Errorf("Testbeds subtabs lack %s", want)
		}
	}
	if strings.Contains(body, `href="/suites/suites"`) || strings.Contains(body, `href="/suites/steps"`) {
		t.Error("the Testbeds tab shows a subtab it should not")
	}
	for _, kind := range config.PartKinds {
		fallback := s.svc.Catalog().Fallback[kind]
		if _, body := get(t, s, string(entryURL(kind, ""))); fallback == "" || !strings.Contains(body, ">"+fallback+"</a>") {
			t.Errorf("%s does not list its fallback part %q", entryURL(kind, ""), fallback)
		}
	}
}

func TestPlanDetail(t *testing.T) {
	s, svc := fixtureServer(t)
	// A newer execution of nightly with its settings changed is not the
	// plan's last execution.
	addExecution(t, svc, "6", preview.Selection{Plan: "nightly", Keep: "never"}, event.Passed, nil)
	page := shownPage(t, s, "/plans/nightly")
	for _, want := range []string{
		`<a class="tab on" href="/plans" aria-current="page">Plans</a>`,
		`<span class="mute" style="padding:6px 0;font-size:13px">2 plans</span>`,
		`<th>Testbed</th><td><a href="/testbeds/testbeds/m">m</a> <span class="mute">4 combinations</span>`,
		`<th>Suites</th><td> <a href="/suites/suites/smoke">smoke</a>`,
		`<th>Only tests with labels</th><td><span class="mute">Every test</span>`,
		"<th>Options</th><td>keep the testbed if something fails · collect diagnostics on failure · record machine consoles</td>",
		`<th>Last execution</th><td><a href="/executions/2">#2</a> <span class="chip fail">✗ failed</span>`,
		`hx-vals="{"plan":"nightly"}">Run</button>`,
		`<a class="btn" href="/run/nightly">Change and run…</a>`,
		`<a class="btn" href="/edit/plans?from=nightly" title="Copies it into wendy-ui.cue, where you can change it">Copy and edit…</a>`,
		"Change and run changes one execution, not the plan.",
		"<summary class=\"btn\">View CUE</summary>",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the nightly plan page lacks %s", want)
		}
	}
	if fresh := shownPage(t, partsServer(t), "/plans/quickstart"); !strings.Contains(fresh, `<th>Last execution</th><td><span class="mute">Not run yet</span>`) {
		t.Error("a plan never run does not say so")
	}
}

func TestTestbedDetail(t *testing.T) {
	s := partsServer(t)
	big := shownPage(t, s, "/testbeds/testbeds/big")
	for _, want := range []string{
		`<span class="kind">Hardware</span> <a href="/testbeds/hardware/ha">ha</a> <span class="hint">control plane 3 · worker 3 · spare 1</span>`,
		`<span class="kind">Workload cluster</span> <a href="/testbeds/workload-cluster/next">next</a> <span class="hint">Kubernetes v1.37.0 · Cilium</span>`,
		`<th>How machines boot</th><td><span style="white-space:pre-wrap">Network boot</span>`,
		"<th>Network extras</th><td>No NAT64 · no registry mirror</td>",
		`<th>Matrix</th><td><span class="mute">None: one combination.</span>`,
		`<span class="chip mono">netboot</span>`,
		`<th>Used by</th><td><span class="mute">No plan yet</span>`,
		`<summary class="btn">Resolved values</summary>`,
		"3 control plane: 4 CPUs, 2048 MiB memory, 10 GiB disk",
		`<a class="btn primary" href="/run?testbed=big">Run tests on this testbed…</a>`,
		"Copy and edit…",
	} {
		if !strings.Contains(big, want) {
			t.Errorf("testbed big lacks %s", want)
		}
	}
	if strings.Contains(big, "Before the matrix") {
		t.Error("a testbed without a matrix explains one")
	}
	// A part written inside the testbed has no page to link to.
	if adhoc := shownPage(t, s, "/testbeds/testbeds/adhoc"); !strings.Contains(adhoc, `<span class="kind">Hardware</span> <span class="mute">Defined in this testbed</span>`) {
		t.Error("an inline part is not shown as defined in the testbed")
	}
	m := shownPage(t, s, "/testbeds/testbeds/m")
	for _, want := range []string{
		`<td class="mono">k</td>`,
		`<span class="mono">a</span> → Workload cluster <a href="/testbeds/workload-cluster/next">next</a>`,
		`<span class="mono">b</span> → <span class="mute">no change</span>`,
		`1 combination: <span class="mono">a</span>`,
		`Left out: <span class="mono">b</span> not today`,
		"Before the matrix: each combination changes some of these.",
	} {
		if !strings.Contains(m, want) {
			t.Errorf("testbed m lacks %s", want)
		}
	}
}

func TestPartDetail(t *testing.T) {
	s := partsServer(t)
	ha := shownPage(t, s, "/testbeds/hardware/ha")
	for _, want := range []string{
		`<p class="mute" style="margin-top:4px">control plane 3 · worker 3 · spare 1</p>`,
		`<th>Architecture</th><td><span class="mute">amd64 (x86-64) (default)</span>`,
		"3 control plane: 4 CPUs, 2048 MiB memory, 10 GiB disk",
		"1 spare: 2 CPUs, 2048 MiB memory, 10 GiB disk",
		"4 advanced settings at the default",
		`testbed <a href="/testbeds/testbeds/big">big</a>`,
		`testbed <a href="/testbeds/testbeds/m">m</a>`,
	} {
		if !strings.Contains(ha, want) {
			t.Errorf("hardware ha lacks %s", want)
		}
	}
	// An advanced setting a part sets is shown; the rest are counted.
	idle := shownPage(t, s, "/testbeds/hardware/idle")
	for _, want := range []string{`<th>BMC protocol</th><td><span style="white-space:pre-wrap">IPMI</span>`, "3 advanced settings at the default", `<th>Used by</th><td><span class="mute">No testbed yet</span>`} {
		if !strings.Contains(idle, want) {
			t.Errorf("hardware idle lacks %s", want)
		}
	}
	// next is used by the testbed that names it, and by the one whose axis picks it.
	next := shownPage(t, s, "/testbeds/workload-cluster/next")
	for _, want := range []string{`<th>Kubernetes version</th><td><span style="white-space:pre-wrap">v1.37.0</span>`, `testbed <a href="/testbeds/testbeds/m">m</a>`} {
		if !strings.Contains(next, want) {
			t.Errorf("workload cluster next lacks %s", want)
		}
	}
}

func TestTestsDetails(t *testing.T) {
	s, _ := fixtureServer(t)
	suite := shownPage(t, s, "/suites/suites/lifecycle")
	for _, want := range []string{
		`<th>Tests, in order</th><td><div>1. <a href="/suites/tests/deploy">deploy</a> <span class="hint">Creates the workload cluster`,
		`<a class="btn primary" href="/run?suite=lifecycle">Run this suite…</a>`,
		"Copy and edit…",
	} {
		if !strings.Contains(suite, want) {
			t.Errorf("suite lifecycle lacks %s", want)
		}
	}
	if smoke := shownPage(t, s, "/suites/suites/smoke"); !strings.Contains(smoke, `plan <a href="/plans/nightly">nightly</a>`) {
		t.Error("suite smoke does not name the plan that uses it")
	}

	test := shownPage(t, s, "/suites/tests/scale-out")
	for _, want := range []string{
		`1. <a class="mono" href="/suites/steps/capt.scale">capt.scale</a> <span class="mute"> Named step · <span class="mono">by: 1, target: worker</span> · times out after 25m</span>`,
		"<th>Needs</th><td>1 spare machine</td>",
		`<span class="chip mono">scale</span>`,
		`suite <a href="/suites/suites/lifecycle">lifecycle</a>`,
	} {
		if !strings.Contains(test, want) {
			t.Errorf("test scale-out lacks %s", want)
		}
	}
	if deploy := shownPage(t, s, "/suites/tests/deploy"); !strings.Contains(deploy, "<th>Once per testbed</th><td>Later suites that list it reuse its result.</td>") {
		t.Error("a test that runs once per testbed does not say so")
	}

	step := shownPage(t, s, "/suites/steps/capt.scale")
	for _, want := range []string{`<th>Parameters</th><td><pre class="log">`, `test <a href="/suites/tests/scale-out">scale-out</a>`, "choose <b>Named step</b> and pick"} {
		if !strings.Contains(step, want) {
			t.Errorf("step capt.scale lacks %s", want)
		}
	}
}

// Config files says which files loaded, where each error is, and whether the
// last valid config is still in use.
func TestConfigFilesPage(t *testing.T) {
	s, svc := editServer(t)
	file := filepath.Join(svc.StarterDir, "nightly.cue")
	write := func(src string) {
		t.Helper()
		if err := os.WriteFile(file, []byte("package local\n\n"+src+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		svc.Reload()
	}

	write(`plans: p: testbed: "quickstart"`)
	page := shownPage(t, s, configPath)
	for _, want := range []string{
		`<h2 class="title">Config files</h2>`,
		`<span class="pass">✓</span> <span class="mono">` + file + `</span>`,
		"The UIs write only to",
		"wendy-ui.cue",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("a clean config page lacks %q", want)
		}
	}
	if strings.Contains(page, `class="fail"`) {
		t.Error("a clean config page shows an error")
	}

	// A type error fails the whole directory, so its last valid result stays.
	write(`plans: p: testbed: 1`)
	page = shownPage(t, s, configPath)
	for _, want := range []string{
		`<span class="fail">1 error</span>`,
		`<span class="fail">✗</span> <span class="mono">` + file + `</span>`,
		"<b>" + file + ":3:20</b>",
		"conflicting values 1 and string",
		"The last valid config is still in use",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("a broken config page lacks %q", want)
		}
	}
}

// A testbed with no matrix shows how to give it one, and the example it
// shows is CUE that makes two combinations.
func TestMatrixHint(t *testing.T) {
	s, svc := editServer(t)
	if err := svc.SaveEntry(config.KindTestbed, "", "lab", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	lab := shownPage(t, s, "/testbeds/testbeds/lab")
	for _, want := range []string{"give it a matrix in", "wendy-ui.cue", `testbeds: "lab": matrix: {`, `href="/edit/testbeds/lab">Edit testbed</a>`} {
		if !strings.Contains(lab, want) {
			t.Errorf("the hint lacks %s", want)
		}
	}
	if qs := shownPage(t, s, "/testbeds/testbeds/quickstart"); !strings.Contains(qs, "use Copy and edit… and give the copy a matrix") {
		t.Error("a built-in testbed's hint does not say to copy it")
	}

	src := "package local\n\ntestbeds: example: {}\n" + matrixExample(svc.Catalog(), "example") + "\n"
	if err := os.WriteFile(filepath.Join(svc.StarterDir, "example.cue"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	svc.Reload()
	if tb := svc.Catalog().Testbeds["example"]; tb == nil || len(tb.Combinations) != 2 {
		t.Errorf("the example makes %+v, want two combinations; errors: %v", tb, svc.Catalog().AllErrors())
	}
}

// Every subtab that can make an entry deletes one of yours from its pane, or
// says what keeps it in use. An entry from your own file says to delete it
// there, and a built-in offers neither.
func TestDetailPanesDelete(t *testing.T) {
	s, svc := editServer(t)
	for _, e := range []struct {
		kind, name string
		v          map[string]any
	}{
		{config.KindStep, "add-worker", map[string]any{"uses": "capt.scale", "with": map[string]any{"target": "worker", "by": int64(1)}}},
		{config.KindTest, "t", map[string]any{"steps": []any{map[string]any{"uses": "add-worker"}}}},
		{config.KindSuite, "s", map[string]any{"tests": []any{"t"}}},
		{config.KindHardware, "hw", map[string]any{}},
		{config.KindHardware, "spare", map[string]any{}},
		{config.KindTestbed, "lab", map[string]any{"hardware": "hw"}},
		{config.KindPlan, "p", map[string]any{"testbed": "lab", "suites": []any{"s"}}},
	} {
		if err := svc.SaveEntry(e.kind, "", e.name, e.v); err != nil {
			t.Fatalf("%s %s: %v", e.kind, e.name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(svc.StarterDir, "mine.cue"), []byte("package local\n\ntestbeds: mine: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc.Reload()

	for path, used := range map[string]string{
		"/plans/p":                 "",
		"/testbeds/hardware/spare": "",
		"/testbeds/testbeds/lab":   "plan p",
		"/testbeds/hardware/hw":    "testbed lab",
		"/suites/suites/s":         "plan p",
		"/suites/tests/t":          "suite s",
		"/suites/steps/add-worker": "test t",
	} {
		page := shownPage(t, s, path)
		if used == "" {
			if !strings.Contains(page, `hx-post="/edit/`) || !strings.Contains(page, `/delete" hx-confirm="Delete `) || strings.Contains(page, "disabled>Delete") {
				t.Errorf("%s has no enabled Delete", path)
			}
		} else if !strings.Contains(page, "Used by "+used+", so it can't be deleted.") || !strings.Contains(page, `class="btn danger" disabled>Delete`) {
			t.Errorf("%s does not say %s keeps it from being deleted", path, used)
		}
	}
	mine := shownPage(t, s, "/testbeds/testbeds/mine")
	if !strings.Contains(mine, "which Wendy doesn't change; delete it there.") || strings.Contains(mine, ">Delete testbed<") {
		t.Error("an entry from your own file does not say where to delete it")
	}
	if qs := shownPage(t, s, "/testbeds/testbeds/quickstart"); strings.Contains(qs, ">Delete testbed<") || strings.Contains(qs, "delete it there") {
		t.Error("a built-in offers to be deleted")
	}

	if rec := post(t, s, "/edit/hardware/spare/delete", nil, nil); rec.Header().Get("HX-Redirect") != "/testbeds/hardware" {
		t.Errorf("deleting from the pane = %v %s", rec.Header(), rec.Body)
	}
	if svc.Catalog().Parts(config.KindHardware)["spare"] != nil {
		t.Error("the part was not deleted")
	}
}
