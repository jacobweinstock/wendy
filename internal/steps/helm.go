package steps

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/cli"
	helmkube "helm.sh/helm/v4/pkg/kube"
	"helm.sh/helm/v4/pkg/registry"
	"helm.sh/helm/v4/pkg/storage/driver"
)

// chartRelease is a Helm release to install or upgrade.
type chartRelease struct {
	Kubeconfig string
	Namespace  string
	Name       string
	// Chart is an OCI reference, a repo chart name with RepoURL, or a path.
	Chart   string
	RepoURL string
	Version string
	Values  map[string]any
	Timeout time.Duration
}

// helmUpgrade installs the release, or upgrades it if it exists, and waits
// for its resources to be ready. It returns the chart version it installed.
func helmUpgrade(ctx context.Context, log *slog.Logger, r chartRelease) (string, error) {
	settings := cli.New()
	settings.KubeConfig = r.Kubeconfig
	settings.SetNamespace(r.Namespace)
	cfg := action.NewConfiguration(action.ConfigurationSetLogger(log.Handler()))
	if err := cfg.Init(settings.RESTClientGetter(), r.Namespace, "secret"); err != nil {
		return "", err
	}
	rc, err := registry.NewClient(registry.ClientOptWriter(io.Discard))
	if err != nil {
		return "", err
	}
	cfg.RegistryClient = rc

	hist := action.NewHistory(cfg)
	hist.Max = 1
	_, err = hist.Run(r.Name)
	installed := err == nil
	if err != nil && !errors.Is(err, driver.ErrReleaseNotFound) {
		return "", err
	}

	cpo := action.ChartPathOptions{Version: r.Version, RepoURL: r.RepoURL}
	if installed {
		up := action.NewUpgrade(cfg)
		up.ChartPathOptions = cpo
		up.Namespace, up.WaitStrategy, up.Timeout = r.Namespace, helmkube.StatusWatcherStrategy, r.Timeout
		up.SetRegistryClient(rc)
		path, err := up.LocateChart(r.Chart, settings)
		if err != nil {
			return "", fmt.Errorf("locating %s: %w", r.Chart, err)
		}
		ch, err := loader.Load(path)
		if err != nil {
			return "", err
		}
		log.Info(fmt.Sprintf("upgrading %s to %s %s", r.Name, ch.Metadata.Name, ch.Metadata.Version))
		if _, err := up.RunWithContext(ctx, r.Name, ch, r.Values); err != nil {
			return "", err
		}
		return ch.Metadata.Version, nil
	}

	in := action.NewInstall(cfg)
	in.ChartPathOptions = cpo
	in.ReleaseName, in.Namespace, in.CreateNamespace = r.Name, r.Namespace, true
	in.WaitStrategy, in.Timeout = helmkube.StatusWatcherStrategy, r.Timeout
	in.SetRegistryClient(rc)
	path, err := in.LocateChart(r.Chart, settings)
	if err != nil {
		return "", fmt.Errorf("locating %s: %w", r.Chart, err)
	}
	ch, err := loader.Load(path)
	if err != nil {
		return "", err
	}
	log.Info(fmt.Sprintf("installing %s %s into %s", ch.Metadata.Name, ch.Metadata.Version, r.Namespace))
	if _, err := in.RunWithContext(ctx, ch, r.Values); err != nil {
		return "", err
	}
	return ch.Metadata.Version, nil
}

// merge deep-merges src over dst, src winning.
func merge(dst, src map[string]any) map[string]any {
	if dst == nil {
		dst = map[string]any{}
	}
	for k, v := range src {
		sm, sok := v.(map[string]any)
		dm, dok := dst[k].(map[string]any)
		if sok && dok {
			dst[k] = merge(dm, sm)
			continue
		}
		dst[k] = v
	}
	return dst
}
