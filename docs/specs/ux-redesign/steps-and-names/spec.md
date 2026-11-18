# Spec Delta: steps-and-names — V1

Status: implemented. Plan: [plan.md](plan.md); tasks: [todo.md](todo.md).

Follows [feedback-2026-11-18.md](../feedback-2026-11-18.md) items 1, 3 and 8, as agreed. A delta to
[model-rename/spec.md](../model-rename/spec.md) and [web-ux/spec.md](../web-ux/spec.md), both implemented.

## Objective

Three changes to what people write in CUE and read in the UIs:

1. **Your own steps are first-class.** A step is named once and used by many tests, the way a part is named once
   and used by many testbeds.
2. **Built-ins say what they are.** No built-in is named `default`.
3. **"Playground" goes.** It names the repo wendy grew out of, which a user has no reason to know.

## Current Behavior

- **Steps.** A test writes each step in place. `uses:` accepts only a built-in step's ID (`#Step.uses` in
  [schema.cue](../../../../cue/wendy/schema.cue)), so the same command step is copied from test to test. The Tests
  tab's third subtab is "Built-in steps", and a step's kind reads "Built-in step".
- **Built-in names.** Each kind of part has a built-in named `default`. A testbed that doesn't name a part of a kind
  gets it (`*"default" | #PartRef`), and the pickers offer "default". Workload cluster also has `quickstart` (the
  same, with kube-router). The starter config's testbed is named `default` too.
- **Playground.** The word appears in:
  - the quickstart plan's description ("Playground defaults…");
  - the smoke test's and suite's descriptions;
  - the workload cluster's default name `capt-playground`;
  - comments in `schema.cue`, the starter config, `steps/hardware.go` and `provider/hardware/libvirt.go`;
  - `tui.md` and `design.md`.

## New Behavior

### Your own steps

- `#Config` gains `steps: [string]: #Step`. A step defined there is one of a command, a container, Chainsaw tests,
  a Ginkgo test binary, or a built-in step with its parameters set. It can't use another of your steps.
- A test's step can use a built-in step or one of yours with `uses:`. In the test it can still set `name`,
  `timeout`, `retries`, `when`, `continueOnFailure` and `labels`, and these override the step's own. `with:` is
  only for built-ins.
- The loader puts the step's definition in the test's step, so the plan, the engine and events don't change.
- These are `file:line` errors:
  - an unknown step;
  - `with:` on your step;
  - your step named like a built-in;
  - your step using another of yours.
- A step lists the tests that use it ("Used by"). While a test uses it, it can't be deleted or renamed, as with
  parts.
- `config.KindStep` (`steps`) is editable through `service.SaveEntry` and `UsedBy`. `vocab` gets the kind: "step",
  "steps".
- **Web:**
  - The "Built-in steps" subtab becomes **Steps**. It lists built-in steps and yours together, each with where it
    comes from, as the parts subtabs do.
  - Your steps have ＋ New step, Edit step, Copy and edit… and Delete step. Their form uses the same step fields
    as the test form.
  - In the test form, the kind "Built-in step" becomes **Named step**. Its picker lists built-in steps and yours.
- **CLI and TUI:** `wendy list steps` lists both. The TUI only changes as far as it must to build; its screens
  stay with `tui-ux`.

### Built-in names

| Kind | Was | Now |
|---|---|---|
| Tinkerbell | `default` | `chart-latest` |
| Cluster API | `default` | `capi-1.14-capt-0.7` |
| Management cluster | `default` | `kind` |
| Workload cluster | `default`, `quickstart` | `k8s-1.36`, `k8s-1.36-kube-router` |
| Hardware | `default` | `small` |

- A testbed that names no part of a kind gets that kind's built-in from this table. The pickers list it first,
  marked "Used when a testbed picks none". Picking it still writes nothing, so the testbed stays sparse.
- A config can't redefine a built-in's name. It is a `file:line` error that says to copy it under another name,
  as the UIs already require. Without this rule, `chart-latest` could quietly pin an old chart.
- A version bump renames its built-in, e.g. `k8s-1.37`. A config that names the old one gets the usual "no
  workload cluster part named …; pick one of: …" error.
- The starter config's testbed `default` becomes `mine`, the name its plan already has.
- The testbed `quickstart` and the plan `quickstart` keep their names.

### No "playground"

- The quickstart plan's description becomes "One small testbed · kind + libvirt · smoke suite".
- The smoke test's description becomes "Health checks: CAPI and CAPT objects ready, every pod healthy, every
  Kubernetes component running". The smoke suite's becomes "Deploy, then the health checks."
- The workload cluster's default name becomes `wendy`. (The spec first said `workload`, but the glossary guard
  rejects that word on its own, and the name is shown in the UIs.)
- The comments in code, CUE and the starter config are reworded. The starter's "the Config tab" becomes "Config
  files".
- `vocab.Retired` gains `playground`, so the glossary guard catches it.
- `design.md` §14 stays as history, retitled "Origins: the Tinkerbell playground", with a link to
  <https://github.com/tinkerbell/playground>. The design doc's other mentions and `tui.md`'s are reworded.

## Success Criteria

1. A config with `steps: wait: exec: …` and two tests that use `wait` loads. `wendy plan` shows each test with
   the step, and a timeout set in the test overrides the step's.
2. Each error under "Your own steps" is reported with its `file:line`.
3. Deleting or renaming a step a test uses is refused, with its users, in the service and the web UI.
4. The web Steps subtab lists built-in steps and yours, with where each comes from. New, Edit, Copy and edit…
   and Delete work. The test form's Named step picker lists both kinds.
5. No built-in is named `default`. A testbed that leaves out a part resolves to the renamed built-in. The
   quickstart golden differs only in names.
6. Redefining a built-in's name in CUE is a `file:line` error.
7. `vocab.Check` flags "playground", and the glossary guard passes. A case-insensitive search for "playground"
   finds it in `cue/`, `internal/` and `README.md` only in `vocab`'s retired list. In `docs/design.md` it
   appears only in §14.
8. `make vet lint test` pass, and the generated templates are up to date.

## Boundaries

Everything else is inherited from [web-ux/spec.md](../web-ux/spec.md).

- **Never:**
  - parameters for your own steps;
  - your steps using your steps;
  - TUI screen changes (`tui-ux`).
- **Ask first:** renaming the `quickstart` testbed or plan.

## Out of Scope (Deferred)

- Parameters for your own steps (`with:` passed in as environment variables): useful, but not needed to stop the
  copying.
- Editing steps in the TUI: `tui-ux`.

## Resolved Questions

1. **The step kind's label** is "Named step". It covers built-in steps and yours, where "Use a step" reads as an
   action rather than a kind.
