# Plan: testbed-form — V1

From [spec.md](spec.md). Tasks: [todo.md](todo.md).

## Components

1. **Words**, in `internal/vocab` and `internal/ui`:
   - A choice gets an optional description beside its label, read through `vocab.ChoiceHelp`.
   - The three testbed settings get their new labels, and the advanced ones get help.
   - `ui.DefaultLabel` says how a form names a field's default, so the web UI and the TUI read the same.
2. **Settings as radios**, in `edit.templ`: `fieldRow` lists an `FSelect` as radios when every choice has a
   description. The default's radio posts "", so saving stays sparse. A dropdown no longer lists its default twice.
3. **Part pickers**: the kind's blurb under the label, and the fallback read as "<name> (default)".
4. **The part dialog:**
   - `GET /edit/part-dialog?kind=…` renders the part form in a `<dialog>` outside `#editform`, since forms can't
     nest. The form's element IDs get a prefix so they don't clash with the testbed form's.
   - The dialog posts to the usual `POST /edit/{slug}` with `then=picker`. On success the reply re-renders that
     kind's picker with the new part picked (`HX-Retarget`), and `HX-Trigger-After-Settle: partSaved` makes
     `app.js` close the dialog and focus the picker. On error, `formError` targets the dialog's own error area.
5. **Not losing work**, in `app.js`:
   - A form marked `data-unsaved` that has changed asks before the page unloads.
   - `htmx:beforeOnLoad` clears the flag when the reply redirects, which a save or a delete does. htmx 2.0.4 fires
     it before following `HX-Redirect`.
   - Links in forms that lead elsewhere get `target="_blank"` and an ↗.

## Order

Words → radios and pickers → dialog → guard and links → docs. Each step builds and passes on its own.

## Risks

- **Duplicate IDs** between the dialog and the testbed form would break label clicks. `editData.IDPrefix` prefixes
  every ID the part form writes. A test checks that the dialog's IDs are prefixed.
- **Tests that assert old wording** (`default: …`, `used when a testbed picks none`, `Boot mode`) are updated to the
  new wording, not deleted.
- **`beforeunload`** must not fire after a save. Clearing the flag in `htmx:beforeOnLoad` handles this; the live check
  confirms it in a browser.
