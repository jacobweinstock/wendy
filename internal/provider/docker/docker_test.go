package docker

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"
)

// service starts an alpine container that runs script on the default bridge,
// restarted unless oneShot, and removes it when the test ends.
func service(ctx context.Context, t *testing.T, script string, oneShot bool) string {
	t.Helper()
	if _, ok, err := Inspect(ctx, "wendy-no-such-container"); err != nil || ok {
		t.Skipf("docker unavailable: %v", err)
	}
	if err := EnsureImage(ctx, "alpine:3.22", nil); err != nil {
		t.Skipf("no alpine image: %v", err)
	}
	name := fmt.Sprintf("wendy-test-%d", time.Now().UnixNano())
	t.Cleanup(func() { _ = Remove(context.Background(), name) })
	spec := Spec{Name: name, Image: "alpine:3.22", Entrypoint: []string{"sh", "-c"}, Cmd: []string{script}, OneShot: oneShot}
	if err := Ensure(ctx, spec, nil); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestWaitIPWaitsForTheContainerToStart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	name := service(ctx, t, "exec sleep 300", false)
	c, err := Client()
	if err != nil {
		t.Fatal(err)
	}
	// A stopped container has no address, as one Docker is restarting has
	// none.
	now := 0
	if _, err := c.ContainerStop(ctx, name, client.ContainerStopOptions{Timeout: &now}); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(time.Second)
		_, _ = c.ContainerStart(ctx, name, client.ContainerStartOptions{})
	}()
	if _, err := WaitIP(ctx, name, "bridge", "ipv4", 20*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestWaitIPReportsAContainerThatStaysDown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	name := service(ctx, t, "echo cannot reach the upstream; exit 3", true)
	c, err := Client()
	if err != nil {
		t.Fatal(err)
	}
	res := c.ContainerWait(ctx, name, client.ContainerWaitOptions{})
	select {
	case <-res.Result:
	case err := <-res.Error:
		t.Fatal(err)
	}
	_, err = WaitIP(ctx, name, "bridge", "ipv4", time.Second)
	if err == nil {
		t.Fatal("got an address for a container that exited")
	}
	for _, want := range []string{"exited", "exit code 3", "cannot reach the upstream"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not say %q", err, want)
		}
	}
}
