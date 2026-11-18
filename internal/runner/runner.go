// Package runner executes out-of-process steps: commands and containers,
// under wendy's BYO contract.
package runner

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"

	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/provider/docker"
)

// Contract is the BYO contract version.
const Contract = "1"

// exitSkipped is the exit code that means "skipped".
const exitSkipped = 77

// Runners returns the exec and container runners for the engine.
func Runners() map[string]func(context.Context, *engine.Step) error {
	return map[string]func(context.Context, *engine.Step) error{"exec": Exec, "container": Container}
}

// Exec runs a command on the host.
func Exec(ctx context.Context, s *engine.Step) error {
	cfg := s.Config.Exec
	bundle, err := prepareBundle(s)
	if err != nil {
		return err
	}
	env := contractEnv(s, bundle, s.Artifacts, s.Work)
	for k, v := range cfg.Env {
		env[k] = v
	}
	args := make([]string, len(cfg.Command))
	for i, a := range cfg.Command {
		args[i] = os.Expand(a, func(k string) string {
			if v, ok := env[k]; ok {
				return v
			}
			return os.Getenv(k)
		})
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = s.Work
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 10 * time.Second
	stdout := engine.LineWriter(func(l string) { s.Log.Info(l) })
	stderr := engine.LineWriter(func(l string) { s.Log.Info(l, "stream", "stderr") })
	cmd.Stdout, cmd.Stderr = stdout, stderr

	s.Log.Info("running " + strings.Join(args, " "))
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", args[0], err)
	}
	err = cmd.Wait()
	stdout.Close()
	stderr.Close()
	ingestJUnit(s)

	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &exitErr):
		return exitVerdict(exitErr.ExitCode())
	default:
		return err
	}
}

// Container runs an image on the testbed's network.
func Container(ctx context.Context, s *engine.Step) error {
	cfg := s.Config.Container
	c, err := docker.Client()
	if err != nil {
		return err
	}
	if err := docker.EnsureImage(ctx, cfg.Image, func(m string) { s.Log.Info(m) }); err != nil {
		return err
	}
	bundle, err := prepareBundle(s)
	if err != nil {
		return err
	}
	env := contractEnv(s, "/wendy/bundle", "/wendy/artifacts", "/wendy/work")
	for k, v := range cfg.Env {
		env[k] = v
	}
	var envList []string
	for k, v := range env {
		envList = append(envList, k+"="+v)
	}
	binds := []string{bundle + ":/wendy/bundle:ro", s.Artifacts + ":/wendy/artifacts"}
	if s.Work != "" {
		binds = append(binds, s.Work+":/wendy/work:ro")
	}
	hc := &container.HostConfig{Binds: binds}
	var nc *network.NetworkingConfig
	if s.Testbed != nil {
		if net := s.Testbed.State().Network.Name; net != "" {
			hc.NetworkMode = container.NetworkMode(net)
		}
	}
	name := "wendy-step-" + filepath.Base(s.Dir) + "-" + fmt.Sprint(time.Now().UnixNano()%1e6)
	created, err := c.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: name,
		Config: &container.Config{
			Image: cfg.Image, Cmd: cfg.Args, Entrypoint: cfg.Command, Env: envList,
			User: fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		},
		HostConfig: hc, NetworkingConfig: nc,
	})
	if err != nil {
		return fmt.Errorf("creating the container: %w", err)
	}
	defer func() {
		rm, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		_ = docker.Remove(rm, created.ID)
	}()
	wait := c.ContainerWait(ctx, created.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNextExit})
	if _, err := c.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("starting the container: %w", err)
	}
	s.Log.Info("started " + cfg.Image)

	logs, err := c.ContainerLogs(ctx, created.ID, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: true})
	if err == nil {
		stdout := engine.LineWriter(func(l string) { s.Log.Info(l) })
		stderr := engine.LineWriter(func(l string) { s.Log.Info(l, "stream", "stderr") })
		go func() {
			defer logs.Close()
			_, _ = stdcopy.StdCopy(stdout, stderr, logs)
			stdout.Close()
			stderr.Close()
		}()
	}

	select {
	case res := <-wait.Result:
		ingestJUnit(s)
		if res.Error != nil {
			return errors.New(res.Error.Message)
		}
		if res.StatusCode == 0 {
			return nil
		}
		return exitVerdict(int(res.StatusCode))
	case err := <-wait.Error:
		return err
	case <-ctx.Done():
		stop, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = c.ContainerStop(stop, created.ID, client.ContainerStopOptions{})
		ingestJUnit(s)
		return ctx.Err()
	}
}

func exitVerdict(code int) error {
	if code == exitSkipped {
		return engine.Skip("exit 77")
	}
	return engine.Failf("exit status %d", code)
}

// contractEnv is the BYO contract's environment variables, with paths as the
// step will see them.
func contractEnv(s *engine.Step, bundle, artifacts, work string) map[string]string {
	env := map[string]string{
		"WENDY_CONTRACT":      Contract,
		"WENDY_BUNDLE":        bundle,
		"WENDY_ARTIFACTS_DIR": artifacts,
		"WENDY_WORK":          work,
		"KUBECONFIG":          filepath.Join(bundle, "kubeconfigs", engine.WorkloadCluster),
	}
	if s.Testbed != nil {
		env["WENDY_LABELS"] = strings.Join(s.Testbed.Labels, ",")
	}
	return env
}

// prepareBundle snapshots what the step may read: kubeconfigs, state.json
// and its own params. Copies rather than links, so a container sees them.
func prepareBundle(s *engine.Step) (string, error) {
	dir := filepath.Join(s.Dir, "bundle")
	kc := filepath.Join(dir, "kubeconfigs")
	if err := os.MkdirAll(kc, 0o755); err != nil {
		return "", err
	}
	params, _ := json.MarshalIndent(s.Config, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "params.json"), params, 0o644); err != nil {
		return "", err
	}
	if s.Testbed == nil {
		return dir, nil
	}
	st := s.Testbed.State()
	st.Resources = nil
	b, _ := json.MarshalIndent(st, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "state.json"), b, 0o644); err != nil {
		return "", err
	}
	for _, cluster := range []string{engine.ManagementCluster, engine.TinkerbellCluster, engine.WorkloadCluster} {
		if err := copyFile(s.Testbed.Kubeconfig(cluster), filepath.Join(kc, cluster)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return dir, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	// Readable by a container user other than ours.
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// ingestJUnit reports every test case in the artifacts' JUnit files as a
// sub-result of the step.
func ingestJUnit(s *engine.Step) {
	files, _ := filepath.Glob(filepath.Join(s.Artifacts, "*.xml"))
	for _, f := range files {
		cases, err := ParseJUnit(f)
		if err != nil {
			continue
		}
		for _, c := range cases {
			s.Case(c.Name, c.Status, c.Message, c.Ms)
		}
	}
}

// Case is one parsed test case.
type Case struct {
	Name    string
	Status  string
	Message string
	Ms      int64
}

type junitCase struct {
	Name      string  `xml:"name,attr"`
	Classname string  `xml:"classname,attr"`
	Time      float64 `xml:"time,attr"`
	Failure   *struct {
		Message string `xml:"message,attr"`
		Body    string `xml:",chardata"`
	} `xml:"failure"`
	Error *struct {
		Message string `xml:"message,attr"`
		Body    string `xml:",chardata"`
	} `xml:"error"`
	Skipped *struct {
		Message string `xml:"message,attr"`
	} `xml:"skipped"`
}

type junitSuite struct {
	Cases  []junitCase  `xml:"testcase"`
	Suites []junitSuite `xml:"testsuite"`
}

// ParseJUnit reads the test cases of a JUnit file, whether its root is
// <testsuites> or a single <testsuite>.
func ParseJUnit(path string) ([]Case, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root junitSuite
	if err := xml.Unmarshal(b, &root); err != nil {
		return nil, err
	}
	var out []Case
	var walk func(junitSuite)
	walk = func(s junitSuite) {
		for _, c := range s.Cases {
			jc := Case{Name: c.Name, Status: "passed", Ms: int64(c.Time * 1000)}
			if c.Classname != "" && !strings.Contains(c.Name, c.Classname) {
				jc.Name = c.Classname + " " + c.Name
			}
			switch {
			case c.Failure != nil:
				jc.Status, jc.Message = "failed", strings.TrimSpace(c.Failure.Message+"\n"+c.Failure.Body)
			case c.Error != nil:
				jc.Status, jc.Message = "errored", strings.TrimSpace(c.Error.Message+"\n"+c.Error.Body)
			case c.Skipped != nil:
				jc.Status, jc.Message = "skipped", c.Skipped.Message
			}
			out = append(out, jc)
		}
		for _, sub := range s.Suites {
			walk(sub)
		}
	}
	walk(root)
	return out, nil
}
