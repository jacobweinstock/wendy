package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/preview"
)

// RunSingle runs one step against a kept testbed, outside any execution.
// Its events go to an adhoc directory inside the testbed and to onEvent.
func (e *Engine) RunSingle(ctx context.Context, tb *Testbed, st config.Step, onEvent func(event.Event)) (event.Status, error) {
	dir := filepath.Join(tb.Dir, "adhoc", time.Now().Format("20060102-150405")+"-"+safe(st.DisplayName()))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	stream, err := event.Create(filepath.Join(dir, "events.jsonl"), tb.Execution)
	if err != nil {
		return "", err
	}
	sub := stream.Subscribe(0)
	go func() {
		for {
			ev, ok := sub.Next(ctx)
			if !ok {
				return
			}
			if onEvent != nil {
				onEvent(ev)
			}
		}
	}()
	defer sub.Close()

	r := &Execution{ID: tb.Execution, Dir: tb.Dir, Stream: stream, engine: e, ctx: ctx, builds: tb.Builds, pauses: map[string]chan string{}}
	if st.Timeout == "" {
		st.Timeout = "15m"
	}
	ps := preview.Step{Step: st, Key: st.DisplayName()}
	res := r.runStep(ctx, tb, tb.Name, []string{"adhoc", "adhoc", ps.Key}, ps, dir, stepOpts{uncounted: true})
	_ = stream.Close()
	if failing(res.status) {
		return res.status, fmt.Errorf("%s %s: %s", ps.Key, res.status, res.msg)
	}
	return res.status, nil
}
