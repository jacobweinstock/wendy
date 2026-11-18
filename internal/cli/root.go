package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/tinkerbell/wendy/internal/build"
	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/event"
	"github.com/tinkerbell/wendy/internal/hostcheck"
	"github.com/tinkerbell/wendy/internal/preview"
	"github.com/tinkerbell/wendy/internal/report"
	"github.com/tinkerbell/wendy/internal/service"
	"github.com/tinkerbell/wendy/internal/steps"
	"github.com/tinkerbell/wendy/internal/ui"
	"github.com/tinkerbell/wendy/internal/ui/tui"
	"github.com/tinkerbell/wendy/internal/ui/web"
	"github.com/tinkerbell/wendy/internal/vocab"
)

// Execute runs wendy and returns its exit code.
func Execute() int {
	// controller-runtime logs what has no step's context, such as warnings
	// during a teardown, through one global logger, and CAPI's framework makes
	// clients that use it. Unset, it drops them and prints a stack trace.
	ctrllog.SetLogger(logr.FromSlogHandler(slog.Default().Handler()))
	g := &globals{}
	root := &cobra.Command{
		Use:           "wendy",
		Short:         "End-to-end tests for Tinkerbell and Cluster API Provider Tinkerbell",
		Long:          "Wendy (Provisioning Engine for Tinkerbell E2E Runs) builds a Tinkerbell + CAPT environment from CUE config and runs test scenarios against it.\nWith no command it opens the TUI and serves the web UI.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	noWeb := root.Flags().Bool("no-web", false, "do not serve the web UI")
	root.RunE = func(cmd *cobra.Command, _ []string) error {
		return interactive(cmd.Context(), g, !*noWeb, "")
	}
	pf := root.PersistentFlags()
	pf.StringArrayVar(&g.configDirs, "config", nil, "a config directory, after $XDG_CONFIG_HOME/wendy and ./.wendy (repeatable)")
	pf.StringArrayVar(&g.tags, "tag", nil, "set a CUE tag, key=value (repeatable)")
	pf.StringVar(&g.data, "data", "", "data directory (default $XDG_STATE_HOME/wendy)")
	pf.StringVar(&g.addr, "addr", "127.0.0.1:7600", "web UI address")

	root.AddCommand(serveCmd(g), runCmd(g), planCmd(g), vetCmd(g), listCmd(g), envCmd(g), stepCmd(g), reportCmd(g), &cobra.Command{
		Use: "version", Short: "Print the version",
		Run: func(*cobra.Command, []string) {
			fmt.Println("wendy", build.GitRevision())
		},
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := root.ExecuteContext(ctx)
	var ee *exitError
	switch {
	case errors.As(err, &ee):
		if ee.msg != "" {
			fmt.Fprintln(os.Stderr, ee.msg)
		}
		return ee.code
	case err != nil:
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	return 0
}

// selectionFlags adds the flags that select what to run.
func selectionFlags(cmd *cobra.Command) *preview.Selection {
	sel := &preview.Selection{}
	f := cmd.Flags()
	f.StringVarP(&sel.Plan, "plan", "p", "", "run a plan (other flags edit it for this run)")
	f.StringVarP(&sel.Testbed, "testbed", "t", "", "run on a testbed")
	f.StringArrayVar(&sel.Combinations, "combination", nil, "combinations of the testbed's matrix to run, as glob patterns (repeatable)")
	f.StringArrayVarP(&sel.Suites, "suite", "s", nil, "suites to run (repeatable)")
	f.StringVarP(&sel.LabelFilter, "label-filter", "l", "", "Ginkgo label filter, ANDed with the combination's own")
	f.IntVar(&sel.Parallel, "parallel", 0, "combinations at once")
	f.StringVar(&sel.Keep, "keep", "", "keep the testbed: never, failed or always")
	cmd.Flags().Bool("pause-on-failure", false, "wait for a decision before tearing down after a failure")
	return sel
}

func finishSelection(cmd *cobra.Command, sel *preview.Selection) {
	if cmd.Flags().Changed("pause-on-failure") {
		v, _ := cmd.Flags().GetBool("pause-on-failure")
		sel.PauseOnFailure = &v
	}
	if !cmd.Flags().Changed("suite") {
		sel.Suites = nil
	}
	if !cmd.Flags().Changed("combination") {
		sel.Combinations = nil
	}
}

// interactive runs the TUI, and the web UI unless told not to.
func interactive(ctx context.Context, g *globals, withWeb bool, runID string) error {
	svc, err := g.service()
	if err != nil {
		return err
	}
	go svc.Watch(ctx)
	if withWeb {
		url, stopWeb, err := serveWeb(ctx, g, svc)
		if err != nil {
			fmt.Fprintln(os.Stderr, "web UI:", err)
		} else {
			defer stopWeb()
			svc.WebURL = url
		}
	}
	err = tui.Run(ctx, svc, tui.Options{RunID: runID})
	drain(svc)
	return err
}

// drain cancels runs still active when the UI exits and waits for their
// teardown, so nothing is left behind on the host.
func drain(svc *service.Service) {
	active := svc.Active()
	for _, r := range active {
		fmt.Fprintf(os.Stderr, "cancelling #%s and tearing down…\n", r.ID)
		r.Cancel()
	}
	for _, r := range active {
		r.Wait()
	}
}

func serveWeb(ctx context.Context, g *globals, svc *service.Service) (string, func(), error) {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", g.addr)
	if err != nil {
		return "", nil, err
	}
	srv := &http.Server{Handler: web.New(svc, os.Getenv("WENDY_TOKEN")).Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	stop := func() {
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}
	go func() {
		<-ctx.Done()
		stop()
	}()
	return "http://" + ln.Addr().String(), stop, nil
}

func serveCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Serve the web UI only",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, err := g.service()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			go svc.Watch(ctx)
			url, _, err := serveWeb(ctx, g, svc)
			if err != nil {
				return err
			}
			svc.WebURL = url
			slog.Info("web UI", "url", url, "data", g.dataDir(), "gitVersion", build.GitRevision(), "version", build.Version(build.GitRevision()))
			<-ctx.Done()
			drain(svc)
			return nil
		},
	}
}

func runCmd(g *globals) *cobra.Command {
	var plainOut, verbose bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a plan or a selection; the exit code is the verdict",
	}
	sel := selectionFlags(cmd)
	cmd.Flags().BoolVar(&plainOut, "plain", false, "no UI: one line per event, for CI (implied without a terminal)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "with --plain, print every step and log line")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		finishSelection(cmd, sel)
		svc, err := g.service()
		if err != nil {
			return err
		}
		if errs := svc.Catalog().AllErrors(); len(errs) > 0 {
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, e.Error())
			}
			return exitf(2, "config has errors")
		}
		ctx := cmd.Context()
		r, err := svc.Start(context.Background(), *sel)
		if err != nil {
			return exitf(2, "%v", err)
		}
		if !plainOut && term.IsTerminal(int(os.Stdout.Fd())) {
			if err := tui.Run(ctx, svc, tui.Options{RunID: r.ID, ExitWhenDone: true}); err != nil {
				return err
			}
			drain(svc)
			return exitf(exitCode(r.Wait()), "")
		}
		go func() {
			select {
			case <-r.Done():
				return
			case <-ctx.Done():
			}
			fmt.Fprintf(os.Stderr, "#%s  cancelling; tearing down (interrupt again to exit now)\n", r.ID)
			r.Cancel()
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, os.Interrupt)
			<-sig
			os.Exit(130) //nolint:revive // A second interrupt means exit now, without waiting for teardown.
		}()
		return exitf(exitCode(plain(context.Background(), os.Stdout, svc, r, verbose)), "")
	}
	return cmd
}

func planCmd(g *globals) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "plan", Short: "Show what a selection would run, what it needs and how long it takes"}
	sel := selectionFlags(cmd)
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the plan as JSON")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		finishSelection(cmd, sel)
		svc, err := g.service()
		if err != nil {
			return err
		}
		pv, err := svc.Preview(cmd.Context(), *sel)
		if err != nil {
			return exitf(2, "%v", err)
		}
		if asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(pv)
		}
		printPlan(pv)
		if len(hostcheck.Failed(pv.Checks)) > 0 {
			return exitf(1, "")
		}
		return nil
	}
	return cmd
}

func printPlan(pv *service.Preview) {
	p := pv.Preview
	fmt.Printf("%s  %d combination(s) × %d suite(s)\n\n", p.Label, len(p.Combinations), len(p.Combinations[0].Suites))
	varying := varyingParts(p.Combinations)
	for _, c := range p.Combinations {
		fmt.Printf("%s  [%s]\n", c.Name, strings.Join(c.Labels, " "))
		if len(varying) > 0 {
			fmt.Printf("  parts      %s\n", config.PartsLine(c.Parts, varying))
		}
		var setup []string
		for _, s := range c.Setup {
			setup = append(setup, s.Key)
		}
		fmt.Printf("  setup      %s\n", strings.Join(setup, " › "))
		for _, s := range c.Suites {
			var scs []string
			for _, sc := range s.Tests {
				name := sc.Name
				if sc.Skip != "" {
					name += " (skipped: " + sc.Skip + ")"
				}
				scs = append(scs, name)
			}
			fmt.Printf("  %-10s %s\n", s.Name, strings.Join(scs, " › "))
		}
	}
	n := p.Needs
	fmt.Printf("\nNeeds   %d VMs at once · %d GiB RAM · %d GiB disk\n", n.VMs, (n.RAMMiB+1023)/1024, n.DiskGiB)
	if len(p.Builds) == 0 {
		fmt.Println("Builds  none (released artifacts)")
	}
	for _, b := range p.Builds {
		fmt.Printf("Builds  %s from %s %s\n", b.Component, b.Source.Repo, b.Source.Ref)
	}
	if n.Estimate > 0 {
		fmt.Printf("Takes   ~%s (from %d past runs)\n", ui.Dur(n.Estimate.Milliseconds()), n.Samples)
	} else {
		fmt.Println("Takes   unknown (no past runs)")
	}
	fmt.Print("Checks ")
	for _, c := range pv.Checks {
		if c.Optional && !c.OK && c.Name != "disk space" {
			continue
		}
		fmt.Printf("  %s %s", checkGlyph(c.OK), c.Name)
	}
	fmt.Println()
	for _, c := range pv.Checks {
		if !c.OK && c.Optional && c.Name == "disk space" {
			fmt.Printf("  warning: %s\n", c.Detail)
		}
	}
	for _, c := range hostcheck.Failed(pv.Checks) {
		fmt.Printf("  %s %s: %s\n", checkGlyph(false), c.Name, c.Detail)
	}
	for _, w := range p.Warnings {
		fmt.Println("warning:", w)
	}
}

func vetCmd(g *globals) *cobra.Command {
	var offline bool
	cmd := &cobra.Command{
		Use:   "vet",
		Short: "Check the config, the host and the images testbeds need",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, err := g.service()
			if err != nil {
				return err
			}
			cat := svc.Catalog()
			bad := 0
			fmt.Printf("Watching %s\n", strings.Join(cat.Dirs, "  "))
			for _, f := range cat.Files {
				fmt.Printf("%s %s\n", checkGlyph(len(f.Errors) == 0), f.Path)
				for _, e := range f.Errors {
					fmt.Printf("    %d:%d  %s: %s\n", e.Line, e.Col, e.Path, e.Msg)
					bad++
				}
			}
			for _, e := range cat.Errors {
				fmt.Println(checkGlyph(false), e.Error())
				bad++
			}
			sources := false
			images := map[string]bool{}
			var testbeds []config.Testbed
			for _, tb := range cat.Testbeds {
				for _, c := range tb.Combinations {
					if c.Excluded == "" && c.Err == "" {
						testbeds = append(testbeds, c.Testbed)
					}
				}
			}
			upgrades := upgradeVersions(cat)
			for _, e := range testbeds {
				sources = sources || e.UsesSource()
				images[steps.OSImage(e.WorkloadCluster.OSVersion, e.WorkloadCluster.KubernetesVersion, e.Arch)] = true
				for _, v := range upgrades {
					images[steps.OSImage(e.WorkloadCluster.OSVersion, v, e.Arch)] = true
				}
			}
			fmt.Print("\nHost  ")
			for _, c := range hostcheck.Run(cmd.Context(), hostcheck.Options{Sources: sources}) {
				switch {
				case c.OK:
					fmt.Printf(" %s %s", checkGlyph(true), c.Name)
				case c.Optional:
					fmt.Printf(" %s %s (%s)", glyph(event.Skipped), c.Name, c.Detail)
				default:
					fmt.Printf(" %s %s (%s)", checkGlyph(false), c.Name, c.Detail)
					bad++
				}
			}
			fmt.Println()
			if !offline {
				var refs []string
				for r := range images {
					refs = append(refs, r)
				}
				sort.Strings(refs)
				for _, ref := range refs {
					ok, err := steps.ImageExists(cmd.Context(), ref)
					switch {
					case err != nil:
						fmt.Printf("? %s: %v\n", ref, err)
					case ok:
						fmt.Printf("%s %s\n", checkGlyph(true), ref)
					default:
						fmt.Printf("%s %s does not exist\n", checkGlyph(false), ref)
						bad++
					}
				}
			}
			if bad > 0 {
				return exitf(1, "\n%d problem(s)", bad)
			}
			fmt.Println("\nall good")
			return nil
		},
	}
	cmd.Flags().BoolVar(&offline, "offline", false, "skip checking that OS images exist")
	return cmd
}

func listCmd(g *globals) *cobra.Command {
	lists := slices.Concat([]string{config.KindPlan, config.KindTestbed}, partSlugs(), []string{"combinations", "suites", "tests", "steps"})
	return &cobra.Command{
		Use:       "list {" + strings.Join(lists, "|") + "}",
		Short:     "List what the config defines",
		Args:      cobra.ExactArgs(1),
		ValidArgs: lists,
		RunE: func(_ *cobra.Command, args []string) error {
			svc, err := g.service()
			if err != nil {
				return err
			}
			cat := svc.Catalog()
			w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			defer w.Flush()
			if kind, ok := partKindOfSlug(args[0]); ok {
				listParts(w, cat, kind)
				return nil
			}
			switch args[0] {
			case config.KindPlan:
				for _, n := range config.Names(cat.Plans) {
					p := cat.Plans[n]
					target := strings.TrimSpace(p.Value.Testbed + " " + strings.Join(p.Value.Combinations, ","))
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", n, target, strings.Join(p.Value.Suites, ","), p.Origin)
				}
			case config.KindTestbed:
				for _, n := range config.Names(cat.Testbeds) {
					tb := cat.Testbeds[n]
					var what string
					switch {
					case tb.HasMatrix():
						what = vocab.Count(len(tb.Combinations), "combination", "combinations") + ": " + strings.Join(tb.Value.Matrix.Order, " × ")
					case tb.Resolved != nil:
						what = strings.Join(labels(*tb.Resolved), " ")
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", n, config.PartsLine(tb.Value.PartNames(), config.PartKinds), what, tb.Origin)
				}
			case "combinations":
				for _, n := range config.Names(cat.Testbeds) {
					if !cat.Testbeds[n].HasMatrix() {
						continue
					}
					for _, c := range cat.Testbeds[n].Combinations {
						note := ""
						if c.Excluded != "" {
							note = "excluded: " + c.Excluded
						}
						if c.Err != "" {
							note = "error: " + firstLine(c.Err)
						}
						fmt.Fprintf(w, "%s\t%s\t%s\n", n, c.Name, note)
					}
				}
			case "suites":
				for _, n := range config.Names(cat.Suites) {
					s := cat.Suites[n]
					fmt.Fprintf(w, "%s\t%s\t%s\n", n, strings.Join(s.Value.Tests, " › "), s.Origin)
				}
			case "tests":
				for _, n := range config.Names(cat.Tests) {
					s := cat.Tests[n]
					fmt.Fprintf(w, "%s\t%d steps\t%s\t%s\n", n, len(s.Value.Steps), s.Value.Description, s.Origin)
				}
			case "steps":
				for _, b := range cat.BuiltinSteps {
					fmt.Fprintf(w, "%s\t%s\t%s\n", b.ID, b.Schema, b.Description)
				}
				for _, n := range config.Names(cat.Steps) {
					s := cat.Steps[n]
					fmt.Fprintf(w, "%s\t%s\t%s\n", n, stepRuns(s.Value), s.Origin)
				}
			default:
				return fmt.Errorf("unknown list %q", args[0])
			}
			return nil
		},
	}
}

func labels(e config.Testbed) []string {
	var out []string
	for _, k := range config.Names(e.AxisValues()) {
		out = append(out, e.AxisValues()[k])
	}
	return out
}

// stepRuns is what a step runs, in a few words: the built-in step it uses,
// its command, or its container image.
func stepRuns(st config.Step) string {
	switch {
	case st.Uses != "":
		return "uses " + st.Uses
	case st.Exec != nil:
		return strings.Join(st.Exec.Command, " ")
	case st.Container != nil:
		return st.Container.Image
	}
	return ""
}

// envRef parses a kept testbed's address, "<execution>/<combination>".
func envRef(s string) (string, string, error) {
	execution, combination, ok := strings.Cut(s, "/")
	if !ok {
		return "", "", fmt.Errorf("want <execution>/<combination>, e.g. 12/colocated-ipv4-netboot-direct")
	}
	return execution, combination, nil
}

func envCmd(g *globals) *cobra.Command {
	env := &cobra.Command{Use: "env", Short: "Work with kept environments"}
	up := &cobra.Command{Use: "up", Short: "Build an environment (setup only) and keep it"}
	sel := selectionFlags(up)
	up.RunE = func(cmd *cobra.Command, _ []string) error {
		finishSelection(cmd, sel)
		sel.SetupOnly, sel.Keep = true, "always"
		svc, err := g.service()
		if err != nil {
			return err
		}
		r, err := svc.Start(context.Background(), *sel)
		if err != nil {
			return exitf(2, "%v", err)
		}
		go func() { <-cmd.Context().Done(); r.Cancel() }()
		st := plain(context.Background(), os.Stdout, svc, r, false)
		return exitf(exitCode(st), "")
	}
	down := &cobra.Command{
		Use: "down <execution>/<combination>", Short: "Tear down a kept environment", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			run, combo, err := envRef(args[0])
			if err != nil {
				return err
			}
			svc, err := g.service()
			if err != nil {
				return err
			}
			return svc.TearDown(cmd.Context(), run, combo, func(name string, st event.Status, msg string) {
				fmt.Printf("%s %s %s\n", glyph(st), name, msg)
			})
		},
	}
	status := &cobra.Command{
		Use: "status", Short: "List kept and paused environments",
		RunE: func(*cobra.Command, []string) error {
			svc, err := g.service()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			defer w.Flush()
			for _, e := range svc.KeptTestbeds() {
				state := "kept"
				if e.Paused {
					state = "paused"
				}
				fmt.Fprintf(w, "%s/%s\t%s\t%s\t%s\t%s\n", e.Execution, e.Combination, state, e.State.VIPs.Tinkerbell, ui.Ago(e.Since), e.Dir)
			}
			return nil
		},
	}
	kc := &cobra.Command{
		Use: "kubeconfig <execution>/<combination> [management|tinkerbell|workload]", Short: "Print a kept environment's kubeconfig path", Args: cobra.RangeArgs(1, 2),
		RunE: func(_ *cobra.Command, args []string) error {
			run, combo, err := envRef(args[0])
			if err != nil {
				return err
			}
			svc, err := g.service()
			if err != nil {
				return err
			}
			cluster := engine.ManagementCluster
			if len(args) == 2 {
				cluster = args[1]
			}
			sum, err := svc.Execution(run)
			if err != nil {
				return err
			}
			for _, c := range sum.Combinations {
				if c.Name == combo {
					fmt.Println(filepath.Join(svc.ExecutionDir(run), c.Dir, "kubeconfigs", cluster))
					return nil
				}
			}
			return fmt.Errorf("#%s has no combination %s", run, combo)
		},
	}
	env.AddCommand(up, down, status, kc)
	return env
}

func stepCmd(g *globals) *cobra.Command {
	var with, envArg, timeout string
	run := &cobra.Command{
		Use:   "run <step> --env <execution>/<combination>",
		Short: "Run one built-in step against a kept environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, combo, err := envRef(envArg)
			if err != nil {
				return err
			}
			svc, err := g.service()
			if err != nil {
				return err
			}
			sum, err := svc.Execution(r)
			if err != nil {
				return err
			}
			dir := ""
			for _, c := range sum.Combinations {
				if c.Name == combo {
					dir = filepath.Join(svc.ExecutionDir(r), c.Dir)
				}
			}
			if dir == "" {
				return fmt.Errorf("%s has no combination %s", r, combo)
			}
			env, err := engine.LoadTestbed(dir, svc.Engine.Renderer)
			if err != nil {
				return err
			}
			env.Builds = &engine.Builds{}
			st := config.Step{Uses: args[0], Timeout: timeout}
			if with != "" {
				if err := json.Unmarshal([]byte(with), &st.With); err != nil {
					return fmt.Errorf("--with: %w", err)
				}
			}
			status, err := svc.Engine.RunSingle(cmd.Context(), env, st, func(e event.Event) {
				switch e.Type {
				case event.Log:
					fmt.Printf("%s %s\n", e.TS.Format("15:04:05"), e.Msg)
				case event.StepFinished:
					fmt.Printf("%s %s %s %s\n", glyph(e.Status), args[0], ui.Dur(e.Ms), e.Msg)
				}
			})
			if err != nil {
				return exitf(exitCode(status), "%v", err)
			}
			return nil
		},
	}
	run.Flags().StringVar(&envArg, "env", "", "the kept testbed, <execution>/<combination>")
	run.Flags().StringVar(&with, "with", "", "the step's params as JSON")
	run.Flags().StringVar(&timeout, "timeout", "15m", "the step's timeout")
	_ = run.MarkFlagRequired("env")
	step := &cobra.Command{Use: "step", Short: "Run single steps"}
	step.AddCommand(run)
	return step
}

func reportCmd(g *globals) *cobra.Command {
	junit := &cobra.Command{
		Use: "junit <run>", Short: "Write a run's JUnit report and print its path", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			dir := filepath.Join(g.dataDir(), engine.ExecutionsDir, filepath.Base(args[0]))
			if err := report.WriteJUnit(dir); err != nil {
				return err
			}
			fmt.Println(filepath.Join(dir, "report.junit.xml"))
			return nil
		},
	}
	r := &cobra.Command{Use: "report", Short: "Reports"}
	r.AddCommand(junit)
	return r
}
