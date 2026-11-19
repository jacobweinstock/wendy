package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/ui"
	"github.com/tinkerbell/wendy/internal/vocab"
)

// visible is the text people read on a page, one piece per line: text, and
// the title, aria-label, placeholder and hx-confirm attributes. It skips what
// is shown as code (<pre>, <code>, .mono, an option's title, which holds its
// CUE value, and a copy button's title, which is the command it copies) and
// what is never shown (<script>, <style>).
func visible(page string) string {
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return page
	}
	var out []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			if t := strings.Join(strings.Fields(n.Data), " "); t != "" {
				out = append(out, t)
			}
		case html.ElementNode:
			switch n.Data {
			case "pre", "code", "script", "style":
				return
			}
			if slices.Contains(strings.Fields(attr(n, "class")), "mono") {
				return
			}
			for _, a := range n.Attr {
				switch {
				case a.Key == "title" && (n.Data == "option" || attr(n, "data-copy") != ""):
				case a.Key == "title", a.Key == "aria-label", a.Key == "placeholder", a.Key == "hx-confirm":
					out = append(out, a.Val)
				}
			}
		default:
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return strings.Join(out, "\n")
}

// rawOptions are the select options of a page that read as a CUE value that
// is labelled otherwise.
func rawOptions(page string, labelled map[string]bool) []string {
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return nil
	}
	var out []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "option" {
			v := attr(n, "value")
			if labelled[v] && strings.TrimSpace(textOf(n)) == v {
				out = append(out, v)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func textOf(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(textOf(c))
	}
	return b.String()
}

// labelledValues are the values the forms offer that vocab labels as
// something else, so an option reading as one shows the raw value.
func labelledValues() map[string]bool {
	out := map[string]bool{}
	add := func(path string, values []string) {
		for _, v := range values {
			if vocab.Choice(path, v) != v {
				out[v] = true
			}
		}
	}
	fields := slices.Concat(ui.StepFields, ui.PlanDiagnosticsFields)
	for _, kind := range editable {
		fields = append(fields, ui.PartFields(kind)...)
	}
	for _, f := range fields {
		add(f.Path, f.Options)
	}
	add("step.kind", ui.StepKinds)
	add("keep", vocab.ChoiceValues("keep"))
	return out
}

type guardPage struct {
	name, path string
	s          *Server
}

// guardPages are the pages the spec's Testing Strategy names: every tab and
// subtab and an entry of each kind, each execution in grid and tree, Run
// tests three ways, each edit form new, copied and existing, Config files
// with an error, and first launch.
func guardPages(t *testing.T) []guardPage {
	t.Helper()
	s, svc := fixtureServer(t)
	paths := []string{homePath, executionsPath}
	for id := 1; id <= 5; id++ {
		paths = append(paths, fmt.Sprintf("%s?view=grid", executionURL(fmt.Sprint(id))), fmt.Sprintf("%s?view=tree", executionURL(fmt.Sprint(id))))
	}
	paths = append(paths, string(runTestsURL("", nil)), string(runTestsURL("nightly", nil)), runTestsPath+"?testbed=m")
	paths = append(paths, plansPath, testbedsPath, suitesPath)
	for _, tab := range subtabs {
		paths = append(paths, string(entryURL(tab.ID, "")))
	}
	paths = append(paths, string(entryURL(config.KindPlan, "nightly")), string(entryURL(config.KindTestbed, "m")),
		string(entryURL(config.KindTestbed, "big")), string(entryURL(config.KindHardware, "ha")),
		string(entryURL(config.KindWorkloadCluster, "next")))

	cat := svc.Catalog()
	for _, kind := range editable {
		tab, _ := subtabOf(kind)
		first := config.Names(tab.entries(cat))[0]
		if kind == config.KindStep {
			// Built-in steps are not copied; a step of the user's is.
			first = "wait"
			if err := svc.SaveEntry(kind, "", first, map[string]any{"exec": map[string]any{"command": []any{"kubectl", "wait"}}}); err != nil {
				t.Fatal(err)
			}
			paths = append(paths, string(entryURL(kind, first)))
		}
		e, err := svc.Entry(kind, first)
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.SaveEntry(kind, "", "mine", e.Value); err != nil {
			t.Fatalf("saving a copy of %s %s: %v", kind, first, err)
		}
		paths = append(paths, string(editURL(kind, "")), string(copyURL(kind, first)), string(editURL(kind, "mine")))
	}

	var pages []guardPage
	for _, p := range paths {
		pages = append(pages, guardPage{p, p, s})
	}

	first, _ := editServer(t)
	pages = append(pages, guardPage{"/ (first launch)", homePath, first})

	broken, bsvc := editServer(t)
	if err := os.WriteFile(filepath.Join(bsvc.StarterDir, "broken.cue"), []byte("package local\n\nplans: p: testbed: \"nope\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bsvc.Reload()
	if len(bsvc.Catalog().AllErrors()) == 0 {
		t.Fatal("broken.cue has no error")
	}
	pages = append(pages, guardPage{"/config (with an error)", configPath, broken})
	return pages
}

// Every page says only glossary words, and no select shows a raw value.
func TestGlossaryGuard(t *testing.T) {
	labelled := labelledValues()
	for _, p := range guardPages(t) {
		code, body := get(t, p.s, p.path)
		if code != http.StatusOK {
			t.Errorf("GET %s = %d", p.name, code)
			continue
		}
		for _, o := range rawOptions(body, labelled) {
			t.Errorf("%s: a select option reads as the raw value %q", p.name, o)
		}
		found := map[string]bool{}
		for _, f := range vocab.Check(visible(body)) {
			if !found[f.Term] {
				found[f.Term] = true
				t.Errorf("%s says %q (%q); use %s", p.name, f.Term, f.Context, f.Use)
			}
		}
	}
}

// missingTargets are a page's htmx attributes that select an element by ID
// that the page doesn't have. htmx then swaps nothing, so the control looks
// as if it works and does nothing.
func missingTargets(page string) []string {
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return []string{err.Error()}
	}
	ids := map[string]bool{}
	var wanted []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				switch {
				case a.Key == "id":
					ids[a.Val] = true
				case (a.Key == "hx-target" || a.Key == "hx-select" || a.Key == "hx-include") && strings.HasPrefix(a.Val, "#"):
					wanted = append(wanted, a.Key+`="`+a.Val+`"`)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	var out []string
	for _, w := range wanted {
		_, id, _ := strings.Cut(w, `="#`)
		if !ids[strings.TrimSuffix(id, `"`)] {
			out = append(out, w)
		}
	}
	return out
}

func TestHtmxTargetsAreOnThePage(t *testing.T) {
	for _, p := range guardPages(t) {
		_, body := get(t, p.s, p.path)
		if missing := missingTargets(body); len(missing) > 0 {
			t.Errorf("%s selects elements it doesn't have: %v", p.name, missing)
		}
	}
	if got := missingTargets(`<div id="a"></div><select hx-get="/x" hx-target="#a" hx-select="#main"></select>`); !slices.Equal(got, []string{`hx-select="#main"`}) {
		t.Errorf("the check finds %v, want only the missing #main", got)
	}
}

// The guard has to be able to fail, or the test above proves nothing.
func TestGlossaryGuardCatchesWhatItShould(t *testing.T) {
	page := `<html><head><title>Wendy</title></head><body>
<p>Open the catalog</p>
<a href="/" title="New environment">x</a>
<pre>profiles: x</pre>
<code>scenario</code>
<span class="mono">combo</span>
<select>
<option value="external">external</option>
<option value="isoboot" title="isoboot">ISO boot</option>
<option value="quickstart">quickstart</option>
</select>
</body></html>`
	var terms []string
	for _, f := range vocab.Check(visible(page)) {
		terms = append(terms, f.Term)
	}
	if want := []string{"catalog", "environment"}; !slices.Equal(terms, want) {
		t.Errorf("found %v, want %v", terms, want)
	}
	if raw := rawOptions(page, labelledValues()); !slices.Equal(raw, []string{"external"}) {
		t.Errorf("raw options = %v, want [external]", raw)
	}
}
