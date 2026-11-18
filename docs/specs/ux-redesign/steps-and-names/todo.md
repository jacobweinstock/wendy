# Tasks: steps-and-names — V1

From [plan.md](plan.md) and [spec.md](spec.md). Criteria numbers refer to the spec's Success Criteria.

## Phase A: names

- [x] **T1. Rename the built-ins**
  - Done:
    - `fallbacks` reads each part field's default from `#Testbed` into `Catalog.Fallback`.
    - The picker offers the fallback as the empty choice ("picking none"), labelled "<name>: used when a testbed
      picks none". A testbed that names its fallback explicitly reads the same, and saving drops it.
    - The quickstart golden holds resolved values, not part names, so it didn't change here. T8 changed its
      cluster name.
  - Acceptance:
    - `builtin.cue` and the `#Testbed` defaults use the names in the spec's table.
    - `Catalog.Fallback` is read from the schema, and no Go code spells a fallback name.
    - The pickers list the fallback first, as "<name>: used when a testbed picks none". Picking it still writes
      nothing.
    - The starter's testbed is `mine`. The run form and the TUI form prefer `mine` where they preferred `default`.
  - Verify: `make generate && go test ./internal/...`. Regenerate the quickstart golden, and its diff shows only
    names.
  - Files: `cue/wendy/builtin.cue`, `cue/wendy/schema.cue`, `cue/starter.cue.tmpl`, `internal/config/resolve.go`,
    `internal/ui/web/edit.go`, and tests and goldens.
  - Criteria: 5.

- [x] **T2. Reject redefining a built-in**
  - Done:
    - `keepBuiltins` runs on each directory's result. It reports the entry, and puts the built-in back.
    - A redefinition whose fields conflict still fails with CUE's own error first.
    - The resolve tests defined `k8s-1.36` themselves; those lines are gone.
  - Acceptance: a user file that defines a part, testbed, test, suite or plan under a built-in's name gets a
    `file:line` error saying to copy it under another name.
  - Verify: a test in `load_test.go` for one part and one test.
  - Files: `internal/config/load.go`, `internal/config/load_test.go`.
  - Criteria: 6.

## Phase B: your own steps

- [x] **T3. Schema, config and preview**
  - Done:
    - `uses` is a string, and the guard is `if uses != _|_ if #Builtins[uses] != _|_`. So `with` on your step
      is CUE's own "not allowed" error at its line.
    - Tests and your steps share `checkStep`. `Expand` reads `#Step`'s default timeout from the schema.
  - Acceptance:
    - `#Config.steps` exists, and `#Step.uses` is a string, with `with` guarded by `#Builtins`.
    - `Catalog.BuiltinSteps` and `Catalog.Steps` exist, merged across directories.
    - `crossCheck` reports each error listed under "Your own steps" in the spec.
    - `cat.Expand` applies the overrides from plan decision 6, and `preview` and `cli/upgrades.go` use it.
  - Verify: config tests for each error, and a preview test where two tests use `wait`, one overriding the
    timeout.
  - Files: `cue/wendy/schema.cue`, `internal/config/{types,load}.go`, `internal/preview/preview.go`,
    `internal/cli/upgrades.go`, tests.
  - Criteria: 1, 2.

- [x] **T4. Service**
  - Done: `Spec.Taken` counts built-in step IDs as taken, so a step of yours can't take one. A built-in step's
    origin reads "built-in" in the error.
  - Acceptance: `KindStep` can be saved, renamed and deleted. `stepRefs`, `testRefs` and `UsedBy` work as in plan
    decision 8, and deleting or renaming a used step is refused with its users.
  - Verify: tests in `service/edit_test.go`.
  - Files: `internal/service/edit.go`, `internal/config/edit.go` (`SpecOf`), `internal/vocab/kinds.go` (Steps:
    "step", "steps"), tests.
  - Criteria: 3.

- [x] **T5. Web: the Steps subtab and detail**
  - Done:
    - `stepView.Builtin` is now `Uses`, since it can name a step of yours.
    - `builtinUsers` replaces `stepUsers` and lists tests and steps.
    - The detail heading is "What it runs", since a "Runs" heading on its own matches a retired pattern.
    - I left out a "Make a step from it…" button on built-in steps, because the spec doesn't include it.
  - Acceptance:
    - The Steps subtab lists built-in steps and yours, with where each comes from.
    - Your step's detail shows what it runs, its timeout, its Used by, Edit step (or Copy and edit…), and View CUE.
    - A built-in step's Used by also lists the steps of yours that use it.
  - Verify: tests in `config_test.go`, and the glossary guard renders a step of yours.
  - Files: `internal/ui/web/{pages.go,config.templ}`, `config_test.go`, `guard_test.go`.
  - Criteria: 4.

- [x] **T6. Web: the step form and Named step**
  - Done:
    - Test steps and a step entry share `postedStep`.
    - The entry's form is one step block (`stepData.Single`) with no number, move or remove.
    - `usesFor` gives a test's steps built-in steps and yours, and your step's form built-in steps only.
    - The tests live in `TestYourStepsInTheWebUI`.
  - Acceptance:
    - New step, Edit step, Copy and edit… and Delete step work, with the step block from the test form.
    - In the test form, the kind reads "Named step", and its picker lists built-in steps and yours.
  - Verify: tests in `edit_test.go` for save, copy, delete-refused and the picker.
  - Files: `internal/ui/web/{edit.go,edit.templ}`, `internal/ui/fields.go`, `internal/vocab/choices.go`,
    `edit_test.go`.
  - Criteria: 4.

- [x] **T7. CLI and TUI**
  - Done: `list steps` keeps the built-in rows as they were and appends yours (name, what it runs, origin); the
    `cli` module redesigns the output. The TUI needed only the label in its test.
  - Acceptance: `wendy list steps` lists built-in steps and yours, with origin. The TUI builds, and its step list
    reads `BuiltinSteps`.
  - Verify: the CLI test for `list steps`, and `go test ./internal/ui/tui/`.
  - Files: `internal/cli/root.go`, `internal/ui/tui/*.go`, tests.
  - Criteria: 1.

## Phase C: words

- [x] **T8. No "playground", and docs**
  - Done:
    - The default cluster name is `wendy`, not `workload`: the glossary guard rejects "workload" on its own,
      and the name shows in the UIs. The spec notes this.
    - `design.md` calls the source "the earlier scripts" outside §14.
    - `.wendy/wendy.cue` is refreshed, as you approved.
  - Acceptance:
    - The descriptions, the default cluster name `workload`, the comments and the starter config are reworded as
      in the spec.
    - `vocab.Retired` has `playground`.
    - In `design.md`, §14 is "Origins: the Tinkerbell playground", with the link, and the other mentions are
      reworded. `tui.md` is reworded.
    - The README describes your own steps, including the override edge case, and the new built-in names.
  - Verify: `make vet lint test`, the `make generate` hash check, and
    `grep -rin playground cue internal README.md docs/design.md`.
  - Files: `cue/wendy/{builtin,schema}.cue`, `cue/starter.cue.tmpl`, `internal/ui/fields.go`,
    `internal/steps/hardware.go`, `internal/provider/hardware/libvirt.go`, `internal/vocab/retired.go`,
    `README.md`, `docs/design.md`, `docs/mockups/tui.md`.
  - Criteria: 7, 8.

### Checkpoint

`make vet lint test` pass, the generated templates are up to date, and `spec.md` is marked
`Status: implemented`.
