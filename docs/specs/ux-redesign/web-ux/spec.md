# Spec: web-ux — V1

Module of [map.md](../map.md). Depends on: model-rename (implemented). Siblings: cli, tui-ux.

Status: implemented. Plan: [plan.md](plan.md); tasks: [todo.md](todo.md).

## Objective

Give the web UI the map's information architecture and words. That means:

- five tabs: Home · Executions · Plans · Testbeds · Tests;
- no config entry deeper than tab → subtab → item;
- a Run tests form;
- kept testbeds shown on their execution's page and on Home;
- Config as an indicator instead of a tab;
- only glossary words in what people read.

The approved mockup ([mockups/web/](../../../mockups/web/)) is the reference for layout and copy. Where the mockup and the map
disagree, the map wins.

**User:** as in the map: a Tinkerbell or CAPT developer checking a change, and a CI maintainer writing plans.

## Current Behavior

- **Nav:** Home · Runs · Catalog · Environments · Config. The Catalog has a group row (Plans · Build · Test),
  then subtabs, then a list, then detail.
- **Routes:**
  - pages: `/runs`, `/runs/new`, `/runs/{id}`, `/catalog/{kind id}/{name}`, `/environments`, `/config`,
    `/edit/{kind id}/{name}`;
  - API: `/api/runs…`, `/api/plan`, `/api/catalog` and `/api/catalog/events`, `/api/environments` and
    `/api/environments/{run}/{combo}/teardown`.
- **Copy:** model-rename fixed the model words. Pages still say:
  - "Start a run", "New run", "Start run", "Cancel run", "Open run", "Re-run";
  - "Kept environments", "environment kept", "No environments are up";
  - "Needs attention", "Active", "No finished runs yet";
  - "What" / "How" cards, "Copy KUBECONFIG export", and a 🔍 placeholder.
- **Kept testbeds** have their own page, `/environments`. An execution's page does not show its kept testbed.
- **Keys and palette:** keys `1`–`5` are hard-coded in [app.js](../../../../internal/ui/web/static/app.js) to the old tabs.
  The palette offers "Re-run …" and "Go to Runs/Catalog/Environments".
- **Go identifiers:**
  - local to the web: `runPage`, `runsPage`, `newRunPage`, `runData`, `envsPage`, `envCard`, `catalogPage`,
    `scenarioDetail`, `planPreview` and the `.profile` CSS class;
  - shared in [view.go](../../../../internal/ui/view.go): `KindCombo` and "run tree".

## New Behavior

### Pages and routes

| Route | Screen | Was |
|---|---|---|
| `/` | Home, or first launch | same |
| `/executions` `?status=running\|paused\|passed\|failed\|errored\|kept` `&q=` | Executions | `/runs` |
| `/executions/{n}` `?view=grid\|tree&combination=&node=` | Execution | `/runs/{id}` |
| `/executions/{n}/files/…` | an execution's files | `/runs/{id}/files/…` |
| `/run`, `/run?testbed=t`, `/run?suite=s`, `/run/{plan}` | Run tests | `/runs/new?from=…` |
| `/plans`, `/plans/{name}` | Plans | `/catalog/plans/…` |
| `/testbeds/{subtab}/{name}` | Testbeds and parts. `subtab` is a `vocab` slug: `testbeds`, `tinkerbell`, `cluster-api`, `management-cluster`, `workload-cluster`, `hardware` | `/catalog/testbeds/…`, `/catalog/clusterAPI/…` |
| `/tests/{subtab}/{name}` | `suites`, `tests`, `steps` | `/catalog/suites/…` |
| `/edit/{slug}`, `/edit/{slug}/{name}` | edit forms, by the same slugs | `/edit/{kind id}/…` |
| `/config` | Config files, no longer a tab | same |

- `/testbeds` and `/tests` open their first subtab. A subtab with no item named opens its first item, as today.
- **Old routes return 404.** There are no redirects.

| API | Does | Was |
|---|---|---|
| `GET`, `POST /api/executions` | list; start from a plan or the form | `/api/runs` |
| `GET`, `DELETE /api/executions/{n}` | one execution | `/api/runs/{id}` |
| `POST /api/executions/{n}/{action}` | `cancel`, `run-again`, `run-failed-again`, `retry`, `continue`, `abort` | `rerun`, `rerun-failed`, … |
| `GET /api/executions/{n}/events` | SSE, unchanged | |
| `POST /api/executions/{n}/{combination}/teardown` | tear down a kept testbed | `/api/environments/{run}/{combo}/teardown` |
| `GET /api/kept-testbeds` | kept and paused testbeds | `/api/environments` |
| `POST /api/preview` | the Run tests form's preview | `/api/plan` |
| `GET /api/config`, `GET /api/config/events` | the loaded config, and changes to it | `/api/catalog…` |
| `POST /api/starter`, `POST /api/reload` | unchanged | |

### Top bar, keys and palette

- **Top bar, left to right:**
  - the logo;
  - the tabs Home · Executions · Plans · Testbeds · Tests;
  - `⚠ n config errors`, linking to `/config`, shown only when there are errors;
  - `● n running`, linking to `/executions?status=running`, shown only while something runs;
  - the theme toggle;
  - **Run tests…** (primary).
- Every page marks its tab with `aria-current`. An execution page marks Executions, and an edit form marks
  Testbeds or Tests.
- **Keys:**
  - `1`–`5` go to the tabs. [app.js](../../../../internal/ui/web/static/app.js) reads their order from the top bar's links, so
    it is defined in one place;
  - `n` opens Run tests, `/` filters, `:` and ⌘K open the palette, `?` shows the keys, and `f` follows the
    running step.
- **Palette:** the same actions as today, in glossary words: Run *plan*, Open #*n*, Run #*n* again, Run tests…,
  and Go to each tab or to Config files. Each item also has hidden search aliases: run, environment, profile,
  scenario, matrix and catalog. A search matches them, but they are never shown.

### Screens

**Home**

- **Waiting for you:** one full-width banner per paused execution. It reads "#412 nightly paused after a
  failure. The testbed is kept so you can look around." and shows the failure's title. Its actions are Open
  execution · Retry step · Continue · Abort and tear down.
- **Left: Plans.** Each plan shows its name, its description, and its last execution (a chip with glyph and
  duration, linking to it) or "Not run yet". Its actions are **Run** (primary on the first plan) and Change and
  run…. The list ends with a dashed "Run tests…" row: "Pick a testbed, suites and options".
- **Right:**
  - **Running:** number, plan, combinations, a progress bar, steps done, the current step and the time left.
    When empty: "Nothing is running."
  - **Kept testbeds** (only when there are some): one line each, `412/ha-1.37`, with its plan and "kept 2 h ago".
    Each line has Copy `export KUBECONFIG` and Tear down…, plus a sentence saying they use the host's memory and
    disk.
  - **Recent:** the last five finished executions, then All executions.
- **First launch:** as today, with the new words. It shows "Welcome to Wendy", the built-in `quickstart` plan
  with Run, Create `./.wendy/wendy.cue`, and host checks.

**Executions**

- A filter, "Filter by number, plan or testbed". Both `412` and `#412` match.
- Status segments: All · Running · Waiting for you · Passed · Failed · Errored · Kept testbeds.
- Columns:
  - Execution: `#412`;
  - Plan: a link, or "—" when the tests were picked without a plan (their label is only the testbed, which the
    next column shows), with "(changed)" when the form changed a plan;
  - Result;
  - Testbed: a link, `× n` combinations, and a `kept` chip;
  - Started and Took;
  - actions: Cancel execution while running, otherwise Run again and Delete.
- Empty states: "No executions match." with Show all executions; "No executions yet." with Run tests….

**Execution**

- Crumbs: Executions › #412 nightly.
- **Summary line:**
  - the status chip, which reads "‖ paused after a failure" when paused;
  - the elapsed time;
  - "plan nightly · testbed ci · 4 combinations";
  - progress, steps done, and ✓ ✗ ! counts;
  - the time left;
  - Grid/Tree, with more than one combination;
  - actions: Cancel execution, or Run failed again (*n*) · Run again · JUnit report.
- **Failure card:** as today, with the new words. It lists "Diagnostics:" links, or "Collected so far:" while
  paused. When paused it offers Retry step · Continue (record the failure, run the rest) · Abort and tear down. An
  errored failure explains: "Errored means Wendy or the host failed, not Tinkerbell or CAPT. Running it again
  usually helps."
- **Kept testbed card**, under the summary, for each kept or paused combination. It moves here from
  `/environments`:
  - title: "Kept testbed `412/ha-1.37`";
  - clusters, Tinkerbell, machines, BMC and network;
  - Copy `export KUBECONFIG`, a download for each kubeconfig, and Tear down…. With several kept combinations, the
    card in view is open and the others fold to one line each (resolved question 1).
- **Grid and tree:** as today. Phase names come from `vocab`, and the legend says what errored means.

**Run tests** (`/run`)

- Crumbs: "Run tests", or "Plans › nightly › Run tests" when started from a plan.
- **Testbed card:** a testbed picker, with its description underneath. Under Combinations:
  - a testbed without a matrix says "One: this testbed has no matrix.";
  - a testbed with a matrix shows its axis values as chips, all on, plus a names field that takes `*` globs.
- **Tests card:** suites as checkboxes with their descriptions. The "Only tests with labels" field explains
  `&&`, `||` and `!` with an example, and lists the labels in use.
- **Options card:**
  - Combinations at once ("Each one needs its own machines.");
  - Keep testbed: Never · If it fails · Always;
  - Pause on failure: "Wait for me before tearing anything down";
  - Collect diagnostics: On failure · Always · Never;
  - Record machine consoles.
- **Preview** (right): problems first, then:
  - "*n* combinations × *m* tests", plus "*k* of *N*" when narrowed;
  - each combination;
  - Needs · Builds from source · Takes about · Host checks;
  - notes about keeping everything, pausing with parallel combinations, and disk space.

  It updates through `/api/preview` as the form changes.
- **Save bar:** "Changes apply to this execution only. Plan nightly is not changed." (or "This execution only.
  Your config files are not changed."), then Cancel and **Run**. Run is disabled while the preview shows a
  problem. Starting an execution opens its page.
- `?testbed=` and `?suite=` preselect a testbed or a suite. `/run/{plan}` starts from a plan's values.

**Plans**

- A list grouped by origin (built-in, then each file), with a filter. The detail shows:
  - the name and an origin chip, then the description;
  - Testbed: a link, plus "*n* combinations" or "one combination";
  - Suites: links;
  - Only tests with labels: the expression, or "Every test";
  - Options, as one line, e.g. "2 combinations at once · keep the testbed if it fails · always collect
    diagnostics";
  - Last execution: `#n`, status and when, or "Not run yet".
- Actions: **Run** · Change and run….
- A hint that plans are written in CUE and that "Change and run" changes one execution, not the plan. Then
  View CUE.

**Testbeds**

- **Subtabs:** Testbeds · Tinkerbell · Cluster API · Management cluster · Workload cluster · Hardware, from
  `vocab.Kinds`. Each subtab has "＋ New testbed" (or "＋ New Tinkerbell part", and so on), a filter and the kind's
  blurb.
- **Testbed detail:**
  - Parts: the kind, a link to the part and a one-line summary. An inline part reads "Defined in this testbed".
  - Settings, with human labels.
  - Network extras.
  - Matrix: each axis value → what it changes, then "*n* combinations: a, b, …". Without one: "None: one
    combination."
  - Labels.
  - Used by: the plans.
  - A **Resolved values** disclosure: management cluster, Tinkerbell, Cluster API, workload cluster, machines,
    hardware, network and setup steps. With a matrix it starts with "Before the matrix: each combination changes
    some of these."
  - Actions: **Run tests on this testbed…** · Edit testbed (or Copy and edit… for a built-in). Then View CUE.
- **Part detail:**
  - its summary;
  - the common fields, with defaults marked "(default)";
  - Machines, for hardware;
  - advanced fields only when set, otherwise "*n* advanced settings at the default";
  - Used by: the testbeds;
  - Edit *part* or Copy and edit…, then View CUE.

**Tests**

- **Subtabs:** Suites · Tests · Built-in steps, with "＋ New suite" and "＋ New test".
- **Suite:** its tests in order, with descriptions, and Used by (plans). Actions: **Run this suite…** · Edit suite
  (or Copy and edit…). Then View CUE.
- **Test:**
  - steps: each one's name or what it runs, and "times out after 15m";
  - spare machines needed;
  - labels;
  - "Runs once per testbed: later suites that list it reuse its result";
  - Used by: the suites;
  - Edit test (or Copy and edit…).
- **Built-in step:** its description, its parameters, Used by (tests), and how to use it in a test.

**Edit forms**

- The field model and the save flow are unchanged.
- **Titles:** New *testbed*, Edit *testbed* ha, or Copy of *testbed* ha, with crumbs back to the subtab. A copy of
  a built-in carries a note.
- **Save bar:** "Saved to `./.wendy/wendy-ui.cue`. Wendy checks it against the schema first." Then:
  - Delete *testbed*: disabled when something uses it, with "Used by …, so it can't be deleted.";
  - Cancel;
  - **Save *testbed***.
- The testbed form shows its matrix read-only, with "Matrices are written in CUE for now."
- **Step forms:**
  - kinds are labelled Built-in step · Command · Container · Chainsaw tests · Ginkgo test binary, each with a
    one-line description;
  - "Environment variables";
  - "Run once per testbed";
  - "Only on combinations with labels" for `when`.

**Config files** (`/config`)

- Title: "Config files".
- The watched directories, the last reload, the number of errors, and Reload now.
- Every loaded file, marked ✓ or ✗, with its errors beside it (`file:line` and the message).
- A note that the last valid config is still in use.
- It is reached from the indicator, first launch and the palette.

### Words

- Only glossary words: `vocab.Check` finds nothing in what people read.
- These come from `vocab`, never from string arithmetic:
  - kind names and plurals;
  - status words, glyphs and colour classes;
  - phase names;
  - choice labels.
- Execution IDs show as `#412`. Kept testbed addresses show as `412/<combination>`.
- Every select option shows its `vocab` label, with the CUE value in its `title`.
- Buttons are verb + object, and confirmations name what they affect.
- "Wendy" in prose, `wendy` for the command. No emoji in placeholders.

### Shared with the TUI

[fields.go](../../../../internal/ui/fields.go) labels change for both UIs: step kinds, "Environment variables", "Run once per
testbed" and "Only on combinations with labels". The TUI shows them as soon as this module ships.

[view.go](../../../../internal/ui/view.go) renames `KindCombo` → `KindCombination` and "run tree" → "execution tree".

TUI screens themselves stay with `tui-ux`.

### Go identifiers and style

- Web-local names follow the glossary, for example:
  - `runPage` → `executionPage`, `runsPage` → `executionsPage`, `newRunPage` → `runTestsPage`;
  - `runData` → `executionData`, `envCard` → `keptTestbedCard`;
  - `catalogPage` → `plansPage`, `testbedsPage` and `testsPage`;
  - `scenarioDetail` → `testDetail`, `planPreview` → `previewCard`, `.profile` → `.planrow`.
- [wendy.css](../../../../internal/ui/web/static/wendy.css) gains only the mockup's layout rules: `.banner`, `.kv`,
  `.partrow`, `.h4`, `.tree .grp`, `.disabled-why` and `h2.title`. They use the existing colour variables. There
  are no new colours or kinds of component.

## Tech Stack

Go, templ v0.3.1118 (`out/tools`), vendored htmx, and plain JS in `app.js`. No new dependencies and no JS build.

## Commands

```
make generate                      # after any .templ change
go test ./internal/ui/...          # inner loop
make vet lint test
make verify-generate               # runs git diff; ask first
go run ./cmd/wendy serve --addr 192.168.2.50:7601 --data /tmp/wendy-web-ux   # manual check; ask first
```

## Project Structure

```
internal/ui/web/
  layout.templ        shell, top bar, palette, keys help
  home.templ          Home and first launch
  executions.templ    Executions, Execution, failure and kept testbed cards, grid, tree
  run.templ           Run tests and its preview
  config.templ        Plans, Testbeds and parts, Tests, Config files
  edit.templ          edit forms (as today)
  pages.go api.go helpers.go edit.go
  static/app.js static/wendy.css
internal/ui/view.go, fields.go    shared with the TUI
```

`views.templ` is split by tab, because almost every page in it changes (resolved question 2).

## Code Style

```templ
// choice is a select option: what people read is the glossary label, and the
// CUE value is in the tooltip for those who write the config.
templ choice(path, value string, selected bool) {
	<option value={ value } title={ value } selected?={ selected }>{ vocab.Choice(path, value) }</option>
}
```

Tabs are one ordered list in Go, rendered as the top bar. `app.js` takes keys `1`–`5` from those rendered links,
not from its own list.

## Testing Strategy

- **Glossary guard.** One test renders every route against fixtures: a config with a matrix testbed, user parts
  and a plan, and executions that passed, failed with diagnostics, errored, are kept, and are paused (with a live
  PID). It covers:
  - every tab and subtab, and an item of each kind;
  - each execution in grid and tree;
  - Run tests: blank, from a plan, and for a matrix testbed;
  - each edit form, new and existing;
  - Config files with an error, and first launch.

  It runs `vocab.Check` over the visible text: text nodes plus `title`, `aria-label`, `placeholder` and
  `hx-confirm`, but not `<pre>` or code (CUE source, step ids). It also fails when a select option's label is a
  raw choice value.
- **Nav depth.** Every link to a config entry on any rendered page is `/plans/<name>`,
  `/testbeds/<subtab>/<name>` or `/tests/<subtab>/<name>`, or shallower.
- **Routes.** Each new route answers 200, and 404 for an unknown item. `/runs`, `/catalog`, `/environments` and
  `/api/runs` answer 404.
- **Per screen:**
  - Home: the Waiting for you banner and its actions; Kept testbeds.
  - Execution: the kept testbed card, with tear down posting to `/api/executions/{n}/{combination}/teardown`.
  - Executions: the Kept testbeds filter.
  - Run tests: no Matrix/Environment choice; combinations only for a matrix testbed; a plan's values prefilled;
    Run disabled on a preview problem.
  - Plans: detail.
  - Testbeds: Resolved values.
  - Top bar: the indicators appear only when they apply, and the tab links come in order.
- **Existing tests:** the web page, edit and a11y tests move to the new routes. a11y keeps every control labelled.
  TUI tests keep passing with the shared label changes.
- **No JS tests.** There is no JS toolchain on this machine. `app.js` changes are small, and are checked in a
  browser.

## Boundaries

- **Always:**
  - use words from `vocab`, and keep the glossary guard green;
  - run `make generate` after `.templ` edits;
  - keep a11y labels and the existing colour variables.
- **Ask first:**
  - a new colour or kind of component;
  - a new dependency or JS library;
  - changing service or engine behaviour (small read-only helpers for screens are fine);
  - a glossary change;
  - serving on a non-loopback address;
  - any git command, including `make verify-generate`.
- **Never:**
  - redirects or aliases for old routes;
  - a retired word in visible text;
  - a JS build step or npm;
  - changing TUI screens (`tui-ux`) or CLI commands (`cli`);
  - committing.

## Success Criteria

1. The top bar is Home · Executions · Plans · Testbeds · Tests, then the indicators (only when they apply) and
   Run tests…. Keys `1`–`5` reach the tabs in that order.
2. No config entry is deeper than tab → subtab → item. The Catalog groups, the Environments tab and the Config tab
   are gone, and old routes answer 404.
3. A kept testbed's details, kubeconfigs and tear down are on its execution's page and in Home's Kept testbeds.
   Executions has a Kept testbeds filter.
4. A paused execution shows Home's Waiting for you banner, with Retry step, Continue, and Abort and tear down.
5. Run tests has Testbed, Tests and Options cards and a live preview. Testbeds with and without a matrix run from
   it, with no Matrix/Environment choice.
6. Every select shows a human label, with the CUE value in its tooltip.
7. The glossary guard passes on every rendered page. "Run" appears only as a verb, and IDs show as `#n`.
8. Plans, Testbeds (and the five parts) and Tests (suites, tests, built-in steps) have list and detail pages as
   in the mockup, including a testbed's Resolved values.
9. Web-local identifiers and the shared ones in `internal/ui` carry no retired word.
10. `make vet lint test` pass, and the generated templates are up to date.

## Out of Scope (Deferred)

- TUI screens (`tui-ux`) and CLI commands (`cli`).
- Editing plans and matrices in forms, and "Save as plan": the map defers them.
- Left-side navigation, the tree test of the new nav, and undo instead of confirm: deferred in the map.
- JS tests and tooling: no npm on this machine.
- Mockup-only pieces: the Mockup menu, toasts that stand in for real actions, the glossary page and the demo data.

## Resolved Questions

Both proposals were accepted:

1. **Several kept testbeds in one execution:** one card per kept or paused combination. The card for the
   combination in view (the failed one, or the one picked in the tree) is open. The others fold to one line each:
   address, status and Tear down….
2. **`views.templ` is split by tab:** move first, then edit, so each step stays reviewable.
