# Tasks: run-options — V1

From [plan.md](plan.md) and [spec.md](spec.md). Criteria numbers refer to the spec's Success Criteria.

## Phase A — Stop reading the environment's fields

- [x] **R1. Upgrade steps take their target from their parameters only**
  - Done: skip messages name the parameter; `upgradeTinkerbell` extracted; `internal/steps/upgrade_test.go` is the package's first test file.
  - Acceptance: `capt.upgrade.kubernetes`, `tinkerbell.upgrade` and `capi.upgrade` no longer read `Config.Upgrades`; with no target each skips with a message naming the parameter (`with.version`, `with.to`, `with.core` or `with.capt`); with a target they behave as before. `tinkerbell.upgrade`'s body is extracted to a named function. Registered descriptions and the three built-in scenario descriptions in `builtin.cue` no longer mention `upgrades.*`. Criterion 5.
  - Verify: `go test ./internal/steps/ -run Upgrade -v`
  - Files: `internal/steps/capt.go`, `tinkerbell.go`, `capi.go`, `internal/steps/upgrade_test.go` (new), `cue/wendy/builtin.cue`

- [x] **R2. `vet` finds upgrade targets in scenario steps**
  - Done: `upgradeVersions` in `internal/cli/upgrades.go`. It checks every catalog scenario's versions against every environment's OS version and arch, which over-approximates (a scenario no suite uses still counts) but never misses an image.
  - Acceptance: the image list includes the OS image for a Kubernetes version named by a `capt.upgrade.kubernetes` step's `with.version` in any catalog scenario; the `e.Upgrades` branch is gone. Criterion 7.
  - Verify: `go test ./internal/cli/`; `go run ./cmd/wendy vet`
  - Files: `internal/cli/root.go`, `internal/cli/*_test.go` (new test beside the extracted helper)

- [x] **R3. Evidence comes from the profile**
  - Done: `#Evidence` and `Profile.Evidence`; `config.DefaultEvidence()`; `New` picks the profile's evidence and `buildCombo` applies it; `TestEvidenceComesFromTheProfile` covers always, never and no profile.
  - Acceptance: `#Profile` accepts `evidence: {consoles, dump}` with the current defaults; `config.Profile` has `Evidence`; `effective` carries it; `buildCombo` sets each combination's `Env.Evidence` from it, and `dump: "never"` leaves `Combo.Evidence` empty; a selection with no profile gets the defaults; `collectBuilds` no longer reads `env.Upgrades` (step-parameter sources are still collected). Criteria 2, 6.
  - Verify: `go test ./internal/plan/ ./internal/config/`
  - Files: `cue/wendy/schema.cue`, `internal/config/types.go`, `internal/plan/plan.go`, `internal/plan/plan_test.go`

### Checkpoint A
`go test ./...` passes with `upgrades` and environment `evidence` still in the schema.

## Phase B — Remove

- [x] **R4. Remove `upgrades` and environment `evidence`**
  - Done: `#Upgrades`, `Environment.Upgrades`, `Upgrades` and `CAPIUpgrade` removed (`TinkerbellTarget` kept for the step); `assemble` and `build` no longer take the spec. Migration messages for environment `evidence` and `upgrades`, and for an axis patching `evidence`, each at the right file:line (`TestMovedFieldsExplainThemselves`). `quickstart` golden unchanged.
  - Depends on: R1, R2, R3
  - Acceptance: `#Environment` and `EnvSpec` drop `evidence` and `upgrades`; `#ResolvedEnvironment` drops `upgrades`, keeps `evidence`; `config.Environment.Upgrades` and its types (`Upgrades`, `CAPIUpgrade`; `TinkerbellTarget` stays if steps still use it) are removed; `assemble` stops passing them; an environment with either field, and a matrix axis patching `evidence`, fails with a message naming the new home at the right file:line. `quickstart` golden unchanged. Criteria 1 (as corrected in the plan), 3, 4, 8.
  - Verify: `go test -race ./...`; `grep -rn "Config.Upgrades\|env.Upgrades\|e.Upgrades" internal` is empty
  - Files: `cue/wendy/schema.cue`, `internal/config/types.go`, `resolve.go`, `load.go`, `resolve_test.go`

## Phase C — Docs and gate

- [x] **R5. Docs, starter and final verification**
  - Done: README section "What a run records, and upgrade targets"; `design.md` schema and scenario notes; starter and `.wendy/wendy.cue` (identical) show profile `evidence` and a commented upgrade scenario. `make vet`, `make lint` (0 issues) and `make test` (race) pass; generated templates unchanged; `wendy plan --profile quickstart` and `wendy vet` work.
  - Depends on: R4
  - Acceptance: README and `design.md` describe profile `evidence` and step-level upgrade targets; `starter.cue.tmpl` and `.wendy/wendy.cue` (kept identical) show a profile with `evidence` and a commented upgrade scenario; `make test vet lint` pass; generated templates unchanged. Criteria 9, 10.
  - Verify: `make test && make vet && make lint`; `go run ./cmd/wendy vet`; `go run ./cmd/wendy plan --profile quickstart`
  - Files: `README.md`, `design.md`, `cue/starter.cue.tmpl`, `.wendy/wendy.cue`

## Notes

- Commit per task with `git commit -s`; ask before every git command.
- Run `make generate` only if a `.templ` changes (none expected here).
