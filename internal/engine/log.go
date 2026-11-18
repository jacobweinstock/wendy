package engine

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/tinkerbell/wendy/internal/event"
)

// logHandler sends each record to the step's log file and to the event
// stream, so the UIs show it live and step.log keeps it.
type logHandler struct {
	mu          *sync.Mutex
	w           io.Writer
	emit        func(event.Event)
	combination string
	path        []string
	attrs       []slog.Attr
	prefix      string
}

func newLogger(w io.Writer, emit func(event.Event), combination string, path []string) *slog.Logger {
	return slog.New(&logHandler{mu: &sync.Mutex{}, w: w, emit: emit, combination: combination, path: path})
}

func (h *logHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= slog.LevelDebug }

func (h *logHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Message)
	attrs := map[string]any{}
	add := func(a slog.Attr) bool {
		k := h.prefix + a.Key
		attrs[k] = a.Value.Any()
		fmt.Fprintf(&b, " %s=%v", k, a.Value.Any())
		return true
	}
	for _, a := range h.attrs {
		add(a)
	}
	r.Attrs(add)
	msg := b.String()

	h.mu.Lock()
	fmt.Fprintf(h.w, "%s %-5s %s\n", r.Time.Format(time.TimeOnly), r.Level, msg)
	h.mu.Unlock()
	if r.Level >= slog.LevelInfo {
		e := event.Event{Type: event.Log, Combination: h.combination, Path: h.path, Level: strings.ToLower(r.Level.String()), Msg: r.Message}
		if len(attrs) > 0 {
			e.Attrs = attrs
		}
		h.emit(e)
	}
	return nil
}

func (h *logHandler) WithAttrs(as []slog.Attr) slog.Handler {
	c := *h
	c.attrs = append(append([]slog.Attr{}, h.attrs...), as...)
	return &c
}

func (h *logHandler) WithGroup(name string) slog.Handler {
	c := *h
	c.prefix = h.prefix + name + "."
	return &c
}

// lineWriter turns a byte stream into log lines, for subprocess output.
type lineWriter struct {
	mu   sync.Mutex
	buf  []byte
	line func(string)
}

// LineWriter returns a writer that calls line for each complete line written
// to it. Flush the remainder with Close.
func LineWriter(line func(string)) io.WriteCloser { return &lineWriter{line: line} }

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := strings.IndexByte(string(w.buf), '\n')
		if i < 0 {
			break
		}
		w.line(strings.TrimRight(string(w.buf[:i]), "\r"))
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

func (w *lineWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.line(string(w.buf))
		w.buf = nil
	}
	return nil
}
