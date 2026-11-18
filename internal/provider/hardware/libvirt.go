// Package hardware provides the machines Tinkerbell provisions. v1 has
// libvirt.
package hardware

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/digitalocean/go-libvirt/socket/dialers"
	"libvirt.org/go/libvirtxml"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/provider/docker"
)

// Machine is what a backend needs to build one machine.
type Machine struct {
	engine.Machine
	CPUs    int
	MemMiB  int
	DiskGiB int
}

// Hardware creates and destroys machines.
type Hardware interface {
	Create(ctx context.Context, tb *engine.Testbed, ms []Machine) error
	Delete(ctx context.Context, name string) error
}

// For returns the backend of an environment.
func For(b config.Backend) (Hardware, error) {
	if b.Type == "libvirt" {
		return Libvirt{Backend: b}, nil
	}
	return nil, fmt.Errorf("unsupported hardware backend %q", b.Type)
}

// Libvirt builds machines as libvirt domains on the environment's bridge.
type Libvirt struct{ Backend config.Backend }

// Connect opens a libvirt connection for uri.
func Connect(uri string) (*libvirt.Libvirt, error) {
	l := libvirt.NewWithDialer(dialers.NewLocal(dialers.WithLocalTimeout(10 * time.Second)))
	if uri == "" {
		uri = string(libvirt.QEMUSystem)
	}
	if err := l.ConnectToURI(libvirt.ConnectURI(uri)); err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", uri, err)
	}
	return l, nil
}

func (h Libvirt) Create(ctx context.Context, tb *engine.Testbed, ms []Machine) error {
	l, err := Connect(h.Backend.URI)
	if err != nil {
		return err
	}
	defer l.Disconnect() //nolint:errcheck // Nothing useful can be done when closing the connection fails.
	bridge := tb.State().Network.Bridge
	v6 := tb.Config.Network.Family == "ipv6"
	for _, m := range ms {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Tracked first, so a half-built machine is still torn down.
		if err := tb.Track(engine.Resource{Kind: "vm", Name: m.Name, Attrs: map[string]string{"uri": h.Backend.URI, "diskPath": h.Backend.DiskPath}}); err != nil {
			return err
		}
		if _, err := l.DomainLookupByName(m.Name); err == nil {
			continue
		}
		disk := filepath.Join(h.Backend.DiskPath, m.Name+"-disk.img")
		if err := sparse(disk, int64(m.DiskGiB)<<30); err != nil {
			return err
		}
		x, err := domainXML(m, disk, Console(h.Backend.DiskPath, m.Name), bridge, h.Backend.Firmware, v6)
		if err != nil {
			return err
		}
		if _, err := l.DomainDefineXML(x); err != nil {
			return fmt.Errorf("defining %s: %w", m.Name, err)
		}
	}
	return nil
}

// Console is where a machine's serial console is written. It sits beside
// the disks: qemu runs as its own user and cannot write under $HOME.
func Console(diskPath, name string) string {
	return filepath.Join(diskPath, name+"-console.log")
}

func sparse(path string, size int64) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	if err := f.Truncate(size); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func domainXML(m Machine, disk, console, bridge string, fw *config.Firmware, v6 bool) (string, error) {
	native := m.Arch == runtime.GOARCH
	arch, machine := "x86_64", "q35"
	if m.Arch == "arm64" {
		arch, machine = "aarch64", "virt"
	}
	d := &libvirtxml.Domain{
		Type:        "kvm",
		Name:        m.Name,
		Description: "wendy machine",
		Memory:      &libvirtxml.DomainMemory{Value: uint(m.MemMiB), Unit: "MiB"},
		VCPU:        &libvirtxml.DomainVCPU{Value: uint(m.CPUs)},
		OS: &libvirtxml.DomainOS{
			Type:        &libvirtxml.DomainOSType{Arch: arch, Machine: machine, Type: "hvm"},
			BootDevices: []libvirtxml.DomainBootDevice{{Dev: "hd"}, {Dev: "network"}},
		},
		Features: &libvirtxml.DomainFeatureList{ACPI: &libvirtxml.DomainFeature{}},
		CPU:      &libvirtxml.DomainCPU{Mode: "host-passthrough"},
		Devices: &libvirtxml.DomainDeviceList{
			Disks: []libvirtxml.DomainDisk{{
				Device: "disk",
				// Zero writes stay holes, so a streamed image costs only
				// its data on the host.
				Driver: &libvirtxml.DomainDiskDriver{Name: "qemu", Type: "raw", Discard: "unmap", DetectZeros: "unmap"},
				Source: &libvirtxml.DomainDiskSource{File: &libvirtxml.DomainDiskSourceFile{File: disk}},
				Target: &libvirtxml.DomainDiskTarget{Dev: "vda", Bus: "virtio"},
			}},
			Interfaces: []libvirtxml.DomainInterface{{
				Source: &libvirtxml.DomainInterfaceSource{Bridge: &libvirtxml.DomainInterfaceSourceBridge{Bridge: bridge}},
				MAC:    &libvirtxml.DomainInterfaceMAC{Address: m.MAC},
				Model:  &libvirtxml.DomainInterfaceModel{Type: "virtio"},
			}},
			Serials: []libvirtxml.DomainSerial{{
				Source: &libvirtxml.DomainChardevSource{File: &libvirtxml.DomainChardevSourceFile{Path: console}},
				Target: &libvirtxml.DomainSerialTarget{Port: new(uint)},
			}},
			Consoles: []libvirtxml.DomainConsole{{
				Source: &libvirtxml.DomainChardevSource{File: &libvirtxml.DomainChardevSourceFile{Path: console}},
				Target: &libvirtxml.DomainConsoleTarget{Type: "serial"},
			}},
			Graphics: []libvirtxml.DomainGraphic{{VNC: &libvirtxml.DomainGraphicVNC{AutoPort: "yes", Listen: "127.0.0.1"}}},
		},
	}
	if !native {
		// Another architecture runs under TCG, slowly.
		d.Type = "qemu"
		d.CPU = &libvirtxml.DomainCPU{Mode: "custom", Model: &libvirtxml.DomainCPUModel{Value: "cortex-a57"}}
		if m.Arch == "amd64" {
			d.CPU.Model.Value = "qemu64"
		}
	}
	if m.Arch == "amd64" {
		d.Features.APIC = &libvirtxml.DomainFeatureAPIC{}
	}
	if fw != nil {
		d.OS.Loader = &libvirtxml.DomainLoader{Path: fw.Code, Readonly: "yes", Type: "pflash", Secure: "no"}
		d.OS.NVRam = &libvirtxml.DomainNVRam{Template: fw.Vars}
	} else {
		// Firmware able to do secure boot, without enrolled keys, so nothing
		// is actually enforced.
		d.OS.Firmware = "efi"
		d.OS.FirmwareInfo = &libvirtxml.DomainOSFirmwareInfo{Features: []libvirtxml.DomainOSFirmwareFeature{
			{Enabled: "no", Name: "enrolled-keys"},
			{Enabled: "yes", Name: "secure-boot"},
		}}
		if m.Arch == "amd64" {
			d.Features.SMM = &libvirtxml.DomainFeatureSMM{State: "on"}
		}
	}
	if v6 && m.Arch == "amd64" {
		// OVMF tries IPv4 PXE first; on IPv6 that is minutes of retries.
		d.QEMUCommandline = &libvirtxml.DomainQEMUCommandline{Args: []libvirtxml.DomainQEMUCommandlineArg{
			{Value: "-fw_cfg"}, {Value: "name=opt/org.tianocore/IPv4PXESupport,string=false"},
		}}
	}
	return d.Marshal()
}

func (h Libvirt) Delete(_ context.Context, name string) error {
	l, err := Connect(h.Backend.URI)
	if err != nil {
		return err
	}
	defer l.Disconnect() //nolint:errcheck // Nothing useful can be done when closing the connection fails.
	return deleteDomain(l, name, h.Backend.DiskPath)
}

func deleteDomain(l *libvirt.Libvirt, name, diskPath string) error {
	dom, err := l.DomainLookupByName(name)
	if err == nil {
		if state, _, err := l.DomainGetState(dom, 0); err == nil && libvirt.DomainState(state) != libvirt.DomainShutoff {
			_ = l.DomainDestroy(dom)
		}
		if err := l.DomainUndefineFlags(dom, libvirt.DomainUndefineNvram|libvirt.DomainUndefineManagedSave); err != nil {
			return fmt.Errorf("undefining %s: %w", name, err)
		}
	} else if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "no domain") {
		return err
	}
	var errs []error
	for _, f := range []string{filepath.Join(diskPath, name+"-disk.img"), Console(diskPath, name)} {
		err := os.Remove(f)
		if errors.Is(err, os.ErrPermission) {
			// libvirt leaves consoles root's, and a disk qemu's, in a
			// sticky directory.
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			_, err = docker.RunOnce(ctx, helperImage, []string{diskPath + ":/d"}, "rm", "-f", "/d/"+filepath.Base(f))
			cancel()
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// PowerOff forces a machine off, as pulling its plug would.
func PowerOff(uri, name string) error {
	l, err := Connect(uri)
	if err != nil {
		return err
	}
	defer l.Disconnect() //nolint:errcheck // Nothing useful can be done when closing the connection fails.
	dom, err := l.DomainLookupByName(name)
	if err != nil {
		return err
	}
	return l.DomainDestroy(dom)
}

// helperImage reads and removes host files wendy's user cannot.
const helperImage = "docker.io/library/alpine:3.22"

// StreamConsoles copies each machine's serial console into dir until ctx is
// done, so a run keeps every console even after the machine is deleted.
// libvirt writes consoles as root with mode 0600, so they are read through
// a helper container, which is torn down with the environment.
func StreamConsoles(ctx context.Context, tb *engine.Testbed, diskPath, dir string, names []string, log func(string)) error {
	name := tb.HostName("consoles")
	if err := tb.Track(engine.Resource{Kind: "container", Name: name}); err != nil {
		return err
	}
	if err := docker.Ensure(ctx, docker.Spec{
		Name: name, Image: helperImage, Binds: []string{diskPath + ":/disks:ro"}, Cmd: []string{"sleep", "infinity"},
	}, log); err != nil {
		return err
	}
	for _, n := range names {
		go streamConsole(ctx, name, "/disks/"+n+"-console.log", filepath.Join(dir, n+".log"))
	}
	return nil
}

func streamConsole(ctx context.Context, container, src, dst string) {
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer out.Close()
	cw := &countWriter{w: out}
	for ctx.Err() == nil {
		// tail -F waits for a file that does not exist yet and follows it
		// across truncation, which a machine power cycle can cause.
		_ = docker.ExecStream(ctx, container, cw, "tail", "-c", fmt.Sprintf("+%d", cw.n+1), "-F", src)
		select {
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

func init() {
	engine.RegisterTeardown("vm", func(_ context.Context, _ *engine.Testbed, r engine.Resource) error {
		l, err := Connect(r.Attrs["uri"])
		if err != nil {
			return err
		}
		defer l.Disconnect() //nolint:errcheck // Nothing useful can be done when closing the connection fails.
		return deleteDomain(l, r.Name, r.Attrs["diskPath"])
	})
}
