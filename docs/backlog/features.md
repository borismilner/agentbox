# Backlog - the next order of usefulness

Written 2026-08-07 against commit `69230d4`, after reading `wiki/FACTS.md`,
`00-vision.md`, `05-roadmap.md`, `07-field-requests.md` (FR51-FR95 in full),
`08-assignments.md`, `09-sync.md` and `STATUS.md`.

The founding problem is solved: a question reaches the human, a card looks
better than a dialog, and thirteen milestones of surface sit on top of it.
Every proposal below starts from the same observation. **AgentBox is the only
process on this machine that sees all of it** - which agent asked, what for,
how long the human took, what he chose, who is holding what, and whether he is
even at the desk. Nothing else has that view, and almost nothing has been built
on it yet. Thirty-nine tools answer "how do I reach him". None answers "should
I, and what happened last time I did".

Two sections. Section one extends what is there. Section two argues for things
that would change what the product is, and says where each one strains a
principle rather than pretending it does not.

Numbering is `F-nn` for this document and does not collide with the `FR` series
in `01-requirements.md` and `07-field-requests.md`. Where a proposal promotes an
existing FR, it says so.

---

## Section one: extensions

### F-01. Is he there, and is it worth asking

**The job it does.** An agent about to block for an answer has no idea what it
is about to block on. It cannot tell whether Boris is typing three feet away or
out for two hours, whether four other cards are already pending, whether this
project's questions usually come back in ten seconds or forty minutes, or
whether the desktop is in recording mode where its card will queue behind a
thirty-minute fuse. So it does the only thing it can: it asks, and it waits.
A read that returns those facts lets it choose instead. Present and idle-free
means ask now. Away with three cards already waiting means batch this one with
the next two. Idle-held with a median latency of forty minutes on a decision
worth two minutes of work means take the safe path and record what was chosen.
Who asked for this: FR88 recorded exactly this failure from the other end, a
card unanswered for half an hour that was dead at the agent's end when it was
finally answered, and called it "exactly the case AgentBox exists for". FR95
left the same hole open twice, a question whose arrival-anchored timeout expires
while it is queued and an `act_unless_stopped` that proceeds unseen. Both are
agents committing to a wait they had no information about.

**Why now.** Every fact this returns already exists and none of it is exposed.
`internal/presence/idle.go` runs the X11 screensaver read, the fullscreen check
and the GNOME do-not-disturb read on a five-second poll. `internal/daemon`
knows the pending count, the do-not-disturb latch, the recording-mode fuse and
whether the drop-down panel or app window is on screen. `store.go:463` computes
a median answer time per agent for the History surface. The roster knows how
many peers are in the same area. This is assembly, not mechanism, and it is the
first feature that changes what an agent *does* rather than what the human's
screen looks like. That is the one advantage the product has not spent.

**What it would take.** One MCP tool and one CLI verb over one new daemon
method. Return: presence (`present` / `idle Ns` / `dnd` / `quiet-hours` /
`recording` / `fullscreen-held`), pending count and the age of the oldest,
median and p90 answer time for this agent over the last thirty days and over
this hour of the day, whether a host surface is open so an ask would route
inline, and the mute state for this agent so an agent that has been muted stops
shouting. Reads never refuse, matching the rule set in `09-sync.md` that
visibility must not depend on good manners. Two days for the daemon method and
the tool, one for the CLI, one for the manual entry that teaches the decision
rather than the schema.

**What could go wrong.** The real risk is not the tool, it is what an agent
concludes from it. "He is away" is one inference away from "so I will decide",
and a wrong guess made confidently is worse than a wait. Two mitigations, both
load-bearing. The tool description must say the read is facts and not
permission, in the same voice `retract`'s description uses. And an agent that
chooses not to ask because of what it read should be required to say so: a
`notify` at info level naming the decision it took instead, which lands in
F-02's ledger. Without that pairing this feature quietly converts visible waits
into invisible decisions, which is the one trade the product must not make.

**Size.** Four days.

### F-02. The decisions that were made without you

**The job it does.** Some questions are never answered by a human. A card times
out and its default is delivered. An `act_unless_stopped` window elapses and the
agent proceeds. A question is dismissed to get it off the screen. One expires
while queued behind recording mode. Every one of those is a decision that was
made in Boris's name, and the only place any of it is visible today is as an
outcome chip on an inbox row he has to go looking for. The History surface
counts `answered` and treats everything else as absence. So the single most
important class of item in the record - the ones where the product answered for
him - is the class with no surface. This is a panel and a CLI report that lists
them: what was asked, by whom, in which project, what was delivered instead of
an answer, and how long ago. Who asked for this: FR95 named two of these holes
and left them open, and FR88's whole story is a decision taken in silence
because nobody was looking.

**Why now.** The data is complete and needs no migration. `items.state` already
carries `answered`, `dismissed` and `expired`, `items.dflt` carries what was
delivered, and the `transitions` table logs every state change with a
timestamp, so "resolved without a human" is a query, not a schema change. It is
also the accountability half of anything in the delegation direction. Standing
decisions (B-2) must not be built before the surface that shows what was
decided for him, because a rule with no ledger is a rule nobody can audit.

**What it would take.** One store query over `items` joined to `transitions`,
one Home panel ("decided without you: 4 today"), one filter on the Inbox that
is the same query, and `agentbox decided --since 7d [--json]`. Add one detail
the record is missing: when a default is delivered, write *why* into the
transition row (`timeout`, `veto-elapsed`, `dismissed`, `queued-expiry`), which
is a one-column migration on `transitions` and the only new storage in the item.
Three days, plus a day for the surface.

**What could go wrong.** It will be an uncomfortable panel, which is the point,
and there is a real chance it reads as nagging rather than as information. Keep
it on Home as a count that opens a filtered Inbox rather than as a card that
arrives; a notification about missed notifications is the noise FR30 exists to
remove. Second cost: honest counting will show that some defaults are working
exactly as intended, which pushes toward suppressing the routine ones, and that
suppression is how the panel becomes decorative. Do not add a hide rule in the
first version.

**Size.** Four days.

### F-03. The return brief

**The job it does.** The Agents board answers "who needs me right now" well
enough that Boris watches it instead of terminals. It cannot answer "what
happened while I was gone", and that is the question he actually has when he
comes back to the desk after three hours. Today it is reconstructed from four
surfaces: Inbox for what resolved itself, Agents for who is blocked and for how
long, Assignments for runs that fired or overran, History for the shape of it.
The return brief is one screen: since you left, three agents ran and two
finished, one has been waiting on `repo:agentbox` for forty minutes, one asked
something two hours ago and got its default, the nightly usage assignment
overran and was killed, and here are the two things still waiting. Who asked for
this: FR81 got Home built because Sessions was "empty and not that important
functionality", and Home answers the present tense well. The past tense is the
gap that opened when the board replaced the row of terminals, because a
terminal keeps its scrollback and a board does not.

**Why now.** The idle monitor already detects the return and already fires one
summary chime on it (FR29), so there is an existing event with nothing attached
to it. FR44's `missed_while_away` column already marks the items that arrived
while he was gone. The three data sources are the same three queries Home,
Agents and Assignments already run. And the `speak` path exists, so the brief
can be two sentences out loud when he sits down, which is the version that
actually gets consumed.

**What it would take.** Define "away" as the existing idle threshold crossing,
store the timestamp of the last presence transition, and window every panel
query on it. One new Home panel that only appears when the away window is
non-empty, plus a spoken two-line version behind `[presence] speak_return`
defaulting off until he has heard it. Reuse the FR29 return event as the
trigger. One week, most of it the panel and the wording.

**What could go wrong.** The failure mode is a brief that fires after a
five-minute coffee and says nothing, training him to ignore it. It needs a
floor (no brief under thirty minutes away, and none if nothing happened) and
that floor is a guess that will need tuning by use rather than by argument,
which is the same "living with the defaults" note `[flood]` carries in STATUS.
Second cost: it duplicates information three surfaces already show, and
duplication that drifts is worse than absence, so it must be the same queries
and not parallel ones.

**Size.** One week.

### F-04. Recall: search everything AgentBox has ever held

**The job it does.** The store holds every question and answer for months,
every walkthrough with its steps and annotations, every assignment run with its
summary and its `data` blob, and the session surface writes every transcript to
disk as JSON with a markdown sibling. That is a complete record of every
decision taken with an agent on this machine, and there is no way to search
across it. The Inbox search covers items only and only by substring. FR59 named
the same hole for walkthroughs alone and called search "the one thing that makes
a library worth more than a folder", because a walkthrough of a subsystem is the
start of the next piece of work there only if it can be found six weeks later.
The general version answers the question that costs the most time in practice:
what did we decide about this, and when. It serves the human as a search surface
and agents as a `recall` tool, and the agent half is the one that changes
behaviour. A session that can read what was decided about the panel's height
clamp in August does not re-litigate it in October, and the auto-memory note
that a carried handoff item repeated unchanged means nobody looked is exactly
the failure a searchable record prevents.

**Why now.** `modernc.org/sqlite` ships FTS5, so the index is a virtual table
and three triggers rather than a dependency decision. Four sources are already
in one place or one directory. And the corpus has reached the size where it is
worth searching: fifty-five sessions of items, walkthroughs, runs and
transcripts. It also composes with everything else here - B-2 needs to find
prior answers, F-02 needs to link a default to the last time the same thing was
asked properly.

**What it would take.** One migration adding an FTS5 table over items
(title, body, answer, reply), walkthrough steps, and assignment run summaries,
kept current by triggers; one indexer pass over the transcript directory on
daemon start and on write, because those live as files rather than rows; one
`recall` MCP tool and `agentbox recall QUERY`; one search surface, which can be
the Library tab widened rather than a tenth rail entry. Ranking by recency
first, because the most recent decision about a thing is usually the live one.
Two weeks, the transcript indexer being the awkward third of it.

**What could go wrong.** Search over a private record is search over everything
he has ever typed into a card, and the corpus contains fragments of things that
were never meant to be re-read, including bodies of questions about production
systems. Nothing crosses the socket that does not cross it today, and secrets
are already never stored, so the exposure is not new - but a tool that returns
arbitrary historical text to any agent in any project is a new shape of
exposure, and it needs a rule. Suggest: `recall` defaults to the calling
session's own project and requires an explicit `scope: "all"` to go wider, with
that widening logged. Second cost: an index that drifts from the store is worse
than no index, so the triggers must be tested against the retention pruner,
which deletes rows at daemon start.

**Size.** Two weeks.

### F-05. Start a session where the work is

**The job it does.** The Session surface spawns a real `claude` child, renders it
better than a terminal does, and runs it in the daemon's working directory,
which is almost never where the work is. So a session started from the app is
useful for talking and useless for doing, and every real session still starts in
a terminal. Three small things close that: a working-directory picker, a project
list that is not typed by hand, and a refusal when the directory already has an
agent in it. The project list is the interesting part, because the store already
knows every directory an agent has interrupted from (`items.cwd`, added in
migration 0003) and the roster derives areas from the git top-level. The list
of places Boris works has been accumulating for two months and nobody has read
it. Who asked for this: the working-directory picker is item 11 in STATUS's own
"do this next", and the owner direction in `MEMORY.md` is a desktop app where a
session launches without a terminal at all.

**Why now.** `session.Config` already carries `Dir`, `Mode` and `Model`
(`internal/session/driver.go:22-30`), so the daemon side of this is passing a
string that is currently empty. The refusal is the part that earns its keep: on
2026-08-04 two agents in one checkout swept an unfinished doc into an unrelated
commit and dropped a finished commit off main, and M13 made that collision
*visible*. A launch-time check against the roster makes the most common form of
it *impossible*, and it is the cheapest possible down payment on the worktree
work in B-3.

**What it would take.** A picker on the Session surface backed by three
sources merged: recent `items.cwd` values ranked by recency, roster areas, and
free text with a git-top-level resolve. Pass `Dir` through. Before spawning,
ask the roster who is in that area; if anyone is, show the peer's purpose and
activity and require a confirm rather than blocking outright, because two agents
in one tree is sometimes correct and always worth seeing first. One week.

**What could go wrong.** A confirm that is usually correct to click through
becomes a click-through, which is how the check stops working. It should show
the peer's actual purpose line and not a generic warning, so the decision has
content. Second cost: the picker makes the Session surface the front door,
which is a direction FR81 explicitly demoted for being empty and unimportant.
It stops being empty only if launching from it is genuinely better than a
terminal, and today it is not, because of F-06.

**Size.** One week.

### F-06. Approve an edit without leaving the panel

**The job it does.** The session surface offers Plan and Full and nothing
between, because the stream-json `control_request` protocol is not handled, and
`default` or `acceptEdits` would stall the moment the child wanted a permission
decision. So the two modes on offer are read-only and no-brakes. That is the
single reason the panel cannot replace a terminal for real work: real work is
done in `default`, approving tool calls as they come. Handling the protocol
means the child's permission prompt renders inline in the panel or as an
AgentBox card, is answered with a keystroke, and the answer goes back down the
same pipe. Who asked for this: it is item 11 in STATUS, deferred deliberately
in M8, and it is the load-bearing dependency of every ambition to launch
sessions from the app.

**Why now.** Because F-05 and B-3 are worth nothing without it. A launcher that
can only start sessions in the two modes nobody uses for real work is a demo. It
is also the last piece of the M8 slice-2 promise: the panel already renders
turns, streams tokens, hosts asks inline and persists conversations, and this is
the missing verb.

**What it would take.** Parse `control_request` from the NDJSON stream in
`internal/session/streaming.go`, model the request kinds, present each one
through the existing inline ask host (the drop-down panel and app window both
became ask hosts on 2026-08-06, so the surface exists), write `control_response`
back on the stdin encoder. Then open `default` and `acceptEdits` in the mode
selector. The risk is that a request the code does not recognise must not hang
the child, so an unknown kind needs a deny-and-report path rather than a
silence. One to two weeks, mostly protocol fidelity and the timeout behaviour.

**What could go wrong.** The panel becomes a place where destructive tool calls
are approved with a keystroke, on the same keyboard path as answering a
notification, which brushes against vision principle 3. The mitigation the
product already has is the FR28 undo grace, and a permission approval should get
the longer end of that clamp rather than the shorter one. Second cost: this is
protocol work against another program's interface, which will break when that
interface changes, and it is the first place AgentBox becomes coupled to a
Claude Code version rather than to a CLI contract.

**Size.** Two weeks.

### F-07. A session that survives closing the window

**The job it does.** Closing the app window kills the child of every live
session. The conversation is saved and reopenable, the process is not. So the
app window is a place you must not close while work is running, which is exactly
the property the tray-and-daemon model was built to avoid, and it is a strange
asymmetry: the window's X button hides to the tray and keeps sessions alive,
while a real close ends them. Keep-alive means the daemon owns the children,
the window is a view onto them, and a session that was running when the window
closed is still running when it opens. Who asked for this: STATUS item 11,
deferred in M8.

**Why now.** Assignments already proved the daemon can own a session's lifecycle
without a window: a run spawns a child, renders in the Session surface, and has
its child stopped on completion by the daemon rather than by a surface. Sessions
started by the human are the same object with a different owner, and the
lifecycle code to copy is already written and tested.

**What it would take.** Move child ownership from the window's lifetime to the
daemon's, add an explicit end for a session (the conversation stays, per the
assignment rule), decide the ceiling for an abandoned child. Assignments picked
one hour and made it deliberately not a knob; a human-started session probably
deserves an idle timeout rather than a wall-clock one. Three to five days.

**What could go wrong.** Thirty idle `claude` processes is the exact failure
`08-assignments.md` calls out and the reason a run's child is stopped when it
finishes. Human-started sessions have no natural completion event, so the
ceiling is doing all the work and it must be visible: the roster should show an
alive-but-idle session as its own state, or this feature silently accumulates
processes and tokens.

**Size.** One week.

### F-08. The questions the History surface cannot answer

**The job it does.** History runs exactly one query and renders three views of
it: totals, per-agent, per-day, over four fixed windows. It answers "what did
this cost me" at the level of a count and a median. It cannot answer which
project costs the most interruptions, which kind of question is the expensive
one, which hour of the day is being eaten, whether the tail is the problem
rather than the median, or how many items resolved without him and how. Those
are the questions that would change how he works rather than tell him how much
he worked. Per-project is the sharpest of them: if one repo produces sixty
percent of the interruptions, that is a fact about that repo's setup and not
about agents in general.

**Why now.** Every column needed is already stored - `project`, `kind`,
`session_key`, `state`, `created_at`, `resolved_at` - and the only reason the
answers are missing is that one `SELECT` chose five columns. It is also the
measurement layer under F-02 and B-2: a standing decision is only proposable if
the record can group identical questions, and grouping needs kind and project in
the aggregate.

**What it would take.** Widen the stats query, add p90 beside the median, add
group-by controls to the surface for project, kind and outcome, add an
hour-of-day histogram, keep the four windows and add a custom range. Keep
`agentbox stats --json` as the same shape plus new fields so nothing that reads
it breaks. One week, and the surface is most of it.

**What could go wrong.** Analytics surfaces grow controls faster than they grow
answers, and this one is one segmented control today, which is honest. The guard
is to add each grouping only with the question it answers written next to it in
the UI, and to refuse a chart nobody asked a question for. Second cost: the
retention pruner deletes items below `history.keep_level` after thirty days, so
every long-window answer is quietly biased toward warnings and errors. That is
already true and already unstated; if this surface starts being used for
judgment, it has to say so on screen.

**Size.** One week.

### F-09. A month of assignment runs as a series

**The job it does.** Boris's own founding example for assignments was
"periodically check the usage of Claude and display a summary, making it a
warning notification when getting critical usage and maybe even collecting usage
statistics for later analysis". The first three clauses shipped. The fourth did
not: `assignment_runs.data` holds the `agentbox-data` block from every run, and
the surface shows it as raw JSON on one row at a time. The design doc says "a
month of runs reads back as a series" and there is no series. Charting it closes
the one gap between what assignments promised and what they do, and it is the
smallest such gap in the product.

**Why now.** Because the data has been accumulating since 2026-08-01 with
nothing reading it, and because the chart engine is already in Go: the markdown
renderer draws bar, line, area, scatter, pie and doughnut from a fence, server
side. A series over one numeric key across a run history is that engine pointed
at a different source.

**What it would take.** Scan an assignment's runs, collect the union of numeric
keys in `data`, offer them as selectable series, render through the existing
chart path with the run's timestamp as the x axis. Handle the schema drift that
will certainly happen when a run starts emitting a differently shaped block:
show the gap rather than interpolating it. Three to four days.

**What could go wrong.** It invites the assignment author to design a schema,
and there is no schema, deliberately. The right posture is that the chart is
best-effort over whatever keys are there, and a key that appears in twelve runs
and vanishes in the thirteenth draws twelve points and stops. Anything cleverer
is a schema by the back door.

**Size.** Four days.

### F-10. The long undo

**The job it does.** Right now an action either needs a human or it does not.
`confirm_action` blocks until answered. `act_unless_stopped` proceeds after a
countdown measured in seconds. There is nothing for the shape that actually
dominates when he is away from the desk: the agent should do the thing, and he
should be able to reverse it when he next looks, hours later. The long undo is
`act_and_report`: the agent performs the action and posts an item carrying the
command that reverses it. The card says what was done and offers Undo, and that
button stays live in the Inbox until it is used or it expires. A formatting pass,
a dependency bump, a generated migration, a rebase onto main - all things where
waiting six hours for a yes costs more than reversing a no.

**Why now.** Both halves exist. FR32 action buttons already run a caller-supplied
command through `sh -c` in the caller's working directory, persisted in
`items.actions` across a restart since migration 0003, and FR28 already
implements the concept of an answer that has not landed yet. What is missing is
the framing that makes those two into a delegation primitive rather than two
features. It is also the honest counterpart to F-01: an agent that learns he is
away needs somewhere to put a decision it took anyway, and "done, reversible"
is a better answer than a `notify` nobody can act on.

**What it would take.** One item kind, one required field (the undo argv, not a
shell string, using `config.SplitArgv` which is already fuzz-tested), an expiry
after which the undo button greys out with the reason, and a rule that the undo
runs in the recorded `cwd` and reports its exit status back into the item.
Refuse to accept an `act_and_report` with no undo command, because the whole
contract is the reversal. One to one and a half weeks.

**What could go wrong.** This is the highest-risk item in section one, and the
risk is a lie: an agent that supplies an undo command which does not actually
undo. AgentBox cannot verify it, and a card that promises reversibility it does
not have is worse than no card. Two mitigations. The manual entry must be
written as a contract with examples of undo commands that are honest (`git
revert <sha>`, `git checkout <sha> -- path`) and examples that are not (`rm` of
a generated file that was not generated). And the item must show the undo
command verbatim on the card, so the promise is inspectable rather than
asserted. Second cost: it expands what an agent may do unattended, which is a
real change in the product's posture and belongs in the vision text rather than
in a tool description.

**Size.** One and a half weeks.

### F-11. Triage by mission

**The job it does.** With several agents running, a card's identity pill gives a
hue and a name and nothing about what the agent is for. The purpose line, which
is the one piece of information that makes triage possible, lives only on the
Agents board. So answering a question means either recognising the agent by hue
or switching surfaces to find out why it exists. Putting the purpose in the card
footer, and adding a peek at what else is queued, turns the card from an
interruption into a thing with a place in the day. Who asked for this: `09-sync.md`
records the decision to leave it out ("cheap and it widens every card; it can
ride the FR list if triage by mission turns out to be wanted") and FR48 records
queue peek as parked. Three days of running four sessions at once is the answer
to whether it turned out to be wanted.

**Why now.** The card already receives the session key (migration 0012), and the
roster already maps a session key to its purpose, so this is a join and a line
of layout. It is the cheapest item in this document and the one most directly
produced by the way he now works.

**What it would take.** Purpose on the card footer, elided to one line, absent
rather than blank when a session never announced. Queue peek as the count that is
already there made hoverable or expandable into titles and kinds, no new
window. Two to three days.

**What could go wrong.** Every card gets wider or taller, which is the reason it
was left out, and card height is already estimated from raw text length so a
markdown body can overflow. The footer must be a fixed single line that does not
enter the height estimate. Second cost: a purpose from an hour ago on a card
arriving now is stale in the same way the board can be, and a confidently wrong
purpose is worse than none - so show its age when it is over an hour old, which
the board already does.

**Size.** Three days.

---

## Section two: the bets

### B-1. Away without becoming a cloud service

**The insight.** The most obvious hole in a product whose whole point is
reachability is that it can only reach one screen. FR20 has sat in the parked
bucket since the first requirements pass as "webhook relay for away-from-desk
delivery", and it has stayed there because a webhook needs the network and
principle 5 says no network. But principle 5 is not one claim, it is two, and
the product has already separated them once. `[speech] command` takes any argv
that satisfies a narrow contract, which is how piper became optional and Kokoro
became possible without AgentBox knowing what either is. The same move applies
here: **AgentBox never speaks a protocol, it runs a program the human chose.**
Delivery becomes `[relay] command`, an argv fed one JSON item on stdin. On this
machine that program is `ntfy publish`, or `signal-cli send`, or
`matrix-commander`, or `mail`. AgentBox holds no credentials, opens no socket,
speaks no HTTP, and the policy tests that assert no surface fetches from a host
stay green because the binary still makes no network call.

**The harder half is answering.** Delivery one way is a notification service and
he already has five of those. The thing that would make the product
indispensable is answering a blocking question from the phone, which needs an
inbound path, and an inbound path is where a cloud service starts. The trick
that avoids it is the same one in reverse: AgentBox never listens, it *asks*.
`[relay] poll_command` is an argv run every N seconds that prints back whatever
answers have arrived, one per line, `<token> <answer>`. `ntfy subscribe --poll`,
`signal-cli receive`, an IMAP fetch - all of them are a program the human owns
and configures, and all of them mean the daemon's only inbound surface is still
a unix socket and a subprocess's stdout. This is also the answer to FR19, the
parked subscribe API, without building it: FR19 imagined server push to a
companion surface, which needs a server. A polled subprocess gets the same
companion surface with nothing listening, so FR19 should be closed as
superseded rather than left in the bucket.

**Where it strains a principle, plainly.** Principle 5 says "local only. Unix
socket, no network listener, no cloud, no telemetry." After this, three of those
four are still literally true and the fourth is unchanged. What changes is the
spirit: the content of a question leaves the machine, over a channel AgentBox
did not write and cannot audit. That is a real loss and it should be stated in
the vision rather than smuggled in as a config key. My argument for taking it:
the alternative is not privacy, it is a question that sits unread for six hours,
and the product's founding complaint is precisely that. The relay is also
strictly opt-in with no default, which means the shipped posture is unchanged
and a user who never sets `[relay] command` has exactly the product they have
today.

**The security work is the interesting part, not the plumbing.** An answer
arriving from a channel AgentBox cannot authenticate can approve a destructive
action, and that is unacceptable without three rules. First, a per-item
capability token, random and single-use, minted when the item is relayed, so an
answer must quote something only the delivered message contained. Second, a
whitelist of what may be answered remotely: choice and confirm yes, free-text
reply probably, `request_secret` never (the value would cross the relay, which
defeats the entire design of that tool), and destructive confirms only when the
caller opted the item in with a flag. Third, every remotely-answered item is
marked as such in the store and in the Inbox, forever, because the audit
question "did he actually say this" must have an answer. Add the obvious
practical rule: no relay while the desk is live, since a question that will be
answered in ten seconds should not become a phone notification.

**What it would take.** Config plumbing and `SplitArgv` reuse is a day. The
outbound path with a formatter and the do-not-relay-while-present gate is three
days. The poll loop, token minting, verification, whitelist and audit marking is
the bulk, call it a week and a half. A recipes page with four working transports
is two days and is the difference between a feature and a feature nobody
configures. Three weeks, and it should not start until F-01 exists, because
"is he away" is the gate the whole thing hangs on.

**What it costs beyond the principle.** A second delivery path doubles the
surface where an item can be lost, and the failure modes are ugly and remote: a
transport that silently drops, a poll command that starts failing at 3am, an
answer delivered twice. The exactly-once guarantee is currently enforced in one
place with one store; it now has to hold across a channel with no delivery
receipt. That is the real engineering cost and it is larger than the code.

### B-2. Standing decisions: making delegation a gradient

**The insight.** Right now a thing either needs a human or it does not, and that
binary is why the interruption count does not fall over time. The record says it
should: if an agent asks the same question forty times and gets the same answer
forty times, that was never a question, it was a policy the human had not been
asked to write down. The database has the evidence and no way to act on it. The
tempting version of this feature is to mine the record and start answering, and
that is the version that betrays the product, for two reasons. Technically,
substring similarity cannot tell two questions apart reliably enough to bet a
`rm -rf` on. Ethically, an answer AgentBox invented is indistinguishable in the
record from one he gave, and the moment that is true the history stops being
evidence.

**So the design inverts it. The agent names the question; the human writes the
rule; AgentBox only ever proposes.** Add an optional `policy_key` to `ask_user`
and `confirm_action`: a stable string the caller chooses, like
`agentbox/deploy-on-dirty` or `format-before-commit`. It changes nothing on its
own. What it does is make identity exact rather than inferred, which is the hard
problem solved by one optional field. Then: when a key has been answered the
same way three times, AgentBox offers to promote it. The promotion card is the
only place a rule is ever born, it is always the human's click, and the rule
carries a scope (this project or everywhere), an expiry (thirty days by default,
because a policy that outlives its reason is the failure mode), and the answer
it will give. From then on a matching question is answered instantly, and the
item lands in history marked "answered by rule R, which you created on
2026-08-14", visible in F-02's ledger, with a one-click revoke that also reopens
the question.

**Why it is possible now and was not before.** Three prerequisites landed
recently and none of them was built for this. The session key (migration 0012)
means an item can be attributed exactly. F-02's ledger gives the surface where
an automated answer is visible rather than silent. F-08's grouping gives the
counting that makes a proposal defensible. And FR89's asymmetry rule - `retract`
touches only your own items, `dismiss --all` is the human's door alone - is
exactly the model this needs: an agent may name a policy key, and only the human
may turn one into a rule.

**Where the line is, and where it strains something.** Hard limits, all of them
load-bearing. Never `request_secret`, ever. Never a first-time question. Never a
question whose options changed since the rule was made, which means the rule
stores a hash of the option set and refuses on a mismatch rather than guessing.
Never silently: an auto-answer plays the same earcon as an arriving item at
info level, because the human must be able to hear a rule firing and think "that
is no longer true". The principle it strains is the honest one to name: vision
principle 1 is "unmissable, not annoying", and this feature makes some things
missable on purpose. That is the trade. My argument for taking it is that the
current alternative to a rule is not a considered answer, it is a timeout
default, which is an auto-answer with no author, no expiry and no audit.

**What it would take.** Optional field on two tools plus schema and manual, two
days. A `rules` table with scope, expiry, option-set hash and provenance, three
days. Match-and-apply in the daemon's item path with the earcon and the ledger
entry, four days. The promotion card, the rules list in Settings, and revoke,
one week. Three to four weeks with the testing this deserves, and it should be
built after F-02 and F-08 rather than beside them.

**The cost nobody will notice until later.** Rules rot. The whole feature is a
bet that a thirty-day expiry plus an audible firing plus a visible ledger is
enough to keep them honest, and there is no way to know that from the armchair.
It should ship with the expiry short enough to be annoying and be lengthened by
use, not the reverse.

### B-3. The place work starts, not only the place it interrupts from

**The insight.** Assignments and sessions are the same object seen from two
ends, and the product has not noticed. An assignment with an empty schedule is
an ad-hoc launch with a saved prompt, a model, a mode and a directory. A session
you want again tomorrow is an assignment with a schedule. Both spawn through
`internal/session`, both render in the Session surface, both appear on the
roster. Unifying them is what turns AgentBox from a place agents interrupt from
into the place they are started from, which is the direction the owner has
already stated, and it is mostly a naming and surface job over machinery that
exists.

**Then the part that is genuinely new: the worktree.** The coordination layer
exists because two agents in one checkout destroyed work on 2026-08-04, and M13
made that collision visible. Visible is not prevented. The next order is to make
it structurally impossible for the common case: launching a session creates a
git worktree and a branch for it, the child runs there, the roster row shows the
branch, and finishing offers to merge, keep or discard. Two agents in one repo
then cannot touch each other's files, because they are not in the same files.
This is the feature that converts the sync layer from advice into a guarantee,
and it converts the launcher from a convenience into the reason to use it. It
also composes with the assignment scheduler in the way `09-sync.md` predicted
when it called a scheduled run "the first real two-agent customer": a nightly
run in its own worktree cannot collide with a live session at all, so the
`repo:X` lock stops being the only thing between them.

**And the resource nobody owns: the budget.** The global instructions tell every
session to poll `claude -p /usage` and hand off before the quota runs out,
because running out mid-task is the one way work gets lost. That is a shared
resource with no owner, polled independently by every session, and AgentBox
already owns every other shared resource on this machine through the lock table.
The launcher is where it belongs: a usage panel fed by an assignment (the
example Boris himself wrote when asking for assignments), and a gate that
refuses to start a run when the weekly number is above a threshold and says so.
It is the same shape as a lock, and it is the difference between a launcher and
a scheduler that knows what it is spending.

**What it would take.** The unification is a week: one storage model, an
"ad-hoc" trigger that is already the empty-schedule case, and a launcher surface
that is the assignment editor with the schedule field optional. Worktrees are
two weeks and the hard part is not `git worktree add`, it is the ending -
merge, keep or discard with uncommitted changes present, and a worktree whose
session died. The budget gate is a week including the panel. Four to six weeks
total, and F-06 is a hard dependency, because a launcher whose sessions stall on
a permission prompt is worse than a terminal.

**What it costs.** Two vision non-goals, both already conditionally revisited by
M8: "not a terminal replacement" and "not a chat client". This is the commit
rather than the experiment, and after it the product is an agent workbench that
also notifies, not a notifier that also hosts sessions. That is a bigger change
than any feature in section one, and it changes what a new user is being asked
to adopt. Second cost, concrete: worktrees put AgentBox in the business of
mutating git state, which is the first time it writes to the repository rather
than reading it. FR78 already chose snapshot-over-git for walkthrough excerpts
on the grounds that "git is the repair path, not the durable one", and this bet
goes the other way. It needs the same care that decision got.

### B-4. One human, two machines

**The insight.** The team dimension is a firm non-goal and should mostly stay
one, but "team" is hiding two different things and only one of them deserves the
refusal. Multi-human is a different product. Multi-machine is not, and it is a
live gap in how this owner already works: he runs a 128-vCPU build box
specifically to keep the laptop free, agents run there, and those agents cannot
reach him at all. They interrupt into a terminal on a box he is not sitting at,
which is the founding problem of the product reproduced exactly, one SSH hop
away. `09-sync.md` says "not cross-machine, ever" and notes that `vm:boris-vm`
is a name, not a network. That is the crack worth widening.

**Why it is possible without breaking anything.** The transport already exists
and he already uses it. `ssh -R` forwards a unix socket. The remote `agentbox`
client speaks to a forwarded socket exactly as it speaks to a local one, the
peer-UID check still applies at the local end, and the daemon still has no
network listener of any kind. The authentication is SSH's, which is stronger
than anything AgentBox would write, and the credential story is the one he
already maintains. In other words the cross-machine version of this product is a
documented socket-forwarding recipe plus the small amount of work needed to make
the roster honest about it.

**What honest means, and it is most of the work.** A card from the VM must say
so, because "run the tests" means something different over there. Identity gains
a host, which the roster's `{Agent, Project, Session}` key does not carry today.
The area derivation breaks: two checkouts of the same repo on two machines
derive the same area from the git top-level and would be reported as peers in
one tree, which is exactly the false collision `09-sync.md` was careful to
avoid. So area needs a host component, and the discovery rider needs to know
that same-repo-different-host is not a collision. Locks are the opposite case
and the more interesting one: a lock on the deploy or on the VM itself *should*
be shared across the hop, and today it cannot be.

**What it would take.** A host field on the session key and through the roster,
two days. Area derivation with a host component plus the discovery rule, three
days. Card provenance in the UI, two days. The recipe, the systemd unit for the
forward, and the failure behaviour when the tunnel drops mid-question, which is
the part that will actually hurt, one week. Two to three weeks.

**What it costs, and what I would still refuse.** It costs the flat "not
cross-machine, ever" claim, which is currently a clean line and becomes a
qualified one: local to one human, over transports he owns. That is a defensible
place to stand. What I would refuse, and would write into the vision explicitly
rather than leave to be inferred: multi-human. The entire safety model here is
asymmetry between one human and N agents - `retract` touches only your own
items, `dismiss --all` is the human's door, resume from pause belongs to the
human alone, break-lock is unilateral behind a confirm. Add a second human and
every one of those rules needs an owner, a permission model and a conflict
story, which is a different product and the one that has to become a service to
work. Say no on the record, and say it because of the asymmetry rather than
because of the network.

### B-5. The desktop as a loop rather than a script

**The insight.** `drive_desktop` synthesises real X11 input, the target lock
verifies the receiving window before every click and keystroke, the strip makes
presence unmissable, and the pause latch gives the human the keyboard back
mid-run. That is a genuinely good delegation primitive with one blindness at the
centre: the agent cannot see. It clicks coordinates it was told, so every drive
script is an open loop written in advance, and anything the screen does that the
agent did not predict is invisible until a human notices. FR57 asked for the
missing half and put the preference order in the right place: ask the
application for structured state first, read the AT-SPI accessibility tree
second, and fall back to a targeted downscaled capture last, with the agent
required to say which of the three it used. Closing the loop is the difference
between replaying a recorded macro and actually delegating a task in a GUI that
has no CLI, which on this machine is the IDE.

**Why it is possible now.** The lease and the lock did the hard part. There is
already a moment when AgentBox knows exactly which window is the subject, that
the human has granted the desktop, and that a HANDS OFF strip is on screen
saying so. A read scoped to *that* window, during *that* run, is a much smaller
and much more defensible thing than a general screenshot tool, and it is the
version FR57 argued for. FR54 (what the target application has open) and FR55
(guided navigation) are the same capability from two other angles, and all three
have been sitting unbuilt because none of them is worth building alone.

**What it would take.** An AT-SPI client is the bulk: a new dependency or a
`gdbus` subprocess, a tree walk scoped to the locked window, and a serialisation
that is small enough to put in an agent's context, which means naming roles and
labels rather than dumping a tree. Two weeks. A fallback capture path, scoped to
the window and downscaled, one week. The verb that lets an agent act on what it
read, meaning click-by-role rather than click-by-coordinate, is where the value
is and it is another two weeks. Four to six weeks, and it is the least certain
estimate in this document, because AT-SPI coverage in a JetBrains window is an
empirical question nobody here has answered.

**What it costs, honestly.** This is the most invasive thing in the document.
Reading the accessibility tree of a window means reading whatever is on screen,
including things that are on screen by accident. FR57's own privacy rule is the
right one and must be enforced rather than documented: capture on request only,
never on a timer, log that a read happened and never its content, and only ever
inside a granted lease with the strip up. It also strains the vision more
quietly than the network bets do, by making AgentBox a thing that observes the
desktop rather than a thing that appears on it. And I rank it last for a reason:
its value depends entirely on how good the accessibility tree turns out to be in
the one application that matters, which is unknown, so the honest first step is
a two-day spike that dumps the tree of a GoLand window and looks at it, not a
six-week commitment.

---

## Ranked, by value over cost

1. **F-01, is he there and is it worth asking.** Four days over data that all
   exists, and the first feature that changes what an agent does rather than
   what a screen shows. Everything below it gets better once agents can read
   the human's state.
2. **F-09, assignment runs as a series.** Four days, uses the Go chart engine
   already shipped, and closes the one clause of the owner's own founding
   assignment example that never landed.
3. **F-02, the decisions made without you.** Four days, no migration worth
   mentioning, and it is the accountability surface without which B-2 must not
   be built.
4. **F-11, triage by mission.** Three days for a join and a line of layout,
   answering a question `09-sync.md` explicitly parked pending evidence that
   running four sessions at once would produce.
5. **F-05, start a session where the work is.** One week, and the launch-time
   collision refusal is the cheapest possible prevention of the failure that
   caused the whole coordination layer to exist.
6. **B-1, away without a cloud.** Three weeks and one honestly-argued principle
   change, against the largest hole in the product. Highest absolute value in
   the document; ranked here only because of what it costs to decide.
7. **F-03, the return brief.** One week, and it fills the past tense that
   disappeared when the board replaced the terminals.
8. **F-07, a session that survives closing the window.** One week to copy a
   lifecycle assignments already proved, and it removes a trap in a surface he
   is meant to trust.
9. **F-04, recall over everything AgentBox has held.** Two weeks over a corpus
   that has been accumulating for two months with nothing reading it.
10. **F-06, approve an edit without leaving the panel.** Two weeks, and it is a
    hard dependency of B-3 and the last piece of the M8 promise.
11. **F-08, the questions History cannot answer.** One week, modest on its own,
    and the measurement layer under B-2.
12. **B-2, standing decisions.** Three to four weeks. The only thing here that
    would make the interruption count fall over time rather than rise.
13. **F-10, the long undo.** One and a half weeks, real value while he is away,
    and the highest chance of shipping a promise the code cannot keep.
14. **B-3, the place work starts.** Four to six weeks, the biggest change in
    what the product is, and the one whose worktree half turns the coordination
    layer from advice into a guarantee.
15. **B-4, one human two machines.** Two to three weeks for a gap that is real
    for this owner specifically and invisible to anyone else.
16. **B-5, the desktop as a loop.** Four to six weeks on the least certain
    estimate here. Do the two-day AT-SPI spike before believing any of it.

## What I would build next

**F-01.** It is four days, it needs no new principle, and every fact it returns
is already computed by the presence gate, the daemon queue and the stats query -
it is assembly. That is not the argument, though. The argument is that
thirty-nine tools all answer the same question from the same side: how do I
reach the human. Not one of them answers whether reaching him now is the right
move, so every agent on this machine makes that call with no information and
always makes it the same way, by asking and waiting. F-01 is the first tool
that gives an agent a reason to behave differently, and behaviour is where the
remaining value is. It is also load-bearing for the two bets that matter: B-1
cannot decide whether to relay without knowing he is away, and B-2 cannot
propose a rule without a record shaped by which questions were worth asking. It
should ship paired with the notify convention in its own "what could go wrong",
so that an agent which decides not to ask leaves a trace, and F-02 should follow
it immediately for the same reason. Build F-01, then F-02, then decide B-1 with
him, because that decision is his and not a session's.

## The non-goal I would revisit

**"No remote or mobile delivery in v1"** (`00-vision.md`, non-goal 3, and FR20
in the parked bucket). It is the only non-goal that contradicts the product's own
stated purpose. The problem statement says a question sits unread for minutes or
hours because the terminal is buried; a question sits unread far longer when the
human is not in the building, and today the product's answer to that is nothing
at all. B-1 argues the version that does not become a cloud service: AgentBox
runs a transport the human chose and still opens no socket, speaks no protocol
and holds no credentials. The vision should be amended in the same visible way
ADR-0009 amended principle 6 rather than quietly extended by a config key.

Two others deserve a note. **"Not a terminal replacement" and "not a chat
client"** were already conditionally revisited by M8 and are revisited properly
by B-3; the honest move is to stop calling them non-goals rather than to keep
qualifying them. And the one I would **not** revisit is multi-human, which is
not in the vision's list but is a firm non-goal in `09-sync.md`. Refuse it on
the record, and refuse it because the safety model is the asymmetry between one
human and N agents, not because of the network.

## The idea I rejected, and nearly did not

**Pre-filling the answer AgentBox expects.** Given the record, it is often
obvious what he will choose, and pre-selecting that option would remove more
keystrokes than anything else in this document. A card arrives with the likely
answer already highlighted, Enter sends it, and the median two seconds becomes
half a second. It saves more per card than anything else here, and I kept
trying to find a safe version of it.

There is none, and vision principle 3 says exactly why: "a stolen keystroke that
answers a question by accident is the worst possible failure". Pre-selection
converts every stray Enter from a no-op into an answer, and it does it hardest
in precisely the case the product was built for, when a card has just appeared
over something he was typing into. The FR28 undo grace does not save it either,
because the failure is silent: he would never learn that the answer he sent was
not the answer he meant. And the second-order cost is worse than the first. Once
the highlight is usually right, reading the card stops being the habit, and the
whole value of a question is that a human looked at it. B-2 is the version of
this idea that survives contact with the principle: the same saving, taken
deliberately and in advance, with an author, an expiry and an audible firing,
instead of taken invisibly one keystroke at a time.
