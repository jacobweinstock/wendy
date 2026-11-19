package registry

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestMirrorCertsChainToOneCA(t *testing.T) {
	data := t.TempDir()
	name := MirrorName("ghcr.io")
	dir, fp, err := mirrorCerts(data, name)
	if err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(dir, "tls.crt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tls.LoadX509KeyPair(filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")); err != nil {
		t.Fatalf("the mirror could not serve its pair: %v", err)
	}

	caPEM, err := MirrorCA(data)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(caPEM)) {
		t.Fatalf("MirrorCA is not PEM: %q", caPEM)
	}
	block, _ := pem.Decode(first)
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: name}); err != nil {
		t.Errorf("%s does not verify against MirrorCA: %v", name, err)
	}

	_, again, err := mirrorCerts(data, name)
	if err != nil {
		t.Fatal(err)
	}
	if second, _ := os.ReadFile(filepath.Join(dir, "tls.crt")); again != fp || string(second) != string(first) {
		t.Error("a second call replaced the CA or the mirror's certificate")
	}
}
