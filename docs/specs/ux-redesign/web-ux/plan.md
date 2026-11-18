# Plan: web-ux — V1

From [spec.md](spec.md) (approved). Tasks: [todo.md](todo.md).

## Approach

The work goes in four stages. After each task the build, vet and full test suite pass, so the work can stop
after any task:

1. Split `views.templ` by tab, as a move only.
2. Switch the routes and the top bar in one pass, so every link agrees.
3. Rework one screen at a time, starting with the screens the others link to.
4. Finish with a sweep.

The glossary guard comes early as a ratchet. It renders every page, and a list holds the retired words each
page is still allowed to show. Each screen task empties its page's entries, and the last task deletes the list.
So every task shows its progress in the test, and no task can bring a retired word back.

**Identifiers.** Go identifiers of a screen are renamed in that screen's task with `~/go/bin/gopls rename`, which
also updates `*_templ.go`. Each rename is followed by fixing `.templ` sources, strings and comments by hand,
then `make generate` and a build. The last task sweeps whatever is left.

## Components and decisions

1. **The split (W1).**
   - `views.templ` becomes `layout.templ`, `home.templ`, `executions.templ`, `run.templ` and `config.templ`.
   - The move uses the editor tools, creating each new file and cutting it from `views.templ`, so it shows in
     the chat diff.
   - A throwaway script checks that the set of `templ` components and their bodies is unchanged.
2. **Routes in one place (W2).**
   - URL helpers (`executionURL`, `planURL`, `testbedURL`, `testURL`, `editURL`, `runTestsURL`) build every link.
     Templates, the palette and redirects use only them.
   - Subtab slugs come from `vocab.Kinds`.
   - The top bar comes from one ordered `tabs` list.
   - The catalog's group row disappears: its three groups become the Plans, Testbeds and Tests tabs.
   - `app.js` reads the keys `1`–`5` from the rendered tab links, and its own tab list goes away.
   - Palette items get a hidden `aliases` field, which the palette's fuzzy match also searches.
3. **`/environments` lives until W5.** W2 takes it out of the nav, but it stays reachable from Home's kept list
   until W5 moves its card onto the execution page. W5 then deletes the page and its route.
4. **Test fixtures (W2).** `fixture_test.go` builds a server with:
   - a config: a testbed with a matrix, user parts, and a plan;
   - a data dir with executions that passed, failed with diagnostics, errored, are kept, and are paused.

   Each execution is written with `engine.WriteSummary` and a few events. The paused execution carries this
   process's PID, so `ReadSummary` doesn't mark it errored. Route, guard and screen tests share it.
5. **Glossary guard (W3).**
   - **Decision (accepted):** use `golang.org/x/net/html` to tokenize pages. It is already in `go.mod` at v0.62.0
     as an indirect dependency. Using it in a test only drops its `// indirect` marker, with no new module or
     version. Visible text is text nodes plus `title`, `aria-label`, `placeholder` and `hx-confirm`. It skips
     `<pre>`, `<code>`, `.mono`, `<script>` and `<style>`.
   - The guard also fails on a select option whose label is a raw choice value.
6. **Selects (W3).** One `choice` component renders every option, with the `vocab` label as text and the CUE
   value in `title`. Edit forms and the Run tests form both use it.
7. **Shared labels (W3).** [fields.go](../../../../internal/ui/fields.go) gets the step-kind labels with one-line
   descriptions, "Environment variables", "Run once per testbed" and "Only on combinations with labels".
   [view.go](../../../../internal/ui/view.go) gets `KindCombination` and "execution tree". TUI tests are run to confirm
   they still pass.
8. **Kept testbed cards (W5).**
   - One component, `keptTestbedCard`, shows one combination: its clusters, Tinkerbell, machines, BMC, network,
     the kubeconfig actions and Tear down.
   - The execution page shows one card per kept or paused combination. The card in view is open and the others
     fold to one line, whose address links to `?combination=` to open it. (Not `<details>`: the live page
     re-renders on every event, which would close a card the reader opened, and Tear down… can't sit inside
     a `<summary>`.)
   - Home uses a one-line form of the same data.
9. **Run disabled on a problem (W7).** The preview fragment carries the Run button as an htmx out-of-band swap,
   so it is enabled exactly when the preview has no problem. htmx is already vendored.
10. **Resolved values (W9).** The rows the testbed page shows today (clusters, Cluster API, machines, setup)
    move under a `<details>` named "Resolved values". Nothing new is resolved.
11. **CSS.** Each screen task adds the mockup layout rules it needs (`.banner` in W6, `.kv` and `.partrow` in
    W8–W9, `.h4` and `.disabled-why` in W11). All of them use existing colour variables.

## Order

W1 split → W2 nav, routes and fixtures → W3 guard and shared words → W4 Executions → W5 Execution and kept
testbeds → W6 Home → W7 Run tests → W8 Plans → W9 Testbeds → W10 Tests → W11 edit forms → W12 Config files,
sweep and docs.

All of it is sequential, since every task edits the files the split creates:

- W4 and W5 come before Home, because Home reuses their status chips and kept-testbed line.
- Plans, Testbeds and Tests come after Run tests, because their actions link to it.

## Risks

- **W2 touches many files.** The route change is mechanical, but links live in templates, Go, `app.js` and
  tests. Mitigations:
  - the URL helpers;
  - a grep for literal `/runs`, `/catalog`, `/environments` and `/api/` paths before W2 ends;
  - the route test, which requires 404 for the old paths.
- **Guard false positives.** Examples: "Run failed again", or a step id such as `env` inside prose. The
  retired patterns in `vocab` are already tested. Any new false positive is fixed in the page's wording, not by
  weakening `vocab.Check`, unless the pattern is wrong. Changing a pattern is a glossary change, so I ask first.
- **Paused and running fixtures** are only summaries, so no in-process execution answers Retry or Continue.
  Rendering tests are fine with that. Action tests keep using real engine executions with test steps, as the
  engine tests do.
- **`app.js` has no tests.** Its changes are small (keys from the nav, palette aliases). They are checked in a
  browser on the user's LAN address, after asking first.
- **The TUI shares `fields.go`.** Label changes appear in the TUI right away. Its tests run in every checkpoint.
- **Docs** still describe the old routes (`design.md` §11 says web-ux renames them, and the README
  mentions the UI). W12 updates both.

## Checkpoints

- **After every task:** `make generate`, `go build ./...`, `make vet test` and `gofmt -l`.
- **After W2, W7 and W12:** `make lint`, plus a browser check if you agree to the serve address.
- **After W12:** `make verify-generate` (runs `git diff`; ask first, or use the hash check as in model-rename),
  then the spec is marked implemented.
