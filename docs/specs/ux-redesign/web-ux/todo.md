# Tasks: web-ux — V1

From [plan.md](plan.md) and [spec.md](spec.md). Criteria refer to the spec's
Success Criteria, in order.

Every task ends with the checkpoint in the plan: `make generate`, `go build ./...`, `make vet test` (which includes
the TUI tests, since the TUI shares `internal/ui`) and `gofmt -l internal cmd`. From W3 on, each screen task also
empties its pages' entries in the glossary guard's allowance list.

- [x] **W1. Split `views.templ` by tab (move only)**
  - Done:
    - The five files hold the 31 components as listed. Each file imports only what it uses.
    - `templcmp.py` (session script) confirms that all 31 components, with the comment lines above them, are
      byte-identical to the backup of `views.templ`.
    - Each section was created in its new file and cut from `views.templ` with the editor, so both show in the
      chat diff.
    - Only the header was left. It was removed with `views_templ.go` using `rm`, as agreed. No hand-written Go
      file changed. The checkpoint passes.
  - Acceptance:
    - `views.templ` is gone. Its components move unchanged into five files:
      - `layout.templ`: `shell`, `topbar`, `tab`, `overlays`;
      - `home.templ`: `homePage`, `firstLaunch`;
      - `executions.templ`: `runsPage`, `resultChips`, `runPage`, `runBody`, `runHeader`, `statusChip`,
        `failCard`, `runGrid`, `runTree`, `nodeDetail`, `envsPage`, `envCard`;
      - `run.templ`: `newRunPage`, `planPreview`;
      - `config.templ`: `catalogPage`, `catalogDetail`, `planDetail`, `testbedDetail`, `usedByCell`,
        `partDetail`, `suiteDetail`, `scenarioDetail`, `builtinDetail`, `configPage`, `errLine`.
    - Each file has only the imports it uses.
    - Every component's body is byte-identical to before. No Go file other than the generated ones changes.
  - Verify: checkpoint. A throwaway script compares each `templ` component's body before and after the split.
  - Files: `internal/ui/web/views.templ` → `layout.templ`, `home.templ`, `executions.templ`, `run.templ`,
    `config.templ`, and their `*_templ.go`.

- [x] **W2. Nav, routes, palette and fixtures**
  - Done:
    - New `urls.go` holds every path, `navTabs` and the URL helpers. One `entryURL(kind, name)` replaces the
      planned `planURL`, `testbedURL` and `testURL`, so no helper sits unused. `topTabOf` names a kind's tab
      for the edit form crumbs. The edit fragments `/edit-step` and `/edit/part-summary` are constants too.
      Route patterns stay literal, like the rest of the route table.
    - The start and run-again APIs answer 201 with `Location` set to the execution's page. The palette goes
      there, so `app.js` holds no paths.
    - Subtabs mark themselves `aria-current="true"`. `page` is kept for the top-bar tab, so a page has one.
    - The action query is `?combination=`. The execution page reading `combination=` moved to W5, with the
      kept cards it opens.
    - The crawl test found that "Open full log" linked a combination's `step.log`, which the engine never
      writes. As agreed, it now shows only for a step.
    - Tests:
      - `fixture_test.go` (with a self-check);
      - `routes_test.go`: routes, the top bar, and a crawl from Home and Config files. Every link answers 200
        and no config link is deeper than tab, subtab and name.
      - The existing tests use the new URLs. An unknown subtab is now 404.
    - The checkpoint and `make lint` pass. The grep finds only `/environments`, which stays until W5.
    - `design.md` and `web.html` still show old paths; W12 handles them.
    - Served at 192.168.2.50:7601 for the browser check.
  - Depends on: W1
  - Acceptance:
    - **Top bar:** one ordered `tabs` list renders it: Home · Executions · Plans · Testbeds · Tests, then
      `⚠ n config errors` and `● n running` (each only when it applies), the theme toggle and **Run tests…**.
      Each page marks its tab with `aria-current`, and edit forms mark Testbeds or Tests.
    - **Pages:** the routes in the spec's table: `/executions`, `/executions/{n}`, `/executions/{n}/files/…`,
      `/run`, `/run/{plan}`, `/plans/{name}`, `/testbeds/{slug}/{name}`, `/tests/{suites|tests|steps}/{name}`,
      `/edit/{slug}/{name}` and `/config`.
      - Subtab slugs come from `vocab.Kinds`.
      - The catalog's group row is gone: its groups are now the Plans, Testbeds and Tests tabs.
      - The execution page's query takes `combination=`.
    - **API:** the spec's table: `/api/executions…` with actions `cancel`, `run-again`, `run-failed-again`,
      `retry`, `continue` and `abort`; `/api/executions/{n}/{combination}/teardown`; `/api/kept-testbeds`;
      `/api/preview`; `/api/config` and `/api/config/events`. `/api/starter` and `/api/reload` are unchanged.
    - **Links:** URL helpers (`executionURL`, `planURL`, `testbedURL`, `testURL`, `editURL`, `runTestsURL`) build
      every link, in templates, Go and redirects. No literal page or API path remains outside them.
    - **Old routes** `/runs…`, `/catalog…`, `/api/runs…`, `/api/plan`, `/api/catalog…` and `/api/environments`
      answer 404. `/environments` leaves the nav but still renders, linked from Home's kept list, until W5.
    - **Keys:** `app.js` takes keys `1`–`5` from the rendered tab links, and `n` opens `/run`.
    - **Palette:** items read Run *plan*, Open #*n*, Run #*n* again, Run tests…, Go to *tab* and Go to Config
      files. Each item has hidden `aliases` (run, environment, profile, scenario, matrix, catalog) that the fuzzy
      match also searches.
    - **Fixtures:** `fixture_test.go` builds a server from:
      - a config with a matrix testbed, user parts and a plan;
      - a data dir with executions that passed, failed with diagnostics, errored, are kept, and are paused (this
        process's PID).
  - Verify: checkpoint and `make lint`. Then:
    - a route test: every new route answers 200, an unknown item 404, and each old route 404;
    - a top-bar test: tab order and `aria-current`, and the indicators only when they apply;
    - a nav-depth test: every link to a config entry on any rendered page is `/plans/<name>`,
      `/testbeds/<slug>/<name>` or `/tests/<subtab>/<name>`, or shallower;
    - a grep of `internal/ui/web` for literal `/runs`, `/catalog`, `/environments` and old `/api/` paths;
    - a browser check of keys and palette, after asking about the serve address.
  - Files: `layout.templ` and the other `.templ` files (links only), `pages.go`, `api.go`, `helpers.go`,
    `edit.go`, `static/app.js`, `fixture_test.go` (new), `routes_test.go` (new), `pages_test.go`,
    `edit_test.go`, `a11y_test.go`. That is more than five files, but the change is mechanical (see the plan's
    risks).
  - Criteria: 1, and part of 2.

- [x] **W3. Glossary guard, select labels and shared words**
  - Done:
    - `guard_test.go` renders 59 pages: the fixture's tabs, subtabs and entries, each execution in grid and
      tree, Run tests three ways, and each editable kind new, copied and existing (a copy saved as `mine`).
      It also renders first launch, and Config files with a broken file.
    - Option `title`s count as code, since `choice` puts the CUE value there by design.
    - The raw-option check builds its labelled values from `ui.PartFields`, `StepFields`,
      `PlanDiagnosticsFields`, the step kinds and `keeps`, so `vocab` needed no new export.
    - The allowance comes from the first run, and every entry belongs to a later screen task. One is a false
      positive: "Runs" heading a step is a verb but matches the "Runs" heading pattern. Per the plan's risk
      note, the pattern stays. W11 heads a step with its number, as the mockup does.
    - `choice` renders every select of choice values: edit form selects, the step kind and diagnostics.
      Testbed names, the part picker and yes/no selects aren't choice values, so they keep their options.
    - The step-kind select shows only the label. Each kind's description is a hint under the step head, shown
      by the existing `data-only` CSS, as in the mockup. The TUI keeps "label: description".
    - Shared words:
      - Step-kind descriptions come from the mockup, checked against `runner.go` and `cue/wendy/steps.cue`.
      - The `when` hint now says what it does.
      - The Parameters hint no longer says "catalog". It is shared text, found by the guard.
    - `KindCombination`'s value is now `"combination"`. It never reaches HTML.
    - `vocab/choices.go` is unchanged: its step-kind labels already matched.
    - `go.mod`: offline tidy needed `charmbracelet/x/xpty`, so as agreed it ran with the network.
      - `golang.org/x/net`, `charm.land/huh/v2` and `github.com/a-h/templ` became direct (the last two were
        drift).
      - `testify` left the graph and `objx` became indirect.
      - `go.sum` gained xpty, conpty and errors.
    - The checkpoint, the TUI tests and `make lint` pass.
  - Depends on: W2
  - Acceptance:
    - **Guard:** `guard_test.go` renders every page in the spec's Testing Strategy against the fixtures. It
      collects the visible text with `golang.org/x/net/html`:
      - text nodes, plus `title`, `aria-label`, `placeholder` and `hx-confirm`;
      - skipping `<pre>`, `<code>`, `.mono`, `<script>` and `<style>`.

      It runs `vocab.Check` over that text. An allowance list, keyed by route, holds the retired terms each page
      may still show. The test fails on any term outside a page's allowance, and on an allowance entry the page
      no longer needs, so the list only shrinks. It also fails when a select option's label is a raw choice
      value.
    - **Guard self-test:** the guard catches a planted retired word, a planted raw enum label, and a retired word
      in a `title`. It ignores one inside `<pre>`.
    - **`choice` component:** renders every select option in the edit forms and the Run tests form, with the
      `vocab` label as text and the CUE value in `title`.
    - **Shared words in `internal/ui`, for both UIs:**
      - the step kinds read Built-in step · Command · Container · Chainsaw tests · Ginkgo test binary, each with
        a one-line description;
      - "Environment variables";
      - "Run once per testbed";
      - "Only on combinations with labels" for `when`.
    - **`view.go`:** `KindCombo` → `KindCombination` (gopls) and "run tree" → "execution tree".
    - **`go.mod`:** `golang.org/x/net` loses its `// indirect` marker, via `GOPROXY=off go mod tidy` so nothing
      is downloaded. If tidy needs the network, I'll stop and ask.
  - Verify: checkpoint, the guard and its self-test, and the TUI tests.
  - Files: `guard_test.go` (new), `layout.templ` (`choice`), `edit.templ`, `run.templ`, `internal/ui/fields.go`,
    `internal/ui/view.go` and their users, `internal/vocab/choices.go` (step-kind labels only, if they differ),
    `go.mod`.
  - Criteria: 6, and the start of 7.

- [x] **W4. Executions**
  - Done:
    - Each status segment carries its own match. Running and Waiting for you split unfinished executions, so
      one paused on a failure is only under Waiting for you. Kept testbeds includes paused executions, whose
      testbeds are up too, as in `KeptTestbeds`. Status labels come from `vocab`; an unknown status lists all.
    - Filter: `#41` matches numbers starting with 41. Other text matches number, plan or testbed, including a
      plan's testbed.
    - Plan shows "—" when the tests were picked without a plan, since that label is only the testbed. The
      spec says so now. A plan or testbed the config no longer has shows without a link.
    - The kept chip links to the execution, where W5 adds the cards, so this page no longer links
      `/environments`.
    - The running result reads "● 3/10 steps", as in the mockup.
    - New `waiting()` in `pages.go` replaces the paused condition in `resultChips`. `statusChip` keeps its copy
      until W5.
    - `helpers.go` needed no change.
    - `executions_test.go`:
      - every segment;
      - `2`, `#2`, plan and testbed filters;
      - each kind of row, including "(changed)";
      - both empty states.
    - The guard has no `/executions` entry left. The checkpoint and `make lint` pass.
  - Depends on: W3
  - Acceptance:
    - **Filter:** "Filter by number, plan or testbed". Both `412` and `#412` match.
    - **Status segments:** All · Running · Waiting for you · Passed · Failed · Errored · Kept testbeds.
    - **Columns:**
      - Execution: `#412`;
      - Plan: a link, or the selection's label, with "(changed)" when the form changed a plan;
      - Result;
      - Testbed: the selection's, or the plan's, as a link, with `× n` and a `kept` chip;
      - Started and Took;
      - actions: Cancel execution while running, otherwise Run again and Delete.
    - **Empty states:** "No executions match." with Show all executions; "No executions yet." with Run tests….
    - `runsPage` and `runsData` are renamed to `executionsPage` and `executionsData`.
  - Verify: checkpoint. Tests for the Kept testbeds filter, the `#` filter and both empty states. `/executions`
    has no entry left in the allowance list.
  - Files: `executions.templ`, `pages.go`, `helpers.go`, `executions_test.go` (new).
  - Criteria: part of 3.

- [x] **W5. Execution page and kept testbed cards**
  - Done:
    - Folding: a folded line's address links to `?combination=<name>` in the same view, rather than a
      `<details>`. Live pages re-render on every event, which would close a card the reader opened, and
      Tear down… can't sit in a `<summary>`. The plan says so now.
    - The card in view: `?combination=` if it names a kept one, else the combination of a node picked in the
      tree or grid, else the failed one, else the first. Grid/Tree and Follow keep `?combination=`.
    - New `service.KeptTestbedsOf(sum)`, which `KeptTestbeds` now uses, so the page loads only its own
      execution's testbeds.
    - Card rows: Clusters (as "workload cluster" with the kubeconfig path), Tinkerbell, Versions, Machines,
      BMC (its `vocab` label) and Network, each only when setup recorded it. Versions stays from the old card.
    - The guard caught "workload kubeconfig", so downloads read "workload cluster kubeconfig".
    - The summary line reuses W4's `executionRowOf` for its plan and testbed links, with "(changed)" after the
      plan. The crumbs drop "(edited)".
    - The paused failure card drops its own kubeconfig buttons, since the kept card under it has them.
    - Home's kept lines and Needs attention's Inspect open their card with `keptTestbedURL`. Home lost its
      Environments button; its other words wait for W6.
    - The legend lists the statuses, then what errored means. Grid cells' titles name their column.
    - Renames as listed, plus `comboFiles` → `combinationFiles`, element ids `run`/`run-body` →
      `execution`/`execution-body`, and stale `RunDir` comments. Removed: `envsPage`, `envCard`, `envStyle`,
      `envSince`, `teardownURL`, `sortedKeys`.
    - The fixture's kept testbeds now have clusters, kubeconfigs, a machine, a BMC and a network. #3 has an
      errored failure, and #5 a paused one with a console collected so far.
    - Tests:
      - the summary and failure cards of #2, #3 and #5;
      - one kept card;
      - two kept, folded and opened by URL, by view, by a bad name, and by picking in the grid;
      - tear down through the route;
      - `/environments` returns 404.
    - The guard has no execution page left in the allowance. The checkpoint and `make lint` pass.
  - Depends on: W4
  - Acceptance:
    - **Crumbs:** Executions › #412 *label*.
    - **Summary line:**
      - the status chip, "‖ paused after a failure" when paused;
      - elapsed time;
      - "plan p · testbed t · n combinations";
      - progress, steps done, ✓ ✗ ! counts and the time left;
      - Grid/Tree, with more than one combination;
      - actions: Cancel execution, or Run failed again (*n*) · Run again · JUnit report.
    - **Failure card** in the new words:
      - "Diagnostics:" links, or "Collected so far:" while paused;
      - when paused, Retry step · Continue (record the failure, run the rest) · Abort and tear down;
      - when errored, the explanation of errored.
    - **Kept testbed cards** (`keptTestbedCard`): one per kept or paused combination, under the summary. Each
      shows its title with the address, clusters, Tinkerbell, machines, BMC and network, Copy `export
      KUBECONFIG`, the kubeconfig downloads and Tear down…, which posts to
      `/api/executions/{n}/{combination}/teardown`. The card in view is open, and the others fold to one line:
      address, status and Tear down…. The page's `?combination=` names the card in view (moved from W2).
    - **Grid legend:** says what errored means.
    - **`/environments` is gone:** `envsPage`, its route and its template go, and Home's kept lines link to their
      execution.
    - **Renames:** `runPage` → `executionPage`, `runData` → `executionData`, `runBody`, `runHeader`, `runGrid`
      and `runTree` → `execution…`, `envCard` → `keptTestbedCard`, `comboURL` → `combinationURL`, `FailCombo` →
      `FailedCombination`.
  - Verify: checkpoint. Tests for:
    - one kept card;
    - two kept combinations, one open and one folded;
    - the teardown route;
    - `/environments` answering 404;
    - the paused summary chip.

    Execution pages have no entries left in the allowance list.
  - Files: `executions.templ`, `home.templ` (kept links), `pages.go`, `api.go`, `helpers.go`,
    `executions_test.go`.
  - Criteria: 3.

- [x] **W6. Home**
  - Done:
    - Waiting, Running and Recent come from one pass over the executions, using W4's `waiting()`, so Home's
      Running matches the Executions filter. The top bar's running count now does too, rather than counting
      in-process executions.
    - Kept testbeds lists kept, not paused, ones. A paused testbed is in its banner.
    - New `decisionAPI` and a standalone `failureTitle`, shared with the Execution page.
    - Plan names link to their plan. The last-execution chip's title names the execution and its status.
    - Tests in `home_test.go`: the banner and its actions, none without one, Kept testbeds only when kept, plans
      with "Not run yet", Running, and Recent with All executions. `/` left the allowance list.
  - Depends on: W5
  - Acceptance:
    - **Waiting for you:** one banner per paused execution: "#412 *plan* paused after a failure. The testbed is
      kept so you can look around." It shows the failure's title and Open execution · Retry step · Continue ·
      Abort and tear down.
    - **Plans (left):** name, description, and the last execution chip or "Not run yet". **Run** (primary on the
      first plan) and Change and run…. The list ends with a dashed "Run tests…" row.
    - **Right:**
      - Running: number, plan, combinations, progress, steps, the current step and the time left; "Nothing is
        running." when empty;
      - Kept testbeds, only when there are some: the host-resources sentence, then each line with Copy `export
        KUBECONFIG` and Tear down…;
      - Recent: the last five, then All executions.
    - **First launch:** "Welcome to Wendy", `quickstart` with Run, Create `./.wendy/wendy.cue`, and host checks.
    - **CSS:** `.banner` is added, using existing colour variables.
    - **Renames:** `planCard` → `planRow`, `activeRun` → `runningExecution`, `.profile` → `.planrow`.
  - Verify: checkpoint. Tests for:
    - the banner and its actions with a paused fixture, and none without one;
    - the Kept testbeds card only when something is kept;
    - "Not run yet".

    `/` and first launch have no entries left in the allowance list.
  - Files: `home.templ`, `pages.go`, `helpers.go`, `static/wendy.css`, `home_test.go` (new).
  - Criteria: 4, and part of 3.

- [x] **W7. Run tests**
  - Done:
    - Axis values are on by default, as the spec says. Turning one off narrows the combinations, together with
      the names field (both apply). A hidden `axesOf` names the testbed the checkboxes belong to, so a form
      drawn for another testbed leaves every value on. Turning a whole axis off is a problem: "No combination
      matches…".
    - "Pick at least one suite" is a problem in the form, since the preview runs `smoke` when given none.
      `formSelection` enforces it for Run too.
    - The Run button is one `runButton` component. The page renders it, and `/api/preview` returns it
      out of band (`hx-swap-oob`), disabled with a title while the preview shows a problem.
    - "Combinations at once" shows only for a testbed with a matrix, where it means something. Collect
      diagnostics is a segmented choice like Keep testbed, as in the mockup.
    - "k of N" counts only combinations that aren't excluded, and shows only when narrowed. The headline counts
      tests that run on each combination.
    - Notes cover the preview's warnings, keeping every testbed, pausing with combinations at once (a paused
      one keeps its slot, as the engine's semaphore does), and why a host check failed (which covers disk).
    - Axis values and combination names show as code, since they're names the config chose.
    - The `.runform` field rules and `.kv` come from the mockup. `.kv` arrived here rather than in W9. The unused
      `.groups` rules went.
    - Renames as listed, plus the `preview` and `keptTestbeds` API handlers (were `plan` and `environments`).
    - `run_test.go` holds the run-form tests:
      - diagnostics overrides and no suite;
      - from a plan and blank;
      - testbed with and without a matrix, `?suite=`, no matrix/environment choice, and switching testbeds;
      - narrowing by axis and name;
      - Run disabled on a problem, on the page and out of band.
    - The checkpoint and `make lint` pass, and `/run…` left the allowance list.
  - Depends on: W6
  - Acceptance:
    - **Crumbs:** Run tests, or Plans › *plan* › Run tests.
    - **Testbed card:** the picker with the testbed's description. Combinations show "One: this testbed has no
      matrix." or the axis chips (all on) and a names field taking `*` globs.
    - **Tests card:** suites with descriptions, and "Only tests with labels" with its syntax help and the labels in
      use.
    - **Options card:** Combinations at once, Keep testbed, Pause on failure, Collect diagnostics, Record machine
      consoles.
    - **Preview** through `/api/preview`:
      - problems first;
      - "*n* combinations × *m* tests", plus "*k* of *N*" when narrowed;
      - the combinations;
      - Needs · Builds from source · Takes about · Host checks;
      - notes for keeping every testbed, pausing with parallel combinations, and disk space.
    - **Save bar:** its text, Cancel and **Run**. Run is disabled while the preview has a problem, using an htmx
      out-of-band swap. Starting an execution opens its page.
    - **Query:** `?testbed=` and `?suite=` preselect a testbed or a suite. `/run/{plan}` starts from the plan.
    - **Renames:** `newRunPage` → `runTestsPage`, `newRunData` → `runTestsData`, `planPreview` → `previewCard`,
      `planFragment` → `previewFragment`, `planHeadline` → `previewHeadline`, `scenarioChain` → `testChain`.
  - Verify: checkpoint and `make lint`. Tests for:
    - no Matrix/Environment choice;
    - combinations only for a matrix testbed;
    - a plan's values prefilled, and `?suite=`;
    - Run disabled with no suite selected.

    Every `/run` variant has no entries left in the allowance list. Then a browser check, after asking.
  - Files: `run.templ`, `pages.go`, `api.go`, `helpers.go`, `run_test.go` (new; the run-form tests move here from
    `pages_test.go`).
  - Criteria: 5.

- [x] **W8. Plans**
  - Done:
    - One frame, `entriesFrame`, draws every config tab. `plansPage`, `testbedsPage` and `testsPage` fill in the
      detail. `catalogPage`, `catalogDetail` and the catalog identifiers are gone or renamed: `subtab`,
      `subtabs`, `subtabOf`, `subtabsOf`, `entryItem`, `entriesData`, `entriesTab` and `entriesPage` (gopls).
    - The Plans list has no subtabs, so its bar counts the plans.
    - The detail is as listed. Plan combinations narrowed by name show as "Only a-*". The Options line names
      only what is set (combinations at once only above one).
    - Last execution is `service.LastResult`: the newest finished execution with the plan's key, so a changed
      one doesn't count. The test adds a newer changed execution to prove it.
    - The catalog tests moved to `config_test.go`. `pages_test.go` keeps the shared `get` and `partsServer`.
  - Depends on: W7
  - Acceptance:
    - **List:** grouped by origin, with a filter.
    - **Detail:**
      - name and origin chip, then the description;
      - Testbed: a link with "*n* combinations" or "one combination";
      - Suites: links;
      - Only tests with labels, or "Every test";
      - Options as one line;
      - Last execution: the newest unchanged execution of the plan (`#n`, status, when), or "Not run yet".
    - **Actions:** **Run** · Change and run…, then the CUE hint and View CUE.
    - `plansPage` takes over from `catalogPage` for this tab.
  - Verify: checkpoint. Tests for the detail rows, the last execution, and "Not run yet". `/plans…` has no
    entries left in the allowance list.
  - Files: `config.templ`, `pages.go`, `helpers.go`, `config_test.go` (new; the catalog tests move here from
    `pages_test.go`).
  - Criteria: part of 8.

- [x] **W9. Testbeds and parts**
  - Done:
    - Testbed detail: Parts (kind, link, `partSummaryOf`, or "Defined in this testbed"); the three settings by
      their form labels and `vocab` labels; Network extras; Matrix, where each axis value says what it changes
      (a part as a link, a setting by its label) and the runnable and left-out combinations; Labels as code
      chips; Used by; Resolved values; actions; View CUE.
    - A testbed that doesn't resolve says so and links Config files, instead of Resolved values.
    - Part detail: summary; common fields with "(default)"; Machines for hardware (defaults from the form's
      columns, which match the schema); advanced fields only when set, else counted; Used by; edit or copy;
      View CUE. "Sets nothing" went, since every field now shows its value or default.
    - `partSummaryOf` labels the CNI.
    - CSS: `.tree .grp`, `.partrow`, `.partrow .kind` and `h2.title` (`.kv` came in W7).
    - Tests cover testbeds with and without a matrix, an inline part, Resolved values, and parts with defaults,
      an advanced setting and the advanced count.
  - Depends on: W8
  - Acceptance:
    - **Subtabs:** from `vocab.Kinds`, each with "＋ New *one*", a filter and the kind's blurb.
    - **Testbed detail:**
      - Parts: kind, link and summary, or "Defined in this testbed" for an inline part;
      - Settings, with human labels;
      - Network extras;
      - Matrix: each axis value → what it changes, and the combinations, or "None: one combination.";
      - Labels;
      - Used by: plans;
      - **Resolved values** as a disclosure, with a note first when there is a matrix;
      - actions: **Run tests on this testbed…** · Edit testbed (or Copy and edit…), then View CUE.
    - **Part detail:**
      - its summary;
      - common fields, with "(default)" on defaults;
      - Machines, for hardware;
      - advanced fields only when set, otherwise "*n* advanced settings at the default";
      - Used by: testbeds;
      - Edit *part* (or Copy and edit…), then View CUE.
    - **CSS:** `.kv` and `.partrow` are added.
    - `testbedsPage` takes over from `catalogPage` for this tab.
  - Verify: checkpoint. Tests for the testbed detail (with and without a matrix, inline part, Resolved values)
    and the part detail (defaults, advanced count). `/testbeds…` has no entries left in the allowance list.
  - Files: `config.templ`, `pages.go`, `helpers.go`, `static/wendy.css`, `config_test.go`.
  - Criteria: part of 8.

- [x] **W10. Tests**
  - Done:
    - Suite, test and built-in step details as listed. A test's steps each read name or built-in step (a link),
      kind, what it runs, and "times out after …". A test that runs once per testbed has a "Once per testbed"
      row, since a "Runs" heading matches the retired "Runs" heading pattern.
    - Tests keep View CUE, like every other entry. A built-in step shows its parameters schema instead.
    - New `stepUsers` lists the tests that use a built-in step.
    - `scenarioDetail` became `testDetail`. `catalogPage` is gone.
    - The guard's allowance list now holds only the edit form's "Runs", for W11.
    - The checkpoint and `make lint` pass.
  - Depends on: W9
  - Acceptance:
    - **Subtabs:** Suites · Tests · Built-in steps, with ＋ New suite and ＋ New test.
    - **Suite:** its tests in order with descriptions, and Used by (plans). Actions: **Run this suite…** · Edit
      suite (or Copy and edit…), then View CUE.
    - **Test:**
      - steps, each with what it runs and "times out after …";
      - spare machines;
      - labels;
      - the once-per-testbed sentence;
      - Used by: suites;
      - Edit test (or Copy and edit…).
    - **Built-in step:** description, parameters, Used by (tests), and how to use it.
    - **Renames:** `testsPage` takes over the last of `catalogPage`, which goes. `scenarioDetail` → `testDetail`.
  - Verify: checkpoint. Tests for each detail. `/tests…` has no entries left in the allowance list.
  - Files: `config.templ`, `pages.go`, `helpers.go`, `config_test.go`.
  - Criteria: 8.

- [x] **W11. Edit forms**
  - Done:
    - Titles, crumbs and the built-in copy note as listed. Sections use `.h4` headings, not cards, as in the
      mockup.
    - The save bar names the UI file from `service.UIFilePath`, not a fixed path, so it's right when
      `--config` points elsewhere. Delete confirms with "It is removed from …".
    - The testbed form finds its matrix through the catalog testbed it edits or copies, and shows it with
      W9's `matrixView`. A new testbed reads "None: this testbed is one combination."
    - Step numbers come from a CSS counter, so added, removed and moved steps renumber without a request.
    - The guard's allowance list is gone, and the guard fails on any retired word.
    - The checkpoint and `make lint` pass.
  - Depends on: W10
  - Acceptance:
    - **Titles:** New *one*, Edit *one* *name*, or Copy of *one* *name*, with crumbs back to the subtab. A copy
      of a built-in has a note.
    - **Save bar:** "Saved to `./.wendy/wendy-ui.cue`. Wendy checks it against the schema first." Then:
      - Delete *one*: disabled when `service.UsedBy` lists users, with "Used by …, so it can't be deleted.";
      - Cancel;
      - **Save *one***.
    - **Testbed form:** shows its matrix read-only, with "Matrices are written in CUE for now."
    - **Step forms:** show W3's kind labels with descriptions. Each step is headed by its number, not "Runs"
      (moved from W3, where the guard's allowance holds it for `/edit/tests…`).
    - **CSS:** `.h4` and `.disabled-why` are added.
  - Verify: checkpoint. Tests for the titles, the disabled Delete with its reason, and the matrix note. `/edit…`
    has no entries left in the allowance list.
  - Files: `edit.templ`, `edit.go`, `static/wendy.css`, `edit_test.go`.
  - Criteria: 6.

- [x] **W12. Config files, sweep and docs**
  - Done:
    - Config files has the mockup's title, `file:line:col` error positions, and a note when nothing is wrong
      saying where the UIs write. With errors it says the last valid config is still in use only when it is
      (`Catalog.Stale`). A cross-check error leaves the directory loaded, so it gets just "Fix the file and save".
    - The sweep (session `sweep_web.py`, which adds "catalog" and skips names other packages own) finds no
      retired word in identifiers under `internal/ui`. Renames: `ui.RunView` → `ExecutionView`, `Node.Combo` →
      `Combination`; in the web, `listRuns`, `getRun`, `startRun`, `deleteRun`, `runAction`, `runEvents` →
      `…Execution…`, `CatalogJSON` → `ConfigJSON`, `catalog` → `loadedConfig`, `catalogEvents` → `configEvents`,
      `comboCounts` → `combinationCounts`. The TUI's own locals stay for `tui-ux`.
    - README, `design.md` §11 (API, screens, keys, mockup links) and `spec.md` are updated.
      `web.html` says it is superseded by `mockup/`.
    - `make verify-generate` diffs against git, so with nothing committed it can't pass; the hash check of
      the generated templates passed instead. `make vet lint test` pass. Served at `192.168.2.50:7601`, every
      tab answers 200 and `/runs`, `/catalog`, `/environments`, `/api/runs` answer 404.
  - Depends on: W11
  - Acceptance:
    - **`/config`** is titled "Config files". It shows:
      - the watched directories, the last reload (`Catalog.LoadedAt`), the error count and Reload now;
      - every file, marked ✓ or ✗, with its errors beside it;
      - the last-valid-config note.
    - **Guard:** the allowance list is gone, and the guard passes with none.
    - **Sweep:** the session sweep script, pointed at `internal/ui/web` and `internal/ui`, finds no retired word
      in identifiers or file names.
    - **Docs:**
      - README "Editing in the UIs" and `design.md` §11 (routes, screens, keys) describe the new web UI;
      - `spec.md` is marked implemented.
  - Verify:
    - `make vet lint test`;
    - `make verify-generate` (ask first), or the hash check of the generated templates;
    - a browser check of every tab, after asking.
  - Files: `config.templ`, `pages.go`, `guard_test.go`, `README.md`, `design.md`, `spec.md`.
  - Criteria: 2, 7, 9 and 10.

### Checkpoint

`make vet lint test` pass, the generated templates are up to date, the glossary guard passes with no allowance
list, and `spec.md` is marked `Status: implemented`.
