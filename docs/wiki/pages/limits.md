# Linux, X11, and the things it will not become

> **In short.** AgentBox runs on Linux with X11, and there is no Wayland, macOS or
> Windows build. It carries discrete questions and events, so it is not your chat
> window, not your terminal and not the machine's notification centre. The rough
> edges at the bottom are the ones known and not fixed.
>
> **Read on if** you would rather find the reasons to say no here than later.
> **Skip to** [[Safe on a work machine?|is-it-safe]], or [[Install|install]].

A page of limits is cheaper to write than to earn back, so this one is complete as
far as anybody knows. Nothing below is softened.

## X11 is not a soft requirement

Five things need X11, and between them they are the product. Placing a card on the
monitor your pointer is on. Noticing that the focused window is fullscreen, which is
what holds a chime while you are presenting. The three global hotkeys. Driving the
desktop with synthetic input. And `agentbox summon`, which is a
`_NET_ACTIVE_WINDOW` message and has no other way to exist.

The CLI and the MCP tools are ordinary process work and do not care.

That is the shape of it. On a session without X11 you have a working queue and no
way to be interrupted by it, which is not the product.

> [!IMPORTANT]
> There is no Wayland build, no macOS build and no Windows build, and Wayland is
> not a near miss. Card placement, fullscreen detection, the hotkeys, the target
> lock a driven script uses and the summon key all read or write X. Keeping the code
> portable where portability is free is a rule here; trading Linux quality for it is
> not.

## What it does not try to be

It is not the place your conversations live. The tools carry discrete events and
questions, and the transcript stays in the agent's own tooling. The one exception is
a session AgentBox started itself, which does get a conversation surface, and even
there the point is starting and steering the run rather than being the window you
read a model in.

It is not a terminal replacement, and it is not the machine's notification centre.
It shows what agents and your own scripts send it, and nothing else. Your desktop's
own notifications, your mail and your calendar stay where they are, and there is no
plan to collect them.

There is no remote and no mobile delivery. A card appears on the machine the agent
is running on, full stop, and if you are not at that machine then the inbox is what
you come back to. A relay has been floated and does not exist. Neither does a cloud,
an account or a sync service, which is the same sentence as
[[nothing leaves the machine|is-it-safe]].

## A card's height is a guess, and a wide table can beat it

The window opens at a size estimated from the item's text: a base height, twenty
pixels per line of body wrapped at 52 columns, and a bit more for whatever the
answer zone is. For prose that is close enough to look deliberate, and a frameless
window has to be, because empty space under a two-line question reads as a bug.

A table, a long code block or a diagram is not prose, and 52 columns says nothing
useful about how tall one of those will render. So a body like that can arrive with
the card already scrolling, or looking tighter than it should. It is cosmetic rather
than dangerous, and it is the roughest edge you are likely to see in a normal week.

Two things blunt it: the body scrolls inside the card, and a very long one folds
away behind a hint so the question stays visible above it. Neither is as good as the
real answer, which is that long content belongs in the reading window. An agent that
sends a report as a card is using the wrong tool, and
[[the tool list|what-agents-can-do]] has the right one.

## The hands-off strip refuses to get out of the way

While an agent holds your desktop, an amber strip sits at the top of the screen
saying so. It is designed to step aside for a fullscreen window and leave a
four-pixel amber line in its place, so a film is not covered by a 620 by 62 sign for
a whole run.

Half of that works.

Watched for the first time on 2026-08-06, the marker maps exactly as specified:
`1920x4`, amber, across the whole top edge. The strip does not yield. It is a
notification-type window, and the compositor layers notifications above a fullscreen
window whatever the stacking order says, so lowering it cannot win, and a fullscreen
app gets both the strip and the line.

That is a decision rather than an open bug, and the reason is the direction of the
failure. What this feature exists to prevent is a covered strip reading as "the
desktop is yours" while an agent is driving. What happens instead is a sign that
will not be covered, which is the same error in the safe direction. The fix is to
hide the strip rather than lower it, and an unmap and remap cycle mid-run risks
taking your keyboard back on the way in, which would be worse than a strip over a
film.

## Amending a review is not built

A walkthrough is a change you walk step by step and hand back once. Revising one
after you have started marking it up does not exist: the tool that would do it is
registered, always refuses, and tells the agent to create a fresh walkthrough
instead. Your marks and comments are never overwritten, which is the property that
was worth keeping while amendment waits.

**Next:** [[what enforces the safety claims|is-it-safe]], or
[[the words this wiki uses|glossary]].
