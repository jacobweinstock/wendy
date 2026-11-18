package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	clusterctl "sigs.k8s.io/cluster-api/cmd/clusterctl/client"
	"sigs.k8s.io/yaml"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
)

type upgradeCAPIParam struct {
	Core string       `json:"core,omitempty"`
	CAPT *config.CAPT `json:"capt,omitempty"`
}

func init() {
	// The version check calls GitHub on every invocation.
	os.Setenv("CLUSTERCTL_DISABLE_VERSIONCHECK", "true")

	engine.Register("capi.init",
		"Installs CAPI and CAPT into the management cluster with clusterctl.",
		"{}", capiInit)
	engine.Register("capi.upgrade",
		"Upgrades CAPI and CAPT with clusterctl, to the release in `core` and the CAPT release or build in `capt`.",
		"{core?: string, capt?: #CAPT}", capiUpgrade)
}

// clusterctlConfig writes the clusterctl config for a CAPT target.
func clusterctlConfig(ctx context.Context, s *engine.Step, capt config.CAPT, name string) (string, error) {
	rt, err := runtimeOf(ctx, s)
	if err != nil {
		return "", err
	}
	t, err := captTarget(s.Testbed, capt, s.Testbed.State().Registry.Pull)
	if err != nil {
		return "", err
	}
	rt["capt"] = t
	var cfg map[string]any
	if err := s.Testbed.Render(rt, "clusterctl", &cfg, nil); err != nil {
		return "", err
	}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(s.Testbed.Dir, "clusterctl")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	s.Outputs["captVersion"] = t["version"]
	return path, os.WriteFile(path, b, 0o644)
}

func capiInit(ctx context.Context, s *engine.Step, _ struct{}) error {
	tb := s.Testbed
	path, err := clusterctlConfig(ctx, s, tb.Config.ClusterAPI.CAPT, "clusterctl.yaml")
	if err != nil {
		return err
	}
	if tb.Config.Tinkerbell.Topology == "external" {
		if err := externalKubeconfig(ctx, s); err != nil {
			return fmt.Errorf("external Tinkerbell kubeconfig: %w", err)
		}
	}
	c, err := clusterctl.New(ctx, path)
	if err != nil {
		return err
	}
	core := tb.Config.ClusterAPI.Core
	captVersion, _ := s.Outputs["captVersion"].(string)
	s.Log.Info(fmt.Sprintf("clusterctl init: cluster-api %s, tinkerbell %s", core, captVersion))
	var out []clusterctl.Components
	withFramework(s, func() {
		out, err = c.Init(ctx, clusterctl.InitOptions{
			Kubeconfig:              clusterctl.Kubeconfig{Path: tb.Kubeconfig(engine.ManagementCluster)},
			CoreProvider:            "cluster-api:" + core,
			BootstrapProviders:      []string{"kubeadm:" + core},
			ControlPlaneProviders:   []string{"kubeadm:" + core},
			InfrastructureProviders: []string{"tinkerbell:" + captVersion},
			WaitProviders:           true,
			WaitProviderTimeout:     10 * time.Minute,
		})
	})
	if err != nil {
		return fmt.Errorf("clusterctl init: %w", err)
	}
	for _, comp := range out {
		s.Log.Info(fmt.Sprintf("installed %s %s", comp.Name(), comp.Version()))
	}
	return tb.Update(func(st *engine.State) {
		st.Versions["cluster-api"] = core
		st.Versions["capt"] = captVersion
	})
}

// externalKubeconfig gives CAPT, in the management cluster, the Tinkerbell
// cluster's kubeconfig. It is labelled so clusterctl move carries it.
func externalKubeconfig(ctx context.Context, s *engine.Step) error {
	c, err := kube(s, engine.ManagementCluster)
	if err != nil {
		return err
	}
	if err := ensureNamespace(ctx, c, "capt-system"); err != nil {
		return err
	}
	kc, err := os.ReadFile(s.Testbed.Kubeconfig(engine.TinkerbellCluster))
	if err != nil {
		return err
	}
	sec := &corev1.Secret{
		ObjectMeta: metaName("external-tinkerbell-kubeconfig"),
		Data:       map[string][]byte{"kubeconfig": kc},
	}
	sec.Namespace = "capt-system"
	sec.Labels = map[string]string{"clusterctl.cluster.x-k8s.io/move": "", "clusterctl.cluster.x-k8s.io": ""}
	if err := c.Create(ctx, sec); apierrors.IsAlreadyExists(err) {
		return c.Update(ctx, sec)
	} else if err != nil {
		return err
	}
	return nil
}

func capiUpgrade(ctx context.Context, s *engine.Step, p upgradeCAPIParam) error {
	if p.Core == "" && p.CAPT == nil {
		return engine.Skip("no target: set the step's `with: {core: ...}` or `with: {capt: ...}`")
	}
	tb := s.Testbed
	capt := tb.Config.ClusterAPI.CAPT
	if p.CAPT != nil {
		capt = *p.CAPT
	}
	core := p.Core
	if core == "" {
		core = tb.Config.ClusterAPI.Core
	}
	path, err := clusterctlConfig(ctx, s, capt, "clusterctl-upgrade.yaml")
	if err != nil {
		return err
	}
	captVersion, _ := s.Outputs["captVersion"].(string)
	before := tb.State().Versions
	s.Log.Info(fmt.Sprintf("cluster-api %s → %s, tinkerbell %s → %s", before["cluster-api"], core, before["capt"], captVersion))
	c, err := clusterctl.New(ctx, path)
	if err != nil {
		return err
	}
	withFramework(s, func() {
		err = c.ApplyUpgrade(ctx, clusterctl.ApplyUpgradeOptions{
			Kubeconfig:              clusterctl.Kubeconfig{Path: tb.Kubeconfig(engine.ManagementCluster)},
			CoreProvider:            "capi-system/cluster-api:" + core,
			BootstrapProviders:      []string{"capi-kubeadm-bootstrap-system/kubeadm:" + core},
			ControlPlaneProviders:   []string{"capi-kubeadm-control-plane-system/kubeadm:" + core},
			InfrastructureProviders: []string{"capt-system/tinkerbell:" + captVersion},
			WaitProviders:           true,
			WaitProviderTimeout:     10 * time.Minute,
		})
	})
	if err != nil {
		// A provider that will not upgrade is CAPI/CAPT's problem, not wendy's.
		return engine.Fail(fmt.Errorf("clusterctl upgrade: %w", err))
	}
	return tb.Update(func(st *engine.State) {
		st.Versions["cluster-api"] = core
		st.Versions["capt"] = captVersion
	})
}
