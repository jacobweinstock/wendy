// Package cli is wendy's command line.
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/report"
	"github.com/tinkerbell/wendy/internal/runner"
	"github.com/tinkerbell/wendy/internal/service"

	// Registers the built-in steps.
	_ "github.com/tinkerbell/wendy/internal/steps"
)

// globals are the flags every command shares.
type globals struct {
	configDirs []string
	tags       []string
	data       string
	addr       string
}

// dataDir is $XDG_STATE_HOME/wendy unless --data says otherwise.
func (g *globals) dataDir() string {
	if g.data != "" {
		return g.data
	}
	if s := os.Getenv("XDG_STATE_HOME"); s != "" {
		return filepath.Join(s, "wendy")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "wendy")
}

// dirs are the config directories, later ones taking precedence.
func (g *globals) dirs() []string {
	var out []string
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		out = append(out, filepath.Join(x, "wendy"))
	} else if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, ".config", "wendy"))
	}
	if wd, err := os.Getwd(); err == nil {
		out = append(out, filepath.Join(wd, ".wendy"))
	}
	for _, d := range g.configDirs {
		if abs, err := filepath.Abs(d); err == nil {
			out = append(out, abs)
		}
	}
	return out
}

// work is the directory BYO steps see: the last config directory that
// exists, else the working directory.
func (g *globals) work() string {
	dirs := g.dirs()
	for i := len(dirs) - 1; i >= 0; i-- {
		if st, err := os.Stat(dirs[i]); err == nil && st.IsDir() {
			return dirs[i]
		}
	}
	wd, _ := os.Getwd()
	return wd
}

func (g *globals) service() (*service.Service, error) {
	data := g.dataDir()
	if err := os.MkdirAll(data, 0o755); err != nil {
		return nil, err
	}
	builtins := engine.Builtins()
	r, err := config.NewRenderer(builtins)
	if err != nil {
		return nil, err
	}
	loader := &config.Loader{Dirs: g.dirs(), Tags: g.tags, Builtins: builtins}
	eng := &engine.Engine{Data: data, Work: g.work(), Renderer: r, Runners: runner.Runners(), Finish: report.WriteJUnit}
	svc := service.New(loader, eng)
	wd, _ := os.Getwd()
	svc.StarterDir = filepath.Join(wd, ".wendy")
	return svc, nil
}

// exitError carries an exit code out of a command.
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }

func exitf(code int, format string, args ...any) error {
	return &exitError{code: code, msg: fmt.Sprintf(format, args...)}
}
