# Spec Delta: plans-and-matrices — V1

Status: implemented. Plan: [plan.md](plan.md); tasks: [todo.md](todo.md).

Follows [feedback-2026-11-18.md](../feedback-2026-11-18.md) items 4, 5, 6 and 9, as agreed. A delta to
[web-ux/spec.md](../web-ux/spec.md), which is implemented. It is independent of
[steps-and-names](../steps-and-names/spec.md); that spec only fills the Steps subtab this one moves.

## Objective

The plan is what people run most, so they should be able to make one in the web UI. The nav should also read
like the model: a plan picks a testbed and suites.

- **Plans:** New, Edit, Copy and edit… and Delete, plus Save as plan… on the run form.
- **Run a plan…:** the top-bar button. Every execution runs a plan, saved or unsaved.
- **Suites:** the Tests tab's new name.
- **Matrix:** a clearer hint now, then an editor on the testbed form.

## Current Behavior

- **Plans** are CUE-only. A plan's page has Run, Change and run…, View CUE, and the hint "Plans are written in
  CUE." The map deferred editing plans and "Save as plan".
- **Running:** the top bar has **Run tests…** (`n`, `/run`), a form with Testbed, Tests and Options cards and a
  preview. An execution with no plan shows "—" in Executions' Plan column. "(changed)" marks a plan whose settings
  the form changed.
- **Tests tab:** Tests (`/tests/{suites,tests,steps}/{name}`), with subtabs Suites · Tests · Built-in steps.
- **Matrix:** CUE-only. The testbed form shows a matrix read-only. With none, it says "None: this testbed is one
  combination. To test several, add a `matrix` to it in CUE.", which doesn't say where or how.

## New Behavior

### The Suites tab

- The top bar is Home · Executions · Plans · Testbeds · **Suites** (keys `1`–`5`), then **Run a plan…**.
- Routes are `/suites`, `/suites/{suites,tests,steps}/{name}`. `/tests…` answers 404, and "tests" stays a palette
  alias.
- Subtabs: Suites · Tests · Steps.

### Run a plan…

- The top-bar button reads **Run a plan…** (`n`, `/run`). The form's first card is **Plan**: a saved plan, which
  fills in the rest, or **Blank**. Then come Testbed, Tests, Options and the preview, as today. `/run/{plan}`
  opens with that plan picked.
- An execution from a blank form shows **Unsaved plan** where it now shows "—". "(changed)" stays for a saved plan
  run with changes.
- The save bar has **Save as plan…**, which asks for a name, saves to `wendy-ui.cue` and then shows that plan
  picked. Then Cancel and **Run**. Run never saves.
- The plan page keeps Change and run….

### Editing plans

- The Plans tab gets ＋ New plan, and each plan gets Edit plan (or Copy and edit… for a built-in or a plan from
  your own files) and Delete plan.
- Deleting asks to confirm. Nothing refers to a plan, so it has no Used by. Past executions keep their record.
- The plan form is the run form's Testbed, Tests and Options cards with Name and Description added, and the same
  live preview. Its save bar matches the other forms: "Saved to …", Delete plan, Cancel and **Save plan**.
- Every `#Plan` field is in the form: description, testbed, combinations, suites, labelFilter, parallel, keep,
  pauseOnFailure, diagnostics and consoles. Saving is sparse, like the other kinds.
- `config.KindPlan` becomes editable through `service.SaveEntry`, with the same checks before writing.
- The "Plans are written in CUE" hint goes.

### Matrix

- **The hint first.** A testbed with no matrix shows a short CUE example, filled in with its own name and the
  workload cluster parts that exist, and says which file defines the testbed:

  ```cue
  testbeds: mine: matrix: axes: k8s: {
  	"1.36": workloadCluster: "k8s-1.36"
  	"1.37": workloadCluster: "k8s-1.37"
  }
  ```

  The README's matrix section uses the same example.
- **Then the editor**, a Matrix section on the testbed form:
  - **＋ Vary a part or setting** adds an axis with:
    - a short name, e.g. `k8s`;
    - what it varies: one kind of part, or topology, boot mode or network family;
    - its values, as checkboxes of that kind's parts or that setting's choices.
  - Axes can move up and down and be removed. Their order names the combinations, as in CUE.
  - Below the axes, the combinations update live. Each can be left out, with a reason, which becomes `exclude`.
  - Saving writes `matrix.order`, `axes` and `exclude`.
  - A matrix the editor can't express stays read-only and unchanged on save, as today: one where a value sets more
    than one field, or sets a part's fields instead of naming a part.
- The testbed page's Matrix section gets an Edit matrix link to that section of the form.

## Success Criteria

1. The top bar reads Home · Executions · Plans · Testbeds · Suites, then Run a plan…. `/tests…` answers 404.
2. A plan made with ＋ New plan appears in Plans, runs, and is written sparsely to `wendy-ui.cue`. Edit, Copy and
   edit… and Delete work. A rejected plan writes nothing and shows its errors beside the form.
3. Save as plan… on the run form saves a plan with the form's values. Run alone saves nothing.
4. An execution from a blank form shows "Unsaved plan".
5. On the testbed form, two axes with two values each show four combinations live. One of them can be left out
   with a reason. Saving writes order, axes and exclude. The testbed page then shows the same combinations, and
   reopening the form shows the matrix editable. A matrix the editor can't express is unchanged after a save.
6. A testbed with no matrix shows the CUE example with its own name and the file that defines it.
7. The glossary guard passes, `make vet lint test` pass, and the generated templates are up to date.
8. README "Editing in the UIs" and `design.md` §11 describe the new tab, button, plan forms and matrix editor.

## Boundaries

Everything else is inherited from [web-ux/spec.md](../web-ux/spec.md).

- **Never:** TUI screens or TUI plan editing (`tui-ux`).
- **Ask first:** any change to `#Plan`'s or `matrix`'s schema.

## Out of Scope (Deferred)

- In the matrix editor, values that set several fields: they stay CUE-only, which is rare and still works.
- Plan and matrix editing in the TUI: `tui-ux`.

## Resolved Questions

1. **A blank form's testbed:** none. Run stays disabled until one is picked, because a blank plan shouldn't
   guess.
