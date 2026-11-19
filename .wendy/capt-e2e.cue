// The CAPT playground's e2e matrix (tinkerbell/playground, capt/e2e/cue) as a
// wendy testbed: four axes of two values each, 16 combinations, each running
// the playground's provisioning and health checks.
package local

import "github.com/tinkerbell/wendy/cue/wendy"

wendy.#Config & {
	parts: {
		// The playground's e2e pins this version and deploys kube-router.
		workloadCluster: "k8s-1.35-kube-router": {
			kubernetesVersion: "v1.35.2"
			cni:               "kube-router"
		}
		// The smoke suite claims no spare, so building one only costs time.
		// The disk keeps wendy's size rather than the playground's 4 GiB, which
		// is smaller than the CAPT image's partitions.
		hardware: "small-no-spare": machines: spare: count: 0
	}

	testbeds: "capt-e2e": {
		description:     "The CAPT playground's e2e matrix: topology × IP family × boot mode × registry"
		workloadCluster: "k8s-1.35-kube-router"
		hardware:        "small-no-spare"
		// Combinations are named <topology>-<ipFamily>-<bootMode>-<registry>,
		// as the playground names its combos, and carry the same labels.
		matrix: {
			order: ["topology", "ipFamily", "bootMode", "registry"]
			axes: {
				topology: {colocated: topology: "colocated", external: topology: "external"}
				ipFamily: {ipv4: network: family: "ipv4", ipv6: network: family: "ipv6"}
				bootMode: {netboot: bootMode: "netboot", isoboot: bootMode: "isoboot"}
				registry: {direct: network: mirror: enabled: false, mirror: network: mirror: enabled: true}
			}
		}
	}

	plans: "capt-e2e": {
		description: "CAPT playground e2e matrix · 16 combinations · smoke suite"
		testbed:     "capt-e2e"
		suites: ["smoke"]
	}
}
