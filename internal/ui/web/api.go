// Package web is wendy's web UI and HTTP API.
package web

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/preview"
	"github.com/tinkerbell/wendy/internal/service"
)

// Server serves the web UI and the API over a Service.
type Server struct {
	svc   *service.Service
	token string
}

// New returns a server. With a token, every request must carry it, as a
// bearer token or a ?token= query (for the browser's first page).
func New(svc *service.Service, token string) *Server { return &Server{svc: svc, token: token} }

// Handler is the server's routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/executions", s.listExecutions)
	mux.HandleFunc("POST /api/executions", s.startExecution)
	mux.HandleFunc("GET /api/executions/{id}", s.getExecution)
	mux.HandleFunc("DELETE /api/executions/{id}", s.deleteExecution)
	mux.HandleFunc("POST /api/executions/{id}/{action}", s.executionAction)
	mux.HandleFunc("GET /api/executions/{id}/events", s.executionEvents)
	mux.HandleFunc("POST /api/executions/{id}/{combination}/teardown", s.teardown)
	mux.HandleFunc("GET /api/kept-testbeds", s.keptTestbeds)
	mux.HandleFunc("POST /api/preview", s.preview)
	mux.HandleFunc("GET /api/config", s.loadedConfig)
	mux.HandleFunc("GET /api/config/events", s.configEvents)
	mux.HandleFunc("POST /api/starter", s.starter)
	mux.HandleFunc("POST /api/reload", func(w http.ResponseWriter, _ *http.Request) {
		s.svc.Reload()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("GET /executions/{id}/files/", http.HandlerFunc(s.files))
	s.pages(mux)
	return s.auth(mux)
}

func (s *Server) auth(next http.Handler) http.Handler {
	if s.token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if c, err := r.Cookie("wendy_token"); err == nil && got == "" {
			got = c.Value
		}
		if q := r.URL.Query().Get("token"); q != "" {
			got = q
			//nolint:gosec // G124: Secure only over TLS; the UI is plain HTTP on localhost by default.
			http.SetCookie(w, &http.Cookie{Name: "wendy_token", Value: q, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil})
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func (s *Server) listExecutions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.svc.Executions())
}

func (s *Server) getExecution(w http.ResponseWriter, r *http.Request) {
	sum, err := s.svc.Execution(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

// selection reads a Selection from a JSON body or a form.
func selection(r *http.Request) (preview.Selection, error) {
	var sel preview.Selection
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return sel, json.NewDecoder(r.Body).Decode(&sel)
	}
	if err := r.ParseForm(); err != nil {
		return sel, err
	}
	f := r.Form
	sel.Plan, sel.Testbed, sel.LabelFilter, sel.Keep = f.Get("plan"), f.Get("testbed"), f.Get("labelFilter"), f.Get("keep")
	if c := strings.TrimSpace(f.Get("combinations")); c != "" {
		sel.Combinations = strings.Fields(strings.ReplaceAll(c, ",", " "))
	} else if f.Has("combinations") {
		sel.Combinations = []string{}
	}
	if f.Has("suites") || f.Has("suitesSet") {
		sel.Suites = append([]string{}, f["suites"]...)
	}
	if p, err := strconv.Atoi(f.Get("parallel")); err == nil {
		sel.Parallel = p
	}
	if f.Has("pauseSet") {
		v := f.Get("pauseOnFailure") == "on" || f.Get("pauseOnFailure") == "true"
		sel.PauseOnFailure = &v
	}
	if f.Has("diagnosticsSet") {
		d := config.Diagnostics{Collect: f.Get("diagnostics"), Consoles: f.Get("consoles") == "on" || f.Get("consoles") == "true"}
		if !slices.Contains(diagnosticsChoices, d.Collect) {
			return sel, fmt.Errorf("diagnostics must be one of %s", strings.Join(diagnosticsChoices, ", "))
		}
		sel.Diagnostics = &d
	}
	return sel, nil
}

func (s *Server) startExecution(w http.ResponseWriter, r *http.Request) {
	sel, err := s.formSelection(r)
	if err != nil {
		formError(w, r, err)
		return
	}
	run, err := s.svc.Start(context.Background(), sel)
	if err != nil {
		formError(w, r, err)
		return
	}
	started(w, r, run)
}

// started answers a request that started an execution: htmx goes to its page,
// and other clients get its summary with the page as the Location.
func started(w http.ResponseWriter, r *http.Request, run *engine.Execution) {
	u := string(executionURL(run.ID))
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", u)
		return
	}
	w.Header().Set("Location", u)
	writeJSON(w, http.StatusCreated, run.Summary())
}

// formError shows err under the form for htmx, and as JSON otherwise.
func formError(w http.ResponseWriter, r *http.Request, err error) {
	formErrorAt(w, r, err, "#form-error")
}

// formErrorAt shows err in the element target selects for htmx, and as JSON
// otherwise.
func formErrorAt(w http.ResponseWriter, r *http.Request, err error, target string) {
	if r.Header.Get("HX-Request") == "" {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("HX-Retarget", target)
	w.Header().Set("HX-Reswap", "innerHTML")
	_, _ = io.WriteString(w, editErrorHTML(err)) //nolint:gosec // G705: editErrorHTML escapes everything it writes.
}

func (s *Server) deleteExecution(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.Delete(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", string(executionsURL(nil)))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) executionAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	combination := r.URL.Query().Get("combination")
	var err error
	switch a := r.PathValue("action"); a {
	case "cancel":
		err = s.svc.Cancel(id)
	case "continue":
		err = s.svc.Decide(id, combination, engine.Continue)
	case "abort":
		err = s.svc.Decide(id, combination, engine.Abort)
	case "retry":
		err = s.svc.Decide(id, combination, engine.Retry)
	case "run-again", "run-failed-again":
		var run *engine.Execution
		run, err = s.svc.RunAgain(context.Background(), id, a == "run-failed-again")
		if err == nil {
			started(w, r, run)
			return
		}
	default:
		err = fmt.Errorf("unknown action %q", a)
	}
	if err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sse prepares a response for server-sent events.
func sse(w http.ResponseWriter) (http.Flusher, bool) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	return fl, true
}

// executionEvents streams an execution's events after ?after, or after Last-Event-ID
// when the browser resumes. With ?as=update it sends only "update" ticks,
// at most one per updateEvery, for pages that re-render rather than
// consume events.
func (s *Server) executionEvents(w http.ResponseWriter, r *http.Request) {
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if id := r.Header.Get("Last-Event-ID"); id != "" {
		after, _ = strconv.ParseInt(id, 10, 64)
	}
	sub, err := s.svc.Events(r.PathValue("id"), after)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	defer sub.Close()
	fl, ok := sse(w)
	if !ok {
		return
	}
	if r.URL.Query().Get("as") == "update" {
		updates(r.Context(), w, fl, sub)
		return
	}
	for {
		e, ok := sub.Next(r.Context())
		if !ok {
			fmt.Fprint(w, "event: end\ndata: {}\n\n")
			fl.Flush()
			return
		}
		if err := event.WriteSSE(w, e); err != nil {
			return
		}
		fl.Flush()
	}
}

const updateEvery = 700 * time.Millisecond

// updates coalesces events into ticks. A chatty step must not make the
// browser refetch per log line, yet the last change must never be dropped.
func updates(ctx context.Context, w http.ResponseWriter, fl http.Flusher, sub *event.Sub) {
	for {
		e, ok := sub.Next(ctx)
		if !ok {
			if ctx.Err() == nil {
				fmt.Fprint(w, "event: end\ndata: {}\n\n")
				fl.Flush()
			}
			return
		}
		seq := e.Seq
		var closed bool
		wait, cancel := context.WithTimeout(ctx, updateEvery)
		for {
			e, ok := sub.Next(wait)
			if !ok {
				closed = wait.Err() == nil
				break
			}
			seq = e.Seq
		}
		cancel()
		if ctx.Err() != nil {
			return
		}
		fmt.Fprintf(w, "id: %d\nevent: update\ndata: %d\n\n", seq, seq)
		if closed {
			fmt.Fprint(w, "event: end\ndata: {}\n\n")
		}
		fl.Flush()
		if closed {
			return
		}
	}
}

// ConfigJSON is the loaded config as the API returns it.
type ConfigJSON struct {
	Dirs     []string                              `json:"dirs"`
	Files    []config.FileStatus                   `json:"files"`
	Errors   []config.Error                        `json:"errors"`
	LoadedAt string                                `json:"loadedAt"`
	Testbeds map[string]*config.TestbedItem        `json:"testbeds"`
	Suites   map[string]*config.Item[config.Suite] `json:"suites"`
	Tests    map[string]*config.Item[config.Test]  `json:"tests"`
	Plans    map[string]*config.Item[config.Plan]  `json:"plans"`
	Steps    []config.Builtin                      `json:"steps"`
}

func (s *Server) loadedConfig(w http.ResponseWriter, _ *http.Request) {
	c := s.svc.Catalog()
	writeJSON(w, http.StatusOK, ConfigJSON{
		Dirs: c.Dirs, Files: c.Files, Errors: c.AllErrors(), LoadedAt: c.LoadedAt.Format("15:04:05"),
		Testbeds: c.Testbeds, Suites: c.Suites, Tests: c.Tests,
		Plans: c.Plans, Steps: c.BuiltinSteps,
	})
}

// configEvents tells the browser when the config or an execution changes.
func (s *Server) configEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := sse(w)
	if !ok {
		return
	}
	ch, cancel := s.svc.Subscribe()
	defer cancel()
	fmt.Fprint(w, ": hello\n\n")
	fl.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case c := <-ch:
			kind := "execution"
			if c.Catalog {
				kind = "config"
			}
			b, _ := json.Marshal(c)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, b)
			fl.Flush()
		}
	}
}

func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	sel, err := s.formSelection(r)
	if err != nil && r.Header.Get("HX-Request") == "" {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var pv *service.Preview
	if err == nil {
		pv, err = s.svc.Preview(r.Context(), sel)
	}
	if r.Header.Get("HX-Request") != "" {
		s.previewFragment(w, r, pv, err)
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, pv)
}

func (s *Server) keptTestbeds(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.svc.KeptTestbeds())
}

func (s *Server) teardown(w http.ResponseWriter, r *http.Request) {
	execution, combination := r.PathValue("id"), r.PathValue("combination")
	// Teardown can take minutes; it outlives the request.
	go func() {
		_ = s.svc.TearDown(context.Background(), execution, combination, nil)
	}()
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", string(executionURL(execution)))
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) starter(w http.ResponseWriter, r *http.Request) {
	path, err := s.svc.WriteStarter()
	if err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", configPath)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"path": path})
}

// files serves an execution's directory: artifacts, logs, kubeconfigs.
func (s *Server) files(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if strings.Contains(id, "/") || strings.Contains(id, "..") {
		http.NotFound(w, r)
		return
	}
	prefix := string(executionFileURL(id, ""))
	http.StripPrefix(strings.TrimSuffix(prefix, "/"), http.FileServer(noListingFS{http.Dir(s.svc.ExecutionDir(id))})).ServeHTTP(w, r)
}

// noListingFS hides nothing but refuses to follow paths out of the
// execution's directory.
type noListingFS struct{ http.FileSystem }

func (f noListingFS) Open(name string) (http.File, error) {
	if strings.Contains(name, "..") {
		return nil, errors.New("invalid path")
	}
	return f.FileSystem.Open(name)
}
