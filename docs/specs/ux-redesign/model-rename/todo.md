# Tasks: model-rename — V1

From [plan.md](plan.md) and [spec.md](spec.md). Criteria refer to
the spec's Success Criteria, in order. Every task ends with the checkpoint in the plan: `make generate`,
`go build ./...`, `make vet test`, the starter test, and `.wendy/wendy.cue` identical to the starter.

- [x] **R1. `plan` package → `preview`**
  - Done: gopls renamed the package, and the directory was moved by hand because command-line gopls can't move
    one. Files are now `preview.go` and `preview_test.go`. Also renamed: `preview.Plan` → `preview.Preview`, engine
    `Run.Plan` → `Run.Preview`, `Service.Plan()` → `Service.Preview()` with its result's field `Preview`
    (JSON `preview`, so `wendy plan --json` prints `preview`), and the test helpers `testPreview` and `runSteps`.
    Executions write and read `preview.json`. The `.templ` source and the comments that meant the dry run were
    fixed by hand. Tests (16 packages), vet and lint pass. Web, TUI and CLI helpers named `plan…` (`planPreview`,
    `printPlan`, `planLine`) wait for their own modules.
  - Acceptance: `internal/plan` is `internal/preview`, with `preview.Preview`, `preview.Selection` and the rest
    renamed to match. An execution writes `preview.json`, and the UIs read it. `wendy plan` still works. No other
    behavior changes. Part of criterion 5.
  - Verify: checkpoint; `grep -rn '"plan.json"\|internal/plan"' --include=*.go internal cmd` finds nothing
  - Files: `internal/plan/*` → `internal/preview/*`; importers in `engine`, `service`, `ui`, `ui/web`, `ui/tui`, `cli`

- [x] **R2. Scenario → test**
  - Done: CUE `tests:` and `#Test`, and a suite's `tests: [...]`. Built-ins, starter and `.wendy/wendy.cue` are
    updated. gopls renamed `config.Test`, `Suite.Tests`, `Catalog.Tests`, `config.KindTest` (now `"tests"`),
    `preview.Test` and `Suite.Tests` (JSON `tests` in `preview.json`), `ui.KindTest` (node kind `"test"`),
    `ui.TestFields`, the engine's `skipTest` and `runSteps` (which also runs setup), the web `editData.Tests`, and the
    test helpers. Error paths, messages, JSON tags, `.templ`, `app.js` comments and the TUI help and palette were
    fixed by hand. New `vocab.KindOf`. The web Suites, Tests and Built-in steps tabs and their blurbs come from
    `vocab` (`kindTab`), as do the TUI tab labels. Routes are `/catalog/tests` and `/edit/tests`, and `wendy list tests`
    works. New tests: `TestStarterLoads` and `TestRetiredKeysAreRejected`, which later tasks extend. Tests (16
    packages), vet and lint pass. Left for their modules: the CLI's `Long` help text and the web template names
    `scenarioDetail` and `scenarioChain`. The built-in `evidence` test is still listed until R6.
  - Depends on: R1
  - Acceptance: CUE `tests:` and `#Test`, and a suite's `tests: [...]`. `config.Test`, `Catalog.Tests` and
    `config.KindTest == "tests"` (the `vocab` id). `preview` and `ui` test types are renamed. Web routes are
    `/catalog/tests` and `/edit/tests`, and the TUI tab follows `vocab`. Built-ins, starter and `.wendy/*.cue` are
    updated. `scenarios:` fails with the schema error. A new test loads `cue/starter.cue.tmpl` with no errors.
    Part of criteria 1, 6 and 7.
  - Verify: checkpoint; the starter test; an old-key test for `scenarios:`
  - Files: `cue/wendy/{schema,builtin}.cue`, `cue/starter.cue.tmpl`, `.wendy/wendy.cue`, `internal/config/*`,
    `internal/preview/*`, `internal/service/edit.go`, `internal/ui/{view,fields}.go`, web and TUI catalog and editor,
    tests

- [x] **R3. Five parts under `parts:`; management cluster split out**
  - Done: CUE `parts: {tinkerbell, clusterAPI, managementCluster, workloadCluster, hardware}` with `#ClusterAPIPart`,
    `#ManagementClusterPart` and `#WorkloadClusterPart`; built-ins in one `_parts`; the resolved shape and
    `render.cue` use `managementCluster`, `clusterAPI` and `workloadCluster` (`#Workload` → `#WorkloadCluster`).
    `Catalog.Parts(kind)` reads one map keyed by kind; `ReadSpec`/`Spec.CUE` nest parts under `parts:`; error paths
    for named parts are `parts.<kind>.<name>`. `ManagementClusterFields` holds the node image. `singular` (service
    and web) and the picker labels come from `vocab`, with a fallback for environments and matrices until R4/R5.
    `wendy list` takes the vocab slugs (`cluster-api`, `management-cluster`, `workload-cluster`). The pre-parts
    migration hints are deleted with their tests; the axis evidence hint stays until R6. New tests: the management
    split (environments and an axis), the old part keys in `TestRetiredKeysAreRejected`, "＋ New Cluster API part",
    five labelled pickers. The golden changed in the three renamed keys only. Checkpoint, lint and the starter check
    pass.
  - Deviations: web routes use the kind **ids** (`/catalog/clusterAPI`, `/edit/workloadCluster`), not slugs, so the
    web needs no slug↔id mapping; web-ux will redo routes. The TUI has no part tabs, pickers or environment editor
    today, so "five part tabs and pickers" in the TUI moves to tui-ux; R3 only fixed the TUI's environment detail
    rows (Management cluster, Cluster API, Workload cluster) and widened its label column. `TestAdvancedFields` now
    lets a one-field form (the management cluster part) fold nothing.
  - Depends on: R2
  - Acceptance: CUE `parts: {tinkerbell, clusterAPI, managementCluster, workloadCluster, hardware}`. The Cluster API
    part holds only `core` and `capt`, and the Management cluster part holds `type` and `image`. Environments pick five
    parts. The resolved shape uses `managementCluster`, `clusterAPI` and `workloadCluster`, and `render.cue`, the
    steps and `assemble`/`ownerOf` follow. `config.PartKinds` holds the five `vocab` ids in glossary order. Field
    lists: `ClusterAPIFields`, `ManagementClusterFields` and `WorkloadClusterFields`. `ReadSpec` and `Spec.CUE` nest
    parts under `parts:`. The web and TUI show five part tabs and five pickers, and the web reads
    "＋ New Cluster API part". `singular` is replaced by `vocab` lookups. `upgrade-provider` → `upgrade-cluster-api`.
    The quickstart golden is unchanged apart from renamed keys. Swapping the management cluster part leaves Cluster
    API values alone, and the other way round. Old part keys fail with the schema error. Criteria 4 and 6, part of
    1, 2 and 7.
  - Verify: checkpoint; golden; management-split and old-key tests; the web "New Cluster API part" test
  - Files: `cue/wendy/{schema,builtin,render}.cue`, starter and `.wendy/*.cue`, `internal/config/{types,resolve,load,edit}.go`,
    `internal/steps/*`, `internal/ui/fields.go`, `internal/service/edit.go`, web and TUI catalog and editors, tests

- [x] **R4. Environment → testbed, with its matrix**
  - Done: CUE `testbeds:` with an optional `matrix: {order, axes, exclude}`; axis values are a closed
    `#TestbedPatch` (the testbed's own fields), and `#ResolvedTestbed` is the resolved shape. `Catalog.Testbeds`
    holds `TestbedItem{Item[TestbedSpec], Resolved, Combinations}`: `Resolved` is the testbed without its matrix
    (shown on its pages), and a testbed without a matrix is one combination named after it. A combination is the
    spec with its axis values written over it (`combine`), then picked, assembled and evaluated like any testbed.
    `Selection{Profile, Testbed, Combinations}`, `Profile{Testbed, Combinations}`; preview JSON `combinations` and a
    combination's `testbed`. `TestbedSpec.Picks` serves `UsedBy` and `wendy list <part>`. Web: a Testbeds tab first
    in Build (from `vocab`), no Matrices tab, the testbed page shows its matrix (axes, combinations, exclusions), the
    editor notes the matrix's axes and keeps it on save, and the run form picks a testbed and offers combinations and
    axis filters only when it has a matrix. TUI: catalog tabs Profiles, Testbeds, Suites, Tests, Built-in steps (now
    named constants), and the run form picks a testbed. CLI: `-t/--testbed`, `--combination`,
    `wendy list testbeds|combinations`. The quickstart golden is unchanged. New tests: matrix equivalence (names,
    order, axes), override, two axes setting one field (pick and setting), a pick on one axis composing with a field
    patch on another, the profile's combinations without a matrix (config and preview), `environments:` and
    `matrices:` in the old-key test, evidence in an axis value, the editor keeping a matrix, the run form, the
    testbed page. Checkpoint, lint and the starter check pass.
  - Decisions: `--tag` lost its `-t` shorthand so `-t` is `--testbed` (asked; option 1). An axis's part pick applies
    before any axis's field patch, so a pick and a patch on different axes compose whatever their order; a patched
    part has no name in `Combination.Parts`. Picking another testbed than the profile's drops the profile's
    combinations. Evidence in an axis value is now the schema's "field not allowed", so `evidenceMoved` and its test
    went in R4 rather than R6. Routes use kind ids, as in R3 (`/catalog/testbeds` is also the slug).
  - Also fixed: the Management cluster catalog tab (R3) had no source view or "used by".
  - Left for later: kept testbeds (`/environments`, `svc.Environments()`, `wendy env`, the TUI Environments tab) in
    R7; "Keep environment" with `keep` in R5; the step fields labelled "Environment" mean environment variables.
  - Depends on: R3
  - Acceptance: CUE `testbeds:` with an optional `matrix: {order, axes, exclude}`, and `matrices:` is gone. Axis
    values patch the testbed as written, overriding its values for the keys they set. Two axes setting one key
    differently are an error. `Catalog.Testbeds` holds `TestbedItem{TestbedSpec, Combinations}`, and `config.Testbed`
    / `#ResolvedTestbed` is the resolved shape. `Combo` → `Combination` in `config` and `preview`.
    `Selection{Testbed, Combinations}` and the profile's `testbed` and `combinations`. `combinations` on a testbed
    without a matrix is an error naming the profile. Service `UsedBy` and reference checks work over testbeds and
    their axes. The web has no Matrices tab, the testbed page shows the matrix, the editor keeps `matrix` on save, and
    the run forms pick a testbed. CLI: `-t/--testbed` and `--combination`, and `wendy list testbeds|combinations`. The
    `versions`-style matrix test expands to the same combinations, names and labels as before. `environments:` and
    `matrices:` fail with the schema error. Criterion 3, part of 1, 6 and 7.
  - Verify: checkpoint; `make lint`; matrix-equivalence, override, two-axis-conflict and old-key tests; the editor
    keeps a matrix
  - Files: `cue/wendy/schema.cue`, starter and `.wendy/*.cue`, `internal/config/*`, `internal/preview/*`,
    `internal/service/*`, `internal/ui/web/*`, `internal/ui/tui/{form,screens}.go`, `internal/cli/{root,parts}.go`, tests

- [x] **R5. Profile → plan; keep `failed`**
  - Done: CUE `plans:` and `#Plan` (with a doc comment), `_plans: quickstart`, `keep: *"never" | "failed" |
    "always"`. `config.Plan`, `Catalog.Plans`, `KindPlan = "plans"`, `Selection.Plan` (JSON `plan`), errors at
    `plans.<name>`. The estimate key is `plan:<name>`, built once by `preview.PlanKey` (service, web and TUI used to
    spell it out). The engine keeps a failed testbed on `keep: failed`, and `vocab` labels `failed` "If it fails".
    Web: a Plans group and tab from `vocab`, `/catalog/plans`, home and the palette start plans by name (`plan`
    form field), the run form says "Keep testbed", and its preview card is titled "Preview". TUI: a Plans tab
    (`tabPlans`), home and the palette, "Keep testbed", and the form's box titled "Preview". CLI: `--plan/-p`,
    `wendy list plans`, `--keep never|failed|always`. New tests: `profiles:` in the old-key test, `keep:
    "onFailure"` rejected at the plan, the plans page and run form. `wendy plan --plan quickstart` prints the same
    combinations, steps, needs and builds as `--profile quickstart` did; only "Takes" changed, since past
    executions were recorded under the old key. Checkpoint, lint and the starter check pass.
  - Decision: the run form's preview card and the TUI form's box were titled "Plan"; they now say "Preview", so
    "plan" names one thing. Their web and TUI identifiers (`planPreview`, `planView`, …) are left to web-ux and
    tui-ux, as is the `.profile` CSS class.
  - Left for later: evidence wording and `dump: "onFailure"` (R6); "run" as a noun on the home page (R7).
  - Depends on: R4
  - Acceptance: CUE `plans:` and `#Plan`, `config.Plan`, `Catalog.Plans` and `KindPlan == "plans"`.
    `Selection.Plan`. `keep: never|failed|always`, re-keyed in `vocab` and in both run forms. The `quickstart` plan.
    `--plan` replaces `--profile`. Web and TUI show Plans in place of Profiles, routes follow, and home lists plans.
    `profiles:` and `keep: "onFailure"` fail with the schema error. Part of criteria 1, 2 and 7.
  - Verify: checkpoint; `go run ./cmd/wendy plan --plan quickstart` prints the same combinations and steps as
    `--profile quickstart` did before the task
  - Files: `cue/wendy/{schema,builtin}.cue`, starter and `.wendy/*.cue`, `internal/config/*`, `internal/preview/*`,
    `internal/vocab/choices.go`, `internal/ui/web/*`, `internal/ui/tui/*`, `internal/cli/root.go`, tests

- [x] **R6. Evidence → diagnostics**
  - Done: CUE `#Diagnostics {collect: *"onFailure" | "always" | "never", consoles: *true | bool}`, which a plan
    writes flat as `diagnostics:` and `consoles:`, and `#ResolvedTestbed.diagnostics`. The built-in `evidence`
    test is gone: the engine owns the four collect steps (`diagnosticsSteps`) and runs them as the `diagnostics`
    phase, into `<combination>/diagnostics/`, with events at `diagnostics/diagnostics/<step>`. `config.Diagnostics`
    and `DefaultDiagnostics()`, `Plan.Collects()`, `Selection.Diagnostics`, and `Failure.Diagnostics` (the steps'
    `diagnostics` output, by kind). `vocab` re-keys the phase and the `diagnostics` choice. Web and TUI run forms
    say "Collect diagnostics" (`diagnostics`, `diagnosticsSet` form fields), and the failure card "Diagnostics:".
    New tests: a failing combination runs the phase and links its files, `never` skips it, `always` runs it on a
    pass; `evidence:` in a plan and `diagnostics:` in an axis value are refused. The quickstart golden changes only
    in the `diagnostics` block. Checkpoint, lint and the starter check pass.
  - Decisions: `preview.Combination.Evidence` is gone rather than renamed, since `preview` can't import the engine's
    step list and the UI tree draws the phase from events. The diagnostic kinds (`workflow`, `console`, `bmc`,
    `dump`) keep their names; "Cluster dump" is what that file is.
  - Slip: the four `diagnostic(...)` call sites in `internal/steps/collect.go` were changed with `sed`, so they
    don't show in the chat diff.
  - Depends on: R5
  - Acceptance: a plan's `diagnostics: onFailure|always|never` and `consoles`. The resolved `diagnostics: {collect,
    consoles}`, `Selection.Diagnostics` and the run forms' override. The engine phase id and directory are
    `diagnostics`, and `Failure.Diagnostics` holds what the failure card links. The built-in `evidence` scenario is
    gone, and the engine runs its steps as the diagnostics phase. `vocab.Phase("diagnostics")` is "Collect
    diagnostics", and the choice is re-keyed. `evidence:` fails with the schema error. Part of criteria 1, 5 and 7.
  - Verify: checkpoint; an engine test where a failing combination runs the diagnostics phase into `diagnostics/`
  - Files: `cue/wendy/{schema,builtin}.cue`, starter and `.wendy/*.cue`, `internal/config/types.go`,
    `internal/preview/*`, `internal/engine/{run,summary}.go`, `internal/steps/{collect,hardware}.go`, `internal/vocab/*`,
    `internal/ui/*`, tests

- [x] **R7. Run (noun) → execution; combo → combination in executions**
  - Done: executions are numbered (`1`, `2`, … `412`) in `<data>/executions/<n>` (`engine.ExecutionsDir`), with
    `execution.json` as the summary. `engine.CompareIDs` orders IDs by number for the list and `service.Active`,
    and the next-ID scan reads plain numbers. Events carry `execution` and `combination`, and their types are
    `execution.started|finished` and `combination.started|finished|paused|resumed`. The summary has `combinations`
    (`CombinationSummary`). `state.json` has `execution`, `combination` and `workloadCluster`, and a testbed's
    resolved config is `testbed.json`. Go: `engine.Run` → `Execution`, `engine.Env` → `Testbed` (`NewTestbed`,
    `LoadTestbed`, `Step.Testbed`, locals `tb`), `WorkloadState` → `WorkloadClusterState`, the cluster constants
    → `ManagementCluster`, `TinkerbellCluster` and `WorkloadCluster`. `service`: `Executions`, `Execution`,
    `ExecutionDir`, `RunAgain`, `KeptTestbed(s)` (JSON `execution`, `combination`), `Change.Execution`. The engine,
    service, report, runner, event and steps lose `run`, `combo` and `environment` from identifiers and comments.
    IDs show as `#412` in the web, TUI and plain CLI output and in service errors. A kept testbed's address stays
    bare, `412/<combination>`, since it is what `--env` takes. New tests: IDs past 9 are allocated and listed by
    number, the event log's JSON names, and the JUnit names. Checkpoint, lint and the starter check pass.
  - Decisions: the renames were scripted with gopls (`rename`, with `definition` to check a local's type before
    renaming it), so `config.Step.Env` (environment variables) and the runner's env-var maps kept their names; the
    same pass renamed `env` parameters in `internal/provider/*`. The TUI executions header said "PROFILE /
    SELECTION", missed in R5; it now says "PLAN / SELECTION". A skipped test's reason is "the combination
    stopped" or "an earlier step failed", not the retired "not run".
  - Left for later: the kept-testbed screens (`/environments`, the Environments tab and its wording, `wendy env`,
    `--env`) and "run" as a noun in UI text and routes (`/runs`, Runs, Re-run, `runPage`) belong to `web-ux`,
    `tui-ux` and `cli`, as the spec's surfaces section says; R4's note had put them here. For R8's sweep: the
    built-in step ids `env.*` and `workload.cni`, `#Render`'s `workload` field, the files `engine/env.go`,
    `steps/env.go` and `steps/workload.go`, the step docs' "workload node/pod", and `provider/network`'s
    `wendy.run`/`wendy.combo` labels.
  - Depends on: R6
  - Acceptance: IDs are numbers in `<data>/executions/<n>`, and lists and the next-ID scan sort numerically. Events
    have fields `execution` and `combination`, and types `execution.*` and `combination.*`. Summary `combinations`,
    `ComboSummary` → `CombinationSummary`, JUnit suites per combination, and `state.json` `execution`, `combination`
    and `workloadCluster`. Go nouns in `engine`, `event`, `service`, `runner` and `report` are renamed (e.g.
    `ListRuns` → `ListExecutions`, `RunStarted` → `ExecutionStarted`). Kept testbeds are addressed as
    `<n>/<combination>`. Criterion 5, part of 7.
  - Verify: checkpoint; a test that allocates IDs past 9 and lists them in order; event and JUnit tests on the new
    names
  - Files: `internal/event/*`, `internal/engine/*`, `internal/service/*`, `internal/runner/*`, `internal/report/*`,
    `internal/cli/plain.go`, readers in `internal/ui/*`, tests

- [x] **R8. Docs, glossary and sweep**
  - Done: README rewritten for the new model, with the glossary, a parts table, a matrix in a testbed, plans and an
    execution's directory layout; its CUE examples load and expand as described. `design.md`: summary,
    concepts, architecture, package layout, storage, the abridged schema, the example (it loads), the execution
    model, the built-in steps, §10 `state.json` key by key, the event shape and types, and the UI and CLI sections.
    The starter comments already spoke the new model (R3–R6), and `.wendy/*.cue` was already in its shape. The sweep
    (a script over Go identifiers, JSON tags, CUE and file names in the owned packages) renamed: the built-in step
    ids `env.*` → `testbed.*` and `workload.cni` → `workloadCluster.cni`; `#Render`'s `env` → `testbed` and
    `workload` → `workloadCluster`; `engine/env.go` → `testbed.go`, `steps/env.go` → `testbed.go`,
    `steps/workload.go` → `workloadcluster.go` (moved with `mv`); the steps' `env*`/`workload*` functions; the
    step docs that said "workload node/pod"; and `provider/network`'s labels → `wendy.execution` and
    `wendy.combination`. What it still finds are third-party keys in `render.cue`: the Tinkerbell chart's `envs`,
    Rufio's `providerOptions`, clusterctl's `providers` and Tinkerbell actions' `environment`. The `env` subcommands'
    arguments read `<execution>/<combination>`. `make vet lint test` and the starter check pass.
  - Decision: `make verify-generate` was skipped (asked): it diffs against the git index, and nothing here is
    committed. Instead, every `*_templ.go` is byte-identical after `make generate`.
  - Depends on: R7
  - Acceptance: README rewritten for the new model, including the glossary. `design.md` (concepts, schema,
    §10 BYO `state.json`) and the starter comments are updated, and `.wendy/*.cue` is rewritten. A sweep finds no
    retired word in CUE, JSON tags, file and directory names, or Go identifiers of `config`, `preview`, `engine`,
    `service`, `runner`, `event`, `report`, `steps` and `vocab`. `spec.md` is marked implemented.
    Criteria 7, 8 and 9.
  - Verify: `make vet lint test`; `make verify-generate` (ask first); a retired-word grep over those packages and the CUE
  - Files: `README.md`, `design.md`, `cue/starter.cue.tmpl`, `.wendy/*.cue`, `spec.md`, and any
    leftovers the sweep finds

### Checkpoint
`make vet lint test verify-generate` pass, the starter loads, and `spec.md` is marked
`Status: implemented`.
