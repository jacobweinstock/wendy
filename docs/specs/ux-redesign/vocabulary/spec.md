# Spec: vocabulary — V1

Module of [map.md](../map.md). Depends on: nothing. Required by: model-rename, cli, web-ux, tui-ux.

Status: implemented. Plan: [plan.md](plan.md); tasks: [todo.md](todo.md).

## Objective

Give wendy one source for the words its surfaces show, so the web UI, TUI and CLI cannot drift apart again, and
switch the words that don't depend on the config rename to it now.

Today the same words are defined in several places:

| Words | Defined in |
|---|---|
| Status glyphs | `event.Status.Glyph` ([event.go](../../../../internal/event/event.go)) |
| Status words and colour classes | `ui.Word`, `ui.Class` ([view.go](../../../../internal/ui/view.go)) |
| Phase names ("Build", "Setup (n steps)") | [view.go](../../../../internal/ui/view.go), [run.go](../../../../internal/ui/tui/run.go) grid columns |
| Plurals | `plural` in [helpers.go](../../../../internal/ui/web/helpers.go) (appends "s"), `plural` in [style.go](../../../../internal/ui/tui/style.go), `singular` in [edit.go](../../../../internal/service/edit.go) and [helpers.go](../../../../internal/ui/web/helpers.go) (trims "s") |
| Choice labels | none: forms show raw values (`onFailure`, `colocated`) from `ui.Field.Options` ([fields.go](../../../../internal/ui/fields.go)), [form.go](../../../../internal/ui/tui/form.go) |

**User:** the developers of the later modules, and through them every wendy user. They see the new status and
phase words and labelled choices as soon as this module ships.

### What this module delivers

A new leaf package, `internal/vocab`, holding:

1. **Statuses.** For every `event.Status`: word, glyph and colour class. Words follow the glossary: passed, failed,
   errored, timed out, cancelled, skipped, running, waiting, paused. "not run" and "paused on failure" are retired
   as status words. Contextual phrases such as "paused after a failure" belong to the screens (web-ux, tui-ux).
2. **Phases.** Display names for the execution phases, keyed by the engine's phase ids: Build images, Set up
   testbed, Collect diagnostics, Tear down.
3. **Choices.** A display label for every value of every choice a form offers, keyed by field path and value, e.g.
   `topology/colocated` → "On the management cluster", `keep/onFailure` → "If it fails". Values are still what's
   written to CUE. Only the label changes.
4. **Kinds.** The target kinds from the glossary: Plan, Testbed, the five parts, Suite, Test and Built-in step. Each
   has an id (its CUE collection key), a slug (URL and CLI), singular and plural labels, and a one-line blurb. They
   are defined and tested here, and wired in by `model-rename` when the config kinds change. This also fixes
   problem #10 there, since no label is ever built by adding or trimming an "s".
5. **Count.** `Count(n, one, many)`, the single plural helper.
6. **Retired terms.** The list of retired words and phrases from the map (Environment as a wendy concept,
   Profile, Scenario, Catalog, Providers, Workload alone, Evidence, combo, Target, "Ginkgo label filter", "run" as a
   noun, "not run"), each with the glossary word to use instead and its allowed exceptions ("environment
   variables", "Workload cluster", "Run tests"). `Check(text)` returns each retired use with its replacement and
   the surrounding text.

### Wired in now

- Web, TUI and `--plain` CLI output take statuses from `vocab`. `event.Status.Glyph`, `ui.Word` and `ui.Class`
  are removed. A skipped step reads "skipped", not "not run".
- The execution tree and grid in both UIs show the phase names above.
- Every form's choices (web and TUI edit forms, both run forms) show labels and submit values.
- The web and TUI plural helpers are replaced by `vocab.Count`.

### Not wired yet (later modules)

- Kinds: `model-rename` switches `config` kinds, the catalog tabs and `singular` to `vocab.Kinds`.
- Guard tests over rendered output: each surface module (`cli`, `web-ux`, `tui-ux`) adds a test that runs
  `vocab.Check` over its own pages, screens or help text once that surface is converted. A guard added now would
  fail on every screen that still uses the old words.

## Tech Stack

Go (module's existing version), no new dependencies. `internal/vocab` imports only the standard library and
`internal/event`.

## Commands

```
make build
make test
make vet
make lint
make generate          # after .templ changes
make verify-generate
```

## Project Structure

```
internal/vocab/vocab.go      → package doc (the glossary), statuses, phases, Count
internal/vocab/kinds.go      → Kinds
internal/vocab/choices.go    → choice labels
internal/vocab/retired.go    → retired terms and Check
internal/vocab/*_test.go     → unit tests
```

Callers changed: `internal/event`, `internal/ui` (`view.go`, `fields.go` tests), `internal/ui/web` (helpers,
pages, templates), `internal/ui/tui` (style, run, edit, form), `internal/cli/plain.go`.

## Code Style

Data as tables, behaviour as small functions, doc comments per go.dev/doc/comment:

```go
// Status is how a status reads everywhere wendy shows one.
type Status struct {
	Word  string // e.g. "timed out"
	Glyph string // e.g. "✗"
	Class string // the colour family: pass, fail, err, run, pause or skip
}

// StatusOf is the word, glyph and colour class of s.
func StatusOf(s event.Status) Status { … }

// Choice is the label a form shows for value of the field at path, e.g.
// Choice("topology", "colocated") is "On the management cluster".
func Choice(path, value string) string { … }
```

## Testing Strategy

Unit tests in `internal/vocab` and focused updates where callers change:

- Every `event.Status` constant has a non-empty word, glyph and class, and no two statuses share a word.
- Every option of every `ui.Field` list, the step kinds, and the run forms' keep and evidence choices have a
  choice label (test in `internal/ui`, so a new option without a label fails).
- Kinds: ids, slugs and labels are unique; slugs are kebab-case; no kind label trips `Check`.
- `Check` table tests: flags "Environments" but not "environment variables"; flags "a run", "Re-run" and "Runs" but
  not "Run tests" or "Run again"; flags "Workload" but not "Workload cluster"; reports the replacement and context.
- Choice round trip: a form shows the label, and saving writes the value (one web edit test, one TUI form test).
- Existing web, TUI and plain-output tests are updated for "skipped" and the new phase names. The a11y label test
  still passes.

## Boundaries

- Always: take every status, phase, choice label and plural from `vocab`; keep `vocab` a leaf (stdlib and `event`
  only); regenerate templates; ask before any git command.
- Ask first: any word not in the glossary of [map.md](../map.md); adding a dependency.
- Never: change config kinds, CUE keys, event paths, routes, navigation or CLI commands (later modules); keep a
  second copy of a status word or glyph outside `vocab`; commit.

## Success Criteria

- `internal/vocab` exists with statuses, phases, choices, kinds, `Count`, the retired list and `Check`, and imports
  only the standard library and `internal/event`.
- No package other than `vocab` defines a status word, glyph or colour class. `event.Status.Glyph`, `ui.Word`,
  `ui.Class` and the web and TUI `plural` helpers are gone.
- The execution tree and grid show "Build images", "Set up testbed", "Collect diagnostics" and "Tear down" in both
  UIs. A skipped step shows "– skipped".
- No form in either UI shows a raw choice value as its label, and saving still writes the value.
- `--plain` output is unchanged apart from status words.
- `make vet`, `make lint`, `make test` and `make verify-generate` pass.

## Out of Scope (Deferred)

- Using `vocab.Kinds` in config, the catalog and `singular` (fixes problem #10): `model-rename`.
- Guard tests over rendered pages, screens and help: `cli`, `web-ux`, `tui-ux`.
- Contextual copy such as "paused after a failure", "Waiting for you" and button labels: `web-ux`, `tui-ux`.
- Re-keying choice labels whose values change (`keep: onFailure` → `failed`, `evidence.dump` → `diagnostics`):
  `model-rename`, together with the values.
- A separate glyph for "timed out" (it shares ✗ with failed today): not part of the glossary decision.

## Resolved Questions

Both proposals were accepted:

1. **README glossary.** It goes into the `vocab` package doc now, and into the README with `model-rename`, which
   rewrites the README for the new config.
2. **Guard tests.** This module ships `Check` and its tests. `cli`, `web-ux` and `tui-ux` each add a guard test over
   their own output when they convert. This amends the testing strategy in [map.md](../map.md).
