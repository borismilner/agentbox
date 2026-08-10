# Drawing the wiki's frames

`tools/wiki/draw.py` renders the wiki's frames from fixtures instead of
photographing a live desktop (FR99). This file is how a person runs it.

It is the sibling of `SHOTS.md`, which documents `shots.py`, the photographer.
Both still exist and the split is deliberate: see "which one to use" below.

## Running it

```bash
python3 tools/wiki/draw.py            # every frame, into docs/wiki/img/
python3 tools/wiki/draw.py s1         # just one
python3 tools/wiki/draw.py s1 --out /tmp/look   # somewhere else, to judge first
python3 tools/wiki/draw.py --keep-serving       # URLs + a live server, to iterate
```

It needs `frontend/node_modules` (it runs vite), `google-chrome` or `chromium`,
and `node`. A Go toolchain and `optipng` are both optional and it says so when it
skips them. It does not need a daemon, a desktop, an X display or a lock, and it
does not touch Boris's queue, config or state. Nothing on his screen changes
while it runs, which is the practical difference from `shots.py`.

`--keep-serving` is the one to use while writing a frame. It prints a URL per
frame and leaves the server up, so you edit `frames.js`, reload, and look -
vite hot-reloads the fixture like any other module.

## Where the parts are

| Path | What it is |
|---|---|
| `frontend/draw/frames.js` | the frames, as data. Editing a frame means editing this |
| `frontend/draw/md/` | the markdown and artifact sources the document frames show |
| `frontend/draw/rendered.js` | those sources as HTML, generated - never edited |
| `frontend/draw/runtime.js` | the stand-in for `@wailsio/runtime`: answers from the fixture instead of the daemon |
| `frontend/draw/index.html` | the surface's page, and the frozen clock |
| `frontend/draw/desk.html` | the desktop every frame sits on |
| `frontend/draw.config.js` | vite with the alias, serving only - it never writes a bundle |
| `tools/wiki/drawhtml/` | renders `md/` with the product's own markdown and artifact renderers |
| `tools/wiki/shoot.mjs` | drives the browser over CDP and captures one frame |
| `tools/wiki/draw.py` | build, serve, measure, capture |

Three of those are worth knowing about on their own, because each exists to close
a specific way a drawing can be wrong.

**`drawhtml`** turns `frontend/draw/md/*.md` into `rendered.js` by calling
`webui.RenderMarkdown` and `webui.RenderArtifact` - the same functions the daemon
calls. The viewer, the artifact and the console show *rendered* markdown, and a
fixture that hand-wrote that HTML would be drawing a renderer nobody ships.
`draw.py` runs it before serving, so a change to `mdhtml.go` shows up in the next
drawing rather than quietly making three frames wrong. Its output is committed so
a machine without Go can still draw; it just draws what the last person with Go
generated. A file named `*.artifact.*` goes through `RenderArtifact` instead and
takes its title and id from a first-line marker.

**`shoot.mjs`** is the capture. It speaks the DevTools protocol directly (Node
ships a WebSocket client, and adding puppeteer to the product's frontend so the
docs can take a picture is the worse trade), and its whole reason to exist is
that it WAITS: for `data-drawn`, for a frame's own readiness selector, and then
for every animation to settle. `AGENTBOX_CDP` is how `draw.py` hands it the
browser. Called with `-` instead of an output path it measures instead.

**`desk.html`** is the desktop. The surface goes in an iframe sized to exactly
the window Go opens, because the strip is `height: 100vh` and the card is
`min-height: 100%` - a viewport unit inside a plain div would mean the picture
rather than the window. The wallpaper and the blurred window behind are
deliberately not product and deliberately not any particular application.

Every frame uses it. Three have to - the two strips and the progress window are
ABOUT where they sit, and `place` puts them where the code does: `top` is
`window.toast_top_inset`, `corner` is `x11.corner`'s 28 and 52. The rest use
`centre`, which is where a window manager puts a window it just opened, and they
use it for two reasons. A frame cropped flush to its window reads on a page as a
flat rectangle beside one that has a desktop in it, which was Boris's complaint
on 2026-08-10 and the reason this stopped being a three-frame feature. And a card
or a toast is a TRANSPARENT frameless window sized to its own content
(`webui.go:483`), so the CSS shadow it carries is clipped by the window edge -
what actually draws a shadow under one is the desktop, and a frame with no
desktop in it had nowhere for it to fall.

The shadow and the corner rounding on the iframe are the compositor's, not the
product's. That is the line: this file may draw what a window manager draws
around a window, and nothing that belongs inside one.

## The four things that keep a drawing honest

**It is the shipped surface.** The only substitution in the whole chain is the
door to the daemon. The Svelte components, `app.css`, the token hashing and the
layout are the product's own, so a drawn frame is the product's markup around
content written for the page. If a card changes shape, redrawing shows it.

**The height comes from the product, not from the picture.** Every frameless
surface measures its own laid-out height and hands it to Go through
`bridge.fit()`; `draw.py` reads that number back and captures exactly it. So a
drawn frame is as tall as the window a real run would have opened.

The measuring pass has to lift the page's height constraint to get a true answer,
and that is not a detail. `app.css` sets `height: 100%` on `html`, `body` and
`#root`, which is right for a window and wrong for a probe: a surface that is
`min-height: 100%` then fills the 2000px measuring viewport and reports 2000 as
the height it needs. The progress window did exactly that. The toast is the
subtler version of the same failure - the old measuring pass read the height out
of a DOM dump before the last resize had landed, and under-reported by 22px.

**A redraw is byte-identical.** Drawing `s1` twice produces the same PNG to the
md5, because `index.html` freezes `Date.now()` before the bundle loads, the
fixtures compute their deadlines from that same instant, and `shoot.mjs` settles
every animation before it captures. Without the clock, every redraw is a new
picture (`expires in 1:56`, then `1:55`). Without the animation settling, a card
caught mid-drop is a different file every time. With both, a diff in
`docs/wiki/img/` means the product changed, which is the whole reason FR99 wanted
a file in the repo rather than a photograph.

**The rule FR99 set, which no code enforces:** a drawn frame may say nothing a
real run would contradict. Nothing checks this and nothing can. Three habits
stand in for it:

- Derive, do not invent. `waitingHues` in `s1` are computed with the product's
  own `identityHue()` for the agents named in the fiction, not picked as
  colours. A hand-picked shade is exactly the kind of wrong a photograph cannot
  be. The same goes for geometry: the corner inset in `s10` is `x11.corner`'s
  own 28 and 52, and the strip's 48px from the top edge is
  `window.toast_top_inset`.
- Pass an empty `Theme: {}`, so `app.css`'s defaults decide every colour. A
  fixture that sets tokens is a fixture painting a product that does not exist.
  The one field ever worth setting is a switch whose default you are matching -
  `s7` passes `artifactsEnabled: true` because that is what `config.go` ships.
- Let Go render anything that Go renders. That is what `drawhtml` is for.

## It is also a regression check, which was not the plan

Because a redraw is byte-identical and the frames are the SHIPPED surfaces, a
diff in `docs/wiki/img/` after a frontend change means the change moved
something on screen. That is worth more than it sounds: nothing else in this
repository renders a Svelte component to pixels, and jsdom cannot answer a
question about layout at all.

It earned this on the day it was built. R-05 added a mount-time pull to
`Card.svelte`, and redrawing `s1` produced the same PNG to the md5 - a real
browser mounting the real component through the changed path, with the fixture's
`View` answering nothing, which is precisely the "the pull returns nothing, the
push wins" case. It is not a substitute for the desktop (this is chrome, not
WebKitGTK, and no daemon is involved), but it is a cheap first answer to "did I
break the card", and it is available before anybody takes the desktop.

## Which one to use

**Draw by default.** Everything in `docs/wiki/DESIGN.md` section 5 is a staged
frame - the copy was written before the shot in every case - so the photography
was never buying evidence, only costing a desktop.

**Photograph when the point of the frame IS that this is real.** Three frames
qualify and all three are still photographs:

- `install-doctor.png`. It is a terminal transcript; drawing one is writing
  fiction in a monospace font.
- `history-stats.png`. Its entire argument is that the rows are this machine's
  real, unpruned history, rehearsal agents and all (DESIGN.md S12: "tidying this
  table would make it a nicer lie"). A drawn one would be the lie.
- `review-board.png`. Its spec asks for a real change from this repo, because a
  real diff needs no fiction. Drawing it would replace a real review of real code
  with a fabricated one, and the board's fixture would have to hand-write
  chroma-highlighted lines that `boardrender.go` produces today.

`README.md` uses six of the drawn frames rather than keeping a set of its own.
The set it had (`docs/img/*.png`) predated the rename - the window title bars in
it said `qq` - and is deleted.

`settings.md` keeps its `SHOT:` placeholder for a related reason: that frame's
subject is that the preview cannot drift from the result, which is Go's token
builder answering `previewTheme`. A fixture would have to hand-write the palette
it is claiming cannot be hand-written.

## Adding a frame

1. Write its entry in `frames.js`. Copy comes from `DESIGN.md` section 5,
   verbatim where that file quotes it.
2. Key `calls` by the **Go** method name (`Theme`, `Ready`, `Inbox`), because
   that is what `bridge.js` builds its FQN from. Lowercase silently answers
   `undefined`, which usually renders as an empty surface rather than an error.
3. Give it `out:` matching the file name the pages already reference, so the
   redrawn frame replaces its photograph rather than arriving beside it.
4. If the frame is about WHERE the window sits, give it a `desk`. If it needs
   more than a paint before it is finished - anything with an iframe in it - give
   it a `ready` selector.
5. `--keep-serving`, look at it, then run it for real and read the PNG.

A surface that never calls `fit()` cannot use `height: "fit"`; `draw.py` says so
by name rather than guessing, and the frame needs an explicit height.

## What drawing turned up

Worth reading before drawing the next one, because all of these are the kind of
thing only drawing finds.

**The specification was wrong about the product, and a photograph had already
proved it.** DESIGN.md said the shared-values block sits below the four agent
rows. `Agents.svelte:292` renders it above them and always has. The photographed
frame showed the real order and was accepted as a good frame, so the sentence
survived a sitting it contradicted. Drawing catches this because a fixture makes
you state what you expect before you see it.

**A page claimed a control the product hides.** `documents-and-artifacts.md` said
a preview and code toggle sits above every artifact, in the paragraph directly
above `agentbox show --artifact` - and `app.css:960` hides that toggle for
exactly the standalone case that command opens. The drawn frame has no toggle in
it because the product has none there. The page now says so.

**A window at its default height is not automatically a good frame.** The app
window opens at 1180x860 and four agents fill about 700 of it, so the honest
capture had 160px of empty board under the last row - which reads on a page as a
screen with nothing in it, the thing DESIGN.md warns about for the empty inbox.
`s2` declares 720 instead. That is a height a human could drag to and it hides
nothing, but it is a judgement, and a frame that needed one should say so in its
entry. `s7` needed the same call for the same reason.

**The old capture waited on the wrong clock.** `chrome --headless --screenshot
--virtual-time-budget=8000` fast-forwards the page's timers, not its CPU, so the
budget is spent in a few real milliseconds and anything still working when it
runs out is photographed unfinished. Every surface survived that. The artifact
did not: a sandboxed iframe holding half a megabyte of inline React came out an
empty stage in one run and a working canary console in the next, from the same
fixture. That is why `shoot.mjs` exists, and why a frame can declare what
"finished" means for it.

**An artifact that fails to run says nothing.** While chasing the above, a
fixture that rendered blank showed an `interactive` badge, a runtime label and an
empty stage, with the bar's error slot empty. Whatever the cause, a blank stage
and a working one look identical to a reader. Filed in `docs/backlog/ux.md`.
