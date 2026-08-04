# Handoff - AgentBox: FR81 shipped; a second agent still in flight in this tree

**Written:** 2026-08-04 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
cat /tmp/agentbox-agents.md   # the parallel-agent ledger; is Agent B (M12 panel) still working?
git status -sb                # B's panel diff uncommitted = B unfinished; clean = B landed
git log --oneline -5          # expect docs(handoff), then c15fce6 fix(assignments), 48950dc, c915fab
make deployed                 # 1898d37 = pre-FR81 build still serving (deploy is deliberately pending)
```

**The tree may be shared.** Session 37 ran two agents in one checkout: this
stream (FR81, all committed and pushed) and Agent B (M12's custom HTML panel,
mid-flight at write time). Before editing or building ANYTHING, read the
ledger at `/tmp/agentbox-agents.md` and reconcile:

- **If B's work landed** (tree clean, STATUS says the panel runs): run
  `make check`, then `make deploy` - the deployed daemon is still `1898d37`
  and has NEITHER the panel NOR the FR81 restyle. Deploy was held back on
  purpose: `make deploy` builds from the working tree, which held B's
  half-done code. After deploy, exercise the panel and one restyled surface
  live. Then pick up the queue below.
- **If B is still in flight or the tree holds their uncommitted diff and the
  ledger says nothing:** do not build, deploy, or touch dist. Work queue
  items that avoid their files, or wait. Their claimed files are in the
  ledger (Assignments.svelte, assignpanel.go, artifact libs, mdhtml.go).
- **If the ledger is gone** (/tmp cleared by a reboot): the git log and
  STATUS tell you whether the panel landed; a dirty tree of the files above
  means an abandoned session - ask Boris before cleaning anything up.

The queue after that (was item 3 of the previous handoff; FR81 was item 2):

**1. FR73 - a card body must be readable after the card closes.** The inbox
row truncates the body to a tooltip and offers no detail view; Boris hit
this 2026-07-31. Nothing new needs storing - it is a reader. Note the Inbox
surface was restyled this session; extend it in its current shape.

**2. FR65 - open a citation in the editor.** An open button per code block
on the review board, next to copy, running a configured editor command
template. The JetBrains invocation is under "Mechanics discovered" in
[docs/07-field-requests.md](docs/07-field-requests.md).

**3. FR74's fullscreen marker** - built in session 34, never exercised.
Needs a real fullscreen window and Boris's consent to drive; read the
one-UI trap in `CLAUDE.md` first. The unknown: whether GTK honours a
4px-tall window at all.

## Where we are

Session 37 (2026-08-04) shipped FR81's second half: History, Inbox, Library,
Settings, Session and the app shell now speak the visual language Home and
Assignments established (UI caps for labels, mono for data, Home's tile
grammar, one recipe per control, serif empty states; Library rebuilt on the
shared list shape). Committed as `c915fab` + docs `48950dc`, pushed to
origin. Every surface was exercised live from an isolated worktree build -
screenshots dark and light, populated Library driven by pointer against a
copy of the real store. Full detail: history.md "Thirty-seventh session".

In parallel, a second agent built M12's last piece (the custom HTML panel
running in the artifact sandbox, two-way channel into `SetAssignmentParams`).
At write time they had landed `c15fce6` (a run keeps saved values when no
spec declares them) and still held the main panel diff uncommitted. Their
STATUS draft claims the panel runs end to end with 568 tests; treat that as
theirs to confirm, not this handoff's.

Read [docs/08-assignments.md](docs/08-assignments.md) before touching
assignments; two conventions live there (a run's last assistant message is
its summary; a fenced agentbox-data block becomes the run's `data` column).

## Live state (volatile - verify on resume)

- **Background jobs:** none from this stream. Agent B's session may still be
  running on this machine.
- **PRs:** none, ever (this repo pushes `main` directly).
- **Git:** at write time `main` was ahead of origin by B's `c15fce6` plus
  this handoff's own commit, and deliberately NOT pushed from this session -
  a push would publish B's commit while they might still amend it. B was
  asked via the ledger to push `main` when they land. Until that push **this
  handoff is LOCAL TO THIS MACHINE and invisible on boris-vm**. This
  stream's FR81 commits (`c915fab`, `48950dc`) are pushed. Uncommitted (ALL Agent B's, do not touch): the
  panel diff across Assignments.svelte, assignpanel.go+test, artifact.go+test,
  artifact libs, mdhtml.go, daemon.go+tests, policy tests, both manuals,
  docs/STATUS.md, docs/08-assignments.md, plus a rebuilt dist. Two of MY
  lines ride inside their dirty docs/STATUS.md (the FR81 header sentence and
  the removed "0d." queue item) - hunks merged, could not be staged apart;
  B folds them in (ledger note, ~11:15).
- **Deployed:** `1898d37` - pre-FR81, pre-panel. The desktop UI Boris sees
  does NOT yet show this session's restyle. Deploy after B lands (see Do
  this next). GitLab push-mirrors to GitHub on its own; a rejected direct
  `git push github` ("cannot lock ref") usually lost the race to the mirror -
  fetch and compare before treating it as failure. Pushing origin is enough.
- **In-flight edits (this stream):** none. FR81 is complete and committed.
- **History numbering:** session 37 = FR81 (committed); B was asked via the
  ledger to log theirs as session 38.
- **Carried from session 36, still pending on Boris:** delete the old-name
  fallback dirs (`rm -rf ~/.config/qq ~/.local/state/qq ~/.cache/qq
  ~/.local/share/qq`) after a few quiet days; schedule-or-delete the ad-hoc
  "Claude usage check" assignment (`a0eff4b720959`).
- **The session-34 `-race` flake watch item** did not recur in this
  session's gate runs either. Keep watching; do not close.

## Blocked on you (Boris)

Nothing - proceed autonomously after the ledger/git reconciliation above.
The two session-36 conveniences (fallback dirs, "Claude usage check") remain
whenever convenient.

## I can do solo (no input needed)

1. The reconciliation + deploy in "Do this next" (once B has landed).
2. FR73 (inbox detail reader).
3. FR65 (open citation in editor).

## Facts - verified vs assumed

- [verified] FR81 restyle exercised live this session: all six surfaces
  screenshotted from a demo build (dark; History+Library also light), and
  populated Library driven by pointer on a state copy - hover reveal,
  delete-confirm strip, Keep - all behaving.
- [verified] `make check` green in the isolated worktree (HEAD + FR81 only;
  564 tests, 21 packages, race on). NOT run over B's combined diff.
- [verified] `c915fab`'s dist was built from exactly its committed sources
  (worktree build), and both FR81 commits are on origin.
- [verified] Deployed daemon answered `1898d37` and `agentbox control state`
  answered "no run" after the last restore (~11:05).
- [assumed] Everything about Agent B's stream: their 568-test count, the
  panel running end to end, their STATUS/08-assignments edits - all from
  their uncommitted drafts, none independently exercised here.
- [assumed] Light theme on Inbox, Settings, Session (tokens verified on
  History and Library only); inbox triage keyboard focus handoff (code
  untouched, but unexercised since the restyle).
- [assumed] The FR74 marker's behaviour over a fullscreen window (unchanged
  since session 34).

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| Session-36 HANDOFF (rename fallout, queue, mirror race) | history.md "Thirty-sixth session"; mirror race + fallback dirs + usage-check carried above |
| The panel design brief from the old handoff's item 1 | Superseded by B's implementation; design record in docs/08-assignments.md + their session entry when committed |
| This session's working notes (audit, recipes) | history.md "Thirty-seventh session"; the recipes are IN the shipped surfaces |
| Parallel-agent coordination protocol | /tmp/agentbox-agents.md (volatile); pattern recorded in history.md session 37 |

## Map

1. [docs/STATUS.md](docs/STATUS.md) - current state, queue (B's uncommitted edits pending there at write time).
2. [docs/history.md](docs/history.md) - "Thirty-seventh session" is this one.
3. [docs/07-field-requests.md](docs/07-field-requests.md) - FR81 marked shipped; FR73/FR65/FR74 mechanics.
4. [docs/08-assignments.md](docs/08-assignments.md) - read before touching assignments.
5. [CLAUDE.md](CLAUDE.md) - the traps; the pkill one has a new costume (history.md, session 37).
6. `/tmp/agentbox-agents.md` - the parallel-agent ledger (volatile, may be gone after reboot).
