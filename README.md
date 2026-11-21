# Wendy

Wendy (Provisioning Engine for Tinkerbell E2E Runs) runs end-to-end tests for [Tinkerbell](https://github.com/tinkerbell/tinkerbell) and
[Cluster API Provider Tinkerbell](https://github.com/tinkerbell/cluster-api-provider-tinkerbell) (CAPT).
It builds a testbed from a declarative CUE description (kind, a libvirt network and machines, a
virtual BMC, Tinkerbell and CAPT, from releases or from source) and runs tests on it. Each
execution keeps enough diagnostics to debug a failure hours later.

Most of the time you drive it from its TUI or web UI. `wendy run` is for CI.
See [docs/design.md](docs/design.md) for the design, and [docs/](docs/README.md) for the specs behind it.

## Requirements

A Linux host with:

- Docker
- libvirt (`qemu:///system`), with your user able to use its socket
- `/dev/kvm`
- Only for building Tinkerbell or CAPT from source: `git`, `make`, `go` and `docker buildx`

`wendy vet` checks the host, the config and the images your testbeds need.

## Install

```sh
go install github.com/tinkerbell/wendy/cmd/wendy@latest
```

Or download a binary from the [releases](https://github.com/tinkerbell/wendy/releases).

## Use

```sh
wendy
```

This opens the TUI and serves the web UI on <http://127.0.0.1:7600>. The first launch offers a starter
config. The built-in `quickstart` plan runs the `smoke` suite on one kind + libvirt testbed with no
config at all.

### Glossary

```
Parts ──▶ Testbed (+ optional matrix ▶ combinations) ─┐
                                                      ├─▶ Plan ──run──▶ Execution ─▶ (kept testbed)
          Steps ▶ Tests ▶ Suites ─────────────────────┘
```

*Run* is only a verb: you run a plan. What that creates is an execution.

| Term | Meaning |
|---|---|
| **Plan** | What to test, where and how: the suites, the testbed, and options such as how many combinations go at once and whether to keep the testbed. |
| **Execution** | One time wendy carries out a plan: it builds each combination's testbed, runs the suites, collects diagnostics and tears down. It has a number (`#412`), a status and a verdict. |
| **Preview** | What a plan would do, shown before it runs: combinations, what it needs, what it builds, how long it takes, host checks. |
| **Testbed** | What tests run on: one part of each kind, the settings no part owns (topology, boot mode, network), and an optional matrix. |
| **Part** | A reusable piece of a testbed, changed on its own. There are five kinds: Tinkerbell, Cluster API, Management cluster, Workload cluster and Hardware. |
| **Matrix** | A testbed's axes. Each axis value patches the testbed or picks a part. |
| **Axis** | One dimension of a matrix, such as `k8s` or `bootMode`. |
| **Combination** | One point in the matrix: one concrete testbed. A testbed without a matrix has one combination. |
| **Kept testbed** | A built testbed an execution left up, after a failure or when asked. It is addressed as `<execution>/<combination>`, e.g. `412/ha-1.37`. |
| **Suite** | Tests run in order on one testbed. |
| **Test** | A named, ordered list of steps, such as `deploy` or `scale-out`. |
| **Step** | One action in a test, timed and reported: a named step, a command, a container, Chainsaw tests or a Ginkgo test binary. |
| **Named step** | A step a test uses by name (`uses:`): a built-in step compiled into Wendy, such as `capt.scale`, or one of your own under `steps:`. |
| **Label** | A tag on a test or combination. A label filter picks by label, e.g. `scale && !upgrade`. |
| **Diagnostics** | What wendy collects to debug a failure: cluster dump, Tinkerbell objects, BMC logs, machine consoles. |
| **Artifacts** | Any file an execution saved: step output, the JUnit report, diagnostics. |
| **Config** | The CUE files wendy loads. |

Statuses: passed ✓ · failed ✗ (Tinkerbell or CAPT) · errored ! (wendy or the host) · timed out ·
cancelled ⊘ · skipped – · running ● · waiting ○ · paused ‖.

### Config

wendy loads every `.cue` file in `$XDG_CONFIG_HOME/wendy`, then `./.wendy`, then each `--config`
directory, and reloads them when they change. Everything they define (plans, testbeds and their
parts, suites and tests) can be browsed and run from both UIs. To list them:

```sh
wendy list plans
wendy plan --plan quickstart   # the preview: what it would run, what it needs, how long it takes
```

#### Testbeds are made of parts

A testbed names one **part** of each kind, so what you test, what it runs on and what you deploy are
written, reused and changed separately. Parts are written under `parts:`.

| Part | CUE | Holds |
|---|---|---|
| Tinkerbell | `parts: tinkerbell` | how Tinkerbell is installed: chart or source build, namespace, chart values |
| Cluster API | `parts: clusterAPI` | Cluster API core and CAPT, as releases or source builds |
| Management cluster | `parts: managementCluster` | the kind cluster that hosts Cluster API and CAPT: its node image |
| Workload cluster | `parts: workloadCluster` | the cluster CAPT deploys: Kubernetes version, CNI, OS image |
| Hardware | `parts: hardware` | the architecture, machine counts and sizes, the BMC and libvirt |

The testbed itself sets what more than one part reads, so none owns it: `topology` (`colocated` or
`external`), `bootMode` (`netboot` or `isoboot`) and `network` (IP family, NAT64 and the registry
mirror). One Tinkerbell part serves a netboot and an isoboot testbed alike.

```cue
parts: hardware: ha:             machines: {controlPlane: count: 3, worker: count: 3}
parts: workloadCluster: next:    kubernetesVersion: "v1.37.0"

testbeds: ha: {
	hardware:        "ha"
	workloadCluster: "next" // the parts left out are their kind's built-in
	bootMode:        "isoboot"
}
```

A part is a name or is written inline (`hardware: {machines: worker: count: 5}`). A part left out is
its kind's built-in:

| Part | Built-in | What it is |
|---|---|---|
| Tinkerbell | `chart-latest` | the latest Tinkerbell chart |
| Cluster API | `capi-1.14-capt-0.7` | Cluster API v1.14.4 and CAPT v0.7.0 |
| Management cluster | `kind` | a kind cluster with kind's node image |
| Workload cluster | `k8s-1.36` | Kubernetes v1.36.4 with Cilium (`k8s-1.36-kube-router` uses kube-router) |
| Hardware | `small` | one control plane, one worker and one spare |

Built-ins can't be redefined; copy one under a new name to change it. A version bump renames its
built-in, so a config naming the old one gets an error that lists the current names.

#### A matrix makes a testbed several combinations

A testbed may carry a `matrix:`, written under the testbed in the file that defines it; a testbed
with none shows an example on its page. Each combination takes one value of every axis, and each
value patches the testbed as you would write it: a setting, a part picked by name, or a part's
fields. Three Kubernetes versions on one Tinkerbell are small workload cluster parts and one axis:

```cue
parts: workloadCluster: "k8s-1.37": kubernetesVersion: "v1.37.0" // k8s-1.36 is built in

testbeds: versions: matrix: {
	order: ["k8s", "size"]
	axes: {
		k8s: {"1.36": workloadCluster: "k8s-1.36", "1.37": workloadCluster: "k8s-1.37"}
		size: {
			small: {}
			ha:    hardware: "ha"                           // a string picks a part
			big:   hardware: machines: worker: count: 5     // a struct patches its fields
		}
	}
}
```

A combination is named by its values in `order`, e.g. `1.37-ha`. Two axes that set one value
differently in a combination is an error. `wendy list combinations` lists them, `wendy plan` shows
each combination's parts where they differ, and `wendy list tinkerbell|cluster-api|management-cluster|workload-cluster|hardware`
shows each part, where it is defined and which testbeds use it.

#### Plans

A plan names one testbed and the suites to run on it, and says how the execution behaves:

```cue
plans: nightly: {
	testbed:      "versions"
	combinations: ["1.37-*"]   // glob patterns; empty means every combination
	suites: ["lifecycle"]
	parallel:    2             // combinations at once
	keep:        "failed"      // leave the testbed up: never, failed or always
	diagnostics: "always"      // collect diagnostics: onFailure (the default), always or never
	consoles:    false         // capture the machines' consoles while they run
}
```

The same testbed can be a quiet nightly in one plan and a full set of diagnostics in another.

An upgrade step carries its own target: `with: {version: "v1.37.0"}` on `capt.upgrade.kubernetes`,
`with: {to: ...}` on `tinkerbell.upgrade`, `with: {core: ..., capt: ...}` on `capi.upgrade`. The
built-in `upgrade-k8s`, `upgrade-tinkerbell` and `upgrade-cluster-api` tests have no target and
skip; copy one and set it.

#### Steps

A test writes each step in place, or uses one by name. `uses:` names a built-in step, or one of your
own under `steps:`, which any number of tests can share:

```cue
steps: settle: {exec: command: ["./hack/settle.sh"], timeout: "10m"}

tests: "scale-and-settle": steps: [
	{uses: "capt.scale", with: {target: "worker", by: 1}},
	{uses: "settle"},
	{uses: "settle", name: "settle-again", timeout: "20m"}, // the test's own settings win
]
```

A step of yours is a command, a container, Chainsaw tests, a Ginkgo test binary, or a built-in step
with its parameters set; it can't use another of yours. A test can set `name`, `timeout`, `retries`,
`when`, `continueOnFailure` and `labels` over it. A setting left at the schema's default (a 15m
timeout, no retries) doesn't count, so a test can't set a step that times out later back to exactly
15m. A step that a test uses can't be deleted or renamed.

#### Editing in the UIs

The web UI's top bar is **Home**, **Executions**, **Plans**, **Testbeds** and **Suites** (keys `1`–`5`),
then **Run a plan…** (`n`). Testbeds has a subtab for testbeds and one for each kind of part
(Tinkerbell, Cluster API, Management cluster, Workload cluster, Hardware); Suites has Suites, Tests and
Steps. Each subtab says what it holds, and each entry shows what it sets and what uses it; a
testbed shows its matrix and resolved values. **Config files**, reached from the ⚠ indicator or the
palette (`⌘K`), lists every loaded file with its errors. The web UI creates, edits, copies, renames
and deletes:

- **Plans.** The same cards as Run a plan… (testbed and combinations, suites and labels, options),
  with a name and a description, and the same live preview.
- **Parts.** The settings people change often come first, and the rest sit under "Advanced
  settings". Hardware has a table of machines.
- **Testbeds.** A name, a description, and one picker per kind of part, each saying what that kind is,
  with a one-line summary of what you chose. **New … part…** makes a part in a dialog and picks it,
  without leaving the testbed form, and **Open ↗** shows a part in a new tab. A part written inside the
  testbed in your CUE file shows as "inline" and is kept as it is unless you pick a named part. Its
  settings (where Tinkerbell runs, how machines boot, IP version) list each choice with what it does,
  and the default is marked; keep the defaults unless you are testing one of them. The **matrix**
  editor adds axes, each varying one kind of part or one of those settings over the values you tick;
  the combinations update as you go, and any can be left out with a reason. A matrix whose values set
  more than that is shown as it is written and kept on save; edit it in CUE.
- **Tests** are made of **steps**: a named step, a command, a container, Chainsaw tests or a
  Ginkgo test binary. Steps can be added, reordered and removed.
- **Steps** of your own, with the same fields as one step of a test.
- **Suites** list tests in the order they run.

Wherever the web UI makes an entry, it can delete one you made, after asking: on the entry's page, beside **Edit**,
and in its edit form. Forms that only pick an entry, such as a testbed's part pickers and **Run a plan…**, don't
delete it; **Open ↗** opens a part's page in a new tab.

A part, testbed, suite, test or step that something uses can't be deleted or renamed; the button is disabled and
says what uses it. An entry from one of your own files is deleted in that file, and a built-in can't be deleted.
Leaving a form with unsaved changes asks first.

The TUI edits tests and suites (`a` add, `e` edit, `c` copy, `D` delete). Its testbed and part
editors are still to come.

**Run a plan…** starts an execution from a saved plan or a blank one, with a live preview of what it
will do. Changes are for that execution only; **Save as plan…** keeps them as a new plan. An
execution from a blank form shows "Unsaved plan". Every execution's page has **Settings**, which lists
exactly what it ran with, and **Change and run…**, which opens Run a plan… filled in with them.

Two options say what happens to the testbed, each choice with what it does:

- **When it finishes** (`keep`): tear down the testbed (the default), keep it if something fails, or always keep
  it.
- **When a step fails** (`pauseOnFailure`): stop (the default), or pause and wait for you with everything up. A
  paused combination offers **Retry step**, **Carry on** (record the failure, run the rest) and **Stop**, which
  then keeps or tears down the testbed as When it finishes says. Pausing only applies when a step fails: an
  execution that passes is kept or torn down as When it finishes says.

The UIs write what they create to `./.wendy/wendy-ui.cue` and never touch your own files. Built-ins
and entries from your files can be copied into it to change them, under a new name, since a built-in's
name can't be reused. A change is checked against the schema first and is not written if it is
rejected.

### CI

```sh
wendy run --plan quickstart --plain
```

`--plain` prints one line per event. The exit code is the verdict: `0` passed, `1` failed, `2`
errored, `3` cancelled.

This repository's workflows run wendy on GitHub-hosted runners with KVM. `ci.yaml` runs `quickstart` on each PR.
Two more run on demand (Actions › Run workflow) and put each combination on its own runner, so a matrix takes as
long as its slowest combination: `capt-e2e.yaml` runs the CAPT playground's 16 combinations, and `custom-e2e.yaml`
builds a testbed from a form: matrix axes, Tinkerbell, Cluster API and Kubernetes versions or builds from source,
machines, upgrade targets and diagnostics. The run's summary shows what was planned and each combination's verdict,
and each combination's execution is an artifact.

```sh
gh workflow run custom-e2e.yaml -f suite=lifecycle -f topology=both -f tinkerbell=@main -f upgrade-kubernetes-to=v1.37.0
```

### Executions and kept testbeds

Executions live in `$XDG_STATE_HOME/wendy/executions/<n>` (`--data` to change the root). An
execution's directory is self-contained, so one downloaded from CI can be browsed by putting it in
`<dir>/executions/` and running `wendy serve --data <dir>`. It holds:

```
executions/412/
  execution.json      the summary: status, selection, combinations, failures
  preview.json        what it was asked to do
  events.jsonl        every event, in order
  report.junit.xml    one testsuite per combination
  <combination>/      the combination's testbed: testbed.json, state.json, kubeconfigs/, steps' artifacts,
                      and diagnostics/ when they were collected
```

A testbed kept after an execution (`--keep failed` or `always`) is addressed as
`<execution>/<combination>`, e.g. `412/1.37-ha`, and managed with `wendy env status`,
`wendy env kubeconfig` and `wendy env down`. In the web UI, a kept testbed's card on its execution's
page lists each part with its values labelled. Each cluster's row has **Copy export KUBECONFIG** and
**Download kubeconfig** for that cluster. Home's Kept testbeds has a copy button per cluster, and
Tear down.

To reach the web UI from another machine, set `--addr` and protect it with `WENDY_TOKEN`; open it
once as `http://<host>:7600/?token=<token>`.

## Development

```sh
make build            # bin/wendy
make test
make lint             # golangci-lint, downloaded to out/linters on first use
make generate         # regenerate the web UI templates (*_templ.go)
make snapshot         # release archives in dist/, without publishing
```

The generated templates are committed so that `go install` works without templ. CI fails when they
are out of date.
