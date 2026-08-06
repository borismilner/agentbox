# Handoff - AgentBox: FR95 closed, and four robustness finds behind it

*Written by session 52, which finished FR95's last answer, watched it on the real
desktop, and then spent the rest of the session on the robustness pass Boris
asked for - one of which found a live bug the tests could never have.*

**Written:** 2026-08-06 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean and level with origin/main (pushed)
                            # origin is gitlab and IS main's upstream; the `github`
                            # remote is far behind and is not where this goes
make deployed               # expect 8b6344f1af55 or later
agentbox control state      # expect "no run: the desktop is the human's", no quiet suffix
agentbox pending            # expect "nothing pending"
agentbox sync agents        # your row; ghosts from `claude -p` checks are harmless
make check                  # gofmt + vet + race + the new JS tests, ~2 min
```

**Nothing is in flight. No field request is open.** The queue below is what is
left over, in the order it is worth doing.

### The queue, in order

1. **Look at the two surfaces this session could not see.** The display went to
   sleep partway through (`gnome-screenshot` returns black; the screensaver's
   `GetActive` says true), so two things are wired, tested and unwatched: the new
   **`quiet_hotkey` knob** under "Hands off" in settings (`agentbox app --tab
   settings`), and the **cards-held behaviour with a run live** - everything in
   FR95 was watched with no run, which is the common case but not the only one.
2. **The frontend diff parser swallows a lying hunk header.** Found while fixing
   the Go one (queue item 4 below): a count of twenty digits is not negative in
   JS, merely enormous, so every following file is consumed into one body and the
   reader loses them with nothing on screen to say why. The fix is to end a hunk
   when a body line is not a body line at all (a valid one starts with ` `, `+`,
   `-`, `\` or is empty), in `frontend/src/lib/diff.js`. **It changes the render
   path, so it needs a screen** - there is a test ready to be written beside the
   eleven in `frontend/src/lib/diff.test.js`, and `internal/change` is the shape
   to copy.
3. **Four assumed things about surfaces, still unseen.** Carried from sessions
   50 and 51; each needs a specific staging, listed under "Facts" below.
4. **Consider giving `[editor]` a settings control.** Unchanged and still his
   call: the value is an argv array and the descriptor table has no kind for one,
   which is why `speech.command` has none either. The honest shape is a new kind.
5. **Flood control (FR30) is a documented `must` that was never built.**
   `docs/01-requirements.md` promises per-agent rate limits collapsing into one
   stack card; nothing in the daemon does it, so an agent in a loop papers the
   screen and (now) can fill a recording's held queue. Deliberately NOT built
   this session: it changes when a card appears, which is a decision about his
   notifications that he has not made. Worth putting to him as one question.

## Where we are

FR95 was three quarters built when this session started. Its last answer - **the
card column goes quiet with the sign** - is built, deployed and watched, so the
request is closed and no field request is open.

**While the sign is demoted:** cards queue instead of appearing and drain the
moment it goes loud; the earcon still plays (recording mode is NOT do-not-disturb
- DND holds the chime and this keeps it); the spoken line IS held, because a
voice reading a card aloud lands in the take; urgent waits but goes to the front
of the queue so it is first out; the progress window closes with the cards; and
nothing is lost - the inbox has everything from the second it arrives.

Then Boris asked for an adversarial robustness pass. Four things came out of it,
all shipped: a **panic guard on every timer goroutine** (they had none, and a
panic there ends the process and every agent parked on a question), the
**coverage validator** the standard has been missing since session 49, a **JS
test runner** for the frontend's shared modules, and a **fuzz target** that found
a real overflow within seconds of existing.

## What was verified on the real desktop

Not read off the diff.

- **Three notifies during a demoted sign left `wmctrl -l` with no AgentBox
  window at all**, and `agentbox control state` read `quiet: the sign is demoted
  for 30m0s more, 3 cards waiting`.
- **Going loud put the URGENT card on screen first**, with "2 waiting" in its
  footer - the ordering fix visible in one screenshot
  (`fr95-drained.png`, scratchpad path under Live state).
- **A live `agentbox progress` window vanished on `control quiet`**, and the
  completion toast it raised while demoted drained on `control loud`.
- **The coverage validator, live against this repo's own diff:** `4 of 5 hunks
  are not cited by any step ... internal/daemon/walkthroughs.go:4-10, ...`, and
  the read path recomputes the same numbers. The test walkthrough was deleted
  afterwards; his library has only his two.
- **A post-deploy smoke test of the final build** (`8b6344f1af55`): quiet, one
  notify, `1 card waiting`, zero AgentBox windows, loud, dismissed.

> **The one thing a test could not have found.** `FuzzParse` existed for about
> thirty seconds before it produced `@@ -10000000000000000000 +0 @@`: the
> hand-rolled `atoi` in `internal/change` had no ceiling, so twenty digits
> overflowed int and came back NEGATIVE, and that negative flowed into every span
> and slice built from the geometry. The blast radius had grown that same morning,
> because the new coverage arithmetic runs on every walkthrough READ as well as
> every create.

## The four robustness finds, and why each was worth it

- **Timers had no recover.** Every RPC handler has had one since the beginning.
  Every `time.AfterFunc` - toast expiry, escalation, undo grace, the FR95 fuse,
  the pause nag - and four subsystem tickers had none. The guard wraps the
  CALLBACK, never the loop: a recover around a ticker goroutine swallows the
  panic and ends the reaper with it, which trades a loud death for a silent one.
- **The coverage rule had no validator**, and it was the rule that most needed
  one. Overlap counts as covered, a pure-deletion hunk collapses to the seam it
  left, a deleted file is not counted either way, and no diff means uncomputed
  rather than clean. Warnings name the hunks by path and line, because "3 of 11"
  sends the author back to diff the diff by hand.
- **`parseDiff` had two callers and no test.** Eleven now, on node's own runner,
  no framework - a dist committed so a machine without npm can build must not
  start needing npm to be tested. `make test-js` is in `make check` and skips
  itself when node is absent.
- **A silent divergence in FR95's own delivery.** Two flips (a hotkey against the
  fuse) can each release control's lock and then be scheduled in the other order,
  leaving the daemon holding every card while the strip says loud - no symptom on
  screen, no way back but a restart. The sink is serialized by its own mutex and
  reads the mode at its turn rather than carrying the value its caller wrote.
  Mutating it back makes the test fail under `-race`.

## Live state (volatile - verify on resume)

- **Deployed:** `8b6344f1af55`. Every commit after it is docs and tests, so no
  deploy is owed - but check `make deployed` against `git log` before assuming,
  and remember that `make deployed` asks the running daemon because a replaced
  binary is not a deployed binary.
- **Git:** `main`, clean, **pushed to `origin`** (gitlab). Nine commits this
  session, oldest first: `6617403` (FR95 cards), `8473edb` + `d62d616` (FR95
  docs), `36fbc04` (panic guard), `cb52c59` (held count), `f73b0e6` (settings
  knob), `5bc8997` (coverage validator), `a78094b` (JS runner), `8b6344f` (the
  overflow). Run `git status -sb`: that is the only truthful answer.
- **Two remotes, and only one of them is the tree.** `origin` is
  `git@gitlab.com:fu-bar/agentbox.git` and is `main`'s upstream. `github`
  (`git@github.com:borismilner/agentbox.git`) is far behind and is not where this
  work goes. A bare `git push` is right because the upstream is set.
- **The desktop is asleep.** The screensaver is active and the monitor is off;
  `LockedHint` says no but a capture comes back black either way. Nothing was
  woken. Anything visual waits.
- **Nothing pending, no locks held, no strip on screen, nothing quiet.** All
  checked at the end. The desktop was never taken this session (no
  `request_control` at all - every check was CLI and `wmctrl`).
- **A peer session shares this checkout:** `proc-832814-3533968`, detached,
  carrying the SessionStart placeholder purpose. It made no commits. Two agents
  in one checkout is how an unfinished doc once got swept into an unrelated
  commit, so no catch-all `git add` without looking at `git status` first.
- **Captures live in a session scratchpad** and will not survive a reboot:
  `/tmp/claude-1000/-home-boris-milner-me-projects-agentbox/f18188cc-7c16-42b5-94a4-c857a0587336/scratchpad/`
  (`fr95-drained.png` is the urgent-first screenshot; `settings-1.png` is the
  black one that proved the screen was asleep). Deliberately not committed.
- **Usage at handoff:** session ~15%, resetting 2026-08-06 23:40 Asia/Jerusalem;
  week (all models) ~27%, resets 2026-08-12 05:00. **Boris capped this session at
  35% of the week**, so there was room left - this is a clean stopping point, not
  a rescue.
- **In-flight edits: none. Background jobs: none. PRs:** none, ever.

## Blocked on you (Boris)

Nothing - proceed autonomously. Four things stay yours and block nothing:

- **FR84's other half** (a long body still pushes a form's fields below the fold)
  needs your word, because the approach that fixes it is the one you did not
  pick. Mock: [docs/mocks/fr84-form-shapes.html](docs/mocks/fr84-form-shapes.html),
  approach C.
- **Flood control (FR30)** is a `must` in the requirements that was never built
  (queue item 5). Building it changes when a card appears, so it wants one
  sentence from you before anybody writes it.
- **Your PostToolUse hook writes the raw Bash command as the activity line.**
  Carried from six handoffs. It is your `~/.claude/settings.json`, so the wording
  is yours; the first line, or the first 80 characters, would read better than
  four lines of shell on the hands-off strip.
- **Whether the re-record is worth scheduling** (STATUS priority 2). Unchanged.

## I can do solo (no input needed)

1. **The two unseen surfaces** (queue item 1) - the moment there is a screen.
2. **The frontend parser's lying-hunk fix** (queue item 2) - also needs a screen,
   because it changes the render path.
3. **The four older unseen surfaces** (queue item 3).
4. **More fuzz targets.** The first paid for itself in thirty seconds and
   `FuzzCover` followed it (both clean now: 11.8M and 6.9M executions).
   `walkthrough.Parse` and the payload builder are the next two worth pointing it
   at - both read agent-authored text straight off the wire.

## Facts - verified vs assumed

- [verified] **FR95 end to end**, every bullet under "What was verified" above,
  on the deployed build.
- [verified] **`make check` passes** (gofmt, vet, race, and now the JS tests)
  after every slice.
- [verified] **The panic guard holds**: mutating the toast timer back to an
  unguarded callback makes the test take the whole package's run down.
- [verified] **The convergence guard holds**: mutating the sink back to
  pass-the-value makes the sixty-flip test fail under `-race`.
- [verified] The session-51 set, unchanged: the demoted marker is coverable by a
  fullscreen window, a frameless 4px window at `+0+0` sits over GNOME's top bar,
  `Ctrl+Alt+Q` fires through XTEST, and `wmctrl -lG` reports DOUBLED coordinates
  on this desktop (use `xwininfo`).
- [assumed] **That the `quiet_hotkey` knob renders correctly** under "Hands off".
  It is wired at both ends and the descriptor test covers the wiring; nobody has
  looked at it. A control can be fully styled in the stylesheet and unstyled on
  screen - that trap has cost this project three build-deploy-look cycles.
- [assumed] **That cards queue the same way with a run live.** The mechanism does
  not read the run at all, but every live check this session was made with no run
  on the desktop.
- [assumed] **That the 30-minute fuse fires in a live daemon.** Carried from
  session 51: the timer is driven directly in the test and nobody has watched
  half an hour pass.
- [assumed] **That the demoted marker behaves on a second monitor.** Carried;
  one monitor here, so only the fallback has ever run.
- [assumed] The session-50/51 carried set, unchanged: that the *"This session has
  left the board"* wording ever paints, that `speak` and `diff` read back on
  screen, that an agent row's Recent items block paints, that the demo fallback
  paints, that the domain drawer animation is what Boris wants, and the older
  list behind those (the inbox detail's `found: false` path, the keyboard route
  to a resolved row's detail, a detail holding a mermaid diagram,
  `provisionalFor` retiring a hook-only row, the 200-key prefix cap, `@me` in a
  shared key, a real client's `await_signal` parked past 20s, the
  holder-parked-on-ask_user and 600s long-wait lock warnings, `webui-demo agents`
  still rendering, and the identity cross-check's no-node skip path).

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| Session 51's queue item 1 (FR95's cards half) | Built. The behaviour is in the FR95 entry, the reasoning in session 52 of [docs/history.md](docs/history.md) |
| Session 51's queue items 2 and 3 (coverage validator, `quiet_hotkey` knob) | Both built. The validator is `internal/walkthrough/coverage.go` with the rule quoted in [internal/manual/walkthrough.md](internal/manual/walkthrough.md); the knob is in `internal/webui/settings.go` and unwatched (queue item 1) |
| Session 51's measurement section and its probe | The four traps are permanent in "Mechanics discovered" ([docs/07-field-requests.md](docs/07-field-requests.md)); the probe was never committed and needs no recovery |
| Session 51's mock-driving section (`drive_mock.py`) | The three things worth knowing are in session 51 of [docs/history.md](docs/history.md). If a third mock earns this treatment it earns a place under `tools/` |
| Session 51's captures and scratchpad path | Gone with that session; this session's two are listed fresh in Live state |

## Map

1. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers used in
   commits and handoffs. **No FR is open.** FR95's entry carries Boris's four
   answers, what shipped, the two things a held card does not stop, and what is
   deliberately not held.
2. [docs/STATUS.md](docs/STATUS.md) - current state. Priority 1c (FR95) is done;
   the tests paragraph now names the JS runner and the fuzz find.
3. [docs/history.md](docs/history.md) - session by session; this session is
   "Fifty-second", and its second half is the robustness pass.
4. [internal/manual/walkthrough.md](internal/manual/walkthrough.md) - the
   authoring standard. Rule 49 now says AgentBox checks it, and how.
5. [docs/agent-manual.md](docs/agent-manual.md) - "He can be recording, and your
   card waits", under "Taking the desktop": what an agent must budget for.
6. [docs/06-configuration.md](docs/06-configuration.md) - `[control]`, with
   `quiet_hotkey` and what it now takes down with the strip.
7. [CLAUDE.md](CLAUDE.md) - traps that have cost sessions. **Read it before
   touching the build, the daemon, or driving the desktop.**
