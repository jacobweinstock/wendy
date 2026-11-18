package steps

import (
	"context"
	"fmt"
	"time"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
)

type upgradeTinkerbellParam struct {
	To *config.TinkerbellTarget `json:"to,omitempty"`
}

func init() {
	engine.Register("tinkerbell.install",
		"Installs the Tinkerbell chart, released or built from source, into the cluster that hosts Tinkerbell.",
		"{}", func(ctx context.Context, s *engine.Step, _ struct{}) error {
			cfg := s.Testbed.Config.Tinkerbell
			return installTinkerbell(ctx, s, config.TinkerbellTarget{Chart: cfg.Chart, Source: cfg.Source})
		})
	engine.Register("tinkerbell.upgrade",
		"Upgrades Tinkerbell to the chart release or build in `to`.",
		"{to?: #TinkerbellTarget}", upgradeTinkerbell)
}

func upgradeTinkerbell(ctx context.Context, s *engine.Step, p upgradeTinkerbellParam) error {
	to := p.To
	if to == nil || (to.Chart == nil && to.Source == nil) {
		return engine.Skip("no target: set the step's `with: {to: {chart: ...}}` or `with: {to: {source: ...}}`")
	}
	if to.Source == nil && to.Chart.Location == "" {
		to.Chart.Location = s.Testbed.Config.Tinkerbell.Chart.Location
	}
	before := s.Testbed.State().Versions["tinkerbell"]
	if err := installTinkerbell(ctx, s, *to); err != nil {
		return err
	}
	s.Log.Info(fmt.Sprintf("Tinkerbell %s → %s", before, s.Testbed.State().Versions["tinkerbell"]))
	return nil
}

// installTinkerbell installs or upgrades the chart and waits for it.
func installTinkerbell(ctx context.Context, s *engine.Step, to config.TinkerbellTarget) error {
	cfg := s.Testbed.Config.Tinkerbell
	rel := chartRelease{
		Kubeconfig: s.Testbed.Kubeconfig(engine.TinkerbellCluster),
		Namespace:  cfg.Namespace,
		Name:       "tinkerbell",
		Timeout:    15 * time.Minute,
	}
	if dl, ok := ctx.Deadline(); ok {
		rel.Timeout = time.Until(dl) - 30*time.Second
	}
	if to.Source != nil {
		out, err := s.Testbed.Builds.Get("tinkerbell", *to.Source)
		if err != nil {
			return err
		}
		rel.Chart, _ = out["chart"].(string)
	} else {
		v, err := chartVersion(ctx, s.Testbed.Execution, to.Chart.Location, to.Chart.Version)
		if err != nil {
			return err
		}
		rel.Chart, rel.Version = to.Chart.Location, v
	}

	rt, err := runtimeOf(ctx, s)
	if err != nil {
		return err
	}
	if to.Source != nil {
		// An upgrade to a build uses the build's images too.
		out, _ := s.Testbed.Builds.Get("tinkerbell", *to.Source)
		pull := s.Testbed.State().Registry.Pull
		if img, _ := out["image"].(string); img != "" {
			rt["tinkerbellImage"] = pullRef(img, pull)
		}
		if img, _ := out["agentImage"].(string); img != "" {
			rt["agentImage"] = pullRef(img, pull)
		}
	} else {
		delete(rt, "tinkerbellImage")
	}
	var values map[string]any
	if err := s.Testbed.Render(rt, "tinkerbellValues", &values, nil); err != nil {
		return err
	}
	values = merge(values, cfg.Values)
	rel.Values = values
	s.Log.Info("Tinkerbell VIP " + s.Testbed.State().VIPs.Tinkerbell)

	version, err := helmUpgrade(ctx, s.Log, rel)
	if err != nil {
		return fmt.Errorf("helm: %w", err)
	}
	s.Outputs["chart"] = rel.Chart
	s.Outputs["version"] = version
	return s.Testbed.Update(func(st *engine.State) { st.Versions["tinkerbell"] = version })
}
