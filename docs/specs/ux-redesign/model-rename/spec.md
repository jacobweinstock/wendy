# Spec: model-rename — V1

Module of [map.md](../map.md). Depends on: vocabulary (implemented). Required by: cli,
web-ux, tui-ux.

Status: implemented. Plan: [plan.md](plan.md); tasks: [todo.md](todo.md).

## Objective

Make wendy's data model speak the glossary: what users write in CUE, what an execution leaves on disk, and the
Go code that connects them. This module also makes the two model changes the map approved:

1. **Renames.** Environment → testbed, profile → plan, scenario → test, run (noun) → execution, combo →
   combination, evidence → diagnostics, and the part kinds → Tinkerbell, Cluster API, Management cluster, Workload
   cluster, Hardware. This covers CUE keys, JSON fields, file and directory names, error messages, docs and Go
   identifiers (full rename, by gopls where it can).
2. **Matrix in the testbed.** A testbed may carry a `matrix:`. The `matrices:` kind goes away, and a plan names
   one testbed, with or without a matrix.
3. **Management cluster as its own part.** Its node image leaves the Cluster API part, which then holds only
   Cluster API and CAPT.

The UIs and CLI change only where the model forces them to (kind ids, the matrix kind going away, the selection
flags). Their layout, wording and command structure are left to `web-ux`, `tui-ux` and `cli`.

**User:** everyone who writes wendy config, reads an execution's files or events, or works on wendy's code.

## Current Behavior

- CUE ([schema.cue](../../../../cue/wendy/schema.cue)): top-level `tinkerbells`, `providers`, `hardware`, `workloads`,
  `environments`, `scenarios`, `suites`, `matrices` and `profiles`. `#Environment` names four parts plus
  `topology`, `bootMode` and `network`. A Providers part holds `core`, `capt` and `management`.
- A `#Matrix` has `base` (an environment), `order`, `axes`, `exclude` and `suites`. An axis value either selects a
  part by name (`workload: "next"`) or patches the *resolved* environment (`tinkerbell: bootMode: "isoboot"`)
  ([resolve.go](../../../../internal/config/resolve.go) `splitAxisValue`).
- A profile names a matrix (and `combos`) or an environment, and holds `evidence: {consoles, dump}`.
- The resolved environment ([types.go](../../../../internal/config/types.go) `Environment`) has `management`, `capi`,
  `workload` and `evidence`. Steps, `render.cue` and the bundle's `state.json` read it.
- Executions are `r-0412` in `<data>/runs/`. Events carry `run` and `combo`. The failure phase is `evidence`, and
  `plan.json`, `summary.json` and the JUnit report use the same words.
- `load.go` carries migration hints for configs written before parts existed.

## New Behavior

### CUE

```cue
parts: {
	tinkerbell:        [string]: #TinkerbellPart        // chart, source, values, namespace, captainos, cluster
	clusterAPI:        [string]: #ClusterAPIPart        // core, capt
	managementCluster: [string]: #ManagementClusterPart // type (kind), image
	workloadCluster:   [string]: #WorkloadClusterPart   // name, kubernetesVersion, osVersion, cni, kubeVipVersion
	hardware:          [string]: #HardwarePart          // arch, backend, bmc, machines
}
testbeds: [string]: #Testbed
tests:    [string]: #Test   // was #Scenario; unchanged fields
suites:   [string]: #Suite  // `tests: [...]`, was `scenarios: [...]`
plans:    [string]: #Plan
```

```cue
#Testbed: {
	description?:      string
	tinkerbell:        *"default" | #PartRef
	clusterAPI:        *"default" | #PartRef
	managementCluster: *"default" | #PartRef
	workloadCluster:   *"default" | #PartRef
	hardware:          *"default" | #PartRef
	topology?:         string
	bootMode?:         string
	network?:          {...}
	matrix?: {
		order: [...string]
		axes: [string]: [string]: {...} // each value is a patch of this testbed (see below)
		exclude: [...{[string]: string, reason: string}]
	}
}

#Plan: {
	description?:   string
	testbed:        string
	combinations:   [...string] | *[]            // globs over combination names; empty is all
	suites:         [...string]
	labelFilter:    *"" | string
	parallel:       *1 | int & >=1
	keep:           *"never" | "failed" | "always"
	pauseOnFailure: *false | bool
	diagnostics:    *"onFailure" | "always" | "never"
	consoles:       *true | bool
}
```

- **Axis values patch the testbed as written,** using the same keys: a setting (`bootMode: "isoboot"`,
  `network: family: "ipv6"`), a part picked by name (`workloadCluster: "1.37"`), or a part's fields patched
  (`hardware: machines: worker: count: 5`, merged over the testbed's part). One shape for a testbed and its axes
  replaces today's mix of picks and resolved-shape patches.
- A testbed without a matrix is one combination, named after the testbed. Combination names, labels, `exclude` and
  the intrinsic axes work as today.
- A plan's `combinations` on a testbed without a matrix is an error that names the plan.
- Built-ins keep their meaning under the new names, in [builtin.cue](../../../../cue/wendy/builtin.cue): the `default` part of
  each kind, testbed `quickstart`, plan `quickstart`, tests and suites. `upgrade-provider` becomes
  `upgrade-cluster-api`, and the engine's `evidence` scenario becomes the hidden `diagnostics` phase, not a test.

### Resolved testbed

`#ResolvedEnvironment` / `config.Environment` become `#ResolvedTestbed` / `config.Testbed`, with `management` →
`managementCluster`, `capi` → `clusterAPI`, `workload` → `workloadCluster` and `evidence` → `diagnostics:
{collect, consoles}`. `render.cue`, the steps and `state.json` read the new names. What gets built is unchanged.

### Executions

| Today | New |
|---|---|
| ID `r-0412`, directory `<data>/runs/r-0412` | ID `412`, directory `<data>/executions/412`, shown as `#412` |
| kept testbed `r-0412/<combo>` | `412/<combination>` |
| event fields `run`, `combo` | `execution`, `combination` |
| phase `evidence`, its directory and `Failure.Evidence` | `diagnostics` |
| `plan.json` (package `plan`, `plan.Plan`, `plan.Selection`) | `preview.json` (package `preview`, `preview.Preview`, `preview.Selection`) |
| `Selection{Profile, Environment, Matrix, Combos, Evidence}` | `Selection{Plan, Testbed, Combinations, Diagnostics}` |
| summary `combos`, JUnit suite per combo | `combinations`, JUnit suite per combination |

`vocab.Phase` is re-keyed from `evidence` to `diagnostics`. The `vocab` choices `keep` and `evidence.dump` become
`keep` (`failed`) and `diagnostics`.

### Go

Identifiers follow the glossary in every package this module touches, renamed with gopls where possible, e.g.
`EnvSpec` → `TestbedSpec`, `Profile` → `Plan`, `Scenario` → `Test`, `Combo*` → `Combination*`, a noun `Run` →
`Execution`. `config` kind constants come from `vocab.Kinds`. `config.PartKinds` is the five `Part` kinds in
glossary order, and `singular`-by-trimming goes away, which fixes problem #10 ("＋ New provider"). Identifiers local
to `web`, `tui` and `cli` that only those modules display (e.g. `runPage`) are left to `web-ux`, `tui-ux` and
`cli`.

### Surfaces, only where the model forces it

- **Web and TUI:** catalog tabs come from `vocab.Kinds` (Plans; testbeds and the five parts; suites, tests and
  built-in steps). The Matrices tab and pages go away, and a testbed's page shows its matrix. The environment editor
  becomes the testbed editor, with five part pickers, and it keeps a testbed's `matrix` on save. The run forms pick a
  testbed, and offer combinations when it has a matrix. Routes follow the kind ids (`/catalog/testbeds/…`).
- **CLI:** the selection flags become `--plan`, `-t/--testbed` and `--combination` (`--profile`, `--env`,
  `--matrix` and `--combo` go away), and `wendy list` takes the new kind slugs. Other commands are left to `cli`.

### Messages and docs

- Config errors and hints use the glossary. The migration hints for pre-parts configs are deleted: old keys get
  the schema's own "field not allowed" error at their position.
- README: the config sections are rewritten for the new model, and gain the glossary (deferred here by
  `vocabulary`). `design.md`, `cue/starter.cue.tmpl` and `.wendy/wendy.cue` (kept identical) are updated, and
  `.wendy/wendy-ui.cue` is rewritten in the new shape.

## Commands

```
make build
make test
make vet
make lint
make generate          # after .templ changes
make verify-generate   # runs git diff; ask first
go test -race ./internal/config/ ./internal/preview/ ./internal/engine/   # inner loop
```

## Project Structure

```
cue/wendy/schema.cue, builtin.cue, render.cue, steps.cue   → schema, built-ins, resolved shape
cue/starter.cue.tmpl, .wendy/*.cue                          → starter and local config (identical)
internal/config/                                            → types, load, resolve (matrix in testbed), edit
internal/preview/  (was internal/plan/)                     → selection, preview, estimates
internal/engine/, internal/service/, internal/runner/       → executions, phases, data layout, events
internal/event/, internal/report/, internal/steps/          → event fields, JUnit, steps reading the testbed
internal/vocab/                                             → re-keyed phase and choices
internal/ui/, internal/ui/web/, internal/ui/tui/, internal/cli/  → forced changes only (see above)
README.md, design.md                                  → docs
```

## Code Style

As in the rest of the repo. Renames go through gopls (`gopls rename`) so every reference moves together, and
comments are re-read after each rename so they still say what they mean:

```go
// TestbedSpec is a testbed as written: the part it picks of each kind, the
// settings no part owns, and its matrix. A part is a name, or a Part written
// inline.
type TestbedSpec struct { … }
```

## Testing Strategy

- **Golden resolution:** the quickstart testbed resolves to the same values as today's quickstart environment, apart
  from the renamed keys.
- **Matrix in a testbed:** a testbed with the axes of today's `versions`-style matrix expands to the same
  combinations, names and labels as before. Axis values patch settings, pick parts and patch part fields. A plan's
  `combinations` on a testbed without a matrix is an error naming the plan.
- **Management cluster part:** swapping it leaves Cluster API values alone, and the other way round. An axis can
  pick one.
- **Old keys:** `environments:`, `profiles:`, `scenarios:`, `matrices:`, `providers:` and `workloads:` each fail with
  the schema's error at their position.
- **Executions:** a new execution gets the next numeric ID in `executions/`. Events carry `execution` and
  `combination`, and failure diagnostics land under the `diagnostics` phase. JUnit names use combinations.
- **Kinds:** `config` kinds equal the `vocab.Kinds` ids, and the web shows "＋ New Cluster API part".
- Existing config, preview, engine, service, runner, report, web, TUI and CLI tests move with the renames.

## Boundaries

- Always: rename with gopls where it can; change schema, built-ins, starter, `.wendy/wendy.cue`, docs and tests
  together; keep the build green after every task; ask before any git command (including `make verify-generate`).
- Ask first: changing what a testbed builds (only names and where things are written change); any surface change
  beyond the forced ones listed above; adding a dependency.
- Never: accept an old key or keep an alias for one; leave a retired word in CUE, JSON, file names or Go
  identifiers of the packages this module owns; commit.

## Success Criteria

1. The schema, built-ins, starter and `.wendy/*.cue` use only the new keys. An old key fails with the schema error at
   its position.
2. The quickstart testbed resolves to today's values under the new names. `wendy run --plan quickstart --plain` runs
   the same steps as `wendy run --profile quickstart --plain` does today.
3. A testbed with a matrix expands to the same combinations as the equivalent matrix today. Axis values patch the
   testbed as written. A plan names one testbed.
4. The management cluster is its own part, and the Cluster API part holds only `core` and `capt`.
5. Executions are numbered (`412`), live in `executions/`, and their events, summaries, `preview.json`, JUnit and
   diagnostics use the new names.
6. `config` kinds are the `vocab.Kinds` ids. The web catalog shows the new kinds with no Matrices tab, the testbed
   editor has five part pickers, and "＋ New Cluster API part" reads correctly.
7. No retired word remains in the CUE, JSON tags, file and directory names, or Go identifiers of `config`,
   `preview`, `engine`, `service`, `runner`, `event`, `report`, `steps` or `vocab`.
8. README (with the glossary), `design.md` and the starter describe the new model.
9. `make vet`, `make lint`, `make test` and `make verify-generate` pass.

## Out of Scope (Deferred)

- Web and TUI navigation, screens and wording (`web-ux`, `tui-ux`), and the identifiers local to them.
- CLI commands other than the selection flags and `list` kinds: `up`, `down`, `status`, `kubeconfig`, `step`,
  `check` and `preview` (`cli`).
- Glossary guard tests over rendered output (`cli`, `web-ux`, `tui-ux`).
- Editing a testbed's matrix in a form: the editor keeps it, and CUE writes it.
- Naming the `adhoc` phase: it needs a glossary word.

## Resolved Questions

All four proposals were accepted:

1. Axis values patch the testbed as written: a setting, a part picked by name, or a part's fields.
2. Execution IDs are plain numbers (`412`, directory `executions/412`, shown as `#412`).
3. Old keys get the schema's "field not allowed" error at their position, and the migration hints are deleted.
4. The selection flags become `--plan`, `-t/--testbed` and `--combination` in this module. The other CLI changes stay
   with `cli`.
