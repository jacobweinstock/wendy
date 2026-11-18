package tui

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/hostcheck"
	"github.com/tinkerbell/wendy/internal/preview"
	"github.com/tinkerbell/wendy/internal/service"
	"github.com/tinkerbell/wendy/internal/ui"
	"github.com/tinkerbell/wendy/internal/vocab"
)

// newRun is the New run form: one form, no wizard, with the plan beside it.
type newRun struct {
	base string

	testbed      string
	combinations string
	suites       []string
	labels       string
	parallel     string
	keep         string
	pause        bool

	form *huh.Form
	gen  int
	last string
	pv   *service.Preview
	err  error
	w, h int
}

// newRun opens the form pre-filled from a selection, or from a plan when base
// names one ("Edit & run").
func (m *model) newRun(sel preview.Selection, base string) tea.Cmd {
	f := &newRun{base: base, keep: "never", parallel: "1"}
	if base != "" {
		if p, ok := m.cat.Plans[base]; ok {
			pl := p.Value
			sel.Testbed, sel.Combinations, sel.Suites = pl.Testbed, pl.Combinations, pl.Suites
			sel.LabelFilter, sel.Parallel, sel.Keep = pl.LabelFilter, pl.Parallel, pl.Keep
			pause := pl.PauseOnFailure
			sel.PauseOnFailure = &pause
		}
	}
	f.testbed = sel.Testbed
	f.combinations = strings.Join(sel.Combinations, " ")
	f.suites = sel.Suites
	if len(f.suites) == 0 {
		f.suites = []string{"smoke"}
	}
	f.labels = sel.LabelFilter
	if sel.Parallel > 0 {
		f.parallel = strconv.Itoa(sel.Parallel)
	}
	if sel.Keep != "" {
		f.keep = sel.Keep
	}
	if sel.PauseOnFailure != nil {
		f.pause = *sel.PauseOnFailure
	}
	f.build(m)
	f.resize(m.w, m.h)
	m.form = f
	return tea.Batch(f.form.Init(), f.replan(m))
}

func (f *newRun) build(m *model) {
	var testbeds []huh.Option[string]
	for _, n := range config.Names(m.cat.Testbeds) {
		label := n
		if tb := m.cat.Testbeds[n]; tb.HasMatrix() {
			label += "  " + vocab.Count(len(tb.Combinations), "combination", "combinations")
		}
		testbeds = append(testbeds, huh.NewOption(label, n))
	}
	if f.testbed == "" && len(testbeds) > 0 {
		f.testbed = testbeds[0].Value
		// The starter config's testbed, when there is one.
		if _, ok := m.cat.Testbeds["mine"]; ok {
			f.testbed = "mine"
		}
	}
	var suites []huh.Option[string]
	for _, n := range config.Names(m.cat.Suites) {
		suites = append(suites, huh.NewOption(n, n).Selected(slices.Contains(f.suites, n)))
	}
	f.form = huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Testbed").Options(testbeds...).Value(&f.testbed).Height(min(len(testbeds)+1, 6)),
		huh.NewInput().Title("Combinations").Placeholder("all; or globs, e.g. colocated-ipv4-*").Value(&f.combinations).
			DescriptionFunc(func() string { return f.combinationCount(m) }, []any{&f.testbed, &f.combinations}),
		huh.NewMultiSelect[string]().Title("Suites").Options(suites...).Value(&f.suites).Height(min(len(suites)+1, 5)),
		huh.NewInput().Title("Label filter").Placeholder("optional, e.g. !mirror").Value(&f.labels),
		huh.NewInput().Title("Parallel").Value(&f.parallel).Validate(func(s string) error {
			if n, err := strconv.Atoi(s); err != nil || n < 1 {
				return fmt.Errorf("a number, at least 1")
			}
			return nil
		}),
		huh.NewSelect[string]().Title("Keep testbed").Options(choiceOptions("keep")...).Value(&f.keep).Inline(true),
		huh.NewConfirm().Title("Pause on failure").Description("wait for me before tearing down").Value(&f.pause),
	)).WithShowHelp(false)
}

// choiceOptions are the values of the field at fieldPath, each shown under
// its vocab label.
func choiceOptions(fieldPath string) []huh.Option[string] {
	var opts []huh.Option[string]
	for _, v := range vocab.ChoiceValues(fieldPath) {
		opts = append(opts, huh.NewOption(vocab.Choice(fieldPath, v), v))
	}
	return opts
}

func (f *newRun) resize(w, h int) {
	f.w, f.h = w, h
	if f.form != nil {
		f.form.WithWidth(f.formWidth()).WithHeight(max(h-6, 10))
	}
}

func (f *newRun) formWidth() int {
	if f.w < 100 {
		return f.w - 4
	}
	return f.w/2 - 4
}

// combinationCount is "4 of 16 match" for a testbed with a matrix.
func (f *newRun) combinationCount(m *model) string {
	tb, ok := m.cat.Testbeds[f.testbed]
	if !ok || !tb.HasMatrix() {
		return "only for a testbed with a matrix"
	}
	n, total := 0, 0
	for _, c := range tb.Combinations {
		if c.Excluded != "" {
			continue
		}
		total++
		if matchesAny(c.Name, strings.Fields(f.combinations)) {
			n++
		}
	}
	return fmt.Sprintf("%d of %d match", n, total)
}

func matchesAny(name string, pats []string) bool {
	if len(pats) == 0 {
		return true
	}
	for _, p := range pats {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// selection is what the form says to run.
func (f *newRun) selection() preview.Selection {
	sel := preview.Selection{
		Plan: f.base, Testbed: f.testbed, LabelFilter: f.labels, Keep: f.keep, Suites: append([]string{}, f.suites...),
		// Empty means all, so it is sent even then, over a plan's pick.
		Combinations: strings.Fields(strings.ReplaceAll(f.combinations, ",", " ")),
	}
	if sel.Combinations == nil {
		sel.Combinations = []string{}
	}
	sel.Parallel, _ = strconv.Atoi(f.parallel)
	pause := f.pause
	sel.PauseOnFailure = &pause
	return sel
}

// replan recomputes the preview when the form's values changed.
func (f *newRun) replan(m *model) tea.Cmd {
	sel := f.selection()
	key := fmt.Sprintf("%+v %v", sel, *sel.PauseOnFailure)
	if key == f.last && f.pv != nil {
		return nil
	}
	f.last = key
	f.gen++
	gen := f.gen
	svc := m.svc
	return func() tea.Msg {
		pv, err := svc.Preview(context.Background(), sel)
		return previewMsg{gen: gen, pv: pv, err: err}
	}
}

func (f *newRun) setPreview(msg previewMsg) {
	if msg.gen != f.gen {
		return
	}
	f.pv, f.err = msg.pv, msg.err
}

func (f *newRun) update(m *model, msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc":
			m.form = nil
			return nil
		case "ctrl+r", "ctrl+s":
			return m.start(f.selection())
		}
	}
	fm, cmd := f.form.Update(msg)
	if nf, ok := fm.(*huh.Form); ok {
		f.form = nf
	}
	switch f.form.State {
	case huh.StateCompleted:
		return m.start(f.selection())
	case huh.StateAborted:
		m.form = nil
		return nil
	default:
	}
	return tea.Batch(cmd, f.replan(m))
}

func (f *newRun) view(m *model) string {
	title := " New run"
	if f.base != "" {
		title += " · based on " + sBold.Render(f.base)
	}
	h := m.h - 3
	left := box("", f.form.View(), f.formWidth()+2, h, true)
	right := box("Preview", f.planView(m.w-f.formWidth()-10), m.w-f.formWidth()-6, h, false)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	if m.w < 100 {
		body = left
	}
	return title + "\n" + body + "\n" +
		footer("tab/enter", "next field", "space", "toggle", "←→", "choose", "ctrl+r", "start run", "esc", "cancel")
}

func (f *newRun) planView(w int) string {
	if f.err != nil {
		return statusStyle(event.Failed).Render(f.err.Error())
	}
	if f.pv == nil {
		return sMute.Render("planning…")
	}
	p := f.pv.Preview
	var b strings.Builder
	nSuites := 0
	if len(p.Combinations) > 0 {
		nSuites = len(p.Combinations[0].Suites)
	}
	b.WriteString(sBold.Render(fmt.Sprintf("%s × %s", vocab.Count(len(p.Combinations), "combination", "combinations"), vocab.Count(nSuites, "suite", "suites"))) + "\n\n")
	var first string
	for i, c := range p.Combinations {
		if i >= 6 {
			b.WriteString(sMute.Render(fmt.Sprintf("… and %d more\n", len(p.Combinations)-i)))
			break
		}
		var lines []string
		for _, s := range c.Suites {
			var scs []string
			for _, sc := range s.Tests {
				if sc.Skip != "" {
					scs = append(scs, sc.Name+"(–)")
				} else {
					scs = append(scs, sc.Name)
				}
			}
			lines = append(lines, fmt.Sprintf("  %-10s %s", s.Name, strings.Join(scs, " › ")))
		}
		chain := strings.Join(lines, "\n")
		if i > 0 && chain == first {
			b.WriteString(fit(c.Name, w-10) + sMute.Render("  (same)") + "\n")
			continue
		}
		first = chain
		b.WriteString(c.Name + "\n" + sMute.Render(chain) + "\n")
	}
	n := p.Needs
	row := func(k, v string) { fmt.Fprintf(&b, "%s %s\n", fit(sMute.Render(k), 8), v) }
	b.WriteString("\n")
	row("Needs", fmt.Sprintf("%d VMs at once · %d GiB RAM · %d GiB disk", n.VMs, (n.RAMMiB+1023)/1024, n.DiskGiB))
	if len(p.Builds) == 0 {
		row("Builds", "none (released artifacts)")
	}
	for _, bd := range p.Builds {
		row("Builds", bd.Component+" from "+bd.Source.Repo+" "+bd.Source.Ref)
	}
	if n.Estimate > 0 {
		row("Takes", fmt.Sprintf("~%s (from %s)", ui.Dur(n.Estimate.Milliseconds()), vocab.Count(n.Samples, "past run", "past runs")))
	} else {
		row("Takes", "unknown (no past runs)")
	}
	var checks []string
	seen := map[string]bool{}
	for _, c := range f.pv.Checks {
		if (c.Optional && c.OK) || (c.Optional && c.Name != "disk space") || seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		checks = append(checks, checkGlyph(c)+" "+c.Name)
	}
	row("Checks", strings.Join(checks, "  "))
	for _, c := range f.pv.Checks {
		if !c.OK && (!c.Optional || c.Name == "disk space") {
			b.WriteString("         " + sMute.Render(fit(c.Detail, w-10)) + "\n")
		}
	}
	b.WriteString("\n" + sMute.Render("This run only; your config files are not changed."))
	return b.String()
}

func checkGlyph(c hostcheck.Check) string {
	switch {
	case c.OK:
		return glyph(event.Passed)
	case c.Optional:
		return glyph(event.Errored)
	}
	return glyph(event.Failed)
}
