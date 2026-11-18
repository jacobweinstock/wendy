# Plan: model-rename — V1

From [spec.md](spec.md) (approved). Tasks: [todo.md](todo.md).

## Approach

Rename one concept at a time, end to end: CUE, Go types and identifiers, JSON, the surfaces the model forces, docs
for that key, and tests. After each task the build, vet and full test suite pass, and the starter config and
`.wendy/*.cue` load, so the work can stop after any task. The concepts go in dependency order: free the word "plan"
first, then rename what other changes build on (tests, parts), then fold the matrix into the testbed before the plan
names a testbed, and leave executions, the most widely read data, for last.

The Go renames use `~/go/bin/gopls rename` (installed, not on PATH). gopls renames identifiers in Go, including the
generated `*_templ.go`, but not in `.templ` sources, JSON tags, string literals or comments. So each rename is
followed by: a grep of `.templ`, tags, strings and comments for the old word; fixing those by hand; `make generate`;
then a build.

## Components and decisions

1. **`preview` package.** `internal/plan` → `internal/preview`: `plan.Plan` → `preview.Preview`, `plan.Selection` →
   `preview.Selection`, `plan.json` → `preview.json`. The `wendy plan` command keeps its name until `cli`.
2. **Kind ids from `vocab`.** Each task points its `config.Kind*` constant at the `vocab.Kinds` id it introduces
   (`tests`, the five part ids, `testbeds`, `plans`). Web routes and `wendy list` use `Kind.Slug`, the field `vocab`
   defines for URLs and CLI arguments (`/catalog/cluster-api/…`). `service.singular` and the web `singular`
   become lookups of `Kind.One`.
3. **Parts under `parts:`.** `#Config.parts` holds the five kinds. `config.Spec` stays keyed by kind id, and
   `ReadSpec` and `Spec.CUE` nest and un-nest the part kinds under `parts:`, so the editor code is unchanged.
   `assemble` and `ownerOf` map the five parts to the resolved shape's `tinkerbell`, `clusterAPI`,
   `managementCluster`, `workloadCluster`, `hardware` and `arch`.
4. **Testbed with matrix.** The catalog holds `Testbeds map[string]*TestbedItem`. Each item is the spec as written
   (`TestbedSpec`) and its `Combinations`. A testbed without a matrix has one combination named after the testbed,
   whose `Testbed` field is the resolved `config.Testbed`. `MatrixItem`, `Environments` and `EnvSpecs` go away. A
   combination's testbed is the spec with each axis value merged in, then picked, assembled and evaluated like any
   testbed, so one code path resolves both.
   - **Decision for review: patch semantics.** An axis value *overrides* the testbed's own value for the keys it
     sets. That is what "patch" means, and a testbed that sets `bootMode: "isoboot"` can still have a `boot` axis.
     Two axes that set the same key differently are an error, as two axes picking one kind of part are today.
     (Today a patch is unified with the parts, so it can only fill fields a part left unset.)
5. **Plan.** `#Plan` names one `testbed`. `effective` and `preview.New` take the testbed's combinations, filtered by
   `combinations` globs. `combinations` on a testbed without a matrix is an error naming the plan.
6. **Diagnostics.** The plan's `diagnostics` and `consoles`; the resolved `diagnostics: {collect, consoles}`;
   `Selection.Diagnostics`; the engine phase and directory `diagnostics`; `Failure.Diagnostics`. The built-in
   `evidence` scenario becomes the engine's own list of diagnostics steps, run as the phase and not shown as a test.
   `vocab.Phase` and the choices are re-keyed (`keep: failed`, `diagnostics`).
7. **Executions.** IDs are numbers allocated in `<data>/executions/`, with directory names as numbers. Lists sort
   numerically, not by name. Event fields `execution` and `combination`, event types `execution.*` and
   `combination.*`, summary `combinations`, and `state.json` `execution`, `combination` and `workloadCluster`.
   Kubeconfig file names (`management`, `tinkerbell`, `workload`) stay, matching the cluster names the CLI keeps.
8. **Forced surface changes**, kept minimal:
   - Web and TUI: catalog tabs and blurbs from `vocab.Kinds` (the Matrices tab goes away). The testbed detail shows
     its matrix, and the editor has five part pickers and keeps `matrix` on save. Run forms pick a testbed.
   - CLI: `--plan`, `-t/--testbed` and `--combination`, and `wendy list` slugs.

## Order and parallelism

R1 preview → R2 tests → R3 parts → R4 testbed + matrix → R5 plans → R6 diagnostics → R7 executions → R8 docs and
sweep. All sequential, because each task renames identifiers the next one edits. R7 could start after R4, but it
waits so that R5 and R6 don't edit files R7 renames.

## Risks

- **Renames gopls cannot see** (`.templ`, JSON tags, strings, comments, CUE): mitigated by the grep pass and
  `make generate` after every rename. The glossary check of R8 catches what is left.
- **Many files per task.** The renames are mechanical, but several tasks touch more than five files. Each task's
  diff is the rename plus the behavior listed in its acceptance; anything else waits.
- **Axis patch semantics** (decision above): tests pin both the override and the two-axis conflict.
- **`state.json` is read by user-written steps** (Chainsaw `--values`, commands). Its keys change in R7, and
  README and `design.md` §10 describe the new names in R8.
- **Execution IDs as plain numbers** sort wrongly as names (`10` before `9`); listing and the next-ID scan parse
  them as numbers, and a test creates IDs past 9.
- **Executions in the old `runs/` directory** are no longer listed (no backward compatibility). The final summary
  says so.
- **Blurb test.** `pages_test.go` requires catalog blurbs to start with "A"/"An", and some `vocab` part blurbs
  don't. The test changes to require a non-empty blurb from `vocab`, which `vocab`'s own tests already check.
- **The starter is not loaded by any test today.** R2 adds a test that loads `cue/starter.cue.tmpl`, so each
  later schema change must keep it valid.

## Checkpoints

After every task: `make generate`, `go build ./...`, `make vet test`, the starter test, and
`diff cue/starter.cue.tmpl .wendy/wendy.cue`. After R4 and R8, also `make lint`. After R8, `make verify-generate`
(runs `git diff`; ask first), and the spec is marked implemented.
