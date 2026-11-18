# Spec: editor-service — V1

Module of [map.md](../map.md) (see its amendment). Depends on: catalog-model, composition-plan, run-options (all implemented). Required by: web-catalog-ui, tui-editor.

Status: implemented. Archived. Plan: [plan.md](plan.md); tasks: [todo.md](todo.md).

## Objective

Make environments and the four parts editable through the service layer both UIs share, and fix the field model they build forms from, so each UI module only has to draw forms.

Four pieces:

1. **Editable kinds.** `config.Kinds` and the service accept `environments`, `tinkerbells`, `providers`, `hardware` and `workloads` beside scenarios and suites: create, edit, copy, rename and delete, written to `wendy-ui.cue`.
2. **References and "used by".** An environment's part names must exist; a part cannot be deleted or renamed away while something uses it. The answer to "what uses this?" is one exported call both UIs show on detail pages.
3. **Field model corrections.** Fix what the earlier modules left inconsistent, and mark rarely-changed fields so a form can fold them away.
4. **Run evidence override.** `plan.Selection` can carry an evidence override, so a run form can change evidence for one run (`run-options` deferred this to the first UI that needs it).

**User:** the two UI modules, and through them anyone editing config without writing CUE.

### Defects in the current field model (found while reading the code)

- `ui.EnvironmentFields` still lists `evidence.consoles` and `evidence.dump`, but `run-options` made environments reject `evidence`. A form built from the list would fail on save. Evidence belongs to the profile, so the environment's own fields reduce to `description`, and a `ProfileEvidenceFields` list holds the two evidence settings for run forms.
- `TestEveryEnvironmentFieldHasOneHome` still counts those two fields as part of the environment; it moves to the profile list.

### Acceptance stories

1. As a UI, I save `{hardware: "ha", workload: "k8s-1.37"}` as environment `x`; the catalog then has environment `x` resolved from those parts, and `wendy-ui.cue` holds only what I set.
2. As a UI, saving an environment that names a part nothing defines is rejected with the unknown name, a suggestion, and no file written.
3. As a UI, deleting or renaming a part that an environment or a matrix axis uses is refused with the names of what uses it, as it is for scenarios used by suites today.
4. As a UI, `UsedBy(kind, name)` tells me what refers to any entry, so a detail page can show it.
5. As a UI, I can start a run with `evidence.dump = "always"` without editing a profile, and the plan is marked as edited.

## Tech Stack

Unchanged: Go 1.26, CUE v0.17.1. No new dependencies.

## Commands

```
Test:       make test
Lint/Vet:   make lint && make vet
Per pkg:    go test -race ./internal/service/ ./internal/config/ ./internal/ui/ ./internal/plan/
```

## Project Structure

```
internal/config/edit.go    → Kinds gains environments and the part kinds
internal/service/edit.go   → SaveEntry/DeleteEntry/prepare/refs for the new kinds; exported UsedBy; usedBy covers parts, environments, matrices
internal/ui/fields.go      → Field.Advanced; EnvironmentFields (description only); ProfileEvidenceFields; part reference helpers
internal/plan/plan.go      → Selection.Evidence; effective() applies it and marks the plan edited
internal/ui/web, ui/tui    → compile-only changes (the removed environment cases come back in their own modules)
```

## Code Style

Follows the repo. The shape of the central addition:

```go
// UsedBy names what refers to an entry, e.g. "environment ha" or
// "matrix versions". Parts are used by environments and by matrix axes that
// pick them; environments by profiles and matrices; scenarios and suites as
// before.
func (s *Service) UsedBy(kind, name string) []string

// Selection gains an optional override of the profile's evidence.
Evidence *config.Evidence `json:"evidence,omitempty"`
```

References are checked where suites' scenarios are today (`Service.refs`), against the catalog together with the spec being saved, so an environment can name a part created in the same edit session.

## Testing Strategy

Go table tests beside the code.

- `internal/service`: create, rename, copy and delete for an environment and for each part kind; unknown part name rejected with suggestion and no write; delete/rename refused while used (by an environment, by a matrix axis, by a profile); copy of a built-in part and of the built-in `quickstart` environment loads back identically.
- `internal/config`: `Kinds` contents; `Taken` and `SpecOf` already cover the part kinds.
- `internal/ui`: each environment-form field has one home (updated), `ProfileEvidenceFields` set and get round-trip, `Advanced` flags are set only on fields that exist.
- `internal/plan`: a `Selection.Evidence` overrides the profile's, applies to every combination, marks the plan edited and changes `Key`.
- Existing UI tests keep passing; the web and TUI environment 404/no-action tests are replaced in the UI modules.

## Boundaries

- **Always:** keep the UI file sparse (only what was set); never write a reference that does not resolve; give every refusal a message naming what blocks it; keep the resolved `config.Environment` shape; run `make test vet lint`.
- **Ask first:** editing matrices or profiles through the service (deferred below); letting the UI shadow a built-in name such as `default`; changing `Selection`'s JSON beyond adding `evidence`.
- **Never:** draw any form, page or TUI screen here; add a part or kind beyond the five; make the service resolve environments itself (the loader does that).

## Success Criteria

1. `config.Kinds` lists `environments`, `tinkerbells`, `providers`, `hardware`, `workloads`, `scenarios`, `suites`.
2. For each new kind, `SaveEntry` creates, `Entry` reads, a rename moves, a copy duplicates and `DeleteEntry` removes, and the result loads with `AllErrors()` empty.
3. An environment naming an unknown part is rejected with `unknown <kind> "<name>"` and a "did you mean" suggestion; nothing is written. A part defined by another file, or created in the same spec, resolves.
4. `DeleteEntry` and a renaming `SaveEntry` refuse while `UsedBy` is non-empty, and the error lists it. `UsedBy` covers: part ← environments and matrix axes; environment ← profiles and matrices (`base`); scenario ← suites; suite ← profiles and matrices.
5. `ui.EnvironmentFields` has only `description`; `ProfileEvidenceFields` has the two evidence settings; the union of all field lists still equals the original 46 settings plus `description`.
6. `ui.Field.Advanced` is set on the fields that a typical environment never changes (listed in the plan), and no list has an `Advanced` field that is `Required`.
7. `plan.Selection.Evidence` overrides the profile's evidence for every combination, sets `Edited`, and changes the plan `Key`; without it nothing changes.
8. `make test`, `make vet`, `make lint` pass.

## Out of Scope (Deferred)

- Editing matrices and profiles through the service or UIs (written in CUE): add when asked; the service shape here extends to them.
- A UI way to change a built-in's name such as `default` (see Open Questions).
- Preview of a not-yet-saved environment's resolved result: needs a public single-environment resolve in `config`.
- Moving a part between kinds, or bulk operations.

## Resolved Questions

1. The UI does not save a part under a built-in's name; changing a built-in `default` stays a CUE edit.
2. `Advanced` is a flag on `ui.Field`, shared by both UIs.
