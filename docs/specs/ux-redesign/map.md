# Capability Map: UX Redesign — V1

Status: approved. Module specs follow in build order: vocabulary → model-rename → cli, web-ux, tui-ux.

## Objective

Make wendy usable without having to learn it first, and keep it visually minimal. Two parts:

1. **Vocabulary.** One plain word for each concept, the same in the web UI, TUI, CLI, CUE, errors and docs.
2. **Information architecture.** Fewer places to look, shallower navigation, one obvious next action on each screen.

**User:** a Tinkerbell or CAPT developer who opens wendy to check a change, and a CI maintainer who writes
plans. Both know Kubernetes, Cluster API and Tinkerbell. We can't assume they know test-framework tools such as
Ginkgo or Chainsaw. wendy's words should match the tools they know (Jakob's law), and must not reuse words those
tools already use for something else.

**Compatibility:** none is needed, so nothing is off-limits. This design does change CUE keys, CLI commands and
flags, URLs, event paths, data directory names and Go identifiers wherever they carry a retired term. Anything this
document doesn't name stays as it is.

## Why: what is wrong today

| # | Problem | Where |
|---|---|---|
| 1 | **"Environment" means three things:** a catalog recipe (`environments:`), a live set of clusters (the Environments tab, `wendy env`, "Keep environment"), and env vars (the step form's "Environment" field). `--env` takes a recipe name on `wendy run` but `<run>/<combo>` on `wendy step run`. | [views.templ](../../../internal/ui/web/views.templ), [fields.go](../../../internal/ui/fields.go), [root.go](../../../internal/cli/root.go) |
| 2 | **"Build" means two things:** a catalog group (Tinkerbell, Providers…) and the phase that builds images from source. "Build from repo" is a third use. | [pages.go](../../../internal/ui/web/pages.go), [view.go](../../../internal/ui/view.go) |
| 3 | **"Evidence" means three things:** a profile setting, a built-in scenario, and the failure card's links. The same kind of files are called "Artifacts" on a step. | [builtin.cue](../../../cue/wendy/builtin.cue), [views.templ](../../../internal/ui/web/views.templ) |
| 4 | **"Labels" means five things:** test labels, step labels, combination labels (shown as "Axes"), machine labels, and the label filter, which is described only by pointing at Ginkgo's syntax. | [views.templ](../../../internal/ui/web/views.templ), [schema.cue](../../../cue/wendy/schema.cue) |
| 5 | **Too many synonyms for one idea:** combo / combination; part / inline; scenario = "one test"; profile = "a saved run"; "skipped" vs "(not run)". | across UIs |
| 6 | **Jargon and internal names:** "Providers" (the libvirt/BMC layer is also called a provider), "Workload" on its own (a Kubernetes "workload" is a pod or deployment), "Target", "Once", raw enum values like `onFailure` and `colocated` shown as labels. | [fields.go](../../../internal/ui/fields.go), [views.templ](../../../internal/ui/web/views.templ) |
| 7 | **Deep navigation:** the web UI's Catalog has a tab, then a group row, then subtabs, then a list, then detail, which is four levels before content. "Catalog" and "Config" are generic labels that don't say what's behind them. | [views.templ](../../../internal/ui/web/views.templ) `catalogPage` |
| 8 | **Two ways to say "what to test":** the run form makes you choose "Target: Matrix \| Environment" first. A matrix is an environment with axes. | [views.templ](../../../internal/ui/web/views.templ) `newRunPage` |
| 9 | **The web UI and TUI disagree:** the TUI catalog has Profiles, Environments, Matrices, Suites, Scenarios and Steps, with no parts. The web UI has three groups and ten tabs. | [screens.go](../../../internal/ui/tui/screens.go), [pages.go](../../../internal/ui/web/pages.go) |
| 10 | **Labels built from identifiers:** `singular("providers")` gives "＋ New provider" for a Providers part. | [helpers.go](../../../internal/ui/web/helpers.go) |
| 11 | **"Run" is both the noun and the verb:** "run the run", "which run is running?", "Re-run", "Cancel run", "Run this environment…". A profile is defined as "a saved run", so a definition is described as one of its own executions. | [views.templ](../../../internal/ui/web/views.templ), [pages.go](../../../internal/ui/web/pages.go) |
| 12 | **The management cluster is set inside the Providers part** (`management.image`), so changing its node image means a new Cluster API/CAPT part, and the other way round. | [fields.go](../../../internal/ui/fields.go) `ProvidersFields` |

## Principles (with sources)

These are the rules every module applies. Every source below was fetched and checked:

1. **One word per concept: no synonyms, no homonyms.** Keep a glossary. ([Shopify Polaris content](https://shopify.dev/docs/apps/design/content), [Microsoft: don't use common words in new ways](https://learn.microsoft.com/en-us/style-guide/word-choice/dont-use-common-words-in-new-ways), [Apple HIG writing](https://developer.apple.com/design/human-interface-guidelines/writing))
2. **Use the users' words, not internal ones.** People pick the same term less than 20% of the time, so don't trust the designer's favourite. Test names, and accept aliases in search. ([NN/g heuristic #2](https://www.nngroup.com/articles/ten-usability-heuristics/), Furnas et al. 1987, *The vocabulary problem*, CACM 30(11))
3. **Navigation labels are specific nouns with strong information scent.** Avoid generic labels like "Catalog" and "Settings". ([NN/g menu design](https://www.nngroup.com/articles/menu-design/), [NN/g information scent](https://www.nngroup.com/articles/information-scent/))
4. **Minimalism and progressive disclosure.** Show the few common choices first and fold the rest away. ([NN/g heuristic #8](https://www.nngroup.com/articles/aesthetic-minimalist-design/), [NN/g progressive disclosure](https://www.nngroup.com/articles/progressive-disclosure/), [Hick's law](https://lawsofux.com/hicks-law/))
5. **Recognition over recall.** Use pick lists over typed names, and show human labels, not enum identifiers. ([NN/g](https://www.nngroup.com/articles/recognition-and-recall/))
6. **Each screen has one primary action, labelled verb + object.** ([GOV.UK button](https://design-system.service.gov.uk/components/button/), [NN/g OK/Cancel](https://www.nngroup.com/articles/ok-cancel-or-cancel-ok/))
7. **Status is shown as glyph + word + colour, with the same words everywhere.** ([WCAG 1.4.1](https://www.w3.org/WAI/WCAG22/Understanding/use-of-color.html), [Carbon status indicators](https://www.carbondesignsystem.com/building-blocks/core/patterns/status-indicators))
8. **Summary first, then detail.** Put the key word at the left edge of each line. ([NN/g dashboards](https://www.nngroup.com/articles/dashboards-preattentive/), [NN/g F-pattern](https://www.nngroup.com/articles/f-shaped-pattern-reading-web-content-discovered/))
9. **Errors say what happened and how to fix it, next to the cause.** ([GOV.UK error message](https://design-system.service.gov.uk/components/error-message/), [NN/g error messages](https://www.nngroup.com/articles/error-message-guidelines/))
10. **Empty states give one next action.** ([NN/g empty states](https://www.nngroup.com/articles/empty-state-interface-design/), [Carbon empty states](https://carbondesignsystem.com/patterns/empty-states-pattern/))
11. **Confirmations name what will be affected.** ([NN/g confirmation dialogs](https://www.nngroup.com/articles/confirmation-dialog/))
12. **The CLI uses the same nouns as the UI, with no similar-sounding commands, and consistent verbs and flags.** ([clig.dev](https://clig.dev/), [Heroku CLI style guide](https://devcenter.heroku.com/articles/cli-style-guide))

## Glossary (target vocabulary)

This is the single source of truth. Every user-facing string uses these words and nothing else for these concepts.
It goes into the README and into one Go file that both UIs read (see `vocabulary`).

```
Parts ──▶ Testbed (+ optional matrix ▶ combinations) ─┐
                                                      ├─▶ Plan ──run──▶ Execution ─▶ (kept testbed)
          Steps ▶ Tests ▶ Suites ─────────────────────┘
```

**Nouns and the verb are never the same word.** *Run* is only a verb, and always has an object: "Run nightly",
"Run tests". The thing it creates is an *execution*. The status of an execution in progress is *running*.

| Term | Meaning | Replaces |
|---|---|---|
| **Plan** | What to test, where and how: the suites, the testbed, and options such as how many combinations go at once and whether to keep the testbed. You run a plan: `wendy run nightly`. | Profile |
| **Execution** | One time wendy carries out a plan: it builds each combination's testbed, runs the suites, collects diagnostics and tears down. Has a number (`#412`), a status and a verdict. | Run (noun) |
| **Preview** | What a plan would do, shown before it runs: combinations, what it needs, what it builds, how long it takes, host checks. | `wendy plan`, the "Plan" pane |
| **Testbed** | What tests run on: one part of each kind, plus the settings no part owns (topology, boot mode, network), plus an optional matrix. | Environment (recipe) |
| **Part** | A reusable piece of a testbed, changed on its own. There are five kinds: |  |
| · **Tinkerbell** | How Tinkerbell is installed: chart or source build, namespace, values. | Tinkerbell part |
| · **Cluster API** | Cluster API core and CAPT, as releases or source builds. Nothing about any cluster. | Providers part |
| · **Management cluster** | The kind cluster that hosts Cluster API and CAPT, and Tinkerbell when it runs on the management cluster: its node image (and so its Kubernetes version). | `management.image` in the Providers part |
| · **Workload cluster** | The cluster CAPT deploys and the tests act on: Kubernetes version, CNI, OS image. | Workload part |
| · **Hardware** | The machines: architecture, counts and sizes, BMC, libvirt. | Hardware part |
| **Matrix** | A testbed's axes. Each value patches the testbed or picks a part. | Matrix (as a separate catalog kind) |
| **Axis** | One dimension of a matrix, such as `k8s` or `bootMode`. | — |
| **Combination** | One point in the matrix: one concrete testbed. A testbed without a matrix has one combination. | combo |
| **Kept testbed** | A built testbed an execution left up, after a failure or when asked. It is addressed as `<execution>/<combination>`, e.g. `412/ha-1.37`. | Environment (live), kept environment |
| **Suite** | Tests run in order on one testbed. | — |
| **Test** | A named, ordered list of steps, such as `deploy` or `scale-out`. | Scenario |
| **Step** | One action in a test, timed and reported. It is one of: built-in, command, container, Chainsaw tests, or a Ginkgo test binary. The last two are named for the tool they run, and their forms say what that is in one line. | — |
| **Built-in step** | A step compiled into wendy (`uses: "capt.scale"`). | Step (catalog tab) |
| **Label** | A tag on a test or combination. A **label filter** is an expression that picks by label, e.g. `scale && !upgrade`. The form explains `&&`, `\|\|` and `!` next to the field. | Axes (as shown), "Ginkgo label filter" |
| **Diagnostics** | What wendy collects to debug a failure: cluster dump, Tinkerbell objects, BMC logs, machine consoles. | Evidence |
| **Artifacts** | Any file an execution saved: step output, JUnit report, diagnostics. | Evidence links, Files |
| **Config** | The CUE files wendy loads. | Catalog (as a place) |
| Statuses | passed ✓ · failed ✗ (Tinkerbell or CAPT) · errored ! (wendy or the host) · timed out · cancelled ⊘ · skipped – · running ● · waiting ○ · paused ‖ | "not run", "never run" |

Execution phases, as shown in the execution tree and grid: **Build images** → **Set up testbed** → *suites* →
**Collect diagnostics** → **Tear down**.

### Words we must not use for wendy concepts

These already mean something in Tinkerbell, Cluster API or Kubernetes: **Workflow, Template, Action, Task, Job,
Machine** (a CAPI/Rufio object), **Provider** (a clusterctl provider; wendy's libvirt/BMC layer), **Workload** on
its own (always "workload cluster"), and **Environment** (env vars only, always written "environment variables").
**Run** is never a noun.

## Vocabulary changes (audit)

| Where | Today | Proposed | Why |
|---|---|---|---|
| CUE, UI | `environments:` / Environment | `testbeds:` / Testbed | removes the homonym (#1) |
| CUE, UI | `profiles:` / Profile | `plans:` / Plan | says what it is (what to test, where, how) without leaning on the execution. "Test plan" is the common testing term. "Profile" suggests a user profile or profiling |
| UI, CLI, data | Run (noun) / Runs tab / `r-0412` | Execution / Executions tab / `#412` | #11: run stays a verb only |
| CLI, UI | `wendy plan`, "Plan (updates as you edit)" | `wendy preview`, "Preview" | frees "plan" for the definition |
| CUE, UI | `scenarios:` / Scenario | `tests:` / Test | the blurb already says "one test". Suite → test → step is the universal hierarchy |
| CUE, UI | `matrices:` (own kind, `base:`, `suites:`) | `matrix:` field on a testbed, no `base`, no `suites` | one way to say "what to test" (#8). Suites belong to plans |
| CUE | `tinkerbells:` `providers:` `hardware:` `workloads:` | `parts: {tinkerbell:, clusterAPI:, managementCluster:, workloadCluster:, hardware:}` | names what each configures, avoids awkward plurals, mirrors "Parts" in the UI |
| Testbed | `tinkerbell:` `providers:` `hardware:` `workload:` | `tinkerbell:` `clusterAPI:` `managementCluster:` `workloadCluster:` `hardware:` | matches the part kinds |
| Part | `management.image` in the Providers part | `image` in a Management cluster part | #12: the management cluster changes on its own |
| Plan | `combos:` | `combinations:` | one term (#5) |
| Plan, execution form | `keep: never\|onFailure\|always` | `keep: never\|failed\|always`, shown as "Never · If it fails · Always" | human labels (principle 5) |
| Plan, execution form | `evidence: {consoles, dump}`, "Dump evidence" | `diagnostics: onFailure\|always\|never`, `consoles: bool`, shown as "Collect diagnostics" and "Record machine consoles" | #3 |
| Plan, execution form | `labelFilter`, "Ginkgo label filter" | same key, shown as "Only tests with labels", with the expression syntax explained beside the field | users aren't assumed to know Ginkgo |
| Built-in test | `evidence` | internal phase "Collect diagnostics", not a test | it is a phase, not a test the user picks |
| Test | `once: true`, "Once per environment" | same key, shown as "Run once per testbed" | |
| Step form | "Environment" (exec/container env) | "Environment variables" | #1 |
| Step form | kinds `uses/exec/container/chainsaw/ginkgo` | shown as Built-in · Command · Container · Chainsaw tests · Ginkgo test binary, each with a one-line description. CUE keys are unchanged | plain words, tool names explained |
| Testbed form | `topology: colocated\|external` | shown as "Tinkerbell runs on: the management cluster · its own cluster" | enum shown as meaning |
| Testbed detail | "Axes" row (combination labels) | "Labels" | #4 |
| Testbed detail | "Parts … inline" | "Defined in this testbed" | plain words |
| Hardware | machine `labels` | "Hardware labels" (on the Tinkerbell Hardware object) | #4 |
| Web nav | Home · Runs · Catalog · Environments · Config | Home · Executions · Plans · Testbeds · Tests | see IA below |
| Catalog groups | Profiles · Build · Test | removed | #2, #7 |
| Execution tree | Build, Setup, evidence, teardown | Build images, Set up testbed, Collect diagnostics, Tear down | #2, #3 |
| Execution form | "New run", Target: Matrix \| Environment, then What / How | "Run tests", then Testbed, Tests, Options | #8, #11 |
| Execution form | "Parallel", "Keep environment", "Dump evidence", "Machine consoles", "Start run" | "Combinations at once", "Keep testbed", "Collect diagnostics", "Record machine consoles", "Run" | |
| Preview | Needs / Builds / Takes / Checks | Needs / Builds from source / Takes about / Host checks | |
| Home | "Start a run", "Custom run…", "Needs attention", "Edit & run", "never run" | "Plans", "Run tests…", "Waiting for you", "Change and run", "Not run yet" | specific, plain, no "run" noun |
| Execution actions | "Re-run", "Re-run failed (n)", "Cancel run", "Open run" | "Run again", "Run failed again (n)", "Cancel execution", "Open execution" | #11 |
| Detail | "Copy to edit…", "Run this environment…" | "Copy and edit", "Run tests on this testbed…" | verb + object |
| Buttons | "Copy KUBECONFIG export" | "Copy `export KUBECONFIG`" | says what is copied |
| Execution grid | `(not run)` and "skipped" | "skipped" | #5 |
| Product name | "Welcome to Wendy" / "wendy looked in" | "Wendy" in prose, `wendy` for the command | consistent casing |

### CUE after the change

```cue
parts: hardware: ha: machines: {controlPlane: count: 3, worker: count: 3}
parts: managementCluster: "k8s-1.35": image: "kindest/node:v1.35.0"
parts: workloadCluster: "1.36": kubernetesVersion: "v1.36.4"
parts: workloadCluster: "1.37": kubernetesVersion: "v1.37.0"

testbeds: ci: {
	hardware:          "ha"
	managementCluster: "k8s-1.35"
	bootMode:          "isoboot"
	matrix: {
		order: ["k8s", "boot"]
		axes: {
			k8s:  {"1.36": workloadCluster: "1.36", "1.37": workloadCluster: "1.37"} // a string picks a part
			boot: {net: bootMode: "netboot", iso: {}}                                 // a struct patches fields
		}
	}
}

plans: nightly: {
	testbed: "ci"
	suites: ["lifecycle"]
	keep:        "failed"
	diagnostics: "always"
}
```

## Information architecture

### Navigation (web and TUI identical)

```
Home · Executions · Plans · Testbeds · Tests        ⚠ 2 config errors   ● 1 running   [Run tests…]
```

| Tab | Holds | Subtabs |
|---|---|---|
| **Home** | Waiting for you (paused executions), Plans (each with Run), Running, Kept testbeds, Recent | — |
| **Executions** | History, filterable by status. A **Kept** filter replaces the Environments tab | — |
| **Plans** | What to test, where and how: detail, with "Run" and "Change and run" | — |
| **Testbeds** | Testbeds, and the parts they are made of | Testbeds · Tinkerbell · Cluster API · Management cluster · Workload cluster · Hardware |
| **Tests** | What runs on a testbed | Suites · Tests · Built-in steps |

- **Depth:** any config entry is at most tab → subtab → item. The Catalog group row goes away.
- **The Environments tab goes away.** A kept testbed belongs to an execution. Its card (clusters, IPs, machines,
  kubeconfig, shell, tear down) appears on that execution's page and in Home's "Kept testbeds". Executions → Kept
  lists them all.
- **Config stops being a tab.** It becomes an indicator that only shows when there are errors (`⚠ 2 config errors`)
  and links to `/config`. That page also lists the loaded files and is linked from first launch.
- Keys `1`–`5` follow the new order in both UIs. The command palette accepts the old words as search aliases
  (run, environment, profile, scenario, matrix, catalog), but never shows them (principle 2).

### Screens

- **Home:** a paused execution gets a full-width "Waiting for you" banner at the top with its three actions (Retry
  step, Continue, Abort and tear down). Below it: Plans on the left, then Running, Kept testbeds and Recent on the
  right. First launch is unchanged apart from wording.
- **Run tests (form):** three cards. **Testbed** has a testbed picker, and when it has a matrix, combination globs
  or axis chips. **Tests** has suites and "Only tests with labels". **Options** has combinations at once, keep
  testbed, pause on failure, collect diagnostics and record consoles. The preview stays on the right. The
  Matrix/Environment radio goes away. "Save as plan" is deferred (below).
- **Execution:** a summary line first (status, time, counts), then the failure card, then grid or tree. A kept
  testbed shows its card under the summary.
- **Testbed detail:** parts as links, then settings, matrix (axes and combination count), labels, and used by. The
  resolved values (clusters, CAPI, machines, setup steps) move under a "Resolved values" disclosure (principle 4).
- **Forms:** every select shows human labels, with the CUE value in a tooltip. The rest of the field model is
  unchanged (common first, Advanced folded).
- **Visual:** no new components or colours. Emoji in placeholders (🔍) go. The top bar stays a top bar because it
  matches the TUI's tab row (consistency across surfaces outweighs NN/g's left-nav preference for apps at 5 items).

### CLI

| Today | Proposed |
|---|---|
| `wendy run --profile p` `-e/--env e` `-m/--matrix m` `--combo g` | `wendy run [plan]` · `-t/--testbed t` · `--combination g` (a plan, or a testbed and suites) |
| `wendy plan …` | `wendy preview [plan] …` (same flags as `run`) |
| `wendy env up` | `wendy up --testbed t` (build and keep, no suites) |
| `wendy env down <run>/<combo>` | `wendy down <execution>/<combination>` |
| `wendy env status` | `wendy status` (kept testbeds and running executions) |
| `wendy env kubeconfig <run>/<combo> [cluster]` | `wendy kubeconfig <execution>/<combination> [management\|tinkerbell\|workload]` |
| `wendy step run <step> --env <run>/<combo>` | `wendy step <built-in> <execution>/<combination> [--with json]` |
| `wendy list profiles\|environments\|tinkerbells\|providers\|workloads\|matrices\|combos\|scenarios\|steps…` | `wendy list plans\|testbeds\|tinkerbell\|cluster-api\|management-cluster\|workload-cluster\|hardware\|combinations\|suites\|tests\|steps` |
| `wendy report junit <run>` | `wendy report junit <execution>` |
| `wendy vet` | `wendy check` (matches "Host checks" in the UI) |
| `wendy serve` | unchanged |

`run` is the only command named with the verb run, and it takes a plan or a testbed, never an execution.
`up`, `down`, `status` follow `docker compose` and `vagrant`. They don't collide with `wendy list testbeds`, which
lists definitions, not live things (clig.dev: no similar-sounding commands).

## Modules

| Module id | Responsibility | Depends on |
|---|---|---|
| vocabulary | The glossary as code: one Go source for kind names (singular and plural), status words, phase names and enum display labels, used by both UIs (fixes #10). The retired-terms list the glossary guard reads. The README glossary. | — |
| model-rename | CUE schema, built-ins, starter config and `.wendy/wendy.cue`: `parts:` with five kinds (the management cluster split out of the Cluster API part), `testbeds:` with `matrix:`, `plans:`, `tests:`, `keep`/`diagnostics`. Go config types and kinds, the preview (today's plan package), execution IDs and data directories, event paths, artifact directories, JUnit names, error and migration messages, `design.md`. Go identifiers renamed to match (gopls rename). | vocabulary |
| cli | Command and flag changes in the CLI table. Help text uses glossary words only. | model-rename |
| web-ux | New nav and routes, Environments tab folded into Executions/Home, Config indicator, Run tests form, screen changes, copy, enum labels. | model-rename |
| tui-ux | The same tabs, order, screens and copy as the web UI. Catalog tabs follow the Testbeds/Tests subtabs. | model-rename |

Build order: vocabulary → model-rename → cli, web-ux, tui-ux (siblings, either order).

Why these boundaries:

- `vocabulary` is first and tiny so that the three surface modules can't drift apart. Each reads the same words.
- `model-rename` does the schema rename, the management cluster split and the matrix merge together because they
  touch the same files and tests. Doing them separately would mean renaming `matrices` only to delete it.
- `cli`, `web-ux` and `tui-ux` can each be verified on their own: CLI help goldens, rendered-page tests, TUI view tests.

## Commands

```
make build            # bin/wendy
make test             # unit tests, including rendered-page and glossary tests
make vet
make lint
make generate         # after any .templ change; make verify-generate in CI
```

## Testing strategy

- **Glossary guard (`Check` in `vocabulary`; one guard test per surface):** each surface module's test renders its
  web pages, TUI screens or `wendy --help` for every command, and runs `vocab.Check` over the visible text. It fails
  on any retired term or phrase used as a wendy concept:
  - Environment(s), except "environment variables"
  - Profile, Scenario, Catalog, Providers, Workload part, Evidence, combo, Target, "Ginkgo label filter"
  - "run" as a noun: "Runs", "New run", "Re-run", "Cancel run", "Open run", "a run", "the run", "this run"

  It also fails on a raw enum value used as a label (`onFailure`, `colocated`).
- **Existing tests move with the rename:** the config golden resolution, preview goldens, edit and service tests,
  web page tests, and a11y label tests. The quickstart plan's resolved testbed stays the same apart from renamed keys.
- **Management cluster split:** a resolution test shows a testbed that swaps only its management cluster part keeps
  the same Cluster API values, and the other way round. A matrix axis can pick a management cluster part.
- **Nav depth:** a web test asserts every config entry's URL is `/<tab>/<subtab>/<name>` or shallower.
- Each module's spec adds its own focused tests. No coverage targets.

## Boundaries

- **Always:** use only glossary words in new strings; change schema, docs, starter config and tests together; keep
  `cue/starter.cue.tmpl` and `.wendy/wendy.cue` identical; regenerate templates; ask before any git command.
- **Ask first:** adding a term to the glossary or changing one; adding a dependency or a new UI component; any change
  to what steps or `render.cue` do (as opposed to what they're called).
- **Never:** keep an old term as a second name in any UI (aliases are allowed only in palette search); add
  compatibility shims or migration of old configs; commit.

## Success criteria

- The glossary guard passes on every web page, TUI screen and CLI help. "Run" appears only as a verb.
- The web UI and TUI show the same five tabs in the same order with the same names. `1`–`5` reach them in both.
- No config entry is more than tab → subtab → item deep. The Environments and Config tabs and the Catalog groups are gone.
- A kept testbed's kubeconfig, shell and tear down are reachable from its execution page and from Home.
- The Run tests form has no Matrix/Environment choice. A testbed with or without a matrix runs from the same form.
- The management cluster is its own part. The Cluster API part has no cluster settings.
- Every select in every form shows a human label. No raw enum value appears as a label.
- `wendy run quickstart --plain` passes with the same steps as today's `wendy run --profile quickstart --plain`.
- `make vet`, `make lint`, `make test` and `make verify-generate` pass.

## Assumptions

1. Users know Kubernetes, Cluster API and Tinkerbell, so domain words such as management cluster, workload cluster,
   netboot, isoboot, CNI and BMC stay as they are. They aren't assumed to know test tools such as Ginkgo or
   Chainsaw, so those names appear only where wendy runs that tool, with a one-line explanation. Wendy's own
   invented terms are what change.
2. Visual styling (colours, glyphs, layout grid) is mostly right. This is about words and structure, not a re-skin.
3. The engine's behaviour is unchanged. Only names, config shape and where things are shown change. The management
   cluster split changes where its setting is written, not how the cluster is built.
4. No usability testing with real users before V1. The tree-test recommendation is deferred (below).

## Out of Scope (Deferred)

- **Tree test and first-click test of the new nav with 5–10 Tinkerbell contributors:** the research recommends it,
  and it should happen after V1 ships.
- **Testbed inheritance (`from: "other-testbed"`)** to share a base between testbeds with different matrices: CUE
  embedding covers it for now.
- **Left-side navigation in the web UI:** revisit if tabs grow past five.
- **Editing plans in the UIs, and "Save as plan" from the Run tests form:** plans stay CUE-only, as profiles are today.
- **Non-kind management clusters:** the Management cluster part leaves room, but V1 has only kind.
- **Undo instead of confirm** for delete and tear down: tear down can't be undone, and deleting an execution is rare.

## Resolved Questions

All seven recommendations were accepted:

1. *Plan* is what to test, where and how. *Execution* is each time a plan runs. *Run* is only a verb. Recipe,
   Pipeline, Trial and Session were the alternatives. Job and Build are ruled out because Rufio and Kubernetes have
   Jobs and wendy already builds images.
2. The management cluster is its own part, so it can be named (by its Kubernetes version), reused and picked by a
   matrix axis like the other parts.
3. The definition is a *Testbed*. "Environment" means environment variables only.
4. Matrix is a field of a testbed, not its own kind.
5. Scenario becomes *Test*.
6. Plans is a top-level tab, so there are five tabs.
7. `wendy vet` becomes `wendy check`.
