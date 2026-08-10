# Stop babysitting your agents

> **In short.** AgentBox gives every AI agent on your machine one place to reach
> you: a card over whatever you are doing, a sound that says what kind of thing
> arrived before you look, and an answer that goes straight back to the code
> waiting on it. It exists so you can leave an agent running for an hour and
> still be the one who decides what matters.
>
> **Read on if** you are not yet convinced an agent can be left alone.
> **Skip to** [[Safe on a work machine?|is-it-safe]], or [[Install|install]].

## An agent with nowhere to put the question

An agent works for twenty minutes while you are somewhere else. Then it needs one
word from you.

It has nowhere to put the question, so it does one of two things, and both cost
you. It stops and waits in a terminal nobody is looking at. Or it decides for
itself, and you find out afterwards, in the diff.

That is the problem. Not the model, not the prompt. The last meter.

## This is what arrives

![A card from release-bot on project checkout-api asking where release 2026.7.30 should go first, with three numbered options and a countdown reading expires in 1:57](img/card.png)

<sub>Every option carries its number. The whole exchange is one keystroke.</sub>

<sub>Every option carries its number. The whole exchange is one keystroke.</sub>

The card appears dead centre of the monitor your pointer is on, above every
window, and it does not take your keyboard. That last part is the decision the
rest of the product is built around: a card that grabs focus while you are typing
can swallow a password, or let a stray keystroke approve a production deploy. So
your typing keeps going where it was going, and when you are ready you press the
summon key and answer with one digit.

Answers wait three seconds behind an undo strip before they are sent. A mis-click
costs nothing.

And when a second agent asks something while that card is up, the footer says so.
You are never told about one question and quietly left with two.

## Not everything is a question

Most of what an agent has to say is not a question, and interrupting you for all
of it would train you to ignore the one that matters.

So there are five levels and six short sounds, the sixth because a question gets
its own chime whatever level it carries. Info and success take themselves off the
screen after six seconds. Warnings, errors and anything urgent wait to be read,
because a notice with no deadline was worth sending. Urgent is the only level that
pierces do not disturb, and you can switch that off too.

An agent may also attach one spoken line to anything it puts on screen, read out
after the chime. The sound tells you roughly how much something matters. The
sentence tells you what it was, without your eyes leaving what you were doing.

## The hour you were not at your desk

Everything that arrived while you were gone is in the inbox. Still-pending
questions first, answerable right there with the same keys a card takes, then the
day's history: who asked, what for, and how long you took to answer.

Nothing truncates there, and that rule was bought the hard way. A countdown card
with a 946-character body expired unanswered while the desk was empty, and all
that survived was 140 characters in a tooltip. For a tool whose whole purpose is
that a message is not lost, losing the message is the wrong failure. Every row
now opens and reads its item back whole.

With several agents running there is a board that says which of them is asking
you something, which is stuck behind another one, which is deliberately waiting,
and which has gone quiet long enough to worry about.

## Can it go on a work machine

Three questions, and straight answers.

**Can it leak a secret?** A credential goes from your keyboard into a `0600` file
and the agent is handed a path, never the value. The log records that a secret was
asked for. It has never recorded one.

**Can agent-authored content phone home?** No. An interface an agent writes runs
with no network at all, and an image in agent prose may name a file on your disk
and nothing else. Both are enforced in the code that renders them, not by a
promise on a page.

**Can I switch it off?** Do not disturb, per-agent mute, quiet hours, and a gate
that holds everything while you are away or presenting. Urgent is the one level
that still gets through, and even that has a switch.

> [!IMPORTANT]
> AgentBox runs on Linux with X11, and that is not a soft requirement. Placing a
> card, detecting a fullscreen app, the global hotkeys, driving the desktop and
> the summon key all need X11. There is no Wayland, macOS or Windows build.

## What it is, underneath

One Go binary and a resident daemon that owns the windows. Thirty-nine tools over
MCP, which is how a coding agent reaches it, and a shell command for nearly all of
the same jobs, which is how hooks, Makefiles and cron reach the same daemon. A
unix socket with a peer check on every connection, no network listener of any
kind, no account, no telemetry, nothing leaving the machine.

Because a blocking question is also a shell command, a human answer is an exit
code:

```sh
if agentbox confirm --title "Push to main?" --body "12 commits, tests green"; then
    git push
fi
```

No parsing. `0` is yes, `1` is no, `3` is nobody answered.

**Next:** [[what a question actually looks like|the-card]], or straight to
[[Install|install]].
