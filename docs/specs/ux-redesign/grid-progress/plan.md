# Plan: grid-progress — V1

From [spec.md](spec.md). Tasks: [todo.md](todo.md).

## Components

1. **Start in order.** `Execution.execute` takes a Combinations at once slot in the loop, in the preview's order,
   then starts the combination's goroutine. Marking it running and emitting `combination.started` move from
   `runCombination` into the loop, so a combination has started before the next one waits for a slot. A
   combination still waiting when the execution is cancelled is marked cancelled there, as `runCombination` does
   today.
2. **Live state, in `internal/ui`.** The web UI and, later, the TUI read the same answers from the
   `ExecutionView`, keyed by combination:
   - `Phase(c, name)` is a grid cell. It is the summary's verdict when there is one, and skipped once the
     combination is done, as today. Otherwise it comes from the events:
     - the worst failure under the phase or suite, so a paused or stopping combination shows where it failed;
     - running once it has started, between steps too;
     - waiting before that.

     The build column only shows running while the shared build is running. A combination's build verdict
     still comes when it starts.
   - `Now(c)` is the step a combination is on: the step it paused at, or its running step.
   - `Elapsed(c)` is the time since a running or paused combination started, and the summary's time once it is
     done.
3. **Grid.**
   - Each row's name gets its combination's status glyph, ‖ when paused, with the word as its title.
   - The last column shows today's failure message, except for a running or paused combination. Those show
     `Now` as a link to that step in the tree, as Home's Running card words it (`pathLabel`). A paused one also
     shows its failure's first line.
   - Time shows `Elapsed`.
   - The build column appears when the execution builds anything, not only once a build has finished.
4. **Docs.** `design.md`'s Execution row, the spec's status, and the docs index.

## Order

Start order (engine) → live state (ui) → grid (web, templates) → docs.

## Risks

- **The summary and the events are read separately,** so a refresh can see one a moment ahead of the other.
  Verdicts come from the summary first, and the events only fill in what has no verdict yet, so a finished cell
  never goes back to running.
- **Cancelling while combinations wait for a slot.** The loop marks each one cancelled instead of starting it,
  and takes no slot it can't give back.
