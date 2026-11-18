package event

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStreamReplayThenFollow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	s, err := Create(path, "1")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		s.Emit(Event{Type: Log, Msg: "before"})
	}
	sub := s.Subscribe(1)
	s.Emit(Event{Type: Log, Msg: "after"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var seqs []int64
	for i := 0; i < 3; i++ {
		e, ok := sub.Next(ctx)
		if !ok {
			t.Fatal("subscription ended early")
		}
		seqs = append(seqs, e.Seq)
	}
	if seqs[0] != 2 || seqs[1] != 3 || seqs[2] != 4 {
		t.Fatalf("seqs = %v, want [2 3 4]", seqs)
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := sub.Next(ctx); ok {
		t.Fatal("event after close")
	}

	reopened, err := Create(path, "1")
	if err != nil {
		t.Fatal(err)
	}
	if e := reopened.Emit(Event{Type: Log}); e.Seq != 5 {
		t.Fatalf("reopened seq = %d, want 5", e.Seq)
	}
}

// The log is read by people and tools, so its fields and types say execution
// and combination.
func TestEventJSONNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	s, err := Create(path, "12")
	if err != nil {
		t.Fatal(err)
	}
	s.Emit(Event{Type: CombinationStarted, Combination: "c"})
	_ = s.Close()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"execution":"12"`, `"combination":"c"`, `"type":"combination.started"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("%s lacks %s", b, want)
		}
	}
}

func TestWorst(t *testing.T) {
	cases := []struct{ a, b, want Status }{
		{"", Passed, Passed},
		{Passed, Skipped, Passed},
		{Failed, Errored, Errored},
		{Errored, Failed, Errored},
		{TimedOut, Failed, TimedOut},
	}
	for _, c := range cases {
		if got := Worst(c.a, c.b); got != c.want {
			t.Errorf("Worst(%q, %q) = %q, want %q", c.a, c.b, got, c.want)
		}
	}
}
