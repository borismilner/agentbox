# Handoff - AgentBox: FR95 settled at the mock, three quarters built

*Written by session 51, which measured FR95's one load-bearing mechanic before
mocking it, had Boris settle four questions at the mock (he took every
recommendation), built three of the four, and found the fourth question itself.*

**Written:** 2026-08-06 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean; 8 commits ahead of origin unless Boris pushed
make deployed               # expect e0a54250a579 or later
agentbox control state      # expect "no run: the desktop is the human's", no quiet suffix
agentbox pending            # expect "nothing pending"
agentbox sync agents        # your row; ghosts from `claude -p` checks are harmless
agentbox sync locks         # expect "no locks held"
make check                  # gofmt + vet + race, ~2 min
```

**One thing is in flight and it is item 1: FR95's fourth answer.** Everything
else in FR95 is shipped, deployed and watched on screen.

### The queue, in order

1. **FR95's cards half - "demoting silences the column too".** Boris's fourth
   answer at the mock, and the only part not built. While the sign is demoted,
   **cards queue instead of appearing and drain the moment it goes loud**; the
   earcon still plays so he knows one arrived; nothing is lost (the inbox has
   them); an urgent card waits rather than interrupting. **Why it is last and
   biggest:** it is the first FR95 slice to touch the presentation path. The
   shape to copy is already there - the DND suppression in `d.present`
   (`internal/daemon/daemon.go` around the `suppressed`/`silentIdle` block, ~line
   1120) enqueues an item without making it current, and `breaksDndLocked`
   decides who gets through. **Recording mode is NOT DND**, though: DND holds the
   chime and this must keep it, so the two cannot simply be the same predicate.
   The daemon already knows the mode: `d.control.Quieted()` returns it with the
   fuse's remaining time, non-blocking. Test it beside the six FR95 tests at the
   end of `internal/daemon/control_test.go`.
2. **Give the coverage rule a validator.** Carried unchanged from sessions 49 and
   50, still the one open finding from the standard's trial. The standard demands
   a traversal accounting for every changed line and nothing checks it:
   `walkthrough.captured` logs `ranges/cited/missed` for citations and never
   compares the cited set against the diff's changed set. The spec holds both
   halves at validation time.
3. **A settings-surface knob for `quiet_hotkey`.** `pause_hotkey` has one under
   "Hands off" (`internal/webui/settings.go`); its neighbour now exists in the
   config and does not. Small and mechanical - the descriptor table already has a
   keys kind.
4. **Four assumed things about surfaces, still unseen.** Carried; each needs a
   specific staging, listed under "Facts" below.
5. **Consider giving `[editor]` a settings control.** Unchanged and still his
   call: the value is an argv array and the descriptor table has no kind for one,
   which is why `speech.command` has none either. The honest shape is a new kind.

## Where we are

FR95 is "get the hands-off strip out of a screen recording". Its shape came in
already settled, so this session did the three things that were left: measure the
mechanic it rests on, mock it, and build what he chose.

**Boris drove the mock and took all four recommendations:**

- **A hotkey plus the same verb in the shell.** `Ctrl+Alt+Q`, and
  `agentbox control quiet|loud` for the line above `obs` in a recording script.
- **Not persisted, and it expires.** Dies with the daemon and a 30-minute fuse
  takes it back to loud. A second press restarts the fuse ("still recording").
- **Colour carries the pause on four pixels.** Amber driving, green paused,
  accent asking - the marker already switched to accent for asking.
- **Cards queue while demoted.** The one not built; item 1 above.

The fourth question was the mock's own find and is worth keeping in mind while
building it: the strip is not the only thing AgentBox puts in the top-centre
column, so demoting the strip alone leaves FR94's three-minute nag in the shot.

## What was verified on the real desktop

Not read off the diff. Every capture is in the scratchpad listed under Live state.

- **Demoted, on screen:** the strip window gone, a `1920x4` amber line at `+0+0`,
  amber across the full width (sampled x=20 through x=1900), and visible over
  GNOME's own top bar.
- **A kiosk-fullscreen browser window covers it completely.** The whole centre
  column of the capture is the window's blue, top row included, and
  `wmctrl -lG` still lists `agentbox · hands off marker 1920x4` behind it. It
  reappears when the window goes. **This retires the session-50 `[assumed]`.**
- **Paused is `#4FB286` exactly** on the four pixels, which is `--k-success`.
- **`Ctrl+Alt+Q` fires**, tested the session-50 way (`agentbox drive "key
  ctrl+alt+q"`, because XTEST presses do trigger passive grabs): one press added
  `· quiet: the sign is demoted for 30m0s more` to `control state`, the next
  removed it.
- **Loud again puts the strip back top-centre**, after two demote/promote round
  trips: `620x62+650+48` per `xwininfo`, with its surface colour starting at pixel
  x=654.

> **The defect only the screen could find, for the third session running.**
> `x11.plain` was written to decline the notification type and
> `_NET_WM_STATE_ABOVE`, and `unlisted()` - the next line - added ABOVE straight
> back as a post-map client message, the one route Mutter honours. The code read
> exactly right and a fullscreen window still had four amber pixels on top of it.

> **And one bug that was not a bug.** `wmctrl -lG` reports doubled coordinates on
> this desktop: the strip at `+650+48` is listed as `1300 96`. It was read as a
> placement regression and disproved by `xwininfo` and the pixels, which agree
> with each other. The commit written against it was reworded to say what was
> actually observed. **Do not trust `wmctrl -lG` for geometry here.**

## The measurements, and the order to make them in

Three of four lied the first time, each with a plausible answer rather than an
error. All are in "Mechanics discovered" now, and they are the reason a probe is
worth writing carefully:

- **Mutter decorates a bare X11 window** - a 4px window comes back ~30px tall
  wearing a title bar, and the first probe measured that. `_MOTIF_WM_HINTS` with
  decorations 0.
- **`import -window root` cannot see a fullscreen window at all** (Mutter
  unredirects it), so the same probe read "covered" and "not covered" depending
  only on the tool. Use `gnome-screenshot -f`.
- **A pre-map `_NET_WM_STATE` is ignored** - FULLSCREEN as well as ABOVE. A test
  window that set it before mapping came up at `y=102`. `--kiosk` for Chrome
  (`--start-fullscreen` does nothing in `--app` mode) or
  `wmctrl -r NAME -b add,fullscreen` after the map.
- **AgentBox confounds its own measurement:** while an agent holds the desktop, a
  fullscreen window makes FR74 open ITS marker at `+0+0`. Measure a candidate
  somewhere else on screen, or release the desktop first.

The probe that answered all this is **not committed**; a copy is
`fr95-probe-main.go.txt` in the scratchpad below. It has to live under
`internal/` to resolve the module's xgb dependency.

## The mock, and how it was checked before he saw it

[docs/mocks/fr95-recording-mode.html](docs/mocks/fr95-recording-mode.html), and
it was **driven headless in Chrome over the DevTools protocol before it went on
his screen** - 35 assertions over every state, the settle path included. The
driver is `drive_mock.py` in the scratchpad and it is worth reusing:

- `google-chrome --headless=new --remote-debugging-port=N --user-data-dir=...`,
  then the websocket from `http://127.0.0.1:N/json`. **`websocket-client` must be
  given `suppress_origin=True`** or Chrome answers 403 with a message about
  `--remote-allow-origins`, which is the whole reason the first two runs failed.
- The two assertions that earned their keep were layout, not behaviour: no option
  title taller than three lines, and every badge on one line. A page can pass
  every behavioural check and read as a column of single words.
- **A class collision caused the worse of the two defects.** The recording-frame
  panel was `.rec` and the badge modifier is `.tag.rec`, so the panel's
  `width: 320px` landed on every RECOMMENDED badge. Renamed to `.recside`, with
  the reason in the stylesheet.

## Live state (volatile - verify on resume)

- **Deployed:** `e0a54250a579`, which is HEAD~1. The only commit after it is
  docs, so nothing is owed - but check `make deployed` against `git log` before
  assuming.
- **Git:** `main`, clean, **8 commits ahead of `origin/main`** at handoff and
  **not pushed** - Boris pushes `main` himself. Oldest first: `60b766d` `ab8e64f`
  `ed0a1f8` `1e98d79` `fd45c09` `9f49ec8` `e0a5425` `5764e68`, plus this
  handoff's own commit. Run `git status -sb`: that is the only truthful answer.
- **One commit was amended after its deploy.** `e0a5425` was `83e39fb` before its
  message was corrected, so a binary built at `83e39fb` was briefly deployed with
  a sha that is no longer in history. Redeployed at `e0a5425`; nothing to do.
- **Background jobs: none. PRs:** none, ever.
- **Nothing pending, no locks held, no strip on screen, nothing paused, nothing
  quiet.** All checked at the end. The desktop was taken and released four times
  this session and is not held now.
- **The FR95 mock is open on his screen** as artifact `a395825f7f3be`, with
  `watch=true` - editing the file re-runs it. His answers already arrived through
  the bridge (`decide`: hotkey, fuse, colour, queue) and are recorded in the FR95
  entry, so nothing depends on the window staying up.
- **A peer session shares this checkout:** `proc-752938`, detached, carrying the
  SessionStart placeholder purpose. It made no commits this session. Two agents in
  one checkout is how an unfinished doc once got swept into an unrelated commit,
  so no catch-all `git add`.
- **Captures and tools live in a session scratchpad** and will not survive a
  reboot:
  `/tmp/claude-1000/-home-boris-milner-me-projects-agentbox/6ee9c4a4-75cf-4353-83fe-18a0ee959971/scratchpad/`
  (`w1-demoted.png`, `x1-kiosk.png`, `x2-uncovered.png`, `x3-paused.png`,
  `y1-final.png` are the states above; `drive_mock.py`, `measure.py`,
  `fr95-probe-main.go.txt` are the tools). Deliberately not committed.
- **Usage at handoff:** session **81%**, resets 2026-08-06 18:39 Asia/Jerusalem
  (minutes away, so a cap here costs nothing); week (all models) **25%**, resets
  2026-08-12 05:00. This is a stopping point, not a rescue - item 1 was left
  because it is the biggest slice and the session budget was thin, not because
  anything blocks it.
- **In-flight edits: none.**

## Blocked on you (Boris)

Nothing - proceed autonomously. Three things stay yours and block nothing:

- **FR84's other half** (a long body still pushes a form's fields below the fold)
  needs your word, because the approach that fixes it is the one you did not
  pick. Mock: [docs/mocks/fr84-form-shapes.html](docs/mocks/fr84-form-shapes.html),
  approach C.
- **Your PostToolUse hook writes the raw Bash command as the activity line.**
  Carried from five handoffs, and this session watched the strip read
  `SP=/tmp/claude-1000/...` followed by four lines of shell while FR95 was being
  verified - the hands-off sign quoting a `convert` pipeline at you. It is your
  `~/.claude/settings.json`, so the wording is yours; the first line, or the first
  80 characters, would read better.
- **Whether the re-record is worth scheduling** (STATUS priority 2). Unchanged.

## I can do solo (no input needed)

1. **FR95's cards half** (queue item 1) - settled, specified, unbuilt.
2. **The coverage validator** (queue item 2).
3. **The `quiet_hotkey` settings knob** (queue item 3).
4. **The four unseen surfaces** (queue item 4).
5. **A JS test runner** (STATUS priority 6). `parseDiff` is a shared module with
   two callers and no test of its own.

## Facts - verified vs assumed

- [verified] **FR95's three built slices**, every bullet under "What was verified"
  above, on the real desktop.
- [verified] **The demoted marker is coverable by a fullscreen window.** This was
  session 50's `[assumed]` and it is now watched, but only after the ABOVE bug was
  found - the first attempt disproved it for the wrong reason.
- [verified] **A frameless 4px window at `+0+0` is visible over GNOME's top bar**
  with no window type and no ABOVE.
- [verified] **`make check` passes** (gofmt, vet, race) after every slice.
- [verified] The session-50 set, unchanged: FR94 end to end, Super combinations
  ungrabbable on GNOME/X11, and the session-49 set behind it.
- [assumed] **That the demoted marker behaves on a second monitor.** `demote`
  reads the strip's own monitor before closing its window and falls back to the
  pointer's screen when the mode is armed on an idle desktop. One monitor here, so
  the fallback is all that has ever run.
- [assumed] **That the 30-minute fuse fires in a live daemon.** The timer is
  driven directly in the test and `Loud("the 30 minute fuse")` is what it calls;
  nobody has watched half an hour pass. Cheap to check with a shortened constant
  if it ever matters.
- [assumed] The session-50 carried set, unchanged: that the *"This session has
  left the board"* wording ever paints, that `speak` and `diff` read back on
  screen, that an agent row's Recent items block paints, that the demo fallback
  paints, that the domain drawer animation is what Boris wants, and the older
  list behind those (the inbox detail's `found: false` path, the keyboard route to
  a resolved row's detail, a detail holding a mermaid diagram, `provisionalFor`
  retiring a hook-only row, the 200-key prefix cap, `@me` in a shared key, a real
  client's `await_signal` parked past 20s, the holder-parked-on-ask_user and 600s
  long-wait lock warnings, `webui-demo agents` still rendering, and the identity
  cross-check's no-node skip path).

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| Session 50's queue item 1 (FR95, "mock it before building it") | Done. The mock is [docs/mocks/fr95-recording-mode.html](docs/mocks/fr95-recording-mode.html), the four decisions and Boris's answers are in the FR95 entry, and the build is session 51 of [docs/history.md](docs/history.md) |
| Session 50's whole FR94 build narrative and its "hour it cost" section | All still true and all in session 50 of [docs/history.md](docs/history.md). The Super-grab finding is permanent in the field-requests doc under "Mechanics discovered" |
| Session 50's four strip-state captures and their scratchpad path | Gone with the session that took them; this session's captures are listed fresh in Live state |
| The FR94 mock reopen instructions | Nothing depends on it: the file is committed and every decision it settled is in the FR94 entry |

## Map

1. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers used in
   commits and handoffs. **FR95 is the only open one and three quarters of it is
   built**; its entry carries Boris's four answers, the measurements, and what is
   left. "Mechanics discovered" gained three traps this session.
2. [docs/STATUS.md](docs/STATUS.md) - current state. Priority 1c is FR95.
3. [docs/history.md](docs/history.md) - session by session; this session is
   "Fifty-first".
4. [docs/06-configuration.md](docs/06-configuration.md) - `[control]`, now with
   `quiet_hotkey` beside `pause_hotkey`.
5. [docs/agent-manual.md](docs/agent-manual.md) - "He can pause you, and you
   wait" under "Taking the desktop". **It says nothing about recording mode yet**,
   and probably should not: nothing changes for an agent, the sign is simply
   smaller. Worth one line if item 1 lands, because a queued card DOES change what
   an agent can expect.
6. [CLAUDE.md](CLAUDE.md) - traps that have cost sessions. **Read it before
   touching the build, the daemon, or driving the desktop.**
7. [docs/09-sync.md](docs/09-sync.md) - FR83, all five slices, the chip vocabulary.
