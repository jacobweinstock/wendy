package tui

import (
	"strings"
	"testing"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/service"
	_ "github.com/tinkerbell/wendy/internal/steps" // registers the built-in steps
	"github.com/tinkerbell/wendy/internal/ui"
)

func editModel(t *testing.T) *model {
	t.Helper()
	dir := t.TempDir()
	svc := service.New(&config.Loader{Dirs: []string{dir}, Builtins: engine.Builtins()}, &engine.Engine{Data: t.TempDir()})
	svc.StarterDir = dir
	m := &model{svc: svc, w: 108, h: 32}
	m.refresh()
	return m
}

func setField(t *testing.T, e *editor, path, val string) {
	t.Helper()
	for i, f := range e.fields {
		if f.Path == path {
			e.vals[i] = val
			return
		}
	}
	t.Fatalf("no field %s", path)
}

func TestEditorSavesASuite(t *testing.T) {
	m := editModel(t)
	m.openEditor(config.KindSuite, "", "new")
	e := m.edit
	if !strings.Contains(e.view(m), "New suite") {
		t.Fatalf("view:\n%s", e.view(m))
	}
	e.name = "mine"
	setField(t, e, "tests", "deploy\nsmoke")
	e.submit(m)
	if m.edit != nil {
		t.Fatalf("editor still open: %s", e.err)
	}
	su := m.svc.Catalog().Suites["mine"]
	if su == nil || len(su.Value.Tests) != 2 || su.Value.Tests[1] != "smoke" {
		t.Fatalf("saved suite = %+v", su)
	}

	m.openEditor(config.KindSuite, "mine", "edit")
	if got := ui.Get(m.edit.value, m.edit.fields[0]); got != "" {
		t.Errorf("description = %q; an unset field should stay unset", got)
	}
	m.openEditor(config.KindSuite, "lifecycle", "edit")
	if m.toast == "" {
		t.Error("a built-in suite was opened for editing in place")
	}
}

func TestEditorKeepsTheFormOnAnError(t *testing.T) {
	m := editModel(t)
	m.openEditor(config.KindSuite, "", "new")
	e := m.edit
	e.name = "bad"
	setField(t, e, "tests", "nope")
	e.submit(m)
	if m.edit == nil || e.err == "" {
		t.Fatal("a rejected suite closed the editor or reported nothing")
	}
	if e.vals[indexOf(t, e, "tests")] != "nope" {
		t.Error("the form lost what the user typed")
	}
}

func TestEditorBuildsATestFromSteps(t *testing.T) {
	m := editModel(t)
	m.openEditor(config.KindTest, "", "new")
	e := m.edit
	e.name = "acme"
	e.submit(m)
	if e.stage != stageSteps {
		t.Fatalf("stage = %v, want the step list", e.stage)
	}
	if !strings.Contains(e.view(m), "no steps yet") {
		t.Errorf("view:\n%s", e.view(m))
	}

	e.openStep(m, -1)
	if !strings.Contains(e.view(m), "Step") {
		t.Errorf("step view:\n%s", e.view(m))
	}
	e.step, e.stage = nil, stageSteps
	e.steps = []map[string]any{{"container": map[string]any{"image": "ghcr.io/acme/e2e:v1"}}}
	if !strings.Contains(e.view(m), "ghcr.io/acme/e2e:v1") {
		t.Errorf("steps view:\n%s", e.view(m))
	}
	e.save(m)
	if m.edit != nil {
		t.Fatalf("test not saved: %s", e.err)
	}
	if tc := m.svc.Catalog().Tests["acme"]; tc == nil || len(tc.Value.Steps) != 1 {
		t.Fatalf("test = %+v", tc)
	}
}

func TestChoicesShowLabelsAndStoreValues(t *testing.T) {
	var got []string
	for _, o := range choiceOptions("keep") {
		got = append(got, o.Key+"="+o.Value)
	}
	if want := "Never=never If it fails=failed Always=always"; strings.Join(got, " ") != want {
		t.Errorf("keep options = %q, want %q", strings.Join(got, " "), want)
	}

	m := editModel(t)
	m.openEditor(config.KindTest, "", "new")
	e := m.edit
	e.name = "acme"
	e.submit(m)
	e.openStep(m, -1)
	if v := e.view(m); !strings.Contains(v, "Named step: A step by name") {
		t.Errorf("the step form does not label its kind:\n%s", v)
	}
}

func indexOf(t *testing.T, e *editor, path string) int {
	t.Helper()
	for i, f := range e.fields {
		if f.Path == path {
			return i
		}
	}
	t.Fatalf("no field %s", path)
	return -1
}
