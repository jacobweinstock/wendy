# Wendy's docs

Start with the user guide, [../README.md](../README.md). To see how Wendy works, read [design.md](design.md).
Everything else here records how Wendy got to where it is, and what is planned next.

| Doc | What it is | Status |
|---|---|---|
| [design.md](design.md) | The design: concepts, architecture, config, execution, API and UIs | current |
| [mockups/web/](mockups/web/index.html) | The web UI mockup the web UI follows | current |
| [mockups/tui.md](mockups/tui.md) | TUI wireframes | current until `tui-ux` |
| [mockups/web.html](mockups/web.html) | Earlier web wireframes | superseded by `mockups/web/` |

## Specs

Each initiative has a capability map (`map.md`) and one folder per module or change. Each folder holds three
files:

- `spec.md`: what and why, with success criteria;
- `plan.md`: how, in what order;
- `todo.md`: the tasks, checked off as they're done.

A spec's `Status:` line is `proposed`, `approved` or `implemented`. A change to an implemented spec gets its own
folder, with a delta spec. New work follows this layout, not the skill's default (`SPEC-*.md` at the root and
`tasks/`).

### [ux-redesign](specs/ux-redesign/map.md): words, nav and screens

| Module or change | Status |
|---|---|
| [vocabulary](specs/ux-redesign/vocabulary/spec.md) | implemented |
| [model-rename](specs/ux-redesign/model-rename/spec.md) | implemented |
| [web-ux](specs/ux-redesign/web-ux/spec.md) | implemented |
| `cli` | no spec yet |
| `tui-ux` | no spec yet |
| [steps-and-names](specs/ux-redesign/steps-and-names/spec.md) | implemented |
| [plans-and-matrices](specs/ux-redesign/plans-and-matrices/spec.md) | implemented |
| [testbed-form](specs/ux-redesign/testbed-form/spec.md) | implemented |

steps-and-names and plans-and-matrices come from [feedback-2026-11-18.md](specs/ux-redesign/feedback-2026-11-18.md);
testbed-form comes from using the new-testbed form.

### [archive/catalog](specs/archive/catalog/map.md): parts, testbeds and editors

This initiative is implemented. Its specs use words that ux-redesign later retired (environment, profile,
scenario, catalog).

[catalog-model](specs/archive/catalog/catalog-model/spec.md) ·
[run-options](specs/archive/catalog/run-options/spec.md) ·
[composition-plan](specs/archive/catalog/composition-plan/spec.md) ·
[editor-service](specs/archive/catalog/editor-service/spec.md) ·
[web-catalog-ui](specs/archive/catalog/web-catalog-ui/spec.md) ·
[environment-settings](specs/archive/catalog/environment-settings/spec.md)
