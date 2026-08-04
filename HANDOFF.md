# Handoff - AgentBox: FR83 slice 2 (locks) is finished; slice 3 (signals) is next

*Written by session 42, which measured the client's idle cap, found every blocking
card had been dying at 30 minutes, and then built and verified the locks.*

**Written:** 2026-08-04 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean, in sync with origin/main
git log --oneline -7        # newest: 34a699d 4053f57 1310614 e8bef04 b5a72b8 2e7ef4a de9ba4d
make deployed               # 1310614d158a or newer, NOT "(dirty)"
agentbox sync agents        # the roster, live
agentbox sync locks         # the lock table; expect "no locks held"
tools/sync-probe.py rider   # slice 1's end-to-end check; must print PASS
tools/sync-probe.py locks   # slice 2's whole acceptance list; must print PASS
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

Two notes on that snippet, both learned the hard way this session. It gives you a
row of your OWN, separate from your mcp child's row, so the board shows one
session twice - prefer `mcp__agentbox__announce` if you have it, and only fall
back to the CLI. And after a `/clear` your mcp child keeps the PREVIOUS
conversation's purpose, so announce again even if a row already looks like yours.

### Slice 3 is the next build, and nothing gates it

The measurement that gated slices 2 and 3 is **done** (see below), so slice 3 is
a straight read of the acceptance list in [docs/09-sync.md](docs/09-sync.md):
post/await, one global cursor, gap reporting on trimmed retention, migration
0008, the built-in `agents:<area>` and `to:<key>` topics. Four facts to build on:

- **A parked call is safe now.** The keep-alive ticker
  (`internal/mcp/keepalive.go`) sends one progress notification a minute while
  any tool call is parked, so `await_signal` can park up to `wait_max_s` (1500s)
  without the client aborting it. Do not add a second ticker.
- **`wait_max_s` is load-bearing for a reason the design did not know:** the
  client **abandons** an aborted call without telling the server - no
  cancellation, no closed pipe - so nothing outside the daemon will ever end a
  park it has given up on.
- **A notice can ride the rider instead of a signal.** "The human broke your
  lock" was designed as a `lock:NAME` signal and shipped on the slice-1 envelope
  instead (`locks.notices` + `SyncRider`). When slice 3 lands, decide whether to
  keep both paths or route notices through signals; do not do it twice by
  accident.
- **The lock/roster lock order is a rule** and signals will face it too: the
  roster reads every observer BEFORE taking `r.mu`, because subsystems call back
  into it. The comment is on `roster.snapshot`.

### Looking at the surface, which is how the defects get found

```bash
loginctl show-session $(loginctl | awk '/boris/ {print $1; exit}') -p LockedHint  # no = unlocked
tools/sync-probe.py board &      # a holder, a waiter and an orphan, held 150s
DISPLAY=:0 agentbox app --tab agents
DISPLAY=:0 wmctrl -l | grep 'agentbox · app'          # xdotool's --name misses the middle dot
DISPLAY=:0 wmctrl -ia <WINID> && sleep 1
DISPLAY=:0 import -window <WINID> /tmp/board.png      # then READ the png, do not assume
```

To drive it (clicking Break lock, for instance) take the desktop first -
`agentbox control request "reason"` - and `agentbox control release` after.

## Where we are

FR83's goal: agents on this machine that can see, find and wait for each other,
plus one surface where Boris watches all of them. **Slices 1 and 2 are complete,
deployed and verified live**: presence, purpose, activity, peer discovery, the
Agents surface, the discovery rider, and now named locks with orphaning, deadlock
refusal, break, and the board rendering holds and waits. Slices 3 and 4 (signals,
shared values) are untouched. Two shipped defects were fixed on the way: FR88
(every blocking card died at 30 minutes) and FR87 (a restart rewound every
activity line). We stopped clean: nothing half-applied, `main` pushed, the
deployed build equal to the newest Go commit.

## What this session changed, and why it matters

**The measurement that was supposed to be a number was a shipped defect.** Claude
Code aborts a stdio tool call silent for **1800s**, and nothing in the child ever
spoke - so a card Boris answered at minute 40 replied to a caller that was already
gone, and `timeout_s: 0` ("waits forever") was the worst case rather than the
safest. FR88 in [docs/07-field-requests.md](docs/07-field-requests.md); fixed by
`internal/mcp/keepalive.go`. Three mechanics worth carrying:

- **The cap lives in the client**, so no probe written as an MCP client can find
  it. `tools/idlecap-probe.sh` drives two headless `claude -p` sessions against a
  throwaway MCP server that only parks - one silent, one ticking. 35 minutes of
  wall clock, and it is repeatable.
- **The client sends `_meta.progressToken` on every `tools/call`**, so a server
  may always keep a call alive. Progress resets the idle clock; nothing else does.
- **A second, hard ceiling exists at 1e8 ms (~27.8 h)** and is effectively no
  limit. Only the idle cap bites.

**Locks work, and the design was wrong in two places.** Both were found by trying
to satisfy its own acceptance list rather than by reading code:

- **`make deploy` cannot take a sync lock.** It stops the daemon the lock lives
  in, so the hold vanishes mid-install and the second agent gets a green light at
  the worst moment. The deploy takes an flock now (`make deploy` waits and says
  who holds it). Any future lock over agentbox's own lifecycle has this problem.
- **A wrapped hold named the wrong process.** The lock must be taken before the
  command starts, so the only pid it can name then is the wrapper's - and a killed
  wrapper looks like finished work while the command runs on. The wrap re-points
  the hold at the command once it exists (a re-acquire may correct the pid).

## Traps this session paid for

- **Never `pkill -f`.** `pkill -f "go test ./internal/mcp"` killed the invoking
  shell, because the pattern matched the shell's own command line. It is in
  CLAUDE.md and it still cost a cycle. Kill by pid, from `pgrep -x`.
- **A test that hangs is worse than a test that fails.** A `t.Fatalf` while an MCP
  tool handler was still parked deadlocked the SDK session's `Close`, so a broken
  keep-alive hung the suite. Release parked handlers in `t.Cleanup`.
- **`xdotool search --name "agentbox · app"` finds nothing** through the shell
  quoting; `wmctrl -l` finds the window. The app window id was `0x03600008` this
  session, and window ids do not survive a restart.
- **A probe that reads only structured output misses the rider** (a text block),
  and one that reads only text misses the lock results (structured). `structured()`
  and `riders()` in `tools/sync-probe.py` are the two halves.

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

- **Deployed:** `1310614d158a`, clean stamp, verified with `make deployed`. HEAD
  is `34a699d`, and the two commits after `1310614` are docs only, so a
  `make deployed` sha older than HEAD is expected here - compare against the newest
  commit that touched Go, not against HEAD.
- **Git:** clean, `main` pushed to `origin` (GitLab, which push-mirrors to
  GitHub). This session: `aa81aa2` (keepalive + the measurement), `de9ba4d` (the
  lock table), `2e7ef4a` (tools, CLI, surface), `b5a72b8` (the pid fix, break CLI,
  probe scenarios), `e8bef04` (docs), `1310614` (FR87), `4053f57` (docs).
- **Background jobs: none.** No probe children, no stray `sync attach`. `pgrep -x
  agentbox` should show the daemon plus one `agentbox mcp` per live session.
  **PRs:** none, ever - Boris pushes `main`.
- **The pending "Deadlock refused: probe:repo" toasts are mine, from testing.**
  `tools/sync-probe.py locks` constructs a lock cycle on purpose, and a refused
  deadlock toasts by design, so every run fires one - and a warning stays pending
  until it is clicked, so they came back after each `make deploy`. Boris asked
  about them mid-session; that is FR89 (an item cannot be retracted or cleared
  without the mouse). If you run that probe, say so before he asks.
- **The roster holds only real sessions.** No fixtures left. To put lock rows back
  for a visual check use `tools/sync-probe.py board` (real sessions, real locks,
  150s) rather than hand-rolled fakes.
- **The desktop was unlocked** and both the Agents board and the Break lock
  confirm were looked at directly. If a capture returns the wallpaper, the session
  is locked again: `loginctl show-session <id> -p LockedHint` answers it.
- **Usage:** Boris lifted the 95% cap for 2026-08-04 only ("you can use the whole
  100% of tokens for today"). The week resets 2026-08-05 05:00 Asia/Jerusalem, so
  that permission has expired for a session resuming later. Read it with
  `claude -p /usage 2>/dev/null | grep -E '^Current (session|week)'`.
- **Rename fallout still on disk, on purpose** (session 36): `~/.config/qq`,
  `~/.local/state/qq`, `~/.cache/qq`, `~/.local/share/qq` are fallback copies;
  `~/.local/bin/qq` is a compat symlink.

## Blocked on you (Boris)

Nothing - proceed autonomously. Two things you may want to weigh in on, neither of
which blocks work:

- **Whether to finish FR83** (signals, then shared values) or stop after locks and
  clear the older queue. You were asked at the end of sessions 40 and 41 and did
  not answer; the default, still, is to continue in the design's order.
- **FR84's visual approach.** You said it needs thinking about, so it waits for a
  mock and your eyes on it.

## I can do solo (no input needed)

1. **Slice 3, signals** - acceptance list in [docs/09-sync.md](docs/09-sync.md),
   with the parked-call ceiling already measured and the ticker already shipped.
2. **Slice 4**, shared values.
3. **FR85 with FR86 together** - one identity colour, one project name, pinned by
   a test over a fixed table of identities.
4. **The lock chip's duplication** - a blocked row says "blocked: lock X, held by
   Y" in the chip AND "waiting on X for 20s, held by Y" in the line below. Honest,
   but it reads twice. Trim the chip on the surface, not in the daemon (the CLI
   has no second line).
5. **FR89** - `agentbox dismiss [ID|--all]` plus a retraction for the agent that
   posted an item. Small, and it is the only reason a probe's toast had to be
   explained by hand.
6. **FR84** last of these, mocked before built.

## Facts - verified vs assumed

- [verified] **The client's idle cap is 1800s of silence, and progress ticks
  defeat it.** Measured twice against the real client: a silent park was aborted
  at 1800s with the client's own message; a park with progress every 120s ran
  2100s and returned normally. `tools/idlecap-probe.sh`.
- [verified] **Slice 2's whole acceptance list, live against the deployed
  daemon** (`tools/sync-probe.py locks` prints PASS): grant, a refusal carrying
  the holder's purpose and activity, a timeout as a result, the announce gate
  teaching an unannounced session, a refused deadlock naming both locks, an orphan
  that outlives its session and is NOT handed on while its pid lives, and the
  broken-lock notice arriving on a later `set_activity`.
- [verified] **The board renders it and Break lock works by clicking.**
  Photographed: an orphan block reading "its pid N is still alive, so nobody gets
  this until it exits" with a Break lock button; a holder row with a
  `deploy:agentbox` chip; a waiter reading `blocked: lock deploy:agentbox, held by
  claude` with a clickable holder. Clicking Break showed "Reassigns the lock. It
  does not stop the process." and, after confirming, the orphan block cleared and
  the board repainted from the daemon's push.
- [verified] **The deadlock warning reaches Boris as a toast** - photographed.
- [verified] **The CLI wrap holds and releases** - `agentbox sync lock test:demo
  -- sh -c 'sleep 4'` showed a row named `sh` on the board with the lock chip, and
  the table was empty the moment the command exited.
- [verified] **FR87 is fixed across a real daemon restart**: the row came back on
  the line the session had moved to, not the announce's.
- [verified] `make check` passes (gofmt, vet, race) with the new tests, and the
  keep-alive tests fail with the ticker neutered - checked by neutering it.
- [verified] No em-dashes, curly quotes or filler vocabulary in the files touched.
- [assumed] **The `holder parked on ask_user` warning fires in the field.** It has
  a unit test, and its toast path is the same `warnOf` the deadlock refusal proved
  on screen, but the combination was never run live.
- [assumed] **The 600s long-wait warning.** Same path, never waited out.
- [assumed] That the hook recipes in [docs/recipes.md](docs/recipes.md) work as
  written - the CLI under them is exercised, the hooks have never been installed in
  a real `settings.json`. Left assumed by sessions 40, 41 and 42.
- [assumed] That `webui-demo agents` still renders. Its break now says "there is
  no daemon behind it" instead of faking one, and that path was not re-opened.
- [assumed] That a Claude session's own `acquire_lock` tool call parks correctly
  through a real client. The daemon side and the CLI are exercised; the MCP tool
  was driven by the probe's children, which are real mcp children but not a real
  Claude turn.

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| The previous handoff's "measure the MCP client's idle cap first" section, with its recipe and its warning that every wait number was a guess | Done. The numbers and the two surprises are FR88 in [docs/07-field-requests.md](docs/07-field-requests.md), the design's wait contract in [docs/09-sync.md](docs/09-sync.md), and the probe itself is `tools/idlecap-probe.sh` |
| The previous handoff's two established facts for slice 2 (the 120s CLI ceiling, and `mockBreak` needing to survive) | Both consumed: the 120s ceiling is why `sync lock` has `--ttl` and a signal-safe wrap (comment at the top of `cmd/agentbox/synclock.go`), and `mockBreak` is deleted now that real rows render |
| The previous handoff's FR87 entry as an open defect | Fixed and verified; the entry in `docs/07-field-requests.md` now carries the fix and its two limits |
| The previous handoff's four board defects and the discovery-rider description | History: session 41 in [docs/history.md](docs/history.md), and the slice 1 record in `docs/09-sync.md` |
| The previous handoff's `tools/mcp-probe.py` vs `sync-probe.py` confusion note | Both are committed and self-documenting; this handoff names which does what once, under Traps |
| Nothing else was removed | The rest is either live state (rewritten above) or history that moved into `docs/history.md` when it happened |

## Map

1. [docs/09-sync.md](docs/09-sync.md) - FR83, the design. Slices 1 and 2 complete;
   read before any sync work, including what building each slice changed.
2. [docs/STATUS.md](docs/STATUS.md) - current state, what works, known gaps.
3. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers; FR89 is the
   newest, FR84/FR85/FR86/FR89 are the open ones.
4. [docs/history.md](docs/history.md) - session by session; this session is
   "Forty-second".
5. [docs/agent-manual.md](docs/agent-manual.md) - the tool reference, now including
   the lock family and "Taking turns over a shared resource".
   `internal/manual/agent.md` is the embedded copy; both were updated.
6. [docs/06-configuration.md](docs/06-configuration.md) - the `[sync]` knobs and
   why each default is what it is.
7. [CLAUDE.md](CLAUDE.md) - traps that have cost sessions; read before touching
   the build or the daemon.
8. `tools/sync-probe.py` - `rider`, `locks`, `board` scenarios;
   `tools/idlecap-probe.sh` - the client's idle cap; `tools/mcp-probe.py` - one
   one-shot tool call.
