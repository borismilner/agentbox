# Handoff - AgentBox: the wiki's pictures are drawings, and they are live

*Written by session 60, which took one assignment from Boris - create the wiki's
examples instead of capturing them, fix the pages, push them - and finished it.
Twelve of the fifteen frames are drawn, every one sits on a desktop, both wiki
hosts have them, and `README.md` uses them too. Four defects in the drawing had
to be fixed first and four wrong claims in the docs came out of it.*

**Written:** 2026-08-10 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb                          # expect clean, in sync with origin/main
python3 tools/wiki/draw.py --out /tmp/x # expect 12 frames, no error, ~4 minutes
bash tools/wiki/publish.sh --dry-run    # expect "already up to date" on both
agentbox pending                        # expect nothing pending
```

**The deployed daemon is untouched by this session.** Nothing here goes in the
binary except `tools/wiki/drawhtml`, which is a command nobody runs at runtime.
`make deployed` still reports `c306e5e5a972`; that is session 59's note and still
correct.

**Budget.** The week stood at 76% (all models) when this was written, and the
session itself at 19%. The week resets Aug 12, 4:59am (Asia/Jerusalem). Check
`claude -p /usage | grep -E '^Current '` before starting anything large - there
is room for one more substantial piece of work before the reset, not several.

1. **Robustness band A, seven left**: R-06, R-07, R-09, R-10, R-12, R-13, R-15,
   in `docs/backlog/robustness.md`'s own order. **R-12 and R-13 are the
   hours-sized ones** and are the place to start; R-06, R-07, R-09 and R-10 are a
   day each, and R-15 needs a live MCP-host repro before it can be fixed at all.
2. **U-16 wants one more repro** before its fix is chosen. Carried unchanged from
   session 59, which means nobody has looked at it since - go and observe it
   rather than re-reading the entry.
3. **U-17 and U-18 are new and both are artifact defects.** U-17 is hours and
   well understood. U-18 is a symptom with no diagnosis yet and should be taken
   with it, because the reproduction is the same window.

## Where we are

The wiki's frames were photographs from a 2026-08-07 sitting; two had been
redrawn in session 59. All the rest are drawings now, and the two things that
made drawing worth doing both showed up: a drawn frame can be composed exactly
(the photographer could never get four agent rows, because `sync lock` mints a
session key per lock), and a fixture makes you state what you expect before you
see it, which is what turns a wrong document into a visible error.

## Live state (volatile - verify on resume)

- **Background jobs:** none. Several stray `vite` servers were started by hand
  while debugging and killed with `pkill -f draw.config.js`; check with
  `pgrep -af "draw.config"` if a port seems taken.
- **PRs:** none, ever. Boris pushes `main` directly.
- **Git:** `main` clean and **pushed to `origin` (gitlab)** through `98740c1`,
  plus a docs commit from `/handoff` after it. The `github` remote is a mirror of
  the *code* repo and was **not** pushed this session; the permission classifier
  blocked it in session 59 and nothing has changed there.
- **Wiki:** both hosts current (`publish.sh --dry-run` says so). The GitHub wiki
  is a separate repo from the code mirror and pushes fine.
- **Deployed daemon:** untouched, `c306e5e5a972`.
- **Desktop:** never taken. Nothing this session needed it, which is the point of
  drawing.
- **In-flight edits:** none.

## Blocked on you (Boris)

1. **Whether `docs/backlog/features.md`'s one non-goal revision is on.** Carried
   unchanged from sessions 56 through 59 and still nobody's: B-1, "away without
   becoming a cloud service", wants the vision's non-goal 3 amended in public the
   way ADR-0009 amended principle 6. A principle change, therefore yours.
2. **Whether the Esc notice earns its place.** Carried from 58 and seen on screen
   in 59: it reads well and the window grows to fit it, so the question is
   whether you want it daily. One string in `internal/daemon/daemon.go`.
3. **Whether the agents board should lead with shared values.** The surface puts
   the blackboard above the agent rows and always has; whether that is right is a
   design call. The frame is drawn either way.
4. **U-17's shape.** An artifact opened on its own has no way to read its source,
   because the preview/code toggle is hidden for exactly that case. Two fixes and
   the choice is taste: put the toggle back in the bar for every shape, or move
   it into the viewer's title bar beside `find`, `A-`, `A+`. The entry argues
   both.

## I can do solo (no input needed)

1. **R-12 and R-13**, the two hours-sized band-A items left.
2. **U-16's second repro**, then its fix.
3. **U-18's diagnosis** - why a failed artifact leaves the bar's error slot empty.
4. **R-06, R-07, R-09, R-10**, a day each.
5. **R-40's open fixes (2) and (3)**: a hostile payload against the card, and
   executing `buildDocument` to assert the CSP and sandbox attributes that today
   are only checked as string constants.
6. **U-05**, hours: `theme.motion = "reduced"` is honoured by four components of
   the thirteen that animate.
7. **`settings.md`'s SHOT placeholder**, if somebody wants it: it needs either a
   photograph or a fixture that can answer `previewTheme` with Go's own token
   builder, and `DRAW.md` says why the second is the harder half.

## Facts - verified vs assumed

- [verified] **All fifteen images accounted for, and every drawn one read on
  screen before publishing.** Twelve drawn; three deliberately photographed and
  left alone (`install-doctor`, `history-stats`, `review-board`).
- [verified] **A redraw is byte-identical.** Checked by drawing four frames twice
  into different directories and comparing md5s, after the animation-settling fix
  and again after the desktop change.
- [verified] **The measuring and capture defects are real and fixed.** The
  progress window measured 1857px in a 2000px probe before the fix and 259px
  after; the artifact rendered in one run and not the next under the old capture,
  and has been consistent since.
- [verified] **Both wikis are current**, by `publish.sh --dry-run` reporting
  "already up to date" for gitlab and github, and by fetching two published pages
  and reading back the captions.
- [verified] **`make check` exits 0** - gofmt, vet, race tests, 43 vitest passing
  and 1 expected fail.
- [verified] **U-17 by drawing it**: the drawn artifact frame has no toggle
  because `app.css:960-963` hides it, and the page that claimed otherwise is
  corrected.
- [assumed] **U-18's cause.** The symptom reproduced many times: a failed
  artifact shows bar, badge, runtime label, empty stage and an empty error slot.
  Why the failure never reached the note is not diagnosed.
- [assumed] **That the desk's blurred backdrop reads well to a stranger.** Boris
  approved the direction by calling the flat frames ugly and asking for all of
  them to match; nobody has read the pages end to end since as a new reader.
- [assumed] The carried set from sessions 50, 51, 55, 57 and 59, none of which
  this session looked at: the artifact-restart-while-streaming edge; the
  30-minute fuse in a live daemon; the demoted marker on a second monitor;
  `perform.py`'s fullscreen check on two monitors; the domain drawer animation;
  the demo fallback painting; a detail holding a mermaid diagram;
  `provisionalFor` retiring a hook-only row; the 200-key prefix cap; `@me` in a
  shared key; a real client's `await_signal` parked past 20s; the two lock
  warnings; and the identity cross-check's no-node skip.

## Two traps this session paid for

- **A virtual clock is not a wait.** `chrome --headless --screenshot
  --virtual-time-budget=N` fast-forwards a page's timers, not its CPU, so the
  budget is spent in a few real milliseconds and the picture is of whatever
  finished. It looks like flakiness and reads like a content bug: an hour went
  into blaming the fixture's markup - a stat row, then a class name, then a grid
  - before "two runs of one input differ" pointed at timing. If a capture is
  non-deterministic, suspect the wait before the input.
- **`git checkout <file>` on a file with uncommitted work throws the work away.**
  Used it to revert one line of debug instrumentation in `frontend/draw/runtime.js`
  and it also reverted `index.html`'s whole layout script, which was an hour of
  uncommitted harness work in the same directory. Rewriting it cost ten minutes;
  it could have cost the session. Revert the line, not the file.

## What this session changed

| Commit | What |
|---|---|
| `d0401e0` | the nine remaining frames, the desk, `drawhtml`, `shoot.mjs`, and the four drawing defects |
| `9f9a0f6` | captions on every frame, the two new placements, the artifact-toggle correction |
| `d158e32` | every frame on a desktop; README repointed; `docs/img` screenshots deleted; the diff review card |
| `af73111` | FR99 closed, DESIGN.md's four corrections, DRAW.md rewritten, U-17 and U-18 filed |

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| Session 59's "the remaining eleven wiki frames" item | Done. What it taught is in `DRAW.md`: the four ways a drawing goes wrong, and which frames must stay photographs |
| `docs/img/{card,review,progress,viewer,artifact,app}.png` | Deleted, not moved. They were taken before the rename and said `qq` in their title bars; `README.md` uses the drawn frames now |
| DESIGN.md's "crop to the card plus about 24px of dark desktop" | Superseded in place, with the reason attached: a transparent frameless window clips its own shadow, so the desktop is what draws one |
| Session 59's note that `make deployed` is behind HEAD | Still true and still fine, restated once in "Do this next" rather than argued again |

## Map

1. [HANDOFF.md](HANDOFF.md) - this file.
2. **[docs/backlog/README.md](docs/backlog/README.md) - read this first.** One
   order across all three audits. Tier 1 is seven robustness items plus U-16.
3. [docs/STATUS.md](docs/STATUS.md) - current state, updated today.
4. [tools/wiki/DRAW.md](tools/wiki/DRAW.md) - how the frames are drawn, what each
   part of the drawing exists to prevent, and which frames must stay photographs.
5. [docs/wiki/DESIGN.md](docs/wiki/DESIGN.md) - section 5 is the shot list, now
   carrying four corrections the drawing forced.
6. [docs/backlog/ux.md](docs/backlog/ux.md) - U-16 is the open band-A item; U-17
   and U-18 are new and both about artifacts.
7. [docs/backlog/robustness.md](docs/backlog/robustness.md) - 45 items in six
   bands; seven band-A left.
8. [docs/07-field-requests.md](docs/07-field-requests.md) - FR99 is closed and
   its entry is the short version of this work.
9. [docs/wiki/FACTS.md](docs/wiki/FACTS.md) - **read before quoting any number
   about the product.**
10. [docs/history.md](docs/history.md) - session 60 is at the top.
