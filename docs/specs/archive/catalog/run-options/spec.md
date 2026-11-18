# Spec: run-options — V1

Module of [map.md](../map.md). Depends on: catalog-model (implemented). Required by: web-catalog-ui, tui-editor.

Status: implemented. Archived. Plan: [plan.md](plan.md); tasks: [todo.md](todo.md).

## Objective

Take the two run-level settings out of the environment so an environment describes only what is built:

- **`evidence`** (capture consoles, when to dump evidence) becomes a **Profile** option. It is about how a run behaves, not what it builds.
- **`upgrades`** (target Kubernetes, Tinkerbell and CAPI/CAPT versions) is removed from the environment. Each upgrade step takes its own target, which the steps already accept (`version`, `to`, `core`/`capt`).

**User:** someone running the same environment as a quick check, a nightly and a debug session, or testing upgrades to different targets.

**Why:** today, "dump evidence always" or "upgrade to v1.37" means copying the environment, and every copy repeats Tinkerbell, hardware and workload. After this change the environment is stable and the run or the scenario carries the variation.

### How it works after the change

```cue
profiles: debug: {
	environment: "ha"
	suites:      ["lifecycle"]
	evidence: {consoles: true, dump: "always"}
}

// A scenario that upgrades to a chosen version: a copy of the built-in, with a target.
scenarios: "upgrade-to-1.37": {
	description: "Rolls the workload cluster to Kubernetes v1.37.0"
	requires: spare: 1
	steps: [
		{uses: "capt.upgrade.kubernetes", timeout: "60m", with: version: "v1.37.0"},
		{uses: "assert.healthy", timeout: "20m"},
	]
}
```

### Acceptance stories

1. As a user, I set `evidence.dump: "always"` on a profile and every combination of that profile dumps evidence; the environment is unchanged and other profiles are unaffected.
2. As a user, a profile with no `evidence` behaves exactly as an environment with no `evidence` does today.
3. As a user, I write a scenario whose upgrade steps carry `with: {version: ...}` and it upgrades to that target, with no `upgrades` block anywhere.
4. As a user, the built-in `upgrade-k8s`, `upgrade-tinkerbell` and `upgrade-provider` scenarios, which have no target, skip with a message that says what to add.
5. As a user with an environment that still has `upgrades`, `wendy vet` tells me where it went.

## Tech Stack

Unchanged from catalog-model: Go 1.26, CUE v0.17.1. No new dependencies.

## Commands

```
Test:       make test
Lint/Vet:   make lint && make vet
Plan check: go run ./cmd/wendy plan --profile quickstart
Docs gen:   make generate && make verify-generate    # only if a .templ changes
```

## Project Structure

```
cue/wendy/schema.cue      → #Profile gains evidence; #Environment loses upgrades; #ResolvedEnvironment loses upgrades
cue/wendy/builtin.cue     → upgrade scenario descriptions rewritten (no upgrades.* references)
internal/config/          → types.go (Profile.Evidence, Environment.Upgrades removed), resolve.go (EnvSpec drops Evidence and Upgrades), load.go (migration hint for `upgrades`/`evidence`)
internal/plan/plan.go     → Selection/effective carry evidence; Combo.Env.Evidence set from it; collectBuilds drops the env.Upgrades branch
internal/steps/           → capt.go, tinkerbell.go, capi.go: drop the environment fallback; update skip messages
internal/cli/root.go      → `vet` image check reads upgrade versions from scenario steps
README.md, design.md
```

## Code Style

Follows the repo (see catalog-model). One snippet of the intended shape of the plan change:

```go
// Evidence is how a run records what went wrong: whether to capture machine
// consoles and when to dump the cluster. A profile sets it; unset means the
// defaults the schema gives.
type Evidence struct {
	Consoles bool   `json:"consoles"`
	Dump     string `json:"dump"`
}

// In buildCombo: the environment is resolved without a run, so the run's
// evidence is applied to the combination's copy.
rc.Env.Evidence = eff.Evidence
```

## Testing Strategy

Go table tests beside the code.

- `internal/config`: profile `evidence` decodes with defaults; the old `environments.*.upgrades` and `environments.*.evidence` fields report a migration message; the `quickstart` golden is unchanged (evidence defaults still resolve identically).
- `internal/plan`: a profile's evidence reaches every combination's `Env.Evidence`; `dump: "never"` removes the evidence steps from the plan; builds are still collected from step parameters (`tinkerbell.upgrade` `to.source`, `capi.upgrade` `capt.source`) and no longer from the environment.
- `internal/steps`: each upgrade step skips with the new message when it has no target, and runs unchanged with one.
- `internal/cli`: the `vet` image list includes a version named only in a step's `with`.

## Boundaries

- **Always:** keep the resolved `config.Environment` JSON shape except for the removed `upgrades` key; keep upgrade step parameters as they are; give every skip a message that says what to set; run `make test vet lint` before finishing.
- **Ask first:** adding scenario-level parameters (see Open Questions); changing what `evidence` captures; any change to `environment.json` beyond removing `upgrades`.
- **Never:** keep an `upgrades` fallback "for now"; design part-swapping matrix axes (composition-plan); touch the web or TUI run forms (their modules own that).

## Success Criteria

1. `#Profile` accepts `evidence: {consoles, dump}` with the schema's current defaults (`true`, `"onFailure"`); `#Environment` and `#ResolvedEnvironment` no longer accept `upgrades` or an environment-level `evidence`.
2. A plan for a profile with `dump: "always"` has every combination's `Env.Evidence.Dump == "always"`; with `"never"`, `Combo.Evidence` is empty. A selection with no profile gets the defaults.
3. The `quickstart` golden is unchanged.
4. No code reads an environment's upgrade targets: `grep -rn "Config.Upgrades\|env.Upgrades\|e.Upgrades" internal` is empty.
5. `tinkerbell.upgrade`, `capi.upgrade` and `capt.upgrade.kubernetes` with no target skip with a message naming the parameter to set; with a target they behave as before (covered by existing step tests or new ones).
6. `wendy plan` builds-needed output for a scenario that upgrades to a source (`with: to: source: repo: ...`) is unchanged.
7. `wendy vet` checks the OS image for a Kubernetes version that only a step's `with.version` names.
8. An environment with `upgrades` or `evidence` fails to load with a message naming the new home (`profile evidence`, `step parameter`), at the right file:line.
9. README and `design.md` describe profile evidence and step-level targets; the starter config shows one profile with `evidence`.
10. `make test`, `make vet`, `make lint` pass.

## Out of Scope (Deferred)

- Editing evidence in the web or TUI run form: web-catalog-ui and tui-editor (the form uses the profile's value until then).
- A `--evidence` CLI flag or a per-run override on `Selection`: add when a UI needs it.
- Scenario-level parameters (see Open Questions).
- Making `consoles` a hardware concern: it controls what the machines log, but it is only meaningful when evidence is collected, so it stays with evidence.

## Resolved Questions

1. Scenario-level parameters are deferred. Upgrade targets are step parameters, and the built-in upgrade scenarios are templates to copy.
2. The built-in `lifecycle` and `upgrades` suites keep skipping their upgrade scenarios when there is no target.
