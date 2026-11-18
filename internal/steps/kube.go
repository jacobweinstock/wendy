// Package steps holds wendy's built-in steps. Importing it registers them.
package steps

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/tinkerbell/wendy/internal/engine"
)

// fieldOwner is wendy's name in server-side apply.
const fieldOwner = "wendy"

var (
	schemeOnce sync.Once
	scheme     *runtime.Scheme
)

// Scheme knows core Kubernetes and the CAPI types the framework helpers use.
// Tinkerbell and CAPT objects are handled as unstructured, so wendy is not
// tied to their API versions.
func Scheme() *runtime.Scheme {
	schemeOnce.Do(func() {
		scheme = runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		_ = controlplanev1.AddToScheme(scheme)
		_ = bootstrapv1.AddToScheme(scheme)
	})
	return scheme
}

// kube is a client for one of the testbed's clusters. It logs to the step.
func kube(s *engine.Step, cluster string) (client.Client, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", s.Testbed.Kubeconfig(cluster))
	if err != nil {
		return nil, fmt.Errorf("%s kubeconfig: %w", cluster, err)
	}
	cfg.Timeout = 30 * time.Second
	return client.New(cfg, client.Options{Scheme: Scheme(), Log: logr.FromSlogHandler(s.Log.Handler())})
}

// proxy is a CAPI framework ClusterProxy for the management cluster.
func proxy(s *engine.Step) framework.ClusterProxy {
	return framework.NewClusterProxy("management", s.Testbed.Kubeconfig(engine.ManagementCluster), Scheme())
}

// apply server-side-applies rendered objects.
func apply(ctx context.Context, c client.Client, objs ...map[string]any) error {
	for _, o := range objs {
		u := &unstructured.Unstructured{Object: o}
		if err := c.Apply(ctx, client.ApplyConfigurationFromUnstructured(u), client.FieldOwner(fieldOwner), client.ForceOwnership); err != nil {
			return fmt.Errorf("applying %s %s/%s: %w", u.GetKind(), u.GetNamespace(), u.GetName(), err)
		}
	}
	return nil
}

// ensureNamespace creates a namespace unless it exists.
func ensureNamespace(ctx context.Context, c client.Client, name string) error {
	err := c.Create(ctx, &corev1.Namespace{ObjectMeta: metaName(name)})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// eventually polls fn until it passes or ctx ends. A Gomega failure inside
// fn fails the step as a product failure.
func eventually(ctx context.Context, interval time.Duration, fn func(g gomega.Gomega)) {
	g := engine.NewGomega()
	timeout := time.Hour
	if dl, ok := ctx.Deadline(); ok {
		timeout = time.Until(dl)
	}
	g.Eventually(ctx, fn).WithTimeout(timeout).WithPolling(interval).Should(gomega.Succeed())
}

// list returns every object of a group and kind in ns, at whatever version
// the cluster serves.
func list(ctx context.Context, c client.Client, group, kind, ns string) ([]unstructured.Unstructured, error) {
	m, err := c.RESTMapper().RESTMapping(schema.GroupKind{Group: group, Kind: kind})
	if err != nil {
		return nil, fmt.Errorf("resolving %s.%s: %w", kind, group, err)
	}
	l := &unstructured.UnstructuredList{}
	l.SetGroupVersionKind(m.GroupVersionKind.GroupVersion().WithKind(kind + "List"))
	opts := []client.ListOption{}
	if ns != "" {
		opts = append(opts, client.InNamespace(ns))
	}
	if err := c.List(ctx, l, opts...); err != nil {
		return nil, err
	}
	return l.Items, nil
}

func str(o unstructured.Unstructured, fields ...string) string {
	v, _, _ := unstructured.NestedString(o.Object, fields...)
	return v
}

func num(o unstructured.Unstructured, fields ...string) int64 {
	v, _, _ := unstructured.NestedInt64(o.Object, fields...)
	return v
}

func boolean(o unstructured.Unstructured, fields ...string) bool {
	v, _, _ := unstructured.NestedBool(o.Object, fields...)
	return v
}

// condition is the status of a condition type, "" when absent.
func condition(o unstructured.Unstructured, t string) string {
	conds, _, _ := unstructured.NestedSlice(o.Object, "status", "conditions")
	for _, raw := range conds {
		c, _ := raw.(map[string]any)
		if c["type"] == t {
			s, _ := c["status"].(string)
			return s
		}
	}
	return ""
}

// workloadClusterKubeconfig fetches the workload cluster's kubeconfig from its
// Secret and stores it in the bundle.
func workloadClusterKubeconfig(ctx context.Context, s *engine.Step, c client.Client) error {
	sec := &corev1.Secret{}
	key := client.ObjectKey{Namespace: s.Testbed.Config.Tinkerbell.Namespace, Name: s.Testbed.Config.WorkloadCluster.Name + "-kubeconfig"}
	if err := c.Get(ctx, key, sec); err != nil {
		return err
	}
	data := sec.Data["value"]
	if len(data) == 0 {
		return fmt.Errorf("secret %s has no kubeconfig", key)
	}
	return s.Testbed.SetKubeconfig(engine.WorkloadCluster, data)
}

// ginkgoRouter is GinkgoWriter while wendy runs: the CAPI framework logs
// there, and each line goes to every step currently calling into it.
type ginkgoRouter struct {
	mu      sync.Mutex
	loggers map[*slog.Logger]int
	lw      io.WriteCloser
}

var router = &ginkgoRouter{loggers: map[*slog.Logger]int{}}

func init() {
	router.lw = engine.LineWriter(func(line string) {
		router.mu.Lock()
		defer router.mu.Unlock()
		line = strings.TrimPrefix(line, "INFO: ")
		for l := range router.loggers {
			l.Info(line)
		}
		if len(router.loggers) == 0 && os.Getenv("WENDY_DEBUG") != "" {
			fmt.Fprintln(os.Stderr, line)
		}
	})
	ginkgo.GinkgoWriter = router
}

// framework routes the CAPI framework's log to s for the duration of fn.
func withFramework(s *engine.Step, fn func()) {
	router.mu.Lock()
	router.loggers[s.Log]++
	router.mu.Unlock()
	defer func() {
		router.mu.Lock()
		router.loggers[s.Log]--
		if router.loggers[s.Log] <= 0 {
			delete(router.loggers, s.Log)
		}
		router.mu.Unlock()
	}()
	fn()
}

func (r *ginkgoRouter) Write(p []byte) (int, error) { return r.lw.Write(p) }
func (r *ginkgoRouter) Print(a ...any)              { fmt.Fprint(r, a...) }
func (r *ginkgoRouter) Printf(f string, a ...any)   { fmt.Fprintf(r, f, a...) }
func (r *ginkgoRouter) Println(a ...any)            { fmt.Fprintln(r, a...) }
func (r *ginkgoRouter) TeeTo(io.Writer)             {}
func (r *ginkgoRouter) ClearTeeWriters()            {}
