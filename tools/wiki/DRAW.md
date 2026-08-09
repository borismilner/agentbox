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

It needs `frontend/node_modules` (it runs vite) and `google-chrome` or
`chromium` on PATH. It does not need a daemon, a desktop, an X display or a
lock, and it does not touch Boris's queue, config or state. Nothing on his
screen changes while it runs, which is the practical difference from `shots.py`.

`--keep-serving` is the one to use while writing a frame. It prints a URL per
frame and leaves the server up, so you edit `frames.js`, reload, and look -
vite hot-reloads the fixture like any other module.

## Where the parts are

| Path | What it is |
|---|---|
| `frontend/draw/frames.js` | the frames, as data. Editing a frame means editing this |
| `frontend/draw/runtime.js` | the stand-in for `@wailsio/runtime`: answers from the fixture instead of the daemon |
| `frontend/draw/index.html` | the desktop the surface sits on, and the frozen clock |
| `frontend/draw.config.js` | vite with the alias, serving only - it never writes a bundle |
| `tools/wiki/draw.py` | build, serve, measure, photograph |

## The two things that keep a drawing honest

**It is the shipped surface.** The only substitution in the whole chain is the
door to the daemon. The Svelte components, `app.css`, the token hashing and the
layout are the product's own, so a drawn frame is the product's markup around
content written for the page. If a card changes shape, redrawing shows it.

**The height comes from the product, not from the picture.** Every frameless
surface measures its own laid-out height and hands it to Go through
`bridge.fit()`; `draw.py` reads that number back and photographs exactly it. So
a drawn frame is as tall as the window a real run would have opened. The first
version guessed a viewport instead, and the card - which is `min-height: 100%` -
stretched down all 2000px of it.

**A redraw is byte-identical.** Drawing `s1` twice produces the same PNG to the
md5, because `index.html` freezes `Date.now()` before the bundle loads and the
fixtures compute their deadlines from that same instant. Without it every redraw
is a new picture (`expires in 1:56`, then `1:55`) and the diff is noise. With it,
a diff in `docs/wiki/img/` means the product changed, which is the whole reason
FR99 wanted a file in the repo rather than a photograph.

**The rule FR99 set, which no code enforces:** a drawn frame may say nothing a
real run would contradict. Nothing checks this and nothing can. Two habits stand
in for it:

- Derive, do not invent. `waitingHues` in `s1` are computed with the product's
  own `identityHue()` for the agents named in the fiction, not picked as
  colours. A hand-picked shade is exactly the kind of wrong a photograph cannot
  be.
- Pass an empty `Theme: {}`, so `app.css`'s defaults decide every colour. A
  fixture that sets tokens is a fixture painting a product that does not exist.

## Which one to use

**Draw by default.** Everything in `docs/wiki/DESIGN.md` section 5 is a staged
frame - the copy was written before the shot in every case - so the photography
was never buying evidence, only costing a desktop.

**Photograph when the point of the frame IS that this is real.** Two candidates,
and both should carry a caption saying so:

- `install-doctor.png` and anything else that is terminal output. It is a
  transcript; drawing one is writing fiction in a monospace font.
- Any frame added to answer "does it really look like that?" - a doubt raised
  about the product, settled with a camera.

Everything else is a drawing, and `shots.py` stays for those two and for the
day a surface changes enough that somebody wants to see it for real.

## Adding a frame

1. Write its entry in `frames.js`. Copy comes from `DESIGN.md` section 5,
   verbatim where that file quotes it.
2. Key `calls` by the **Go** method name (`Theme`, `Ready`, `Inbox`), because
   that is what `bridge.js` builds its FQN from. Lowercase silently answers
   `undefined`, which usually renders as an empty surface rather than an error.
3. Give it `out:` matching the file name the pages already reference, so the
   redrawn frame replaces its photograph rather than arriving beside it.
4. `--keep-serving`, look at it, then run it for real and read the PNG.

A surface that never calls `fit()` cannot use `height: "fit"`; `draw.py` says so
by name rather than guessing, and the frame needs an explicit height.

## What it does not do yet

Only `s1` (the card) is drawn. The other twelve frames in DESIGN.md section 5
are still the photographs from the 2026-08-07 sitting, and the four that sitting
never produced are still outstanding. Each is a `frames.js` entry and the
harness is the same; the app-shell frames (S2's agents board, the inbox, the
history stats) need their surface's mount-time `calls` filled in, which is more
fixture than the card needed but no new mechanism.
