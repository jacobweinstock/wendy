package web

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/hostcheck"
	"github.com/tinkerbell/wendy/internal/preview"
	"github.com/tinkerbell/wendy/internal/service"
	"github.com/tinkerbell/wendy/internal/ui"
	"github.com/tinkerbell/wendy/internal/vocab"
)

var keeps = vocab.ChoiceValues("keep")

// diagnosticsChoices are the choices for when to collect diagnostics, from the
// same field the editors use.
var diagnosticsChoices = func() []string {
	for _, f := range ui.PlanDiagnosticsFields {
		if f.Path == "diagnostics" {
			return f.Options
		}
	}
	return nil
}()

func capital(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// singular is how a sentence names one entry of a tab or kind, e.g. "Cluster
// API part".
func singular(tab string) string {
	if k, ok := vocab.KindOf(tab); ok {
		return k.One
	}
	return strings.TrimSuffix(tab, "s")
}

// plural is how a sentence names several entries of a kind, e.g. "Cluster
// API parts".
func plural(kind string) string {
	if k, ok := vocab.KindOf(kind); ok {
		return k.Many
	}
	return kind
}

// kindLabel is a kind's name as a label, e.g. "Workload cluster".
func kindLabel(kind string) string {
	if k, ok := vocab.KindOf(kind); ok {
		return k.Label
	}
	return capital(kind)
}

// shortPath shows a path the way people type it: ./.wendy/x.cue, ~/.config/….
func shortPath(p string) string {
	if p == "" || p == config.BuiltIn || !filepath.IsAbs(p) {
		return p
	}
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, p); err == nil && !strings.HasPrefix(rel, "..") {
			return "./" + rel
		}
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// checkStatus is how a host check reads: passed, failed, or errored when
// only some features need what it checks.
func checkStatus(c hostcheck.Check) event.Status {
	switch {
	case c.OK:
		return event.Passed
	case c.Optional:
		return event.Errored
	}
	return event.Failed
}

func checkClass(c hostcheck.Check) string { return cls(checkStatus(c)) }
func checkGlyph(c hostcheck.Check) string { return glyph(checkStatus(c)) }

func quickLine(p *planRow) string {
	s := p.Desc
	if p.Typical != "" {
		s += " About " + p.Typical + "."
	}
	return s
}

// progressLine is how far a running execution is: steps, the step it is on,
// and the time left.
func progressLine(e runningExecution) string {
	parts := []string{fmt.Sprintf("%d/%d steps", e.Sum.Steps.Done, e.Sum.Steps.Total)}
	if e.Now != "" {
		parts = append(parts, "now "+e.Now)
	}
	if e.ETA != "" {
		parts = append(parts, e.ETA)
	}
	return strings.Join(parts, " · ")
}

// previewHeadline is how much a preview runs: "4 combinations × 6 tests".
func previewHeadline(p *preview.Preview) string {
	return vocab.Count(len(p.Combinations), "combination", "combinations") + " × " + vocab.Count(testCount(p), "test", "tests")
}

// labelExample is a label expression to show as an example, made of labels
// in use when there are two.
func labelExample(labels []string) string {
	if len(labels) < 2 {
		return "scale && !upgrade"
	}
	return labels[0] + " && !" + labels[1]
}

func nodeTitle(n *ui.Node) string {
	if n.Kind == ui.KindCombination || len(n.Path) == 0 {
		return n.Name
	}
	return pathLabel(n.Path)
}

func caseName(e event.Event) string {
	if len(e.Path) == 0 {
		return e.Msg
	}
	return e.Path[len(e.Path)-1]
}

func (d executionData) count(s event.Status) int {
	n := d.View.Counts[s]
	if s == event.Failed {
		n += d.View.Counts[event.TimedOut]
	}
	return n
}

// builtin is the selected built-in step.
func (d entriesData) builtin() *config.Builtin {
	for i := range d.Cat.BuiltinSteps {
		if d.Cat.BuiltinSteps[i].ID == d.Sel {
			return &d.Cat.BuiltinSteps[i]
		}
	}
	return nil
}

// partSummaryOf is a part in one line, for a testbed's picker: the settings
// that tell parts of its kind apart, with defaults filled in.
func partSummaryOf(cat *config.Catalog, kind, name string) string {
	it := cat.Parts(kind)[name]
	if it == nil {
		return "No " + singular(kind) + " is called " + name + "."
	}
	v := map[string]any(it.Value)
	// setting is a field's value, or its default when the part leaves it out.
	setting := func(path string) string {
		for _, f := range ui.PartFields(kind) {
			if f.Path == path {
				if got := ui.Get(v, f); got != "" {
					return got
				}
				return f.Default
			}
		}
		return ""
	}
	switch kind {
	case config.KindHardware:
		var roles []string
		for _, r := range ui.MachineRoles {
			roles = append(roles, strings.ToLower(r.Label)+" "+setting("machines."+r.Key+".count"))
		}
		return strings.Join(roles, " · ")
	case config.KindWorkloadCluster:
		return "Kubernetes " + setting("kubernetesVersion") + " · " + vocab.Choice("cni", setting("cni"))
	case config.KindClusterAPI:
		capt := "CAPT " + setting("capt.version")
		if repo := setting("capt.source.repo"); repo != "" {
			capt = "CAPT built from " + repo
		}
		return "Cluster API " + setting("core") + " · " + capt
	case config.KindManagementCluster:
		if image := setting("image"); image != "" {
			return "kind · " + image
		}
		return "kind · its own node image"
	case config.KindTinkerbell:
		if repo := setting("source.repo"); repo != "" {
			return "built from " + repo
		}
		return "chart " + setting("chart.version")
	}
	return ""
}

// optionsLine is how a plan runs, in one line: "2 combinations at once ·
// keep the testbed if it fails · always collect diagnostics".
func optionsLine(p config.Plan) string {
	var parts []string
	if p.Parallel > 1 {
		parts = append(parts, fmt.Sprintf("%d combinations at once", p.Parallel))
	}
	parts = append(parts, map[string]string{
		"never": "never keep the testbed", "failed": "keep the testbed if it fails", "always": "always keep the testbed",
	}[cmp(p.Keep, "never")])
	if p.PauseOnFailure {
		parts = append(parts, "pause on failure")
	}
	d := p.Collects()
	parts = append(parts, map[string]string{
		"onFailure": "collect diagnostics on failure", "always": "always collect diagnostics", "never": "never collect diagnostics",
	}[d.Collect])
	if d.Consoles {
		parts = append(parts, "record machine consoles")
	}
	return strings.Join(parts, " · ")
}

func networkLine(e service.KeptTestbed) string {
	n := e.State.Network
	parts := []string{n.Name}
	for _, s := range []string{n.Subnet, n.Subnet6} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " · ")
}
