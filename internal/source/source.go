// Package source builds Tinkerbell and CAPT from a git ref or a local
// checkout, with each repository's own Makefile. wendy only orchestrates.
package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"sigs.k8s.io/yaml"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/provider/registry"
)

// Default repositories for a source that names only a ref.
var defaultRepos = map[string]string{
	"tinkerbell": "https://github.com/tinkerbell/tinkerbell",
	"capt":       "https://github.com/tinkerbell/cluster-api-provider-tinkerbell",
}

// Checkout is a resolved source tree.
type Checkout struct {
	Repo    string `json:"repo"`
	Ref     string `json:"ref,omitempty"`
	Dir     string `json:"dir"`
	Commit  string `json:"commit"`
	Version string `json:"version"`
	Dirty   bool   `json:"dirty"`
}

// Runner runs a command in a directory, sending its output to w.
type Runner func(ctx context.Context, dir string, w io.Writer, env []string, name string, args ...string) (string, error)

// Exec is the Runner that runs real commands.
func Exec(ctx context.Context, dir string, w io.Writer, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	// Makefiles derive paths from $PWD, not the process's directory.
	cmd.Env = append(append(os.Environ(), "PWD="+dir), env...)
	var out strings.Builder
	if w != nil {
		cmd.Stdout = io.MultiWriter(&out, w)
		cmd.Stderr = w
	} else {
		cmd.Stdout = &out
	}
	err := cmd.Run()
	if err != nil {
		return out.String(), fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(out.String()), nil
}

// Builder resolves and builds sources.
type Builder struct {
	Cache string
	Run   Runner
	Log   io.Writer
}

// ExpandHome expands a leading ~ in a repo path.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// CacheDir is where a remote repository is cloned.
func (b Builder) CacheDir(repo string) string {
	sum := sha256.Sum256([]byte(repo))
	return filepath.Join(b.Cache, hex.EncodeToString(sum[:6]))
}

// Resolve finds the tree to build. A local repository with no ref is built
// where it sits, uncommitted work included; anything else is checked out in
// the cache, never in someone's working tree.
func (b Builder) Resolve(ctx context.Context, component string, src config.Source) (Checkout, error) {
	repo := ExpandHome(src.Repo)
	if repo == "" {
		repo = defaultRepos[component]
	}
	co := Checkout{Repo: repo, Ref: src.Ref}
	if st, err := os.Stat(repo); err == nil && st.IsDir() {
		abs, _ := filepath.Abs(repo)
		co.Repo = abs
		if src.Ref == "" {
			co.Dir = abs
			status, err := b.Run(ctx, abs, nil, nil, "git", "status", "--porcelain")
			if err != nil {
				return co, err
			}
			co.Dirty = status != ""
		}
	}
	if co.Dir == "" {
		dir, err := b.sync(ctx, co.Repo, src.Ref)
		if err != nil {
			return co, err
		}
		co.Dir = dir
	}
	commit, err := b.Run(ctx, co.Dir, nil, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return co, err
	}
	co.Commit = commit
	return co, nil
}

// sync clones once, then fetches, and detaches at what ref resolves to. It
// holds a lock: two builds writing one clone leave a broken tree.
func (b Builder) sync(ctx context.Context, repo, ref string) (string, error) {
	dir := b.CacheDir(repo)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	lock, err := os.OpenFile(dir+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return "", err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) //nolint:errcheck // Closing the file releases the lock anyway.

	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		_ = os.RemoveAll(dir)
		if _, err := b.Run(ctx, filepath.Dir(dir), b.Log, nil, "git", "clone", "--quiet", repo, dir); err != nil {
			return "", err
		}
	}
	// A swallowed fetch failure would report a moved branch as a pass of the
	// wrong commit.
	if _, err := b.Run(ctx, dir, b.Log, nil, "git", "fetch", "--quiet", "--force", "--tags", "origin", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
		return "", fmt.Errorf("cannot refresh %s: %w", repo, err)
	}
	var commit string
	if ref != "" {
		// A branch has to be read off the remote; the local one is stale.
		for _, cand := range []string{"origin/" + ref + "^{commit}", ref + "^{commit}"} {
			if c, err := b.Run(ctx, dir, nil, nil, "git", "rev-parse", "--verify", "--quiet", cand); err == nil && c != "" {
				commit = c
				break
			}
		}
		if commit == "" {
			return "", fmt.Errorf("cannot resolve ref %q in %s", ref, repo)
		}
	} else {
		_, _ = b.Run(ctx, dir, nil, nil, "git", "remote", "set-head", "--auto", "origin")
		c, err := b.Run(ctx, dir, nil, nil, "git", "rev-parse", "--verify", "--quiet", "origin/HEAD^{commit}")
		if err != nil || c == "" {
			return "", fmt.Errorf("cannot resolve the default branch of %s", repo)
		}
		commit = c
	}
	if _, err := b.Run(ctx, dir, nil, nil, "git", "checkout", "--quiet", "--detach", commit); err != nil {
		return "", err
	}
	return dir, nil
}

// TinkerbellOutputs are what later steps need from a Tinkerbell build.
type TinkerbellOutputs struct {
	Checkout
	Chart      string `json:"chart"`
	Image      string `json:"image"`
	AgentImage string `json:"agentImage"`
}

// BuildTinkerbell builds the tinkerbell and tink-agent images and the
// chart. Images are tagged with the version, which names the commit, so a
// clean tree already published is not rebuilt; a dirty one always is.
func (b Builder) BuildTinkerbell(ctx context.Context, co Checkout) (TinkerbellOutputs, error) {
	version, err := b.Run(ctx, co.Dir, nil, nil, "go", "run", "--buildvcs=true", "./script/version/")
	if err != nil {
		return TinkerbellOutputs{}, err
	}
	// The repo reports uncommitted work as +dirty; + is not legal in a tag.
	co.Version = strings.ReplaceAll(version, "+", "-")
	out := TinkerbellOutputs{
		Checkout:   co,
		Chart:      filepath.Join(co.Dir, "out", "helm", "tinkerbell-"+co.Version+".tgz"),
		Image:      registry.PushEndpoint + "/tinkerbell",
		AgentImage: registry.PushEndpoint + "/tink-agent",
	}
	if !co.Dirty && fileExists(out.Chart) && registry.Has(ctx, "tinkerbell", co.Version) && registry.Has(ctx, "tink-agent", co.Version) {
		fmt.Fprintf(b.Log, "%s is already built and published\n", co.Version)
		return out, nil
	}
	// `make image` is a plain `docker build`, whose result reaches the image
	// store `docker push` reads only on the engine's own builder, which is
	// named after the Docker context. Any other builder, ours or the user's
	// current one, would leave it in that builder's cache.
	engineBuilder, err := b.Run(ctx, co.Dir, nil, nil, "docker", "context", "show")
	if err != nil {
		return out, err
	}
	img := out.Image + ":" + co.Version
	steps := []struct {
		env []string
		cmd []string
	}{
		{[]string{"BUILDX_BUILDER=" + engineBuilder}, []string{"make", "cross-compile", "image", "IMAGE_NAME=" + img, "VERSION=" + co.Version}},
		{nil, []string{"docker", "push", img}},
		{[]string{"BUILDX_BUILDER=" + registry.Builder}, []string{"make", "cross-compile-agent", "build-push-image-agent", "IMAGE_NAME_AGENT=" + out.AgentImage, "VERSION=" + co.Version}},
		// Packaged, not pushed: the package carries the version as its
		// appVersion, which is what the image tags follow.
		{nil, []string{"make", "helm-package", "VERSION=" + co.Version}},
	}
	for _, st := range steps {
		if _, err := b.Run(ctx, co.Dir, b.Log, st.env, st.cmd[0], st.cmd[1:]...); err != nil {
			return out, err
		}
	}
	return out, nil
}

// CAPTOutputs are what later steps need from a CAPT build.
type CAPTOutputs struct {
	Checkout
	Repository string `json:"repository"`
	Image      string `json:"image"`
}

// BuildCAPT builds the manager image and a clusterctl release on disk, and
// publishes the image to the local registry.
func (b Builder) BuildCAPT(ctx context.Context, co Checkout, arch string) (CAPTOutputs, error) {
	out := CAPTOutputs{Checkout: co, Repository: filepath.Join(co.Dir, "out", "release", "infrastructure-tinkerbell")}
	if arch == "" {
		arch = runtime.GOARCH
	}
	meta := func() (string, string) {
		var m struct {
			Version string `json:"version"`
			Commit  string `json:"commit"`
		}
		b, err := os.ReadFile(filepath.Join(co.Dir, "dist", "metadata.json"))
		if err != nil || json.Unmarshal(b, &m) != nil {
			return "", ""
		}
		return "v" + m.Version + "-" + arch, m.Commit
	}
	tag, built := meta()
	if co.Dirty || built != co.Commit || !fileExists(filepath.Join(out.Repository, tag, "infrastructure-components.yaml")) {
		if _, err := b.Run(ctx, co.Dir, b.Log, nil, "make", "build-image"); err != nil {
			return out, err
		}
		tag, _ = meta()
		if tag == "" {
			return out, fmt.Errorf("the build wrote no dist/metadata.json")
		}
		if _, err := b.Run(ctx, co.Dir, b.Log, nil, "make", "release-local", "RELEASE_TAG="+tag); err != nil {
			return out, err
		}
	}
	co.Version, out.Version = tag, tag
	local, err := managerImage(filepath.Join(out.Repository, tag, "infrastructure-components.yaml"))
	if err != nil {
		return out, err
	}
	out.Image = registry.PushEndpoint + "/cluster-api-provider-tinkerbell:" + tag
	if !co.Dirty && registry.Has(ctx, "cluster-api-provider-tinkerbell", tag) {
		return out, nil
	}
	for _, st := range [][]string{{"docker", "tag", local, out.Image}, {"docker", "push", out.Image}} {
		if _, err := b.Run(ctx, co.Dir, b.Log, nil, st[0], st[1:]...); err != nil {
			return out, err
		}
	}
	return out, nil
}

// managerImage is the image release-local points the manager at.
func managerImage(components string) (string, error) {
	b, err := os.ReadFile(components)
	if err != nil {
		return "", err
	}
	for _, doc := range strings.Split(string(b), "\n---") {
		var obj struct {
			Kind string `json:"kind"`
			Spec struct {
				Template struct {
					Spec struct {
						Containers []struct {
							Name  string `json:"name"`
							Image string `json:"image"`
						} `json:"containers"`
					} `json:"spec"`
				} `json:"template"`
			} `json:"spec"`
		}
		if yaml.Unmarshal([]byte(doc), &obj) != nil || obj.Kind != "Deployment" {
			continue
		}
		for _, c := range obj.Spec.Template.Spec.Containers {
			if c.Name == "manager" {
				return c.Image, nil
			}
		}
	}
	return "", fmt.Errorf("no manager image in %s", components)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
