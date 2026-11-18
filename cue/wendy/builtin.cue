package wendy

// A testbed that names no part of a kind gets the one #Testbed defaults to.
// Names say what a part is, so a version bump renames its part.
_parts: {
	tinkerbell: "chart-latest": {}
	clusterAPI: "capi-1.14-capt-0.7": {
		core: "v1.14.4"
		capt: version: "v0.7.0"
	}
	managementCluster: kind: {}
	workloadCluster: {
		"k8s-1.36": kubernetesVersion: "v1.36.4"
		"k8s-1.36-kube-router": {
			kubernetesVersion: "v1.36.4"
			cni:               "kube-router"
		}
	}
	hardware: small: {}
}

_testbeds: quickstart: workloadCluster: "k8s-1.36-kube-router"

_plans: quickstart: {
	description: "One small testbed · kind + libvirt · smoke suite"
	testbed:     "quickstart"
	suites: ["smoke"]
}

_deploy: [
	{uses: "capt.cluster.create", timeout: "10m"},
	{uses: "assert.workflows", timeout: "15m"},
	{uses: "assert.apiserver", timeout: "15m"},
	{uses: "workloadCluster.cni", timeout: "10m"},
	{uses: "assert.nodes", timeout: "15m"},
]

_checks: [
	{uses: "assert.capi-ready", timeout: "20m"},
	{uses: "assert.pods", timeout: "15m"},
	{uses: "assert.components", timeout: "15m"},
]

_tests: {
	deploy: {
		description: "Creates the workload cluster and waits until every node is Ready."
		once:        true
		steps:       _deploy
	}
	smoke: {
		description: "Health checks: CAPI and CAPT objects ready, every pod healthy, every Kubernetes component running."
		steps:       _checks
	}
	"scale-out": {
		description: "Adds one worker by claiming a spare machine and waits until it is a Ready node."
		labels: ["scale"]
		requires: spare: 1
		steps: [
			{uses: "capt.scale", timeout: "25m", with: {target: "worker", by: 1}},
			{uses: "assert.healthy", timeout: "20m"},
		]
	}
	"scale-in": {
		description: "Removes one worker and checks its hardware is released."
		labels: ["scale"]
		steps: [
			{uses: "capt.scale", timeout: "20m", with: {target: "worker", by: -1}},
			{uses: "assert.healthy", timeout: "20m"},
		]
	}
	"upgrade-k8s": {
		description: "Rolls the workload cluster to a Kubernetes version, consuming a spare and freeing the old hardware. Copy it and set `with: {version: ...}` on its first step; without a version it skips."
		labels: ["upgrade"]
		requires: spare: 1
		steps: [
			{uses: "capt.upgrade.kubernetes", timeout: "60m"},
			{uses: "assert.healthy", timeout: "20m"},
		]
	}
	"upgrade-tinkerbell": {
		description: "Upgrades the Tinkerbell chart. Copy it and set `with: {to: ...}` on its first step; without a target it skips."
		labels: ["upgrade"]
		steps: [
			{uses: "tinkerbell.upgrade", timeout: "20m"},
			{uses: "assert.healthy", timeout: "20m"},
		]
	}
	"upgrade-cluster-api": {
		description: "Upgrades Cluster API and CAPT with clusterctl. Copy it and set `with: {core: ..., capt: ...}` on its first step; without a target it skips."
		labels: ["upgrade"]
		steps: [
			{uses: "capi.upgrade", timeout: "30m"},
			{uses: "assert.healthy", timeout: "20m"},
		]
	}
	pivot: {
		description: "Moves the CAPI objects from the kind cluster into the workload cluster."
		steps: [
			{uses: "capt.move", timeout: "40m"},
			{uses: "assert.healthy", timeout: "20m"},
		]
	}
	remediation: {
		description: "Powers a worker off through its BMC and waits for a MachineHealthCheck to replace it."
		requires: spare: 1
		steps: [
			{uses: "capt.remediate", timeout: "40m"},
			{uses: "assert.healthy", timeout: "20m"},
		]
	}
	delete: {
		description: "Deletes the workload cluster and checks every machine is deprovisioned."
		steps: [{uses: "capt.cluster.delete", timeout: "20m"}]
	}
}

_suites: {
	smoke: {
		description: "Deploy, then the health checks."
		tests: ["deploy", "smoke"]
	}
	lifecycle: {
		description: "Deploy, scale, upgrade Kubernetes, remediate and delete."
		tests: ["deploy", "scale-out", "upgrade-k8s", "scale-in", "remediation", "delete"]
	}
	upgrades: {
		description: "Deploy, then upgrade Tinkerbell, Cluster API and Kubernetes."
		tests: ["deploy", "upgrade-tinkerbell", "upgrade-cluster-api", "upgrade-k8s"]
	}
}
