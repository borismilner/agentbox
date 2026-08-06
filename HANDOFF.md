# Handoff - AgentBox: FR94 shipped, and one new field request took its place

*Written by session 50, which mocked FR94, had Boris settle it, built it in four
slices, found its first hotkey was dead on arrival, and took FR95 from him
mid-build.*

**Written:** 2026-08-06 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean; see Live state about ahead-of-origin
make deployed               # expect 9156e474f2a0 or later
agentbox control state      # expect "no run: the desktop is the human's"
agentbox pending            # expect "nothing pending"
agentbox sync agents        # your row; ghosts from `claude -p` checks are harmless
agentbox sync locks         # expect "no locks held"
make check                  # gofmt + vet + race, ~2 min
```

**Nothing is in flight.** One field request Boris has filed is open: **FR95**,
and it is item 1 below.

### The queue, in order

1. **FR95 - get the strip out of a screen recording.** Filed 2026-08-06 while
   FR94 was being built. Boris, verbatim: *"The hands off panel should also be
   hidable for cases when we need to record the screen and don't want it to be
   shown over the recording."* **Its shape is already settled** and that is the
   valuable part, so read the FR95 entry in
   [docs/07-field-requests.md](docs/07-field-requests.md) before anything else:
   the strip is **demoted, not hidden** - it drops to FR74's existing 4px
   top-edge marker - and, in his words, *"generally it should live on top of any
   and all surfaces; when demoted for purposes of recording or stuff like that it
   can be overlapped."* That second half is load-bearing: the demoted marker
   gives up the `keepOnTop` fight, so a window over the top edge simply covers
   it. **Mock it before building it.** Three questions are open in the entry:
   what turns it on, whether it survives a restart, and what four pixels say when
   the desktop is also paused.
2. **Give the coverage rule a validator.** Carried unchanged from session 49 and
   still the one open finding from the standard's trial. The standard demands a
   traversal accounting for every changed line, and nothing checks it:
   `walkthrough.captured` logs `ranges/cited/missed` for citations and never
   compares the cited set against the diff's changed set. The spec already holds
   both halves at validation time.
3. **Four assumed things about surfaces, still unseen.** Carried; each needs a
   specific staging, listed under "Facts" below.
4. **Consider giving `[editor]` a settings control.** Unchanged and still his
   call: the value is an argv array and the descriptor table in
   `internal/webui/settings.go` has no kind for one, which is why
   `speech.command` has none either. The honest shape is a new knob kind.

## Where we are

FR94 is **shipped, deployed and exercised on the real desktop** - the whole of
it, including the escalation. Boris can now take his keyboard and mouse back
mid-run with `Ctrl+Alt+Escape` or the strip's own Pause button, and hand them on
again, without the run ending.

The session's shape was: mock, settle, build in four slices, and one hour lost to
a hotkey that reported success and did nothing.

## FR94, as built

Everything below was settled at the mock by Boris himself, driving it. **Two of
his four answers went against the recommendation**, which is the argument for
mocking rather than speccing. Reasons live in the FR94 entry.

- **The inverted strip**, not the collapsed pill. Same 620x62 window, green,
  `PAUSED - YOURS`, the frozen activity line still readable in italic, a filled
  Resume button and a counter. At **2 min** it turns amber and reads
  `AGENT WAITING`; at **3 min** one card is raised with a Resume now button.
- **No auto-resume, ever.** Only he resumes. The agent's own wait ends at 10 min
  and it is told the desktop is still his and the run still its own.
- **An in-flight `drive_desktop` finishes its current step, except `type`, which
  parks between characters.** He chose "finish the step"; the `type` carve-out is
  the narrowing he accepted, because a 40-character type step at 300 wpm is
  seconds of typing into whatever he just switched to. `Type` releases Shift
  before parking - a latch held for minutes with a modifier down is the stuck
  keyboard this is supposed to avoid.
- **A desktop-wide latch**, not per-run. It parks the live run *and* holds off
  every other agent's `request_control`, and it is legal with no run at all (a
  pre-emptive "not now", which paints as `PAUSED - YOURS` / "nothing is driving"
  with an **Allow agents** button).

Where it lives: `internal/daemon/control.go` (the latch, `gate`, the nag timer),
`internal/hand/hand.go` (the `Park` interface, `Run` and `Type`),
`cmd/agentbox/daemon.go` (the `grab` helper, now carrying both hotkeys),
`frontend/src/surfaces/Control.svelte`, `[control] pause_hotkey` in the config
and in the settings surface under "Hands off".

## The hour it cost, so nobody spends it again

**`Super+Escape` was the first default and it never once fired.** The daemon
logged `hotkey.grabbed for=pause hotkey=Super+Escape` and the key was dead.

> **GNOME Shell takes every `Super` combination before a core X11 passive grab
> can see it, and does it silently.** `XGrabKey` returns success - no
> `BadAccess` - so every signal AgentBox has says the grab worked.

Measured, not guessed (the table is in "Mechanics discovered"): `Super+F9` and
`Super+P` swallowed; `Ctrl+Alt+Escape`, `Ctrl+Alt+P`, `Ctrl+Alt+space`,
`Ctrl+Alt+comma`, `Ctrl+Shift+Escape` all delivered. **No Super in any default or
suggestion.** The panel's `Ctrl+Alt+grave` was already in the right family.

Two techniques worth keeping, both in the same entry:

- **`agentbox drive "key ctrl+alt+grave"` is a real end-to-end hotkey test.**
  XTEST presses do trigger passive grabs, so firing a combination known to work
  through the identical path is what isolated the fault to Super.
- **A probe that imports `internal/...` must live under `internal/` itself.** A
  `main.go` in the scratchpad cannot compile against it at all.

## What was verified on the real desktop

Not read off the diff. Captures are in the scratchpad listed under Live state.

- **A parked drive.** `drive_desktop` sent through a fresh `agentbox mcp` child
  held **21.45s** with the pointer frozen at `1228,85`, then ran both steps on
  resume and landed on `900,640` exactly, returning `{"steps": 2}`.
- **The hotkey**, after the fix: one press paused a live run, the next resumed it.
- **Three strip states**, captured: amber `HANDS OFF` + Pause, green
  `PAUSED - YOURS` at `0s` with the italic frozen line, amber `AGENT WAITING` at
  `2m 12s`.
- **The idle latch**: `PAUSED - YOURS` / "nothing is driving; agents are held off
  until you release it" / **Allow agents**, and the strip comes down on resume.
- **The second agent.** A `request_control` from another identity blocked **23
  seconds** across the pause and was granted the instant of the resume.
- **The three-minute card**, watched arriving at exactly `paused_s: 180`: a
  warning toast, *"claude has been parked for 3m0s"*, with a **Resume now**
  button, under an amber `AGENT WAITING` strip. Its button was then clicked for
  real and the log shows the whole path -
  `action.started ... command: agentbox control resume` then
  `action.finished ... output: "resumed: agents may drive again"` - with the
  state back to `driving`. **"End the run" was deliberately not built** as a
  third button: the agent gives up on its own at ten minutes with its run
  intact, which is a gentler end than cutting it off mid-sequence, and an
  irreversible button on a card read in passing is the wrong shape for a state
  that resolves itself.

> **The defect only the screen could find.** At `2m 50s` with the run released,
> the strip read `AGENT WAITING` over the line "nothing is driving" - the
> escalation firing with nobody to escalate about. One line (`warm` now requires
> `!idle`), and it is the second time in three sessions that a surface passed
> every test and lied on screen.

> **A mock on his screen is HIS.** Driving the FR94 mock to check it worked while
> Boris was clicking in it cost twenty minutes of confusion - the page scrolled
> between screenshot and click, selections changed on their own, and one stray
> click emitted a decision he had not made. It was him, and he said so. CLAUDE.md
> already warns never to click at a coordinate read off an earlier screenshot;
> the missing half is that the moment a mock is in front of him, get out of it.

## Live state (volatile - verify on resume)

- **Deployed:** `9156e474f2a0`. Anything committed after it is docs; check
  `make deployed` against `git log` before assuming a deploy is owed.
- **Git:** `main`, clean, **pushed** (`1f193b4..b5d83c9` to
  `gitlab.com:fu-bar/agentbox.git`, on Boris's word at the end of the session).
  Fourteen commits, oldest first: `11778f1` `8b06691` `16d2e78` `eca7190`
  `808237f` `2c3584e` `4421863` `171ba0e` `254285b` `31da412` `12d27fe`
  `0a3d95e` `9156e47` `b5d83c9`, plus this handoff's own commit, which is
  written after the push and so is local until somebody pushes again.
- **Background jobs: none. PRs:** none, ever - Boris pushes `main`.
- **Nothing pending, no locks held, no strip on screen, nothing paused.** All
  four checked at the end (`agentbox control state` says "no run: the desktop is
  the human's"). The desktop was taken and released several times during the
  session and is not held now.
- **A second session was on this machine** at 16:56, `proc-722811`, purpose
  "finding why the same image build failures keep coming back and fixing them
  for good", area `repo:assignments`. Different repo, no shared files - but it
  did collide on one thing, which is worth knowing because it is not code:
  **`~/.claude/last-handoff.md` is a single global file two `/handoff` runs
  write within minutes of each other**, and it overwrote this assignment's
  pointer with its own. It was merged back by hand, under an
  `agentbox sync lock last-handoff`, with both entries kept and a note at the
  top saying the first line is whichever finished last. **So do not trust that
  pointer to name this assignment** - `/resume` from this directory finds the
  local `HANDOFF.md` first anyway, which is the route that cannot be raced.
- **The FR94 mock window was closed by a deploy.** Reopen it with
  `agentbox show --artifact docs/mocks/fr94-pause-resume.html` if wanted, but
  nothing depends on it: the file is committed and every decision it settled is
  written into the FR94 entry.
- **Captures live in a session scratchpad** and will not survive a reboot:
  `/tmp/claude-1000/-home-boris-milner-me-projects-agentbox/aea199b3-203e-4f55-930c-b3e14708b8e5/scratchpad/`
  (`crop-10-driving.png`, `crop-11-paused.png`, `crop-13-warm.png`,
  `crop-15.png` are the four strip states; `park.log` is the parked drive's
  timing). Deliberately not committed.
- **Usage at handoff:** session **62%**, resets 2026-08-06 18:40
  Asia/Jerusalem; week (all models) **23%**, resets 2026-08-12 05:00. Neither is
  near a trigger, so this handoff is a stopping point rather than a rescue.
- **In-flight edits: none.**

## Blocked on you (Boris)

Nothing - proceed autonomously. Three things stay yours and block nothing:

- **FR84's other half** (a long body still pushes a form's fields below the fold)
  needs your word, because the approach that fixes it is the one you did not
  pick. Mock: [docs/mocks/fr84-form-shapes.html](docs/mocks/fr84-form-shapes.html),
  approach C.
- **Your PostToolUse hook writes the raw Bash command as the activity line.**
  Carried from four handoffs now, and this session watched it on the hands-off
  strip itself: while FR94 was being verified the strip read
  `SP=/tmp/claude-1000/-home-boris-milner-me-projects-agentbox/a...` instead of
  what the agent was doing. It is your `~/.claude/settings.json`, so the wording
  is yours; the first line, or the first 80 characters, would read better.
- **Whether the re-record is worth scheduling** (STATUS priority 2). Unchanged.

## I can do solo (no input needed)

1. **Mock FR95** and put it in front of him - shape settled, three questions
   open, and the working rule says mock before build.
2. **The coverage validator** (queue item 2).
3. **The four unseen surfaces** (queue item 3).
4. **A JS test runner** (STATUS priority 6). `parseDiff` is a shared module with
   two callers and no test of its own.

## Facts - verified vs assumed

- [verified] **FR94 end to end**, every bullet under "What was verified" above.
- [verified] **Super combinations cannot be grabbed on GNOME/X11**, measured
  across seven combinations.
- [verified] **`make check` passes** (gofmt, vet, race) after every slice.
- [verified] The session-49 set, unchanged: the TL;DR control on both sides, the
  Agents board's Activity and Signals blocks, FR74's marker at `+0+0` and the
  fact that its strip does NOT step aside, and the editor ladder's four rungs.
- [assumed] **That the demoted marker in FR95 will actually be coverable.** The
  reasoning is sound - dropping `keepOnTop` and the notification type is exactly
  what makes a window sit above it - but nothing has been built or watched.
- [assumed] **That the *"This session has left the board"* wording ever paints.**
  It needs a row to vanish between the click and the reply; unseen for three
  sessions now.
- [assumed] **That `speak` and `diff` read back on screen.** Migration 0012's
  columns, insert, select and wire are all tested; no item raised since has been
  opened in the inbox detail. Raise an `agentbox review` with a diff, answer it,
  then open its row.
- [assumed] **That an agent row's Recent items block paints.** Activity and
  Signals were seen; Items needs a session that has raised something and then had
  its row opened. **This session raised one** (the FR94 nag card), so the staging
  now exists.
- [assumed] **That the demo fallback paints.** `Bridge.AgentDetail` falls back to
  the fixture when no daemon is behind the build; unit-tested, unreachable on
  screen while a real daemon owns the session bus.
- [assumed] **That the domain drawer animation is what Boris wants.** Unchanged.
- [assumed] The older carried-over set, unchanged: the inbox detail's
  `found: false` path, the keyboard route to a resolved row's detail, a detail
  holding a mermaid diagram, `provisionalFor` retiring a hook-only row after ten
  minutes, the 200-key prefix cap on the surface, `@me` in a shared key, a real
  client's `await_signal` parked past 20s, the holder-parked-on-ask_user and 600s
  long-wait lock warnings, `webui-demo agents` still rendering, and the identity
  cross-check's no-node skip path.

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| Session 49's queue item 1 (FR94, "mock it before building it") | Done. The mock is [docs/mocks/fr94-pause-resume.html](docs/mocks/fr94-pause-resume.html), the four decisions and their reasons are in the FR94 entry, and the build is session 50 of [docs/history.md](docs/history.md) |
| Session 49's whole "the experiment, and what it cost the standard" section, and the five defects | All still true and all in session 49 of [docs/history.md](docs/history.md). Only the one finding still open is carried, as queue item 2 |
| Session 49's FR74-marker and editor-ladder sections | Both closed and recorded: FR74's verdict is STATUS priority 1, the editor ladder is under FR65 in the field-requests doc |
| Session 49's "the row TOGGLES" trap and its capture paths | The trap is in session 49 of history.md; its scratchpad is gone. This session's captures are listed fresh in Live state |
| The rule-numbering warning | Now permanent in STATUS under the authoring-standard bullet, not something a handoff has to keep repeating |

## Map

1. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers used in
   commits and handoffs. **FR95 is the only open one**, and its shape is already
   settled inside the entry. FR94's entry carries the four decisions. "Mechanics
   discovered" gained the Super-grab finding and the two techniques that found it.
2. [docs/STATUS.md](docs/STATUS.md) - current state. Priority 1 is FR74's
   verdict, 1b is FR94 (shipped), 1c is FR95.
3. [docs/history.md](docs/history.md) - session by session; this session is
   "Fiftieth".
4. [docs/06-configuration.md](docs/06-configuration.md) - the new `[control]`
   section, with the Super warning next to it.
5. [docs/agent-manual.md](docs/agent-manual.md) - "He can pause you, and you
   wait" under "Taking the desktop". `internal/manual/agent.md` is the in-binary
   short form and has the same fact in one bullet.
6. [CLAUDE.md](CLAUDE.md) - traps that have cost sessions. **Read it before
   touching the build, the daemon, or driving the desktop.**
7. [docs/09-sync.md](docs/09-sync.md) - FR83, all five slices, the chip vocabulary.
