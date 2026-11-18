package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"sigs.k8s.io/yaml"
)

// UIFile is the one config file the UIs write. Hand-written files are never
// touched.
const UIFile = "wendy-ui.cue"

// The kinds of entries other than parts: the #Config field each one lives
// under, which is its vocab ID.
const (
	KindPlan    = "plans"
	KindTestbed = "testbeds"
	KindTest    = "tests"
	KindSuite   = "suites"
	KindStep    = "steps"
)

// Kinds lists the kinds the service edits: testbeds, their five kinds of
// part, tests, suites, the user's own steps, and plans.
var Kinds = slices.Concat([]string{KindTestbed}, PartKinds, []string{KindTest, KindSuite, KindStep, KindPlan})

// Spec is what the UI file holds: for each kind, entries by name, each one the
// CUE value as plain data. It is sparse: only what a user set is in it, so the
// schema's defaults keep applying.
type Spec map[string]map[string]map[string]any

// Entries returns the entries of a kind.
func (s Spec) Entries(kind string) map[string]map[string]any { return s[kind] }

// Has reports whether the spec defines the entry.
func (s Spec) Has(kind, name string) bool {
	_, ok := s[kind][name]
	return ok
}

// Set adds or replaces an entry.
func (s Spec) Set(kind, name string, v map[string]any) {
	if s[kind] == nil {
		s[kind] = map[string]map[string]any{}
	}
	s[kind][name] = v
}

// Delete removes an entry.
func (s Spec) Delete(kind, name string) {
	delete(s[kind], name)
	if len(s[kind]) == 0 {
		delete(s, kind)
	}
}

// ReadSpec reads the UI file. A file that does not exist is an empty spec.
func ReadSpec(path string) (Spec, error) {
	src, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Spec{}, nil
	}
	if err != nil {
		return nil, err
	}
	f, err := parser.ParseFile(path, src)
	if err != nil {
		return nil, err
	}
	body := configBody(f)
	if body == nil {
		return nil, fmt.Errorf("%s was not written by wendy's UI; edit it by hand or delete it", filepath.Base(path))
	}
	v := cuecontext.New().BuildExpr(body)
	spec := Spec{}
	if err := v.Decode(&spec); err != nil {
		return nil, err
	}
	for kind, byName := range spec["parts"] {
		for name, e := range byName {
			m, _ := e.(map[string]any)
			spec.Set(kind, name, m)
		}
	}
	delete(spec, "parts")
	return spec, nil
}

// configBody is the struct of `wendy.#Config & {…}`, the shape of the UI file.
func configBody(f *ast.File) *ast.StructLit {
	for _, d := range f.Decls {
		e, ok := d.(*ast.EmbedDecl)
		if !ok {
			continue
		}
		if b, ok := e.Expr.(*ast.BinaryExpr); ok && b.Op == token.AND {
			if s, ok := b.Y.(*ast.StructLit); ok {
				return s
			}
		}
	}
	return nil
}

// UIHeader tells whoever opens the UI file what it is.
const UIHeader = "// Written by wendy's UI. Edit entries in the UI, not here: the UI rewrites this file.\n" +
	"// Put hand-written config in its own file.\n"

// CUE renders the spec as the UI file, in the given CUE package.
func (s Spec) CUE(pkg string) ([]byte, error) {
	out := map[string]any{}
	parts := map[string]any{}
	for kind, entries := range s {
		if IsPart(kind) {
			parts[kind] = entries
		} else {
			out[kind] = entries
		}
	}
	if len(parts) > 0 {
		out["parts"] = parts
	}
	ctx := cuecontext.New()
	v := ctx.Encode(out)
	if v.Err() != nil {
		return nil, v.Err()
	}
	node := v.Syntax(cue.Final())
	body, err := format.Node(node)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "%spackage %s\n\nimport \"%s\"\n\nwendy.#Config & %s\n", UIHeader, pkg, ImportPath, body)
	return format.Source(b.Bytes())
}

var packageClause = regexp.MustCompile(`(?m)^package\s+([A-Za-z_][A-Za-z0-9_]*)`)

// PackageName is the CUE package of the other files in dir, which the UI file
// must share. It defaults to the starter config's.
func PackageName(dir string) string {
	for _, f := range configFiles(dir) {
		if filepath.Base(f) == UIFile || filepath.Ext(f) != ".cue" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if m := packageClause.FindSubmatch(b); m != nil {
			return string(m[1])
		}
	}
	return "local"
}

// unknownReference matches the reference errors only the whole catalog can
// judge: a suite's tests, the parts a testbed or its matrix axes name, a
// plan's testbed and suites, and the steps a test uses. Entry, axis and value
// names may hold dots, as in "1.36".
var unknownReference = regexp.MustCompile(`^(suites\.[^.]+\.tests\.\d+|testbeds\..+\.(tinkerbell|clusterAPI|managementCluster|workloadCluster|hardware)|plans\..+\.(testbed|suites\.\d+|combinations)|tests\..+\.steps\.\d+\.uses)$`)

// Check evaluates the spec on its own and returns what the schema rejects.
// References to entries that may live in other files are not checked here.
func Check(s Spec, builtins []Builtin) ([]Error, error) {
	src, err := s.CUE("local")
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "wendy-check-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, UIFile), src, 0o600); err != nil {
		return nil, err
	}
	cat := (&Loader{Dirs: []string{dir}, Builtins: builtins}).Load()
	var out []Error
	for _, e := range cat.AllErrors() {
		if !unknownReference.MatchString(e.Path) {
			out = append(out, e)
		}
	}
	return out, nil
}

// WriteSpec writes the UI file atomically, so a watcher never reloads half of
// it.
func WriteSpec(dir string, s Spec) error {
	src, err := s.CUE(PackageName(dir))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".wendy-ui-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(src); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, UIFile))
}

// SpecOf is how an existing catalog entry reads as an editable value, so a
// hand-written or built-in entry can be copied into the UI file.
func SpecOf(kind string, cat *Catalog, name string) (map[string]any, error) {
	var v any
	switch kind {
	case KindTestbed:
		// As written: which part it uses of each kind, or the inline part,
		// and its matrix.
		if it := cat.Testbeds[name]; it != nil {
			v = it.Value
		}
	case KindTinkerbell, KindClusterAPI, KindManagementCluster, KindWorkloadCluster, KindHardware:
		if it := cat.Parts(kind)[name]; it != nil {
			v = it.Value
		}
	case KindTest:
		if it := cat.Tests[name]; it != nil {
			v = it.Value
		}
	case KindSuite:
		if it := cat.Suites[name]; it != nil {
			v = it.Value
		}
	case KindStep:
		if it := cat.Steps[name]; it != nil {
			v = it.Value
		}
	case KindPlan:
		if it := cat.Plans[name]; it != nil {
			v = it.Value
		}
	}
	if v == nil {
		return nil, fmt.Errorf("no %s %q", strings.TrimSuffix(kind, "s"), name)
	}
	return toMap(v)
}

// toMap turns a decoded catalog value into plain data. Integers stay integers,
// which the schema requires, and nulls are dropped, since the schema's lists
// and structs do not accept them.
func toMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	m, _ := prune(out).(map[string]any)
	return m, nil
}

// FromYAML parses YAML or JSON into plain data, with integers as integers.
func FromYAML(s string) (any, error) {
	b, err := yaml.YAMLToJSON([]byte(s))
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return prune(out), nil
}

func prune(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			if e == nil {
				delete(x, k)
				continue
			}
			x[k] = prune(e)
		}
	case []any:
		for i := range x {
			x[i] = prune(x[i])
		}
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		f, _ := x.Float64()
		return f
	}
	return v
}

// Taken returns the names of kind that other files define, which an entry in
// the UI file cannot reuse without unifying with them.
func (s Spec) Taken(kind string, cat *Catalog) []string {
	var names []string
	switch kind {
	case KindTestbed:
		names = Names(cat.Testbeds)
	case KindTinkerbell, KindClusterAPI, KindManagementCluster, KindWorkloadCluster, KindHardware:
		names = Names(cat.Parts(kind))
	case KindTest:
		names = Names(cat.Tests)
	case KindSuite:
		names = Names(cat.Suites)
	case KindPlan:
		names = Names(cat.Plans)
	case KindStep:
		// A built-in step's ID is taken too, since uses would be ambiguous.
		names = Names(cat.Steps)
		for _, b := range cat.BuiltinSteps {
			names = append(names, b.ID)
		}
	}
	return slices.DeleteFunc(names, func(n string) bool { return s.Has(kind, n) })
}
