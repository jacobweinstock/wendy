# Plan: plans-and-matrices — V1

Implements [spec.md](spec.md). Tasks: [todo.md](todo.md).

## Approach

The plan form and the run form become one component, because a plan has the same fields that the run form
already collects. The matrix editor is a new section on the testbed form, built on the existing `matrixView` and
`htmx` fragments. The tab rename comes first because it's the smallest, and it moves the routes every later test
uses.

## Decisions

1. **Suites tab.** The top-level tab `tests` becomes `suites` in `configTabs` and `urls.go`. The subtab slugs
   (`suites`, `tests`, `steps`) and the `vocab` kinds don't change. Keys keep their order, and "tests" becomes a
   palette alias.
2. **One form, two modes.** `run.templ`'s cards become one component, used by `/run` (mode run) and by
   `/edit/plans…` (mode plan).
   - Only mode run has the **Plan** card.
   - Only mode plan has **Name** and **Description**.
   - Each mode has its own save bar.
3. **Form to plan.** `formSelection` already reads the cards into a `preview.Selection`. A new
   `planOf(sel) map[string]any` writes the plan sparsely: fields at their schema default are left out.
   **Save as plan…** reveals a name field inline in the save bar (no JS `prompt`), then posts the run form to
   `/edit/plans`.
4. **`KindPlan` is editable.**
   - `config.SpecOf`, and the service's `originOf`, `Owned`, `prepare` and `refs` handle it. The new `planRefs`
     checks that the testbed and suites exist, and that combinations are only set when the testbed has a matrix.
   - `UsedBy(KindPlan)` is always empty.
   - The plan detail gets Edit plan or Copy and edit…, and Delete plan. The Plans tab gets ＋ New plan.
5. **Blank form.** `/run` with no plan shows the Plan card set to **Blank** and no testbed picked. Run is disabled
   with "Pick a testbed" (`errNoTestbed`), the way `errNoSuite` works today. This replaces the run form's
   preference for `mine`.
6. **Unsaved plan.** The Executions row and the execution page show it where the plan is empty.
7. **Matrix hint.**
   - A `matrixExample` component fills the snippet with the testbed's name and the first two workload cluster
     parts (or `k8s-1.36` and `k8s-1.37` when fewer exist).
   - It ends with "Add it to <file>", where the file is the testbed's `Origin`.
   - A built-in testbed says "Copy and edit… it first".
8. **Matrix editor.**
   - **Form fields:**
     - `ax.<i>.name`;
     - `ax.<i>.varies`: a part kind's ID, or `topology`, `bootMode` or `network.family`;
     - `ax.<i>.v`: the checked values;
     - `ex.<combination>` and `ex.<combination>.reason` for a combination left out.
   - **Values offered:** a part kind offers its parts' names. A setting offers its choice values, labelled as
     `ui` labels them.
   - **Live combinations:** `hx-post` to `/edit/matrix-combinations`, a fragment in the same style as
     `part-summary`, renders them.
   - **Saving:** `matrixFrom(form)` builds `order`, `axes` and `exclude`.
   - **What the editor can express:** `editableMatrix(m)` returns true when every value sets exactly one field
     (a part's name, or one of the three settings), and every `exclude` names every axis. Otherwise the matrix is
     read-only, as today, and kept unchanged on save.
   - **An axis that varies a part:** the testbed's own pick for that kind stays as the base, and each axis value
     overrides it, which is what the schema already does.

## Order and checkpoints

| Phase | Tasks | Checkpoint |
|---|---|---|
| A. Nav | T1 Suites tab | `go test ./internal/ui/web/` |
| B. Plans | T2 `KindPlan` in config and service · T3 plan form and plan actions · T4 Run a plan… | `make vet lint test`, and the glossary guard passes |
| C. Matrix | T5 hint · T6 editor, reading and writing · T7 live combinations and leaving one out | `make vet lint test` |
| D. Docs | T8 README and `design.md` §11 | the `make generate` hash check, and a live check at `192.168.2.50:7601` |

## Risks

- **`cyclop`.** The run page's handlers are close to the complexity limit, and the mode split adds branches.
  `runTestsData` gets a mode, and its handlers are split into small helpers.
- **Axis names on re-save.** Saving must keep existing axis and value names, so combination names (and so
  history) don't change. `order` carries the axis order, and value names come from the parts or choices, so they
  stay stable.
- **Excludes after a change.** When an axis value is unchecked, any exclude that names it is dropped quietly,
  because its combinations no longer exist.
