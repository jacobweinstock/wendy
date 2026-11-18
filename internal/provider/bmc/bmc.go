// Package bmc provides virtual BMCs: one shared virtualbmc container (sushy-
// tools for Redfish, vbmc for IPMI) that every combination's network joins.
package bmc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/provider/docker"
	"github.com/tinkerbell/wendy/internal/provider/hardware"
	"github.com/tinkerbell/wendy/internal/provider/hostalloc"
)

// One container serves every combination, so it has one credential pair.
const (
	Container = "wendy-vbmc"
	User      = "root"
	Pass      = "calvin"

	poolName    = "default"
	poolService = "pool"
)

// sushyConf binds '::' so IPv6-only Rufio pods reach it; with bindv6only=0 it
// still accepts IPv4. libvirtd resolves the firmware paths on the host.
const sushyConf = `import os
SUSHY_EMULATOR_LISTEN_IP = u'::'
SUSHY_EMULATOR_LISTEN_PORT = 443
SUSHY_EMULATOR_OS_CLOUD = None
SUSHY_EMULATOR_LIBVIRT_URI = u'qemu:///system'
SUSHY_EMULATOR_IGNORE_BOOT_DEVICE = False
SUSHY_EMULATOR_FEATURE_SET = u'full'
SUSHY_EMULATOR_AUTH_FILE = u'/etc/sushy/htpasswd'
SUSHY_EMULATOR_SSL_CERT = u'/etc/sushy/sushy.cert'
SUSHY_EMULATOR_SSL_KEY = u'/etc/sushy/sushy.key'
_ovmf_code = os.environ.get('OVMF_CODE', u'/usr/share/OVMF/OVMF_CODE.fd')
_ovmf_vars = os.environ.get('OVMF_VARS', u'/usr/share/OVMF/OVMF_VARS.fd')
SUSHY_EMULATOR_BOOT_LOADER_MAP = {
    u'UEFI': {u'x86_64': _ovmf_code},
    u'Legacy': {u'x86_64': None},
}
# sushy-tools rebuilds <loader> and <nvram> whenever it sets UEFI boot mode.
SUSHY_EMULATOR_SECURE_BOOT_DISABLED_NVRAM = _ovmf_vars
SUSHY_EMULATOR_VMEDIA_DEVICES = {
    u'Cd': {u'Name': 'Virtual CD', u'MediaTypes': [u'CD', u'DVD']},
}
`

// Ensure starts the shared container if needed and joins it to net.
func Ensure(ctx context.Context, data, image, net string, fw *config.Firmware, log func(string)) error {
	dir := filepath.Join(data, "vbmc")
	if err := material(dir); err != nil {
		return err
	}
	var env []string
	if fw != nil {
		env = append(env, "OVMF_CODE="+fw.Code, "OVMF_VARS="+fw.Vars)
	}
	if err := docker.Ensure(ctx, docker.Spec{
		Name: Container, Image: image, Privileged: true, Network: net,
		Env: append(env, "SUSHY_EMULATOR_CONFIG=/etc/sushy/sushy-emulator.conf"),
		Binds: []string{
			"/var/run/libvirt:/var/run/libvirt",
			filepath.Join(dir, "sushy.key") + ":/etc/sushy/sushy.key:ro",
			filepath.Join(dir, "sushy.cert") + ":/etc/sushy/sushy.cert:ro",
			filepath.Join(dir, "htpasswd") + ":/etc/sushy/htpasswd:ro",
			filepath.Join(dir, "sushy-emulator.conf") + ":/etc/sushy/sushy-emulator.conf:ro",
		},
		Labels: map[string]string{"wendy.shared": "vbmc"},
	}, log); err != nil {
		return err
	}
	return docker.Connect(ctx, net, Container)
}

// material writes the credentials, TLS key pair and config the container
// mounts. They are shared by every combination, so they are made once.
func material(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "sushy-emulator.conf"), []byte(sushyConf), 0o644); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, "htpasswd")); err != nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(Pass), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "htpasswd"), []byte(User+":"+string(hash)+"\n"), 0o644); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "sushy.cert")); err == nil {
		return nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "wendy-vbmc", Organization: []string{"wendy"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	kb, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "sushy.key"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "sushy.cert"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}

// Leave detaches net from the shared container, and removes the container
// once no combination's network is attached.
func Leave(ctx context.Context, net string) error {
	if err := docker.Disconnect(ctx, net, Container); err != nil {
		return err
	}
	nets, err := docker.Networks(ctx, Container)
	if err != nil {
		return err
	}
	for n := range nets {
		if strings.HasPrefix(n, "wendy-") {
			return nil
		}
	}
	return docker.Remove(ctx, Container)
}

// Host is the container's address on a network.
func Host(ctx context.Context, net, family string) (string, error) {
	return docker.IP(ctx, Container, net, family)
}

// Ports allocates a vBMC port for each machine.
func Ports(data, envID string, names []string) (map[string]int, error) {
	out := map[string]int{}
	err := hostalloc.Update(data, func(h *hostalloc.Host) error {
		for _, n := range names {
			out[n] = h.BMCPort(envID, n)
		}
		return nil
	})
	return out, err
}

// Register adds a machine to vbmc on its port and starts it. vbmc exits 0
// even when it cannot bind, so the listed status is what is checked.
func Register(ctx context.Context, name string, port int) error {
	status, registered := listed(ctx, name)
	if status != "" && (status == "error" || registered != fmt.Sprint(port)) {
		_, _, _ = docker.Exec(ctx, Container, "vbmc", "stop", name)
		_, _, _ = docker.Exec(ctx, Container, "vbmc", "delete", name)
		status = ""
	}
	if status == "" {
		if out, code, err := docker.Exec(ctx, Container, "vbmc", "add", "--username", User, "--password", Pass, "--port", fmt.Sprint(port), name); err != nil || code != 0 {
			return fmt.Errorf("vbmc add %s: %v %s", name, err, out)
		}
	}
	if out, code, err := docker.Exec(ctx, Container, "vbmc", "start", name); err != nil || code != 0 {
		if !strings.Contains(out, "already running") {
			return fmt.Errorf("vbmc start %s: %v %s", name, err, out)
		}
	}
	if st, _ := listed(ctx, name); st != "running" {
		logs, _ := docker.Logs(ctx, Container, "20")
		return fmt.Errorf("vbmc for %s is %q on port %d, not running:\n%s", name, st, port, logs)
	}
	return nil
}

// listed is a machine's status and port in `vbmc list`.
func listed(ctx context.Context, name string) (status, port string) {
	out, _, err := docker.Exec(ctx, Container, "vbmc", "list")
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "|")
		if len(f) < 5 {
			continue
		}
		if strings.TrimSpace(f[1]) == name {
			return strings.TrimSpace(f[2]), strings.TrimSpace(f[4])
		}
	}
	return "", ""
}

// Unregister removes a machine from vbmc.
func Unregister(ctx context.Context, name string) error {
	if st, _ := listed(ctx, name); st == "" {
		return nil
	}
	_, _, _ = docker.Exec(ctx, Container, "vbmc", "stop", name) //nolint:dogsled // A machine that is not running fails to stop and is deleted all the same.
	_, _, err := docker.Exec(ctx, Container, "vbmc", "delete", name)
	return err
}

// EnsurePool makes sure libvirt has the storage pool sushy-tools attaches
// virtual media from. Its name is fixed by sushy-tools, and it is libvirt's
// conventional pool, so wendy only removes one it created itself.
func EnsurePool(data, uri, envID string) error {
	l, err := hardware.Connect(uri)
	if err != nil {
		return err
	}
	defer l.Disconnect() //nolint:errcheck // Nothing useful can be done when closing the connection fails.
	return hostalloc.Update(data, func(h *hostalloc.Host) error {
		h.AddUser(poolService, envID)
		if _, err := l.StoragePoolLookupByName(poolName); err == nil {
			return nil
		}
		dir := "/tmp/wendy-iso"
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		p, err := l.StoragePoolDefineXML(fmt.Sprintf(`<pool type="dir"><name>%s</name><target><path>%s</path></target></pool>`, poolName, dir), 0)
		if err != nil {
			return fmt.Errorf("defining the %s pool: %w", poolName, err)
		}
		if err := l.StoragePoolCreate(p, 0); err != nil {
			return fmt.Errorf("starting the %s pool: %w", poolName, err)
		}
		h.PoolCreated = true
		return nil
	})
}

func releasePool(data, uri, envID string) error {
	return hostalloc.Update(data, func(h *hostalloc.Host) error {
		if h.RemoveUser(poolService, envID) || !h.PoolCreated {
			return nil
		}
		l, err := hardware.Connect(uri)
		if err != nil {
			return err
		}
		defer l.Disconnect() //nolint:errcheck // Nothing useful can be done when closing the connection fails.
		if p, err := l.StoragePoolLookupByName(poolName); err == nil {
			_ = l.StoragePoolDestroy(p)
			if err := l.StoragePoolUndefine(p); err != nil {
				return err
			}
		}
		h.PoolCreated = false
		return nil
	})
}

func init() {
	engine.RegisterTeardown("vbmc-net", func(ctx context.Context, _ *engine.Testbed, r engine.Resource) error {
		return Leave(ctx, r.Name)
	})
	engine.RegisterTeardown("vbmc-machine", func(ctx context.Context, _ *engine.Testbed, r engine.Resource) error {
		return Unregister(ctx, r.Name)
	})
	engine.RegisterTeardown("bmc-ports", func(_ context.Context, tb *engine.Testbed, r engine.Resource) error {
		return hostalloc.Update(tb.DataDir(), func(h *hostalloc.Host) error {
			h.ReleaseBMCPorts(r.Name)
			return nil
		})
	})
	engine.RegisterTeardown("pool", func(_ context.Context, tb *engine.Testbed, r engine.Resource) error {
		err := releasePool(tb.DataDir(), r.Attrs["uri"], r.Name)
		if err != nil && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	})
}
