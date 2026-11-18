package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/ui"
	"github.com/tinkerbell/wendy/internal/vocab"
)

// EditError is a change the config would not accept. The UIs show Errs next
// to the form.
type EditError struct{ Errs []config.Error }

func (e *EditError) Error() string {
	parts := make([]string, 0, len(e.Errs))
	for _, c := range e.Errs {
		if c.Path != "" {
			parts = append(parts, c.Path+": "+c.Msg)
		} else {
			parts = append(parts, c.Msg)
		}
	}
	return strings.Join(parts, "; ")
}

func editErr(format string, args ...any) error {
	return &EditError{Errs: []config.Error{{Msg: fmt.Sprintf(format, args...)}}}
}

var entryName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

// UIFilePath is the file the UIs write their entries to.
func (s *Service) UIFilePath() string { return filepath.Join(s.StarterDir, config.UIFile) }

// Spec is what the UI file holds now.
func (s *Service) Spec() (config.Spec, error) { return config.ReadSpec(s.UIFilePath()) }

// Entry is a testbed, part, test or suite as an editor sees it.
type Entry struct {
	Kind, Name string
	Value      map[string]any
	// Editable is whether the UI file owns it. Anything else can be copied
	// into the UI file under a new name, but not changed.
	Editable bool
	// Origin is the file that defines it.
	Origin string
}

// Entry returns an entry for editing: the UI's own, or a copy of what a
// config file or the built-ins define.
func (s *Service) Entry(kind, name string) (*Entry, error) {
	spec, err := s.Spec()
	if err != nil {
		return nil, err
	}
	if v, ok := spec[kind][name]; ok {
		return &Entry{Kind: kind, Name: name, Value: v, Editable: true, Origin: displayUIFile(s)}, nil
	}
	cat := s.Catalog()
	v, err := config.SpecOf(kind, cat, name)
	if err != nil {
		return nil, err
	}
	return &Entry{Kind: kind, Name: name, Value: v, Origin: originOf(cat, kind, name)}, nil
}

func displayUIFile(s *Service) string { return filepath.Base(s.UIFilePath()) }

func originOf(c *config.Catalog, kind, name string) string {
	switch kind {
	case config.KindTestbed:
		if it := c.Testbeds[name]; it != nil {
			return it.Origin
		}
	case config.KindTinkerbell, config.KindClusterAPI, config.KindManagementCluster, config.KindWorkloadCluster, config.KindHardware:
		if it := c.Parts(kind)[name]; it != nil {
			return it.Origin
		}
	case config.KindTest:
		if it := c.Tests[name]; it != nil {
			return it.Origin
		}
	case config.KindSuite:
		if it := c.Suites[name]; it != nil {
			return it.Origin
		}
	case config.KindPlan:
		if it := c.Plans[name]; it != nil {
			return it.Origin
		}
	case config.KindStep:
		if it := c.Steps[name]; it != nil {
			return it.Origin
		}
		if c.IsBuiltinStep(name) {
			return config.BuiltIn
		}
	}
	return ""
}

// Owned lists the names of a kind that the UI file defines.
func (s *Service) Owned(kind string) []string {
	spec, err := s.Spec()
	if err != nil {
		return nil
	}
	return config.Names(spec[kind])
}

// SaveEntry creates or replaces an entry in the UI file. oldName is the name
// the entry had when editing began, or "" for a new one; a different name
// renames it. Nothing is written unless the whole config accepts the result.
func (s *Service) SaveEntry(kind, oldName, name string, v map[string]any) error {
	s.editMu.Lock()
	defer s.editMu.Unlock()
	if !slices.Contains(config.Kinds, kind) {
		return editErr("unknown kind %q", kind)
	}
	if !entryName.MatchString(name) {
		return editErr("a name is letters, digits, '.', '_' and '-', starting with a letter or digit")
	}
	spec, err := s.Spec()
	if err != nil {
		return err
	}
	cat := s.Catalog()
	switch {
	case oldName == "" && spec.Has(kind, name):
		return editErr("%s %q already exists", singular(kind), name)
	case oldName != "" && !spec.Has(kind, oldName):
		return editErr("%s %q is not defined in %s, so it cannot be changed here", singular(kind), oldName, config.UIFile)
	case slices.Contains(spec.Taken(kind, cat), name):
		return editErr("%s %q is already defined in %s", singular(kind), name, originOf(cat, kind, name))
	case oldName != "" && oldName != name && spec.Has(kind, name):
		return editErr("%s %q already exists", singular(kind), name)
	}
	if oldName != "" && oldName != name {
		if by := s.UsedBy(kind, oldName); len(by) > 0 {
			return editErr("%s %q is used by %s; change those first", singular(kind), oldName, strings.Join(by, ", "))
		}
		spec.Delete(kind, oldName)
	}
	if err := prepare(kind, v); err != nil {
		return err
	}
	spec.Set(kind, name, v)
	if err := s.refs(spec, kind, v); err != nil {
		return err
	}
	return s.commit(spec)
}

// DeleteEntry removes an entry from the UI file, unless something uses it.
func (s *Service) DeleteEntry(kind, name string) error {
	s.editMu.Lock()
	defer s.editMu.Unlock()
	spec, err := s.Spec()
	if err != nil {
		return err
	}
	if !spec.Has(kind, name) {
		return editErr("%s %q is not defined in %s, so it cannot be deleted here", singular(kind), name, config.UIFile)
	}
	if by := s.UsedBy(kind, name); len(by) > 0 {
		return editErr("%s %q is used by %s; change those first", singular(kind), name, strings.Join(by, ", "))
	}
	spec.Delete(kind, name)
	return s.commit(spec)
}

// prepare completes an entry and rejects it when its form left something
// out.
func prepare(kind string, v map[string]any) error {
	var missing []string
	switch kind {
	case config.KindTest:
		for i, st := range ui.Steps(v) {
			for _, m := range ui.Valid(st) {
				missing = append(missing, fmt.Sprintf("step %d: %s", i+1, m))
			}
		}
	case config.KindStep:
		missing = ui.Valid(v)
	case config.KindPlan:
		if tb, _ := v["testbed"].(string); tb == "" {
			missing = append(missing, "a testbed")
		}
		if l, _ := v["suites"].([]any); len(l) == 0 {
			missing = append(missing, "at least one suite")
		}
	case config.KindTinkerbell, config.KindClusterAPI, config.KindManagementCluster, config.KindWorkloadCluster, config.KindHardware:
		missing = ui.Missing(v, ui.PartFields(kind))
	}
	if len(missing) > 0 {
		return editErr("required: %s", strings.Join(missing, "; "))
	}
	return nil
}

// refs checks, once spec is written, that what an entry names exists: the
// tests of a suite, the parts of a testbed, and the steps a test or a step
// uses. Names may be defined in other files or earlier in the same spec.
func (s *Service) refs(spec config.Spec, kind string, v map[string]any) error {
	switch kind {
	case config.KindSuite:
		return s.suiteRefs(spec, v)
	case config.KindTestbed:
		return s.testbedRefs(spec, v)
	case config.KindTest:
		return s.testRefs(spec, v)
	case config.KindStep:
		return s.stepRefs(v)
	case config.KindPlan:
		return s.planRefs(spec, v)
	}
	return nil
}

// planRefs rejects a testbed or suite nothing defines, and combinations on a
// testbed that has no matrix to pick them from.
func (s *Service) planRefs(spec config.Spec, v map[string]any) error {
	cat := s.Catalog()
	var errs []config.Error
	tb, _ := v["testbed"].(string)
	testbeds := append(config.Names(cat.Testbeds), config.Names(spec[config.KindTestbed])...)
	if !slices.Contains(testbeds, tb) {
		errs = append(errs, refError("testbed", "testbed", tb, testbeds))
	} else if l, _ := v["combinations"].([]any); len(l) > 0 && !s.hasMatrix(spec, tb) {
		errs = append(errs, config.Error{Path: "combinations", Msg: fmt.Sprintf("testbed %q has no matrix, so there are no combinations to pick", tb)})
	}
	suites := append(config.Names(cat.Suites), config.Names(spec[config.KindSuite])...)
	l, _ := v["suites"].([]any)
	for _, e := range l {
		if name := fmt.Sprint(e); !slices.Contains(suites, name) {
			errs = append(errs, refError("suites", "suite", name, suites))
		}
	}
	if len(errs) > 0 {
		return &EditError{Errs: errs}
	}
	return nil
}

// hasMatrix reports whether a testbed, in the UI file or the config, has a
// matrix.
func (s *Service) hasMatrix(spec config.Spec, testbed string) bool {
	if tb, ok := spec[config.KindTestbed][testbed]; ok {
		return tb["matrix"] != nil
	}
	tb := s.Catalog().Testbeds[testbed]
	return tb != nil && tb.HasMatrix()
}

// refError is an entry naming something of a kind that nothing defines, with
// the closest name that does.
func refError(path, kind, name string, known []string) config.Error {
	msg := fmt.Sprintf("unknown %s %q", kind, name)
	if sg := config.Suggest(name, known); sg != "" {
		msg += fmt.Sprintf(" (did you mean %q?)", sg)
	}
	return config.Error{Path: path, Msg: msg}
}

// testRefs rejects a step that uses a step nothing defines.
func (s *Service) testRefs(spec config.Spec, v map[string]any) error {
	cat := s.Catalog()
	known := append(config.Names(cat.Steps), config.Names(spec[config.KindStep])...)
	for _, b := range cat.BuiltinSteps {
		known = append(known, b.ID)
	}
	var errs []config.Error
	for i, st := range ui.Steps(v) {
		name, _ := st["uses"].(string)
		if name == "" || slices.Contains(known, name) {
			continue
		}
		msg := fmt.Sprintf("step %d: unknown step %q", i+1, name)
		if sg := config.Suggest(name, known); sg != "" {
			msg += fmt.Sprintf(" (did you mean %q?)", sg)
		}
		errs = append(errs, config.Error{Path: fmt.Sprintf("steps.%d.uses", i), Msg: msg})
	}
	if len(errs) > 0 {
		return &EditError{Errs: errs}
	}
	return nil
}

// stepRefs rejects a step of the user's that uses anything but a built-in
// step, so steps never chain.
func (s *Service) stepRefs(v map[string]any) error {
	name, _ := v["uses"].(string)
	if name == "" || s.Catalog().IsBuiltinStep(name) {
		return nil
	}
	msg := fmt.Sprintf("%q is not a built-in step; a step of yours can use only those", name)
	return &EditError{Errs: []config.Error{{Path: "uses", Msg: msg}}}
}

// testbedRefs rejects a part name nothing defines.
func (s *Service) testbedRefs(spec config.Spec, v map[string]any) error {
	var errs []config.Error
	for _, kind := range config.PartKinds {
		name, isName := v[kind].(string)
		if !isName {
			continue
		}
		known := append(config.Names(s.Catalog().Parts(kind)), config.Names(spec[kind])...)
		if slices.Contains(known, name) {
			continue
		}
		msg := fmt.Sprintf("unknown %s %q", singular(kind), name)
		if sg := config.Suggest(name, known); sg != "" {
			msg += fmt.Sprintf(" (did you mean %q?)", sg)
		}
		errs = append(errs, config.Error{Path: kind, Msg: msg})
	}
	if len(errs) > 0 {
		return &EditError{Errs: errs}
	}
	return nil
}

func (s *Service) suiteRefs(spec config.Spec, v map[string]any) error {
	known := append(config.Names(s.Catalog().Tests), config.Names(spec[config.KindTest])...)
	l, _ := v["tests"].([]any)
	var errs []config.Error
	for _, e := range l {
		name := fmt.Sprint(e)
		if slices.Contains(known, name) {
			continue
		}
		msg := fmt.Sprintf("unknown test %q", name)
		if sg := config.Suggest(name, known); sg != "" {
			msg += fmt.Sprintf(" (did you mean %q?)", sg)
		}
		errs = append(errs, config.Error{Path: "tests", Msg: msg})
	}
	if len(errs) > 0 {
		return &EditError{Errs: errs}
	}
	return nil
}

// commit checks spec against the schema, writes it and reloads the catalog.
func (s *Service) commit(spec config.Spec) error {
	if errs, err := config.Check(spec, s.Loader.Builtins); err != nil {
		return err
	} else if len(errs) > 0 {
		return &EditError{Errs: errs}
	}
	if len(spec) == 0 {
		if err := os.Remove(s.UIFilePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else if err := config.WriteSpec(s.StarterDir, spec); err != nil {
		return err
	}
	s.Reload()
	return nil
}

// UsedBy names what refers to an entry, as "kind name": tests by suites,
// suites by plans, testbeds by plans, parts by the testbeds that name them,
// themselves or in an axis of their matrix, and the user's steps by the tests
// that use them.
func (s *Service) UsedBy(kind, name string) []string {
	c := s.Catalog()
	var out []string
	switch kind {
	case config.KindStep:
		for _, n := range config.Names(c.Tests) {
			if slices.ContainsFunc(c.Tests[n].Value.Steps, func(st config.Step) bool { return st.Uses == name }) {
				out = append(out, "test "+n)
			}
		}
	case config.KindTest:
		for _, n := range config.Names(c.Suites) {
			if slices.Contains(c.Suites[n].Value.Tests, name) {
				out = append(out, "suite "+n)
			}
		}
	case config.KindSuite:
		for _, n := range config.Names(c.Plans) {
			if slices.Contains(c.Plans[n].Value.Suites, name) {
				out = append(out, "plan "+n)
			}
		}
	case config.KindTestbed:
		for _, n := range config.Names(c.Plans) {
			if c.Plans[n].Value.Testbed == name {
				out = append(out, "plan "+n)
			}
		}
	case config.KindTinkerbell, config.KindClusterAPI, config.KindManagementCluster, config.KindWorkloadCluster, config.KindHardware:
		for _, n := range config.Names(c.Testbeds) {
			if c.Testbeds[n].Value.Picks(kind, name) {
				out = append(out, "testbed "+n)
			}
		}
	}
	return out
}

// singular is how a message names one entry of a kind, e.g. "Cluster API
// part".
func singular(kind string) string {
	if k, ok := vocab.KindOf(kind); ok {
		return k.One
	}
	return strings.TrimSuffix(kind, "s")
}
