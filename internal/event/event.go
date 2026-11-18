// Package event defines wendy's events and the append-only log that stores
// them. The log is the single source of truth: UIs, reports and replays are
// all built from it.
package event

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Event types.
const (
	ExecutionStarted    = "execution.started"
	ExecutionFinished   = "execution.finished"
	CombinationStarted  = "combination.started"
	CombinationFinished = "combination.finished"
	CombinationPaused   = "combination.paused"
	CombinationResumed  = "combination.resumed"
	StepStarted         = "step.started"
	StepFinished        = "step.finished"
	Log                 = "log"
	Case                = "case"
	Artifact            = "artifact"
)

// Status is a verdict, or the state of something not finished yet.
type Status string

const (
	Passed    Status = "passed"
	Failed    Status = "failed"
	Errored   Status = "errored"
	Skipped   Status = "skipped"
	Cancelled Status = "cancelled"
	TimedOut  Status = "timedout"
	Running   Status = "running"
	Waiting   Status = "waiting"
	Paused    Status = "paused"
)

// severity orders verdicts so the worst one decides a parent's.
var severity = map[Status]int{
	Skipped: 0, Passed: 1, Failed: 2, TimedOut: 3, Errored: 4, Cancelled: 5,
}

// Worst is the more severe of two verdicts. An empty status counts as none.
func Worst(a, b Status) Status {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	if severity[b] > severity[a] {
		return b
	}
	return a
}

// Done reports whether the status is a final verdict.
func (s Status) Done() bool {
	_, ok := severity[s]
	return ok
}

// Event is one immutable, sequenced record.
type Event struct {
	V           int            `json:"v"`
	Seq         int64          `json:"seq"`
	TS          time.Time      `json:"ts"`
	Execution   string         `json:"execution"`
	Combination string         `json:"combination,omitempty"`
	Path        []string       `json:"path,omitempty"`
	Type        string         `json:"type"`
	Status      Status         `json:"status,omitempty"`
	Ms          int64          `json:"ms,omitempty"`
	Level       string         `json:"level,omitempty"`
	Msg         string         `json:"msg,omitempty"`
	Attrs       map[string]any `json:"attrs,omitempty"`
}

// Stream is an execution's event log: it appends to events.jsonl and fans
// out to subscribers.
type Stream struct {
	mu        sync.Mutex
	execution string
	path      string
	f         *os.File
	seq       int64
	subs      map[*Sub]struct{}
	done      bool
}

// Create starts a new log at path.
func Create(path, execution string) (*Stream, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	s := &Stream{execution: execution, path: path, f: f, subs: map[*Sub]struct{}{}}
	evs, err := ReadFile(path)
	if err == nil && len(evs) > 0 {
		s.seq = evs[len(evs)-1].Seq
	}
	return s, nil
}

// Emit stamps e with the next sequence number and the time, records it and
// delivers it to every subscriber.
func (s *Stream) Emit(e Event) Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return e
	}
	s.seq++
	e.V, e.Seq, e.Execution = 1, s.seq, s.execution
	if e.TS.IsZero() {
		e.TS = time.Now()
	}
	b, _ := json.Marshal(e)
	_, _ = s.f.Write(append(b, '\n'))
	for sub := range s.subs {
		sub.push(e)
	}
	return e
}

// Subscribe replays every event after seq from the file, then follows live.
func (s *Stream) Subscribe(after int64) *Sub {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub := newSub()
	if evs, err := ReadFile(s.path); err == nil {
		for _, e := range evs {
			if e.Seq > after {
				sub.push(e)
			}
		}
	}
	if s.done {
		sub.end()
		return sub
	}
	s.subs[sub] = struct{}{}
	sub.detach = func() {
		s.mu.Lock()
		delete(s.subs, sub)
		s.mu.Unlock()
	}
	return sub
}

// Close ends the log; subscribers drain what they have and then finish.
func (s *Stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return nil
	}
	s.done = true
	for sub := range s.subs {
		sub.end()
	}
	return s.f.Close()
}

// Sub is a subscription. It queues without bound, so a slow reader never
// blocks the engine and never loses an event.
type Sub struct {
	mu     sync.Mutex
	q      []Event
	notify chan struct{}
	closed bool
	detach func()
}

func newSub() *Sub { return &Sub{notify: make(chan struct{}, 1)} }

func (s *Sub) push(e Event) {
	s.mu.Lock()
	s.q = append(s.q, e)
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// end marks the log finished for this subscriber: Next returns false once
// the queue drains.
func (s *Sub) end() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// Next blocks for the next event. It returns false once the log is closed and
// drained, or ctx is done.
func (s *Sub) Next(ctx context.Context) (Event, bool) {
	for {
		s.mu.Lock()
		if len(s.q) > 0 {
			e := s.q[0]
			s.q = s.q[1:]
			s.mu.Unlock()
			return e, true
		}
		closed := s.closed
		s.mu.Unlock()
		if closed {
			return Event{}, false
		}
		select {
		case <-ctx.Done():
			return Event{}, false
		case <-s.notify:
		}
	}
}

// Close stops the subscription.
func (s *Sub) Close() {
	if s.detach != nil {
		s.detach()
	}
	s.end()
}

// Replay is a subscription to a finished log: every event after seq, then
// the end.
func Replay(path string, after int64) (*Sub, error) {
	evs, err := ReadFile(path)
	if err != nil {
		return nil, err
	}
	sub := newSub()
	for _, e := range evs {
		if e.Seq > after {
			sub.push(e)
		}
	}
	sub.end()
	return sub, nil
}

// ReadFile reads every event in an events.jsonl file.
func ReadFile(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Decode(f)
}

// Decode reads JSON-lines events, skipping a torn final line.
func Decode(r io.Reader) ([]Event, error) {
	var out []Event
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var e Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

// WriteSSE writes e as a server-sent event whose id is its sequence number,
// so a client reconnecting with Last-Event-ID resumes where it left off.
func WriteSSE(w io.Writer, e Event) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.Seq, e.Type, b)
	return err
}
