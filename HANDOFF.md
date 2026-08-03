# Handoff - AgentBox: renamed, decluttered, and the one piece M12 left open

**Written:** 2026-08-03 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git log --oneline -3        # head: this handoff; then 5a39f31 declutter, 1898d37 fresh-history head
make deployed               # must answer 1898d37 (everything newer is docs)
agentbox control state      # "no run: the desktop is the human's"
git status -sb              # expect clean, in sync with origin/main
```

Then pick up the queue. **Ask Boris what he wants next before starting item 1**
if he is around - he has reset priorities twice in three sessions. If he is
not, item 1 is the honest continuation.

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

Session 36 (2026-08-03) renamed the project to AgentBox at Boris's request,
totally: module `github.com/borismilner/agentbox`, binary/CLI `agentbox`, wire
`agentbox.v1.*`, env `AGENTBOX_*`, unit, desktop entry, icons, MCP registration,
the logo's bubble lettering, the recorded takes, and the live deployment
(config/state/cache copied with the daemon stopped; nothing lost - stats
answered 311 interruptions right after). The git history was restarted as
fifteen subsystem commits (his call), force-pushed to GitLab
(`fu-bar/agentbox`, path renamed by Boris) and to the public GitHub mirror
(`borismilner/agentbox`, renamed; old URL redirects). His GitHub profile README
and borismilner.github.io were migrated too. The same session decluttered the
docs (see the ledger below). The work queue itself is untouched since session
35: M12 runs end to end, the custom HTML panel is its one open piece.

Read [docs/08-assignments.md](docs/08-assignments.md) before touching
assignments. Two conventions are in it: a run's last assistant message is its
summary, and a fenced ```agentbox-data block in that message is lifted into the
run's `data` column.

## Live state (volatile - verify on resume)

- **Background jobs:** none. **PRs:** none, ever (this repo pushes `main`).
- **Git:** clean; `main` pushed and in sync with `origin` (GitLab) and
  `github` (mirror remote). Head is this handoff's own commit, on top of
  `5a39f31` (docs declutter). **GitLab push-mirrors to GitHub on its own**
  (survived the rename): a direct `git push github` can lose the race to the
  mirror and be rejected with "cannot lock ref"; fetch and compare heads
  before treating that as a real failure. Pushing `origin` is enough.
- **Deployed:** `1898d37`, clean stamp, daemon answering `make deployed`.
  Everything newer is docs, so the running daemon has all shipped code.
- **In-flight edits:** none.
- **Rename fallout still on disk, on purpose:** `~/.config/qq`,
  `~/.local/state/qq`, `~/.cache/qq`, `~/.local/share/qq` are fallback copies
  (delete after a quiet few days); `~/.local/bin/qq` is a compat symlink to
  `agentbox`. Claude sessions started before the rename hold dead
  `mcp__qq__*` tools until they restart; new sessions get
  `mcp__agentbox__*` from the user-scope registration in `~/.claude.json`.
- **On Boris's live machine (carried from session 35, not re-checked):** one
  ad-hoc assignment **"Claude usage check"** (`a0eff4b720959`), never run;
  schedule-or-delete is his call. Three saved run transcripts under
  `~/.local/state/agentbox/sessions/` (real run output; leave them).
- **How to exercise the MCP tools from a fresh session:** you cannot call
  `mcp__agentbox__*` for a tool added after your own `agentbox mcp` child
  started. Speak stdio JSON-RPC to a new `agentbox mcp` instead - the recipe
  is under "Mechanics discovered" in
  [docs/07-field-requests.md](docs/07-field-requests.md).
- **CLAUDE.md is in git since the rename** (force-added past
  `~/.gitignore_global`, which ignores it by default), so the one-UI trap now
  travels with the repo.
- **The session-34 `-race` flake watch item** did not recur through this
  session's many full gate runs either. Keep watching; do not close it yet.

## Blocked on you (Boris)

Nothing - proceed autonomously. Two things when convenient:

- **Delete the old-name fallback dirs** once a few quiet days pass (one line:
  `rm -rf ~/.config/qq ~/.local/state/qq ~/.cache/qq ~/.local/share/qq`).
- **"Claude usage check"** is still sitting in Assignments, ad-hoc. Give it a
  schedule (`daily 09:00`) or delete it.

## I can do solo (no input needed)

1. The custom HTML panel's channel (item 1).
2. The visual pass (item 2).
3. FR73 and FR65 (item 3).

## Facts - verified vs assumed

- [verified] The deployed daemon is the fresh-history build: `make deployed`
  answered `1898d37`, clean stamp, after the rename migration.
- [verified] `make check` green after every change this session (gofmt + vet +
  `go test ./... -race`; 21 packages, 564 top-level tests as of 2026-08-03).
- [verified] Post-rename live pass: a success card rendered over the desktop,
  the app window opened on Home with brand casing correct (screenshotted),
  `agentbox control state` answers, the compat symlink runs the new binary,
  `org.wails.agentbox` is on the session bus, `stats --since 30d` reads 311
  interruptions (history survived the state move).
- [verified] Mirrors live: GitHub repo renamed with the fresh history at head
  (old URL 301-redirects), profile README shows AgentBox, and
  borismilner.github.io rebuilt with zero old-name matches.
- [verified] The declutter lost nothing: every removal is in the ledger below
  or in history.md's session-36 entry, and the stale counts (30 tools, 564
  tests, fresh-history git notes) were corrected against the code.
- [assumed] The FR74 marker's behaviour over a fullscreen window (unchanged
  since session 34: mechanism proven, the marker window never mapped).
- [assumed] That `claude --model <id>` accepts whatever id Boris types into an
  assignment. The flag is still passed through unvalidated.
- [assumed] Home in the LIGHT theme, and the Assignments surface in it - both
  exercised in his dark theme only.
- [assumed] The "Claude usage check" assignment still exists and is still
  ad-hoc (not re-read from the store this session).
- [assumed] The spoken lines played audibly (`agentbox say` exited clean;
  nobody could hear it from here).

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| The session-35 HANDOFF (M12 shipped, live-state details) | `docs/history.md` "Thirty-fifth session"; still-live facts carried above |
| Roadmap M8-M11 slice narratives (269 lines) | `docs/history.md` "What M9/M10/M11 shipped" sections + "Session UI decisions (M8)"; plan/acceptance stay in `docs/05-roadmap.md` |
| The three M9 cutover defects (SIGSEGV replay, `--watch` flag order, quit-on-last-window) - roadmap was their only home | `docs/history.md`, M9 detail, cutover bullet |
| STATUS's superseded 2026-07-31 priority preamble | The quote in `docs/history.md` "Thirty-first session"; the 2026-08-01 reset stays in STATUS "Do this next" |
| Stale counts (23/20 tools, 498 tests/20 packages, "nine era commits", GitLab date) | Corrected in place: 30 tools, 564 tests/21 packages, fresh-history notes in STATUS "Environment facts" + history "Git archaeology" |
| The old-name lineage (banned from repo docs) | Agent memory `renamed-to-agentbox` (`~/.claude/projects/-home-boris-milner-me-projects-agentbox/memory/`) |

## Map

1. [docs/STATUS.md](docs/STATUS.md) - current state, what works, known gaps, the queue.
2. [docs/08-assignments.md](docs/08-assignments.md) - the M12 design and how a run hands something back. Read before touching assignments.
3. [docs/history.md](docs/history.md) - session-by-session record; this session is "Thirty-sixth".
4. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers used in commits; FR82 shipped.
5. [docs/agent-manual.md](docs/agent-manual.md) - the agent-facing reference. `internal/manual/agent.md` is the embedded short version; `internal/manual/assignment.md` is the brief a run is spawned with.
6. [CLAUDE.md](CLAUDE.md) - traps that have cost sessions; read before touching the build or the daemon.
