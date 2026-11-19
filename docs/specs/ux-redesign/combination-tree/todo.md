# Tasks: combination-tree — V1

From [plan.md](plan.md) and [spec.md](spec.md). Criteria numbers refer to the spec's Success Criteria.

All four tasks are done. Notes from building them:

- **T2:**
  - The pick tests use new executions rather than #5, whose paused combination is also the first, so each case
    of the order is told apart. #6 has one each of passed, failed, running and paused.
  - The node pane's consoles and kubeconfigs come from the shown combination. Before, they came from the picked
    row's, which for a build row meant the first combination.
  - `make lint` reports two findings in code this change doesn't touch: `gofumpt` on `diagnosticKinds` in
    `pages.go`, and `revive` in `internal/config/load_test.go`. `make vet test` pass.
- **T3:** The check served a copy of execution #16 (`capt-e2e`, 16 combinations) from a temporary data directory,
  cut while `colocated-ipv6-isoboot-direct` ran `assert.nodes`:
  - the tree opened on that combination with Follow on its running step, and all 16 names fit the dropdown;
  - three fired refreshes swapped the page body each time; the dropdown stayed open with all 16 entries, and the
    step times moved on (7m43, then 8m21);
  - with the menu made to scroll, its scroll position was the same after a refresh;
  - a click outside and Esc each closed it;
  - picking `colocated-ipv4-netboot-direct` showed only its rows, and Follow kept `?combination=`.
- **T4:** The mockup's dropdown also closes on a click outside or Esc.

- [x] **T1: One combination's rows.**
  - Acceptance:
    - `ExecutionView.Tree(c)` lists the build's rows, then `c`'s phases, suites, tests and steps, every one
      expanded, with its top level at depth 0;
    - it has no combination row and no other combination's rows;
    - `Rows` and `Label` are unchanged.
  - Verify: a new test in `view_test.go`, then `go test ./internal/ui/...`, which runs the TUI's tests too
    (criterion 8).
  - Files: `internal/ui/view.go`, `internal/ui/view_test.go`.
- [x] **T2: The page shows one combination's tree.**
  - Acceptance:
    - criteria 2–7 and 10;
    - the dropdown's markup from criterion 1: the summary, every combination with its glyph in the listed order,
      and the shown one marked.
  - Verify: web tests, then `make generate vet lint test`. The tests cover:
    - the pick: paused, first running, a failed second combination, and the first;
    - `?combination=` and `?combination=nope`;
    - `?node=` on another combination's step;
    - Follow staying in `?combination=` while another combination started a step later;
    - a single combination (#1): no dropdown and no combination row;
    - the labels, and the grid's combination link.

    The tests that pin the grid's old combination links are updated.
  - Files: `internal/ui/web/pages.go`, `internal/ui/web/executions.templ` (and its generated file),
    `internal/ui/web/static/wendy.css`, `internal/ui/web/executions_test.go`.
- [x] **T3: The dropdown stays open across a live refresh.**
  - Acceptance: criterion 1's check by hand. Fired refreshes leave the dropdown open, re-drawn, at the same scroll.
    A click outside or Esc closes it.
  - Verify: by hand, as the plan's Risks describe. It needs a port forwarded to a second `wendy` serving a
    temporary data directory.
  - Files: `internal/ui/web/static/app.js`.
- [x] **T4: Mockup and docs.**
  - Acceptance:
    - criteria 9 and 11;
    - the spec's status reads implemented, as does the docs index, with notes from building here.
  - Verify: read through, and open the mockup's tree in the browser.
  - Files: `docs/mockups/web/app.js`, `docs/design.md`, `docs/README.md`, this folder.
