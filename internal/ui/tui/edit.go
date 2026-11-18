package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/ui"
	"github.com/tinkerbell/wendy/internal/vocab"
)

type editStage int

const (
	stageForm  editStage = iota // the entry's settings
	stageSteps                  // a test's step list
	stageStep                   // one step's settings
)

// editor creates and edits a test or suite. Its form is
// built from the same field tables as the web UI's, so both offer the same
// settings.
type editor struct {
	kind string
	// mode is "new", "edit" or "copy".
	mode string
	// orig is the name being edited, so saving under another name renames it.
	orig   string
	name   string
	value  map[string]any
	fields []ui.Field
	vals   []string
	form   *huh.Form
	stage  editStage
	err    string
	w, h   int

	steps []map[string]any
	sel   int
	step  *stepEdit
}

// stepEdit is the form of one step.
type stepEdit struct {
	idx  int // -1 for a new step
	kind string
	vals []string
	form *huh.Form
}

// catalogKind is the kind of what the catalog tab lists, if the UI can edit
// it.
func (m *model) catalogKind() string {
	switch m.catalog.tab {
	case tabSuites:
		return config.KindSuite
	case tabTests:
		return config.KindTest
	}
	return ""
}

// openEditor starts the editor on a new entry, or on name: edited in place,
// or copied into a new entry.
func (m *model) openEditor(kind, name, mode string) tea.Cmd {
	e := &editor{kind: kind, mode: mode, value: ui.NewValue(kind), w: m.w, h: m.h}
	if name != "" {
		entry, err := m.svc.Entry(kind, name)
		if err != nil {
			m.sayf("%v", err)
			return nil
		}
		if mode == "edit" && !entry.Editable {
			m.sayf("%s is defined in %s; press c to copy it into %s", name, entry.Origin, config.UIFile)
			return nil
		}
		e.value = entry.Value
		if mode == "edit" {
			e.orig, e.name = name, name
		} else {
			e.name = name + "-copy"
		}
	}
	switch kind {
	case config.KindTest:
		e.fields = ui.TestFields
		e.steps = ui.Steps(e.value)
	default:
		e.fields = ui.SuiteFields
	}
	e.build(m.cat)
	m.edit = e
	return e.form.Init()
}

// resizeForms fits whichever form is open to the terminal.
func (m *model) resizeForms() {
	if m.form != nil {
		m.form.resize(m.w, m.h)
	}
	if m.edit != nil {
		m.edit.resize(m.w, m.h)
	}
}

func (e *editor) title() string {
	what := strings.TrimSuffix(e.kind, "s")
	switch e.mode {
	case "edit":
		return "Edit " + what + " " + e.orig
	case "copy":
		return "Copy to a new " + what
	}
	return "New " + what
}

func (e *editor) resize(w, h int) {
	e.w, e.h = w, h
	for _, f := range []*huh.Form{e.form, e.stepForm()} {
		if f != nil {
			f.WithWidth(w - 6).WithHeight(max(h-7, 8))
		}
	}
}

func (e *editor) stepForm() *huh.Form {
	if e.step == nil {
		return nil
	}
	return e.step.form
}

func builtinIDs(cat *config.Catalog) []string {
	out := make([]string, 0, len(cat.BuiltinSteps))
	for _, b := range cat.BuiltinSteps {
		out = append(out, b.ID)
	}
	return out
}

// build makes the settings form from the entry's current values.
func (e *editor) build(cat *config.Catalog) {
	e.vals = make([]string, len(e.fields))
	for i, f := range e.fields {
		e.vals[i] = ui.Get(e.value, f)
	}
	nameField := huh.NewInput().Title("Name *").Description("letters, digits, . _ -").Value(&e.name).
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return errors.New("a name is required")
			}
			return nil
		})
	fs := []huh.Field{nameField}
	for i, f := range e.fields {
		fs = append(fs, widget(f, &e.vals[i], builtinIDs(cat)))
	}
	title := strings.ToUpper(e.kind[:1]) + strings.TrimSuffix(e.kind, "s")[1:]
	e.form = huh.NewForm(huh.NewGroup(fs...).Title(title)).WithShowHelp(false)
	e.resize(e.w, e.h)
}

// widget is the input for a field. Selects offer "not set" first, which
// leaves the schema's default in force.
func widget(f ui.Field, v *string, uses []string) huh.Field {
	title := f.Label
	if f.Required {
		title += " *"
	}
	switch f.Kind {
	case ui.FSelect, ui.FBool:
		choices := ui.Choices(f)
		if f.Path == "uses" {
			choices = uses
		}
		if *v == f.Default {
			// The first option stands for the default, so it is not listed
			// again; saving it unsets the field, which reads the same.
			*v = ""
		}
		if *v != "" && !slices.Contains(choices, *v) {
			choices = append(slices.Clone(choices), *v)
		}
		none := ui.DefaultLabel(f)
		if f.Default == "" {
			none = "not set"
		}
		opts := []huh.Option[string]{huh.NewOption(none, "")}
		for _, c := range choices {
			opts = append(opts, huh.NewOption(ui.ChoiceLabel(f, c), c))
		}
		sel := huh.NewSelect[string]().Title(title).Description(f.Help).Options(opts...).Value(v)
		if len(opts) <= 4 {
			return sel.Inline(true)
		}
		return sel.Height(7)
	case ui.FLines, ui.FMap, ui.FYAML:
		return huh.NewText().Title(title).Description(f.Help).Placeholder(f.Default).Lines(3).Value(v).
			Validate(func(s string) error { return ui.Set(map[string]any{}, f, s) })
	case ui.FText, ui.FInt, ui.FWords:
	}
	return huh.NewInput().Title(title).Description(f.Help).Placeholder(f.Default).Value(v).
		Validate(func(s string) error { return ui.Set(map[string]any{}, f, s) })
}

// apply stores the form's values in the entry.
func (e *editor) apply() error {
	var errs []error
	for i, f := range e.fields {
		errs = append(errs, ui.Set(e.value, f, e.vals[i]))
	}
	return errors.Join(errs...)
}

func (e *editor) update(m *model, msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case k.String() == "ctrl+s" && e.stage != stageStep:
			return e.saveNow(m)
		case e.stage == stageSteps:
			return e.updateSteps(m, k)
		case k.String() == "esc" && e.stage == stageStep:
			e.step, e.stage, e.err = nil, stageSteps, ""
			return nil
		case k.String() == "esc":
			m.edit = nil
			return nil
		}
	}
	if e.stage == stageStep {
		return e.updateStep(m, msg)
	}
	fm, cmd := e.form.Update(msg)
	if nf, ok := fm.(*huh.Form); ok {
		e.form = nf
	}
	switch e.form.State {
	case huh.StateCompleted:
		return e.submit(m)
	case huh.StateAborted:
		m.edit = nil
	default:
	}
	return cmd
}

// submit handles a finished settings form: a test goes on to its steps,
// the rest are saved.
func (e *editor) submit(m *model) tea.Cmd {
	if err := e.apply(); err != nil {
		e.err = err.Error()
		e.build(m.cat)
		return e.form.Init()
	}
	if e.kind == config.KindTest {
		e.stage, e.err = stageSteps, ""
		return nil
	}
	return e.save(m)
}

// saveNow saves from wherever the user is, with what the form holds so far.
func (e *editor) saveNow(m *model) tea.Cmd {
	if e.stage == stageForm {
		if err := e.apply(); err != nil {
			e.err = err.Error()
			return nil
		}
	}
	return e.save(m)
}

func (e *editor) save(m *model) tea.Cmd {
	if e.kind == config.KindTest {
		ui.SetSteps(e.value, e.steps)
	}
	if err := m.svc.SaveEntry(e.kind, e.orig, strings.TrimSpace(e.name), e.value); err != nil {
		e.err = err.Error()
		if e.stage == stageForm {
			e.build(m.cat)
			return e.form.Init()
		}
		return nil
	}
	m.edit = nil
	m.sayf("saved %s %s to %s", strings.TrimSuffix(e.kind, "s"), strings.TrimSpace(e.name), config.UIFile)
	m.refresh()
	return nil
}

func (e *editor) updateSteps(m *model, k tea.KeyPressMsg) tea.Cmd {
	s := k.String()
	switch {
	case up(s):
		e.sel = max(e.sel-1, 0)
	case down(s):
		e.sel = min(e.sel+1, max(len(e.steps)-1, 0))
	case s == "a":
		return e.openStep(m, -1)
	case (s == "enter" || s == "e") && len(e.steps) > 0:
		return e.openStep(m, e.sel)
	case s == "d" && len(e.steps) > 0:
		e.steps = slices.Delete(e.steps, e.sel, e.sel+1)
		e.sel = min(e.sel, max(len(e.steps)-1, 0))
	case s == "K" && e.sel > 0:
		e.steps[e.sel], e.steps[e.sel-1] = e.steps[e.sel-1], e.steps[e.sel]
		e.sel--
	case s == "J" && e.sel < len(e.steps)-1:
		e.steps[e.sel], e.steps[e.sel+1] = e.steps[e.sel+1], e.steps[e.sel]
		e.sel++
	case s == "s":
		return e.save(m)
	case s == "esc" || s == "b":
		e.stage, e.err = stageForm, ""
		e.build(m.cat)
		return e.form.Init()
	}
	return nil
}

func (e *editor) openStep(m *model, idx int) tea.Cmd {
	st := map[string]any{}
	if idx >= 0 {
		st = e.steps[idx]
	}
	kind := ui.StepKindOf(st)
	if kind == "" {
		kind = ui.StepUses
	}
	se := &stepEdit{idx: idx, kind: kind, vals: make([]string, len(ui.StepFields))}
	for i, f := range ui.StepFields {
		se.vals[i] = ui.Get(st, f)
	}
	e.step, e.stage, e.err = se, stageStep, ""
	e.buildStep(m.cat)
	return se.form.Init()
}

// buildStep makes the step form: what it runs and the settings every step
// has, then a page for the fields of the chosen kind.
func (e *editor) buildStep(cat *config.Catalog) {
	se := e.step
	kindLabel := func(k string) string { return vocab.Choice("step.kind", k) + ": " + ui.StepKindHelp[k] }
	kinds := make([]huh.Option[string], 0, len(ui.StepKinds))
	for _, k := range ui.StepKinds {
		kinds = append(kinds, huh.NewOption(kindLabel(k), k))
	}
	common := []huh.Field{huh.NewSelect[string]().Title("Runs").Options(kinds...).Value(&se.kind).Height(6)}
	for i, f := range ui.StepFields {
		if len(f.Only) == 0 {
			common = append(common, widget(f, &se.vals[i], nil))
		}
	}
	groups := []*huh.Group{huh.NewGroup(common...).Title("Step")}
	uses := builtinIDs(cat)
	for _, k := range ui.StepKinds {
		var fs []huh.Field
		for i, f := range ui.StepFields {
			if slices.Contains(f.Only, k) {
				fs = append(fs, widget(f, &se.vals[i], uses))
			}
		}
		groups = append(groups, huh.NewGroup(fs...).Title(kindLabel(k)).WithHideFunc(func() bool { return se.kind != k }))
	}
	se.form = huh.NewForm(groups...).WithShowHelp(false)
	e.resize(e.w, e.h)
}

func (e *editor) updateStep(m *model, msg tea.Msg) tea.Cmd {
	se := e.step
	fm, cmd := se.form.Update(msg)
	if nf, ok := fm.(*huh.Form); ok {
		se.form = nf
	}
	switch se.form.State {
	case huh.StateAborted:
		e.step, e.stage = nil, stageSteps
	case huh.StateCompleted:
		st := map[string]any{}
		var errs []error
		for i, f := range ui.StepFields {
			if len(f.Only) == 0 || slices.Contains(f.Only, se.kind) {
				errs = append(errs, ui.Set(st, f, se.vals[i]))
			}
		}
		if missing := ui.Missing(st, ui.FieldsFor(se.kind)); len(missing) > 0 {
			errs = append(errs, fmt.Errorf("a %s step needs %s", se.kind, strings.Join(missing, ", ")))
		}
		if err := errors.Join(errs...); err != nil {
			e.err = err.Error()
			e.buildStep(m.cat)
			return se.form.Init()
		}
		if se.idx < 0 {
			e.steps = append(e.steps, st)
			e.sel = len(e.steps) - 1
		} else {
			e.steps[se.idx] = st
		}
		e.step, e.stage, e.err = nil, stageSteps, ""
	default:
	}
	return cmd
}

func (e *editor) view(m *model) string {
	title := " " + e.title() + sMute.Render("   saved to "+config.UIFile)
	if e.name != "" && e.kind == config.KindTest && e.stage != stageForm {
		title = " " + e.title() + " · " + sBold.Render(e.name)
	}
	h := m.h - 4
	var body, keys string
	switch e.stage {
	case stageForm:
		body = box("", e.form.View(), m.w-2, h, true)
		keys = footer("tab/enter", "next", "shift+tab", "previous", "alt+enter", "new line in a list", "ctrl+s", "save now", "esc", "cancel")
	case stageStep:
		body = box("", e.step.form.View(), m.w-2, h, true)
		keys = footer("tab/enter", "next", "alt+enter", "new line in a list", "esc", "back to steps")
	default:
		body = box("", e.stepsView(), m.w-2, h, true)
		keys = footer("↑↓", "select", "a", "add", "enter", "edit", "d", "delete", "K J", "move", "s", "save", "esc", "settings")
	}
	if e.err != "" {
		keys = statusStyle(event.Failed).Render(" "+fit(e.err, m.w-3)) + "\n" + keys
	}
	return title + "\n" + body + "\n" + keys
}

func (e *editor) stepsView() string {
	var b strings.Builder
	b.WriteString(sBold.Render("Steps") + sMute.Render("   run in order; each is a built-in, a command, a container, Chainsaw or Ginkgo") + "\n\n")
	for i, st := range e.steps {
		kind := ui.StepKindOf(st)
		line := fmt.Sprintf("%2d  %-10s %s", i+1, kind, stepSummary(st, kind))
		if i == e.sel {
			line = sSel.Render(fit("> "+line, 90))
		} else {
			line = "  " + line
		}
		b.WriteString(line + "\n")
	}
	if len(e.steps) == 0 {
		b.WriteString(sMute.Render("  no steps yet: press a to add one") + "\n")
	}
	return b.String()
}

func stepSummary(st map[string]any, kind string) string {
	for _, f := range ui.StepFields {
		if len(f.Only) > 0 && f.Required && slices.Contains(f.Only, kind) {
			v := strings.ReplaceAll(ui.Get(st, f), "\n", " ")
			if name := ui.Get(st, ui.StepFields[0]); name != "" {
				return name + sMute.Render("  "+v)
			}
			return v
		}
	}
	return ui.Get(st, ui.StepFields[0])
}
