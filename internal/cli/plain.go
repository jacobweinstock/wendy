package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/preview"
	"github.com/tinkerbell/wendy/internal/service"
	"github.com/tinkerbell/wendy/internal/ui"
	"github.com/tinkerbell/wendy/internal/vocab"
)

// glyph is the symbol of s.
func glyph(s event.Status) string { return vocab.StatusOf(s).Glyph }

// checkGlyph is the symbol of a check that passed when ok, and failed when not.
func checkGlyph(ok bool) string {
	if ok {
		return glyph(event.Passed)
	}
	return glyph(event.Failed)
}

// planLine summarises a plan in one line: "8 combos × smoke · parallel 2 · ~1h50".
func planLine(p *preview.Preview) string {
	combos := fmt.Sprintf("%d combos", len(p.Combinations))
	if len(p.Combinations) == 1 {
		combos = p.Combinations[0].Name
	}
	var suites []string
	if len(p.Combinations) > 0 {
		for _, s := range p.Combinations[0].Suites {
			suites = append(suites, s.Name)
		}
	}
	line := fmt.Sprintf("%s × %s · parallel %d", combos, strings.Join(suites, ", "), p.Parallel)
	if p.Needs.Estimate > 0 {
		line += " · ~" + ui.Dur(p.Needs.Estimate.Milliseconds())
	}
	return line
}

// plain prints a run one line per event worth a line, until it finishes,
// and returns its verdict. Safe for CI logs and grep.
func plain(ctx context.Context, w io.Writer, svc *service.Service, r *engine.Execution, verbose bool) event.Status {
	p := &plainPrinter{
		w: w, svc: svc, r: r, verbose: verbose,
		interactive: term.IsTerminal(int(os.Stdin.Fd())),
		in:          bufio.NewReader(os.Stdin),
	}
	fmt.Fprintf(w, "#%s  plan  %s\n", r.ID, planLine(r.Preview))
	if len(r.Preview.Builds) == 0 {
		p.out(glyph(event.Passed), engine.PhaseBuild, "0m00", "(released artifacts)")
	}
	sub := r.Stream.Subscribe(0)
	defer sub.Close()
	for {
		e, ok := sub.Next(ctx)
		if !ok {
			return r.Wait()
		}
		p.event(e)
	}
}

type plainPrinter struct {
	w           io.Writer
	svc         *service.Service
	r           *engine.Execution
	verbose     bool
	interactive bool
	in          *bufio.Reader
}

func (p *plainPrinter) out(glyph, what, dur, msg string) {
	line := fmt.Sprintf("#%s  %s %-58s %6s", p.r.ID, glyph, what, dur)
	if msg != "" {
		line += "  " + firstLine(msg)
	}
	fmt.Fprintln(p.w, strings.TrimRight(line, " "))
}

func (p *plainPrinter) event(e event.Event) {
	switch e.Type {
	case event.StepStarted:
		if (len(e.Path) == 2 && e.Path[0] != engine.PhaseBuild) || (p.verbose && len(e.Path) == 3) {
			p.out(glyph(event.Running), where(e), "", "")
		}
	case event.StepFinished:
		fail := e.Status == event.Failed || e.Status == event.Errored || e.Status == event.TimedOut
		switch {
		case len(e.Path) == 2 && e.Path[0] != engine.PhaseBuild:
			p.out(glyph(e.Status), where(e), ui.Dur(e.Ms), e.Msg)
		case len(e.Path) == 3 && (fail || p.verbose || e.Path[0] == engine.PhaseBuild):
			p.out(glyph(e.Status), where(e), ui.Dur(e.Ms), e.Msg)
		}
	case event.Log:
		if p.verbose {
			fmt.Fprintf(p.w, "#%s      %s %s\n", p.r.ID, e.TS.Format("15:04:05"), e.Msg)
		}
	case event.CombinationPaused:
		p.paused(e)
	case event.ExecutionFinished:
		p.finished(e)
	}
}

// where names an event's step. A phase's path repeats its name, so the
// repeat is dropped.
func where(e event.Event) string {
	path := e.Path
	if len(path) == 3 {
		switch path[0] {
		case engine.PhaseBuild, engine.PhaseSetup, engine.PhaseTeardown, engine.PhaseDiagnostics:
			path = []string{path[0], path[2]}
		}
	}
	s := strings.Join(path, " › ")
	if e.Combination != "" {
		return e.Combination + "  " + s
	}
	return s
}

// paused shows where to look at a paused combination, then asks the
// terminal what to do, or stops it when there is no one to ask.
func (p *plainPrinter) paused(e event.Event) {
	id := p.r.ID
	var f *engine.Failure
	dir := ""
	if sum, err := p.svc.Execution(id); err == nil {
		for _, c := range sum.Combinations {
			if c.Name == e.Combination {
				f, dir = c.Failure, c.Dir
			}
		}
	}
	p.out(glyph(event.Paused), e.Combination+"  paused on failure", "", e.Msg)
	if f != nil && f.Related != "" {
		fmt.Fprintf(p.w, "#%s    Related: %s\n", id, f.Related)
	}
	fmt.Fprintf(p.w, "#%s    kubeconfigs: %s\n", id, filepath.Join(p.svc.ExecutionDir(id), dir, "kubeconfigs"))
	if !p.interactive {
		fmt.Fprintf(p.w, "#%s    not a terminal: stopping (use the TUI or the web UI to decide)\n", id)
		_ = p.r.Decide(e.Combination, engine.Abort)
		return
	}
	// The execution keeps streaming events while the terminal waits for an answer.
	go p.ask(e.Combination)
}

func (p *plainPrinter) ask(combination string) {
	for {
		fmt.Fprintf(p.w, "#%s    [r]etry step  [c]arry on  [x] stop > ", p.r.ID)
		line, err := p.in.ReadString('\n')
		if err != nil {
			_ = p.r.Decide(combination, engine.Abort)
			return
		}
		d := map[string]string{"r": engine.Retry, "c": engine.Continue, "x": engine.Abort, "a": engine.Abort}[strings.TrimSpace(line)]
		if d != "" {
			_ = p.r.Decide(combination, d)
			return
		}
	}
}

// finished prints the verdict and what the execution left behind.
func (p *plainPrinter) finished(e event.Event) {
	id, sum := p.r.ID, p.r.Summary()
	passed, failed, errored := 0, 0, 0
	for _, c := range sum.Combinations {
		switch c.Status {
		case event.Passed:
			passed++
		case event.Failed:
			failed++
		case event.Errored, event.TimedOut:
			errored++
		default:
		}
	}
	st := vocab.StatusOf(e.Status)
	fmt.Fprintf(p.w, "#%s  %s %s  %d passed · %d failed · %d errored · %s\n", id, st.Glyph, st.Word, passed, failed, errored, ui.Dur(e.Ms))
	for _, te := range sum.TeardownErrors {
		fmt.Fprintf(p.w, "#%s    teardown: %s\n", id, te)
	}
	for _, c := range sum.Combinations {
		if c.Kept {
			fmt.Fprintf(p.w, "#%s    kept: %s  (wendy env down %s/%s)\n", id, filepath.Join(p.svc.ExecutionDir(id), c.Dir), id, c.Name)
		}
	}
	fmt.Fprintf(p.w, "#%s  report  %s\n", id, filepath.Join(p.svc.ExecutionDir(id), "report.junit.xml"))
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// exitCode maps a verdict to the process's exit code.
func exitCode(s event.Status) int {
	switch s {
	case event.Passed, event.Skipped:
		return 0
	case event.Failed:
		return 1
	case event.Cancelled:
		return 3
	default:
		return 2
	}
}
