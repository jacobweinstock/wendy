# Tasks: grid-progress — V1

From [plan.md](plan.md) and [spec.md](spec.md). Criteria numbers refer to the spec's Success Criteria.

All four tasks are done. Notes from building them:

- **T1:**
  - The new test also found a data race. `Execution.update` copied the summary under the lock and wrote it after,
    but the copy shared the combinations' slice and `Nodes` maps, so two combinations at once could write
    `execution.json` while it was being marshalled. It now writes under the lock, which also keeps two updates from
    sharing the temporary file.
  - A combination still waiting for a slot when the execution is cancelled is marked cancelled in the loop, and
    takes no slot.
- **T2:**
  - A parent's status in the view can't lead to the running step: once its finished steps have rated it, starting
    the next step doesn't set it back to running. `Now` searches every child instead. The tree's setup row shows
    the same thing (✓ while its next step runs); that is outside this change.
- **T3:**
  - `TestExecutionFoldsOtherKeptTestbeds` finds the grid's combination link with a regex, which now allows the
    status glyph before it.
  - The live check served a copy of execution #4 (`capt-e2e`, 16 combinations) from a temporary data directory,
    cut while `colocated-ipv4-netboot-direct` ran `assert.workflows`:
    - its row showed ● and `smoke › deploy › assert.workflows`;
    - its Time counted up across refreshes (34m02, then 34m09);
    - the link opened the tree at that step, and the 14 waiting rows showed ○ with no time.
- **T4:** `design.md` also says combinations start in the order they're listed.

- [x] **T1: Start combinations in order.**
  - Acceptance: criterion 4.
  - Verify: an engine test with six combinations, at one and at two at once, checks the order of
    `combination.started` against the preview's order. `go test -race ./internal/engine/`.
  - Files: `internal/engine/run.go`, `internal/engine/engine_test.go`.
- [x] **T2: Live state in the execution view.**
  - Acceptance: `Phase`, `Now` and `Elapsed` behave as the plan says: running between two steps, the failure
    while paused, verdicts from the summary.
  - Verify: `go test ./internal/ui/`.
  - Files: `internal/ui/view.go`, `internal/ui/view_test.go`.
- [x] **T3: The grid shows what is running.**
  - Acceptance: criteria 1–3 and 5.
  - Verify: a web test of a live execution checks the running cell, the glyph, the step link and the time.
    `make generate vet lint test`.
  - Files: `internal/ui/web/pages.go`, `internal/ui/web/executions.templ` (and its generated file),
    `internal/ui/web/executions_test.go`.
- [x] **T4: Docs.**
  - Acceptance: criteria 6 and 7.
  - Verify: read through.
  - Files: `docs/design.md`, `docs/README.md`, this folder.
