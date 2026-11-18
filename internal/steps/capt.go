package steps

import (
	"context"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	clusterctl "sigs.k8s.io/cluster-api/cmd/clusterctl/client"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/provider/distro"
)

// Labels CAPT puts on Hardware it has claimed for a machine.
const (
	ownerNameLabel = "v1alpha1.tinkerbell.org/ownerName"
	roleLabelKey   = "tinkerbell.org/role"
)

type scaleParam struct {
	Target   string `json:"target"`
	By       int32  `json:"by"`
	Replicas *int32 `json:"replicas,omitempty"`
}

type versionParam struct {
	Version string `json:"version,omitempty"`
}

func init() {
	engine.Register("capt.cluster.create", "Renders and applies the workload cluster, and fetches its kubeconfig.", "{}", clusterCreate)
	engine.Register("capt.cluster.delete", "Deletes the workload cluster and checks every machine is deprovisioned.", "{}", clusterDelete)
	engine.Register("capt.scale", "Scales the control plane or the MachineDeployment, claiming spares to grow and returning freed hardware to the spares.",
		`{target: *"worker" | "controlPlane", by: *1 | int, replicas?: int & >=0}`, scale)
	engine.Register("capt.upgrade.kubernetes", "Rolls the workload cluster to the Kubernetes version in `version`.",
		`{version?: =~"^v[0-9]+\\.[0-9]+\\.[0-9]+$"}`, upgradeKubernetes)
	engine.Register("capt.move", "Pivots: moves the CAPI objects into the workload cluster and makes it the management cluster.", "{}", move)
	engine.Register("capt.remediate", "Powers a worker off through its BMC and waits for a MachineHealthCheck to replace it.", "{}", remediate)
}

// names are the workload cluster's objects' names, as rendered.
type names struct {
	Cluster           string `json:"cluster"`
	ControlPlane      string `json:"controlPlane"`
	MachineDeployment string `json:"machineDeployment"`
	TemplateCP        string `json:"templateCP"`
	TemplateWorker    string `json:"templateWorker"`
}

func renderWorkloadCluster(ctx context.Context, s *engine.Step, version string) ([]map[string]any, names, error) {
	rt, err := runtimeOf(ctx, s)
	if err != nil {
		return nil, names{}, err
	}
	var fill map[string]any
	if version != "" {
		fill = map[string]any{"kubernetesVersion": version}
	}
	var objs []map[string]any
	var n names
	if err := s.Testbed.Render(rt, "workloadCluster", &objs, fill); err != nil {
		return nil, n, err
	}
	err = s.Testbed.Render(rt, "names", &n, fill)
	return objs, n, err
}

func clusterCreate(ctx context.Context, s *engine.Step, _ struct{}) error {
	objs, n, err := renderWorkloadCluster(ctx, s, s.Testbed.State().WorkloadCluster.KubernetesVersion)
	if err != nil {
		return err
	}
	c, err := kube(s, engine.ManagementCluster)
	if err != nil {
		return err
	}
	ns := s.Testbed.Config.Tinkerbell.Namespace
	if err := ensureNamespace(ctx, c, ns); err != nil {
		return err
	}
	if err := apply(ctx, c, objs...); err != nil {
		return engine.Fail(err)
	}
	s.Log.Info(fmt.Sprintf("applied cluster %s/%s (%d objects); waiting for its kubeconfig", ns, n.Cluster, len(objs)))
	eventuallyErr(ctx, 5*time.Second, func() error { return workloadClusterKubeconfig(ctx, s, c) })
	s.Outputs["cluster"] = n.Cluster
	return nil
}

func clusterDelete(ctx context.Context, s *engine.Step, _ struct{}) error {
	c, err := kube(s, engine.ManagementCluster)
	if err != nil {
		return err
	}
	tink, err := kube(s, engine.TinkerbellCluster)
	if err != nil {
		return err
	}
	ns := s.Testbed.Config.Tinkerbell.Namespace
	cl := &clusterv1.Cluster{}
	key := client.ObjectKey{Namespace: ns, Name: s.Testbed.Config.WorkloadCluster.Name}
	if err := c.Get(ctx, key, cl); apierrors.IsNotFound(err) {
		return engine.Skip("no workload cluster")
	} else if err != nil {
		return err
	}
	if err := c.Delete(ctx, cl); err != nil {
		return err
	}
	s.Log.Info("deleted cluster " + key.String() + "; waiting for it and its machines to go")
	eventually(ctx, 10*time.Second, func(g gomega.Gomega) {
		err := c.Get(ctx, key, &clusterv1.Cluster{})
		g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue(), "cluster %s still exists", key)
		machines, err := list(ctx, c, clusterGroup, "Machine", ns)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(machines).To(gomega.BeEmpty(), "%d Machines left", len(machines))
		hw, err := list(ctx, tink, tinkerbellGroup, "Hardware", ns)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		var owned []string
		for _, h := range hw {
			if h.GetLabels()[ownerNameLabel] != "" {
				owned = append(owned, h.GetName())
			}
		}
		g.Expect(owned).To(gomega.BeEmpty(), "Hardware still claimed: %v", owned)
	})
	_ = os.Remove(s.Testbed.Kubeconfig(engine.WorkloadCluster))
	return nil
}

// hardware lists the testbed's Hardware.
func hardwareList(ctx context.Context, tink client.Client, ns string) ([]unstructured.Unstructured, error) {
	return list(ctx, tink, tinkerbellGroup, "Hardware", ns)
}

func relabel(ctx context.Context, tink client.Client, h unstructured.Unstructured, role string) error {
	patch := []byte(fmt.Sprintf(`{"metadata":{"labels":{%q:%q}}}`, roleLabelKey, role))
	return tink.Patch(ctx, &h, client.RawPatch("application/merge-patch+json", patch))
}

// claimSpares relabels n unclaimed spares so CAPT can give them role.
func claimSpares(ctx context.Context, s *engine.Step, n int, role string) ([]string, error) {
	tink, err := kube(s, engine.TinkerbellCluster)
	if err != nil {
		return nil, err
	}
	hw, err := hardwareList(ctx, tink, s.Testbed.Config.Tinkerbell.Namespace)
	if err != nil {
		return nil, err
	}
	var claimed []string
	for _, h := range hw {
		if len(claimed) == n {
			break
		}
		if h.GetLabels()[roleLabelKey] == roleLabel(roleSpare) && h.GetLabels()[ownerNameLabel] == "" {
			if err := relabel(ctx, tink, h, role); err != nil {
				return nil, err
			}
			claimed = append(claimed, h.GetName())
			s.Log.Info(fmt.Sprintf("hardware %s claimed as %s (was spare)", h.GetName(), role))
		}
	}
	if len(claimed) < n {
		return claimed, fmt.Errorf("needed %d spare machines, found %d free", n, len(claimed))
	}
	return claimed, nil
}

// releaseFreed waits for n machines of role, other than keep, to be
// released by CAPT, and relabels them as to.
func releaseFreed(ctx context.Context, s *engine.Step, n int, role, to string, keep []string) error {
	tink, err := kube(s, engine.TinkerbellCluster)
	if err != nil {
		return err
	}
	ns := s.Testbed.Config.Tinkerbell.Namespace
	var freed []unstructured.Unstructured
	eventually(ctx, 10*time.Second, func(g gomega.Gomega) {
		hw, err := hardwareList(ctx, tink, ns)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		freed = freed[:0]
		for _, h := range hw {
			if h.GetLabels()[roleLabelKey] == role && h.GetLabels()[ownerNameLabel] == "" && !slices.Contains(keep, h.GetName()) {
				freed = append(freed, h)
			}
		}
		g.Expect(len(freed)).To(gomega.BeNumerically(">=", n), "waiting for %d %s machine(s) to be released", n, role)
	})
	for _, h := range freed[:n] {
		if err := relabel(ctx, tink, h, to); err != nil {
			return err
		}
		s.Log.Info(fmt.Sprintf("hardware %s released; now %s", h.GetName(), to))
	}
	return nil
}

func getCluster(ctx context.Context, c client.Client, s *engine.Step) (*clusterv1.Cluster, error) {
	cl := &clusterv1.Cluster{}
	return cl, c.Get(ctx, client.ObjectKey{Namespace: s.Testbed.Config.Tinkerbell.Namespace, Name: s.Testbed.Config.WorkloadCluster.Name}, cl)
}

func remaining(ctx context.Context) []interface{} {
	d := 30 * time.Minute
	if dl, ok := ctx.Deadline(); ok {
		d = time.Until(dl)
	}
	return []interface{}{d, 10 * time.Second}
}

func scale(ctx context.Context, s *engine.Step, p scaleParam) error {
	px := proxy(s)
	c := px.GetClient()
	cl, err := getCluster(ctx, c, s)
	if err != nil {
		return err
	}
	ns := cl.Namespace
	_, n, err := renderWorkloadCluster(ctx, s, "")
	if err != nil {
		return err
	}
	if p.Target == roleControlPlane {
		kcp := &controlplanev1.KubeadmControlPlane{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: n.ControlPlane}, kcp); err != nil {
			return err
		}
		cur := ptrVal(kcp.Spec.Replicas)
		want := target(cur, p)
		summarize(s, fmt.Sprintf("KubeadmControlPlane %s replicas %d → %d", kcp.Name, cur, want))
		if want > cur {
			if _, err := claimSpares(ctx, s, int(want-cur), "control-plane"); err != nil {
				return err
			}
		}
		withFramework(s, func() {
			framework.ScaleAndWaitControlPlane(ctx, framework.ScaleAndWaitControlPlaneInput{
				ClusterProxy: px, Cluster: cl, ControlPlane: kcp, Replicas: want, WaitForControlPlane: remaining(ctx),
			})
		})
		if want < cur {
			return releaseFreed(ctx, s, int(cur-want), "control-plane", "spare", nil)
		}
		return nil
	}
	md := &clusterv1.MachineDeployment{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: n.MachineDeployment}, md); err != nil {
		return err
	}
	cur := ptrVal(md.Spec.Replicas)
	want := target(cur, p)
	summarize(s, fmt.Sprintf("MachineDeployment %s replicas %d → %d", md.Name, cur, want))
	if want > cur {
		if _, err := claimSpares(ctx, s, int(want-cur), "worker"); err != nil {
			return err
		}
	}
	withFramework(s, func() {
		framework.ScaleAndWaitMachineDeployment(ctx, framework.ScaleAndWaitMachineDeploymentInput{
			ClusterProxy: px, Cluster: cl, MachineDeployment: md, Replicas: want, WaitForMachineDeployments: remaining(ctx),
		})
	})
	if want < cur {
		return releaseFreed(ctx, s, int(cur-want), "worker", "spare", nil)
	}
	return nil
}

func ptrVal(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

func target(cur int32, p scaleParam) int32 {
	if p.Replicas != nil {
		return *p.Replicas
	}
	return max(cur+p.By, 0)
}

// summarize logs msg and records it as the step's one-line summary.
func summarize(s *engine.Step, msg string) {
	s.Outputs["summary"] = msg
	s.Log.Info(msg)
}

// upgradeKubernetes rolls the control plane, then the workers. Each rollout
// surges by one machine, so a spare is claimed for the control plane, the
// control plane's old machine serves the workers' surge, and the workers'
// old machine becomes the spare: nothing is left over.
func upgradeKubernetes(ctx context.Context, s *engine.Step, p versionParam) error {
	version := p.Version
	if version == "" {
		return engine.Skip("no target: set the step's `with: {version: ...}`")
	}
	from := s.Testbed.State().WorkloadCluster.KubernetesVersion
	if version == from {
		return engine.Skip("already at " + version)
	}
	summarize(s, fmt.Sprintf("Kubernetes %s → %s", from, version))

	objs, n, err := renderWorkloadCluster(ctx, s, version)
	if err != nil {
		return err
	}
	px := proxy(s)
	c := px.GetClient()
	cl, err := getCluster(ctx, c, s)
	if err != nil {
		return err
	}
	ns := cl.Namespace
	var templates []map[string]any
	for _, o := range objs {
		if o["kind"] == "TinkerbellMachineTemplate" {
			templates = append(templates, o)
		}
	}
	if err := apply(ctx, c, templates...); err != nil {
		return err
	}

	// Control plane.
	claimed, err := claimSpares(ctx, s, 1, "control-plane")
	if err != nil {
		return err
	}
	kcp := &controlplanev1.KubeadmControlPlane{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: n.ControlPlane}, kcp); err != nil {
		return err
	}
	cpReplicas := ptrVal(kcp.Spec.Replicas)
	patch := client.MergeFrom(kcp.DeepCopy())
	kcp.Spec.Version = version
	kcp.Spec.MachineTemplate.Spec.InfrastructureRef.Name = n.TemplateCP
	if err := c.Patch(ctx, kcp, patch); err != nil {
		return engine.Fail(err)
	}
	s.Log.Info("rolling the control plane")
	eventually(ctx, 15*time.Second, func(g gomega.Gomega) {
		ms := &clusterv1.MachineList{}
		g.Expect(c.List(ctx, ms, client.InNamespace(ns), client.HasLabels{clusterv1.MachineControlPlaneLabel})).To(gomega.Succeed())
		var upgraded int32
		for _, m := range ms.Items {
			if m.DeletionTimestamp != nil {
				continue
			}
			g.Expect(m.Spec.Version).To(gomega.Equal(version), "machine %s is at %s", m.Name, m.Spec.Version)
			g.Expect(m.Status.NodeRef.IsDefined()).To(gomega.BeTrue(), "machine %s has no node yet", m.Name)
			upgraded++
		}
		g.Expect(upgraded).To(gomega.Equal(cpReplicas))
		g.Expect(len(ms.Items)).To(gomega.Equal(int(cpReplicas)), "old control-plane machines still being removed")
	})
	if err := releaseFreed(ctx, s, 1, "control-plane", "worker", claimed); err != nil {
		return err
	}

	// Workers.
	md := &clusterv1.MachineDeployment{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: n.MachineDeployment}, md); err != nil {
		return err
	}
	patch = client.MergeFrom(md.DeepCopy())
	md.Spec.Template.Spec.Version = version
	md.Spec.Template.Spec.InfrastructureRef.Name = n.TemplateWorker
	if err := c.Patch(ctx, md, patch); err != nil {
		return engine.Fail(err)
	}
	s.Log.Info("rolling the workers")
	withFramework(s, func() {
		framework.WaitForMachineDeploymentMachinesToBeUpgraded(ctx, framework.WaitForMachineDeploymentMachinesToBeUpgradedInput{
			Lister: c, Cluster: cl, KubernetesUpgradeVersion: version, MachineCount: int(ptrVal(md.Spec.Replicas)), MachineDeployment: *md,
		}, remaining(ctx)...)
	})
	if ptrVal(md.Spec.Replicas) > 0 {
		if err := releaseFreed(ctx, s, 1, "worker", "spare", nil); err != nil {
			return err
		}
	}
	return s.Testbed.Update(func(st *engine.State) { st.WorkloadCluster.KubernetesVersion = version })
}

// move pivots the cluster: CAPI and, when colocated, Tinkerbell are
// installed into the workload cluster, the objects are moved there, and the
// kind cluster that bootstrapped them is deleted.
func move(ctx context.Context, s *engine.Step, _ struct{}) error {
	tb := s.Testbed
	colocated := tb.Config.Tinkerbell.Topology != "external"
	bootstrap := tb.Kubeconfig(engine.ManagementCluster)
	workloadCluster := tb.Kubeconfig(engine.WorkloadCluster)
	b, err := os.ReadFile(bootstrap)
	if err != nil {
		return err
	}
	if err := os.WriteFile(bootstrap+".bootstrap", b, 0o600); err != nil { //nolint:gosec // G703: the path is the combination's own kubeconfig.
		return err
	}

	if colocated {
		s.Log.Info("installing Tinkerbell into the workload cluster")
		if err := installTinkerbellInto(ctx, s, workloadCluster); err != nil {
			return err
		}
	}
	path, err := clusterctlConfig(ctx, s, tb.Config.ClusterAPI.CAPT, "clusterctl.yaml")
	if err != nil {
		return err
	}
	cc, err := clusterctl.New(ctx, path)
	if err != nil {
		return err
	}
	core := tb.State().Versions["cluster-api"]
	capt := tb.State().Versions["capt"]
	withFramework(s, func() {
		_, err = cc.Init(ctx, clusterctl.InitOptions{
			Kubeconfig: clusterctl.Kubeconfig{Path: workloadCluster}, CoreProvider: "cluster-api:" + core,
			BootstrapProviders: []string{"kubeadm:" + core}, ControlPlaneProviders: []string{"kubeadm:" + core},
			InfrastructureProviders: []string{"tinkerbell:" + capt}, WaitProviders: true, WaitProviderTimeout: 10 * time.Minute,
		})
	})
	if err != nil {
		return fmt.Errorf("clusterctl init on the workload cluster: %w", err)
	}
	s.Log.Info("moving CAPI objects")
	withFramework(s, func() {
		err = cc.Move(ctx, clusterctl.MoveOptions{
			FromKubeconfig: clusterctl.Kubeconfig{Path: bootstrap},
			ToKubeconfig:   clusterctl.Kubeconfig{Path: workloadCluster},
			Namespace:      tb.Config.Tinkerbell.Namespace,
		})
	})
	if err != nil {
		return engine.Fail(fmt.Errorf("clusterctl move: %w", err))
	}
	wb, err := os.ReadFile(workloadCluster)
	if err != nil {
		return err
	}
	if err := tb.SetKubeconfig(engine.ManagementCluster, wb); err != nil {
		return err
	}
	if colocated {
		// Its Tinkerbell would answer on the same VIP as the new one.
		s.Log.Info("deleting the bootstrap kind cluster")
		if err := (distro.Kind{}).Down(ctx, tb.HostName("")); err != nil {
			return err
		}
	}
	return nil
}

func installTinkerbellInto(ctx context.Context, s *engine.Step, kubeconfig string) error {
	cfg := s.Testbed.Config.Tinkerbell
	rt, err := runtimeOf(ctx, s)
	if err != nil {
		return err
	}
	var values map[string]any
	if err := s.Testbed.Render(rt, "tinkerbellValues", &values, nil); err != nil {
		return err
	}
	rel := chartRelease{Kubeconfig: kubeconfig, Namespace: cfg.Namespace, Name: "tinkerbell", Values: merge(values, cfg.Values), Timeout: 15 * time.Minute}
	if cfg.Source != nil {
		out, err := s.Testbed.Builds.Get("tinkerbell", *cfg.Source)
		if err != nil {
			return err
		}
		rel.Chart, _ = out["chart"].(string)
	} else {
		rel.Chart = cfg.Chart.Location
		rel.Version = s.Testbed.State().Versions["tinkerbell"]
	}
	_, err = helmUpgrade(ctx, s.Log, rel)
	return err
}

// remediate pulls a worker's plug through its BMC and waits for CAPI to
// replace it.
func remediate(ctx context.Context, s *engine.Step, _ struct{}) error {
	px := proxy(s)
	c := px.GetClient()
	cl, err := getCluster(ctx, c, s)
	if err != nil {
		return err
	}
	ns := cl.Namespace
	tink, err := kube(s, engine.TinkerbellCluster)
	if err != nil {
		return err
	}
	rt, err := runtimeOf(ctx, s)
	if err != nil {
		return err
	}
	var mhc map[string]any
	if err := s.Testbed.Render(rt, "machineHealthCheck", &mhc, nil); err != nil {
		return err
	}
	if err := apply(ctx, c, mhc); err != nil {
		return err
	}
	defer func() {
		u := &unstructured.Unstructured{Object: mhc}
		_ = c.Delete(context.Background(), u)
	}()

	ms := &clusterv1.MachineList{}
	if err := c.List(ctx, ms, client.InNamespace(ns), client.MatchingLabels{"pool": "worker-a"}); err != nil {
		return err
	}
	if len(ms.Items) == 0 {
		return engine.Skip("no worker to remediate")
	}
	victim := ms.Items[0]
	tms, err := list(ctx, c, infraGroup, "TinkerbellMachine", ns)
	if err != nil {
		return err
	}
	hw := ""
	for _, tm := range tms {
		if tm.GetName() == victim.Spec.InfrastructureRef.Name {
			hw = str(tm, "spec", "hardwareName")
		}
	}
	if hw == "" {
		return fmt.Errorf("no hardware found for machine %s", victim.Name)
	}
	if _, err := claimSpares(ctx, s, 1, "worker"); err != nil {
		return err
	}

	job := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "bmc.tinkerbell.org/v1alpha1",
		"kind":       "Job",
		"metadata":   map[string]any{"generateName": "wendy-power-off-", "namespace": ns},
		"spec": map[string]any{
			"machineRef": map[string]any{"name": hw, "namespace": ns},
			"tasks":      []any{map[string]any{"powerAction": "off"}},
		},
	}}
	job.SetGroupVersionKind(schema.GroupVersionKind{Group: bmcGroup, Version: "v1alpha1", Kind: "Job"})
	if err := tink.Create(ctx, job); err != nil {
		return err
	}
	summarize(s, fmt.Sprintf("machine %s (hardware %s) powered off via its BMC", victim.Name, hw))

	eventually(ctx, 15*time.Second, func(g gomega.Gomega) {
		err := c.Get(ctx, client.ObjectKeyFromObject(&victim), &clusterv1.Machine{})
		g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue(), "machine %s not yet remediated", victim.Name)
		md := &clusterv1.MachineDeployment{}
		g.Expect(c.Get(ctx, client.ObjectKey{Namespace: ns, Name: victim.Labels[clusterv1.MachineDeploymentNameLabel]}, md)).To(gomega.Succeed())
		g.Expect(ptrVal(md.Status.ReadyReplicas)).To(gomega.Equal(ptrVal(md.Spec.Replicas)), "replacement not ready")
	})
	s.Log.Info("machine replaced")
	return releaseFreed(ctx, s, 1, "worker", "spare", nil)
}
