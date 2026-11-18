// Package vocab is the one place for the words wendy shows: how statuses
// read, the names of the execution phases, the labels of the choices a form
// offers, the names of the kinds of config entry, and the terms wendy no
// longer uses. The web UI, the TUI and the CLI take their words from here so
// that they say the same thing.
//
// Wendy's glossary:
//
//   - Plan: what to test, where and how: the suites, the testbed, and options
//     such as whether to keep the testbed if something fails. You run a plan.
//   - Execution: one time wendy carries out a plan. It has a number, a status
//     and a verdict.
//   - run: only ever a verb, with an object: run a plan, run tests.
//   - Preview: what a plan would do, shown before it runs.
//   - Testbed: what tests run on: one part of each kind, the settings no part
//     owns (where Tinkerbell runs, how machines boot, the network), and an
//     optional matrix.
//   - Part: a reusable piece of a testbed: Tinkerbell, Cluster API, Management
//     cluster, Workload cluster or Hardware.
//   - Matrix, axis, combination: a testbed's axes, and each concrete testbed
//     they make. A testbed without a matrix has one combination.
//   - Kept testbed: a built testbed an execution left up, addressed as
//     <execution>/<combination>.
//   - Suite, test, step: tests run in order on one testbed; a named, ordered
//     list of steps; one action in a test. A built-in step comes with wendy.
//   - Label: a tag on a test or a combination. "Only tests with labels" picks
//     tests by label.
//   - Diagnostics: what wendy collects to debug a failure.
//   - Artifacts: any file an execution saved.
//
// Retired lists the words these replace.
package vocab

import (
	"fmt"

	"github.com/tinkerbell/wendy/internal/event"
)

// Status is how a status reads everywhere wendy shows one.
type Status struct {
	// Word is the status in words, e.g. "timed out".
	Word string
	// Glyph is its symbol, so that no status is told apart by colour alone.
	Glyph string
	// Class is its colour family: pass, fail, err, run, pause or skip.
	Class string
}

var statuses = map[event.Status]Status{
	event.Passed:    {"passed", "✓", "pass"},
	event.Failed:    {"failed", "✗", "fail"},
	event.Errored:   {"errored", "!", "err"},
	event.TimedOut:  {"timed out", "✗", "fail"},
	event.Cancelled: {"cancelled", "⊘", "skip"},
	event.Skipped:   {"skipped", "–", "skip"},
	event.Running:   {"running", "●", "run"},
	event.Waiting:   {"waiting", "○", "skip"},
	event.Paused:    {"paused", "‖", "pause"},
}

// StatusOf is how s reads. A status wendy does not know reads as itself, with
// a blank glyph.
func StatusOf(s event.Status) Status {
	if st, ok := statuses[s]; ok {
		return st
	}
	return Status{Word: string(s), Glyph: " ", Class: "skip"}
}

var phases = map[string]string{
	"build":       "Build images",
	"setup":       "Set up testbed",
	"diagnostics": "Collect diagnostics",
	"teardown":    "Tear down",
}

// Phase is the name of the execution phase the engine calls id, e.g. "Set up
// testbed" for setup. An id it does not know is returned unchanged.
func Phase(id string) string {
	if p, ok := phases[id]; ok {
		return p
	}
	return id
}

// Count is n followed by one or many, whichever n calls for: "1 error",
// "2 errors".
func Count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
