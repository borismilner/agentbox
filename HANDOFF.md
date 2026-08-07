# Handoff - AgentBox: the surfaces have an audit, an order, and their first test

*Written by session 57, which cleared session 56's whole solo list: the owed deploy,
the UX audit, the merged backlog, the install shot, and the first test in this
project's history to mount a Svelte component. Then it fixed one item off its own
new backlog, drove it on the real desktop, and shipped it. Nothing is owed.*

**Written:** 2026-08-07 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean, ~25 ahead of origin/main. origin is
                            # gitlab and is main's upstream; `github` is a mirror
make deployed               # expect 5485842cc01a. HEAD is a couple of commits past
                            # it and every one of them is docs, so no deploy is owed
make check                  # gofmt + vet + race + node + vitest, ~4 min
agentbox pending            # expect nothing pending
```

**No deploy is owed.** The deployed daemon is `5485842cc01a`. Every commit after
it is docs, wiki images or the screenshot harness - nothing that changes what the
daemon runs. Verify with `make deployed` rather than by comparing to HEAD. So:

1. **The screenshots are done.** Ten are live on both wikis and seven pages that
   carried a `SHOT:` placeholder now carry an image. What is left is smaller and is
   listed under "the shots still outstanding" below.
2. **Then tier 1 of [docs/backlog/README.md](docs/backlog/README.md)**, which is the
   new file that says what to build in what order across all three audits. U-02 is
   the one to take first: it is a day, and it is what makes U-01 worth building.

## Where we are

Session 56 built the wiki and three audits were commissioned; two landed and the UX
one died on a session limit. This session wrote it, then wrote the thing that was
missing above all three: one order across them. Then it built the test harness that
the audit's own worst finding says the project cannot see without, and used it to
fix one item.

The through-line worth carrying: **three of this session's findings were things a
document asserted and the code contradicted.** That is now four sessions running.

## Live state (volatile - verify on resume)

- **Background jobs:** none.
- **PRs:** none, ever. Boris pushes `main` directly.
- **Git:** `main` at `03f66ad`, clean, **25 commits ahead of `origin/main`**. Nothing
  is pushed. Ten commits are this session's.
- **Deployed daemon:** `5485842cc01a`, running under `agentbox.service` on Boris's
  own state, verified by asking the daemon. That is the last commit that changed
  code; everything after it is documentation, so the daemon is current even though
  it is not literally HEAD.
- **The wiki is live and level** at https://gitlab.com/fu-bar/agentbox/-/wikis/home
  and https://github.com/borismilner/agentbox/wiki. This session published
  `install.md` plus ten screenshots, and verified every image serves on both hosts
  at matching byte counts. Never edit a page in either browser: the next publish
  overwrites it.
- **In-flight edits:** none.

## Blocked on you (Boris)

**One thing.** The other two were settled during the session: vitest is confirmed as
the test runner ("vitest is fine, keep it"), and U-03 was exercised and deployed.

1. **Whether `docs/backlog/features.md`'s one non-goal revision is on.** Carried
   unchanged from session 56: it argues for revisiting "no remote or mobile delivery
   in v1" as a relay running as a subprocess you chose rather than as a cloud
   service, and asks that the vision be amended visibly the way ADR-0009 amended
   principle 6. That is a principle change and therefore yours. It is ranked 6th in
   features.md and holds the highest absolute value in that file.

### The shots still outstanding

None of these blocks anything, and all of them need the desktop again.

- **`viewer.png`** was captured but is scrolled to the alert callouts instead of the
  table, mermaid diagram and code block its spec asks for. Retake with
  `--only S8 --viewer-scroll N`, tuning N. No page references it yet.
- **`progress.png`** and **`history-stats.png`** are good and no page asks for them.
  Someone has to decide where they go: DESIGN gives `notifications.md` both the toast
  and the progress window while the rhythm rule allows one image per page.
- **`is-it-safe.md:18`** and **`settings.md:51`** still hold placeholders. Their
  shots - the secret card mid-entry, and Appearance with an unsaved change - are not
  in the plan at all and would need adding to `shots.py`.
- **Known gaps in what did ship**, for whoever retakes: the agents board shows seven
  roster rows where DESIGN wants four, which cannot be fixed from the staging side
  because `sync lock` mints its own session key on purpose, so every lock is a row.
  All four chips are in frame, which is the other half of what DESIGN asks. The
  hands-off pair sits over the review board rather than an editor. The artifact's bar
  has the interactive badge and the runtime but no preview/code toggle.

## I can do solo (no input needed)

1. **Tier 1 of the merged backlog**: robustness band A, thirteen items left of
   fifteen, plus U-01 and U-02. About three weeks of work, so a session takes a
   slice. R-15, R-03 and R-14 are the hours-sized ones to start with.
2. **U-02** (the Go answer path returns nothing, so a refusal cannot reach a
   surface). It is the precondition for U-01 and it is a day. Doing it makes U-01
   worth building, and U-01 is the single worst UX finding.
3. **More surface tests.** The rig exists and fifteen of seventeen surfaces have
   none. R-40's fixes (2) and (3) are still open: a hostile payload against the card,
   and executing `buildDocument` to assert the CSP and sandbox attributes that today
   are only checked as string constants.
4. **U-05**, hours: `theme.motion = "reduced"` is honoured by four components out of
   the thirteen that animate, because the global rule in `app.css` is scoped to
   `"none"`. Fixable in one CSS rule.
5. **R-30**, which band C holds but which is the only security-shaped item in the
   three documents: the review board's file jail is lexical, so a symlink reads any
   file into a review.

## Facts - verified vs assumed

- [verified] **The deploy landed, twice.** `make deployed` reports `5485842cc01a`,
  asked of the running daemon rather than read off the file. The first deploy of the
  session cleared session 56's debt (`74b350326153`); the second shipped U-03. It is
  the newest commit that touched code.
- [verified] **`make check` passes in full**, twice, including the new `test-svelte`
  stage. Not by reading the tail: `check` is a prerequisite of `deploy-locked`, and
  `make deploy` exited 0 through to the install. This closes session 56's `[assumed]`.
- [verified] **The card's height is measured, not estimated.** `Card.svelte:143-177`,
  a ResizeObserver against real layout. Session 56's handoff said the opposite and
  STATUS carried the same claim under "known gaps"; both are corrected. The dead
  agent's last words were right.
- [verified] **U-06 is real, and was checked rather than argued.** A probe showed the
  re-measure count unchanged across the shrink. The first version of that test said
  the opposite, because it counted the card's mount-time measurement as a response to
  the change under test. `settle()` in `frontend/test/card.test.js` is the fix, and
  the episode is the best argument in the repo for tier 3.
- [verified] **`daemonUp` is never assigned.** Grep over the whole tree finds the
  declaration and one reader. The status strip's "daemon up" has no condition at all.
  Note the honest limit, which U-04 states: the window dies with the daemon, so this
  is a health indicator that cannot indicate rather than a lie you can catch on
  screen.
- [verified] **The wiki image serves on both hosts**, 200 and 18722 bytes each.
- [verified] **R-01 and R-02 were already fixed** by `1d00fd2` and `robustness.md`
  still listed them as open. Both now carry a fixed marker. Without that the merged
  backlog would have opened by recommending two finished pieces of work.
- [verified] **U-03 looks right on screen.** Driven over XTEST against a daemon on
  throwaway state: `y` on a choice row gave the amber `y does nothing to this one`,
  `j` retired it and showed the next row's own hint, `1` answered and returned
  `staging` to the caller with nothing refused, `n` refused again afterwards. Boris's
  own store was never touched - zero demo items in his history.
- [verified] **The inbox's pixels lag its store.** A screenshot two seconds after the
  answering keystroke still showed both rows waiting; the daemon coalesces queue
  pushes. Nothing is wrong, but a shot of this surface is not a reading of it, which
  matters for the screenshot sitting.
- [assumed] **That the twelve staged screenshots come out.** `shots.py` has 82 tests
  and has never touched a desktop. Unchanged from session 56.
- [assumed] **That the artifact-restart-while-streaming edge is still open.** Nobody
  has watched it. Unchanged from session 56, and still cheap to close.
- [assumed] The carried set from sessions 50, 51 and 55, none of which this session
  looked at and all of which are therefore exactly as unverified as they were: the
  30-minute fuse in a live daemon; the demoted marker on a second monitor;
  `[flood]`'s defaults for ordinary work; `perform.py`'s fullscreen check on two
  monitors; the domain drawer animation; the demo fallback painting; a detail holding
  a mermaid diagram; `provisionalFor` retiring a hook-only row; the 200-key prefix
  cap; `@me` in a shared key; a real client's `await_signal` parked past 20s; the two
  lock warnings; and the identity cross-check's no-node skip.

## What this session changed

| Commit | What |
|---|---|
| `1734fe3` | `docs/backlog/ux.md` - fifteen items in six bands, the audit session 56 owed |
| `37d10bf` | `docs/backlog/README.md` - one order across all 76 items; R-01/R-02 marked fixed |
| `d9a3575` | `install.md`'s shot, from real `doctor` output, published to both hosts |
| `1650275` | vitest + jsdom, and `Card.svelte` mounted for the first time (10 tests) |
| `c29df92` | session log, and the card-height claim corrected in STATUS |
| `9deff91` | U-03 fixed: the inbox says so when a triage key does nothing (8 tests) |
| `b690530` | session log for U-03 |
| `f842bef` | U-03 exercised on the real inbox and deployed; vitest confirmed |
| `1468cb9`, `98ee3e7`, `0566035` | eight defects in `shots.py`, found by running it |
| `1b07237` | the twelve screenshots, and seven pages that were asking for one |

## Map

1. [HANDOFF.md](HANDOFF.md) - this file.
2. **[docs/backlog/README.md](docs/backlog/README.md) - read this first.** One order
   across all three audits, and the rule that produced it. New this session.
3. [docs/backlog/ux.md](docs/backlog/ux.md) - the surfaces, 15 items. New this session.
4. [docs/backlog/robustness.md](docs/backlog/robustness.md) - 45 items in six bands.
   R-01, R-02 fixed; R-40 started.
5. [docs/backlog/features.md](docs/backlog/features.md) - eleven extensions, five
   bets, ranked.
6. [docs/wiki/FACTS.md](docs/wiki/FACTS.md) - **read before quoting any number about
   the product.** Audited from source, with the line behind each claim.
7. [docs/STATUS.md](docs/STATUS.md) - current state. Two entries corrected this
   session; treat the rest with the suspicion four sessions of drift have earned.
8. `frontend/test/` - the surface tests, and `frontend/vitest.config.js`, which
   explains the two boundaries (why they live outside `src`, and why
   `@wailsio/runtime` is aliased rather than `bridge.js` mocked).
9. [tools/wiki/SHOTS.md](tools/wiki/SHOTS.md) - the screenshot runbook. Its top
   section now lists two outstanding shots rather than three, and records exactly how
   the third was made.
10. [docs/history.md](docs/history.md) - the session log; session 57 is at the top.
11. `~/me/study/guides/writing/ai-tells/` - outside this repo: the prose rules and
    `scan-tells.py`. Both new backlog files were scanned; the only remaining hits in
    `ux.md` are four literal quotations of glyphs the interface itself prints.
