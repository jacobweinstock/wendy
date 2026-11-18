# Spec: composition-plan — V1

Module of [map.md](../map.md). Depends on: catalog-model (implemented). Required by: web-catalog-ui, tui-editor.

Status: implemented. Archived. Plan: [plan.md](plan.md); tasks: [todo.md](todo.md).

## Objective

Make parts the unit of variation in matrices, and make them visible everywhere a plan or catalog is listed:

1. A **matrix axis can select a part by name**, so "three Kubernetes versions on one Tinkerbell" is three small workload parts and one axis, with no repeated environment.
2. **Every combination knows which parts it uses**, and `wendy plan`, `wendy list` and the combination data show them.
3. **Parts can be listed** from the CLI, since after catalog-model they exist only in CUE and nothing shows them.

**User:** someone building a test matrix, or reading a plan to see what a run will build.

### How it works

```cue
workloads: "k8s-1.36": kubernetesVersion: "v1.36.4"
workloads: "k8s-1.37": kubernetesVersion: "v1.37.0"
hardware:  ha:         machines: {controlPlane: count: 3, worker: count: 3}

matrices: versions: {
	base:  "default"
	order: ["k8s", "size"]
	axes: {
		k8s: {
			"1.36": workload: "k8s-1.36"   // a string selects a part
			"1.37": workload: "k8s-1.37"
		}
		size: {
			small: {}                       // the base environment's own hardware
			ha:    hardware: "ha"
			big:   hardware: machines: worker: count: 5   // a struct patches resolved fields, as today
		}
	}
	suites: ["smoke"]
}
```

**The rule:** an axis value is a patch of the resolved environment, except that a key naming a kind of part (`tinkerbell`, `providers`, `hardware`, `workload`) with a **string** value selects that part by name. A struct value patches the resolved fields exactly as it does today, so every existing matrix keeps working.

### Acceptance stories

1. As a user, an axis value `workload: "k8s-1.37"` builds that combination with the named workload and everything else from the base environment.
2. As a user, a mistyped part name in an axis reports `file:line`, the axis path and a "did you mean" suggestion, once, not per combination.
3. As a user, `wendy plan` shows each combination's parts, e.g. `hardware ha · workload k8s-1.37`, so I can see what differs between combinations.
4. As a user, `wendy list hardware` (and `tinkerbells`, `providers`, `workloads`) shows each part, where it is defined, and which environments use it.
5. As a user, existing matrices that patch fields (`network: family: "ipv6"`) produce the same combinations and names as before.

## Tech Stack

Unchanged: Go 1.26, CUE v0.17.1. No new dependencies.

## Commands

```
Test:       make test
Lint/Vet:   make lint && make vet
Try it:     go run ./cmd/wendy list combos
            go run ./cmd/wendy plan --profile quickstart
            go run ./cmd/wendy list hardware
```

## Project Structure

```
cue/wendy/schema.cue       → #Matrix axis values documented as patch-or-part-selection (still {...})
internal/config/resolve.go → expand: apply part selections, then patches; report unknown names once, at the axis
internal/config/types.go   → Combo.Parts (kind → name, "" for inline or patched)
internal/plan/plan.go      → Combo gains Parts; needs() unchanged (reads resolved hardware)
internal/cli/root.go       → `list` handles the four part kinds; plan output prints parts
internal/ui/tui, ui/web    → show Combo.Parts where a combination is shown (read-only; no new pages)
README.md, design.md
```

## Code Style

Follows the repo. The core of the change is one function that decides what a patch means:

```go
// splitAxisValue separates what an axis value does: the parts it selects by
// name, and the patch it applies to the resolved environment. A part-kind key
// holding a string selects a part; anything else is a patch.
func splitAxisValue(patch map[string]any) (selects map[string]string, rest map[string]any) {
	selects, rest = map[string]string{}, map[string]any{}
	for k, v := range patch {
		if kind, ok := kindOfField(k); ok {
			if name, isName := v.(string); isName {
				selects[kind] = name
				continue
			}
		}
		rest[k] = v
	}
	return selects, rest
}
```

Selections from several axes apply in axis order, so a later axis wins on the same kind; the resolver reports a combination where two axes select the same kind with different parts (almost always a mistake).

## Testing Strategy

Go table tests beside the code, plus golden output for combination names.

- `internal/config`: an axis selecting parts expands to the right resolved environments; mixed (select + patch) values; unknown part reported once with suggestion and position; two axes selecting the same kind is an error; the existing 16-combination playground test is unchanged.
- `internal/plan`: `Combo.Parts` is set for matrix combinations and for a plain environment; label filters and implicit filters behave as before; host `Needs` reflects a swapped-in hardware part.
- `internal/cli`: `list <kind>` output, including "used by".
- No new UI behavior is tested beyond "builds and shows parts where a combination is shown".

## Boundaries

- **Always:** keep combination names and axis semantics for existing matrices byte-for-byte; resolve each combination from the base environment's parts, never from a copy of its flat fields; report errors with position and a suggestion; run `make test vet lint`.
- **Ask first:** making part names axis values or labels that `when:` and label filters can match (see Open Questions); changing how combinations are named; adding sugar such as `parts: workload: [a, b]`.
- **Never:** edit the resolved `config.Environment` shape; build catalog tabs or editors for parts (the UI modules own those); change `evidence` or `upgrades` (run-options owns those).

## Success Criteria

1. An axis value with a part-kind key set to a string selects that part; the combination resolves to the same environment as writing an environment that names those parts (golden-compared).
2. Struct-valued axis patches are unchanged: the existing `TestLoadExample` (16 combinations, 4 excluded) and `plan` tests pass unmodified.
3. An unknown part in an axis reports one error at the axis value's `file:line`, path `matrices.<m>.axes.<axis>.<value>.<kind>`, with up to three suggestions, regardless of how many combinations use it.
4. Two axes selecting different parts of one kind in the same combination is reported as an error naming both axes; selecting the same part twice is not.
5. `Combo.Parts` holds the named part of each kind (and nothing for an inline or field-patched one); it is present in `wendy plan` output and in the combination data the UIs read.
6. `wendy plan` prints a combination's parts after its axes, only where they differ between the combinations shown.
7. `wendy list tinkerbells|providers|hardware|workloads` prints name, origin and the environments that use it; `wendy list environments` also shows the parts each uses.
8. A host-needs estimate for a combination with a swapped hardware part reflects that part's machines.
9. README and `design.md` document the rule and the example above.
10. `make test`, `make vet`, `make lint` pass.

## Out of Scope (Deferred)

- Shorthand that builds an axis from a list of parts (`parts: workload: [...]`): add after seeing how people write these.
- Part names as label filter values: see Open Questions.
- Catalog tabs, detail pages and "used by" in the web UI and TUI: web-catalog-ui and tui-editor (they read `Combo.Parts` and the `EnvSpec` part names).
- Profile-level overrides of a part (`profiles: x: {environment: "ha", workload: "k8s-1.37"}`): a promising shortcut for "run this environment with a different workload", but it changes `Selection` and both run forms.
- Parallelism limits that account for part sizes: the existing `needs()` estimate is enough.

## Resolved Questions

1. Part names are not labels in V1; use a distinct matrix axis.
2. A profile-level part override stays out of scope.
3. Two axes selecting different parts of one kind in a combination is an error.
