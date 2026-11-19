# Plan: delete-where-created — V1

From [spec.md](spec.md). Tasks: [todo.md](todo.md).

## Components

1. **One delete button**, `deleteButton(kind, name, usedBy, file)` in `edit.templ`. It is the edit form's
   `deleteAction` moved out so the detail panes share it.
2. **Detail panes:** `entryActions` in `config.templ` replaces the six `@editAction` calls: Edit or Copy and edit…,
   then Delete for an entry of yours, or the "delete it there" line for one from your own files.
3. **Removed after review:** the pickers' Delete (`deleteLink`, `editData.Removable`, `editDelete`'s `then=picker`)
   and the run form's Delete plan (`runTestsData.FromYours`, `then=run`). The spec's "Not on forms that pick" says
   why. The dialog's `partSaved` event stayed renamed `partPicked`, dispatched on the new select, since that still
   reaches `app.js` after the swap.

## Order

Shared button and detail panes → docs.

## Risks

- **`HX-Trigger-After-Settle` on a removed element** wouldn't reach `app.js`, so the dialog's event names its target,
  the new select.
