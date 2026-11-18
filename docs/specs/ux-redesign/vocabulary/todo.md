# Tasks: vocabulary — V1

From [plan.md](plan.md) and [spec.md](spec.md). Criteria refer to the
spec's Success Criteria, in order.

- [x] **V1. `internal/vocab` package**
  - Done: `vocab.go` (glossary package doc, `StatusOf`, `Phase`, `Count`), `choices.go` (`Choice`, `HasChoice`,
    `ChoiceValues`), `kinds.go` (`Kinds`, with `Label` for tab headings and `Part` marking the five parts) and
    `retired.go` (`Retired`, `Check`). The phase test is tied to the engine's phase constants. `Check` matches "run"
    only in noun phrases, and is tested both for terms it must flag and for glossary text it must allow. Tests, vet
    and lint pass, and `vocab` depends only on `event`.
  - Acceptance: the package doc holds the glossary. `StatusOf` covers every `event.Status` constant with the
    glossary words (skipped, not "not run"; paused, not "paused on failure"). `Phase` names `build`, `setup`,
    `evidence` and `teardown`, and returns any other id unchanged. `Count(1, "error", "errors")` reads "1 error".
    `Choice` labels every value of `topology`, `bootMode`, `network.family`, `arch`, `cni`, `bmc.protocol`,
    `evidence.dump`, `keep` and `step.kind`, and returns an unknown value unchanged. `Kinds` holds Plan, Testbed, the
    five parts, Suite, Test and Built-in step with unique ids, kebab-case slugs and non-empty labels and blurbs.
    `Check` flags each retired term with its replacement and context, and skips the listed exceptions. The package
    imports only the standard library and `internal/event`. Criterion 1.
  - Verify: `go test -race ./internal/vocab/`; `go list -deps ./internal/vocab/ | grep tinkerbell/wendy` lists only
    `vocab` and `event`
  - Files: `internal/vocab/vocab.go`, `internal/vocab/choices.go`, `internal/vocab/kinds.go`,
    `internal/vocab/retired.go`, `internal/vocab/vocab_test.go`

- [x] **V2. Web UI on `vocab`**
  - Done: the template helpers `glyph`, `word`, `cls`, `colName`, `pathLabel`, `comboCounts` and the host-check
    glyphs read from `vocab`. `Node.Label` names the phases for both UIs, while `Node.Name` keeps the engine id
    that other code compares. `ui.Word` and `ui.Class` are now wrappers until V4. The web `plural` is gone. Edit
    selects, the "default: …" option, the step-kind select and the run form's keep and dump choices show labels.
    `StepKindHelp` now adds to the label instead of repeating it. Tests: `TestSelectsShowLabelsAndSubmitValues`
    and `TestNodeLabelNamesPhases`. Templates regenerated with no further changes; lint clean.
  - Note: `make verify-generate` runs `git diff`, so it needs approval before it runs.
  - Depends on: V1
  - Acceptance: the web's `glyph`, `word` and `cls` come from `vocab.StatusOf`. The execution tree (via `ui/view.go`)
    and grid show Build images, Set up testbed (n steps), Collect diagnostics and Tear down. A skipped step reads
    "skipped". Every select in the edit forms and the run form shows a label, including the empty "default: …"
    option and the step-kind select, and submits the value. The web `plural` is gone, replaced by `vocab.Count`. The
    API still validates values. Tests that pinned old words are updated, and a test saves a labelled choice and
    finds the value in `wendy-ui.cue`. Criteria 3 and 4 for the web.
  - Verify: `make generate && go test -race ./internal/ui/ ./internal/ui/web/`
  - Files: `internal/ui/view.go`, `internal/ui/web/{pages.go,helpers.go,views.templ,edit.templ}`, their generated
    `*_templ.go`, and `internal/ui/web/{pages_test.go,edit_test.go}`

- [x] **V3. TUI on `vocab`**
  - Done: `statusStyle` and `glyph` read from `vocab`, and a `word` helper replaces `ui.Word`. The status glyphs in
    run, screen and kept-environment lines come from `vocab`. Grid columns are keyed by engine id, headed by
    `vocab.Phase` and sized to their headings. The TUI `plural` is gone, replaced by `vocab.Count`. Field selects
    and their "default: …" entry, the step-kind select and its page titles, and the run form's keep select (new
    `choiceOptions` helper) show labels and store values. Test: `TestChoicesShowLabelsAndStoreValues`. Tests and
    lint pass, and a one-off render confirmed the grid's alignment.
  - Depends on: V1 (and V2's `view.go` change for the tree)
  - Acceptance: TUI glyphs, status words and colours come from `vocab.StatusOf`. Grid columns use `vocab.Phase`. The
    TUI `plural` is gone, replaced by `vocab.Count`. Edit-form selects, the step-kind select and the run form's keep
    select show labels and store values, and an unknown current value is still offered. Tests pinned to old words are
    updated, and a form test checks that a labelled choice stores its value. Criteria 3 and 4 for the TUI.
  - Verify: `go test -race ./internal/ui/tui/`
  - Files: `internal/ui/tui/{style.go,run.go,screens.go,edit.go,form.go,tui.go}`, `internal/ui/tui/edit_test.go`

- [x] **V4. CLI, removal of old helpers, choice coverage**
  - Done: `plain.go` gains `glyph` and `checkGlyph`. `--plain`, `step run`, `env status`, `plan` and `vet` take every
    glyph from `vocab`, so no glyph literal is left in the CLI. The final `--plain` line prints the status word
    ("timed out", not "timedout"). `event.Status.Glyph`, `ui.Word` and `ui.Class` are deleted. Test:
    `TestEveryChoiceHasALabel` covers every select field's options and default, and the step kinds. 12 status
    glyphs hard-coded in `views.templ` and one in `tui.go` were found during the final check; they now read from
    `vocab`. The TUI's follow on/off marker (● / ○ in `run.go`) is a toggle, not a status, so it stays.
    `make vet lint test` pass. `make verify-generate` exits 0, and its templ step reports 0 updates.
  - Depends on: V2, V3
  - Acceptance: `plain.go` and `root.go` take glyphs from `vocab`. `event.Status.Glyph`, `ui.Word` and `ui.Class`
    are deleted, and no package outside `vocab` defines a status word, glyph or colour class. A test in
    `internal/ui` fails if any `Field` option, step kind or run-form choice lacks a `vocab.Choice` label (the
    catalog-filled `uses` is exempt). `--plain` output differs only in status words. `spec.md` is
    marked `Status: implemented`. Criteria 2, 5 and 6.
  - Verify: `make vet lint test verify-generate`; `grep -rn 'func (s Status) Glyph\|func Word(\|func Class(' internal/`
    finds nothing
  - Files: `internal/cli/{plain.go,root.go}`, `internal/event/event.go`, `internal/ui/view.go`,
    `internal/ui/fields_test.go`, `spec.md`

### Checkpoint
`make vet lint test verify-generate` pass; the web and TUI show the new status and phase words and labelled
choices; `spec.md` is marked `Status: implemented`.
