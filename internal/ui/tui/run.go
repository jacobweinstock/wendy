package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/ui"
	"github.com/tinkerbell/wendy/internal/vocab"
)

// runScreen is one run: tree, details and, on failure, the failure card.
type runScreen struct {
	id       string
	rv       *ui.ExecutionView
	sel      string
	follow   bool
	expanded map[string]bool
	grid     bool
	gridSel  int
	cancel   context.CancelFunc
}

func newRunScreen(m *model, id string) (*runScreen, error) {
	sum, err := m.svc.Execution(id)
	if err != nil {
		return nil, err
	}
	v := ui.New(*sum, m.svc.ExecutionDir(id))
	rs := &runScreen{id: id, rv: v, follow: true, expanded: map[string]bool{}, grid: len(sum.Combinations) > 1}
	sub, err := m.svc.Events(id, 0)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	rs.cancel = func() { cancel(); sub.Close() }
	p := m.p
	go func() {
		for {
			e, ok := sub.Next(ctx)
			if !ok {
				if ctx.Err() == nil && p != nil {
					p.Send(runEndMsg{id: id})
				}
				return
			}
			if p != nil {
				p.Send(runEventMsg{id: id, e: e})
			}
		}
	}()
	return rs, nil
}

func (rs *runScreen) close() {
	if rs.cancel != nil {
		rs.cancel()
	}
}

// settle selects what matters once a run has finished: its failure, else
// its first combination.
func (rs *runScreen) settle(m *model) {
	rs.follow = false
	if combo, f, _ := rs.failure(m); f != nil {
		for k, n := range rs.rv.ByKey {
			if n.Combination == combo && strings.Join(n.Path, "/") == strings.Join(f.Path, "/") {
				rs.sel = k
				return
			}
		}
	}
	if len(rs.rv.Root) > 0 {
		rs.sel = rs.rv.Root[len(rs.rv.Root)-1].Key
		if len(rs.rv.Root) > 1 && rs.rv.Root[0].Kind == ui.KindBuild {
			rs.sel = rs.rv.Root[1].Key
		}
	}
}

func (rs *runScreen) apply(e event.Event) {
	rs.rv.Apply(e)
	if rs.follow && rs.rv.Running != "" {
		rs.sel = rs.rv.Running
	}
}

func (rs *runScreen) summary(m *model) *engine.Summary {
	for _, s := range m.runs {
		if s.ID == rs.id {
			return s
		}
	}
	s, err := m.svc.Execution(rs.id)
	if err != nil {
		return &rs.rv.Summary
	}
	return s
}

// failure is the combination failure to show: a paused one first.
func (rs *runScreen) failure(m *model) (string, *engine.Failure, bool) {
	for combo, f := range rs.rv.Paused {
		return combo, f, true
	}
	for _, c := range rs.summary(m).Combinations {
		if c.Failure != nil {
			return c.Name, c.Failure, c.Paused
		}
	}
	return "", nil, false
}

func (rs *runScreen) rows() []ui.Row { return rs.rv.Rows(rs.expanded) }

func (rs *runScreen) selIndex(rows []ui.Row) int {
	for i, r := range rows {
		if r.Key == rs.sel {
			return i
		}
	}
	return 0
}

func (rs *runScreen) node() *ui.Node {
	if n := rs.rv.ByKey[rs.sel]; n != nil {
		return n
	}
	rows := rs.rows()
	if len(rows) > 0 {
		return rows[0].Node
	}
	return nil
}

func (rs *runScreen) comboDir(m *model, combo string) string {
	for _, c := range rs.summary(m).Combinations {
		if c.Name == combo || combo == "" {
			return filepath.Join(m.svc.ExecutionDir(rs.id), c.Dir)
		}
	}
	return m.svc.ExecutionDir(rs.id)
}

// update handles a key on the run screen. It is one flat switch over keys
// so the whole keymap reads in one place.
func (rs *runScreen) update(m *model, k tea.KeyPressMsg) tea.Cmd { //nolint:cyclop // See above.
	s := k.String()
	rows := rs.rows()
	i := rs.selIndex(rows)
	combo, failure, paused := rs.failure(m)
	cur := rs.node()
	if cur != nil && cur.Combination != "" {
		combo = cur.Combination
	}
	dir := rs.comboDir(m, combo)
	if rs.grid {
		sum := rs.summary(m)
		switch {
		case up(s):
			rs.gridSel = max(rs.gridSel-1, 0)
			return nil
		case down(s):
			rs.gridSel = min(rs.gridSel+1, max(len(sum.Combinations)-1, 0))
			return nil
		case s == "enter" && rs.gridSel < len(sum.Combinations):
			rs.grid, rs.follow = false, false
			name := sum.Combinations[rs.gridSel].Name
			for _, r := range rs.rv.Root {
				rs.expanded[r.Key] = r.Combination == name
				if r.Combination == name {
					rs.sel = r.Key
				}
			}
			return nil
		}
	}
	switch {
	case s == "esc":
		rs.close()
		m.run = nil
		m.refresh()
	case s == "q":
		return m.quit()
	case s == "?":
		m.help = true
	case s == ":":
		m.palette = &palette{}
	case up(s) && len(rows) > 0:
		rs.follow, rs.sel = false, rows[max(i-1, 0)].Key
	case down(s) && len(rows) > 0:
		rs.follow, rs.sel = false, rows[min(i+1, len(rows)-1)].Key
	case s == "right" && cur != nil:
		rs.expanded[cur.Key] = true
	case s == "left" && cur != nil:
		if len(rows) > 0 && rows[i].Expanded && rows[i].HasKids {
			rs.expanded[cur.Key] = false
		}
	case s == "f":
		rs.follow = true
		if rs.rv.Running != "" {
			rs.sel = rs.rv.Running
		}
	case s == "g":
		rs.grid = true
	case s == "t":
		rs.grid = false
	case s == "l" && cur != nil && cur.Dir != "":
		return page(filepath.Join(m.svc.ExecutionDir(rs.id), cur.Dir, "step.log"))
	case s == "a" && cur != nil:
		return pageList(m.svc.ExecutionDir(rs.id), cur.Files)
	case s == "o":
		return console(dir, failure)
	case s == "k":
		m.sayf("copied: %s", kubeconfigExport(dir))
		return tea.SetClipboard(kubeconfigExport(dir))
	case s == "s":
		return shell(dir)
	case s == "w" || s == "b" || s == "T":
		kind := map[string]string{"w": "workflow", "b": "bmc", "T": "logs"}[s]
		if failure != nil && failure.Diagnostics[kind] != "" {
			p := filepath.Join(m.svc.ExecutionDir(rs.id), failure.Diagnostics[kind])
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				return pageDir(p)
			}
			return page(p)
		}
		m.sayf("no %s in the diagnostics", kind)
	case s == "d":
		if failure != nil {
			if files := dumpFiles(m.svc.ExecutionDir(rs.id), failure.Diagnostics); len(files) > 0 {
				return pageList(m.svc.ExecutionDir(rs.id), files)
			}
		}
		m.sayf("no cluster dump in the diagnostics")
	case s == "r" && paused:
		return decide(m, rs.id, combo, engine.Retry)
	case s == "c" && paused:
		return decide(m, rs.id, combo, engine.Continue)
	case s == "x" && paused:
		id := rs.id
		prompt := "Stop " + combo + " and tear down its testbed? (y/N)"
		if ui.StopKeeps(rs.summary(m)) {
			prompt = "Stop " + combo + "? Its testbed is kept so you can look around. (y/N)"
		}
		m.confirm = &confirm{prompt: prompt, yes: func() tea.Cmd { return decide(m, id, combo, engine.Abort) }}
	case s == "x":
		id := rs.id
		m.confirm = &confirm{prompt: "Cancel #" + id + "? Its environments are torn down. (y/N)", yes: func() tea.Cmd {
			if err := m.svc.Cancel(id); err != nil {
				m.sayf("%v", err)
			}
			return nil
		}}
	case s == "R":
		r, err := m.svc.RunAgain(context.Background(), rs.id, rs.grid)
		if err != nil {
			m.sayf("%v", err)
			return nil
		}
		m.openRun(r.ID)
	}
	return nil
}

func decide(m *model, id, combo, d string) tea.Cmd {
	if err := m.svc.Decide(id, combo, d); err != nil {
		m.sayf("%v", err)
	}
	return nil
}

// console opens the console of the machine the failure names, else the first.
func console(dir string, f *engine.Failure) tea.Cmd {
	files, _ := filepath.Glob(filepath.Join(dir, "consoles", "*.log"))
	if len(files) == 0 {
		return nil
	}
	pick := files[0]
	if f != nil {
		for _, c := range files {
			if strings.Contains(f.Related, strings.TrimSuffix(filepath.Base(c), ".log")) {
				pick = c
			}
		}
	}
	return page(pick)
}

func pageList(root string, files []string) tea.Cmd {
	if len(files) == 0 {
		return nil
	}
	if len(files) == 1 {
		return page(filepath.Join(root, files[0]))
	}
	f, err := os.CreateTemp("", "wendy-artifacts-*.txt")
	if err != nil {
		return nil
	}
	for _, p := range files {
		fmt.Fprintln(f, filepath.Join(root, p))
	}
	f.Close()
	return page(f.Name())
}

func pageDir(dir string) tea.Cmd {
	var files []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			files = append(files, rel)
		}
		return nil
	})
	sort.Strings(files)
	return pageList(dir, files)
}

// dumpFiles are the files of every cluster dump in diagnostics, relative to
// the execution's directory root.
func dumpFiles(root string, diagnostics map[string]string) []string {
	var files []string
	for kind, rel := range diagnostics {
		if kind != "dump" && !strings.HasPrefix(kind, "dump.") {
			continue
		}
		_ = filepath.WalkDir(filepath.Join(root, rel), func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				r, _ := filepath.Rel(root, p)
				files = append(files, r)
			}
			return nil
		})
	}
	sort.Strings(files)
	return files
}

func (rs *runScreen) view(m *model) string {
	sum := rs.summary(m)
	v := rs.rv
	status := v.Status
	if sum.Status.Done() {
		status = sum.Status
	}
	elapsed := sum.Ms
	if !status.Done() {
		elapsed = time.Since(sum.Started).Milliseconds()
	}
	title := sBold.Render(fmt.Sprintf(" Runs › #%s %s", rs.id, sum.Label))
	if sum.Edited {
		title += sMute.Render(" (edited)")
	}

	done := v.Done()
	total := max(v.Total, done)
	stats := fmt.Sprintf("%s %s  %s  %s  %d/%d steps   %s %d  %s %d  %s %d",
		glyph(status), word(status), ui.Dur(elapsed),
		bar(done, total, 30, v.Counts[event.Failed]+v.Counts[event.Errored] > 0), done, total,
		glyph(event.Passed), v.Counts[event.Passed], glyph(event.Failed), v.Counts[event.Failed]+v.Counts[event.TimedOut],
		glyph(event.Errored), v.Counts[event.Errored])
	if !status.Done() {
		if est := engine.PastDurations(m.svc.Engine.Data, sum.Key); len(est) > 0 {
			sort.Slice(est, func(i, j int) bool { return est[i] < est[j] })
			if left := est[len(est)/2] - time.Duration(elapsed)*time.Millisecond; left > time.Minute {
				stats += sMute.Render(fmt.Sprintf("     about %d min left", int(left.Minutes())))
			}
		}
	}
	out := []string{title, " " + stats}
	h := m.h - 3

	combo, f, paused := rs.failure(m)
	if f != nil && (paused || status.Done()) {
		card := rs.failureCard(m, combo, f, paused)
		out = append(out, card)
		h -= lipgloss.Height(card)
	}

	if rs.grid {
		out = append(out, rs.gridView(m, sum, h))
		out = append(out, footer("enter", "open combo", "R", "re-run failed combos", "g", "grid", "t", "tree", "esc", "back", "?", "help"))
		return strings.Join(out, "\n")
	}

	rows := rs.rows()
	idx := rs.selIndex(rows)
	tw := min(46, m.w/2)
	treeH := h - 3
	start := 0
	if idx >= treeH {
		start = idx - treeH + 1
	}
	var tree strings.Builder
	for j := start; j < len(rows) && j < start+treeH; j++ {
		r := rows[j]
		marker := " "
		if r.HasKids {
			marker = "▸"
			if r.Expanded {
				marker = "▾"
			}
		}
		g := glyph(r.Status)
		dur := ""
		if r.Status != event.Waiting && r.Status != event.Skipped {
			dur = ui.Dur(r.Elapsed())
		}
		label := strings.Repeat("  ", r.Depth) + marker + g + " " + r.Label()
		line := fit(label, tw-11) + fmt.Sprintf("%7s", dur)
		if r.Key == rs.sel {
			line = sSel.Render(fit(stripANSI(label), tw-11) + fmt.Sprintf("%7s", dur))
		}
		tree.WriteString(line + "\n")
	}
	follow := sMute.Render("○ follow off — f to track the running step")
	if rs.follow {
		follow = statusStyle(event.Running).Render("● follow") + sMute.Render(" — tracking the running step")
	}
	tree.WriteString(strings.Repeat("\n", max(treeH-(min(len(rows), start+treeH)-start), 0)) + follow)

	detail := rs.detail(m.w-tw-6, h-2)
	if m.w < 100 {
		out = append(out, box("", tree.String(), m.w-2, h, true))
	} else {
		out = append(out, lipgloss.JoinHorizontal(lipgloss.Top, box("", tree.String(), tw, h, true), box("", detail, m.w-tw-4, h, false)))
	}
	keys := []string{"↑↓", "select", "→←", "expand/collapse", "f", "follow", "l", "log", "a", "artifacts", "o", "console"}
	switch {
	case paused:
		keys = []string{"s", "shell", "r", "retry step", "c", "carry on", "x", "stop", "w/T/o/b/d", "diagnostics", "↑↓", "select"}
	case !status.Done():
		keys = append(keys, "x", "cancel run")
	default:
		keys = append(keys, "R", "re-run")
	}
	keys = append(keys, "esc", "back", "?", "help")
	out = append(out, footer(keys...))
	return strings.Join(out, "\n")
}

func (rs *runScreen) failureCard(m *model, combo string, f *engine.Failure, paused bool) string {
	var b strings.Builder
	head := statusStyle(f.Status).Render(fmt.Sprintf("%s %s", vocab.StatusOf(f.Status).Glyph, strings.ToUpper(word(f.Status))))
	fmt.Fprintf(&b, "%s   Failed after %s:  %s\n\n", head, ui.Dur(f.Ms), strings.Join(f.Path, " › "))
	b.WriteString(firstLine(f.Msg) + "\n")
	if f.Related != "" {
		b.WriteString(sBold.Render("Related:") + "  " + f.Related + "\n")
	}
	var diag []string
	for _, k := range []struct{ key, kind, label string }{{"w", "workflow", "workflow"}, {"T", "logs", "Tinkerbell logs"}, {"o", "console", "console"}, {"b", "bmc", "BMC log"}} {
		if f.Diagnostics[k.kind] != "" {
			diag = append(diag, sKey.Render("["+k.key+"]")+" "+k.label)
		}
	}
	for kind := range f.Diagnostics {
		if kind == "dump" || strings.HasPrefix(kind, "dump.") {
			diag = append(diag, sKey.Render("[d]")+" cluster dumps")
			break
		}
	}
	if len(diag) > 0 {
		b.WriteString("\nDiagnostics   " + strings.Join(diag, "   ") + "\n")
	}
	if paused {
		dir := rs.comboDir(m, combo)
		b.WriteString("\nThe environment is kept so you can look around.\n")
		b.WriteString("kubeconfigs  " + sMute.Render(filepath.Join(dir, "kubeconfigs")) + "\n\n")
		b.WriteString(sKey.Render("[ s  Open shell here ]") + "   " + sKey.Render("[ r  Retry step ]") + "   " +
			sKey.Render("[ c  Carry on ]") + "   " + sKey.Render("[ x  Stop ]"))
	}
	st := sFail.Width(m.w - 4)
	if f.Status == event.Errored {
		st = st.BorderForeground(cErr)
	}
	return st.Render(b.String())
}

func (rs *runScreen) detail(w, h int) string {
	n := rs.node()
	if n == nil {
		return ""
	}
	var b strings.Builder
	title := strings.Join(n.Path, " › ")
	if n.Kind == ui.KindCombination {
		title = n.Name
	}
	b.WriteString(sBold.Render(fit(title, w-10)) + "  " + sMute.Render(ui.Dur(n.Elapsed())) + "\n")
	if n.Summary != "" {
		b.WriteString(n.Summary + "\n")
	} else if n.Msg != "" {
		b.WriteString(statusStyle(n.Status).Render(firstLine(n.Msg)) + "\n")
	}
	b.WriteString(sMute.Render(strings.Repeat("─", max(w-2, 1))) + "\n")
	logs := n.Logs
	for _, c := range n.Children {
		if len(logs) == 0 && c.Status == event.Running {
			logs = c.Logs
		}
	}
	room := h - 7
	if len(n.Cases) > 0 {
		room -= min(len(n.Cases), 6) + 1
	}
	if len(logs) > room {
		logs = logs[len(logs)-max(room, 0):]
	}
	for _, e := range logs {
		b.WriteString(sMute.Render(e.TS.Format("15:04:05")) + "  " + fit(e.Msg, w-12) + "\n")
	}
	if n.Status == event.Running {
		b.WriteString("▌\n")
	}
	if len(n.Cases) > 0 {
		b.WriteString(sTitle.Render("CASES") + "\n")
		for i, c := range n.Cases {
			if i >= 6 {
				break
			}
			b.WriteString(glyph(c.Status) + " " + fit(c.Path[len(c.Path)-1], w-6) + "\n")
		}
	}
	lines := strings.Count(b.String(), "\n")
	b.WriteString(strings.Repeat("\n", max(h-lines-4, 0)))
	b.WriteString(sMute.Render(strings.Repeat("─", max(w-2, 1))) + "\n")
	b.WriteString(sKey.Render("l") + " full log   " + sKey.Render("a") + fmt.Sprintf(" artifacts (%d)   ", len(n.Files)) + sKey.Render("o") + " console\n")
	b.WriteString(sKey.Render("k") + " kubeconfig   " + sKey.Render("s") + " shell with KUBECONFIG set")
	return b.String()
}

func (rs *runScreen) gridView(m *model, sum *engine.Summary, h int) string {
	keys := []string{"build", "setup"}
	seen := map[string]bool{"build": true, "setup": true}
	for _, c := range sum.Combinations {
		var names []string
		for k := range c.Nodes {
			if !seen[k] {
				names = append(names, k)
			}
		}
		sort.Strings(names)
		for _, n := range names {
			seen[n] = true
			keys = append(keys, n)
		}
	}
	nameW := 36
	for _, c := range sum.Combinations {
		nameW = max(nameW, len(c.Name)+2)
	}
	// A column is as wide as its heading, so "Set up testbed" fits over its glyphs.
	widths := make([]int, len(keys))
	var b strings.Builder
	head := strings.Repeat(" ", nameW+3)
	for i, k := range keys {
		label := vocab.Phase(k)
		widths[i] = max(8, lipgloss.Width(label)+2)
		head += fit(label, widths[i])
	}
	b.WriteString(sTitle.Render(head+"time") + "\n")
	for i, c := range sum.Combinations {
		line := fmt.Sprintf(" %s ", fit(c.Name, nameW+1))
		for j, k := range keys {
			st, ok := c.Nodes[k]
			if !ok {
				st = event.Waiting
				if c.Status.Done() {
					st = event.Skipped
				}
			}
			line += "   " + glyph(st) + strings.Repeat(" ", widths[j]-4)
		}
		line += fmt.Sprintf("%6s", ui.Dur(c.Ms))
		if c.Failure != nil {
			line += "   " + statusStyle(c.Failure.Status).Render(fit(firstLine(c.Failure.Msg), max(m.w-lipgloss.Width(line)-8, 10)))
		}
		if i == rs.gridSel {
			line = sSel.Render(">" + stripANSI(line)[1:])
		}
		b.WriteString(line + "\n")
	}
	for _, c := range sum.Combinations {
		if c.Status == event.Errored {
			b.WriteString("\n" + glyph(event.Errored) + sMute.Render(" errored = wendy or the host failed, not Tinkerbell/CAPT. Re-running usually helps.") + "\n")
			break
		}
	}
	return box("", b.String(), m.w-2, h, true)
}
