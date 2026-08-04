# Handoff - AgentBox: FR83 slice 1 is finished; slice 2 (locks) is next

*Written by session 41, which looked at the live Agents board for the first time,
fixed the four defects that looking found, and built the discovery rider.*

**Written:** 2026-08-04 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean, in sync with origin/main
git log --oneline -8        # newest code: d3c6c19 94ae073 f9c9c07 e73997e 0566667
make deployed               # 94ae0735dd40 or newer, and NOT "(dirty)"
agentbox sync agents        # the roster, live
agentbox control state      # "no run: the desktop is the human's"
tools/sync-probe.py rider   # slice 1's own end-to-end check; must print PASS
```

**Announce yourself before you touch anything** (standing rule in Boris's global
`~/.claude/CLAUDE.md`, and this repo is where it was built). Your own
`mcp__agentbox__announce` works if your mcp child postdates the last deploy; if it
is missing from your tool list, the CLI always works:

```bash
export AGENTBOX_SESSION_KEY="$(head -c8 /dev/urandom | od -An -tx1 | tr -d ' \n')"
export AGENTBOX_AGENT=claude                          # new: skips name guessing
setsid agentbox sync attach --area repo:agentbox >/dev/null 2>&1 &
agentbox sync announce "<why this session exists>" --area repo:agentbox
agentbox sync activity "<what you are doing now>"     # and again as it changes
```

### Looking at the surface, which is how every defect this session fixed was found

Slice 2 renders lock and orphan rows, so it has to be looked at too. The whole
recipe, no re-learning needed:

```bash
loginctl show-session $(loginctl | awk '/boris/ {print $1; exit}') -p LockedHint  # no = unlocked
DISPLAY=:0 agentbox app --tab agents
W=$(DISPLAY=:0 xdotool search --name "agentbox · app" | head -1)
DISPLAY=:0 xdotool windowactivate $W && sleep 1
DISPLAY=:0 import -window $W /tmp/board.png     # then READ the png, do not assume
```

If a capture returns the wallpaper and `_NET_ACTIVE_WINDOW` fails, the session is
locked and that is not a broken window. Put rows on the board first (snippet under
Live state) or the surface has nothing to show.

### Slice 2 is the next build, and one measurement gates it

**Measure the MCP client's tool-call idle cap first.** It is the last guessed
number in the design (`wait_max_s = 1500` in
[docs/09-sync.md](docs/09-sync.md)) and it decides what a parked lock wait may
do, so it gates slices 2 and 3 but nothing else. Park a call past the cap against
a fresh `agentbox mcp`, once with progress notifications and once without.
`tools/sync-probe.py` already has the plumbing - a `Session` class that keeps a
child open and a `tool(name, args, timeout=...)` that waits - so the probe is a
new scenario function in that file, not new machinery.

Then build slice 2 from the acceptance list in
[docs/09-sync.md](docs/09-sync.md), with two facts already established:

- **A CLI hold dies at 120s.** A foreground shell call from a Claude session is
  killed at exactly 120s (SIGTERM, exit 143); an explicit timeout caps at 600s.
  `make deploy` runs longer than that, so `agentbox sync lock NAME -- CMD` cannot
  be the naive wrap: `--ttl` is the normal path, and a wrapped hold must release
  on SIGTERM or every long command leaves an orphan.
- **`internal/webui/agents.go` has a `mockBreak` that slice 2 deletes.** The mock
  (`agentbox webui-demo agents`) is still the only way to see the lock and orphan
  rows, so keep it working until the real thing renders them.

## Where we are

FR83's goal: agents on this machine that can see, find and wait for each other,
plus one surface where Boris watches all of them. **Slice 1 is complete, deployed
and verified live** - presence, purpose, activity, peer discovery by repo, the
Agents surface with real data on screen, the discovery rider, and the teaching
that makes every session announce itself. Slices 2 to 4 (locks, signals, shared
values) are untouched. We stopped at a clean point: nothing half-applied, the
roster free of test fixtures, `main` pushed and the deployed build equal to HEAD.

## What this session changed, and why it matters

**The board was lying in four ways, and only looking found any of them.** All four
are fixed, deployed, and re-checked on screen; the details and the evidence are in
[docs/history.md](docs/history.md) (forty-first session). The two general rules
worth carrying into slice 2:

- **Throttling is only safe with something that flushes it.** `roster.Flush` had
  no caller anywhere, though its own comment said the daemon ticked it, so a push
  dropped inside the 250ms throttle waited for unrelated traffic - over a minute,
  measured. Locks and signals will want the same throttle; they need the same tick.
- **State derived from elapsed time has to be recomputed by a clock, not by
  traffic.** `workingFor` decayed only when some verb caused a push, so an idle
  board froze at "3 working" beside ages of 4m53s while the CLI called the same
  rows quiet. A lock's own age and a wait's timeout have this shape too.

**The discovery rider works and is proved end to end.** When a session's area
gains or loses an agent, one line rides back on the next response envelope
(`sync` on the JSON-RPC response) and the child appends it to that tool's result.
Two mechanics that were not obvious:

- **`proto.Identity.Via` is load-bearing, not bookkeeping.** A session's hooks
  call the CLI with that session's own key several times a minute, so without a
  way to tell an mcp child from a shell, every arrival was consumed by whichever
  hook fired next and the model heard nothing. Only `via: mcp` spends the news.
- **The child dials the daemon through six separate helpers.** Wiring two of them
  passed a unit test and failed live on `set_activity`, the most frequent tool
  there is. Any new sync tool must collect its rider through `CallRidden` +
  `noteRider`, or it will silently drop the line.

## Traps this session paid for

- **`go build ./...` does NOT rewrite `./agentbox`.** It builds the packages and
  leaves the binary from whenever you last ran `make build`. Probing with a stale
  `AGENTBOX_BIN` cost a full debugging cycle chasing a bug that was already fixed.
  Use `go build -o agentbox ./cmd/agentbox` or `make build`.
- **`tools/mcp-probe.py` already existed** and the previous handoff implied no
  probe did, which is why a second one got written. It does one-shot calls only;
  `tools/sync-probe.py` (added this session) holds several sessions open, and
  that is what anything about two agents needs.
- **A rider is a text block, not structured output.** A probe that reads only the
  structured result sees nothing and concludes the feature is broken.

## Deferred by Boris, explicitly, until FR83 is finished

- **FR84** - a form card clips sentence-length choice options mid-word, and the
  fields sit below the fold. His words: "we'll have to think of a better visual
  approach". Mock it before building it.
- **FR85** - one agent, two identity colours (Go and the frontend hash with
  different separators). **Do this together with FR86**, which is the same story
  arriving by a second route: `Project` is `filepath.Base(cwd)`, so an agent in
  `frontend/src` reports project `src` and gets a different colour from its peers
  in the same repo. Fixing one separator without fixing the project name leaves
  the colour still split.

New this session, both recorded in
[docs/07-field-requests.md](docs/07-field-requests.md), neither fixed:

- **FR86** - the project name above. Cheap: `deriveArea` already computes the repo
  root and throws it away.
- **FR87** - a daemon restart replays the *announce's* activity line, so every row
  comes back saying something that was true an hour ago, timestamped as fresh.
  Visible after any `make deploy`.

Then the older queue, unchanged: **FR73** (a card body must be readable after the
card closes), **FR65** (open a citation in the editor), **FR74's fullscreen
marker** (built session 34, never exercised, needs consent to drive), and the
**"Claude usage check" assignment retry** (`a0eff4b720959`, failed on a model
limit, not a defect).

## Live state (volatile - verify on resume)

- **Deployed:** `94ae0735dd40`, verified by `make deployed`, clean stamp. **It holds
  every Go change this session made**; the commits after it are docs and
  `tools/*.py` only, which never enter the binary. So a `make deployed` sha older
  than HEAD is expected here and is not a signal that something was left
  undeployed - compare against the newest commit that touched Go.
- **Git:** clean, `main` pushed to `origin` (GitLab, which push-mirrors to GitHub
  on its own). This session's commits: `0566667` (the four board defects),
  `e73997e` (the rider), `f9c9c07` (the rider on every tool, and naming who left),
  `94ae073` (docs), `d3c6c19` (the two-session probe).
- **Background jobs:** none. Every `agentbox sync attach` and every probe child
  this session started is gone; `pgrep -x agentbox` should show the daemon plus
  one `agentbox mcp` per live session. **PRs:** none, ever - Boris pushes `main`.
- **The roster holds only real sessions.** No test fixtures. To put rows back for
  a visual check, one line per fake session:

  ```bash
  ( AGENTBOX_SESSION_KEY=peer1 AGENTBOX_AGENT=codex setsid agentbox sync attach --area repo:agentbox >/dev/null 2>&1 & )
  AGENTBOX_SESSION_KEY=peer1 agentbox sync announce "FR73: the inbox reader" --area repo:agentbox
  AGENTBOX_SESSION_KEY=peer1 agentbox sync activity "editing internal/webui/inbox.go"
  ```

  Kill them by pid - **never `pkill agentbox` or `pkill -f`**, which killed a
  session's own shell on 2026-08-04. Iterate `pgrep -x agentbox`, read
  `/proc/PID/cmdline`, and kill only what matches `sync attach`.
- **Another session was live in `~/me/d2d`** while this was written (a real peer,
  different project, no coordination needed). Check the roster rather than
  assuming you are alone.
- **The desktop was unlocked** and `agentbox app --tab agents` was looked at
  directly. If a capture of the window rect returns the wallpaper and
  `_NET_ACTIVE_WINDOW` fails, the session is locked again and that is not a broken
  window: `loginctl show-session <id> -p LockedHint` answers it.
- **Usage:** 88% of the weekly limit when this was written; **Boris lifted the 95%
  cap for 2026-08-04 ("you can use the whole 100% of tokens for today")**. That
  permission was for today only. The week resets 2026-08-05 05:00 Asia/Jerusalem.
  Read it with `claude -p /usage 2>/dev/null | grep -E '^Current (session|week)'`.
- **Rename fallout still on disk, on purpose** (session 36): `~/.config/qq`,
  `~/.local/state/qq`, `~/.cache/qq`, `~/.local/share/qq` are fallback copies;
  `~/.local/bin/qq` is a compat symlink.

## Blocked on you (Boris)

Nothing - proceed autonomously. Two things you may want to weigh in on, neither of
which blocks work:

- **Whether to keep going through FR83's slices** (locks, then signals, then
  shared values) or to stop after slice 1 and clear the older queue first. You
  were asked at the end of session 40 and did not answer; the default, still, is
  to continue in the design's order.
- **FR84's visual approach.** You said it needs thinking about, so it waits for a
  mock and your eyes on it.

## I can do solo (no input needed)

1. **The MCP idle-cap probe** - the last guessed number in the design, and it
   gates slices 2 and 3.
2. **Slice 2, locks** - acceptance list in [docs/09-sync.md](docs/09-sync.md),
   with the measured 120s ceiling changing the Makefile wrap the design describes.
3. **Slices 3 and 4**, signals and shared values, in that order.
4. **FR85 with FR86 together** - one identity colour, one project name, pinned by
   a test over a fixed table of identities.
5. **FR87** - the child replays its latest activity, not the announce's.
6. **FR84** last of these, mocked before built.

## Facts - verified vs assumed

- [verified] **The Agents surface renders real roster data correctly.** Looked at
  on screen with four live rows, before and after the fixes, and photographed
  both times. The same rows that read "3 working" at ages of 4m53s now read
  `quiet` with the working count gone from the header, agreeing with
  `agentbox sync agents` at the same instant.
- [verified] The four board defects are fixed in the deployed build: state decays
  on an idle board, a throttled push arrives, the area caption is the repo root
  (checked with an agent in `frontend/src`) and absent for a declared foreign
  area, and names read `agent`/`codex`/`aider` rather than `systemd`.
- [verified] **The discovery rider works end to end against the deployed daemon**,
  by two live mcp children: announce carries no rider, an unrelated call with
  nothing new is silent, a peer joining puts the line on the next `set_activity`,
  it is said once, and a hook's CLI call fired in between does not eat it. Repeat
  with `tools/sync-probe.py rider`.
- [verified] **A child re-attaches after a daemon restart and replays its
  announce** - session 40 had this as `[assumed]`. Watched through two
  `make deploy` restarts. It replays the announce's activity line, not the
  latest one, which is FR87.
- [verified] `make check` passes (gofmt, vet, race) with the new tests in it, and
  the two tick tests fail with the tick neutered - checked by neutering it and
  re-running, not by assuming.
- [verified] No em-dashes, curly quotes or filler vocabulary in any file this
  session touched.
- [assumed] That the hook recipes in [docs/recipes.md](docs/recipes.md) work as
  written. The CLI underneath them is exercised and `AGENTBOX_AGENT` is now
  documented there, but the hooks have never been installed in a real
  `settings.json`. Session 40 left this assumed and it still is.
- [assumed] That `webui-demo agents` still renders after this session's
  `area_path` change. The fixture was updated in the same commit and the code
  path is shared with the live surface, but the mock itself was not re-opened.
- [assumed] That the rider reads well *in a real agent's transcript*. It was read
  as JSON from a probe, not seen arriving mid-task in a Claude session, because
  every child on this machine predates the deploy that added it.

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| The previous handoff's "The one thing Boris has not seen" section (the Agents surface with real data) | Done and photographed. What looking at it found is [docs/history.md](docs/history.md) session 41 and the four defect entries in [docs/STATUS.md](docs/STATUS.md) |
| The previous handoff's discovery-rider description as "not built, pick this up first" | Built, deployed, verified. The design's own record is the slice 1 entry in [docs/09-sync.md](docs/09-sync.md), which now carries what the rider forced on the design (`via`, the envelope, the remembered peer name) |
| The previous handoff's note that session 40's probe script was gone and should be rewritten | Superseded: `tools/mcp-probe.py` already existed (one-shot calls) and `tools/sync-probe.py` now holds several sessions open. Both are committed and self-documenting, so no future session needs to rewrite either |
| The previous handoff's paragraph on speaking stdio JSON-RPC by hand (the ~80-line recipe) | The recipe is the tool now: `tools/sync-probe.py`, whose docstring says what it gets right that a here-doc does not |
| The previous handoff's "`partial: true` is expected while any child predates the deploy" note | Still true and still by design, but no longer a live caveat worth top billing: it is the documented behaviour in [docs/agent-manual.md](docs/agent-manual.md) and the design's "absence is never asserted on partial data" rule |
| Nothing else was removed | The rest of session 40's handoff is either still live state (rewritten above) or history that moved into `docs/history.md` when it happened |

## Map

1. [docs/09-sync.md](docs/09-sync.md) - FR83, the design. Slice 1 complete; read before any sync work, and read the slice 1 entry for what building it changed.
2. [docs/STATUS.md](docs/STATUS.md) - current state, what works, known gaps, the queue.
3. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers; FR86 and FR87 are the newest.
4. [docs/history.md](docs/history.md) - session by session; this session is "Forty-first".
5. [docs/agent-manual.md](docs/agent-manual.md) - the tool reference, including the rider ("The line you did not ask for"). `internal/manual/agent.md` is the embedded copy.
6. [docs/recipes.md](docs/recipes.md) - the hooks that keep the roster honest for nothing.
7. [CLAUDE.md](CLAUDE.md) - traps that have cost sessions; read before touching the build or the daemon.
8. `tools/sync-probe.py` - two live mcp sessions at once; `tools/mcp-probe.py` - one tool call.
