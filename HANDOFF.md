# Handoff - AgentBox: M12 complete, FR81 shipped, both live on the desktop

**Written:** 2026-08-04 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean, in sync with origin/main
git log --oneline -6        # head: this handoff; then 368ffbf/bacb64b/c15fce6 (panel), 48950dc/c915fab (FR81)
make deployed               # must answer this handoff's commit, CLEAN (no "(dirty)")
agentbox control state      # "no run: the desktop is the human's"
```

**Three** agents worked this checkout on 2026-08-04. Two shipped code and are
done, committed, pushed and deployed: M12's custom HTML panel (session 38) and
FR81's visual pass (session 37). The third (session 39) wrote a design and
touched no code. No code is in flight.

**First, though: Boris asked for a new feature this session and it is designed,
not built.** FR83, multi-agent coordination plus a live Agents surface, in
[docs/09-sync.md](docs/09-sync.md) with the field case in
[docs/07-field-requests.md](docs/07-field-requests.md). It is the newest thing
he asked for, it outranks everything below by the field-requests rule, and it is
waiting on three things only he can start: his triage of the three open
questions at the foot of 09-sync.md, the surface mock
(`agentbox webui-demo agents` over canned roster data, per this file's own
mock-first rule), and the slice-0 CLI spike. Do not start slice 1 without the
mock; the doc says why. The prerequisite inside it - a session key on
`proto.Identity` - also fixes a shipped FR74 defect, and that part could ship
alone if he wants a small win first.

Then the queue:

**1. FR73 - a card body must be readable after the card closes.** The inbox
row truncates the body to a tooltip and offers no detail view, so a card that
timed out takes its message with it. Boris hit this on 2026-07-31. Nothing new
needs storing - it is a reader. The Inbox surface was restyled on 2026-08-04;
extend it in its current shape (joined rows, UI-caps section labels, mono only
for data).

**2. FR65 - open a citation in the editor.** An open button per code block on
the review board, next to copy, running a configured editor command template.
The JetBrains invocation is under "Mechanics discovered" in
[docs/07-field-requests.md](docs/07-field-requests.md).

**3. FR74's fullscreen marker** - built in session 34, still never exercised.
Needs a real fullscreen window and Boris's consent to drive: take the desktop
with `request_control`, and read the one-UI trap in `CLAUDE.md` first. The
specific unknown is whether GTK honours a 4px-tall window at all.

**4. Retry the "Claude usage check" assignment** (`a0eff4b720959`) - its one
manual run failed on 2026-08-04 for an environmental reason, not a defect:
`You've reached your Fable 5 limit. /model to switch models.` The assignment
declares no model, so a run inherits whatever `claude` defaults to; Boris
switched his default to Opus 5 the same hour, so a retry may simply pass.
`run_assignment` then `assignment_runs` to see. It is still ad-hoc
(schedule-or-delete remains Boris's call).

## Where we are

Two parallel sessions on 2026-08-04 closed the two open items the 2026-08-03
handoff named.

**Session 37 - FR81's visual pass** (this stream): History, Inbox, Library,
Settings, Session and the app shell now speak the visual language Home and
Assignments established - UI caps for section labels with mono reserved for
data, Home's tile grammar (number first, mono tabular, lowercase label), one
recipe each for segmented controls, ghost buttons, search boxes and serif
empty states. Library was rebuilt on the shared header/search/joined-rows
shape (onboarding moved into its empty state, ages on the shared 1Hz ticker,
delete revealed on hover behind an inline confirm) and its inverted hover rule
went with the rewrite. Viewer already conformed and was untouched. Commits
`c915fab` + `48950dc`.

**Session 38 - M12's last piece** (the other agent): the custom HTML panel
runs in the artifact sandbox with a two-way channel - out through
`emit("params", ...)` into `SetAssignmentParams`, in through
`window.agentbox.params` and the `agentbox:params` event - plus an
`AssignmentsChanged` daemon poke that replaced the surface's 3s poll. Commits
`c15fce6`, `bacb64b`, `368ffbf`. Their record is history.md "Thirty-eighth
session"; STATUS.md carries the mechanics.

So **M0 through M12 are complete** and the app's surfaces read as one product.
Read [docs/08-assignments.md](docs/08-assignments.md) before touching
assignments: a run's last assistant message is its summary, and a fenced
agentbox-data block in it becomes the run's `data` column.

## Live state (volatile - verify on resume)

- **Background jobs:** none. **PRs:** none, ever (this repo pushes `main`).
- **Git:** clean, `main` pushed and in sync with `origin` (GitLab). **GitLab
  push-mirrors to GitHub on its own**: a direct `git push github` can lose the
  race and be rejected with "cannot lock ref"; fetch and compare heads before
  treating that as a real failure. Pushing `origin` is enough.
- **Deployed:** this handoff's commit, clean stamp, daemon answering. The live
  daemon has BOTH streams. It was briefly deployed dirty at 11:05 (built while
  this file was uncommitted) and redeployed clean afterwards - if `make
  deployed` ever reports `(dirty)`, that is the cause and a redeploy from a
  clean tree fixes it.
- **In-flight edits:** none from either stream.
- **The parallel-agent ledger** lived at `/tmp/agentbox-agents.md` (file
  ownership, dist rules, who may displace the desktop's one daemon). It is
  volatile and probably gone; the pattern is recorded in history.md session 37
  and is worth recreating if two agents ever share this tree again.
- **Rename fallout still on disk, on purpose** (from session 36):
  `~/.config/qq`, `~/.local/state/qq`, `~/.cache/qq`, `~/.local/share/qq` are
  fallback copies (delete after a quiet few days); `~/.local/bin/qq` is a
  compat symlink to `agentbox`.
- **How to exercise an MCP tool added this session:** you cannot call
  `mcp__agentbox__*` for a tool newer than your own `agentbox mcp` child.
  Speak stdio JSON-RPC to a fresh `agentbox mcp` instead - the recipe is under
  "Mechanics discovered" in [docs/07-field-requests.md](docs/07-field-requests.md).
- **The session-34 `-race` flake watch item** did not recur through either
  stream's gate runs. Keep watching; do not close it yet.

## Blocked on you (Boris)

Nothing - proceed autonomously. Two things when convenient:

- **Delete the old-name fallback dirs** once a few quiet days pass:
  `rm -rf ~/.config/qq ~/.local/state/qq ~/.cache/qq ~/.local/share/qq`
- **"Claude usage check"** is still ad-hoc. Give it a schedule
  (`daily 09:00`) or delete it - and note its one run failed on a model usage
  limit (see "Do this next" 4), not on anything AgentBox did.

## I can do solo (no input needed)

1. FR73 (the inbox detail reader).
2. FR65 (open a citation in the editor).
3. Retry the "Claude usage check" run and report what it does now.

## Facts - verified vs assumed

- [verified] FR81 is live in the DEPLOYED build, not only in a test build: the
  History surface was opened from the deployed daemon and screenshotted
  showing Boris's real data (47 interruptions, agents zsh/claude/claude-code/
  python3) in the new tile grammar, label voice and segmented control.
- [verified] The FR81 restyle exercised live during session 37: all six
  surfaces screenshotted from an isolated worktree build (dark; History and
  Library also in light), and a populated Library driven by pointer against a
  copy of the real store - hover reveal, delete-confirm strip, Keep.
- [verified] `make check` green on the FR81 stream in isolation (564 tests, 21
  packages, race on). The other agent reported 568 tests green over the merged
  tree.
- [verified] The failed run `r00e68daef6dc` failed on
  `You've reached your Fable 5 limit`, and used the panel's stored params
  (`warn_at: 90`, `window: "7d"`) - which is incidental live evidence that the
  no-spec params fix works.
- [verified] `agentbox control state` answers "the desktop is the human's";
  no control strip is stranded on screen.
- [assumed] Every claim about the panel's behaviour beyond the run record
  above - the two-way channel, the daemon poke - is the other agent's
  verification, read from their commits and STATUS, not re-exercised here.
- [assumed] Light theme on Inbox, Settings and Session (tokens were verified
  on History and Library only), and the inbox triage keyboard focus handoff
  (its code was untouched but unexercised since the restyle).
- [assumed] The FR74 marker's behaviour over a fullscreen window (unchanged
  since session 34: mechanism proven, the marker window never mapped).
- [assumed] That `claude --model <id>` accepts whatever id Boris types into an
  assignment. The flag is still passed through unvalidated.

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| Session-36 HANDOFF (rename fallout, queue, mirror race) | history.md "Thirty-sixth session"; the mirror race, fallback dirs and usage-check item are carried above |
| The custom-panel design brief that was item 1 of the 2026-08-03 handoff | Shipped; the contract is in docs/08-assignments.md + STATUS, the story in history.md "Thirty-eighth session" |
| This handoff's own earlier draft (written mid-flight, with branch logic for "is the other agent done?") | Resolved: both streams landed, so the branch collapsed to the linear queue above |
| Session-37 working notes (surface audit, the style recipes) | history.md "Thirty-seventh session"; the recipes are IN the shipped surfaces |
| The parallel-agent coordination protocol (/tmp ledger, volatile) | history.md "Thirty-seventh session", the two-agent paragraph |

## Map

1. [docs/STATUS.md](docs/STATUS.md) - current state, what works, known gaps, the queue.
2. [docs/history.md](docs/history.md) - session 37 is FR81, session 38 is the panel.
3. [docs/08-assignments.md](docs/08-assignments.md) - the M12 design, the panel contract; read before touching assignments.
4. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers used in commits; FR81/FR82 shipped, FR73/FR65/FR74 mechanics.
5. [docs/agent-manual.md](docs/agent-manual.md) - the agent-facing reference (`agentbox docs agent` prints it).
6. [CLAUDE.md](CLAUDE.md) - the traps that have cost sessions; read before touching the build or the daemon.
