# Tasks: execution-settings — V1

From [plan.md](plan.md) and [spec.md](spec.md). Criteria numbers refer to the spec's Success Criteria.

All six tasks are done. Notes from building them:

- **T1:**
  - `New` now takes parallel, keep, pause, suites and diagnostics from `Normalize`, instead of defaulting each one
    on its own.
  - `Summary.Effective` sits last in the struct, so its doc comment doesn't split gofmt's alignment of the fields
    above it.
- **T2:**
  - The run form's radios post their own values (`keep`; `pauseOnFailure` `true` or `false`). `runForm` reads
    `true` as well as `on`.
  - The `keeps` var went; only the glossary guard read it.
- **T3:**
  - The Settings `<details>` has `hx-preserve`, so a live execution's refreshes don't close it.
  - Change and run… leaves `parallel` out at its default, because the run form posts it only for a testbed with a
    matrix. With it in, the preview's selection differed from the execution's and missed its timing.
  - The fixture now records `Effective`, as the engine does. `TestExecutionSettings` clears it to cover older
    executions.
- **T4:**
  - `ui.StopKeeps` decides, for the web UI and the TUI, whether Stop keeps the testbed.
  - Cancelling an execution with a paused combination now also follows `keep`.
  - `decisionButtons` serves Home's banner and the failure card.
- **T5:** the CLI's interactive pause prompt reads `[r]etry step  [c]arry on  [x] stop`. The TUI's other old words
  (environment, run) are left to `tui-ux`.
- **T6:** the live check at `192.168.2.50:7601` served a copy of execution #2 from a temporary data directory:
  - Settings showed Tear down the testbed (default) and Pause and wait for me;
  - Change and run… prefilled both, and previewed 14m00 from #2.

- [x] **T1. Record what an execution ran with**
  - Acceptance: `Preview.Effective` and `Summary.Effective` hold the selection over its plan with every default filled
    in. `New` derives parallel, keep, pause, suites and diagnostics from it.
  - Verify: `TestEffectiveSelection` in `preview_test.go`.
  - Files: `internal/preview/preview.go`, `internal/engine/{summary.go,run.go}`, tests.
  - Criteria: 1.

- [x] **T2. Words, and the Options card**
  - Acceptance: `keep` and `pauseOnFailure` have the spec's labels and descriptions. The run and plan forms show
    them as radio lists with the default marked. A plan's page uses the same words.
  - Verify: tests in `vocab_test.go`, `edit_test.go`, `config_test.go`; the label check.
  - Files: `internal/vocab/choices.go`, `internal/ui/web/{run.templ,helpers.go,pages.go}`, `static/wendy.css`, tests.
  - Criteria: 3.

- [x] **T3. Settings and Change and run… on the execution's page**
  - Acceptance: every execution's page lists its settings, with defaults marked, and Change and run… opens the run
    form with them. An older unsaved execution shows its settings too.
  - Verify: `TestExecutionSettings` in `executions_test.go`.
  - Files: `internal/ui/web/{executions.templ,pages.go}`, `fixture_test.go`, tests.
  - Criteria: 1, 2.

- [x] **T4. Stop follows When it finishes**
  - Acceptance:
    - A stopped combination is kept or torn down as `keep` says.
    - Home's banner and the failure card read Retry step, Carry on and Stop. Stop's confirmation says which happens.
  - Verify: `TestPauseStop` in `engine_test.go`; `TestStopConfirmSaysWhatHappensToTheTestbed`; tests in
    `home_test.go` and `executions_test.go`.
  - Files: `internal/engine/run.go`, `internal/ui/view.go`, `internal/ui/web/{executions.templ,home.templ,pages.go}`,
    tests.
  - Criteria: 4.

- [x] **T5. TUI and CLI**
  - Acceptance: the TUI's run form, plan view, paused keys and help, and the CLI's flag help and pause prompt, use the
    new words. None says "Abort" or "Wait for me before tearing".
  - Verify: `go test ./internal/ui/tui ./internal/cli`.
  - Files: `internal/ui/tui/{form.go,run.go,tui.go,screens.go}`, `internal/cli/{root.go,plain.go}`, tests.
  - Criteria: 5.

- [x] **T6. Docs and checks**
  - Acceptance: README and `design.md` describe the Settings section, Change and run… and the two choices.
    `docs/README.md` lists this change.
  - Verify: `make vet lint test`, the generated-template hash check and a live check.
  - Files: `README.md`, `docs/design.md`, `docs/README.md`.
  - Criteria: 6, 7.
