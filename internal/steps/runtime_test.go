package steps

import (
	"os"
	"path/filepath"
	"testing"
)

// kind's containerd finds an IPv6 registry's hosts.toml only under
// certs.d/[addr]:port; without it, it pulls over HTTPS from a plain-HTTP
// registry.
func TestWriteCertsDirKeepsIPv6Brackets(t *testing.T) {
	dir := t.TempDir()
	host := "[fd00:ca7e:7::7]:5000"
	rt := map[string]any{
		"certsDir":      dir,
		"registryFiles": map[string]map[string]string{host: {"hosts.toml": "x"}},
	}
	if err := writeCertsDir(rt); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, host, "hosts.toml")); err != nil {
		t.Error(err)
	}
}
