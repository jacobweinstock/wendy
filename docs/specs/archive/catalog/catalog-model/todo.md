# Tasks: catalog-model — V1

From [plan.md](plan.md) and [spec.md](spec.md). Criteria numbers refer to the spec's Success Criteria. Each task fits one focused session and touches about five files or fewer.

## Phase A — De-risk

- [x] **T1. Spike: resolve a composite environment**
  - Result: resolve in Go after the directory merge; see plan.md decision 1. Spike test deleted; the baseline `TestQuickstartGolden` and `testdata/quickstart.golden.json` were captured from the unchanged code and stay.
  - Acceptance: a test builds a composite with a named part, an inline part and an omitted part (built-in default) and resolves it to a flat environment; a bad part name and a bad part value each report a file:line. The approach (CUE-side or Go-side) is recorded in a comment at the top of `schema.cue`.
  - Verify: `go test ./internal/config/ -run TestResolveSpike -v`; delete the spike test once T3 supersedes it.
  - Files: `internal/config/load_test.go` (temporary), `cue/wendy/schema.cue`

## Phase B — Core model

- [x] **T2. Schema, part defaults and the quickstart golden**
  - Depends on: T1
  - Acceptance: `schema.cue` defines `tinkerbells`, `providers`, `hardware`, `workloads` and the composite `#Environment` (name, inline or omitted for each part); `setup` derives from the resolved topology; the old inline fields are rejected. `builtin.cue` defines a `default` of each part and `quickstart` in the new form. A golden test holds today's resolved `quickstart` (captured from the current code first) and asserts deep equality including `setup`. Criteria 1, 2, 3 (CUE side).
  - Verify: `go test ./internal/config/ -run TestQuickstartGolden`; `go run ./cmd/wendy vet`
  - Files: `cue/wendy/schema.cue`, `cue/wendy/builtin.cue`, `internal/config/load_test.go`, `internal/config/testdata/quickstart.golden.json`

- [x] **T3. Go types, loader and reference errors**
  - Done: `resolve.go` (post-merge resolver), `Catalog.{EnvSpecs,Tinkerbells,Providers,Hardware,Workloads}`, `Part`, `EnvSpec`, matrix `base` as a name with expansion moved after the merge. Tests: golden, named/inline/omitted parts, unknown-part suggestion with file:line, faulty part reported once, unused part checked, part from another directory. The matrix and example tests in `load_test.go` were migrated.
  - Also changed in the schema: `management.type` and `hardware.backend.type` now default to `kind` and `libvirt` (the only values), so a part need not repeat them.
  - Still red until T5: `internal/config/edit_test.go` (environment kind in the editor model).
  - Depends on: T2
  - Acceptance: `Catalog` gains `Tinkerbells`, `Providers`, `Hardware`, `Workloads` as `Item[...]`; `Catalog.Environments` still holds the flat resolved environment; `EnvRef` records which parts each environment named. An unknown part name reports `file:line`, the path (e.g. `environments.ha.hardware`) and up to three suggestions. A bad part reports once, at its own position. Matrix `base` is an environment name resolved by the loader; existing flat axis patches still expand. Criteria 3, 4, 5, 6.
  - Verify: `go test -race ./internal/config/`
  - Files: `internal/config/types.go`, `internal/config/load.go`, `internal/config/load_test.go`

## Phase C — Migrate and extend (T4, T5, T6 are independent)

- [x] **T4. Migrate fixtures, starter and docs**
  - Done: fixtures in `config`, `plan`, `service`, `ui/tui`, `ui/web`; `starter.cue.tmpl` and `.wendy/wendy.cue`; README (new "Environments are made of parts" section) and `design.md`. Added a migration hint: the old `capi`, `management`, `network` and `arch` fields now report where they moved, instead of "field not allowed".
  - Depends on: T3
  - Acceptance: every test fixture uses the new form through one helper; `starter.cue.tmpl` and `.wendy/wendy.cue` use parts and `base: "default"`; README and `design.md` describe the four parts, show an example, and note the migration from the inline form. Criterion 8.
  - Verify: `go test ./internal/plan/ ./internal/service/ ./internal/ui/... ./internal/config/`; `go run ./cmd/wendy vet`; `go run ./cmd/wendy plan --profile quickstart`
  - Files: `cue/starter.cue.tmpl`, `.wendy/wendy.cue`, `README.md`, `design.md`, test fixtures in `internal/plan`, `internal/service`, `internal/ui`

- [x] **T5. Editor plumbing for the new kinds**
  - Done: `SpecOf`, `Taken` and `Check` handle the part kinds; `Check` does not flag a part another file defines.
  - Depends on: T3
  - Acceptance: `Spec`, `SpecOf`, `Taken` and `Check` handle the four part kinds; a part written to `wendy-ui.cue` loads back identical; `SpecOf` for an environment returns its part references and inline parts, not the flat form. Criterion 7.
  - Verify: `go test ./internal/config/ -run 'Spec|Edit|Taken|Check'`
  - Files: `internal/config/edit.go`, `internal/config/edit_test.go`

- [x] **T6. Field model per part**
  - Done: `TinkerbellFields`, `ProvidersFields`, `HardwareFields`, `WorkloadFields`, `EnvironmentFields` (description, evidence) and `PartFields(kind)`. `TestEveryEnvironmentFieldHasOneHome` pins the original 46 environment settings to exactly one home each.
  - Depends on: T3
  - Acceptance: `internal/ui/fields.go` exposes a field list per part (replacing `EnvFields` and `EnvGroups`); the union equals today's `EnvFields` set, each field in exactly one part, `arch` on Hardware; `Get`/`Set` round-trip per part; `NewValue` and `NormalizeEnv` work per part; machine-table helpers move to Hardware. Criterion 9.
  - Verify: `go test ./internal/ui/`
  - Files: `internal/ui/fields.go`, `internal/ui/fields_test.go`

### Checkpoint C
`go build ./...` compiles (web and TUI may reference removed `EnvFields`; T7 resolves that), `go test ./internal/config/ ./internal/plan/ ./internal/ui/` green.

## Phase D — Interim UI state and verification

- [x] **T7. Disable environment editors**
  - Done: `config.Kinds` is scenarios and suites only; the service, web (routes, templates, section model) and TUI (tab, form, palette action) environment editors are removed. 404 tests cover `/edit/environments`. The stepper CSS and JS in `wendy.css` and `app.js` (`.stepnav`, `.pane`, `details.adv`, `showPane`) now have no caller; they are kept for web-catalog-ui to reuse.
  - Depends on: T4, T5, T6
  - Acceptance: `config.Kinds` no longer lists `environments`; no "＋ New environment" or "Edit/Copy" for environments in the web catalog; `/edit/environments` returns 404; the TUI offers no "new environment" or environment edit action and the service rejects the kind. Web and TUI compile against the new field model; the scenario and suite editors are unchanged. The environment section of `edit.templ`/`edit.go` that no longer has a caller is removed, not left dead.
  - Verify: `go test -race ./internal/service/ ./internal/ui/...`; `make generate && make verify-generate` if `.templ` changed
  - Files: `internal/config/edit.go`, `internal/service/edit.go`, `internal/ui/web/{edit.go,edit.templ,pages.go,views.templ}`, `internal/ui/tui/{edit.go,tui.go}`

- [x] **T8. Full verification**
  - Done: `make test` (race), `make vet`, `make lint` (0 issues) pass; generated templates current by checksum (`make verify-generate` itself runs git, so it was not run); `wendy plan --profile quickstart` and `--profile mine` work; web smoke test passes.
  - Depends on: T7
  - Acceptance: all ten spec success criteria hold; `wendy plan --profile quickstart` is unchanged apart from part names; no references to removed names remain.
  - Verify: `make test && make vet && make lint`; `go run ./cmd/wendy vet`; `go run ./cmd/wendy plan --profile quickstart`; `grep -rn "EnvGroups\|EnvFields" internal` is empty
  - Files: none expected

## Notes

- Commit per task, with `git commit -s` and a title that says what changed; ask before every git command.
- Run `make generate` only when a `.templ` file changes, using the pinned templ in `out/tools`.
