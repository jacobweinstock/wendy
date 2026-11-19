# Spec Delta: delete-where-created — V1

Status: implemented. Plan: [plan.md](plan.md); tasks: [todo.md](todo.md).

A delta to [web-ux/spec.md](../web-ux/spec.md), [plans-and-matrices](../plans-and-matrices/spec.md) and
[testbed-form](../testbed-form/spec.md), all implemented. It comes from this request: "all of the locations that
provide the ability to create objects (Plans, Testbed, Suites, etc) should have the ability to delete any user
created objects too."

## Objective

Wherever the web UI can create an entry, you can also delete one you made. Delete sits where you're looking at the
entry itself: on its page and in its edit form. A mistake should be as easy to undo as it was to make (NN/g, user
control and freedom), and an action that can't be taken should say why (NN/g, visibility of system status).

Revised after review: the first version also put Delete on forms that only *pick* an entry, the testbed form's part
pickers and Run a plan…'s Plan card. It was removed from both (see "Not on forms that pick").

## Current Behavior

These places create entries:

| Where | Creates | Delete today |
|---|---|---|
| Plans, Testbeds (and its five part subtabs), Suites, Tests and Steps: **＋ New …** | that kind | only inside **Edit …**, in the form's save bar |
| The testbed form's pickers: **New … part…** (dialog) | parts | none |
| Run a plan…: **Save as plan…** | plans | none |
| TUI catalog: `a` | tests, suites | `D` |

The TUI already deletes where it creates, so it doesn't change.

## New Behavior

"Yours" means defined in the UI file (`wendy-ui.cue`). The UIs never change your own CUE files, and built-ins can't
be deleted.

- **Every subtab's detail pane** gets **Delete <kind>** after Edit for an entry of yours. It asks first. It then
  shows the subtab's list, as deleting from the form does. While something uses the entry, the button is disabled
  and says what uses it, as in the edit form.
- An entry from one of your own files shows no Delete button. It says "Defined in <file>, which Wendy doesn't
  change; delete it there."
- Every delete goes through `POST /edit/{slug}/{name}/delete` and `service.DeleteEntry`, with the same checks, and
  then shows the kind's list.

### Not on forms that pick

The testbed form's part pickers and Run a plan…'s Plan card make or pick an entry, but don't delete one:

- **Wrong object.** The testbed form edits a testbed and the run form runs a plan. A part is shared by every
  testbed that picks it, so deleting it there acts on something other than what the page is about.
- **Misleading.** Next to a picker, "Delete" reads as "take this out of my testbed".
- **It skips Save and Cancel.** Everything else on the testbed form waits for Save, and Cancel drops it. A delete
  would happen at once, and Cancel couldn't undo it.
- **Easy to misclick.** A destructive link between two harmless ones (Open ↗ and New … part…), there for some
  parts and not others.
- **Not needed.** Open ↗ opens the part's page in a new tab, where Delete is, and the testbed form stays as it was.
  A plan's page is one click from Plans.

## Success Criteria

1. On each subtab, an entry of yours that nothing uses has an enabled Delete with a confirmation, and deleting it
   shows the list without it.
2. An entry of yours that something uses has a disabled Delete that names what uses it. An entry from your own file
   has none, and says where to delete it. A built-in has neither.
3. The testbed form's part pickers offer Open ↗ and New … part…, never Delete, for any part.
4. Run a plan… offers no Delete, even for a plan of yours.
5. The glossary guard and the label check pass, as do `make vet lint test`, and the generated templates are up to
   date.
6. README and `design.md` §11 say where entries can be deleted.

## Boundaries

Everything else is inherited from [web-ux/spec.md](../web-ux/spec.md).

- **Never:** delete from or change a hand-written CUE file; delete a built-in.
- **Ask first:** deleting several entries at once.

## Out of Scope (Deferred)

- Undo after a delete. The confirmation guards against mistakes, and the entry can be made again.
