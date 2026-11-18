# Plan: environment-settings — V1

From [spec.md](spec.md) (approved). Tasks: [todo.md](todo.md).

## Approach

The resolved environment keeps its shape, so only the front door changes: where settings are written and how they are routed into it. Work goes bottom-up: schema and resolver, then field lists, then the web forms, then docs.

## Components

1. **Schema and `EnvSpec`.** `#Environment` gains optional `topology`, `bootMode` and `network` (open struct; `#ResolvedEnvironment` still validates the values and supplies defaults). `config.EnvSpec` gains the same three fields.
2. **`assemble` and `pick`.** `assemble` takes the environment's settings as well as the parts: `topology` and `bootMode` go into the `tinkerbell` section, `network` to the top level; the Providers part's `management` goes to the top level, not `capi`. The schema would accept a Tinkerbell part's `topology` and `bootMode` (they are still valid resolved fields), so `pick` rejects the four old keys (`topology`, `bootMode`, `network`, `management`) in a Tinkerbell part directly, with the migration hint, at the part's position. One mechanism for all four.
3. **`ownerOf`.** Errors at `network`, `tinkerbell.topology` and `tinkerbell.bootMode` report at the environment (no part kind); `management` reports at the Providers part. `ownerOf` checks these before the existing mapping.
4. **Field lists.** `TinkerbellFields` loses five groups of fields; `ProvidersFields` gains `management.image` (Advanced); source fields on both get `Advanced`; `EnvironmentFields` gains topology, boot mode and IP family (common) and NAT64, mirror, upstreams (Advanced). The existing `applyForm` already sets `EnvironmentFields` from `f.<path>` form values, so saving needs no new code.
5. **Web.** The environment editor renders `EnvironmentFields` in a "Settings" section after the pickers (reusing the part form's field and Advanced rendering). `partSummaryOf` for Tinkerbell shows chart or build only. Part blurbs, the environment detail view and its list row show topology, boot mode and IP family from the resolved environment.
6. **Docs/templates.** README, `design.md`, `cue/starter.cue.tmpl` and `.wendy/wendy.cue` (identical), part blurbs.

## Order and parallelism

S1 (schema/resolver) → S2 (field lists) → S3 (web) → S4 (docs). S2 can start with S1 but its tests need the resolver. All sequential; each is small.

## Risks

- **Hidden required field:** `capt.version` is `RequiredUnless: capt.source`; with source now Advanced the form must still treat the pair correctly and the Advanced fold must open when a source is set. Verify in S2/S3 tests.
- **Matrix patches on moved fields:** unaffected because they patch the resolved shape; a test pins it.
- **Existing configs:** old Tinkerbell parts with the moved keys now error; the hint is the migration path. The built-in `default` parts, the starter and `.wendy/wendy.cue` are updated in S1/S4 so a fresh `wendy init` loads.
- **`tinkerbell.cluster`** (external topology) stays in the Tinkerbell part; its schema condition reads the environment's `topology`, which is merged before evaluation, so it still works. A test covers an external environment.

## Checkpoints

After S1: `go test ./internal/config/` and the golden pass. After S2: `go test ./internal/ui/`. After S3: web tests, `make generate` current. After S4: `make vet lint test`.
