package web

import (
	"context"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/tinkerbell/wendy/internal/event"
)

func TestRoutesAnswerAndOldOnesAreGone(t *testing.T) {
	s, svc := fixtureServer(t)
	failed, err := svc.Execution("2")
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := string(executionFileURL("2", failed.Combinations[0].Failure.Diagnostics["workflow"]))

	for _, p := range []string{
		"/", "/executions", "/executions?status=running", "/executions/1", "/executions/2", "/executions/2?view=tree",
		"/executions/5", diagnostics,
		"/run", "/run/nightly", "/run?testbed=m",
		"/plans", "/plans/nightly",
		"/testbeds", "/testbeds/testbeds/m", "/testbeds/hardware/ha", "/testbeds/workload-cluster/next",
		"/suites", "/suites/suites/smoke", "/suites/steps",
		"/edit/testbeds", "/edit/testbeds?from=big", "/edit/cluster-api",
		"/config",
		apiExecutions, executionAPI("2"), apiKeptTestbeds, apiConfig,
	} {
		if code, _ := get(t, s, p); code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", p, code)
		}
	}
	for _, p := range []string{
		"/executions/99", executionAPI("99"), "/run/nope", "/plans/nope", "/testbeds/nope", "/testbeds/testbeds/nope",
		"/testbeds/suites/smoke", "/suites/hardware", "/edit/plans/nope", "/edit/clusterAPI",
		// The old routes are gone, not redirected.
		"/runs", "/runs/new", "/runs/1", "/catalog", "/catalog/tests", "/environments", "/api/runs", "/api/catalog", "/api/environments",
		"/tests", "/tests/suites/smoke",
	} {
		if code, _ := get(t, s, p); code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, code)
		}
	}
	if rec := post(t, s, "/api/plan", url.Values{"testbed": {"quickstart"}}, nil); rec.Code != http.StatusNotFound {
		t.Errorf("POST /api/plan = %d, want 404", rec.Code)
	}
}

var (
	tabHref    = regexp.MustCompile(`<a class="tab(?: on)?" href="([^"]*)"`)
	currentTab = regexp.MustCompile(`<a class="tab on" href="([^"]*)" aria-current="page">`)
)

func TestTopBar(t *testing.T) {
	render := func(l layout) string {
		t.Helper()
		var b strings.Builder
		if err := topbar(l).Render(context.Background(), &b); err != nil {
			t.Fatal(err)
		}
		return html.UnescapeString(b.String())
	}

	quiet := render(layout{Tab: "testbeds"})
	var hrefs []string
	for _, m := range tabHref.FindAllStringSubmatch(quiet, -1) {
		hrefs = append(hrefs, m[1])
	}
	if want := []string{"/", "/executions", "/plans", "/testbeds", "/suites"}; !slices.Equal(hrefs, want) {
		t.Errorf("tabs = %v, want %v", hrefs, want)
	}
	if m := currentTab.FindAllStringSubmatch(quiet, -1); len(m) != 1 || m[0][1] != "/testbeds" {
		t.Errorf("current tabs = %v, want only /testbeds", m)
	}
	if strings.Contains(quiet, "config error") || strings.Contains(quiet, "status=running") {
		t.Error("an indicator shows with nothing to indicate")
	}
	if !strings.Contains(quiet, `<a id="run-tests" class="btn primary" href="/run">Run a plan…</a>`) {
		t.Error("the top bar lacks Run a plan…")
	}

	busy := render(layout{Running: 2, ConfigErrors: 1})
	if !strings.Contains(busy, `href="/config"`) || !strings.Contains(busy, "⚠ 1 config error<") {
		t.Error("the config error indicator is missing")
	}
	if !strings.Contains(busy, `href="/executions?status=running"`) || !strings.Contains(busy, " 2 "+word(event.Running)+"<") {
		t.Error("the running indicator is missing")
	}
	if strings.Contains(busy, "aria-current") {
		t.Error("a page with no tab marks one")
	}

	// Each page marks the tab it sits in; forms mark the tab of what they edit.
	s, _ := fixtureServer(t)
	for p, want := range map[string]string{
		"/": "/", "/executions": "/executions", "/executions/2": "/executions", "/plans/nightly": "/plans",
		"/testbeds/hardware/ha": "/testbeds", "/edit/hardware": "/testbeds", "/edit/testbeds?from=big": "/testbeds",
		"/suites/suites/smoke": "/suites", "/edit/tests": "/suites", "/edit/suites": "/suites", "/edit/steps": "/suites",
		"/run": "", "/config": "",
	} {
		_, body := get(t, s, p)
		got := ""
		if m := currentTab.FindStringSubmatch(body); m != nil {
			got = m[1]
		}
		if got != want {
			t.Errorf("%s marks tab %q, want %q", p, got, want)
		}
	}
}

var hrefAttr = regexp.MustCompile(`\shref="([^"]*)"`)

// Every link on every page reachable from Home and Config files answers, and
// a link to a config entry is never deeper than its tab, subtab and name.
func TestEveryLinkAnswersAndConfigLinksAreShallow(t *testing.T) {
	s, _ := fixtureServer(t)
	queue := []string{homePath, configPath}
	seen := map[string]bool{homePath: true, configPath: true}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		code, body := get(t, s, p)
		if code != http.StatusOK {
			t.Errorf("GET %s = %d", p, code)
			continue
		}
		if strings.Contains(p, "/files/") {
			continue
		}
		for _, m := range hrefAttr.FindAllStringSubmatch(body, -1) {
			u := html.UnescapeString(m[1])
			if !strings.HasPrefix(u, "/") || strings.HasPrefix(u, "/static/") || seen[u] {
				continue
			}
			seen[u] = true
			if tooDeep(u) {
				t.Errorf("%s links %s, deeper than a config entry", p, u)
			}
			queue = append(queue, u)
		}
	}
}

func tooDeep(u string) bool {
	path, _, _ := strings.Cut(u, "?")
	segs := strings.Split(strings.Trim(path, "/"), "/")
	switch "/" + segs[0] {
	case plansPath:
		return len(segs) > 2
	case testbedsPath, suitesPath:
		return len(segs) > 3
	}
	return false
}
