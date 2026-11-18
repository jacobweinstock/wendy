package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/hostcheck"
	"github.com/tinkerbell/wendy/internal/preview"
	"github.com/tinkerbell/wendy/internal/service"
	"github.com/tinkerbell/wendy/internal/ui"
	"github.com/tinkerbell/wendy/internal/vocab"
)

//go:generate go run github.com/a-h/templ/cmd/templ@v0.3.1118 generate

//go:embed static
var static embed.FS

// The /api/config/events types a page's main area re-renders on.
const (
	onAnyChange    = "execution config"
	onConfigChange = "config"
)

// layout is what every page's frame shows.
type layout struct {
	Tab  string
	Self string
	// Stream is the event stream that tells the page it is out of date, and
	// Events the event types that do. app.js holds the stream only while the
	// page is visible.
	Stream string
	Events string
	// Refresh re-renders the page's main area when the stream says so.
	Refresh      bool
	Running      int
	ConfigErrors int
	Palette      string
}

// paletteItem is one action of the command palette. Aliases are words that
// also find it but are never shown: people search with the words they know,
// including the ones wendy used before.
type paletteItem struct {
	Label   string `json:"label"`
	Kind    string `json:"kind"`
	Href    string `json:"href,omitempty"`
	Post    string `json:"post,omitempty"`
	Body    string `json:"body,omitempty"`
	Aliases string `json:"aliases,omitempty"`
}

// navAliases are the palette aliases of each tab's "Go to" item.
var navAliases = map[string]string{
	"executions": "runs kept environments",
	"plans":      "profiles",
	"testbeds":   "environments matrices matrix catalog parts",
	"suites":     "tests scenarios catalog steps",
}

// layout is the frame of a page. With events, its main area re-renders on
// those /api/config/events types.
func (s *Server) layout(r *http.Request, tab, events string) layout {
	var items []paletteItem
	for _, n := range config.Names(s.svc.Catalog().Plans) {
		b, _ := json.Marshal(preview.Selection{Plan: n})
		items = append(items, paletteItem{Label: "Run " + n, Kind: "plan", Post: apiExecutions, Body: string(b), Aliases: "profile"})
	}
	running := 0
	for i, r := range s.svc.Executions() {
		if !r.Status.Done() && !waiting(r) {
			running++
		}
		if i >= 10 {
			continue
		}
		items = append(items,
			paletteItem{Label: "Open #" + r.ID, Kind: r.Label, Href: string(executionURL(r.ID)), Aliases: "run"},
			paletteItem{Label: "Run #" + r.ID + " again", Kind: "same selection", Post: executionAPI(r.ID, "run-again"), Aliases: "rerun re-run"})
	}
	items = append(items, paletteItem{Label: "Run a plan…", Kind: "page", Href: string(runTestsURL("", nil)), Aliases: "new run tests"})
	for _, t := range navTabs {
		items = append(items, paletteItem{Label: "Go to " + t.Label, Kind: "page", Href: t.Href, Aliases: navAliases[t.ID]})
	}
	items = append(items, paletteItem{Label: "Go to Config files", Kind: "page", Href: configPath, Aliases: "catalog"})
	// json.Marshal escapes <, > and &, so the data cannot close its script tag.
	b, _ := json.Marshal(items)
	q := r.URL.Query()
	q.Del("token")
	self := r.URL.Path
	if len(q) > 0 {
		self += "?" + q.Encode()
	}
	// The count matches the Running filter the indicator links to, so an
	// execution waiting for someone is not counted.
	l := layout{
		Tab: tab, Self: self, Running: running,
		ConfigErrors: len(s.svc.Catalog().AllErrors()), Palette: string(b),
	}
	if events != "" {
		l.Stream, l.Events, l.Refresh = apiConfigEvents, events, true
	}
	return l
}

// assets maps each static file to its URL, versioned by its content so
// browsers can keep it rather than fetch it again on every page.
var assets = func() map[string]string {
	m := map[string]string{}
	_ = fs.WalkDir(static, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := static.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		name := strings.TrimPrefix(p, "static/")
		m[name] = "/static/" + name + "?v=" + hex.EncodeToString(sum[:6])
		return nil
	})
	return m
}()

func asset(name string) string { return assets[name] }

func (s *Server) pages(mux *http.ServeMux) {
	sub, _ := fs.Sub(static, "static")
	files := http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
	mux.Handle("GET /static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A versioned URL names one content forever.
		if r.URL.Query().Has("v") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	}))
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /executions", s.executionsPage)
	mux.HandleFunc("GET /executions/{id}", s.executionPage)
	mux.HandleFunc("GET /run", s.runTestsPage)
	mux.HandleFunc("GET /run/{plan}", s.runTestsPage)
	mux.HandleFunc("GET /plans", s.entriesTab("plans"))
	mux.HandleFunc("GET /plans/{name}", s.entriesTab("plans"))
	mux.HandleFunc("GET /testbeds", s.entriesTab("testbeds"))
	mux.HandleFunc("GET /testbeds/{sub}", s.entriesTab("testbeds"))
	mux.HandleFunc("GET /testbeds/{sub}/{name}", s.entriesTab("testbeds"))
	mux.HandleFunc("GET /suites", s.entriesTab("suites"))
	mux.HandleFunc("GET /suites/{sub}", s.entriesTab("suites"))
	mux.HandleFunc("GET /suites/{sub}/{name}", s.entriesTab("suites"))
	mux.HandleFunc("GET /config", s.configPage)
	s.editRoutes(mux)
}

func render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Formatting helpers for the templates.

func dur(ms int64) string         { return ui.Dur(ms) }
func cls(s event.Status) string   { return vocab.StatusOf(s).Class }
func word(s event.Status) string  { return vocab.StatusOf(s).Word }
func glyph(s event.Status) string { return vocab.StatusOf(s).Glyph }
func ago(t time.Time) string      { return ui.Ago(t) }

func width(done, total int) templ.SafeCSS {
	p := 0
	if total > 0 {
		p = min(done*100/total, 100)
	}
	return templ.SafeCSS(fmt.Sprintf("width:%d%%", p))
}

func indent(depth int) templ.SafeCSS {
	return templ.SafeCSS(fmt.Sprintf("padding-left:%dpx", 6+18*depth))
}

func elapsed(s *engine.Summary) int64 {
	if s.Status.Done() || s.Started.IsZero() {
		return s.Ms
	}
	return time.Since(s.Started).Milliseconds()
}

func vals(kv ...string) string {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// pathLabel is an event path for people: Set up testbed › wait, not
// setup › setup › wait.
func pathLabel(p []string) string {
	var out []string
	for i, s := range p {
		switch {
		case i == 0:
			s = vocab.Phase(s)
		case s == p[i-1]:
			continue
		}
		out = append(out, s)
	}
	return strings.Join(out, " › ")
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return s
}

func combinationCounts(s *engine.Summary) string {
	if len(s.Combinations) < 2 {
		return ""
	}
	n := map[string]int{}
	for _, c := range s.Combinations {
		n[glyph(c.Status)]++
	}
	var out []string
	for _, st := range []event.Status{event.Passed, event.Failed, event.Errored} {
		if g := glyph(st); n[g] > 0 {
			out = append(out, fmt.Sprintf("%d%s", n[g], g))
		}
	}
	return strings.Join(out, " ")
}

func kept(s *engine.Summary) bool {
	for _, c := range s.Combinations {
		if c.Kept || c.Paused {
			return true
		}
	}
	return false
}

func paused(s *engine.Summary) bool {
	for _, c := range s.Combinations {
		if c.Paused {
			return true
		}
	}
	return false
}

// waiting reports whether an execution is paused on a failure, waiting for
// someone to retry, continue or abort.
func waiting(s *engine.Summary) bool {
	return s.Status == event.Paused || (paused(s) && !s.Status.Done())
}

func planLine(p config.Plan) string {
	target := p.Testbed
	if len(p.Combinations) > 0 {
		target += " (" + strings.Join(p.Combinations, ", ") + ")"
	}
	line := target + " · " + strings.Join(p.Suites, ", ")
	if p.Parallel > 1 {
		line += fmt.Sprintf(" · parallel %d", p.Parallel)
	}
	return line
}

func (s *Server) estimate(key string) (time.Duration, int) {
	past := engine.PastDurations(s.svc.Engine.Data, key)
	if len(past) == 0 {
		return 0, 0
	}
	p := &preview.Preview{}
	p.Estimate(past)
	return p.Needs.Estimate, len(past)
}

func (s *Server) eta(sum *engine.Summary) string {
	est, n := s.estimate(sum.Key)
	if n == 0 {
		return ""
	}
	if left := est - time.Since(sum.Started); left > time.Minute {
		return fmt.Sprintf("about %d min left", int(left.Minutes()))
	}
	return ""
}

func exportLine(dir string) string {
	return "export KUBECONFIG=" + filepath.Join(dir, "kubeconfigs", engine.ManagementCluster)
}

// Home.

// planRow is a plan as Home lists it.
type planRow struct {
	Name, Desc string
	Last       *engine.Summary
	Typical    string
}

// runningExecution is an execution in Home's Running card, with the step it
// is on.
type runningExecution struct {
	Sum *engine.Summary
	Now string
	ETA string
}

// waitingExecution is an execution paused after a failure, waiting for
// someone to retry, continue or abort: Home's banner.
type waitingExecution struct {
	Sum         *engine.Summary
	Combination string
	Failure     *engine.Failure
}

type homeData struct {
	First       bool
	Dirs        []string
	StarterPath string
	Checks      []hostcheck.Check
	Quick       *planRow
	Plans       []planRow
	Waiting     []waitingExecution
	Running     []runningExecution
	Kept        []service.KeptTestbed
	Recent      []*engine.Summary
}

// recentOnHome is how many finished executions Home lists.
const recentOnHome = 5

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	cat := s.svc.Catalog()
	d := homeData{First: !cat.HasUserConfig, Dirs: cat.Dirs, StarterPath: filepath.Join(s.svc.StarterDir, "wendy.cue")}
	for _, n := range config.Names(cat.Plans) {
		p := cat.Plans[n].Value
		row := planRow{Name: n, Desc: p.Description, Last: s.svc.LastResult(n)}
		if row.Desc == "" {
			row.Desc = planLine(p)
		}
		if est, k := s.estimate(preview.PlanKey(n)); k > 0 {
			row.Typical = ui.Dur(est.Milliseconds())
		}
		d.Plans = append(d.Plans, row)
		if n == "quickstart" {
			d.Quick = &row
		}
	}
	if d.First {
		d.Checks = shownChecks(s.svc.FirstChecks(r.Context()))
	}
	for _, sum := range s.svc.Executions() {
		switch {
		case waiting(sum):
			d.Waiting = append(d.Waiting, waitingOf(sum))
		case !sum.Status.Done():
			d.Running = append(d.Running, s.runningOf(sum))
		case len(d.Recent) < recentOnHome:
			d.Recent = append(d.Recent, sum)
		}
	}
	for _, e := range s.svc.KeptTestbeds() {
		if !e.Paused {
			d.Kept = append(d.Kept, e)
		}
	}
	render(w, r, homePage(s.layout(r, "home", onAnyChange), d))
}

// waitingOf is a paused execution's banner: the combination it paused on and
// that combination's failure.
func waitingOf(sum *engine.Summary) waitingExecution {
	w := waitingExecution{Sum: sum}
	for _, c := range sum.Combinations {
		if c.Paused && w.Combination == "" {
			w.Combination, w.Failure = c.Name, c.Failure
		}
	}
	return w
}

func (s *Server) runningOf(sum *engine.Summary) runningExecution {
	e := runningExecution{Sum: sum, ETA: s.eta(sum)}
	if v, err := ui.Load(s.svc.ExecutionDir(sum.ID)); err == nil && v.Running != "" {
		e.Now = pathLabel(v.ByKey[v.Running].Path)
	}
	return e
}

// shownChecks drops optional checks that passed or don't apply yet, keeping
// the host check to what can stop the first execution.
func shownChecks(cs []hostcheck.Check) []hostcheck.Check {
	var out []hostcheck.Check
	seen := map[string]bool{}
	for _, c := range cs {
		if seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		out = append(out, c)
	}
	return out
}

// Executions.

type executionsData struct {
	Rows   []executionRow
	Q      string
	Status string
	// Any is whether there are executions at all, so an empty list can say
	// whether the filter hid them or there are none yet.
	Any bool
}

// executionRow is an execution as the Executions list shows it. A plan or
// testbed the config no longer has shows without a link.
type executionRow struct {
	Sum                 *engine.Summary
	Plan, Testbed       string
	PlanURL, TestbedURL templ.SafeURL
}

// executionFilter is a status segment of the Executions list: its query
// value, its label and which executions it lists.
type executionFilter struct {
	Value, Label string
	match        func(*engine.Summary) bool
}

var executionFilters = []executionFilter{
	{"", "All", func(*engine.Summary) bool { return true }},
	{"running", capital(word(event.Running)), func(s *engine.Summary) bool { return !s.Status.Done() && !waiting(s) }},
	{"paused", "Waiting for you", waiting},
	{"passed", capital(word(event.Passed)), func(s *engine.Summary) bool { return s.Status == event.Passed }},
	{"failed", capital(word(event.Failed)), func(s *engine.Summary) bool { return s.Status == event.Failed || s.Status == event.TimedOut }},
	{"errored", capital(word(event.Errored)), func(s *engine.Summary) bool { return s.Status == event.Errored }},
	{"kept", "Kept testbeds", kept},
}

func (s *Server) executionsPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	d := executionsData{Q: q.Get("q"), Status: q.Get("status")}
	f := executionFilters[0]
	for _, ef := range executionFilters {
		if ef.Value == d.Status {
			f = ef
		}
	}
	d.Status = f.Value
	cat := s.svc.Catalog()
	for _, sum := range s.svc.Executions() {
		d.Any = true
		row := executionRowOf(cat, sum)
		if f.match(sum) && row.matches(d.Q) {
			d.Rows = append(d.Rows, row)
		}
	}
	render(w, r, executionsPage(s.layout(r, "executions", onAnyChange), d))
}

// executionRowOf is how the list shows sum: its plan, and its testbed, which
// is the selection's or else its plan's.
func executionRowOf(cat *config.Catalog, sum *engine.Summary) executionRow {
	row := executionRow{Sum: sum, Plan: sum.Selection.Plan, Testbed: sum.Selection.Testbed}
	if p, ok := cat.Plans[row.Plan]; ok {
		row.PlanURL = entryURL(config.KindPlan, row.Plan)
		row.Testbed = cmp(row.Testbed, p.Value.Testbed)
	}
	if _, ok := cat.Testbeds[row.Testbed]; ok {
		row.TestbedURL = entryURL(config.KindTestbed, row.Testbed)
	}
	return row
}

// matches reports whether the row passes the list's filter: "#41" matches
// the executions whose number starts with 41, and other text any execution
// whose number, plan or testbed contains it.
func (row executionRow) matches(q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if n, ok := strings.CutPrefix(q, "#"); ok {
		return strings.HasPrefix(row.Sum.ID, n)
	}
	return strings.Contains(strings.ToLower(row.Sum.ID+" "+row.Plan+" "+row.Testbed), q)
}

func statusURL(q, status string) templ.SafeURL {
	v := url.Values{}
	if q != "" {
		v.Set("q", q)
	}
	if status != "" {
		v.Set("status", status)
	}
	return executionsURL(v)
}

// Execution.

type link struct {
	Label string
	Href  templ.SafeURL
}

type executionData struct {
	Sum               *engine.Summary
	View              *ui.ExecutionView
	Row               executionRow
	Self              string
	Rows              []ui.Row
	Sel               *ui.Node
	Follow            bool
	Grid              bool
	Cols              []string
	Failure           *engine.Failure
	FailedCombination string
	Paused            bool
	// Kept are the execution's kept or paused testbeds. InView is the
	// combination whose card is open, and Combination the one the URL named.
	Kept        []service.KeptTestbed
	InView      string
	Combination string
	Export      string
	Kubeconfig  templ.SafeURL
	Consoles    []link
	Logs        string
	Live        bool
	ETA         string
	Report      bool
	Failed      int
}

func (d executionData) nodeURL(key string) templ.SafeURL {
	return executionURL(d.Sum.ID) + templ.SafeURL("?node="+url.QueryEscape(key))
}

func (d executionData) combinationURL(name string) templ.SafeURL {
	for _, n := range d.View.Root {
		if n.Kind == ui.KindCombination && n.Name == name {
			return d.nodeURL(n.Key)
		}
	}
	return executionURL(d.Sum.ID)
}

// viewURL shows the execution as a grid or a tree, keeping the kept testbed
// card the URL opened.
func (d executionData) viewURL(view string) templ.SafeURL {
	return d.pageURL(view, d.Combination)
}

// keptURL opens a combination's kept testbed card, in the same view.
func (d executionData) keptURL(combination string) templ.SafeURL {
	view := "tree"
	if d.Grid {
		view = "grid"
	}
	return d.pageURL(view, combination)
}

func (d executionData) pageURL(view, combination string) templ.SafeURL {
	q := url.Values{"view": {view}}
	if combination != "" {
		q.Set("combination", combination)
	}
	return executionURL(d.Sum.ID) + templ.SafeURL("?"+q.Encode())
}

// keptInView is the combination whose kept testbed card is open: the first of
// names that kept its testbed, else the first kept one.
func keptInView(kept []service.KeptTestbed, names ...string) string {
	for _, n := range names {
		for _, k := range kept {
			if n != "" && k.Combination == n {
				return n
			}
		}
	}
	if len(kept) > 0 {
		return kept[0].Combination
	}
	return ""
}

// showKept shows an execution's kept testbeds. The card open is the one named
// (by the URL), else the picked node's, else the failed combination's.
func (d *executionData) showKept(kept []service.KeptTestbed, named string) {
	d.Kept = kept
	if keptInView(kept, named) == named {
		d.Combination = named
	}
	picked := ""
	if !d.Follow && d.Sel != nil {
		picked = d.Sel.Combination
	}
	d.InView = keptInView(kept, d.Combination, picked, d.FailedCombination)
}

func (d executionData) eventsURL() string {
	return fmt.Sprintf("%s?as=update&after=%d", executionAPI(d.Sum.ID, "events"), d.View.LastSeq)
}

func (d executionData) file(rel string) templ.SafeURL {
	return executionFileURL(d.Sum.ID, rel)
}

func (d executionData) action(a string) string {
	return decisionAPI(d.Sum.ID, d.FailedCombination, a)
}

func (d executionData) cell(c engine.CombinationSummary, col string) event.Status {
	if st, ok := c.Nodes[col]; ok {
		return st
	}
	if c.Status.Done() {
		return event.Skipped
	}
	return event.Waiting
}

func colName(c string) string { return vocab.Phase(c) }

// diagnosticKinds are the files collecting diagnostics finds, in the order the
// failure card links them, with their labels.
var diagnosticKinds = []struct{ Kind, Label string }{
	{"workflow", "Workflow"}, {"console", "Console"}, {"bmc", "BMC log"}, {"dump", "Cluster dump"},
}

// diagnostics are the failure's diagnostics as links: the known kinds first,
// then any other.
func (d executionData) diagnostics() []link {
	var out []link
	if d.Failure == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, k := range diagnosticKinds {
		if p := d.Failure.Diagnostics[k.Kind]; p != "" {
			seen[k.Kind] = true
			out = append(out, link{k.Label, d.file(p)})
		}
	}
	for _, k := range config.Names(d.Failure.Diagnostics) {
		if !seen[k] {
			out = append(out, link{k, d.file(d.Failure.Diagnostics[k])})
		}
	}
	return out
}

func (d executionData) failureTitle() string { return failureTitle(d.Failure) }

// failureTitle says what failed, how, and after how long.
func failureTitle(f *engine.Failure) string {
	verb := "Failed"
	switch f.Status {
	case event.Errored:
		verb = "Errored"
	case event.TimedOut:
		verb = "Timed out"
	default:
	}
	return fmt.Sprintf("%s %s after %s: %s", glyph(f.Status), verb, ui.Dur(f.Ms), pathLabel(f.Path))
}

func (s *Server) executionPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sum, err := s.svc.Execution(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	executionDir := s.svc.ExecutionDir(id)
	v, err := ui.Load(executionDir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	d := executionData{Sum: sum, View: v, Live: !sum.Status.Done(), Self: s.layout(r, "", "").Self}
	d.Row = executionRowOf(s.svc.Catalog(), sum)
	d.Grid = q.Get("view") == "grid" || (q.Get("view") == "" && q.Get("node") == "" && len(sum.Combinations) > 1)
	for _, c := range sum.Combinations {
		if c.Failure != nil && (d.Failure == nil || c.Paused) {
			d.Failure, d.FailedCombination, d.Paused = c.Failure, c.Name, c.Paused
		}
		switch c.Status {
		case event.Failed, event.Errored, event.TimedOut:
			d.Failed++
		default:
		}
	}
	key := q.Get("node")
	d.Follow = key == ""
	if d.Follow {
		switch {
		case v.Running != "":
			key = v.Running
		case d.Failure != nil:
			for k, n := range v.ByKey {
				if n.Combination == d.FailedCombination && slices.Equal(n.Path, d.Failure.Path) {
					key = k
				}
			}
		case len(v.Root) > 0:
			key = v.Root[len(v.Root)-1].Key
		}
	}
	expanded := map[string]bool{}
	if n := v.ByKey[key]; n != nil {
		d.Sel = n
		for p := n; p != nil; p = p.Parent() {
			expanded[p.Key] = true
		}
	}
	d.Rows = v.Rows(expanded)
	d.showKept(s.svc.KeptTestbedsOf(sum), q.Get("combination"))

	combinationDir := ""
	for _, c := range sum.Combinations {
		if combinationDir == "" || (d.Sel != nil && c.Name == d.Sel.Combination) || (d.Sel == nil && c.Name == d.FailedCombination) {
			combinationDir = c.Dir
		}
	}
	if combinationDir != "" {
		d.combinationFiles(executionDir, combinationDir, d.Live || kept(sum))
	}
	if d.Sel != nil {
		d.Logs = logText(d.Sel)
	}
	d.Cols = gridColumns(sum, v)
	if d.Live {
		d.ETA = s.eta(sum)
	}
	// Service.ExecutionDir keeps executionDir inside the executions directory.
	if _, err := os.Stat(filepath.Join(executionDir, "report.junit.xml")); err == nil { //nolint:gosec // G703: see above.
		d.Report = true
	}
	if r.Header.Get("HX-Request") != "" && r.Header.Get("HX-Target") == "execution-body" {
		render(w, r, executionBody(d))
		return
	}
	l := s.layout(r, "executions", "")
	if d.Live {
		l.Stream, l.Events = d.eventsURL(), "update"
	}
	render(w, r, executionPage(l, d))
}

// combinationFiles links a combination's consoles and, while its testbed is
// up, how to reach its clusters.
func (d *executionData) combinationFiles(executionDir, combinationDir string, testbedUp bool) {
	// Service.ExecutionDir keeps executionDir inside the executions directory.
	dir := filepath.Join(executionDir, combinationDir)
	if testbedUp {
		d.Export = exportLine(dir)
		if _, err := os.Stat(filepath.Join(dir, "kubeconfigs", engine.ManagementCluster)); err == nil { //nolint:gosec // G703: see above.
			d.Kubeconfig = d.file(filepath.Join(combinationDir, "kubeconfigs", engine.ManagementCluster))
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "consoles", "*.log"))
	for _, f := range files {
		d.Consoles = append(d.Consoles, link{strings.TrimSuffix(filepath.Base(f), ".log"), d.file(filepath.Join(combinationDir, "consoles", filepath.Base(f)))})
	}
}

// logText is the tail of a node's log, or of its running child's when it
// has none of its own.
func logText(n *ui.Node) string {
	logs := n.Logs
	for _, c := range n.Children {
		if len(logs) == 0 && c.Status == event.Running {
			logs = c.Logs
		}
	}
	if len(logs) > 400 {
		logs = logs[len(logs)-400:]
	}
	var b strings.Builder
	for _, e := range logs {
		b.WriteString(e.TS.Local().Format("15:04:05") + "  " + e.Msg + "\n")
	}
	if n.Status == event.Running {
		b.WriteString("▌")
	}
	return b.String()
}

// gridColumns are the grid's columns: build when anything is built, setup,
// then every suite in the order the combinations run them.
func gridColumns(sum *engine.Summary, v *ui.ExecutionView) []string {
	var cols []string
	if len(sum.Builds) > 0 {
		cols = append(cols, engine.PhaseBuild)
	}
	cols = append(cols, engine.PhaseSetup)
	for _, root := range v.Root {
		if root.Kind != ui.KindCombination {
			continue
		}
		for _, ch := range root.Children {
			if ch.Kind == ui.KindSuite && !slices.Contains(cols, ch.Name) && !isPhase(ch.Name) {
				cols = append(cols, ch.Name)
			}
		}
	}
	return cols
}

func isPhase(name string) bool {
	switch name {
	case engine.PhaseSetup, engine.PhaseTeardown, engine.PhaseDiagnostics, engine.PhaseAdhoc:
		return true
	}
	return false
}

// Run tests.

type facetValue struct {
	Value string
	On    bool
}

type facet struct {
	Axis   string
	Values []facetValue
}

// suiteChoice is a suite the form offers, with what it tests.
type suiteChoice struct{ Name, Desc string }

type runTestsData struct {
	// From is the plan the form started from, if any.
	From string
	// FormURL is where the form re-renders itself when the testbed changes:
	// Run tests, or the plan form it is part of.
	FormURL string
	// Plans are the saved plans the Plan card offers.
	Plans       []string
	Testbed     string
	TestbedDesc string
	// HasMatrix is whether the testbed has combinations to pick from.
	HasMatrix    bool
	Combinations string
	NamesExample string
	Facets       []facet
	Testbeds     []string
	Suites       []suiteChoice
	Picked       []string
	LabelFilter  string
	Labels       []string
	Parallel     int
	Keep         string
	Pause        bool
	// Diagnostics and Consoles are the diagnostics the form shows: the plan's
	// (or the defaults), or what the form was last set to.
	Diagnostics string
	Consoles    bool
	Preview     *service.Preview
	Problem     string
	Total       int
}

var (
	errNoTestbed     = errors.New("pick a testbed")
	errNoSuite       = errors.New("pick at least one suite")
	errNoCombination = errors.New("no combination matches; turn an axis value back on or change the names")
)

func (s *Server) runTestsPage(w http.ResponseWriter, r *http.Request) {
	// The Plan card's picker names its plan in the query.
	from := cmp(r.PathValue("plan"), r.URL.Query().Get("from"))
	if _, ok := s.svc.Catalog().Plans[from]; from != "" && !ok {
		http.NotFound(w, r)
		return
	}
	d := s.runForm(r, from)
	d.FormURL = runTestsPath
	d.Plans = config.Names(s.svc.Catalog().Plans)
	render(w, r, runTestsPage(s.layout(r, "", ""), d))
}

// runForm fills in the cards of the Run tests form and of a plan's form:
// from the plan named from, or from what the form posted when it re-renders
// itself, with the preview of what it would run.
func (s *Server) runForm(r *http.Request, from string) runTestsData {
	cat := s.svc.Catalog()
	q := r.URL.Query()
	d := runTestsData{From: from, Testbeds: config.Names(cat.Testbeds), Labels: labelsInUse(cat)}
	for _, n := range config.Names(cat.Suites) {
		d.Suites = append(d.Suites, suiteChoice{n, cat.Suites[n].Value.Description})
	}
	var sel preview.Selection
	switch p, ok := cat.Plans[d.From]; {
	case q.Has("suitesSet"):
		// The form re-rendering itself, e.g. after picking another testbed.
		sel, _ = selection(r)
		d.From = q.Get("plan")
		d.Pause = q.Get("pauseOnFailure") == "on"
	case ok:
		pr := p.Value
		sel = preview.Selection{
			Plan: d.From, Testbed: pr.Testbed, Combinations: pr.Combinations,
			Suites: pr.Suites, LabelFilter: pr.LabelFilter, Parallel: pr.Parallel, Keep: pr.Keep,
		}
		d.Pause = pr.PauseOnFailure
	default:
		sel = preview.Selection{Testbed: q.Get("testbed"), Suites: q["suite"]}
	}
	// A blank form picks no testbed rather than guess one.
	d.Testbed = sel.Testbed
	if len(sel.Suites) == 0 && !q.Has("suitesSet") {
		sel.Suites = []string{"smoke"}
	}
	d.Picked = sel.Suites
	d.LabelFilter = sel.LabelFilter
	d.Parallel = max(sel.Parallel, 1)
	d.Keep = cmp(sel.Keep, "never")
	var problem error
	if tb := cat.Testbeds[d.Testbed]; tb != nil {
		d.TestbedDesc = tb.Value.Description
		if tb.HasMatrix() {
			axes, set := pickedAxes(q, d.Testbed)
			d.HasMatrix = true
			d.Combinations = strings.Join(sel.Combinations, " ")
			d.Facets = facets(tb.Value.Matrix, axes, set)
			d.Total, d.NamesExample = selectable(tb)
			sel, problem = applyAxes(sel, tb, axes, set)
		}
	}
	if !d.HasMatrix {
		// What was picked of another testbed's matrix does not apply.
		sel.Combinations = nil
	}
	base := s.baseDiagnostics(sel.Plan)
	diag := base
	if sel.Diagnostics != nil {
		diag = *sel.Diagnostics
	}
	d.Diagnostics, d.Consoles = diag.Collect, diag.Consoles
	if diag == base {
		sel.Diagnostics = nil
	} else {
		sel.Diagnostics = &diag
	}
	pause := d.Pause
	sel.PauseOnFailure = &pause
	switch {
	case problem != nil:
	case sel.Testbed == "":
		problem = errNoTestbed
	case len(sel.Suites) == 0:
		problem = errNoSuite
	}
	if problem == nil {
		d.Preview, problem = s.svc.Preview(r.Context(), sel)
	}
	if problem != nil {
		d.Problem = problem.Error()
	}
	return d
}

func cmp(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// labelsInUse are the labels the tests carry, for the label field's help.
func labelsInUse(cat *config.Catalog) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range cat.Tests {
		for _, l := range t.Value.Labels {
			if !seen[l] {
				seen[l] = true
				out = append(out, l)
			}
		}
	}
	slices.Sort(out)
	return out
}

// selectable is how many combinations of a testbed can run (those not
// excluded), and a names pattern to show as an example.
func selectable(tb *config.TestbedItem) (int, string) {
	n, example := 0, ""
	for _, c := range tb.Combinations {
		if c.Excluded != "" {
			continue
		}
		n++
		if example == "" {
			example = c.Name + "*"
			if i := strings.Index(c.Name, "-"); i > 0 {
				example = c.Name[:i+1] + "*"
			}
		}
	}
	return n, example
}

// pickedAxes are the axis values the form leaves on for a testbed, and
// whether the form says: a form drawn for another testbed, or none, leaves
// them all on.
func pickedAxes(form url.Values, testbed string) ([]string, bool) {
	if form.Get("axesOf") != testbed {
		return nil, false
	}
	return form["axis"], true
}

// facets are a matrix's axes and their values, each on when the form left it
// on, or all on when the form says nothing.
func facets(m *config.Matrix, picked []string, set bool) []facet {
	order := slices.Clone(m.Order)
	for _, a := range config.Names(m.Axes) {
		if !slices.Contains(order, a) {
			order = append(order, a)
		}
	}
	var out []facet
	for _, a := range order {
		f := facet{Axis: a}
		for _, v := range config.Names(m.Axes[a]) {
			f.Values = append(f.Values, facetValue{v, !set || slices.Contains(picked, a+"="+v)})
		}
		if len(f.Values) > 0 {
			out = append(out, f)
		}
	}
	return out
}

// applyAxes narrows a selection of a testbed with a matrix to the
// combinations whose every axis value is on and whose name the selection's
// patterns match. With every value on it leaves the selection as it is.
func applyAxes(sel preview.Selection, tb *config.TestbedItem, picked []string, set bool) (preview.Selection, error) {
	if !set {
		return sel, nil
	}
	on := map[string]map[string]bool{}
	narrowed := false
	for axis, values := range tb.Value.Matrix.Axes {
		on[axis] = map[string]bool{}
		for v := range values {
			on[axis][v] = slices.Contains(picked, axis+"="+v)
			narrowed = narrowed || !on[axis][v]
		}
	}
	if !narrowed {
		return sel, nil
	}
	var names []string
	for _, c := range tb.Combinations {
		ok := c.Excluded == "" && nameMatches(c.Name, sel.Combinations)
		for axis, v := range c.Axes {
			if vs, isAxis := on[axis]; isAxis {
				ok = ok && vs[v]
			}
		}
		if ok {
			names = append(names, c.Name)
		}
	}
	if len(names) == 0 {
		return sel, errNoCombination
	}
	sel.Combinations = names
	return sel, nil
}

// nameMatches reports whether a combination's name matches any of patterns,
// as the preview matches them; no patterns match every name.
func nameMatches(name string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// baseDiagnostics are the diagnostics an execution has when the form leaves
// them alone: its plan's, or the defaults.
func (s *Server) baseDiagnostics(plan string) config.Diagnostics {
	if p := s.svc.Catalog().Plans[plan]; p != nil {
		return p.Value.Collects()
	}
	return config.DefaultDiagnostics()
}

// formSelection is selection plus what the Run tests form adds: the axis
// values left on, and that at least one suite is picked.
func (s *Server) formSelection(r *http.Request) (preview.Selection, error) {
	sel, err := selection(r)
	if err != nil {
		return sel, err
	}
	// A form always posts diagnostics. Only what differs from the plan's (or
	// the default) is an override, so an untouched form stays an unedited plan.
	if sel.Diagnostics != nil && *sel.Diagnostics == s.baseDiagnostics(sel.Plan) {
		sel.Diagnostics = nil
	}
	if r.Form == nil {
		return sel, nil
	}
	if r.Form.Has("suitesSet") && sel.Testbed == "" {
		return sel, errNoTestbed
	}
	if r.Form.Has("suitesSet") && len(sel.Suites) == 0 {
		return sel, errNoSuite
	}
	tb := s.svc.Catalog().Testbeds[sel.Testbed]
	if tb == nil || !tb.HasMatrix() {
		return sel, nil
	}
	axes, set := pickedAxes(r.Form, sel.Testbed)
	return applyAxes(sel, tb, axes, set)
}

func (s *Server) previewFragment(w http.ResponseWriter, r *http.Request, pv *service.Preview, err error) {
	problem := ""
	if err != nil {
		problem = err.Error()
	}
	total := 0
	if pv != nil && pv.Preview != nil {
		if tb := s.svc.Catalog().Testbeds[pv.Preview.Selection.Testbed]; tb != nil && tb.HasMatrix() {
			total, _ = selectable(tb)
		}
	}
	render(w, r, previewFragment(pv, problem, total))
}

func needsLine(p *preview.Preview) string {
	n := p.Needs
	s := fmt.Sprintf("%d VMs · %d GiB memory · %d GiB disk", n.VMs, (n.RAMMiB+1023)/1024, n.DiskGiB)
	if at := min(p.Parallel, len(p.Combinations)); at > 1 {
		s += fmt.Sprintf(" (%d combinations at once)", at)
	}
	return s
}

// testChain is a suite's tests in order, each skipped one with why.
func testChain(s preview.Suite) string {
	var out []string
	for _, t := range s.Tests {
		if t.Skip != "" {
			out = append(out, t.Name+" (skipped: "+t.Skip+")")
		} else {
			out = append(out, t.Name)
		}
	}
	return strings.Join(out, " › ")
}

// testCount is how many tests a preview runs on each combination.
func testCount(p *preview.Preview) int {
	if len(p.Combinations) == 0 {
		return 0
	}
	n := 0
	for _, s := range p.Combinations[0].Suites {
		for _, t := range s.Tests {
			if t.Skip == "" {
				n++
			}
		}
	}
	return n
}

func sameAsFirst(p *preview.Preview, i int) bool {
	if i == 0 {
		return false
	}
	a, b := p.Combinations[0].Suites, p.Combinations[i].Suites
	if len(a) != len(b) {
		return false
	}
	for j := range a {
		if a[j].Name != b[j].Name || testChain(a[j]) != testChain(b[j]) {
			return false
		}
	}
	return true
}

// buildsLine is what the preview builds from source, or "" for nothing.
func buildsLine(p *preview.Preview) string {
	var out []string
	for _, b := range p.Builds {
		s := b.Component + " from " + b.Source.Repo
		if b.Source.Ref != "" {
			s += "@" + b.Source.Ref
		}
		out = append(out, s)
	}
	return strings.Join(out, ", ")
}

func takesLine(n preview.Needs) string {
	if n.Samples == 0 {
		return "Unknown until it has run once"
	}
	return fmt.Sprintf("%s (from %s)", ui.Dur(n.Estimate.Milliseconds()), vocab.Count(n.Samples, "past execution", "past executions"))
}

// previewNotes are what to know before running a preview: its warnings,
// what keeping every testbed and pausing alongside parallel combinations
// mean, and why a host check failed.
func previewNotes(pv *service.Preview) []string {
	p := pv.Preview
	notes := slices.Clone(p.Warnings)
	if p.Keep == "always" {
		notes = append(notes, "Every combination's testbed stays up after it finishes. Tear them down from Home when you are done.")
	}
	if p.PauseOnFailure && min(p.Parallel, len(p.Combinations)) > 1 {
		notes = append(notes, "A paused combination keeps its machines and its place, so fewer combinations run at once until you decide.")
	}
	for _, c := range pv.Checks {
		if !c.OK && c.Detail != "" {
			notes = append(notes, c.Name+": "+c.Detail)
		}
	}
	return notes
}

// Plans, Testbeds and Suites.

// configTabs are the top-bar tabs that list config entries, each with the
// subtabs below.
var configTabs = []string{"plans", "testbeds", "suites"}

// subtab is one subtab of a config tab: its kind, what it is called,
// which top-bar tab it sits in, what it holds, and how to list its entries.
type subtab struct {
	ID, Label, Top, Blurb string
	// entries maps each entry's name to the file that defines it.
	entries func(*config.Catalog) map[string]string
}

// subtabs is every subtab, in display order. Adding one is one row
// here.
var subtabs = []subtab{
	kindSubtab(config.KindPlan, func(c *config.Catalog) map[string]string { return originsOf(c.Plans) }),
	kindSubtab(config.KindTestbed, func(c *config.Catalog) map[string]string {
		out := map[string]string{}
		for n, it := range c.Testbeds {
			out[n] = it.Origin
		}
		return out
	}),
	kindSubtab(config.KindTinkerbell, partOrigins(config.KindTinkerbell)),
	kindSubtab(config.KindClusterAPI, partOrigins(config.KindClusterAPI)),
	kindSubtab(config.KindManagementCluster, partOrigins(config.KindManagementCluster)),
	kindSubtab(config.KindWorkloadCluster, partOrigins(config.KindWorkloadCluster)),
	kindSubtab(config.KindHardware, partOrigins(config.KindHardware)),
	kindSubtab("suites", func(c *config.Catalog) map[string]string { return originsOf(c.Suites) }),
	kindSubtab("tests", func(c *config.Catalog) map[string]string { return originsOf(c.Tests) }),
	kindSubtab(config.KindStep, func(c *config.Catalog) map[string]string {
		out := originsOf(c.Steps)
		for _, st := range c.BuiltinSteps {
			out[st.ID] = config.BuiltIn
		}
		return out
	}),
}

// kindSubtab is the subtab of the vocab kind with that ID, in the top-bar tab
// that lists its kind.
func kindSubtab(id string, entries func(*config.Catalog) map[string]string) subtab {
	k, ok := vocab.KindOf(id)
	if !ok {
		panic("no vocab kind " + id)
	}
	return subtab{ID: k.ID, Label: k.Label, Top: configTab(id), Blurb: k.Blurb, entries: entries}
}

// partOrigins lists the parts of a kind for a tab.
func partOrigins(kind string) func(*config.Catalog) map[string]string {
	return func(c *config.Catalog) map[string]string { return originsOf(c.Parts(kind)) }
}

func originsOf[T any](m map[string]*config.Item[T]) map[string]string {
	out := make(map[string]string, len(m))
	for n, it := range m {
		out[n] = it.Origin
	}
	return out
}

// subtabOf is the subtab of a kind.
func subtabOf(id string) (subtab, bool) {
	for _, t := range subtabs {
		if t.ID == id {
			return t, true
		}
	}
	return subtab{}, false
}

// subtabsOf are the subtabs of a top-bar tab, in order.
func subtabsOf(group string) []subtab {
	var out []subtab
	for _, t := range subtabs {
		if t.Top == group {
			out = append(out, t)
		}
	}
	return out
}

type entryItem struct {
	Name, Origin string
}

type entriesData struct {
	// Top is the top-bar tab, and Tab the kind of the subtab shown in it.
	Top string
	Tab string
	// Sub is the subtab being shown: its label, top-bar tab and blurb.
	Sub   subtab
	Q     string
	Items []entryItem
	// Total is how many entries the subtab has, before the filter.
	Total  int
	Sel    string
	Origin string
	Cat    *config.Catalog
	Source string
	// Editable is whether the UI file defines the selected entry.
	Editable bool
	// UsedBy are the entries that refer to the selected one, as "kind name".
	UsedBy []string
	// Last is the selected plan's newest finished, unchanged execution.
	Last *engine.Summary
}

// canEdit is whether the tab holds something the UIs can create and edit.
func (d entriesData) canEdit() bool { return slices.Contains(editable, d.Tab) }

// testbedPart is one part a testbed uses. A part written in the testbed has
// no name and no page.
type testbedPart struct {
	Label, Name, Summary string
	URL                  templ.SafeURL
}

// testbedParts are the selected testbed's parts, one per kind.
func (d entriesData) testbedParts() []testbedPart {
	tb := d.Cat.Testbeds[d.Sel]
	if tb == nil {
		return nil
	}
	names := tb.Value.PartNames()
	out := make([]testbedPart, 0, len(config.PartKinds))
	for _, kind := range config.PartKinds {
		p := testbedPart{Label: kindLabel(kind), Name: names[kind]}
		if p.Name != "" {
			p.URL = entryURL(kind, p.Name)
			p.Summary = partSummaryOf(d.Cat, kind, p.Name)
		}
		out = append(out, p)
	}
	return out
}

// usedByLink is a "kind name" from Service.UsedBy as its kind, its name and
// the name's page.
func usedByLink(s string) (kind, name string, href templ.SafeURL) {
	kind, name, _ = strings.Cut(s, " ")
	tab := map[string]string{
		"testbed": config.KindTestbed, "plan": config.KindPlan, "suite": config.KindSuite, "test": config.KindTest,
		"step": config.KindStep,
	}[kind]
	if tab == "" {
		return kind, name, ""
	}
	return kind, name, entryURL(tab, name)
}

func (d entriesData) itemURL(name string) templ.SafeURL {
	u := entryURL(d.Tab, name)
	if d.Q != "" {
		u += templ.SafeURL("?q=" + url.QueryEscape(d.Q))
	}
	return u
}

// entriesTab serves a top-bar tab that lists config entries: plans, testbeds
// or suites.
func (s *Server) entriesTab(top string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.entriesPage(w, r, top) }
}

// entriesPage lists one subtab of a top-bar tab, named in the path by its
// slug (the first subtab when the path names none), and shows one entry. An
// unknown subtab or entry is not found; with no entry named, the first is
// shown.
func (s *Server) entriesPage(w http.ResponseWriter, r *http.Request, top string) {
	cat := s.svc.Catalog()
	tab := subtabsOf(top)[0]
	if slug := r.PathValue("sub"); slug != "" {
		kind, ok := kindBySlug(slug)
		t, found := subtabOf(kind)
		if !ok || !found || t.Top != top {
			http.NotFound(w, r)
			return
		}
		tab = t
	}
	d := entriesData{Top: top, Tab: tab.ID, Sub: tab, Sel: r.PathValue("name"), Q: r.URL.Query().Get("q"), Cat: cat}
	origins := tab.entries(cat)
	if _, ok := origins[d.Sel]; d.Sel != "" && !ok {
		http.NotFound(w, r)
		return
	}
	d.Total = len(origins)
	for _, n := range config.Names(origins) {
		if d.Q == "" || strings.Contains(n, d.Q) {
			d.Items = append(d.Items, entryItem{n, origins[n]})
		}
	}
	// Built-ins first, then file by file, as the mockup groups them.
	sort.SliceStable(d.Items, func(i, j int) bool {
		a, b := d.Items[i].Origin, d.Items[j].Origin
		if (a == config.BuiltIn) != (b == config.BuiltIn) {
			return a == config.BuiltIn
		}
		return a < b
	})
	if d.Sel == "" && len(d.Items) > 0 {
		d.Sel = d.Items[0].Name
	}
	d.Origin = origins[d.Sel]
	if d.canEdit() {
		d.Editable = slices.Contains(s.svc.Owned(d.Tab), d.Sel)
	}
	d.Source = sourceOf(cat, d.Tab, d.Sel)
	switch {
	case d.Tab == config.KindPlan:
		d.Last = s.svc.LastResult(d.Sel)
	case d.Tab == config.KindStep && cat.IsBuiltinStep(d.Sel):
		d.UsedBy = builtinUsers(cat, d.Sel)
	default:
		d.UsedBy = s.svc.UsedBy(d.Tab, d.Sel)
	}
	l := s.layout(r, top, onConfigChange)
	switch top {
	case "plans":
		render(w, r, plansPage(l, d))
	case "testbeds":
		render(w, r, testbedsPage(l, d))
	default:
		render(w, r, suitesPage(l, d))
	}
}

// sourceOf is how an entry is written: its CUE, or a built-in step's
// parameters schema.
func sourceOf(cat *config.Catalog, kind, name string) string {
	switch kind {
	case config.KindPlan:
		if it := cat.Plans[name]; it != nil {
			return it.Source
		}
	case config.KindTestbed:
		if it := cat.Testbeds[name]; it != nil {
			return it.Source
		}
	case config.KindSuite:
		if it := cat.Suites[name]; it != nil {
			return it.Source
		}
	case config.KindTest:
		if it := cat.Tests[name]; it != nil {
			return it.Source
		}
	case config.KindStep:
		if it := cat.Steps[name]; it != nil {
			return it.Source
		}
		for _, st := range cat.BuiltinSteps {
			if st.ID == name {
				return st.Schema
			}
		}
	default:
		if it := cat.Parts(kind)[name]; it != nil {
			return it.Source
		}
	}
	return ""
}

// builtinUsers are what use a built-in step, as "kind name": the tests that
// use it, and the user's steps that do.
func builtinUsers(cat *config.Catalog, id string) []string {
	var out []string
	for _, n := range config.Names(cat.Tests) {
		if slices.ContainsFunc(cat.Tests[n].Value.Steps, func(st config.Step) bool { return st.Uses == id }) {
			out = append(out, "test "+n)
		}
	}
	for _, n := range config.Names(cat.Steps) {
		if cat.Steps[n].Value.Uses == id {
			out = append(out, "step "+n)
		}
	}
	return out
}

// kvRow is one row of a detail table. Default marks a value the entry leaves
// to its default; Code, one shown as code.
type kvRow struct {
	Label, Value  string
	Default, Code bool
}

// fieldValue is how a form field's value reads: a choice's label, Yes or No,
// or the text.
func fieldValue(f ui.Field, v string) string {
	switch {
	case f.Kind == ui.FSelect:
		return vocab.Choice(f.Path, v)
	case f.Kind == ui.FBool && v == "true":
		return "Yes"
	case f.Kind == ui.FBool && v == "false":
		return "No"
	}
	return v
}

// settingRow is a field of an entry: its value, or its default.
func settingRow(f ui.Field, got string) kvRow {
	row := kvRow{Label: f.Label, Value: fieldValue(f, got), Code: f.Kind == ui.FLines || f.Kind == ui.FMap || f.Kind == ui.FYAML}
	if got == "" {
		row.Value, row.Default, row.Code = fieldValue(f, f.Default), true, false
		if f.Default == "" {
			row.Value = "Not set"
		}
	}
	return row
}

// partSettings are what a part sets, in its form's order: the common fields,
// set or at their default, then the advanced fields it sets, and how many
// advanced fields it leaves at their default. Machines are left to
// machineLines.
func partSettings(kind string, p config.Part) (common, advanced []kvRow, atDefault int) {
	v := map[string]any(p)
	for _, f := range ui.PartFields(kind) {
		if ui.IsMachineField(f) {
			continue
		}
		got := ui.Get(v, f)
		switch {
		case !f.Advanced:
			common = append(common, settingRow(f, got))
		case got != "":
			advanced = append(advanced, settingRow(f, got))
		default:
			atDefault++
		}
	}
	return common, advanced, atDefault
}

// machineLines are a hardware part's machine sets, one line each, with the
// defaults filled in: "3 control plane: 4 CPUs, 2048 MiB memory, 10 GiB disk".
func machineLines(p config.Part) []string {
	v := map[string]any(p)
	var out []string
	for _, r := range ui.MachineRoles {
		get := func(column string) string {
			f := ui.MachineField(r.Key, column)
			if got := ui.Get(v, f); got != "" {
				return got
			}
			return f.Default
		}
		out = append(out, fmt.Sprintf("%s %s: %s CPUs, %s MiB memory, %s GiB disk",
			get("count"), strings.ToLower(r.Label), get("cpus"), get("memMiB"), get("diskGiB")))
	}
	return out
}

// testbedSetting is the value of a testbed setting, by its form field's path:
// as resolved, else as written.
func testbedSetting(tb *config.TestbedItem, field string) string {
	var resolved, written string
	switch field {
	case "topology":
		written = tb.Value.Topology
		if tb.Resolved != nil {
			resolved = tb.Resolved.Tinkerbell.Topology
		}
	case "bootMode":
		written = tb.Value.BootMode
		if tb.Resolved != nil {
			resolved = tb.Resolved.Tinkerbell.BootMode
		}
	case "network.family":
		written, _ = tb.Value.Network["family"].(string)
		if tb.Resolved != nil {
			resolved = tb.Resolved.Network.Family
		}
	}
	return cmp(resolved, written)
}

// testbedSettings are a testbed's choices (topology, boot mode, IP family),
// labelled as its form labels them.
func testbedSettings(tb *config.TestbedItem) []kvRow {
	var out []kvRow
	for _, f := range ui.TestbedFields {
		if f.Kind == ui.FSelect {
			out = append(out, kvRow{Label: f.Label, Value: fieldValue(f, cmp(testbedSetting(tb, f.Path), f.Default))})
		}
	}
	return out
}

// networkExtras are a testbed's network options beyond its IP family.
func networkExtras(t *config.Testbed) string {
	nat64, mirror := "No NAT64", "no registry mirror"
	if t.Network.NAT64 {
		nat64 = "NAT64 on"
	}
	if t.Network.Mirror.Enabled {
		mirror = "registry mirror on"
	}
	return nat64 + " · " + mirror
}

// testbedLabels are the labels every combination of a testbed carries, which
// `when` and label filters match.
func testbedLabels(t *config.Testbed) []string {
	v := t.AxisValues()
	var out []string
	for _, k := range config.Names(v) {
		if !slices.Contains(out, v[k]) {
			out = append(out, v[k])
		}
	}
	return out
}

// axisChanges is an axis of a matrix and what each of its values changes.
type axisChanges struct {
	Axis   string
	Values []axisValue
}

type axisValue struct {
	Name    string
	Changes []change
}

// change is one thing an axis value changes: the part of a kind it picks
// (with its page), or a setting.
type change struct {
	Label, Value string
	URL          templ.SafeURL
}

// matrixRows are a matrix's axes in order, each value with its changes.
func matrixRows(m *config.Matrix) []axisChanges {
	var out []axisChanges
	for _, f := range facets(m, nil, false) {
		row := axisChanges{Axis: f.Axis}
		for _, v := range f.Values {
			row.Values = append(row.Values, axisValue{v.Value, patchChanges(m.Axes[f.Axis][v.Value])})
		}
		out = append(out, row)
	}
	return out
}

// patchChanges says what an axis value's patch to the testbed changes.
func patchChanges(patch any) []change {
	m, _ := patch.(map[string]any)
	var out []change
	for _, k := range config.Names(m) {
		switch v := m[k].(type) {
		case string:
			if slices.Contains(config.PartKinds, k) {
				out = append(out, change{kindLabel(k), v, entryURL(k, v)})
				continue
			}
			out = append(out, settingChange(k, v))
		case map[string]any:
			if slices.Contains(config.PartKinds, k) {
				out = append(out, change{Label: kindLabel(k), Value: "changed in this testbed"})
				continue
			}
			for _, sub := range config.Names(v) {
				if s, ok := v[sub].(string); ok {
					out = append(out, settingChange(k+"."+sub, s))
				} else {
					b, _ := json.Marshal(v[sub])
					out = append(out, settingChange(k+"."+sub, string(b)))
				}
			}
		default:
			b, _ := json.Marshal(v)
			out = append(out, settingChange(k, string(b)))
		}
	}
	return out
}

// settingChange is a testbed setting an axis value sets, by its form field's
// path, labelled as the testbed form labels it.
func settingChange(field, v string) change {
	for _, f := range ui.TestbedFields {
		if f.Path == field {
			return change{Label: f.Label, Value: fieldValue(f, v)}
		}
	}
	return change{Label: field, Value: v}
}

// runnable are the combinations of a testbed that aren't excluded.
func runnable(tb *config.TestbedItem) []string {
	var out []string
	for _, c := range tb.Combinations {
		if c.Excluded == "" {
			out = append(out, c.Name)
		}
	}
	return out
}

func excludedCombinations(tb *config.TestbedItem) []config.Combination {
	var out []config.Combination
	for _, c := range tb.Combinations {
		if c.Excluded != "" {
			out = append(out, c)
		}
	}
	return out
}

// resolvedRows are what a testbed comes to with every default applied.
func resolvedRows(t *config.Testbed) []kvRow {
	tink := "chart " + cmp(chartVersion(t.Tinkerbell.Chart), "latest")
	if src := t.Tinkerbell.Source; src != nil {
		tink = "built from " + strings.TrimSpace(src.Repo+" "+src.Ref)
	}
	capt := "CAPT " + t.ClusterAPI.CAPT.Version
	if src := t.ClusterAPI.CAPT.Source; src != nil {
		capt = "CAPT built from " + strings.TrimSpace(src.Repo+" "+src.Ref)
	}
	var machines []string
	for _, r := range ui.MachineRoles {
		m := t.Hardware.Machines[r.Key]
		machines = append(machines, fmt.Sprintf("%d %s: %d CPUs, %d MiB memory, %d GiB disk", m.Count, strings.ToLower(r.Label), m.CPUs, m.MemMiB, m.DiskGiB))
	}
	var setup []string
	for _, st := range t.Setup {
		setup = append(setup, st.DisplayName())
	}
	management := t.ManagementCluster.Type + " · " + cmp(t.ManagementCluster.Image, "its own node image")
	return []kvRow{
		{Label: "Management cluster", Value: management},
		{Label: "Tinkerbell", Value: strings.Join([]string{
			tink, "namespace " + t.Tinkerbell.Namespace,
			strings.ToLower(vocab.Choice("topology", t.Tinkerbell.Topology)),
			strings.ToLower(vocab.Choice("bootMode", t.Tinkerbell.BootMode)),
		}, " · ")},
		{Label: "Cluster API", Value: "Cluster API " + t.ClusterAPI.Core + " · " + capt},
		{Label: "Workload cluster", Value: strings.Join([]string{
			t.WorkloadCluster.Name, "Kubernetes " + t.WorkloadCluster.KubernetesVersion,
			vocab.Choice("cni", t.WorkloadCluster.CNI), "OS " + t.WorkloadCluster.OSVersion,
			"kube-vip " + t.WorkloadCluster.KubeVipVersion,
		}, " · ")},
		{Label: "Machines", Value: strings.Join(machines, "\n")},
		{Label: "Hardware", Value: strings.Join([]string{
			vocab.Choice("arch", t.Arch), t.Hardware.Backend.Type + " " + t.Hardware.Backend.URI,
			"disks in " + t.Hardware.Backend.DiskPath, "BMC " + vocab.Choice("bmc.protocol", t.Hardware.BMC.Protocol),
		}, " · ")},
		{Label: "Network", Value: vocab.Choice("network.family", t.Network.Family) + " · " + networkExtras(t)},
		{Label: "Setup steps", Value: strings.Join(setup, " › "), Code: true},
	}
}

func chartVersion(c *config.Chart) string {
	if c == nil {
		return ""
	}
	return c.Version
}

// planCombinations says how many combinations a plan runs on its testbed.
func planCombinations(cat *config.Catalog, p config.Plan) string {
	tb := cat.Testbeds[p.Testbed]
	if tb == nil {
		return ""
	}
	if !tb.HasMatrix() {
		return "one combination"
	}
	n := 0
	for _, c := range runnable(tb) {
		if nameMatches(c, p.Combinations) {
			n++
		}
	}
	return vocab.Count(n, "combination", "combinations")
}

// matrixExample is a matrix that a testbed with none could start from, in
// CUE: one axis over two workload cluster parts the config has.
func matrixExample(cat *config.Catalog, testbed string) string {
	values := []string{"k8s-1.36", "k8s-1.37"}
	if parts := config.Names(cat.Parts(config.KindWorkloadCluster)); len(parts) >= 2 {
		values = parts[:2]
	}
	var b strings.Builder
	// An axis makes combinations only once order lists it.
	fmt.Fprintf(&b, "testbeds: %q: matrix: {\n\torder: [\"cluster\"]\n\taxes: cluster: {\n", testbed)
	for _, v := range values {
		fmt.Fprintf(&b, "\t\t%q: workloadCluster: %q\n", v, v)
	}
	b.WriteString("\t}\n}")
	return b.String()
}

// stepView is a test's step as its page lists it.
type stepView struct {
	// Uses is the named step it uses, built-in or the user's, if any; each
	// has a page.
	Name, Uses, Kind, Detail, Timeout string
}

// stepViews are a test's steps: what each runs, how, and when it times out.
func stepViews(steps []config.Step) []stepView {
	var out []stepView
	for _, st := range steps {
		v := stepView{Name: st.Name, Timeout: cmp(st.Timeout, "15m")}
		switch {
		case st.Uses != "":
			v.Uses, v.Kind = st.Uses, vocab.Choice("step.kind", ui.StepUses)
			var with []string
			for _, k := range config.Names(st.With) {
				b, _ := json.Marshal(st.With[k])
				with = append(with, k+": "+strings.Trim(string(b), `"`))
			}
			v.Detail = strings.Join(with, ", ")
		case st.Container != nil:
			v.Kind, v.Detail = vocab.Choice("step.kind", ui.StepContainer), st.Container.Image
		case st.Exec != nil:
			v.Kind, v.Detail = vocab.Choice("step.kind", ui.StepExec), strings.Join(st.Exec.Command, " ")
		}
		if v.Name == "" && v.Uses == "" {
			v.Name = st.DisplayName()
		}
		out = append(out, v)
	}
	return out
}

// Kept testbeds.

// address names a kept testbed as people type it: execution/combination.
func address(e service.KeptTestbed) string { return e.Execution + "/" + e.Combination }

// keptStatus is how a kept testbed's line reads: paused, or its verdict.
func keptStatus(e service.KeptTestbed) event.Status {
	if e.Paused {
		return event.Paused
	}
	return e.Status
}

func kubeconfigURL(e service.KeptTestbed, cluster string) templ.SafeURL {
	return executionFileURL(e.Execution, filepath.Join(filepath.Base(e.Dir), "kubeconfigs", cluster))
}

// keptRow is one row of a kept testbed card, with a line per thing it lists.
type keptRow struct {
	Label string
	Lines []keptLine
}

// keptLine is what people read of one thing, then what they copy (shown as
// code).
type keptLine struct{ Text, Code string }

// keptRows are what a kept testbed is made of, as far as its setup got.
func keptRows(e service.KeptTestbed) []keptRow {
	st := e.State
	var rows []keptRow
	add := func(label string, lines ...keptLine) {
		if len(lines) > 0 && lines[0] != (keptLine{}) {
			rows = append(rows, keptRow{label, lines})
		}
	}
	var clusters, versions, machines []keptLine
	for _, k := range config.Names(st.Clusters) {
		clusters = append(clusters, keptLine{clusterLabel(k), st.Clusters[k]})
	}
	for _, k := range config.Names(st.Versions) {
		versions = append(versions, keptLine{Code: k + " " + st.Versions[k]})
	}
	for _, m := range st.Machines {
		machines = append(machines, keptLine{Code: strings.TrimSpace(m.Name + " " + m.Role + " " + m.IP)})
	}
	add("Clusters", clusters...)
	add("Tinkerbell", keptLine{Code: st.VIPs.Tinkerbell})
	add("Versions", versions...)
	add("Machines", machines...)
	if st.BMC.Protocol != "" {
		add("BMC", keptLine{Code: vocab.Choice("bmc.protocol", st.BMC.Protocol) + " " + st.BMC.Host})
	}
	if st.Network.Name != "" {
		add("Network", keptLine{Code: networkLine(e)})
	}
	return rows
}

// clusterLabel names one of a testbed's clusters, e.g. "workload cluster".
func clusterLabel(cluster string) string { return cluster + " cluster" }

// gridLegend are the statuses the grid's legend lists before it says what
// errored means.
var gridLegend = []event.Status{event.Passed, event.Failed, event.Running, event.Waiting, event.Skipped}

// kubeconfigs are the clusters of a kept testbed that have a kubeconfig to
// download.
func kubeconfigs(e service.KeptTestbed) []string {
	var out []string
	for _, k := range config.Names(e.State.Clusters) {
		if _, err := os.Stat(filepath.Join(e.Dir, "kubeconfigs", k)); err == nil {
			out = append(out, k)
		}
	}
	return out
}

// Config files.

func (s *Server) configPage(w http.ResponseWriter, r *http.Request) {
	cat := s.svc.Catalog()
	var loose []config.Error
	for _, e := range cat.AllErrors() {
		if e.File == "" {
			loose = append(loose, e)
		}
	}
	render(w, r, configPage(s.layout(r, "", onConfigChange), cat, loose,
		filepath.Join(s.svc.StarterDir, "wendy.cue"), shortPath(s.svc.UIFilePath())))
}

// errPos returns where e is, as file:line:col, or "" when e has no position.
func errPos(e config.Error) string {
	switch {
	case e.Line == 0:
		return shortPath(e.File)
	case e.File == "":
		return fmt.Sprintf("%d:%d", e.Line, e.Col)
	}
	return fmt.Sprintf("%s:%d:%d", shortPath(e.File), e.Line, e.Col)
}
