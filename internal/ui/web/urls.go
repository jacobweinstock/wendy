package web

import (
	"net/url"
	"path/filepath"
	"slices"

	"github.com/a-h/templ"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/vocab"
)

// The web UI's paths. Pages and redirects build links only with the helpers
// below, so a route changes here and in the route table, nowhere else.
const (
	homePath       = "/"
	executionsPath = "/executions"
	runTestsPath   = "/run"
	plansPath      = "/plans"
	testbedsPath   = "/testbeds"
	suitesPath     = "/suites"
	configPath     = "/config"
	editPath       = "/edit"
	// editStepPath and partSummaryPath answer an edit form with a fragment:
	// one more step row, and the summary of a picked part. partDialogPath is
	// the form that makes a part in the testbed form's dialog. The matrix
	// paths answer the testbed form's matrix editor with an axis and with the
	// combinations its axes make.
	editStepPath           = "/edit-step"
	partSummaryPath        = editPath + "/part-summary"
	partDialogPath         = editPath + "/part-dialog"
	matrixAxisPath         = editPath + "/matrix-axis"
	matrixCombinationsPath = editPath + "/matrix-combinations"

	apiExecutions   = "/api/executions"
	apiKeptTestbeds = "/api/kept-testbeds"
	apiPreview      = "/api/preview"
	apiConfig       = "/api/config"
	apiConfigEvents = "/api/config/events"
	apiStarter      = "/api/starter"
	apiReload       = "/api/reload"
)

// navTab is one tab of the top bar.
type navTab struct{ ID, Label, Href string }

// navTabs are the top bar's tabs, in order. Keys 1 to 5 follow this order.
var navTabs = []navTab{
	{"home", "Home", homePath},
	{"executions", "Executions", executionsPath},
	{"plans", "Plans", plansPath},
	{"testbeds", "Testbeds", testbedsPath},
	{"suites", "Suites", suitesPath},
}

// executionsURL is the executions list, with q as its query.
func executionsURL(q url.Values) templ.SafeURL {
	if len(q) == 0 {
		return executionsPath
	}
	return templ.SafeURL(executionsPath + "?" + q.Encode())
}

// executionURL is an execution's page.
func executionURL(id string) templ.SafeURL {
	return templ.SafeURL(executionsPath + "/" + url.PathEscape(id))
}

// keptTestbedURL is an execution's page with the card of one combination's
// kept testbed open.
func keptTestbedURL(execution, combination string) templ.SafeURL {
	return executionURL(execution) + templ.SafeURL("?"+url.Values{"combination": {combination}}.Encode())
}

// executionFileURL serves a file of an execution, rel to its directory.
func executionFileURL(id, rel string) templ.SafeURL {
	return executionURL(id) + templ.SafeURL("/files/"+filepath.ToSlash(rel))
}

// executionAPI is an execution's API path, followed by more path elements
// such as an action.
func executionAPI(id string, more ...string) string {
	p := apiExecutions + "/" + url.PathEscape(id)
	for _, m := range more {
		p += "/" + m
	}
	return p
}

// teardownAPI tears down a kept testbed: one combination of an execution.
func teardownAPI(execution, combination string) string {
	return executionAPI(execution, url.PathEscape(combination), "teardown")
}

// decisionAPI answers a combination paused after a failure with decision:
// retry, continue or abort, which the UIs call Retry step, Carry on and Stop.
func decisionAPI(execution, combination, decision string) string {
	return executionAPI(execution, decision) + "?" + url.Values{"combination": {combination}}.Encode()
}

// runTestsURL is the Run tests form: blank, or started from a plan, with q
// preselecting a testbed or a suite.
func runTestsURL(plan string, q url.Values) templ.SafeURL {
	p := runTestsPath
	if plan != "" {
		p += "/" + url.PathEscape(plan)
	}
	if len(q) > 0 {
		p += "?" + q.Encode()
	}
	return templ.SafeURL(p)
}

// configTab is the top-bar tab that lists entries of a kind: plans, testbeds
// (testbeds and their parts) or suites (suites, tests and steps).
func configTab(kind string) string {
	switch {
	case kind == config.KindPlan:
		return "plans"
	case kind == config.KindTestbed || slices.Contains(config.PartKinds, kind):
		return "testbeds"
	}
	return "suites"
}

// topTabOf is the top-bar tab that lists entries of a kind.
func topTabOf(kind string) navTab {
	id := configTab(kind)
	for _, t := range navTabs {
		if t.ID == id {
			return t
		}
	}
	panic("no top-bar tab " + id)
}

// slugOf names a kind in URLs.
func slugOf(kind string) string {
	k, _ := vocab.KindOf(kind)
	return k.Slug
}

// kindBySlug is the kind a URL names, and whether there is one.
func kindBySlug(slug string) (string, bool) {
	for _, k := range vocab.Kinds {
		if k.Slug == slug {
			return k.ID, true
		}
	}
	return "", false
}

// entryURL is the page of a config entry of a kind, or with no name the list
// of that kind: /plans/<name>, /testbeds/<slug>/<name> or /suites/<slug>/<name>.
func entryURL(kind, name string) templ.SafeURL {
	p := plansPath
	switch configTab(kind) {
	case "testbeds":
		p = testbedsPath + "/" + slugOf(kind)
	case "suites":
		p = suitesPath + "/" + slugOf(kind)
	}
	if name != "" {
		p += "/" + url.PathEscape(name)
	}
	return templ.SafeURL(p)
}

// editURL is the form that creates an entry of a kind, or with a name edits
// that entry.
func editURL(kind, name string) templ.SafeURL {
	p := editPath + "/" + slugOf(kind)
	if name != "" {
		p += "/" + url.PathEscape(name)
	}
	return templ.SafeURL(p)
}

// copyURL is the form that creates an entry of a kind as a copy of another.
func copyURL(kind, from string) templ.SafeURL {
	return editURL(kind, "") + templ.SafeURL("?from="+url.QueryEscape(from))
}

// partDialogURL is the form that makes a part of a kind in the testbed form's
// dialog.
func partDialogURL(kind string) string {
	return partDialogPath + "?" + url.Values{"kind": {kind}}.Encode()
}
