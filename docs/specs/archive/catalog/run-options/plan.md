# Plan: run-options — V1

Implements [spec.md](spec.md). Tasks: [todo.md](todo.md).

Status: proposed (awaiting review). No code until this plan is approved.

## Approach

Make every consumer independent of the environment's `evidence` and `upgrades` first, then delete them. Each task compiles and passes on its own; the removal is last and small.

```
steps stop reading Config.Upgrades ─┐
vet reads targets from step params ─┼─▶ remove from schema, types, resolver ─▶ docs + final gate
plan takes evidence from the profile┘
```

## Design decisions to confirm

1. **The resolved environment keeps `evidence`.** Steps (`hardware.create` reads `Evidence.Consoles`) and the engine (`Evidence.Dump`) read `config.Environment.Evidence`, and the `quickstart` golden includes it. So `#ResolvedEnvironment.evidence` stays, with its defaults. What changes is the source: the *user-facing* `#Environment` loses `evidence`, `#Profile` gains it, and the plan writes the profile's value into each combination's resolved environment. This corrects spec criterion 1, which said the resolved schema drops it; criterion 1 now reads: "`#Environment` rejects `evidence` and `upgrades`; `#ResolvedEnvironment` rejects `upgrades` and keeps `evidence` with defaults, set by the plan."
2. **A matrix axis that patches `evidence` is rejected.** Axis patches go over the resolved environment, so without a check a patch to `evidence` would be silently overwritten by the plan. The resolver reports it with the same migration message as the environment field. `upgrades` is already rejected by the schema once removed.
3. **`Selection` gets no evidence override** (deferred, as the spec says). The plan reads evidence from the effective profile; a selection with no profile gets the schema defaults.
4. **Testability:** the three upgrade steps decide to skip before touching the cluster. `tinkerbell.upgrade` does this inside an inline closure, so it is extracted to a named function, like the other two, to be tested directly.

## Phases and checkpoints

### Phase A — Stop reading the environment's fields (R1, R2, R3: independent)
Steps, `vet` and the plan each stop reading `Upgrades`/`Evidence` from the environment.
**Checkpoint:** all tests pass with both fields still in the schema; behavior for configs that never set them is identical.

### Phase B — Remove (R4)
Schema, types, resolver, migration messages, fixtures.
**Checkpoint:** criteria 1, 4, 8; `quickstart` golden unchanged.

### Phase C — Docs and gate (R5)
**Checkpoint:** `make test vet lint` pass; `grep` for removed names is empty.

## Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Evidence applied after resolution is missed by a path that builds a combination without the plan (e.g. `wendy env up`, `LoadEnv` from `environment.json`) | Medium: wrong dump setting on kept environments | Evidence is set inside `buildCombo`, which every path through `plan.New` uses; `environment.json` records the applied value; test `SetupOnly` |
| Removing `Environment.Upgrades` breaks the images check in `vet` | Medium | R2 moves it to step params before R4 deletes the field |
| Users relying on the environment `upgrades` fallback | Low (pre-1.0, approved) | R4's migration message names the new home |
| A matrix patching `evidence` silently ignored | Medium | Decision 2: explicit error |
| Built-in upgrade scenarios now always skip without a target | Low, same as today with `upgrades` unset | Skip message names the parameter to set; descriptions say so |

## Parallelism

R1, R2, R3 touch disjoint files (`internal/steps`, `internal/cli`, `internal/plan` + profile schema) and may run concurrently. R4 depends on all three; R5 on R4.
