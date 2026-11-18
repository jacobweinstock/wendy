package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/a-h/templ"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/service"
	"github.com/tinkerbell/wendy/internal/ui"
)

// editData is what the editor page shows.
type editData struct {
	// Kind is the #Config field: testbeds, a kind of part, tests, suites or
	// steps.
	Kind string
	// Mode is "new", "edit" or "copy".
	Mode string
	Name string
	// Orig is the name being edited, so saving under another name renames it.
	Orig string
	// From is the entry a copy started from; fields the form does not show
	// are carried over from it.
	From   string
	Origin string
	Value  map[string]any
	Steps  []stepData
	// Uses are the named steps a step can use: the built-in steps, and for a
	// test's steps the user's own too.
	Uses []string
	// Tests are the names a suite can list.
	Tests []string
	// Cat is the loaded config, for the choices a testbed's pickers offer.
	Cat *config.Catalog
	// UsedBy are the entries that refer to the one being edited, as "kind
	// name"; while there are any, it can't be deleted.
	UsedBy []string
	// UIFile is where the form saves, as people type the path.
	UIFile string
	// IDPrefix sets the form's element IDs apart from another form's on the
	// same page, as a dialog's must be.
	IDPrefix string
}

type stepData struct {
	Idx   string
	Kind  string
	Value map[string]any
	// Single marks the one step of a step entry's form, which has no
	// number and does not move.
	Single bool
}

func (d editData) fields() []ui.Field {
	switch d.Kind {
	case config.KindTest:
		return ui.TestFields
	case config.KindSuite:
		return ui.SuiteFields
	case config.KindStep:
		return nil
	}
	return ui.PartFields(d.Kind)
}

// usesFor are the named steps a step of an entry of kind can use: a step of
// the user's can use only a built-in step, so steps never chain.
func usesFor(cat *config.Catalog, kind string) []string {
	out := make([]string, 0, len(cat.BuiltinSteps)+len(cat.Steps))
	for _, b := range cat.BuiltinSteps {
		out = append(out, b.ID)
	}
	if kind != config.KindStep {
		out = append(out, config.Names(cat.Steps)...)
	}
	return out
}

// tabLabel is how a kind's subtab is labelled.
func tabLabel(kind string) string {
	if t, ok := subtabOf(kind); ok {
		return t.Label
	}
	return capital(kind)
}

// tabBlurb is what a kind's subtab says it holds.
func tabBlurb(kind string) string {
	t, _ := subtabOf(kind)
	return t.Blurb
}

// commonFields are the settings a typical entry sets. The machine settings are
// left out: the machine table shows them, and a testbed's description has a
// card of its own.
func (d editData) commonFields() []ui.Field {
	var out []ui.Field
	for _, f := range d.fields() {
		if !f.Advanced && !ui.IsMachineField(f) && (d.Kind != config.KindTestbed || f.Path != "description") {
			out = append(out, f)
		}
	}
	return out
}

// advancedFields are the settings a form folds away.
func (d editData) advancedFields() []ui.Field {
	var out []ui.Field
	for _, f := range d.fields() {
		if f.Advanced {
			out = append(out, f)
		}
	}
	return out
}

// inlineChoice is the picker value for a part written inside the testbed in
// the config file, which has no name to pick.
const inlineChoice = "@inline"

// pickerValue is what a testbed's picker shows as chosen: the part's name,
// "@inline" for one written in place, or "" when it gets its kind's
// fallback, named or not.
func (d editData) pickerValue(kind string) string {
	switch v := d.Value[kind].(type) {
	case string:
		if v == d.Cat.Fallback[kind] {
			return ""
		}
		return v
	case map[string]any:
		return inlineChoice
	}
	return ""
}

// pickedPart is the name of the part a picker stands for, the fallback when
// it picks none.
func (d editData) pickedPart(kind string) string {
	return cmp(d.pickerValue(kind), d.Cat.Fallback[kind])
}

// openPart is the part a picker's Open link leads to, or "" for a part
// written inside the testbed, which has no page.
func (d editData) openPart(kind string) string {
	if d.pickerValue(kind) == inlineChoice {
		return ""
	}
	return d.pickedPart(kind)
}

// partChoices are the parts a picker offers by name. The fallback is not
// among them, since the picker offers it first, as picking none.
func (d editData) partChoices(kind string) []string {
	var out []string
	for _, n := range config.Names(d.Cat.Parts(kind)) {
		if n != d.Cat.Fallback[kind] {
			out = append(out, n)
		}
	}
	return out
}

// inlineJSON is a part written inside the testbed, kept in the form so a save
// writes it back unchanged.
func (d editData) inlineJSON(kind string) string {
	m, ok := d.Value[kind].(map[string]any)
	if !ok {
		return ""
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// matrixTestbed is the testbed whose matrix the form shows, read-only: the
// one being edited or copied, when it has a matrix.
func (d editData) matrixTestbed() *config.TestbedItem {
	if tb := d.Cat.Testbeds[cmp(d.Orig, d.From)]; tb != nil && tb.HasMatrix() {
		return tb
	}
	return nil
}

// pickerSummary is the one-line summary under a picker.
func (d editData) pickerSummary(kind string) string {
	if d.pickerValue(kind) == inlineChoice {
		return "Written in the config file; choose a part to replace it."
	}
	return partSummaryOf(d.Cat, kind, d.pickedPart(kind))
}

// advancedSet reports whether any folded setting already has a value, so the
// fold opens on what the user has changed instead of hiding it.
func (d editData) advancedSet() bool {
	for _, f := range d.advancedFields() {
		if ui.Get(d.Value, f) != "" {
			return true
		}
	}
	return false
}

// title is the form's heading: New testbed, Edit testbed big, or Copy of
// testbed big.
func (d editData) title() string {
	what := singular(d.Kind)
	switch d.Mode {
	case "edit":
		return "Edit " + what + " " + d.Orig
	case "copy":
		return "Copy of " + what + " " + d.From
	}
	return "New " + what
}

// backURL is where Cancel goes: the entry edited or copied, else its subtab.
func (d editData) backURL() templ.SafeURL {
	return entryURL(d.Kind, cmp(d.Orig, d.From))
}

func (s *Server) editRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /edit/{slug}", s.editPage)
	mux.HandleFunc("GET /edit/{slug}/{name}", s.editPage)
	mux.HandleFunc("POST /edit/{slug}", s.editSave)
	mux.HandleFunc("POST /edit/{slug}/{name}/delete", s.editDelete)
	mux.HandleFunc("GET /edit-step", s.editStep)
	mux.HandleFunc("GET "+partSummaryPath, s.partSummary)
	mux.HandleFunc("GET "+partDialogPath, s.partDialog)
	mux.HandleFunc("GET "+matrixAxisPath, s.matrixAxis)
	mux.HandleFunc("POST "+matrixCombinationsPath, s.matrixCombinations)
}

// partSummary answers a testbed picker's change with the summary of the part
// it now shows, and that part's Open link. It is a fragment for htmx to swap
// in.
func (s *Server) partSummary(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if !slices.Contains(config.PartKinds, kind) {
		http.NotFound(w, r)
		return
	}
	name := r.URL.Query().Get("p." + kind)
	text, open := "Written in the config file; choose a part to replace it.", ""
	if name != inlineChoice {
		cat := s.svc.Catalog()
		open = cmp(name, cat.Fallback[kind])
		text = partSummaryOf(cat, kind, open)
	}
	render(w, r, pickerSummary(kind, text, open))
}

// partDialog is the form, for the testbed form's dialog, that makes a part of
// the kind the query names.
func (s *Server) partDialog(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if !slices.Contains(config.PartKinds, kind) {
		http.NotFound(w, r)
		return
	}
	render(w, r, partDialog(editData{
		Kind: kind, Mode: "new", Value: ui.NewValue(kind), Cat: s.svc.Catalog(),
		UIFile: shortPath(s.svc.UIFilePath()), IDPrefix: "new-",
	}))
}

// sameOrigin rejects a change requested by another site's page. The editor
// writes config that can run commands, so a form on any web page must not be
// able to post to a UI served on localhost.
func sameOrigin(w http.ResponseWriter, r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		http.Error(w, "cross-site request refused", http.StatusForbidden)
		return false
	}
	if o := r.Header.Get("Origin"); o != "" {
		if u, err := url.Parse(o); err != nil || u.Host != r.Host {
			http.Error(w, "cross-site request refused", http.StatusForbidden)
			return false
		}
	}
	return true
}

// editable are the kinds this UI has a form for. The service edits more; a
// kind joins this list together with its form.
var editable = []string{config.KindTestbed, config.KindTinkerbell, config.KindClusterAPI, config.KindManagementCluster, config.KindWorkloadCluster, config.KindHardware, config.KindTest, config.KindSuite, config.KindStep, config.KindPlan}

// editKind is the kind the path's slug names, when the UI has a form for it.
func (s *Server) editKind(w http.ResponseWriter, r *http.Request) (string, bool) {
	kind, ok := kindBySlug(r.PathValue("slug"))
	if !ok || !slices.Contains(editable, kind) {
		http.NotFound(w, r)
		return "", false
	}
	return kind, true
}

func (s *Server) editPage(w http.ResponseWriter, r *http.Request) {
	kind, ok := s.editKind(w, r)
	if !ok {
		return
	}
	if kind == config.KindPlan {
		s.planFormPage(w, r)
		return
	}
	cat := s.svc.Catalog()
	d := editData{
		Kind: kind, Mode: "new", Value: ui.NewValue(kind), Uses: usesFor(cat, kind), Tests: config.Names(cat.Tests), Cat: cat,
		UIFile: shortPath(s.svc.UIFilePath()),
	}
	name := r.PathValue("name")
	from := r.URL.Query().Get("from")
	if name != "" {
		from = name
	}
	if from != "" {
		e, err := s.svc.Entry(kind, from)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		d.Value, d.Origin = e.Value, e.Origin
		if name != "" && e.Editable {
			d.Mode, d.Orig, d.Name = "edit", name, name
			d.UsedBy = s.svc.UsedBy(kind, name)
		} else {
			d.Mode, d.From, d.Name = "copy", from, from+"-copy"
		}
	}
	switch kind {
	case config.KindTest:
		for i, st := range ui.Steps(d.Value) {
			d.Steps = append(d.Steps, stepData{Idx: fmt.Sprint(i), Kind: stepKind(st), Value: st})
		}
	case config.KindStep:
		d.Steps = []stepData{{Idx: "0", Kind: stepKind(d.Value), Value: d.Value, Single: true}}
	}
	render(w, r, editPage(s.layout(r, configTab(kind), ""), d))
}

// stepKind is the kind to edit a step as. A step copied from a config file
// has expanded, so a preset is edited as the container or command it became.
func stepKind(st map[string]any) string {
	if k := ui.StepKindOf(st); k != "" {
		return k
	}
	return ui.StepUses
}

var stepIdx = regexp.MustCompile(`^[0-9]{1,16}$`)

func (s *Server) editStep(w http.ResponseWriter, r *http.Request) {
	idx := r.URL.Query().Get("i")
	if !stepIdx.MatchString(idx) {
		http.Error(w, "bad step index", http.StatusBadRequest)
		return
	}
	render(w, r, stepRow(stepData{Idx: idx, Kind: ui.StepUses, Value: map[string]any{}}, usesFor(s.svc.Catalog(), config.KindTest)))
}

func (s *Server) editSave(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(w, r) {
		return
	}
	kind, ok := s.editKind(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		formError(w, r, err)
		return
	}
	// A part made in the testbed form's dialog shows its errors there, and
	// its reply is the kind's picker with the part picked.
	inDialog := r.PostForm.Get("then") == "picker" && slices.Contains(config.PartKinds, kind)
	fail := func(err error) {
		if inDialog {
			formErrorAt(w, r, err, "#dialog-error")
			return
		}
		formError(w, r, err)
	}
	orig, from, name := r.PostForm.Get("orig"), r.PostForm.Get("from"), strings.TrimSpace(r.PostForm.Get("name"))
	base := map[string]any{}
	switch {
	case orig != "":
		e, err := s.svc.Entry(kind, orig)
		if err != nil || !e.Editable {
			fail(fmt.Errorf("%q cannot be changed here", orig))
			return
		}
		base = e.Value
	case from != "":
		if e, err := s.svc.Entry(kind, from); err == nil {
			base = e.Value
		}
	}
	var v map[string]any
	var err error
	if kind == config.KindPlan {
		v, err = s.planFromForm(r)
	} else {
		v, err = applyForm(r, kind, base)
	}
	if err == nil && kind == config.KindTestbed && r.PostForm.Has("matrixSet") {
		// Only the editor posts matrixSet; a matrix it can't show is kept.
		err = setMatrix(s.svc.Catalog(), r.PostForm, v)
	}
	if err != nil {
		fail(err)
		return
	}
	if err := s.svc.SaveEntry(kind, orig, name, v); err != nil {
		fail(err)
		return
	}
	if inDialog {
		s.pickPart(w, r, kind, name)
		return
	}
	next := entryURL(kind, name)
	if kind == config.KindPlan && r.PostForm.Get("then") == "run" {
		// Save as plan… goes back to the run form, with the new plan picked.
		next = runTestsURL(name, nil)
	}
	w.Header().Set("HX-Redirect", string(next))
}

// pickPart answers a part saved in the testbed form's dialog. The kind's
// picker, with the part picked, replaces the one on the form, which keeps
// everything else typed in it, and the partSaved event closes the dialog.
func (s *Server) pickPart(w http.ResponseWriter, r *http.Request, kind, name string) {
	ev, _ := json.Marshal(map[string]any{"partSaved": map[string]string{"kind": kind}})
	w.Header().Set("HX-Retarget", "#picker-"+kind)
	w.Header().Set("HX-Reswap", "outerHTML")
	w.Header().Set("HX-Trigger-After-Settle", string(ev))
	render(w, r, partPicker(editData{Kind: config.KindTestbed, Value: map[string]any{kind: name}, Cat: s.svc.Catalog()}, kind))
}

// planFormData is a plan's form: what the other forms have, and the cards of
// the Run tests form, which are a plan's settings.
type planFormData struct {
	editData
	runTestsData
	Description string
}

// planFormPage is the form that creates a plan, edits one in the UI file, or
// copies one there.
func (s *Server) planFormPage(w http.ResponseWriter, r *http.Request) {
	name, q := r.PathValue("name"), r.URL.Query()
	from := cmp(name, q.Get("from"))
	p := planFormData{editData: editData{Kind: config.KindPlan, Mode: "new", UIFile: shortPath(s.svc.UIFilePath())}}
	if from != "" {
		e, err := s.svc.Entry(config.KindPlan, from)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		p.Origin = e.Origin
		p.Description, _ = e.Value["description"].(string)
		if name != "" && e.Editable {
			p.Mode, p.Orig, p.Name = "edit", name, name
		} else {
			p.Mode, p.editData.From, p.Name = "copy", from, from+"-copy"
		}
	}
	if q.Has("suitesSet") {
		// The form re-rendering itself keeps what was typed.
		p.Name, p.Description = q.Get("name"), q.Get("f.description")
	}
	p.runTestsData = s.runForm(r, from)
	p.FormURL = r.URL.Path
	render(w, r, planFormPage(s.layout(r, configTab(config.KindPlan), ""), p))
}

// planFromForm is the plan a posted form describes. It is sparse, like every
// entry the UIs write: what the schema defaults is left out.
func (s *Server) planFromForm(r *http.Request) (map[string]any, error) {
	// The diagnostics as posted: formSelection drops what matches the plan the
	// form started from, which a new plan must keep.
	posted, err := selection(r)
	if err != nil {
		return nil, err
	}
	sel, err := s.formSelection(r)
	if err != nil {
		return nil, err
	}
	sel.Diagnostics = posted.Diagnostics
	v := map[string]any{"testbed": sel.Testbed, "suites": anyList(sel.Suites)}
	if d := strings.TrimSpace(r.PostForm.Get("f.description")); d != "" {
		v["description"] = d
	}
	if len(sel.Combinations) > 0 {
		v["combinations"] = anyList(sel.Combinations)
	}
	if sel.LabelFilter != "" {
		v["labelFilter"] = sel.LabelFilter
	}
	if sel.Parallel > 1 {
		v["parallel"] = int64(sel.Parallel)
	}
	if sel.Keep != "" && sel.Keep != "never" {
		v["keep"] = sel.Keep
	}
	if sel.PauseOnFailure != nil && *sel.PauseOnFailure {
		v["pauseOnFailure"] = true
	}
	if d := sel.Diagnostics; d != nil {
		def := config.DefaultDiagnostics()
		if d.Collect != def.Collect {
			v["diagnostics"] = d.Collect
		}
		if d.Consoles != def.Consoles {
			v["consoles"] = d.Consoles
		}
	}
	return v, nil
}

func anyList(l []string) []any {
	out := make([]any, len(l))
	for i, s := range l {
		out[i] = s
	}
	return out
}

func (s *Server) editDelete(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(w, r) {
		return
	}
	kind, ok := s.editKind(w, r)
	if !ok {
		return
	}
	if err := s.svc.DeleteEntry(kind, r.PathValue("name")); err != nil {
		formError(w, r, err)
		return
	}
	w.Header().Set("HX-Redirect", string(entryURL(kind, "")))
}

// applyForm sets what the form says on base and returns it. Fields the form
// does not have are left as they were.
func applyForm(r *http.Request, kind string, base map[string]any) (map[string]any, error) {
	var fields []ui.Field
	switch kind {
	case config.KindTest:
		fields = ui.TestFields
	case config.KindSuite:
		fields = ui.SuiteFields
	default:
		fields = ui.PartFields(kind)
	}
	var errs []error
	for _, f := range fields {
		if !r.PostForm.Has("f." + f.Path) {
			continue
		}
		errs = append(errs, ui.Set(base, f, r.PostForm.Get("f."+f.Path)))
	}
	if kind == config.KindTestbed {
		errs = append(errs, applyPickers(r, base)...)
	}
	switch kind {
	case config.KindTest:
		var steps []map[string]any
		for i, idx := range r.PostForm["step"] {
			st, stepErrs, err := postedStep(r, idx, fmt.Sprintf("step %d", i+1))
			if err != nil {
				return nil, err
			}
			errs = append(errs, stepErrs...)
			steps = append(steps, st)
		}
		ui.SetSteps(base, steps)
	case config.KindStep:
		// The form is the whole step, so what it posts replaces the entry, as
		// a test's steps are replaced.
		st, stepErrs, err := postedStep(r, "0", "the step")
		if err != nil {
			return nil, err
		}
		errs = append(errs, stepErrs...)
		base = st
	}
	if err := errors.Join(errs...); err != nil {
		return nil, joinedMessage(err)
	}
	return base, nil
}

// postedStep reads the step the form posted under index idx. who names it in
// errors, e.g. "step 2".
func postedStep(r *http.Request, idx, who string) (map[string]any, []error, error) {
	if !stepIdx.MatchString(idx) {
		return nil, nil, errors.New("bad step index")
	}
	k := r.PostForm.Get("s" + idx + ".kind")
	if !slices.Contains(ui.StepKinds, k) {
		return nil, nil, fmt.Errorf("%s: choose what it runs", who)
	}
	st := map[string]any{}
	var errs []error
	for _, f := range ui.FieldsFor(k) {
		if err := ui.Set(st, f, r.PostForm.Get("s"+idx+"."+f.Path)); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", who, err))
		}
	}
	if missing := ui.Missing(st, ui.FieldsFor(k)); len(missing) > 0 {
		errs = append(errs, fmt.Errorf("%s needs %s", who, strings.Join(missing, ", ")))
	}
	return st, errs, nil
}

// maxInlineBytes bounds a part the form carries back, which only ever holds
// what a person wrote by hand in a config file.
const maxInlineBytes = 64 << 10

// applyPickers sets a testbed's parts from its pickers. A picker that was not
// posted leaves the part as it was; picking none leaves it out, so the
// fallback applies as it does for a testbed written in CUE.
func applyPickers(r *http.Request, base map[string]any) []error {
	var errs []error
	for _, kind := range config.PartKinds {
		if !r.PostForm.Has("p." + kind) {
			continue
		}
		switch choice := r.PostForm.Get("p." + kind); choice {
		case "":
			delete(base, kind)
		case inlineChoice:
			raw := r.PostForm.Get("inline." + kind)
			if len(raw) > maxInlineBytes {
				errs = append(errs, fmt.Errorf("%s: the inline part is too large", kind))
				continue
			}
			v, err := config.FromYAML(raw)
			m, isMap := v.(map[string]any)
			if err != nil || !isMap {
				errs = append(errs, fmt.Errorf("%s: the inline part could not be read back", kind))
				continue
			}
			base[kind] = m
		default:
			base[kind] = choice
		}
	}
	return errs
}

// joinedMessage is a joined error as one line.
func joinedMessage(err error) error {
	return errors.New(strings.ReplaceAll(err.Error(), "\n", "; "))
}

// editErrorHTML is how a failed save reads under the form.
func editErrorHTML(err error) string {
	var ee *service.EditError
	if !errors.As(err, &ee) {
		return `<span class="fail">` + html.EscapeString(err.Error()) + `</span>`
	}
	var b strings.Builder
	for _, e := range ee.Errs {
		b.WriteString(`<div class="fail">`)
		if e.Path != "" {
			b.WriteString(`<span class="mono">` + html.EscapeString(e.Path) + `</span> `)
		}
		b.WriteString(html.EscapeString(e.Msg) + `</div>`)
	}
	return b.String()
}
