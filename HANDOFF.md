# Handoff - AgentBox: FR83's four primitives are all built; the teaching half of slice 5 is what is left

*Written by session 44, which built shared values, found its own ownership check
wrong by restarting the daemon, found its own probe proving nothing by looking at
which pid it recorded, and put the blackboard on the board.*

**Written:** 2026-08-04 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean, in sync with origin/main
git log --oneline -12       # newest four: af2db6c ed792e2 3528eca 0645d08
make deployed               # af2db6c9e79e or newer, NOT "(dirty)"
agentbox pending            # expect "nothing pending"
agentbox sync agents        # the roster, live
agentbox sync locks         # expect "no locks held"
agentbox sync get 'claims/*'    # expect "no keys under claims/"
tools/sync-probe.py rider   # slice 1's end-to-end check; must print PASS
tools/sync-probe.py locks   # slice 2's acceptance list; must print PASS
tools/sync-probe.py signals # slice 3's acceptance list; must print PASS
tools/sync-probe.py shared  # slice 4's; RESTARTS THE DAEMON; must print PASS
```

**Announce yourself before you touch anything** (standing rule in Boris's global
`~/.claude/CLAUDE.md`). Your own `mcp__agentbox__announce` works if your mcp child
postdates the last deploy; otherwise the CLI always does:

```bash
export AGENTBOX_SESSION_KEY="$(head -c8 /dev/urandom | od -An -tx1 | tr -d ' \n')"
export AGENTBOX_AGENT=claude
setsid agentbox sync attach --area repo:agentbox >/dev/null 2>&1 &
agentbox sync announce "<why this session exists>" --area repo:agentbox
agentbox sync activity "<what you are doing now>"     # and again as it changes
```

Two notes on that snippet, both learned the hard way. It gives you a row of your
OWN, separate from your mcp child's row, so the board shows one session twice -
prefer `mcp__agentbox__announce` if you have it, and only fall back to the CLI. And
after a `/clear` your mcp child keeps the PREVIOUS conversation's purpose, so
announce again even if a row already looks like yours.

### FR83 has no build left in it except teaching

**Slices 1 to 4 are complete, deployed and verified live.** All four primitives -
presence and discovery, locks, signals, shared values - plus the Agents surface
showing all four. What remains of the design is **slice 5, the teaching half**,
whose acceptance list in [docs/09-sync.md](docs/09-sync.md) is: a Claude session in
an unrelated project, given no instruction beyond its own configuration, announces
itself and appears in the roster; and a second session in the same repo learns about
the first without being told to look.

Three of the four doors are already open (the global `~/.claude/CLAUDE.md` section,
both manuals, the CLI). The one that is not is **the hooks in
[docs/recipes.md](docs/recipes.md)**, which no session has ever installed in a real
`settings.json` - it is the oldest `[assumed]` in this file and it is what slice 5's
acceptance actually tests. Note the ordering trap already documented there: a hook
that posts a signal before its SessionStart announce lands is refused by the
announce gate.

### Then the short queue Boris deferred until FR83 was finished

- **FR85 with FR86 together** - one agent, two identity colours (Go and the
  frontend hash with different separators), and `Project` is `filepath.Base(cwd)`
  so an agent in `frontend/src` reports project `src` and gets a different colour
  from its peers. Fixing one without the other leaves the colour split.
  `deriveArea` already computes the repo root and throws it away.
- **FR84** - a form card clips sentence-length choice options mid-word, and the
  fields sit below the fold. His words: "we'll have to think of a better visual
  approach". Mock it before building it.

Then the older queue, unchanged: **FR73** (a card body must be readable after the
card closes), **FR65** (open a citation in the editor), **FR74's fullscreen
marker** (built session 34, never exercised, needs consent to drive), and the
**"Claude usage check" assignment retry** (`a0eff4b720959`, failed on a model
limit, not a defect).

### The lock order, which four subsystems now obey

**Read every other subsystem's state BEFORE taking your own mutex.** The roster
reads observers (`asking`, `driving`, locks, listens) outside `r.mu`; the lock table
posts its `lock:NAME` signal outside `l.mu`; the signal hub's `pushed()` reads its
callback under `s.mu` and calls it outside; shared values read `presentFn`, `alive`,
`post` and `changed` under `sh.mu` and call every one of them outside. Getting any
of these backwards deadlocks the daemon on the first board repaint. The comment is
on `roster.snapshot`.

### Looking at the surface, which is how four of five slices found their defect

```bash
loginctl show-session $(loginctl | awk '/boris/ {print $1; exit}') -p LockedHint  # no = unlocked
DISPLAY=:0 agentbox app --tab agents
DISPLAY=:0 wmctrl -l | grep 'agentbox · app'          # xdotool's --name misses the middle dot
DISPLAY=:0 wmctrl -ia <WINID> && sleep 1
DISPLAY=:0 import -window <WINID> /tmp/board.png      # then READ the png, do not assume
DISPLAY=:0 xdotool getwindowgeometry <WINID>          # REQUIRED before any click - see below
```

**`xdotool` clicks in SCREEN coordinates; `import -window` captures WINDOW pixels.**
A click computed off a screenshot lands wherever the window happens to sit. This
window was at (370,170), so two "clicks on a row" both hit the sidebar and navigated
to Library, and a third "nothing happened, so the row is inert" was a click into
empty space that proved nothing. Add the origin from `getwindowgeometry` to every
coordinate you read off a png.

To drive more than a click, take the desktop first - `agentbox control request
"reason"` - and `agentbox control release` after. A single click on agentbox's own
already-focused window is the one case that does not need the strip; say it out loud
with `agentbox speak` before and after, and put the pointer back where it was
(`xdotool getmouselocation --shell` first).

For rows to look at: `tools/sync-probe.py board` (real sessions, real locks, a real
parked listener, 150s). For the blackboard specifically, a fixture has to make one
claim from a process that really dies - see the probe's `dying_claim`, and the trap
under it.

## Where we are

FR83's goal: agents on this machine that can see, find and wait for each other, plus
one surface where Boris watches all of them. **Slices 1 to 4 are complete, deployed
and verified live**, and the composition the whole feature was for now works end to
end: park on a signal, take a lock, claim a chunk, and watch the chain on the board
while it happens. Four defects were fixed on the way through this session: the
roster-only ownership check (mine, found by restarting the daemon), the probe that
proved nothing (mine, found by reading which pid it recorded), and two legibility
defects on the surface. We stopped clean: nothing half-applied, `main` pushed, the
deployed build equal to the newest Go commit.

## What this session changed, and why it matters

**Shared values shipped: the blackboard.** One `shared` tool with `op: get | set |
delete`, `agentbox sync get|set|del` beside it, migrations 0010 and 0011, and
`shared_max_bytes` in `[sync]`. Five mechanics worth carrying:

- **Zero is a value here, which is slice 3's decision used the other way round.**
  `after_seq` could fold "omitted" into 0 because nothing needed to demand a zero. A
  claim is exactly that thing: versions start at 1, so `if_version: 0` means "only
  if this key does not exist". Hence a pointer on the wire and a *string* flag on
  the CLI - an int flag cannot tell "0" from "not given", and the difference is a
  claim versus an overwrite.
- **Every CAS is ONE SQL statement**, deliberately. Read-then-write under a mutex
  would be atomic only because this daemon happens to be one process holding one
  pool - the kind of true a dev instance breaks silently. `RETURNING` is what turns
  SQL's silence about a losing write into an answer.
- **The refusal is the interesting half of the API.** A losing claim returns the
  current value, version, owner and whether that owner still runs, because
  "somebody was faster" and "somebody died holding this" call for opposite next
  moves. `stale: true` is a normal outcome, not an error.
- **Nothing here is ever trimmed**, the deliberate opposite of signals: retention
  exists because events are history, and a claim is not. A full table (1000 keys)
  refuses a NEW key rather than evicting somebody's claim, and never blocks an
  update.
- **A `get` on a key ending in `*` reads the family.** Not in the design; it is what
  makes one-key-per-item usable, and it reuses the topic prefix rule and its LIKE
  escaping rather than inventing a second one.

**The ownership check was wrong, and only restarting the daemon showed it.**
Ownership was recorded (session key, agent name) and checked against the live
roster - correct all day, and false for one second per daemon restart. The roster is
memory only, so a restart empties it, and until every mcp child redials, **every
owned claim read as abandoned**: an invitation to take over a chunk somebody is
writing, which is the exact failure the primitive exists to prevent. Migration 0009's
lesson a third time - "gone" cannot be told from "not here yet" by looking at what is
left. Migration 0011 records the owning process, and a read answers in two steps: on
the roster means alive, otherwise the pid decides. Zero means no pid was recorded,
which is honest for a CLI write.

**A probe can pass and prove nothing, and the tell was which pid it recorded.**
`Session.close()` kills the mcp child, and the pid a claim records is that child's
PARENT - the agent process, deliberately, because a child may be restarted while the
agent works on. Every `Session` in one probe run shares the python process as that
parent, so a "dead" session left a pid that was very much alive. The scenario had
passed against the roster-only build and would have passed forever. The dying
claimer is now a process of its own (the probe re-runs itself with an internal
`__claim` verb), and there are two of them: one gets taken over, one stays abandoned
across the restart.

**The blackboard is on the Agents surface**, which is the last thing 09-sync.md's
prose promised. Its own block beside the lock table, abandoned claims sorted to the
top, the abandoned count in the heading in the warning colour. One frame after a
deploy captured the distinction the feature rests on: roster rows healed to 21s old
while the claims kept their true 3m age - presence does not survive a restart,
coordination state does.

**Also, at Boris's request: a global usage-budget rule** in `~/.claude/CLAUDE.md`.
Every session periodically reads `claude -p /usage` and hands off when the weekly
limit is above 98%, or when the session limit is above 98% and its reset is more than
20 minutes away. This handoff exists because that rule fired at 97%.

## Traps this session paid for

- **`pkill -f` bit again, in the session that documents it** (session 43 did the
  same). `pkill -f board-shared.py` killed the invoking shell, exit 144, because the
  pattern matched the shell's own command line. Kill by pid from `ps -eo pid,args`.
- **`xdotool` screen coordinates versus window pixels** - see above. It cost three
  screenshots and one false conclusion.
- **A leftover claim table blocks the next fixture.** Shared values are never
  trimmed, so a fixture killed mid-run leaves its claims and the next run's
  `if_version: 0` is refused. Wipe first (`agentbox sync del KEY`).
- **A repaint racing daemon shutdown logs `shared_read_failed: sql: database is
  closed`.** Warned and returns nil, beside the pre-existing `signal_post_failed`
  noise from the same moment. Not a defect; do not chase it.

## Live state (volatile - verify on resume)

- **Deployed:** `af2db6c9e79e`, clean stamp, verified with `make deployed`. HEAD is
  `af2db6c` and it is the newest Go commit, so they match exactly.
- **Git:** clean, `main` pushed to `origin` (GitLab, which push-mirrors to GitHub).
  This session, oldest first: `0a3cab8` (store + proto + migration 0010), `ebe6880`
  (the daemon subsystem), `345f96a` (MCP tool, CLI, both manuals), `5ad9443` (the pid
  fix + migration 0011), `3ae2174` (the probe), `2d89de2` (docs), `02e3241` (the
  surface), `0645d08` (the probe's dying claimer, which the surface fixture exposed),
  `3528eca` (two surface legibility fixes), `ed792e2` (docs), `af2db6c` (the
  prefix-miss message + a test).
- **Background jobs: none.** The board fixture cleaned up and exited; no dev daemons,
  no stray `sync attach`. `pgrep -ax agentbox` should show exactly the daemon plus
  one `agentbox mcp` per live session - if a second `agentbox daemon` appears, read
  its `/proc/<pid>/environ` for `AGENTBOX_INSTANCE` before touching it. **PRs:**
  none, ever - Boris pushes `main`.
- **Nothing pending, no locks held, and the blackboard is empty.** `agentbox sync get
  'claims/*'` answers "no keys under claims/". Every probe deletes its own claims,
  unlike its signals, which stay by design.
- **The desktop was unlocked** and the Agents board was looked at directly, four
  times. If a capture returns the wallpaper, the session is locked again:
  `loginctl show-session <id> -p LockedHint` answers it. An `agentbox · app` window
  may still be open on the Library tab from the mis-aimed clicks.
- **Usage:** week (all models) at **97%** when this was written, resetting
  2026-08-05 05:00 Asia/Jerusalem. A session resuming before that reset has almost
  nothing to spend; one resuming after has a full week. Read it with
  `claude -p /usage 2>/dev/null | grep -E '^Current '`, and see the new
  usage-budget section in `~/.claude/CLAUDE.md` for when to hand off.
- **Rename fallout still on disk, on purpose** (session 36): `~/.config/qq`,
  `~/.local/state/qq`, `~/.cache/qq`, `~/.local/share/qq` are fallback copies;
  `~/.local/bin/qq` is a compat symlink.
- **In-flight edits: none.**

## Blocked on you (Boris)

Nothing - proceed autonomously. Two things you may want to weigh in on, neither of
which blocks work:

- **Whether slice 5's teaching is worth building now** or whether FR83 is finished
  enough at four primitives and the older queue should come first. The design counts
  teaching as part of the feature ("the difference between a feature that exists and
  a feature every agent uses"), and three of its four doors are already open; the
  hooks are the missing one. You were asked at the end of sessions 40 to 43 about
  continuing FR83 and did not answer, and the default has been to continue in the
  design's order.
- **FR84's visual approach.** You said it needs thinking about, so it waits for a
  mock and your eyes on it.

## I can do solo (no input needed)

1. **Slice 5's hooks** - install the `recipes.md` recipes in a real `settings.json`
   and run the acceptance list, which retires this file's oldest `[assumed]`.
2. **FR85 with FR86 together** - one identity colour, one project name, pinned by a
   test over a fixed table of identities.
3. **The lock chip's duplication** - a blocked row says "blocked: lock X, held by Y"
   in the chip AND "waiting on X for 20s, held by Y" in the line below. Honest, but
   it reads twice. Trim the chip on the surface, not in the daemon (the CLI has no
   second line).
4. **Two additions signals deliberately left out**, either of which is small: the
   row detail could list recent signals posted and received (bounded store read per
   row, so on expand rather than in the snapshot), and a listening row could show
   the wait's own age beside its topics.
5. **A shared-value row could open** the way an agent row does, showing the full
   value when it is longer than the 40ch the line gives it. Today it highlights on
   hover and does nothing on click, which is honest but is a dead end a human will
   try.
6. **FR84** last of these, mocked before built.

## Facts - verified vs assumed

- [verified] **Slice 4's whole acceptance list, live against the deployed daemon**
  (`tools/sync-probe.py shared` prints PASS): three sessions racing over a ten-chunk
  table claim 10 of 10 with zero double-claims and no session winning them all; two
  chunks abandoned by processes that really died read as ownerless and name the agent
  that left them, while eight live ones do not; the refusal on an abandoned key says
  "take it over with if_version 1"; a peer parked on `shared:probe:claims/*` is woken
  by the take-over with the key and version and NO value; a real `systemctl --user
  restart` leaves all ten claims with their versions, the live owners still live and
  the dead one still dead; the table drains; and the CLI exits 0 on a won claim and 1
  on a lost one.
- [verified] **The pid fix in the field, twice.** Once through the probe's restart,
  and once through a `make deploy`: one frame after the daemon came back, two live
  claims still read as live and the abandoned one still read as abandoned, from a
  daemon that had never seen either session.
- [verified] **The surface renders it, photographed.** All three cases at once - two
  live claims, one `owner gone` with an amber dot and its chip, and an unowned
  counter reading "no owner: shared state rather than a claim" - with the heading
  reading "4 · 1 abandoned". Clicking a shared row highlights it and does not expand
  (inert by design); clicking an agent row still opens its detail panel with the
  block above it. Both clicks re-done at true screen coordinates after the first
  attempt missed.
- [verified] **The CLI's messages, run by hand**: an empty family says "no keys
  under claims/" and exits 1; a missing key says "does not exist (version 0)" and
  exits 1; a won claim exits 0; a lost one exits 1 naming the live winner.
- [verified] `make check` passes (gofmt, vet, race) with the new tests, and slices 1
  to 3's probes still PASS against this build.
- [verified] No em-dashes, curly quotes or filler vocabulary in the lines this
  session added to the docs (checked over `git diff`, not by eye).
- [assumed] **The empty-roster case on screen.** The condition that used to hide the
  blackboard and the orphaned locks behind "No agents attached" is fixed, and the fix
  is one boolean - but it could not be photographed, because this session's own mcp
  child is always on the roster, so `agents.length === 0` never happened.
- [assumed] **The 200-key prefix cap on the surface and in a tool result.** Tested at
  the store layer only; no run has ever held more than four shared values.
- [assumed] **`@me` in a shared key.** The child expands it the way it does for
  topics; nothing exercised it.
- [assumed] **A Claude session's own `await_signal` parks correctly through a real
  client for longer than a few seconds.** The daemon side, the CLI and the MCP tool
  are all exercised, and the keep-alive ticker was measured in session 42 - but no
  park in the last two sessions lasted more than 20 seconds, so the 1500s ceiling and
  the ticker have not been exercised together on a signal.
- [assumed] **The `holder parked on ask_user` lock warning** and **the 600s long-wait
  lock warning.** Unit-tested, same `warnOf` path the deadlock refusal proved on
  screen, never run live. Carried from session 42.
- [assumed] That the hook recipes in [docs/recipes.md](docs/recipes.md) work as
  written - the CLI under them is exercised, the hooks have never been installed in a
  real `settings.json`. Left assumed by sessions 40 to 44, and it is exactly what
  slice 5's acceptance list tests.
- [assumed] That `webui-demo agents` still renders. Its fixture gained three shared
  values this session and that path was not re-opened.

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| The previous handoff's "Slice 4 is the next build" section, with its four facts (post through the signal hub, the store is at 0009, copy the trim/gap pattern for owners, any verb needing an area carries the cwd) | All four are consumed. Slice 4's record in [docs/09-sync.md](docs/09-sync.md) has what each one turned into, including the two migrations and why 0011 is not an edit to 0010 |
| The previous handoff's FR89 section (dismiss, retract, pending) as a live-state item | Shipped and verified in session 43. FR89 in [docs/07-field-requests.md](docs/07-field-requests.md); the probe's own cleanup is `OwnToasts` in `tools/sync-probe.py` |
| The previous handoff's FR90 paragraph and its three consequences | Fixed and verified in session 43. Slice 3's record in 09-sync.md ends with it, and session 43 in [docs/history.md](docs/history.md) has the full account |
| The previous handoff's "signals shipped" section, with the doorbell/AUTOINCREMENT/`after_seq` mechanics and the listening chip | All four are in slice 3's record in 09-sync.md and in session 43 of history.md. The `after_seq` decision is repeated here only where slice 4 inverted it |
| The previous handoff's "unix socket path 108-byte limit" and "test asserting at the wrong layer" traps | Session 43 in history.md. Neither recurred, and neither is on the path of the work now in front of this file |
| The previous handoff's board-fixture command list | Kept, minus the lines that only mattered to the signals fixture, plus the xdotool coordinate rule this session paid for |
| Nothing else was removed | The rest is either live state (rewritten above) or history that moved into `docs/history.md` when it happened |

## Map

1. [docs/09-sync.md](docs/09-sync.md) - FR83, the design. Slices 1 to 4 complete;
   read before any sync work, including what building each slice changed. Four of
   the five slices found something the design had wrong, and each record says what.
2. [docs/STATUS.md](docs/STATUS.md) - current state, what works, known gaps.
3. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers; FR89 and
   FR90 are fixed; FR84/FR85/FR86 are the open ones.
4. [docs/history.md](docs/history.md) - session by session; this session is
   "Forty-fourth".
5. [docs/agent-manual.md](docs/agent-manual.md) - the tool reference, now including
   `shared` and "Splitting work nobody doubles". `internal/manual/agent.md` is the
   embedded copy; both were updated, and a test (`TestManualListsEveryTool`) fails
   if a tool ships without them.
6. [docs/06-configuration.md](docs/06-configuration.md) - the `[sync]` knobs and why
   each default is what it is, including why `shared_max_bytes` has no retention
   knob beside it.
7. [docs/recipes.md](docs/recipes.md) - the hooks slice 5 needs installed.
8. [CLAUDE.md](CLAUDE.md) - traps that have cost sessions; read before touching the
   build or the daemon.
9. `tools/sync-probe.py` - `rider`, `locks`, `signals`, `shared`, `board` scenarios;
   `tools/idlecap-probe.sh` - the client's idle cap; `tools/mcp-probe.py` - one
   one-shot tool call.
