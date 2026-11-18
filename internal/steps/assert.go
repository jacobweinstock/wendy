package steps

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/tinkerbell/wendy/internal/engine"
)

const (
	clusterGroup      = "cluster.x-k8s.io"
	controlPlaneGroup = "controlplane.cluster.x-k8s.io"
	infraGroup        = "infrastructure.cluster.x-k8s.io"
	tinkerbellGroup   = "tinkerbell.org"
	bmcGroup          = "bmc.tinkerbell.org"
)

// Container waiting reasons that never clear, and ones that may but are
// only tolerated for stuckGrace.
var (
	terminalReasons = []string{"CreateContainerConfigError", "CreateContainerError", "InvalidImageName"}
	stuckReasons    = []string{"CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull"}
)

const stuckGrace = 2 * time.Minute

func init() {
	engine.Register("assert.workflows", "Every machine's Tinkerbell workflow reaches SUCCESS.", "{}", assertWorkflows)
	engine.Register("assert.apiserver", "The workload cluster's API server answers /readyz.", "{}", assertAPIServer)
	engine.Register("assert.nodes", "Every node of the workload cluster is Ready.", "{}", assertNodes)
	engine.Register("assert.capi-ready", "The Cluster, control plane, MachineDeployments, Machines and CAPT objects report ready.", "{}", assertCAPIReady)
	engine.Register("assert.pods", "Every pod on the workload cluster is running and ready, or has completed.", "{}", assertPods)
	engine.Register("assert.components", "Every required component on the workload cluster has a ready pod.", "{}", assertComponents)
	engine.Register("assert.healthy", "All of: CAPI ready, nodes, pods and components.", "{}", func(ctx context.Context, s *engine.Step, p struct{}) error {
		for _, fn := range []func(context.Context, *engine.Step, struct{}) error{assertCAPIReady, assertNodes, assertPods, assertComponents} {
			if err := fn(ctx, s, p); err != nil {
				return err
			}
		}
		return nil
	})
}

// desired is how many control-plane and worker machines the cluster should
// have now, from its specs, so checks stay right after scaling.
func desired(ctx context.Context, c client.Client, ns string) (cp, workers int64, err error) {
	kcps, err := list(ctx, c, controlPlaneGroup, "KubeadmControlPlane", ns)
	if err != nil {
		return 0, 0, err
	}
	for _, o := range kcps {
		cp += num(o, "spec", "replicas")
	}
	mds, err := list(ctx, c, clusterGroup, "MachineDeployment", ns)
	if err != nil {
		return 0, 0, err
	}
	for _, o := range mds {
		workers += num(o, "spec", "replicas")
	}
	return cp, workers, nil
}

func isState(state, want string) bool {
	return state == want || strings.HasSuffix(state, "_"+want)
}

// workflowProgress is "n/m actions" and, when an action failed, which one.
func workflowProgress(o unstructured.Unstructured) (string, string) {
	tasks, _, _ := unstructured.NestedSlice(o.Object, "status", "tasks")
	done, total := 0, 0
	failed := ""
	for _, t := range tasks {
		tm, _ := t.(map[string]any)
		actions, _ := tm["actions"].([]any)
		for _, a := range actions {
			am, _ := a.(map[string]any)
			total++
			st, _ := am["state"].(string)
			switch {
			case isState(st, "SUCCESS"):
				done++
			case isState(st, "FAILED") || isState(st, "TIMEOUT"):
				name, _ := am["name"].(string)
				msg, _ := am["message"].(string)
				failed = fmt.Sprintf("action %q %s", name, strings.ToLower(strings.TrimPrefix(st, "STATE_")))
				if msg != "" {
					failed += ": " + msg
				}
			}
		}
	}
	if failed == "" {
		if cur := str(o, "status", "currentState", "actionName"); cur != "" && (isState(str(o, "status", "state"), "FAILED") || isState(str(o, "status", "state"), "TIMEOUT")) {
			failed = fmt.Sprintf("action %q %s", cur, strings.ToLower(str(o, "status", "state")))
		}
	}
	return fmt.Sprintf("%d/%d actions", done, total), failed
}

func assertWorkflows(ctx context.Context, s *engine.Step, _ struct{}) error {
	mgmt, err := kube(s, engine.ManagementCluster)
	if err != nil {
		return err
	}
	tink, err := kube(s, engine.TinkerbellCluster)
	if err != nil {
		return err
	}
	ns := s.Testbed.Config.Tinkerbell.Namespace
	seen := map[string]string{}
	eventually(ctx, 10*time.Second, func(g gomega.Gomega) {
		cp, workers, err := desired(ctx, mgmt, ns)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		wfs, err := list(ctx, tink, tinkerbellGroup, "Workflow", ns)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		for _, wf := range wfs {
			state := str(wf, "status", "state")
			progress, failed := workflowProgress(wf)
			line := fmt.Sprintf("workflow %s %s %s", wf.GetName(), strings.ToLower(strings.TrimPrefix(state, "STATE_")), progress)
			if seen[wf.GetName()] != line {
				seen[wf.GetName()] = line
				s.Log.Info(line)
			}
			if isState(state, "FAILED") || isState(state, "TIMEOUT") {
				gomega.StopTrying(fmt.Sprintf("workflow %s is %s and will not recover: %s", wf.GetName(), state, failed)).Now()
			}
		}
		want := int(cp + workers)
		g.Expect(len(wfs)).To(gomega.BeNumerically(">=", want), "waiting for %d workflows, have %d", want, len(wfs))
		for _, wf := range wfs {
			g.Expect(isState(str(wf, "status", "state"), "SUCCESS")).To(gomega.BeTrue(), "workflow %s is %s", wf.GetName(), str(wf, "status", "state"))
		}
	})
	return nil
}

func workloadClusterClientset(s *engine.Step) (*kubernetes.Clientset, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", s.Testbed.Kubeconfig(engine.WorkloadCluster))
	if err != nil {
		return nil, err
	}
	cfg.Timeout = 15 * time.Second
	return kubernetes.NewForConfig(cfg)
}

func assertAPIServer(ctx context.Context, s *engine.Step, _ struct{}) error {
	cs, err := workloadClusterClientset(s)
	if err != nil {
		return err
	}
	logged := false
	eventually(ctx, 10*time.Second, func(g gomega.Gomega) {
		body, err := cs.Discovery().RESTClient().Get().AbsPath("/readyz").DoRaw(ctx)
		if err != nil && !logged {
			logged = true
			s.Log.Info("waiting for the workload cluster's API server at " + s.Testbed.State().VIPs.ControlPlane)
		}
		g.Expect(err).ToNot(gomega.HaveOccurred(), "API server not ready: %s", string(body))
	})
	return nil
}

func assertNodes(ctx context.Context, s *engine.Step, _ struct{}) error {
	mgmt, err := kube(s, engine.ManagementCluster)
	if err != nil {
		return err
	}
	wl, err := kube(s, engine.WorkloadCluster)
	if err != nil {
		return err
	}
	ns := s.Testbed.Config.Tinkerbell.Namespace
	last := ""
	eventually(ctx, 10*time.Second, func(g gomega.Gomega) {
		cp, workers, err := desired(ctx, mgmt, ns)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		nodes := &corev1.NodeList{}
		g.Expect(wl.List(ctx, nodes)).To(gomega.Succeed())
		var notReady []string
		for _, n := range nodes.Items {
			if !nodeReady(n) {
				notReady = append(notReady, n.Name)
			}
		}
		line := fmt.Sprintf("%d of %d nodes Ready", len(nodes.Items)-len(notReady), cp+workers)
		if line != last {
			last = line
			s.Log.Info(line)
		}
		g.Expect(nodes.Items).To(gomega.HaveLen(int(cp+workers)), "expected %d nodes, have %d", cp+workers, len(nodes.Items))
		g.Expect(notReady).To(gomega.BeEmpty(), "nodes not Ready: %v", notReady)
	})
	return nil
}

func nodeReady(n corev1.Node) bool {
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// terminal stops the wait when an object reports a failure that will not
// clear on its own.
func terminal(o unstructured.Unstructured) {
	id := strings.ToLower(o.GetKind()) + "/" + o.GetName()
	if r := str(o, "status", "failureReason"); r != "" {
		gomega.StopTrying(fmt.Sprintf("%s failed: %s: %s", id, r, str(o, "status", "failureMessage"))).Now()
	}
	if m := str(o, "status", "failureMessage"); m != "" {
		gomega.StopTrying(fmt.Sprintf("%s failed: %s", id, m)).Now()
	}
	if str(o, "status", "phase") == "Failed" {
		gomega.StopTrying(id + " is in phase Failed").Now()
	}
}

// conditionTrue asserts the first of the aliases the object reports is
// True; CAPI renamed conditions between v1beta1 and v1beta2.
func conditionTrue(g gomega.Gomega, o unstructured.Unstructured, aliases ...string) {
	id := strings.ToLower(o.GetKind()) + "/" + o.GetName()
	for _, t := range aliases {
		if st := condition(o, t); st != "" {
			g.Expect(st).To(gomega.Equal("True"), "%s condition %s is %s", id, t, st)
			return
		}
	}
	g.Expect(aliases).To(gomega.BeEmpty(), "%s reports none of the conditions %v", id, aliases)
}

func assertCAPIReady(ctx context.Context, s *engine.Step, _ struct{}) error {
	c, err := kube(s, engine.ManagementCluster)
	if err != nil {
		return err
	}
	ns := s.Testbed.Config.Tinkerbell.Namespace
	eventually(ctx, 15*time.Second, func(g gomega.Gomega) {
		cp, workers, err := desired(ctx, c, ns)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		get := func(group, kind string) []unstructured.Unstructured {
			objs, err := list(ctx, c, group, kind, ns)
			g.Expect(err).ToNot(gomega.HaveOccurred())
			return objs
		}
		clusters := get(clusterGroup, "Cluster")
		g.Expect(clusters).To(gomega.HaveLen(1))
		for _, o := range clusters {
			terminal(o)
			g.Expect(str(o, "status", "phase")).To(gomega.Equal("Provisioned"), "cluster/%s phase", o.GetName())
			conditionTrue(g, o, "Available", "Ready")
			conditionTrue(g, o, "InfrastructureReady")
			conditionTrue(g, o, "ControlPlaneAvailable", "ControlPlaneReady")
		}
		for _, o := range get(infraGroup, "TinkerbellCluster") {
			terminal(o)
			g.Expect(boolean(o, "status", "ready")).To(gomega.BeTrue(), "tinkerbellcluster/%s is not ready", o.GetName())
		}
		for _, o := range get(controlPlaneGroup, "KubeadmControlPlane") {
			terminal(o)
			conditionTrue(g, o, "Available", "Ready")
			g.Expect(num(o, "status", "replicas")).To(gomega.Equal(cp), "kubeadmcontrolplane/%s replicas", o.GetName())
			g.Expect(num(o, "status", "readyReplicas")).To(gomega.Equal(cp), "kubeadmcontrolplane/%s readyReplicas", o.GetName())
		}
		for _, o := range get(clusterGroup, "MachineDeployment") {
			terminal(o)
			g.Expect(str(o, "status", "phase")).To(gomega.Equal("Running"), "machinedeployment/%s phase", o.GetName())
			g.Expect(num(o, "status", "readyReplicas")).To(gomega.Equal(num(o, "spec", "replicas")), "machinedeployment/%s readyReplicas", o.GetName())
			g.Expect(num(o, "status", "availableReplicas")).To(gomega.Equal(num(o, "spec", "replicas")), "machinedeployment/%s availableReplicas", o.GetName())
		}
		machines := get(clusterGroup, "Machine")
		g.Expect(int64(len(machines))).To(gomega.Equal(cp+workers), "expected %d Machines, have %d", cp+workers, len(machines))
		for _, o := range machines {
			terminal(o)
			g.Expect(str(o, "status", "phase")).To(gomega.Equal("Running"), "machine/%s phase", o.GetName())
			g.Expect(str(o, "status", "nodeRef", "name")).ToNot(gomega.BeEmpty(), "machine/%s has no nodeRef", o.GetName())
			conditionTrue(g, o, "Ready")
		}
		tms := get(infraGroup, "TinkerbellMachine")
		g.Expect(int64(len(tms))).To(gomega.Equal(cp+workers), "expected %d TinkerbellMachines, have %d", cp+workers, len(tms))
		for _, o := range tms {
			terminal(o)
			g.Expect(boolean(o, "status", "ready")).To(gomega.BeTrue(), "tinkerbellmachine/%s is not ready", o.GetName())
		}
	})
	return nil
}

func assertPods(ctx context.Context, s *engine.Step, _ struct{}) error {
	c, err := kube(s, engine.WorkloadCluster)
	if err != nil {
		return err
	}
	firstSeen := map[string]time.Time{}
	eventually(ctx, 10*time.Second, func(g gomega.Gomega) {
		pods := &corev1.PodList{}
		g.Expect(c.List(ctx, pods)).To(gomega.Succeed())
		g.Expect(pods.Items).ToNot(gomega.BeEmpty(), "no pods on the workload cluster")
		var unhealthy []string
		for i := range pods.Items {
			pod := &pods.Items[i]
			id := pod.Namespace + "/" + pod.Name
			if pod.DeletionTimestamp != nil || pod.Status.Phase == corev1.PodSucceeded {
				continue
			}
			for _, cs := range append(slices.Clone(pod.Status.InitContainerStatuses), pod.Status.ContainerStatuses...) {
				w := cs.State.Waiting
				if w == nil {
					continue
				}
				if slices.Contains(terminalReasons, w.Reason) {
					gomega.StopTrying(fmt.Sprintf("pod/%s container %s is in %s: %s", id, cs.Name, w.Reason, w.Message)).Now()
				}
				if slices.Contains(stuckReasons, w.Reason) {
					key := id + "/" + w.Reason
					if _, ok := firstSeen[key]; !ok {
						firstSeen[key] = time.Now()
					}
					if stuck := time.Since(firstSeen[key]); stuck > stuckGrace {
						gomega.StopTrying(fmt.Sprintf("pod/%s container %s stuck in %s for %s: %s", id, cs.Name, w.Reason, stuck.Round(time.Second), w.Message)).Now()
					}
				}
			}
			if pod.Status.Phase != corev1.PodRunning {
				unhealthy = append(unhealthy, fmt.Sprintf("pod/%s phase=%s", id, pod.Status.Phase))
				continue
			}
			// Init containers are excluded: a completed one reports ready=false.
			for _, cs := range pod.Status.ContainerStatuses {
				if !cs.Ready {
					unhealthy = append(unhealthy, fmt.Sprintf("pod/%s container %s not ready", id, cs.Name))
				}
			}
		}
		g.Expect(unhealthy).To(gomega.BeEmpty(), "unhealthy pods:\n  %s", strings.Join(unhealthy, "\n  "))
	})
	return nil
}

// components are matched as pod name prefixes.
func components(cni string) []string {
	return []string{"etcd", "kube-apiserver", "kube-controller-manager", "kube-scheduler", "kube-proxy", "coredns", cni}
}

func assertComponents(ctx context.Context, s *engine.Step, _ struct{}) error {
	c, err := kube(s, engine.WorkloadCluster)
	if err != nil {
		return err
	}
	want := components(s.Testbed.Config.WorkloadCluster.CNI)
	eventually(ctx, 10*time.Second, func(g gomega.Gomega) {
		pods := &corev1.PodList{}
		g.Expect(c.List(ctx, pods)).To(gomega.Succeed())
		var missing []string
		for _, comp := range want {
			found := false
			for i := range pods.Items {
				p := &pods.Items[i]
				if strings.HasPrefix(p.Name, comp) && p.Status.Phase == corev1.PodRunning && allReady(p) {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, comp)
			}
		}
		g.Expect(missing).To(gomega.BeEmpty(), "components with no ready pod: %s", strings.Join(missing, ", "))
	})
	return nil
}

func allReady(p *corev1.Pod) bool {
	for _, cs := range p.Status.ContainerStatuses {
		if !cs.Ready {
			return false
		}
	}
	return len(p.Status.ContainerStatuses) > 0
}
