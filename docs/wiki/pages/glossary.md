# Thirteen words, in the shortest form that is still true

> **In short.** Every other page in this wiki uses these words without stopping to
> explain them, which is what keeps those pages short. If one of them did not land
> somewhere else, it is defined here in one line.
>
> **Read on if** a word somewhere else went past you. **Skip to**
> [[What it is|home]], or [[A question on screen|the-card]].

No jargon is defended here. Where a word has a plain-English equivalent, the plain
one is the definition and the technical one is the thing in brackets.

## The three words everything else rests on

| Word | What it means |
|---|---|
| agent | A program that works on its own, deciding its next step as it goes. On your machine that usually means a coding agent running in a terminal. |
| MCP | The convention a coding agent uses to reach tools outside itself. AgentBox is one of those tools, and registering it once is what lets every session reach you. |
| daemon | The one AgentBox process that stays running in the background and owns every window. Everything else is a short-lived command that talks to it. |

## What actually arrives on your screen

| Word | What it means |
|---|---|
| card | The window a question arrives in: dead centre of the monitor you are looking at, above every other window, and it never takes your keyboard. |
| item | Anything an agent put in your queue, question or not. A card is what an item looks like on screen; the inbox is where it is afterwards. |
| level | How much an item claims to matter: info, success, warning, error or urgent. It picks the colour, the sound, and whether the thing takes itself off your screen. |
| earcon | The short sound that plays before you look. There are six, so what kind of thing arrived is known before your eyes move. |

> [!NOTE]
> Level and blocking are different things, and confusing them is the commonest
> mistake on this vocabulary. A question at `info` still gets a full card and still
> waits for you, because whether something blocks is a property of the call rather
> than of its level. Urgent is the only level that changes the rules by itself.

## Words that only matter with several agents running

| Word | What it means |
|---|---|
| area | Which piece of work a session is in, usually the repository it started in. It is how AgentBox knows which agents are in a position to collide with each other. |
| session key | The private id of one running session. It is why two agents that both call themselves `claude` are still two separate rows on your board. |
| lock | A named claim on one shared thing: the deploy, a repository, a machine. One agent holds it, the others queue, and you stop being the one who sequences them. |
| signal | A note one agent posts and others wait for. Waiting for a peer this way costs one parked call rather than a check every five minutes. |

## Two things an agent can put in front of you

| Word | What it means |
|---|---|
| artifact | A small interface an agent wrote for you to use rather than read: a slider, a form, a control panel. It runs with no network at all. |
| walkthrough | A change laid out as steps you walk at your own pace on the review board, and hand back in one go instead of a dozen turns. |

**Next:** [[what a question looks like when it arrives|the-card]], or
[[the limits worth knowing before you install it|limits]].
