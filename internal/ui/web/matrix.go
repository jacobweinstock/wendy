package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/ui"
	"github.com/tinkerbell/wendy/internal/vocab"
)

// The testbed form's matrix editor. It edits the matrices most testbeds have,
// where each axis value picks a part or sets one setting; anything richer
// stays CUE-only and is shown read-only.

// matrixSettings are the testbed settings an axis can vary, by their path.
var matrixSettings = []string{"topology", "bootMode", "network.family"}

// axisNames are the names a new axis gets for what it varies.
var axisNames = map[string]string{"topology": "topology", "bootMode": "boot", "network.family": "family"}

// variesChoice is something an axis can vary: a kind of part, or a setting.
type variesChoice struct{ ID, Label string }

func variesChoices() []variesChoice {
	var out []variesChoice
	for _, k := range config.PartKinds {
		out = append(out, variesChoice{k, kindLabel(k)})
	}
	for _, p := range matrixSettings {
		if f, ok := settingField(p); ok {
			out = append(out, variesChoice{p, f.Label})
		}
	}
	return out
}

func settingField(path string) (ui.Field, bool) {
	i := slices.IndexFunc(ui.TestbedFields, func(f ui.Field) bool { return f.Path == path })
	if i < 0 {
		return ui.Field{}, false
	}
	return ui.TestbedFields[i], true
}

// axisOption is one value an axis can take: a part's name or a setting's
// value (ID), and the axis value it is written as, which names combinations.
type axisOption struct {
	ID, Name, Label string
	On              bool
}

// axisRow is one axis as the editor shows it.
type axisRow struct {
	Idx, Name, Varies string
	Options           []axisOption
}

// axisOptions are the values an axis varying what can take. Those in values,
// by ID, are on and keep the name they are written as.
func axisOptions(cat *config.Catalog, varies string, values map[string]string) []axisOption {
	part := slices.Contains(config.PartKinds, varies)
	var ids []string
	if part {
		ids = config.Names(cat.Parts(varies))
	} else if f, ok := settingField(varies); ok {
		ids = f.Options
	}
	out := make([]axisOption, 0, len(ids))
	for _, id := range ids {
		name, on := values[id]
		label := id
		if !part {
			label = vocab.Choice(varies, id)
		}
		out = append(out, axisOption{ID: id, Name: cmp(name, id), Label: label, On: on})
	}
	return out
}

// newAxisRow is an axis the editor adds, varying the workload cluster until
// the user picks something else.
func newAxisRow(cat *config.Catalog, idx, name, varies string) axisRow {
	if !slices.ContainsFunc(variesChoices(), func(c variesChoice) bool { return c.ID == varies }) {
		varies = config.KindWorkloadCluster
	}
	if name == "" {
		name = cmp(axisNames[varies], slugOf(varies))
	}
	return axisRow{Idx: idx, Name: name, Varies: varies, Options: axisOptions(cat, varies, nil)}
}

// patchPick reads what an axis value sets when it sets exactly one thing the
// editor offers: a part by name, or one setting.
func patchPick(patch any) (varies, id string, ok bool) {
	m, isMap := patch.(map[string]any)
	if !isMap || len(m) != 1 {
		return "", "", false
	}
	for k, v := range m {
		if k == "network" {
			n, isMap := v.(map[string]any)
			family, isStr := n["family"].(string)
			return "network.family", family, isMap && len(n) == 1 && isStr
		}
		s, isStr := v.(string)
		return k, s, isStr && (slices.Contains(config.PartKinds, k) || k == "topology" || k == "bootMode")
	}
	return "", "", false
}

// patchOf is what an axis value varying varies writes to pick id.
func patchOf(varies, id string) map[string]any {
	if varies == "network.family" {
		return map[string]any{"network": map[string]any{"family": id}}
	}
	return map[string]any{varies: id}
}

// editableRows are a matrix's axes as the editor shows them, and whether it
// can show them at all: each axis's values set one thing it offers, the same
// kind of thing, and each exclude names every axis.
func editableRows(cat *config.Catalog, m *config.Matrix) ([]axisRow, bool) {
	if m == nil {
		return nil, true
	}
	if len(m.Axes) != len(m.Order) {
		return nil, false
	}
	var rows []axisRow
	for i, axis := range m.Order {
		row := axisRow{Idx: fmt.Sprint(i), Name: axis}
		values := map[string]string{}
		for name, patch := range m.Axes[axis] {
			varies, id, ok := patchPick(patch)
			if !ok || (row.Varies != "" && row.Varies != varies) {
				return nil, false
			}
			row.Varies, values[id] = varies, name
		}
		row.Options = axisOptions(cat, row.Varies, values)
		rows = append(rows, row)
	}
	for _, ex := range m.Exclude {
		if len(ex) != len(m.Order)+1 {
			return nil, false
		}
	}
	return rows, true
}

// postedAxis is an axis as the form posts it: its values, each the ID picked
// and the name it is written as.
type postedAxis struct {
	Name, Varies string
	Values       [][2]string
}

// axesOf are the axes the rows have, leaving out those with no value on.
func axesOf(rows []axisRow) []postedAxis {
	var out []postedAxis
	for _, r := range rows {
		a := postedAxis{Name: r.Name, Varies: r.Varies}
		for _, o := range r.Options {
			if o.On {
				a.Values = append(a.Values, [2]string{o.ID, o.Name})
			}
		}
		if len(a.Values) > 0 {
			out = append(out, a)
		}
	}
	return out
}

// postedAxes are the axes the form posts, in its order. An axis with no
// value picked is left out.
func postedAxes(cat *config.Catalog, form url.Values) ([]postedAxis, error) {
	var rows []axisRow
	seen := map[string]bool{}
	for _, idx := range form["ax"] {
		if !stepIdx.MatchString(idx) {
			return nil, errors.New("bad axis index")
		}
		p := "ax" + idx + "."
		name := strings.TrimSpace(form.Get(p + "name"))
		if name == "" {
			return nil, errors.New("an axis needs a name")
		}
		if seen[name] {
			return nil, fmt.Errorf("two axes are named %q", name)
		}
		seen[name] = true
		values := map[string]string{}
		for _, id := range form[p+"v"] {
			values[id] = strings.TrimSpace(form.Get(p + "as." + id))
		}
		row := newAxisRow(cat, idx, name, form.Get(p+"varies"))
		row.Options = axisOptions(cat, row.Varies, values)
		rows = append(rows, row)
	}
	return axesOf(rows), nil
}

// comboRow is one combination the editor lists, and whether it is left out.
type comboRow struct {
	Name, Reason string
	Values       map[string]string
	Out          bool
}

// comboRows are the axes' combinations, named as the config names them,
// each left out as out says.
func comboRows(axes []postedAxis, out func(c comboRow) (bool, string)) []comboRow {
	if len(axes) == 0 {
		return nil
	}
	rows := []comboRow{{Values: map[string]string{}}}
	for _, a := range axes {
		var next []comboRow
		for _, r := range rows {
			for _, v := range a.Values {
				c := comboRow{Name: strings.TrimPrefix(r.Name+"-"+v[1], "-"), Values: map[string]string{}}
				for k, x := range r.Values {
					c.Values[k] = x
				}
				c.Values[a.Name] = v[1]
				next = append(next, c)
			}
		}
		rows = next
	}
	for i := range rows {
		rows[i].Out, rows[i].Reason = out(rows[i])
	}
	return rows
}

// leftOutInForm says which combinations the form leaves out, and why.
func leftOutInForm(form url.Values) func(comboRow) (bool, string) {
	return func(c comboRow) (bool, string) {
		return form.Get("ex."+c.Name) != "", form.Get("ex." + c.Name + ".reason")
	}
}

// setMatrix sets a testbed's matrix to what the form's editor describes, or
// removes it when the editor has no axis with a value picked.
func setMatrix(cat *config.Catalog, form url.Values, testbed map[string]any) error {
	m, err := matrixFrom(cat, form)
	switch {
	case err != nil:
		return err
	case m == nil:
		delete(testbed, "matrix")
	default:
		testbed["matrix"] = m
	}
	return nil
}

// matrixFrom is the matrix the form's editor describes, or nil for none.
func matrixFrom(cat *config.Catalog, form url.Values) (map[string]any, error) {
	axes, err := postedAxes(cat, form)
	if err != nil || len(axes) == 0 {
		return nil, err
	}
	order := make([]any, 0, len(axes))
	values := map[string]any{}
	for _, a := range axes {
		order = append(order, a.Name)
		vs := map[string]any{}
		for _, v := range a.Values {
			vs[v[1]] = patchOf(a.Varies, v[0])
		}
		values[a.Name] = vs
	}
	m := map[string]any{"order": order, "axes": values}
	var exclude []any
	for _, c := range comboRows(axes, leftOutInForm(form)) {
		if !c.Out {
			continue
		}
		ex := map[string]any{"reason": cmp(strings.TrimSpace(c.Reason), "left out")}
		for axis, v := range c.Values {
			ex[axis] = v
		}
		exclude = append(exclude, ex)
	}
	if len(exclude) > 0 {
		m["exclude"] = exclude
	}
	return m, nil
}

// matrixRows are the testbed's axes for the editor, and whether the editor
// can show its matrix.
func (d editData) matrixRows() ([]axisRow, bool) {
	tb := d.matrixTestbed()
	if tb == nil {
		return nil, true
	}
	return editableRows(d.Cat, tb.Value.Matrix)
}

// combinations are the combinations the rows make, left out as the
// testbed's matrix leaves them out.
func (d editData) combinations(rows []axisRow) []comboRow {
	tb := d.matrixTestbed()
	return comboRows(axesOf(rows), func(c comboRow) (bool, string) {
		if tb == nil {
			return false, ""
		}
		reason, out := tb.Value.Matrix.Excluded(c.Values)
		return out, reason
	})
}

// matrixAxis answers the editor with one axis: a new one, or one whose
// "varies" changed, with nothing picked.
func (s *Server) matrixAxis(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	idx := q.Get("i")
	if !stepIdx.MatchString(idx) {
		http.Error(w, "bad axis index", http.StatusBadRequest)
		return
	}
	render(w, r, axisRowView(newAxisRow(s.svc.Catalog(), idx, q.Get("ax"+idx+".name"), q.Get("ax"+idx+".varies"))))
}

// matrixCombinations answers the editor with the combinations its axes make.
func (s *Server) matrixCombinations(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	axes, err := postedAxes(s.svc.Catalog(), r.PostForm)
	if err != nil {
		render(w, r, formProblem(err.Error()))
		return
	}
	render(w, r, combinationList(comboRows(axes, leftOutInForm(r.PostForm))))
}
