# Tasks: composition-plan — V1

From [plan.md](plan.md) and [spec.md](spec.md). Criteria numbers refer to the spec's Success Criteria.

## Phase A — Core

- [x] **C1. Part selection in matrix expansion**
  - Done: `splitAxes`/`splitAxisValue` in `resolve.go`; `Matrix.AxisPos`; `Combo.Parts`. Unknown names are checked once per axis value at its position; conflicting selections set `Combo.Err` on that combination only. The unmodified 16-combination test passes.
  - Acceptance: an axis value with a part-kind key (`tinkerbell`, `providers`, `hardware`, `workload`) holding a string selects that part; remaining keys patch the resolved environment as today. An unknown name reports once, at the axis value's `file:line`, path `matrices.<m>.axes.<axis>.<value>.<kind>`, with suggestions. Two axes selecting different parts of one kind in a combination sets `Combo.Err` naming both axes; the same part twice is fine. `Matrix.AxisPos` is captured in `decodeMatrix`; `config.Combo.Parts` records the final named parts. A combination equals an environment naming the same parts (golden-compared); the existing 16-combination test is unchanged. Criteria 1, 2, 3, 4.
  - Verify: `go test -race ./internal/config/`
  - Files: `internal/config/resolve.go`, `types.go`, `load.go`, `resolve_test.go`, `load_test.go` (unchanged expectations)

## Phase B — Surfacing

- [x] **C2. Combination parts in the plan and `wendy plan`**
  - Done: `plan.Combo.Parts` (keyed by kind, e.g. `workloads`); `wendy plan` prints a `parts` line only for the kinds that differ across the combinations shown; `Needs` follows the swapped hardware (7 VMs for a 3+3+1 part, tested).
  - Depends on: C1
  - Acceptance: `plan.Combo` has `Parts`, set for matrix combinations and for a plain environment (from `EnvSpecs`); `wendy plan` prints parts after a combination's labels, only the kinds that differ across the combinations shown; host `Needs` reflects a swapped hardware part. Criteria 5, 6, 8.
  - Verify: `go test ./internal/plan/ ./internal/cli/`; `go run ./cmd/wendy plan --profile quickstart` is unchanged for a single combination
  - Files: `internal/plan/plan.go`, `plan_test.go`, `internal/cli/root.go`, `internal/cli/*_test.go`

- [x] **C3. `wendy list` for parts**
  - Done: `list tinkerbells|providers|hardware|workloads` print name, origin and the environments using each (or `unused`); `list environments` shows each one's parts. `config.PartsLine` is shared by the CLI, web and TUI.
  - Acceptance: `wendy list tinkerbells|providers|hardware|workloads` prints name, origin and the environments using it; `wendy list environments` also shows each environment's parts; the command's usage and valid-args list the new kinds. Criterion 7.
  - Verify: `go test ./internal/cli/`; `go run ./cmd/wendy list hardware`
  - Files: `internal/cli/root.go`, `internal/cli/*_test.go`

- [x] **C4. UIs show a combination's parts**
  - Done: a "Parts" row on the web environment page and the TUI environment detail, with a web test. The matrix detail pages list axes, not individual combinations, so there was no combination row to extend; `Combo.Parts` is available to them when a later UI module lists combinations.
  - Depends on: C2
  - Acceptance: where the web catalog and the TUI show a matrix combination or an environment's axes (`internal/ui/web/pages.go` axis line, `internal/ui/tui/screens.go`), the parts are shown too; read-only, no new pages or editors. Templates regenerated if a `.templ` changes. Criterion 5.
  - Verify: `go test ./internal/ui/...`; `make generate` then confirm generated files are current by checksum
  - Files: `internal/ui/web/pages.go`, `helpers.go`, `views.templ` (+ generated), `internal/ui/tui/screens.go`

## Phase C — Docs and gate

- [x] **C5. Docs and final verification**
  - Done: README (rule plus example), `design.md`, starter and `.wendy/wendy.cue` (identical) with a part-selecting matrix example. `make vet`, `make lint` (0 issues), `make test` (race) pass; templates current; verified end to end against a scratch config (`list hardware`, `list environments`, `list combos`, `plan` with four combinations).
  - Depends on: C2, C3, C4
  - Acceptance: README and `design.md` document the axis rule with the example from the spec; the starter config's commented matrix shows a part selection; `make test vet lint` pass. Criteria 9, 10.
  - Verify: `make test && make vet && make lint`; `go run ./cmd/wendy vet`; `go run ./cmd/wendy list combos`
  - Files: `README.md`, `design.md`, `cue/starter.cue.tmpl`, `.wendy/wendy.cue` (kept identical)

## Notes

- Commit per task with `git commit -s`; ask before every git command.
- `run-options` is built first; both modules touch `internal/plan/plan.go` and `internal/cli/root.go`, so they are not built concurrently.
