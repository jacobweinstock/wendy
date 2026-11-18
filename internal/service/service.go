// Package service is the API the TUI, the web UI and the CLI share: the
// catalog, executions, kept testbeds and their events.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	wendycue "github.com/tinkerbell/wendy/cue"
	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/hostcheck"
	"github.com/tinkerbell/wendy/internal/preview"
)

// Service owns the catalog and the executions of this process.
type Service struct {
	Loader *config.Loader
	Engine *engine.Engine
	// StarterDir is where the first-launch starter config and the UI file
	// (config.UIFile) are written.
	StarterDir string
	// WebURL is shown in the UIs when the web UI is being served.
	WebURL string

	mu      sync.RWMutex
	editMu  sync.Mutex
	cat     *config.Catalog
	active  map[string]*engine.Execution
	subs    map[chan Change]struct{}
	checked []hostcheck.Check
}

// Change tells subscribers what moved: the catalog, or an execution.
type Change struct {
	Catalog   bool
	Execution string
}

// New loads the catalog.
func New(l *config.Loader, e *engine.Engine) *Service {
	s := &Service{Loader: l, Engine: e, active: map[string]*engine.Execution{}, subs: map[chan Change]struct{}{}}
	s.cat = l.Load()
	return s
}

// Watch reloads the catalog as config files change, until ctx is done.
func (s *Service) Watch(ctx context.Context) {
	_ = s.Loader.Watch(ctx, s.setCatalog)
}

// Reload re-reads the config now.
func (s *Service) Reload() *config.Catalog {
	c := s.Loader.Load()
	s.setCatalog(c)
	return c
}

func (s *Service) setCatalog(c *config.Catalog) {
	s.mu.Lock()
	s.cat = c
	s.mu.Unlock()
	s.notify(Change{Catalog: true})
}

// Catalog is the current catalog.
func (s *Service) Catalog() *config.Catalog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cat
}

// Subscribe returns changes to the catalog and to executions.
func (s *Service) Subscribe() (<-chan Change, func()) {
	ch := make(chan Change, 64)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}
}

func (s *Service) notify(c Change) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for ch := range s.subs {
		select {
		case ch <- c:
		default:
		}
	}
}

// Preview is a preview with what the host needs to know before running it.
type Preview struct {
	Preview *preview.Preview  `json:"preview"`
	Checks  []hostcheck.Check `json:"checks"`
}

// Preview resolves a selection, estimates it from past executions and checks
// the host.
func (s *Service) Preview(ctx context.Context, sel preview.Selection) (*Preview, error) {
	p, err := preview.New(s.Catalog(), sel)
	if err != nil {
		return nil, err
	}
	p.Estimate(engine.PastDurations(s.Engine.Data, p.Key))
	return &Preview{Preview: p, Checks: s.HostChecks(ctx, p)}, nil
}

// HostChecks checks what p needs, or the basics without a preview.
func (s *Service) HostChecks(ctx context.Context, p *preview.Preview) []hostcheck.Check {
	o := hostcheck.Options{DiskPaths: []string{s.Engine.Data}}
	if p != nil {
		o.Sources = len(p.Builds) > 0
		o.DiskGiB = p.Needs.DiskGiB
		seen := map[string]bool{}
		for _, c := range p.Combinations {
			if d := c.Testbed.Hardware.Backend.DiskPath; d != "" && !seen[d] {
				seen[d] = true
				o.DiskPaths = append(o.DiskPaths, d)
			}
		}
	}
	return hostcheck.Run(ctx, o)
}

// Start starts an execution of a selection.
func (s *Service) Start(ctx context.Context, sel preview.Selection) (*engine.Execution, error) {
	pv, err := s.Preview(ctx, sel)
	if err != nil {
		return nil, err
	}
	if failed := hostcheck.Failed(pv.Checks); len(failed) > 0 {
		return nil, fmt.Errorf("host check failed: %s: %s", failed[0].Name, failed[0].Detail)
	}
	if errs := s.Catalog().AllErrors(); s.Catalog().Stale && len(errs) > 0 {
		// The last good config is in use; say so rather than surprise.
		pv.Preview.Warnings = append(pv.Preview.Warnings, "config has errors; running the last valid config")
	}
	r, err := s.Engine.Start(pv.Preview, s.configDigest())
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.active[r.ID] = r
	s.mu.Unlock()
	s.notify(Change{Execution: r.ID})
	go s.follow(r) //nolint:gosec // The execution outlives the request that started it; its stream ends follow.
	return r, nil
}

// follow tells subscribers about each change to an execution, and forgets it
// once it is done.
func (s *Service) follow(r *engine.Execution) {
	sub := r.Stream.Subscribe(0)
	defer sub.Close()
	for {
		ev, ok := sub.Next(context.Background())
		if !ok {
			break
		}
		switch ev.Type {
		case event.StepFinished, event.CombinationStarted, event.CombinationFinished, event.CombinationPaused, event.CombinationResumed, event.ExecutionFinished:
			s.notify(Change{Execution: r.ID})
		}
	}
	<-r.Done()
	s.mu.Lock()
	delete(s.active, r.ID)
	s.mu.Unlock()
	s.notify(Change{Execution: r.ID})
}

// Active is the executions this process is running, newest first.
func (s *Service) Active() []*engine.Execution {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*engine.Execution, 0, len(s.active))
	for _, r := range s.active {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b *engine.Execution) int { return engine.CompareIDs(b.ID, a.ID) })
	return out
}

func (s *Service) activeExecution(id string) *engine.Execution {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active[id]
}

// Executions lists every execution, newest first.
func (s *Service) Executions() []*engine.Summary {
	out := engine.ListExecutions(s.Engine.Data)
	for i, sum := range out {
		if r := s.activeExecution(sum.ID); r != nil {
			live := r.Summary()
			out[i] = &live
		}
	}
	return out
}

// Execution returns one execution's summary.
func (s *Service) Execution(id string) (*engine.Summary, error) {
	if r := s.activeExecution(id); r != nil {
		sum := r.Summary()
		return &sum, nil
	}
	return engine.ReadSummary(s.ExecutionDir(id))
}

// ExecutionDir is an execution's directory. An id that is not a plain name,
// such as "..", gets a directory that no execution has.
func (s *Service) ExecutionDir(id string) string {
	if id = filepath.Base(id); id == "." || id == ".." {
		id = "invalid"
	}
	return filepath.Join(s.Engine.Data, engine.ExecutionsDir, id)
}

// Events subscribes to an execution's events after seq: live for an active
// execution, a replay for a finished one.
func (s *Service) Events(id string, after int64) (*event.Sub, error) {
	if r := s.activeExecution(id); r != nil {
		return r.Stream.Subscribe(after), nil
	}
	return event.Replay(filepath.Join(s.ExecutionDir(id), "events.jsonl"), after)
}

// Cancel stops an active execution.
func (s *Service) Cancel(id string) error {
	r := s.activeExecution(id)
	if r == nil {
		return fmt.Errorf("#%s is not running", id)
	}
	r.Cancel()
	return nil
}

// Decide answers a paused combination: engine.Retry, Continue or Abort.
func (s *Service) Decide(id, combination, decision string) error {
	r := s.activeExecution(id)
	if r == nil {
		return fmt.Errorf("#%s is not running", id)
	}
	return r.Decide(combination, decision)
}

// RunAgain starts an execution with the same selection; with failedOnly,
// narrowed to the combinations that did not pass.
func (s *Service) RunAgain(ctx context.Context, id string, failedOnly bool) (*engine.Execution, error) {
	sum, err := s.Execution(id)
	if err != nil {
		return nil, err
	}
	sel := sum.Selection
	if failedOnly {
		var names []string
		for _, c := range sum.Combinations {
			if c.Status != event.Passed && c.Status != event.Skipped {
				names = append(names, c.Name)
			}
		}
		if len(names) == 0 {
			return nil, fmt.Errorf("every combination of #%s passed", id)
		}
		sel.Combinations = names
	}
	return s.Start(ctx, sel)
}

// Delete removes a finished execution that keeps no testbed.
func (s *Service) Delete(id string) error {
	if s.activeExecution(id) != nil {
		return fmt.Errorf("#%s is still running", id)
	}
	sum, err := engine.ReadSummary(s.ExecutionDir(id))
	if err != nil {
		return err
	}
	for _, c := range sum.Combinations {
		if c.Kept {
			return fmt.Errorf("#%s keeps the testbed of %s; tear it down first", id, c.Name)
		}
	}
	err = os.RemoveAll(s.ExecutionDir(id))
	s.notify(Change{Execution: id})
	return err
}

// KeptTestbed is a combination's testbed that is still up: kept after its
// execution, or paused on a failure.
type KeptTestbed struct {
	Execution   string       `json:"execution"`
	Combination string       `json:"combination"`
	Label       string       `json:"label"`
	Paused      bool         `json:"paused"`
	Status      event.Status `json:"status"`
	Since       time.Time    `json:"since"`
	Dir         string       `json:"dir"`
	State       engine.State `json:"state"`
}

// KeptTestbeds lists every kept or paused testbed.
func (s *Service) KeptTestbeds() []KeptTestbed {
	var out []KeptTestbed
	for _, sum := range s.Executions() {
		out = append(out, s.KeptTestbedsOf(sum)...)
	}
	return out
}

// KeptTestbedsOf lists an execution's kept or paused testbeds, in the order
// of its combinations.
func (s *Service) KeptTestbedsOf(sum *engine.Summary) []KeptTestbed {
	var out []KeptTestbed
	for _, c := range sum.Combinations {
		if !c.Kept && !c.Paused {
			continue
		}
		dir := filepath.Join(s.ExecutionDir(sum.ID), c.Dir)
		tb, err := engine.LoadTestbed(dir, s.Engine.Renderer)
		if err != nil {
			continue
		}
		since := sum.Finished
		if since.IsZero() {
			since = sum.Started
		}
		out = append(out, KeptTestbed{
			Execution: sum.ID, Combination: c.Name, Label: sum.Label, Paused: c.Paused, Status: c.Status,
			Since: since, Dir: dir, State: tb.State(),
		})
	}
	return out
}

// TearDown destroys a kept testbed, or aborts a paused one.
func (s *Service) TearDown(ctx context.Context, executionID, combination string, report func(name string, status event.Status, msg string)) error {
	sum, err := s.Execution(executionID)
	if err != nil {
		return err
	}
	var dir string
	for _, c := range sum.Combinations {
		if c.Name == combination {
			if c.Paused {
				return s.Decide(executionID, combination, engine.Abort)
			}
			dir = filepath.Join(s.ExecutionDir(executionID), c.Dir)
		}
	}
	if dir == "" {
		return fmt.Errorf("#%s has no combination %s", executionID, combination)
	}
	tb, err := engine.LoadTestbed(dir, s.Engine.Renderer)
	if err != nil {
		return err
	}
	errs := engine.RunTeardown(ctx, tb, func(name string, st event.Status, msg string, _ int64) {
		if report != nil {
			report(name, st, msg)
		}
	})
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	sum.Combinations = markTornDown(sum.Combinations, combination)
	err = engine.WriteSummary(s.ExecutionDir(executionID), sum)
	s.notify(Change{Execution: executionID})
	return err
}

func markTornDown(cs []engine.CombinationSummary, combination string) []engine.CombinationSummary {
	for i := range cs {
		if cs[i].Name == combination {
			cs[i].Kept = false
		}
	}
	return cs
}

// WriteStarter writes the commented starter config, only when there is no
// config at all. It is the one file the UIs ever create.
func (s *Service) WriteStarter() (string, error) {
	if s.Catalog().HasUserConfig {
		return "", errors.New("a config already exists")
	}
	if err := os.MkdirAll(s.StarterDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(s.StarterDir, "wendy.cue")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(wendycue.Starter); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	s.Reload()
	return path, nil
}

// FirstChecks are the host checks shown on first launch, computed once.
func (s *Service) FirstChecks(ctx context.Context) []hostcheck.Check {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.checked == nil {
		s.checked = hostcheck.Run(ctx, hostcheck.Options{})
	}
	return s.checked
}

// LastResult is the latest finished execution of a plan, for the Home screen.
func (s *Service) LastResult(plan string) *engine.Summary {
	for _, sum := range s.Executions() {
		if sum.Key == preview.PlanKey(plan) && sum.Status.Done() {
			return sum
		}
	}
	return nil
}

func (s *Service) configDigest() string {
	h := sha256.New()
	for _, f := range s.Catalog().Files {
		b, _ := os.ReadFile(f.Path)
		h.Write([]byte(f.Path))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}
