# Which of your agents needs you, on one screen

> **In short.** With several agents running, the Agents board answers the same
> three questions on every row: why this agent exists, what it is doing right now,
> and whether it is stuck. An agent writes the first two. AgentBox works out the
> third itself, so a stuck agent cannot report itself as fine.
>
> **Read on if** you have more than one agent running and no way to tell which one
> is waiting on you. **Skip to** [[Taking turns|taking-turns]] for the primitives
> this board is watching.

![The Agents board: a shared-values block over rows grouped by area. One agent is chipped asking you, one blocked while it waits on the lock deploy:checkout-api held by release-bot, one listening on tests:green, and a dim row reading no purpose given](img/agents-board.png)

## Three questions, always in the same place

Every row reads the same way. The purpose is the headline: one line the agent
wrote about why it exists, "cutting release 2026.7.30" rather than a process
name. Under it, who it is and what it is doing right now, with the age of that
line beside it, in the same grammar the hands-off strip uses:
`test-runner · checkout-api · running the pre-release suite · 12s`.

On the right, one chip. That is the state, and the agent did not write it.

Rows group by the repo the agent is standing in, derived from the git top level
and the origin rather than from anything an agent declares, so two worktrees of
one checkout land together. That grouping is the point: two agents in one tree is
the thing you want to see before either of them does.

An agent that never said what it is for still gets a row, dim, reading `no
purpose given`. A board that showed only well-behaved agents would not be worth
opening.

## Blocked and listening are the pair to get right

Nine chips, and one order of precedence, so a row that is several things at once
shows the one that matters most. `asking you` wins: a question of theirs is pending,
and that row is why [[your inbox|nothing-gets-lost]] has something in it. Then
`driving desktop`, which means that agent [[has your mouse|hands-off]]. Then
`blocked`, `listening`, `reporting` for a live progress bar, plain `working`, and
after 90 seconds with nothing said, `quiet` with the age shown.

Two of those look alike on screen and mean opposite things. Blocked is an agent
that cannot proceed and somebody else is the reason. Listening is an agent parked
on a signal it expects, which is the feature working, so a listening row holds that
chip instead of decaying to quiet. Waiting on purpose must not look like hanging.

A blocked row carries its own second line, and that line is the whole story while
it lasts: `waiting on deploy:checkout-api for 20s, held by release-bot`, place 2
of 3. The holder is a button. Click it and you land on the holder's row, so two
agents waiting on each other is something you follow rather than a diagram you
assemble in your head.

## The chip is the daemon's word, not the agent's

An agent writes two things on its row and neither is a claim about how it is
doing. Purpose and activity colour a row; they never define it. Everything the chip
says comes from something the daemon can see for itself: a question of theirs that
nobody has answered yet, a call parked in `acquire_lock`, a held desktop run, a
connection that is or is not still open.

That split is not tidiness. A self-reported state would be at its least reliable
in exactly the case this screen exists for, which is an agent that has stopped
being able to report.

Even so, the board managed to lie once, and it lied in the reassuring direction.
On 2026-08-04 it was photographed reading "3 working" beside activity lines that
had not changed for 4m53s. Every push at the surface was caused by a verb, and
"working" decays on elapsed time alone, so with nothing happening on the machine
nothing recomputed it. The shell's `agentbox sync agents` read the same roster
correctly as quiet the whole time. It ticks itself once a second now and pushes
whenever the board would otherwise be wrong, and the lesson generalises past this
surface: anything derived from elapsed time has to be recomputed by a clock rather
than by traffic.

The row exists before the model says anything, too. A session's start hook
announces it for nothing, which is why a fresh `claude -p` in a scratch directory
that had never heard of AgentBox appeared on the roster within one second, wearing
a placeholder purpose that reads as unfinished.

## What a dead session leaves behind

Two blocks sit above the agents, and both are there because coordination state
outlives the agent that made it.

One is headed "Locks with no live holder". A session that died holding a lock does
not release it, because the work it started may still be running, so the row names
the lock, the agent that is gone, and what the recorded process is doing: its pid is
still alive and nobody gets this until it exits, or the pid is gone and the next
waiter is granted on the next tick. Break lock lives here, behind a confirm that
says what it does.

The other is the shared blackboard, with abandoned claims sorted to the top and
their count in the heading in the warning colour. A claim whose owner is gone is
work that was started and never finished, and nothing else on the board can say so:
not the lock table, because a claim is not a lock, and not the agent's own row,
because the agent is gone.

Both blocks show with nobody on the board at all, which sounds obvious and was not:
they used to hide behind "No agents attached", and an empty roster is the exact
state where leftover coordination state matters most.

> [!CAUTION]
> Breaking a lock hands it to the next waiter and stops nothing. Do it while the old
> holder's process is still alive and you have two agents inside one deploy, which
> is the failure orphaning exists to prevent. The row tells you which case you are
> in before you press it.

## Where the list stops being the whole truth

Presence lives in memory, on purpose: a hold must not outlive the ability to see
who is holding it. So a daemon restart empties the roster, children redial and
replay what they announced, and the first touch of a lock after a restart says
honestly that it is gone.

When any session on the machine is too old to have a roster row of its own, the
header says so in a sentence rather than quietly listing fewer agents. "You are
alone" has to be true when it is said, or not said.

And this is a window, not a fence. Nothing here stops an agent from editing a file
another one is halfway through. That is what [[taking turns|taking-turns]] is for,
and the board is where you watch it happen.

**Next:** [[how agents wait for each other instead of asking you|taking-turns]],
or [[what happens when one of them needs your mouse|hands-off]].
