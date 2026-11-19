# Tasks: delete-where-created — V1

From [plan.md](plan.md) and [spec.md](spec.md). Criteria numbers refer to the spec's Success Criteria.

All four tasks are done. T2 and T3 were first built as Delete on the pickers and the run form, then reversed after
review; the spec's "Not on forms that pick" says why. Notes:

- **T1:** `.actions` now centres its items, so the "why" text lines up with the buttons.
- **T2/T3:**
  - Removed: `deleteLink`, `editData.Removable`, `runTestsData.FromYours`, `editDelete`'s `then`, the
    `.linkbtn.fail` style and `TestPickerDeletesItsPart`.
  - `TestPartDialog` and `TestRunTestsSaveAsPlan` now check that neither form offers Delete.
  - The dialog's event stayed `partPicked`, with `target: "#p-<kind>"`.
- **T4:** the live check at `192.168.2.50:7601`, against temporary config and state directories: Delete hardware
  part on its pane led to the Hardware list without it, and the built-in `small` offered no Delete.

- [x] **T1. Delete in every detail pane**
  - Acceptance: an entry of yours has Delete, enabled or disabled with what uses it. One from your own file says
    where to delete it. A built-in has neither. The edit form's save bar uses the same button.
  - Verify: tests in `config_test.go`, and the existing delete tests in `edit_test.go`.
  - Files: `internal/ui/web/{edit.templ,config.templ}`, `static/wendy.css`, tests.
  - Criteria: 1, 2.

- [x] **T2. No Delete on the testbed form's pickers**
  - Acceptance: a picker offers Open ↗ and New … part…, never Delete, on the page and in the replies to a change
    or a dialog save.
  - Verify: `TestPartDialog` in `edit_test.go`.
  - Files: `internal/ui/web/{edit.templ,edit.go}`, `static/{app.js,wendy.css}`, tests.
  - Criteria: 3.

- [x] **T3. No Delete on Run a plan…**
  - Acceptance: the Plan card offers no Delete, even for a plan of yours.
  - Verify: `TestRunTestsSaveAsPlan` in `run_test.go`.
  - Files: `internal/ui/web/{run.templ,pages.go}`, tests.
  - Criteria: 4.

- [x] **T4. Docs and checks**
  - Acceptance: README "Editing in the UIs" and `design.md` §11 say where entries can be deleted.
    `docs/README.md` lists this change.
  - Verify: `make vet lint test`, the generated-template hash check and a live check.
  - Files: `README.md`, `docs/design.md`, `docs/README.md`.
  - Criteria: 5, 6.
