# Handoff - AgentBox: band A halved, and the wiki's frames are drawn now

*Written by session 59, which took the one thing session 58 owed - nobody had looked
at U-01 or U-02 on a screen - did it, and found a new defect doing it (U-16). Then
FR99 (the wiki's frames are drawn rather than photographed) and six robustness
band-A items: R-03, R-04, R-05, R-08, R-11, R-14. All deployed. Band A is down from
fifteen to seven.*

**Written:** 2026-08-09 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean, in sync with origin/main (gitlab)
make deployed               # expect c306e5e5a972 - see the note below, this is
                            # NOT the newest commit and that is correct
agentbox pending            # expect nothing pending
```

**On the deployed build.** `make deployed` reports `c306e5e5a972` while HEAD is
`5622237`. That is not drift: the two commits after `c306e5e` changed
`tools/wiki/draw.py` and docs only, nothing that goes in the binary. Redeploy if you
prefer them equal; nothing depends on it.

**Budget.** Boris raised the cap from 70% to 75% of the weekly quota for this
session and it ended at ~72%, so there is room. Check
`claude -p /usage | grep -E '^Current '` before starting; the week resets Aug 12,
4:59am (Asia/Jerusalem).

1. **Robustness band A, seven left**: R-06, R-07, R-09, R-10, R-12, R-13, R-15, in
   `docs/backlog/robustness.md`'s own order. **R-12 and R-13 are the hours-sized
   ones** and are the place to start; R-06, R-07, R-09 and R-10 are a day each, and
   R-15 needs a live MCP-host repro before it can be fixed at all.
2. **U-16 wants one more repro** before its fix is chosen. It is the only ux band-A
   item left and the entry says exactly what to do.
3. **The remaining eleven wiki frames.** The mechanism is finished and two frames
   are drawn; each of the rest is a `frames.js` entry. `tools/wiki/DRAW.md` is the
   runbook.

## Where we are

Session 57 wrote the audits and the one order across them. 58 executed the top of
it. This one finished what 58 owed and then took six more.

The through-line, now six sessions old, held again twice and in a new form. **A
document asserted something and the code contradicted it** - this time
`docs/backlog/ux.md` claiming "nothing here was found by looking at a running
window" (U-16 was), and `docs/wiki/DESIGN.md` saying the shared-values block sits
below the agent rows when `Agents.svelte` has always put it above. Both are
corrected in place rather than quietly fixed.

## Live state (volatile - verify on resume)

- **Background jobs:** none. A `vite` server leaked from an early `draw.py` run and
  was found still up an hour later; that is fixed (`5622237`) and verified.
- **PRs:** none, ever. Boris pushes `main` directly.
- **Git:** `main` clean. **Pushed to `origin` (gitlab), which is main's upstream**,
  through `33d951b`. **`5622237` is one commit past that and is NOT pushed** - push
  it. The `github` remote is a mirror and was **not** pushed this session: the
  permission classifier blocked it, so it needs Boris or a permission rule.
- **Deployed daemon:** `c306e5e5a972`, asked of the running daemon. See the note
  above on why that is behind HEAD.
- **Desktop:** released. Held twice, both released.
- **In-flight edits:** none.

## Blocked on you (Boris)

1. **Whether `docs/backlog/features.md`'s one non-goal revision is on.** Carried
   unchanged from sessions 56, 57 and 58, and nobody has looked at it: B-1, "away
   without becoming a cloud service", wants the vision's non-goal 3 amended in
   public the way ADR-0009 amended principle 6. A principle change, therefore
   yours. Ranked 6th in features.md, highest absolute value in that file.
2. **Whether the Esc notice earns its place.** Carried from 58 and now *seen*: Esc
   on the only card paints "nothing else is waiting, so there is nothing to move
   this behind." It reads well on screen and the window grows to fit it, so the
   question is no longer whether it works but whether you want it daily. One string
   in `internal/daemon/daemon.go` (`Defer`) if the answer is no.
3. **Whether the agents board should lead with shared values.** New. The surface
   puts the blackboard above the agent rows and always has; DESIGN.md said the
   opposite and is now corrected to match the code. Whether the code is *right* is
   a design call and is yours - the frame `s2` is drawn either way.
4. **The `github` mirror push** (see Live state).

## I can do solo (no input needed)

1. **R-12 and R-13**, the two hours-sized band-A items left.
2. **U-16's second repro**, then its fix.
3. **The eleven remaining wiki frames**, a fixture each.
4. **R-06, R-07, R-09, R-10**, a day each.
5. **R-40's open fixes (2) and (3)**: a hostile payload against the card, and
   executing `buildDocument` to assert the CSP and sandbox attributes that today are
   only checked as string constants.
6. **U-05**, hours: `theme.motion = "reduced"` is honoured by four components of the
   thirteen that animate, because the global rule in `app.css` is scoped to `"none"`.

## Facts - verified vs assumed

- [verified] **U-01 and U-02 on a real desktop.** Esc on the only card paints the
  amber line in full; the window grows 470x199 to 470x241 and recentres, so it is
  not clipped; the `x` puts it away and the window shrinks back to 199; answering
  with `1` clears it and hands `staging` to the caller. Screenshots were read, not
  just captured.
- [verified] **U-16's behaviour, twice, from X rather than by inference.** The first
  card of a fresh daemon took focus and its whole keymap worked; a second raised
  seconds after the first closed did not (`xdotool getwindowfocus` named the
  terminal), `1` did nothing twice, and the same `1` answered after one click.
- [verified] **All six robustness fixes pass, and their tests can fail.** Every one
  was checked by neutering the fix: R-14's file and R-03's countdown test hang to
  the 120s test timeout when neutered, which is the defect rather than a proxy for
  it. `make check` exits 0 (gofmt, vet, race tests, 43 vitest passing + 1 expected
  fail).
- [verified] **R-05 seen on a real card** in the live queue on the deployed build,
  answered, answer returned. Not a scratch daemon.
- [verified] **The drawn frames are byte-identical across runs**, and redrawing the
  card after R-05 gave the same md5 - a real browser through the changed mount path.
- [verified] **`draw.py` no longer leaks its vite server**, checked by process list
  after a run.
- [assumed] **Whether U-16 is "always" or "sometimes".** n=1 each way, minutes
  apart, and the two argue for different fixes. This is the one thing to settle
  before touching it.
- [assumed] **One `make check` run failed once and never again.** Three subsequent
  full runs and `-race -count=3/4` over the three packages this session touched were
  all clean, so it is not attributable to this work - but it was seen, and a second
  sighting would matter.
- [assumed] The carried set from sessions 50, 51, 55 and 57, none of which this
  session looked at: the artifact-restart-while-streaming edge; the 30-minute fuse
  in a live daemon; the demoted marker on a second monitor; `perform.py`'s
  fullscreen check on two monitors; the domain drawer animation; the demo fallback
  painting; a detail holding a mermaid diagram; `provisionalFor` retiring a
  hook-only row; the 200-key prefix cap; `@me` in a shared key; a real client's
  `await_signal` parked past 20s; the two lock warnings; and the identity
  cross-check's no-node skip.

## Two traps this session paid for

- **`request_control` cannot protect a run that stops the daemon.** The HANDS OFF
  strip is a window the daemon draws, so `make stop` takes it down: control is
  granted and the one signal saying so vanishes exactly when the driving starts.
  Boris saw a card he did not raise, with nothing on screen explaining it, and
  reasonably asked whether the session was stuck. `set_activity` answering
  `{"live":false}` is how you know the strip is already gone. In CLAUDE.md.
- **A commit message with an AI tell is refused, and the refusal kills the whole
  Bash call.** `harness` as a verb is one of the banned words. A
  `python3 <<PY ... PY && git add && git commit` one-liner leaves the *file edit*
  undone too - the symptom is a file that looks like the edit never applied, because
  it did not. In CLAUDE.md.

## What this session changed

| Commit | What |
|---|---|
| `5900934` | U-01 and U-02 marked seen on screen; U-16 written up; the CLAUDE.md strip trap |
| `13fc41d` | FR99: `tools/wiki/draw.py` + `frontend/draw/`, and `card.png` drawn |
| `b25112d` | the agents board drawn, and the DESIGN.md spec it contradicted corrected |
| `2d9d7ff` | R-14: the tools that promise not to block are bounded |
| `f998a1e` | R-03: an undo past the deadline no longer strands the caller |
| `cec53eb` | R-04: an oversized item is refused by name, not by a dropped connection |
| `6bb45c9` | R-05: the card and toast pull their view on mount |
| `d6f0dd6` | R-05 seen on a real card; the frames as a regression check |
| `d597259` | session 59 in history.md; the commit-guard trap |
| `4d89618` | R-11: one unreadable row no longer stops the daemon starting |
| `c306e5e` | R-08: a graced answer ships on shutdown |
| `33d951b` | STATUS.md brought current (it was session 53's) |
| `5622237` | `draw.py` stops the vite server it started |

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| Session 58's "the live check, exactly" recipe | Done, so the recipe is spent. What it taught survives as two things: the CLAUDE.md trap about the strip, and R-05's entry recording that raising one card into the live queue needs no daemon swap at all - which is the cheaper shape for most future checks |
| The claim in `ux.md` that nothing was found by looking at a running window | Not deleted. Amended in place with what changed and why it matters, because the sentence was an argument about the audit's own reach and U-16 is the counter-example |
| Session 58's "four shots still outstanding" | Superseded, not lost: they are named in `tools/wiki/DRAW.md` under what the drawing does not do yet, and FR99 in `docs/07-field-requests.md` |
| The 4 MB literal duplicated in two scanners | Now `proto.MaxLineBytes`, with the reasoning about why raising it is the wrong fix attached to the constant |

## Map

1. [HANDOFF.md](HANDOFF.md) - this file.
2. **[docs/backlog/README.md](docs/backlog/README.md) - read this first.** One order
   across all three audits, current as of today. Tier 1 is seven robustness items
   plus U-16.
3. [docs/STATUS.md](docs/STATUS.md) - current state, updated today.
4. [docs/backlog/robustness.md](docs/backlog/robustness.md) - 45 items in six bands.
   R-01, R-02, R-03, R-04, R-05, R-08, R-11, R-14 fixed; R-40 started.
5. [docs/backlog/ux.md](docs/backlog/ux.md) - 16 surface items. U-01, U-02, U-03
   fixed; **U-16 is the new one and the only band-A item left**.
6. [docs/backlog/features.md](docs/backlog/features.md) - B-1 waits on Boris.
7. [tools/wiki/DRAW.md](tools/wiki/DRAW.md) - how the frames are drawn, what keeps a
   drawing honest, and which frames should still be photographed.
8. [docs/07-field-requests.md](docs/07-field-requests.md) - FR99 carries what
   building it turned up.
9. [docs/wiki/FACTS.md](docs/wiki/FACTS.md) - **read before quoting any number about
   the product.**
10. [docs/history.md](docs/history.md) - session 59 is at the top.
11. Tests worth reading before writing more: `internal/mcp/deadline_test.go` (a
    daemon that accepts and says nothing), `internal/store/corrupt_test.go` (a blob
    written directly), `frontend/test/mount-pull.test.js` (mount with no push at
    all). Each was verified by neutering its fix.
