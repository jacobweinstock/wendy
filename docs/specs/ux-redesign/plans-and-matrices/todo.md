# Tasks: plans-and-matrices — V1

From [plan.md](plan.md) and [spec.md](spec.md). Criteria numbers refer to the spec's Success Criteria.

All eight tasks are done. Notes from building them:

- **T1:** the top-bar tab's ID is `suites`; the subtab slugs didn't change. `/edit/tests` stays, since edit URLs
  use the kind's slug.
- **T2:** `config.Check` evaluates the UI file on its own. It now also ignores a plan's testbed and suites, and a
  test's `uses`, since those can be defined in other files. `TestEntriesMayNameWhatOtherFilesDefine` covers this.
- **T3:**
  - `runForm` is shared by Run a plan… and the plan form. `planFormData` embeds `editData` and `runTestsData`.
  - `deleteAction` is shared by both save bars.
  - `planFromForm` takes the posted diagnostics, so a copy keeps its source's diagnostics.
- **T4:**
  - The Plan card sits outside `#runform`, so changing it reloads the page through `?from=`, not the preview.
  - Save as plan… is a `<details>` in the save bar. With `then=run` it returns to `/run/<name>`.
  - `first` went away with the default testbed.
- **T5:** `matrixExample` includes `order`, because an axis makes no combinations without it. The test loads the
  example. The README's own example defined `k8s-1.36`, which is now built in, so that line went.
- **T6/T7:**
  - The editor lives in `matrix.go`.
  - Value names that differ from the part or setting they pick are kept, through hidden `as` inputs.
  - `app.js` fires a change on the form after a row moves or is removed, so the combinations refresh.
  - The read-only matrix type in `pages.go` is now `axisChanges`.
- **T8:** the live check at `192.168.2.50:7601` against `.wendy/`: every tab answers 200, and `/tests…` 404.

## Phase A: nav

- [x] **T1. The Suites tab**
  - Acceptance:
    - The top bar reads Home · Executions · Plans · Testbeds · Suites.
    - The routes are `/suites/{suites,tests,steps}/{name}`, and `/tests…` answers 404.
    - The palette has "Go to Suites", with the alias "tests". Keys `1`–`5` are unchanged.
  - Verify: tests in `routes_test.go` and `config_test.go`, and the glossary guard.
  - Files: `internal/ui/web/{urls.go,pages.go,layout.templ}`, tests.
  - Criteria: 1.

## Phase B: plans

- [x] **T2. `KindPlan` in config and service**
  - Acceptance: plans can be saved, renamed, copied and deleted through the service. `planRefs` rejects an unknown
    testbed or suite, and combinations on a testbed with no matrix. A rejected plan writes nothing.
  - Verify: tests in `service/edit_test.go`.
  - Files: `internal/config/edit.go`, `internal/service/edit.go`, tests.
  - Criteria: 2.

- [x] **T3. The plan form and plan actions**
  - Acceptance:
    - ＋ New plan, Edit plan, Copy and edit… and Delete plan work.
    - The form is the run form's cards plus Name and Description, with a live preview, and it saves sparsely.
    - The "Plans are written in CUE" hint is gone.
  - Verify: tests in `edit_test.go` for new, edit, copy, delete and a rejected plan.
  - Files: `internal/ui/web/{run.templ,pages.go,edit.go,config.templ}`, tests.
  - Criteria: 2.

- [x] **T4. Run a plan…**
  - Acceptance:
    - The top bar has **Run a plan…**, and the form's first card is Plan, a saved plan or Blank.
    - A blank form picks no testbed, and Run says "Pick a testbed".
    - **Save as plan…** saves the form as a plan, and Run alone saves nothing.
    - An execution from a blank form shows "Unsaved plan".
  - Verify: tests in `run_test.go` and `executions_test.go`, and the glossary guard.
  - Files: `internal/ui/web/{run.templ,pages.go,layout.templ,executions.templ,home.templ}`, tests.
  - Criteria: 3, 4.

## Phase C: matrix

- [x] **T5. The matrix hint**
  - Acceptance: a testbed with no matrix shows the CUE example with its own name and parts, and the file to add it
    to. A built-in testbed is told to Copy and edit… first. The README's matrix section uses the same example.
  - Verify: a test in `config_test.go` and `edit_test.go`.
  - Files: `internal/ui/web/{config.templ,edit.templ,pages.go}`, `README.md`, tests.
  - Criteria: 6.

- [x] **T6. The matrix editor: reading and writing**
  - Acceptance:
    - The testbed form's Matrix section lists the axes, each with a name, what it varies and checkboxes for its
      values. Axes can be added, moved and removed.
    - Saving writes `order` and `axes`.
    - A matrix the editor can't express is read-only, and unchanged on save.
  - Verify: tests in `edit_test.go`: build a 2×2 matrix and save it, reopen it as editable, and check that a
    complex matrix is unchanged after a save.
  - Files: `internal/ui/web/{edit.go,edit.templ}`, `static/app.js` (moving axes, as with steps), tests.
  - Criteria: 5.

- [x] **T7. Live combinations, and leaving one out**
  - Acceptance:
    - The combinations update as axes change. Each can be left out with a reason, which saves as `exclude`.
    - Unchecking a value drops the excludes that name it.
    - The testbed page shows the same combinations, and has an Edit matrix link.
  - Verify: tests in `edit_test.go` for the fragment, an exclude round trip, and the dropped exclude.
  - Files: `internal/ui/web/{edit.go,edit.templ,urls.go,config.templ}`, tests.
  - Criteria: 5.

## Phase D: docs

- [x] **T8. README and design**
  - Acceptance: README "Editing in the UIs" and `docs/design.md` §11 describe the Suites tab, Run a plan…, the plan
    forms and the matrix editor.
  - Verify: `make vet lint test`, the `make generate` hash check, and a live check of every tab at
    `192.168.2.50:7601`.
  - Files: `README.md`, `docs/design.md`.
  - Criteria: 7, 8.

### Checkpoint

`make vet lint test` pass, the generated templates are up to date, and `spec.md` is marked
`Status: implemented`.
