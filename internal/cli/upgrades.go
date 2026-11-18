package cli

import (
	"slices"

	"github.com/tinkerbell/wendy/internal/config"
)

// upgradeVersions are the Kubernetes versions that tests upgrade to, which
// need their machine image as much as a testbed's own version does.
func upgradeVersions(cat *config.Catalog) []string {
	var out []string
	for _, name := range config.Names(cat.Tests) {
		for _, st := range cat.Tests[name].Value.Steps {
			st = cat.Expand(st)
			if st.Uses != "capt.upgrade.kubernetes" {
				continue
			}
			if v, _ := st.With["version"].(string); v != "" && !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
	}
	return out
}
