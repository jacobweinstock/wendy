# Spec Delta: environment-settings — V1

Status: implemented. Archived.

Delta to [catalog-model/spec.md](../catalog-model/spec.md) and [editor-service/spec.md](../editor-service/spec.md) (both implemented). Not a new module in [map.md](../map.md); it corrects field placement found while using the web editors.

## Objective

Put each setting where the thing it describes lives, so the parts stay reusable. Three corrections, all from review of the web editors:

1. **Cross-cutting test dimensions move to the environment.** `topology`, `bootMode`, `network.family`, `network.nat64` and `network.mirror` are written on the environment, not in a Tinkerbell part. Each is read by several parts and none belongs to one: boot mode is read by the chart values, the `TinkerbellMachine` `bootOptions`, the isoboot workflow's static netplan and the host's libvirt storage pool. Left in a part, every value multiplies the parts (`ha-netboot`, `ha-iso`, ...), which is the coupling this redesign exists to remove. Each is already a matrix axis, so a matrix still varies them.
2. **The management cluster's kind node image moves to the Providers part**, as an Advanced field. The management cluster always hosts Cluster API and CAPT, and hosts Tinkerbell only when the topology is `colocated`; only kind creation reads the image.
3. **Build from source is Advanced for both Tinkerbell and CAPT.** It is a developer workflow.

Decision recorded, no change: `hardware.bmc.protocol` and the other harness settings stay as they are.

**User:** anyone composing environments in the web UI or in CUE.

## Current Behavior

- A Tinkerbell part holds `management`, `network` and the `tinkerbell` fields, including `topology` and `bootMode` ([cue/wendy/schema.cue](../../../../../cue/wendy/schema.cue) `#TinkerbellPart`, [internal/ui/fields.go](../../../../../internal/ui/fields.go) `TinkerbellFields`).
- `#Environment` holds a description and four part references.
- `assemble` ([internal/config/resolve.go](../../../../../internal/config/resolve.go)) sends the Tinkerbell part's `management` and `network` to the top level of the resolved environment, and the rest to `tinkerbell`.
- The source fields of Tinkerbell and CAPT are in the common section of their forms.

## New Behavior

### Where each setting is written

| Setting | Was | Now |
|---|---|---|
| `topology`, `bootMode` | Tinkerbell part | environment |
| `network.family`, `network.nat64`, `network.mirror.*` | Tinkerbell part | environment |
| `management.image` | Tinkerbell part | Providers part |
| `source.repo/ref` (Tinkerbell), `capt.source.repo/ref` | common | Advanced |

The Tinkerbell part keeps `chart`, `source`, `values`, `namespace`, `captainos` and, for the external topology, `cluster`. After this, its only common fields are the chart version and the build source; the Providers part's are the two versions.

```cue
environments: ha: {
	tinkerbell: "stable"
	providers:  "default"
	hardware:   "default"
	workload:   "default"
	topology:   "external"
	bootMode:   "isoboot"
	network: {family: "ipv6", mirror: enabled: true}
}
```

All of the environment's new fields are optional; omitted ones take the schema defaults, as part fields do.

### Resolution

- `assemble` merges the environment's `topology` and `bootMode` into the resolved `tinkerbell` section and its `network` into the top-level `network`; the Providers part's `management` goes to the top-level `management`. The resolved environment's shape is unchanged, so `render.cue`, steps, the golden test's structure and matrix patches (`tinkerbell: bootMode: ...`) keep working as they are.
- A key in a part that now belongs elsewhere is an error naming where it moved, in the same style as the existing migration hints: `topology`/`bootMode`/`network` in a Tinkerbell part → "set it on the environment"; `management` in a Tinkerbell part → "set it in the providers part".
- Errors in the moved fields report at the environment (or the Providers part for `management`), not at the Tinkerbell part. `ownerOf` maps the paths accordingly.
- A matrix axis that patches a moved field is unchanged: it patches the resolved environment, as today.

### Editors

- **Environment form (web):** a "Settings" section after the four part pickers. Common: topology, boot mode, IP family. Advanced: NAT64, registry mirror, mirror upstreams. `EnvironmentFields` carries them, with the same labels, options and defaults the Tinkerbell fields had.
- **Tinkerbell part form:** no longer shows them. **Providers part form:** shows "kind node image" under Advanced, and CAPT/Tinkerbell source fields are Advanced.
- Tinkerbell part summary ([internal/ui/web/helpers.go](../../../../../internal/ui/web/helpers.go) `partSummaryOf`) drops boot mode and topology and shows what the part still holds (chart or build). The environment's detail and list views show topology, boot mode and IP family where they show them today.
- Saved environments keep their inline parts and fields the form does not show (`applyForm` preservation), as before.

### Docs and templates

README, `design.md`, the part blurbs, `cue/starter.cue.tmpl` and `.wendy/wendy.cue` (kept identical) describe the new placement.

## Success Criteria

- `make vet`, `make lint` and `make test` pass; templates are current.
- The golden resolution of the quickstart environment is unchanged (same resolved environment from the new layout).
- An environment with `topology`, `bootMode` and `network` set resolves to the same `Environment` as the old layout did; one environment can pair one Tinkerbell part with `netboot` and another with `isoboot`, with no second part.
- A Tinkerbell part that sets `topology`, `bootMode`, `network` or `management` fails to load with a message saying where the setting moved, at the part's position.
- An invalid value (`bootMode: "pxe"`) reports at `environments.<name>.bootMode`, not at a part.
- A matrix with a `bootMode` axis still expands, and `wendy plan` shows the varying boot mode as today.
- The web environment editor creates, edits and saves the five settings; the Tinkerbell form does not offer them; the Providers form offers the node image; unrelated fields survive an edit.
- The a11y label test passes for the changed forms.
- Hardware `bmc.protocol` and the other harness fields are untouched.

## Boundaries

- Always: change the schema, `assemble`, field lists, migration hints, docs and tests together; keep `cue/starter.cue.tmpl` and `.wendy/wendy.cue` identical; ask before any git command.
- Ask first: moving any further field between parts and the environment; changing the resolved environment's shape.
- Never: change what `render.cue` or the steps read; move `arch` (it stays with Hardware, whose machines it describes); commit.

## Out of Scope (Deferred)

- Moving `arch` to the environment: it describes the hardware's machines, and is still an axis.
- Renaming the intrinsic axes or changing matrix patch shapes: no UX gain this round.
- Editing the external topology's `cluster` in a form: unexposed today, still CUE-only.
- Hiding `bmc.protocol` and other harness plumbing from forms: decided to leave as is.
- The `tui-editor` module: still has no spec; it will build on these field lists.

## Open Questions

1. **Order in `#Environment` and the form:** the settings follow the four part pickers (proposed). Alternative: before them. Proposed is fine unless you object.
2. **An old Tinkerbell part with these keys is an error, not silently accepted** (consistent with your earlier "yes, error" on old fields). Proposed.
3. **Run form:** no change; it picks a profile, and a profile's matrix already varies these.
