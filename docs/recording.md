# The video recorder

> **Frozen on 2026-08-06.** The showcase was dropped for good, so these scripts
> are no longer run or maintained. Kept for the record.

Four scripts, each with one job. `tools/showcase/record.sh` runs the camera;
`tools/showcase/perform.py` runs the show (and rehearses it); `tools/showcase/verify.sh`
says whether the finished file is usable; `tools/showcase/take.sh` is all three in one
process, gated on a click. The camera and the show know nothing about each other except
through two marks.

## record.sh - the camera

```bash
tools/showcase/record.sh check          # what would break, before anything changes
tools/showcase/record.sh prepare        # clear the frame, release the fullscreen hold
tools/showcase/record.sh start [--out FILE]
tools/showcase/record.sh mark in|out    # the performance sets these itself
tools/showcase/record.sh park           # pointer somewhere it hides nothing
tools/showcase/record.sh stop           # cut to the marks, normalise, restore, report
tools/showcase/record.sh abort          # stop, restore, keep the raw file
tools/showcase/record.sh status
```

**The desk it records on.** Two monitors make one 3000x1920 X root: `DP-1-2`
(1080x1920, portrait, primary) at +0+0 and `DP-1-3` (1920x1080, the recorded
one) at +1080+0. One trap here costs takes and screenshots alike: **`wmctrl
-lG` reports doubled coordinates on this desk** - a window at x=1080 reads as
2160 - so never crop or click from its numbers. `xwininfo`, `wmctrl -e` and
`agentbox drive where` are all correct.

**What it captures.** One monitor, the widest by default, at 1920x1080 and 60 fps,
encoded on the iGPU with `h264_vaapi` so the CPU stays free for the deck, the speech
engine and the driving. Measured: 356 of 356 frames, no drops after the first, 0.99x
real time. Audio is the default sink's *monitor* source, which is the loopback of
what came out of the speakers, so there is no microphone anywhere in the graph.

**Volume is not a variable.** PipeWire's monitor tap is pre-volume: the same line
captured with the speakers at 10% and at 60% peaked at -7.6 and -8.6 dB, which is
line-to-line variation and not the knob. Set the speakers anywhere, mute them even.
The finished file is normalised to -14 LUFS, which is what YouTube plays back at.

**The edges are the point.** It records with pre-roll and post-roll and cuts the
finished file to the two marks, with a half-second fade at each end. The viewer's
first frame is the first slide, not a terminal.

**What it changes, and puts back.** Ordinary windows sitting in the capture region
are moved off it; `[presence] fullscreen_auto_dnd` is turned off, because AgentBox holds
every card while a fullscreen app has focus and the deck is a fullscreen app; the
pointer is parked. `stop` and `abort` both restore all three, and either is safe to
run twice.

## perform.py - the show

```bash
.venv-deck/bin/python tools/showcase/perform.py --dry-run
.venv-deck/bin/python tools/showcase/perform.py --park 1130,540 --marks
.venv-deck/bin/python tools/showcase/perform.py --from 7 --to 9   # a rehearsal
```

The narration is **not** in it. It lives in the deck's speaker notes, which
`tools/showcase/deck.py` generates, so the words on screen and the words in the air
are one artifact and cannot drift. A blank line in a note is a beat boundary. Each
beat is split into sentences and spoken with `agentbox say --wait`, which returns when
the audio has actually stopped - so the slide turns on the end of a sentence and
twelve minutes of narration stay in sync without one sleep tuned by hand.

What the file does hold is the other half: what happens on screen between the beats,
per slide, as the same commands `docs/showcase.md` gives a human presenter. It also
parks the pointer on the recorded monitor before the first slide, because every window
AgentBox opens is placed on the monitor the pointer is on.

The synthetic input is stagecraft and never the subject: it is how the cards get
answered with nobody in the frame, and the deck does not mention it.

## Rehearsing

```bash
.venv-deck/bin/python tools/showcase/perform.py --rehearse --park 1130,540
```

The take without the camera and without the voice. Every command runs, every window
opens, every click is driven and every card is checked; a narration beat becomes a
0.8s pause (`--beat-pause`). It ends with a per-slide timing table, the length the real
take will be, and a verdict - exit 1 and a list if anything failed.

It also enforces five things the film cannot tell you about until it is too late:

- **Every card that is answered actually got answered.** A `submit` whose click and
  whose Return both leave the card on screen is a failure, not a note: the next
  `clear` waves the question away while the voice says it was answered.
- **Every window that is closed actually closed** (`close_window`, with the window
  manager as the fallback when a click misses).

- **The deck is still fullscreen** at the start of every slide and after every input
  step (`deck_is_fullscreen`, matched against the recorded region). The run stops the
  moment it is not.
- **The plan is safe to drive** (`lint_plan`): no `scroll` without a `move` in front of
  it, no coordinates without a `window` or `screen` frame.
- **The keyboard is on a Latin group** before anything types.

`take.sh` runs it before it rolls and refuses to record if it fails. Skip it only with
`take.sh --no-rehearse`, and only if you have just rehearsed.

## verify.sh - the film

```bash
tools/showcase/verify.sh ~/Videos/agentbox-showcase-20260725-2233.mp4 [seconds]
```

The performance can report success and the film can still be unusable, because the
stage is what the camera saw and not what the script believed. This samples the bottom
44 px of the frame every ten seconds and prints every moment the deck was not
fullscreen. That strip is the whole test: a slide fills it with the deck's near-black
background (mean brightness ~10 of 255) and the desktop taskbar fills it with icons and
white text (~38), so one number per sample decides it. Threshold 20, exit 1 if anything
is flagged. Twenty seconds instead of watching seventeen minutes.

`take.sh` runs it on the file it just cut, and notifies either way.

## Running a take

```bash
tools/showcase/take.sh          # rehearsal, then the take, then the verification
```

The two marks have to bracket the performance with nothing else in between, so a take
is one process from the confirmation to the finished file:

1. `agentbox confirm` - a card, and nothing is touched until it is clicked. A take costs
   twenty-five minutes of somebody's screen; starting one while they are typing wastes both.
2. `record.sh prepare`, then the deck into its slideshow, **verified to be fullscreen
   on the monitor being recorded** - not merely fullscreen somewhere, which is how one
   take recorded an empty desktop while the slideshow ran on the other screen.
3. `record.sh start`, then `perform.py --marks`, then `record.sh stop`.

Run it from a terminal on the *other* monitor. Anything on the recorded one that is
not the deck ends up in the pre-roll, which is cut, but a window that raises itself
mid-take is in the film.

### The two arguments it passes on

`--park X,Y` is where the pointer waits between beats: `record.sh park` prints the
numbers it computes for the recorded monitor (its left edge plus 50, half its height),
and `take.sh` passes the same pair to `perform.py`. On this desk that is `1130,540`.

`--marks` is what makes `perform.py` set the in and out points itself. Without it the
marks have to be set from outside, and whatever happens between those two commands -
an agent thinking, a human reading - lands inside the video.

### Editing the narration

The twenty-two blocks are the `d.notes(...)` calls in `tools/showcase/deck.py` and the
third argument of each `d.handson(...)`. To rewrite them in bulk, locate them with
`ast` - walk for `Call` nodes whose `func.attr` is `notes` or `handson`, take
`args[0]` or `args[2]`, and replace by span from the last to the first so earlier
offsets stay valid. A regex over triple-quoted strings does not work: it matches the
module docstring's closing delimiter and shifts every block by one.

**A blank line in a note is a beat, and the plan places every beat.** `perform.py`
checks the two against each other at startup and warns per slide, because a beat the
deck does not have is silence where a sentence was meant to be, and a beat the plan
does not place arrives after the visuals it belongs to. Splitting a paragraph is how
you buy a pause for something to happen on screen.

## What has been recorded so far

| file | what it is |
|---|---|
| `~/Videos/agentbox-showcase-20260725-1936.mp4` | the wrong screen: prepare had moved the deck to the portrait monitor. Fixed since. |
| `~/Videos/agentbox-showcase-partial-13slides.mp4` | 8:32, thirteen slides, stopped by hand. The picture and the sound are right; the content is not finished. |
| `~/Videos/agentbox-showcase-20260725-2233.mp4` | 17:27, the rewritten pitch, all 22 slides performed. **Usable to 13:04 only**: at that point the slideshow closed itself and the rest of the film is the OnlyOffice editor. Three defects in one take, all fixed since - see below. |
| `~/Videos/agentbox-showcase-20260725-2344.mp4` | **17:36, the take that worked.** All 22 slides, rehearsal clean, `verify.sh` clean at 106 samples, narration -19.1 dB mean / -1.4 dB peak. Uploaded to YouTube on 2026-07-26. One element is missing from it: the progress bar (slide 11) ran for half a minute *underneath* the fullscreen deck, so the slide is on screen with nothing on it - see "The progress bar nobody could see". |

All four were recorded under the old brand and renamed with the project on
2026-07-26, so the deck *inside* each film - the uploaded one included - wears
the old name.

## Open, and known

- **The history tab shows whatever has been asking on this machine**, rehearsals
  included, so slide 17 puts `showcase`, `zsh` and `probe` on camera next to `claude`.
  Boris decided (2026-07-25) to leave it: the honest table is the point of the slide.
  `docs/showcase.md` keeps the prune for the day an audience needs it, as his call.
- **Slides 19 and 22 have never run inside a take.** The third take stopped at 13 and
  everything since has been rehearsed piecemeal. Slide 19 in particular spawns a real
  `claude` child in the panel.

### What the 22:33 take cost, and what was wrong

Seventeen minutes recorded, thirteen usable. Three defects, none of them visible while
it ran:

1. **The slideshow closed itself at 13:04.** Slide 17 scrolls the report window three
   times. The first scroll moved the pointer into that window; `run_slide` then parked
   the pointer (it parks after a run of input steps) which put it back over the
   slideshow; the second scroll had no `move` of its own, so nine wheel notches went to
   the slideshow, where one notch is one slide. The show advanced past its last slide
   and exited to the editor. Everything after that is the editor with slide 22 in it,
   because `key right` on a deck that is already at the end does nothing.
2. **The veto card was never detected.** Its step was `("run", "agentbox veto ... &")` and
   `sh()` used `capture_output=True`: run() waits for the pipe to close, so it waited
   out the whole twelve-second countdown. The card appeared and expired inside that one
   step, `wait_card` then found nothing, and the narration described a card the viewer
   could no longer see.
3. **The release tag typed as Hebrew.** `2026.7.3` reached the input card as
   `2026ץ7ץ3`: `internal/hand` plans a keystroke against group 1's keysyms, but the X
   server types the keycode in whatever group is *active*, and `us,il,us` had the second
   one selected.

Fixes: every scroll now moves into its window first and `lint_plan()` refuses one that
does not; `sh()` treats a trailing `&` as background; `record.sh prepare` sets and
restores GNOME's input source and `perform.py` re-asserts it before typing; and
`perform.py` stops the run the moment the deck is not fullscreen instead of filming the
rest. The remaining AgentBox-side fix is in `internal/hand`: lock the planned group while
typing rather than trusting the active one.

### The progress bar nobody could see (2026-07-26)

The 23:44 take is clean by every check the tooling has and still misses one element.
Slide 11 sells the progress bar; the bar ran the whole 27 seconds and **no frame of
the film contains it**. It was not on the wrong monitor and it was not held: it was
directly underneath the fullscreen slideshow.

`x11.raise` already explains the mechanism for cards. Mutter promotes a *focused
fullscreen* window into the same stacking layer that always-on-top windows live in,
and inside a layer the focused window wins. A card survives that because
`x11.settle` sets `_NET_WM_STATE_ABOVE` and restacks with `_NET_RESTACK_WINDOW`
after the map. The progress window never did: it was deliberately "an ordinary
window - in the taskbar, stackable, closable" and it declines focus
(`quiet` + `showNoActivate`), which is exactly the combination that maps it below a
fullscreen app.

**Boris's rule (2026-07-26): every surface AgentBox puts up must be top-most, otherwise
it will be missed.** `x11.above` is settle's stacking half without the taskbar half,
and `progress.go` calls it after the map. The window is still ordinary in every other
way - it takes no focus and keeps its taskbar entry.

Scope, checked frame by frame against this film rather than assumed: toasts (slide 6,
top right), every card, the artifact (slide 16), the report viewer and the app window
(slide 17) were all visible over the fullscreen deck. Cards and toasts get it from
`settle`; the viewer, artifact and app window get it only because they take focus when
they map - nothing sets ABOVE for them. If any of those is ever made focus-free, it
will vanish the same way.

### What the 23:44 take cost, and what was wrong

Nothing was thrown away, but three defects were found and fixed *before* the camera
rolled, and two of the three had been misdiagnosed in an earlier take:

1. **`take.sh` drove the wrong window.** It resolved the deck with
   `wmctrl -l | grep -i showcase | head -1`, and the terminal the take was started
   from had "showcase" in its title - an older window, so it matched
   first. The script moved *that terminal* onto the recorded monitor, raised it over
   the deck and clicked the play button into it. It now resolves by `WM_CLASS`
   (`DesktopEditors`), never by title.
2. **`close` steps never ran.** `INPUT_STEPS = ("drive", "submit", "close")` is
   tested before the `close` branch, so a close step - which carries
   `(title, script)` rather than `(script)` - had its *title* driven as a script and
   `close_window` was dead code. The report viewer and the app window stayed on
   screen from slide 17 onward, and at slide 22 `clear_cards()` read the app window
   (titled with the bare product name, as every window is) as a card it could not
   dismiss. **This, not a missed click, is why the report window "survived its own
   close click" in the 22:33 take.**
3. **Do-not-disturb delivers what it held.** Slide 13 ends with `agentbox dnd off`,
   which releases the changelog notice it had been holding: the toast arrived over
   *slide 14*, and because any window called `agentbox` reads as a card, it also made
   slide 14's answered review card look like it was still on screen. Slide 13 now
   clears the released notice on its own slide.

Two smaller ones from the same evening: `take.sh`'s early exits left
`fullscreen_auto_dnd = false` in the config (the next `prepare` would then find the
knob already off, skip the backup, and no `stop` would ever restore it - every
failure path now goes through `bail`, which runs `record.sh abort`), and the
slideshow needed longer than the single 2.5 s wait to map, so a stage that was
perfectly fine was refused. It polls for 20 s now, and checks whether the deck is
already fullscreen *before* touching it.

### Closed since

- **The content pass** (2026-07-25): the deck is a sales pitch rather than a feature
  tour - rationale, then benefit, then proof; natural examples instead of a film
  reference; the progress bar, the report and the history tab all shown; a "what is
  next" slide so a stakeholder knows it is alive; and nothing addressed to the
  presenter left on a slide.
- **The self-driving is out of the pitch.** The pointer driving itself is how the demo
  gets recorded, not a feature to sell. The two reveal slides are gone and so is the
  opening claim; `perform.py` says so at the top of the file, so it does not come back.
- **Windows landed on the wrong monitor** (fixed 2026-07-25): the viewer, the artifact
  and the app window never asked to be placed, so Mutter chose the primary - the
  portrait screen on this desk - while the take recorded the wide one. They now call
  `UI.placeOn` after the map, like cards do through `bridge.resize`. Verified live: the
  artifact at +1590, the report at +1590, the app window at +1450, the progress bar in
  the wide monitor's bottom-right corner.
