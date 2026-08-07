# Handoff - AgentBox: the wiki is live, and twenty-two doc claims were wrong

*Written by session 56, which built the public wiki and, because Boris said to read
the code and not the documents, found that the documents disagreed with the code in
twenty-two places. A deploy is owed and the UX backlog is half-built.*

**Written:** 2026-08-07 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean, and AHEAD of origin/main with nothing
                            # pushed. origin is gitlab and is main's upstream; the
                            # `github` remote is a mirror and far behind
make deployed               # expect 69230d4f7e32, which is NOT HEAD - see below
make check                  # gofmt + vet + race + JS, ~4 min
agentbox pending            # expect nothing pending
```

**A deploy IS owed, unlike last session.** `1d00fd2` fixes two daemon defects and
the running daemon is still `69230d4f7e32` from 2026-08-06, so neither fix is live
on Boris's own machine. Commit first is already done, so:

```bash
make deploy                 # takes the flock, ends by running `deployed` itself
```

Then, in priority order, the three things this session left:

1. **Redo the UX audit.** It is the one deliverable Boris asked for that does not
   exist. Its agent died on a session limit partway through, and its last words are
   worth keeping: *"Card measures itself via ResizeObserver, so the height story is
   more subtle than stated."* That contradicts the premise it was given (that a
   card's height is estimated from raw text length), so check
   `frontend/src/surfaces/Card.svelte` before repeating the claim. The brief that
   produced the other two backlogs is the model: read `docs/wiki/FACTS.md` for
   truth, `frontend/src/surfaces/*` and `frontend/src/lib/*` for the surfaces, and
   write `docs/backlog/ux.md` in the same item shape as `docs/backlog/robustness.md`.
2. **Write `docs/backlog/README.md`**, the single prioritized list across all three
   backlogs. Nothing merges them today, so the three files are three opinions with
   no order between them. `features.md` ends with its own ranking and
   `robustness.md` with six consequence-ordered bands; the missing judgement is how
   a band-A robustness item ranks against F-01.
3. **Take the screenshots.** `tools/wiki/shots.py --yes`, runbook in
   `tools/wiki/SHOTS.md`, about five minutes of Boris's desktop. It has never been
   run. Read the runbook's "what usually goes wrong" first, and expect to retake
   two or three. Three shots the finished pages ask for are NOT in the plan and need
   adding: the secret card mid-entry (`is-it-safe.md:18`), the Appearance page with
   an unsaved change (`settings.md:51`), and terminal output from `make doctor`
   (`install.md:96`, which needs no daemon swap at all and can be done any time).

## Where we are

Boris asked for a product wiki: features and rationale, engaging rather than dull,
nothing that only a maintainer cares about, every page opening with a summary for
somebody in a hurry, and zero traces of machine authorship. Eighteen pages plus a
sidebar are written, published to both hosts, and verified rendering in a real
browser. On the way, reading the code instead of the docs turned up twenty-two
false claims, all fixed, with `docs/wiki/FACTS.md` now the audited fact base. Two
reproduced daemon defects were fixed with tests. Three backlogs were commissioned;
two landed and the UX one died on a session limit.

## Live state (volatile - verify on resume)

- **Background jobs:** none. A headless Chrome on port 9222 was started for the
  render check and stopped; confirm with `curl -sf http://127.0.0.1:9222/json/version`
  expecting failure.
- **PRs:** none, ever. Boris pushes `main` directly.
- **Git:** `main` at `b9dc272`, clean, **13 commits ahead of `origin/main`** (origin
  is gitlab and is main's upstream; the `github` remote is a mirror and far behind).
  Nothing is pushed. The wiki repos ARE pushed and are ahead of what origin knows.
- **Deployed daemon:** `69230d4f7e32`, which is 13 commits behind HEAD and does not
  have the two fixes in `1d00fd2`. **This is the one thing that matters most.**
- **In-flight edits:** none.
- **The wiki is live** at https://gitlab.com/fu-bar/agentbox/-/wikis/home and
  https://github.com/borismilner/agentbox/wiki, both at the content of `dcc86fe`.
  Anything committed to `docs/wiki/pages/` since then is NOT published until
  `make wiki` runs again (`make wiki-dry` shows what it would change, `make
  wiki-check` lints without publishing). As of this handoff they are level. Never
  edit a page in either browser: the next publish overwrites it.

## Blocked on you (Boris)

**Two things, neither urgent.**

1. **Five minutes of desktop for the screenshots**, when convenient. The script
   refuses to start while another session is live, and it takes your daemon down
   for the duration, so it is not something to start while you are relying on
   AgentBox to reach you.
2. **Whether `docs/backlog/features.md`'s one non-goal revision is on.** It argues
   for revisiting "no remote or mobile delivery in v1" as a relay that runs as a
   subprocess you chose rather than as a cloud service, and asks that the vision be
   amended visibly the way ADR-0009 amended principle 6, not quietly via a config
   key. That is a principle change and therefore yours.

## I can do solo (no input needed)

1. The UX audit (item 1 above). Nothing about it needs Boris.
2. `docs/backlog/README.md`, the merged priority order (item 2).
3. `install.md`'s missing shot, which is terminal output and needs no daemon swap.
4. The band-A robustness items after the two already fixed. `robustness.md` lists
   thirteen more in the same band, each with the test that would catch it.
5. The first Svelte rendering test. Nothing in the repo renders any of 32 Svelte
   files, and `robustness.md` names mounting `Card.svelte` under vitest and jsdom
   as the single highest-value missing test: it turns three findings from invisible
   into red.

## Facts - verified vs assumed

- [verified] **The wiki renders on both hosts.** Driven in a headless Chrome:
  the mermaid sequence diagram renders on GitLab and on GitHub, `img/card.png`
  loads at 470px on both, `<kbd>`, tables and alerts render, and no wiki link is
  broken on either. Screenshots were taken and looked at.
- [verified] **Relative image paths work on both hosts.** GitHub rewrites them per
  page depth, emitting `wiki/img/card.png` from the front page and `img/card.png`
  from a subpage, both resolving to the same file. This contradicts what the
  mechanics research concluded, so do not re-derive it from that research.
- [verified] **Both daemon fixes fail without the fix.** Checked by stashing
  `daemon.go` and re-running each test, which is the only check that proves a test
  is doing anything.
- [verified] **The wiki lint passes** on all nineteen files, and the prose scanner
  passes every page except for middle dots inside code spans that quote what the
  interface prints. Those are quotations and are left alone deliberately.
- [verified] **The earcon durations**, measured from the embedded WAVs: pop 90 ms,
  tick 160, thud 220, twotone 260, chime 340, insist 430.
- [verified] **`Daemon.Promote` used to no-op** for an item not in memory. Read at
  the line, independently of the audit that reported it.
- [assumed] **That `make check` passed in full.** It was run and its tail showed
  the webui package and all 13 JS tests green with no failure line, but the run was
  not read end to end and the daemon package's own `ok` line was not seen. Both new
  tests pass individually. Re-run it; it is four minutes.
- [assumed] **That the artifact-restart-while-streaming edge is still open.** It may
  have been closed by the stable fence id (`internal/webui/artifact.go:240`) and the
  `data-live` hydration guard (`frontend/src/lib/artifact.svelte.js:383`), and may
  not, because neither helps if a re-render replaces the node rather than patching
  it. Nobody has watched it. Both docs now say exactly that rather than declaring it
  fixed. **Watching it is cheap and would close a carried item.**
- [assumed] **That the twelve staged screenshots come out.** `shots.py` has 82 tests
  and has never touched a desktop. Every `agentbox` call in it that stages or raises
  a surface is unexercised.
- [assumed] The carried set from session 55, none of which this session looked at,
  and all of which are therefore still exactly as unverified as they were: that the
  30-minute fuse fires in a live daemon; that the demoted marker behaves on a second
  monitor; that `[flood]`'s defaults are right for ordinary work (**this one is now
  more interesting, because flood control is what produced the unanswerable
  question**); that `perform.py`'s fullscreen check holds on two monitors; and the
  session-50/51 set (the domain drawer animation, the demo fallback painting, a
  detail holding a mermaid diagram, `provisionalFor` retiring a hook-only row, the
  200-key prefix cap, `@me` in a shared key, a real client's `await_signal` parked
  past 20s, the two lock warnings, and the identity cross-check's no-node skip).

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| Session 55's whole handoff (this file's previous contents) | Session 55 survives in [docs/history.md](docs/history.md) under "Fifty-fifth session". Its carried `[assumed]` set is reproduced above rather than dropped, because a carried note that nobody re-states is a note nobody looks at |
| 88 lines of session narrative at the top of STATUS.md | [docs/history.md](docs/history.md), which is the log and already held all of it. STATUS said so itself in a paragraph below the narrative it was duplicating |
| The FR83 block in STATUS.md, forty lines summarising five slices | [docs/09-sync.md](docs/09-sync.md), which the block itself said was worth reading rather than summarising. The two shipped bugs it named are kept in STATUS, because they belong to no single slice |
| Nine config keys documented in 06-configuration.md that no code reads | Deleted as knobs, kept as behaviour: the new "Behaviour that is fixed, and has no knob" table in [docs/06-configuration.md](docs/06-configuration.md) says what each one described and why there is no key for it |
| The single 1161-line `ai-text-tells.md` under `~/me/study` | Both forms now live at `~/me/study/guides/writing/ai-tells/`: the whole file as the source, and seven per-layer files generated from it by `split.py`. Coverage was verified line by line |

## Map

1. [HANDOFF.md](HANDOFF.md) - this file.
2. [docs/STATUS.md](docs/STATUS.md) - current state, and "Do this next".
3. [docs/wiki/FACTS.md](docs/wiki/FACTS.md) - **read this before quoting any number
   about the product.** Audited from source, with the line behind each claim and a
   table of what the older docs get wrong.
4. [docs/backlog/robustness.md](docs/backlog/robustness.md) - 45 items in six
   consequence-ordered bands, and nineteen things verified correct so nobody
   refiles them.
5. [docs/backlog/features.md](docs/backlog/features.md) - eleven extensions, five
   bets, ranked.
6. [docs/wiki/DESIGN.md](docs/wiki/DESIGN.md) - the wiki's page inventory, template,
   voice guide and the twelve shot specs.
7. [tools/wiki/SHOTS.md](tools/wiki/SHOTS.md) - the screenshot runbook, including
   which pages ask for which shot.
8. [docs/history.md](docs/history.md) - the session log; session 56 is at the top.
9. `~/me/study/guides/writing/ai-tells/` - outside this repo: why the wiki's prose
   rules are what they are, measured against four corpora rather than asserted.
   Three pieces of standard advice in it are backwards, so read its README before
   editing anyone's prose. The scanner is `scan-tells.py` in that directory.
10. `~/.claude/skills/scrub-ai-tells/SKILL.md` and `~/.claude/commands/scrub-ai.md`
    - the skill and the `/scrub-ai` command built from that research. `~/.claude` is
    NOT a git repo, so the copy of record is `skill.md` in the directory above; if
    the two differ, trust the one in the study repo.
