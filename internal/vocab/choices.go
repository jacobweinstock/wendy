package vocab

// choice is a value a form field offers, the label people read for it, and
// optionally what picking it does.
type choice struct{ value, label, help string }

// choices are the values each form field offers, by field path, in the order
// a form lists them.
var choices = map[string][]choice{
	"topology": {
		{"colocated", "On the management cluster", "Tinkerbell shares the kind cluster with Cluster API and CAPT. The simplest setup, and the lightest."},
		{"external", "On its own cluster", "Tinkerbell gets a kind cluster of its own, which CAPT reaches from the management cluster. Pick this to test Tinkerbell kept apart from Cluster API."},
	},
	"bootMode": {
		{"netboot", "Network boot", "Machines boot over the network from Tinkerbell, with PXE and iPXE. How most installs boot."},
		{"isoboot", "ISO boot", "Machines boot from an ISO image that Tinkerbell serves, which the BMC attaches as virtual media. Pick this to test booting without PXE."},
	},
	"network.family": {
		{"ipv4", "IPv4", "Machines and clusters use IPv4 addresses."},
		{"ipv6", "IPv6", "Machines and clusters use IPv6 addresses. NAT64 lets them reach registries that are IPv4 only."},
	},
	"arch":         {{"amd64", "amd64 (x86-64)", ""}, {"arm64", "arm64", ""}},
	"cni":          {{"cilium", "Cilium", ""}, {"kube-router", "kube-router", ""}},
	"bmc.protocol": {{"redfish", "Redfish", ""}, {"ipmi", "IPMI", ""}},
	"diagnostics":  {{"onFailure", "On failure", ""}, {"always", "Always", ""}, {"never", "Never", ""}},
	"keep":         {{"never", "Never", ""}, {"failed", "If it fails", ""}, {"always", "Always", ""}},
	"step.kind": {
		{"uses", "Named step", ""},
		{"exec", "Command", ""},
		{"container", "Container", ""},
		{"chainsaw", "Chainsaw tests", ""},
		{"ginkgo", "Ginkgo test binary", ""},
	},
}

// Choice is the label a form shows for value of the field at path, e.g. "On
// the management cluster" for topology colocated. The form still saves value.
// A value with no label reads as itself.
func Choice(path, value string) string {
	for _, c := range choices[path] {
		if c.value == value {
			return c.label
		}
	}
	return value
}

// ChoiceHelp says what picking value of the field at path does, and when to
// pick it, or "" when the choice has no description.
func ChoiceHelp(path, value string) string {
	for _, c := range choices[path] {
		if c.value == value {
			return c.help
		}
	}
	return ""
}

// Described reports whether every value the field at path offers has a
// description, so a form can list them with it.
func Described(path string) bool {
	for _, c := range choices[path] {
		if c.help == "" {
			return false
		}
	}
	return len(choices[path]) > 0
}

// HasChoice reports whether Choice has a label for value of the field at path.
func HasChoice(path, value string) bool {
	for _, c := range choices[path] {
		if c.value == value {
			return true
		}
	}
	return false
}

// ChoiceValues are the values Choice labels for the field at path, in the
// order a form lists them.
func ChoiceValues(path string) []string {
	out := make([]string, 0, len(choices[path]))
	for _, c := range choices[path] {
		out = append(out, c.value)
	}
	return out
}
