package registry

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"time"
)

// keyPair is a certificate and its private key.
type keyPair struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

// MirrorCA returns the PEM certificate every mirror's certificate chains to,
// creating it on first use. One CA serves every combination on the host, as
// the mirrors do.
func MirrorCA(data string) (string, error) {
	var out string
	err := lockedPKI(data, func(dir string) error {
		ca, err := ensureCA(dir)
		if err != nil {
			return err
		}
		out = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.cert.Raw}))
		return nil
	})
	return out, err
}

// mirrorCerts returns the directory holding the certificate a mirror serves,
// tls.crt and tls.key for its container name, and the fingerprint of the CA
// that signed it.
func mirrorCerts(data, name string) (string, string, error) {
	var leafDir, fp string
	err := lockedPKI(data, func(dir string) error {
		ca, err := ensureCA(dir)
		if err != nil {
			return err
		}
		leafDir = filepath.Join(dir, name)
		fp = fingerprint(ca.cert)
		return ensureLeaf(leafDir, name, ca)
	})
	return leafDir, fp, err
}

// lockedPKI runs fn on the mirrors' PKI directory under an exclusive lock, so
// concurrent wendy processes agree on one CA.
func lockedPKI(data string, fn func(dir string) error) error {
	dir := filepath.Join(data, "mirrors")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("locking %s: %w", dir, err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) //nolint:errcheck // Closing the file releases the lock anyway.
	return fn(dir)
}

func ensureCA(dir string) (*keyPair, error) {
	certPath, keyPath := filepath.Join(dir, "ca.crt"), filepath.Join(dir, "ca.key")
	if p, err := loadPair(certPath, keyPath); err != nil || p != nil {
		return p, err
	}
	tmpl := &x509.Certificate{
		Subject:               pkix.Name{CommonName: "wendy mirror CA", Organization: []string{"wendy"}},
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	if err := issue(certPath, keyPath, tmpl, nil); err != nil {
		return nil, err
	}
	return loadPair(certPath, keyPath)
}

// ensureLeaf writes a certificate for name signed by ca into dir, unless one
// is already there.
func ensureLeaf(dir, name string, ca *keyPair) error {
	certPath, keyPath := filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	// One that cannot be read is replaced rather than reported: it is only
	// ever wendy's to make.
	if p, err := loadPair(certPath, keyPath); err == nil && p != nil &&
		p.cert.CheckSignatureFrom(ca.cert) == nil && slices.Contains(p.cert.DNSNames, name) {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return issue(certPath, keyPath, &x509.Certificate{
		Subject:     pkix.Name{CommonName: name, Organization: []string{"wendy"}},
		DNSNames:    []string{name},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, ca)
}

// issue makes a key and a certificate from tmpl, signed by parent or, when
// parent is nil, by itself, and writes both.
func issue(certPath, keyPath string, tmpl *x509.Certificate, parent *keyPair) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	tmpl.SerialNumber = serial
	tmpl.NotBefore = time.Now().Add(-time.Hour)
	tmpl.NotAfter = time.Now().AddDate(10, 0, 0)
	signer, signerKey := tmpl, key
	if parent != nil {
		signer, signerKey = parent.cert, parent.key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signer, &key.PublicKey, signerKey)
	if err != nil {
		return err
	}
	kb, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err := writeAtomic(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600); err != nil {
		return err
	}
	return writeAtomic(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}

// loadPair reads a pair issue wrote. Both missing is (nil, nil).
func loadPair(certPath, keyPath string) (*keyPair, error) {
	cb, err := os.ReadFile(certPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	kb, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	cblock, _ := pem.Decode(cb)
	kblock, _ := pem.Decode(kb)
	if cblock == nil || kblock == nil {
		return nil, fmt.Errorf("%s or %s is not PEM", certPath, keyPath)
	}
	cert, err := x509.ParseCertificate(cblock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", certPath, err)
	}
	key, err := x509.ParseECPrivateKey(kblock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", keyPath, err)
	}
	return &keyPair{cert: cert, key: key}, nil
}

func writeAtomic(path string, b []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func fingerprint(c *x509.Certificate) string {
	sum := sha256.Sum256(c.Raw)
	return hex.EncodeToString(sum[:8])
}
