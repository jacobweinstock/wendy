# Tasks: web-catalog-ui — V1

From [plan.md](plan.md) and [spec.md](spec.md). Criteria numbers refer to the spec's Success Criteria. Starts after editor-service is done.

## Phase A — Navigation and read-only

- [x] **W1. Grouped tabs, the four part tabs and their lists**
  - Done: `catalogTabList` table (groups, URL name, label, blurb, entries) drives the group row, tab row, blurbs and lists, replacing the per-tab `switch`es; group row (Profiles, Build, Test) with `aria-current`; ten tabs fit in the Build row without wrapping because only one group's tabs show; table-integrity and rendering tests in `pages_test.go`.
  - Acceptance: `catalogTabs` is a table of groups (Profiles; Build: Tinkerbell, Providers, Hardware, Workloads, Environments, Matrices; Test: Suites, Scenarios, Steps) with kind, URL name, singular label and blurb; one lookup per tab replaces the `switch`es in `catalogPage`; the page shows a group row and, for Build and Test, a tab row; every existing `/catalog/<tab>` URL still works and an unknown tab falls back as today; the four part tabs list entries by origin with the filter box; each tab has a one-sentence blurb in the "A X is …" form; the Environment blurb describes parts. CSS for the group row uses theme variables. Criteria 2 (nav), spec story 4.
  - Verify: `make generate && go test ./internal/ui/web/`
  - Files: `internal/ui/web/pages.go`, `views.templ` (+ generated), `static/wendy.css`, `internal/ui/web/pages_test.go` (new)

- [x] **W2. Detail pages for parts, environments and matrices**
  - Done: part pages list what the part sets (rows from the shared field lists, machines one row per role), "Used by" links, and say "Sets nothing" or "not used" when empty; environment pages link each part (inline ones read "inline") and show "Used by"; matrix pages have a "Base" link. The composition module's plain-text parts-row test was replaced by link tests.
  - Depends on: W1
  - Acceptance: a part's page shows its set fields (from the shared field lists), "Used by" as links (environments, matrices) from `Service.UsedBy`, the actions row and "View CUE source"; the environment page links each part to its page; the matrix page has a "Base" row linking to its base environment; entries with no users say "not used". Criterion 1, 5.
  - Verify: `make generate && go test ./internal/ui/web/`
  - Files: `internal/ui/web/pages.go`, `helpers.go`, `views.templ` (+ generated), `pages_test.go`

## Phase B — Editing

- [x] **W3. Part editors**
  - Done: the four parts join `editable`; `applyForm` uses `ui.PartFields`; common fields, hardware machine table (blank cells unset, labelled by role and column) and an "Advanced settings" fold that opens when an advanced value is set; create, edit, rename, copy and delete end to end, with a used part refused and listing its users; required fields rejected under the form; cross-site POSTs refused. The earlier 404 test for `/edit/hardware` was updated.
  - Depends on: W2
  - Acceptance: `/edit/<part kind>[/name]` and the save and delete routes work for the four part kinds, which join the web `editable` list in this task; the form shows common fields then `details.adv` (open when an advanced field has a value); Hardware shows the machine table (blank cells unset, integers posted as integers); `applyForm` is generalized through `ui.PartFields`; create, edit, copy, rename and delete work end to end, and deleting a used part is refused with its users; the "＋ New" button and "Edit this…/Copy to edit…" appear on the part tabs. Criteria 3, 6.
  - Verify: `make generate && go test -race ./internal/ui/web/ ./internal/service/`
  - Files: `internal/ui/web/edit.go`, `edit.templ` (+ generated), `pages.go`, `static/wendy.css`, `internal/ui/web/edit_test.go`

- [x] **W4. Environment editor**
  - Done: `environments` joins `editable`; the form is description plus four labelled pickers (built-in default first), each with a summary rendered on load and refreshed through `GET /edit/part-summary` (HTML-escaped, 404 for an unknown kind), "Open" and "New…" links; "default" is saved as no choice so the file stays sparse; an inline part is offered as "inline (written in the config file)", carried in a hidden field and written back unchanged, or replaced by a named part; an unknown part is rejected under the form. The old 404 guard became a test that only kinds with forms are editable.
  - Depends on: W3
  - Acceptance: `environments` joins `editable`; the form has name, description and four pickers (catalog parts of the kind, built-in `default` first), each with an edit link, a "New…" link and a one-line summary that updates on change through `GET /edit/part-summary` and is also rendered on load; an inline part is shown as "inline (written in the config file)" and survives an edit unchanged unless a named part is chosen; saving writes only references (and any preserved inline part); a reference to a missing part is rejected under the form. Criterion 4.
  - Verify: `make generate && go test -race ./internal/ui/web/`
  - Files: `internal/ui/web/edit.go`, `edit.templ` (+ generated), `static/wendy.css`, `static/app.js` (only if the summary needs more than `hx-trigger`), `edit_test.go`

## Phase C — Run form and finish

- [x] **W5. Run-form evidence control**
  - Done: "Dump evidence" select and "Machine consoles" checkbox in the How card, starting at the profile's evidence (defaults with no profile), posted with an `evidenceSet` marker; `formSelection` drops an override equal to the profile's (or default), so an untouched form stays an unedited plan; a bad dump value is refused; the plan preview now shows "(edited)".
  - Depends on: editor-service E3, W4
  - Acceptance: the "How" card has consoles and dump controls starting at the selected profile's evidence (schema defaults with no profile); `formSelection` sets `Selection.Evidence` only when they differ, so an untouched form stays an unedited plan; the plan preview marks an edited plan. Criterion 7.
  - Verify: `make generate && go test ./internal/ui/web/`
  - Files: `internal/ui/web/views.templ` (+ generated), `api.go`, `pages.go`, `api_test.go` (or `pages_test.go`)

- [x] **W6. Cleanup, accessibility, docs and gate**
  - Done: stepper CSS and script removed (and the unused `--num-bg`); `a11y_test.go` checks every control on the editors, step fragment, run form and catalog has a label, with negative cases so the checker can fail. It found and I fixed: the command-palette input (on every page), the step-kind select, and five run-form fields whose `<label>` had no association. README rewritten for the new tabs, editors and run control. `make vet`, `make lint` (0 issues) and `make test` (race) pass; templates current.
  - Depends on: W5
  - Acceptance: the stepper rules (`.stepnav`, `.stepbtn`, `.pane`, `.panenav`) and the stepper script are removed with no caller left; one test checks every input in every editor page and the run form has a label; README's Catalog and Editing sections describe the new tabs, editors and run-form control; `make test vet lint` pass; generated templates current; I list what to look at in a browser. Criteria 8, 9, 10.
  - Verify: `make test && make vet && make lint`; checksum check of generated files; `grep -n "stepnav\|showPane" internal/ui/web/static/*` is empty
  - Files: `internal/ui/web/static/wendy.css`, `app.js`, `internal/ui/web/a11y_test.go` (new), `README.md`

## Notes

- Commit per task with `git commit -s`; ask before every git command.
- Edit files with the edit tool, one edit per call; regenerate templates only with the pinned `make generate`.
