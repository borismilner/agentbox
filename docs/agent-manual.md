# AgentBox: the agent manual

This is the complete reference for an AI agent (Claude Code or any LLM with tool
use or shell access) driving **AgentBox**. Read it once and you know everything
needed to interrupt a human well: when to ask, which tool to use, what comes
back, and what never to do.

For a short in-binary version run `agentbox docs agent`. For the machine-readable
wire contract run `agentbox schema`. Config knobs (the human's side) are in
[06-configuration.md](06-configuration.md); integration snippets in
[recipes.md](recipes.md).

## What AgentBox is

AgentBox is a desktop interaction hub. An agent uses it to reach the human at the
keyboard: post a result, ask a blocking question, get a secret, show a diff for
approval, report progress. AgentBox renders these as calm desktop cards/toasts
(centered card for decisions, top toast for glances), plays a quiet earcon,
keeps an inbox and history, and returns the human's answer to the agent. A
background daemon owns the windows; your call auto-spawns it if it is not up.

**The one rule: interrupt sparingly.** A blocking question pulls a human out of
flow. Only ask for a decision you cannot make safely yourself. Use
`notify_user` for things to glance at, `report_progress` for long work, and a
blocking tool only when you genuinely need an answer to proceed.

## Two ways in (use MCP)

AgentBox has two equal-rank interfaces (ADR-0004):

1. **MCP** (`agentbox mcp`, a stdio server) - the primary path for an LLM agent.
   The model calls tools directly. Identity is filled in automatically.
2. **CLI** (`agentbox <command>`) - for shell scripts, hooks, cron, and manual use.
   Same daemon, same behavior; the answer comes back as exit code + stdout.

If you are an LLM with tool use, prefer MCP. If you are running shell commands,
use the CLI.

### Setup (one time, per project)

Register AgentBox's MCP server in the project's `.mcp.json`:

```json
{"mcpServers": {"agentbox": {"command": "agentbox", "args": ["mcp"]}}}
```

`agentbox docs setup` prints a ready-to-paste version (and a Claude Code hooks
alternative for Stop/Notification if you cannot use MCP). The first tool call
auto-spawns the daemon; nothing else to start.

## Decision guide: which tool, when

| You want to... | MCP tool | CLI | Blocks? |
| --- | --- | --- | --- |
| Say what this session is for, and see who else is here | `announce` | `agentbox sync announce` | no |
| Say what you are doing right now | `set_activity` | `agentbox sync activity` | no |
| Check for other agents before touching a shared tree | `list_agents` | `agentbox sync agents` | no |
| Take turns over a shared resource (deploy, repo, VM) | `acquire_lock` | `agentbox sync lock` | yes |
| Take it only if it is free right now | `try_lock` | - | no |
| Hand it to whoever is behind you | `release_lock` | `agentbox sync unlock` | no |
| Tell the other agents something happened | `post_signal` | `agentbox sync post` | no |
| Wait to be told, instead of polling for it | `await_signal` | `agentbox sync await` | yes |
| Claim one item of fanned-out work, so nobody doubles it | `shared` (`op="set"`, `if_version=0`) | `agentbox sync set --if-version 0` | no |
| Read shared state, or a whole claim table | `shared` (`op="get"`) | `agentbox sync get` | no |
| Tell the human something (result, FYI) | `notify_user` | `agentbox notify` | no |
| Take back something you posted | `retract` | `agentbox dismiss ID` | no |
| See what is still waiting for him | - | `agentbox pending` | no |
| Ask a single choice (2-9 options) | `ask_user` (with `options`) | `agentbox ask` | yes |
| Ask for free text | `ask_user` (no `options`) | `agentbox input` | yes |
| Get a yes/no | `confirm_action` | `agentbox confirm` | yes |
| Do something unless stopped (countdown) | `act_unless_stopped` | `agentbox veto` | yes |
| Collect several fields at once | `ask_user_form` | `agentbox form` | yes |
| Get a secret/credential | `request_secret` | `agentbox secret` | yes |
| Get a diff approved before applying | `request_review` | `agentbox review` | yes |
| Show rich markdown (report, plan, table) | `show_document` | `agentbox show` | no |
| Report progress on a long task | `report_progress` | `agentbox progress` | no |
| Give the human something to use, not read | `show_artifact` | `agentbox show --artifact` | no |
| Wait for them to use it | `await_artifact_event` | `agentbox artifact wait` | yes |
| See what they have already done in it | `read_artifact_events` | `agentbox artifact read` | no |
| Hand a change over for a step-by-step review | `create_walkthrough` | `agentbox walkthrough create` | no |
| Wait for that review to come back, whole | `await_walkthrough` | `agentbox walkthrough await` | yes |
| Pick up a review submitted after its agent left | `read_walkthrough` (`ack`) | `agentbox walkthrough read --ack` | no |
| See the work AgentBox runs on its own | `list_assignments` | - | no |
| Write or improve one of those | `create_assignment` / `update_assignment` | - | no |
| Try it now and read the result | `run_assignment`, `assignment_runs` | - | no |
| Take the desktop for a while (hands off) | `request_control` | `agentbox control request` | yes |
| Say what you are doing while you hold it | `set_activity` | `agentbox control activity` | no |
| Give the desktop back | `release_control` | `agentbox control release` | no |
| Take it back mid-run (the human only) | - | `agentbox control pause` / `resume` | no |

Picking between the close ones:

- **confirm_action vs act_unless_stopped**: if proceeding is the expected
  outcome and you just want a chance to be stopped, use `act_unless_stopped`
  (it proceeds on timeout). If you genuinely need a yes before acting, use
  `confirm_action`.
- **ask_user vs ask_user_form**: one decision -> `ask_user`; several related
  inputs in one go -> `ask_user_form`.
- **notify_user vs report_progress**: a single event -> `notify_user`;
  incremental progress on one task -> `report_progress` (do not spam notifies).
- **request_review vs create_walkthrough**: one diff, one approve/reject ->
  `request_review` (a card, gone when answered). A change worth walking -
  steps, citations, questions, a verdict per step, comments anchored to lines
  -> `create_walkthrough` (a durable board review the human hands back in one
  turn, and a library record afterwards).

## MCP tools (full reference)

Every blocking tool waits for the human and returns their answer. `title` is
always required; `body` is optional markdown shown under it.

**Every tool that puts something on screen also takes `speak`**: one short line
read out loud after the item's earcon, when the human has speech turned on. It is
your sentence, not the title - AgentBox never reads a screen aloud. Write what you
would say to somebody in the next room, and leave it out when there is nothing
worth saying: omitting it means the chime alone, which is the default.

    notify_user(level="error", title="migration failed on staging",
                speak="The staging migration failed and rolled back. Nothing was lost.")

Do not restate the title, do not read a diff or a table out, and do not narrate
every step - the same restraint that applies to interrupting at all applies here,
more so, because a voice cannot be glanced past.

### announce  (non-blocking)
Say what this session is FOR, and find out who else is already working where you
are. **This should be your first AgentBox call.**

The human watches a live Agents board of every session on the machine. Your
`purpose` is the headline of your row there, so write the one line they would
recognise - "porting the settings surface to the new theme" - not a summary of
your tools or a restatement of your prompt.

- Args: `purpose` (req), `activity` (what you are doing right now; `set_activity`
  carries it from then on), `area` (a `kind:scope` tag such as
  `subsystem:webui`), `tags`.
- Returns: `{ok, peers: [{key, agent, project, purpose, activity, state, cwd}],
  alone, partial, note}`.
- `peers` are the agents sharing your area. If any come back, **you are not
  working alone**: partition the work or wait, rather than editing the same tree
  at the same time.
- `alone: true` is returned only when it is certainly true. `partial: true` means
  the roster cannot see everybody (a session older than this feature has no row),
  and on `partial` you must never conclude you are alone.
- Call it again if the mission changes. You do not need to call it repeatedly
  otherwise; `set_activity` is the per-step verb.

You do not have to announce to be visible - the daemon registers what your child
knows for free, so a silent session still appears. What it cannot know is why you
exist, which is the whole reason to announce.

Where the hooks in [recipes.md](recipes.md) are installed, your row is already there
before your first tool call, wearing a placeholder purpose (`<dir> session (purpose
not yet stated)`) that is deliberately worded as unfinished - a hook cannot know
what you are for, and a confident guess would read as your own answer. Announce to
replace it. Seeing a row that looks like yours is not a reason to skip announcing:
after a `/clear` your own child keeps the previous conversation's purpose.

### list_agents  (non-blocking)
The live roster: every session the daemon can see, with identity, purpose,
current activity and derived state. The same roster the human's Agents surface
renders, so you and they cannot see different answers.

- Args: `area`, `project` (both optional filters; omit for everybody).
- Returns: `{ok, agents: [...], partial, note}`.
- Use it before editing a shared tree, or to find the peer you want to
  coordinate with. Do **not** poll it in a loop - ask when you are about to act.

### The line you did not ask for

When your area gains or loses an agent, the news is appended to the result of
whatever tool you call next, as a line beginning `sync:`. It names who arrived,
the purpose they announced and the state they are in:

```
sync: 1 agent joined repo:agentbox: codex "FR73: making a card body readable
after it closes" (working). Coordinate before you edit a shared file: split the
work or wait.
```

This is why polling is pointless. Each arrival and departure is reported once, on
a call you were making anyway - `set_activity`, a notification, anything - so an
agent that is deep in a file still finds out. `announce` and `list_agents` never
carry it, because their own result already shows you the roster.

A line like that is a request to stop and coordinate before continuing, not
decoration.

**Area is derived, not declared.** It comes from the repo you are working in, so
two sessions in one checkout (or in two worktrees of one repo) find each other
without either one spelling it out. A declared `area` tag narrows searches; it
never narrows what you are told about, so you cannot accidentally hide from a
peer by tagging yourself differently.

**State is the daemon's word, never yours.** You write `purpose` and `activity`;
everything else on your row - `asking`, `driving`, `blocked`, `working`, `quiet`,
`unannounced` - is derived from what the daemon observed. That is what makes the
board trustworthy: no agent can claim to be busy, and none can hide that it is
stopped behind a lock.

### acquire_lock  (blocking) / try_lock / release_lock  (non-blocking)

A lock is how two agents take turns over one resource instead of colliding over
it. Take one before anything shared: the deploy, a repo other sessions edit, the
VM, an external service, a scarce quota.

- Args (`acquire_lock`): `name`, `timeout_s` (0 waits as long as the daemon
  allows a parked call), `note`, `release_on_detach`.
- Returns: `{ok, name, granted, timed_out, refused, holder, holder_purpose,
  holder_activity, held_s, queue, orphaned, reason, deadlock, note}`.
- `try_lock` takes the same args minus `timeout_s` and never waits.
- `release_lock` takes `name`. Only the holder can release.

```
acquire_lock(name="deploy:agentbox", timeout_s=600, note="deploying the mirror fix")
  -> {granted: true}
  ... do the work ...
release_lock(name="deploy:agentbox")
```

**Names are a convention, not a registry.** `kind:scope`, the same idiom as
everything else: `deploy:agentbox`, `repo:agentbox`, `vm:boris-vm`. Two agents
that pick different names for one resource have protected nothing.

**A refusal or a timeout carries the whole picture** - who holds it, what they
announced, what they are doing right now, how long they have held it, how many
are queued - so deciding what to do next never needs a second call. A timeout is
a result and not an error: nothing changed because you asked, and re-arming is one
call.

**Your session dying does not free your lock.** It goes *orphaned* instead, with
the process id you were running under, and the next agent is granted it only when
that process is gone too - because a dead session does not prove the `make deploy`
it started has finished. `release_on_detach: true` opts out, for a critical
section that is exactly this session and nothing else.

**A deadlock is refused rather than suffered.** If waiting would close a cycle
("you asked for deploy:agentbox, held by codex; codex waits on repo:agentbox,
held by you"), the acquire fails at once and names both sides. Take locks in one
order across agents and it never comes up.

**You can be told you lost one.** The human can break a lock from his Agents
board; breaking reassigns it and does **not** stop you, so the notice rides your
next AgentBox call and you must stop touching what it protected.

**Never hold a lock across a question to the human.** Everything behind you then
waits on something only he can end - the daemon warns him when it sees it, but the
agent that arranged it is the one at fault.

### post_signal  (non-blocking) / await_signal  (blocking)

A lock is how two agents take turns. A signal is how one tells the other that it
is their turn - and it is what replaces the loop where you sleep, call a status
tool, decide, and look again, spending a model turn per look and reacting a whole
interval late.

- Args (`post_signal`): `topic` (exact, in the `kind:scope` idiom), `data`
  (optional small JSON, capped at 16 KB).
- Returns: `{ok, topic, seq, delivered, note}`. `seq` is this signal's place in
  the one global sequence, which is also the cursor to read it from.
- Args (`await_signal`): `topics` (exact names, or prefixes ending in `*`),
  `after_seq` (your cursor; omit for "from now on"), `timeout_s`.
- Returns: `{ok, signals:[{seq, topic, agent, project, key, data, at_ms}],
  cursor, received, timed_out, gap, oldest_seq, more, note}`.

```
post_signal(topic="tests:green", data={"suite": "race"})
  -> {seq: 41, delivered: 1}

await_signal(topics=["tests:green"], timeout_s=600)
  -> {signals: [{seq: 41, topic: "tests:green", agent: "claude", data: {...}}],
      cursor: 41}
```

**A signal is stored, so it is delivered whether or not anybody was listening.**
`delivered: 0` says nobody was parked at that instant, not that the signal was
lost: a peer that was busy picks it up later by cursor, and a daemon restart loses
nothing inside the retention window (`signal_keep` 1000 per topic,
`signal_keep_days` 7).

**The cursor is how you never miss one.** Pass the `cursor` you were last given
back as `after_seq`, and you get everything matching since then **in one batch** -
three events that fired while you were editing a file arrive together on your next
call, rather than one wake per event. Omit `after_seq` and you wait for what
happens from now on, which is the right reading when you have nothing to resume
from. `more: true` means the batch was capped and one more call takes the rest
without parking.

**Topics are names, or families.** An exact name waits for one event; a name
ending in `*` is a plain string prefix, so `done:*` catches `done:migration-3`.
Not globbing, not regex - a topic is a name.

**To message one agent, post to its private topic.** Every session's key names
`to:<key>`, and the key is on its roster row from `announce` or `list_agents`. So
messaging a peer is `post_signal(topic="to:"+peer_key, data={...})` and listening
is `await_signal(topics=["to:@me"])`, where `@me` expands to your own key. A
request and a reply are two signals with the reply topic named in the request.
This carries structured data between programs; it is not a chat.

**Three topics the daemon posts for you:**

| Topic | Posted when |
|---|---|
| `agents:<area>` | an agent joins, announces or leaves your area |
| `lock:<name>` | that lock changes hands, with the reason and the new holder |
| `to:<your key>` | anything a peer addressed to you |

`lock:<name>` is how you learn a resource freed *without* queueing for it, and
`agents:<area>` is for an agent that is genuinely idle - one mid-task gets the
`sync:` rider on whatever it calls next, with no call of its own.

**A timeout is a result, not an error**, exactly as it is for a lock: the cursor
comes back unchanged, so waiting again misses nothing that happened in between.
Every park is bounded by `[sync] wait_max_s` (1500s), because the MCP client
abandons a tool call it has heard nothing about for 1800s and never tells the
daemon it did - so re-arming is the normal shape of a long wait, not a failure.

**`gap: true` is the answer you must not skim.** Your cursor is older than what
retention still holds, so signals between the two are gone and the batch cannot be
complete. Treat whatever you were tracking through them as *unknown*, not as *not
having happened*: a batch that silently skipped what retention ate is how two
agents come to believe they each own one chunk of work.

**Nothing ever warns the human about a wait here.** Listening is the intended
steady state, which is precisely why a lock wait warns and a signal wait does not.

The composition is the point:

```
await_signal(topics=["tests:green"])      # park, no polling
acquire_lock(name="deploy:agentbox")      # then take the resource
```

and fanning work out is one `post_signal("done:<chunk>")` per finished piece
against one `await_signal(["done:*"])`.

Anti-patterns: polling a status tool in a loop (this is what replaces it); parking
speculatively on a maybe-someday topic when `await_signal` occupies your turn;
posting a `*` (a wildcard belongs in `topics`, never in a topic you post to);
dropping the cursor after a timeout, which can miss whatever arrived between two
calls; and sending a payload where a file path would do.

### shared  (non-blocking, all three operations)

The blackboard. A lock says whose turn it is and a signal says something happened;
only this can say **chunk 7 is mine**. One tool with `op: get | set | delete`,
folded into one door because none of the three blocks - the rule that splits
`acquire_lock` from `try_lock` exists so you can tell from the door whether calling
parks your turn, and here the answer is always no.

- Args: `op`, `key`, `value` (for `set`, small JSON), `if_version`, `own`.
- Returns: `{ok, op, found, value, values, more, applied, stale, note}`, where a
  value is `{key, value, version, owner, owner_agent, owner_gone, updated_ms}`.

**`if_version` is the whole feature.** Versions start at 1, so 0 is free to mean
something, and what it means is *claim*:

| `if_version` | Meaning | Use |
|---|---|---|
| omitted | write no matter what | seeding a table, a value nobody contends |
| `0` | **only if the key does not exist** | claiming an item, first writer wins |
| `N` | only if it is still at `N` | updating state you have read |

```
shared(op="set", key="claims/chunk-3", value={"worker": "me"}, if_version=0, own=True)
  -> {applied: true, value: {key: "claims/chunk-3", version: 1, owner: "8b94…"}}

shared(op="set", key="claims/chunk-3", value={"worker": "me"}, if_version=0)
  -> {applied: false, stale: true,
      value: {version: 1, owner_agent: "codex"},
      note: "claims/chunk-3 is already claimed by codex, which is still running…"}
```

**A refusal is a result, not an error.** `stale: true` comes back with the current
value and version, so a losing claimer moves to the next key and an updater retries
immediately - no second call to find out what it lost to, and no lock-read-write-
unlock anywhere.

**One key per item, not one table under one hot key.** Ten workers claiming
`claims/<chunk>` is first-writer-wins per item, and every worker that loses has
already lost cheaply. Ten workers CAS-ing one `claims` object means nine retries
per round, each of them a model turn.

**`own=True` is what makes abandoned work visible.** The owner's session key and
agent name are recorded in the row at write time, and every read checks the key
against the live roster. A session that dies holding a claim leaves
`owner_gone: true` on it - so the table is drainable after a death or a daemon
restart instead of stuck forever on a chunk nobody is working on. Take it over by
writing the key with its current `version` as `if_version`. Leave `own` false for
state that belongs to nobody, like a counter.

**A read may take a family.** A `key` ending in `*` returns every key under that
prefix (same prefix rule as topics), so a ten-chunk claim table is one call.

**Every write posts `shared:<key>`.** Waiting for somebody to claim or finish
something is `await_signal(["shared:claims/*"])` - there is one wake mechanism in
this design and this is not a second one. The signal carries the key, the new
version and the owner, never the value: it is a doorbell, so read the value with a
`get` when it rings.

**Values survive a daemon restart and are never trimmed**, which is the deliberate
difference from signals. Retention on a claim table would hand one chunk to two
agents. They leave when you delete them - so delete a key when its work is done -
and the cap on how many may exist (1000) *refuses a new key* rather than evicting
somebody's claim.

Anti-patterns: a shared value as a message (post a signal); one hot key for a
fan-out (one key per item); a payload instead of a pointer (`shared_max_bytes` is
16 KB, and a file path is the idiom); polling a key in a loop (park on
`shared:<key>`); claiming without `own`, which leaves nobody able to tell your live
work from a corpse; and leaving finished claims behind, which is what fills the
table.

### notify_user  (non-blocking)
Post a desktop notification. Returns immediately.
- Args: `title` (req), `body`, `level` (`info`|`success`|`warning`|`error`|`urgent`, default `info`), `actions` (array of `{label, exec}`, max 3 - buttons that run a shell command in your cwd on click; the verbatim command shows on hover).
- Returns: `{id}`.
- Use for progress and results, never for questions.

### retract  (non-blocking)

The other half of posting: taking something back. agentbox had four ways to create an
item and none to retire one, so a toast that stopped being true waited on the
human's screen until he clicked it - and a warning survives a daemon restart by
design, so it came back afterwards (FR89).

- Args: `id` (the one `notify_user` returned), or none to withdraw everything this
  session still has pending.
- Returns: `{ok, retracted, ids, note}`.

```
id = notify_user(title="Build failed", level="error")   -> {id: "k77..."}
... you fix the build ...
retract(id=id)                                          -> {retracted: 1}
```

**It can only ever touch what YOU posted.** Retiring another agent's item would be
answering its question for it, and clearing the human's whole queue is his own call
(`agentbox dismiss --all` from his terminal, or `agentbox pending` to see what is
there). An agent that could empty his queue could hide a question it did not want
answered.

**Use it the moment an announcement stops being true.** A stale warning is worse
than no warning: it costs him a click and teaches him that your notifications do not
mean anything.

### ask_user  (blocking)
Ask a question. Give 2-9 `options` for a single choice, or omit them for free
text.
- Args: `title` (req), `body`, `options` (2-9 strings), `timeout_s` (int; 0 = wait forever), `default` (applied on timeout).
- Returns: `{answered, answer, reply, default_applied}`. `answer` is the chosen option; `reply` is set instead when the human typed free text (the "/" reply hatch); `default_applied=true` if the timeout default was used.

### confirm_action  (blocking)
Yes/no.
- Args: `title` (req), `body`.
- Returns: `{answered, confirmed, reply}`. `confirmed` is the boolean.

### act_unless_stopped  (blocking)
Announce an action with a live countdown; it proceeds when the window elapses
unless the human stops it.
- Args: `title` (req), `body`, `window_s` (countdown seconds, default 15).
- Returns: `{vetoed}`. `vetoed=false` means proceed; `true` means the human stopped you.

### ask_user_form  (blocking)
A small form, up to 6 typed fields, answered in one card.
- Args: `title` (req), `body`, `fields` (req array), `timeout_s`.
- Each field: `{key (req), label, type ("choice"|"text"|"bool") (req), options (2-9, for choice), default}`.
- Returns: `{answered, values}` where `values` is a `{key: value}` map.

### request_secret  (blocking)
Masked credential entry. The value is written to a 0600 file and is NEVER
returned over the wire.
- Args: `title` (req), `body`, `timeout_s`.
- Returns: `{provided, path}`. Read the file at `path` when you need the value; never print or echo it; delete it when done.

### show_document  (non-blocking)
Open markdown in AgentBox's reading window (rich headings, code, tables, alerts,
charts, math, images) - far better than dumping into a terminal.
- Args: `path` (a file) or `content` (inline markdown), `title`.
- Returns: `{shown}`.

#### Math

Write TeX however you normally would: `$E = mc^2$` or `\(E = mc^2\)` inline,
`$$...$$` or `\[...\]` for display, or a ```math fence if you would rather be
explicit. Prices are safe - `$5 and $10` renders as money, not as a formula.
A formula AgentBox cannot typeset shows your TeX instead of disappearing, so a typo
is visible rather than silent.

#### Images

`![alt](/absolute/path/to/plot.png)` always works. AgentBox reads the file itself
and hands the window the bytes, so:

- **Use an absolute path** in anything you send over the socket - a card body, a
  notification, prose in a session turn. A relative one is refused there:
  AgentBox's working directory is not yours, and guessing would be wrong more often
  than right.
- **In a file you ask AgentBox to show**, a relative path works and is the better
  spelling. `agentbox show report.md` reads `![](out/chart.png)` against the
  document's own directory, so the file stays portable and reads correctly in an
  editor too.
- **PNG, JPEG, GIF or WebP**, checked by content and not by extension. For a
  vector picture use a ```chart or ```mermaid fence, which AgentBox draws itself.
- **2 MB per image.**
- **A URL is never fetched.** `![x](https://...)` renders as a placeholder saying
  so. Rendering your prose must not make a request to a host the human never saw,
  and it will not. If you need a remote image, download it first and reference
  the file.

Anything refused still shows its alt text and the reason, so write alt text worth
reading.

### speak  (non-blocking, or blocking with `wait`)
Say one line out loud, if the human turned speech on. No card, no notification, no
inbox entry - just a voice.
- Args: `text`, `wait`.
- Returns: `{spoken, waited}` - it reached the daemon; it is still silent if speech
  is off.
- Use it when something is worth hearing but not worth interrupting for, or to talk
  somebody through a long job they are only half watching. For anything that also
  needs a card, put the sentence in that tool's `speak` field instead, so the chime
  and the voice arrive together rather than as two separate events.
- **`wait` is for narration.** It returns when the line has finished playing rather
  than when it was queued, so the next thing you do lands on its last word: a
  sequence of lines that reads as speech, or a sentence that has to be heard before
  a card appears. AgentBox measures the audio to answer that (it counts the PCM), so
  it is the real end of the sound, not an estimate from the word count. Leave it off
  for an ordinary aside - an aside should not cost you the seconds it takes to read.
- A timeout bounds the **wait**, not the line: `agentbox say --wait --timeout 1` on a
  four-second sentence returns exit 4 after a second and the sentence still finishes
  out loud. AgentBox does not truncate its own speech.

### drive_desktop  (blocks while the script runs)
Move the pointer, click, drag, scroll and type on the human's desktop - real
synthetic input, so any application accepts it. Use it to do the thing rather
than describe it: open a menu, fill a field, click the button in a window you
just showed, demonstrate a workflow while they watch.
- Args: `script` (req, one step per line - `window TITLE` / `screen`,
  `move X Y`, `click [button|X Y [button]]`, `double`, `drag X1 Y1 X2 Y2`,
  `scroll N`, `type TEXT`, `key ctrl+alt+t`, `wait MS`, `speed N`, `wpm N`),
  `speed` (1 = a hand's pace), `wpm` (typing speed, default 300).
- Returns: `{steps}` - how many ran.
- Coordinates: `400` from the near edge, `-46` from the far edge, `60%`
  across, `center`, `~` the pointer, `~+30` relative to it. The whole script
  is validated before the first event fires; movements follow a human curve.
- **Name the window you mean.** `window TITLE` is a target lock, not just a
  coordinate frame: it raises that window, follows it if it moves, and then
  checks, before every single click, that the pointer really is over it, and
  before every `type` and `key` that the keyboard really is in it. A mismatch
  raises the window and looks once more; if that does not fix it the step fails
  and says what was there instead:

      line 4 (type deploy to prod): the keyboard is in "notes.txt" (gedit.Gedit),
      not "Terminal" - refusing to type into it

  A window that closed, moved or ended up behind something else now stops the
  script, where it used to send the rest of your keystrokes into whatever had
  taken its place. `screen` gives the lock up, and enforces nothing - there is
  nothing to compare against - so reach for it only when you really do mean the
  desktop itself. A menu, a tooltip or the target's own dialog counts as the
  target, so driving a menu works as it always did.
- It is the human's own desktop - drive it as sparingly as you would interrupt
  them, and announce it (a `speak` line) before taking the wheel.
- For anything longer than a step or two, ask for the desktop properly first:
  `request_control`, below. A spoken line is heard once and then it is over; the
  strip stays on screen for as long as you hold the wheel.

### request_control  (blocking) / set_activity / release_control  (non-blocking)
The desktop handover (FR74). `request_control` asks for the human's desktop and
blocks until they allow it; `set_activity` says what you are doing now;
`release_control` gives it back. Full rules in **Taking the desktop** below -
read them before your first handover, because the whole feature rests on the
strip being on screen for exactly as long as you are driving.
- Args: `reason` (req, what you are about to do in their terms), `window_s`
  (seconds of silence that count as consent, default 20) on request;
  `activity` (req) on set_activity; release takes none.
- Returns: `{granted, denied, live, state, activity, held_by, reason}`. Read
  `granted` / `denied` / `held_by` apart before touching anything.

### show_artifact  (non-blocking)
Run interactive HTML in a window: a chart to hover, a slider to drag, a control
panel to click. Write a self-contained document, or a React component module with
an `export default` - React 19 and Tailwind v4 are already in the page, so
`import React from "react"`, JSX and utility classes work as written.
- Args: `html` (inline) or `path` (a file, with `watch` to re-run it on every
  save), `title`.
- Returns: `{shown, artifact_id}`. Keep the id: it is what you wait on.
- Two absolute rules. **No CDN and no other package**: the artifact runs with no
  network at all, so a script tag or an import of anything but react/react-dom is
  a missing library, and AgentBox will tell you which in the artifact's own bar.
  **`window.agentbox.emit(name, data)` is the only way out**: it cannot fetch, reach
  AgentBox, or store anything. Everything else about the sandbox is in
  [decisions/ADR-0010-artifact-sandbox.md](decisions/ADR-0010-artifact-sandbox.md).

### await_artifact_event  (blocking)
Wait for the human to do something in an artifact and return it.
- Args: `artifact_id` (omit for any artifact), `names` (omit for any event),
  `timeout_s` (0 waits as long as you do).
- Returns: `{received, event: {artifact_id, name, data, at_ms}, timed_out}`.
- This is the loop worth building: show a panel, wait on it, do the work they
  asked for by clicking, then show the result. Prefer it over a card when the
  answer is a number, a shape or a selection rather than one of nine options.

### read_artifact_events  (non-blocking)
Take everything they have done since you last looked.
- Args: `artifact_id`, `names`.
- Returns: `{events: [...]}`, newest value per event name - a slider dragged for a
  while is one number, not forty. Use it while you are working; use
  `await_artifact_event` when you have nothing to do but wait.

### request_review  (blocking)
Show a unified diff to approve or request changes. Use before applying a patch
the human should see.
- Args: `title` (req), `diff` (unified diff text) or `path` (a diff file), `body`.
- Returns: `{answered, approved, comment}`.

### report_progress  (non-blocking)
A live progress bar that never steals focus.
- Args: `id` (omit on the first call, pass the returned id on every later call), `title` (set on the first call), `status`, `percent` (0-100), `indeterminate` (bool, spinner when there is no known fraction), `done` (bool), `error` (string, to report failure).
- Returns: `{id}` - reuse it.
- Pattern: start without `id` -> get `id` -> update with `id` + `percent` -> finish with `id` + `done=true` (or `error`). Prefer this over repeated `notify_user`.

### create_walkthrough  (non-blocking)
Create a durable, step-by-step code review and open it on the review board.
The human walks it whole - verdicts, notes, line-anchored comments - and
hands everything back in one turn; the review persists in AgentBox's store past
both your sessions.
- Args: `spec` (req, the version-1 spec object below), `no_show` (store
  without opening the board).
- Returns: `{walkthrough_id, rev, warnings}`. Warnings are non-blocking
  teaching notes; hard failures come back as tool errors with directions.
- **Creating one captures the source it cites**, from the working tree you were
  reading, so the review survives the next checkout. Without that a walkthrough
  keeps only line numbers, and the board reads whatever the file says later - a
  deleted file breaks the step, and an edited one silently shows different code
  under your prose and your margin notes. A citation AgentBox could not read comes
  back as a warning naming it; the review is still created, and that block falls
  back to reading the file. Reviews stored before this existed are repaired from
  the pinned commit with `agentbox walkthrough repair` (one id, or all of them).

The spec, in short:

```json
{"version": 1, "title": "...", "repo_root": "/abs/path", "pinned": "commit sha",
 "base": "sha?", "diff": "the change's unified diff",
 "domains": [{"id": "kebab-id", "title": "The migration",
              "blurb": "one line, shown when this group opens"}],
 "out_of_scope": [{"paths": "glob", "reason": "..."}],
 "steps": [{"id": "kebab-id", "kind": "ground|code|none|check", "domain": "kebab-id",
   "title": "...", "purpose": "Serves: which requirement. Decided by: what.",
   "tldr": {"bottom": "the one sentence that has to survive",
            "points": ["a load-bearing fact that stands on its own", "..."]},
   "prose": [{"t": "text"}, {"t": "a bound phrase", "bind": "name"}, {"code": "chip"}],
   "code": [{"path": "repo/relative.go", "lines": [10, 40],
             "notes": [{"at": [12, 14], "text": "why this matters"}]}],
   "binds": {"name": {"block": 0, "lines": [12, 18]}},
   "checks": [{"q": "...", "a": "hidden until revealed"}],
   "cmds": [{"cmd": "make check", "expect": "...", "recorded": "YYYY-MM-DD"}]}]}
```

`domains` group the steps into the two to four subjects a change actually has,
and the board then shows one group at a time: the current domain's stations
listed in the rail, the others collapsed to a line with their own progress, and
`[` / `]` moving between them. Optional and worth skipping under about eight
steps. Once any domain is declared every step needs one, and a domain's steps
must be consecutive - the board walks one domain at a time, so a domain the
order leaves and returns to would open twice.

`tldr` is required on `code` and `check` steps, and **the board opens in it**.
It is not the shortened version of the step: the reader it serves has a very
short attention span and must still come away with mastery of what matters most
here, so nothing important is cut and what changes is the structure. `bottom` is
one sentence (up to 220 characters), `points` are up to six facts that each
stand alone and in any order (280 each). `t` on the board switches between the
TL;DR and the full text. Write it last, from the finished step - a TL;DR written
first summarises what you meant to say.

A prose segment carries `"p": true` to start a new paragraph at it. Segments
are inline by necessity - a bound phrase sits mid-sentence - so without it a
step renders as one wall of text with sentences fused at the seams.

`notes` on a block are the annotation channel: each renders as a numbered
badge on its first line and its text in the margin beside the code, outside
the block. Put the "why" there rather than in the prose above the block.

`binds` are how prose points at code: a segment `{"t": "...", "bind": "name"}`
lights that region while the reader is on the phrase.

Three rules, all validated with directions: a file-backed block never states
added/removed (the diff manifest is the only carrier - AgentBox derives every
marking); prose never contains literal line numbers (bind a phrase to a code
region instead - numbers in sentences go stale silently); a diff-carrying
review should end with a `check` step (finishing is an observation, not a
feeling).

**The authoring standard** - structure, annotation, coverage, the gate - is
served as the MCP resource `agentbox://standards/walkthrough` and printed by
`agentbox docs walkthrough`. Read it before authoring a walkthrough; it is the part
that decides whether the review is worth the human's time. Blocks cite at most 400 lines each; a snippet block
(`{"snippet": {"lang", "text", "added", "del"}}`) carries content that lives
in no file.

### await_walkthrough  (blocking)
Wait until the human submits their review, and receive the whole handback.
- Args: `walkthrough_id` (omit to take the next submission from any
  walkthrough), `timeout_s` (0 waits as long as you do).
- Returns: `{submitted, review, timed_out, gone}`. `review` leads with the
  unclear steps (each with the note saying what is unclear - answer these
  first), then every step's verdict/note/comments/checks, `not_reviewed`
  (always present), and the tally. `gone=true` means the walkthrough was
  deleted while you waited.
- A submission made while nobody waited is claimed here too, exactly once -
  await after the fact is the same as await before it.

### read_walkthrough  (non-blocking)
Fetch a walkthrough's full stored state: spec, the human's marks and
comments so far, and the last submission payload if any.
- Args: `walkthrough_id` (req), `ack` (take a waiting submission, exactly
  once).
- Returns: `{walkthrough}` - the stored review.
- `ack:true` is the fresh-session pattern: a review submitted after its
  agent was gone moves to `delivered` for exactly one caller.

### list_walkthroughs  (non-blocking)
The review library, most recently touched first.
- Args: `find` (matches title, step content, cited paths), `state`
  (`open`|`submitted`|`delivered`), `limit`.
- Returns: `{walkthroughs: [{id, title, state, counted_steps, understood, unclear, comments, ...}]}`.

### amend_walkthrough  (refuses in this build)
Revising a stored walkthrough by step id is designed but not built yet; this
tool refuses with directions. Create a fresh walkthrough for revised
content. A submitted, unread review is never overwritten either way - take
the handback first.

### delete_walkthrough  (non-blocking)
Remove a walkthrough permanently, marks and comments included.
- Args: `walkthrough_id` (req).
- Returns: `{deleted}`. An agent awaiting it is released with `gone=true`.
- Prefer leaving finished reviews in the library - they are the human's
  record too.

### list_assignments  (non-blocking)
The human's assignments: work AgentBox carries out by launching a Claude agent, on
a schedule or on demand.
- Args: none.
- Returns: `{assignments: [{id, name, description, kind, schedule, enabled,
  model, dir, running, last_run_ms, next_run_ms, last_state, last_summary}]}`.
- `kind` is `ad-hoc` | `periodic` | `scheduled`. Open with this before
  creating anything - an assignment that already exists usually wants
  editing, not a twin.

### read_assignment  (non-blocking)
One assignment whole, plus AgentBox's own diagnosis of it.
- Args: `assignment_id` (req), `runs` (recent runs to include, default 5).
- Returns: `{assignment: {assignment, kind, running, placeholders, unfilled,
  unused, problems, runs}}`.
- `unfilled` are `{{placeholders}}` no parameter fills; `unused` are knobs the
  prompt never reads; `problems` are faults in the stored spec. Read before
  you improve one, so you start from the same picture the human sees.

### create_assignment  (non-blocking)
Create an assignment. It starts enabled.
- Args: `name` (req), `prompt` (req), `description`, `spec`, `params`,
  `panel_html`, `model`, `mode` (`plan`|`full`), `dir`, `schedule`,
  `enabled`.
- Returns: `{assignment_id, created, kind, next_run_ms, warnings}`.
- Refuses a spec that would render wrong (every fault at once) and a
  schedule it cannot parse. Warns about the merely unfinished.

### update_assignment  (non-blocking)
Change one. **Only the fields you send change** - send a prompt and the
schedule, knobs and values stay exactly as the human left them.
- Args: `assignment_id` (req), then any of `create_assignment`'s fields.
- Returns: the same shape as create.
- `params` merges over the stored values, so setting one knob does not clear
  the others. A `spec` replaces the knobs entirely: values whose knob
  survives are kept, values whose knob is gone are dropped and named in
  `warnings`.
- `enabled:false` is the pause switch. Prefer it over deleting.

### delete_assignment  (non-blocking)
Remove an assignment and its whole run history.
- Args: `assignment_id` (req). Returns `{deleted}`.

### run_assignment  (non-blocking)
Run one now, outside its schedule.
- Args: `assignment_id` (req), `overrides` (parameter values for this run
  only; the stored values are untouched).
- Returns: `{run_id}` at once - a run is a whole conversation, so poll
  `assignment_runs` for the outcome.
- Refuses if a run of the same assignment is already in flight.

### assignment_runs  (non-blocking)
The run history, newest first.
- Args: `assignment_id` (req), `limit` (default 50).
- Returns: `{runs: [{id, started_ms, ended_ms, state, trigger, params,
  summary, error, session_id, data}]}`.
- `state` is `running` | `ok` | `failed` | `skipped`. `params` is what the run
  actually used, not what the definition says now. `data` is whatever the run
  recorded for later analysis.

## Assignments (work AgentBox gives you)

An assignment inverts the usual direction: instead of you summoning AgentBox, AgentBox
summons an agent - on a schedule, or when somebody asks - with the whole
toolbox available while it runs. A run is an ordinary session, so the human
can open it, read it and take it over.

You are the one who writes them. The seven tools above are the authoring
surface: propose a prompt and its knobs, `run_assignment` once so the human
sees it work, read the run back, and keep updating until they are satisfied.

Three triggers, one field:

| `schedule` | Runs |
| --- | --- |
| empty | only when a human or an agent asks (ad-hoc) |
| `every 30m`, `every 4h`, `every 1d` | on an interval from the last run |
| `daily 09:00`, `weekly mon 09:00` | at a wall-clock time |

A missed slot is skipped and recorded, never caught up - a laptop shut for the
weekend must not wake and fire three checks at once. One minute is the floor
on an interval.

The prompt is a template. `{{key}}` is substituted from the parameters at run
time, literally and non-recursively, and a placeholder nothing fills is left
verbatim in the prompt rather than silently dropped. Declare a knob for each
one in `spec`:

```json
[{"key": "window", "type": "enum", "values": ["24h", "7d"], "default": "7d"},
 {"key": "threshold", "type": "slider", "min": 0, "max": 100, "unit": "%", "default": 80},
 {"type": "markdown", "body": "Above the threshold this warns; above 95 it goes urgent."}]
```

`type` is `text` | `number` | `slider` | `toggle` | `enum` | `path` |
`markdown`. AgentBox renders them as a form. A `markdown` block carries `body`
instead of a key and sits between the controls, which is what makes a
generated panel read as something somebody designed. `panel_html` is the
escape hatch - a React/Tailwind panel run in the artifact sandbox (no
network) - and it does not replace the spec: the values live in the database
either way, so a panel that fails to load can never make an assignment
uneditable, and only a `{{key}}` with a knob behind it substitutes into the
prompt.

The panel's channel is two-way:

- **Values out:** `window.agentbox.emit("params", {key: value, ...})`. The keys
  you send merge over the stored values - one key changes one key, the same
  rule `update_assignment` follows. `"params"` is the only event a panel can
  send; any other name is shown in the panel's bar as undelivered.
- **Values in:** `window.agentbox.params` holds the current values (`{}` until
  the surface's first push, moments after load), and every change - a typed
  knob turned, an agent's `update_assignment` - fires an `agentbox:params`
  CustomEvent on `window` whose `detail` is the fresh map. A React panel
  typically does `useState(window.agentbox.params)` plus a listener that calls
  the setter with `e.detail`.

What the human sets in the panel is what `read_assignment` returns and what
the next run's `{{key}}` substitution uses, so a panel is also how a run gets
input from the human without asking: leave a control, read the value.

**Declare a knob for every key your panel writes.** A panel may write a key no
knob describes and it is stored - but the save path drops undeclared keys (with
a warning), so the next `update_assignment` that sends any `params` erases it.
A `text` knob you never look at is enough to make the key permanent.

Write the prompt for an agent nobody is watching. Say what to do, what counts
as worth interrupting for, and how to report - `notify_user` for something
worth knowing, `level="urgent"` when the number is bad, `show_document` for a
summary worth reading.

## Taking the desktop (the hands-off strip)

When you are about to drive the desktop for more than a step or two - a demo, a
screenshot run, a browser you are clicking through - ask for it with this, not
with a card of your own: `request_control` (blocks), then `set_activity` as you
go, then `release_control`. From a shell, the same three verbs:

```bash
agentbox control request "photographing the review board" --window 20   # blocks
agentbox control activity "scrolling to the second block"               # while driving
agentbox control release                                                # when done
agentbox control state                                                  # who holds it
```

And two the human has that you do not (FR94):

```bash
agentbox control pause     # take the keyboard and mouse back, mid-run
agentbox control resume    # hand them on again
```

One always-on-top strip appears at the top centre and stays there for the whole
run: first asking (your reason, a countdown, Deny and Now), then driving (the
activity line and how long it has been that way). **Presence is the entire
signal** - on screen means the desktop is yours, gone means it is the human's
again. So the strip must live exactly as long as your run does: keep it up until
you are finished, move `activity` along as you go, and release it the moment you
stop. Never let it close while you are still driving, and never leave it up after
you are.

Requesting blocks. Silence for `window_s` seconds counts as consent (default 20;
`Now` grants it early, `Deny` refuses immediately). One desktop cannot be shared,
so a second requester is refused with the holder's name rather than queued -
`held_by` (CLI exit 3) means wait and ask again, not drive anyway. A queue would
be worse: it would hand you the wheel at a moment you had stopped watching for it.

### He can pause you, and you wait

Ctrl+Alt+Escape, or the Pause button on the strip, and the desktop is his again
without your run ending. This exists because the alternative was him reaching for
the mouse anyway, which is the collision the strip was built to prevent.

What it looks like from your side:

- **The strip inverts rather than vanishing.** Same window, green instead of
  amber, `PAUSED - YOURS`, your activity line still there in italic so he can see
  what he is resuming into. Past two minutes it turns amber and reads
  `AGENT WAITING`, because by then the message is about you, not about him.
- **`drive_desktop` parks.** It stops at the end of the step it is on - inside a
  `type`, between characters, so a whole word never lands in whatever he just
  switched to - and then blocks. It does not fail, and it does not queue: a queue
  that drained on resume would be a burst of clicks into his work.
- **So does `request_control`.** A paused desktop is not handed to the next agent
  when the parked run releases. The latch is on the desktop, not on the run.
- **Waiting is correct.** After ten minutes you are told the desktop is still his
  and your run is still yours - not that you lost it. Wait again, or go and do
  something that does not need a screen.
- **Nothing you can call resumes it.** There is deliberately no MCP verb, because
  a pause an agent can undo is a suggestion.

Your `set_activity` still writes the strip while parked, and it is worth using:
"waiting to finish the settings walkthrough" is what tells him what resuming
costs him.

Why not `confirm_action` for this: a card is answered and gone, which leaves
nothing on screen for the minutes you then spend driving. That gap is what makes
a human reach for the mouse mid-run, and it killed three drive sequences in ten
minutes before this existed.

### He can be recording, and your card waits

Ctrl+Alt+Q, or `agentbox control quiet`, and the hands-off strip drops to four
amber pixels on the top edge so an internal tool is not in every frame of his
screen recording. That part changes nothing for you: the desktop is still
whoever's it was, `request_control` and `drive_desktop` behave exactly the same,
and the mode expires on its own after thirty minutes.

**What does change is the card column, and it is worth knowing before you park on
a question:**

- **Everything you put on screen waits.** `notify_user`, every blocking ask, and
  the progress window: they queue instead of appearing and they all drain the
  moment he goes loud. Urgent waits too, and it comes out first when the
  recording ends.
- **He still hears it arrive.** The earcon plays for the card that would have
  taken the screen, so this is not do-not-disturb - it is the picture, not the
  notification. The spoken `speak` line is held until the card is actually on
  screen, because a voice reading over a take is the loudest thing AgentBox does.
- **Nothing is lost and nothing is refused.** The item is in the inbox from the
  second it arrives, its timeout runs from arrival exactly as it always does, and
  `agentbox pending` lists it. A blocking call parks the way it would behind any
  other card. One consequence to be deliberate about: an `act_unless_stopped`
  window elapses while the card is held, so the action proceeds unseen - the same
  as under do-not-disturb, and the reason a veto is the wrong shape for anything
  you would not want done unwatched.
- **So budget for the wait.** A question asked mid-recording can sit for the rest
  of the take. If your call has a `timeout_s`, that clock is running: pick a
  window that survives one, or check `agentbox control state` first - it says
  `quiet: the sign is demoted for 24m more, 3 cards waiting` when the mode is on.

## CLI reference

For shell/hook use. Common flags on every command: `--agent NAME`,
`--project NAME`, `--session ID`, and `--json` (print the full result object
instead of the bare answer).

| Command | Purpose | Key flags |
| --- | --- | --- |
| `agentbox notify` | fire-and-forget event | `--title` (req), `--body`, `--level`, `--action "Label::cmd"` (x3) |
| `agentbox ask` | blocking choice (2-9) | `--title` (req), `--option` (x2-9, req), `--body`, `--level`, `--timeout`, `--default`, `--strict` |
| `agentbox input` | blocking free text | `--title` (req), `--body`, `--timeout`, `--default`, `--multiline` |
| `agentbox confirm` | blocking yes/no | `--title` (req), `--body`, `--timeout`, `--default`, `--strict` |
| `agentbox veto` | act-unless-stopped | `--title` (req), `--in SEC`, `--body`, `--level` |
| `agentbox secret` | masked secret | `--title` (req), `--to-file PATH` and/or `--stdout`, `--timeout` |
| `agentbox form` | multi-field form (1-6) | `--title` (req), `--field SPEC` (x1-6), `--body`, `--timeout` |
| `agentbox review` | diff approval | `--title` (req), `--diff-file PATH` (or stdin), `--body`, `--timeout` |
| `agentbox say` | read a line out loud | `TEXT...`, or piped on stdin; `--wait`, `--timeout SEC` |
| `agentbox drive` | synthetic pointer/keys | script on stdin or flags; `--window TITLE`, `--speed N`, `--wpm N` |
| `agentbox control` | ask for the desktop, hands-off strip | `request REASON [--window SEC]`, `activity LINE`, `release`, `state`; `pause`, `resume`, `quiet`, `loud` are the human's verbs |
| `agentbox sync` | who else is here, and what you are for | presence: `announce PURPOSE [--area A] [--activity LINE]`, `activity LINE`, `agents [--area A] [--project P]`, `peers`, `attach`. leases: `lock NAME [--ttl N] [-- CMD]`, `unlock NAME`, `locks`. signals: `post TOPIC [DATA]`, `await TOPIC... [--after SEQ]`. shared: `get`, `set`, `del`. Plus `--key`, `--json` |
| `agentbox show` | markdown viewer | `FILE` or `-`, `--watch`, `--title`; `--artifact FILE` runs interactive HTML instead |
| `agentbox artifact` | hear what the human did in an artifact | `wait --id ID [--name N] [--timeout SEC]`, `read --id ID [--name N]`, `--json` |
| `agentbox panel` | roll the session panel down or up | `show`\|`hide`\|`toggle`\|`state`, `--json` |
| `agentbox walkthrough` | durable board reviews | `create --spec FILE`, `open ID`, `list [--find Q] [--state S]`, `read ID [--ack]`, `await [ID] [--timeout SEC]`, `repair [ID]`, `delete ID` |
| `agentbox progress` | progress bar from stdin | `--title` (req), `--indeterminate` |
| `agentbox stats` | interruption insights | `--since 24h\|7d\|30d\|0`, `--json` |
| `agentbox dnd` | do-not-disturb | `on`\|`off`\|`status` |
| `agentbox mute` / `unmute` | silence an agent | `AGENT` or `--list` |
| `agentbox summon` | raise/focus current card | |
| `agentbox inbox` / `app` | open the UI | `app --tab home\|session\|agents\|assignments\|inbox\|history\|viewer\|library\|settings` |
| `agentbox status` / `version` / `logs` | daemon health, build, event log | `logs --follow` |
| `agentbox quit` | stop the daemon gracefully | |

`--strict` (on `ask`/`confirm`) disables the free-text reply hatch. The `--field`
spec is `type:key[:opt1,opt2][=default]`, e.g. `choice:env:staging,prod=staging`,
`text:tag`, `bool:notify`.

### Exit codes (stable contract)

Every blocking command maps its outcome to an exit code. Branch on these:

| Code | Meaning |
| --- | --- |
| 0 | answered / yes / approved / proceeded / success |
| 1 | no / vetoed / rejected |
| 2 | usage error (bad flags) - fix and retry |
| 3 | unanswered (timed out with no default, or dismissed) |
| 4 | transport/daemon error - retry, or the daemon could not start |

The bare answer prints on stdout (the chosen option, the typed text, etc.);
`--json` prints the full result object instead.

### Result object (what blocking calls return)

```json
{
  "id": "k...",
  "answered": true,
  "answer": "the chosen option, or yes/no for confirm",
  "reply": "free text, if the human typed instead of choosing",
  "values": {"field_key": "value"},
  "default_applied": true,
  "vetoed": false,
  "approved": true,
  "secret_path": "/path/to/secret-file"
}
```

When `answered=true`, exactly one of `answer` / `reply` / `values` / `vetoed` /
`approved` carries the result for that kind. A secret returns `secret_path`
(the value is on disk, 0600, never in the JSON unless the human opted into
`--stdout`).

## Identity (who is interrupting)

Every interaction carries an identity so the human knows who is asking and
the inbox/stats can group by agent:

- `agent` - defaults to your process name; set with `--agent` (CLI). Over MCP it
  is derived automatically.
- `project` - defaults to the working-directory basename; set with `--project`.
- `session` - empty by default; set with `--session` or the `AGENTBOX_SESSION_ID`
  environment variable.

`AGENTBOX_SESSION_ID` matters for the in-app **Session tab**: when AgentBox launches a
Claude session itself (`agentbox app --tab session`), it spawns the child with an
`AGENTBOX_SESSION_ID`, and any ask/confirm/notify you make with that session id
renders inline in that tab instead of as a standalone card. In a normal terminal
agent you can leave session empty. `AGENTBOX_INSTANCE` selects a named, isolated
daemon (separate socket/state) - rarely needed.

## Limits and validation

- Choice options: 2-9. Form fields: 1-6. Action buttons: 0-3.
- `report_progress` percent is clamped to 0-100.
- `act_unless_stopped` / `--in` window must be positive (omit to take the
  configured default, 15s).
- `request_secret` needs a sink: `--to-file` and/or `--stdout`.
- `urgent` level breaks through Do-Not-Disturb (unless the human disabled that).

## Patterns

**Arriving on a shared machine:** `announce("<why this session exists>")` first,
then read the `peers` it hands back. Nobody there, `partial` false: proceed.
Peers there: decide how to split before you edit anything, and say so through the
peer's row rather than guessing. `partial` true: proceed as though you have
company, because you may.

**Staying legible:** `set_activity` whenever what you are doing changes. Two
lines of yours are on the human's board - the purpose (why) and the activity
(now) - and the second one going stale is what "stuck" looks like from outside.

**Naming things so the next agent understands them:** one idiom, `kind:scope`.
`repo:agentbox`, `subsystem:webui`, `role:release`, `vm:boris-vm`. It costs
nothing and it means two agents that never met agree on what a name refers to.

**Long task with progress (MCP):** start `report_progress` with no `id` and a
`title`; on each step call it again with the returned `id` and a new `percent`;
finish with `done=true` (or `error="..."`). Completion shows a success/error
toast. The bar lives in its own surface and never blocks your other calls.

**Long task with progress (shell):**
```sh
seq 0 5 100 | while read p; do echo "$p building"; sleep 0.3; done | agentbox progress --title "Build"
```

**Get approval before a risky change:** `request_review` with the unified diff;
proceed only on `approved=true`; surface `comment` if changes were requested.

**Proceed by default, allow a stop:** `act_unless_stopped` ("Pushing to main in
15s"); proceed when `vetoed=false`.

**Need a credential:** `request_secret`; read the returned `path`; use the value;
delete the file; never log it.

**Report a result, no question:** `notify_user` (`success` when it worked,
`error`/`urgent` when it did not).

**Hand a change over for a real review:** `create_walkthrough` with steps
citing the change (diff included), then `await_walkthrough` on the returned
id. If your session ends first, nothing is lost: the next session calls
`read_walkthrough` with `ack:true` and takes the submission exactly once.
Act on the `unclear` set first - each entry carries the human's note saying
what needs answering.

## Anti-patterns (do not)

- Do not ask a question with `notify_user` (it does not wait or return an
  answer). Use a blocking tool.
- Do not poll or loop blocking calls to "check in" - one ask per real decision.
- Do not print, echo, or log a secret value; only ever touch the file at the
  returned path.
- Do not spam `notify_user` for incremental progress; use `report_progress`.
- Do not set `urgent` for routine messages; it escalates and pierces DND.
- Do not speak on every call. A line the human hears twenty times is noise they
  will turn off, and then they lose it for the one that mattered.
- Do not block on a question the human gave you a default/policy for - act.
- Do not ask for the desktop with a card, or with a spoken line alone. Both are
  over the moment they are read, and the human has no way to tell whether you
  are still driving. `request_control`, and leave the strip up.
- Do not state added/removed on a walkthrough's file-backed blocks or write
  line numbers into its prose - supply the diff, bind the phrases, and AgentBox
  derives the rest. The validator refuses both, with directions.
- Do not delete a walkthrough to "clean up" after reading its handback; the
  library is the human's record of what was reviewed and what they said.
- Do not work anonymously. A session that never calls `announce` shows on the
  human's board as a dim row with no purpose, which is worse than useless to
  them: they can see that something is running and nothing about what.
- Do not announce once and then go quiet. A purpose from an hour ago beside an
  activity line from an hour ago is indistinguishable from a hung session, and
  the human cannot tell which of their agents needs rescuing.
- Do not poll `list_agents` in a loop waiting for the roster to change. Ask when
  you are about to act.
- Do not conclude you are alone from a roster that said `partial`. It means the
  daemon cannot see everybody, so absence is not evidence - the same rule that
  applies to a silent card applies to a quiet board.
- Do not assume a shared tree is yours because nothing has gone wrong yet. Two
  agents editing one checkout is how a catch-all `git add` sweeps somebody
  else's unfinished work into your commit, and how a `git reset` drops their
  finished one. Both happened here on 2026-08-04, in one hour.

## Where to look next

- `agentbox docs agent` - this manual, short form, in the binary.
- `agentbox schema` - the JSON Schema for the Item/Result wire protocol.
- `agentbox docs setup` - paste-ready MCP/hook registration.
- [recipes.md](recipes.md) - copy-paste integration snippets.
- [06-configuration.md](06-configuration.md) - the human's config knobs.
