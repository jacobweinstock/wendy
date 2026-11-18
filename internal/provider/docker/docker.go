// Package docker wraps the Docker Engine API calls wendy makes.
package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

var (
	once      sync.Once
	cli       *client.Client
	errClient error
)

// Client is the process's Docker client.
func Client() (*client.Client, error) {
	once.Do(func() {
		cli, errClient = client.New(client.FromEnv)
	})
	return cli, errClient
}

// IsNotFound reports whether err means the object does not exist.
func IsNotFound(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "No such") || strings.Contains(err.Error(), "not found"))
}

// EnsureImage pulls ref unless it is already present.
func EnsureImage(ctx context.Context, ref string, log func(string)) error {
	c, err := Client()
	if err != nil {
		return err
	}
	if _, err := c.ImageInspect(ctx, ref); err == nil {
		return nil
	}
	if log != nil {
		log("pulling " + ref)
	}
	resp, err := c.ImagePull(ctx, ref, client.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pulling %s: %w", ref, err)
	}
	defer resp.Close()
	if err := resp.Wait(ctx); err != nil {
		return fmt.Errorf("pulling %s: %w", ref, err)
	}
	return nil
}

// Inspect returns a container, or ok false when there is none.
func Inspect(ctx context.Context, name string) (container.InspectResponse, bool, error) {
	c, err := Client()
	if err != nil {
		return container.InspectResponse{}, false, err
	}
	res, err := c.ContainerInspect(ctx, name, client.ContainerInspectOptions{})
	if IsNotFound(err) {
		return container.InspectResponse{}, false, nil
	}
	if err != nil {
		return container.InspectResponse{}, false, err
	}
	return res.Container, true, nil
}

// Spec is a long-running service container.
type Spec struct {
	Name       string
	Image      string
	Cmd        []string
	Entrypoint []string
	Env        []string
	Binds      []string
	Network    string
	IPv6       string
	Privileged bool
	HostNet    bool
	CapAdd     []string
	// Ports maps a host address ("127.0.0.1:5010") to a container port ("5000/tcp").
	Ports  map[string]string
	Labels map[string]string
	// OneShot containers run to completion and are not restarted.
	OneShot bool
}

// Ensure creates and starts the container unless one by that name exists, in
// which case it is started if stopped.
func Ensure(ctx context.Context, s Spec, log func(string)) error {
	c, err := Client()
	if err != nil {
		return err
	}
	if ins, ok, err := Inspect(ctx, s.Name); err != nil {
		return err
	} else if ok {
		if ins.State != nil && !ins.State.Running {
			_, err := c.ContainerStart(ctx, s.Name, client.ContainerStartOptions{})
			return err
		}
		return nil
	}
	if err := EnsureImage(ctx, s.Image, log); err != nil {
		return err
	}
	hc := &container.HostConfig{Binds: s.Binds, Privileged: s.Privileged, CapAdd: s.CapAdd}
	if !s.OneShot {
		hc.RestartPolicy = container.RestartPolicy{Name: container.RestartPolicyUnlessStopped}
	}
	if s.HostNet {
		hc.NetworkMode = "host"
	}
	cfg := &container.Config{Image: s.Image, Cmd: s.Cmd, Entrypoint: s.Entrypoint, Env: s.Env, Labels: s.Labels}
	if len(s.Ports) > 0 {
		hc.PortBindings = network.PortMap{}
		cfg.ExposedPorts = network.PortSet{}
		for host, ctr := range s.Ports {
			p, err := network.ParsePort(ctr)
			if err != nil {
				return err
			}
			ip, port, _ := strings.Cut(host, ":")
			addr, _ := netip.ParseAddr(ip)
			hc.PortBindings[p] = []network.PortBinding{{HostIP: addr, HostPort: port}}
			cfg.ExposedPorts[p] = struct{}{}
		}
	}
	var nc *network.NetworkingConfig
	if s.Network != "" {
		es := &network.EndpointSettings{}
		if s.IPv6 != "" {
			a, err := netip.ParseAddr(s.IPv6)
			if err != nil {
				return err
			}
			es.IPAMConfig = &network.EndpointIPAMConfig{IPv6Address: a}
		}
		nc = &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{s.Network: es}}
	}
	if _, err := c.ContainerCreate(ctx, client.ContainerCreateOptions{Name: s.Name, Config: cfg, HostConfig: hc, NetworkingConfig: nc}); err != nil {
		return fmt.Errorf("creating %s: %w", s.Name, err)
	}
	if _, err := c.ContainerStart(ctx, s.Name, client.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("starting %s: %w", s.Name, err)
	}
	return nil
}

// Remove force-removes a container; a missing one is not an error.
func Remove(ctx context.Context, name string) error {
	c, err := Client()
	if err != nil {
		return err
	}
	_, err = c.ContainerRemove(ctx, name, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
	if IsNotFound(err) {
		return nil
	}
	return err
}

// Networks are the networks a container is attached to.
func Networks(ctx context.Context, name string) (map[string]*network.EndpointSettings, error) {
	ins, ok, err := Inspect(ctx, name)
	if err != nil || !ok {
		return nil, err
	}
	if ins.NetworkSettings == nil {
		return nil, nil
	}
	return ins.NetworkSettings.Networks, nil
}

// Connect attaches a container to a network unless it already is.
func Connect(ctx context.Context, net, name string) error {
	nets, err := Networks(ctx, name)
	if err != nil {
		return err
	}
	if _, ok := nets[net]; ok {
		return nil
	}
	c, err := Client()
	if err != nil {
		return err
	}
	_, err = c.NetworkConnect(ctx, net, client.NetworkConnectOptions{Container: name})
	return err
}

// Disconnect detaches a container from a network; not attached is fine.
func Disconnect(ctx context.Context, net, name string) error {
	c, err := Client()
	if err != nil {
		return err
	}
	_, err = c.NetworkDisconnect(ctx, net, client.NetworkDisconnectOptions{Container: name, Force: true})
	if err != nil && (IsNotFound(err) || strings.Contains(err.Error(), "is not connected")) {
		return nil
	}
	return err
}

// IP is a container's address on a network, in the given family.
func IP(ctx context.Context, name, net, family string) (string, error) {
	nets, err := Networks(ctx, name)
	if err != nil {
		return "", err
	}
	es, ok := nets[net]
	if !ok || es == nil {
		return "", fmt.Errorf("%s is not on network %s", name, net)
	}
	a := es.IPAddress
	if family == "ipv6" {
		a = es.GlobalIPv6Address
	}
	if !a.IsValid() {
		return "", fmt.Errorf("%s has no %s address on %s", name, family, net)
	}
	return a.String(), nil
}

// Exec runs a command in a running container and returns its combined
// output and exit code.
func Exec(ctx context.Context, name string, cmd ...string) (string, int, error) {
	c, err := Client()
	if err != nil {
		return "", -1, err
	}
	ex, err := c.ExecCreate(ctx, name, client.ExecCreateOptions{Cmd: cmd, AttachStdout: true, AttachStderr: true})
	if err != nil {
		return "", -1, err
	}
	att, err := c.ExecAttach(ctx, ex.ID, client.ExecAttachOptions{})
	if err != nil {
		return "", -1, err
	}
	defer att.Close()
	var out bytes.Buffer
	if _, err := stdcopy.StdCopy(&out, &out, att.Reader); err != nil && !errors.Is(err, io.EOF) {
		return out.String(), -1, err
	}
	ins, err := c.ExecInspect(ctx, ex.ID, client.ExecInspectOptions{})
	if err != nil {
		return out.String(), -1, err
	}
	return out.String(), ins.ExitCode, nil
}

// ExecStream runs a command in a running container and copies its output
// to w until it exits or ctx is done.
func ExecStream(ctx context.Context, name string, w io.Writer, cmd ...string) error {
	c, err := Client()
	if err != nil {
		return err
	}
	ex, err := c.ExecCreate(ctx, name, client.ExecCreateOptions{Cmd: cmd, AttachStdout: true, AttachStderr: true})
	if err != nil {
		return err
	}
	att, err := c.ExecAttach(ctx, ex.ID, client.ExecAttachOptions{})
	if err != nil {
		return err
	}
	defer att.Close()
	done := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(w, io.Discard, att.Reader)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RunOnce runs a command to completion in a throwaway container, as root,
// for host files wendy's own user cannot touch.
func RunOnce(ctx context.Context, image string, binds []string, cmd ...string) (string, error) {
	name := fmt.Sprintf("wendy-once-%d", time.Now().UnixNano())
	if err := Ensure(ctx, Spec{Name: name, Image: image, Binds: binds, Cmd: cmd, OneShot: true}, nil); err != nil {
		return "", err
	}
	defer Remove(context.Background(), name) //nolint:errcheck // A leftover one-shot container is harmless and does not change the result.
	c, err := Client()
	if err != nil {
		return "", err
	}
	res := c.ContainerWait(ctx, name, client.ContainerWaitOptions{})
	select {
	case r := <-res.Result:
		out, _ := Logs(ctx, name, "50")
		if r.StatusCode != 0 {
			return out, fmt.Errorf("%v exited %d: %s", cmd, r.StatusCode, out)
		}
		return out, nil
	case err := <-res.Error:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Logs returns the last lines of a container's log.
func Logs(ctx context.Context, name, tail string) (string, error) {
	c, err := Client()
	if err != nil {
		return "", err
	}
	rc, err := c.ContainerLogs(ctx, name, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Tail: tail, Timestamps: true})
	if err != nil {
		return "", err
	}
	defer rc.Close()
	var out bytes.Buffer
	_, err = stdcopy.StdCopy(&out, &out, rc)
	return out.String(), err
}
