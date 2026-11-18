# Capability Map: Decoupled Catalog — V1

Status: implemented, then superseded by the [ux-redesign](../../ux-redesign/map.md) initiative's words. Archived.

## Why

`#Environment` bundles three unrelated concerns, so changing one means copying and editing all of them:

- **What we test**: management cluster, Tinkerbell, network, CAPI/CAPT.
- **What it runs on**: machine counts and sizes, BMC, libvirt backend.
- **What we deploy**: the workload cluster (Kubernetes version, CNI, OS).

Two run concerns also sit in the environment: `evidence` (a run option) and `upgrades` (scenario inputs). A matrix varies a full environment copy, so "three Kubernetes versions on one Tinkerbell" needs three copies of everything.

Goal: a user composes a test from small named parts, each with one purpose, and each editable and reusable on its own.

## Decisions so far

| Decision | Outcome |
|---|---|
| Split | Four parts: Tinkerbell (with management cluster and network), Cluster API / CAPT, Hardware, Workload |
| Composite | `Environment` = one of each part, referenced by name |
| Compatibility | Breaking. Built-ins and the starter config are migrated; no legacy inline form |
| Run concerns | `evidence` moves to Profile; upgrade targets become scenario parameters |
| Surfaces | CUE schema, plan, CLI, web UI and TUI |

## Target vocabulary (shared context for every module)

```
Tinkerbell ─┐
Cluster API ┼─▶ Environment ─▶ (Matrix varies one part) ─▶ Profile ─▶ run
Hardware   ─┤                                              ▲
Workload   ─┘                          Suites ▶ Scenarios ▶ Steps
```

Part names are provisional; see Open Questions.

## Modules

| Module id | Responsibility | Depends on |
|---|---|---|
| catalog-model | CUE schema and Go types for the four parts and the composite Environment; resolution to today's concrete `config.Environment`; the shared editor field model and write-back to `wendy-ui.cue`; migrated built-ins and starter config | — |
| run-options | `evidence` on Profile; upgrade targets as scenario parameters (built-in upgrade scenarios, `capt.upgrade.*`, `tinkerbell.upgrade`, `capi.upgrade`) | catalog-model |
| composition-plan | Matrix axes that vary one part (by name or by field patch); intrinsic axes and labels derived from the right part; combination naming; host-needs estimate; `wendy plan`, `list` and `vet` output | catalog-model |
| web-catalog-ui | Catalog tabs and detail pages for the parts; per-part editors; an environment editor that picks parts and shows "used by"; run form evidence override; tab descriptions | editor-service |
| tui-editor | The same parts and editors in the TUI | editor-service |

Build order: catalog-model → run-options, composition-plan → editor-service → web-catalog-ui, tui-editor

> **Amendment (approved): `editor-service`.** Looking at the code after the first three modules, both UI modules need the same backend work: making environments and parts editable through the service, "used by" and reference checks, a corrected field model, and the run-evidence override on `Selection`. Built inside either UI module, the other would depend on it. So it becomes its own module, [editor-service/spec.md](editor-service/spec.md), ahead of both. The two UI modules then stay independent siblings, as originally intended.
>
> | editor-service | Environments and the four parts editable through `service` and `config.Kinds`; exported "used by"; reference checks on save and delete; field-model corrections (`ui.Field.Advanced`, environment fields); `Selection.Evidence` override in the plan | catalog-model, composition-plan, run-options |

Why these boundaries:

- `catalog-model` owns the editor field definitions in `internal/ui/fields.go` because the web UI and the TUI both build their forms from them. Neither UI module redefines fields.
- `run-options` is separate from the model because it changes Profile and scenario semantics, not the shape of an environment, and can be verified with plan and step tests alone.
- `composition-plan` is separate because matrix and label behavior is testable with golden plan output, with no UI.
- The two UI modules are siblings that do not depend on each other. Either can ship first.

## Assumptions

1. Steps and the engine keep reading the resolved `config.Environment`. Only how it is authored and assembled changes.
2. Parts are referenced by name and may also be written inline, so a one-off environment needs no extra catalog entries.
3. wendy is pre-1.0, so no deprecation window is needed.
4. A matrix axis value may name a different part (e.g. a different Workload) or patch fields within one.

## Resolved Questions

All five proposals were accepted:

1. Parts are **Tinkerbell**, **Providers** (Cluster API and CAPT), **Hardware** and **Workload**.
2. `management` (kind) and `network` go with Tinkerbell.
3. `arch` lives on Hardware; the Workload's OS image follows it.
4. Catalog tabs are grouped as "Build" (Tinkerbell, Providers, Hardware, Workloads, Environments, Matrices) and "Test" (Suites, Scenarios, Steps), with Profiles on top.
5. Upgrade targets are step parameters (`with: {version: ...}`); the built-in upgrade scenarios are templates users copy and set.

## Out of Scope (Deferred)

- Multiple workload clusters per environment (one environment, many workloads): needs engine state changes.
- Non-libvirt hardware backends and non-kind management clusters: the schema already leaves room.
- Sharing parts across config directories or publishing them: no demand yet.
- Renaming `wendy env` (kept environments) to avoid confusion with catalog Environments: separate CLI change.
