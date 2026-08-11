# What X11 buys, and the things it will not become

> **In short.** AgentBox builds and runs on Linux, macOS and Windows, and nothing
> that carries a message needs a display server. X11 is where it looks its best,
> because that is the one desktop where AgentBox places its own windows, and two
> capabilities are X11-only. Nobody has run the macOS build yet. It carries
> discrete questions and events, so it is not your chat window, not your terminal
> and not the machine's notification centre. The rough edges at the bottom are the
> ones known and not fixed.
>
> **Read on if** you would rather find the reasons to say no here than later.
> **Skip to** [[Safe on a work machine?|is-it-safe]], or [[Install|install]].

A page of limits is cheaper to write than to earn back, so this one is complete as
far as anybody knows. Nothing below is softened.

## X11 is an enhancement, and a real one

This page used to say "X11 is not a soft requirement". It was right for as long as
the code made it true, and it does not any more. Everything that delivers a
message, asks a question or brings back an answer is portable: the daemon, the
socket, the CLI, all twenty MCP tools, and every surface that puts something in
front of you. Without X11 they all still work. `make check` compiles macOS and
Windows on every run, so that is a checked claim.

What X11 adds is **placement**. That is worth being precise about, because it is
the difference between a card you notice and a card you go looking for:

- A card lands dead centre of the monitor your pointer is on. Measured on
  2026-08-11: `x=2505` on a 2560-wide screen starting at 1440, which is centred to
  the pixel. Without X11 the window manager centres it on whatever it considers
  the current screen, and vertically wherever it likes.
- A toast takes a slot in a managed top-centre column, so two of them and the
  hands-off strip do not stack on top of each other.
- Both appear **above without taking your keystrokes**. That one is X11's
  `_NET_WM_USER_TIME = 0` trick, and it is the single behaviour with no guaranteed
  equivalent anywhere else. On another desktop the card takes focus when it
  appears.
- The drop-down panel rolls from the top edge of the right screen at the right
  size. Elsewhere it appears at full height wherever the WM puts it.

None of that loses a message. All of it is why GNOME on X11 is the desktop
AgentBox is developed against, and the one it looks right on.

> [!IMPORTANT]
> **Two capabilities are genuinely X11-only, and each says so rather than
> pretending.** The three **global hotkeys**, because a grab needs a display server
> that lets a client claim a key for the whole desktop, which Wayland refuses by
> design. And **driving the desktop** - moving the pointer and typing - because
> the events have to be indistinguishable from a hand for an application not to
> treat them specially, and that is what X11's XTEST is for. `agentbox summon` is
> in the same family: it is a `_NET_ACTIVE_WINDOW` message and has no other way to
> exist. On a session without them, everything else works and those three report
> that they cannot.

> [!NOTE]
> **The macOS build has never been run.** Every package except the two that link a
> native UI is compiled for macOS on every check, and the UI toolkit supports the
> platform, so the honest claim is "no platform-locked call is left and the toolkit
> is there" - not "somebody has used it". The same goes for Windows, where the
> whole tree compiles but nobody has started it. If you run either, the thing worth
> reporting is whether the surfaces appear at all.

> [!WARNING]
> **On Windows the socket has one lock instead of two.** Everywhere else AgentBox
> checks both that the socket is reachable only by you and that the process which
> connected is you. Windows unix sockets carry no credentials and offer no call
> that asks, so there the first check is all there is: a directory only your
> account can open. It is the same first line of defence every platform has, and it
> is the one place a security property is thinner. See
> [[Safe on a work machine?|is-it-safe]].

Speech is thinner off Linux, for a dull reason. The engine writes raw audio into a
player's pipe and macOS's own player will not read from one. Install sox and it
works. Do not, and speech is silent while everything else carries on.

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
