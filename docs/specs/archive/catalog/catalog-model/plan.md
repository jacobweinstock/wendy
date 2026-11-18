# Plan: catalog-model — V1

Implements [spec.md](spec.md). Tasks: [todo.md](todo.md).

Status: proposed (awaiting review). No code until this plan is approved.

## Approach

Resolve parts into today's flat `config.Environment`, so everything downstream (plan, engine, steps, matrices) keeps reading what it reads now. The change is concentrated at the authoring boundary: CUE schema, loader, editor model.

```
CUE config ──▶ composite environments + part collections
                         │  resolve (named | inline | omitted→default)
                         ▼
                 flat #Environment (unchanged shape, incl. setup)
                         │
          Catalog.Environments  ──▶ plan / engine / steps / matrices (unchanged)
```

## Dependency graph

```
T1 spike: CUE resolution
   │
T2 schema + built-in defaults + quickstart golden ──▶ T3 Go types + loader + part errors
                                                          │
                          ┌───────────────────────────────┼───────────────┐
                          ▼                               ▼               ▼
                 T4 migrate fixtures,           T5 edit.go kinds     T6 ui field model
                 starter, docs                  (Spec/SpecOf/Taken)       │
                          │                               │               │
                          └──────────────┬────────────────┴───────────────┘
                                         ▼
                          T7 disable env editors (web, TUI, service)
                                         ▼
                          T8 full verification
```

T4, T5 and T6 are independent once T3 lands and can run in parallel. T1 must come first because it decides the approach T2 and T3 build on.

## Design decisions to confirm

These fall out of the spec and are the parts of the plan most worth your review.

1. **Resolution happens in Go, after the directories are merged (T1 spike: confirmed).** Name references already resolve across directories in `crossCheck`, so a part in one config directory must be usable from an environment in another; per-directory CUE resolution cannot do that. Parts are stored as raw maps of only what the user wrote (schema `{...}`, no defaults). Resolution assembles the flat environment from them (Tinkerbell part → `management`, `network`, `tinkerbell.*`; Providers → `capi.*`; Hardware → `arch`, `hardware.*`; Workload → `workload.*`), unifies it with the flat schema (now `#Resolved`, which keeps all defaults and `setup`), and decodes it. Keeping defaults out of parts keeps them overridable, so matrix patches such as `network: family: "ipv6"` still derive `nat64` as today. A schema error is mapped back to the part by its path (e.g. `hardware.machine` → Hardware), deduplicated, and reported once at the part's position. A part nobody uses is checked against the built-in defaults of the other parts. Matrix expansion moves to the same post-merge step.
   Deviation from the spec's code sketch: parts are `Item[Part]` with `type Part map[string]any`, not typed structs, because decoding typed structs loses the difference between "unset" and "false/zero".
2. **Matrices keep their flat shape in this module.** `matrices.*.base` becomes an environment *name* (a string) that the loader resolves to the flat environment; axis patches stay flat (`network: family: "ipv6"`), exactly as today. This keeps every existing matrix semantic and leaves part-swapping axes to composition-plan. Today `base: environments.default` is a struct; after this change it is `base: "default"`.
3. **A part's own errors surface at the part.** Parts are decoded and validated as their own collections first, and environments then reference them, so a bad `hardware.small` reports once, at its own position (criterion 5).
4. **Interim editors.** `config.Kinds` (the editable-kinds list the web editor, service and catalog "＋ New" button read) drops `environments`. Parts are not added to it either; their editors arrive with the UI modules. `SpecOf`, `Taken` and `Check` still handle all kinds so the later modules only add UI.

## Phases and checkpoints

### Phase A — De-risk (T1)
A throwaway test proves a composite with named, inline and omitted parts resolves to a flat environment in CUE (or Go), and that a bad reference keeps its position.
**Checkpoint:** approach chosen and recorded at the top of `schema.cue`; the golden for `quickstart` equality is written and passes against the old shape.

### Phase B — Core model (T2, T3)
Schema, built-in default parts, loader and Go types, reference errors.
**Checkpoint:** `go test ./internal/config/` green with the quickstart golden (criteria 1–6). `wendy vet` on the built-ins is clean. Other packages may not compile their fixtures yet.

### Phase C — Migrate and extend (T4, T5, T6, in parallel)
Fixtures, starter and docs move to the new form; editor plumbing learns the new kinds; field model splits per part.
**Checkpoint:** `go test ./...` compiles and passes except the editor-disable tests (T7). Criteria 7–9.

### Phase D — Interim UI state and verification (T7, T8)
Editors disabled; full gate.
**Checkpoint:** `make test vet lint` pass; `wendy plan --profile quickstart` matches today's output apart from part names (criterion 10); `make verify-generate` clean if any `.templ` changed.

## Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| CUE comprehension over part names loses error positions or makes errors cryptic | High: error quality is a stated criterion | T1 spike tests a bad reference and a bad part before committing to the approach; Go-side resolution is the fallback |
| Hidden `setup` derivation (`#Environment.setup` reads `tinkerbell.topology`) breaks when topology comes from a part | High: runs would get the wrong preamble | Derive `setup` on the resolved flat environment, never on the composite; golden compares `setup` |
| Defaults for omitted parts reintroduce staleness (pinned versions) | Medium | Defaults live only in `builtin.cue` (already where `quickstart` pins them) and are named `default`; a user can shadow them |
| Many test fixtures use the flat form (6 test files) | Medium: noisy, mechanical | One fixture helper builds the new form from flat maps; migrate in T4 as one reviewable commit |
| `Kinds` is read by service, web and TUI | Medium: partial removal leaves dead routes | T7 greps every reader; a test asserts `/edit/environments` 404s |
| `wendy-ui.cue` files users already wrote use the old inline form | Low (pre-1.0, break approved) | Load error names the field and points at the migration note in the README |

## Parallelism

Sequential: T1 → T2 → T3. After T3, T4/T5/T6 touch disjoint files (`testdata`/starter/docs; `internal/config/edit*.go`; `internal/ui/fields*.go`) and can run concurrently. T7 and T8 are sequential at the end.

## Out of scope for this plan

Everything under the spec's "Out of Scope (Deferred)". In particular no `.templ` page for parts, no part tabs, no `evidence`/`upgrades` move.
