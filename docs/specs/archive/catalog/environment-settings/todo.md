# Tasks: environment-settings — V1

From [plan.md](plan.md) and [spec.md](spec.md). Criteria refer to the spec's Success Criteria, in order.

- [x] **S1. Schema, `EnvSpec`, `assemble`, `pick`, `ownerOf`, hints**
  - Done: `#Environment`/`EnvSpec` take `topology`, `bootMode`, `network`; `assemble` routes them and the Providers part's `management`; `checkMoved` (named, unused and inline Tinkerbell parts) reports the four old keys with where each moved; `ownerOf` reports the settings at the environment; the old-environment hint sends `management` to a providers part and `network` is no longer an old field. Golden unchanged. Tests: settings, invalid value location, moved keys, management image.
  - Acceptance: `#Environment` and `EnvSpec` take optional `topology`, `bootMode`, `network`; `assemble` routes them (and the Providers part's `management`) into the resolved environment, so the quickstart golden is unchanged; `pick` rejects `topology`, `bootMode`, `network`, `management` in a Tinkerbell part with a message saying where each moved, at the part's position; errors in the moved fields report at `environments.<name>.<field>` (or the Providers part for `management`); a matrix axis patching `tinkerbell.bootMode` still works; an external-topology environment resolves. Built-in parts and `.wendy/wendy.cue`/starter examples load. Criteria 2, 3, 4, 5.
  - Verify: `go test -race ./internal/config/ ./internal/plan/`
  - Files: `cue/wendy/schema.cue`, `internal/config/resolve.go`, `internal/config/load.go`, `internal/config/resolve_test.go`, `internal/config/testdata/*`

- [x] **S2. Field lists**
  - Done: the five settings are in `EnvironmentFields`; `management.image` is in `ProvidersFields`; the four source fields are Advanced; `NormalizePart` and its caller are gone (a Tinkerbell part no longer holds the topology; an external environment's cluster defaults in the schema). Field tests follow the new ownership.
  - Depends on: S1
  - Acceptance: `TinkerbellFields` has no topology, boot mode, network or `management.image`; `ProvidersFields` has `management.image` (Advanced); the four source fields are Advanced; `EnvironmentFields` has description, topology, boot mode, IP family (common), NAT64, mirror, upstreams (Advanced), with the old labels, options and defaults; the field-coverage tests (every schema setting has a field, defaults match the schema) pass with the new ownership; `NormalizePart` and `Missing` still treat `capt.version`/`capt.source` correctly. Criterion 7.
  - Verify: `go test -race ./internal/ui/ ./internal/service/`
  - Files: `internal/ui/fields.go`, `internal/ui/fields_test.go`, `internal/service/edit_test.go`

- [x] **S3. Web forms and summaries**
  - Done: the environment form has a Settings card (common fields, Advanced fold that opens when set); `commonFields` leaves the description to its own card; Tinkerbell summary shows chart or build; Tinkerbell, Providers and Environment blurbs updated; templates regenerated. `TestEnvironmentEditorSavesSettings`.
  - Depends on: S2
  - Acceptance: the environment editor has a "Settings" section after the pickers (common fields visible, Advanced folded, folded open when set); saving writes the five settings and leaves unrelated fields (inline parts, unknown keys) intact; Tinkerbell form lacks them; Providers form shows the node image under Advanced; the Advanced fold opens when a source is set; `partSummaryOf` for Tinkerbell shows chart or build; environment views show topology, boot mode and IP family; part blurbs updated; a11y test passes; templates regenerated. Criteria 7, 8.
  - Verify: `make generate && go test -race ./internal/ui/web/`
  - Files: `internal/ui/web/edit.templ`, `internal/ui/web/views.templ`, `internal/ui/web/{edit.go,helpers.go,pages.go}` plus tests and generated `*_templ.go`

- [x] **S4. Docs and templates**
  - Done: README, `design.md` (concepts and settings table), `cue/starter.cue.tmpl` and `.wendy/wendy.cue` (identical). `make test vet lint` pass; `wendy vet` is clean.
  - Depends on: S3
  - Acceptance: README, `design.md`, `cue/starter.cue.tmpl` and `.wendy/wendy.cue` (identical) show the new placement; no doc says boot mode, topology or network belong to a Tinkerbell part. Full gate passes. Criterion 1.
  - Verify: `make test vet lint`; `diff cue/starter.cue.tmpl .wendy/wendy.cue`
  - Files: `README.md`, `design.md`, `cue/starter.cue.tmpl`, `.wendy/wendy.cue`

### Checkpoint
`make test vet lint` pass and templates current; `spec.md` marked `Status: implemented`.
