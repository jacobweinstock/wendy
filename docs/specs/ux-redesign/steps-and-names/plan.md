# Plan: steps-and-names — V1

Implements [spec.md](spec.md). Tasks: [todo.md](todo.md).

## Approach

The work comes in three slices, ordered by risk:

1. **Built-in names.** Mechanical, but it changes goldens and many tests.
2. **Your own steps.** A new kind, from the schema through the service to the web UI.
3. **No "playground".** Copy changes.

Each slice ends green.

## Decisions

1. **The fallback names live only in the CUE schema.** `#Testbed`'s part fields read
   `*"chart-latest" | #PartRef`, and so on for each kind. The loader reads these defaults into
   `Catalog.Fallback[kind]`, so Go never spells them. This replaces the literal `"default"` in `resolve.go`
   (`checkUnused`) and `web/edit.go`, where the part picker and saving a testbed use it.
2. **Redefining a built-in is caught in the loader.** It compares each directory's entries with the built-ins
   (`base` in `Load`). An entry that a user file wrote, under a built-in's name, gets a `file:line` error: "… is
   built in; copy it under another name". This applies to every kind.
3. **Catalog fields.** `Catalog.Steps []Builtin` becomes `Catalog.BuiltinSteps`. The new
   `Catalog.Steps map[string]*Item[Step]` holds yours, and directories merge it like the other kinds.
4. **`#Step.uses` becomes a string.** `with: #Builtins[uses]` applies only when `#Builtins[uses] != _|_`. The
   reference check moves to `crossCheck` in Go, which needs both built-in IDs and your steps, wherever they are
   defined. `config.Check` validates `wendy-ui.cue` on its own, so it can't see steps defined in other files.
5. **Expansion happens in `preview`, not when the config loads.** `preview` calls `cat.Expand(step)` on each step.
   Tests stay as written, so the editors, Copy and edit… and View CUE show `uses: "wait"`. `cli/upgrades.go`
   calls `Expand` too, so a step of yours that wraps an upgrade step still counts as an upgrade.
6. **Overrides.** A field the test sets overrides the step's own when it differs from the schema's default:
   - `name` is set;
   - `timeout` isn't `15m`;
   - `retries` isn't 0;
   - `when` is set;
   - `continueOnFailure` is true;
   - `labels` is not empty.

   The expanded step's name is the test's, else your step's own, else its key, so the tree and events show
   `wait`. One edge case: a test can't set a step back to exactly `15m` if the step sets longer. The README says
   so.
7. **Web.**
   - The Steps subtab lists built-in steps (origin "built-in") and yours together. `builtinDetail` stays, and a new
     `stepDetail` shows yours.
   - The step form is the test form's step block holding one step, with no add, move or remove. It posts `s.0.*`,
     so `applyForm`'s step parsing is reused.
8. **Service.** `KindStep` is added to:
   - `originOf`, `Owned` and `config.SpecOf`;
   - `prepare`, using `ui.Valid`;
   - `refs`, with a new `stepRefs`: `uses` may name only a built-in, `with` only goes with a built-in, and the
     name may not be a built-in's ID;
   - a new `testRefs`: each `uses` names a built-in or one of your steps;
   - `UsedBy`, which lists the tests that use the step.

## Order and checkpoints

| Phase | Tasks | Checkpoint |
|---|---|---|
| A. Names | T1 built-in names · T2 redefinition error | `make vet test`. The quickstart golden differs only in names. |
| B. Steps | T3 schema, config, preview · T4 service · T5 web list and detail · T6 web forms and Named step · T7 CLI and TUI | `make vet lint test`, and the glossary guard passes |
| C. Words | T8 playground and docs | `make vet lint test`, `make generate` hash check |

T3–T7 run in order. T8 can follow any time after T1.

## Risks

- **The CUE guard.** If CUE rejects `#Builtins[uses] != _|_`, the fallback is `with?: _`, with `with` checked in
  Go against the built-in's schema. T3 tries the guard first.
- **Golden churn.** The quickstart golden changes, but only in names. I'll regenerate it and review the diff.
- **`.wendy/` is your local config.** It has a testbed named `default` and a "playground" comment. That testbed
  isn't a built-in, so it keeps loading, and I'll leave the file alone unless you want it refreshed from the new
  starter.
