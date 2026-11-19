package steps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The action's own error, which reaches Smee's log as tink-agent syslog
// output, is what a failure names, not tink-agent's own records or another
// machine's errors.
func TestActionErrorFromTinkerbellLog(t *testing.T) {
	lines := []string{
		`{"msg":"syslog message received","logger":"smee","syslog":{"app-name":"tink-agent","hostname":"wendy-b2c5fb-cp1","msg":"time=\"2026-11-19T16:17:25Z\" level=fatal msg=\"failed to do request: Head \\\"https://172.20.0.2:5000/v2/tinkerbell/cluster-api-provider-tinkerbell/ubuntu/manifests/2404-v1.35.2-amd64.gz\\\": http: server gave HTTP response to HTTPS client\"\n"}}`,
		`{"msg":"syslog message received","logger":"smee","syslog":{"app-name":"tink-agent","hostname":"wendy-b2c5fb-cp1","msg":{"error":"task exited with non-zero code: 1, error: %!w(<nil>)","level":0,"msg":"error executing action"}}}`,
		`{"msg":"syslog message received","logger":"smee","syslog":{"app-name":"tink-agent","hostname":"wendy-b2c5fb-worker1","msg":"time=\"2026-11-19T16:18:00Z\" level=fatal msg=\"another machine\"\n"}}`,
		`{"msg":"syslog message received","logger":"smee","syslog":{"app-name":"tink-agent","hostname":"wendy-b2c5fb-cp1","msg":"time=\"2026-11-19T16:17:20Z\" level=info msg=\"Beginning write of image\"\n"}}`,
		`not json`,
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tinkerbell_tinkerbell-0_tinkerbell.log"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := `failed to do request: Head "https://172.20.0.2:5000/v2/tinkerbell/cluster-api-provider-tinkerbell/ubuntu/manifests/2404-v1.35.2-amd64.gz": http: server gave HTTP response to HTTPS client`
	if got := actionError(dir, "wendy-b2c5fb-cp1"); got != want {
		t.Errorf("actionError = %q, want %q", got, want)
	}
	if got := actionError(dir, "wendy-b2c5fb-cp2"); got != "" {
		t.Errorf("a machine that logged nothing got %q", got)
	}
}
