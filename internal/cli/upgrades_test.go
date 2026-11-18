package cli

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/tinkerbell/wendy/internal/config"
)

func TestUpgradeVersionsComeFromStepParameters(t *testing.T) {
	cat := &config.Catalog{Tests: map[string]*config.Item[config.Test]{
		"a": {Value: config.Test{Steps: []config.Step{
			{Uses: "capt.upgrade.kubernetes", With: map[string]any{"version": "v1.37.0"}},
			{Uses: "capt.upgrade.kubernetes"}, // no target: nothing to fetch
			{Uses: "assert.healthy", With: map[string]any{"version": "ignored"}},
		}}},
		"b": {Value: config.Test{Steps: []config.Step{
			{Uses: "capt.upgrade.kubernetes", With: map[string]any{"version": "v1.37.0"}},
			{Uses: "capt.upgrade.kubernetes", With: map[string]any{"version": "v1.38.1"}},
		}}},
	}}
	if got, want := upgradeVersions(cat), []string{"v1.37.0", "v1.38.1"}; !slices.Equal(got, want) {
		t.Errorf("upgradeVersions = %v, want %v", got, want)
	}
}

// A step of the user's that upgrades Kubernetes counts like the built-in it
// uses.
func TestUpgradeVersionsSeeThroughYourSteps(t *testing.T) {
	cat := &config.Catalog{
		Steps: map[string]*config.Item[config.Step]{
			"to-1.38": {Value: config.Step{Uses: "capt.upgrade.kubernetes", With: map[string]any{"version": "v1.38.1"}}},
		},
		Tests: map[string]*config.Item[config.Test]{
			"a": {Value: config.Test{Steps: []config.Step{{Uses: "to-1.38"}}}},
		},
	}
	if got, want := upgradeVersions(cat), []string{"v1.38.1"}; !slices.Equal(got, want) {
		t.Errorf("upgradeVersions = %v, want %v", got, want)
	}
}

// list steps shows the built-in steps and then the user's own.
func TestListStepsShowsYours(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wendy.cue"), []byte("package a\n\nsteps: wait: exec: command: [\"kubectl\", \"wait\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	cmd := listCmd(&globals{configDirs: []string{dir}, data: t.TempDir()})
	cmd.SetArgs([]string{"steps"})
	runErr := cmd.Execute()
	os.Stdout = stdout
	_ = w.Close()
	out, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatal(runErr)
	}
	if !strings.Contains(string(out), "capt.scale") || !regexp.MustCompile(`(?m)^wait +kubectl wait +.*wendy\.cue$`).Match(out) {
		t.Errorf("list steps =\n%s", out)
	}
}
