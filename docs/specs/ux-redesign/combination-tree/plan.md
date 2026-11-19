# Plan: combination-tree — V1

From [spec.md](spec.md). Tasks: [todo.md](todo.md).

## Components

1. **One combination's rows, in `internal/ui`.** `ExecutionView.Tree(combination)` returns the build's rows, then
   the combination's phases, suites, tests and steps, every one expanded, with its top level at depth 0. `Rows` and
   `Label` stay as they are, for the TUI.
2. **Which combination and which row, in `pages.go`.**
   - **The combination shown** is picked in the spec's order: a `?combination=` that names one, the combination of
     the `?node=` row, a paused one, the first running one, the failed one, the first. Statuses come from
     `ui.CombinationStatus`, as the grid's do.
   - **`d.Combination`** becomes any combination the URL names, not only a kept one. `showKept` still opens a
     named card only if that combination is kept, so the kept cards behave as today.
   - **Follow** picks the shown combination's `Now`, else the build's running step while the shared build runs,
     else the step at its failure's path, else its last row.
   - **A `?node=` the tree doesn't show** (another combination's step) counts as none. Then Follow applies, so the
     page never selects a row it doesn't draw.
   - **Links:**
     - `nodeURL` keeps a `?combination=` the URL named when the row is in that tree, so after picking a row,
       Follow (`viewURL("tree")`) stays in the combination.
     - The grid's combination names and the dropdown's entries use `pickURL(name)`, `?view=tree&combination=name`.
     - The grid's step links stay `?node=`.
3. **Template and CSS.**
   - With more than one combination, `executionTree` starts with "Combination" and a dropdown,
     `<details class="dropdown" id="combination">`:
     - the summary shows the shown combination's glyph and name, then ▾;
     - the menu has one link per combination, in the listed order, each with its glyph, and the shown one is
       marked `on`.
   - Rows read `treeLabel(n)`. That is "Set up testbed", or "suite smoke" for a suite that isn't a phase
     (`engine.Phase*`), and `Label()` for everything else. The folded marker `›` goes, since nothing folds.
   - `wendy.css` gets `details.dropdown`: the summary looks like a small button, and the menu opens under it, over
     the page, scrolling past 60vh. The mockup links the same file (`static` is a symlink), so both draw it alike.
4. **Staying open, in `static/app.js`.**
   - Before an htmx swap, remember each open `details.dropdown` by id, with its menu's scroll, and restore both
     after the swap.
   - A click outside an open dropdown, or Esc, closes it.
   - About fifteen lines in the existing file.
5. **Mockup.** `treeView` in `docs/mockups/web/app.js` draws the same dropdown, and the mockup's `app.js` closes it
   on a click outside.
6. **Docs.** `design.md`'s Execution row, then the spec's status and the docs index.

## Decision for review

**The dropdown is a `<details>` menu of links, not a `<select>`.**

- The links keep the status glyphs' colours, as the grid's are.
- Picking needs no JavaScript, like Grid/Tree and the tree's rows.
- A `<select>` would need JavaScript to navigate on change. It would also have to hold refreshes while focused,
  and the page would stop updating for as long as focus stayed on it.

## Order

`ui.Tree` → the web page: pick, Follow, tree, dropdown markup and CSS → `app.js` → mockup and docs. Each step builds
and passes on its own. Before `app.js`, a live refresh can close an open dropdown, but picking already works.

## Risks

- **The kept cards ride on `?combination=`.** Widening `d.Combination` could change which card opens. `showKept`
  keeps its own check that a named combination is kept, and `TestExecutionFoldsOtherKeptTestbeds` pins the
  behaviour.
- **Mismatched URLs,** such as `?combination=a&node=<b's step>`. The combination wins, as the spec orders it, and
  the node is ignored. Links the page writes never mismatch.
- **JavaScript has no tests** (no npm here). The check is by hand:
  - build `bin/wendy` and serve a copy of a running execution from a temporary data directory, on a port you
    forward;
  - open the dropdown, then fire the `refresh` event that live events fire on `[data-refresh]`, from the browser;
  - the dropdown should stay open, its statuses re-drawn and its scroll kept.

  A copy only replays its events, so it never refreshes by itself.
- **`Node.Depth` is set while walking,** as `Rows` already does. The web page now calls only `Tree`, and the TUI
  only `Rows`, each on its own view.
