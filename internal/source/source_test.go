package source

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinkerbell/wendy/internal/config"
)

type fakeGit struct {
	calls []string
	out   map[string]string
}

func (f *fakeGit) run(_ context.Context, _ string, _ io.Writer, _ []string, name string, args ...string) (string, error) {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if strings.HasPrefix(call, "git clone") {
		_ = os.MkdirAll(filepath.Join(args[len(args)-1], ".git"), 0o755)
	}
	for prefix, out := range f.out {
		if strings.HasPrefix(call, prefix) {
			return out, nil
		}
	}
	return "", nil
}

func TestResolveLocalWorkingTree(t *testing.T) {
	repo := t.TempDir()
	g := &fakeGit{out: map[string]string{"git status": " M main.go", "git rev-parse HEAD": "abc1234"}}
	co, err := Builder{Cache: t.TempDir(), Run: g.run}.Resolve(context.Background(), "tinkerbell", config.Source{Repo: repo})
	if err != nil {
		t.Fatal(err)
	}
	if co.Dir != repo || !co.Dirty || co.Commit != "abc1234" {
		t.Errorf("checkout = %+v", co)
	}
	for _, c := range g.calls {
		if strings.HasPrefix(c, "git checkout") || strings.HasPrefix(c, "git clone") {
			t.Errorf("touched the working tree: %s", c)
		}
	}
}

func TestResolveRefInCache(t *testing.T) {
	g := &fakeGit{out: map[string]string{
		"git rev-parse --verify --quiet origin/main^{commit}": "def5678",
		"git rev-parse HEAD": "def5678",
	}}
	b := Builder{Cache: t.TempDir(), Run: g.run}
	co, err := b.Resolve(context.Background(), "capt", config.Source{Ref: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if co.Repo != "https://github.com/tinkerbell/cluster-api-provider-tinkerbell" || co.Dir != b.CacheDir(co.Repo) || co.Dirty {
		t.Errorf("checkout = %+v", co)
	}
	want := []string{"git clone", "git fetch", "git rev-parse --verify --quiet origin/main", "git checkout --quiet --detach def5678", "git rev-parse HEAD"}
	if len(g.calls) != len(want) {
		t.Fatalf("calls = %q", g.calls)
	}
	for i, w := range want {
		if !strings.HasPrefix(g.calls[i], w) {
			t.Errorf("call %d = %q, want %q", i, g.calls[i], w)
		}
	}
}

func TestManagerImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "infrastructure-components.yaml")
	doc := "kind: Namespace\nmetadata: {name: capt-system}\n---\nkind: Deployment\nspec:\n  template:\n    spec:\n      containers:\n      - name: kube-rbac-proxy\n        image: x\n      - name: manager\n        image: ghcr.io/tinkerbell/cluster-api-provider-tinkerbell:v0.7.1-amd64\n"
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	img, err := managerImage(path)
	if err != nil || img != "ghcr.io/tinkerbell/cluster-api-provider-tinkerbell:v0.7.1-amd64" {
		t.Errorf("image = %q, %v", img, err)
	}
}
