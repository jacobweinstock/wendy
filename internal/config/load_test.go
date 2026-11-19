package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	wendycue "github.com/tinkerbell/wendy/cue"
)

// testBuiltins accepts any params for every step the built-in tests use.
func testBuiltins() []Builtin {
	ids := []string{
		"testbed.network", "testbed.registry", "testbed.distro", "testbed.bmc",
		"tinkerbell.install", "tinkerbell.upgrade", "capi.init", "capi.upgrade",
		"hardware.create", "hardware.register",
		"capt.cluster.create", "capt.cluster.delete", "capt.scale", "capt.upgrade.kubernetes", "capt.move", "capt.remediate",
		"workloadCluster.cni", "assert.workflows", "assert.apiserver", "assert.nodes", "assert.capi-ready", "assert.pods",
		"assert.components", "assert.healthy",
		"collect.cluster", "collect.tinkerbell", "collect.bmc", "collect.consoles",
	}
	var out []Builtin
	for _, id := range ids {
		out = append(out, Builtin{ID: id, Schema: "{...}"})
	}
	return out
}

func writeConfig(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// resolved is what the named testbed resolves to, failing the test when it
// does not.
func resolved(t *testing.T, cat *Catalog, name string) Testbed {
	t.Helper()
	tb := cat.Testbeds[name]
	if tb == nil || tb.Resolved == nil {
		t.Fatalf("testbed %s did not resolve: %v", name, cat.AllErrors())
	}
	return *tb.Resolved
}

func TestLoadBuiltinsOnly(t *testing.T) {
	cat := (&Loader{Builtins: testBuiltins()}).Load()
	if errs := cat.AllErrors(); len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if cat.HasUserConfig {
		t.Error("HasUserConfig with no directories")
	}
	qs, ok := cat.Plans["quickstart"]
	if !ok {
		t.Fatal("no quickstart plan")
	}
	if qs.Origin != BuiltIn {
		t.Errorf("quickstart origin = %q", qs.Origin)
	}
	tb := resolved(t, cat, "quickstart")
	if got := tb.Hardware.Machines["spare"].Count; got != 1 {
		t.Errorf("spare count = %d", got)
	}
	if got := len(tb.Setup); got != 8 {
		t.Errorf("setup has %d steps, want 8", got)
	}
	if !cat.Tests["deploy"].Value.Once {
		t.Error("deploy should be once")
	}
	if got := cat.Suites["smoke"].Value.Tests; strings.Join(got, ",") != "deploy,smoke" {
		t.Errorf("smoke suite = %v", got)
	}
}

const example = `package acme

import "github.com/tinkerbell/wendy/cue/wendy"

wendy.#Config & {
	parts: clusterAPI: acme: {core: "v1.12.4", capt: version: "v0.7.0"}

	testbeds: default: clusterAPI: "acme"

	tests: "acme-smoke": {
		description: "Our app runs on a freshly provisioned cluster"
		steps: [
			{name: "deploy-app", container: image: "ghcr.io/acme/e2e-smoke:v1.4.0"},
			{name: "policies", chainsaw: path: "chainsaw/policies"},
			{name: "suite", ginkgo: binary: "./e2e.test"},
		]
	}

	suites: nightly: tests: ["deploy", "acme-smoke", "scale-out", "upgrade-k8s", "scale-in", "delete"]

	parts: tinkerbell: dev: source: repo: "~/repos/tinkerbell/tinkerbell"
	parts: clusterAPI: dev: {
		core: "v1.12.4"
		capt: source: repo: "~/repos/tinkerbell/cluster-api-provider-tinkerbell"
	}
	testbeds: dev: {tinkerbell: "dev", clusterAPI: "dev"}

	testbeds: full: {
		clusterAPI: "acme"
		matrix: {
			order: ["topology", "ipFamily", "bootMode", "registry"]
			axes: {
				topology: {colocated: topology: "colocated", external: {topology: "external", tinkerbell: cluster: type: "kind"}}
				ipFamily: {ipv4: network: family: "ipv4", ipv6: network: family: "ipv6"}
				bootMode: {netboot: bootMode: "netboot", isoboot: bootMode: "isoboot"}
				registry: {direct: network: mirror: enabled: false, mirror: network: mirror: enabled: true}
			}
			exclude: [{bootMode: "isoboot", registry: "mirror", reason: "not yet"}]
		}
	}

	plans: {
		"pr-check": {testbed: "full", combinations: ["colocated-ipv4-netboot-direct"], suites: ["smoke"]}
		"my-branch": {testbed: "dev", suites: ["smoke", "nightly"], keep: "failed"}
	}
}
`

func TestLoadExample(t *testing.T) {
	dir := writeConfig(t, map[string]string{"acme.cue": example})
	cat := (&Loader{Dirs: []string{dir}, Builtins: testBuiltins()}).Load()
	if errs := cat.AllErrors(); len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	m := cat.Testbeds["full"]
	if m == nil || len(m.Combinations) != 16 {
		t.Fatalf("full combinations = %v", m)
	}
	var excluded int
	for _, c := range m.Combinations {
		if c.Err != "" {
			t.Errorf("%s: %s", c.Name, c.Err)
		}
		if c.Excluded != "" {
			excluded++
		}
	}
	if excluded != 4 {
		t.Errorf("excluded = %d, want 4", excluded)
	}
	ext := m.Combinations[8]
	if ext.Name != "external-ipv4-netboot-direct" {
		t.Fatalf("combination 8 = %s", ext.Name)
	}
	if ext.Testbed.Tinkerbell.Topology != "external" || ext.Testbed.Tinkerbell.Cluster == nil {
		t.Errorf("external combination tinkerbell = %+v", ext.Testbed.Tinkerbell)
	}
	var distros int
	for _, s := range ext.Testbed.Setup {
		if s.Uses == "testbed.distro" {
			distros++
		}
	}
	if distros != 2 {
		t.Errorf("external setup has %d testbed.distro steps, want 2", distros)
	}
	if v6 := m.Combinations[4].Testbed; v6.Network.Family != "ipv6" || !v6.Network.NAT64 {
		t.Errorf("ipv6 combination network = %+v", v6.Network)
	}

	dev := resolved(t, cat, "dev")
	if dev.Tinkerbell.Source == nil || dev.Tinkerbell.Source.Repo != "~/repos/tinkerbell/tinkerbell" {
		t.Errorf("dev tinkerbell = %+v", dev.Tinkerbell)
	}
	if dev.ClusterAPI.CAPT.Source == nil || !dev.UsesSource() {
		t.Errorf("dev capt = %+v", dev.ClusterAPI.CAPT)
	}
	if def := resolved(t, cat, "default"); def.Tinkerbell.Chart == nil || def.Tinkerbell.Chart.Version != "latest" {
		t.Errorf("default chart = %+v", def.Tinkerbell.Chart)
	}

	steps := cat.Tests["acme-smoke"].Value.Steps
	if steps[0].Kind() != "container" || steps[0].Container.Image != "ghcr.io/acme/e2e-smoke:v1.4.0" {
		t.Errorf("container step = %+v", steps[0])
	}
	cs := steps[1].Container
	if cs == nil || cs.Image != "ghcr.io/kyverno/chainsaw:v0.2.15" || !strings.Contains(strings.Join(cs.Args, " "), "--test-dir /wendy/work/chainsaw/policies") {
		t.Errorf("chainsaw preset = %+v", cs)
	}
	if g := steps[2].Exec; g == nil || g.Command[0] != "./e2e.test" || g.Command[2] != "--ginkgo.junit-report=${WENDY_ARTIFACTS_DIR}/junit.xml" {
		t.Errorf("ginkgo preset = %+v", g)
	}
	if steps[2].Timeout != "15m" {
		t.Errorf("default timeout = %q", steps[2].Timeout)
	}

	if o := cat.Plans["pr-check"].Origin; !strings.HasSuffix(o, "acme.cue") {
		t.Errorf("pr-check origin = %q", o)
	}
}

func TestLoadErrors(t *testing.T) {
	good := writeConfig(t, map[string]string{"a.cue": example})
	bad := writeConfig(t, map[string]string{"b.cue": `package b

import "github.com/tinkerbell/wendy/cue/wendy"

wendy.#Config & {
	tests: x: steps: [{uses: "assert.nodes", timeout: "15 min"}]
}
`})
	refs := writeConfig(t, map[string]string{"c.cue": `package c

plans: nightly: {testbed: "quickstart", suites: ["lifecylce"]}
`})
	cat := (&Loader{Dirs: []string{good, bad, refs}, Builtins: testBuiltins()}).Load()
	all := cat.AllErrors()
	var sawDuration, sawSuggest bool
	for _, e := range all {
		if strings.Contains(e.Path, "timeout") && strings.Contains(e.Msg, "not a duration") && strings.HasSuffix(e.File, "b.cue") && e.Line == 6 {
			sawDuration = true
		}
		if strings.Contains(e.Msg, `unknown suite "lifecylce" (did you mean "lifecycle"?)`) {
			sawSuggest = true
		}
	}
	if !sawDuration || !sawSuggest {
		t.Fatalf("errors = %v", all)
	}
	if _, ok := cat.Testbeds["full"]; !ok {
		t.Error("a bad directory took the good one down")
	}
}

func TestLoadKeepsLastGood(t *testing.T) {
	dir := writeConfig(t, map[string]string{"a.cue": example})
	l := &Loader{Dirs: []string{dir}, Builtins: testBuiltins()}
	if cat := l.Load(); len(cat.AllErrors()) > 0 {
		t.Fatal(cat.AllErrors())
	}
	if err := os.WriteFile(filepath.Join(dir, "a.cue"), []byte("package acme\nplans: {"), 0o644); err != nil {
		t.Fatal(err)
	}
	cat := l.Load()
	if len(cat.AllErrors()) == 0 || !cat.Stale {
		t.Fatalf("broken file: errors=%v stale=%v", cat.AllErrors(), cat.Stale)
	}
	if _, ok := cat.Plans["pr-check"]; !ok {
		t.Error("last good config was dropped")
	}
}

func testRuntime() map[string]any {
	return map[string]any{
		"vips":           map[string]any{"tinkerbell": "172.20.255.254", "hookos": "172.20.255.253", "controlPlane": "172.20.255.252"},
		"prefixLen":      16,
		"netmask":        "255.255.0.0",
		"podCIDR":        "10.244.0.0/16",
		"serviceCIDR":    "10.96.0.0/12",
		"nameServers":    []string{"8.8.8.8"},
		"trustedProxies": []string{"10.244.0.0/24"},
		"machines": []map[string]any{
			{"name": "c1-cp1", "role": "control-plane", "mac": "02:00:00:00:00:01", "ip": "172.20.255.200", "gateway": "172.20.0.1", "arch": "amd64", "bmcPort": 16231, "labels": map[string]string{}},
		},
		"bmc":                map[string]any{"host": "172.20.0.2", "user": "root", "pass": "calvin"},
		"sshKey":             "ssh-ed25519 AAAA test",
		"agentImage":         "ghcr.io/tinkerbell/tink-agent",
		"images":             map[string]any{"oci2disk": "o", "writefile": "w", "kexec": "k", "waitdaemon": "wd", "alpine": "a"},
		"osRegistry":         "ghcr.io/tinkerbell/cluster-api-provider-tinkerbell/ubuntu",
		"insecureRegistries": []string{},
		"capt":               map[string]any{"version": "v0.7.0", "repository": "https://example/releases"},
		"registryFiles":      map[string]map[string]string{},
	}
}

func TestRender(t *testing.T) {
	cat := (&Loader{Builtins: testBuiltins()}).Load()
	tb := resolved(t, cat, "quickstart")
	r, err := NewRenderer(testBuiltins())
	if err != nil {
		t.Fatal(err)
	}
	rt := testRuntime()

	var objs []map[string]any
	if err := r.Render(tb, rt, "workloadCluster", &objs, nil); err != nil {
		t.Fatal(err)
	}
	if len(objs) != 7 {
		t.Fatalf("the workload cluster has %d objects", len(objs))
	}
	tmt := objs[5]["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)
	wf := tmt["templateOverride"].(string)
	if !strings.Contains(wf, "IMG_URL: ghcr.io/tinkerbell/cluster-api-provider-tinkerbell/ubuntu:2404-v1.36.4-amd64.gz") {
		t.Errorf("workflow:\n%s", wf)
	}
	if strings.Contains(wf, "static netplan") {
		t.Error("ipv4 netboot should not write a static netplan")
	}
	if err := r.Render(tb, rt, "workloadCluster", &objs, map[string]any{"kubernetesVersion": "v1.37.0"}); err != nil {
		t.Fatal(err)
	}
	if name := objs[5]["metadata"].(map[string]any)["name"]; name != "wendy-control-plane-v1-37-0" {
		t.Errorf("upgraded template name = %v", name)
	}

	var values map[string]any
	if err := r.Render(tb, rt, "tinkerbellValues", &values, nil); err != nil {
		t.Fatal(err)
	}
	if values["publicIP"] != "172.20.255.254" || values["artifactsFileServer"] != "http://172.20.255.253:7173" {
		t.Errorf("values = %v", values)
	}

	var hw []map[string]any
	if err := r.Render(tb, rt, "hardware", &hw, nil); err != nil {
		t.Fatal(err)
	}
	if len(hw) != 1 {
		t.Fatalf("hardware = %v", hw)
	}

	tb.Network.Family = "ipv6"
	tb.Tinkerbell.BootMode = "isoboot"
	rt["subnet6"] = "fd00:cafe::/64"
	rt["vips"] = map[string]any{"tinkerbell": "fd00:cafe::fffe", "hookos": "fd00:cafe::fffd", "controlPlane": "fd00:cafe::fffc"}
	if err := r.Render(tb, rt, "tinkerbellValues", &values, nil); err != nil {
		t.Fatal(err)
	}
	smee := values["deployment"].(map[string]any)["envs"].(map[string]any)["smee"].(map[string]any)
	dhcpv6, _ := smee["dhcpv6Enabled"].(bool)
	staticIPAM, ok := smee["isoStaticIPAMEnabled"].(bool)
	if !dhcpv6 || !ok || staticIPAM || values["publicIPv6"] != "fd00:cafe::fffe" {
		t.Errorf("ipv6 isoboot values = %v", values)
	}
	if err := r.Render(tb, rt, "workloadCluster", &objs, nil); err != nil {
		t.Fatal(err)
	}
	opts := objs[5]["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["bootOptions"].(map[string]any)
	if opts["isoURL"] != "http://[fd00:cafe::fffe]:7080/iso6/hook.iso" {
		t.Errorf("bootOptions = %v", opts)
	}
}

// The mirrors serve TLS by name: CaptainOS gets the names and the CA as OSIE
// files, adds the names to /etc/hosts before the image is streamed by name,
// and oci2disk trusts the CA; workload nodes get both for containerd.
func TestRenderMirror(t *testing.T) {
	cat := (&Loader{Builtins: testBuiltins()}).Load()
	tb := resolved(t, cat, "quickstart")
	r, err := NewRenderer(testBuiltins())
	if err != nil {
		t.Fatal(err)
	}
	rt := testRuntime()
	rt["osRegistry"] = "wendy-mirror-ghcr-io:5000/tinkerbell/cluster-api-provider-tinkerbell/ubuntu"
	rt["registryFiles"] = map[string]map[string]string{"ghcr.io": {"hosts.toml": "h", "ca.crt": "CA"}}
	rt["mirror"] = map[string]any{"ca": "CA", "hosts": "172.20.0.2 wendy-mirror-ghcr-io\n"}

	var values map[string]any
	if err := r.Render(tb, rt, "tinkerbellValues", &values, nil); err != nil {
		t.Fatal(err)
	}
	if smee := values["deployment"].(map[string]any)["envs"].(map[string]any)["smee"].(map[string]any); smee["bootstrapEnabled"] != true {
		t.Errorf("smee = %v, want bootstrapEnabled so osieFiles are served", smee)
	}

	var hw []map[string]any
	if err := r.Render(tb, rt, "hardware", &hw, nil); err != nil {
		t.Fatal(err)
	}
	files, _ := hw[0]["spec"].(map[string]any)["metadata"].(map[string]any)["osieFiles"].([]any)
	got := map[string]any{}
	for _, f := range files {
		m := f.(map[string]any)
		got[m["path"].(string)] = m["contents"]
	}
	if got["wendy/hosts"] != "172.20.0.2 wendy-mirror-ghcr-io\n" || got["wendy/mirror-ca.crt"] != "CA" {
		t.Errorf("osieFiles = %v", files)
	}

	var objs []map[string]any
	if err := r.Render(tb, rt, "workloadCluster", &objs, nil); err != nil {
		t.Fatal(err)
	}
	wf := objs[5]["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["templateOverride"].(string)
	hosts, stream := strings.Index(wf, "name: add mirror hosts"), strings.Index(wf, "name: stream image")
	if hosts < 0 || stream < hosts || !strings.Contains(wf, "/osie_files/wendy/mirror-ca.crt:/etc/ssl/certs/wendy-mirror-ca.crt:ro") ||
		!strings.Contains(wf, "IMG_URL: wendy-mirror-ghcr-io:5000/") {
		t.Errorf("workflow:\n%s", wf)
	}
	spec := objs[2]["spec"].(map[string]any)["kubeadmConfigSpec"].(map[string]any)
	paths := map[string]bool{}
	for _, f := range spec["files"].([]any) {
		paths[f.(map[string]any)["path"].(string)] = true
	}
	cmds := spec["preKubeadmCommands"].([]any)
	if !paths["/etc/containerd/certs.d/ghcr.io/ca.crt"] || !paths["/etc/wendy/mirror-hosts"] ||
		cmds[0] != "cat /etc/wendy/mirror-hosts >> /etc/hosts" || cmds[1] != "systemctl restart containerd" {
		t.Errorf("control plane files = %v, preKubeadmCommands = %v", paths, cmds)
	}

	delete(rt, "mirror")
	if err := r.Render(tb, rt, "workloadCluster", &objs, nil); err != nil {
		t.Fatal(err)
	}
	if wf := objs[5]["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["templateOverride"].(string); strings.Contains(wf, "add mirror hosts") || strings.Contains(wf, "mirror-ca.crt") {
		t.Errorf("without the mirrors the workflow still adds their names or CA:\n%s", wf)
	}
}

func TestStarterLoads(t *testing.T) {
	dir := writeConfig(t, map[string]string{"wendy.cue": string(wendycue.Starter)})
	if errs := (&Loader{Dirs: []string{dir}, Builtins: testBuiltins()}).Load().AllErrors(); len(errs) > 0 {
		t.Fatal(errs)
	}
}

func TestRetiredKeysAreRejected(t *testing.T) {
	for _, key := range []string{"scenarios", "tinkerbells", "providers", "hardware", "workloads", "environments", "matrices", "profiles"} {
		dir := writeConfig(t, map[string]string{"a.cue": "package a\n\n" + key + ": x: {}\n"})
		errs := (&Loader{Dirs: []string{dir}, Builtins: testBuiltins()}).Load().AllErrors()
		if len(errs) != 1 || strings.TrimPrefix(errs[0].Path, "#Config.") != key || !strings.Contains(errs[0].Msg, "not allowed") || errs[0].Line != 3 {
			t.Errorf("%s: errors = %v, want one at line 3 saying it is not allowed", key, errs)
		}
	}
}

func TestRetiredKeepValueIsRejected(t *testing.T) {
	dir := writeConfig(t, map[string]string{"a.cue": "package a\n\nplans: p: {testbed: \"quickstart\", suites: [\"smoke\"], keep: \"onFailure\"}\n"})
	cat := (&Loader{Dirs: []string{dir}, Builtins: testBuiltins()}).Load()
	for _, e := range cat.AllErrors() {
		if strings.HasSuffix(e.Path, "plans.p.keep") && e.Line == 3 {
			return
		}
	}
	t.Fatalf("keep: onFailure was not rejected at the plan: %v", cat.AllErrors())
}

// The old name for a plan's diagnostics is not an alias: it gets the
// schema's own error at its position.
func TestOldDiagnosticsKeyIsRejected(t *testing.T) {
	dir := writeConfig(t, map[string]string{"a.cue": "package a\n\nplans: p: {testbed: \"quickstart\", suites: [\"smoke\"], evidence: dump: \"always\"}\n"})
	cat := (&Loader{Dirs: []string{dir}, Builtins: testBuiltins()}).Load()
	for _, e := range cat.AllErrors() {
		if strings.HasSuffix(e.Path, "plans.p.evidence") && strings.Contains(e.Msg, "not allowed") && e.Line == 3 {
			return
		}
	}
	t.Fatalf("evidence was not rejected at the plan: %v", cat.AllErrors())
}

// A built-in's name always means what wendy ships: writing one, even with
// fields that unify, is an error at its position, and the built-in stays.
func TestBuiltinsCannotBeRedefined(t *testing.T) {
	dir := writeConfig(t, map[string]string{"a.cue": `package a

parts: hardware: small: machines: worker: count: 3
tests: smoke: labels: ["mine"]
`})
	cat := (&Loader{Dirs: []string{dir}, Builtins: testBuiltins()}).Load()
	want := map[string]int{"parts.hardware.small": 3, "tests.smoke": 4}
	for _, e := range cat.AllErrors() {
		if line, ok := want[e.Path]; ok && e.Line == line && strings.Contains(e.Msg, "is built in; copy it under another name") {
			delete(want, e.Path)
		}
	}
	if len(want) > 0 {
		t.Errorf("not reported: %v; errors = %v", want, cat.AllErrors())
	}
	if hw := cat.Parts(KindHardware)["small"]; hw == nil || hw.Origin != BuiltIn {
		t.Errorf("hardware small = %+v, want the built-in", hw)
	}
	if tc := cat.Tests["smoke"]; tc == nil || len(tc.Value.Labels) > 0 {
		t.Errorf("test smoke = %+v, want the built-in", tc)
	}
}

// A step of yours stands in for every step that uses it, and what the test
// sets itself wins.
func TestYourStepsExpand(t *testing.T) {
	dir := writeConfig(t, map[string]string{"a.cue": `package a

steps: wait: {exec: command: ["kubectl", "wait"], timeout: "30m", retries: 2}
tests: one: steps: [{uses: "wait"}]
tests: two: steps: [{uses: "wait", name: "settle", timeout: "5m"}]
`})
	cat := (&Loader{Dirs: []string{dir}, Builtins: testBuiltins()}).Load()
	if errs := cat.AllErrors(); len(errs) > 0 {
		t.Fatal(errs)
	}
	one := cat.Expand(cat.Tests["one"].Value.Steps[0])
	if one.Name != "wait" || one.Timeout != "30m" || one.Retries != 2 || one.Exec == nil || one.Uses != "" {
		t.Errorf("one's step = %+v", one)
	}
	two := cat.Expand(cat.Tests["two"].Value.Steps[0])
	if two.Name != "settle" || two.Timeout != "5m" || two.Retries != 2 {
		t.Errorf("two's step = %+v", two)
	}
	if got := cat.Tests["one"].Value.Steps[0].Uses; got != "wait" {
		t.Errorf("the test is not kept as written: uses %q", got)
	}
}

func TestYourStepErrors(t *testing.T) {
	for _, c := range []struct{ name, src, path, msg string }{
		{"unknown", `tests: t: steps: [{uses: "wiat"}]`, "tests.t.steps.0.uses", `unknown step "wiat" (did you mean "wait"?)`},
		{"with", `tests: t: steps: [{uses: "wait", with: x: 1}]`, "with", "not allowed"},
		{"built-in name", `steps: "assert.nodes": exec: command: ["true"]`, "steps.assert.nodes", "is a built-in step"},
		{"uses yours", `steps: other: uses: "wait"`, "steps.other.uses", "can't use another of yours"},
	} {
		dir := writeConfig(t, map[string]string{"a.cue": "package a\n\nsteps: wait: exec: command: [\"true\"]\n" + c.src + "\n"})
		cat := (&Loader{Dirs: []string{dir}, Builtins: testBuiltins()}).Load()
		found := false
		for _, e := range cat.AllErrors() {
			if strings.HasSuffix(e.Path, c.path) && strings.Contains(e.Msg, c.msg) && e.Line == 4 {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no error at %s line 4 saying %q: %v", c.name, c.path, c.msg, cat.AllErrors())
		}
	}
}
