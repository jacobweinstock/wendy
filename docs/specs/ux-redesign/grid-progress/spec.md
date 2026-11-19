# Spec Delta: grid-progress — V1

Status: implemented. Plan: [plan.md](plan.md); tasks: [todo.md](todo.md). Mockup: [mockups/web/](../../../mockups/web/index.html),
Execution: running, matrix (#412).

A delta to [web-ux/spec.md](../web-ux/spec.md), implemented, which keeps the execution grid "as today". It comes
from running the 16-combination `capt-e2e` plan. Execution #4's grid showed ○ waiting in every cell but one: Set up
testbed ✓ on `colocated-ipv4-isoboot-direct`, the third row, which was deploying. Nothing on the page said which
combination was running, or what it was doing.

## Objective

- **The grid shows what is running.** On an execution's page you can tell at a glance which combination is
  running, which phase or suite it is in, and which step it is on (NN/g heuristic #1, visibility of system status).
  The [mockup](../../../mockups/web/app.js) `gridView` already shows a running phase as ●; the implementation lost
  it.
- **Combinations start in the order they are listed,** so with one at a time the grid fills from the top down.

## Current Behavior

- **Cells only show verdicts.** They come from `CombinationSummary.Nodes`, which the engine writes when setup or a
  suite *finishes*. A phase or suite that is running shows ○ waiting, the same as one that hasn't started. The
  legend lists ● running, but no cell ever shows it.
- **A running row looks like a waiting one.**
  - The combination's name has no status.
  - Time stays empty until the combination finishes, because `Ms` is set at the end.
  - The last column only ever holds a failure message.
- **The header** says "running · 8/264 steps", but not where.
- **The order is arbitrary.** Every combination's goroutine races for a Combinations at once slot, so with one slot
  they start in an arbitrary order. In #4 the third row went first.
- **The TUI's grid** reads the same `Nodes` and has the same gaps.
- **The tree** is not affected: it shows live statuses and has Follow the running step.

## New Behavior

### Grid cells

- A cell shows its phase or suite's live status, taken from the execution's events (the `ui.ExecutionView` the tree
  already draws from), so the grid and the tree always agree:
  - **● running** from its first step starting until it finishes, including the moment between two of its steps;
  - ○ waiting before it starts;
  - its verdict once it finishes, as today.

### The combination's row

- **Every row's name starts with its combination's status glyph:** ○ waiting, ● running, ‖ paused, ✓ passed and so
  on, with the status word in its title. This is the same glyph the tree's combination picker shows.
- **A running or paused combination's last column says where it is:** its current step, written as Home's Running
  card writes it, e.g. `Set up testbed › tinkerbell.install`. It links to that step in the tree. A failure message
  still takes that column when there is one.
- **Time counts up while it runs:** the time since the combination started, updated with each refresh.

### Order

- **Combinations start in the order the preview lists them,** each as soon as a slot is free. With Combinations at
  once = 1, they run top to bottom. With *n*, the first *n* start together.

## Success Criteria

1. While a combination's setup runs, its Set up testbed cell shows ● running. While a suite runs, that suite's cell
   does, between steps too. Cells that haven't started show ○ waiting, and finished cells show the same verdicts as
   today.
2. Every row's name is preceded by its combination's status glyph, with the status word as its title.
3. A running combination's row:
   - names its current step, e.g. `Set up testbed › tinkerbell.install`;
   - links that step to it in the tree;
   - shows a Time that grows with each refresh.

   A paused combination's row shows ‖ and the step it paused at.
4. With Combinations at once = 1, combinations start in the listed order. With *n*, no combination starts before
   every combination listed above it has started. An engine test covers both.
5. A web test renders a live execution and checks the running cell, the glyph, the step link and the time. The
   existing execution, guard and label tests pass, as do `make vet lint test`, and the generated templates are up
   to date.
6. `design.md`'s Execution row says the grid shows each combination's status and what a running one is doing.
7. The web mockup's grid shows a status glyph before each combination's name, and a running combination's current
   step and time.

## Boundaries

Everything else is inherited from [web-ux/spec.md](../web-ux/spec.md).

- **Never:**
  - change what `CombinationSummary.Nodes` means, or the `execution.json` format. Nodes stay verdicts, which the
    combination's own verdict and older executions rely on, and the live statuses come from the events;
  - add a status, glyph or colour.
- **Ask first:** changing the execution header, or how the tree view picks its node.

## Out of Scope (Deferred)

- A "Now running" line in the execution header, visible from the tree view too. The grid row covers it, and the
  tree has Follow the running step.
- Home's Running card naming the combination as well as the step.
- Ordering combinations by estimated time, or running the ones that failed last time first.
- Showing a phase's or suite's step count (e.g. `3/8`) inside its cell.
- The TUI's grid. It has the same gaps and reads the same data, but waits for `tui-ux`.

## Decisions

1. The start order (Order, criterion 4) ships with this change.
2. The TUI's grid waits for `tui-ux`.
