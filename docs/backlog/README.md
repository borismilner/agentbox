# The backlog, in one order

Three audits produced three files, each ordered inside itself and none of them
ordered against the others. This is the missing judgement: one sequence across all
seventy-six items, and the rule that produced it.

> **In a hurry.** Finish robustness band A (thirteen items left, about three weeks),
> taking U-01, U-02 and U-03 with it because they are the same failures seen from
> the surface. Then build the test harness, R-40. Then F-01. Everything else waits,
> and B-1 waits on Boris rather than on capacity.

## The three files

| File | What it audits | Items | Ordered by |
|---|---|---|---|
| [robustness.md](robustness.md) | the daemon, the store, the socket, the desktop layer | 45, in six bands | what the user loses |
| [ux.md](ux.md) | the seventeen Svelte surfaces | 15, in six bands | what the user loses |
| [features.md](features.md) | what AgentBox does not do yet | 11 extensions, 5 bets | value over cost |

## The rule

The product's claim is that a message from an agent to a human is never lost. So
the sequence is: **first stop losing messages, then be able to see that you have
stopped, then be worth more.** In three clauses:

1. **A message that cannot be delivered or answered outranks everything.** That is
   robustness band A and ux band A, and the two interleave rather than queue - they
   are frequently the same defect from opposite ends. U-02 is literally the other
   half of R-01: the daemon could not report a refusal, so the surface could not
   show one.
2. **Then the ability to find out.** R-40 is the highest-ranked item that fixes no
   defect at all. Nothing in the repo executes a single line of Svelte, so eleven of
   the fifteen ux items and five of the robustness ones cannot be caught by anything
   except somebody reading the file, which is how they were caught this time and is
   not a process. Until it exists, every surface change ships on one person looking
   at one screen on one machine.
3. **Then value.** features.md's own ranking holds from there, unchanged.

Cost breaks ties inside a clause and never across one. A three-week band-A fix
still precedes a four-day feature.

### Why every band-A item outranks F-01

This is the question the three files could not answer separately, so it gets a
direct answer. F-01 is correctly first in features.md and it is genuinely the best
feature here: four days, no new principle, and the first tool that changes what an
agent *does* rather than what a screen shows. It still ranks below the last band-A
item.

F-01 makes an agent cleverer about *whether* to interrupt. Band A is about
interruptions that cannot be answered once made. A better decision to ask is worth
nothing when the asking is the part that breaks, and it is worth less than nothing
if it teaches agents to trust a channel that drops answers. Build the channel,
then make agents smart about using it.

The same argument does not extend to band B. Band B is the hub failing to serve,
which is bad and loud; band A is failing quietly while claiming success. F-01 could
reasonably be argued above band B. It is placed below it here because band B is
mostly hours-sized and there are only eleven items of it, so the delay it imposes
on F-01 is two weeks and not two months.

## The order

**Tier 0 - done.** R-01 and R-02, fixed and deployed in `1d00fd2` on 2026-08-07,
each with its reproduction kept as a test. Both entries stay in robustness.md with
a fixed marker, because the reasoning is what stops them coming back.

**Tier 1 - the claim is false while these stand.** Robustness band A, the thirteen
remaining of fifteen, plus the three ux band-A items. About three weeks together. Take them in
robustness.md's own order, which is already consequence-ordered, with these three
folded in:

- **U-01** (the card has no failure path, in any of its 26 calls) next to R-05,
  because both are the card being unable to say what is happening to it.
- **U-02** (the Go answer path returns nothing) early, because it is the
  precondition for U-01 being worth building: a card that can display a failure
  still needs to be told about one. It is also the reason R-01 was invisible for as
  long as it was.
- **U-03** (the inbox discards the one answer the daemon gives it) immediately
  after U-02, since it is hours once the return values exist.

**Tier 2 - the hub stops serving.** Robustness band B, eleven items, R-16 to R-26.
Loud failures, mostly hours each. R-26 (no bound on anything an agent puts in a
card) is the one to take first: it is the only band-B item an agent can trigger by
accident on an ordinary day.

**Tier 3 - the ability to find out.** R-40, plus the three additions ux.md's U-15
names: an a11y checker, a contrast check over the token sets, and a keymap
inventory checked against the hints each surface renders. A week for the rig and
its first three tests, a day each for the additions.

Ranked here and not lower because everything after this point is new surface, and
new surface built without it inherits the same blind spot. Ranked here and not
higher because a test that proves a broken thing is broken is worth less than
fixing it, and band A is fully understood already.

The first test should be U-06's, not R-40's own suggestion. The card's fit height
is a number, assertable in both directions, and U-06 hands it two known-wrong cases
to pin on day one.

**Tier 4 - the first features.** features.md's ranking, unmodified: F-01, then
F-09, F-02, F-11, F-05. F-01 and F-02 ship as a pair for the reason features.md
gives - an agent that decides not to ask must leave a trace.

**Tier 5 - the rest, interleaved by band and cost.** Robustness bands C, D and E
(R-27 to R-39) against ux bands B through E (U-04 to U-14) against features.md's
items 7 to 16. Three notes on that interleaving:

- **U-05** (`theme.motion = "reduced"` is honoured by four components of thirteen)
  is hours and fixes a setting that currently misleads. Do it whenever something
  else touches `app.css`.
- **U-09, U-10 and U-11** are three ways to lose work by pressing the obvious key,
  and they are hours each. They are worth more than their band suggests, because
  each one is a thing Boris will hit personally.
- **R-30** (the review board's file jail is lexical, so a symlink reads any file)
  is in band C but is the only security-shaped item in the three documents. Treat
  it as tier 2.

**Blocked on Boris, not on capacity.** B-1, "away without becoming a cloud
service". It is ranked 6th in features.md and holds the highest absolute value in
that file, and it cannot be scheduled by a session because it needs the vision's
non-goal 3 amended in public, the way ADR-0009 amended principle 6. features.md
argues the case at its "non-goal I would revisit". That is a decision, not a task.

## What this order costs

| Tier | Items | Rough cost |
|---|---|---|
| 1 | 13 robustness + 3 ux | three weeks |
| 2 | 11 robustness (+ R-30 pulled up) | two weeks |
| 3 | R-40 and the three checks U-15 adds | two weeks |
| 4 | F-01, F-09, F-02, F-11, F-05 | four weeks |
| 5 | everything else | months, and re-rank before starting |

Roughly eleven weeks to the end of tier 4. Re-rank at the end of tier 3 instead of
following this file to the bottom. Tier 3 exists precisely because the project
cannot currently see its own surfaces, so the first thing it will produce is a
changed list.

## Cross-reference

Where the same defect appears in two files:

| Pair | Relationship |
|---|---|
| R-01 and U-02 | one defect. R-01 is the daemon half and is fixed; U-02 is the shape of the call that hid it, and is not |
| R-05 and U-01 | the card cannot show its own state, from the pull side and the failure side |
| R-40 and U-15 | one item. U-15 adds the three checks R-40 does not name and does not restate the rest |
| R-26 and U-06, U-07 | agent-authored content sizing a surface; R-26 is the bound, the ux items are the measurement |
| B-2 and F-08 | features.md says it: F-08 is the measurement layer under B-2 |

## Keeping this file honest

It goes stale the moment anything ships. Two habits:

- When an item is fixed, mark it in its own file the way R-01 and R-02 are marked,
  and correct the tier table here. R-01 and R-02 sat unmarked for a day and this
  file would have opened by recommending two finished pieces of work.
- The three audits were each taken at a commit and say so. When one is re-run,
  re-run this too, because the ordering rule is stable but the contents are not.
