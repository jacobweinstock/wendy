// Package tui is wendy's terminal UI: the same screens as the web UI, over
// the same Service.
package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/hostcheck"
	"github.com/tinkerbell/wendy/internal/preview"
	"github.com/tinkerbell/wendy/internal/service"
	"github.com/tinkerbell/wendy/internal/vocab"
)

// Options say what the TUI opens on.
type Options struct {
	// RunID opens that run's screen.
	RunID string
	// ExitWhenDone quits once that run finishes.
	ExitWhenDone bool
}

const (
	tabHome = iota
	tabRuns
	tabCatalog
	tabEnvs
	tabConfig
)

var tabNames = []string{"Home", "Runs", "Catalog", "Environments", "Config"}

type (
	changeMsg   service.Change
	tickMsg     time.Time
	runEventMsg struct {
		id string
		e  event.Event
	}
	runEndMsg  struct{ id string }
	doneMsg    struct{ err error }
	toastMsg   string
	previewMsg struct {
		gen int
		pv  *service.Preview
		err error
	}
)

type model struct {
	svc  *service.Service
	opts Options
	p    *tea.Program
	ctx  context.Context

	w, h int
	tab  int

	cat    *config.Catalog
	runs   []*engine.Summary
	envs   []service.KeptTestbed
	checks []hostcheck.Check

	home    homeState
	runsT   runsState
	catalog catalogState
	envT    listState
	cfg     listState

	run     *runScreen
	form    *newRun
	edit    *editor
	palette *palette
	help    bool
	confirm *confirm
	toast   string
	toastAt time.Time
}

type listState struct{ sel int }

type confirm struct {
	prompt string
	yes    func() tea.Cmd
}

// Run shows the TUI until the user quits.
func Run(ctx context.Context, svc *service.Service, opts Options) error {
	m := &model{svc: svc, opts: opts, ctx: ctx, w: 108, h: 32}
	m.refresh()
	p := tea.NewProgram(m, tea.WithContext(ctx))
	m.p = p
	ch, cancel := svc.Subscribe()
	defer cancel()
	go func() {
		for c := range ch {
			p.Send(changeMsg(c))
		}
	}()
	go func() {
		checks := svc.FirstChecks(ctx)
		p.Send(checksMsg(checks))
	}()
	if opts.RunID != "" {
		m.openRun(opts.RunID)
	}
	_, err := p.Run()
	if m.run != nil {
		m.run.close()
	}
	if err != nil && ctx.Err() != nil {
		return nil
	}
	return err
}

type checksMsg []hostcheck.Check

func (m *model) refresh() {
	m.cat = m.svc.Catalog()
	m.runs = m.svc.Executions()
	m.envs = m.svc.KeptTestbeds()
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *model) Init() tea.Cmd { return tick() }

func (m *model) sayf(format string, args ...any) {
	m.toast, m.toastAt = fmt.Sprintf(format, args...), time.Now()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.resizeForms()
		return m, nil
	case checksMsg:
		m.checks = msg
		return m, nil
	case changeMsg:
		m.refresh()
		if m.form != nil && msg.Catalog {
			return m, m.form.replan(m)
		}
		return m, nil
	case tickMsg:
		if time.Since(m.toastAt) > 6*time.Second {
			m.toast = ""
		}
		return m, tick()
	case toastMsg:
		m.sayf("%s", string(msg))
		m.refresh()
		return m, nil
	case doneMsg:
		if msg.err != nil {
			m.sayf("%v", msg.err)
		}
		m.refresh()
		return m, nil
	case runEventMsg:
		if m.run != nil && m.run.id == msg.id {
			m.run.apply(msg.e)
		}
		return m, nil
	case runEndMsg:
		if m.opts.ExitWhenDone && m.opts.RunID == msg.id {
			return m, tea.Quit
		}
		if m.run != nil && m.run.id == msg.id {
			m.run.settle(m)
		}
		return m, nil
	case previewMsg:
		if m.form != nil {
			m.form.setPreview(msg)
		}
		return m, nil
	}

	if m.edit != nil {
		return m, m.edit.update(m, msg)
	}
	if m.form != nil {
		return m, m.form.update(m, msg)
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	k := key.String()
	switch {
	case m.confirm != nil:
		c := m.confirm
		m.confirm = nil
		if k == "y" || k == "Y" {
			return m, c.yes()
		}
		return m, nil
	case m.help:
		m.help = false
		return m, nil
	case m.palette != nil:
		return m, m.palette.update(m, key)
	}
	if k == "ctrl+c" {
		return m, m.quit()
	}
	if m.run != nil {
		return m, m.run.update(m, key)
	}
	if m.filtering() {
		return m, m.updateTab(key)
	}
	switch k {
	case "q":
		return m, m.quit()
	case "?":
		m.help = true
		return m, nil
	case ":":
		m.palette = &palette{}
		return m, nil
	case "n":
		return m, m.newRun(preview.Selection{}, "")
	case "1", "2", "3", "4", "5":
		m.tab = int(k[0] - '1')
		return m, nil
	}
	return m, m.updateTab(key)
}

func (m *model) filtering() bool {
	return (m.tab == tabRuns && m.runsT.typing) || (m.tab == tabCatalog && m.catalog.typing)
}

func (m *model) updateTab(k tea.KeyPressMsg) tea.Cmd {
	switch m.tab {
	case tabHome:
		return m.updateHome(k)
	case tabRuns:
		return m.updateRuns(k)
	case tabCatalog:
		return m.updateCatalog(k)
	case tabEnvs:
		return m.updateEnvs(k)
	case tabConfig:
		return m.updateConfig(k)
	}
	return nil
}

// quit asks first while runs are active: quitting cancels and tears them down.
func (m *model) quit() tea.Cmd {
	if n := len(m.svc.Active()); n > 0 {
		m.confirm = &confirm{
			prompt: fmt.Sprintf("%s still running. Quit, cancel and tear down? (y/N)", vocab.Count(n, "run is", "runs are")),
			yes:    func() tea.Cmd { return tea.Quit },
		}
		return nil
	}
	return tea.Quit
}

// start runs a selection and opens its screen.
func (m *model) start(sel preview.Selection) tea.Cmd {
	r, err := m.svc.Start(context.Background(), sel)
	if err != nil {
		m.sayf("%v", err)
		return nil
	}
	m.form = nil
	m.refresh()
	m.openRun(r.ID)
	return nil
}

func (m *model) openRun(id string) {
	if m.run != nil {
		m.run.close()
	}
	rs, err := newRunScreen(m, id)
	if err != nil {
		m.sayf("%v", err)
		return
	}
	m.run = rs
}

// shell suspends the TUI for $SHELL in an environment's directory, with
// KUBECONFIG set to its management cluster.
func shell(dir string) tea.Cmd {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	cmd := exec.Command(sh) //nolint:gosec,noctx // Running the user's own $SHELL in the suspended terminal is the point.
	cmd.Dir = dir
	kc := dir + "/kubeconfigs/"
	cmd.Env = append(os.Environ(),
		"KUBECONFIG="+kc+engine.ManagementCluster,
		"KUBECONFIG_WORKLOAD="+kc+engine.WorkloadCluster,
		"KUBECONFIG_TINKERBELL="+kc+engine.TinkerbellCluster,
	)
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return doneMsg{err} })
}

// page opens a file in $PAGER.
func page(path string) tea.Cmd {
	pager := os.Getenv("PAGER")
	if pager == "" {
		pager = "less"
	}
	args := strings.Fields(pager)
	//nolint:gosec,noctx // Running the user's own $PAGER in the suspended terminal is the point.
	return tea.ExecProcess(exec.Command(args[0], append(args[1:], path)...), func(err error) tea.Msg { return doneMsg{err} })
}

// edit opens a file at a line in $EDITOR.
func edit(path string, line int) tea.Cmd {
	ed := os.Getenv("EDITOR")
	if ed == "" {
		ed = "vi"
	}
	args := strings.Fields(ed)
	if line > 0 {
		args = append(args, fmt.Sprintf("+%d", line))
	}
	//nolint:gosec,noctx // Running the user's own $EDITOR in the suspended terminal is the point.
	return tea.ExecProcess(exec.Command(args[0], append(args[1:], path)...), func(err error) tea.Msg { return doneMsg{err} })
}

// kubeconfigExport is the shell line that points kubectl at an environment.
func kubeconfigExport(dir string) string {
	return "export KUBECONFIG=" + dir + "/kubeconfigs/" + engine.ManagementCluster
}

func (m *model) View() tea.View {
	var body string
	switch {
	case m.edit != nil:
		body = m.edit.view(m)
	case m.form != nil:
		body = m.form.view(m)
	case m.run != nil:
		body = m.run.view(m)
	default:
		body = m.header() + "\n" + m.tabView()
	}
	if m.palette != nil {
		body = overlay(body, m.palette.view(m), m.w, m.h)
	}
	if m.help {
		body = overlay(body, m.helpView(), m.w, m.h)
	}
	if m.confirm != nil {
		body = overlay(body, sBox.BorderForeground(cErr).Render(m.confirm.prompt), m.w, m.h)
	}
	if m.toast != "" {
		lines := strings.Split(body, "\n")
		for len(lines) < m.h {
			lines = append(lines, "")
		}
		lines[m.h-1] = lipgloss.NewStyle().Foreground(cErr).Render(" " + fit(m.toast, m.w-2))
		body = strings.Join(lines[:m.h], "\n")
	}
	v := tea.NewView(body)
	v.AltScreen = true
	return v
}

func (m *model) header() string {
	var tabs []string
	for i, n := range tabNames {
		label := fmt.Sprintf("%d %s", i+1, n)
		if i == tabConfig {
			if errs := len(m.cat.AllErrors()); errs > 0 {
				label += fmt.Sprintf(" !%d", errs)
			}
		}
		if i == m.tab {
			tabs = append(tabs, sTabOn.Render(label))
		} else {
			tabs = append(tabs, sTab.Render(label))
		}
	}
	left := sBold.Render("◆ Wendy") + "   " + strings.Join(tabs, " ")
	right := ""
	if n := len(m.svc.Active()); n > 0 {
		right = statusStyle(event.Running).Render(fmt.Sprintf("%s %d %s", vocab.StatusOf(event.Running).Glyph, n, word(event.Running)))
	}
	gap := max(m.w-lipgloss.Width(left)-lipgloss.Width(right)-1, 1)
	return left + strings.Repeat(" ", gap) + right
}

func (m *model) tabView() string {
	h := m.h - 2
	switch m.tab {
	case tabHome:
		return m.homeView(h)
	case tabRuns:
		return m.runsView(h)
	case tabCatalog:
		return m.catalogView(h)
	case tabEnvs:
		return m.envsView(h)
	case tabConfig:
		return m.configView(h)
	}
	return ""
}

// overlay centres a box over the screen.
func overlay(base, box string, w, h int) string {
	lines := strings.Split(base, "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}
	bl := strings.Split(box, "\n")
	top := max((h-len(bl))/3, 1)
	left := max((w-lipgloss.Width(box))/2, 0)
	for i, l := range bl {
		if top+i >= len(lines) {
			break
		}
		lines[top+i] = strings.Repeat(" ", left) + l
	}
	return strings.Join(lines, "\n")
}

func (m *model) helpView() string {
	keys := [][2]string{
		{"1-5", "switch tabs"},
		{"↑↓ j/k", "move"},
		{"enter", "open or run"},
		{"esc", "back"},
		{"/", "filter"},
		{":", "command palette"},
		{"n", "new run"},
		{"?", "this help"},
		{"q", "quit (asks while runs are active)"},
	}
	switch {
	case m.run != nil:
		keys = append(keys, [][2]string{
			{"f", "follow the running step"},
			{"→ ←", "expand / collapse"},
			{"g t", "grid / tree"},
			{"l", "full log in $PAGER"},
			{"a", "artifacts"},
			{"o", "console"},
			{"k", "copy KUBECONFIG export"},
			{"s", "shell"},
			{"r", "retry step (paused)"},
			{"c", "carry on (paused)"},
			{"x", "cancel run / stop (paused)"},
			{"R", "re-run (failed combos in grid)"},
			{"w T b d", "diagnostics: workflow, Tinkerbell logs, BMC log, cluster dumps"},
		}...)
	case m.tab == tabRuns:
		keys = append(keys, [][2]string{{"R", "re-run"}, {"x", "cancel"}, {"D", "delete run (asks)"}, {"tab", "status filter"}}...)
	case m.tab == tabEnvs:
		keys = append(keys, [][2]string{{"s", "shell"}, {"k", "copy KUBECONFIG export"}, {"o", "console"}, {"D", "tear down (asks)"}}...)
	case m.tab == tabConfig:
		keys = append(keys, [][2]string{{"enter", "open in $EDITOR"}, {"r", "reload now"}}...)
	case m.tab == tabCatalog:
		keys = append(keys, [][2]string{{"← →", "tabs"}, {"v", "view CUE source"}, {"a", "add (tests, suites)"}, {"e", "edit one defined in the UI"}, {"c", "copy into the UI file to edit"}, {"D", "delete one defined in the UI (asks)"}}...)
	case m.tab == tabHome:
		keys = append(keys, [][2]string{{"e", "edit & run"}, {"tab", "next pane"}, {"i", "create starter config"}}...)
	}
	var b strings.Builder
	b.WriteString(sBold.Render("Keys") + "\n\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "  %s %s\n", fit(sKey.Render(k[0]), 10), k[1])
	}
	b.WriteString("\n" + sMute.Render("any key closes"))
	return sBox.Render(b.String())
}

// palette is the command palette: a fuzzy search over actions.
type palette struct {
	query string
	sel   int
}

type action struct {
	label, kind string
	run         func(m *model) tea.Cmd
}

func (p *palette) actions(m *model) []action {
	var out []action
	for _, n := range config.Names(m.cat.Plans) {
		out = append(out, action{"run " + n, "start a plan", func(m *model) tea.Cmd { return m.start(preview.Selection{Plan: n}) }})
	}
	for i, r := range m.runs {
		if i >= 10 {
			break
		}
		id := r.ID
		out = append(out,
			action{"rerun #" + id, "re-run with the same selection", func(m *model) tea.Cmd {
				run, err := m.svc.RunAgain(context.Background(), id, false)
				if err != nil {
					m.sayf("%v", err)
					return nil
				}
				m.openRun(run.ID)
				return nil
			}},
			action{"open #" + id, r.Label, func(m *model) tea.Cmd { m.openRun(id); return nil }})
	}
	for i, n := range tabNames {
		out = append(out, action{strings.ToLower(n), "go to " + n, func(m *model) tea.Cmd { m.tab = i; return nil }})
	}
	out = append(out,
		action{"new run", "pick what to run", func(m *model) tea.Cmd { return m.newRun(preview.Selection{}, "") }},
		action{"new test", "a list of steps run in order", func(m *model) tea.Cmd { return m.openEditor(config.KindTest, "", "new") }},
		action{"new suite", "tests run in order", func(m *model) tea.Cmd { return m.openEditor(config.KindSuite, "", "new") }},
		action{"reload", "re-read the config", func(m *model) tea.Cmd { m.svc.Reload(); return nil }})
	var match []action
	for _, a := range out {
		if fuzzy(p.query, a.label) {
			match = append(match, a)
		}
	}
	sort.SliceStable(match, func(i, j int) bool {
		return strings.HasPrefix(match[i].label, p.query) && !strings.HasPrefix(match[j].label, p.query)
	})
	return match
}

func fuzzy(q, s string) bool {
	q, s = strings.ToLower(q), strings.ToLower(s)
	i := 0
	for _, r := range s {
		if i < len(q) && rune(q[i]) == r {
			i++
		}
	}
	return i == len(q)
}

func (p *palette) update(m *model, k tea.KeyPressMsg) tea.Cmd {
	acts := p.actions(m)
	switch k.String() {
	case "esc":
		m.palette = nil
	case "enter":
		m.palette = nil
		if p.sel < len(acts) {
			return acts[p.sel].run(m)
		}
	case "up", "ctrl+p":
		p.sel = max(p.sel-1, 0)
	case "down", "ctrl+n":
		p.sel = min(p.sel+1, max(len(acts)-1, 0))
	case "backspace":
		if p.query != "" {
			p.query = p.query[:len(p.query)-1]
		}
		p.sel = 0
	default:
		if t := k.Text; t != "" {
			p.query += t
			p.sel = 0
		}
	}
	return nil
}

func (p *palette) view(m *model) string {
	var b strings.Builder
	b.WriteString(":" + p.query + "▌\n" + sMute.Render(strings.Repeat("─", 56)) + "\n")
	for i, a := range p.actions(m) {
		if i >= 8 {
			break
		}
		if i == p.sel {
			b.WriteString(sSel.Render("> "+fit(a.label, 26)+"  "+fit(a.kind, 28)) + "\n")
		} else {
			b.WriteString("  " + fit(a.label, 26) + "  " + sMute.Render(fit(a.kind, 28)) + "\n")
		}
	}
	b.WriteString("\n" + sMute.Render("Everywhere:  ? help   : commands   / filter   1-5 tabs\n             esc back   q quit (asks if runs are active)"))
	return sBox.Width(62).Render(b.String())
}
