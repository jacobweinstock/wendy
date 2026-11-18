# Plan: vocabulary — V1

From [spec.md](spec.md) (approved). Tasks: [todo.md](todo.md).

## Approach

Build `internal/vocab` first, complete and tested, while the old helpers still exist. Then move one surface at a
time over to it: web, then TUI, then CLI. Delete each old helper once nothing calls it. Each surface task leaves the
build green and shows the new words on that surface, so the work can stop safely after any task.

## Components

1. **`internal/vocab`** (new, leaf: stdlib and `internal/event` only).
   - `vocab.go`: package doc with the glossary; `Status{Word, Glyph, Class}` and `StatusOf(event.Status)`; `Phase(id)`
     for the engine's phase ids (`build` → "Build images", `setup` → "Set up testbed", `evidence` → "Collect
     diagnostics", `teardown` → "Tear down"; any other id is returned as is); `Count(n, one, many)`.
   - `choices.go`: `Choice(path, value)` from a table keyed by field path, holding today's values (`topology`,
     `bootMode`, `network.family`, `arch`, `cni`, `bmc.protocol`, `evidence.dump`, `keep`, `step.kind`). An unknown
     pair returns the value, so a missing label shows up in the coverage test rather than as a blank.
   - `kinds.go`: `Kind{ID, Slug, One, Many, Blurb}` and `Kinds`, in glossary order. Data only.
   - `retired.go`: `Retired` (pattern, use instead, exceptions) and `Check(text) []Finding{Term, Use, Context}`.
     Patterns are case-sensitive, word-bounded regexps. Exceptions are applied by masking the allowed phrases
     ("environment variables", "Workload cluster", "Run tests", "Run again", …) before matching.
2. **Shared tree (`internal/ui/view.go`).** Phase node names and the "Setup (n steps)" label come from
   `vocab.Phase`. `ui.Word` and `ui.Class` become thin wrappers during the move, and are deleted in the last task.
3. **Web.** The template helpers `glyph`, `word` and `cls` call `vocab.StatusOf`; `colName` calls `vocab.Phase`;
   `plural(n, what)` is replaced by `vocab.Count` at each call site. `(not run)` in the tree becomes `(skipped)`.
   `selectOptions` and the step-kind select show `vocab.Choice` labels. The run form's `keeps` and `evidenceDumps`
   become value lists with `vocab.Choice` labels. The API keeps validating values, not labels.
4. **TUI.** `glyph` and `statusStyle` use `vocab.StatusOf`; grid columns use `vocab.Phase`; `plural` becomes
   `vocab.Count`; the edit form's selects and the run form's keep select are built with `huh.NewOption(label, value)`.
5. **CLI and cleanup.** `plain.go` and `root.go` take glyphs from `vocab`. Then `event.Status.Glyph`, `ui.Word` and
   `ui.Class` are deleted. A choice-coverage test in `internal/ui` walks every `Field` list, the step kinds and the
   run forms' choices.

## Order and parallelism

V1 (package) → V2 (web) → V3 (TUI) → V4 (CLI, removal, coverage). V2 and V3 only depend on V1 and touch different
packages, so they could run in parallel. They stay sequential because V2 also changes the shared `view.go` that V3's
tree relies on. V4 needs both, because it deletes what they stopped using.

## Risks

- **Tests pinned to old words.** Web, TUI and plain-output tests assert "not run", "Setup", "Build" and "on
  failure". Each task updates the tests of its own surface and runs that package's tests with `-race`.
- **Raw values outside the options.** The empty option reads "default: onFailure", the raw default value, so it
  takes the label too ("default: On failure"). The schema only accepts the offered values, so a select never has to
  show a value it doesn't offer. The TUI keeps its existing handling of an unknown current value.
- **Phase ids not in the table** (`adhoc`). `vocab.Phase` returns unknown ids unchanged, so the single-step view
  looks as it does today. Naming it would need a new glossary word, which is ask-first.
- **Retired-term false positives.** "run" is everywhere as a verb. Patterns match only the noun phrases listed in
  the spec, and `Check` has table tests for both directions before any surface uses it.
- **Generated templates.** V2 edits `.templ` files; `make generate` uses the pinned templ already in
  `out/tools`, and `make verify-generate` is part of the V2 and V4 checks.

## Checkpoints

- After V1: `go test -race ./internal/vocab/`
- After V2: `make generate && go test -race ./internal/ui/ ./internal/ui/web/`
- After V3: `go test -race ./internal/ui/tui/`
- After V4: `make vet lint test verify-generate`; spec marked `Status: implemented`.
