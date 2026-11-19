package tui

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/preview"
	"github.com/tinkerbell/wendy/internal/ui"
	"github.com/tinkerbell/wendy/internal/vocab"
)

func up(k string) bool   { return k == "up" || k == "k" }
func down(k string) bool { return k == "down" || k == "j" }

// panes lays out two panes side by side, or stacked below 100 columns.
func (m *model) panes(leftTitle, left, rightTitle, right string, h, focus int) string {
	if m.w < 100 {
		if focus == 0 {
			return box(leftTitle, left, m.w-2, h-2, true)
		}
		return box(rightTitle, right, m.w-2, h-2, true)
	}
	lw := m.w/2 - 2
	return lipgloss.JoinHorizontal(lipgloss.Top,
		box(leftTitle, left, lw, h-2, focus == 0),
		box(rightTitle, right, m.w-lw-4, h-2, focus == 1))
}

// Home.

type homeState struct {
	sel  int
	pane int
}

func (m *model) homeItems() []string {
	return append(config.Names(m.cat.Plans), "")
}

func (m *model) updateHome(k tea.KeyPressMsg) tea.Cmd {
	items := m.homeItems()
	switch s := k.String(); {
	case up(s):
		m.home.sel = max(m.home.sel-1, 0)
	case down(s):
		m.home.sel = min(m.home.sel+1, len(items)-1)
	case s == "tab":
		m.home.pane = 1 - m.home.pane
	case s == "i" && !m.cat.HasUserConfig:
		path, err := m.svc.WriteStarter()
		if err != nil {
			m.sayf("%v", err)
			return nil
		}
		m.sayf("wrote %s; edit it and wendy reloads it", path)
	case s == "enter" && m.home.pane == 1:
		if a := m.attention(); len(a) > 0 {
			m.openRun(a[0].ID)
		} else if act := m.svc.Active(); len(act) > 0 {
			m.openRun(act[0].ID)
		}
	case s == "enter":
		name := items[min(m.home.sel, len(items)-1)]
		if name == "" {
			return m.newRun(preview.Selection{}, "")
		}
		return m.start(preview.Selection{Plan: name})
	case s == "e":
		if name := items[min(m.home.sel, len(items)-1)]; name != "" {
			return m.newRun(preview.Selection{Plan: name}, name)
		}
	}
	return nil
}

// attention is runs waiting on a decision.
func (m *model) attention() []*engine.Summary {
	var out []*engine.Summary
	for _, r := range m.runs {
		if r.Status == event.Paused {
			out = append(out, r)
		}
	}
	return out
}

func planLine(p config.Plan) string {
	target := p.Testbed
	if len(p.Combinations) > 0 {
		target += " › " + strings.Join(p.Combinations, ",")
	}
	return target + " · " + strings.Join(p.Suites, ", ")
}

func (m *model) homeView(h int) string {
	if !m.cat.HasUserConfig {
		return m.firstLaunch(h)
	}
	items := m.homeItems()
	inner := m.w/2 - 6
	if m.w < 100 {
		inner = m.w - 6
	}
	descW := max(inner-30, 8)
	var l strings.Builder
	l.WriteString("\n")
	for i, n := range items {
		label := "+ Custom run…"
		meta := ""
		if n != "" {
			label = n
			if last := m.svc.LastResult(n); last != nil {
				meta = glyph(last.Status) + " " + ui.Dur(last.Ms)
			} else {
				meta = sMute.Render("  —")
			}
		} else {
			l.WriteString(sMute.Render(" "+strings.Repeat("─", inner-2)) + "\n")
		}
		desc := ""
		if p, ok := m.cat.Plans[n]; ok {
			desc = sMute.Render(fit(planLine(p.Value), descW))
		}
		line := fmt.Sprintf(" %s %s %s", fit(label, 18), fit(desc, descW), meta)
		if i == m.home.sel && m.home.pane == 0 {
			line = sSel.Render(fit(stripANSI(line), lipgloss.Width(line)))
		}
		l.WriteString(line + "\n")
	}
	if n := items[min(m.home.sel, len(items)-1)]; n != "" {
		p := m.cat.Plans[n].Value
		var c strings.Builder
		fmt.Fprintf(&c, "%s\n", planLine(p))
		fmt.Fprintf(&c, "keep testbed: %s   parallel: %d\n", p.Keep, p.Parallel)
		if last := m.svc.LastResult(n); last != nil {
			fmt.Fprintf(&c, "last: %s %s (%s)", glyph(last.Status), ui.Dur(last.Ms), last.ID)
		} else {
			c.WriteString("never run")
		}
		if est := engine.PastDurations(m.svc.Engine.Data, preview.PlanKey(n)); len(est) > 0 {
			pl := &preview.Preview{}
			pl.Estimate(est)
			fmt.Fprintf(&c, "   typical: ~%s", ui.Dur(pl.Needs.Estimate.Milliseconds()))
		}
		c.WriteString("\n\n  " + sKey.Render("[ enter  Run ]") + "   " + sKey.Render("[ e  Edit & run ]"))
		l.WriteString("\n" + sBox.Render(c.String()))
	}

	var r strings.Builder
	r.WriteString(sTitle.Render("ACTIVE") + "\n")
	active := m.svc.Active()
	if len(active) == 0 {
		r.WriteString(sMute.Render("nothing running") + "\n")
	}
	for _, run := range active {
		s := run.Summary()
		where := ""
		fmt.Fprintf(&r, "%s #%s  %s %s %s\n", glyph(s.Status), run.ID, fit(s.Label, 16), sMute.Render(where), ui.Dur(time.Since(s.Started).Milliseconds()))
		fmt.Fprintf(&r, "        %s  %d/%d\n", bar(s.Steps.Done, s.Steps.Total, 20, s.Steps.Failed+s.Steps.Errored > 0), s.Steps.Done, s.Steps.Total)
	}
	r.WriteString("\n" + sTitle.Render("RECENT") + "\n")
	shown := 0
	for _, s := range m.runs {
		if !s.Status.Done() || shown >= 5 {
			continue
		}
		shown++
		fmt.Fprintf(&r, "%s #%s  %s %6s  %s\n", glyph(s.Status), s.ID, fit(s.Label+comboCounts(s), 22), ui.Dur(s.Ms), sMute.Render(ui.Ago(s.Finished)))
	}
	att := m.attention()
	if len(att) > 0 || len(m.envs) > 0 {
		r.WriteString("\n" + statusStyle(event.Paused).Render("NEEDS ATTENTION") + "\n")
		for _, s := range att {
			fmt.Fprintf(&r, "%s #%s  paused on failure · environment kept\n", glyph(event.Paused), s.ID)
			r.WriteString(sMute.Render("        → enter to inspect, s for a shell") + "\n")
		}
		for _, e := range m.envs {
			if !e.Paused {
				fmt.Fprintf(&r, "%s %s/%s kept · %s\n", glyph(event.Passed), e.Execution, e.Combination, ui.Ago(e.Since))
			}
		}
	}
	if m.svc.WebURL != "" {
		r.WriteString("\n" + sMute.Render("web UI: "+m.svc.WebURL))
	}
	return m.panes("Start a run", l.String(), "", r.String(), h-1, m.home.pane) + "\n" +
		footer("enter", "run", "e", "edit & run", "tab", "next pane", "1-5", "tabs", "n", "new run", ":", "commands", "?", "help", "q", "quit")
}

func comboCounts(s *engine.Summary) string {
	if len(s.Combinations) < 2 {
		return ""
	}
	counts := map[event.Status]int{}
	for _, c := range s.Combinations {
		counts[c.Status]++
	}
	out := ""
	for _, st := range []event.Status{event.Passed, event.Failed, event.Errored, event.TimedOut} {
		if counts[st] > 0 {
			out += fmt.Sprintf(" %d%s", counts[st], vocab.StatusOf(st).Glyph)
		}
	}
	return out
}

func (m *model) firstLaunch(h int) string {
	var b strings.Builder
	b.WriteString("\n   " + sBold.Render("Welcome to Wendy.") + "\n\n")
	b.WriteString("   No config found yet. wendy looked in:\n     " + sMute.Render(strings.Join(m.cat.Dirs, "   ")) + "\n\n")
	b.WriteString("   You can still run the built-in plan right now:\n\n")
	desc := ""
	if p, ok := m.cat.Plans["quickstart"]; ok {
		desc = p.Value.Description
	}
	est := ""
	if d := engine.PastDurations(m.svc.Engine.Data, preview.PlanKey("quickstart")); len(d) > 0 {
		pl := &preview.Preview{}
		pl.Estimate(d)
		est = "   ≈ " + ui.Dur(pl.Needs.Estimate.Milliseconds())
	}
	b.WriteString("   " + sSel.Render("> quickstart") + "     " + sMute.Render(desc) + est + "\n\n")
	b.WriteString("   Or press  " + sKey.Render("i") + "  to create ./.wendy/wendy.cue with a commented starter config.\n")
	b.WriteString("   wendy reloads it automatically as you edit it.\n\n")
	b.WriteString("   Host check  ")
	if m.checks == nil {
		b.WriteString(sMute.Render("checking…"))
	}
	for _, c := range m.checks {
		switch {
		case c.OK:
			b.WriteString(glyph(event.Passed) + " " + c.Name + "   ")
		case c.Optional:
			b.WriteString(glyph(event.Failed) + " " + c.Name + sMute.Render(" ("+c.Detail+")") + "   ")
		default:
			b.WriteString(glyph(event.Failed) + " " + c.Name + "   ")
		}
	}
	m.home.sel = 0
	return box("", b.String(), m.w-2, h-2, false) + "\n" +
		footer("enter", "run quickstart", "i", "create starter config", "?", "help", "q", "quit")
}

// Runs.

type runsState struct {
	sel    int
	filter string
	typing bool
	status int
}

var statusFilters = []string{"all", "running", "failed", "errored", "passed"}

func (m *model) filteredRuns() []*engine.Summary {
	var out []*engine.Summary
	for _, r := range m.runs {
		if m.runsT.filter != "" && !strings.Contains("#"+r.ID+" "+r.Label, m.runsT.filter) {
			continue
		}
		switch statusFilters[m.runsT.status] {
		case "running":
			if r.Status.Done() {
				continue
			}
		case "failed":
			if r.Status != event.Failed {
				continue
			}
		case "errored":
			if r.Status != event.Errored && r.Status != event.TimedOut {
				continue
			}
		case "passed":
			if r.Status != event.Passed {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

func (m *model) updateRuns(k tea.KeyPressMsg) tea.Cmd {
	s := k.String()
	if m.runsT.typing {
		switch s {
		case "enter", "esc":
			m.runsT.typing = false
		case "backspace":
			if m.runsT.filter != "" {
				m.runsT.filter = m.runsT.filter[:len(m.runsT.filter)-1]
			}
		default:
			m.runsT.filter += k.Text
		}
		m.runsT.sel = 0
		return nil
	}
	runs := m.filteredRuns()
	var cur *engine.Summary
	if m.runsT.sel < len(runs) {
		cur = runs[m.runsT.sel]
	}
	switch {
	case up(s):
		m.runsT.sel = max(m.runsT.sel-1, 0)
	case down(s):
		m.runsT.sel = min(m.runsT.sel+1, max(len(runs)-1, 0))
	case s == "/":
		m.runsT.typing = true
	case s == "tab":
		m.runsT.status = (m.runsT.status + 1) % len(statusFilters)
		m.runsT.sel = 0
	case s == "esc":
		m.runsT.filter = ""
	case cur == nil:
	case s == "enter":
		m.openRun(cur.ID)
	case s == "R":
		r, err := m.svc.RunAgain(context.Background(), cur.ID, false)
		if err != nil {
			m.sayf("%v", err)
			return nil
		}
		m.openRun(r.ID)
	case s == "x":
		if err := m.svc.Cancel(cur.ID); err != nil {
			m.sayf("%v", err)
		}
	case s == "D":
		id := cur.ID
		m.confirm = &confirm{prompt: "Delete #" + id + " and its artifacts? (y/N)", yes: func() tea.Cmd {
			if err := m.svc.Delete(id); err != nil {
				m.sayf("%v", err)
			}
			m.refresh()
			return nil
		}}
	}
	return nil
}

func (m *model) runsView(h int) string {
	var b strings.Builder
	filter := m.runsT.filter
	if m.runsT.typing {
		filter += "▌"
	}
	b.WriteString(" / filter: " + fit(filter, 24) + "   status: ")
	for i, f := range statusFilters {
		if i == m.runsT.status {
			b.WriteString(sTabOn.Render(f))
		} else {
			b.WriteString(" " + f + " ")
		}
	}
	b.WriteString("     sort: newest\n\n")
	b.WriteString(sTitle.Render(fmt.Sprintf("  %-8s %-26s %-22s %-14s %-7s %s", "ID", "PLAN / SELECTION", "RESULT", "STARTED", "TOOK", "ENV")) + "\n")
	runs := m.filteredRuns()
	for i, r := range runs {
		if i >= h-6 {
			break
		}
		label := r.Label
		if r.Edited {
			label += " (edited)"
		}
		result := glyph(r.Status) + " " + word(r.Status)
		if !r.Status.Done() {
			result = glyph(r.Status) + fmt.Sprintf(" %d/%d %s", r.Steps.Done, r.Steps.Total, word(r.Status))
		}
		env := "—"
		for _, c := range r.Combinations {
			if c.Kept {
				env = "kept"
			}
			if c.Paused {
				env = "kept"
				result += " " + glyph(event.Paused) + " paused"
			}
		}
		took := ui.Dur(r.Ms)
		if !r.Status.Done() {
			took = ui.Dur(time.Since(r.Started).Milliseconds())
		}
		line := fmt.Sprintf("  %s %s %s %s %s %s", fit("#"+r.ID, 8), fit(label, 26), fit(result, 22), fit(started(r.Started), 14), fit(took, 7), env)
		if i == m.runsT.sel {
			line = sSel.Render(fit(stripANSI(line), m.w-2))
		}
		b.WriteString(line + "\n")
	}
	if len(runs) == 0 {
		b.WriteString(sMute.Render("  no runs yet: press n for a new run, or 1 for Home") + "\n")
	}
	return box("", b.String(), m.w-2, h-3, false) + "\n" +
		footer("enter", "open", "R", "re-run", "x", "cancel", "D", "delete run (asks first)", "/", "filter", "tab", "status", "?", "help")
}

func started(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if time.Since(t) < 24*time.Hour && t.Day() == time.Now().Day() {
		return t.Format("15:04") + " today"
	}
	return ui.Ago(t)
}

// Catalog.

type catalogState struct {
	tab    int
	sel    int
	filter string
	typing bool
}

var catalogTabs = []string{kindLabel(config.KindPlan), kindLabel(config.KindTestbed), kindLabel("suites"), kindLabel("tests"), kindLabel("steps")}

// The catalog's tabs, by their place in catalogTabs.
const (
	tabPlans = iota
	tabTestbeds
	tabSuites
	tabTests
	tabSteps
)

// kindLabel is the label vocab gives the kind with that ID.
func kindLabel(id string) string {
	k, _ := vocab.KindOf(id)
	return k.Label
}

type catItem struct {
	name, origin string
}

func (m *model) catalogItems() []catItem {
	c := m.cat
	var out []catItem
	add := func(name, origin string) {
		if m.catalog.filter == "" || strings.Contains(name, m.catalog.filter) {
			out = append(out, catItem{name, origin})
		}
	}
	switch m.catalog.tab {
	case tabPlans:
		for _, n := range config.Names(c.Plans) {
			add(n, c.Plans[n].Origin)
		}
	case tabTestbeds:
		for _, n := range config.Names(c.Testbeds) {
			add(n, c.Testbeds[n].Origin)
		}
	case tabSuites:
		for _, n := range config.Names(c.Suites) {
			add(n, c.Suites[n].Origin)
		}
	case tabTests:
		for _, n := range config.Names(c.Tests) {
			add(n, c.Tests[n].Origin)
		}
	case tabSteps:
		for _, s := range c.BuiltinSteps {
			add(s.ID, config.BuiltIn)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].origin < out[j].origin })
	return out
}

func (m *model) updateCatalog(k tea.KeyPressMsg) tea.Cmd {
	s := k.String()
	if m.catalog.typing {
		switch s {
		case "enter", "esc":
			m.catalog.typing = false
		case "backspace":
			if m.catalog.filter != "" {
				m.catalog.filter = m.catalog.filter[:len(m.catalog.filter)-1]
			}
		default:
			m.catalog.filter += k.Text
		}
		m.catalog.sel = 0
		return nil
	}
	items := m.catalogItems()
	switch {
	case up(s):
		m.catalog.sel = max(m.catalog.sel-1, 0)
	case down(s):
		m.catalog.sel = min(m.catalog.sel+1, max(len(items)-1, 0))
	case s == "left" || s == "h":
		m.catalog.tab = (m.catalog.tab + len(catalogTabs) - 1) % len(catalogTabs)
		m.catalog.sel = 0
	case s == "right" || s == "l":
		m.catalog.tab = (m.catalog.tab + 1) % len(catalogTabs)
		m.catalog.sel = 0
	case s == "/":
		m.catalog.typing = true
	case s == "esc":
		m.catalog.filter = ""
	case s == "a" && m.catalogKind() != "":
		return m.openEditor(m.catalogKind(), "", "new")
	case len(items) == 0:
	case s == "enter":
		return m.runCatalogItem(items[m.catalog.sel].name)
	case (s == "e" || s == "c" || s == "D") && m.catalogKind() != "":
		return m.editCatalogItem(s, items[m.catalog.sel].name)
	case s == "v":
		if src := m.catalogSource(items[m.catalog.sel].name); src != "" {
			f, err := os.CreateTemp("", "wendy-*.cue")
			if err != nil {
				return nil
			}
			_, _ = f.WriteString(src)
			f.Close()
			return page(f.Name())
		}
	}
	return nil
}

// editCatalogItem edits ("e"), copies ("c") or deletes ("D") the selected item.
func (m *model) editCatalogItem(key, name string) tea.Cmd {
	kind := m.catalogKind()
	switch key {
	case "e":
		return m.openEditor(kind, name, "edit")
	case "c":
		return m.openEditor(kind, name, "copy")
	}
	if !slices.Contains(m.svc.Owned(kind), name) {
		m.sayf("%s is not defined in %s, so it cannot be deleted here", name, config.UIFile)
		return nil
	}
	m.confirm = &confirm{
		prompt: fmt.Sprintf("Delete %s %s from %s? (y/N)", strings.TrimSuffix(kind, "s"), name, config.UIFile),
		yes: func() tea.Cmd {
			if err := m.svc.DeleteEntry(kind, name); err != nil {
				m.sayf("%v", err)
			}
			return nil
		},
	}
	return nil
}

// runCatalogItem opens New run pre-filled with the item.
func (m *model) runCatalogItem(name string) tea.Cmd {
	switch m.catalog.tab {
	case tabPlans:
		return m.newRun(preview.Selection{Plan: name}, name)
	case tabTestbeds:
		return m.newRun(preview.Selection{Testbed: name}, "")
	case tabSuites:
		return m.newRun(preview.Selection{Suites: []string{name}}, "")
	case tabTests:
		for _, sn := range config.Names(m.cat.Suites) {
			if slices.Contains(m.cat.Suites[sn].Value.Tests, name) {
				return m.newRun(preview.Selection{Suites: []string{sn}}, "")
			}
		}
	}
	return m.newRun(preview.Selection{}, "")
}

func (m *model) catalogSource(name string) string {
	c := m.cat
	switch m.catalog.tab {
	case tabPlans:
		return c.Plans[name].Source
	case tabTestbeds:
		return c.Testbeds[name].Source
	case tabSuites:
		return c.Suites[name].Source
	case tabTests:
		return c.Tests[name].Source
	}
	return ""
}

func (m *model) catalogDetail(name string) string {
	c := m.cat
	var b strings.Builder
	head := func(origin string) {
		b.WriteString(sBold.Render(name) + "   " + sMute.Render(origin) + "\n")
	}
	row := func(k, v string) { fmt.Fprintf(&b, "%s %s\n", fit(sMute.Render(k), 18), v) }
	switch m.catalog.tab {
	case tabPlans:
		p := c.Plans[name]
		head(p.Origin)
		b.WriteString(p.Value.Description + "\n\n")
		row("Testbed", planLine(p.Value))
		row("At once", vocab.Count(max(p.Value.Parallel, 1), "combination", "combinations"))
		row("When it finishes", vocab.Choice("keep", cmp.Or(p.Value.Keep, "never")))
		row("When a step fails", vocab.Choice("pauseOnFailure", strconv.FormatBool(p.Value.PauseOnFailure)))
		if p.Value.LabelFilter != "" {
			row("Labels", p.Value.LabelFilter)
		}
	case tabTestbeds:
		item := c.Testbeds[name]
		head(item.Origin)
		row("Parts", config.PartsLine(item.Value.PartNames(), config.PartKinds))
		if v := item.Resolved; v != nil {
			row("Axes", strings.Join(labelsOf(*v), " "))
			row("Management cluster", strings.TrimSpace(v.ManagementCluster.Type+" "+v.ManagementCluster.Image))
			row("Cluster API", v.ClusterAPI.Core)
			capt := v.ClusterAPI.CAPT.Version
			if v.ClusterAPI.CAPT.Source != nil {
				capt = "source " + v.ClusterAPI.CAPT.Source.Repo + " " + v.ClusterAPI.CAPT.Source.Ref
			}
			row("CAPT", capt)
			tink := v.Tinkerbell.Chart.Version
			if v.Tinkerbell.Source != nil {
				tink = "source " + v.Tinkerbell.Source.Repo + " " + v.Tinkerbell.Source.Ref
			}
			row("Tinkerbell", tink)
			row("Workload cluster", v.WorkloadCluster.KubernetesVersion+" · "+v.WorkloadCluster.CNI)
			var ms []string
			for _, r := range config.Names(v.Hardware.Machines) {
				ms = append(ms, fmt.Sprintf("%s %d", r, v.Hardware.Machines[r].Count))
			}
			row("Machines", strings.Join(ms, " · "))
			var setup []string
			for _, s := range v.Setup {
				setup = append(setup, s.DisplayName())
			}
			row("Setup", strings.Join(setup, " › "))
		}
		if item.HasMatrix() {
			row("Matrix", strings.Join(item.Value.Matrix.Order, " × "))
			b.WriteString("\n")
			for _, co := range item.Combinations {
				note := ""
				if co.Excluded != "" {
					note = sMute.Render(" excluded: " + co.Excluded)
				}
				if co.Err != "" {
					note = glyph(event.Errored) + " " + firstLine(co.Err)
				}
				last := ""
				for _, r := range m.runs {
					for _, rc := range r.Combinations {
						if rc.Name == co.Name && last == "" && r.Status.Done() {
							last = glyph(rc.Status)
						}
					}
				}
				fmt.Fprintf(&b, "  %s %s%s\n", fit(last, 1), co.Name, note)
			}
		}
	case tabSuites:
		s := c.Suites[name]
		head(s.Origin)
		b.WriteString(s.Value.Description + "\n\n")
		row("Tests", strings.Join(s.Value.Tests, " › "))
	case tabTests:
		s := c.Tests[name]
		head(s.Origin)
		b.WriteString(s.Value.Description + "\n\n")
		for i, st := range s.Value.Steps {
			detail := st.Kind()
			if st.Uses != "" && len(st.With) > 0 {
				detail = fmt.Sprintf("%v", st.With)
			}
			row(fmt.Sprintf("Step %d", i+1), fmt.Sprintf("%s  %s  timeout %s", st.DisplayName(), sMute.Render(detail), st.Timeout))
		}
		if s.Value.Requires.Spare > 0 {
			row("Needs", vocab.Count(s.Value.Requires.Spare, "spare machine", "spare machines"))
		}
		if len(s.Value.Labels) > 0 {
			row("Labels", strings.Join(s.Value.Labels, " "))
		}
		if s.Value.Once {
			row("Once", "runs once per testbed; later suites reuse it")
		}
		var used []string
		for _, sn := range config.Names(c.Suites) {
			if slices.Contains(c.Suites[sn].Value.Tests, name) {
				used = append(used, sn)
			}
		}
		row("Used by", "suites: "+strings.Join(used, ", "))
	case tabSteps:
		for _, s := range c.BuiltinSteps {
			if s.ID == name {
				head("built-in")
				b.WriteString(s.Description + "\n\n")
				row("Params", s.Schema)
			}
		}
	}
	b.WriteString("\n" + sKey.Render("[ enter  Run this… ]") + "   " + sKey.Render("[ v  View CUE source ]"))
	if m.catalogKind() != "" {
		if slices.Contains(m.svc.Owned(m.catalogKind()), name) {
			b.WriteString("   " + sKey.Render("[ e  Edit ]"))
		} else {
			b.WriteString("   " + sKey.Render("[ c  Copy to edit ]"))
		}
	}
	return b.String()
}

func labelsOf(e config.Testbed) []string {
	vals := e.AxisValues()
	var out []string
	for _, k := range config.Names(vals) {
		out = append(out, vals[k])
	}
	return out
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}

func (m *model) catalogView(h int) string {
	var tabs []string
	for i, t := range catalogTabs {
		if i == m.catalog.tab {
			tabs = append(tabs, sTabOn.Render(t))
		} else {
			tabs = append(tabs, sTab.Render(t))
		}
	}
	filter := m.catalog.filter
	if m.catalog.typing {
		filter += "▌"
	}
	top := " " + strings.Join(tabs, " ") + "      / filter: " + filter + "\n"
	items := m.catalogItems()
	var l strings.Builder
	lastOrigin := ""
	for i, it := range items {
		if it.origin != lastOrigin {
			l.WriteString(sMute.Render(it.origin) + "\n")
			lastOrigin = it.origin
		}
		line := "  " + it.name
		if i == m.catalog.sel {
			line = sSel.Render("> " + it.name)
		}
		l.WriteString(line + "\n")
	}
	detail := ""
	if m.catalog.sel < len(items) {
		detail = m.catalogDetail(items[m.catalog.sel].name)
	}
	lw := min(34, m.w/3)
	body := lipgloss.JoinHorizontal(lipgloss.Top, box("", l.String(), lw, h-4, true), box("", detail, m.w-lw-4, h-4, false))
	foot := footer("↑↓", "select", "←→", "tabs", "/", "filter", "enter", "run…", "v", "view source", "?", "help")
	if m.catalogKind() != "" {
		foot = footer("↑↓", "select", "←→", "tabs", "/", "filter", "enter", "run…", "a", "add", "e", "edit", "c", "copy", "D", "delete", "?", "help")
	}
	return top + body + "\n" + foot
}

// Environments.

func (m *model) updateEnvs(k tea.KeyPressMsg) tea.Cmd {
	s := k.String()
	if len(m.envs) == 0 {
		return nil
	}
	m.envT.sel = min(m.envT.sel, len(m.envs)-1)
	e := m.envs[m.envT.sel]
	switch {
	case up(s):
		m.envT.sel = max(m.envT.sel-1, 0)
	case down(s):
		m.envT.sel = min(m.envT.sel+1, len(m.envs)-1)
	case s == "enter":
		m.openRun(e.Execution)
	case s == "s":
		return shell(e.Dir)
	case s == "k":
		m.sayf("copied: %s", kubeconfigExport(e.Dir))
		return tea.SetClipboard(kubeconfigExport(e.Dir))
	case s == "o" && len(e.State.Machines) > 0:
		return page(filepath.Join(e.Dir, "consoles", e.State.Machines[0].Name+".log"))
	case s == "D":
		run, combo := e.Execution, e.Combination
		m.confirm = &confirm{prompt: fmt.Sprintf("Tear down %s/%s? This deletes its clusters and machines. (y/N)", run, combo), yes: func() tea.Cmd {
			m.sayf("tearing down %s/%s…", run, combo)
			p := m.p
			go func() {
				err := m.svc.TearDown(context.Background(), run, combo, nil)
				if err != nil {
					p.Send(toastMsg(fmt.Sprintf("tear down %s/%s: %v", run, combo, err)))
					return
				}
				p.Send(toastMsg(fmt.Sprintf("tore down %s/%s", run, combo)))
			}()
			return nil
		}}
	}
	return nil
}

func (m *model) envsView(h int) string {
	var l strings.Builder
	for i, e := range m.envs {
		g, state := glyph(event.Passed), "kept · "+ui.Ago(e.Since)
		if e.Paused {
			g, state = glyph(event.Paused), "paused · since "+e.Since.Format("15:04")
		}
		line := fmt.Sprintf("%s #%s %s\n    %s · %s", g, e.Execution, e.Combination, state, e.Label)
		if i == m.envT.sel {
			line = sSel.Render(fmt.Sprintf("> %s #%s %s", vocab.StatusOf(e.Status).Glyph, e.Execution, e.Combination)) + "\n    " + state + " · " + e.Label
		}
		l.WriteString(line + "\n")
	}
	if len(m.envs) == 0 {
		l.WriteString(sMute.Render("No kept or paused environments.\n\nA run keeps its environment with\nkeep: failed or always, and while\nit is paused on a failure."))
	}
	var r strings.Builder
	if m.envT.sel < len(m.envs) {
		e := m.envs[m.envT.sel]
		st := e.State
		row := func(k, v string) { fmt.Fprintf(&r, "%s %s\n", fit(sMute.Render(k), 11), v) }
		r.WriteString(sBold.Render("#"+e.Execution+" · "+e.Combination) + "\n\n")
		row("Clusters", "management  kind "+st.ID)
		if _, err := os.Stat(filepath.Join(e.Dir, "kubeconfigs", engine.WorkloadCluster)); err == nil {
			row("", "workload    "+st.WorkloadCluster.Name+"  ("+st.WorkloadCluster.KubernetesVersion+")")
		}
		row("Tinkerbell", st.VIPs.Tinkerbell+"   chart "+st.Versions["tinkerbell"])
		for i, mc := range st.Machines {
			k := ""
			if i == 0 {
				k = "Machines"
			}
			row(k, fmt.Sprintf("%s  %-13s %s", mc.Name, mc.Role, mc.IP))
		}
		row("BMC", fmt.Sprintf("%s  https://%s:443/redfish/v1/Systems/<machine>", st.BMC.Protocol, st.BMC.Host))
		row("Kubeconfigs", filepath.Join(e.Dir, "kubeconfigs"))
		r.WriteString("\n" + sKey.Render("[ s Shell ]") + "  " + sKey.Render("[ k Copy KUBECONFIG export ]") + "  " + sKey.Render("[ D Tear down ]"))
	}
	return m.panes("", l.String(), "", r.String(), h-1, 0) + "\n" +
		footer("↑↓", "select", "enter", "open run", "s", "shell", "k", "copy kubeconfig", "o", "console", "D", "tear down (asks first)", "?", "help")
}

// Config.

func (m *model) updateConfig(k tea.KeyPressMsg) tea.Cmd {
	s := k.String()
	files := m.cat.Files
	switch {
	case up(s):
		m.cfg.sel = max(m.cfg.sel-1, 0)
	case down(s):
		m.cfg.sel = min(m.cfg.sel+1, max(len(files)-1, 0))
	case s == "r":
		m.svc.Reload()
		m.refresh()
		m.sayf("reloaded")
	case s == "enter" && m.cfg.sel < len(files):
		f := files[m.cfg.sel]
		line := 0
		if len(f.Errors) > 0 {
			line = f.Errors[0].Line
		}
		return edit(expand(f.Path), line)
	}
	return nil
}

func expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

func (m *model) configView(h int) string {
	c := m.cat
	var b strings.Builder
	errs := c.AllErrors()
	status := fmt.Sprintf("last reload %s", c.LoadedAt.Format("15:04:05"))
	if len(errs) > 0 {
		status += lipgloss.NewStyle().Foreground(cFail).Render(fmt.Sprintf(" (%s)", vocab.Count(len(errs), "error", "errors")))
	}
	b.WriteString("Watching  " + sMute.Render(strings.Join(c.Dirs, "  ")) + "\n" + status + "\n\n")
	if len(c.Files) == 0 {
		b.WriteString(sMute.Render("No config files yet. Home shows how to create a starter config.") + "\n")
	}
	for i, f := range c.Files {
		g := glyph(event.Passed)
		if len(f.Errors) > 0 {
			g = glyph(event.Failed)
		}
		line := g + " " + f.Path
		if i == m.cfg.sel {
			line = sSel.Render("> " + f.Path)
		}
		b.WriteString(line + "\n")
		for _, e := range f.Errors {
			fmt.Fprintf(&b, "    %s  %s: %s\n", sBold.Render(fmt.Sprintf("%d:%d", e.Line, e.Col)), e.Path, e.Msg)
		}
	}
	for _, e := range c.Errors {
		b.WriteString(glyph(event.Failed) + " " + e.Error() + "\n")
	}
	if len(errs) > 0 {
		b.WriteString("\nThe last valid config is still in use, so everything else keeps working.\nFix the file and save. wendy reloads it automatically.\n")
	}
	return box("", b.String(), m.w-2, h-3, false) + "\n" +
		footer("↑↓", "select", "enter", "open file in $EDITOR", "r", "reload now", "?", "help")
}
