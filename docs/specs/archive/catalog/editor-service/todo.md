# Tasks: editor-service — V1

From [plan.md](plan.md) and [spec.md](spec.md). Criteria numbers refer to the spec's Success Criteria.

## Phase A — Independent

- [x] **E1. Field model corrections**
  - Done: `Field.Advanced` on 17 fields (none required, none machine settings); `EnvironmentFields` is `description` only; `ProfileEvidenceFields` added; coverage test counts evidence under the profile list; `TestAdvancedFields` and `TestProfileEvidenceFieldsRoundTrip`.
  - Acceptance: `ui.Field` has `Advanced bool`, set on the fields listed in the plan (none `Required`); `EnvironmentFields` holds only `description`; `ProfileEvidenceFields` holds `evidence.consoles` and `evidence.dump`; `NewValue` and `Missing` work per part kind (already so); the field-coverage test counts the evidence settings under the profile list and still totals the original 46 settings plus `description`. Criteria 5, 6.
  - Verify: `go test -race ./internal/ui/`
  - Files: `internal/ui/fields.go`, `internal/ui/fields_test.go`

- [x] **E3. `Selection.Evidence` override**
  - Done: `Selection.Evidence`; `effective` starts from the profile's evidence and an override that differs marks the plan edited (an override equal to the profile's does not); `New` reads it from the effective selection. `TestEvidenceOverride`.
  - Acceptance: `plan.Selection.Evidence *config.Evidence` (JSON `evidence,omitempty`); `effective` applies it over the profile's, marks the plan edited when it differs, and `New` gives every combination that evidence; `Key` changes with it; with nil nothing changes. Criterion 7.
  - Verify: `go test -race ./internal/plan/`
  - Files: `internal/plan/plan.go`, `internal/plan/plan_test.go`

## Phase B — Service

- [x] **E2. Editable kinds, references and `UsedBy`**
  - Done: `config.Kinds` has all seven; web `editable` list keeps `/edit/environments` and the part routes at 404 (existing web test still passes); `config.SelectedParts` shared with the resolver; `Service.UsedBy` exported and covers parts, environments, scenarios and suites; `environmentRefs` rejects unknown parts with a suggestion and counts parts from the same spec; `prepare` normalizes and checks parts; `originOf` reads `EnvSpecs` so an environment that fails to resolve still has an origin. Tests: lifecycle, unknown part, built-in parts, matrix and profile users, required fields, copies of built-ins.
  - Depends on: E1
  - Acceptance: `config.Kinds` lists all seven kinds; the web UI has its own `editable` list (scenarios, suites) used by `editKind` and `canEdit`, so `/edit/environments` and `/edit/hardware` still 404; `config` exports the axis-selection helper the resolver uses; `Service.UsedBy` is exported and covers part ← environments and matrix axes, environment ← profiles and matrices (`base`), scenario ← suites, suite ← profiles and matrices; `Service.refs` rejects an environment naming an unknown part (message `unknown <kind> "<name>"` plus suggestion, nothing written), counting parts in the same spec; `prepare` checks required part fields and normalizes a Tinkerbell part; `originOf` handles the part kinds; create, copy, rename, delete work for each new kind and load back with no errors; delete and rename are refused while used, listing the users. Criteria 1, 2, 3, 4.
  - Verify: `go test -race ./internal/service/ ./internal/config/ ./internal/ui/web/`
  - Files: `internal/config/edit.go`, `internal/config/resolve.go`, `internal/service/edit.go`, `internal/service/edit_test.go`, `internal/ui/web/edit.go`, `internal/ui/web/pages.go`, `internal/ui/web/edit_test.go`

### Checkpoint B
`go build ./...` and `go test ./...` pass; the web and TUI behave as before.

## Phase C — Gate

- [x] **E4. Full verification**
  - Done: `make vet`, `make lint` (0 issues) and `make test` (race) pass; `evidence` appears in `ui/fields.go` only in the profile list.
  - Depends on: E2, E3
  - Acceptance: all eight criteria hold; no stale references to the removed environment evidence fields.
  - Verify: `make test && make vet && make lint`; `grep -rn "evidence" internal/ui/fields.go` shows it only in the profile list
  - Files: none expected

## Notes

- Commit per task with `git commit -s`; ask before every git command.
- Edit files with the edit tool, one edit per call.
