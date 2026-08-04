# Handoff - AgentBox: FR83 is finished, all five slices, and the last one was not documentation

*Written by session 45, which set out to install a hook recipe and found it could
not work, fixed the session key underneath it, and watched a session that had never
heard of AgentBox put itself on the board in one second.*

**Written:** 2026-08-04 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean, in sync with origin/main
git log --oneline -12       # newest four: 940ce52 0112716 67c8e9c 0fd62bf
make deployed               # 940ce52ac690 or newer, NOT "(dirty)"
agentbox pending            # expect "nothing pending"
agentbox sync agents        # your own row should be here, put there by a hook
agentbox sync locks         # expect "no locks held"
tools/sync-probe.py rider   # slice 1; must print PASS
tools/sync-probe.py locks   # slice 2; must print PASS
tools/sync-probe.py signals # slice 3; must print PASS
tools/sync-probe.py shared  # slice 4; RESTARTS THE DAEMON; must print PASS
```

**Announcing yourself is now automatic**, which is new as of this session and is
the whole of slice 5. A SessionStart hook in `~/.claude/settings.json` puts your row
on the board before you do anything, and its stdout tells you which peers share your
area - so you have probably already been told. Still call
`mcp__agentbox__announce` yourself: the hook can only write a placeholder purpose
(`agentbox session (purpose not yet stated)`), and your row is dim and uninformative
until you replace it with a line Boris would recognise. Then keep `set_activity`
current; the PostToolUse hooks keep the line honest between your calls.

The CLI fallback in previous handoffs (`export AGENTBOX_SESSION_KEY=...` plus
`setsid agentbox sync attach`) is **obsolete and was wrong** - it is what produced a
second row for one session. Do not use it. `agentbox sync announce` from any shell
inside your session now finds your session by itself.

### FR83 is done. The queue Boris deferred until it was finished

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

### What slice 5 changed under everything, and why it matters to you

**The session key is now DERIVED, not minted.** `proc-PID-STARTTIME`, where PID is
the agent process found by walking up past the shells (`agentProcess` in
`cmd/agentbox/main.go`, the same walk `agentName` already used). Read the long
comment on `inheritedSessionKey`; the short version is that a session has two
mouths - its mcp child and its hooks - and a minted key is a secret the child
cannot hand to a hook, because a hook runs inside an environment Claude Code has
already finished building. Claude's own session id does not bridge them either: the
child keeps the id it was spawned with and `/clear` mints a new one.

Consequences worth knowing before you touch this:

- **`inheritedSessionKey` may return empty; `sessionKey` may not.** That is
  deliberate. A CLI call belonging to no session must be refused, where minting a
  random key would put a phantom row on the board once per invocation.
- **The start time is load-bearing.** Pids are recycled, and without it a new
  process landing on a dead agent's number would inherit its locks and its claims.
  `procStatField` reads it by documented field number; field 21 is usually zero and
  field 23 is a memory size, and the test pins it against `/proc/uptime` because
  both of those would pass a check that only asked for non-zero.
- **A session whose mcp child predates the deploy still shows two rows.** Its child
  holds the old random key while its hooks derive the new one. Transitional, clears
  as sessions restart, and this session is an example of it (row
  `d9cab8bb468444d5`).
- **`|| true` on the installed hooks is not defensive habit.** `agentbox sync`
  exits 4 with no daemon and `make deploy` stops the daemon on purpose, so without
  it one deploy prints a hook failure into every live session on the machine.

### The change that lives OUTSIDE this repo

`~/.claude/settings.json` now has three AgentBox hooks (one SessionStart, two
PostToolUse). Boris's own hooks - atuin, fingerprint-guard - and everything outside
`.hooks` were left byte-identical, verified by diffing `del(.hooks)` before and
after. **This is his live config and it is not under version control**, so if you
need to revert:

```bash
jq '(.hooks.SessionStart, .hooks.PostToolUse) |= map(select([.hooks[].command] | any(test("agentbox sync")) | not))
    | del(.hooks.SessionStart | select(length == 0))' ~/.claude/settings.json > /tmp/s.json
# read /tmp/s.json, then move it into place
```

The canonical copy of what was installed is `docs/recipes.md`, and
`agentbox docs setup` prints it pointed at the installed binary.

### The lock order, which four subsystems obey

**Read every other subsystem's state BEFORE taking your own mutex.** The roster
reads observers (`asking`, `driving`, locks, listens) outside `r.mu`; the lock table
posts its `lock:NAME` signal outside `l.mu`; the signal hub's `pushed()` reads its
callback under `s.mu` and calls it outside; shared values read `presentFn`, `alive`,
`post` and `changed` under `sh.mu` and call every one of them outside. Getting any
of these backwards deadlocks the daemon on the first board repaint. The comment is
on `roster.snapshot`.

### Looking at the surface, which is how five of five slices found their defect

```bash
loginctl show-session $(loginctl | awk '/boris/ {print $1; exit}') -p LockedHint  # no = unlocked
DISPLAY=:0 agentbox app --tab agents
DISPLAY=:0 wmctrl -l | grep 'agentbox · app'          # xdotool's --name misses the middle dot
DISPLAY=:0 wmctrl -ia <WINID> && sleep 1
DISPLAY=:0 import -window <WINID> /tmp/board.png      # then READ the png, do not assume
DISPLAY=:0 xdotool getwindowgeometry <WINID>          # REQUIRED before any click
```

**`xdotool` clicks in SCREEN coordinates; `import -window` captures WINDOW pixels.**
Add the origin from `getwindowgeometry` to every coordinate you read off a png. To
drive more than a click, take the desktop first (`agentbox control request
"reason"`) and release after.

For rows to look at: `tools/sync-probe.py board` (real sessions, real locks, a real
parked listener, 150s). Slice 5 needs no fixture - opening any new Claude session
puts a real row there.

## Where we are

FR83's goal: agents on this machine that can see, find and wait for each other, plus
one surface where Boris watches all of them. **All five slices are complete,
deployed and verified live.** Four primitives (presence, locks, signals, shared
values), the Agents surface showing all four, and now the teaching that makes it the
default rather than an option nobody uses. We stopped clean.

## What this session changed, and why it matters

**Slice 5 was the item four sessions had left alone because it looked like prose,
and it contained a build.** Three of its four doors had been opened by writing
documentation. The fourth needed a program to run, and it had been wrong since
session 40. The recipe told you to export a random `AGENTBOX_SESSION_KEY`, which no
hook can ever see - so every hook wrote a SECOND row, and two half-stale rows read
as two agents on the one surface whose job is saying how many there are. The tell
that killed the next idea, using Claude's own session id, was reading two
environments side by side: this session's mcp child carried `60bb7c0f` while its own
shell carried `bd75f814`.

**The generalisable lesson: a door nobody has walked through is not a door.** It is
worth carrying past this feature, because the repo that writes down every trap it
pays for had this one sitting in a docs file for four sessions.

**Both halves of the acceptance were run for real**, and neither is visible in a
diff. A fresh `claude -p` in a scratch directory that had never heard of AgentBox
appeared on the roster within one second, with no instruction and no token spent on
the announce. Then a session in this repo, forbidden in its prompt from using any
tool, answered "two other agents are in this directory with me", named this
session's purpose, and said how it knew: "this session's startup hook auto-announced
me and handed back the roster of peers in my area". A SessionStart hook's stdout is
context, which is the mechanism the second half of the acceptance rests on and which
the design never stated.

**One thing that looked like a defect and was not.** A hook-announced row reads
`[detached]` with no pid behind it, which looks like a row nothing can ever retire.
`provisionalFor` already retires an unattached row after ten minutes. The design had
thought about it; the ten-minute comment says why.

## Traps this session paid for

- **A recipe that has never been executed is a guess**, however carefully written.
  This one had survived an adversarial review.
- **Settings hooks load at session start.** This session installed them and does not
  run them; only new sessions do. Do not conclude from your own row that the install
  failed.
- **`claude -p` is the cheap way to test a hook end to end.** It fires SessionStart,
  spawns an mcp child and exits, for a few hundred tokens. Poll the roster while it
  runs; its row goes when the process does.
- **Your own row is not evidence about a fix younger than your mcp child.** This
  session's row came back from every deploy on its announce-time activity rather than
  the line `set_activity` had moved it to, which is FR87 and was fixed in session 42.
  The child started at 16:49 and the fix landed at 18:59, so it runs a binary without
  `rememberActivity`. Check `ps -o lstart` against the fix before filing a
  regression.
- **The scratch project is at**
  `/tmp/claude-1000/.../scratchpad/unrelated-widget-shop` and the settings backup
  beside it. Session scratchpads are not durable; the revert recipe above does not
  depend on them.

## Live state (volatile - verify on resume)

- **Deployed:** `940ce52ac690`, clean stamp, verified with `make deployed`. HEAD is
  `940ce52` plus this handoff's own commit, so the newest code commit is deployed.
- **Git:** clean, `main` pushed to `origin` (GitLab, which push-mirrors to GitHub).
  This session, oldest first: `0fd62bf` (the derived key + its tests), `67c8e9c` (the
  records), `0112716` (the `|| true` fix), `940ce52` (the board's wording, found by looking at
  it), and this handoff.
- **`~/.claude/settings.json` carries three new hooks** (see above). Not under
  version control, so no commit records it.
- **Background jobs: none.** `pgrep -ax agentbox` should show the daemon plus one
  `agentbox mcp` per live session; if a second `agentbox daemon` appears, read its
  `/proc/<pid>/environ` for `AGENTBOX_INSTANCE` before touching it. **PRs:** none,
  ever - Boris pushes `main`.
- **Nothing pending, no locks held, blackboard empty.**
- **The desktop was unlocked and the Agents board was looked at twice**, before and
  after the wording fix. An `agentbox · app` window may still be open on the Agents
  tab. If a capture returns the wallpaper the session is locked again:
  `loginctl show-session <id> -p LockedHint` answers it.
- **Two dead `unrelated-widget-shop` rows may still be on the board**, left by the
  `claude -p` fixtures. They are hook-announced rows with no child behind them, so
  `provisionalFor` retires them ten minutes after the last one was written.
- **Usage:** week (all models) at **97%** when this was written, resetting
  2026-08-05 05:00 Asia/Jerusalem, unchanged from session 44's reading. Read it with
  `claude -p /usage 2>/dev/null | grep -E '^Current '`.
- **Rename fallout still on disk, on purpose** (session 36): `~/.config/qq`,
  `~/.local/state/qq`, `~/.cache/qq`, `~/.local/share/qq` are fallback copies;
  `~/.local/bin/qq` is a compat symlink.
- **In-flight edits: none.**

## Blocked on you (Boris)

Nothing - proceed autonomously. Two things you may want to weigh in on:

- **The hooks in your global settings.** They are live, they cost no tokens, and
  every new session on the machine now announces itself. If the placeholder purpose
  or the activity lines read wrong on your board, that is a wording call and it is
  yours. Revert recipe above.
- **FR84's visual approach.** You said it needs thinking about, so it waits for a
  mock and your eyes on it. Unanswered since session 40.

## I can do solo (no input needed)

1. **FR85 with FR86 together** - one identity colour, one project name, pinned by a
   test over a fixed table of identities.
2. **The lock chip's duplication** - a blocked row says "blocked: lock X, held by Y"
   in the chip AND "waiting on X for 20s, held by Y" in the line below. Honest, but
   it reads twice. Trim the chip on the surface, not in the daemon (the CLI has no
   second line).
3. **Photograph a hook-announced row** on the real board, which this session never
   did - the one visual thing slice 5 leaves unverified.
4. **Two additions signals deliberately left out**: the row detail could list recent
   signals posted and received (bounded store read per row, so on expand rather than
   in the snapshot), and a listening row could show the wait's own age beside its
   topics.
5. **A shared-value row could open** the way an agent row does, showing the full
   value when it is longer than the 40ch the line gives it. Today it highlights on
   hover and does nothing on click, which is honest but is a dead end a human will
   try.
6. **FR84** last of these, mocked before built.

## Facts - verified vs assumed

- [verified] **Slice 5's whole acceptance list, live, twice.** A fresh `claude -p`
  in a directory that had never heard of AgentBox on the roster in 1s with the
  placeholder purpose; and a tool-forbidden session in this repo naming its peers
  and attributing them to its own startup hook. Re-confirmed after the final deploy
  with the exact installed config: one row, key `proc-675459-4319514`, 1s.
- [verified] **A hook and an mcp child write ONE row.** A session whose model called
  `announce` itself held exactly one row while alive (three for this repo, minus this
  session's two), and its purpose replaced the hook's placeholder in place.
- [verified] **The two-row failure it fixes, watched happening.** An announce run by
  hand from this session put a duplicate of this very session on the board, because
  this session's child predates the fix.
- [verified] **The daemon-down case is silent.** `sync announce` exits 4 with no
  daemon; wrapped in `|| true` as installed it exits 0, so no session shows a hook
  failure during a deploy. Both states run by hand.
- [verified] **Everything outside `.hooks` in `~/.claude/settings.json` is
  unchanged**, by diffing `jq -S 'del(.hooks)'` before and after.
- [verified] **`agentbox docs setup`'s new snippet is valid JSON** and points at the
  invoking binary, parsed with `jq -e`.
- [verified] `make check` passes (gofmt, vet, race) with the new tests, and the
  derived key's five tests run the walk over a real process tree with both shapes in
  it.
- [verified] No em-dashes, curly quotes or filler vocabulary in the lines this
  session added, checked over `git diff` rather than by eye.
- [verified] **How a hook-announced row looks on the real board, photographed
  twice.** Its own area group with the project path, the chip reading "seen, not
  attached", the derived key on the row, and the age beside the line. The first
  capture found the defect: the line read "last seen through an item", true only of
  the card path it was written for and false for the hook that is now the usual
  source. It reads "announced on its behalf" now, and the second capture confirms it
  live. Five of five slices have now found something by looking.
- [assumed] **That `provisionalFor` really retires a hook-only row after ten
  minutes.** Read in the code and it explains what was seen; no run waited it out.
- [assumed] **A non-Claude agent's derived key.** The walk stops at the first
  non-placeholder ancestor, so `aider` or a bare script gets its own key - but only
  Claude Code has been exercised. A hand-run call from Boris's terminal resolves to
  `gnome-shell`, which is a key rather than a refusal; harmless for the keyless verbs
  and untested for the others.
- [assumed] **The 200-key prefix cap on the surface and in a tool result.** Tested at
  the store layer only; no run has ever held more than four shared values.
- [assumed] **`@me` in a shared key.** The child expands it the way it does for
  topics; nothing exercised it.
- [assumed] **A Claude session's own `await_signal` parking through a real client for
  longer than a few seconds.** The daemon side, the CLI and the MCP tool are all
  exercised and the keep-alive ticker was measured in session 42, but no park in the
  last three sessions lasted more than 20 seconds, so the 1500s ceiling and the
  ticker have not been exercised together on a signal.
- [assumed] **The `holder parked on ask_user` lock warning** and **the 600s long-wait
  lock warning.** Unit-tested, same `warnOf` path the deadlock refusal proved on
  screen, never run live. Carried from session 42.
- [assumed] That `webui-demo agents` still renders. Its fixture gained three shared
  values in session 44 and that path has not been re-opened.

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| The previous handoff's CLI announce snippet (`export AGENTBOX_SESSION_KEY`, `setsid agentbox sync attach`) and its two "learned the hard way" notes | Deleted rather than moved: it is the thing this session proved wrong, and the double row it warned about was caused by the snippet itself. The mechanism that replaced it is under "What slice 5 changed" above and in 09-sync.md's "Identity: the session key" |
| The previous handoff's "FR83 has no build left in it except teaching" section, with the four-doors state and the ordering trap | Consumed. Slice 5's record in [docs/09-sync.md](docs/09-sync.md) has what installing it found; the four doors are all open and door 3 says so |
| The previous handoff's "shared values shipped" section, with its five mechanics (zero is a value, one-SQL-statement CAS, the refusal is the interesting half, nothing is trimmed, `*` reads the family) | All five are in slice 4's record in 09-sync.md and in session 44 of [docs/history.md](docs/history.md). None is on the path of the work now in front of this file |
| The previous handoff's ownership-check and probe-pid paragraphs | Slice 4's record in 09-sync.md, which carries migration 0009's lesson in full, and session 44 in history.md |
| The previous handoff's three shared-value traps (`pkill -f`, a leftover claim table, the `shared_read_failed` shutdown race) | Session 44 in history.md. `pkill` is in [CLAUDE.md](CLAUDE.md) where every session reads it; the other two only bite a session running the shared fixture |
| The previous handoff's global usage-budget paragraph | Shipped into `~/.claude/CLAUDE.md`, which every session loads. Only the current reading is kept, under live state |
| Nothing else was removed | The rest is either live state (rewritten above) or history that moved into `docs/history.md` when it happened |

## Map

1. [docs/09-sync.md](docs/09-sync.md) - FR83, the design. **All five slices
   complete**; read before any sync work. Every slice's record says what building it
   changed, and all five found something the design had wrong. "Identity: the session
   key" carries slice 5's mechanism.
2. [docs/STATUS.md](docs/STATUS.md) - current state, what works, known gaps.
3. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers; FR84/FR85/FR86
   are the open ones.
4. [docs/history.md](docs/history.md) - session by session; this session is
   "Forty-fifth".
5. [docs/recipes.md](docs/recipes.md) - the hooks, now installed, and why there is no
   key in the snippet.
6. [docs/agent-manual.md](docs/agent-manual.md) - the tool reference.
   `internal/manual/agent.md` is the embedded copy; a test
   (`TestManualListsEveryTool`) fails if a tool ships without them.
7. [docs/06-configuration.md](docs/06-configuration.md) - the `[sync]` knobs and why
   each default is what it is.
8. [CLAUDE.md](CLAUDE.md) - traps that have cost sessions; read before touching the
   build or the daemon.
9. `tools/sync-probe.py` - `rider`, `locks`, `signals`, `shared`, `board` scenarios;
   `tools/idlecap-probe.sh` - the client's idle cap; `tools/mcp-probe.py` - one
   one-shot tool call.
