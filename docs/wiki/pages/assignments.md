# Work that starts without you

> **In short.** An assignment is a prompt AgentBox hands to an agent on a schedule,
> or the moment you press Run now. The tunable parts are typed controls rather than
> prose you edit, every run keeps what it recorded, and a run is an ordinary session
> you can open and take over part way through.
>
> **Read on if** you have a check you keep meaning to run and never do. **Skip to**
> [[Sessions and the panel|sessions]] for the conversation a run happens in.

## A prompt with knobs, so retuning it is not editing prose

The prompt carries `{{placeholders}}`, and what fills them is a small set of typed
controls: text, a number, a slider, a toggle, a list of allowed values, a path, or
a block of markdown that is there to explain the rest. They render with the same
machinery the settings surface uses, so they are validated and they look like the
rest of the app.

That is the difference between an assignment and a cron line.

Raising a threshold from 80 to 90 is a slider, not a careful edit inside a
paragraph you wrote three weeks ago and no longer remember the shape of.

An agent can write the whole thing, which is usually how one starts. The seven
assignment tools are an MCP surface first, so any agent in any project can read an
assignment, propose a better prompt and write it back, and New opens a session
pointed at exactly those tools so the authoring conversation happens where the
assignment will live.

There is an escape hatch for controls the knobs cannot express: the agent writes an
HTML panel, and it runs in the sandbox AgentBox already uses for agent-authored
interfaces, with no network. The values it writes go to the database like any
other, and the typed knobs stay available for every assignment, so a panel that
throws on load can never make an assignment uneditable.

## Three ways it starts, and the one you will use first

| It runs | Written as | Good for |
|---|---|---|
| only when asked | leave the schedule empty | anything you want on a button |
| on an interval from the last run | `every 30m`, `every 4h` | a check that should keep sampling |
| at a wall-clock time | `daily 09:00`, `weekly mon 09:00` | the report you want before standup |

Run now is the one that matters on the day you write it. An assignment you cannot
try is an assignment you find out about at 9am on a Tuesday, when what it actually
does is print an error into a summary nobody is reading. Pressing Run puts
[[a real session|sessions]] on screen, streaming, and you watch it work or watch it
fail.

> [!WARNING]
> A scheduled run is a real agent with real permissions, started while nobody is
> watching it. Each assignment carries its own model and its own mode, plan or full
> access, so give it the narrowest prompt that does the job and watch one Run now
> before you put a schedule on it.

## A missed slot is skipped, not caught up

Shut the laptop on Friday with three daily assignments armed and the obvious
behaviour is the wrong one. On Monday morning all three fire at once, plus the ones
that were due over the weekend, and the machine spends its first ten minutes
running checks about windows that have already closed.

So a slot that passed while AgentBox was not running is skipped, settled on
2026-08-01, with the shut laptop as the whole argument.

It is not silently dropped either, which is the other wrong answer: the runs list
gets a row reading `3 scheduled runs missed while agentbox was not running`, and the
count is the point. Nothing pretends the work happened, and nothing pretends the gap
did not.

The same instinct decides what happens when a run overruns its own interval. One
run per assignment at a time, and a second launch is refused rather than queued: a
usage check that has taken longer than its own four hours must not be joined by
another on top of it.

## AgentBox says when an assignment cannot work

A broken assignment is usually broken in one of two boring ways, and both are
visible without running it.

If the prompt asks for `{{threshold}}` and no knob fills it, the page says so, and
says what the consequence would be: the run sees those braces as literal text and
reasons about them. If a knob exists that the prompt never mentions, it says that
too, in the only wording that is any use to you: turning that knob changes
nothing.

Both are checked when you save, not when it fires. A prompt that has drifted away
from its own controls is the failure mode of any template with a form in front of
it, and finding out at 9am is what the diagnosis exists to prevent.

## What a run leaves behind

The run's last message is its summary. That is stated in the brief every run is
spawned with, along with the rest of what changes when nobody typed the prompt:
report in the reply, interrupt only when it matters, and do not end with an offer to
continue.

Each row in the history carries its state, what triggered it, when it started, how
long it took, the parameter values it actually ran with, and its summary. Open one
and the whole report is there.

To keep a measurement rather than a sentence, a run ends with a fenced block tagged
`agentbox-data` holding JSON. AgentBox lifts it out of the prose into a column of its
own, so you read a report and a month of runs reads back as a series. A block that
was never closed is left in the summary instead of swallowing it, because a
half-written block is a mistake worth seeing.

None of that waits for you to go and look. A run has every tool an agent has, so it
can send a notification when something matters, an urgent one when it cannot wait,
and a question when the decision is yours. Scheduled does not mean silent.

## The ceilings, and why they are not knobs

An hour is the limit on a single run. Past that it is killed and recorded as
overrun, because a wedged child would otherwise keep its assignment marked as
already running forever, and an assignment stuck in that state quietly never runs
again.

A run that dies loudly is the better failure.

Its child is stopped when it finishes and the transcript stays. A daily assignment
that left one alive per run would have thirty idle processes by the end of the
month.

And a run never steals your place. An assignment firing while you are typing into
another conversation does not move your selection, for the same reason nothing else
in AgentBox takes your keyboard: what you are in the middle of outranks what
arrived. Where it does show up is the [[Agents board|agents-board]], announced with
the assignment's name as its purpose before the child's first token.

**Next:** [[the conversation a run happens in|sessions]], or
[[the board where every running agent shows up|agents-board]].
