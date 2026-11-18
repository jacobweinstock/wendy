package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func get(t *testing.T, s *Server, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.String()
}

const partsConfig = `package local

parts: hardware: ha: machines: {controlPlane: {count: 3, cpus: 4}, worker: count: 3}
parts: hardware: idle: {machines: worker: count: 9, bmc: protocol: "ipmi"}
parts: workloadCluster: next: kubernetesVersion: "v1.37.0"

testbeds: big: {hardware: "ha", workloadCluster: "next"}
testbeds: adhoc: hardware: {machines: worker: count: 5}

testbeds: m: {
	hardware: "ha"
	matrix: {
		order: ["k"]
		axes: k: {a: workloadCluster: "next", b: {}}
		exclude: [{k: "b", reason: "not today"}]
	}
}
`

func partsServer(t *testing.T) *Server {
	t.Helper()
	s, svc := editServer(t)
	if err := os.WriteFile(filepath.Join(svc.StarterDir, "parts.cue"), []byte(partsConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	svc.Reload()
	return s
}
