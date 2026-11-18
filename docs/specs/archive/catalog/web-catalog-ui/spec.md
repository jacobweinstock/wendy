# Spec: web-catalog-ui — V1

Module of [map.md](../map.md). Depends on: editor-service (proposed). Required by: nothing. Sibling of tui-editor.

Status: implemented. Archived. Plan: [plan.md](plan.md); tasks: [todo.md](todo.md).

## Objective

Let someone browse, compose and edit environments and their parts in the web UI, so that "same Tinkerbell, different cluster size" is a few clicks and no CUE.

The web UI gets:

1. **A catalog that shows the parts.** Four new tabs (Tinkerbell, Providers, Hardware, Workloads), each with a description and a detail page that shows what the part sets and what uses it.
2. **Part editors.** Create, edit, copy, rename and delete a part, with the settings people change often up front and the rest folded under "Advanced".
3. **An environment editor that picks parts.** Name, description, and one picker per kind, each with a one-line summary of the chosen part and links to edit or create one. No page of forty fields.
4. **Navigation that stays usable.** The catalog now has eleven tabs; they are grouped so the row does not overflow.
5. **A run-form evidence control.** The "How" card can override evidence for one run.

**User:** a developer running Tinkerbell end-to-end tests who would rather not write CUE to change a machine count or a Kubernetes version.

### Layout

```
Catalog     [ Profiles ]  [ Build ]  [ Test ]            ← group row (groups of tabs)
              Tinkerbell · Providers · Hardware · Workloads · Environments · Matrices   ← tabs of the selected group
```

- **Profiles** (a group of one): Profiles.
- **Build:** Tinkerbell, Providers, Hardware, Workloads, Environments, Matrices.
- **Test:** Suites, Scenarios, Steps.

URLs stay `/catalog/<tab>` and `/catalog/<tab>/<name>`; the group is derived from the tab. Tab URL names are the kind names (`tinkerbells`, `providers`, `hardware`, `workloads`) and their labels are singular in the tab row ("Tinkerbell", "Workload").

### Environment editor

```
Environment  [ name ] [ description ]

Tinkerbell   [ default (built-in)  ▾ ]   Edit · New…
             netboot · colocated · chart latest
Providers    [ default (built-in)  ▾ ]   Edit · New…
             Cluster API v1.14.4 · CAPT v0.7.0
Hardware     [ ha                  ▾ ]   Edit · New…
             control-plane 3 · worker 3 · spare 1
Workload     [ k8s-1.37            ▾ ]   Edit · New…
             Kubernetes v1.37.0 · cilium
```

The summary under a picker updates when the choice changes (an htmx request, no page reload). An environment written in CUE with an *inline* part shows that picker as "inline (written in the config file)", keeps the inline value on save, and offers a named part to replace it with.

### Acceptance stories

1. As a user, I open Catalog › Build › Hardware, press "＋ New hardware", set worker count 3 under the common settings, name it `ha`, and save; it appears in the list and on the Environments I pick it for.
2. As a user, I create an environment by naming it and choosing a hardware and a workload; I never see a Tinkerbell or Providers field.
3. As a user, on a hardware part's page I see which environments and matrices use it, and deleting it while used is refused with that list.
4. As a user, every tab explains in a sentence what it holds and how it is used.
5. As a user, on the New run page I can set evidence for this run (consoles, dump) with the selected profile's values as the starting point.
6. As a keyboard or screen-reader user, every field has a label, tabs are links, and nothing depends on color alone.

## Tech Stack

Go 1.26; `a-h/templ` v0.3.1118 (pinned in the Makefile); htmx (vendored); plain CSS with the existing theme variables (light and Tinkerbell dark). No new dependencies.

## Commands

```
Generate:   make generate          # templ, pinned version in out/tools
Test:       make test
Lint/Vet:   make lint && make vet
Run:        go run ./cmd/wendy serve --addr 127.0.0.1:7600
Check gen:  make verify-generate   # runs git; compare file checksums instead if git is not wanted
```

## Project Structure

```
internal/ui/web/pages.go       → catalogTabs, groups, labels, blurbs; per-part list data; part detail data
internal/ui/web/views.templ    → group row + tab row; part detail pages; environment detail with linked parts; matrix base row; run form evidence card
internal/ui/web/edit.go        → editRoutes for the new kinds; applyForm for parts and environments; summary endpoint
internal/ui/web/edit.templ     → part form (common + Advanced), hardware machine table, environment form with pickers
internal/ui/web/api.go         → formSelection reads the evidence override
internal/ui/web/static/        → wendy.css (group row, pickers; stepper rules removed), app.js (stepper removed)
internal/ui/web/*_test.go      → handler tests, below
README.md                      → the Catalog and Editing sections
```

## Code Style

Follows the repo: templ components, small handlers, `htmx` for partial updates, `hx-post` forms redirecting with `HX-Redirect`, errors shown under the form with `editErrorHTML`. Field rows reuse `fieldRow`; the folded group reuses `details.adv`. One representative piece:

```go
// partForm is what a part editor shows: the fields a typical part sets, then
// the rest under "Advanced". Both lists come from ui.PartFields.
type partForm struct {
	Kind          string
	Common, Rest  []ui.Field
}

func splitAdvanced(fields []ui.Field) (common, rest []ui.Field) {
	for _, f := range fields {
		if f.Advanced {
			rest = append(rest, f)
		} else {
			common = append(common, f)
		}
	}
	return common, rest
}
```

Colors, spacing and the dark theme come from the existing CSS variables; no hard-coded colors.

## Testing Strategy

Go `httptest` tests beside the handlers (the existing `editServer` helper), plus the repo's checks.

- **Catalog:** each new tab renders with its blurb, lists entries by origin, and its detail page shows "Used by"; group row marks the current group; unknown tab falls back as today.
- **Editors:** create/edit/copy/delete for one part kind end to end through the form; hardware machine table posts counts as integers; Advanced fields round-trip; an environment saved with named parts loads resolved; an inline part survives an environment edit; a rejected save writes nothing and shows the error under the form.
- **Safety:** cross-site POSTs are refused for the new routes (as today); deleting a used part is refused with the users listed.
- **Run form:** the evidence control posts into `Selection.Evidence` and shows the profile's values by default.
- **Accessibility:** a test asserts every input in the new forms has a matching `label for` (the same check, written once, over all forms).
- **Visual:** not browser-tested in this environment; I will list what to look at in the review message.

## Boundaries

- **Always:** keep the UI file sparse; keep forms working with JavaScript off (the summary line is an enhancement); use theme variables; give each tab a one-sentence description; run `make generate` and confirm generated files are current; `make test vet lint`.
- **Ask first:** adding matrix or profile editors; adding any new JS beyond a few lines; changing URL shapes of existing pages.
- **Never:** hard-code colors; add a dependency or npm tooling; edit hand-written config files; write forms with a column of unlabelled inputs.

## Success Criteria

1. `/catalog/tinkerbells|providers|hardware|workloads` render with a blurb, an item list and a detail page; the detail page shows the part's settings, "Used by" (environments, matrices), "Copy to edit…" or "Edit this…", and "View CUE source".
2. The catalog shows a group row (Profiles, Build, Test) and, for Build and Test, a tab row; every existing `/catalog/<tab>` URL still works; ten tabs never wrap on a 1280px viewport.
3. New/edit/copy/delete work for each part kind; the form shows the common settings first and the rest under "Advanced", which is open when an advanced field already has a value; hardware shows its machine table.
4. The environment form has name, description and four pickers; the picker lists the catalog's parts of that kind with the built-in `default` first; the summary under each updates on change; saving writes only the references (and any preserved inline part).
5. The environment detail page links each part to its page; the matrix detail page shows its base environment as a link.
6. Deleting or renaming a used entry is refused and lists the users.
7. The New run page has an evidence control (consoles, dump) that starts at the selected profile's values and, when changed, marks the plan "edited".
8. Every input in the editors and the run form has a programmatically associated label.
9. The stepper styles and script from the earlier long environment form are removed (no caller remains).
10. `make test`, `make vet`, `make lint` pass; generated templates are current.

## Out of Scope (Deferred)

- Matrix and profile editors: written in CUE; the service shape from editor-service extends to them.
- A live preview of the fully resolved environment while editing (needs a public resolve in `config`); the per-picker summary covers the common need.
- Returning to the environment editor after "New…" on a part (plain navigation in V1).
- Drag-to-reorder, bulk edit, or importing parts.
- The TUI: tui-editor.

## Resolved Questions

1. Navigation is a two-level row (groups, then tabs).
2. Inline parts in the environment editor are preserved and shown as "inline", replaceable by a named part, not editable in place.
3. The evidence control in the run form is in scope now.
4. The stepper CSS and script are removed.
