// Package config loads CUE configuration into a Catalog.
package config

import (
	"cmp"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/cue/token"

	wendycue "github.com/tinkerbell/wendy/cue"
)

// ImportPath is how users import the schema.
const ImportPath = "github.com/tinkerbell/wendy/cue/wendy"

// BuiltIn is the Origin of everything wendy itself defines.
const BuiltIn = "built-in"

// schemaRoot is where the embedded schema lives in the CUE overlay. It never
// exists on disk.
const schemaRoot = "/wendy-schema"

// Builtin describes a built-in step for the generated #Builtins.
type Builtin struct {
	ID          string
	Schema      string
	Description string
}

// Item is one named catalog entry and where it was defined.
type Item[T any] struct {
	Name   string
	Origin string
	Pos    string
	Source string
	Value  T
}

// Combination is one combination of a testbed: the testbed with one value of
// each axis of its matrix written over it. A testbed without a matrix is one
// combination, named after it.
type Combination struct {
	Name string
	// Axes holds the value the combination takes on each axis of the matrix.
	Axes map[string]string
	// Parts names the part the combination uses of each kind. A kind built
	// from an inline part, or from a part an axis patched, has no entry.
	Parts    map[string]string
	Testbed  Testbed
	Excluded string
	Err      string
}

// TestbedItem is a testbed as written, resolved, and the combinations it
// makes.
type TestbedItem struct {
	Item[TestbedSpec]
	// Resolved is the testbed without its matrix, with every default
	// applied. It is nil when the testbed does not resolve; the catalog's
	// errors say why.
	Resolved *Testbed
	// Combinations are one per combination of the matrix, or the one
	// combination of a testbed without a matrix when it resolves.
	Combinations []Combination
}

// HasMatrix reports whether the testbed is several combinations.
func (t *TestbedItem) HasMatrix() bool { return t.Value.Matrix != nil }

// Error is a config error with the position a user can jump to.
type Error struct {
	File string `json:"file,omitempty"`
	Line int    `json:"line,omitempty"`
	Col  int    `json:"col,omitempty"`
	Path string `json:"path,omitempty"`
	Msg  string `json:"msg"`
}

func (e Error) Error() string {
	var b strings.Builder
	if e.File != "" {
		fmt.Fprintf(&b, "%s:%d:%d: ", e.File, e.Line, e.Col)
	}
	if e.Path != "" {
		b.WriteString(e.Path + ": ")
	}
	b.WriteString(e.Msg)
	return b.String()
}

// FileStatus is a loaded file and its errors.
type FileStatus struct {
	Path   string  `json:"path"`
	Errors []Error `json:"errors,omitempty"`
}

// Catalog is everything the config directories define, plus the built-ins.
type Catalog struct {
	Dirs          []string
	Files         []FileStatus
	Errors        []Error
	LoadedAt      time.Time
	HasUserConfig bool
	// Stale is set when a directory failed to load and its last good result
	// is still in use.
	Stale bool

	// Testbeds are the testbeds as written, each with what it resolves to.
	Testbeds     map[string]*TestbedItem
	Tests        map[string]*Item[Test]
	Suites       map[string]*Item[Suite]
	Plans        map[string]*Item[Plan]
	BuiltinSteps []Builtin
	// Steps are the user's own steps, as written; Expand puts one in place of
	// a step that uses it.
	Steps map[string]*Item[Step]
	// Fallback is, by kind of part, the part a testbed gets when it names
	// none.
	Fallback map[string]string

	// parts are the parts of each kind, by kind; see Parts.
	parts map[string]map[string]*Item[Part]
	// stepTimeout is #Step's default timeout, which a step that uses one of
	// the user's does not override.
	stepTimeout string
}

// Names returns the sorted keys of a catalog map.
func Names[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Loader evaluates config directories. It remembers the last good result of
// each directory so a broken file never takes the rest of the config down.
type Loader struct {
	Dirs     []string
	Tags     []string
	Builtins []Builtin

	mu   sync.Mutex
	last map[string]*dirResult
}

type dirResult struct {
	files  []string
	cat    *Catalog
	errors []Error
}

// Load evaluates every directory and merges them, later ones taking
// precedence over earlier ones name by name.
func (l *Loader) Load() *Catalog {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.last == nil {
		l.last = map[string]*dirResult{}
	}
	ctx := cuecontext.New()
	cat := newCatalog(l.Builtins)
	cat.LoadedAt = time.Now()
	cat.Dirs = l.Dirs

	schema, err := loadSchema(ctx, l.Builtins)
	if err != nil {
		cat.Errors = append(cat.Errors, toErrors(err)...)
		return cat
	}
	cat.Fallback = fallbacks(schema)
	if v, ok := schema.LookupPath(cue.ParsePath("#Step.timeout")).Default(); ok {
		cat.stepTimeout, _ = v.String()
	}
	base := extract(ctx.CompileString("{}").Unify(schema.LookupPath(cue.ParsePath("#Config"))))
	cat.merge(base)
	cat.Errors = append(cat.Errors, base.Errors...)

	for _, dir := range l.Dirs {
		files := configFiles(dir)
		if len(files) == 0 {
			delete(l.last, dir)
			continue
		}
		cat.HasUserConfig = true
		res := l.loadDir(ctx, dir, files)
		if res.cat != nil {
			res.errors = append(res.errors, res.cat.keepBuiltins(base)...)
		}
		if res.cat == nil {
			if prev := l.last[dir]; prev != nil {
				res.cat = prev.cat
				cat.Stale = true
			}
		} else {
			l.last[dir] = res
		}
		byFile := map[string][]Error{}
		for _, e := range res.errors {
			byFile[e.File] = append(byFile[e.File], e)
		}
		for _, f := range files {
			rel := displayPath(f)
			cat.Files = append(cat.Files, FileStatus{Path: rel, Errors: byFile[rel]})
			delete(byFile, rel)
		}
		for _, errs := range byFile {
			cat.Errors = append(cat.Errors, errs...)
		}
		if res.cat != nil {
			cat.merge(res.cat)
		}
	}
	cat.resolve(ctx, schema.LookupPath(cue.ParsePath("#ResolvedTestbed")))
	cat.crossCheck()
	return cat
}

func (l *Loader) loadDir(ctx *cue.Context, dir string, files []string) *dirResult {
	res := &dirResult{files: files}
	ov, err := overlay(dir, l.Builtins)
	if err != nil {
		res.errors = []Error{{File: displayPath(dir), Msg: err.Error()}}
		return res
	}
	cfg := &load.Config{Dir: dir, Overlay: ov, DataFiles: true, Tags: tagsFor(files, l.Tags)}
	// Loaded together so the user's import and the #Config unified below are
	// one instance; closedness from two copies of a definition conflicts.
	insts := load.Instances([]string{".", ImportPath}, cfg)
	if len(insts) < 2 {
		return res
	}
	for _, inst := range insts {
		if inst.Err != nil {
			res.errors = toErrors(inst.Err)
			return res
		}
	}
	schemaV := ctx.BuildInstance(insts[1])
	v := ctx.BuildInstance(insts[0])
	if v.Err() != nil {
		res.errors = toErrors(v.Err())
		return res
	}
	v = schemaV.LookupPath(cue.ParsePath("#Config")).Unify(v)
	if err := v.Validate(); err != nil {
		res.errors = toErrors(err)
		return res
	}
	res.cat = extract(v)
	res.errors = res.cat.Errors
	return res
}

func newCatalog(builtins []Builtin) *Catalog {
	steps := slices.Clone(builtins)
	sort.Slice(steps, func(i, j int) bool { return steps[i].ID < steps[j].ID })
	c := &Catalog{
		Testbeds:     map[string]*TestbedItem{},
		Tests:        map[string]*Item[Test]{},
		Suites:       map[string]*Item[Suite]{},
		Plans:        map[string]*Item[Plan]{},
		Steps:        map[string]*Item[Step]{},
		BuiltinSteps: steps,
		parts:        map[string]map[string]*Item[Part]{},
	}
	for _, kind := range PartKinds {
		c.parts[kind] = map[string]*Item[Part]{}
	}
	return c
}

// extract decodes every entry of an evaluated #Config, entry by entry, so one
// incomplete testbed does not hide the others.
func extract(v cue.Value) *Catalog {
	c := newCatalog(nil)
	for _, kind := range PartKinds {
		each(v, "parts."+kind, func(name string, pv cue.Value) {
			if it, err := decodeItem[Part](name, pv); err != nil {
				c.Errors = append(c.Errors, err...)
			} else {
				c.parts[kind][name] = it
			}
		})
	}
	each(v, "testbeds", func(name string, tv cue.Value) {
		tb, errs := decodeTestbed(name, tv)
		c.Errors = append(c.Errors, errs...)
		if tb != nil {
			c.Testbeds[name] = tb
		}
	})
	each(v, "tests", func(name string, sv cue.Value) {
		if it, err := decodeItem[Test](name, sv); err != nil {
			c.Errors = append(c.Errors, err...)
		} else {
			c.Tests[name] = it
		}
	})
	each(v, "suites", func(name string, sv cue.Value) {
		if it, err := decodeItem[Suite](name, sv); err != nil {
			c.Errors = append(c.Errors, err...)
		} else {
			c.Suites[name] = it
		}
	})
	each(v, "plans", func(name string, pv cue.Value) {
		if it, err := decodeItem[Plan](name, pv); err != nil {
			c.Errors = append(c.Errors, err...)
		} else {
			c.Plans[name] = it
		}
	})
	each(v, "steps", func(name string, sv cue.Value) {
		if it, err := decodeItem[Step](name, sv); err != nil {
			c.Errors = append(c.Errors, err...)
		} else {
			c.Steps[name] = it
		}
	})
	return c
}

// fallbacks reads, by kind of part, the part #Testbed defaults to.
func fallbacks(schema cue.Value) map[string]string {
	out := map[string]string{}
	for _, kind := range PartKinds {
		v, _ := schema.LookupPath(cue.ParsePath("#Testbed." + kind)).Default()
		if name, err := v.String(); err == nil {
			out[kind] = name
		}
	}
	return out
}

func each(v cue.Value, field string, fn func(string, cue.Value)) {
	it, err := v.LookupPath(cue.ParsePath(field)).Fields()
	if err != nil {
		return
	}
	for it.Next() {
		fn(it.Selector().Unquoted(), it.Value())
	}
}

func decodeItem[T any](name string, v cue.Value) (*Item[T], []Error) {
	if err := v.Validate(cue.Concrete(true)); err != nil {
		return nil, toErrors(err)
	}
	var out T
	if err := v.Decode(&out); err != nil {
		return nil, toErrors(err)
	}
	return &Item[T]{Name: name, Origin: origin(v), Pos: posString(v), Source: source(v), Value: out}, nil
}

// decodeTestbed reads a testbed, and the order of its axis values and where
// each is written. What it resolves to is worked out once the directories are
// merged, because the parts it names may come from another.
func decodeTestbed(name string, tv cue.Value) (*TestbedItem, []Error) {
	it, errs := decodeItem[TestbedSpec](name, tv)
	if errs != nil {
		return nil, errs
	}
	m := it.Value.Matrix
	if m == nil {
		return &TestbedItem{Item: *it}, nil
	}
	mv := tv.LookupPath(cue.ParsePath("matrix"))
	for _, axis := range m.Order {
		if _, ok := m.Axes[axis]; !ok {
			errs = append(errs, Error{Path: fmt.Sprintf("testbeds.%s.matrix.order", name), Msg: fmt.Sprintf("axis %q has no values in axes", axis)})
		}
	}
	if len(errs) > 0 {
		return nil, withPos(errs, mv)
	}
	m.Values = map[string][]string{}
	m.AxisPos = map[string]map[string]string{}
	for _, axis := range m.Order {
		m.AxisPos[axis] = map[string]string{}
		values, _ := mv.LookupPath(cue.MakePath(cue.Str("axes"), cue.Str(axis))).Fields()
		for values.Next() {
			value := values.Selector().Unquoted()
			m.Values[axis] = append(m.Values[axis], value)
			m.AxisPos[axis][value] = posString(values.Value())
		}
	}
	return &TestbedItem{Item: *it}, nil
}

func product(axes [][]string) [][]string {
	out := [][]string{{}}
	for _, values := range axes {
		var next [][]string
		for _, prefix := range out {
			for _, v := range values {
				next = append(next, append(slices.Clone(prefix), v))
			}
		}
		out = next
	}
	return out
}

func (c *Catalog) merge(o *Catalog) {
	mergeMap(c.Testbeds, o.Testbeds, func(i *TestbedItem) string { return i.Origin })
	for _, kind := range PartKinds {
		mergeMap(c.parts[kind], o.parts[kind], func(i *Item[Part]) string { return i.Origin })
	}
	mergeMap(c.Tests, o.Tests, func(i *Item[Test]) string { return i.Origin })
	mergeMap(c.Suites, o.Suites, func(i *Item[Suite]) string { return i.Origin })
	mergeMap(c.Plans, o.Plans, func(i *Item[Plan]) string { return i.Origin })
	mergeMap(c.Steps, o.Steps, func(i *Item[Step]) string { return i.Origin })
}

// mergeMap lets a later directory replace an entry, except that a built-in
// never replaces something a user wrote.
func mergeMap[V any](dst, src map[string]V, origin func(V) string) {
	for k, v := range src {
		if old, ok := dst[k]; ok && origin(v) == BuiltIn && origin(old) != BuiltIn {
			continue
		}
		dst[k] = v
	}
}

// keepBuiltins reports each entry a user file writes under a built-in's
// name, and puts the built-in back, so a built-in's name always means what
// wendy ships.
func (c *Catalog) keepBuiltins(base *Catalog) []Error {
	var errs []Error
	for _, kind := range PartKinds {
		errs = append(errs, keepBuiltin(c.parts[kind], base.parts[kind], "parts."+kind)...)
	}
	errs = append(errs, keepBuiltin(c.Tests, base.Tests, "tests")...)
	errs = append(errs, keepBuiltin(c.Suites, base.Suites, "suites")...)
	errs = append(errs, keepBuiltin(c.Plans, base.Plans, "plans")...)
	for name, tb := range c.Testbeds {
		if b, ok := base.Testbeds[name]; ok && tb.Origin != BuiltIn {
			errs = append(errs, builtinError(tb.Pos, "testbeds."+name, name))
			c.Testbeds[name] = b
		}
	}
	return errs
}

func keepBuiltin[T any](m, base map[string]*Item[T], path string) []Error {
	var errs []Error
	for name, it := range m {
		if b, ok := base[name]; ok && it.Origin != BuiltIn {
			errs = append(errs, builtinError(it.Pos, path+"."+name, name))
			m[name] = b
		}
	}
	return errs
}

func builtinError(pos, path, name string) Error {
	return posError(pos, path, fmt.Sprintf("%q is built in; copy it under another name to change it", name))
}

// crossCheck reports references to names nothing defines, and steps that do
// not say what to run.
func (c *Catalog) crossCheck() {
	for _, name := range Names(c.Steps) {
		it := c.Steps[name]
		path := "steps." + name
		switch {
		case c.IsBuiltinStep(name):
			c.pushError(posError(it.Pos, path, fmt.Sprintf("%q is a built-in step; name yours something else", name)))
		case c.Steps[it.Value.Uses] != nil:
			c.pushError(posError(it.Pos, path+".uses", "a step of yours can't use another of yours; use a built-in step or say what it runs"))
		default:
			c.checkStep(it.Pos, path, it.Value)
		}
	}
	for _, name := range Names(c.Tests) {
		t := c.Tests[name]
		for i, st := range t.Value.Steps {
			c.checkStep(t.Pos, fmt.Sprintf("tests.%s.steps.%d", name, i), st)
		}
	}
	suites := Names(c.Suites)
	for _, name := range Names(c.Suites) {
		s := c.Suites[name]
		for i, tn := range s.Value.Tests {
			if _, ok := c.Tests[tn]; !ok {
				c.addRefError(s.Pos, fmt.Sprintf("suites.%s.tests.%d", name, i), "test", tn, Names(c.Tests))
			}
		}
	}
	for _, name := range Names(c.Plans) {
		p := c.Plans[name]
		path := "plans." + name
		tb, ok := c.Testbeds[p.Value.Testbed]
		switch {
		case !ok:
			c.addRefError(p.Pos, path+".testbed", "testbed", p.Value.Testbed, Names(c.Testbeds))
		case len(p.Value.Combinations) > 0 && !tb.HasMatrix():
			c.pushError(posError(p.Pos, path+".combinations",
				fmt.Sprintf("testbed %q has no matrix, so plan %s has no combinations to pick", p.Value.Testbed, name)))
		}
		for i, s := range p.Value.Suites {
			if _, ok := c.Suites[s]; !ok {
				c.addRefError(p.Pos, fmt.Sprintf("%s.suites.%d", path, i), "suite", s, suites)
			}
		}
	}
}

func (c *Catalog) addRefError(pos, path, kind, name string, candidates []string) {
	msg := fmt.Sprintf("unknown %s %q", kind, name)
	if s := Suggest(name, candidates); s != "" {
		msg += fmt.Sprintf(" (did you mean %q?)", s)
	}
	c.pushError(posError(pos, path, msg))
}

// checkStep reports a step that does not say what it runs exactly once, or
// uses a step nothing defines.
func (c *Catalog) checkStep(pos, path string, st Step) {
	n := 0
	for _, set := range []bool{st.Uses != "", st.Exec != nil, st.Container != nil} {
		if set {
			n++
		}
	}
	if n != 1 {
		c.pushError(posError(pos, path, "a step needs exactly one of uses, exec, container, chainsaw or ginkgo"))
		return
	}
	if st.Uses != "" && !c.IsBuiltinStep(st.Uses) && c.Steps[st.Uses] == nil {
		known := Names(c.Steps)
		for _, b := range c.BuiltinSteps {
			known = append(known, b.ID)
		}
		c.addRefError(pos, path+".uses", "step", st.Uses, known)
	}
}

// IsBuiltinStep reports whether id names a step compiled into wendy.
func (c *Catalog) IsBuiltinStep(id string) bool {
	return slices.ContainsFunc(c.BuiltinSteps, func(b Builtin) bool { return b.ID == id })
}

// Expand returns st with the step of the user's it uses in its place. What st
// sets itself, other than the schema's defaults, overrides the step's own,
// and it keeps st's name, or else the step's. Any other step is returned as
// it is.
func (c *Catalog) Expand(st Step) Step {
	mine, ok := c.Steps[st.Uses]
	if !ok || c.IsBuiltinStep(st.Uses) {
		return st
	}
	out := mine.Value
	out.Name = cmp.Or(st.Name, out.Name, st.Uses)
	if st.Timeout != "" && st.Timeout != c.stepTimeout {
		out.Timeout = st.Timeout
	}
	if st.Retries != 0 {
		out.Retries = st.Retries
	}
	if st.When != "" {
		out.When = st.When
	}
	if st.ContinueOnFailure {
		out.ContinueOnFailure = true
	}
	if len(st.Labels) > 0 {
		out.Labels = st.Labels
	}
	return out
}

// pushError records an error against the file it is in, if that file is
// known, and otherwise with the catalog's.
func (c *Catalog) pushError(e Error) {
	for i := range c.Files {
		if c.Files[i].Path == e.File {
			c.Files[i].Errors = append(c.Files[i].Errors, e)
			return
		}
	}
	c.Errors = append(c.Errors, e)
}

func posError(pos, path, msg string) Error {
	e := Error{Path: path, Msg: msg}
	if parts := strings.Split(pos, ":"); len(parts) == 3 {
		e.File = parts[0]
		// A position without numbers still names its file.
		e.Line, _ = strconv.Atoi(parts[1])
		e.Col, _ = strconv.Atoi(parts[2])
	}
	return e
}

// AllErrors is every error, file-attributed or not.
func (c *Catalog) AllErrors() []Error {
	out := slices.Clone(c.Errors)
	for _, f := range c.Files {
		out = append(out, f.Errors...)
	}
	return out
}

// Suggest returns the candidate closest to name, if one is close enough to be
// a likely typo.
func Suggest(name string, candidates []string) string {
	best, bestD := "", len(name)/2+2
	for _, c := range candidates {
		if d := levenshtein(name, c); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

func origin(v cue.Value) string {
	p := userPos(v, 3)
	if !p.IsValid() {
		return BuiltIn
	}
	return displayPath(p.Filename())
}

func posString(v cue.Value) string {
	p := userPos(v, 3)
	if !p.IsValid() {
		return ""
	}
	return fmt.Sprintf("%s:%d:%d", displayPath(p.Filename()), p.Line(), p.Column())
}

func isSchemaFile(f string) bool {
	return f == "" || strings.Contains(f, "/cue.mod/gen/") || strings.HasPrefix(f, schemaRoot)
}

// userPos is the first position of v in a user's file. A value unified with
// the schema reports the schema's position first, so its conjuncts are
// searched too.
func userPos(v cue.Value, depth int) token.Pos {
	if p := v.Pos(); p.IsValid() && !isSchemaFile(p.Filename()) {
		return p
	}
	if depth == 0 {
		return token.NoPos
	}
	if op, args := v.Expr(); op == cue.AndOp {
		for _, a := range args {
			if p := userPos(a, depth-1); p.IsValid() {
				return p
			}
		}
	}
	return token.NoPos
}

func source(v cue.Value) string {
	b, err := format.Node(v.Syntax(cue.Docs(true), cue.Final()))
	if err != nil {
		return ""
	}
	return string(b)
}

func toErrors(err error) []Error {
	var out []Error
	for _, e := range cueerrors.Errors(err) {
		ce := Error{Path: strings.Join(e.Path(), "."), Msg: errMsg(e)}
		for _, p := range cueerrors.Positions(e) {
			f := p.Filename()
			if f == "" || strings.Contains(f, "/cue.mod/gen/") || strings.HasPrefix(f, schemaRoot) {
				continue
			}
			ce.File, ce.Line, ce.Col = displayPath(f), p.Line(), p.Column()
			break
		}
		out = append(out, ce)
	}
	if len(out) == 0 && err != nil {
		out = append(out, Error{Msg: err.Error()})
	}
	return out
}

var durationField = regexp.MustCompile(`(^|\.)timeout$`)

func errMsg(e cueerrors.Error) string {
	f, args := e.Msg()
	msg := fmt.Sprintf(f, args...)
	if durationField.MatchString(strings.Join(e.Path(), ".")) && strings.Contains(msg, "out of bound") {
		return "not a duration (e.g. \"15m\")"
	}
	return msg
}

func displayPath(p string) string {
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, p); err == nil && !strings.HasPrefix(rel, "..") {
			return "./" + rel
		}
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+"/") {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func configFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		switch filepath.Ext(e.Name()) {
		case ".cue", ".yaml", ".yml", ".json":
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}

// tagsFor keeps only the tags a directory declares, so one -t flag can serve
// several directories without the others rejecting it.
func tagsFor(files, tags []string) []string {
	var text strings.Builder
	for _, f := range files {
		b, _ := os.ReadFile(f)
		text.Write(b)
	}
	var out []string
	for _, t := range tags {
		key, _, _ := strings.Cut(t, "=")
		if strings.Contains(text.String(), "@tag("+key) {
			out = append(out, t)
		}
	}
	return out
}

// schemaFiles is the embedded package plus #Builtins generated from the
// steps compiled into this binary.
func schemaFiles(builtins []Builtin) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := fs.WalkDir(wendycue.FS, "wendy", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := wendycue.FS.ReadFile(p)
		out[filepath.Base(p)] = b
		return err
	})
	out["builtins_gen.cue"] = []byte(BuiltinsCUE(builtins))
	return out, err
}

// BuiltinsCUE renders #Builtins for the given steps.
func BuiltinsCUE(builtins []Builtin) string {
	var b strings.Builder
	b.WriteString("package wendy\n\n#Builtins: {\n")
	for _, s := range builtins {
		schema := s.Schema
		if schema == "" {
			schema = "{}"
		}
		fmt.Fprintf(&b, "\t%q: %s\n", s.ID, schema)
	}
	b.WriteString("}\n")
	return b.String()
}

func overlay(dir string, builtins []Builtin) (map[string]load.Source, error) {
	files, err := schemaFiles(builtins)
	if err != nil {
		return nil, err
	}
	ov := map[string]load.Source{}
	for name, b := range files {
		ov[filepath.Join(dir, "cue.mod", "gen", ImportPath, name)] = load.FromBytes(b)
	}
	if _, err := os.Stat(filepath.Join(dir, "cue.mod", "module.cue")); err != nil {
		ov[filepath.Join(dir, "cue.mod", "module.cue")] = load.FromString("module: \"wendy.local/config\"\nlanguage: version: \"v0.17.0\"\n")
	}
	return ov, nil
}

func loadSchema(ctx *cue.Context, builtins []Builtin) (cue.Value, error) {
	files, err := schemaFiles(builtins)
	if err != nil {
		return cue.Value{}, err
	}
	ov := map[string]load.Source{
		schemaRoot + "/cue.mod/module.cue": load.FromString("module: \"github.com/tinkerbell/wendy\"\nlanguage: version: \"v0.17.0\"\n"),
	}
	for name, b := range files {
		ov[schemaRoot+"/cue/wendy/"+name] = load.FromBytes(b)
	}
	insts := load.Instances([]string{"./cue/wendy"}, &load.Config{Dir: schemaRoot, Overlay: ov})
	if insts[0].Err != nil {
		return cue.Value{}, insts[0].Err
	}
	v := ctx.BuildInstance(insts[0])
	return v, v.Err()
}

func withPos(errs []Error, v cue.Value) []Error {
	pos := posError(posString(v), "", "")
	for i := range errs {
		if errs[i].File == "" {
			errs[i].File, errs[i].Line, errs[i].Col = pos.File, pos.Line, pos.Col
		}
	}
	return errs
}
