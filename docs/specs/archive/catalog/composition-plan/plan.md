# Plan: composition-plan — V1

Implements [spec.md](spec.md). Tasks: [todo.md](todo.md).

Status: proposed (awaiting review). No code until this plan is approved.

## Approach

Matrix expansion already assembles a base environment's parts (`resolver.pick` + `assemble`) and then unifies axis patches over the result. Part selection slots in between: swap the picked part for the named one, re-assemble, then apply the remaining patches.

```
base env ─▶ pick parts ─▶ [axis selections replace parts] ─▶ assemble ─▶ unify struct patches ─▶ resolved combo
                                   ▲
                       splitAxisValue(patch): string under a part-kind key = selection
```

Surfacing is separate and small: record part names on `Combo`, print them, list parts.

## Design decisions to confirm

1. **Selections are validated once per axis value, not per combination.** The resolver checks every axis value's selected names against the catalog before the cartesian product, so an unknown name reports once at the axis value's position. This needs the position of each axis value, which `decodeMatrix` can capture from the CUE value (`Matrix.AxisPos`, `axis → value → "file:line:col"`), the same way `Item.Pos` is captured.
2. **Conflicts are checked per combination.** Two axes selecting different parts of one kind is an error on that combination (`Combo.Err`), naming both axes, so one bad pair does not hide the combinations that are fine. The matrix is unusable for that profile, as with any `Combo.Err` today.
3. **Axis keys that name part kinds.** `tinkerbell`, `hardware` and `workload` are keys of both the resolved environment and the part kinds; `providers` is the part kind but the resolved field is `capi`. Only the four part-kind names with string values select; `capi: ...` remains a field patch.
4. **`Combo.Parts` comes from the same pick the resolver uses**, so it can never disagree with what was built. For a plain environment selection, the plan reads `cat.EnvSpecs[name].Value.PartNames()`. For a matrix combination, the resolver stores the final names on `config.Combo`.
5. **`wendy plan` prints parts only where they differ across the combinations shown.** For one combination, nothing is added; for a matrix, only the parts that vary appear, so the common case stays uncluttered.

## Dependency graph

```
C1 part selection in matrix expansion ─▶ C2 Combo.Parts in plan + plan output ─▶ C4 UIs show parts ─┐
C3 `wendy list` for parts (independent of C1/C2) ──────────────────────────────────────────────────┼─▶ C5 docs + gate
```

C3 can run in parallel with C1.

## Phases and checkpoints

### Phase A — Core (C1)
**Checkpoint:** criteria 1–4 pass; the unmodified `TestLoadExample` and `plan` tests pass.

### Phase B — Surfacing (C2, C3, C4)
**Checkpoint:** criteria 5–8; `wendy plan`, `wendy list`, TUI and web show parts.

### Phase C — Docs and gate (C5)
**Checkpoint:** `make test vet lint` pass.

## Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Re-assembling per selection changes combination results for existing matrices | High: silent behavior change | Existing matrices have no selections, so the new path is skipped; golden-style test compares the playground's 16 resolved environments before and after |
| Struct patch and selection on the same kind in one value (`workload: "x"` and, separately, `workload: cni: "cilium"`) | Medium: ordering surprises | A single value cannot hold both (one key, one type); across axes, selection applies first, patches after, documented and tested |
| Position of an axis value is missing for values built by CUE comprehension | Low | Fall back to the matrix's position |
| Plan output gets noisy | Low | Decision 5 |
| UI modules later want more than names | Low | `Combo.Parts` is a plain map; extend then |

## Parallelism

C1 and C3 are independent. C2 needs C1 (the resolver stores the names). C4 needs C2. C5 is last.
