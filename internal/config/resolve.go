package config

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	cueerrors "cuelang.org/go/cue/errors"

	"github.com/tinkerbell/wendy/internal/vocab"
)

// The kinds of part: their vocab IDs, the keys they are written under in
// parts:, and the fields a testbed names them with.
const (
	KindTinkerbell        = "tinkerbell"
	KindClusterAPI        = "clusterAPI"
	KindManagementCluster = "managementCluster"
	KindWorkloadCluster   = "workloadCluster"
	KindHardware          = "hardware"
)

// PartKinds lists the kinds of part in glossary order.
var PartKinds = []string{KindTinkerbell, KindClusterAPI, KindManagementCluster, KindWorkloadCluster, KindHardware}

// IsPart reports whether kind is a kind of part.
func IsPart(kind string) bool { return slices.Contains(PartKinds, kind) }

// partLabel is how a sentence names one part of a kind, e.g. "workload
// cluster part".
func partLabel(kind string) string {
	k, _ := vocab.KindOf(kind)
	return k.One
}

// Part is one reusable piece of a testbed: Tinkerbell, Cluster API, the
// management cluster, the workload cluster or hardware. It holds only the
// fields the user set; the resolved testbed supplies every default.
type Part map[string]any

// TestbedSpec is a testbed as written: the part it uses of each kind, the
// settings no single part owns, and its matrix. A part is a name, or a Part
// written inline.
type TestbedSpec struct {
	Description       string `json:"description,omitempty"`
	Tinkerbell        any    `json:"tinkerbell"`
	ClusterAPI        any    `json:"clusterAPI"`
	ManagementCluster any    `json:"managementCluster"`
	WorkloadCluster   any    `json:"workloadCluster"`
	Hardware          any    `json:"hardware"`
	// Topology, BootMode and Network are read by several parts, so none owns
	// them. Like a part's fields they hold only what was written.
	Topology string         `json:"topology,omitempty"`
	BootMode string         `json:"bootMode,omitempty"`
	Network  map[string]any `json:"network,omitempty"`
	Matrix   *Matrix        `json:"matrix,omitempty"`
}

// Ref is how the testbed names its part of a kind: a name (string) or an
// inline Part.
func (s TestbedSpec) Ref(kind string) any {
	switch kind {
	case KindTinkerbell:
		return s.Tinkerbell
	case KindClusterAPI:
		return s.ClusterAPI
	case KindManagementCluster:
		return s.ManagementCluster
	case KindWorkloadCluster:
		return s.WorkloadCluster
	case KindHardware:
		return s.Hardware
	}
	return nil
}

// setRef sets how the testbed names its part of a kind.
func (s *TestbedSpec) setRef(kind string, ref any) {
	switch kind {
	case KindTinkerbell:
		s.Tinkerbell = ref
	case KindClusterAPI:
		s.ClusterAPI = ref
	case KindManagementCluster:
		s.ManagementCluster = ref
	case KindWorkloadCluster:
		s.WorkloadCluster = ref
	case KindHardware:
		s.Hardware = ref
	}
}

// PartNames reports the name of the part the testbed uses of each kind. A
// part written inline has no name.
func (s TestbedSpec) PartNames() map[string]string {
	out := map[string]string{}
	for _, k := range PartKinds {
		if n, ok := s.Ref(k).(string); ok {
			out[k] = n
		}
	}
	return out
}

// Picks reports whether the testbed names the part, itself or in an axis
// value of its matrix.
func (s TestbedSpec) Picks(kind, name string) bool {
	if s.PartNames()[kind] == name {
		return true
	}
	if s.Matrix == nil {
		return false
	}
	for _, values := range s.Matrix.Axes {
		for _, v := range values {
			patch, _ := v.(map[string]any)
			if SelectedParts(patch)[kind] == name {
				return true
			}
		}
	}
	return false
}

// PartsLine says which part of each given kind is used, e.g. "Hardware ha ·
// Workload cluster k8s-1.37". A kind with no named part reads "inline".
func PartsLine(parts map[string]string, kinds []string) string {
	out := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		name := parts[kind]
		if name == "" {
			name = "inline"
		}
		k, _ := vocab.KindOf(kind)
		out = append(out, k.Label+" "+name)
	}
	return strings.Join(out, " · ")
}

// Parts returns the catalog's parts of a kind.
func (c *Catalog) Parts(kind string) map[string]*Item[Part] { return c.parts[kind] }

// resolver turns the testbeds of a merged catalog into concrete ones. It runs
// after the directories are merged because a testbed may name a part another
// directory defines.
type resolver struct {
	ctx    *cue.Context
	schema cue.Value
	cat    *Catalog
	seen   map[string]bool
}

// resolve fills in what every testbed resolves to and the combinations it
// makes. schema is #ResolvedTestbed.
func (c *Catalog) resolve(ctx *cue.Context, schema cue.Value) {
	r := &resolver{ctx: ctx, schema: schema, cat: c, seen: map[string]bool{}}
	r.checkMoved()
	used := map[string]bool{}
	for _, name := range Names(c.Testbeds) {
		// A directory that failed to reload hands back the items of its last
		// load, which the previous catalog may still be serving.
		tb := &TestbedItem{Item: c.Testbeds[name].Item}
		c.Testbeds[name] = tb
		p, ok := r.pick(name, tb.Pos, tb.Value)
		if !ok {
			continue
		}
		for k, n := range p.names {
			used[k+"/"+n] = true
		}
		if t, ok := r.build(name, p, ""); ok {
			tb.Resolved = &t
		}
		if !tb.HasMatrix() {
			if tb.Resolved != nil {
				tb.Combinations = []Combination{{Name: name, Axes: map[string]string{}, Parts: maps.Clone(p.names), Testbed: *tb.Resolved}}
			}
			continue
		}
		r.expand(name, tb)
	}
	r.checkUnused(used)
}

// picked is the parts a testbed uses.
type picked struct {
	parts map[string]Part
	// names holds the name of each named part; an inline part has none.
	names      map[string]string
	pos        map[string]string
	testbedPos string
	// settings are the testbed's own: topology, boot mode and network.
	settings TestbedSpec
}

// testbedSettings are the settings a testbed holds because more than one part
// reads them, so a Tinkerbell part may not set them.
var testbedSettings = []string{"bootMode", "network", "topology"}

// pick finds the parts spec uses, written at pos, and reports the names
// nothing defines.
func (r *resolver) pick(name, pos string, spec TestbedSpec) (*picked, bool) {
	p := &picked{parts: map[string]Part{}, names: map[string]string{}, pos: map[string]string{}, testbedPos: pos, settings: spec}
	ok := true
	for _, kind := range PartKinds {
		path := fmt.Sprintf("testbeds.%s.%s", name, kind)
		switch ref := spec.Ref(kind).(type) {
		case string:
			it, found := r.cat.Parts(kind)[ref]
			if !found {
				r.refError(pos, path, partLabel(kind), ref, Names(r.cat.Parts(kind)))
				ok = false
				continue
			}
			p.parts[kind], p.names[kind], p.pos[kind] = it.Value, ref, it.Pos
		case map[string]any:
			p.parts[kind], p.pos[kind] = Part(ref), pos
		default:
			r.report(posError(pos, path, "name a "+partLabel(kind)+" or write one inline"))
			ok = false
		}
	}
	return p, ok
}

// checkMoved reports the testbed's settings a Tinkerbell part holds, whether
// or not a testbed uses the part, in a part a testbed writes inline, and in a
// matrix axis value's patch of the part.
func (r *resolver) checkMoved() {
	check := func(part map[string]any, pos, path, msg string) {
		for _, key := range testbedSettings {
			if _, ok := part[key]; ok {
				r.report(posError(pos, path+"."+key, key+msg))
			}
		}
	}
	const onTheTestbed = " is the testbed's: set it on the testbed"
	for _, name := range Names(r.cat.Parts(KindTinkerbell)) {
		it := r.cat.Parts(KindTinkerbell)[name]
		check(it.Value, it.Pos, partPath(KindTinkerbell, name), onTheTestbed)
	}
	for _, name := range Names(r.cat.Testbeds) {
		tb := r.cat.Testbeds[name]
		if inline, ok := tb.Value.Tinkerbell.(map[string]any); ok {
			check(inline, tb.Pos, "testbeds."+name+".tinkerbell", onTheTestbed)
		}
		if m := tb.Value.Matrix; m != nil {
			for _, axis := range m.Order {
				for _, value := range m.Values[axis] {
					patch, _ := m.Axes[axis][value].(map[string]any)
					if tink, ok := patch[KindTinkerbell].(map[string]any); ok {
						check(tink, axisPos(tb, axis, value), axisPath(name, axis, value)+".tinkerbell",
							" is the testbed's: write it beside tinkerbell in the axis value")
					}
				}
			}
		}
	}
}

// partPath is where a named part is written: parts.<kind>.<name>.
func partPath(kind, name string) string { return "parts." + kind + "." + name }

// axisPath is where a value of a testbed's matrix axis is written.
func axisPath(testbed, axis, value string) string {
	return fmt.Sprintf("testbeds.%s.matrix.axes.%s.%s", testbed, axis, value)
}

// axisPos is the position of an axis value, or of its testbed when the value
// has none.
func axisPos(tb *TestbedItem, axis, value string) string {
	if pos := tb.Value.Matrix.AxisPos[axis][value]; pos != "" {
		return pos
	}
	return tb.Pos
}

// assemble lays the parts out as the fields of a #ResolvedTestbed. A key a
// part does not know lands in the section it would belong to, so the schema
// rejects it there and the error maps back to the part.
func assemble(p *picked) map[string]any {
	flat := map[string]any{}
	section := func(name string) map[string]any {
		m, _ := flat[name].(map[string]any)
		if m == nil {
			m = map[string]any{}
			flat[name] = m
		}
		return m
	}
	for k, v := range p.parts[KindTinkerbell] {
		// The keys checkMoved reports are left out so each is reported once.
		if !slices.Contains(testbedSettings, k) {
			section("tinkerbell")[k] = v
		}
	}
	if s := p.settings; s.Topology != "" {
		section("tinkerbell")["topology"] = s.Topology
	}
	if s := p.settings; s.BootMode != "" {
		section("tinkerbell")["bootMode"] = s.BootMode
	}
	if n := p.settings.Network; n != nil {
		flat["network"] = n
	}
	// The other parts are each a section of their own, named as the kind,
	// except that a Hardware part's arch is the testbed's.
	for _, kind := range []string{KindClusterAPI, KindManagementCluster, KindWorkloadCluster, KindHardware} {
		for k, v := range p.parts[kind] {
			if kind == KindHardware && k == archKey {
				flat[k] = v
				continue
			}
			section(kind)[k] = v
		}
	}
	return flat
}

// eval unifies the assembled testbed with the schema.
func (r *resolver) eval(flat map[string]any) (Testbed, error) {
	v := r.ctx.Encode(flat).Unify(r.schema)
	var t Testbed
	if err := v.Validate(cue.Concrete(true)); err != nil {
		return t, err
	}
	return t, v.Decode(&t)
}

// build resolves one testbed and reports what is wrong with it, at the part
// each error belongs to. With only set, just that kind's errors are reported.
func (r *resolver) build(name string, p *picked, only string) (Testbed, bool) {
	t, err := r.eval(assemble(p))
	if err == nil {
		return t, true
	}
	for _, e := range toErrors(err) {
		kind, rel := ownerOf(e.Path)
		if only != "" && kind != only {
			continue
		}
		var pos, path string
		switch {
		case kind != "" && p.names[kind] != "":
			pos, path = p.pos[kind], partPath(kind, p.names[kind])
		case kind != "":
			pos, path = p.pos[kind], "testbeds."+name+"."+kind
		default:
			pos, path = p.testbedPos, "testbeds."+name
		}
		if rel != "" {
			path += "." + rel
		}
		r.report(posError(pos, path, e.Msg))
	}
	return t, false
}

// ownerOf is the kind of part a path in a #ResolvedTestbed comes from, and the
// path within that part. A path no part owns has no kind.
func ownerOf(path string) (kind, rel string) {
	first, rest, _ := strings.Cut(path, ".")
	switch first {
	case "network":
		return "", path
	case KindTinkerbell:
		if setting, _, _ := strings.Cut(rest, "."); setting == "topology" || setting == "bootMode" {
			return "", rest
		}
		return KindTinkerbell, rest
	case archKey:
		return KindHardware, path
	case KindClusterAPI, KindManagementCluster, KindWorkloadCluster, KindHardware:
		return first, rest
	}
	return "", path
}

// checkUnused validates the parts no testbed uses, each next to the fallback
// of every other kind, so a mistake in one shows up before it is used.
func (r *resolver) checkUnused(used map[string]bool) {
	defaults := &picked{parts: map[string]Part{}, names: map[string]string{}, pos: map[string]string{}}
	for _, kind := range PartKinds {
		name := r.cat.Fallback[kind]
		d, ok := r.cat.Parts(kind)[name]
		if !ok {
			return
		}
		defaults.parts[kind], defaults.names[kind], defaults.pos[kind] = d.Value, name, d.Pos
	}
	for _, kind := range PartKinds {
		for _, name := range Names(r.cat.Parts(kind)) {
			if used[kind+"/"+name] {
				continue
			}
			it := r.cat.Parts(kind)[name]
			p := &picked{parts: map[string]Part{}, names: map[string]string{}, pos: map[string]string{}}
			for k := range defaults.parts {
				p.parts[k], p.names[k], p.pos[k] = defaults.parts[k], defaults.names[k], defaults.pos[k]
			}
			p.parts[kind], p.names[kind], p.pos[kind] = it.Value, name, it.Pos
			r.build("", p, kind)
		}
	}
}

// expand fills a testbed's combinations: for each choice of one value on
// every axis, the testbed as written with those values written over it.
func (r *resolver) expand(name string, tb *TestbedItem) {
	m := tb.Value.Matrix
	if !r.checkAxes(name, tb) {
		return
	}
	axisValues := make([][]string, len(m.Order))
	for i, axis := range m.Order {
		axisValues[i] = m.Values[axis]
	}
	for _, values := range product(axisValues) {
		c := Combination{Name: strings.Join(values, "-"), Axes: map[string]string{}}
		over := make([]axisValue, len(m.Order))
		for i, axis := range m.Order {
			c.Axes[axis] = values[i]
			patch, _ := m.Axes[axis][values[i]].(map[string]any)
			over[i] = axisValue{axis: axis, patch: patch}
		}
		if reason, ok := m.Excluded(c.Axes); ok {
			c.Excluded = reason
		}
		spec, err := r.combine(tb.Value, over)
		if err != nil {
			c.Err = err.Error()
		} else {
			// The testbed's own picks and checkAxes have checked every name
			// spec can hold, so pick finds them all.
			p, _ := r.pick(name, tb.Pos, spec)
			c.Parts = maps.Clone(p.names)
			if t, err := r.eval(assemble(p)); err != nil {
				c.Err = cueerrors.Details(err, nil)
			} else {
				c.Testbed = t
			}
		}
		tb.Combinations = append(tb.Combinations, c)
	}
}

// checkAxes reports the parts a testbed's axis values name that nothing
// defines, once each.
func (r *resolver) checkAxes(name string, tb *TestbedItem) bool {
	m := tb.Value.Matrix
	ok := true
	for _, axis := range m.Order {
		for _, value := range m.Values[axis] {
			patch, _ := m.Axes[axis][value].(map[string]any)
			picks := SelectedParts(patch)
			for _, kind := range PartKinds {
				part, picked := picks[kind]
				if !picked {
					continue
				}
				if _, found := r.cat.Parts(kind)[part]; !found {
					r.refError(axisPos(tb, axis, value), axisPath(name, axis, value)+"."+kind, partLabel(kind), part, Names(r.cat.Parts(kind)))
					ok = false
				}
			}
		}
	}
	return ok
}

// SelectedParts are the parts an axis value picks by name, by kind. The
// resolver and the editors' "used by" share it, so they agree on what counts
// as a pick.
func SelectedParts(axisValue map[string]any) map[string]string {
	out := map[string]string{}
	for _, kind := range PartKinds {
		if name, ok := axisValue[kind].(string); ok {
			out[kind] = name
		}
	}
	return out
}

// axisValue is the value one axis takes in a combination.
type axisValue struct {
	axis  string
	patch map[string]any
}

// combine is the testbed as written with the axis values of one combination
// written over it. A setting replaces the testbed's, a part an axis names
// replaces the testbed's, and a part's fields are merged over the part the
// combination uses, so one axis can resize the hardware another picks. Two
// axes that set one field differently conflict, whatever their order.
func (r *resolver) combine(spec TestbedSpec, over []axisValue) (TestbedSpec, error) {
	if err := conflict(over); err != nil {
		return spec, err
	}
	out := spec
	out.Matrix = nil
	for _, v := range over {
		for kind, name := range SelectedParts(v.patch) {
			out.setRef(kind, name)
		}
	}
	for _, v := range over {
		for _, kind := range PartKinds {
			if fields, ok := v.patch[kind].(map[string]any); ok && len(fields) > 0 {
				part := r.partFields(kind, out.Ref(kind))
				mergeFields(part, fields)
				out.setRef(kind, part)
			}
		}
		if s, ok := v.patch["topology"].(string); ok {
			out.Topology = s
		}
		if s, ok := v.patch["bootMode"].(string); ok {
			out.BootMode = s
		}
		if n, ok := v.patch["network"].(map[string]any); ok {
			network := cloneMap(out.Network)
			mergeFields(network, n)
			out.Network = network
		}
	}
	return out, nil
}

// partFields are a copy of the fields of the part ref names or holds.
func (r *resolver) partFields(kind string, ref any) map[string]any {
	switch ref := ref.(type) {
	case string:
		if it := r.cat.Parts(kind)[ref]; it != nil {
			return cloneMap(it.Value)
		}
	case map[string]any:
		return cloneMap(ref)
	}
	return map[string]any{}
}

// conflict reports two axes that set one field to different values. A part
// one axis picks and fields another merges over it do not conflict.
func conflict(over []axisValue) error {
	type setBy struct {
		axis  string
		value any
	}
	set := map[string]setBy{}
	for _, v := range over {
		for _, l := range leaves(v.patch, "") {
			prev, ok := set[l.path]
			if !ok {
				set[l.path] = setBy{v.axis, l.value}
				continue
			}
			if reflect.DeepEqual(prev.value, l.value) {
				continue
			}
			if IsPart(l.path) {
				return fmt.Errorf("axes %s and %s both select a %s (%q and %q); one axis should choose it",
					prev.axis, v.axis, partLabel(l.path), prev.value, l.value)
			}
			return fmt.Errorf("axes %s and %s both set %s (%s and %s); one axis should set it",
				prev.axis, v.axis, l.path, show(prev.value), show(l.value))
		}
	}
	return nil
}

// leaf is one field a patch sets, by its dotted path.
type leaf struct {
	path  string
	value any
}

// leaves are the fields a patch sets, in path order. A struct is not a field
// of its own: its fields are.
func leaves(patch map[string]any, prefix string) []leaf {
	var out []leaf
	for _, k := range Names(patch) {
		if sub, ok := patch[k].(map[string]any); ok {
			out = append(out, leaves(sub, prefix+k+".")...)
			continue
		}
		out = append(out, leaf{prefix + k, patch[k]})
	}
	return out
}

// show is a value as CUE would write it.
func show(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// mergeFields writes src's fields over dst's, merging the structs both hold.
func mergeFields(dst, src map[string]any) {
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				mergeFields(dm, sm)
				continue
			}
		}
		dst[k] = cloneValue(v)
	}
}

// cloneMap is a deep copy of m, so merging into it leaves m alone.
func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = cloneValue(v)
	}
	return out
}

func cloneValue(v any) any {
	switch v := v.(type) {
	case map[string]any:
		return cloneMap(v)
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = cloneValue(e)
		}
		return out
	}
	return v
}

// refError reports a reference to a name nothing defines, once.
func (r *resolver) refError(pos, path, kind, name string, candidates []string) {
	msg := fmt.Sprintf("unknown %s %q", kind, name)
	if s := Suggest(name, candidates); s != "" {
		msg += fmt.Sprintf(" (did you mean %q?)", s)
	}
	r.report(posError(pos, path, msg))
}

// report adds an error unless the same one was already reported, as happens
// when several testbeds use one faulty part.
func (r *resolver) report(e Error) {
	key := e.File + ":" + strconv.Itoa(e.Line) + ":" + strconv.Itoa(e.Col) + "|" + e.Path + "|" + e.Msg
	if r.seen[key] {
		return
	}
	r.seen[key] = true
	r.cat.pushError(e)
}
