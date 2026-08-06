# Handoff - AgentBox: the standard was put on trial and lost five times

*Written by session 49, which watched the three things session 48 shipped
unwatched, ran the walkthrough-standard experiment Boris asked for, and took one
new field request from him.*

**Written:** 2026-08-06 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean and in sync with origin/main; HEAD is this handoff's own commit
make deployed               # expect 35d1cf4d0e6e - HEAD is DOCS-ONLY ahead of it, which is fine
agentbox pending            # expect "nothing pending"
agentbox sync agents        # your row, plus one or more detached ghosts (see Live state)
agentbox sync locks         # expect "no locks held"
make check                  # gofmt + vet + race, ~2 min
```

**Nothing is in flight.** One field request Boris has filed is open: **FR94**,
and it is item 1 below.

### The queue, in order

1. **FR94 - pause a hands-off run and resume it.** Filed 2026-08-06 and the only
   open field request. Boris, verbatim: *"during hands-off I must be able to
   pause the hands-off and resume it when I suddenly need the keyboard or mouse
   urgently."* A run is binary today, so his only move is to reach for the mouse
   anyway - the exact collision FR74 exists to prevent. **Mock it before building
   it** (the working rule at the top of
   [docs/07-field-requests.md](docs/07-field-requests.md)); the shape, the five
   constraints and the three open questions are in the FR94 entry there. The
   state a run lives in today is `internal/webui/control.go` plus the desktop
   lock in the daemon.
2. **Give the coverage rule a validator.** The one finding from the standard's
   trial still open. The standard demands a traversal that accounts for every
   changed line, and nothing checks it: `walkthrough.captured` logs
   `ranges/cited/missed` for citations and never compares the cited set against
   the diff's changed set. It is the rule that most needs a machine and has none,
   and the spec already holds both halves at validation time.
3. **Four assumed things about surfaces, still unseen.** Each needs a specific
   staging, listed under "Facts" below: the *"This session has left the board"*
   sentence, `speak` and `diff` reading back in the inbox detail, the Recent
   items block on an agent row, and the demo fallback on screen.
4. **Consider giving `[editor]` a settings control.** Unchanged from the last
   handoff and still Boris's call: the value is an argv array and the descriptor
   table in `internal/webui/settings.go` has no kind for one, which is why
   `speech.command` has none either. The honest shape is a new knob kind, not a
   text field that gets shell-split.

## Where we are

Session 48 shipped four field requests and left three things it had never
watched. All three were watched this session and two of them were right. The
third - FR74's fullscreen marker - turned out to be half right, and Boris decided
to leave it.

The larger piece was the experiment: a review of `e511a01` authored WITHOUT
reading the walkthrough standard, then audited against it rule by rule,
rewritten, and created (`w5bd381d0590a`, 12 steps, 4 domains, 30 citations, 0
missed, no warnings). The point was never my draft; it was what the standard
could not tell me. It failed in five places, all now fixed and deployed.

## The experiment, and what it cost the standard

**My draft diverged in fourteen places.** The ones worth remembering: prose
carried the explanation and the margins were nearly empty (6 notes in the draft,
30 in the rewrite - the standard names this as the most common way a good review
reads badly); no `close`, no handoffs, no reading order on any step; no
traversal, which left seven changed files nothing stood on; and `p: true` missing
on seven paragraph-starting segments, each of which would have rendered as a wall
fused across the seam.

**The five defects in the standard itself** (`84e19ef`, `35d1cf4`):

1. **Rules 13-16 did not exist.** The "order the code" section was renumbered to
   29-32 when domains and the TL;DR were inserted and its old numbers were never
   reused: 55 rules numbered up to 59. Renumbered contiguously, 1-56.
2. **It demanded fields it never named.** The runnable-check rule asked for "the
   command, the result to expect, and the date that expectation last held" and
   never said they are `cmd`, `expect`, `recorded`. The first spec written
   against the standard was refused for an unknown field.
3. **`tldr` and `domains` were in no field reference an agent reaches.** The
   in-binary manual listed four easy-to-miss fields and named neither;
   `create_walkthrough`'s own schema description omitted both. An agent that read
   the standard and went looking for the shape found nothing.
4. **"Two to four domains. Six is the cap"** in one rule, with no way to tell
   which was the instruction.
5. **"Say what you did not verify next to the gate" is impossible as written.** A
   check step has no code blocks, and a codeless step is refused a `close`.

Two things it also never covered are now in it: citations are captured from git
AT the pinned sha rather than from the working tree (which is what makes
reviewing an older commit work at all), and a committed bundle belongs out of the
diff you pass. Plus a paragraph on who the reader is - the rules assume somebody
meeting the subsystem for the first time, and here it is usually the author
reading their own change back.

> **Rule numbers are now deliberately not quoted anywhere outside the standard.**
> They have renumbered twice, and every reference to one went stale silently -
> three of them were sitting in the field-requests doc pointing at rules that had
> meant something else for weeks. Name the rule, never number it.

## What else this session settled

**The TL;DR control (`1cd941e`), watched.** Its original subject was gone - Boris
deleted `wfd51b30be854` from the board ninety seconds before the session started
- so the disabled case needed a new one: a two-step probe review of only `ground`
and `none` steps, which are the two kinds the spec allows no `tldr` on. Dim half,
dead `t`, no `t` in the legend, and the hover title *"no step in this review has
a TL;DR - it was written before they existed, or its author left them out"*. On a
review that has them the control is live, the board opens in TL;DR, and a step
without one says *"No TL;DR was written for this step, so it is shown in full"*.

**The Agents board's three detail blocks paint.** Session 48's largest untested
thing. An opened row shows the meta list, an Activity block and a Signals block
with direction arrows and clipped payloads, all with real content.

> **The trap that cost twenty minutes:** the row TOGGLES, and `.row.open` is the
> same colour as `:hover`. Clicking it twice across two attempts opens and closes
> it, and a screenshot taken with the pointer still on the row shows a surface
> that looks broken either way. Move the mouse away before judging - the row that
> stays lit is the open one.

**FR74's marker: right, and its other half is not.** The marker is exactly what
was designed - `1920x4` amber at `+0+0`, present while a fullscreen window has
focus, gone within a beat of leaving it. The strip does not step aside, though,
so a film gets the 620x62 card as well as the line. Not the arithmetic:
`planMark` returns `step: true` on one monitor and `beat` does call
`x.lower(strip)`. The strip is a NOTIFICATION type window with
`_NET_WM_STATE_ABOVE`, and Mutter layers notifications above a fullscreen window
whatever the stacking order says. **Boris decided: leave it, it is the safe
direction** - the guarantee is over-kept, not broken, and hiding the strip risks
taking the keyboard back on the remap.

**The `xdg-open` fallback, finally exercised.** Never reached before because this
machine has `goland` on PATH. Run against four real PATHs rather than a stubbed
one: `goland`, then `subl` when only `/usr/bin` is visible, then `xdg-open` with
no line in the argv, then the honest *"no editor found; set editor.command"*. The
fallback's own launch was then run the way `editor.Start` runs it, through
`systemd-run --user --scope --collect`: this desktop hands a `.go` file to Zed,
which opened at `1:1`. `zed` is itself in the known table, so the line survives on
a machine where its binary is on PATH; only the desktop-handler route drops it.

## Live state (volatile - verify on resume)

- **Deployed:** `35d1cf4d0e6e`. **HEAD is docs-only ahead of it** - the last code
  change was `35d1cf4` and everything after is documentation, so a deploy is not
  owed. `make deployed` disagreeing with `git log` is expected here, not drift.
- **Git:** `main`, clean, pushed. Seven commits this session before this handoff,
  oldest first: `84e19ef` `35d1cf4` `f648598` `338384a` `d8c5abf` `d1411a1`
  `577d599`. The handoff's own commit is HEAD and is the eighth; its sha is not
  written here because a handoff cannot know it, which is a mistake the last two
  handoffs made and had to correct twice.
- **Background jobs: none. PRs:** none, ever - Boris pushes `main`.
- **Nothing pending, no locks held. The desktop was taken and released three
  times** and is not held now.
- **No AgentBox windows are open.** The deploy closed the app window and the board
  Boris had open on #6634; he was told it would and did not stop it. Nothing is
  lost - `agentbox app` and `agentbox walkthrough open ID` bring them back.
- **The library holds two reviews.** `w5bd381d0590a` is this session's, written
  for the experiment and left deliberately - it is a real review of `e511a01` and
  Boris may want to read it. `wf649f2d212b5` is the peer session's #6634 and was
  re-authored twice while this session ran, so **that id will not be the one you
  find** - list rather than assume.
- **Ghost rows on the Agents board.** Each `claude -p /usage` check leaves a
  `detached · agentbox session (purpose not yet stated)` row behind; three checks
  left three. Harmless, recorded under "Mechanics discovered", and
  indistinguishable from a real session that never announced.
- **Captures live in a session scratchpad** and will not survive a reboot:
  `/tmp/claude-1000/-home-boris-milner-me-projects-agentbox/bb7cc757-4986-4814-af41-30e1fe518d94/scratchpad/`
  (`03-hdr.png` is the disabled control's tooltip, `18-clicked-mouse-away.png`
  the Agents detail, `33-ring-code.png` the review's margin notes,
  `51-topedge.png` the FR74 marker). Deliberately not committed.
- **Usage:** session **36%**, resets 2026-08-06 18:40 Asia/Jerusalem; week (all
  models) **20%**, resets 2026-08-12 05:00.
- **In-flight edits: none.**

## Blocked on you (Boris)

Nothing - proceed autonomously. Three things stay yours and block nothing:

- **FR84's other half** (a long body still pushes a form's fields below the fold)
  needs your word, because the approach that fixes it is the one you did not
  pick. Mock: [docs/mocks/fr84-form-shapes.html](docs/mocks/fr84-form-shapes.html),
  approach C.
- **Your PostToolUse hook writes the raw Bash command as the activity line.**
  Carried from the last three handoffs and now watched on screen rather than
  guessed at: this session's Activity block was a column of shell fragments like
  `SP=/tmp/claude-1000/-home-boris-milner-me-projects-agentbox/bb7cc757-4 APP=…`,
  with the two lines `set_activity` actually wrote buried among them. It is your
  `~/.claude/settings.json`, so the wording is yours; the first line, or the
  first 80 characters, would read better.
- **Whether the re-record is worth scheduling** (STATUS priority 2). Unchanged.

## I can do solo (no input needed)

1. **Mock FR94** and put it in front of him - the working rule says mock before
   build, and this one has real open questions (auto-resume, an in-flight
   `drive_desktop`, per-run versus desktop-wide).
2. **The coverage validator** (queue item 2).
3. **The four unseen surfaces** (queue item 3).
4. **A JS test runner** (STATUS priority 6). It would have caught none of the CSS
   problem session 48 paid for - that was a render, not a unit - but `parseDiff`
   is a shared module with two callers and no test of its own.

## Facts - verified vs assumed

- [verified] **The TL;DR control on both sides.** Disabled on a review with none
  (dim, unclickable, `t` inert, legend drops the key, tooltip explains), live on
  one that has them, and the *"shown in full"* line on a step without one.
- [verified] **The Agents board's Activity and Signals blocks paint** with real
  content, and re-opening a row refetches: a signal posted while the row was open
  appeared only after closing and re-opening it.
- [verified] **FR74's marker** at `+0+0`, `1920x4`, `srgb(189,144,60)` sampled
  from x=200 to x=1900, present while a focused fullscreen window is up and gone
  within a beat of leaving it.
- [verified] **FR74's strip does NOT step aside** - the 620x62 card is on screen
  over a focused fullscreen window, with `xwininfo -root -children` showing the
  probe above both agentbox windows in the X order.
- [verified] **The editor ladder, all four rungs, against real PATHs**, and the
  fallback's real launch opening Zed at `1:1`.
- [verified] **`make check` passes** (gofmt, vet, race) after every edit this
  session, and the deployed binary serves the renumbered standard
  (`agentbox docs walkthrough` ends at rule 56).
- [verified] **The experiment's review renders**: 4-domain accordion rail, domain
  blurbs, TL;DR mode by default, numbered margin notes beside the right lines,
  `new` and `captured` badges on every block.
- [assumed] **That the *"This session has left the board"* wording ever paints.**
  It needs a row to vanish between the click and the reply, which is the reason
  it has now survived two sessions unseen.
- [assumed] **That `speak` and `diff` read back on screen.** Migration 0012's
  columns, insert, select and wire are all tested; no item raised since has been
  opened in the inbox detail. Raise an `agentbox review` with a diff, answer it,
  then open its row.
- [assumed] **That an agent row's Recent items block paints.** Activity and
  Signals were seen; Items needs a session that has raised something and then had
  its row opened, and this session raised nothing until the very end.
- [assumed] **That the demo fallback paints.** `Bridge.AgentDetail` falls back to
  the fixture when no daemon is behind the build, and it is unit-tested, but it
  cannot be reached on screen while a real daemon owns the session bus.
- [assumed] **That the domain drawer animation is what Boris wants.** Unchanged
  from the last handoff; he has not said either way.
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
| Session 48's queue items 0, 2 and 3 (the TL;DR control, FR74's marker, the xdg-open fallback) | All three watched this session. The results are in session 49 of [docs/history.md](docs/history.md); FR74's is also in its own entry in [docs/07-field-requests.md](docs/07-field-requests.md) and STATUS priority 1, and the editor ladder is under FR65 there |
| Session 48's queue item 1 and both experiment prompts, verbatim | The experiment was run. The prompts have done their job and the findings replace them: session 49 of history.md, and the five fixes are in the standard itself (`84e19ef`, `35d1cf4`) |
| Session 48's five traps and its "what shipped" section | All still true and all still in session 48 of [docs/history.md](docs/history.md). Only the two that cost THIS session time are carried above (the toggling agent row is new and is in history.md too) |
| Session 48's capture paths | Its scratchpad is gone or going; this session's are listed fresh in Live state |
| Rule numbers quoted in three places in the field-requests doc | Replaced by the rules' names, because the numbering has moved twice and every quoted number was already wrong. STATUS says why, under the authoring-standard bullet |

## Map

1. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers used in
   commits and handoffs. **FR94 is the only open one**, filed 2026-08-06. FR74's
   entry now carries the live look and Boris's decision; FR65's carries the four
   editor rungs. "Mechanics discovered" gained the `claude -p` ghost row.
2. [docs/STATUS.md](docs/STATUS.md) - current state. Priority 1 is FR74's verdict
   and 1b is FR94; the authoring-standard bullet carries the trial and the one
   finding still open.
3. [docs/history.md](docs/history.md) - session by session; this session is
   "Forty-ninth".
4. [internal/manual/walkthrough.md](internal/manual/walkthrough.md) - the
   authoring standard, **56 contiguously numbered rules** since this session, and
   the thing the experiment was run against.
5. [internal/manual/agent.md](internal/manual/agent.md) - the in-binary tool
   reference; its walkthrough section now names `tldr`, `domains` and `cmds`.
   `docs/agent-manual.md` is the long human-facing one and already had them.
6. [CLAUDE.md](CLAUDE.md) - traps that have cost sessions. **Read it before
   touching the build, the daemon, or driving the desktop.**
7. [docs/06-configuration.md](docs/06-configuration.md) - the `[editor]` section
   (FR65) and every other knob.
8. [docs/09-sync.md](docs/09-sync.md) - FR83, all five slices, the chip vocabulary.
