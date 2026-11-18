package steps

import (
	"context"
	"strings"
	"testing"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
)

// An upgrade step with no target of its own skips, and says which parameter to
// set. Each decides before it touches the testbed, so an empty step is
// enough.
func TestUpgradesSkipWithoutATarget(t *testing.T) {
	ctx, s := context.Background(), &engine.Step{}
	for name, tc := range map[string]struct {
		run  func() error
		want string
	}{
		"kubernetes": {func() error { return upgradeKubernetes(ctx, s, versionParam{}) }, "with: {version: ...}"},
		"tinkerbell": {func() error { return upgradeTinkerbell(ctx, s, upgradeTinkerbellParam{}) }, "with: {to:"},
		"tinkerbell empty target": {func() error {
			return upgradeTinkerbell(ctx, s, upgradeTinkerbellParam{To: &config.TinkerbellTarget{}})
		}, "with: {to:"},
		"capi": {func() error { return capiUpgrade(ctx, s, upgradeCAPIParam{}) }, "with: {core:"},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.run()
			if err == nil || !strings.HasPrefix(err.Error(), "skipped:") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want a skip mentioning %q", err, tc.want)
			}
		})
	}
}
