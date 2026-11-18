package wendy

import "list"

// #Step is exactly one of uses, exec, container, chainsaw or ginkgo. The
// presets (chainsaw, ginkgo) are pure CUE that expand to container or exec;
// the engine never sees them.
#Step: {
	name?: string
	labels: [...string]
	timeout: *"15m" | =~"^([0-9]+(\\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$"
	retries: *0 | int & >=0
	// A label-filter expression over the combination's labels.
	when?:             string
	continueOnFailure: *false | bool

	// A named step: a built-in, with its parameters in with, or one of your
	// own steps (#Config.steps), which takes no parameters. #Builtins maps
	// each built-in to its params; wendy generates it from the steps compiled
	// into the binary and adds it at load time. Wendy checks that any other
	// name is one of your steps, since they may be defined in another file.
	uses?: string
	if uses != _|_ if #Builtins[uses] != _|_ {
		with: #Builtins[uses]
	}

	exec?: {
		command: [string, ...string]
		env: [string]: string
	}

	container?: {
		image: string
		command?: [...string]
		args?: [...string]
		env: [string]: string
	}

	chainsaw?: {
		path: string
		selector: [...string]
		image: *"ghcr.io/kyverno/chainsaw:v0.2.15" | string
	}
	if chainsaw != _|_ {
		container: {
			image: chainsaw.image
			args: [
				"test", "--test-dir", "/wendy/work/" + chainsaw.path,
				"--cluster", "management=/wendy/bundle/kubeconfigs/management",
				"--cluster", "workload=/wendy/bundle/kubeconfigs/workload",
				"--values", "/wendy/bundle/state.json",
				"--report-format", "JUNIT-TEST", "--report-path", "/wendy/artifacts", "--report-name", "junit",
				for s in chainsaw.selector {"--selector=" + s},
			]
		}
	}

	ginkgo?: {
		binary:      string
		labelFilter: *"" | string
		args: [...string]
	}
	if ginkgo != _|_ {
		exec: command: list.Concat([[ginkgo.binary, "--ginkgo.label-filter=" + ginkgo.labelFilter,
			"--ginkgo.junit-report=${WENDY_ARTIFACTS_DIR}/junit.xml"], ginkgo.args])
	}
}
