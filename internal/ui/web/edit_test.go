package web

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/service"
	_ "github.com/tinkerbell/wendy/internal/steps" // registers the built-in steps
)

func editServer(t *testing.T) (*Server, *service.Service) {
	t.Helper()
	dir := t.TempDir()
	svc := service.New(&config.Loader{Dirs: []string{dir}, Builtins: engine.Builtins()}, &engine.Engine{Data: t.TempDir()})
	svc.StarterDir = dir
	return New(svc, ""), svc
}

func post(t *testing.T, s *Server, path string, form url.Values, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestEditorCreatesTest(t *testing.T) {
	s, svc := editServer(t)

	rec := post(t, s, "/edit/tests", url.Values{
		"name": {"acme"}, "f.labels": {"app, smoke"},
		"step":    {"1", "2"},
		"s1.kind": {"container"}, "s1.container.image": {"ghcr.io/acme/e2e:v1"}, "s1.container.env": {"A=b"},
		"s2.kind": {"exec"}, "s2.exec.command": {"./run.sh\n--fast"},
	}, nil)
	if rec.Header().Get("HX-Redirect") == "" {
		t.Fatalf("test not saved: %s", rec.Body)
	}
	tc := svc.Catalog().Tests["acme"]
	if tc == nil || len(tc.Value.Steps) != 2 || tc.Value.Steps[1].Exec == nil || len(tc.Value.Steps[1].Exec.Command) != 2 {
		t.Fatalf("test did not load as submitted: %+v", tc)
	}

	for _, p := range []string{"/edit/tests/acme", "/edit/suites", "/edit/tests?from=smoke", "/edit-step?i=7"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d: %s", p, rec.Code, rec.Body)
		}
	}
}

// Only kinds that have a form are editable here. Matrices are not a kind of
// their own, and built-in steps are not written in CUE at all.
func TestOnlyKindsWithFormsAreEditable(t *testing.T) {
	s, _ := editServer(t)
	for _, p := range []string{"/edit/matrices", "/edit/steps/capt.scale", "/edit/plans/nope", "/edit/environments", "/edit/profiles", "/edit/clusterAPI"} {
		if code, _ := get(t, s, p); code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, code)
		}
	}
	if rec := post(t, s, "/edit/matrices", url.Values{"name": {"x"}}, nil); rec.Code != http.StatusNotFound {
		t.Errorf("POST /edit/matrices = %d, want 404", rec.Code)
	}
	for _, kind := range editable {
		if !slices.Contains(config.Kinds, kind) {
			t.Errorf("%s is editable here but not accepted by the service", kind)
		}
	}
}

func TestEditorShowsWhyASaveFailed(t *testing.T) {
	s, svc := editServer(t)
	rec := post(t, s, "/edit/tests", url.Values{
		"name": {"bad"}, "step": {"1"}, "s1.kind": {"uses"}, "s1.uses": {"assert.nodes"}, "s1.timeout": {"15 min"},
	}, nil)
	if rec.Header().Get("HX-Retarget") != "#form-error" || !strings.Contains(rec.Body.String(), "timeout") {
		t.Fatalf("response = %d %v %s", rec.Code, rec.Header(), rec.Body)
	}
	if _, err := os.Stat(svc.UIFilePath()); !os.IsNotExist(err) {
		t.Error("a rejected save wrote the file")
	}
}

func TestPartEditorsCreateAndEdit(t *testing.T) {
	s, svc := editServer(t)

	// Blank machine cells are unset, set ones are integers.
	rec := post(t, s, "/edit/hardware", url.Values{
		"name": {"ha"}, "f.arch": {"arm64"},
		"f.machines.worker.count": {"3"}, "f.machines.worker.cpus": {""}, "f.machines.controlPlane.count": {"3"},
		"f.bmc.protocol": {"ipmi"},
	}, nil)
	if rec.Header().Get("HX-Redirect") != "/testbeds/hardware/ha" {
		t.Fatalf("hardware not saved: %s", rec.Body)
	}
	hw := svc.Catalog().Parts("hardware")["ha"]
	if hw == nil {
		t.Fatal("hardware ha is not in the catalog")
	}
	if _, set := hw.Value["machines"].(map[string]any)["worker"].(map[string]any)["cpus"]; set {
		t.Error("a blank machine cell was saved")
	}
	if tb := svc.Catalog().Testbeds["quickstart"]; tb.Resolved.Hardware.Machines["worker"].Count != 1 {
		t.Error("saving a part changed another testbed")
	}

	if rec := post(t, s, "/edit/tinkerbell", url.Values{
		"name": {"stable"}, "f.chart.version": {"v0.19.0"}, "f.namespace": {"tb"}, "f.values": {"deployment:\n  replicas: 2"},
	}, nil); rec.Header().Get("HX-Redirect") == "" {
		t.Fatalf("tinkerbell not saved: %s", rec.Body)
	}
	if rec := post(t, s, "/edit/workload-cluster", url.Values{"name": {"next"}, "f.kubernetesVersion": {"v1.37.0"}, "f.cni": {"cilium"}}, nil); rec.Header().Get("HX-Redirect") == "" {
		t.Fatalf("workload cluster not saved: %s", rec.Body)
	}
	if rec := post(t, s, "/edit/cluster-api", url.Values{"name": {"edge"}, "f.core": {"v1.14.4"}, "f.capt.version": {"v0.7.0"}}, nil); rec.Header().Get("HX-Redirect") == "" {
		t.Fatalf("Cluster API not saved: %s", rec.Body)
	}
	if rec := post(t, s, "/edit/management-cluster", url.Values{"name": {"pinned"}, "f.image": {"kindest/node:v1.34.0"}}, nil); rec.Header().Get("HX-Redirect") != "/testbeds/management-cluster/pinned" {
		t.Fatalf("management cluster not saved: %s", rec.Body)
	}
	if got := svc.Catalog().Parts("managementCluster")["pinned"]; got == nil || got.Value["image"] != "kindest/node:v1.34.0" {
		t.Errorf("management cluster part = %+v", got)
	}

	// The Advanced fold opens when an advanced setting has a value, and not otherwise.
	open := func(path string) bool {
		code, body := get(t, s, path)
		if code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, code)
		}
		return strings.Contains(body, `<details class="adv" open>`)
	}
	if !open("/edit/tinkerbell/stable") {
		t.Error("the fold is closed over a set advanced setting")
	}
	if open("/edit/workload-cluster/next") {
		t.Error("the fold is open with no advanced setting set")
	}

	// Edit in place and rename.
	if rec := post(t, s, "/edit/workload-cluster", url.Values{"orig": {"next"}, "name": {"newer"}, "f.kubernetesVersion": {"v1.38.0"}, "f.cni": {"cilium"}}, nil); rec.Header().Get("HX-Redirect") != "/testbeds/workload-cluster/newer" {
		t.Fatalf("workload cluster not renamed: %s", rec.Body)
	}
	if _, ok := svc.Catalog().Parts("workloadCluster")["next"]; ok || svc.Catalog().Parts("workloadCluster")["newer"] == nil {
		t.Error("rename did not move the part")
	}
}

func TestPartEditorsRefuseWhatBreaksOthersAndWhatIsIncomplete(t *testing.T) {
	s, svc := editServer(t)
	if err := svc.SaveEntry("hardware", "", "ha", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveEntry("testbeds", "", "big", map[string]any{"hardware": "ha"}); err != nil {
		t.Fatal(err)
	}

	rec := post(t, s, "/edit/hardware/ha/delete", url.Values{}, nil)
	if rec.Header().Get("HX-Retarget") != "#form-error" || !strings.Contains(rec.Body.String(), "testbed big") {
		t.Errorf("deleting a used part: %d %v %s", rec.Code, rec.Header(), rec.Body)
	}
	if svc.Catalog().Parts("hardware")["ha"] == nil {
		t.Error("a used part was deleted")
	}

	rec = post(t, s, "/edit/cluster-api", url.Values{"name": {"bad"}, "f.capt.version": {"v0.7.0"}}, nil)
	if rec.Header().Get("HX-Retarget") != "#form-error" || !strings.Contains(rec.Body.String(), "Cluster API version") {
		t.Errorf("a Cluster API part with no core: %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(svc.UIFilePath()); err != nil {
		t.Errorf("the earlier entries are gone: %v", err)
	}
}

func TestPartEditorPagesAndButtons(t *testing.T) {
	s, _ := editServer(t)
	for _, p := range []string{
		"/edit/tinkerbell", "/edit/cluster-api", "/edit/management-cluster", "/edit/workload-cluster", "/edit/hardware",
		"/edit/hardware?from=small", "/edit/workload-cluster?from=k8s-1.36-kube-router",
	} {
		if code, _ := get(t, s, p); code != http.StatusOK {
			t.Errorf("GET %s = %d", p, code)
		}
	}
	_, hw := get(t, s, "/edit/hardware")
	for _, want := range []string{`aria-labelledby="machines-label"`, "<summary>Advanced settings</summary>", `name="f.machines.spare.count"`} {
		if !strings.Contains(hw, want) {
			t.Errorf("hardware form lacks %q", want)
		}
	}
	if _, tab := get(t, s, "/testbeds/hardware/small"); !strings.Contains(tab, "Copy and edit…") || !strings.Contains(tab, "＋ New hardware part") {
		t.Error("the hardware tab offers no way to create or copy")
	}
	if _, tab := get(t, s, "/testbeds/cluster-api/capi-1.14-capt-0.7"); !strings.Contains(tab, "＋ New Cluster API part") {
		t.Error(`the Cluster API tab does not offer "＋ New Cluster API part"`)
	}
	if rec := post(t, s, "/edit/hardware", url.Values{"name": {"x"}}, map[string]string{"Sec-Fetch-Site": "cross-site"}); rec.Code != http.StatusForbidden {
		t.Errorf("cross-site part save = %d", rec.Code)
	}
}

func TestTestbedEditorComposesParts(t *testing.T) {
	s, svc := editServer(t)
	if err := svc.SaveEntry("hardware", "", "ha", map[string]any{"machines": map[string]any{"worker": map[string]any{"count": int64(3)}}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveEntry("workloadCluster", "", "next", map[string]any{"kubernetesVersion": "v1.37.0"}); err != nil {
		t.Fatal(err)
	}

	rec := post(t, s, "/edit/testbeds", url.Values{
		"name": {"big"}, "f.description": {"three workers"},
		"p.tinkerbell": {""}, "p.clusterAPI": {""}, "p.managementCluster": {""}, "p.hardware": {"ha"}, "p.workloadCluster": {"next"},
	}, nil)
	if rec.Header().Get("HX-Redirect") != "/testbeds/testbeds/big" {
		t.Fatalf("testbed not saved: %s", rec.Body)
	}
	spec, err := svc.Spec()
	if err != nil {
		t.Fatal(err)
	}
	got := spec["testbeds"]["big"]
	if got["hardware"] != "ha" || got["workloadCluster"] != "next" || got["description"] != "three workers" {
		t.Errorf("saved spec = %v", got)
	}
	// Picking none is the absence of a choice, so the file stays sparse.
	if _, set := got["tinkerbell"]; set {
		t.Errorf("a fallback part was written: %v", got)
	}
	if tb := svc.Catalog().Testbeds["big"]; tb == nil || tb.Resolved == nil || tb.Resolved.Hardware.Machines["worker"].Count != 3 || tb.Resolved.WorkloadCluster.KubernetesVersion != "v1.37.0" {
		t.Errorf("testbed did not resolve from its parts: %+v", tb)
	}

	rec = post(t, s, "/edit/testbeds", url.Values{"name": {"bad"}, "p.hardware": {"hardwarr"}}, nil)
	if rec.Header().Get("HX-Retarget") != "#form-error" || !strings.Contains(html.UnescapeString(rec.Body.String()), `unknown hardware part "hardwarr"`) {
		t.Errorf("an unknown part: %d %s", rec.Code, rec.Body)
	}
}

// A matrix the editor can't show, such as one whose value patches a part's
// fields, is shown read-only, and saving the testbed or a copy keeps it.
func TestTestbedEditorKeepsAMatrixItCannotShow(t *testing.T) {
	s, svc := editServer(t)
	hand := "package local\n\ntestbeds: versions: matrix: {order: [\"size\"], axes: size: {one: {}, big: hardware: machines: worker: count: 5}}\n"
	if err := os.WriteFile(filepath.Join(svc.StarterDir, "hand.cue"), []byte(hand), 0o644); err != nil {
		t.Fatal(err)
	}
	svc.Reload()

	page := shownPage(t, s, "/edit/testbeds?from=versions")
	for _, want := range []string{`<td class="mono">size</td>`, "so it is written in CUE. Saving keeps it as it is."} {
		if !strings.Contains(page, want) {
			t.Errorf("the form does not show the testbed's matrix: it lacks %s", want)
		}
	}
	if strings.Contains(page, `name="matrixSet"`) {
		t.Error("the form offers to edit a matrix it can't show")
	}
	rec := post(t, s, "/edit/testbeds", url.Values{"name": {"mine"}, "from": {"versions"}, "f.description": {"copied"}}, nil)
	if rec.Header().Get("HX-Redirect") == "" {
		t.Fatalf("copy not saved: %s", rec.Body)
	}
	if rec := post(t, s, "/edit/testbeds", url.Values{"orig": {"mine"}, "name": {"mine"}, "f.description": {"edited"}}, nil); rec.Header().Get("HX-Redirect") == "" {
		t.Fatalf("edit not saved: %s", rec.Body)
	}
	tb := svc.Catalog().Testbeds["mine"]
	if tb == nil || !tb.HasMatrix() || len(tb.Combinations) != 2 || tb.Value.Description != "edited" {
		t.Errorf("testbed mine = %+v", tb)
	}
}

// The editor shows a matrix whose values each pick a part or set a setting,
// keeps the names its values are written as, and saves what it shows.
func TestMatrixEditor(t *testing.T) {
	s, svc := editServer(t)
	hand := "package local\n\ntestbeds: versions: matrix: {order: [\"boot\"], axes: boot: {net: bootMode: \"netboot\", iso: bootMode: \"isoboot\"}}\n"
	if err := os.WriteFile(filepath.Join(svc.StarterDir, "hand.cue"), []byte(hand), 0o644); err != nil {
		t.Fatal(err)
	}
	svc.Reload()
	page := shownPage(t, s, "/edit/testbeds?from=versions")
	for _, want := range []string{
		`name="ax0.name" value="boot"`, `<option value="bootMode" selected>How machines boot</option>`,
		`value="isoboot" checked>`, `name="ax0.as.isoboot" value="iso"`, `<td class="mono">net</td>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the editor lacks %s", want)
		}
	}
	if fresh := shownPage(t, s, "/edit/testbeds"); !strings.Contains(fresh, "None yet: this testbed is one combination.") {
		t.Error("a new testbed's editor does not say it has no matrix")
	}
	if axis := shownPage(t, s, "/edit/matrix-axis?i=7&ax7.varies=hardware"); !strings.Contains(axis, `name="ax7.v" value="small"`) {
		t.Errorf("a new hardware axis does not offer the hardware parts: %s", axis)
	}

	form := url.Values{
		"name": {"mine"}, "matrixSet": {"1"}, "ax": {"0", "1"},
		"ax0.name": {"k8s"}, "ax0.varies": {"workloadCluster"}, "ax0.v": {"k8s-1.36", "k8s-1.36-kube-router"},
		"ax1.name": {"boot"}, "ax1.varies": {"bootMode"}, "ax1.v": {"netboot", "isoboot"},
		"ex.k8s-1.36-kube-router-isoboot": {"on"}, "ex.k8s-1.36-kube-router-isoboot.reason": {"not yet"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, matrixCombinationsPath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.Handler().ServeHTTP(rec, req)
	if body := rec.Body.String(); !strings.Contains(body, "4 combinations") || !strings.Contains(body, `name="ex.k8s-1.36-kube-router-isoboot" checked`) {
		t.Errorf("the combinations fragment = %s", body)
	}
	if rec := post(t, s, "/edit/testbeds", form, nil); rec.Header().Get("HX-Redirect") == "" {
		t.Fatalf("not saved: %s", rec.Body)
	}
	tb := svc.Catalog().Testbeds["mine"]
	if tb == nil || len(tb.Combinations) != 4 {
		t.Fatalf("testbed mine = %+v", tb)
	}
	if reason, out := tb.Value.Matrix.Excluded(map[string]string{"k8s": "k8s-1.36-kube-router", "boot": "isoboot"}); !out || reason != "not yet" {
		t.Errorf("the combination left out = %v %q", out, reason)
	}
	if again := shownPage(t, s, "/edit/testbeds/mine"); !strings.Contains(again, `name="ax1.name" value="boot"`) {
		t.Error("a matrix the editor saved does not open in the editor")
	}
	if detail := shownPage(t, s, "/testbeds/testbeds/mine"); !strings.Contains(detail, `href="/edit/testbeds/mine#matrix">Edit matrix</a>`) || !strings.Contains(detail, "not yet") {
		t.Error("the testbed's page does not show what was left out, or offer to edit the matrix")
	}

	// Unchecking a value drops the combinations, and excludes, that name it.
	form.Set("orig", "mine")
	form["ax1.v"] = []string{"netboot"}
	if rec := post(t, s, "/edit/testbeds", form, nil); rec.Header().Get("HX-Redirect") == "" {
		t.Fatalf("not saved: %s", rec.Body)
	}
	if m := svc.Catalog().Testbeds["mine"].Value.Matrix; m == nil || len(m.Exclude) != 0 {
		t.Errorf("matrix = %+v, want no exclude", m)
	}
}

func TestTestbedEditorSavesSettings(t *testing.T) {
	s, svc := editServer(t)
	_, page := get(t, s, "/edit/testbeds")
	for _, name := range []string{"f.topology", "f.bootMode", "f.network.family", "f.network.nat64", "f.network.mirror.enabled", "f.network.mirror.upstreams"} {
		if !strings.Contains(page, `name="`+name+`"`) {
			t.Errorf("the testbed form has no %s", name)
		}
	}
	if strings.Contains(page, `<details class="adv" open>`) {
		t.Error("the fold is open with no advanced setting set")
	}

	rec := post(t, s, "/edit/testbeds", url.Values{
		"name": {"iso"}, "f.bootMode": {"isoboot"}, "f.topology": {"external"}, "f.network.family": {"ipv6"}, "f.network.mirror.enabled": {"true"},
	}, nil)
	if rec.Header().Get("HX-Redirect") != "/testbeds/testbeds/iso" {
		t.Fatalf("testbed not saved: %s", rec.Body)
	}
	tb := svc.Catalog().Testbeds["iso"].Resolved
	if tb.Tinkerbell.BootMode != "isoboot" || tb.Tinkerbell.Topology != "external" || tb.Network.Family != "ipv6" || !tb.Network.Mirror.Enabled {
		t.Errorf("settings did not resolve: %+v %+v", tb.Tinkerbell, tb.Network)
	}
	_, page = get(t, s, "/edit/testbeds/iso")
	if !strings.Contains(page, `<details class="adv" open>`) {
		t.Error("the fold is closed over a set advanced setting")
	}

	// A form that leaves a setting blank writes nothing for it.
	if rec := post(t, s, "/edit/testbeds", url.Values{"orig": {"iso"}, "name": {"iso"}, "f.bootMode": {""}, "f.topology": {""}, "f.network.family": {""}, "f.network.mirror.enabled": {""}}, nil); rec.Header().Get("HX-Redirect") == "" {
		t.Fatalf("not saved: %s", rec.Body)
	}
	spec, _ := svc.Spec()
	for _, k := range []string{"bootMode", "topology", "network"} {
		if _, set := spec["testbeds"]["iso"][k]; set {
			t.Errorf("a blank %s was written: %v", k, spec["testbeds"]["iso"])
		}
	}
}

func TestSelectsShowLabelsAndSubmitValues(t *testing.T) {
	s, _ := editServer(t)
	// The testbed's settings are radios saying what each choice does. The
	// default leads, marked, and posts "", so it is listed once.
	page := shownPage(t, s, "/edit/testbeds")
	for _, want := range []string{
		`<legend class="k">Where Tinkerbell runs</legend> <label class="opt"><input type="radio" name="f.topology" value="" checked> <span>On the management cluster <span class="mute">(default)</span> <span class="hint">Tinkerbell shares`,
		`<input type="radio" name="f.topology" value="external"> <span>On its own cluster <span class="hint">`,
		`<input type="radio" name="f.bootMode" value="isoboot"> <span>ISO boot <span class="hint">`,
		`<input type="radio" name="f.network.family" value="ipv6"> <span>IPv6 <span class="hint">`,
		// A dropdown names its default once, and a yes/no reads Yes or No.
		`<option value="">No (default)</option> <option value="true">Yes</option></select>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the testbed form lacks %s", want)
		}
	}
	if strings.Contains(page, `>colocated<`) || strings.Contains(page, `>isoboot<`) || strings.Contains(page, `value="colocated"`) || strings.Contains(page, "default: ") {
		t.Error("the testbed form shows a raw value as a label, lists a default twice, or reads default: …")
	}
	if p := shownPage(t, s, "/edit/hardware"); !strings.Contains(p, `<option value="">amd64 (x86-64) (default)</option> <option value="arm64" title="arm64">arm64</option></select>`) {
		t.Error("the hardware form's architecture does not list its default once")
	}

	rec := post(t, s, "/edit/testbeds", url.Values{"name": {"iso"}, "f.bootMode": {"isoboot"}}, nil)
	if rec.Header().Get("HX-Redirect") == "" {
		t.Fatalf("not saved: %s", rec.Body)
	}
	if page = shownPage(t, s, "/edit/testbeds/iso"); !strings.Contains(page, `<input type="radio" name="f.bootMode" value="isoboot" checked> <span>ISO boot`) {
		t.Error("the saved value is not checked under its label")
	}

	// A step's kind reads as its label, and the form says what the kind does.
	_, page = get(t, s, "/edit/tests?from=smoke")
	for _, want := range []string{`<option value="exec" title="exec">Command</option>`, `<div class="hint" data-only="exec">Runs a program on this host`} {
		if !strings.Contains(page, want) {
			t.Errorf("the step form lacks %s", want)
		}
	}
	// The run form's two choices about the testbed say when they apply, with
	// the default marked, and post their own values.
	page = shownPage(t, s, "/run")
	for _, want := range []string{
		`<span class="k" id="lbl-keep">When it finishes</span>`,
		`<input type="radio" name="keep" value="never" checked> <span>Tear down the testbed <span class="mute"> (default)</span>`,
		`<input type="radio" name="keep" value="failed"> <span>Keep the testbed if something fails <span class="hint">So you can look around.`,
		`<span class="k" id="lbl-pauseOnFailure">When a step fails</span>`,
		`<input type="radio" name="pauseOnFailure" value="false" checked> <span>Stop <span class="mute"> (default)</span>`,
		`<input type="radio" name="pauseOnFailure" value="true"> <span>Pause and wait for me <span class="hint">Everything stays up at the failed step.`,
		`name="diagnostics" value="onFailure"`, `>On failure</label>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the run form lacks %s", want)
		}
	}
	if strings.Contains(page, "Wait for me before tearing") {
		t.Error("the run form still has the old pause checkbox")
	}
}

func TestTestbedEditorKeepsAnInlinePart(t *testing.T) {
	s, svc := editServer(t)
	hand := "package local\n\ntestbeds: adhoc: hardware: machines: worker: count: 5\n"
	if err := os.WriteFile(filepath.Join(svc.StarterDir, "hand.cue"), []byte(hand), 0o644); err != nil {
		t.Fatal(err)
	}
	svc.Reload()

	// The page offers it as inline, and carries it for the save.
	code, page := get(t, s, "/edit/testbeds?from=adhoc")
	if code != http.StatusOK || !strings.Contains(page, `<option value="@inline" selected>`) {
		t.Fatalf("GET = %d; the inline part is not offered as the choice", code)
	}
	m := regexp.MustCompile(`name="inline\.hardware" value="([^"]*)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatal("the inline part is not in the form")
	}

	// What a browser posts back is the picker and the carried part.
	rec := post(t, s, "/edit/testbeds", url.Values{
		"name": {"adhoc-copy"}, "from": {"adhoc"},
		"p.hardware": {inlineChoice}, "inline.hardware": {html.UnescapeString(m[1])}, "p.workloadCluster": {""},
	}, nil)
	if rec.Header().Get("HX-Redirect") == "" {
		t.Fatalf("not saved: %s", rec.Body)
	}
	if tb := svc.Catalog().Testbeds["adhoc-copy"]; tb == nil || tb.Resolved == nil || tb.Resolved.Hardware.Machines["worker"].Count != 5 {
		t.Errorf("the inline part was lost: %+v", tb)
	}

	// Choosing a named part replaces it.
	if err := svc.SaveEntry("hardware", "", "ha", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if rec := post(t, s, "/edit/testbeds", url.Values{"orig": {"adhoc-copy"}, "name": {"adhoc-copy"}, "p.hardware": {"ha"}}, nil); rec.Header().Get("HX-Redirect") == "" {
		t.Fatalf("not saved: %s", rec.Body)
	}
	spec, _ := svc.Spec()
	if spec["testbeds"]["adhoc-copy"]["hardware"] != "ha" {
		t.Errorf("the inline part was not replaced: %v", spec["testbeds"]["adhoc-copy"])
	}
}

func TestTestbedFormAndSummaries(t *testing.T) {
	s, svc := editServer(t)
	if err := svc.SaveEntry("hardware", "", "ha", map[string]any{"machines": map[string]any{"controlPlane": map[string]any{"count": int64(3)}}}); err != nil {
		t.Fatal(err)
	}

	_, page := get(t, s, "/edit/testbeds")
	for f, label := range map[string]string{
		"tinkerbell": "Tinkerbell", "clusterAPI": "Cluster API", "managementCluster": "Management cluster",
		"workloadCluster": "Workload cluster", "hardware": "Hardware",
	} {
		if !strings.Contains(page, `<label class="k" for="p-`+f+`">`+label+`</label>`) || !strings.Contains(page, `id="p-`+f+`"`) {
			t.Errorf("no picker labelled %q for %s", label, f)
		}
	}
	// The fallback leads, as picking none, and each summary is already on
	// the page, under what the kind is.
	fallback := `<option value="" selected>small (default)</option>`
	if !strings.Contains(page, fallback) || !strings.Contains(page, "control plane 1 · worker 1 · spare 1") {
		t.Error("the form does not show the fallback hardware and its summary")
	}
	if strings.Index(page, fallback) > strings.Index(page, ">ha<") {
		t.Error("the fallback does not come first")
	}
	if !strings.Contains(page, `<span class="hint" id="about-hardware">The machines tests run on`) {
		t.Error("the hardware picker does not say what hardware is")
	}

	// A change answers with the summary, and the Open link for that part.
	for name, want := range map[string]string{"": "control plane 1 · worker 1 · spare 1", "ha": "control plane 3 · worker 1 · spare 1"} {
		code, frag := get(t, s, "/edit/part-summary?kind=hardware&p.hardware="+name)
		link := `<span id="open-hardware" hx-swap-oob="true"><a href="/testbeds/hardware/` + cmp(name, "small") + `" target="_blank"`
		if code != http.StatusOK || !strings.HasPrefix(frag, want+"<") || !strings.Contains(frag, link) {
			t.Errorf("summary of %q = %d %q", name, code, frag)
		}
	}
	if _, frag := get(t, s, "/edit/part-summary?kind=hardware&p.hardware=%3Cb%3E"); strings.Contains(frag, "<b>") {
		t.Errorf("a part name was not escaped: %q", frag)
	}
	if code, _ := get(t, s, "/edit/part-summary?kind=bogus"); code != http.StatusNotFound {
		t.Errorf("unknown kind = %d", code)
	}
}

// A part made in the testbed form's dialog is saved as the part form saves
// it. The reply is only that kind's picker, with the part picked, so the rest
// of the testbed form keeps what was typed.
func TestPartDialog(t *testing.T) {
	s, svc := editServer(t)
	page := shownPage(t, s, "/edit/testbeds")
	for _, want := range []string{
		`<form id="editform" hx-post="/edit/testbeds" hx-swap="none" data-unsaved>`,
		`<dialog id="partdialog" class="dialog" aria-labelledby="partdialog-title"></dialog>`,
		`hx-get="/edit/part-dialog?kind=workloadCluster" hx-target="#partdialog" hx-swap="innerHTML" aria-haspopup="dialog">New workload cluster part…</button>`,
		`<a href="/testbeds/workload-cluster/k8s-1.36" target="_blank" rel="noopener">Open`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the testbed form lacks %s", want)
		}
	}

	// The dialog is the part's form, with IDs of its own.
	code, frag := get(t, s, "/edit/part-dialog?kind=workloadCluster")
	frag = html.UnescapeString(frag)
	for _, want := range []string{`<form id="partform" hx-post="/edit/workload-cluster" hx-vals="{"then":"picker"}"`, `id="new-f-name"`, `id="new-f.kubernetesVersion"`, `<div id="dialog-error" role="alert">`} {
		if code != http.StatusOK || !strings.Contains(frag, want) {
			t.Errorf("the dialog = %d, lacks %s", code, want)
		}
	}
	if strings.Contains(frag, `id="f-name"`) {
		t.Error("the dialog's name field has the testbed form's ID")
	}
	if bad := unlabelled(frag); len(bad) > 0 {
		t.Errorf("the dialog has controls with no label: %v", bad)
	}
	for _, kind := range []string{"bogus", "testbeds"} {
		if code, _ := get(t, s, "/edit/part-dialog?kind="+kind); code != http.StatusNotFound {
			t.Errorf("a dialog for %s = %d", kind, code)
		}
	}

	rec := post(t, s, "/edit/workload-cluster", url.Values{"name": {"next"}, "f.kubernetesVersion": {"v1.37.0"}, "then": {"picker"}}, nil)
	h := rec.Header()
	if h.Get("HX-Retarget") != "#picker-workloadCluster" || h.Get("HX-Reswap") != "outerHTML" || h.Get("HX-Redirect") != "" ||
		h.Get("HX-Trigger-After-Settle") != `{"partPicked":{"target":"#p-workloadCluster"}}` {
		t.Fatalf("headers = %v; body %s", h, rec.Body)
	}
	if body := rec.Body.String(); !strings.HasPrefix(body, `<div class="field top" id="picker-workloadCluster">`) ||
		!strings.Contains(body, `<option value="next" selected>next</option>`) || !strings.Contains(body, "Kubernetes v1.37.0") {
		t.Errorf("the reply is not the picker with next picked: %s", body)
	}
	// A part is shared by the testbeds that pick it, so it is deleted from
	// its own page, never from a testbed's picker.
	if body := rec.Body.String(); strings.Contains(body, "/delete") {
		t.Errorf("the picker offers to delete its part: %s", body)
	}
	if svc.Catalog().Parts("workloadCluster")["next"] == nil {
		t.Error("the part was not saved")
	}

	// A rejected part says why in the dialog, and saves nothing.
	rec = post(t, s, "/edit/workload-cluster", url.Values{"name": {"bad name"}, "then": {"picker"}}, nil)
	if rec.Header().Get("HX-Retarget") != "#dialog-error" || !strings.Contains(rec.Body.String(), `class="fail"`) {
		t.Errorf("a rejected part = %v %s", rec.Header(), rec.Body)
	}
	if svc.Catalog().Parts("workloadCluster")["bad name"] != nil {
		t.Error("a rejected part was saved")
	}

	// Leaving the other edit forms asks too, and their links lead away in a
	// new tab.
	if p := shownPage(t, s, "/edit/tests"); !strings.Contains(p, `<form id="editform" hx-post="/edit/tests" hx-swap="none" data-unsaved>`) ||
		!strings.Contains(p, `<a href="/suites/steps" target="_blank" rel="noopener">Steps`) {
		t.Error("the test form is not marked, or its Steps link leaves it")
	}
	if p := shownPage(t, s, "/edit/plans"); !strings.Contains(p, `hx-swap="innerHTML" data-unsaved>`) {
		t.Error("the plan form is not marked")
	}
}

func TestEditorRefusesOtherSites(t *testing.T) {
	s, svc := editServer(t)
	form := url.Values{"name": {"x"}, "step": {"1"}, "s1.kind": {"exec"}, "s1.exec.command": {"true"}}
	for _, hdr := range []map[string]string{
		{"Sec-Fetch-Site": "cross-site"},
		{"Origin": "http://evil.example"},
	} {
		if rec := post(t, s, "/edit/tests", form, hdr); rec.Code != http.StatusForbidden {
			t.Errorf("%v: status %d", hdr, rec.Code)
		}
	}
	if _, ok := svc.Catalog().Tests["x"]; ok {
		t.Error("a cross-site request changed the config")
	}
}

// Each form says what it does to what, where it saves, and why it can't
// delete something still in use.
func TestEditFormTitlesAndSaveBar(t *testing.T) {
	s, svc := editServer(t)
	if err := svc.SaveEntry("hardware", "", "ha", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveEntry("testbeds", "", "big", map[string]any{"hardware": "ha"}); err != nil {
		t.Fatal(err)
	}

	used := shownPage(t, s, "/edit/hardware/ha")
	for _, want := range []string{
		`<h2 class="title">Edit hardware part ha</h2>`,
		`<button type="button" class="btn danger" disabled>Delete hardware part</button> <span class="disabled-why">Used by testbed big, so it can't be deleted.</span>`,
		`<a class="btn" href="/testbeds/hardware/ha">Cancel</a>`,
		`>Save hardware part</button>`,
		"wendy-ui.cue</span>. Wendy checks it against the schema first.",
	} {
		if !strings.Contains(used, want) {
			t.Errorf("the form for a used part lacks %s", want)
		}
	}
	unused := shownPage(t, s, "/edit/testbeds/big")
	if !strings.Contains(unused, `hx-confirm="Delete testbed big? It is removed from `) || strings.Contains(unused, "disabled-why") {
		t.Error("an unused testbed cannot be deleted, or its confirmation does not say what happens")
	}

	copied := shownPage(t, s, "/edit/hardware?from=small")
	for _, want := range []string{`<h2 class="title">Copy of hardware part small</h2>`, `<div class="note">small is built in. Your copy is saved under a new name in `} {
		if !strings.Contains(copied, want) {
			t.Errorf("the copy of a built-in lacks %s", want)
		}
	}
	fresh := shownPage(t, s, "/edit/hardware")
	if !strings.Contains(fresh, `<h2 class="title">New hardware part</h2>`) || strings.Contains(fresh, "Delete hardware part") {
		t.Error("a new part's form is not titled New, or offers Delete")
	}

	// Steps are headed by their number, which CSS counts.
	if steps := shownPage(t, s, "/edit/tests?from=smoke"); !strings.Contains(steps, `<div class="stephead"><b class="stepnum"></b>`) {
		t.Error("a step is not headed by its number")
	}
}

// A step of the user's is made, used and listed like any entry. Its form is
// one step, which can use only a built-in step; a test's steps can use both.
func TestYourStepsInTheWebUI(t *testing.T) {
	s, svc := editServer(t)
	rec := post(t, s, "/edit/steps", url.Values{
		"name": {"wait"}, "step": {"0"}, "s0.kind": {"exec"}, "s0.exec.command": {"kubectl\nwait"}, "s0.timeout": {"30m"},
	}, nil)
	if rec.Header().Get("HX-Redirect") != "/suites/steps/wait" {
		t.Fatalf("step not saved: %s", rec.Body)
	}
	if it := svc.Catalog().Steps["wait"]; it == nil || it.Value.Timeout != "30m" || it.Value.Exec == nil {
		t.Fatalf("step = %+v", it)
	}
	if err := svc.SaveEntry(config.KindTest, "", "t", map[string]any{"steps": []any{map[string]any{"uses": "wait"}}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveEntry(config.KindStep, "", "add-worker", map[string]any{"uses": "capt.scale", "with": map[string]any{"target": "worker", "by": int64(1)}}); err != nil {
		t.Fatal(err)
	}
	if builtin := shownPage(t, s, "/suites/steps/capt.scale"); !strings.Contains(builtin, `step <a href="/suites/steps/add-worker">add-worker</a>`) {
		t.Error("a built-in step's Used by leaves out the steps of yours that use it")
	}

	list := shownPage(t, s, "/suites/steps")
	if !strings.Contains(list, ">wait</a>") || !strings.Contains(list, ">capt.scale</a>") {
		t.Error("the Steps subtab does not list built-in steps and yours")
	}
	detail := shownPage(t, s, "/suites/steps/wait")
	for _, want := range []string{
		"<th>What it runs</th>", "kubectl wait", "<th>Times out after</th><td>30m</td>",
		`test <a href="/suites/tests/t">t</a>`, `href="/edit/steps/wait">Edit step</a>`,
	} {
		if !strings.Contains(detail, want) {
			t.Errorf("step wait's page lacks %s", want)
		}
	}

	form := shownPage(t, s, "/edit/steps/wait")
	if strings.Contains(form, "data-remove") || strings.Contains(form, "stepnum") {
		t.Error("a step's own form numbers, moves or removes its step")
	}
	if strings.Contains(form, `<option value="wait"`) || !strings.Contains(form, `<option value="capt.scale"`) {
		t.Error("a step's own form offers anything but built-in steps")
	}
	if !strings.Contains(form, "Used by test t, so it can't be deleted.") {
		t.Error("a used step can be deleted")
	}
	if test := shownPage(t, s, "/edit/tests/t"); !strings.Contains(test, `<option value="wait" title="wait" selected>`) || !strings.Contains(test, `value="capt.scale"`) {
		t.Error("a test's Named step does not offer built-in steps and yours")
	}
}

// A plan's form is the run form's cards with a name. What it saves is
// sparse, and it edits, copies and deletes like the other forms.
func TestPlanForms(t *testing.T) {
	s, svc := editServer(t)
	if plans := shownPage(t, s, "/plans"); !strings.Contains(plans, `href="/edit/plans">＋ New plan</a>`) {
		t.Error("the Plans tab offers no ＋ New plan")
	}
	fresh := shownPage(t, s, "/edit/plans")
	for _, want := range []string{`<h2 class="title">New plan</h2>`, `name="name"`, "<h3>Testbed</h3>", "<h3>Tests</h3>", "<h3>Options</h3>", ">Save plan</button>"} {
		if !strings.Contains(fresh, want) {
			t.Errorf("the new plan form lacks %s", want)
		}
	}

	// The Options card's radios post every choice, defaults included; a
	// default is not written.
	form := url.Values{
		"name": {"weekly"}, "f.description": {"every week"}, "plan": {""},
		"testbed": {"quickstart"}, "suites": {"smoke"}, "suitesSet": {"1"},
		"keep": {"failed"}, "pauseSet": {"1"}, "pauseOnFailure": {"false"}, "diagnosticsSet": {"1"}, "diagnostics": {"onFailure"}, "consoles": {"on"},
	}
	if rec := post(t, s, "/edit/plans", form, nil); rec.Header().Get("HX-Redirect") != "/plans/weekly" {
		t.Fatalf("plan not saved: %s", rec.Body)
	}
	spec, err := svc.Spec()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"testbed": "quickstart", "suites": []any{"smoke"}, "keep": "failed", "description": "every week"}
	if got := spec["plans"]["weekly"]; len(got) != len(want) || got["keep"] != "failed" || got["description"] != "every week" {
		t.Errorf("saved plan = %v, want only %v", got, want)
	}

	edit := shownPage(t, s, "/edit/plans/weekly")
	for _, want := range []string{`<h2 class="title">Edit plan weekly</h2>`, `value="every week"`, ">Delete plan</button>", `<input type="radio" name="keep" value="failed" checked>`, `<input type="radio" name="pauseOnFailure" value="false" checked>`} {
		if !strings.Contains(edit, want) {
			t.Errorf("the plan's form lacks %s", want)
		}
	}
	if copied := shownPage(t, s, "/edit/plans?from=quickstart"); !strings.Contains(copied, "quickstart is built in. Your copy is saved under a new name in ") {
		t.Error("a copy of a built-in plan does not say where it is saved")
	}

	form.Set("orig", "weekly")
	form.Del("suites")
	if rec := post(t, s, "/edit/plans", form, nil); !strings.Contains(rec.Body.String(), "pick at least one suite") {
		t.Errorf("a plan with no suite: %s", rec.Body)
	}
	if rec := post(t, s, "/edit/plans/weekly/delete", nil, nil); rec.Header().Get("HX-Redirect") != "/plans" {
		t.Errorf("delete: %d %s", rec.Code, rec.Body)
	}
}
