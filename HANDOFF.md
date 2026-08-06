# Handoff - AgentBox: the whole queue cleared in one session, and nothing is blocked

*Written by session 54, which opened on an empty queue with everything blocked on
Boris, asked for all of it at once, and shipped the lot. FR30 is built. FR84 is
closed. Nothing waits on him and nothing is in flight.*

**Written:** 2026-08-06 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean, level with origin/main (pushed)
                            # origin is gitlab and IS main's upstream; the `github`
                            # remote is far behind and is not where this goes
make deployed               # expect 731e20af69be - see "No deploy is owed" below
agentbox control state      # expect "no run: the desktop is the human's"
agentbox pending            # expect "nothing pending"
agentbox sync agents        # your row; ghosts from `claude -p` checks are harmless
make check                  # gofmt + vet + race + JS tests, ~3 min
```

**No deploy is owed.** `731e20af69be` was the last code commit and it is what the
daemon is running; the only thing after it is the docs commit carrying this
handoff. Confirm with `git log --oneline 731e20a..HEAD` before assuming otherwise,
and remember `make deployed` asks the running daemon, because a replaced binary
is not a deployed binary.

**Nothing is in flight, no field request is open, and nothing is blocked on
Boris.** This is the first handoff in a long while with an empty "Blocked on you"
section, so the next session picks its own work from the list below rather than
waiting for a sentence.

### The queue, in order

1. **`[flood]` wants living with before it is tuned.** `burst = 3` in `window_s =
   10` is Boris's number, chosen on paper and shipped the same day; nobody has yet
   had a real agent trip it in the course of ordinary work. If it fires on
   something innocent, the knob is in `~/.config/agentbox/config.toml` - **do not
   change his file to "fix" a judgement he made**; tell him what tripped it.
2. **Two FR30 judgement calls are worth a second opinion**, both deliberate and
   both documented in [docs/STATUS.md](docs/STATUS.md):
   - An open stack card keeps collecting **until the human dismisses it**. That is
     what stops a sustained loop getting a fresh budget every window, but it also
     means a session that flooded once stays collapsed until he closes the card.
   - An urgent item inside a burst **collapses** into the stack card and raises it
     to urgent (which then preempts). It never appears as its own card. The
     alternative - urgent always breaks out - was not built.
3. **`AskPanel.svelte` did not get FR84's fold.** The inline panel has the same
   title/body/controls order as the card, but it lives in the scrolling session
   transcript, so a long body does not put its controls out of reach the way a
   fixed-height card window does. Judged out of scope rather than missed. Look at
   it on a real long-bodied ask before deciding it needs the same treatment.
4. **More fuzz targets** if nothing better presents. Four exist
   (`change.FuzzParse`, `walkthrough.FuzzCover`, `FuzzSpecParse`,
   `FuzzBuildPayload`); `config.SplitArgv` is a new, small, hand-written parser
   and is the obvious fifth.
5. **`tools/showcase/` is frozen but not deleted.** Deleting it is Boris's call
   and he has not made it. Do not delete it on your own initiative; ask.

## Where we are

Session 53 left FR30 as the only substantial thing left, blocked on one sentence.
Session 54 asked for that sentence and four others in the same breath, and Boris
cleared the whole list in two exchanges. Everything he decided was built,
deployed, and exercised on the real desktop the same session.

**What he decided (2026-08-06), because a future session will want the reasoning
and not just the outcome:**

| Question | His answer |
|---|---|
| FR30: collapse or drop? | Collapse into one stack card, nothing dropped |
| FR30: what counts as a flood? | 3 cards in 10s, per agent, and make it configurable |
| FR84's other half | Approach C: fields first, prose folded |
| `[editor]` / `speech.command` control | Build it |
| The showcase re-record | **Dropped for good** - stop maintaining it |
| `webui-demo agents` (needs the daemon down) | Go ahead now |

**All five shipped.** FR30 is `internal/daemon/flood.go` + kind `stack` +
migration 0013 + `[flood]` knobs. FR84's other half folds a body past 240
characters behind "why this is being asked" (`?` or a click). The settings
surface has a `command` kind for argv arrays. The showcase docs carry a frozen
banner. `webui-demo agents` was watched and is fine.

## The part worth reading: six defects, and how each was found

Four unit-test-invisible defects in FR30 and two in FR84. **Not one of them came
from reading the diff**, and that is the finding, not a coincidence:

- **The budget refilled underneath an open stack card**, so a sustained loop
  collected a fresh budget every window - three cards per ten seconds on the
  shipped defaults. Found by watching a live burst and doing the arithmetic.
- **`agentbox dismiss <stack>` cleared the summary and left every notification
  under it pending.** The sweep had been written on the card's own Esc, and there
  are three doors that retire an item. Found while clearing the demo queue.
- **The number key promoted the right item and then put the stack card back in
  front of it**, so pressing 1 read as nothing happening except "1 waiting". Every
  unit test passed because they all had the stack card queued rather than on
  screen, which is not where a human is when they press the key.
- **An answered row still read "waiting on you"** under a footer still counting
  one, because the entries were a snapshot taken at collapse time that nothing
  revisited.
- **A restart un-collapsed the burst.** Every collapsed item is pending in its own
  right, so the restore put the stack card AND all fourteen items back on the
  queue, and the restored card had no budget behind it either (so the next item
  opened a second summary of the same flood). Found in the adversarial pass over
  the diff, then reproduced and fixed against a real `systemctl --user restart`.
- **A card could only ever get taller.** `.card` is `min-height: 100%`, so
  measuring the shell measures the WINDOW; once a card had grown it reported the
  window's height back to Go forever. Older than FR84 and invisible until FR84
  gave a card something that folds. It needed **two** fixes - the measurement has
  to take min-height off for the read, AND anything that can make the card shorter
  has to ask for a re-measure, because under the window's height there is nothing
  left for the observer to observe.

## What was verified on the real desktop

Not read off the diff. Captures in the scratchpad (path in Live state).

- **A real flood, end to end**: seven `agentbox notify` calls, three own cards,
  four collapsed into one card reading "claude: 4 notifications in under a
  second" (`fr30-stack-1.png`).
- **A question caught in a burst** (`fr30-v2-stack.png`): row 1 amber, "waiting on
  you", footer "1 still waiting for an answer; dismissing keeps it". `e` expands,
  `1` opens it as a real card, answering it delivered `Roll back` to the parked
  CLI caller, and the row came back marked `done` and dimmed
  (`fr30-v3-done-row.png`).
- **A real daemon restart** (`systemctl --user restart agentbox.service`) with a
  burst pending: the stack card came back with its four entries, the collapsed
  items did NOT come back as their own cards, and the next notify grew the same
  card rather than opening a second (`fr30-after-restart.png`).
- **FR84's fold** (`fr84-shrinks.png`), with the window measured at each step:
  230px folded, 384 open, 230 folded again.
- **Both settings controls** on the real Settings surface
  (`settings-sessions.png`, `settings-sound.png`). Boris's own `speech.command`
  renders as itself and the form stays clean, which is the round-trip check that
  mattered. Typing into the editor box flipped the footer to "1 key to write ·
  editor.command"; **Revert put it back and his `config.toml` was left byte-identical.**
- **`webui-demo agents`**: every area, badge, the orphan lock's Break lock confirm,
  and an opened row's four blocks.

## Live state (volatile - verify on resume)

- **Deployed:** `731e20af69be`, the last code commit. Only the docs commit
  carrying this handoff sits after it, so no deploy is owed.
- **Git:** `main`, clean, pushed to `origin` (gitlab). Twelve commits this
  session, oldest first: `e8f5d88` (demo surface seen + showcase dropped),
  `5b097ef` (FR30), then the four fixes the live desktop forced - `5cd2847`
  (refilling budget), `6aad71b` (the door that did not sweep), `6cda360` (the
  stack retaking the screen), `e37f2e1` (the row that kept asking) - then
  `e0eaf23` (FR84's fold), `c321e80` and `eadf9da` (the card-shrink defect, which
  took two), `544946f` (argv settings), `8a4f254` (docs), `731e20a` (the restart
  and urgent fixes from the adversarial pass). `git log --oneline 7643938..HEAD`
  is the truthful answer; that sha is the previous session's last commit.
- **Two remotes, and only one of them is the tree.** `origin` is
  `git@gitlab.com:fu-bar/agentbox.git` and is `main`'s upstream. `github`
  (`git@github.com:borismilner/agentbox.git`) is far behind. A bare `git push` is
  right because the upstream is set.
- **Boris's `~/.config/agentbox/config.toml` was NOT modified.** The settings
  control was exercised with Revert rather than Save, and the file's md5 was
  checked after. It has no `[flood]` section, so flood control runs on the
  built-in defaults (3 in 10s).
- **The desktop is his**, nothing pending, no locks held, no agentbox window on
  screen. All checked at the end.
- **Captures live in a session scratchpad** and will not survive a reboot:
  `/tmp/claude-1000/-home-boris-milner-me-projects-agentbox/e1fc6994-afab-4e94-87cb-37f56cb75f91/scratchpad/`
  Deliberately not committed.
- **Usage at handoff:** session ~50%, resetting 2026-08-06 23:40 Asia/Jerusalem;
  week (all models) ~30%, resets 2026-08-12 05:00. A clean stop with room left,
  not a rescue.
- **In-flight edits: none. Background jobs: none. PRs:** none, ever.

## Blocked on you (Boris)

**Nothing - proceed autonomously.** He cleared the entire list at the start of
this session and every answer has shipped. The only thing that is his to say is
whether `tools/showcase/` should be deleted outright (queue item 5), and that is a
question to raise, not a blocker.

## I can do solo (no input needed)

1. **Fuzz `config.SplitArgv`** (queue item 4) - a new hand-written parser with an
   obvious round-trip property, and the cheapest real work on the list.
2. **Look at a long-bodied ask in the inline panel** and decide whether FR84's
   fold belongs there too (queue item 3).
3. **Watch `[flood]` in ordinary use** and report anything it trips on that it
   should not have (queue item 1).

## Facts - verified vs assumed

- [verified] **`make check` passes** (gofmt, vet, race, JS tests) after every
  slice, including the last one.
- [verified] **FR30 works end to end on the deployed build**: collapse, expand,
  number-key open, answer delivered to a parked CLI caller, the row marked done,
  the sweep on dismiss, and survival of a real daemon restart.
- [verified] **FR84's fold and the card-shrink fix**, measured on screen at 230 /
  384 / 230 px.
- [verified] **Both settings controls render and mark exactly one key to write**,
  and Revert leaves Boris's config byte-identical.
- [verified] **`webui-demo agents` renders correctly** - the last unseen surface.
- [verified] The session-51/52/53 set, unchanged: FR95 end to end, the panic
  guard, the convergence guard, both diff parsers surviving a lying hunk header,
  the `quiet_hotkey` knob rendering, the demoted marker's four traps, and the two
  clean fuzz targets.
- [assumed] **That the 30-minute fuse fires in a live daemon.** Carried from
  session 51: the timer is driven directly in the test and nobody has watched half
  an hour pass.
- [assumed] **That the demoted marker behaves on a second monitor.** Carried; one
  monitor here, so only the fallback has ever run.
- [assumed] **That `[flood]`'s defaults are right for ordinary work.** They are
  Boris's number, chosen on paper the same day they shipped, and only a synthetic
  burst has ever tripped them.
- [assumed] The session-50/51 carried set: that the domain drawer animation is
  what Boris wants, that the demo fallback paints, a detail holding a mermaid
  diagram, `provisionalFor` retiring a hook-only row, the 200-key prefix cap,
  `@me` in a shared key, a real client's `await_signal` parked past 20s, the
  holder-parked-on-ask_user and 600s long-wait lock warnings, and the identity
  cross-check's no-node skip path.

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| Session 53's queue item 1 (FR30 blocked, and the two questions to put to Boris) | Both answered and built. The answers and their reasoning are in the decision table above, in the FR30 section of [docs/STATUS.md](docs/STATUS.md), and in the code comments in `internal/daemon/flood.go` |
| Session 53's queue item 2 (`webui-demo agents` unseen) | Watched. Findings and the two harness mechanics are in the FR83 section of [docs/STATUS.md](docs/STATUS.md) |
| Session 53's queue item 3 (`[editor]` has no settings control) | Built. The `command` kind and why neither knob needs a restart are in the settings bullet of [docs/STATUS.md](docs/STATUS.md) |
| Session 53's "Blocked on you" items (FR30, FR84's other half, the re-record, the editor control) | All four answered on 2026-08-06; the decision table above records what he chose, and each is closed in its own doc section |
| Session 53's captures, scratchpad path and its whole narrative | Gone with that session; this session's are listed fresh in Live state, and session 53 survives in [docs/history.md](docs/history.md) |
| The showcase re-record, carried as a priority since session 24 | Dropped for good. Priority 2 of [docs/STATUS.md](docs/STATUS.md) says what that means for `tools/showcase/` and its docs, and the three docs carry a frozen banner |

## Map

1. [docs/STATUS.md](docs/STATUS.md) - current state. The FR30 section carries the
   four things to know before touching flood control; the FR84 section carries the
   card-measurement defect; the settings bullet carries the new `command` kind.
2. [docs/history.md](docs/history.md) - session by session; this session is
   "Fifty-fourth".
3. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers used in
   commits and handoffs. **No FR is open.**
4. [docs/01-requirements.md](docs/01-requirements.md) - FR30, now marked built,
   with the one departure from its wording written down.
5. [docs/06-configuration.md](docs/06-configuration.md) - the real `[flood]` keys.
6. [docs/agent-manual.md](docs/agent-manual.md) - the MCP tool reference.
7. [CLAUDE.md](CLAUDE.md) - traps that have cost sessions. **Read it before
   touching the build, the daemon, or driving the desktop.**
