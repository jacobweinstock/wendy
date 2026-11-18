package cli

import (
	"slices"
	"testing"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/preview"
)

func TestVaryingPartsShowOnlyWhatDiffers(t *testing.T) {
	a := &preview.Combination{Parts: map[string]string{config.KindHardware: "default", config.KindWorkloadCluster: "k8s-1.36", config.KindTinkerbell: "stable"}}
	b := &preview.Combination{Parts: map[string]string{config.KindHardware: "ha", config.KindWorkloadCluster: "k8s-1.36", config.KindTinkerbell: "stable"}}
	c := &preview.Combination{Parts: map[string]string{config.KindHardware: "ha", config.KindWorkloadCluster: "k8s-1.37", config.KindTinkerbell: "stable"}}

	if got := varyingParts([]*preview.Combination{a}); len(got) != 0 {
		t.Errorf("one combination varies in %v", got)
	}
	got := varyingParts([]*preview.Combination{a, b, c})
	if want := []string{config.KindWorkloadCluster, config.KindHardware}; !slices.Equal(got, want) {
		t.Errorf("varying = %v, want %v", got, want)
	}
}

func TestPartUsers(t *testing.T) {
	testbed := func(hw any, m *config.Matrix) *config.TestbedItem {
		return &config.TestbedItem{Item: config.Item[config.TestbedSpec]{Value: config.TestbedSpec{
			Tinkerbell: "default", ClusterAPI: "default", Hardware: hw, WorkloadCluster: "default", Matrix: m,
		}}}
	}
	axisPick := &config.Matrix{Axes: map[string]map[string]any{"size": {"ha": map[string]any{"hardware": "ha"}}}}
	cat := &config.Catalog{Testbeds: map[string]*config.TestbedItem{
		"a": testbed("ha", nil), "b": testbed("ha", nil), "c": testbed("default", nil),
		"d": testbed(map[string]any{"machines": 1}, nil), "e": testbed("default", axisPick),
	}}
	if got := partUsers(cat, config.KindHardware, "ha"); !slices.Equal(got, []string{"a", "b", "e"}) {
		t.Errorf("ha is used by %v", got)
	}
	if got := partUsers(cat, config.KindHardware, "none"); len(got) != 0 {
		t.Errorf("an unused part is used by %v", got)
	}
}
