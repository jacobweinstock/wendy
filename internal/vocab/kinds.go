package vocab

// Kind is a kind of entry in wendy's config, such as a testbed or a suite.
type Kind struct {
	// ID is the key its entries are written under in CUE, e.g. "testbeds";
	// for a part, under parts:. Built-in steps are not written in CUE, but
	// share "steps" with the user's own.
	ID string
	// Slug names the kind in URLs and CLI arguments, e.g. "workload-cluster".
	Slug string
	// Label heads its tab and list, e.g. "Testbeds" or "Cluster API".
	Label string
	// One and Many name one entry and several in a sentence, e.g. "testbed"
	// and "testbeds", or "Cluster API part" and "Cluster API parts".
	One, Many string
	// Blurb says in a sentence or two what an entry of this kind is.
	Blurb string
	// Part reports whether a testbed picks one entry of this kind.
	Part bool
}

// Kinds are the kinds of config entry, in the order the glossary lists them.
var Kinds = []Kind{
	{
		ID: "plans", Slug: "plans", Label: "Plans", One: "plan", Many: "plans",
		Blurb: "A plan says what to test, where and how: the suites, the testbed, and options such as whether to keep the testbed if something fails.",
	},
	{
		ID: "testbeds", Slug: "testbeds", Label: "Testbeds", One: "testbed", Many: "testbeds",
		Blurb: "A testbed is what tests run on: a part of each kind (Tinkerbell, Cluster API, the management and workload clusters, and the hardware) and a few settings, such as how machines boot. A matrix makes it several combinations.",
	},
	{
		ID: "tinkerbell", Slug: "tinkerbell", Label: "Tinkerbell", One: "Tinkerbell part", Many: "Tinkerbell parts", Part: true,
		Blurb: "How Tinkerbell is installed: a chart release or a build from source, its namespace and its chart values.",
	},
	{
		ID: "clusterAPI", Slug: "cluster-api", Label: "Cluster API", One: "Cluster API part", Many: "Cluster API parts", Part: true,
		Blurb: "Cluster API and CAPT: the releases to install, or CAPT built from source.",
	},
	{
		ID: "managementCluster", Slug: "management-cluster", Label: "Management cluster", One: "management cluster part", Many: "management cluster parts", Part: true,
		Blurb: "The kind cluster that hosts Cluster API and CAPT, and Tinkerbell when it runs there. Its node image sets its Kubernetes version.",
	},
	{
		ID: "workloadCluster", Slug: "workload-cluster", Label: "Workload cluster", One: "workload cluster part", Many: "workload cluster parts", Part: true,
		Blurb: "The cluster CAPT deploys on the machines, and that tests act on: its Kubernetes version, CNI and OS image.",
	},
	{
		ID: "hardware", Slug: "hardware", Label: "Hardware", One: "hardware part", Many: "hardware parts", Part: true,
		Blurb: "The machines tests run on: their architecture, how many of each role and how big, and the BMC and libvirt behind them.",
	},
	{
		ID: "suites", Slug: "suites", Label: "Suites", One: "suite", Many: "suites",
		Blurb: "A suite is a list of tests run in order on one testbed.",
	},
	{
		ID: "tests", Slug: "tests", Label: "Tests", One: "test", Many: "tests",
		Blurb: "A test is an ordered list of steps, such as deploying the workload cluster or scaling it.",
	},
	{
		ID: "steps", Slug: "steps", Label: "Steps", One: "step", Many: "steps",
		Blurb: "A step does one thing: a built-in step that comes with Wendy, such as creating the workload cluster, or one of yours. Tests use them by name.",
	},
}

// KindOf is the kind with the given ID, and whether there is one.
func KindOf(id string) (Kind, bool) {
	for _, k := range Kinds {
		if k.ID == id {
			return k, true
		}
	}
	return Kind{}, false
}
