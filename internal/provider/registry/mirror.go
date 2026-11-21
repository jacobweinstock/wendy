package registry

import (
	"context"
	"strings"
	"time"

	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/provider/docker"
)

const (
	// mirrorImage serves each pull-through cache.
	mirrorImage = "docker.io/library/registry:2"
	mirrorPort  = "5000"
	// caLabel records the CA a mirror's certificate chains to.
	caLabel = "wendy.mirror.ca"
	// mirrorStartTimeout bounds the wait for a cache to come up. A cache asks
	// its upstream for auth challenges as it starts and exits if it cannot
	// reach it, so a brief network hiccup costs a restart or two.
	mirrorStartTimeout = 30 * time.Second
)

// MirrorName is the container caching an upstream, and the DNS name its
// certificate is for.
func MirrorName(upstream string) string {
	return "wendy-mirror-" + strings.NewReplacer(".", "-", ":", "-").Replace(upstream)
}

// MirrorHost is a mirror's host:port by name, which a client verifies the
// mirror's certificate against. The name resolves only where something maps
// it to the mirror's address on the combination's network: Docker's DNS in
// kind nodes, /etc/hosts on machines.
func MirrorHost(upstream string) string {
	return MirrorName(upstream) + ":" + mirrorPort
}

// EnsureMirror starts the pull-through cache of an upstream, shared by every
// combination, and joins it to net. It returns the cache's address on net.
// The cache serves TLS only, with a certificate for MirrorName signed by
// MirrorCA.
func EnsureMirror(ctx context.Context, data, upstream, net, family string, log func(string)) (string, error) {
	remote := "https://" + upstream
	if upstream == "docker.io" {
		remote = "https://registry-1.docker.io"
	}
	name := MirrorName(upstream)
	certs, ca, err := mirrorCerts(data, name)
	if err != nil {
		return "", err
	}
	if ins, ok, err := docker.Inspect(ctx, name); err != nil {
		return "", err
	} else if ok && (ins.Config == nil || ins.Config.Labels[caLabel] != ca) {
		// A cache from a wendy that served plain HTTP, or from an earlier CA,
		// would serve a certificate nothing trusts.
		log("recreating " + name + " for the current mirror CA; its cache is kept")
		if err := docker.Remove(ctx, name); err != nil {
			return "", err
		}
	}
	if err := docker.Ensure(ctx, docker.Spec{
		Name: name, Image: mirrorImage,
		Env: []string{
			"REGISTRY_PROXY_REMOTEURL=" + remote,
			"REGISTRY_HTTP_TLS_CERTIFICATE=/certs/tls.crt",
			"REGISTRY_HTTP_TLS_KEY=/certs/tls.key",
		},
		Binds:  []string{name + ":/var/lib/registry", certs + ":/certs:ro"},
		Labels: map[string]string{"wendy.shared": "mirror", caLabel: ca},
	}, log); err != nil {
		return "", err
	}
	if err := docker.Connect(ctx, net, name); err != nil {
		return "", err
	}
	ip, err := docker.WaitIP(ctx, name, net, family, mirrorStartTimeout)
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
