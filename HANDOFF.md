# Handoff - AgentBox: the leftover queue spent, and FR30 is the only thing left

*Written by session 53, which resumed onto an empty queue, closed both solo items
session 52 left, and fixed the one note six handoffs had carried without testing.
Nothing is in flight. The next real work needs one sentence from Boris.*

**Written:** 2026-08-06 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean and level with origin/main (pushed)
                            # origin is gitlab and IS main's upstream; the `github`
                            # remote is far behind and is not where this goes
make deployed               # expect 7609088aae12 - and that is CORRECT, see below
agentbox control state      # expect "no run: the desktop is the human's"
agentbox pending            # expect "nothing pending"
agentbox sync agents        # your row; ghosts from `claude -p` checks are harmless
make check                  # gofmt + vet + race + JS tests, ~2 min
```

**No deploy is owed.** Every commit since `7609088` is tests and docs, so the
running daemon is deliberately older than HEAD. Confirm with `git log --oneline
7609088..HEAD` before assuming otherwise - and remember `make deployed` asks the
running daemon, because a replaced binary is not a deployed binary.

**Nothing is in flight. No field request is open.**

### The queue, in order

1. **FR30 flood control - blocked on one sentence from Boris, and it is the only
   substantial thing left.** `docs/01-requirements.md:90` promises per-agent rate
   limits collapsing into one stack card; nothing in the daemon does it. An agent
   in a loop papers the screen and can now also fill a recording's held queue.
   Not built because it changes WHEN a card appears, which is a decision about his
   notifications. **The two questions to put to him** (session 53 drafted these
   and he has not answered): collapse a burst into one stack card, or rate-limit
   and drop? And what counts as a flood - N cards in M seconds, per agent or per
   project?
2. **`webui-demo agents` still unseen** - the last of the four surfaces. It needs
   the daemon displaced (`make stop`, run the demo, `make restart-daemon`), which
   takes Boris's notifier down for a few minutes and is the trap CLAUDE.md warns
   about (a second AgentBox shows no windows while another is running). Session 53
   judged that not worth doing unprompted while he was at the keyboard. **Ask
   before doing it** - there is no way to read "is Boris busy" off the machine, and
   every other session on it reaches him through that daemon.
3. **`[editor]` still has no settings control.** Unchanged and still his call: the
   value is an argv array and the descriptor table has no kind for one, which is
   why `speech.command` has none either. The honest shape is a new kind.
4. **More fuzz targets** if nothing better presents. Four now exist
   (`change.FuzzParse`, `walkthrough.FuzzCover`, `FuzzSpecParse`,
   `FuzzBuildPayload`). The remaining agent-authored surfaces are thinner.

## Where we are

Session 52 left two solo items and four "unseen surfaces". Both solo items are
done and the surface question turned out to be the wrong question.

**Two fuzz targets, both clean:** `walkthrough.Parse` at 9.0M executions and
`BuildPayload` at 3.1M. They assert what callers trust without checking - that an
accepted spec's citations are sliceable, that glossary marking partitions the
author's text rather than rewriting it, that the tallies match the lists under
them, that no remark is dropped or doubled, that absence travels as `[]` not
`null`, and that a payload always marshals, since a handback `json.Marshal`
refuses loses the whole review at submit. Clean is the honest result; session 52's
target found a real bug in thirty seconds and these found nothing.

**One documented bound came out of it:** `MaxSpecBytes` reads as a 1 MB cap and is
never enforced on its own, so a spec with no diff can be 3 MB. Worst case
measured: 2.8 MB with a 48-term glossary costs 940 ms in `Parse`. Deliberately not
patched - see [docs/STATUS.md](docs/STATUS.md) for why the obvious fix would start
refusing honest specs.

**The Agents row detail was watched**, and the important finding is that two of its
three empty answers are unreachable rather than unseen (below).

## What was verified on the real desktop

Not read off the diff.

- **`Recent items` paints**: twelve rows at the `agentDetailItems` cap, newest
  first, each with kind, state and age (`agents-3.png`). `Signals` paints on both
  a real row and a hook-only one (`agents-5.png`).
- **The hook fix reads correctly on screen** (`agents-6.png`), with the last
  old-format entry still in the activity ring directly above the new ones: three
  wrapped lines against one, and `Signals` back in the first screenful.
- Desktop taken twice with `request_control`, released both times; `control state`
  and `wmctrl` confirmed no strip left on screen.

> **The one thing only the screen could say.** Boris's PostToolUse hook wrote the
> raw Bash command through `cut -c1-70`, and **`cut` truncates every line and drops
> none** - so a heredoc arrived whole. One opened row rendered a Go test file as a
> single wrapping activity line and a commit message as another, pushing
> `Recent items` two screens below the fold. Six handoffs carried this as a wording
> preference. It was a legibility defect with a different cause and a worse
> severity than any of those six descriptions.

## Two empty answers that cannot be reached, and why that closes them

Both are correct defensive code in `frontend/src/surfaces/Agents.svelte`. Neither
is worth another session's staging:

- *"This session has left the board"* needs `found: false`. The surface closes the
  detail the instant the roster stops listing the row (the guard near the top of
  the file, "a row that went away must not keep the detail open under a different
  agent"), so the daemon's answer survives only in the sub-second race between the
  surface's roster copy and the daemon's map, or when the bridge call throws.
- *"Nothing behind it yet"* needs a row with no timeline, no signals and no items -
  and every row on the board got there by announcing, which posts the signal that
  fills the block. Tried on the hook-only row: it showed its meta and its one
  announce, correctly.

## Live state (volatile - verify on resume)

- **Deployed:** `7609088aae12`, older than HEAD **on purpose** - see "Do this next".
- **Git:** `main`, clean, **pushed to `origin`** (gitlab). Five commits this
  session, oldest first: `13b00cd` (the two fuzz targets), `1ce2500` (the spec cap
  note), `27f31d3` (the row detail watched), `dfdc552` (the hook fix recorded), and
  the docs commit carrying this handoff. Run `git status -sb`: that is the only
  truthful answer.
- **Two remotes, and only one of them is the tree.** `origin` is
  `git@gitlab.com:fu-bar/agentbox.git` and is `main`'s upstream. `github`
  (`git@github.com:borismilner/agentbox.git`) is far behind and is not where this
  work goes. A bare `git push` is right because the upstream is set.
- **`~/.claude/settings.json` was edited this session** (the PostToolUse Bash
  activity hook) with Boris's authorisation. It is his file and outside this repo,
  so it is **not** covered by any commit here and no `git` command will restore it.
  JSON validated with `jq -e`. The backup is in this session's scratchpad (path
  below) and dies with a reboot; the fix itself is quoted in full in
  [docs/STATUS.md](docs/STATUS.md), which is what a restore would actually need.
- **The desktop is his**, nothing quiet, no locks held, nothing pending. All
  checked at the end.
- **No peer sessions.** `announce` returned `alone: true`; one hook-only ghost row
  (`proc-1029368-4422447`) from a `claude -p /usage` check, which is harmless.
- **Captures live in a session scratchpad** and will not survive a reboot:
  `/tmp/claude-1000/-home-boris-milner-me-projects-agentbox/0336eafc-3891-4604-b15e-e2b74203cb03/scratchpad/`
  (`agents-2.png` is the wall of shell before the fix, `agents-3.png` the Recent
  items block, `agents-5.png` the hook-only row, `agents-6.png` the fix on screen,
  `settings.json.bak` the hook backup). Deliberately not committed.
- **Usage at handoff:** session ~26%, resetting 2026-08-06 23:40 Asia/Jerusalem;
  week (all models) ~28%, resets 2026-08-12 05:00. This is a clean stopping point
  with room left, not a rescue.
- **In-flight edits: none. Background jobs: none. PRs:** none, ever.

## Blocked on you (Boris)

- **FR30 flood control** is the one that matters - a documented `must` that was
  never built, and the only substantial work left. The two questions are in queue
  item 1. One sentence unblocks it.
- **FR84's other half** (a long body still pushes a form's fields below the fold)
  needs your word, because the approach that fixes it is the one you did not pick.
  Mock: [docs/mocks/fr84-form-shapes.html](docs/mocks/fr84-form-shapes.html),
  approach C.
- **Whether the re-record is worth scheduling** (STATUS priority 2). Unchanged.
- **`[editor]`'s settings control** wants a new descriptor kind for argv arrays.

*The PostToolUse hook item that sat here for six handoffs is gone: fixed and
watched on 2026-08-06.*

## I can do solo (no input needed)

1. **`webui-demo agents`** when the desktop is free (queue item 2).
2. **More fuzz targets**, though the pickings are thinner than they were.

## Facts - verified vs assumed

- [verified] **`make check` passes** (gofmt, vet, race, JS tests) after every slice.
- [verified] **Both new fuzz targets are clean**: 9.0M and 3.1M executions, run
  this session at `-fuzztime 90s` each.
- [verified] **`MaxSpecBytes` is not enforced alone**, and a 2.8 MB spec with a
  full glossary costs 940 ms in `Parse` - measured with a throwaway test that was
  deleted afterwards.
- [verified] **`Recent items` and `Signals` paint**, watched on the real board.
- [verified] **The hook fix produces one line with an ellipsis**, checked both from
  the CLI (`agentbox sync agents`) and on the board.
- [verified] **The two empty answers are unreachable by ordinary use**, established
  by reading the guard in `Agents.svelte` AND by trying the hook-only row.
- [verified] The session-51/52 set, unchanged: FR95 end to end, the panic guard,
  the convergence guard, both diff parsers surviving a lying hunk header, the
  `quiet_hotkey` knob rendering, the demoted marker's four traps.
- [assumed] **That the 30-minute fuse fires in a live daemon.** Carried from
  session 51: the timer is driven directly in the test and nobody has watched half
  an hour pass.
- [assumed] **That the demoted marker behaves on a second monitor.** Carried; one
  monitor here, so only the fallback has ever run.
- [assumed] The session-50/51 carried set, minus the three this session settled:
  that the demo fallback paints, that the domain drawer animation is what Boris
  wants, and the older list (a detail holding a mermaid diagram, `provisionalFor`
  retiring a hook-only row, the 200-key prefix cap, `@me` in a shared key, a real
  client's `await_signal` parked past 20s, the holder-parked-on-ask_user and 600s
  long-wait lock warnings, `webui-demo agents` still rendering, and the identity
  cross-check's no-node skip path).

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| Session 52's queue item 1 (four unseen surfaces) | Three are settled: `Recent items` is verified above, and the two empty answers are closed as unreachable with the reasoning in "Two empty answers". Only `webui-demo agents` survives, as queue item 2 |
| Session 52's "Blocked on you" item about the PostToolUse hook | Fixed and watched. Cause, fix and the lesson are in session 53 of [docs/history.md](docs/history.md) and in the FR83 section of [docs/STATUS.md](docs/STATUS.md) |
| Session 52's solo item 2 (more fuzz targets) | Both built and committed (`13b00cd`). What they assert and why is in the commit message and in session 53 of [docs/history.md](docs/history.md) |
| Session 52's long FR95 narrative and its five robustness finds | All shipped and unchanged; the full account stays in session 52 of [docs/history.md](docs/history.md) rather than being recopied here |
| Session 52's captures and scratchpad path | Gone with that session; this session's are listed fresh in Live state |

## Map

1. [docs/STATUS.md](docs/STATUS.md) - current state. The FR83 section now carries
   the row-detail findings and the hook fix; the walkthrough limits carry the
   unenforced spec cap.
2. [docs/history.md](docs/history.md) - session by session; this session is
   "Fifty-third".
3. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers used in
   commits and handoffs. **No FR is open.**
4. [docs/01-requirements.md](docs/01-requirements.md) - where FR30's unbuilt `must`
   is written down.
5. [internal/manual/walkthrough.md](internal/manual/walkthrough.md) - the authoring
   standard; rule 49 is the one the coverage validator checks.
6. [docs/agent-manual.md](docs/agent-manual.md) - the MCP tool reference.
7. [CLAUDE.md](CLAUDE.md) - traps that have cost sessions. **Read it before
   touching the build, the daemon, or driving the desktop.**
</content>
</invoke>
