# Handoff - AgentBox: FR73 is closed, so the inbox can no longer be the place a message goes to die

*Written by session 47, which gave every inbox row a detail view, found the two
things the store cannot hold, and re-earned the layout trap the hard way - by
opening somebody else's item.*

**Written:** 2026-08-05 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean, in sync with origin/main
git log --oneline -8        # db810e0 is FR73 itself; everything after it is docs/tests
make deployed               # db810e0fd881 - OLDER than HEAD on purpose, see Live state
agentbox pending            # expect "nothing pending"
agentbox sync agents        # your own row, put there by a hook; two peers usually here
agentbox sync locks         # expect "no locks held"
make check                  # gofmt + vet + race, ~2 min
```

**Nothing is in flight.** FR73 shipped and was watched on screen. Pick the next
item from the queue below; none of it is blocked on Boris.

### The queue, in order

1. **FR65 - open a citation in the editor.** The next review-board gap: an open
   button per code block beside copy, running a configured editor command template.
   The JetBrains invocation is already worked out under "Mechanics discovered" in
   [docs/07-field-requests.md](docs/07-field-requests.md). Nothing has been built.
2. **FR74's fullscreen marker is built and has never been seen.** Session 47
   corrected STATUS about this: it is NOT unwritten. `internal/webui/control.go`
   has the marker, `controlmark_test.go` unit-tests its placement rule, the
   `_NET_WM_STATE_FULLSCREEN` read is in `x11.go`, and `keepOnTop` already makes
   the marker its one exception to top-most. Session 34 built the lot. What is
   missing is a live look: a real fullscreen window covering the strip, with the
   marker checked to be still on top of it. Needs consent to drive the desktop and
   a fullscreen app to test against. An untested marker is worth no more than none,
   because a fully covered strip reads as "the desktop is yours" while an agent
   drives.
3. **The row detail's three empty blocks on the Agents board.** `wireAgent.Timeline`,
   `.Signals`, `.Items` and `.Pending` are rendered by Agents.svelte and filled
   **only by demo.go** - `ShowRoster` never sets them, so opening an agent row shows
   the meta list and held locks and none of the history the design promises. The
   honest shape is a bridge call per opened row, which is exactly the shape FR73
   just built and proved (`Bridge.ItemDetail`, `internal/webui/inbox.go` - copy it).
   Note that **"received" signals are not recorded anywhere**: the store's
   `sync_signals` has `session_key` for the POSTER only, so received needs a bounded
   in-memory ring where a wait resolves (`internal/daemon/signals.go`), the way the
   activity ring works.
4. **A resolved review's diff and any spoken line cannot be read back.** Found while
   building FR73 and deliberately not fixed by it: this is a schema change, and FR73
   was a reader. See "Facts" below for exactly what the table holds. Worth its own FR
   number if Boris wants it; it is a named gap in STATUS now either way.

## Where we are

FR73 is closed - the one he filed in his own words, quoting what he could not
recover. **FR65 is now the only open field request**, and FR74's live look sits
behind it; both are older and quieter than FR73 was. Every inbox row now opens a
detail that reads its item back whole, and it was exercised on the exact case he
filed: a veto raised while he was away, expired on its own, its 946-character body
given back rendered with both timestamps and how long it stood. Six commits, all
pushed. Nothing half-done.

## What this session changed, and what each cost

**FR73 in one paragraph.** Clicking any row - pending or resolved - opens a detail
under it: the body through `RenderMarkdown` (the renderer the card used, so it reads
the same way after it closed), both timestamps to the minute plus how long it stood,
what it offered with the default and the taken option marked separately, a form's
answers in its fields' order rather than its map's, and the identity with hue,
session key and id. Nothing is clamped or ellipsised; the list scrolls instead.
`Bridge.ItemDetail(id)` is a call per opened row, not a field on the snapshot,
because a hundred rendered bodies in every push is what the row's 140-character
`Snippet` exists to avoid.

**The FR's "everything needed is already stored" was nearly true, and the gap only
showed in the schema.** `Speak` and `Diff` were written into `wireDetail` and then
taken back out: `proto.Item` has both fields, so it compiled and the tests passed,
but the store's insert names `id, kind, level, title, body, options, fields,
actions, cwd, timeout_s, dflt, agent, project, session, state, created_at` and
neither is a column. `RecentItems` reads that table, so both would have been empty
in every real read - a reader promising two things the read behind it cannot
deliver. **The lesson worth keeping: for a read-back feature, check the INSERT, not
the struct.**

**Two things the change itself broke, both caught before shipping.** A row used to
be inert unless pending, so nobody had reason to click into the list and then type;
with every row opening, reading one row and pressing `d` would have dismissed
whichever row `j/k` was last left on - so a click now moves the triage selection
too. And a row can change under an open detail (answered on its card, triaged from
the keyboard), which left it saying "waiting" and offering a card for something
already answered; one `$effect` now owns that and the row leaving the list.

**One guard added while the screen was locked.** Every `svc("Name")` in bridge.js
must appear in the shipped bundle. A Bridge method is where the committed-`dist`
trap is both silent and fatal - the surface resolves it by name at runtime, so a
stale bundle compiles fine and fails when Boris clicks. Verified by naming a method
the bundle cannot have and watching it fail.

**A STATUS entry was wrong and is now right.** Its priority list said FR74's marker
still needed writing, listing the three pieces it needed; all three are in the code.
Corrected rather than carried forward, and the list renumbered.

## Traps this session paid for

- **Never click a list row at a coordinate read off an earlier screenshot.** This
  is now in [CLAUDE.md](CLAUDE.md) because it cost this session a wrong click on
  **another session's item**. Session 46 recorded it as "clicking a row you already
  opened closes it and the layout moves"; it is true of ANY queue change. Two
  pending items were answered by Boris in the gap between the screenshot and the
  click, the Pending section collapsed, everything moved up. **The fix that made
  the rest of the run repeatable: type a term into the search box first, so the
  target is the only row on screen.** The queue is shared with every other session
  and no lock protects it.
- **`LockedHint` can be `yes` for hours, and there is no way around it.** It was
  `yes` from the first check to roughly two hours later. `import -window` on a
  locked screen photographs the lock screen, and **there is no Xvfb on this
  machine**, so there is no off-screen path either. Read it with
  `loginctl show-session $(loginctl | awk '/boris/ {print $1; exit}') -p LockedHint --value`
  and check it immediately before looking, never once at the start. If it is
  locked, do the docs and wait - a background `until` loop polling it costs nothing
  and wakes you.
- **Building the fixture during a locked screen turned out to be right, not a
  workaround.** A veto raised and expired while he was away IS the case FR73 was
  filed about. `agentbox veto --in 8 --body "$(cat file.md)"` and let it proceed.
- **Verify the window NAME immediately before the shutter**, because ids are
  recycled: `n=$(xdotool getwindowname $id); case "$n" in "agentbox · app") import -window $id ...`.
  Window titles differ by surface: `agentbox · app`, `agentbox · toast`,
  `agentbox · hands off`, `agentbox · <document title>`, and **a card is titled
  plainly `agentbox`** - so use `window =agentbox` in `drive_desktop` for the exact
  match, and note there is also a permanent 1x1 window named `agentbox` to skip.
- **One flat colour in the png is the tell** that the screen was locked or the
  window had not painted. `identify -format "%k colours"` - a real surface gives
  well over a thousand.
- **Hard-wrapped markdown keeps its line breaks.** `html.WithHardWraps()` is on in
  `internal/webui/mdhtml.go`, so a body written at 78 columns renders wrapped at 78
  columns and looks like a layout bug in a wide window. It is not one, and it is
  identical to what the card shows.

## Live state (volatile - verify on resume)

- **Deployed:** `db810e0fd881`, clean stamp - **older than HEAD on purpose.**
  Everything after it is markdown, one Go comment (`inbox.go`) and one test
  (`frontend/policy_test.go`), so the binary's behaviour is current. It was NOT
  redeployed because `make deploy` restarts the daemon, and two other live sessions
  reach Boris through it; a restart for a cosmetic sha match is the wrong trade.
  Redeploy freely at the start of a session, when nobody is mid-question.
- **Git:** clean, `main` pushed to `origin`. This session, oldest first: `db810e0`
  (FR73 itself), `85e9429` (FR73 closed in the docs), `c9b681a` (history),
  `7db2840` (a comment saying why the detail repeats the outcome chip), `edee067`
  (the bundle guard), `71583f6` (what was seen on screen + the CLAUDE.md trap), plus
  this handoff.
- **Background jobs: none.** The `until` loop that waited for the unlock has exited.
  `pgrep -ax agentbox` should show one daemon plus one `agentbox mcp` per live
  session. **PRs:** none, ever - Boris pushes `main`.
- **Nothing pending, no locks held.** Every fixture item this session created was
  resolved: the veto proceeded on its own, Boris answered the choice `Beta` himself,
  and the confirm was answered `no` from its card as part of the demonstration.
- **Two windows were left open on his desktop on purpose**: `agentbox · app` on the
  inbox (search box cleared) and `agentbox · FR73: the card you missed, read back`,
  a viewer holding the captures. Close them if they are in the way; nothing depends
  on them.
- **The captures live in a session scratchpad**, so they will not survive a reboot:
  `/tmp/claude-1000/-home-boris-milner-me-projects-agentbox/7c132d41-4f47-4fba-97c0-f7ce8b6caa1e/scratchpad/`
  (`04-detail-veto.png` is the one that matters). Deliberately not committed - they
  are a demonstration, not repo content.
- **Three sessions were on the board** during this one: this one, one in `~/minimus`
  ("SSVC in Advisories"), one in `~/work/assignments` ("fixing images#9879").
  Neither was touched. The roster reported `partial`, so there may have been more.
- **The desktop was taken once and released once.**
- **Usage:** session **0%** used, resets 2026-08-05 20:00 Asia/Jerusalem; week (all
  models) **5%**, resets 2026-08-12 04:59. Plenty of room.
- **In-flight edits: none.**

## Blocked on you (Boris)

Nothing - proceed autonomously. Two things carried over from the last handoff that
are still yours and still not blocking anything:

- **Your PostToolUse hook writes the raw Bash command as the activity line.** It is
  your `~/.claude/settings.json`, so the wording is yours. This session's board row
  showed a nine-line shell function as its activity, which is worse than session
  46's five-line commit message. Truncating to the first line, or the first 80
  characters, would read better.
- **FR84's other half** (a long body still pushes a form's fields below the fold)
  needs your word, because the approach that fixes it is the one you did not pick.
  The mock is in the repo: [docs/mocks/fr84-form-shapes.html](docs/mocks/fr84-form-shapes.html),
  approach C. Do not build it on a session's initiative.

## I can do solo (no input needed)

1. **FR65** - open a citation in the editor. Mechanics already in the FR doc.
2. **The Agents board's three empty blocks**, on expand rather than in the snapshot.
   `Bridge.ItemDetail` is now the worked example to copy. Received signals need a
   ring; nothing records them today.
3. **FR74's marker, watched live** - needs consent to drive the desktop, which is a
   request rather than a blocker.
4. **The read-back gap in the schema** (a resolved review's diff, a spoken line), if
   it is wanted as more than a documented gap.

## Facts - verified vs assumed

- [verified] **FR73's read-back, on the real screen, on the case Boris filed.** A
  veto with a 946-character body raised while he was away and expired on its own;
  the row said `proceeded`, opening it gave `arrived Aug 5 15:02, ended Aug 5 15:02,
  stood 8s`, the whole body rendered (bold, inline code, a numbered list), and
  `kind veto / level warning / from claude · db-migrations / id k33b0772f589bc668`.
  The body ran past the window and the rest scrolled into reach.
- [verified] **A choice read back its options**: three with descriptions, `Beta`
  carrying both `DEFAULT` and `TAKEN`, `WHAT WENT BACK: Beta`, `stood 2h27m`.
- [verified] **A pending row's `Show the card` raises the card in one click**, and
  the detail carries no "ended" line while the item waits.
- [verified] **A row changing under an open detail re-reads itself.** Answering that
  card `n` on the card moved the row to Recent, dropped the button, and grew
  `ended Aug 5 17:34, stood 1m` and `WHAT WENT BACK: no` with nothing touching the
  detail.
- [verified] **A row leaving the list closes its detail** - filtering the search to a
  term the open row does not match shut it rather than leaving it under another row.
- [verified] **The store persists no `speak` and no `diff`**, read off the insert
  statement in `internal/store/store.go` and confirmed against `Recent`'s SELECT.
- [verified] **The bundle guard can fail**, by naming a Bridge method the bundle
  cannot have and watching it name that method.
- [verified] **FR74's marker exists in code with a unit-tested placement rule**
  (`control.go`, `controlmark_test.go`, `x11.go`), which is what corrected STATUS.
- [verified] `make check` passes (gofmt, vet, race) with the new tests.
- [verified] The early line wraps in a rendered body are `WithHardWraps()` keeping
  the source's own line breaks, not a width clamp - checked in `mdhtml.go` after it
  looked like a layout defect on screen.
- [assumed] **That the `found: false` path ever paints.** It is unit-tested, and on
  the surface it is close to unreachable: the effect closes a detail whose row left
  the list, so the wording ("dropped out of the recent hundred") has never been
  seen. Carried as an assumption rather than claimed.
- [assumed] **That the keyboard route to a resolved row's detail works.** Tab to a
  row, then Enter or Space - built and reasoned about, never pressed. The mouse path
  was exercised thoroughly; this one was not.
- [assumed] **That a detail holding a mermaid diagram, math or an artifact block
  hydrates.** The `use:markdown` action is the same one the card uses, so it should,
  but no fixture with any of the three was opened in a row.
- [assumed] **That `provisionalFor` retires a hook-only row after ten minutes.** Read
  in the code, never waited out. Carried from session 45.
- [assumed] **The 200-key prefix cap** on the surface and in a tool result; tested at
  the store layer only. Carried from session 45.
- [assumed] **`@me` in a shared key**, and **a real client's `await_signal` parked
  longer than 20s** (the 1500s ceiling and the keep-alive ticker have never been
  exercised together). Carried from session 45.
- [assumed] **The `holder parked on ask_user` and 600s long-wait lock warnings** -
  unit-tested, never seen live. Carried from session 42.
- [assumed] That `webui-demo agents` still renders; that path has not been opened
  since session 44 gained three shared values in its fixture. (Note it cannot render
  at all while the deployed daemon holds the session bus - see CLAUDE.md.)
- [assumed] That the identity cross-check's no-node skip path works. Written, never
  exercised. Carried from session 46.

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| The whole FR73 research block from the last handoff (what `wireItem` carried, the two `{@html}` allowlists, `inbox.rows` as the lookup, the keyboard-triage rule) | It is all built and shipped. The decisions and what they cost are in FR73's entry in [docs/07-field-requests.md](docs/07-field-requests.md) and session 47 of [docs/history.md](docs/history.md); the code is `internal/webui/inbox.go` and `frontend/src/surfaces/Inbox.svelte` |
| Session 46's whole "What this session changed" section (FR85's two divergences, FR86, the board's dead ends, the `1fr` shell fix, FR84) | All five are closed and none is on the path of the work ahead. Session 46 in [docs/history.md](docs/history.md) carries each in full; the `1fr` lesson survives as a trap below and in `App.svelte`'s comment |
| Session 46's usage-assignment paragraph (the `qq-data` tag, the stale `cachedUsageUtilization`) | Fixed in the assignment's own prompt and proved by a run. Its record is session 46 of history.md; nothing about it is actionable now |
| Session 46's trap "clicking a row you already opened closes it and the layout moves" | Promoted and widened into [CLAUDE.md](CLAUDE.md)'s trap list, because any queue change does it and it cost this session a wrong click on another session's item |
| The last handoff's note that FR74's marker was "built session 34, never exercised" contradicting STATUS's "needs building" | Resolved in favour of the code, which has all three pieces. STATUS's priority list item 1 now says what is actually missing (a live look) |
| The "After FR73" section's four-item list | Became "The queue, in order" above, with FR73 gone and the schema gap added |

## Map

1. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers used in commits
   and handoffs. **FR65 is the only open one Boris filed**; FR73 closed 2026-08-05
   with what building it found, including the captures it was verified with.
2. [docs/STATUS.md](docs/STATUS.md) - current state, what works, known gaps, and the
   numbered priority tail behind this handoff's short queue.
3. [docs/history.md](docs/history.md) - session by session; this session is
   "Forty-seventh".
4. [docs/09-sync.md](docs/09-sync.md) - FR83, all five slices, the chip vocabulary.
5. [docs/mocks/fr84-form-shapes.html](docs/mocks/fr84-form-shapes.html) - the card
   shapes Boris chose from; approach C is the unbuilt half of FR84.
6. [docs/agent-manual.md](docs/agent-manual.md) - the tool reference.
   `internal/manual/agent.md` is the embedded copy; `TestManualListsEveryTool` fails
   if a tool ships without them.
7. [CLAUDE.md](CLAUDE.md) - traps that have cost sessions. **Read it before touching
   the build, the daemon, or driving the desktop.**
8. `tools/sync-probe.py` - `rider`, `locks`, `signals`, `shared`, `board` scenarios.
   `board` leaves orphaned holds behind on purpose; its own sessions exit early, so a
   blocked row is better made by hand with two `agentbox sync lock` wraps.
