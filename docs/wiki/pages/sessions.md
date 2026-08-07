# An agent you can read while it works

> **In short.** AgentBox runs Claude Code sessions itself and renders each one as a
> conversation: live streaming, thinking folded away, tool calls one line each. A
> hotkey rolls the same sessions down over your work. A question from an agent in a
> session is answered inside that conversation, not in a card over it.
>
> **Read on if** you want the agent and the answer in the same window. **Skip to**
> [[Work it does on its own|assignments]] for the runs nobody starts.

<!-- SHOT: the console rolled down over an editor, one session chip selected, a turn mid-stream, and the inline question sitting above the composer -->

## A turn you can read, with the machinery folded away

A session is a real `claude` child, headless, driven over its event stream. What
you get is not a terminal: prose set to a reading measure, code highlighted with a
copy button, tables with columns, and tokens arriving as they are produced.

Every agent turn carries the time it arrived on a 24-hour clock and how long the
model worked before its first word, so a slow turn is legible as slow rather than
as broken.

The mechanics of a turn collapse so the conclusion gets the space. Thinking sits
behind one caret you can open. A tool call is a single line with the tool's name,
its argument highlighted when the argument is code, and a mark saying whether it
finished, failed, or is still going. That shape was arrived at by having it the
other way first: a reply that ran five commands showed five bordered pills stacked
down the column, each shouting as loudly as the answer.

<kbd>Enter</kbd> sends, <kbd>Shift</kbd> + <kbd>Enter</kbd> gives you a newline,
and <kbd>Esc</kbd> hands the keyboard back out of the composer.

> [!WARNING]
> Plan and Full are the two buttons at the top of a session, and Full is full: the
> child runs with its permission prompts bypassed, so it edits files and runs
> commands without stopping to ask you. Plan can look and cannot change anything.
> The mode is fixed when the process spawns, which is why switching starts a new
> child and carries the conversation across rather than changing its mind.

## Several at once, named by the agent's own first words

Sessions are a list with a dot on whichever ones are working, plus New. Each one
names itself: the heading the agent opens with, or its first sentence, cut down to
about 42 characters. Until it has said anything the prompt you typed stands in.

Asking the model for a title instead would be another round trip and another entry
in the transcript, for a worse answer than the sentence it already wrote.

Rename them if you want. Each is saved to disk as its own file with a markdown
sibling, so a conversation from Tuesday reopens on Friday and is searchable in
between.

## A console that rolls down over your work

<kbd>Ctrl</kbd> + <kbd>Alt</kbd> + the backtick key drops a session console from
the top edge of the monitor your pointer is on, over whatever you were doing, and
the same key rolls it up. So does <kbd>Esc</kbd>. It shows the sessions the app
window shows, which is the point: dropping it down mid-task lands you where you
already were, rather than in a second place your work might be.

This is the one AgentBox window that takes the keyboard. Nothing else does, and the
reason is the direction of the interruption: a card is AgentBox arriving at you and
must never steal a keystroke, while the console is you summoning AgentBox and you
are about to type in it.

It does not slide, and that was measured rather than assumed. The animation holds
38 frames in 222 milliseconds, about 170 a second, more than the display shows, and
it still does not read as sliding: this window grows its own height, so every frame
is a re-composite rather than a texture moving, and the eye reads a growing
rectangle. The roll is off by default and there is a setting for whoever disagrees.

## The question renders where the answer goes

When the agent asking is one running in a session, the question does not get a
window. It appears in that conversation, directly above the composer, wearing the
severity bar a card would have worn.

The reason is what a card would cover. The thing the question is about is on screen
right behind it: the file it read a moment ago, the command it ran, the plan it
wants a word on. A window over the middle of the screen hides the one thing you
needed to answer it.

It still never takes the keyboard, which is the rule [[a card|the-card]] is built
around and this surface inherits. The composer keeps focus, the single-key answers
work only while you are not typing, and the hint line says which of those two you
are currently in.

For a while there was a hole in this, and it was the worst possible hole. A host
meant the app window only, so a question asked while the drop-down console was down
opened a card over the very conversation it belonged to. Since 2026-08-06 the
console is a host too. With both open the question renders in both, and answering
either one resolves it.

Losing the host is handled the other way round: close the window with a question
still pending and it comes back as a card, because an unanswered question must not
leave with the surface it was rendered on. It does not work in reverse, on purpose.
Opening a session window does not pull a card that is already on your screen back
inline, because a question you can see is not a question that needs moving.

## What still arrives as a card

Only three kinds go inline: a choice, a confirm, and a notice. Anything that needs
a text field, a secret, a form, a diff or a countdown gets the full card, and so
does anything at urgent level whatever kind it is. A masked credential field
belongs on a surface built for it, and urgent means the screen, not the window you
might have closed.

A session is Claude Code specifically. It is spawned as a headless child with
AgentBox's own tools wired in, which is what lets a session ask you something in the
first place, and it is why a run of an [[assignment|assignments]] can be read here
like any other conversation.

One layout cost worth knowing, found by looking at a real long question rather than
at the diff. At a window 1000 pixels by 520, a long question left the reply box a
clipped sliver with no Send button, so it could be read and not answered. The panel
yields in two stages now: its body absorbs the squeeze first, then the whole panel
scrolls as a unit, and the composer never shrinks at all.

**Next:** [[the work it starts without you|assignments]], or
[[everything a question can look like|the-card]].
