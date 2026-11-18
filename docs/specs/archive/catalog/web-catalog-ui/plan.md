# Plan: web-catalog-ui — V1

Implements [spec.md](spec.md). Tasks: [todo.md](todo.md).

Status: proposed (awaiting review). No code until this plan is approved. Starts only after editor-service is done.

## Approach

Build it in the order a user meets it: navigation and read-only pages first (so every later step has somewhere to land), then editors one kind at a time, then the run form, then cleanup.

```
W1 nav + tabs + list pages ─▶ W2 detail pages ─▶ W3 part editors ─▶ W4 environment editor ──┐
                                                                                            ├─▶ W6 cleanup + a11y + docs + gate
editor-service E3 ─▶ W5 run-form evidence ─────────────────────────────────────────────────┘
```

## Design decisions to confirm

1. **Tab model.** `catalogTabs` becomes an ordered list of groups, each with ordered tabs (kind, URL name, singular label, blurb). `Profiles` is a group of one. A page's group is derived from its tab, so URLs do not change. One table drives the group row, the tab row, the blurbs and the list data, instead of the parallel `switch`es in `catalogPage` today.
2. **List data per tab.** `catalogPage` builds `origins` with a `switch` per tab; the four parts add four more cases. Replace with one lookup: each tab names how to get `name → origin` (a small func in the tab table), so adding a tab is one table row.
3. **Detail pages** for parts show a table (the part's set fields, formatted by the shared field lists, so it cannot drift from the editor), then "Used by" (links to the environments and matrices), then the existing actions row. "Copy to edit…" and "Edit this…" follow the existing `Editable` logic.
4. **One part form, one template.** Tinkerbell, Providers and Workload use the same component (common fields, then `details.adv` for Advanced); Hardware adds the machine table. The form posts `f.<path>` like the suite and scenario forms, and `applyForm` is generalized to look fields up with `ui.PartFields(kind)`.
5. **Environment form.** Four `<select>`s named `p.<kind>` plus name and description. Options are the catalog's parts of that kind with the built-in `default` first. The summary under each is rendered by `GET /edit/part-summary?kind=&name=` (htmx, `hx-trigger="change"`), a plain text line built from the part's own values; the page renders the same line server-side on load, so it works without JavaScript. An inline part is carried in a hidden field as JSON and re-saved unchanged unless the select changes to a name.
6. **Run-form evidence.** Two controls in the "How" card, defaulting to the selected profile's evidence (or the schema default), posted as `consoles` and `dump`; `formSelection` sets `Selection.Evidence` only when they differ from that default, so an untouched form stays an unedited plan.
7. **Accessibility test is written once and runs over every editor page** (parse the HTML, check each `input`/`select`/`textarea` has a `label for` or `aria-label`), so later forms cannot regress.
8. **Visual review is manual.** The integrated browser cannot reach this host, so each UI task ends with a short "what to look at" list in my report and with the HTML checked by tests.

## Phases and checkpoints

### Phase A — Navigation and read-only (W1, W2)
**Checkpoint:** all eleven tabs render and link; blurbs present; detail pages show parts and used-by; no editor changes yet.

### Phase B — Editing (W3, W4)
**Checkpoint:** create, edit, copy and delete a part of each kind, and create an environment from parts, through the forms.

### Phase C — Run form and finish (W5, W6)
**Checkpoint:** `make test vet lint` pass; generated templates current.

## Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Large `.templ` edits are easy to break and hard to see in a diff | Medium | One task per page family; regenerate and run the web tests after each; `go vet` on generated code |
| Eleven tabs overflow the row | Medium | The group row (decision 1); a width-conscious CSS check by class, not pixels, in review notes |
| Hardware machine table posts integers; empty cells must unset | Medium | Reuse `ui.Set` semantics (blank unsets); a handler test posts mixed blank and set cells |
| An inline part is lost on environment save | High: silent config loss | Hidden JSON field and a test that an environment with an inline hardware part keeps it across an edit |
| Widening the web `editable` list before a handler exists | High | Each of W3/W4 adds its kinds to `editable` in the same task as its handler and tests |
| Dark theme regressions in new components | Low | New CSS uses variables only; a grep check for hex colors in new rules |

## Parallelism

W1 → W2 → W3 → W4 are sequential (shared files). W5 only needs editor-service's E3 and touches the run form, so it can run alongside W3/W4 if wanted; I will do it after W4 to keep one `.templ` change at a time.
