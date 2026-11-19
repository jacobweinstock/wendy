# Plan: execution-settings — V1

From [spec.md](spec.md). Tasks: [todo.md](todo.md).

## Components

1. **Record what ran.**
   - `preview.Normalize(sel)` fills in every default: the smoke suite, one combination at a time, tear down, stop,
     and the default diagnostics.
   - `Preview.Effective` is the selection applied over its plan, normalized. `New` takes parallel, keep, pause, suites
     and diagnostics from it instead of defaulting each one on its own.
   - `engine.Summary.Effective` copies it into `execution.json`, so the API returns it too.
   - An execution recorded before this has no `Effective`. For an unsaved plan, `Normalize(Selection)` is exact. For
     a saved plan, the section shows the plan and says its other settings weren't recorded.
2. **Words.** `vocab` gives `keep` new labels and descriptions, and adds `pauseOnFailure` (`false` Stop, `true`
   Pause and wait for me). Every surface reads them from there: the forms, a plan's page (`optionsLine`), the
   execution's Settings, the TUI and the CLI help.
3. **Run and plan forms.** The Options card's Keep testbed segment and Pause checkbox become two radio lists.
   Each list has explicit values, since a blank would mean "the plan's" on the run form. The form still posts
   `keep`, `pauseSet` and `pauseOnFailure` (`on`/`off`), so `selection()` doesn't change.
4. **Execution page.**
   - A Settings section under the header, a `<details>` with `hx-preserve`, so a live page's refreshes don't close
     it.
   - **Change and run…** beside Run again. It links to `/run?…`, with the query the run form itself posts
     (`suitesSet`, `pauseSet`, `diagnosticsSet` and the values), which `runForm` already renders. A plan that no
     longer exists is left out, so the form starts blank.
5. **Stop.**
   - In the engine, a stopped combination no longer forces teardown (`if cx.aborted { keep = false }` goes). Its
     verdict is failing, so "Keep the testbed if something fails" keeps it.
   - One `decisionButtons` templ serves Home's banner and the execution's failure card: Retry step, Carry on, and
     Stop, whose confirmation says whether the testbed is kept.
   - The API's decision names (`retry`, `continue`, `abort`) don't change; people don't read them.

## Order

Record (preview, engine) → words → forms → execution page and Stop → TUI and CLI → docs.

## Risks

- **Old executions** have no `Effective`. The fallback above covers them, and a test checks an unsaved one.
- **Wording asserted in tests** (Abort and tear down, the plan's options line) is updated to the new words, not
  deleted.
