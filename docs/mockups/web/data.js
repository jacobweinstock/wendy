// Sample data for the UX redesign mockup. Every name here uses the
// vocabulary in ux-redesign/map.md.

const PART_KINDS = ["tinkerbell", "clusterAPI", "managementCluster", "workloadCluster", "hardware"];
const SLUG = { tinkerbell: "tinkerbell", clusterAPI: "cluster-api", managementCluster: "management-cluster", workloadCluster: "workload-cluster", hardware: "hardware" };
const UNSLUG = Object.fromEntries(Object.entries(SLUG).map(([k, v]) => [v, k]));

const KINDS = {
  tinkerbell: {
    label: "Tinkerbell", one: "Tinkerbell part",
    blurb: "How Tinkerbell is installed: a chart release or a build from source, its namespace and chart values.",
    fields: [
      { p: "chart.version", l: "Chart version", d: "latest" },
      { p: "source.repo", l: "Build from repository", adv: 1, h: "A git URL or a local path. A build from source wins over the chart version." },
      { p: "source.ref", l: "Build from branch, tag or commit", adv: 1, h: "Empty with a local path builds the working tree." },
      { p: "namespace", l: "Namespace", d: "tinkerbell", adv: 1 },
      { p: "chart.location", l: "Chart", d: "oci://ghcr.io/tinkerbell/charts/tinkerbell", adv: 1 },
      { p: "values", l: "Chart values", t: "yaml", adv: 1, h: "YAML applied over Wendy's own values." },
      { p: "captainos.flavor", l: "CaptainOS flavor", d: "trixie-slim", adv: 1 },
      { p: "captainos.image", l: "CaptainOS image", d: "ghcr.io/tinkerbell/captain/artifacts:v0.1.0-slim", adv: 1 },
    ],
  },
  clusterAPI: {
    label: "Cluster API", one: "Cluster API part",
    blurb: "Cluster API and CAPT: the releases to install, or CAPT built from source. Nothing here is about a cluster.",
    fields: [
      { p: "core", l: "Cluster API version", d: "v1.14.4", req: 1 },
      { p: "capt.version", l: "CAPT version", d: "v0.7.0", h: "A release. Leave empty when building CAPT from source." },
      { p: "capt.source.repo", l: "Build CAPT from repository", adv: 1, h: "A git URL or a local path." },
      { p: "capt.source.ref", l: "Build CAPT from branch, tag or commit", adv: 1 },
      { p: "capt.repository", l: "CAPT releases", d: "https://github.com/tinkerbell/cluster-api-provider-tinkerbell/releases", adv: 1, h: "A releases URL or a directory." },
    ],
  },
  managementCluster: {
    label: "Management cluster", one: "management cluster part",
    blurb: "The kind cluster that hosts Cluster API and CAPT, and Tinkerbell when it runs there. Its node image sets its Kubernetes version.",
    fields: [
      { p: "image", l: "Node image", d: "kind's default (Kubernetes v1.35.0)", h: "A kind node image, e.g. kindest/node:v1.34.3." },
    ],
  },
  workloadCluster: {
    label: "Workload cluster", one: "workload cluster part",
    blurb: "The cluster CAPT deploys on the machines, and that tests act on: its Kubernetes version, CNI and OS image.",
    fields: [
      { p: "kubernetesVersion", l: "Kubernetes version", d: "v1.36.4", req: 1 },
      { p: "cni", l: "CNI", t: "opts", o: [["cilium", "Cilium"], ["kube-router", "kube-router"]], d: "cilium" },
      { p: "name", l: "Cluster name", d: "capt-playground", adv: 1 },
      { p: "osVersion", l: "Ubuntu version", d: "24.04", adv: 1 },
      { p: "kubeVipVersion", l: "kube-vip version", d: "1.1.2", adv: 1 },
    ],
  },
  hardware: {
    label: "Hardware", one: "hardware part",
    blurb: "The machines tests run on: their architecture, how many of each role and how big, and the BMC and libvirt behind them.",
    fields: [
      { p: "arch", l: "Architecture", t: "opts", o: [["amd64", "amd64 (x86-64)"], ["arm64", "arm64"]], d: "amd64" },
      { t: "machines" },
      { p: "backend.uri", l: "libvirt URI", d: "qemu:///system", adv: 1 },
      { p: "backend.diskPath", l: "Disk directory", d: "/tmp", adv: 1, h: "Where the VMs' disk images are created." },
      { p: "bmc.image", l: "Virtual BMC image", d: "ghcr.io/jacobweinstock/virtualbmc:latest", adv: 1 },
      { p: "bmc.protocol", l: "BMC protocol", t: "opts", o: [["redfish", "Redfish"], ["ipmi", "IPMI"]], d: "redfish", adv: 1 },
    ],
  },
};

const ROLES = [["controlPlane", "Control plane"], ["worker", "Worker"], ["spare", "Spare"]];
const MCOLS = [["count", "Machines", 1], ["cpus", "CPUs", 2], ["memMiB", "Memory MiB", 2048], ["diskGiB", "Disk GiB", 10]];

const SETTINGS = {
  topology: { l: "Tinkerbell runs", o: [["colocated", "On the management cluster"], ["external", "On its own cluster"]], d: "colocated", h: "Its own cluster is a second kind cluster, as in a real deployment." },
  bootMode: { l: "Boot mode", o: [["netboot", "Network boot"], ["isoboot", "ISO boot"]], d: "netboot", h: "How machines start the Tinkerbell installer: over the network, or from a virtual CD through the BMC." },
  family: { l: "IP family", o: [["ipv4", "IPv4"], ["ipv6", "IPv6"]], d: "ipv4" },
};

const BLURBS = {
  plans: "A plan says what to test, where and how: the suites, the testbed, and options such as whether to keep the testbed if something fails. Plans are written in CUE.",
  testbeds: "A testbed is what tests run on: one part of each kind, plus the settings no part owns: where Tinkerbell runs, how machines boot, and the network. Add a matrix to test several combinations.",
  suites: "A suite is a list of tests run in order on one testbed. Plans pick the suites to run.",
  tests: "A test is an ordered list of steps, such as deploying the workload cluster or scaling it.",
  steps: "Built-in steps come with Wendy. Add one to a test as a step of kind Built-in step.",
};

const STEP_KINDS = [
  ["uses", "Built-in step", "A step that comes with Wendy, such as creating or scaling the workload cluster."],
  ["exec", "Command", "Runs a program on this host, with the testbed's kubeconfigs in its environment variables."],
  ["container", "Container", "Runs a container image, with the testbed's kubeconfigs mounted."],
  ["chainsaw", "Chainsaw tests", "Runs declarative Kubernetes tests, written for Kyverno Chainsaw, from a directory in your config."],
  ["ginkgo", "Ginkgo test binary", "Runs a compiled Go test binary written with Ginkgo, and records its results."],
];

const D = {
  firstLaunch: false,
  configError: false,
  plans: {
    quickstart: { desc: "The default testbed and the smoke suite. Works with no config at all.", testbed: "quickstart", suites: ["smoke"], parallel: 1, keep: "never", pause: false, diagnostics: "onFailure", consoles: true, labels: "", origin: "built-in", last: 408 },
    "pr-check": { desc: "Every pull request: one testbed, smoke suite.", testbed: "default", suites: ["smoke"], parallel: 1, keep: "never", pause: false, diagnostics: "onFailure", consoles: true, labels: "", origin: "./.wendy/wendy.cue", last: 411 },
    "my-branch": { desc: "Tinkerbell built from my branch, the full lifecycle, and pause if it fails.", testbed: "my-branch", suites: ["lifecycle"], parallel: 1, keep: "failed", pause: true, diagnostics: "onFailure", consoles: true, labels: "", origin: "./.wendy/wendy.cue", last: 410 },
    nightly: { desc: "Two Kubernetes versions × two boot modes, the full lifecycle.", testbed: "versions", suites: ["lifecycle"], parallel: 2, keep: "failed", pause: false, diagnostics: "always", consoles: true, labels: "", origin: "./.wendy/nightly.cue", last: 412 },
    "upgrade-from-release": { desc: "Start from the last release, then upgrade Tinkerbell, Cluster API and Kubernetes.", testbed: "default", suites: ["upgrades"], parallel: 1, keep: "failed", pause: false, diagnostics: "onFailure", consoles: true, labels: "", origin: "./.wendy/wendy.cue", last: 405 },
  },
  testbeds: {
    default: { desc: "One default part of each kind.", parts: {}, origin: "built-in" },
    quickstart: { desc: "The default testbed with kube-router as the CNI.", parts: { workloadCluster: "quickstart" }, origin: "built-in" },
    "my-branch": { desc: "Tinkerbell built from ~/src/tinkerbell; everything else default.", parts: { tinkerbell: "my-branch" }, origin: "./.wendy/wendy.cue" },
    versions: {
      desc: "Kubernetes 1.36 and 1.37, each with network boot and ISO boot.", parts: {}, origin: "./.wendy/nightly.cue",
      matrix: { order: ["k8s", "boot"], axes: { k8s: { "1.36": ["workloadCluster", "1.36"], "1.37": ["workloadCluster", "1.37"] }, boot: { net: ["bootMode", "netboot"], iso: ["bootMode", "isoboot"] } } },
    },
    ha: { desc: "Three control plane and three worker machines, ISO boot.", parts: { hardware: "ha" }, bootMode: "isoboot", origin: "./.wendy/wendy-ui.cue" },
    ipv6: { desc: "IPv6 with NAT64 and a registry mirror, Tinkerbell on its own cluster.", parts: { managementCluster: "k8s-1.34" }, family: "ipv6", topology: "external", nat64: true, mirror: true, origin: "./.wendy/wendy-ui.cue" },
  },
  parts: {
    tinkerbell: {
      default: { origin: "built-in", v: {} },
      "my-branch": { origin: "./.wendy/wendy.cue", v: { "source.repo": "~/src/tinkerbell" } },
      "v0.22": { origin: "./.wendy/wendy.cue", v: { "chart.version": "v0.22.1" } },
    },
    clusterAPI: {
      default: { origin: "built-in", v: { core: "v1.14.4", "capt.version": "v0.7.0" } },
      "capt-main": { origin: "./.wendy/wendy.cue", v: { core: "v1.14.4", "capt.source.repo": "https://github.com/tinkerbell/cluster-api-provider-tinkerbell", "capt.source.ref": "main" } },
    },
    managementCluster: {
      default: { origin: "built-in", v: {} },
      "k8s-1.34": { origin: "./.wendy/wendy-ui.cue", v: { image: "kindest/node:v1.34.3" } },
    },
    workloadCluster: {
      default: { origin: "built-in", v: { kubernetesVersion: "v1.36.4" } },
      quickstart: { origin: "built-in", v: { kubernetesVersion: "v1.36.4", cni: "kube-router" } },
      "1.36": { origin: "./.wendy/nightly.cue", v: { kubernetesVersion: "v1.36.4" } },
      "1.37": { origin: "./.wendy/nightly.cue", v: { kubernetesVersion: "v1.37.0" } },
    },
    hardware: {
      default: { origin: "built-in", v: {} },
      ha: { origin: "./.wendy/wendy-ui.cue", v: { "machines.controlPlane.count": 3, "machines.worker.count": 3 } },
      arm: { origin: "./.wendy/wendy.cue", v: { arch: "arm64", "machines.worker.cpus": 4 } },
    },
  },
  suites: {
    smoke: { desc: "Deploy, then the health checks.", tests: ["deploy", "smoke"], origin: "built-in" },
    lifecycle: { desc: "Deploy, scale out, upgrade Kubernetes, scale in, remediate and delete.", tests: ["deploy", "scale-out", "upgrade-k8s", "scale-in", "remediation", "delete"], origin: "built-in" },
    upgrades: { desc: "Deploy, then upgrade Tinkerbell, Cluster API and Kubernetes.", tests: ["deploy", "upgrade-tinkerbell", "upgrade-cluster-api", "upgrade-k8s"], origin: "built-in" },
    mine: { desc: "Deploy, then our Chainsaw tests.", tests: ["deploy", "hardware-objects"], origin: "./.wendy/wendy-ui.cue" },
  },
  tests: {
    deploy: { desc: "Creates the workload cluster and waits until every node is Ready.", once: true, mins: 6, steps: [["capt.cluster.create", "10m"], ["assert.workflows", "15m"], ["assert.apiserver", "15m"], ["workload.cni", "10m"], ["assert.nodes", "15m"]] },
    smoke: { desc: "Health checks: Cluster API and CAPT objects Ready, every pod healthy, every Tinkerbell component running.", mins: 3, steps: [["assert.capi-ready", "20m"], ["assert.pods", "15m"], ["assert.components", "15m"]] },
    "scale-out": { desc: "Adds a worker by claiming a spare machine, and waits until it is a Ready node.", labels: ["scale"], spare: 1, mins: 7, steps: [["capt.scale", "25m", "target: worker\nby: 1"], ["assert.healthy", "20m"]] },
    "scale-in": { desc: "Removes a worker and checks its machine is released.", labels: ["scale"], mins: 4, steps: [["capt.scale", "20m", "target: worker\nby: -1"], ["assert.healthy", "20m"]] },
    "upgrade-k8s": { desc: "Rolls the workload cluster to a new Kubernetes version, using a spare machine.", labels: ["upgrade"], spare: 1, mins: 12, steps: [["capt.upgrade.kubernetes", "60m", "version: v1.37.0"], ["assert.healthy", "20m"]] },
    "upgrade-tinkerbell": { desc: "Upgrades the Tinkerbell chart.", labels: ["upgrade"], mins: 5, steps: [["tinkerbell.upgrade", "20m", "to:\n  chart:\n    version: v0.22.1"], ["assert.healthy", "20m"]] },
    "upgrade-cluster-api": { desc: "Upgrades Cluster API and CAPT with clusterctl.", labels: ["upgrade"], mins: 6, steps: [["capi.upgrade", "30m", "core: v1.14.4\ncapt: v0.7.0"], ["assert.healthy", "20m"]] },
    pivot: { desc: "Moves the Cluster API objects from the management cluster into the workload cluster.", mins: 8, steps: [["capt.move", "40m"], ["assert.healthy", "20m"]] },
    remediation: { desc: "Powers a worker off through its BMC and waits for it to be replaced.", spare: 1, mins: 8, steps: [["capt.remediate", "40m"], ["assert.healthy", "20m"]] },
    delete: { desc: "Deletes the workload cluster and checks every machine is deprovisioned.", mins: 3, steps: [["capt.cluster.delete", "20m"]] },
    "hardware-objects": { desc: "Our Chainsaw tests for Tinkerbell Hardware objects.", origin: "./.wendy/wendy-ui.cue", labels: ["hardware"], mins: 3, steps: [{ kind: "chainsaw", name: "hardware objects", path: "tests/chainsaw/hardware", timeout: "15m" }] },
  },
  steps: {
    "capt.cluster.create": ["Creates the workload cluster from CAPT's cluster template and waits for its control plane.", "{}"],
    "assert.workflows": ["Waits until every machine's Tinkerbell workflow succeeded.", "{}"],
    "assert.apiserver": ["Waits until the workload cluster's API server answers.", "{}"],
    "workload.cni": ["Installs the CNI in the workload cluster.", "{}"],
    "assert.nodes": ["Waits until every node in the workload cluster is Ready.", "{}"],
    "assert.capi-ready": ["Checks every Cluster API and CAPT object is Ready.", "{}"],
    "assert.pods": ["Checks every pod is running or completed.", "{}"],
    "assert.components": ["Checks every Tinkerbell component is running.", "{}"],
    "assert.healthy": ["Runs the three health checks above as one step.", "{}"],
    "capt.scale": ["Scales the workers or the control plane up or down.", "{\n  target: \"worker\" | \"controlPlane\"\n  by:     int  // e.g. 1 or -1\n}"],
    "capt.upgrade.kubernetes": ["Rolls the workload cluster to a Kubernetes version.", "{\n  version: string  // e.g. \"v1.37.0\"; without it the step skips\n}"],
    "tinkerbell.upgrade": ["Upgrades the Tinkerbell chart.", "{\n  to: {chart?: {version: string}, source?: {repo: string, ref?: string}}\n}"],
    "capi.upgrade": ["Upgrades Cluster API and CAPT with clusterctl.", "{\n  core?: string\n  capt?: string\n}"],
    "capt.move": ["Moves the Cluster API objects to another cluster.", "{}"],
    "capt.remediate": ["Powers a machine off through its BMC and waits for it to be replaced.", "{}"],
    "capt.cluster.delete": ["Deletes the workload cluster and waits until its machines are deprovisioned.", "{}"],
  },
  // ago is minutes since the execution started; secs is how long it took (or has taken).
  execs: [
    { id: 412, plan: "nightly", status: "running", ago: 22, secs: 22 * 60, eta: "about 1 h 30 min left",
      combos: { "1.36-net": { mark: { at: "lifecycle/scale-out/capt.scale", st: "running" } }, "1.36-iso": { mark: { at: "lifecycle/deploy/assert.workflows", st: "running" } }, "1.37-net": { mark: "waiting" }, "1.37-iso": { mark: "waiting" } } },
    { id: 411, plan: "pr-check", status: "passed", ago: 120, secs: 842 },
    { id: 410, plan: "my-branch", status: "paused", ago: 300, secs: 2470, kept: true, mark: { at: "lifecycle/scale-out/capt.scale", st: "failed" } },
    { id: 409, plan: "nightly", status: "errored", ago: 1440, secs: 6720,
      combos: { "1.36-net": {}, "1.36-iso": {}, "1.37-net": {}, "1.37-iso": { mark: { at: "setup/machines.create", st: "errored" } } } },
    { id: 408, plan: "quickstart", status: "passed", ago: 1500, secs: 831 },
    { id: 407, plan: "pr-check", status: "failed", ago: 2900, secs: 1080, mark: { at: "smoke/smoke/assert.pods", st: "failed" } },
    { id: 405, plan: "upgrade-from-release", status: "failed", ago: 4400, secs: 1990, kept: true, mark: { at: "upgrades/upgrade-cluster-api/capi.upgrade", st: "failed" } },
  ],
  failures: {
    410: { title: "capt.scale failed: worker-2 did not become a Ready node within 25m", msg: "MachineDeployment capt-playground-md-0 has 1 of 2 ready replicas.", related: "Tinkerbell workflow capt-playground-worker-2 stopped at action stream-image: context deadline exceeded" },
    407: { title: "assert.pods failed: 1 pod is not running", msg: "kube-system/cilium-operator-7d9c8b5f4-x2kqp is CrashLoopBackOff (6 restarts).", related: "" },
    405: { title: "capi.upgrade failed: clusterctl upgrade apply exited with code 1", msg: "infrastructure-tinkerbell: version v0.7.0 is not compatible with Cluster API v1.15.0 (contract v1beta2).", related: "" },
    409: { title: "machines.create errored: libvirt could not start worker-1", msg: "Cannot access storage file /tmp/wendy-409/worker-1.qcow2: No space left on device", related: "", errored: true },
  },
};

const SETUP_STEPS = ["management.create", "network.create", "bmc.start", "machines.create", "tinkerbell.install", "clusterapi.install", "hardware.apply"];
const DIAG_STEPS = ["collect.cluster", "collect.tinkerbell", "collect.bmc", "collect.consoles"];

const ST = {
  passed: ["✓", "passed", "pass"], failed: ["✗", "failed", "fail"], errored: ["!", "errored", "err"],
  running: ["●", "running", "run"], waiting: ["○", "waiting", "skip"], paused: ["‖", "paused", "pause"],
  skipped: ["–", "skipped", "skip"], cancelled: ["⊘", "cancelled", "skip"],
};

const GLOSSARY = [
  ["Plan", "What to test, where and how: the suites, the testbed, and options. You run a plan.", "Profile"],
  ["Execution", "One time Wendy carries out a plan. Has a number (#412), a status and a verdict.", "Run (noun)"],
  ["run (verb)", "Always with an object: run a plan, run tests. Never a noun.", "—"],
  ["Preview", "What a plan would do, shown before it runs.", "wendy plan, the Plan pane"],
  ["Testbed", "What tests run on: one part of each kind, the settings no part owns, and an optional matrix.", "Environment"],
  ["Part", "A reusable piece of a testbed: Tinkerbell, Cluster API, Management cluster, Workload cluster, Hardware.", "Tinkerbell/Providers/Hardware/Workload part"],
  ["Matrix · Axis · Combination", "A testbed's axes; each combination is one concrete testbed.", "Matrix kind, combo"],
  ["Kept testbed", "A built testbed an execution left up, addressed as <execution>/<combination>.", "Environment (live)"],
  ["Suite · Test · Step", "Tests run in order · a named list of steps · one action.", "Scenario"],
  ["Label", "A tag on a test or combination. “Only tests with labels” picks by label.", "Axes, Ginkgo label filter"],
  ["Diagnostics", "What Wendy collects to debug a failure.", "Evidence"],
  ["Artifacts", "Any file an execution saved.", "Evidence links, Files"],
];
