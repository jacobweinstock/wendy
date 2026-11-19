# Spec Delta: combination-tree — V1

Status: implemented. Plan: [plan.md](plan.md); tasks: [todo.md](todo.md). Mockup: [mockups/web/](../../../mockups/web/index.html),
Execution: running, matrix (#412), Tree.

A delta to [web-ux/spec.md](../web-ux/spec.md), implemented, which kept the execution tree "as today". The mockup's
tree was never built. Running the 16-combination `capt-e2e` plan (#16) shows the cost: its tree lists all 16
combinations, mostly folded, and finding one means scrolling past the others.

## Objective

- **The tree shows one combination at a time, as the mockup does.** A dropdown above the tree picks the one it
  shows. With 16 combinations you see the one you care about, not a scroll through all of them.
- **The tree shows every step of that combination.** What ran, what is running and what is left are all on screen,
  with nothing to open (NN/g heuristics #1, visibility of system status, and #6, recognition rather than recall).
- **The docs agree with the page.** `design.md` describes this tree, and [grid-progress](../grid-progress/spec.md)'s
  "the tree's combination picker" becomes true.

## Current Behavior

- **One tree for the whole execution:** Build images, then one top-level row per combination, with its phases,
  suites, tests and steps under it.
- **Rows fold.** `ui.ExecutionView.Rows` folds any row that passed, was skipped or is waiting, except a suite.
  Picking a row opens it, and nothing else does. #16 shows "Set up testbed (8 steps) ›" and "deploy ›".
- **A suite's row reads its name**, "smoke". The mockup reads "suite smoke".
- **Collect diagnostics and Tear down rows appear once they start.** The preview doesn't list their steps.
- **Follow the running step** (no `?node=`) picks the step started most recently, in any combination. With several
  combinations at once it moves between them.
- **`?combination=`** only picks which kept testbed card is open.
- **The grid's combination names** link to that combination's row in the tree (`?node=<combination>`).
- **The TUI** draws the same `Rows` and `Label`.

## New Behavior

### Picking a combination

- With more than one combination, a **Combination** dropdown sits above the tree. Closed, it shows the picked
  combination's status glyph and name. Open, it lists every combination in the order the grid lists them, each
  with its status glyph, as the grid's rows do. #16's 16 long names fit, where a row of buttons wouldn't.
- Picking one shows its tree in the same view: `?view=tree&combination=<name>`, the parameter the kept testbed
  lines already use. Which kept testbed card is open follows the same rule as today.
- **A live refresh doesn't close the dropdown.** A running execution's page re-renders on every event, log lines
  included, so without this it would close while you pick.
- With one combination there is no picker, as there is no Grid/Tree.
- With no `?combination=`, or one that names no combination, the tree shows the first of:
  1. the combination of the row `?node=` names;
  2. a paused combination, since it waits for you;
  3. the first running combination, in the listed order;
  4. the failed combination, the one the failure card is about;
  5. the first combination.

### Follow the running step

- Follow stays in the picked combination. It picks its running step, else the step it paused or failed at, else its
  last row.
- With `?combination=` in the URL, following never switches combination. Without it, each refresh picks the
  combination again as above, so with one combination at a time the tree moves on when the next one starts.

### The tree

- The tree is the picked combination's alone. Its top-level rows are Build images (shared, so the same in every
  combination's tree), Set up testbed, each suite, then Collect diagnostics and Tear down. There is no combination
  row; the picker names it.
- **Every row shows, and nothing folds.** A row's time shows once it has one, as today.
- **Labels:** "Set up testbed" without a step count, since its steps show. A suite reads "suite smoke", as in the
  mockup. Phases read as `vocab` names them, as today.
- Picking a row selects it as today, and the node pane doesn't change.

### Links into the tree

- A grid row's combination name opens the tree with that combination picked. Its current step still opens that
  step.
- A kept testbed line's link, in tree view, opens that combination's tree: it already sets `?combination=`.

## Success Criteria

1. In tree view, an execution with more than one combination shows a Combination dropdown. Closed, it shows the
   picked combination's glyph and name. It lists every combination in the listed order with its status glyph, and
   picking one opens that combination's tree. Checked by hand on a running execution: the dropdown stays open across
   live refreshes, and 16 names as long as #16's fit.
2. The tree lists the picked combination's rows only, with no combination row. Every one of them shows: Set up
   testbed and each of its steps, each suite, test and step, and Collect diagnostics and Tear down once they start.
3. Web tests cover the pick: `?combination=b` shows b's tree; `?node=` on one of b's steps shows b's tree with that
   step picked; without either, each case of the order above; and a bad name falls back as if none were given.
4. A web test covers Follow: with `?combination=b`, it picks b's running step while another combination started a
   step more recently.
5. A grid row's combination name opens the tree with that combination picked.
6. A single-combination execution shows no picker, and its tree has no combination row.
7. Set up testbed reads "Set up testbed" with no count, and a suite reads "suite smoke".
8. The TUI draws as it did. Its tests and `ui`'s pass unchanged.
9. `design.md`'s Execution row describes the picker and one combination's tree.
10. Tests that pinned the old tree's links are updated, not removed. `make vet lint test` pass, and the generated
    templates are up to date.
11. The web mockup's tree picks its combination from a dropdown, so the mockup and the page agree.

## Boundaries

Everything else is inherited from [web-ux/spec.md](../web-ux/spec.md).

- **Always:** the picker and the grid show the same status for a combination (`ui.CombinationStatus`).
- **Never:**
  - change the TUI, which waits for `tui-ux`;
  - change the events, `execution.json` or `preview.json`;
  - add a status, glyph or colour.
- **Ask first:**
  - changing the node pane, the execution header or the grid beyond its combination links;
  - JavaScript beyond what the dropdown needs to open the picked combination and to stay open across a live
    refresh. JS has no tests here (no npm), so it stays small.

## Out of Scope (Deferred)

- Folding and unfolding rows by hand. One combination's tree is short enough to show whole.
- Collect diagnostics and Tear down as waiting rows before they start, as the mockup shows them.
- The node pane's differences from the mockup. It already has more than the mockup: consoles and each cluster's
  kubeconfig.
- "Pick a step to see its log" on a phase, suite or test row, as the mockup has.
- Searching or filtering the picker for executions with many more combinations.
- The TUI's tree, which waits for `tui-ux`.

## Decisions

1. **The picker is a dropdown,** not the mockup's segmented links, which don't fit 16 names like
   `colocated-ipv4-netboot-direct`. Statuses show once it's open.
2. **Nothing folds,** as in the mockup.
3. **Collect diagnostics and Tear down appear when they start,** as today. Whether they run depends on the outcome
   and Keep testbed, and the preview doesn't list their steps. Showing them as waiting rows from the start is
   deferred.
4. **With no combination in the URL,** the tree shows a paused one, else the first running one, else the failed
   one, else the first.
