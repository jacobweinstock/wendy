// Package wendy is the schema every wendy configuration is unified with.
//
// Users import it as "github.com/tinkerbell/wendy/cue/wendy". wendy embeds
// this directory and overlays it into every config directory it loads, so the
// import needs no network access and no cue.mod of the user's own.
package wendy

#Config: {
	parts: {
		tinkerbell: [string]:        #TinkerbellPart
		clusterAPI: [string]:        #ClusterAPIPart
		managementCluster: [string]: #ManagementClusterPart
		workloadCluster: [string]:   #WorkloadClusterPart
		hardware: [string]:          #HardwarePart
	}
	testbeds: [string]: #Testbed
	tests: [string]:    #Test
	suites: [string]:   #Suite
	plans: [string]:    #Plan
	// Your own steps, named so many tests can use them with uses:.
	steps: [string]: #Step

	// Built-ins, so a first launch with no config still has something to run.
	parts:    _parts
	testbeds: _testbeds
	tests:    _tests
	suites:   _suites
	plans:    _plans
}

// A part is one reusable piece of a testbed. Its fields are written relative
// to the part and hold only what the user sets: wendy assembles the parts into
// a #ResolvedTestbed, which checks them and supplies every default. Keeping
// defaults out of parts is what lets a matrix axis override them.
//
// A Tinkerbell part holds how Tinkerbell is installed: chart, source, values,
// namespace and captainos, and cluster for the external topology. Topology,
// boot mode and the network are the testbed's, as each is read by more than
// one part.
#TinkerbellPart: {...}

// A Cluster API part holds core and capt: the Cluster API release, and the
// CAPT release or build.
#ClusterAPIPart: {...}

// A Management cluster part holds type and image: the cluster Cluster API is
// installed on.
#ManagementClusterPart: {...}

// A Workload cluster part holds the workload cluster's fields: name,
// kubernetesVersion, osVersion, cni and kubeVipVersion.
#WorkloadClusterPart: {...}

// A Hardware part holds arch, backend, bmc and machines.
#HardwarePart: {...}

// A part is named, or written inline for a one-off. An omitted part is its
// kind's fallback, the default in #Testbed.
#PartRef: string | {...}

// #Testbed picks one of each part, and sets the settings no single part owns.
// What an execution collects on failure is the plan's diagnostics, and what an
// upgrade moves to is its step's parameters. The settings are checked, and
// defaulted, in #ResolvedTestbed. Each part's default is the built-in a
// testbed gets when it names none; wendy reads them from here.
#Testbed: {
	description?:      string
	tinkerbell:        *"chart-latest" | #PartRef
	clusterAPI:        *"capi-1.14-capt-0.7" | #PartRef
	managementCluster: *"kind" | #PartRef
	workloadCluster:   *"k8s-1.36" | #PartRef
	hardware:          *"small" | #PartRef
	topology?:         string
	bootMode?:         string
	network?: {...}
	// A matrix makes the testbed several combinations: one for each choice of
	// a value on every axis. Without one, the testbed is one combination,
	// named after it.
	matrix?: {
		// Axis order; a combination is named by its values joined in this
		// order.
		order: [...string]
		axes: [string]: [string]: #TestbedPatch
		exclude: [...{[string]: string, reason: string}]
	}
}

// #TestbedPatch is one value of a matrix axis: fields of the testbed, written
// as the testbed writes them, that override the testbed's own. A part named
// here replaces the testbed's; a part's fields written here are merged over
// the part the testbed uses.
#TestbedPatch: {
	tinkerbell?:        #PartRef
	clusterAPI?:        #PartRef
	managementCluster?: #PartRef
	workloadCluster?:   #PartRef
	hardware?:          #PartRef
	topology?:          string
	bootMode?:          string
	network?: {...}
}

// #ResolvedTestbed is a testbed with its parts assembled. It is what an
// execution builds from; wendy fills it in and never takes it from a config
// file. Its diagnostics are the execution's: the preview sets them from the
// plan.
#ResolvedTestbed: {
	arch:              *"amd64" | "arm64"
	network:           #Network
	managementCluster: #Distro
	tinkerbell:        #Tinkerbell
	clusterAPI: {
		core: =~"^v[0-9]+\\.[0-9]+\\.[0-9]+"
		capt: #CAPT
	}
	hardware:        #Hardware
	workloadCluster: #WorkloadCluster
	diagnostics:     #Diagnostics
	// setup is the preamble derived from the fields above. It is computed here
	// so `wendy plan` shows it; it is never set by hand.
	setup: [...#Step]
	setup: [
		{uses: "testbed.network", timeout: "10m"},
		{uses: "testbed.registry", timeout: "10m"},
		{uses: "testbed.distro", timeout: "15m", with: cluster: "management"},
		if tinkerbell.topology == "external" {
			{uses: "testbed.distro", timeout: "15m", with: cluster: "tinkerbell"}
		},
		{uses: "testbed.bmc", timeout: "10m"},
		{uses: "tinkerbell.install", timeout: "20m"},
		{uses: "capi.init", timeout: "20m"},
		{uses: "hardware.create", timeout: "10m"},
		{uses: "hardware.register", timeout: "10m"},
	]
}

#Network: {
	family: *"ipv4" | "ipv6"
	nat64:  *(family == "ipv6") | bool
	mirror: {
		enabled: *false | bool
		upstreams: [...string] | *["ghcr.io", "quay.io", "registry.k8s.io", "gcr.io", "docker.io"]
	}
}

// #Distro is a management or Tinkerbell cluster. v1 has kind only; new types
// are added as new disjuncts.
#Distro: {
	type:   *"kind" | "kind"
	image?: string
}

// #Source replaces a release with a build, wherever a version is accepted.
#Source: {
	// A git URL or a local path. A leading ~ is expanded.
	repo?: string
	// A branch, tag or commit. Omitted with a local path, the working tree is
	// built as it is, uncommitted changes included.
	ref?: string
}

#Chart: {
	location: *"oci://ghcr.io/tinkerbell/charts/tinkerbell" | string
	version:  *"latest" | string
}

// A source sits alongside the release fields and wins over them, so a part can
// be derived from one that names a release:
//   parts: tinkerbell: dev: parts.tinkerbell.stable & {source: repo: "~/src/tinkerbell"}
#TinkerbellSource: #Source & {repo: *"https://github.com/tinkerbell/tinkerbell" | string}

#Tinkerbell: {
	topology: *"colocated" | "external"
	if topology == "external" {
		cluster: #Distro
	}
	chart:   #Chart
	source?: #TinkerbellSource
	// Chart values, applied over wendy's own for releases and builds alike.
	values: {...}
	bootMode:  *"netboot" | "isoboot"
	namespace: *"tinkerbell" | string
	captainos: {
		flavor: *"trixie-slim" | string
		image:  *"ghcr.io/tinkerbell/captain/artifacts:v0.1.0-slim" | string
	}
}

#CAPT: {
	// A release: required unless source is set.
	version?: =~"^v[0-9]+\\.[0-9]+\\.[0-9]+"
	// A clusterctl provider repository: a GitHub releases URL or a directory.
	repository: *"https://github.com/tinkerbell/cluster-api-provider-tinkerbell/releases" | string
	source?:    #Source & {repo: *"https://github.com/tinkerbell/cluster-api-provider-tinkerbell" | string}
	if source == _|_ {
		version: string
	}
}

#Hardware: {
	// v1 has libvirt only.
	backend: {
		type:     *"libvirt" | "libvirt"
		uri:      *"qemu:///system" | string
		diskPath: *"/tmp" | string
		// A pinned OVMF build. Without it libvirt picks the firmware itself.
		firmware?: {code: string, vars: string}
	}
	bmc: {
		image:    *"ghcr.io/jacobweinstock/virtualbmc:latest" | string
		protocol: *"redfish" | "ipmi"
	}
	machines: [Role=string]: {
		count:   int & >=0
		cpus:    *2 | int & >0
		memMiB:  *2048 | int & >0
		diskGiB: *10 | int & >0
		labels: [string]: string
	}
	machines: {
		controlPlane: count: *1 | int & >=1
		worker: count:       *1 | int
		spare: count:        *1 | int
	}
}

#WorkloadCluster: {
	name:              *"wendy" | =~"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
	kubernetesVersion: =~"^v[0-9]+\\.[0-9]+\\.[0-9]+$"
	// The image is always CAPT's Ubuntu image; only this varies.
	osVersion:      *"2404" | string
	cni:            *"cilium" | "kube-router"
	kubeVipVersion: *"1.1.2" | string
}

// #TinkerbellTarget is a chart release or a build for tinkerbell.upgrade.
#TinkerbellTarget: {
	chart?:  #Chart
	source?: #TinkerbellSource
}

// #Diagnostics is what an execution collects about a failure: when to collect
// the clusters' state, Tinkerbell's objects and the BMC logs, and whether to
// capture the machines' consoles.
#Diagnostics: {
	collect:  *"onFailure" | "always" | "never"
	consoles: *true | bool
}

// #Plan says what to test, where and how: the suites, the testbed, and how
// to execute them.
#Plan: {
	description?: string
	testbed:      string
	// Glob patterns over the testbed's combination names; empty means all.
	// Only a testbed with a matrix has combinations to pick.
	combinations: [...string] | *[]
	suites: [...string]
	labelFilter: *"" | string
	parallel:    *1 | int & >=1
	// Whether to leave the testbed up when the execution ends: never, when
	// it failed, or always.
	keep:           *"never" | "failed" | "always"
	pauseOnFailure: *false | bool
	// When to collect diagnostics: on failure, always or never.
	diagnostics: #Diagnostics.collect
	// Whether to capture the machines' consoles while they run.
	consoles: #Diagnostics.consoles
}

#SecretRef: {env: string} | {file: string}

#Test: {
	description: *"" | string
	labels: [...string]
	requires: spare: *0 | int & >=0
	// once marks a test that only has to pass once per testbed, such as
	// deploy. Later suites that list it reuse that result.
	once: *false | bool
	steps: [...#Step]
}

#Suite: {
	description: *"" | string
	tests: [...string]
}

