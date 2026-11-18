package registry

import (
	"context"
	"strings"

	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/provider/docker"
)

// mirrorImage serves each pull-through cache.
const mirrorImage = "docker.io/library/registry:2"

// MirrorName is the container caching an upstream.
func MirrorName(upstream string) string {
	return "wendy-mirror-" + strings.NewReplacer(".", "-", ":", "-").Replace(upstream)
}

// EnsureMirror starts the pull-through cache of an upstream, shared by every
// combination, and joins it to net. It returns the address machines and
// nodes on net pull from.
func EnsureMirror(ctx context.Context, upstream, net, family string, log func(string)) (string, error) {
	remote := "https://" + upstream
	if upstream == "docker.io" {
		remote = "https://registry-1.docker.io"
	}
	name := MirrorName(upstream)
	if err := docker.Ensure(ctx, docker.Spec{
		Name: name, Image: mirrorImage,
		Env:    []string{"REGISTRY_PROXY_REMOTEURL=" + remote},
		Binds:  []string{name + ":/var/lib/registry"},
		Labels: map[string]string{"wendy.shared": "mirror"},
	}, log); err != nil {
		return "", err
	}
	if err := docker.Connect(ctx, net, name); err != nil {
		return "", err
	}
	ip, err := docker.IP(ctx, name, net, family)
	if err != nil {
		return "", err
	}
	if family == "ipv6" {
		return "[" + ip + "]:5000", nil
	}
	return ip + ":5000", nil
}

// LeaveMirror detaches a cache from net, removing it once no combination
// uses it. Its cache stays in a volume of the same name.
func LeaveMirror(ctx context.Context, name, net string) error {
	if err := docker.Disconnect(ctx, net, name); err != nil {
		return err
	}
	nets, err := docker.Networks(ctx, name)
	if err != nil {
		return err
	}
	for n := range nets {
		if strings.HasPrefix(n, "wendy-") {
			return nil
		}
	}
	return docker.Remove(ctx, name)
}

func init() {
	engine.RegisterTeardown("mirror-net", func(ctx context.Context, _ *engine.Testbed, r engine.Resource) error {
		return LeaveMirror(ctx, r.Name, r.Attrs["network"])
	})
}
