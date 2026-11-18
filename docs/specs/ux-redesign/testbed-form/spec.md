# Spec Delta: testbed-form — V1

Status: implemented. Plan: [plan.md](plan.md); tasks: [todo.md](todo.md).

A delta to [web-ux/spec.md](../web-ux/spec.md) and [plans-and-matrices](../plans-and-matrices/spec.md), both
implemented. It comes from two problems found using the new-testbed form:

1. Making a part from the testbed form throws the testbed away.
2. The form's settings don't say what they mean or which to pick.

## Objective

A person new to Tinkerbell testing should be able to fill in a testbed from top to bottom without leaving the page.
They should also understand every choice well enough to pick one, or know that the default is fine.

The guidance this follows:

- **Don't lose people's work** (NN/g, error prevention). A side task, such as making a part, happens in place, in a
  modal dialog. Dialogs suit short tasks that must finish before the main one goes on (NN/g, "Modal & Nonmodal
  Dialogs"). Links that leave a half-filled form open in a new tab ("Opening Links in New Browser Windows and
  Tabs"). Leaving an edit form with unsaved changes asks first.
- **Explain choices where they are made** (NN/g, form design; GOV.UK, radios).
  - When the options are few and need explaining, they are listed as radios, each with a sentence saying what it
    does and when to pick it, rather than hidden in a dropdown.
  - Help text sits under the control, not in a tooltip.
  - The default is marked, since most people should keep it.
- **Plain words** (the glossary).
  - Labels say what is chosen: "Where Tinkerbell runs", not "Topology".
  - Hints speak to the person filling in the form, not about the data model ("More than one part reads these").

## Current Behavior

- Under each part picker are links to **Open** the part and make a **New … part…**. Both go to another page, and
  what was typed in the testbed form is lost.
- The **Settings** section is three dropdowns, Topology, Boot mode and IP family, under the hint "More than one part
  reads these, so the testbed holds them." The first option reads "default: On the management cluster", and the same
  choice is listed again below it. Nothing says what a choice does.
- A part picker's first option reads "chart-latest: used when a testbed picks none". Nothing says what a kind of
  part is.
- Every dropdown in every form shows its default as "default: <choice>" and lists the choice a second time. The TUI
  does the same.

## New Behavior

### Making a part from the testbed form

- **New … part…** opens a dialog holding that part's form: Name, its settings and its Advanced settings. Above them
  is what the kind is.
- **Save part** checks and saves the part as the part form does. It then closes the dialog and picks the new part in
  its picker, with its summary. Errors show inside the dialog. **Cancel**, Esc and closing the dialog save nothing.
- Nothing typed in the testbed form changes.
- **Open ↗** opens the part's page in a new tab. So does the test form's **Steps ↗** link.
- Leaving a testbed, part, test, suite, step or plan form with unsaved changes asks first. Saving and deleting
  don't ask.

### Settings that explain themselves

- The section's hint is "Keep the defaults unless you are testing one of these. A matrix can test every choice."
- The three settings are radio lists. Each choice says what it does and when to pick it, and the default is marked
  "(default)":

  | Label | Choices |
  |---|---|
  | **Where Tinkerbell runs** | **On the management cluster** (default): with Cluster API and CAPT, the simplest and lightest setup. **On its own cluster**: a kind cluster of its own, which CAPT reaches from the management cluster. |
  | **How machines boot** | **Network boot** (default): over the network from Tinkerbell, with PXE and iPXE. **ISO boot**: from an ISO image that Tinkerbell serves, attached by the BMC as virtual media. |
  | **IP version** | **IPv4** (default). **IPv6**: NAT64 lets machines reach registries that are IPv4 only. |

- Picking the default writes nothing, so a saved testbed stays sparse, as today.
- The advanced settings have help: NAT64 (on with IPv6 by default), Registry mirror and Mirror upstreams.
- The new labels are also used on the testbed page, by the matrix editor's "varies" choices and in the TUI.
- Each part picker says what its kind is, using the kind's blurb. Its first option reads "chart-latest (default)".
- The testbed blurb names what a testbed is made of instead of "the settings no part owns".

### Defaults in every form

- A dropdown's first option reads "<choice> (default)" and the choice isn't listed again. Yes/no settings read
  "Yes (default)" and "No". The TUI matches.

### Found while building

- Every form field had `class="field top"`, and the top bar's style was `.top`, so each field picked up the bar's
  padding, background, border and `position: sticky`. That is what boxed the fields and made them stick while
  scrolling. The bar's rules are now `nav.top`.

## Success Criteria

1. On the new-testbed form, making a part through **New … part…** leaves every field of the testbed form as it was,
   and picks the new part. A rejected part shows its errors in the dialog and saves nothing.
2. **Open** and **Steps** links in forms open in a new tab. An edit form with changes asks before the page is left.
   A save doesn't ask.
3. The testbed form's three settings are radio lists, with the labels and descriptions above and the default
   marked. Picking the default writes nothing.
4. No dropdown in the web UI or the TUI lists its default twice, or reads "default: …".
5. Each part picker shows its kind's blurb, and its fallback reads "<name> (default)".
6. The glossary guard and the label check pass, as do `make vet lint test`, and the generated templates are up to
   date.
7. README and `design.md` §11 describe the dialog and the new labels.

## Boundaries

Everything else is inherited from [web-ux/spec.md](../web-ux/spec.md).

- **Never:** CUE schema or CUE value changes. `topology`, `bootMode` and `network.family` keep their names and
  values; only what people read changes.
- **Ask first:** dialogs for anything other than making a part from the testbed form.

## Out of Scope (Deferred)

- The matrix editor's existing axes don't list a part made in the dialog until their "varies" choice is changed.
  New axes list it. This is rare, and the save is unaffected.
- Descriptions for the choices in part forms, such as CNI, architecture and BMC protocol. The same mechanism serves
  them once their words are agreed.
- Per-choice descriptions in the TUI, which shows the field's help only (`tui-ux`).
