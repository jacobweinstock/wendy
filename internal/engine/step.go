// Package engine executes previews: builds, setup, suites, tests and steps,
// with a cleanup stack, timeouts, retries, verdicts and diagnostics.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/onsi/gomega"

	"github.com/tinkerbell/wendy/internal/config"
)

// StepFunc is a built-in step. Its ctx carries s.Log as a logr.Logger, so
// libraries that log through the context, such as controller-runtime, write
// to the step's log.
type StepFunc[P any] func(ctx context.Context, s *Step, p P) error

// Step is what a running step can reach.
type Step struct {
	// Testbed is nil for execution-level steps such as source builds.
	Testbed   *Testbed
	Log       *slog.Logger
	Artifacts string
	// Dir is the step's own directory; Artifacts is inside it.
	Dir     string
	Outputs map[string]any
	Defer   func(name string, fn func(context.Context) error)
	// Data is wendy's data directory, for host-wide state and caches.
	Data string
	// Work is the config directory, which BYO steps see as WENDY_WORK.
	Work string
	// Config is the step as configured; exec and container runners read it.
	Config config.Step
	// Case records a sub-result, such as one JUnit test case.
	Case func(name string, status Status, msg string, ms int64)
	// Builds holds the execution's source build outputs by component, repo and ref.
	Builds *Builds
}

// Status re-exports the event status so steps need not import event.
type Status = string

type builtin struct {
	description string
	schema      string
	run         func(ctx context.Context, s *Step, params json.RawMessage) error
}

var (
	regMu    sync.RWMutex
	registry = map[string]builtin{}
)

// Register adds a built-in step. cueSchema is a CUE struct for its params,
// e.g. `{target: *"worker" | "controlPlane", by: int}`; it becomes the step's
// entry in #Builtins, so params are checked by `wendy vet` before anything
// runs.
func Register[P any](id, description, cueSchema string, fn StepFunc[P]) {
	regMu.Lock()
	defer regMu.Unlock()
	registry[id] = builtin{
		description: description,
		schema:      cueSchema,
		run: func(ctx context.Context, s *Step, raw json.RawMessage) error {
			var p P
			if len(raw) > 0 && string(raw) != "null" {
				if err := json.Unmarshal(raw, &p); err != nil {
					return fmt.Errorf("decoding params of %s: %w", id, err)
				}
			}
			return fn(ctx, s, p)
		},
	}
}

// Builtins lists every registered step for the config loader.
func Builtins() []config.Builtin {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]config.Builtin, 0, len(registry))
	for id, b := range registry {
		out = append(out, config.Builtin{ID: id, Schema: b.schema, Description: b.description})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func lookup(id string) (builtin, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	b, ok := registry[id]
	return b, ok
}

type failure struct{ err error }

func (f *failure) Error() string { return f.err.Error() }
func (f *failure) Unwrap() error { return f.err }

type skip struct{ reason string }

func (s *skip) Error() string { return "skipped: " + s.reason }

// Fail marks err as a product failure: Tinkerbell or CAPT misbehaved. Any
// other error a step returns means the harness or the host broke.
func Fail(err error) error {
	if err == nil {
		return nil
	}
	return &failure{err: err}
}

// Failf is Fail with formatting.
func Failf(format string, args ...any) error { return Fail(fmt.Errorf(format, args...)) }

// Skip ends a step without a verdict on the product.
func Skip(reason string) error { return &skip{reason: reason} }

// IsFail reports whether err is a product failure.
func IsFail(err error) bool {
	var f *failure
	return errors.As(err, &f)
}

// gomegaFailure is what the global Gomega fail handler panics with. The step
// wrapper recovers it in the step's own goroutine, so parallel combinations
// do not interfere.
type gomegaFailure struct{ msg string }

func init() {
	gomega.RegisterFailHandler(func(message string, _ ...int) {
		panic(gomegaFailure{msg: message})
	})
}

// NewGomega returns a Gomega whose failures fail the calling step.
func NewGomega() gomega.Gomega {
	return gomega.NewGomega(func(message string, _ ...int) {
		panic(gomegaFailure{msg: message})
	})
}

// Builds holds the outputs of an execution's source builds.
type Builds struct {
	mu  sync.Mutex
	out map[string]map[string]any
	err map[string]error
}

func buildKey(component string, src config.Source) string {
	return component + "|" + src.Repo + "|" + src.Ref
}

// Get returns what building src produced, or why it failed.
func (b *Builds) Get(component string, src config.Source) (map[string]any, error) {
	if b == nil {
		return nil, fmt.Errorf("%s %s was not built in this execution", component, src.Repo)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	k := buildKey(component, src)
	if err := b.err[k]; err != nil {
		return nil, err
	}
	out, ok := b.out[k]
	if !ok {
		return nil, fmt.Errorf("%s %s was not built in this execution", component, src.Repo)
	}
	return out, nil
}

func (b *Builds) set(component string, src config.Source, out map[string]any, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.out == nil {
		b.out, b.err = map[string]map[string]any{}, map[string]error{}
	}
	k := buildKey(component, src)
	b.out[k], b.err[k] = out, err
}

// Snapshot is every successful build's outputs, for execution.json.
func (b *Builds) Snapshot() map[string]map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := map[string]map[string]any{}
	for k, v := range b.out {
		if b.err[k] == nil {
			out[k] = v
		}
	}
	return out
}
