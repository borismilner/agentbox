# AgentBox for agents

AgentBox is a desktop interaction hub. Use it to reach the human at the
keyboard: post an event, ask a blocking question, confirm an action,
announce an act-unless-stopped countdown, or collect a small form. Answers
come back as the call's result. Drive it by running the `agentbox` CLI; every
blocking command prints its result on stdout and sets a stable exit code.

Identity defaults are filled for you: `--agent` (the parent process name),
`--project` (the working directory's basename), `--session` (empty). Pass
them explicitly when running several agents so each gets its own color.

## When to interrupt

Interrupt only for a decision you cannot make safely on your own: a
destructive or irreversible action, a genuinely ambiguous requirement, a
secret you do not have. For progress and results, use `notify` - it is
fire-and-forget and never blocks. A blocking question the user does not
need is the failure mode to avoid.

## Commands

- `notify` - fire-and-forget event. Non-blocking.
  `agentbox notify --level success --title "Tests passed" --body "412, 0 failures"`
  levels: info, success, warning, error, urgent (urgent escalates).
  add up to 3 action buttons that run a command on click:
  `--action "Open PR::gh pr view --web"` (repeatable).

- `ask` - one choice from 2-9 options. Blocks. Prints the chosen label.
  `agentbox ask --title "Deploy?" --option "Run now" --option "Skip" --timeout 300 --default "Skip"`
  `--option "Label::a short description"` adds a description.

- `input` - free text. Blocks. Prints the text. `--multiline` for long input.
  `agentbox input --title "Release tag?"`

- `confirm` - yes/no. Blocks. Exit 0 yes, 1 no.
  `agentbox confirm --title "Push to main?"`

- `veto` - act-unless-stopped. Announce an action with a countdown; it
  proceeds unless the user stops it. Blocks until the window elapses
  (proceed, exit 0) or the user stops it (vetoed, exit 1).
  `agentbox veto --in 15 --title "Pushing to main"`
  Prefer this over a blocking confirm when no answer is the common case.

- `secret` - masked entry for a credential. The daemon writes the value to
  `--to-file PATH` (mode 0600) and it never enters your transcript; pass
  `--stdout` to receive the value instead (it warns the user). Blocks.
  `agentbox secret --title "npm token" --to-file ./token`

- `form` - up to 6 typed fields in one card. Blocks. Prints JSON.
  `agentbox form --title "Release" --field choice:env:staging,prod --field text:tag --field bool:notify=yes`
  field spec: `type:key[:opt1,opt2][=default]`, type is choice|text|bool.

- `progress` - a live progress bar for a long task. Non-blocking; reads
  stdin. Each line is `NN` (percent), `NN status text`, or a bare status
  line; EOF finishes it (a completion toast follows). `--indeterminate`
  starts a spinner when there is no known fraction.
  `migrate.sh | agentbox progress --title "Migrating users"`

- `control` - ask for the desktop before driving it, and say what you are
  doing while you have it. `request` blocks; silence for `--window` seconds
  is consent. Exit 0 granted, 1 denied, 3 another agent holds it.
  `agentbox control request "clicking through the board" --window 20`
  `agentbox control activity "reading the takeaway aloud"` then `agentbox control release`
  One always-on-top strip stays up for the whole run: on screen means hands
  off, gone means the desktop is theirs. Release the moment you stop.

- `stats` - interruption history. `agentbox stats --since 7d` (or 24h, 30d, 0
  for all time); `--json` for the raw object.

Add `--json` to any blocking command for the full result object instead of
the bare answer.

## Exit codes (stable contract)

- 0 - answered / yes / proceeded
- 1 - no / vetoed
- 2 - usage error (bad flags); fix the command and retry
- 3 - unanswered: timed out with no default delivered
- 4 - transport or daemon error

## Result shapes (--json)

- ask / input: `{"id":"k..","answered":true,"answer":"Run now"}`
- reply hatch: `{"id":"k..","answered":true,"reply":"free text"}`
- timeout:     `{"id":"k..","answered":false,"default_applied":true,"answer":"Skip"}`
- confirm:     `answer` is `"yes"` or `"no"`
- veto:        `{"id":"k..","vetoed":true}` or `{"id":"k..","vetoed":false}`
- form:        `{"id":"k..","answered":true,"values":{"env":"prod","tag":"v1.4.0"}}`

Every choice and confirm card also accepts free text as an answer (the
reply hatch) unless you pass `--strict`. Handle a `reply` field as well as
`answer`. `agentbox schema` prints the full JSON Schema for these shapes.

## MCP

`agentbox mcp` is an MCP stdio server; register it so an MCP host (such as
Claude Code) can call these tools. Run `agentbox docs setup` for a paste-ready
`.mcp.json` and hook snippets.

- `announce` -> say what this session is FOR, and get back the agents already
  working where you are (non-blocking). **Your first AgentBox call.** See "Working
  beside other agents" below
- `list_agents` -> the live roster: identity, purpose, current activity, state
- `acquire_lock` -> take a named lock, BLOCKING in a queue until it is yours.
  See "Taking turns over a shared resource"
- `try_lock` -> take it only if it is free right now; never waits
- `release_lock` -> hand it to whoever is queued behind you
- `post_signal` -> tell the other agents something happened (non-blocking). See
  "Telling another agent, and waiting to be told"
- `await_signal` -> BLOCK until a matching signal arrives, and get everything
  since your cursor in one batch. This is what replaces a polling loop
- `shared` -> read and write small shared state with compare-and-swap
  (`op: get | set | delete`, non-blocking). Claiming one item of fanned-out work so
  no two agents double it is `if_version=0`. See "Splitting work nobody doubles"
- `notify_user` -> notify (non-blocking)
- `retract` -> take back an item you posted, before he deals with it. Pass the id
  `notify_user` gave you, or omit it to withdraw everything this session still has
  pending. Use it the moment something you announced stops being true: a warning
  waits on his screen until he CLICKS it, so a "build failed" whose build you have
  since fixed is worse than no notification at all
- `ask_user` -> ask; pass `options` for a single choice, omit them for free text
- `confirm_action` -> confirm; returns `confirmed` true/false
- `act_unless_stopped` -> veto; returns `vetoed` true/false
- `ask_user_form` -> form; returns the field `values`
- `request_secret` -> secret; returns a file `path`, never the value
- `request_review` -> diff review; pass `diff` (or a `path`); blocks and
  returns `approved` true/false plus any `comment`
- `show_document` -> open a markdown file or inline content in the reading
  window (non-blocking); far richer than dumping markdown into the terminal
- `speak` -> say one line out loud (non-blocking; `wait` returns after it is
  heard). No card, no inbox entry - an aside, not an interruption
- `drive_desktop` -> move the pointer, click and type on the human's desktop,
  as they would; validate-then-run script, one step per line. Announce first,
  drive sparingly
- `request_control` -> BLOCK until the human hands you their desktop, then a
  HANDS OFF strip stays on their screen for as long as you hold it. Do this
  before any run of drive_desktop worth the name, and before driving anything
  else on their screen. Returns `granted`, `denied`, or `held_by` (another
  agent has it - wait, do not drive)
- `set_activity` -> what you are doing right now, in one line, kept current
  (non-blocking). It writes your row on the human's Agents board always, and the
  HANDS OFF strip additionally while you hold the desktop.
  `release_control` -> give the desktop back and take the strip down, the moment
  you stop driving. Presence is the signal, so a card or a spoken line cannot
  replace these
- `report_progress` -> a live progress bar (non-blocking). Omit `id` and set
  `title` to start; pass the returned `id` on every later call to update
  `percent` (or `indeterminate`); call `done` (with `error` to fail). Prefer
  this over repeated `notify_user` for incremental progress
- `show_artifact` -> run interactive HTML in a window: a chart to hover, a slider
  to drag, a panel to click. Non-blocking; returns an `artifact_id`
- `await_artifact_event` -> BLOCK until the human does something in an artifact,
  and return what they did (`name` and `data`)
- `read_artifact_events` -> take what they have done already, without blocking;
  repeats of one name are coalesced to the newest
- `create_walkthrough` -> a durable, step-by-step code review on the review
  board. Non-blocking; returns a `walkthrough_id`
- `await_walkthrough` -> BLOCK until the human submits that review; the whole
  handback arrives in one turn, unclear steps first
- `read_walkthrough` / `list_walkthroughs` / `delete_walkthrough` -> the stored
  review library; `read` with `ack:true` takes a waiting submission exactly once
- `amend_walkthrough` -> refuses in this build; create a fresh walkthrough for
  revised content
- `list_assignments` / `read_assignment` -> the work AgentBox runs on its own, and one
  of them whole with AgentBox's diagnosis of its prompt and knobs
- `create_assignment` / `update_assignment` -> write one, or improve one. An
  update changes only the fields you send, and `params` merges over the stored
  values - an edit must not blank what the human tuned
- `run_assignment` -> run one now (non-blocking, returns a `run_id`);
  `assignment_runs` -> how the last ones went. `delete_assignment` removes it
  with its history, so prefer `enabled:false` to pause

## Working beside other agents

You are probably not the only agent on this machine. Several sessions share one
repo, one daemon, one desktop and one human, and none of you can see the others
without asking. This is how you ask.

**Announce first.** `announce("<why this session exists>")` before you do
anything else. The human watches a live Agents board of every session; your
purpose is the headline of your row, so write the line they would recognise
("porting the settings surface to the new theme"), not a summary of your tools.
Call it again if the mission changes. A session that never announces still shows
up - the daemon registers what your child knows, and where the hook recipes are
installed a SessionStart hook has already put your row there wearing a placeholder
purpose - but neither can know what you are FOR, which is the one thing the row is
for. Announce even if a row already looks like yours: after a `/clear` your own
child still carries the previous conversation's purpose.

**Keep the activity line current.** `set_activity("running make check")` as the
work moves on. It is cheap, non-blocking, and it is the difference between "that
session is alive" and "that session is stuck". Re-sending an unchanged line
deliberately does not reset its age, because repeating yourself is not progress.

**Look for company before you touch a shared tree.** `announce` returns the
agents in your area, and `list_agents` answers the same question later. An area
is derived from the repo you are in, so two sessions in one checkout - or in two
worktrees of one repo - find each other without either declaring anything. If
peers come back, coordinate: split the files, or wait. Two agents editing one
tree is how a catch-all `git add` sweeps somebody else's unfinished work into
your commit, which is a thing that has actually happened here.

**`alone` is the only claim worth trusting, and only when it is true.** A read
answers `partial: true` when the roster cannot see everybody - a session whose
child predates this feature has no row. On `partial`, never conclude you are
working alone. Silence is not evidence.

**Company finds you, so read the line when it arrives.** When your area gains or
loses an agent, the news is appended to the result of whatever tool you call
next, as a line starting `sync:` - naming who arrived, what they said they are
for, and what state they are in. You get it once per arrival, on an ordinary call
you were making anyway, which is why you never have to poll for it. It is telling
you something you did not ask about: stop and coordinate before you carry on
editing.

Names for areas and topics follow one idiom, `kind:scope`, so a name reads the
same to the next agent: `repo:agentbox`, `subsystem:webui`, `role:release`.

Anti-patterns, each of which has cost somebody real work:

- **Polling instead of asking once.** Do not loop over `list_agents` waiting for
  something to change. Ask when you are about to act.
- **Announcing once and never updating.** A purpose from an hour ago beside an
  activity line from an hour ago is indistinguishable from a hung session.
- **Assuming the tree is yours.** It is shared until the roster says otherwise,
  and the roster is one call away.

## Taking turns over a shared resource

Finding a peer tells you to coordinate. A lock is how you actually do it, and it
is the difference between two agents agreeing to take turns and two agents both
running the deploy.

**Take one before anything shared** - the deploy, a repo other sessions are
editing, the VM, an external service, a scarce API quota. Names are a convention
in the `kind:scope` idiom, not a registry: `deploy:agentbox`, `repo:agentbox`,
`vm:boris-vm`. Pick the name the other agent reaching for the same thing would
pick, or you will each hold a different name for one resource.

```
acquire_lock(name="deploy:agentbox", timeout_s=600, note="deploying the mirror fix")
  -> granted:true                        # yours until you release it
  ... do the work ...
release_lock(name="deploy:agentbox")
```

**A timeout is a result, not an error.** It comes back with the holder's purpose,
what they are doing right now, how long they have held it and how many agents are
queued - everything you need to decide whether to wait again, do something else,
or go and talk to them. Nothing about the lock changed because you asked, and
re-arming is one more call. `timeout_s: 0` waits as long as the daemon allows a
parked call (25 minutes), which is the right choice when you genuinely cannot
proceed.

**`try_lock` when you have something else useful to do**, `acquire_lock` when you
are stuck without the resource. They are two tools rather than one flag so that
reading the name tells you whether your turn is about to park.

**Release the moment the protected work is done**, not at the end of your
session: everything behind you is stopped until you do. If your session dies
holding a lock, it is not silently freed - the hold goes *orphaned*, and the next
agent waits until the process you recorded is gone too, because a dead session
does not prove the `make deploy` it started has finished. Pass
`release_on_detach: true` when your session IS the critical section and nothing
outlives it.

**Two things you may be told without asking.** If the human breaks your lock from
his Agents board, the next AgentBox call you make carries a line saying so - and
breaking reassigns the lock without stopping *you*, so stop touching what it
protected. The same happens when an orphan of yours is reclaimed.

Anti-patterns here have cost more than the roster ones:

- **Holding a lock across a question to the human.** Every agent behind you is
  then waiting on something only he can end. Ask first, then take the lock.
- **Taking a lock and forgetting to release it.** Set the note, do the work,
  release. If you cannot guarantee the release, prefer `try_lock` plus short
  critical sections.
- **Two locks in the wrong order.** Two agents taking `repo:` and `deploy:` in
  opposite orders is a deadlock; the daemon refuses the second one by name rather
  than letting you both sit there, but the fix is to take them in one order.
- **Inventing a private name for a shared thing.** A lock nobody else takes
  protects nothing.

From a shell (a Makefile, a hook, a non-Claude agent), the same table:

```
agentbox sync lock deploy:agentbox --timeout 600 -- make deploy   # released on ANY exit
agentbox sync lock deploy:agentbox --ttl 900 --note "…"           # detached hold
agentbox sync unlock deploy:agentbox --key KEY
agentbox sync locks                                               # who holds what
```

The wrapped form releases on any exit, signals included, which matters because a
foreground shell call from a Claude session is killed at 120s: for anything
longer, take the hold with `--ttl` and release it yourself.

## Telling another agent, and waiting to be told

A lock is how two agents take turns. A **signal** is how one tells the other that
it is their turn - and it is what replaces the loop where you sleep, call a status
tool, decide, and do it again, spending a whole turn per look and reacting an
interval late.

```
post_signal(topic="tests:green", data={"suite": "race"})   # non-blocking
  -> seq:41, delivered:1

await_signal(topics=["tests:green"], timeout_s=600)        # BLOCKS
  -> signals:[{seq:41, topic:"tests:green", agent:"claude", key:"…", data:{…}}]
     cursor:41
```

**A signal is stored, so it is delivered whether or not anybody was listening.**
`delivered: 0` means nobody was parked at that moment; it does not mean the signal
was lost. A peer that was busy picks it up later, and a daemon restart does not
lose it inside the retention window (1000 per topic, 7 days).

**The cursor is how you never miss one.** Every signal has a place in one global
sequence. Pass the `cursor` you were last given back as `after_seq` and you get
*everything matching since then, in one batch* - three events that fired while you
were editing a file arrive together on your next call. Omit `after_seq` and you
wait for what happens from now on, which is what you want when you have nothing to
resume from.

**Topics are names, or families.** An exact name waits for one event; a name
ending in `*` is a plain string prefix, so `done:*` catches `done:migration-3`.
The `kind:scope` idiom again: `tests:green`, `build:failed`, `done:<chunk>`.

**To message ONE agent, post to its private topic.** Every session's key names
`to:<key>`, and the key is on its roster row from `announce` or `list_agents`. So
"message that agent" is `post_signal(topic="to:"+peer_key, data={…})`, and
listening is `await_signal(topics=["to:@me"])` - `@me` is your own key. A
request and a reply are two signals with the reply topic named in the request.
What travels is structured data between programs, not a conversation.

**Three topics the daemon posts for you**, so you can wait on things you did not
have to instrument:

- `agents:<area>` - an agent joined, announced or left your area. Park here when
  you are genuinely idle and want to know when company arrives; when you are
  mid-task the `sync:` rider tells you anyway, without a call.
- `lock:<name>` - that lock changed hands, with the reason and whoever holds it
  now. This is how you find out a resource freed *without* queueing for it.
- `to:<your key>` - anything addressed to you.

**A timeout is a result, not an error**, exactly as it is for a lock: the cursor
comes back unchanged, so waiting again misses nothing that happened in between.
Any park is bounded by `[sync] wait_max_s` (25 minutes), because the MCP client
abandons a call it has heard nothing about for 1800s and never tells the daemon it
did - so re-arming is normal rather than a failure.

**`gap: true` is the one answer you must not skim.** It means your cursor is older
than what retention still holds, so signals between the two are gone and the batch
cannot be complete. Treat whatever you were tracking through those signals as
*unknown*, not as *not having happened* - a silently incomplete batch is how two
agents come to believe they each own one chunk of work.

The composition is the point. "Deploy when the tests are green" is
`await_signal(["tests:green"])` then `acquire_lock("deploy:agentbox")`: two calls,
no polling, no human in the middle, and the whole chain visible on the human's
board while it happens. Fanning work out is one signal per finished chunk and one
waiter on `done:*`.

Anti-patterns:

- **Polling a status tool in a loop.** That is the thing this replaces. Park once.
- **Parking speculatively.** `await_signal` occupies your turn. Wait for something
  you are actually blocked on; for news you merely want, the rider brings it.
- **Posting a wildcard.** `*` belongs in `topics`, never in a `topic` you post to.
- **Dropping the cursor.** Waiting again from scratch after a timeout re-reads
  nothing and can miss what arrived in the gap between two calls.
- **Sending a payload instead of a pointer.** A signal carries a fact, a request
  or a hand-off, capped at 16 KB. Anything bigger travels as a file path.

From a shell:

```
agentbox sync post tests:green '{"suite":"race"}' --key KEY
agentbox sync await tests:green --timeout 600 --key KEY    # exit 0 got one, 1 timed out
agentbox sync await 'done:*' --after 41 --key KEY           # catch up from a cursor
```

## Splitting work nobody doubles

A lock says whose turn it is and a signal says something happened. Neither can say
**chunk 7 is mine**, which is what a fanned-out job needs before it starts work a
peer already started. That is `shared`: small named state with compare-and-swap,
one tool with `op: get | set | delete`, and none of the three blocks.

```
shared(op="set", key="claims/chunk-3", value={"worker":"me"}, if_version=0, own=True)
  -> applied:true, value:{version:1, owner:"8b94…"}          # it is yours

shared(op="set", key="claims/chunk-3", value={"worker":"me"}, if_version=0)
  -> applied:false, stale:true, value:{version:1, owner_agent:"codex"}
                                                             # somebody was faster
```

**`if_version` is the whole feature.** Versions start at 1, so 0 is free to mean
"only if this key does not exist" - and that is a claim. Omit it to write no matter
what; pass the version you last read to write only if nobody moved it since.

**A refusal is a result, not an error.** `stale: true` comes back with the current
value and version, so a loser goes to the next key and an updater retries at once.
No lock, no read-modify-write, no second call to learn what you lost to.

**One key per item.** Ten workers over `claims/<chunk>` is first-writer-wins per
item and every loss is cheap. Ten workers CAS-ing one `claims` object is nine
retries a round, each of them a model turn.

**`own=True` is what makes abandoned work visible.** Your session key and agent
name go into the row, and every read checks them against the live roster - so a
session that dies holding a claim leaves `owner_gone: true` behind instead of a
chunk nobody will ever finish. Take one over by writing the key with its current
`version` as `if_version`. Leave `own` off for state that belongs to nobody, like a
counter.

**A `key` ending in `*` reads the family**, so a ten-chunk table is one call.

**Every write posts `shared:<key>`.** "Wait until somebody claims or finishes
something" is `await_signal(["shared:claims/*"])` - one wake mechanism in this whole
design, and this is not a second one. The signal carries the key, the version and
the owner, never the value: when it rings, `get` the value.

**Values are never trimmed** - the deliberate difference from signals, because
retention on a claim table would hand one chunk to two agents. They survive a
daemon restart and leave when you delete them, so **delete a key when its work is
done**. The cap on how many may exist refuses a new key rather than evicting
somebody's claim.

Anti-patterns:

- **A shared value used as a message.** Post a signal; a value is state.
- **One hot key for a fan-out.** One key per item, or every loser pays a retry.
- **Claiming without `own`.** Nobody can then tell your live work from a corpse.
- **Leaving finished claims behind.** That is what fills the table.
- **Polling a key.** Park on `shared:<key>` instead.
- **A payload instead of a pointer.** 16 KB, and a file path is the idiom.

From a shell, which is the shape a Makefile or a hook wants:

```
agentbox sync set claims/3 mine --if-version 0 --own --key KEY   # exit 0 won, 1 lost
agentbox sync get 'claims/*' --key KEY                          # the whole table
agentbox sync del claims/3 --key KEY                            # finished with it
```

## Artifacts

`show_artifact` runs your HTML. Write a self-contained document, or a React
component module with an `export default` - React and Tailwind are already in the
page, so `import React from "react"`, JSX and utility classes all work. Two rules,
both absolute:

- **No CDN and no other package.** The artifact runs with no network at all, so a
  script tag or an import of anything but react/react-dom is a missing library.
- **`window.agentbox.emit(name, data)` is the only way out.** It cannot fetch, cannot
  reach AgentBox, cannot store anything. What the human does reaches you because you
  called `await_artifact_event` (or `read_artifact_events`), and then you act with
  your own tools.

That is the loop worth building: show a control panel, wait on it, do the work the
human asked for by clicking, then show the result. Use it when a question has more
than a few answers, or when the answer is a number, a shape or a selection rather
than one of nine options - a card is better for anything a sentence can ask.

## Walkthroughs

For a change worth walking rather than skimming, hand over a walkthrough
instead of a diff card: `create_walkthrough` takes a declarative spec -
ordered steps with prose, citations `{path, lines:[from,to]}` pinned to a
commit, and the change's unified diff as the manifest - and opens the review
board. The human marks each step understood or unclear, writes notes and
anchors comments to lines; everything persists across sessions.
`await_walkthrough` returns the whole review in one turn. Three spec rules,
all validated with directions: never state added/removed on a file-backed
block (the diff is the only carrier); never put literal line numbers in
prose (bind a phrase to a code region instead); end a diff-carrying review
with a check step. CLI:
`agentbox walkthrough create --spec review.json | await ID | read ID --ack | list | delete ID`.

**Read the standard before you write one.** How to structure the steps, where
the explanation goes versus the annotations, what coverage has to account for:
MCP resource `agentbox://standards/walkthrough`, or `agentbox docs walkthrough`. Four
things it will tell you that are easy to miss:

- **Paragraphs are explicit.** Prose is inline segments, so a bound phrase can
  sit mid-sentence. Set `"p": true` on the segment that starts each paragraph
  or the whole step renders as one wall.
- **Notes are the annotation channel.** `notes: [{at: [from,to], text}]` on a
  block renders as a numbered badge on the line and the text in the margin
  beside the code. That is where the "why" belongs - not in the prose above.
- **Binds are how prose points at code.** `{"t": "the guard", "bind": "guard"}`
  with `binds: {guard: {block: 0, lines: [77,79]}}` lights those lines when the
  reader is on the phrase. It is also the answer to the no-line-numbers rule.
- **The glossary keeps definitions out of the prose.** `glossary: [{term,
  short, body?, also?}]` on the spec. AgentBox marks the first occurrence of each
  term in each step and opens the entry only when the reader asks for it
  (click, or `g`). Define the domain acronyms and house terms this reader
  cannot guess - and spell them in the prose the way the entry spells them, or
  agentbox warns that nothing can reach them.

## Assignments

An assignment inverts the usual direction: AgentBox summons an agent, on a schedule
or on demand, with the whole toolbox available while it runs. A run is an
ordinary session, so the human can open it, read it and take it over.

You write them. Propose a prompt and its knobs with `create_assignment`,
`run_assignment` once so the human sees it work, read it back with
`assignment_runs`, and keep updating until they are satisfied.

`schedule` is empty (ad-hoc), `every 30m` / `every 4h` / `every 1d` (from the
last run), or `daily 09:00` / `weekly mon 09:00` (wall clock). A missed slot is
skipped and recorded, never caught up. One minute is the floor.

The prompt is a template: `{{key}}` is substituted from the parameters, and a
placeholder nothing fills is left verbatim rather than dropped. Declare a knob
for each in `spec` - `[{key, type, label?, help?, default?, min?, max?, unit?,
values?, body?}]`, type `text|number|slider|toggle|enum|path|markdown` - and AgentBox
renders the form. `panel_html` is the escape hatch (React/Tailwind in the
artifact sandbox, no network) and its channel is two-way: values out with
`window.agentbox.emit("params", {key: value})`, merged over what is stored;
values in from `window.agentbox.params`, refreshed by an `agentbox:params`
window event on every change. What the human sets there is what
`read_assignment` returns. Declare a spec as well - a panel that fails to load
must never make an assignment uneditable, and a key with no knob behind it is
dropped by the next `params` update.

Write for an agent nobody is watching: say what to do, what is worth
interrupting for, and how to report.

The blocking tools wait for the human exactly like the CLI, and their
results carry the same fields as the JSON shapes above.
