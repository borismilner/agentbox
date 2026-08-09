# Handoff - AgentBox: the answer path learns to say no, and a live check still owed

*Written by session 58, which took tier 1 of the merged backlog and closed the two
items it names: U-02, the Go answer path that could not report a refusal, and U-01,
the card that had no way to show one. Both are deployed. One thing is owed and it is
named below: neither was exercised on a real desktop, because the drive was
interrupted a keystroke in. Boris also set a new direction for the wiki's pictures
(FR99) and a budget cap that ended the session.*

**Written:** 2026-08-09 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean, 5 ahead of origin/main, nothing pushed.
                            # origin is gitlab and is main's upstream; `github` is
                            # a mirror. Boris pushed up to 943258b (FR97) between
                            # sessions 57 and 58, so session 57's handoff saying
                            # "~31 ahead" is stale, not wrong at the time
make deployed               # expect a3e570de160d, equal to the last code commit
agentbox pending            # expect nothing pending
```

**Budget first.** Boris capped this at **70% of the weekly quota** and it was at
exactly 70% when the session stopped, which is why nothing after this line was
started. Check `claude -p /usage | grep -E '^Current '` before doing anything: the
week resets Aug 12, 4:59am (Asia/Jerusalem). If it is still at or over 70%, the only
correct move is to say so and stop.

1. **The live check U-01 and U-02 still owe.** It is twenty minutes and it is the
   one thing standing between "tested" and this project's own bar. The recipe is
   below under "the live check, exactly".
2. **Then FR99: draw the wiki's frames instead of photographing them.** Boris asked
   for this directly and it is not yet started. The entry in
   `docs/07-field-requests.md` says what it buys, what it costs and where the work
   lands.
3. **Then robustness band A**, which is all that is left of tier 1: thirteen items,
   R-03 onwards, in `docs/backlog/robustness.md`'s own order. All three ux band-A
   items are now closed.

## Where we are

Session 57 wrote the ux audit and the one order across all three audits. This
session executed the top of it. U-02 and U-01 are one defect seen from the daemon
and from the webview, and taking them in that order was the audit's own advice: the
wrapper is worth building only once there is something to show in it.

The through-line, now five sessions old: **a document asserted something and the
code contradicted it.** This time it was smaller and it was in the audit itself -
U-02 said "give the answer-path methods a return value in the shape `Triage` already
has (`bool`, or better a short sentence)". The sentence turned out to be the shape
the codebase had already chosen twice (the assignment editor, `BreakLock`), so it
was not a judgement call at all.

## Live state (volatile - verify on resume)

- **Background jobs:** none. The throwaway daemon this session ran was stopped and
  its scratch state is in the session scratchpad, not in the repo.
- **PRs:** none, ever. Boris pushes `main` directly.
- **Git:** `main` clean, **5 commits ahead of `origin/main`** (`943258b`), nothing
  pushed. Boris pushed between sessions 57 and 58, so everything before FR97 is
  already on gitlab. This session's five are local to this machine; boris-vm mirrors
  through git, so a resume there reads stale docs until he pushes again.
- **Deployed daemon:** `a3e570de160d`, running under `agentbox.service` on Boris's
  own state, verified by asking the daemon rather than reading the file. It is the
  newest commit that changed code.
- **Desktop:** released. Control was held for the interrupted drive and given back.
- **In-flight edits:** none.

## Blocked on you (Boris)

1. **Whether `docs/backlog/features.md`'s one non-goal revision is on.** Carried
   unchanged from sessions 56 and 57: B-1, "away without becoming a cloud service",
   argues for revisiting "no remote or mobile delivery in v1" as a relay running as
   a subprocess you chose. It needs the vision's non-goal 3 amended in public the
   way ADR-0009 amended principle 6. That is a principle change and therefore yours.
   Ranked 6th in features.md, highest absolute value in that file.
2. **Whether the new Esc notice earns its place.** Esc on the only card now says
   "nothing else is waiting, so there is nothing to move this behind" instead of
   doing nothing at all. The card's header promises `Esc defer`, so the silence was
   the defect - but this is the one refusal you will meet daily, and only using it
   settles whether it reads as helpful or as nagging. It is one string in
   `internal/daemon/daemon.go` (`Defer`) if the answer is no.

## The live check, exactly

The tests pin U-01 and U-02; a screen has not. The recipe, which is session 57's and
is in CLAUDE.md:

```bash
make build
make stop                                   # takes Boris's daemon down - hold the
                                            # desktop first, and SAY what will appear
S=/tmp/agentbox-check; mkdir -p $S/state $S/config/agentbox
cp ~/.config/agentbox/config.toml $S/config/agentbox/
XDG_STATE_HOME=$S/state XDG_CONFIG_HOME=$S/config setsid ./agentbox daemon &
XDG_STATE_HOME=$S/state XDG_CONFIG_HOME=$S/config \
  ./agentbox ask --title "Where should 2026.7.30 go first?" --option staging --option canary &
python3 tools/uidrive/uidrive.py where      # origin + size of the card
python3 tools/uidrive/uidrive.py keys Escape
python3 tools/uidrive/uidrive.py shot /tmp/after.png
```

What to look for, in order of what would actually be wrong:

- **Esc on the only card** paints the amber line ("nothing else is waiting..."), and
  the window GROWS to fit it. A notice clipped off the bottom of a frameless window
  is the same defect it was written to fix, and jsdom cannot answer this.
- **The × on the notice** puts it away and leaves the question answerable.
- **Answering** (press `1`) clears the notice and hands `staging` back to the caller.
- **The toast**: raise a notify, dismiss it with the daemon stopped underneath, and
  the strip should say so rather than sit there looking unclicked.

Then `XDG_STATE_HOME=$S/state ./agentbox quit`, `make deploy`, `make deployed`.

**The lesson from the interrupted attempt, which is worth more than the check.** A
card from a scratch daemon is indistinguishable from a real one on screen. Holding
the desktop was not enough: Boris saw a card asking about staging or canary, could
not tell it from his own queue, and reasonably asked whether the session was stuck.
Say what is about to appear on the screen, not only that something will.

## I can do solo (no input needed)

1. **The live check above.** First, and it is short.
2. **FR99, the drawn wiki frames.** New this session and asked for directly.
   `tools/wiki/shots.py` and `tools/wiki/SHOTS.md` are what changes; the four
   outstanding shots (viewer scroll, the unplaced progress and history-stats frames,
   the two remaining `SHOT:` placeholders in `is-it-safe.md:18` and
   `settings.md:51`) are all things a drawn frame answers trivially.
3. **Robustness band A**, thirteen items, the rest of tier 1. R-15, R-03 and R-14
   are the hours-sized ones to start with.
4. **More surface tests.** Fifteen of seventeen surfaces still mount nothing. R-40's
   fixes (2) and (3) are open: a hostile payload against the card, and executing
   `buildDocument` to assert the CSP and sandbox attributes that today are only
   checked as string constants.
5. **U-05**, hours: `theme.motion = "reduced"` is honoured by four components of the
   thirteen that animate, because the global rule in `app.css` is scoped to `"none"`.
6. **R-30**: the review board's file jail is lexical, so a symlink reads any file
   into a review. Band C, but the only security-shaped item in the three documents.

## Facts - verified vs assumed

- [verified] **U-02 is deployed and its refusals are readable.** 13 daemon tests,
  one per reason a refusal can happen, asserting a sentence rather than the exact
  words: lowercase, ends in a full stop, 20 characters or more. `make check` passed
  in full and `make deploy` ran through to `deployed`.
- [verified] **U-01's tests can fail.** All 15 passed on the first run, which proves
  nothing. `note()` was neutered to a no-op and 13 of them failed; the 2 that still
  passed are the negative controls ("an answer that lands says nothing"). This is
  the check to repeat on any test written for a defect you cannot see.
- [verified] **The deployed build is `a3e570de160d`**, asked of the running daemon.
- [verified] **Boris's store was never touched.** The interrupted drive ran against
  `XDG_STATE_HOME` in the session scratchpad with his config copied for the theme.
  His daemon was stopped for about four minutes and restored by `make deploy`.
- [verified] **The whole Go suite and the whole vitest suite pass** (33 passed, 1
  expected fail - U-06's, which fails on purpose until U-06 is fixed).
- [assumed] **That U-01 and U-02 look right on screen.** Nobody has seen either. This
  is the one thing this session owes and it is item 1 above.
- [assumed] **That the Esc notice is wanted.** It is a real behaviour change on the
  most common keystroke on the surface and Boris has not seen it yet.
- [assumed] The carried set from sessions 50, 51, 55 and 57, none of which this
  session looked at: the artifact-restart-while-streaming edge; the 30-minute fuse in
  a live daemon; the demoted marker on a second monitor; `perform.py`'s fullscreen
  check on two monitors; the domain drawer animation; the demo fallback painting; a
  detail holding a mermaid diagram; `provisionalFor` retiring a hook-only row; the
  200-key prefix cap; `@me` in a shared key; a real client's `await_signal` parked
  past 20s; the two lock warnings; and the identity cross-check's no-node skip.

## What this session changed

| Commit | What |
|---|---|
| `228a5f6` | FR98, found uncommitted in the tree from 2026-08-08 and committed as its own change |
| `63373b2` | U-02: the answer path can say it refused. Resolver, Source.Promote, Bridge, lateResolver; 19 tests |
| `a3e570d` | U-01: the card can say an answer did not land. One wrapper in bridge.js, lib/trouble.svelte.js, four surfaces; 15 tests |
| `b559b75` | FR99, the wiki's pictures should be drawn rather than photographed |
| this one | ux.md and backlog/README.md marked, session log, this handoff |

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| The `.catch`-per-call-site option for U-01 | Rejected in writing, in `bridge.js`'s own comment above `answering`: 26 sites is 26 chances to forget one, and `.catch(() => {})` turns a silent failure into a silent failure that passes review. The reasoning is what stops it coming back |
| U-01 and U-02's entries in `ux.md` | Nothing removed. Both keep their full text and gained a fixed marker, deliberately: the entry is the argument for why the shape is what it is |
| The handoff's "shots still outstanding" list from session 57 | Superseded rather than lost. All four are now reasons FR99 exists and are named in it; the retake specification stays in `tools/wiki/SHOTS.md` |

## Map

1. [HANDOFF.md](HANDOFF.md) - this file.
2. **[docs/backlog/README.md](docs/backlog/README.md) - read this first.** One order
   across all three audits. Tier 1's ux half is now closed; band A is what is left.
3. [docs/backlog/ux.md](docs/backlog/ux.md) - 15 surface items. U-01, U-02, U-03 fixed.
4. [docs/backlog/robustness.md](docs/backlog/robustness.md) - 45 items in six bands.
   R-01, R-02 fixed; R-40 started.
5. [docs/backlog/features.md](docs/backlog/features.md) - eleven extensions, five
   bets, ranked. B-1 is the one waiting on Boris.
6. [docs/07-field-requests.md](docs/07-field-requests.md) - FR99 is the newest and is
   a direction, not a wish.
7. [docs/wiki/FACTS.md](docs/wiki/FACTS.md) - **read before quoting any number about
   the product.**
8. `frontend/test/failure.test.js` - U-01's tests, and the neutering check that made
   them worth having. `frontend/test/stubs/wailsio-runtime.js` is the seam: its
   per-method `byName` map is what lets one call reject while the rest work.
9. `internal/daemon/refusal_test.go` - U-02's, one test per reason a refusal exists.
10. [docs/history.md](docs/history.md) - session 58 is at the top.
11. `~/me/study/guides/writing/ai-tells/` - outside this repo: the prose rules and
    `scan-tells.py`.
