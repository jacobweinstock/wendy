package steps

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/yaml"

	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/provider/bmc"
	"github.com/tinkerbell/wendy/internal/provider/docker"
)

// controllerNamespaces hold the controllers whose logs explain a failure.
var controllerNamespaces = []string{"capi-system", "capi-kubeadm-bootstrap-system", "capi-kubeadm-control-plane-system", "capt-system"}

func init() {
	engine.Register("collect.cluster", "Dumps each cluster's objects, pods, events and controller logs.", "{}", collectCluster)
	engine.Register("collect.tinkerbell", "Dumps Tinkerbell's workflows, hardware and BMC objects and logs, and names the failed action.", "{}", collectTinkerbell)
	engine.Register("collect.bmc", "Saves the virtual BMC's log.", "{}", collectBMC)
	engine.Register("collect.consoles", "Saves every machine's serial console.", "{}", collectConsoles)
}

// diagnostic records a file the failure card links, by kind, in the step's
// "diagnostics" output.
func diagnostic(s *engine.Step, kind, path string) {
	m, _ := s.Outputs["diagnostics"].(map[string]any)
	if m == nil {
		m = map[string]any{}
		s.Outputs["diagnostics"] = m
	}
	m[kind] = path
}

func writeYAML(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func collectCluster(ctx context.Context, s *engine.Step, _ struct{}) error {
	var errs []error
	for _, cluster := range []string{engine.ManagementCluster, engine.TinkerbellCluster, engine.WorkloadCluster} {
		if cluster == engine.TinkerbellCluster && s.Testbed.Config.Tinkerbell.Topology != "external" {
			continue
		}
		if _, err := os.Stat(s.Testbed.Kubeconfig(cluster)); err != nil {
			continue
		}
		dir := filepath.Join(s.Artifacts, cluster)
		c, err := kube(s, cluster)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		pods := &corev1.PodList{}
		if err := c.List(ctx, pods); err == nil {
			errs = append(errs, writeYAML(filepath.Join(dir, "pods.yaml"), pods))
		}
		nodes := &corev1.NodeList{}
		if err := c.List(ctx, nodes); err == nil {
			errs = append(errs, writeYAML(filepath.Join(dir, "nodes.yaml"), nodes))
		}
		events := &corev1.EventList{}
		if err := c.List(ctx, events); err == nil {
			errs = append(errs, writeYAML(filepath.Join(dir, "events.yaml"), events))
		}
		if cluster == engine.ManagementCluster {
			withFramework(s, func() {
				defer func() { _ = recover() }()
				framework.DumpAllResources(ctx, framework.DumpAllResourcesInput{
					Lister:               c,
					KubeConfigPath:       s.Testbed.Kubeconfig(cluster),
					ClusterctlConfigPath: filepath.Join(s.Testbed.Dir, "clusterctl", "clusterctl.yaml"),
					Namespace:            s.Testbed.Config.Tinkerbell.Namespace,
					LogPath:              filepath.Join(dir, "resources"),
				})
			})
			errs = append(errs, podLogs(ctx, s, cluster, controllerNamespaces, filepath.Join(dir, "logs")))
		}
		diagnostic(s, "dump", dir)
		s.Log.Info("dumped " + cluster)
	}
	return errors.Join(errs...)
}

// podLogs saves the logs of every container in the namespaces.
func podLogs(ctx context.Context, s *engine.Step, cluster string, namespaces []string, dir string) error {
	cfg, err := clientcmd.BuildConfigFromFlags("", s.Testbed.Kubeconfig(cluster))
	if err != nil {
		return err
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tail := int64(5000)
	for _, ns := range namespaces {
		pods, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for _, p := range pods.Items {
			for _, ctr := range p.Spec.Containers {
				rc, err := cs.CoreV1().Pods(ns).GetLogs(p.Name, &corev1.PodLogOptions{Container: ctr.Name, TailLines: &tail}).Stream(ctx)
				if err != nil {
					continue
				}
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s_%s_%s.log", ns, p.Name, ctr.Name)))
				if err == nil {
					_, _ = io.Copy(f, rc)
					f.Close()
				}
				rc.Close()
			}
		}
	}
	return nil
}

func collectTinkerbell(ctx context.Context, s *engine.Step, _ struct{}) error {
	c, err := kube(s, engine.TinkerbellCluster)
	if err != nil {
		return err
	}
	ns := s.Testbed.Config.Tinkerbell.Namespace
	dir := filepath.Join(s.Artifacts, "tinkerbell")
	var errs []error
	kinds := []struct{ group, kind, file string }{
		{tinkerbellGroup, "Workflow", "workflows.yaml"},
		{tinkerbellGroup, "Hardware", "hardware.yaml"},
		{bmcGroup, "Machine", "bmc-machines.yaml"},
		{bmcGroup, "Job", "bmc-jobs.yaml"},
		{bmcGroup, "Task", "bmc-tasks.yaml"},
	}
	var workflows, tasks []unstructured.Unstructured
	for _, k := range kinds {
		objs, err := list(ctx, c, k.group, k.kind, ns)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		switch k.kind {
		case "Workflow":
			workflows = objs
		case "Task":
			tasks = objs
		}
		items := make([]map[string]any, 0, len(objs))
		for _, o := range objs {
			items = append(items, o.Object)
		}
		errs = append(errs, writeYAML(filepath.Join(dir, k.file), map[string]any{"items": items}))
	}
	errs = append(errs, podLogs(ctx, s, engine.TinkerbellCluster, []string{ns}, filepath.Join(dir, "logs")))
	diagnostic(s, "workflow", filepath.Join(dir, "workflows.yaml"))

	// The failure card's "Related": a failed workflow action, else a failed
	// BMC task, whose reason is often the real one.
	for _, wf := range workflows {
		st := str(wf, "status", "state")
		if isState(st, "FAILED") || isState(st, "TIMEOUT") {
			_, failed := workflowProgress(wf)
			s.Outputs["related"] = fmt.Sprintf("workflow %s failed at %s", wf.GetName(), failed)
			break
		}
	}
	if s.Outputs["related"] == nil {
		for _, t := range tasks {
			if condition(t, "Failed") == "True" {
				s.Outputs["related"] = fmt.Sprintf("BMC task %s failed: %s", t.GetName(), conditionMessage(t, "Failed"))
				break
			}
		}
	}
	return errors.Join(errs...)
}

func conditionMessage(o unstructured.Unstructured, t string) string {
	conds, _, _ := unstructured.NestedSlice(o.Object, "status", "conditions")
	for _, raw := range conds {
		c, _ := raw.(map[string]any)
		if c["type"] == t {
			m, _ := c["message"].(string)
			return m
		}
	}
	return ""
}

func collectBMC(ctx context.Context, s *engine.Step, _ struct{}) error {
	logs, err := docker.Logs(ctx, bmc.Container, "1000")
	if err != nil {
		return err
	}
	list, _, _ := docker.Exec(ctx, bmc.Container, "vbmc", "list")
	path := filepath.Join(s.Artifacts, "bmc.log")
	if err := os.WriteFile(path, []byte(list+"\n"+logs), 0o644); err != nil {
		return err
	}
	diagnostic(s, "bmc", path)
	return nil
}

func collectConsoles(_ context.Context, s *engine.Step, _ struct{}) error {
	// The consoles are streamed into the testbed's directory from the start;
	// this reports where they are and what each machine printed last.
	dir := filepath.Join(s.Testbed.Dir, "consoles")
	for _, m := range s.Testbed.State().Machines {
		b, err := os.ReadFile(filepath.Join(dir, m.Name+".log"))
		if err != nil {
			continue
		}
		if last := lastLine(b); last != "" {
			s.Log.Info(fmt.Sprintf("%s console: %s", m.Name, last))
		}
	}
	diagnostic(s, "console", dir)
	return nil
}

func lastLine(b []byte) string {
	var last string
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	for sc.Scan() {
		if l := strings.TrimSpace(sc.Text()); l != "" {
			last = l
		}
	}
	if len(last) > 160 {
		last = last[:160]
	}
	return last
}
