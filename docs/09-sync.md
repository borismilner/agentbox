# 09 - Sync (multi-agent coordination and the agent roster)

**Sync** is one feature with two halves that share every moving part. Agents
coordinate among themselves - take turns on a shared resource, hand work to
each other, wake each other, message each other - through the daemon they
already share. And the human sees all of it live: every agent's purpose, what
it is doing right now, what it holds and what it waits on, in one surface.

Requested by Boris 2026-08-04 (session 39). FR83. This document is the design;
nothing here is implemented. It has survived one adversarial review, whose
findings are folded in. Status: awaiting owner triage, the surface mock, and
the slice-0 spike. An ADR comes at implementation kickoff, the way ADR-0012
did for the review board, and it records what the mock and the spike changed
rather than restating this document.

## Why

Boris, 2026-08-04, five statements in one session:

- *"Multiple concurrent agents can use it in AgentBox to synchronize among
  themselves with maximal efficiency."*
- *"I as a user should be able to monitor using the GUI what exactly each such
  agent is doing at the moment in the most convenient and informative way."*
- *"Every agent using the platform must provide a short description of the
  purpose of the agent and the current thing the agent is doing and update
  these as they change so that I can monitor all works in the most convenient
  way possible."*
- *"Other agents should easily find existing or new joining agents that are
  working on the same area as they are to know they now need to coordinate
  because they are not working alone and may interfere with each other's
  work."*
- *"Agents can for example communicate among themselves and discover each
  other using this platform to achieve maximal cooperation and optimal
  synchronization."*

The repo already carries the scars this feature exists to prevent. CLAUDE.md's
"Traps that have cost sessions" are social locks - rules written down because
nothing enforces them: never `pkill agentbox` (every session holds an mcp
child), `make run` displaces the deployed daemon that other live sessions
reach Boris through. HANDOFF.md records the GitLab-to-GitHub mirror race.
The VM cost rule shares one expensive machine between sessions with no
arbitration but Boris's attention. And FR74 shipped a lock for one resource,
the desktop, with "two real agents racing" still on its not-verified list.
Every one of these is the same missing thing: the agents share a machine, a
repo, a daemon and a human, and they cannot see or wait for each other.

What a session does instead today is ask Boris to sequence the agents by hand
- a card whose answer is "wait, the other session is deploying" - or trust a
rule in a doc. The human is the mutex. That is the workaround whose cost
argues for the feature.

## The shape in one paragraph

The daemon is the one meeting point every agent already has: a single
instance per user session (flock), a unix socket every `agentbox mcp` child
and every CLI call already dials, sub-millisecond round trips, and one
process that can see every waiter at once. Sync adds a small subsystem there:
a **roster** of live agent sessions (who is here, why, what they are doing),
**locks** (take turns), **signals** (wake each other, message each other),
and **shared values** (small state with compare-and-swap). Discovery falls
out of the roster: joining tells an agent who already works its area, a join
is itself a signal, and every later call carries word of new company. Every
wait is one parked, resumable call inside the daemon - no polling loops
anywhere. The webui renders the roster live; the human can always see who
holds what, who waits on whom, and break a lock that has gone wrong.

## Identity: the session key

Today `proto.Identity{Agent, Project, Session}` cannot tell two sessions
apart: `Agent` is the parent process name, `Project` the directory basename,
and `Session` is empty unless AgentBox itself spawned the agent. Two Claude
sessions in one repo are the identical triple. FR74 already lives with the
consequence - the control run checks ownership by agent-name equality, so a
same-named second session can write the first one's HANDS OFF activity line.

Sync cannot be built on that, so it starts with the fix: a fourth identity
field, the **session key**, minted once by the mcp child at startup (or taken
from AGENTBOX_SESSION_ID when AgentBox spawned the session) and stamped on every
call the child makes, like the rest of the identity already is. Every sync
ownership check - who holds this lock, whose roster row, whose private topic
- is the key and only the key. The control run's ownership check moves to the
key in the same change; that is a shipped-defect fix riding along, not a
refactor for its own sake.

## Presence: the attach connection

The daemon has no idea which agents exist. Identity travels inside each
request, the MCP child dials a fresh connection per tool call, and the only
liveness signal in the whole system is "a currently-blocking call's context"
(FR45).

Sync gives the child one persistent connection. The child dials the daemon
and holds an **attach** stream open for its whole life, carrying its identity
and key. The daemon keys a roster row to that connection:

- Attach arrives - the agent is present. The row starts provisional: agent
  name, project, working directory, pid, started-at, no purpose yet.
- Attach drops - the agent is gone. After a short grace (survives a child
  restart) the row is removed and a signal is posted. What happens to its
  locks is deliberately more careful - see Locks.
- Daemon restarts - children redial with backoff and replay their last
  announce, so the roster heals without the agents' models ever knowing.

Two rules keep the attach from creating new traps:

- **The attach never spawns the daemon.** The client's default dial
  auto-spawns; the attach dials with spawn disabled. Otherwise every live
  session's redial loop would resurrect the daemon that `make stop` just
  stopped, and the working-copy workflow in CLAUDE.md would lose the flock
  race to a background reconnect. Acceptance for slice 1 checks this
  directly.
- **The attach is lazy.** The child connects on its first tool call, not at
  process startup, so opening a terminal does not raise the daemon, the tray
  and a webview. The vision's rule stays: the first *call* auto-spawns.

CLI callers and hook scripts are not sessions and get no row; they act on
behalf of one (`--key`) or of nobody. A non-MCP agent that wants a row holds
`agentbox sync attach` open for its lifetime - same contract, same door.

## The mandate: purpose and activity

Boris's third statement is a contract, not a nicety: an agent on this
platform owes the human one line of *why it exists* and one line of *what it
is doing right now*, kept current. The design takes it seriously in four
layers, because a mandate that relies on model discipline alone is a wish:

1. **The child registers what it knows, unasked.** Agent name, project,
   working directory, pid, start time. The roster is complete even if the
   model never says a word - a rude agent shows up as `claude · agentbox · no
   purpose given`, dim, rather than invisible.
2. **`announce` states the purpose.** One non-blocking tool:
   `announce(purpose, activity?, area?)` - "porting the settings surface to
   the new theme". Called once at session start, updated if the mission
   changes. The agent manual and the CLAUDE.md recipe make it the first
   AgentBox call a session makes.
3. **`set_activity` is generalized.** FR74's tool already exists with the
   right meaning and stays one tool: it updates the caller's roster row
   always, and the hands-off strip additionally whenever that caller holds
   the control run. "Running make check", "editing internal/daemon/sync.go".
   Updates are coalesced in the daemon (last line wins, per-session rate
   cap), so an eager agent costs nothing.
4. **The sync verbs enforce it.** Locks, signals and shared values refuse an
   unannounced session with a teaching error naming `announce` - the
   self-teaching rule the rest of AgentBox follows (vision principle 9). The
   presence row and the monitoring reads stay ungated: visibility must not
   depend on good manners.

Zero-token coverage for Claude Code sessions comes from hooks, documented in
recipes.md: a SessionStart hook posts a provisional purpose from the
session's first prompt, and a PostToolUse hook posts `agentbox sync activity
"Edit internal/daemon/sync.go"` as the agent works - the ticker stays
truthful even when the model forgets it, and it costs no tokens because
hooks are shell, not model. Assignments announce themselves: the runner
knows the assignment's name and passes it as the purpose before the child's
first token.

Self-report colors the row; it never defines the row's state. The daemon
knows facts no self-report can fake, and the facts win (the state table
below).

## Discovery: knowing you are not alone

Boris's fourth statement is about the moment two agents' work starts to
overlap: each must find the other *easily*, and a newcomer must be findable
the moment it joins. Discovery is the roster read four ways, and the
important one is the push:

- **Every call carries word of company.** The daemon keeps, per session, a
  cursor on its area's roster generation. When any AgentBox tool result goes
  back to a session whose area gained or lost an agent since that session's
  last call, the response envelope carries a one-line rider and the child
  appends it to the tool result: "sync: a second agent joined this repo -
  claude, 'fixing the settings surface', working". No extra call, no
  parking, no hook, and it arrives at the moment the agent is about to act.
  This is the mechanism that serves "new joining agents" for real; the
  others are entry points and fallbacks.
- **Joining answers "who else is here".** `announce` returns, in the same
  call, the roster rows sharing the caller's area: identity, purpose,
  activity, holds. An agent learns it is not alone in its first AgentBox call,
  before it has touched anything.
- **A join is a signal.** The daemon posts `agents:<area>` signals on join,
  announce and leave, carrying the row. An agent that is genuinely idle can
  park on its area topic; one mid-work relies on the rider instead.
- **`list_agents` filters.** By project, by area tag, or everything - the
  same snapshot the surface renders, so agent and human never see different
  rosters.

**Area is derived first, declared second.** The child canonicalizes what it
can see for free: the project name, and the git top-level and origin URL
when the working directory is a repo - so two checkouts or worktrees of one
repo land in the same area without anyone spelling it. `announce` takes
optional `area` tags in the same `kind:scope` idiom (`subsystem:webui`) for
finer or coarser matching.

**Absence is never asserted on partial data.** A session whose mcp child
predates the sync deploy has no attach and no row, but its items still flow
through the daemon with identity on them. The roster derives "seen recently,
not attached" rows from that traffic, renders them as their own kind, and
`announce`/`list_agents` return `partial: true` while any exist - "you are
alone" must be true when said, or not said (the FR61 rule, applied to
presence).

Discovery is the soft guard and locks are the hard one: finding a peer tells
an agent to coordinate - partition the work, message the peer, or take the
lock it would otherwise have raced.

## The Agents surface

A new rail surface in the app window - **Agents** - plus the Home "agents
working" tile pointing at it. Rows group by area, so two agents in one repo
sit visibly together and overlap is something Boris sees before either agent
does. One row per roster entry:

- The identity pill in the agent's existing hue, the agent and project, and
  the session name when AgentBox spawned it.
- **Purpose** - the announce line. The headline of the row.
- **State chip** - derived from what the daemon observes, in priority order:

| Chip | The daemon knows it because |
|---|---|
| asking you | a blocking item of theirs is pending (the waiters map) |
| driving desktop | they hold the control run |
| blocked: lock NAME | they are parked in `acquire_lock`, with holder and age |
| listening: TOPIC | they are parked in `await_signal` |
| reporting: task 64% | they own a live FR21 progress report |
| working | activity line fresher than a threshold |
| quiet | present, but nothing reported lately - the age is shown |
| no purpose given | present and never announced; the dim row |
| seen, not attached | pre-sync child; derived from item traffic only |

- **Activity** - the self-reported line and how long ago it changed, in the
  strip's own `driving · <activity> · 12s` grammar, which Boris already
  reads.
- **Holds and waits** - chips for held locks; a wait names the holder, and
  clicking it goes to the holder's row. Two agents waiting on each other is
  a drawn edge, not a diagram the human assembles in his head.

Clicking a row opens the detail: the full announce, the recent activity
timeline (a small in-memory ring per session), held locks with ages, recent
signals posted and received, and the agent's recent items from history.
**Break lock** lives here, behind the two-step confirm the library's delete
uses - and its copy says plainly what it does and does not do: breaking
reassigns the lock, it does not stop the ex-holder. The break posts a
`lock:NAME` signal, and the ex-holder's next sync call of any kind carries
the broken notice in its result, so a working agent finds out at its next
touch of the platform rather than only if it happens to touch that lock.

Ages are computed from a monotonic base, and a wall-clock jump (suspend,
resume) rebases every `since` and suppresses wait warnings for one interval
- otherwise the first morning unlid would read every agent as "quiet" and
fire a warning per outstanding wait.

The surface is quiet. No card, no toast, no sound for normal coordination -
interruption stays a cost (vision principle 1). Two exceptions, both about
locks, where waiting means contention rather than success: a **deadlock**
(always a warning toast naming the cycle) and a **long lock wait**
(`[sync] wait_warn_s`, default 600s, 0 disables). A parked `await_signal`
never warns: listening is the intended steady state, and warning on it would
train Boris to ignore the toast that matters.

The wire is the house pattern: the sync subsystem pushes rows over a new
`agentbox:agents` event channel, coalesced to a few emits per second; the
surface consumes the same roster `list_agents` reads, so the two doors
cannot disagree.

## Locks

Named, exclusive, advisory leases. A name is a convention, not a registry -
`deploy:agentbox`, `repo:agentbox`, `vm:boris-vm`, `mirror:github` - and the
manual teaches the `kind:scope` idiom the way it teaches walkthrough
authoring.

- `acquire_lock(name, timeout_s, note?, release_on_detach?)` **blocks** until
  granted or timed out. `timeout_s: 0` waits as long as the caller does -
  the meaning 0 has in every shipped blocking tool, kept on purpose. A
  timeout is a result, not an error, and it carries everything the caller
  needs to decide without a second call: the holder's identity, purpose,
  activity, hold age, and the queue length.
- `try_lock(name, note?)` is the non-blocking form: an immediate grant or an
  immediate refusal carrying the same full picture. A separate tool because
  blocking and non-blocking never share one (the FR74 rule, and this doc
  nearly broke it in its first draft).
- `release_lock(name)` releases. Every release posts a `lock:NAME` signal
  with the reason - released, holder gone, broken - so a waiter always
  learns *why* it won.
- Waiters queue FIFO. No priorities, no shared/read mode, no reentrancy
  count in v1 - each is complexity with no field evidence yet, and the FR
  list is where the evidence would arrive.
- A hold is keyed to the session key, not to a call connection, so an agent
  holds across tool calls. A CLI hold is different on purpose:
  `agentbox sync lock NAME -- CMD` wraps a command flock-style, the hold tied
  to the command's lifetime, released on exit however it exits; `--ttl`
  exists for a detached hold from a script that cannot wrap.

**A dead holder does not silently free a live resource.** The attach dying
proves the mcp child died; it says nothing about the work - a `make deploy`
the session started keeps running after the terminal closes. So on attach
drop a held lock goes **orphaned**, not released: the roster shows it with
the recorded pid, waiters are told the holder is gone, and the grant happens
only when the recorded pid is gone too (a liveness probe, re-checked on a
short tick) or when the human breaks it. `release_on_detach: true` at
acquire time opts a hold into immediate release, for the case where the
session *is* the critical section. The naive version of this - release after
a five-second grace - hands the resource to a second agent while the first
one's deploy still runs, which is the failure the lock exists to prevent.

**Deadlock is refused at acquire time, by name.** The daemon sees the whole
wait-for graph, so an acquire that would close a lock cycle fails
immediately: "refused: would deadlock - you hold repo:agentbox, claude
('docs pass') waits on it while holding deploy:agentbox, which you asked
for". Cheap at this scale, and it turns the worst failure mode of locks into
an ordinary refusal. The graph also carries two edge kinds that are not
locks, because the stalls that will actually happen route through them: *is
parked on a human answer* (the waiters map) and *holds the control run*. A
would-be cycle through those cannot be refused - the human's card is already
up - so it warns instead: the toast names the chain ("deploy:agentbox waits on
an agent that is waiting on you"), which is the one moment a coordination
toast earns its interruption.

**Why queueing, when control refuses.** The control run's no-queue rule is
right for the desktop: the window between ask and grant is human-scale, the
resource is the human's own mouse, and a hidden queue would hand it to an
agent that has moved on. Sync locks are the opposite case: the caller states
its patience explicitly, the queue is visible in the roster rather than
hidden, and the waiter is a parked call that re-arms in one call if it times
out. Refusal-only locks would force every agent into a poll loop, which is
the waste this feature exists to end. Both rules stay, each where its
argument holds - and `try_lock` gives sync the control-style answer when a
caller wants it.

## Signals

Fire-and-forget events with durable pickup, adjacent to what FR19 parked
(server push for companion surfaces) but its own thing: agent-to-agent.

- `post_signal(topic, data?)` - non-blocking, returns the signal's sequence
  number. Topics are free-form names under the `kind:scope` idiom
  (`tests:green`, `handoff:docs`, `lock:deploy:agentbox`). Data is a small
  JSON payload, capped like a card body.
- `await_signal(topics, after_seq?, timeout_s)` - **blocks** until a
  matching signal arrives or the timeout passes, and returns *everything
  matching since the cursor in one batch* plus the next cursor. A waiter
  that was busy when three signals fired catches up in one call. `after_seq`
  omitted means "from now on"; a cursor means "everything I have not seen".
- **One sequence, defined matching.** Sequence numbers are one global
  monotonic counter (the store's rowid), so a cursor is a single number no
  matter how many topics a wait spans. A topic pattern is an exact name or
  a prefix ending in `*` (plain string prefix: `done:*`, `shared:claims/*`);
  `@me` in a pattern is expanded by the child to the caller's own private
  topic, so the daemon has no magic tokens.
- Delivery is fan-out: every parked waiter whose pattern matches wakes with
  the same signal. Deliberately the first multi-consumer hub in the daemon -
  artifacts and walkthrough submissions are single-consumer hand-offs and
  stay that way; a signal is a broadcast by meaning.
- Signals persist (SQLite, the new migration), retained by count per topic
  and by age, so the walkthrough doctrine holds: delivered whether or not
  anyone was waiting, and a daemon restart loses nothing *inside the
  retention window*. A caller whose cursor has fallen off the trimmed edge
  is told - `gap: true` plus the oldest surviving sequence - because a batch
  that silently skips what retention ate is how two agents both come to own
  a chunk. Silence must never read as "nothing happened" (FR61's rule, on
  the wire).

**Direct messages ride the same rails.** Every session's key names a private
topic, `to:<key>`; "message that agent" is `post_signal("to:" + peer, data)`
with the peer's key straight off the roster, and "listen" is
`await_signal(["to:@me"])`. A request/reply is two signals with the reply
topic named in the request. No mailbox subsystem, no new tools, and the
human sees agent-to-agent traffic in the same surface as everything else.
What travels is structured data between programs - a fact, a request, a
handoff - not a conversation; the chat non-goal stands.

The composition is the point. "Run the deploy when the tests are green" is
`await_signal(["tests:green"])` then `acquire_lock("deploy:agentbox")` - two
calls, no polling, no human in the middle, the whole chain visible in the
roster while it happens. Split a migration across three agents: the splitter
seeds per-chunk shared values, each worker claims by CAS and posts
`done:<chunk>`, the splitter parks on `done:*`. Maximal cooperation is these
four primitives composed, not a fifth primitive.

## Shared values

The blackboard: tiny named state with compare-and-swap, for coordination
that is neither a turn nor an event - claim tables for fanned-out work, a
"who is on which chunk" map, a progress counter.

- One tool, `shared`, with an honest object schema (get / set / delete as
  operations on key, value, `if_version`) - none of its forms block, so the
  blocking rule permits the fold, and it saves a tool against a budget that
  is already the design's biggest cost.
- `if_version` is CAS: the write lands only if nobody wrote in between, and
  the refusal returns the current value and version, so one retry loop
  replaces lock-read-write-unlock.
- The idiom for fan-out is **one key per item** (`claims/<chunk>`, CAS from
  empty), not one table under one hot key - per-item first-writer-wins
  instead of every loser paying a model turn to retry a global version.
- A value may carry an owner (the writer's session key, on request). The
  surface and `shared` reads report a value whose owner is no longer
  present, and that is what makes orphaned claims visible after a restart or
  a death instead of permanently un-drainable.
- Values are small by contract (`[sync] shared_max_bytes`, default 16 KB);
  a file path is the idiom for anything bigger.
- Every write posts a `shared:KEY` signal, so waiting on a value is
  `await_signal(["shared:claims/*"])` - one wake mechanism in the whole
  design.
- Persisted in the same migration, so coordination state survives a daemon
  restart even though presence does not.

## The wait contract, and what "maximal efficiency" means

The efficiency claim has to be honest about the runtime the agents actually
have. A Claude session is a turn loop: a parked tool call spends no tokens,
but it does occupy the turn - and MCP clients abort a call that stays silent
too long (Claude Code caps idle tool calls at around half an hour). The
design works with that, not against it:

- **The child keeps parked calls alive.** While any blocking sync call is
  parked, the mcp child emits MCP progress notifications on a ticker, which
  is what marks the call live rather than hung to a client that caps silent
  calls.
- **Every wait has a ceiling anyway.** `[sync] wait_max_s` (default 1500s,
  under the known client cap) bounds any park; hitting it returns an honest
  `timed_out` with the cursor, and re-arming is one call that misses
  nothing. A wait is not a promise to sleep forever; it is the cheapest
  possible unit of waiting, repeated as needed.
- **A lock wait stalling the agent is correct.** It cannot proceed anyway;
  what sync removes is the poll spin and the human-as-mutex, not the wait
  itself. What an agent must not do is park speculatively mid-mission on a
  maybe-someday topic - that is what the discovery rider and the hooks are
  for.

Against the alternatives, per wait: today's shape is sleep, call a status
tool, decide, repeat - a model turn per poll, reacting a full interval late.
Sync's shape is one parked call, woken in sub-millisecond time by a channel
send over a unix socket, resumable by cursor if it ever times out. Refusals
and grants carry the full picture so the follow-up question is never needed.
Activity updates coalesce last-write-wins; roster pushes to the UI are
throttled; `await_signal` batches. Chatty agents get cheap, not punished.
The one lock everything shares is the subsystem's own mutex, held for map
operations only, never across a wait - the control subsystem's rule, kept.

## Two doors

MCP tools, the `addSyncTools` family: `announce`, `acquire_lock`,
`try_lock`, `release_lock`, `post_signal`, `await_signal`, `shared`,
`list_agents` - eight new, plus `set_activity` generalized in place. The
child registers the family only when `[sync] enabled` is on, read at child
startup - a daemon-side flag alone would leave eight always-refusing schemas
in every session's context, a kill switch that kills the feature and keeps
the cost. (Config note, not a live-reload: children started before a flip
keep the tool list they were born with - the FR64-adjacent mechanic already
in "Mechanics discovered".)

CLI (`agentbox sync ...`): `attach`, `announce`, `activity`, `lock NAME
[--timeout N] [--ttl N] [-- CMD]`, `unlock NAME`, `post TOPIC [DATA]`,
`await TOPIC... [--after SEQ] [--timeout N]`, `get KEY`, `set KEY VALUE
[--if-version N]`, `peers [--area A]`, `agents`, `status`. Exit codes follow
the house grammar: 0 granted/delivered, 1 refused/timeout, 3 unanswered,
`--json` everywhere. The CLI is how hooks, Makefiles and non-Claude agents
join the same fabric - and the first consumer is this repo's own
`make deploy`, wrapped in `agentbox sync lock deploy:agentbox -- ...`, which
retires a CLAUDE.md trap by construction.

## Mechanics, grounded

The insertion points follow the control/walkthrough precedent:

- `internal/proto`: `agentbox.v1.sync.*` method constants and typed params;
  the session key joins `proto.Identity`.
- `internal/daemon/sync.go`: the subsystem - roster keyed by attach
  connection, lock table with waiter queues and orphan states, signal
  cursors over the store, its own mutex, never held across a wait. Waiter
  shape (matcher, buffered chan of 1, register-before-check, put-back on
  racing delivery) copied from the three existing hubs; the fan-out delivery
  is the one new behavior.
- The attach stream: one long-lived call whose context IS the presence -
  the FR45 insight promoted from per-call to per-session. Child side: lazy
  dial on first tool call, spawn disabled, redial with backoff, replay the
  last announce. Progress notifications on a ticker for any parked call.
- The discovery rider: the daemon piggybacks a `sync` member on the JSON-RPC
  response envelope when the caller's area roster changed since its last
  call; the child appends the line to the tool result it hands the model.
- `internal/store`: migration 0008 - `sync_signals` (seq, topic, identity,
  data, at; seq is the global cursor) and `sync_shared` (key, value,
  version, owner, updated). Roster and locks are memory only, on principle:
  a hold must not outlive the ability to observe its holder. After a daemon
  restart the locks are gone and the first touch says so honestly.
- `internal/webui`: an `Agents` surface registered like the others, fed over
  `agentbox:agents`, a narrow interface the daemon satisfies structurally.
  Ages from a monotonic base, rebased on wall-clock jumps.
- `internal/mcp`: `addSyncTools(srv, s)` beside `addAssignmentTools`, gated
  on the config read.
- Observability: `sync.announced`, `sync.attach/detach`,
  `sync.lock_acquired/released/orphaned/broken`, `sync.deadlock_refused`,
  `sync.stall_warned`, `sync.signal_posted` in the JSONL log, so
  `agentbox logs --follow` narrates a coordination the way it narrates the
  item lifecycle.

## Configuration

```toml
[sync]
enabled = true            # read by the mcp child at startup; off = the sync
                          # tools are not registered at all (zero context cost)
wait_warn_s = 600         # toast when a LOCK wait exceeds this; 0 disables.
                          # signal waits never warn: listening is success
wait_max_s = 1500         # ceiling on any parked call; hitting it returns
                          # timed_out plus the cursor, re-arming is one call
holder_gone_grace_s = 5   # attach drop -> roster row removed; held locks go
                          # orphaned (pid-checked), never silently released
signal_keep = 1000        # per topic, and:
signal_keep_days = 7      # whichever trims first; a trimmed cursor reports gap
shared_max_bytes = 16384
```

Seven knobs, defaults chosen so nobody opens the file (vision principle 8).
Coalescing rates and emit throttles are constants, not knobs.

## What this is not

- Not cross-machine. Local only, one daemon, one socket (vision principle
  5). The VM is coordinated from this machine's side (`vm:boris-vm` is a
  name, not a network).
- Not enforcement. Locks are advisory; an agent that never calls sync can
  still run `make deploy` bare. The global instructions, the manual, the hooks
  and the Makefile wrap are how convention becomes coverage - see "Making it
  the default for every agent".
- Not a data channel. Signals and shared values are capped small; files and
  the store carry payloads.
- Not a scheduler. Assignments decide *when work starts*; sync decides *how
  running work takes turns*.
- Not a chat. Direct messages carry structured data between programs; the
  human's conversations with agents stay in the agents' own UIs (vision
  non-goal 1), and nothing here renders a thread.

## Relation to what exists

- **FR74 control** keeps its own shape - the human veto, the strip, the
  no-queue rule are about the desktop, not about locks. The roster shows a
  control run as a state chip, the session key fixes its ownership check,
  and the attach gives it the holder-liveness it noted it lacked. Folding
  its storage into the lock table is a later refactor, not part of this.
- **FR19 subscribe API** (parked): adjacent, not subsumed - FR19 is server
  push for companion surfaces; signals are agent-to-agent.
- **FR21 progress / FR45 caller-alive**: both feed the roster's derived
  state untouched.
- **Assignments (M12)**: runs announce automatically and appear in the
  roster like any agent; a scheduled run takes `repo:X` before touching a
  tree a live session may be editing - the first real two-agent customer.
- **FR81**: the Agents surface joins the visual-pass queue like every other
  surface; nothing in it blocks on that pass.

## Slices, each with its acceptance check

0. **The spike.** CLI-only lock and signal against a scratch daemon
   (`AGENTBOX_INSTANCE=dev`), driven by two real Claude sessions doing real
   work in two projects. Not a mock - the mock rule gets its due on the
   surface - but the step that tells us what the results must carry before
   the tool schemas freeze, which is the part an armchair spec gets wrong
   (the FR58 lesson).
1. **Roster and discovery.** The session key, the attach, announce
   (returning same-area peers and `partial`), generalized `set_activity`,
   `list_agents`, the discovery rider, the Agents surface, hook recipes.
   Accept: three real sessions in three projects show three rows with
   purpose and live activity, grouped by area; a second session joining
   this repo gets the first session's row back from its own announce, and
   the first session's next tool result carries the rider; `kill -9` one
   child and its row is gone within the grace; a never-announcing session
   shows dim; a pre-sync session shows as "seen, not attached" and
   `partial` rides the reads; `make stop` with three live sessions leaves
   the daemon down; the strip and the roster agree while one agent drives.
2. **Locks.** Acquire/try/release, orphaning, break, deadlock refusal, the
   human-edge stall warning, the Makefile wrap. Accept: two live sessions
   race `make deploy` and serialize, the second's row showing `blocked:
   deploy:agentbox` with the holder named; kill the holder's child while its
   wrapped subprocess still runs - the waiter is NOT granted until the pid
   dies, and the roster shows the orphan; a constructed A-B/B-A cycle is
   refused with both names and toasts the human; a holder parked on
   `ask_user` while a waiter queues behind it warns with the chain named.
3. **Signals.** Post/await, the global cursor, gap reporting, migration,
   retention, the built-in `agents:<area>` and `to:<key>` topics. Accept: a
   tests-green handoff between two sessions with no polling in either
   transcript; a signal posted with no waiter is picked up after a daemon
   restart by cursor; a cursor older than retention returns `gap: true`;
   two waiters on one topic both wake; a direct request/reply round trip
   between two live sessions; a parked wait outlives the client's idle cap
   thanks to the child's progress ticks, and `wait_max_s` returns a
   resumable timeout.
4. **Shared values.** The `shared` tool, CAS, owners, change signals.
   Accept: three sessions drain a ten-chunk claim table (one key per chunk)
   with zero double-claims; restart the daemon mid-drain - claims survive,
   the dead session's claim reads as ownerless, and the table still drains.
5. **Teaching, which is what makes the mandate real.** See the section below;
   this slice is not documentation garnish, it is the difference between a
   feature that exists and a feature every agent uses. Accept: a Claude
   session in an unrelated project, given no instruction beyond its own
   configuration, announces itself and appears in the roster; and a second
   session in the same repo learns about the first without being told to look.

## Making it the default for every agent

Boris's mandate is that **every** agent using the platform declares itself and
coordinates, not that the option exists. An MCP tool nobody is told to call is
a tool nobody calls, so the teaching is part of the feature and it has four
doors, each reaching a different kind of agent:

1. **`~/.claude/CLAUDE.md`, the global instructions** - the only door that
   reaches every Claude session in every project on this machine, which is the
   scope Boris asked for. Its existing AgentBox section gets the standing
   contract: announce your purpose at session start, keep the activity line
   current as the work changes, check for peers in your area before editing a
   shared tree, and take the lock before a shared resource (the deploy, the
   repo, the VM, the desktop). Written as a habit with a reason, the way the
   interruption-cost paragraph already is, because a rule an agent understands
   survives paraphrase.
2. **The embedded manual, `internal/manual/agent.md`** (served by
   `agentbox docs agent`, and mirrored in the fuller
   [agent-manual.md](agent-manual.md)) - the reference an agent reads when it
   wants to know what the tools do. Sync gets its own section: the roster
   contract, the `kind:scope` naming idiom, the composed patterns (wait then
   lock, claim by CAS, direct request and reply), and the anti-patterns
   (polling instead of parking, holding a lock across a human question,
   announcing once and never updating).
3. **Hooks, in [recipes.md](recipes.md)** - the layer that keeps the roster
   honest when a model forgets: SessionStart announces, PostToolUse updates
   the activity line. Zero tokens, and it means even an agent that ignores
   every instruction still shows up truthfully.
4. **This machine's own workflow** - `make deploy` wrapped in
   `agentbox sync lock deploy:agentbox`, so the repo that invented the trap
   stops relying on agents remembering it.

The manual doors must land in the same slice as the tools, not after. An agent
whose MCP child is older than the deploy cannot see the new tools at all (the
handshake fixes the tool list), so the instructions and the tools have to
arrive together or the first sessions to read the new CLAUDE.md will be told to
call things they cannot reach.

## Mock it before building it

The working rule applies to the surface exactly as FR58 practiced it: a
throwaway `agentbox webui-demo agents` case is **written for this purpose**
(the harness has one case per surface today and none for Agents) and renders
the Agents surface over canned roster data - two working agents in one area,
one asking, one blocked on a lock held by another, one dim and unannounced, one
orphaned lock. Boris walks that before any daemon code exists. Note the one-UI
trap in CLAUDE.md while doing it: showing a working-copy surface means holding
the desktop's only daemon, so it needs a window when no other agent is live.

The primitives get the slice-0 spike instead: protocols are validated by being
driven, not by being looked at, and the spike is throwaway by construction (a
build tag, a dev instance, no migration). Two things the CLI spike cannot
answer, so they need their own small probes rather than being assumed:

- **The client's tool-call idle cap and the progress-notification fix.** No
  MCP client is involved in a CLI spike. Probe it by speaking stdio JSON-RPC
  to a fresh `agentbox mcp` (the recipe is in "Mechanics discovered" in
  07-field-requests.md) and parking a call past the cap, once without progress
  notifications and once with. Until that runs, every wait-ceiling number in
  this document is a review-derived guess.
- **The Bash-tool timeout on a CLI hold.** An agent that wraps a command in
  `agentbox sync lock NAME -- CMD` is subject to its own shell timeout, which
  is far shorter than `wait_max_s`. Measure it and let it set the default, or
  the flock wrapper will look broken the first time a real deploy runs long.

## Open questions (owner calls)

1. **How hard is the announce gate?** Design says: sync verbs refuse
   without an announce; presence and reads never do. The stricter option -
   every AgentBox tool nags - is a knob away if rude agents show up dim too
   often.
2. **The rail spot.** Design says: Agents is its own surface, the Home tile
   links to it. The alternative is a Home panel only, cheaper but capping
   the detail view.
3. **Area granularity.** Design says: derived from the git top-level and
   origin (one repo = one area, worktrees included), refined by declared
   tags. Declared-only is simpler and blind to the common case;
   derived-only cannot express "the webui half". Watch the first real
   overlaps and let them pick.

Two smaller calls made in the design and recorded here rather than left
open: breaking a lock is unilateral behind a confirm (a veto-style card to
the holder first would cost the human a wait while an agent decides), and a
card's footer does not carry its agent's purpose yet - it is cheap and it
widens every card; it can ride the FR list if triage by mission turns out
to be wanted.
