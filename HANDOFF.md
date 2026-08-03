# Handoff - AgentBox: assignments run, and the one piece M12 left open

**Written:** 2026-08-01 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

**Renamed 2026-08-03 (session 36):** the project is AgentBox now - binary
`agentbox`, tools `mcp__agentbox__*`, fresh git history (fifteen subsystem
commits; every hash below this line belongs to the retired history), mirrors
renamed. Claude sessions started before the rename hold dead MCP tools until
they restart. Names and paths below were updated in place; the work queue
itself is unchanged.

## Do this next

```bash
cd ~/me/projects/agentbox
git log --oneline           # fresh history: the tree in subsystem commits, docs last
make deployed               # must answer the commit at head
agentbox control state      # "no run: the desktop is the human's"
git status -sb              # expect clean, in sync with origin/main
```

M12 (assignments, FR82) is finished and exercised live - scheduler, MCP tools,
runner and surface. **Ask Boris what he wants next before starting item 1**: the
queue below is the pre-M12 order, and he has reset priorities twice in two
sessions. If he is not there, item 1 is the honest continuation.

**1. M12's last piece - the custom HTML panel.** An assignment's `panel_html`
stores, edits and round-trips through both the MCP tools and the editor, and the
surface shows a note instead of running it - the `{#if a.panelHtml}` block in
`frontend/src/surfaces/Assignments.svelte`.

This is not a stub to fill in. The existing artifact machinery
(`frontend/src/lib/artifact.svelte.js`) mounts a frame from Go-emitted
`.k-artifact-stage` markup and routes `window.agentbox.emit` through
`bridge.artifactEvent` to whichever agent is awaiting that artifact - the wrong
destination for a parameter panel, whose values have to reach
`SetAssignmentParams`. So it needs a channel of its own. What IS reusable
untouched: `mod.buildDocument({source, runtime, react, tailwind, tokens})` from
`frontend/src/lib/artifact-runtime.js`, the `SANDBOX` attributes (allow-scripts
without allow-same-origin, empty allow list), and the postMessage shape.

The rule that makes the escape hatch safe, and must not be traded away: the
typed knobs stay available for every assignment, so a panel that fails to load
can never make one uneditable. `TestABrokenSpecStillListsAndStillCarriesItsPrompt`
(`internal/store/assignment_test.go`) pins the storage half of it.

**2. FR81's other half:** the visual pass over Session / Inbox / History /
Viewer / Library / Settings so they read as one product. Home and Assignments
are done; nothing else has been touched.

**3. FR73** - a card body must be readable after the card closes (the inbox row
truncates to a tooltip and offers no detail view; Boris hit this on 2026-07-31).
**FR65** - open a citation in the editor.

**4. Not now, but do not lose it: FR74's fullscreen marker.** Built in session
34, still never exercised. Needs a real fullscreen window and Boris's consent to
drive - take the desktop with `request_control`, and read the one-UI trap in
`CLAUDE.md` first. The specific unknown is whether GTK honours a 4px-tall
window at all.

## Where we are

Session 34 built the assignment engine and wired it to nothing - `SetRunner` and
`StartAssignments` were called from nowhere, so the tick loop had never started.
This session finished all three remaining slices and demonstrated each one
against the deployed daemon rather than reading the diff.

Read [docs/08-assignments.md](docs/08-assignments.md) before touching
assignments. Two conventions were decided while building and are now in it: a
run's last assistant message is its summary, and a fenced ```agentbox-data block in
that message is lifted into the run's `data` column.

## Live state (volatile - verify on resume)

- **Background jobs:** none. **PRs:** none, ever (this repo pushes `main`).
- **Git:** clean and pushed; `origin/main` is this handoff's own commit.
- **Deployed:** `1898d37`, clean, from a committed tree (the fresh history's
  head at deploy time). Everything newer is docs, so the running daemon has
  all of this session's code. `make deployed` checks it by asking the
  daemon, never the file.
- **How to exercise the MCP tools from a fresh session:** you cannot call
  `mcp__agentbox__*` for a tool added after your own `agentbox mcp` child started. Speak
  stdio JSON-RPC to a new `agentbox mcp` instead - the recipe is under "Mechanics
  discovered" in [docs/07-field-requests.md](docs/07-field-requests.md).
- **On Boris's live machine, created by this session:** one assignment, **"Claude
  usage check"** (`a0eff4b720959`), **ad-hoc so it never fires by itself**, knobs
  window=7d and warn_at=90%. It was written through the editor to exercise it,
  and it is his worked example from the design doc, so it was left rather than
  deleted. It has never run. Delete it or give it a schedule - that is his call,
  not the next session's.
- **Store changed:** rows in `assignments` / `assignment_runs` only (migration
  0007 already existed). The smoke-test assignment and its four runs were
  deleted at the end.
- **His sessions directory** has three saved conversations from assignment runs
  (`~/.local/state/agentbox/sessions/`, 2026-08-01 15:45 onward). They are real run
  transcripts, which is the point of a run being a session; leave them.
- **The app window** was opened for the live pass and closed to the tray again.
- **The session-34 watch item** (a `make deploy` test-gate failure with the
  output truncated) did NOT recur once in about a dozen full gate runs. Keep
  watching; do not close it yet.
- **CLAUDE.md is in git since the rename** (force-added past
  `~/.gitignore_global`, which ignores it by default), so the one-UI trap now
  travels with the repo. The same warning is in `docs/history.md`.

## Blocked on you (Boris)

Nothing blocking. Two things to look at when convenient:

- **"Claude usage check" is sitting in Assignments, ad-hoc.** Give it a schedule
  (`daily 09:00`) if you want it, or delete it.
- **What next?** The queue above is the old order. If assignments should grow
  instead (a run answering its own cards, more than one at a time), say so.

## I can do solo (no input needed)

1. The custom HTML panel's channel (item 1).
2. The visual pass (item 2).
3. FR73 and FR65 (item 3).

## Facts - verified vs assumed

- [verified] An assignment created through the real MCP stdio server against the
  deployed daemon, run by an agent in 3.3s, with summary and `agentbox-data` both
  landing in the right columns.
- [verified] **The scheduler fires by itself.** Armed `every 1m`, left alone,
  and it ran: `trigger: "schedule"`, using the stored parameter rather than the
  earlier override - the rule the whole overrides design rests on.
- [verified] On the real desktop, with the hands-off strip up: the surface on
  live data; an enum, a text field and a slider each written through and read
  back from the database; Run now watched live (button disabled, running chip,
  the list refreshing itself when it finished); a run expanded to its recorded
  data and the values it used; a new assignment written in the editor; the smoke
  test deleted through the confirm.
- [verified] `make check` green at every commit (gofmt + vet + `go test ./...
  -race`, 21 packages).
- [verified] A run does not take the human's selection, stops its child while
  keeping the transcript, and reports a wordless exit as a failure - all three
  by test, against a stub `claude` that speaks stream-json.
- [verified] `TestManualListsEveryTool` now scans the whole `internal/mcp`
  package, so a tool family in its own file cannot ship undocumented.
- [assumed] The FR74 marker's behaviour over a fullscreen window (unchanged
  since session 34: the mechanism is proven, this window has never been mapped).
- [assumed] That `claude --model <id>` accepts whatever id Boris types into an
  assignment. The flag is still passed through unvalidated.
- [assumed] Home in the LIGHT theme, and the Assignments surface in it - both
  exercised in his dark theme only.
- [assumed] boris-vm was not used this session and is presumed off; `gcloud` in
  this shell is not authenticated.

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| The previous HANDOFF.md (session 34: the tray robot, Home, the assignment engine) | `docs/history.md` "Thirty-fourth session"; FR79-FR82 in `docs/07-field-requests.md` |
| "M12 slices 3, 4 and 5" as the do-next list | Shipped: `98e3547`, `b1ed1e4`, `d46efff`, `84d4267` |
| Session 34's note that Home's click-throughs were unexercised | Exercised: the Assignments door and the rail were both clicked through on the live desktop |
| The session-25 / session-34 `-race` flake watch item | Still open but quiet; moved to Live state with the run count |
| The stdio MCP driver script used to exercise the new tools (a scratchpad file, now gone) | The recipe it encoded is "Mechanics discovered" in `docs/07-field-requests.md` |

## Map

1. [docs/STATUS.md](docs/STATUS.md) - current state, what works, known gaps, the queue.
2. [docs/08-assignments.md](docs/08-assignments.md) - the M12 design, every decision behind it, and how a run hands something back. Read before touching assignments.
3. [docs/history.md](docs/history.md) - session-by-session record; this session is "Thirty-fifth".
4. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers used in commits; FR82 is now shipped.
5. [docs/agent-manual.md](docs/agent-manual.md) - the agent-facing reference, now covering the seven assignment tools. `internal/manual/agent.md` is the embedded short version; `internal/manual/assignment.md` is the brief a run is spawned with.
6. [CLAUDE.md](CLAUDE.md) - traps that have cost sessions; read before touching the build or the daemon.
