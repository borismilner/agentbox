# The YouTube listing

> **Frozen on 2026-08-06.** The showcase was dropped for good. The uploaded take
> stays up as it is; these chapter timings will never be re-cut.

What was pasted into the upload form for
`agentbox-showcase-20260725-2344.mp4` on 2026-07-26 (the take was recorded under
the old brand; the file was renamed with the project, but the uploaded film
still wears the old name end to end). Keep it here so a re-record does
not start from a blank box, and so the chapter times can be regenerated rather than
guessed.

**The chapters below belong to that take and only that take.** Any re-record shifts
them. Regenerate from the take's own log, which prints a start offset per slide:

```bash
grep "^--- slide" <take-output>.log \
  | sed 's/.*--- //' \
  | awk '{gsub(/[()+s]/,"",$3); printf "slide %s  %d:%02d\n", $2, int($3/60), $3%60}'
```

The offsets are relative to the performance's own clock, and `perform.py --marks`
sets the in-point at the start of slide 1, so slide time and film time are the same
thing. The chapter titles below map one-to-one onto the 22 slides.

## Title

```
AgentBox: stop babysitting your AI agents
```

Alternates, if the benefit should lead: `Stop babysitting your AI agents (AgentBox, live
demo)`, or `AgentBox - one place every AI agent on your machine can reach you`.

## Description

```
An agent works for twenty minutes while you are somewhere else. When it needs one
word from you, it has nowhere to put the question, so it either stops and waits in
a terminal nobody is looking at, or it decides for itself and you find out later in
the diff. Either way you end up sitting next to it, which is the opposite of why you
started it.

AgentBox gives every agent on the machine one place to reach you: a card over whatever
you are doing, a sound that says what kind of thing arrived before you look, and an
answer that goes straight back to the code that is blocked on it.

One Go binary and a resident daemon. Fourteen tools over MCP, the same fourteen over
a CLI for shell scripts, hooks and cron. No cloud, no account, no telemetry, nothing
leaves the machine.

Everything in this video is live on one desktop. Every card is real, every answer
travels back through the socket to code that was blocked on it, and nothing is a
mock-up or a replay.

Chapters
0:00 What AgentBox is
0:38 The problem: an agent waiting on one word
1:22 One place every agent can reach you
1:54 What it buys you
2:32 One binary, fourteen tools, every project
2:55 Five levels, five sounds
4:00 Six shapes of asking
4:25 A decision in two seconds
5:42 Act unless stopped
6:11 A countdown, then one card for three questions
7:13 A long job without a notification per step
7:52 Can it be trusted with real work
8:14 A secret, and the off switch
9:32 Reviewing a diff before it ships
10:20 When the answer is not words
11:00 An interface the agent wrote
11:58 A report worth reading, and the record
13:56 Talking back to an agent
14:40 The panel: a live session
15:35 Where it is going
16:31 Installing it
17:00 The close

Install
  curl -fsSL <your-url>/bootstrap.sh | sh
  make install
  claude mcp add --scope user agentbox agentbox mcp

Contact: boris.milner@gmail.com
```

`<your-url>` is still a placeholder: there is no public bootstrap URL yet, and Boris
was told to substitute it before posting.

## Known gap in the uploaded take

7:13 narrates the progress bar and the film does not show one - the window was
underneath the fullscreen deck. See `recording.md`, "The progress bar nobody could
see". If the re-record fixes it, nothing in this listing changes except the chapter
times.
