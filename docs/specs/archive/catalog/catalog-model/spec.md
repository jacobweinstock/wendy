# Spec: catalog-model — V1

Module of [map.md](../map.md). Depends on: nothing. Required by: run-options, composition-plan, web-catalog-ui, tui-editor.

Status: implemented. Archived. Plan: [plan.md](plan.md); tasks: [todo.md](todo.md).

## Objective

Split `#Environment` into four named, reusable catalog parts and make an Environment a composite that picks one of each, so Tinkerbell configuration, provider versions, machine sizing and the workload cluster are authored, copied and edited independently.

**User:** someone writing or editing wendy config, in CUE or (after the UI modules land) in the web UI and TUI.

**Why:** today, changing the cluster size means copying and editing an environment that also holds the Tinkerbell chart, network and CAPI versions. Each concern should have one home and one name.

### The four parts

| Part | CUE collection | Contains (from today's `#Environment`) |
|---|---|---|
| Tinkerbell | `tinkerbells` | `management`, `network`, `tinkerbell` (topology, chart, source, values, bootMode, namespace, captainos) |
| Providers | `providers` | `capi.core`, `capi.capt` (release or source) |
| Hardware | `hardware` | `arch`, `hardware.backend`, `hardware.bmc`, `hardware.machines` |
| Workload | `workloads` | `workload` (name, kubernetesVersion, osVersion, cni, kubeVipVersion) |

`arch` lives on Hardware; the Workload's OS image follows it.

### The composite

```cue
tinkerbells: stable: {bootMode: "netboot"}
hardware: small: {machines: worker: count: 1}
hardware: ha:    {machines: {controlPlane: count: 3, worker: count: 3}}
workloads: "k8s-1.36": {kubernetesVersion: "v1.36.4"}

environments: ha: {
	description: "Stable Tinkerbell on a 3+3 cluster"
	hardware:    "ha"
	workload:    "k8s-1.36"
	// tinkerbell and providers omitted: the built-in "default" of each
}
```

A part reference is a name, or an inline part for one-off use. An omitted reference means the built-in `default` of that part.

### Acceptance stories

1. As a user, I copy a Hardware part, change `worker.count`, and every environment that names it picks the change up; the Tinkerbell part is untouched.
2. As a user, a typo in a part name fails with the file:line of the reference and a "did you mean" list, like today's suite and scenario references.
3. As a user, the built-in `quickstart` profile still runs and resolves to exactly the environment it resolves to today.

## Tech Stack

Go 1.26, CUE (`cuelang.org/go` v0.17.1), `html/template`-free for this module. No new dependencies.

## Commands

```
Build:            make build
Test:             make test                  # go test -race ./...
Test one package: go test -race ./internal/config/ ./internal/ui/ ./internal/plan/
Vet:              make vet
Lint:             make lint
Config check:     go run ./cmd/wendy vet
Plan check:       go run ./cmd/wendy plan --profile quickstart
Templates:        make generate && make verify-generate   # only if a .templ file changes
```

## Project Structure

```
cue/wendy/schema.cue        → #Config collections, the four part schemas, composite #Environment
cue/wendy/builtin.cue       → built-in default parts, quickstart environment and profile, migrated
cue/starter.cue.tmpl        → starter config, migrated to the composite form
internal/config/types.go    → Go types for parts and the resolved Environment
internal/config/load.go     → extract and resolve parts; crossCheck for part references
internal/config/edit.go     → Kinds, SpecOf, Taken, Check for the new kinds
internal/ui/fields.go       → field model per part (replaces EnvFields/EnvGroups); NewValue; NormalizeEnv
internal/ui/web, ui/tui     → minimal edits only: environment editors disabled (see Boundaries)
design.md, README.md  → vocabulary and examples
```

## Code Style

Follows the repo: small functions, doc comments on exported names that say what and what-for (not how), table-driven tests, errors carry a config position. Resolution reuses the matrix approach of compiling a CUE expression in the root's scope so errors keep their positions.

```go
// Part is a named, reusable piece of an environment.
type Part[T any] = Item[T]

// Catalog gains one collection per part.
Tinkerbells map[string]*Item[TinkerbellPart]
Providers   map[string]*Item[ProvidersPart]
Hardware    map[string]*Item[HardwarePart]
Workloads   map[string]*Item[WorkloadPart]

// EnvRef is how an environment names its parts, for "used by" and editing.
type EnvRef struct{ Tinkerbell, Providers, Hardware, Workload string }
```

`Catalog.Environments` keeps holding the resolved `config.Environment`, so plan, engine and steps read what they read today.

## Testing Strategy

Go tests beside the code, table-driven, no new framework.

- `internal/config/load_test.go`: resolution golden (see criteria 1–2), reference errors, inline parts, default parts, matrix still expands (inline-part patches).
- `internal/config/edit_test.go`: the new kinds round-trip through `wendy-ui.cue`; `Taken` and `Check` cover them.
- `internal/ui/fields_test.go`: every old environment field belongs to exactly one part; `Get`/`Set` round-trip per part.
- `internal/plan/plan_test.go`: existing plan tests migrated, results unchanged.
- No UI behavior is tested here beyond "builds and environment editors are off".

## Boundaries

- **Always:** keep `Catalog.Environments` resolved and shaped as today; keep every step and the engine untouched; give every new error a file:line and a suggestion where one exists; run `make test vet lint` before finishing.
- **Ask first:** renaming a part or a CUE collection after approval; adding a fifth part; changing the resolved `Environment` JSON (it is written to `environment.json` in run dirs).
- **Never:** change `evidence` or `upgrades` placement (run-options owns that); design axes that swap whole parts (composition-plan owns that); build the new editors or tabs (the UI modules own those); keep the old inline `#Environment` form.

Interim state until the UI modules land: the web and TUI environment editors are disabled (no "New environment", no "Edit"); the catalog pages still show resolved environments. Parts are authored in CUE.

## Success Criteria

1. `schema.cue` defines `tinkerbells`, `providers`, `hardware`, `workloads` and a composite `#Environment`; the old inline fields are rejected with a clear error.
2. Resolving `environments.quickstart` (built-ins) yields a `config.Environment` deep-equal to today's, including `setup`, with golden test proof.
3. An environment naming a part by name, by inline struct, and by omission (built-in `default`) each resolve correctly.
4. A reference to an unknown part reports `file:line`, the reference path (e.g. `environments.ha.hardware`) and up to three close names.
5. A part reports its own schema errors at its own position, not at every environment that uses it.
6. Changing one part changes every environment that names it and no other part's resolved values.
7. `Spec`, `SpecOf`, `Taken` and `Check` handle the four new kinds; a part written through the UI file loads back identically.
8. The starter config and README examples validate with `wendy vet`.
9. `ui` exposes a field list per part; the union of the lists equals today's `EnvFields` set, each field in exactly one part, with `arch` on Hardware.
10. `make test`, `make vet`, `make lint` pass; `wendy plan --profile quickstart` output is unchanged except for part names.

## Out of Scope (Deferred)

- Moving `evidence` to Profile and upgrade targets to scenario parameters: run-options.
- Matrix axes that swap a part by name, combination naming, plan host-needs: composition-plan.
- Part tabs, per-part editors, "used by" in the web UI: web-catalog-ui.
- Part editors in the TUI: tui-editor.
- Several workloads per environment: needs engine state changes.
- A fifth `Network` part: revisit if Tinkerbell grows unwieldy.

## Resolved Questions

1. An omitted part means the built-in `default` of that part.
2. The collection names stay: `tinkerbells`, `providers`, `hardware`, `workloads`.
3. The web and TUI environment editors are disabled until their modules land.
