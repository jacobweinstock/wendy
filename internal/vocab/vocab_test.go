package vocab_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/vocab"
)

var allStatuses = []event.Status{
	event.Passed, event.Failed, event.Errored, event.Skipped, event.Cancelled,
	event.TimedOut, event.Running, event.Waiting, event.Paused,
}

func TestEveryStatusHasAWordGlyphAndClass(t *testing.T) {
	classes := []string{"pass", "fail", "err", "run", "pause", "skip"}
	words := map[string]event.Status{}
	for _, s := range allStatuses {
		st := vocab.StatusOf(s)
		if st.Word == "" || strings.TrimSpace(st.Glyph) == "" {
			t.Errorf("%s: word %q, glyph %q", s, st.Word, st.Glyph)
		}
		if !slices.Contains(classes, st.Class) {
			t.Errorf("%s: class %q is not one of %v", s, st.Class, classes)
		}
		if other, ok := words[st.Word]; ok {
			t.Errorf("%s and %s both read %q", s, other, st.Word)
		}
		words[st.Word] = s
	}
}

func TestStatusWordsFollowTheGlossary(t *testing.T) {
	for s, want := range map[event.Status]string{
		event.Skipped:  "skipped",
		event.Paused:   "paused",
		event.TimedOut: "timed out",
	} {
		if got := vocab.StatusOf(s).Word; got != want {
			t.Errorf("%s reads %q, want %q", s, got, want)
		}
	}
}

func TestUnknownStatusReadsAsItself(t *testing.T) {
	st := vocab.StatusOf("flaky")
	if st.Word != "flaky" || st.Class != "skip" {
		t.Errorf("got %+v", st)
	}
}

func TestPhaseNamesTheEnginePhases(t *testing.T) {
	for id, want := range map[string]string{
		engine.PhaseBuild:       "Build images",
		engine.PhaseSetup:       "Set up testbed",
		engine.PhaseDiagnostics: "Collect diagnostics",
		engine.PhaseTeardown:    "Tear down",
		engine.PhaseAdhoc:       engine.PhaseAdhoc,
		"smoke":                 "smoke",
	} {
		if got := vocab.Phase(id); got != want {
			t.Errorf("Phase(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestCount(t *testing.T) {
	for n, want := range map[int]string{0: "0 errors", 1: "1 error", 2: "2 errors"} {
		if got := vocab.Count(n, "error", "errors"); got != want {
			t.Errorf("Count(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestChoice(t *testing.T) {
	if got := vocab.Choice("topology", "colocated"); got != "On the management cluster" {
		t.Errorf("topology colocated reads %q", got)
	}
	if got := vocab.Choice("keep", "failed"); got != "If it fails" {
		t.Errorf("keep failed reads %q", got)
	}
	if got := vocab.Choice("topology", "sideways"); got != "sideways" {
		t.Errorf("an unknown value reads %q, want it unchanged", got)
	}
	if !vocab.HasChoice("cni", "kube-router") || vocab.HasChoice("cni", "flannel") {
		t.Error("HasChoice disagrees with the labels")
	}
}

func TestChoicesHaveNoRetiredWords(t *testing.T) {
	for _, path := range []string{"topology", "bootMode", "network.family", "arch", "cni", "bmc.protocol", "diagnostics", "keep", "step.kind"} {
		for _, v := range vocab.ChoiceValues(path) {
			label := vocab.Choice(path, v)
			if label == "" {
				t.Errorf("%s %s has an empty label", path, v)
			}
			for _, text := range []string{label, vocab.ChoiceHelp(path, v)} {
				if f := vocab.Check(text); len(f) > 0 {
					t.Errorf("%s %s reads %q, which uses %q", path, v, text, f[0].Term)
				}
			}
		}
	}
	for _, path := range []string{"topology", "bootMode", "network.family"} {
		if !vocab.Described(path) {
			t.Errorf("a testbed setting, %s, has a choice with no description", path)
		}
	}
	if vocab.Described("cni") || vocab.Described("bogus") {
		t.Error("Described is true for a field whose choices have no descriptions")
	}
}

func TestKinds(t *testing.T) {
	slug := regexp.MustCompile(`^[a-z]+(-[a-z]+)*$`)
	seen := map[string]string{}
	unique := func(what, v string) {
		if prev, ok := seen[what+v]; ok {
			t.Errorf("%s %q is used twice (also by %s)", what, v, prev)
		}
		seen[what+v] = v
	}
	var parts []string
	for _, k := range vocab.Kinds {
		if k.ID == "" || k.Label == "" || k.One == "" || k.Many == "" || k.Blurb == "" {
			t.Errorf("%+v has an empty field", k)
		}
		if !slug.MatchString(k.Slug) {
			t.Errorf("%s: slug %q is not kebab-case", k.ID, k.Slug)
		}
		unique("id", k.ID)
		unique("slug", k.Slug)
		unique("label", k.Label)
		for _, text := range []string{k.Label, k.One, k.Many, k.Blurb} {
			if f := vocab.Check(text); len(f) > 0 {
				t.Errorf("%s: %q uses retired %q", k.ID, text, f[0].Term)
			}
		}
		if k.Part {
			parts = append(parts, k.ID)
		}
	}
	if want := []string{"tinkerbell", "clusterAPI", "managementCluster", "workloadCluster", "hardware"}; !slices.Equal(parts, want) {
		t.Errorf("parts are %v, want %v", parts, want)
	}
}

func TestCheckFlagsRetiredTerms(t *testing.T) {
	for text, want := range map[string]string{
		"Environments":          "Environments",
		"Keep environment":      "environment",
		"Run this environment…": "environment",
		"Profiles":              "Profiles",
		"Scenario":              "Scenario",
		"Catalog":               "Catalog",
		"A Providers part":      "Providers",
		"＋ New providers part":  "providers part",
		"Workload":              "Workload",
		"Dump evidence":         "evidence",
		"8 combos":              "combos",
		"Target":                "Target",
		"Ginkgo label filter":   "Ginkgo label filter",
		"Start a run":           "a run",
		"Cancel this run?":      "this run",
		"New run":               "New run",
		"Re-run":                "Re-run",
		"Re-run failed (2)":     "Re-run",
		"Cancel run":            "Cancel run",
		"Runs":                  "Runs",
		"run ID":                "run ID",
		"(not run)":             "not run",
		"never run":             "never run",
		"keep: onFailure":       "onFailure",
		"Topology: colocated":   "colocated",
		"isoboot":               "isoboot",
		"Delete r-0411 and its logs? Its environment stays": "environment",
	} {
		found := vocab.Check(text)
		if len(found) == 0 {
			t.Errorf("Check(%q) found nothing, want %q", text, want)
			continue
		}
		if found[0].Term != want {
			t.Errorf("Check(%q) found %q, want %q", text, found[0].Term, want)
		}
		if found[0].Use == "" || !strings.Contains(found[0].Context, found[0].Term) {
			t.Errorf("Check(%q): finding %+v lacks a replacement or context", text, found[0])
		}
	}
}

func TestCheckAllowsGlossaryUses(t *testing.T) {
	for _, text := range []string{
		"Environment variables",
		"KEY=value environment variables, one per line",
		"Workload cluster",
		"The workload clusters are deleted",
		"Run tests…",
		"Run again",
		"Run failed again (2)",
		"Run nightly",
		"A suite runs tests in order",
		"Runs a program on this host",
		"Running",
		"● 1 running",
		"Executions",
		"Cluster API",
		"Collect diagnostics",
		"4 combinations",
		"Not run yet",
		"On the management cluster",
		"Only tests with labels",
	} {
		if found := vocab.Check(text); len(found) > 0 {
			t.Errorf("Check(%q) flagged %q", text, found[0].Term)
		}
	}
}

func TestCheckReportsEveryUseInOrder(t *testing.T) {
	found := vocab.Check("Profiles list environments; each run keeps its environment.")
	var terms []string
	for _, f := range found {
		terms = append(terms, f.Term)
	}
	if want := []string{"Profiles", "environments", "each run", "environment"}; !slices.Equal(terms, want) {
		t.Errorf("found %v, want %v", terms, want)
	}
}
