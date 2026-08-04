# Handoff - AgentBox: FR83 slice 3 (signals) is finished; slice 4 (shared values) is last

*Written by session 43, which built signals, found its own gap check was wrong by
running it, and then found a slice-1 defect hiding behind a flaky probe.*

**Written:** 2026-08-04 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean, in sync with origin/main
git log --oneline -11       # newest: a4a98c8 18faeae f522217 16a7a8d b653d09 1d6c49e
make deployed               # 18faeae8c72e or newer, NOT "(dirty)"
agentbox sync agents        # the roster, live
agentbox sync locks         # expect "no locks held"
tools/sync-probe.py rider   # slice 1's end-to-end check; must print PASS
tools/sync-probe.py locks   # slice 2's acceptance list; must print PASS
tools/sync-probe.py signals # slice 3's acceptance list; must print PASS
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

### Slice 4 is the next build, and nothing gates it

The last slice of FR83: **shared values**, the compare-and-swap blackboard.
Acceptance list in [docs/09-sync.md](docs/09-sync.md) - one `shared` tool with
get/set/delete as operations on `{key, value, if_version}`, one key per item as the
fan-out idiom, an owner that reads as ownerless when its session is gone, a
`shared_max_bytes` cap, and a `shared:KEY` signal on every write. Four facts to
build on, each of which cost this session something:

- **The signal hub is done and every write should post through it.**
  `d.signals.emit(topic, id, data)` is the daemon-internal door: no announce gate,
  same delivery, same store. `postSignal` on the Daemon wraps it for a callback
  (`internal/daemon/sync.go`). Do not add a second wake mechanism - "waiting on a
  value" is `await_signal(["shared:claims/*"])` and nothing else.
- **The store is at migration 0009.** Add `sync_shared` as 0010; do NOT try to fold
  it into 0008, which is already applied to Boris's live database. Forward-only is
  the rule (ADR-0005) and the daemon refuses to open a schema newer than the binary
  knows, which also means **`make rollback` past a migration leaves the daemon
  unable to start**. That is true of the last two deploys.
- **The trim/gap pattern is the one to copy for owners.** What retention took is
  RECORDED rather than deduced (`sync_signal_trim`), because "gone" cannot be told
  from "never existed" by looking at what is left. An orphaned claim has the same
  shape: record the owner, and report a value whose owner is absent rather than
  inferring anything from the value alone.
- **Any verb that needs an area must carry the cwd.** FR90's whole lesson: the
  attach was not the only door a session's first call comes through, and a row with
  no area is invisible to every area-filtered read.

### The lock order, which signals had to obey too

Three subsystems now call into each other and the rule has not changed: **read every
other subsystem's state BEFORE taking your own mutex.** The roster reads
observers (`asking`, `driving`, locks, listens) outside `r.mu`; the lock table posts
its `lock:NAME` signal outside `l.mu`; the signal hub's `pushed()` reads its
callback under `s.mu` and calls it outside. Getting any of these backwards deadlocks
the daemon on the first board repaint. The comment is on `roster.snapshot`.

### Looking at the surface, which is how the defects get found

```bash
loginctl show-session $(loginctl | awk '/boris/ {print $1; exit}') -p LockedHint  # no = unlocked
tools/sync-probe.py board &      # a holder, a waiter, a LISTENER and an orphan, 150s
DISPLAY=:0 agentbox app --tab agents
DISPLAY=:0 wmctrl -l | grep 'agentbox · app'          # xdotool's --name misses the middle dot
DISPLAY=:0 wmctrl -ia <WINID> && sleep 1
DISPLAY=:0 import -window <WINID> /tmp/board.png      # then READ the png, do not assume
```

To drive it (clicking Break lock, for instance) take the desktop first -
`agentbox control request "reason"` - and `agentbox control release` after. A single
click on agentbox's own already-focused window is the one case that does not need
the strip; say it out loud with `agentbox speak` before and after, and put the
pointer back where it was (`xdotool getmouselocation` first).

## Where we are

FR83's goal: agents on this machine that can see, find and wait for each other,
plus one surface where Boris watches all of them. **Slices 1, 2 and 3 are complete,
deployed and verified live**: presence, purpose, activity, peer discovery, the
Agents surface, the discovery rider, named locks with orphaning and deadlock
refusal, and now signals - post/await over one global cursor, per-topic retention
that confesses a gap, and the built-in `agents:<area>`, `to:<key>` and `lock:<name>`
topics. **Only slice 4 (shared values) is left.** Three defects were fixed on the
way through this session: the gap check (mine, same day), and FR90 (a slice-1 hole
where a session that announced before it attached had no area at all). We stopped
clean: nothing half-applied, `main` pushed, the deployed build equal to the newest
Go commit.

## What this session changed, and why it matters

**Signals shipped, and the composition the whole feature was for now works.**
"Deploy when the tests are green" is `await_signal(["tests:green"])` then
`acquire_lock("deploy:agentbox")` - two calls, no poll loop spending a model turn
per look, and the chain visible on the board while it happens. Four mechanics worth
carrying:

- **The channel is a doorbell, not a delivery.** A woken waiter re-reads the store
  from its cursor instead of taking a payload off its channel. That is what makes a
  batch a batch, why a buffer of one is enough for the daemon's first multi-consumer
  hub, and why a wake that races a trim is harmless.
- **`after_seq` needs no third state.** Sequences start at 1, so zero IS omitted:
  "from now on" and "everything I have not seen" fit in one integer.
- **AUTOINCREMENT is load-bearing** on `sync_signals.seq`. A plain rowid is reused
  after a delete, so a quiet week that trimmed the table would restart the sequence
  at 1 under every live cursor.
- **The `listening` chip had been on the surface since slice 1 with nothing feeding
  it.** It came from the mock. It sits below `blocked` on purpose - blocked is
  contention, listening is the feature working - and a listening row holds its state
  instead of decaying to `quiet`, which is the "a parked agent must not look hung"
  case. Photographed beside a row that HAD decayed.

**The design's gap check was plausible and wrong, and only running it showed that.**
"A cursor has fallen off the edge if it is below the oldest surviving sequence" is
false for per-topic retention: one quiet topic's ancient row holds the global
minimum down while the awaited topic is trimmed away underneath. Measured on a real
daemon with `signal_keep = 1` - cursor 1, oldest surviving 1, sequences 2 and 3 gone
from the very topic asked about, batch reported as complete. Fixed by recording what
retention takes (migration 0009), per topic, written BEFORE the delete so a crash
between the two over-reports a gap rather than hiding one.

**FR90: a slice-1 defect was hiding behind a flaky probe.** The rider probe started
failing intermittently once signals shipped, and the tempting reading was noise from
another live agent on the shared roster. Two consecutive runs failing *differently*
said otherwise. Only the attach carried a cwd, and the attach is lazy, so an
announce arrives first - always for a hook announcing on a session's behalf - and
the row then had no area. Three consequences, none visible on the board: the row was
invisible to every area-filtered read (so another session's `announce` could answer
`alone: true` with a peer in the same repo), its rider cursor was never initialized
(so its next call repeated the whole area as news), and `peersOf` with an empty area
returned every agent on the machine. Fixed; the rider probe went from failing about
one run in three to 5/5.

## Traps this session paid for

- **`pkill -f` bit again, in the same session that quotes the rule.** Cleaning up a
  throwaway daemon with `pkill -f 'AGENTBOX_INSTANCE'` killed the invoking shell,
  because the pattern matched the shell's own command line (exit 144). The real
  daemon survived only because its cmdline does not contain the string. Kill by pid,
  found by reading `/proc/<pid>/environ` for the instance name.
- **A unix socket path has a ~108-byte limit.** A dev daemon under the session
  scratchpad path died with `bind: invalid argument`. Put `XDG_RUNTIME_DIR` somewhere
  short (`/tmp/x`) and only the state elsewhere.
- **A test that asserts at the wrong layer looks like a product bug.** The rider
  cursor is moved by `Daemon.SyncRider`, not by `roster.Announce`, so a test that
  called the roster directly "proved" a defect that was not there. The existing
  pattern is `d := &Daemon{roster: newRoster(...)}` plus `paramsFor`.
- **A probe's own topics must be namespaced.** `probe:*` names keep an acceptance
  run's signals and locks distinguishable from real ones, and the signals a probe
  posts STAY in the store by design - which is the feature, not a leak.

## Deferred by Boris, explicitly, until FR83 is finished

- **FR84** - a form card clips sentence-length choice options mid-word, and the
  fields sit below the fold. His words: "we'll have to think of a better visual
  approach". Mock it before building it.
- **FR85 with FR86 together** - one agent, two identity colours (Go and the
  frontend hash with different separators), and `Project` is `filepath.Base(cwd)`
  so an agent in `frontend/src` reports project `src` and gets a different colour
  from its peers. Fixing one without the other leaves the colour split.
  `deriveArea` already computes the repo root and throws it away.

Then the older queue, unchanged: **FR73** (a card body must be readable after the
card closes), **FR65** (open a citation in the editor), **FR74's fullscreen
marker** (built session 34, never exercised, needs consent to drive), and the
**"Claude usage check" assignment retry** (`a0eff4b720959`, failed on a model
limit, not a defect).

## Live state (volatile - verify on resume)

- **Deployed:** `18faeae8c72e`, clean stamp, verified with `make deployed`. HEAD is
  `a4a98c8` and it is docs only, so a `make deployed` sha older than HEAD is
  expected - compare against the newest commit that touched Go, not against HEAD.
- **Git:** clean, `main` pushed to `origin` (GitLab, which push-mirrors to GitHub).
  This session, oldest first: `ec236f1` (store + proto + migration 0008),
  `812434d` (the daemon hub, the listening chip, the built-in topics), `dc9a01b`
  (MCP tools, CLI, both manuals), `1d6c49e` (the gap fix + migration 0009),
  `b653d09` (the board fixture), `16a7a8d` (docs), `f522217` (record-before-delete),
  `18faeae` (FR90), `a4a98c8` (docs). Plus `2534981` from session 42.
- **Background jobs: none.** No probe children, no dev daemons, no stray
  `sync attach`; `/tmp/agbgap*` removed. `pgrep -ax agentbox` should show exactly
  the daemon plus one `agentbox mcp` per live session - if a second `agentbox
  daemon` appears, read its `/proc/<pid>/environ` for `AGENTBOX_INSTANCE` before
  touching it. **PRs:** none, ever - Boris pushes `main`.
- **Two pending items are mine, from the acceptance probes.**
  `tools/sync-probe.py locks` builds a lock cycle on purpose and a refused deadlock
  toasts by design, so every run leaves one, and a warning stays pending until it is
  clicked (FR89). `make deployed` reports "2 pending". If you run that probe, say so
  before Boris asks.
- **The roster holds only real sessions.** No fixtures left. To put rows back for a
  visual check use `tools/sync-probe.py board` (real sessions, real locks, a real
  parked listener, 150s) rather than hand-rolled fakes.
- **The desktop was unlocked** and the Agents board was looked at directly, twice.
  If a capture returns the wallpaper, the session is locked again:
  `loginctl show-session <id> -p LockedHint` answers it.
- **Usage:** Boris lifted the 95% cap for 2026-08-04 only ("you can use the whole
  100% of tokens for today"). The week resets 2026-08-05 05:00 Asia/Jerusalem, so
  that permission has expired for a session resuming later. Read it with
  `claude -p /usage 2>/dev/null | grep -E '^Current (session|week)'`.
- **Rename fallout still on disk, on purpose** (session 36): `~/.config/qq`,
  `~/.local/state/qq`, `~/.cache/qq`, `~/.local/share/qq` are fallback copies;
  `~/.local/bin/qq` is a compat symlink.
- **In-flight edits: none.**

## Blocked on you (Boris)

Nothing - proceed autonomously. Two things you may want to weigh in on, neither of
which blocks work:

- **Whether to finish FR83** with slice 4 (shared values) or stop after signals and
  clear the older queue. You were asked at the end of sessions 40, 41 and 42 and did
  not answer; the default, still, is to continue in the design's order. Slice 4 is
  the last one, so this is the last time the question comes up.
- **FR84's visual approach.** You said it needs thinking about, so it waits for a
  mock and your eyes on it.

## I can do solo (no input needed)

1. **Slice 4, shared values** - the acceptance list in
   [docs/09-sync.md](docs/09-sync.md), with the signal hub it needs already built
   and the migration pattern already established.
2. **FR85 with FR86 together** - one identity colour, one project name, pinned by a
   test over a fixed table of identities.
3. **FR89** - `agentbox dismiss [ID|--all]` plus a retraction for the agent that
   posted an item. Small, and it is the only reason a probe's toast has to be
   explained by hand twice a session now.
4. **The lock chip's duplication** - a blocked row says "blocked: lock X, held by
   Y" in the chip AND "waiting on X for 20s, held by Y" in the line below. Honest,
   but it reads twice. Trim the chip on the surface, not in the daemon (the CLI has
   no second line).
5. **Two additions signals deliberately left out**, either of which is small: the
   row detail could list recent signals posted and received (it needs a bounded
   store read per row, so do it on expand rather than in the snapshot), and a
   listening row could show the wait's own age beside its topics.
6. **FR84** last of these, mocked before built.

## Facts - verified vs assumed

- [verified] **Slice 3's whole acceptance list, live against the deployed daemon**
  (`tools/sync-probe.py signals` prints PASS): a parked wait woken by a post; two
  waiters on one topic both woken by one post (`delivered: 2`); a signal posted with
  nobody listening picked up afterwards by cursor with its payload intact; a timeout
  returning the cursor unchanged and a note naming it; a request addressed over
  `to:<key>` with `@me` expanded by the child and the sender's key present so a
  reply is possible; a departure arriving as `agents:repo:agentbox` with
  `event: leave`; a release arriving as `lock:probe:signal-lock` with the reason and
  `free: true`; and the CLI's `sync post` / `sync await` round trip, including exit
  1 on a timeout.
- [verified] **The gap path, live, against the DEPLOYED binary.** A throwaway daemon
  (`AGENTBOX_INSTANCE`, its own state dir, `signal_keep = 1`) with three signals on
  one topic and a cursor of 1 answered `gap: true, oldest_seq: 4` with the note
  saying the batch cannot be complete. Re-checked after every change to the trim
  path. It cannot be shown on Boris's own store, which still holds sequence 1 and so
  has nothing to confess - and the probe prints which of the two cases it exercised
  rather than passing silently either way.
- [verified] **The board renders it.** Photographed: `listening: done:*, tests:green`
  in its own colour beside `blocked: lock deploy:agentbox, held by claude` in
  another, on a board that also carried an orphaned lock and a working row. The
  listening row still read `listening` after 100 seconds while the working row
  beside it had decayed to `quiet`, which is the case the chip exists for. Clicking
  the listening row opened its detail panel intact.
- [verified] **FR90 is fixed and the flake is gone**: the rider probe ran 5/5 PASS
  after it, having failed about one run in three before (two different failure modes,
  both reproduced and captured with `agentbox logs --follow`).
- [verified] `make check` passes (gofmt, vet, race) with the new tests, and slices 1
  and 2's probes still PASS against this build.
- [verified] No em-dashes, curly quotes or filler vocabulary in the lines this
  session added to the docs (checked over `git diff`, not by eye).
- [assumed] **A Claude session's own `await_signal` parks correctly through a real
  client for longer than a few seconds.** The daemon side, the CLI and the MCP tool
  are all exercised, and the keep-alive ticker that makes a long park survive was
  measured in session 42 - but no park in this session lasted more than 20 seconds,
  so the 1500s ceiling and the ticker have not been exercised together on a signal.
- [assumed] **The `holder parked on ask_user` lock warning fires in the field.**
  Unit-tested, same `warnOf` path the deadlock refusal proved on screen, never run
  live. Carried from session 42.
- [assumed] **The 600s long-wait lock warning.** Same path, never waited out.
- [assumed] That the hook recipes in [docs/recipes.md](docs/recipes.md) work as
  written - the CLI under them is exercised, the hooks have never been installed in
  a real `settings.json`. Left assumed by sessions 40 to 43. Note that a hook that
  posts a signal before its SessionStart announce lands will be refused by the
  announce gate.
- [assumed] That `webui-demo agents` still renders. Its break says "there is no
  daemon behind it" rather than faking one, and that path was not re-opened.

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| The previous handoff's "slice 3 is the next build" section, with its four facts about the parked-call ceiling and the notice-versus-signal question | All four are consumed. The ceiling and the ticker are in [docs/09-sync.md](docs/09-sync.md)'s wait contract; the notice-versus-signal question is answered (both paths kept, for different audiences) in slice 3's record there and in the comment on `locks.post` |
| The previous handoff's FR88 section - the idle-cap measurement and its three mechanics | Done and shipped. FR88 in [docs/07-field-requests.md](docs/07-field-requests.md), the probe is `tools/idlecap-probe.sh`, and the numbers are in 09-sync.md's "Mock it before building it" |
| The previous handoff's two slice-2 build surprises (the deploy flock, the wrapped hold's pid) | Both in slice 2's record in [docs/09-sync.md](docs/09-sync.md) and in session 42 of [docs/history.md](docs/history.md); the flock reason is also a comment in the Makefile |
| The previous handoff's FR87 entry as a live-state item | Fixed in session 42 and verified; STATUS no longer lists it as open |
| The previous handoff's "the roster/lock lock order is a rule" note | Kept and widened to three subsystems above, because signals joined the same graph |
| Nothing else was removed | The rest is either live state (rewritten above) or history that moved into `docs/history.md` when it happened |

## Map

1. [docs/09-sync.md](docs/09-sync.md) - FR83, the design. Slices 1, 2 and 3
   complete; read before any sync work, including what building each slice changed.
2. [docs/STATUS.md](docs/STATUS.md) - current state, what works, known gaps.
3. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers; FR90 is the
   newest, FR84/FR85/FR86/FR89 are the open ones.
4. [docs/history.md](docs/history.md) - session by session; this session is
   "Forty-third".
5. [docs/agent-manual.md](docs/agent-manual.md) - the tool reference, now including
   the signal family and "Telling another agent, and waiting to be told".
   `internal/manual/agent.md` is the embedded copy; both were updated, and a test
   (`TestManualListsEveryTool`) fails if a tool ships without them.
6. [docs/06-configuration.md](docs/06-configuration.md) - the `[sync]` knobs and
   why each default is what it is.
7. [CLAUDE.md](CLAUDE.md) - traps that have cost sessions; read before touching the
   build or the daemon.
8. `tools/sync-probe.py` - `rider`, `locks`, `signals`, `board` scenarios;
   `tools/idlecap-probe.sh` - the client's idle cap; `tools/mcp-probe.py` - one
   one-shot tool call.
