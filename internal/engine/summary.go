package engine

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/preview"
)

// Summary is execution.json: enough to list an execution and estimate the
// next one without reading its events.
type Summary struct {
	ID             string               `json:"id"`
	Label          string               `json:"label"`
	Key            string               `json:"key"`
	Edited         bool                 `json:"edited,omitempty"`
	Selection      preview.Selection    `json:"selection"`
	ConfigDigest   string               `json:"configDigest,omitempty"`
	Status         event.Status         `json:"status"`
	Started        time.Time            `json:"started"`
	Finished       time.Time            `json:"finished,omitzero"`
	Ms             int64                `json:"ms,omitempty"`
	PID            int                  `json:"pid,omitempty"`
	Steps          StepCounts           `json:"steps"`
	Builds         []BuildSummary       `json:"builds,omitempty"`
	Combinations   []CombinationSummary `json:"combinations"`
	TeardownErrors []string             `json:"teardownErrors,omitempty"`
}

// StepCounts drive the progress bar.
type StepCounts struct {
	Total   int `json:"total"`
	Done    int `json:"done"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Errored int `json:"errored"`
}

type BuildSummary struct {
	Component string       `json:"component"`
	Repo      string       `json:"repo"`
	Ref       string       `json:"ref,omitempty"`
	Commit    string       `json:"commit,omitempty"`
	Version   string       `json:"version,omitempty"`
	Dirty     bool         `json:"dirty,omitempty"`
	Status    event.Status `json:"status"`
}

type CombinationSummary struct {
	Name   string       `json:"name"`
	Status event.Status `json:"status"`
	Ms     int64        `json:"ms,omitempty"`
	// Nodes are the verdicts of the grid's columns: build, setup and suites.
	Nodes   map[string]event.Status `json:"nodes"`
	Dir     string                  `json:"dir"`
	Kept    bool                    `json:"kept,omitempty"`
	Paused  bool                    `json:"paused,omitempty"`
	Failure *Failure                `json:"failure,omitempty"`
}

// Failure is what the failure card shows.
type Failure struct {
	Path    []string     `json:"path"`
	Status  event.Status `json:"status"`
	Msg     string       `json:"msg"`
	Ms      int64        `json:"ms"`
	Related string       `json:"related,omitempty"`
	// Diagnostics maps a kind (workflow, console, bmc, dump) to a path
	// relative to the execution directory.
	Diagnostics map[string]string `json:"diagnostics,omitempty"`
}

// ExecutionsDir is the directory, under wendy's data directory, that holds
// one directory per execution, named by its ID.
const ExecutionsDir = "executions"

// ReadSummary reads an execution's execution.json.
func ReadSummary(dir string) (*Summary, error) {
	b, err := os.ReadFile(filepath.Join(dir, "execution.json"))
	if err != nil {
		return nil, err
	}
	var s Summary
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	// An execution whose process is gone without finishing it is not running.
	if !s.Status.Done() && s.PID != 0 && s.PID != os.Getpid() && !alive(s.PID) {
		s.Status = event.Errored
	}
	return &s, nil
}

func alive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscallZero) == nil
}

// WriteSummary writes execution.json atomically.
func WriteSummary(dir string, s *Summary) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".execution.json.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "execution.json"))
}

// ListExecutions returns every execution's summary, newest first.
func ListExecutions(data string) []*Summary {
	entries, _ := os.ReadDir(filepath.Join(data, ExecutionsDir))
	var out []*Summary
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if s, err := ReadSummary(filepath.Join(data, ExecutionsDir, e.Name())); err == nil {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b *Summary) int { return CompareIDs(b.ID, a.ID) })
	return out
}

// CompareIDs orders execution IDs by number, so 10 comes after 9. It returns
// -1, 0 or +1, as cmp.Compare does.
func CompareIDs(a, b string) int {
	na, _ := strconv.Atoi(a)
	nb, _ := strconv.Atoi(b)
	return cmp.Or(cmp.Compare(na, nb), cmp.Compare(a, b))
}

// newExecutionDir allocates the next execution ID, 1 onwards, by creating its
// directory: Mkdir is atomic, so two processes never get the same ID.
func newExecutionDir(data string) (string, string, error) {
	root := filepath.Join(data, ExecutionsDir)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", "", err
	}
	for attempt := 0; attempt < 100; attempt++ {
		next := 1
		entries, _ := os.ReadDir(root)
		for _, e := range entries {
			if n, err := strconv.Atoi(e.Name()); err == nil && n >= next {
				next = n + 1
			}
		}
		id := strconv.Itoa(next)
		dir := filepath.Join(root, id)
		if err := os.Mkdir(dir, 0o755); err == nil {
			return id, dir, nil
		} else if !errors.Is(err, os.ErrExist) {
			return "", "", err
		}
	}
	return "", "", fmt.Errorf("could not allocate an execution ID under %s", root)
}

// PastDurations are the durations of past executions with the given key that
// ran to the end, for the preview's estimate. An execution that errored or was
// cancelled stopped part-way, so its duration says nothing about the next one.
func PastDurations(data, key string) []time.Duration {
	var out []time.Duration
	for _, s := range ListExecutions(data) {
		if s.Key == key && (s.Status == event.Passed || s.Status == event.Failed) && s.Ms > 0 {
			out = append(out, time.Duration(s.Ms)*time.Millisecond)
		}
	}
	return out
}
