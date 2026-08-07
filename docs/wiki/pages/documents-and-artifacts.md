# Reports you can read, and interfaces you can use

> **In short.** An agent can open a document in a real reading window, with tables
> that have columns, code that is highlighted, maths that is typeset and diagrams
> that are drawn. Or it can write a small interactive page and block until you use
> it. Neither one can reach the network.
>
> **Read on if** your agents produce things a terminal cannot show. **Skip to**
> [[Reviews you walk|review-board]] for the version of this built around a code
> change.

## A report that a terminal would have flattened

`agentbox show FILE` opens one window with the document in it, on a fixed reading
measure, not stretched to whatever width the window happens to be. Tables
get real columns. Code gets per-language highlighting done before it reaches the
screen, with line numbers past ten lines and a copy button on every block. Alerts,
task lists, footnotes that jump: all of it, because half-rendered markdown is worse
than none.

It behaves like a reading application rather than a card. <kbd>j</kbd> and
<kbd>k</kbd> scroll, <kbd>g</kbd> and <kbd>G</kbd> go to the ends, <kbd>/</kbd>
finds with a live `3/17` count, <kbd>Ctrl</kbd> + <kbd>=</kbd> and
<kbd>Ctrl</kbd> + <kbd>-</kbd> zoom between 70% and 180%, and <kbd>q</kbd> closes
it. The footer keeps the path and the zoom level in view and names those keys, so
nobody has to be told twice.

```sh
agentbox show --watch notes/rollout-plan.md
```

That is the one worth trying first. With `--watch` the window re-renders when the
file changes on disk, keeps your scroll position and re-marks whatever you were
searching for, so an agent writing a document turns into a live preview of it. A
`watching` badge in the title bar says the loop is running.

## Maths and diagrams are drawn here, never fetched

A formula is typeset, in all four spellings an agent might reach for plus an
explicit `math` fence. This page renders the same expression the same way, which is
the only honest way to show it:

$$
\text{error budget} = (1 - 0.999) \times 30 \times 24 \times 60 = 43.2 \text{ minutes}
$$

A `chart` fence becomes a line, bar or scatter plot drawn as vector graphics in the
window's own palette. A `mermaid` fence becomes a diagram. An image gets read off
your disk and inlined, at up to 2 MB, and it must really be a PNG, JPEG, GIF or
WebP: the bytes are sniffed and the extension is ignored.

SVG is the one picture format refused on purpose. A vector drawing from an agent
is a `chart` or a `mermaid` fence, both of which AgentBox draws itself from a
description it can read.

## No network at all, and it was the markup that got bound

The decision underneath both halves of this page is about where a rule is
enforced. An agent can be asked not to do something, and an agent can be prevented
from doing it, and only the second one is worth writing down.

Take the ordinary case first. An image in markdown is a line of punctuation and a
path, and a renderer left to itself turns that into a live request to whatever host
the path named, made by your machine, on the agent's behalf, carrying whatever the
agent chose to put in the path.

So the renderer does not get to do that. A local file is read by Go and handed to
the window as bytes, and a path with any scheme on it, or a protocol-relative `//`,
is never fetched. It renders as a marked placeholder carrying the alt text and the
reason, `remote image not loaded`, because a reader should know something was meant
to be there.

An artifact gets the same treatment one level up. It runs inside a sandbox with no
same-origin access and an opaque origin, its content policy starts at
`default-src 'none'` and includes `connect-src 'none'`, and its permissions
attribute is empty, so there is no camera, no microphone and no location. React
and Tailwind are bundled into the binary instead of pulled from a CDN. A script
tag pointing at a host is stripped, and the bar above the artifact says so by
name: `not loaded (the sandbox has no network; react and tailwind are bundled)`.

The rejected alternative is the one that looks like more freedom: let the markup
through, and review what agents write.

That moves the burden onto the person reading, once per document, forever.

## An artifact is something you operate, and the agent waits on it

Some answers are not a number in a text field. They are a shape, a threshold, a
share of traffic. Nine numbered options cannot carry that, so an agent writes a
small page and blocks until you have used it.

![An artifact running in its own window: a canary console titled How much traffic should the new build take, a slider at 50% of live traffic with 2,100 req/min on the new build under it, and two buttons reading Start the rollout and Hold it. The bar above carries an interactive badge and the runtime it asked for, react + tailwind](img/artifact.png)

Above every artifact is a bar: an `interactive` badge, which runtime it asked for,
a **preview and code toggle**, and a reload button. The code tab is the source
that is running, character for character. Reading what an agent wants to run
before you run it is not a power-user feature here, it is the point of having the
toggle at all.

```sh
agentbox show --artifact canary-console.html
```

What you do in it leaves through one named channel and nothing else. The agent
either parks on the next event or drains what has accumulated, and a dragged
slider arrives as one value, not forty: events coalesce per control, so
what the agent reads is what every control was last set to. A theme change
repaints a running artifact instead of reloading it, so whatever you had typed
into it survives.

`[artifact] enabled = false` is a trust switch rather than a feature flag. With it
off, an artifact is source you can read and nothing in it ever runs, including the
ones already on screen.

## Where this stops

Find will not look inside a chart or a diagram, and that is a decision rather than
a gap: those are vector drawings, and wrapping a word inside one deletes it from
the picture. A legend reading `es` after a search for `vetoes` is what the other
choice looks like.

Inside a running artifact, find works by asking the artifact to search itself and
report a count. Those hits go stale if the artifact redraws, and are rebuilt on
your next keystroke.

An artifact larger than 96 kB is not run at all. The bar says so with the size,
and the source is right there above it. And only React, its DOM package and
Tailwind are bundled, so anything else an artifact tries to import is refused and
named instead of silently missing.

**Next:** [[a change you walk one step at a time|review-board]], or
[[what an agent is allowed to do|what-agents-can-do]].
