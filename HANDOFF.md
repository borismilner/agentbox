# Handoff - AgentBox: four field requests closed, and the review learned two ways to be read

*Written by session 48, which cleared a fix list, took three new asks in the
same sitting, and spent three build-deploy-look cycles believing a diff over the
pixels.*

**Written:** 2026-08-06 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean, in sync with origin/main at e5b2473
make deployed               # expect 35a5bd8ed0df - OLDER than HEAD on purpose, see Live state
agentbox pending            # expect "nothing pending"
agentbox sync agents        # your own row plus a peer in ~/work/minimus
agentbox sync locks         # expect "no locks held"
make check                  # gofmt + vet + race, ~2 min
```

**Nothing is in flight.** No field request Boris has filed is open. The one thing
he asked for and has NOT yet received is item 1 below - it is written, it just
has not been run.

### The queue, in order

1. **Run the walkthrough-standard experiment.** Boris asked for a prompt to give
   an agent that has just written a walkthrough, so it consults AgentBox's
   standard, reports what it had to change, and tells us where the standard is
   thin. The prompt is in "The experiment Boris asked for" below, verbatim and
   ready to paste. **Its point is the agent's answer to question 4** - what the
   standard does not cover - because that is a hole to close, and the standard
   grew by two whole sections today (domains, the TL;DR) without anybody but me
   reading them.
2. **FR74's fullscreen marker has still never been seen.** Unchanged from the
   last two handoffs. `internal/webui/control.go` has the marker,
   `controlmark_test.go` unit-tests its placement rule, the
   `_NET_WM_STATE_FULLSCREEN` read is in `x11.go`. What is missing is a live
   look: a real fullscreen window covering the strip, with the marker checked to
   be still on top of it. Needs consent to drive the desktop and a fullscreen app.
   An untested marker is worth no more than none, because a fully covered strip
   reads as "the desktop is yours" while an agent drives.
3. **The `xdg-open` fallback has never been exercised.** FR65 resolves an editor
   in three steps: `editor.command`, then a table of known launchers, then
   `xdg-open` (which loses the line and says so). Only the first two were watched
   - this machine has `goland` on PATH, so detection never reached the fallback.
   Unit-tested with a stubbed PATH; never seen.
4. **Consider giving `[editor]` a settings control.** Deliberately skipped: the
   value is an argv array and the descriptor table in `internal/webui/settings.go`
   has no kind for one, which is why `speech.command` has none either. If Boris
   wants it, the honest shape is a new knob kind, not a text field that gets
   shell-split.

## Where we are

Session 48 was two halves. The first cleared the queue session 47 left: **FR65**
shipped (an arrow beside copy on every citation, opening the reader's editor at
the cited line), and then Boris said "fix everything needs fixing", which turned
into migration 0012, the Agents board's three empty detail blocks, and a keyboard
collision older than any of it.

The second half was three new asks arriving while that was in flight: **FR91**
(every step gets a TL;DR and the board opens in it), **FR92** (steps group into
domains, one shown at a time, animated), and **FR93** (Esc could not close a
notification). All three shipped and were watched on screen. Twelve commits, all
pushed. Nothing half-done.

## The experiment Boris asked for

Paste this to an agent that has just authored a walkthrough. He asked for it
near the end of the session and it was never run.

```
You just wrote a walkthrough. Before you hand it over, consult AgentBox's own
authoring standard and bring what you wrote up to it.

1. Read the standard. It is `agentbox docs walkthrough` from a shell, or the MCP
   resource `agentbox://standards/walkthrough`. Read all of it, not the headings.
2. Go through your walkthrough against it rule by rule and list, in one place,
   every place yours diverges - the rule number, the step, and what is wrong.
   Include the ones you disagree with, marked as such.
3. Fix them, then create it with `create_walkthrough`. If it is refused, the
   error says what to do; do that rather than working around it.
4. Report back three things: what you had to change, which rules you found
   ambiguous or contradictory when applied to real content, and anything the
   standard does not cover that you had to decide for yourself.

Point 4 is the actual purpose of this exercise, so do not skip it or soften it.
The standard is being tested, not you.
```

The standard is `internal/manual/walkthrough.md` (59 numbered rules now). It
reaches an agent three ways: the MCP resource above, the `walkthrough_standard`
MCP prompt, and `agentbox docs walkthrough`.

## What this session shipped, and what each cost

**FR65 - open a citation in the editor.** `internal/editor` resolves an argv
template and launches it; `Bridge.BoardOpenInEditor` and an arrow beside copy in
every block header. The surface names a REVIEW and a repo-relative path, never a
file - the root comes from the stored walkthrough and `underRoot` refuses
anything outside it.

> **The trap that would have shipped invisibly:** `agentbox.service` is
> `KillMode=control-group` and a JetBrains Toolbox launcher *execs* the IDE rather
> than forking it, so the IDE **is** the daemon's child and dies on the next
> `make deploy`. The launch goes through `systemd-run --user --scope --collect`.
> Cold start only in practice, which is exactly the case testing skips.

**Migration 0012 - `session_key`, `speak`, `diff` on items.** The first is the
only identity naming ONE session; the other two were written into FR73's
read-back and taken out again when the insert turned out not to name them. The
unified-diff parser moved to `frontend/src/lib/diff.js` so the card that asks for
a review and the detail that reads it back use the same one.

**The Agents board's three detail blocks are real.** They were rendered by
`demo.go` and by nothing else. Now assembled per opened row (`Bridge.AgentDetail`
→ `Daemon.AgentDetail`), because the roster is pushed once a second while
anything moves. Three owners meet and none learns about the others: the roster
gained a ring of activity lines a session has moved past, the signal hub gained a
ring of what a session HEARD (**the store cannot know** - a signal is fanned out
by meaning and one row is read by every listener), and the store answers what it
posted and raised.

**FR91 - the TL;DR.** Not a summary field. Boris: "not necessarily less
exhaustive, but optimally structured for a person with a very short attention
span that must still get a mastery level of the most important aspects." That
killed both obvious designs and left a shape: `bottom` plus up to six standalone
`points`, capped **per point** so the bound is on the shape rather than on how
much may be said.

**FR92 - domains.** Two to six groups, every step in one, contiguity validated
(the board walks one at a time, so a domain the order leaves and returns to would
open twice). The rail is an accordion; `[` and `]` move by domain; clicking a
collapsed domain opens it *without* navigating there.

**FR93 - Esc on a notification.** It deferred, so escalation raised the card again
every 20 seconds and ⇧Esc was the only way out and was written down nowhere.

**Two keyboard/layout defects found by using the new controls**, both older than
them: Enter on any focused button on the board ran the button AND the board's own
shortcut, and `all: unset` on a button wiped the grid-column the step sets on
every child.

## Traps this session paid for

- **A control can be fully styled in the stylesheet and unstyled on screen.** The
  segmented mode control rendered as two bare words in the header while its rules
  sat in the bundle with the right scope hash and the markup carried it. Moving it
  into its own component (`frontend/src/lib/board/ModeToggle.svelte`) fixed it.
  **Every `var()` there now carries a fallback**: a var() that resolves to nothing
  takes its whole declaration with it, so a control that has lost its background
  still reads as working to everything except the screen. Now in CLAUDE.md.
- **`agentbox walkthrough open` on an ALREADY-OPEN board retargets the window
  without reloading the page.** After a frontend change the surface you are
  judging can be the old bundle even though the deploy succeeded. Close the window
  or restart the daemon first. This cost two of the three cycles above. Now in
  CLAUDE.md.
- **`make deployed`'s build stamp does not change within a commit.** Two deploys
  of the same commit print the same sha and the same timestamp, one with
  `(dirty)`. It is not a way to tell whether a dirty rebuild actually happened -
  compare `stat -c %y` on the binary, or grep the binary for the current asset
  hash (`grep -ac "board-XXXX.css" ~/.local/bin/agentbox`).
- **A walkthrough fixture needs a `tldr` on every code and check step now**, and
  five test fixtures across three packages had to gain one. If a future spec
  change adds another required field, `grep -rn '"kind": *"code"' --include='*_test.go'`
  finds them all.
- **Borrowing Boris's live config is fine if you restore it by checksum.** The
  FR65 failure wording needed a deliberately broken `editor.command`;
  `~/.config/agentbox/config.toml` was backed up, edited, and restored with its
  md5 confirmed (`815d7770f5ed86892f3932cc152a68c9`).

## Live state (volatile - verify on resume)

- **Deployed:** `35a5bd8ed0df` - **older than HEAD (`e5b2473`) on purpose.**
  Everything after it is markdown (`docs/`, `CLAUDE.md`), so the binary's
  behaviour is current. Not redeployed because `make deploy` restarts the daemon
  and a peer session reaches Boris through it. Redeploy freely at the start of a
  session, when nobody is mid-question.
- **Git:** clean, `main` pushed to `origin` at `e5b2473`. Twelve commits this
  session, oldest first: `657e093` `2c22e9f` `0a456a0` `a730caf` `0bb42d7`
  `e511a01` `89dec68` `4c974ab` `655b076` `72d65cb` `35a5bd8` `e5b2473`.
- **Background jobs: none. PRs:** none, ever - Boris pushes `main`.
- **Nothing pending, no locks held.** The fixture walkthrough this session created
  (`w4568642ae564`) was deleted. Two of Boris's own remain in the library and were
  not touched: `w878f0c51c433` and `wfd51b30be854`, both the peer session's.
- **A GoLand window on `~/me/projects/agentbox` is open on his desktop**, opened
  by FR65's own button during the demonstration and deliberately left (closing an
  IDE window he may since have used is worse). Nothing depends on it.
- **The desktop was taken twice and released twice.**
- **Captures live in a session scratchpad** and will not survive a reboot:
  `/tmp/claude-1000/-home-boris-milner-me-projects-agentbox/e1fc6994-afab-4e94-87cb-37f56cb75f91/scratchpad/`
  (`20-hdr.png` and `21-hdr.png` are the mode control; `14-domainkey-crop.png` is
  the rail accordion). Deliberately not committed.
- **One peer on the board** in `~/work/minimus` ("SSVC in Advisories"), untouched.
- **Usage:** session **23%**, resets 2026-08-06 18:40 Asia/Jerusalem; week (all
  models) **18%**, resets 2026-08-12 05:00. Plenty of room.
- **In-flight edits: none.**

## Blocked on you (Boris)

Nothing - proceed autonomously. Three things are still yours and still not
blocking anything:

- **FR84's other half** (a long body still pushes a form's fields below the fold)
  needs your word, because the approach that fixes it is the one you did not pick.
  Mock: [docs/mocks/fr84-form-shapes.html](docs/mocks/fr84-form-shapes.html),
  approach C. Do not build it on a session's initiative.
- **Your PostToolUse hook writes the raw Bash command as the activity line.** It
  is your `~/.claude/settings.json`, so the wording is yours. This session's board
  row showed a `git push … | tail -2; git` fragment as its activity. Truncating to
  the first line, or the first 80 characters, would read better. Carried from the
  last two handoffs.
- **Whether the re-record is worth scheduling** (STATUS priority 2). Unchanged.

## I can do solo (no input needed)

1. **The walkthrough-standard experiment** (the prompt above), and closing
   whatever holes the agent's answer to question 4 finds.
2. **FR74's marker, watched live** - needs consent to drive the desktop, which is
   a request rather than a blocker.
3. **The `xdg-open` fallback**, exercised with a PATH that has no known launcher
   on it.
4. **A JS test runner** (STATUS priority 6). It would have caught none of today's
   CSS problem - that was a render, not a unit - but `parseDiff` is now a shared
   module with two callers and no test of its own.

## Facts - verified vs assumed

- [verified] **FR65 end to end on the real screen.** With no `editor.command` set
  at all, detection picked `goland`; the caret landed on `141:2`, the cited line.
  The second citation, project already open, routed to that window, switched tabs
  and raised it - `378:2`.
- [verified] **The editor survives a daemon restart.** The window opened by the
  button was still up after a real `make deploy`, and `/proc/<pid>/cgroup` showed
  a transient scope, not `agentbox.service`.
- [verified] **GoLand raises a This Window / New Window / Attach modal** for a
  project it does not already have, and the file then lands in a background tab
  behind the restored session's own.
- [verified] **The failure wording appears beside the block**:
  `editor "nosucheditor-fr65-probe" is not on PATH`, and the next click recovered
  once the config was right.
- [verified] **Enter on a focused button moved the step, and no longer does.**
  Watched from step 2 landing back on step 1, fixed, watched again staying put
  while the daemon log recorded the open the same Enter triggered.
- [verified] **The TL;DR and the domain rail on screen**: the accordion opening
  and collapsing, `[` twice moving back two domains, `t` switching to the full
  text, and the mode control rendering as a filled pill after the component move.
- [verified] `make check` passes (gofmt, vet, race) with every new test.
- [verified] **Boris's config was restored** - md5 back to
  `815d7770f5ed86892f3932cc152a68c9`.
- [assumed] **That the Agents board's new detail blocks paint.** The Go side is
  unit-tested end to end (`internal/webui/agentdetail_test.go`,
  `internal/daemon/agentdetail_test.go`) and the surface was rewritten to fetch
  per opened row, but **an opened agent row was never looked at on screen** - the
  session ran out of screen time on the board instead. This is the largest
  untested thing shipped today. Check it first: open the app's Agents tab, click a
  row, and expect an Activity block, a Signals block, or the honest sentence.
- [assumed] **That the "This session has left the board" wording ever paints.** It
  needs a row to vanish between the click and the reply.
- [assumed] **That the domain drawer animation is what Boris wants.** He asked for
  "elegant and eye-pleasing, maybe even animated"; the drawer and the banner both
  animate and he has not said either way.
- [assumed] **That `speak` and `diff` read back on screen.** The columns, the
  insert, the select and the wire are tested; no item raised since the migration
  has been opened in the inbox detail to look at. Raise a `agentbox review` with a
  diff, answer it, then open its row.
- [assumed] **That a step with no `tldr` renders its "shown in full" line.** Built
  and reasoned about; every fixture this session had one. Boris's two stored
  walkthroughs predate the field and are the natural test.
- [assumed] The older carried-over set, all unchanged from session 47: the
  inbox detail's `found: false` path, the keyboard route to a resolved row's
  detail, a detail holding a mermaid diagram, `provisionalFor` retiring a
  hook-only row after ten minutes, the 200-key prefix cap on the surface, `@me` in
  a shared key, a real client's `await_signal` parked past 20s, the
  holder-parked-on-ask_user and 600s long-wait lock warnings, `webui-demo agents`
  still rendering, and the identity cross-check's no-node skip path.

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| Session 47's whole "Do this next" queue (FR65, FR74, the Agents blocks, the schema gap) | Three of the four shipped today. FR65 is closed in [docs/07-field-requests.md](docs/07-field-requests.md); the Agents blocks and the schema gap are in session 48 of [docs/history.md](docs/history.md); FR74's live look survives as item 2 above, unchanged |
| Session 47's FR73 paragraph, its two-things-broken section, and the bundle-guard note | FR73 is closed and its record is complete in the FR doc and session 47 of history.md. The bundle guard is a test (`frontend/policy_test.go`) and needs no prose |
| Session 47's trap list (LockedHint, the window-name check, the flat-colour tell, hard wraps) | All still true and all still in session 47 of [docs/history.md](docs/history.md). Only the two that cost THIS session time are carried above, plus the two new ones now in [CLAUDE.md](CLAUDE.md) |
| The last handoff's "STATUS entry was wrong" note about FR74 | Resolved then; STATUS's priority list has said the right thing since |
| Session 47's capture paths | Its scratchpad is gone or going; this session's paths are listed fresh in Live state |

## Map

1. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers used in
   commits and handoffs. **FR65, FR91, FR92 and FR93 all closed 2026-08-06, and
   nothing Boris filed is open.** Each entry carries the sentence that decided its
   design and what building it found.
2. [docs/STATUS.md](docs/STATUS.md) - current state, what works, known gaps, and
   the numbered priority tail behind this handoff's short queue.
3. [docs/history.md](docs/history.md) - session by session; this session is
   "Forty-eighth".
4. [internal/manual/walkthrough.md](internal/manual/walkthrough.md) - the authoring
   standard, 59 rules, and the thing item 1 exists to test. Two new sections today:
   domains, and the TL;DR.
5. [docs/06-configuration.md](docs/06-configuration.md) - the `[editor]` section
   (FR65) and every other knob.
6. [docs/agent-manual.md](docs/agent-manual.md) - the tool reference;
   `internal/manual/agent.md` is the embedded copy, and
   `TestManualListsEveryTool` fails if a tool ships without them.
7. [CLAUDE.md](CLAUDE.md) - traps that have cost sessions. **Read it before
   touching the build, the daemon, or driving the desktop.** Two new entries.
8. [docs/09-sync.md](docs/09-sync.md) - FR83, all five slices, the chip vocabulary.
