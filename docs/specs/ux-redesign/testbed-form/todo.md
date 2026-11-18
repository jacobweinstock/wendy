# Tasks: testbed-form — V1

From [plan.md](plan.md) and [spec.md](spec.md). Criteria numbers refer to the spec's Success Criteria.

All five tasks are done. Notes from building them:

- **T1:**
  - The glossary guard reads "that runs" as "run" used as a noun, so the description says "shares the kind cluster
    with Cluster API".
  - NAT64's default reads "on with IPv6".
- **T2:**
  - The radios have no `title` tooltip holding the CUE value, as options do. The guard counts a label's title as
    text people read, and screen readers may read it.
  - The summary reply now carries the picker's Open link out of band. Before, the link kept pointing at the first
    part after the choice changed.
- **T3:**
  - The dialog's form posts `then=picker` through `hx-vals`.
  - `formErrorAt` sends errors to `#dialog-error`.
  - The reply's `HX-Trigger-After-Settle: {"partSaved":{"kind":…}}` closes the dialog and focuses the picker.
  - The dialog empties when it closes.
- **T4:** the guard listens on the document for `input` and `change` from `form[data-unsaved]`. A saved part sets it
  too, since the testbed form then picks a part it hasn't saved.
- **T5:**
  - The live check ran at `192.168.2.50:7601` against a temporary `XDG_CONFIG_HOME` and working directory:
    - a part made in the dialog kept the testbed's name and ISO boot, and was picked;
    - leaving asked first, and saving didn't;
    - the UI file held only `network.family: "ipv6"`.
  - The check also found the `.top` style clash, described in the spec.

- [x] **T1. Words**
  - Acceptance:
    - The testbed settings are labelled Where Tinkerbell runs, How machines boot and IP version. Each choice has a
      description, and the advanced settings have help.
    - The testbed blurb names a testbed's parts.
    - `ui.DefaultLabel` gives "<choice> (default)", and "Yes (default)" or "No (default)" for yes/no settings.
  - Verify: `go test ./internal/vocab ./internal/ui/...`, including the retired-words checks.
  - Files: `internal/vocab/{choices.go,kinds.go}`, `internal/ui/fields.go`, `internal/ui/tui/edit.go`, tests.
  - Criteria: 3, 4.

- [x] **T2. Radios, dropdowns and pickers**
  - Acceptance:
    - The three settings are radio lists, with each choice's description and the default marked. The default
      posts "".
    - Dropdowns don't list their default twice.
    - Each picker shows its kind's blurb, and its fallback reads "<name> (default)".
  - Verify: tests in `edit_test.go`, and the label check in `a11y_test.go`.
  - Files: `internal/ui/web/{edit.templ,edit.go}`, `static/wendy.css`, tests.
  - Criteria: 3, 4, 5.

- [x] **T3. The part dialog**
  - Acceptance:
    - New … part… opens the part's form in a dialog. Save part saves the part, and the reply is that kind's picker
      with the new part picked.
    - Errors go to the dialog. The dialog's IDs don't clash with the testbed form's.
  - Verify: tests in `edit_test.go` for the dialog's fragment, a save with `then=picker`, and a rejected part. The
    label check covers the dialog.
  - Files: `internal/ui/web/{edit.templ,edit.go,urls.go,api.go}`, `static/{app.js,wendy.css}`, tests.
  - Criteria: 1.

- [x] **T4. Not losing work**
  - Acceptance:
    - Edit forms carry `data-unsaved`, and a changed form asks before the page unloads, except after a save or a
      delete.
    - Open and Steps open in a new tab.
  - Verify: a test that the forms carry the marker and the links their target; a live check in a browser.
  - Files: `static/app.js`, `internal/ui/web/{edit.templ,run.templ}`, tests.
  - Criteria: 2.

- [x] **T5. Docs**
  - Acceptance: README "Editing in the UIs" and `design.md` §11 describe the dialog and the new labels.
    `docs/README.md` lists this change.
  - Verify: `make vet lint test`, the generated-template hash check and a live check.
  - Files: `README.md`, `docs/design.md`, `docs/README.md`.
  - Criteria: 6, 7.
