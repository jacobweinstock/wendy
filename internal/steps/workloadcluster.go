package steps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/tinkerbell/wendy/internal/engine"
)

const (
	kubeRouterManifest = "https://raw.githubusercontent.com/cloudnativelabs/kube-router/master/daemonset/kubeadm-kuberouter.yaml"
	ciliumRepo         = "https://helm.cilium.io"
	ciliumVersion      = "1.20.4"
)

func init() {
	engine.Register("workloadCluster.cni", "Installs the workload cluster's CNI (workloadCluster.cni) and waits for it to roll out.", "{}", workloadClusterCNI)
}

func workloadClusterCNI(ctx context.Context, s *engine.Step, _ struct{}) error {
	cfg := s.Testbed.Config
	v6 := cfg.Network.Family == "ipv6"
	switch cfg.WorkloadCluster.CNI {
	case "kube-router":
		return kubeRouter(ctx, s, v6)
	case "cilium":
		values := map[string]any{
			"ipam":     map[string]any{"mode": "kubernetes"},
			"operator": map[string]any{"replicas": 1},
		}
		if v6 {
			values["ipv4"] = map[string]any{"enabled": false}
			values["ipv6"] = map[string]any{"enabled": true}
		}
		v, err := helmUpgrade(ctx, s.Log, chartRelease{
			Kubeconfig: s.Testbed.Kubeconfig(engine.WorkloadCluster), Namespace: "kube-system", Name: "cilium",
			Chart: "cilium", RepoURL: ciliumRepo, Version: ciliumVersion, Values: values, Timeout: 8 * time.Minute,
		})
		if err != nil {
			return fmt.Errorf("installing cilium: %w", err)
		}
		s.Outputs["cilium"] = v
		return nil
	}
	return fmt.Errorf("unsupported CNI %q", cfg.WorkloadCluster.CNI)
}

// kubeRouter applies kube-router's kubeadm manifest. It is IPv4-only, so an
// IPv6 cluster also needs the family flipped, a service range it accepts and
// a generated BGP router id, which it cannot derive from an IPv6 address.
func kubeRouter(ctx context.Context, s *engine.Step, v6 bool) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, kubeRouterManifest, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching kube-router: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching kube-router: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	c, err := kube(s, engine.WorkloadCluster)
	if err != nil {
		return err
	}
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(body), 4096)
	for {
		var obj map[string]any
		if err := dec.Decode(&obj); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return err
		}
		if len(obj) == 0 {
			continue
		}
		if v6 && obj["kind"] == "DaemonSet" {
			addArgs(obj, "--enable-ipv4=false", "--enable-ipv6=true", "--service-cluster-ip-range=fd00:10:96::/112", "--router-id=generate")
		}
		if err := apply(ctx, c, obj); err != nil {
			return err
		}
	}
	s.Log.Info("applied kube-router; waiting for it to roll out")
	eventuallyErr(ctx, 5*time.Second, func() error {
		ds := &appsv1.DaemonSet{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: "kube-system", Name: "kube-router"}, ds); err != nil {
			return err
		}
		if ds.Status.DesiredNumberScheduled == 0 || ds.Status.NumberReady < ds.Status.DesiredNumberScheduled {
			return fmt.Errorf("kube-router %d/%d ready", ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)
		}
		return nil
	})
	return nil
}

func addArgs(ds map[string]any, args ...string) {
	spec, _ := ds["spec"].(map[string]any)
	tmpl, _ := spec["template"].(map[string]any)
	pspec, _ := tmpl["spec"].(map[string]any)
	ctrs, _ := pspec["containers"].([]any)
	if len(ctrs) == 0 {
		return
	}
	c0, _ := ctrs[0].(map[string]any)
	existing, _ := c0["args"].([]any)
	for _, a := range args {
		existing = append(existing, a)
	}
	c0["args"] = existing
}

// eventuallyErr polls fn until it returns nil; the last error fails the step.
func eventuallyErr(ctx context.Context, interval time.Duration, fn func() error) {
	g := engine.NewGomega()
	timeout := time.Hour
	if dl, ok := ctx.Deadline(); ok {
		timeout = time.Until(dl)
	}
	g.Eventually(ctx, fn).WithTimeout(timeout).WithPolling(interval).Should(gomega.Succeed())
}
