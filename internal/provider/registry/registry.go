// Package registry provides the local registry that carries source-built
// images. One registry serves every combination: pushes go to a loopback
// port that Docker and buildkit treat as insecure without configuration, and
// pulls use its address on each combination's network, the only one the kind
// nodes and the machines can reach. The repository path is the same either
// way, so nothing is retagged.
package registry

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/provider/docker"
)

const (
	Container = "wendy-registry"
	Image     = "docker.io/library/registry:3"
	// PushEndpoint is where builds push.
	PushEndpoint = "127.0.0.1:5020"
	// Images outlive the container in this volume, so a clean commit is
	// never rebuilt.
	volume = "wendy-registry-data"
	// Builder is the buildx builder for pushes: on the host network, so
	// 127.0.0.1 is the host and not the builder's own container.
	Builder = "wendy"
)

// Ensure starts the registry and waits until it answers.
func Ensure(ctx context.Context, log func(string)) error {
	if err := docker.Ensure(ctx, docker.Spec{
		Name: Container, Image: Image,
		Ports:  map[string]string{PushEndpoint: "5000/tcp"},
		Binds:  []string{volume + ":/var/lib/registry"},
		Labels: map[string]string{"wendy.shared": "registry"},
	}, log); err != nil {
		return err
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+PushEndpoint+"/v2/", http.NoBody)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("registry at %s did not answer: %w", PushEndpoint, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// EnsureBuilder creates the buildx builder pushes go through.
func EnsureBuilder(ctx context.Context) error {
	if exec.CommandContext(ctx, "docker", "buildx", "inspect", Builder).Run() == nil {
		return nil
	}
	out, err := exec.CommandContext(ctx, "docker", "buildx", "create", "--name", Builder,
		"--driver", "docker-container", "--driver-opt", "network=host").CombinedOutput()
	if err != nil {
		return fmt.Errorf("creating buildx builder %s: %v: %s", Builder, err, out)
	}
	return nil
}

// Join attaches the registry to a network and returns its pull address there.
func Join(ctx context.Context, net, family string) (string, error) {
	if err := docker.Connect(ctx, net, Container); err != nil {
		return "", err
	}
	ip, err := docker.IP(ctx, Container, net, family)
	if err != nil {
		return "", err
	}
	if family == "ipv6" {
		return "[" + ip + "]:5000", nil
	}
	return ip + ":5000", nil
}

// Leave detaches the registry from a network and stops it once no
// combination uses it. Its images stay in the volume.
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

// HostsTOML is the containerd config for pulling from a plain-HTTP registry.
func HostsTOML(addr string) string {
	return fmt.Sprintf("server = \"http://%s\"\n\n[host.\"http://%s\"]\n  capabilities = [\"pull\", \"resolve\"]\n", addr, addr)
}

// Has reports whether the registry already holds repo:tag.
func Has(ctx context.Context, repo, tag string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, fmt.Sprintf("http://%s/v2/%s/manifests/%s", PushEndpoint, repo, tag), nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func init() {
	engine.RegisterTeardown("registry-net", func(ctx context.Context, _ *engine.Testbed, r engine.Resource) error {
		return Leave(ctx, r.Name)
	})
}
