# Plan: editor-service — V1

Implements [spec.md](spec.md). Tasks: [todo.md](todo.md).

Status: proposed (awaiting review). No code until this plan is approved.

## Approach

Three independent pieces, then a gate. The field model and the plan change touch no UI; the service change is the one that needs care.

```
E1 field model fixes ──▶ E2 service: kinds, refs, UsedBy ──┐
E3 Selection.Evidence (plan) ──────────────────────────────┴─▶ E4 gate
```

## Design decisions to confirm

1. **`config.Kinds` stays the list the *service* accepts; the UIs gate separately until their modules land.** Two code paths read `config.Kinds` today: the service, and the web UI (`editKind` routes, and `canEdit` for the "＋ New" button). If E2 widens `Kinds`, `/edit/environments` would become a live route that renders the *suite* form (`editData.fields()` falls through to suite fields) and saves garbage. So E2 also gives the web UI its own `editable` list, containing what it has handlers for (scenarios and suites), and `web-catalog-ui` widens it as it adds each kind. The TUI is unaffected (it maps tabs to kinds by hand).
2. **`Advanced` fields, fixed here so both UIs fold the same ones.**
   - Tinkerbell: `management.image`, `namespace`, `chart.location`, `values`, `captainos.flavor`, `captainos.image`, `network.nat64`, `network.mirror.enabled`, `network.mirror.upstreams`. Common: `topology`, `bootMode`, `chart.version`, `source.repo`, `source.ref`, `network.family`.
   - Providers: `capt.repository`. Common: `core`, `capt.version`, `capt.source.repo`, `capt.source.ref` (building CAPT from a checkout is as common as building Tinkerbell).
   - Hardware: `backend.uri`, `backend.diskPath`, `bmc.image`, `bmc.protocol`. Common: `arch` and the machine table.
   - Workload: `name`, `osVersion`, `kubeVipVersion`. Common: `kubernetesVersion`, `cni`.
   - Environment: none (description only).
3. **`UsedBy` strings are "kind name"** (`environment ha`, `matrix versions`, `profile mine`, `suite smoke`), the format `usedBy` already returns, so existing messages and tests keep their text. Parts are used by environments (`EnvSpecs` part names) and by matrices whose axis values select them (found with `config`'s `splitAxisValue`, exported as a small helper rather than reimplemented).
4. **Reference checks run before the write**, in `Service.refs`, against the catalog plus the spec being saved (as suites do), so an environment can name a part created earlier in the same session. Matrices are not editable here, so no matrix reference needs checking on save; they are covered by `UsedBy` on delete and rename.
5. **Evidence override is a pointer on `Selection`** (`*config.Evidence`, omitted when nil), so "no override" is distinguishable from "override to the zero value". `effective` copies it and marks the plan edited when it differs from the profile's.

## Phases and checkpoints

### Phase A — independent (E1, E3)
**Checkpoint:** `go test ./internal/ui/ ./internal/plan/` pass; nothing else changes behavior.

### Phase B — service (E2)
**Checkpoint:** service tests for all new kinds; web/TUI tests unchanged and passing; `/edit/environments` still 404 in the web UI.

### Phase C — gate (E4)

## Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Widening `Kinds` exposes routes with the wrong form | High | Decision 1; a web test keeps `/edit/environments` and `/edit/hardware` 404 until the web module opens them |
| `UsedBy` misses matrix axis selections | Medium: a part deleted under a matrix | A test with a matrix that selects a part; the helper is shared with the resolver so they cannot disagree |
| `SaveEntry` rename leaves references dangling | Medium | Existing rename check now covers parts and environments through `UsedBy` |
| `Selection` JSON is stored in run records | Low | New field is `omitempty`; old records decode unchanged |
| `ProfileEvidenceFields` unused until a UI lands | Low | Tested for round-trip; documented as for run forms |

## Parallelism

E1 and E3 are independent and touch disjoint packages (`ui`, `plan`). E2 needs E1 (`prepare` uses the part field lists). E4 last.
