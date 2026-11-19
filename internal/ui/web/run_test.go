package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/tinkerbell/wendy/internal/config"
)

const diagnosticsConfig = `package local

plans: loud: {testbed: "quickstart", suites: ["smoke"], diagnostics: "always", consoles: false}
`

func loudServer(t *testing.T) *Server {
	t.Helper()
	s, svc := editServer(t)
	if err := os.WriteFile(filepath.Join(svc.StarterDir, "loud.cue"), []byte(diagnosticsConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	svc.Reload()
	return s
}

// Run tests always posts diagnostics, but only a change is an override, so
// an untouched form leaves the plan unedited.
func TestRunTestsDiagnosticsAreAnOverrideOnlyWhenChanged(t *testing.T) {
	s := loudServer(t)
	selection := func(form url.Values) (*config.Diagnostics, error) {
		req := httptest.NewRequest(http.MethodPost, apiPreview, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		sel, err := s.formSelection(req)
		return sel.Diagnostics, err
	}
	base := url.Values{"testbed": {"quickstart"}, "suitesSet": {"1"}, "suites": {"smoke"}, "diagnosticsSet": {"1"}}
	with := func(kv ...string) url.Values {
		v := url.Values{}
		for k, vs := range base {
			v[k] = vs
		}
		for i := 0; i+1 < len(kv); i += 2 {
			v.Set(kv[i], kv[i+1])
		}
		return v
	}

	// With a plan: the plan's own values (always, consoles off) are no override.
	if d, err := selection(with("plan", "loud", "diagnostics", "always")); err != nil || d != nil {
		t.Errorf("plan's own diagnostics: %v, %v", d, err)
	}
	if d, err := selection(with("plan", "loud", "diagnostics", "always", "consoles", "on")); err != nil || d == nil || !d.Consoles || d.Collect != "always" {
		t.Errorf("consoles turned on: %v, %v", d, err)
	}
	// Without one: the defaults are no override.
	if d, err := selection(with("diagnostics", "onFailure", "consoles", "on")); err != nil || d != nil {
		t.Errorf("default diagnostics: %v, %v", d, err)
	}
	if d, err := selection(with("diagnostics", "never", "consoles", "on")); err != nil || d == nil || d.Collect != "never" {
		t.Errorf("diagnostics never: %v, %v", d, err)
	}
	// A form that does not mention diagnostics leaves them alone, and a bad value is refused.
	if d, err := selection(url.Values{"testbed": {"quickstart"}}); err != nil || d != nil {
		t.Errorf("no diagnostics in the form: %v, %v", d, err)
	}
	if _, err := selection(with("diagnostics", "sometimes")); err == nil {
		t.Error("accepted a diagnostics setting that does not exist")
	}
	// The form's suites are all it runs, so it must pick one.
	none := with("diagnostics", "onFailure")
	none.Del("suites")
	if _, err := selection(none); !errors.Is(err, errNoSuite) {
		t.Errorf("no suite picked: %v", err)
	}
}

// Started from a plan, the form shows the plan's values and says the plan
// does not change.
func TestRunTestsFromAPlan(t *testing.T) {
	page := shownPage(t, loudServer(t), "/run/loud")
	for _, want := range []string{
		`<a href="/plans">Plans</a> › <a href="/plans/loud">loud</a> › Run a plan`,
		`<option value="loud" selected>loud</option>`,
		`<input type="radio" name="diagnostics" value="always" checked>`,
		`<input type="hidden" name="plan" value="loud">`,
		"Changes apply to this execution only. Plan loud is not changed.",
		`<a class="btn" href="/plans/loud">Cancel</a>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the form lacks %s", want)
		}
	}
	if strings.Contains(page, `name="consoles" checked`) {
		t.Error("consoles is on, but the plan turns them off")
	}

	blank := shownPage(t, loudServer(t), "/run")
	if !regexp.MustCompile(`<div class="crumbs">\s*Run a plan\s*</div>`).MatchString(blank) {
		t.Error("the blank form's crumbs are not just Run a plan")
	}
	for _, want := range []string{
		"This execution only. Your config files are not changed.", `<a class="btn" href="/">Cancel</a>`,
		`<option value="" selected>Blank</option>`, `<option value="" selected>Pick a testbed</option>`,
		"Pick a testbed", `id="run-btn" class="btn primary" type="button" hx-post="/api/executions" hx-include="#runform" disabled`,
		"<summary class=\"btn\">Save as plan…</summary>",
	} {
		if !strings.Contains(blank, want) {
			t.Errorf("the blank form lacks %s", want)
		}
	}
	if picked := shownPage(t, loudServer(t), "/run?from=loud"); !strings.Contains(picked, `<input type="hidden" name="plan" value="loud">`) {
		t.Error("the Plan card's picker does not start from the plan it picks")
	}
}

// Save as plan… writes the form as a plan, keeping the diagnostics of the
// plan it started from, and goes back to the form with the new plan picked.
// Run alone saves nothing.
func TestRunTestsSaveAsPlan(t *testing.T) {
	s := loudServer(t)
	form := url.Values{
		"plan": {"loud"}, "testbed": {"quickstart"}, "suites": {"smoke"}, "suitesSet": {"1"}, "keep": {"never"},
		"pauseSet": {"1"}, "diagnosticsSet": {"1"}, "diagnostics": {"always"}, "name": {"louder"}, "then": {"run"},
	}
	if rec := post(t, s, "/edit/plans", form, nil); rec.Header().Get("HX-Redirect") != "/run/louder" {
		t.Fatalf("not saved: %d %s", rec.Code, rec.Body)
	}
	if p := s.svc.Catalog().Plans["louder"]; p == nil || p.Value.Diagnostics != "always" || p.Value.Consoles {
		t.Errorf("plan louder = %+v, want loud's diagnostics", p)
	}

	// The run form runs a plan; a plan is deleted from its own page, so the
	// form doesn't offer it, even for a plan of yours.
	if page := shownPage(t, s, "/run/louder"); strings.Contains(page, "/delete") || strings.Contains(page, "Delete plan") {
		t.Error("the run form offers to delete plan louder")
	}
}

// The form picks a testbed and its suites. Only a testbed with a matrix
// offers combinations, and there is no other way to choose them.
func TestRunTestsPicksATestbed(t *testing.T) {
	s, _ := fixtureServer(t)
	plain := shownPage(t, s, "/run?testbed=big&suite=upgrades")
	for _, want := range []string{
		`<option value="big" selected>big</option>`,
		"One: this testbed has no matrix.",
		`<input type="checkbox" name="suites" value="upgrades" checked>`,
		"Labels in use: <span class=\"mono\">scale, upgrade</span>.",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("a testbed without a matrix: the form lacks %s", want)
		}
	}
	if strings.Contains(plain, `name="combinations"`) || strings.Contains(plain, `value="smoke" checked`) {
		t.Error("a testbed without a matrix offers combinations, or ?suite= did not pick the suite")
	}

	matrix := shownPage(t, s, "/run?testbed=m")
	for _, want := range []string{
		`<input type="hidden" name="axesOf" value="m">`,
		`<input type="checkbox" name="axis" value="k=a" checked>`,
		`<input type="checkbox" name="axis" value="boot=iso" checked>`,
		`name="combinations"`, "4 combinations × ",
	} {
		if !strings.Contains(matrix, want) {
			t.Errorf("a testbed with a matrix: the form lacks %s", want)
		}
	}
	for _, page := range []string{plain, matrix} {
		if strings.Contains(page, `name="matrix"`) || strings.Contains(page, `name="environment"`) {
			t.Error("the form offers a matrix or environment choice")
		}
	}

	// Re-rendered after picking another testbed, what was picked of the
	// first one's matrix is dropped.
	back := shownPage(t, s, "/run?suitesSet=1&suites=smoke&testbed=big&combinations=a-net&axesOf=m&axis=k%3Da")
	if strings.Contains(back, `name="combinations"`) || strings.Contains(back, "no combinations to pick") {
		t.Error("switching to a testbed without a matrix kept the combinations")
	}
}

func previewOf(t *testing.T, s *Server, form url.Values) string {
	t.Helper()
	rec := post(t, s, apiPreview, form, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST %s = %d %s", apiPreview, rec.Code, rec.Body)
	}
	return strings.Join(strings.Fields(rec.Body.String()), " ")
}

// Turning axis values off narrows the combinations, as names do, and the
// preview says how many of all it runs.
func TestRunTestsNarrowsCombinations(t *testing.T) {
	s, _ := fixtureServer(t)
	form := func(kv ...string) url.Values {
		v := url.Values{"testbed": {"m"}, "suitesSet": {"1"}, "suites": {"smoke"}, "axesOf": {"m"}}
		for i := 0; i+1 < len(kv); i += 2 {
			v.Add(kv[i], kv[i+1])
		}
		return v
	}
	all := form("axis", "k=a", "axis", "k=b", "axis", "boot=net", "axis", "boot=iso")
	if got := previewOf(t, s, all); !strings.Contains(got, "<b>4 combinations × ") || strings.Contains(got, " of 4") {
		t.Errorf("every value on: %s", got)
	}
	if got := previewOf(t, s, form("axis", "k=a", "axis", "boot=net", "axis", "boot=iso")); !strings.Contains(got, "<b>2 combinations × ") || !strings.Contains(got, "· 2 of 4") {
		t.Errorf("k=b off: %s", got)
	}
	named := form("axis", "k=a", "axis", "boot=net", "axis", "boot=iso", "combinations", "*-iso")
	if got := previewOf(t, s, named); !strings.Contains(got, "<b>1 combination × ") || !strings.Contains(got, `<td class="mono">a-iso</td>`) {
		t.Errorf("k=b off and *-iso: %s", got)
	}
	if got := previewOf(t, s, form("axis", "boot=net", "axis", "boot=iso")); !strings.Contains(got, "No combination matches") {
		t.Errorf("every k off: %s", got)
	}
}

// Run is disabled while the preview shows a problem, on the page and in
// every preview after.
func TestRunDisabledOnAProblem(t *testing.T) {
	s, _ := fixtureServer(t)
	page := shownPage(t, s, "/run?suitesSet=1&testbed=big")
	if !strings.Contains(page, "Pick at least one suite") || !strings.Contains(page, `id="run-btn" class="btn primary" type="button" hx-post="/api/executions" hx-include="#runform" disabled`) {
		t.Error("with no suite, the page does not say so or offers Run")
	}

	none := url.Values{"testbed": {"big"}, "suitesSet": {"1"}}
	if got := previewOf(t, s, none); !strings.Contains(got, "Pick at least one suite") || !strings.Contains(got, `disabled title="Fix the problem in the preview first" hx-swap-oob="true">Run</button>`) {
		t.Errorf("the preview with no suite: %s", got)
	}
	some := url.Values{"testbed": {"big"}, "suitesSet": {"1"}, "suites": {"smoke"}}
	if got := previewOf(t, s, some); strings.Contains(got, "disabled") || !strings.Contains(got, `hx-swap-oob="true">Run</button>`) {
		t.Errorf("the preview with a suite: %s", got)
	}
}
