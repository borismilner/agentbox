# A sound that says how much to care, and a sentence that says why

> **In short.** Five levels, six short sounds, and one spoken line the agent
> writes itself. Info and success take themselves off the screen after six
> seconds; a warning, an error or anything urgent waits until you have read it.
> A twenty-minute job reports into its own window and never takes a place in the
> queue.
>
> **Read on if** you want to know what reaches you when nothing needs answering.
> **Skip to** [[When it stays quiet|staying-quiet]] for the off switches.

## The sound arrives before the words do

A notice that needs no answer does not get the middle of the screen.

It gets a strip at the top centre, wearing its level: a coloured bar, a tint, and one glyph
per level (a circled i, a check, a warning triangle, a crossed circle, a bell).
You know what kind of thing this is before you have read the title.

<!-- SHOT: a warning-level strip from dependency-bot on checkout-api, top centre of the screen, title reading that two transitive dependencies moved to a yanked version, no countdown in it -->

The title always shows whole; the body wraps to three lines and then offers to
expand in place. One arrives at a time, and a second waits its turn instead of
stacking.

Urgent is the carve-out. It skips the strip for a full card, because something
worth marking urgent must not be able to slide away unread while you are looking
elsewhere.

## Six sounds for five levels

The sixth sound is not a sixth level. Whether a thing blocks you picks the sound as
much as its level does, so a question gets its own rising chime whatever level it
carries, and that chime is the one sound that means somebody is parked waiting on
you.

| The sound | What it means |
|---|---|
| an insistent three-note figure | urgent, and it will play again |
| a rising two-note chime | something is blocked on your answer |
| a short tick | success |
| a two-tone fall | warning |
| a low thud | error |
| a soft pop | everything else |

<sub>Read it top down: the first row that matches wins, which is why an urgent
question sounds urgent rather than sounding like a question.</sub>

Urgent is also the only level allowed to interrupt a sound already playing. Anything
else arriving mid-chime is dropped, not queued, because two earcons in a row
say less than one.

## An earcon cannot tell you what happened

A sound says something happened and roughly how much it matters. It cannot say what
it was. So an agent may attach one spoken line to anything it puts on screen, read
out after the chime, and the agent writes that line itself.

```sh
agentbox notify --level warning \
  --title "Two transitive dependencies moved to a yanked version" \
  --speak "two dependencies were yanked, so the audit is paused until you decide"
```

Reading the title aloud was the obvious alternative and it was rejected twice over.
A title is written to be taken in at a glance, which makes it poor speech. And every
`agentbox notify` already sitting in somebody's scripts would have started talking
on the day it shipped. So there is no heuristic: an item speaks if it carries a line
and is silent if it does not, which is the default.

The line rides on the item, so it inherits every gate the chime has. Quiet hours
silence it at every level including urgent, on the grounds that a chirp at 3am and a
voice at 3am are not the same event, while the card still arrives.

## Which notices leave on their own, and which wait to be read

Only a notify at info or success arms a dismiss timer, and the default is six
seconds. A warning or an error arms nothing: a notice with no deadline was worth
sending, so it stays until you deal with it. <kbd>Esc</kbd> is how, and on a
notification that key means dismiss rather than defer, which
[[a question on screen|the-card]] explains at more length than it should have
needed.

Whatever is on screen escalates if it is blocking you or carries urgent, so an
urgent notice replays even though nobody has to answer it. Every 60 seconds, or
every 20 at urgent, five times, with the spoken line on each replay.

Then it stops, and the card stays exactly where it is.

A question at an empty desk gives up sounding like a metronome without giving up
waiting.

## A long job gets a bar in the corner, not a place in the queue

A reindex that takes twenty minutes is not a question and must never wait behind
one, so progress reports get their own window in the bottom-right corner. It opens
on the first report, grows as more tasks join it, closes when the last one finishes,
and is never in the queue or the inbox. The middle of the screen belongs to whatever
is asking you something.

It maps without taking your keyboard, and its close button is labelled for what it
does: hide, while the tasks keep running.

What reaches you is the finish. A completed task turns into one toast, at success or
at error, which is the only part of a long job that was ever worth a sound.

## When it makes no sound at all

The six earcons ship inside the binary, so nothing is fetched and nothing is
missing. Playing them is somebody else's job: AgentBox tries `pw-play`, `paplay`
and `aplay` in that order, and with none of them installed it switches sound off and
says so once in the log instead of failing. Speech stays off until you point it at a
voice. There are no per-agent or per-level overrides either; they were specified,
then dropped as redundant once the identity colour and the six classes turned out to
be doing that work already.

One mechanism is worth knowing because it looks like a bug. A dismiss timer is armed
in the daemon rather than in the window it closes, which is what makes six seconds
mean six seconds while you are away from the desk. It is also why hovering a strip
does not buy you more of them.

**Next:** [[where a question goes when you were not there for it|nothing-gets-lost]],
or [[every way to make it shut up|staying-quiet]].
