// Package hostcheck reports whether this host can run what a plan needs.
package hostcheck

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// Check is one pre-flight check.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
	// Optional checks only matter for some runs, e.g. git for source builds.
	Optional bool `json:"optional,omitempty"`
}

// Options says what to check beyond the basics.
type Options struct {
	// Sources adds git, make, go and docker buildx.
	Sources bool
	// DiskGiB is how much free space DiskPaths need.
	DiskGiB   int
	DiskPaths []string
}

// Run performs the checks. Without Sources, the build tools are still
// reported, but as optional.
func Run(ctx context.Context, o Options) []Check {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	checks := []Check{docker(ctx), libvirt(ctx), kvm()}
	for _, p := range o.DiskPaths {
		checks = append(checks, disk(p, o.DiskGiB))
	}
	for _, bin := range []string{"git", "make", "go"} {
		c := Check{Name: bin, Optional: !o.Sources}
		if _, err := exec.LookPath(bin); err != nil {
			c.Detail = "only for source builds"
			if o.Sources {
				c.Detail = "needed to build from source"
			}
		} else {
			c.OK = true
		}
		checks = append(checks, c)
	}
	if o.Sources {
		c := Check{Name: "docker buildx"}
		if err := exec.CommandContext(ctx, "docker", "buildx", "version").Run(); err != nil {
			c.Detail = "needed to build tink-agent"
		} else {
			c.OK = true
		}
		checks = append(checks, c)
	}
	return checks
}

// Failed is the checks a run cannot do without.
func Failed(checks []Check) []Check {
	var out []Check
	for _, c := range checks {
		if !c.OK && !c.Optional {
			out = append(out, c)
		}
	}
	return out
}

func docker(ctx context.Context) Check {
	c := Check{Name: "docker"}
	sock := "/var/run/docker.sock"
	if h := os.Getenv("DOCKER_HOST"); len(h) > 7 && h[:7] == "unix://" {
		sock = h[7:]
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", sock)
	if err != nil {
		c.Detail = fmt.Sprintf("cannot reach %s: %v", sock, err)
		return c
	}
	conn.Close()
	c.OK = true
	return c
}

func libvirt(ctx context.Context) Check {
	c := Check{Name: "libvirt", Detail: "qemu:///system"}
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "unix", "/var/run/libvirt/libvirt-sock")
	if err != nil {
		c.Detail = "cannot reach libvirtd: " + err.Error()
		return c
	}
	conn.Close()
	c.OK = true
	return c
}

func kvm() Check {
	c := Check{Name: "/dev/kvm"}
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		c.Detail = err.Error()
		return c
	}
	f.Close()
	c.OK = true
	return c
}

func disk(path string, needGiB int) Check {
	// Machine disks are sparse, so the need is a worst case: a warning.
	c := Check{Name: "disk space", Detail: path, Optional: true}
	for p := path; ; p = filepath.Dir(p) {
		var st syscall.Statfs_t
		if err := syscall.Statfs(p, &st); err == nil && st.Bsize > 0 {
			free := int(st.Bavail * uint64(st.Bsize) >> 30)
			c.OK = free >= needGiB
			c.Detail = fmt.Sprintf("%s: %d GiB free", path, free)
			if !c.OK {
				c.Detail += fmt.Sprintf(", %d GiB needed", needGiB)
			}
			return c
		}
		if p == "/" || p == "." {
			c.Detail = "cannot stat " + path
			return c
		}
	}
}
