# Spec Delta: execution-settings — V1

Status: implemented. Plan: [plan.md](plan.md); tasks: [todo.md](todo.md).

A delta to [web-ux/spec.md](../web-ux/spec.md) and [plans-and-matrices](../plans-and-matrices/spec.md), both
implemented. It comes from two problems found using executions:

1. The settings an unsaved plan ran with can't be seen. Execution #2's page says "unsaved plan · testbed
   quickstart-ipv6" and nothing more.
2. "Pause on failure: Wait for me before tearing anything down" was ticked, the execution passed, and the testbed
   was torn down. The checkbox reads as unconditional. It only acts when a step fails.

## Objective

- **What an execution ran with is visible on its page, and can be run again with changes.** You shouldn't need to
  remember what you picked (NN/g, recognition rather than recall).
- **Each choice says when it applies.** The two controls that decide what happens to the testbed are worded as the
  situations they cover, with every option explained, as on the testbed form.

## Current Behavior

- **Settings are hidden.**
  - An execution saves its selection in `execution.json`, and the API returns it (`GET /api/executions/{id}`,
    `selection`), but no page shows it.
  - Run again repeats it unchanged. There is no way to change it and run, or to save it as a plan, without filling
    in the form again.
- **Keep testbed** (Never · If it fails · Always) decides, when a combination ends, whether its testbed is torn down.
- **Pause on failure** (checkbox, "Wait for me before tearing anything down") applies when a step fails. Diagnostics
  are collected, and then the combination waits with everything up for one of these:
  - **Retry step**;
  - **Continue**, which records the failure and runs the rest;
  - **Abort and tear down**, which tears the testbed down even when Keep testbed says to keep it.

  Without it, a failed step stops the combination, and Keep testbed then decides.
- The TUI's run form and the CLI's `--pause-on-failure` use the same wording.

## New Behavior

### Settings on the execution's page

- Every execution's page gets a **Settings** section, whatever it ran: a saved plan, a changed one or an unsaved
  one. It lists exactly what the execution ran with:
  - Plan (a link, "(changed)" when it was changed, or "Unsaved plan");
  - Testbed and Combinations;
  - Suites and Only tests with labels;
  - Combinations at once;
  - When it finishes and When a step fails (below);
  - Collect diagnostics and Machine consoles.
  Values left at their default show the default, marked "(default)", as on a plan's page.
- **Change and run…** sits beside Run again. It opens Run a plan… filled in with that execution's settings. From
  there it can be run, or kept with Save as plan….

### Two choices that say when they apply

The Options card on Run a plan… and on the plan form shows both as radio lists, each choice with what it does. The
CUE keys and values don't change (`keep`, `pauseOnFailure`).

| Label | Choices |
|---|---|
| **When it finishes** (`keep`) | **Tear down the testbed** (default): pass or fail. **Keep the testbed if something fails**: so you can look around; tear it down from the execution's page when you're done. **Always keep the testbed**: pass or fail. |
| **When a step fails** (`pauseOnFailure`) | **Stop** (default): diagnostics are collected, the rest is skipped, and the testbed is kept or torn down as When it finishes says. **Pause and wait for me**: everything stays up at the failed step; on the execution's page you can retry the step, carry on with the rest, or stop. |

- **Stop, from a pause, follows When it finishes.** It no longer always tears down. The paused card's buttons read
  **Retry step**, **Carry on** (record the failure, run the rest) and **Stop**. Stop's confirmation says whether the
  testbed will be kept or torn down.
- The same labels are used on a plan's page, in the execution's Settings, in the TUI's run form and in the help for
  the CLI's `--keep` and `--pause-on-failure` flags.

## Success Criteria

1. Every execution's page shows its Settings. For an unsaved plan they match what the form ran with, defaults
   included.
2. Change and run… opens Run a plan… with the same testbed, combinations, suites, label filter, parallel, keep, pause,
   diagnostics and consoles. Running it unchanged previews the same combinations and tests.
3. The run and plan forms show When it finishes and When a step fails as radio lists, with a description for each
   choice and the default marked. Saving a plan with the defaults writes neither key.
4. A paused combination that is stopped keeps its testbed when When it finishes says to keep it, and tears it down
   otherwise. Stop's confirmation says which.
5. No page, the TUI or the CLI help says "Wait for me before tearing anything down" or "Abort".
6. The glossary guard and the label check pass, as do `make vet lint test`, and the generated templates are up to
   date.
7. README and `design.md` describe the Settings section, Change and run… from an execution, and the two choices.

## Boundaries

Everything else is inherited from [web-ux/spec.md](../web-ux/spec.md).

- **Never:** change CUE keys or values (`keep: "never" | "failed" | "always"`, `pauseOnFailure`).
- **Ask first:** changing what Run again does.

## Out of Scope (Deferred)

- Showing the testbed's resolved values (parts and settings) as they were when the execution ran. The testbed may
  have changed since, and the Settings section links to it as it is now.
- Renaming the CUE keys to match the new labels. That would need the schema to change.
