# Handoff - AgentBox: FR83 designed, M12 and FR81 shipped, panel proved live

*Written by the FR83 session (39) and corrected by the panel session (38) after
its live pass; the live-state and deployed lines are the panel session's.*

**Written:** 2026-08-04 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean, in sync with origin/main
git log --oneline -6        # head: this handoff; then 728b537 (panel live pass), 3fcee1b/1c19d5a/ef1dc6c (FR83 docs)
make deployed               # 728b537a939e or newer - it holds every code commit AND the embedded manual
agentbox control state      # "no run: the desktop is the human's"
agentbox show docs/09-sync.md   # the design Boris asked for, in the reader
```

**The next feature is designed and not built: FR83, multi-agent coordination
plus a live Agents surface** - [docs/09-sync.md](docs/09-sync.md), field case in
[docs/07-field-requests.md](docs/07-field-requests.md). Boris asked for it on
2026-08-04 across five messages, so it outranks everything below by the
field-requests rule. It is 625 lines and it has already taken one adversarial
review; read it before proposing changes, because the obvious designs are in
there as rejected options with reasons.

**Three gates before slice 1, in this order.** Do not skip the first two; the
doc argues why, and FR58 is the precedent that earned the rule.

1. **Boris triages** the three open questions at the foot of 09-sync.md (how
   hard the announce gate is, whether Agents is its own rail surface, and how
   area granularity is decided).
2. **Mock the surface** - `agentbox webui-demo agents` over canned roster data:
   two working agents in one area, one asking, one blocked on a lock another
   holds, one dim and unannounced, one orphaned lock. Walk it with him before
   any daemon code exists.
3. **Slice-0 spike** - CLI-only lock and signal against a scratch daemon
   (`AGENTBOX_INSTANCE=dev`), driven by two real sessions doing real work. This
   is what tells you what the results must carry before the tool schemas
   freeze.

**One piece can ship alone if a small win is wanted:** the session key on
`proto.Identity`. Every sync primitive needs it, and it fixes a shipped FR74
defect - ownership is checked by `run.identity.Agent` string equality, so a
second same-named session can write the first one's hands-off activity line.
Proven live this session: the strip named its holder **`timeout`**, because
`Agent` is the parent process name and that session was launched under the
`timeout` command.

Then the queue, unchanged and still valid:

**1. FR73 - a card body must be readable after the card closes.** The inbox row
truncates the body to a tooltip and offers no detail view, so a card that timed
out takes its message with it. Boris hit this on 2026-07-31. Nothing new needs
storing - it is a reader. The Inbox surface was restyled on 2026-08-04; extend
it in its current shape (joined rows, UI-caps section labels, mono only for
data).

**2. FR65 - open a citation in the editor.** An open button per code block on
the review board, next to copy, running a configured editor command template.
The JetBrains invocation is under "Mechanics discovered" in
[docs/07-field-requests.md](docs/07-field-requests.md).

**3. FR74's fullscreen marker** - built in session 34, still never exercised.
Needs a real fullscreen window and Boris's consent to drive: take the desktop
with `request_control`, and read the one-UI trap in `CLAUDE.md` first. The
specific unknown is whether GTK honours a 4px-tall window at all.

**4. Retry the "Claude usage check" assignment** (`a0eff4b720959`). Its first
manual run failed on 2026-08-04 for an environmental reason, not a defect:
`You've reached your Fable 5 limit. /model to switch models.` The assignment
declares no model, so a run inherits whatever `claude` defaults to; Boris
switched his default to Opus 5 the same hour, so a retry may simply pass. A
second run fired the same morning from a stray click during the panel's live
pass - not Boris's, and worth ignoring when reading that history.
`run_assignment` then `assignment_runs` to see. Still ad-hoc (schedule-or-delete
remains his call).

## Where we are

M0 through M12 are complete and the app's surfaces read as one product. Three
agents worked this checkout on 2026-08-04: session 37 shipped FR81's visual pass
over the remaining surfaces, session 38 shipped M12's last piece (the custom
HTML panel running in the artifact sandbox with a two-way channel), and session
39 - this stream - designed FR83 and wrote no code. Both code streams are
committed, pushed and deployed. The design is committed and pushed. The stopping
point is clean: FR83 is fully specified and waiting on Boris's triage, and
nothing is half-applied anywhere.

Read [docs/08-assignments.md](docs/08-assignments.md) before touching
assignments: a run's last assistant message is its summary, and a fenced
agentbox-data block in it becomes the run's `data` column.

## Live state (volatile - verify on resume)

- **Background jobs:** none. At FR83-write time two other agent sessions were
  live and one **held the desktop**; the session-38 agent has since released it
  and `agentbox control state` answers "no run". Re-check before driving anyway.
- **PRs:** none, ever - this repo pushes `main` directly.
- **Git:** clean, `main` in sync with `origin` (GitLab). This stream's commits:
  `ef1dc6c` (FR83 field entry + docs index rows), `1c19d5a` (STATUS queue
  pointer + history session 39), `3fcee1b` (the ADR note), plus this handoff.
  `docs/09-sync.md` itself is committed inside `e77029f`, another agent's
  commit, because a catch-all `git add` swept it in while it was still
  untracked; the file is intact at 625 lines and was deliberately not rebased
  out. **GitLab push-mirrors to GitHub on its own**: a direct `git push github`
  can lose the race and be rejected with "cannot lock ref"; fetch and compare
  heads before treating that as a real failure. Pushing `origin` is enough.
- **Deployed:** `728b537a939e`, verified by `make deployed`. It holds every code
  commit and the current embedded manual (`agentbox docs agent` answers with the
  panel-key rule, which is how that was checked). The stamp reads `(dirty)`
  because another agent held uncommitted docs when it was built. With several
  agents in one checkout, `make deploy` from this tree will always stamp dirty -
  build and deploy from a clean worktree at HEAD for an honest stamp.
- **In-flight edits:** none.
- **Two git hazards this tree proved, 2026-08-04** (recorded by the session-37
  agent and by this one; both cost real recovery work). `git commit --amend`
  rewrote another agent's commit because they had committed between the commit
  and the amend. And a `git reset` dropped a finished, unpushed commit off
  `main` while its author was still working - recovered from the reflog, nothing
  lost. While more than one agent shares a checkout: **never `--amend`, never
  `git add -A` or `git add .`, always `git commit -- <explicit paths>`, and push
  promptly so a reset cannot strand your work.**
- **The parallel-agent ledger** lived at `/tmp/agentbox-agents.md`: file
  ownership claims, a hand-maintained `Current holder:` line for the desktop,
  dist rules, and timestamped notes between agents. It is volatile and will not
  survive a reboot, which is why FR83 quotes it at length rather than linking
  it. It is the workaround FR83 replaces.
- **Rename fallout still on disk, on purpose** (session 36): `~/.config/qq`,
  `~/.local/state/qq`, `~/.cache/qq`, `~/.local/share/qq` are fallback copies
  (delete after a quiet few days); `~/.local/bin/qq` is a compat symlink to
  `agentbox`.
- **How to exercise an MCP tool added this session:** you cannot call
  `mcp__agentbox__*` for a tool newer than your own `agentbox mcp` child. Speak
  stdio JSON-RPC to a fresh `agentbox mcp` instead - the recipe is under
  "Mechanics discovered" in
  [docs/07-field-requests.md](docs/07-field-requests.md).
- **The session-34 `-race` flake watch item** did not recur. Keep watching; do
  not close it yet.

## Blocked on you (Boris)

- **FR83's three open questions**, at the foot of
  [docs/09-sync.md](docs/09-sync.md). Slice 1 should not start before these:
  how hard the announce gate is, whether Agents is its own rail surface or a
  Home panel, and how area granularity is decided.
- **Whether FR83 is the next thing built at all**, or whether FR73/FR65 come
  first. Priorities have been reset twice in five sessions, and FR83 is a
  multi-slice feature.

Two older items, when convenient: delete the old-name fallback dirs
(`rm -rf ~/.config/qq ~/.local/state/qq ~/.cache/qq ~/.local/share/qq`), and
give "Claude usage check" a schedule (`daily 09:00`) or delete it.

## I can do solo (no input needed)

1. The FR83 surface mock (`agentbox webui-demo agents` over canned roster data)
   and the slice-0 CLI spike - both are throwaway by construction and both
   answer questions the design left open on purpose.
2. The session key on `proto.Identity` plus the FR74 ownership fix, which is
   useful on its own and is a prerequisite either way.
3. FR73 and FR65.

## Facts - verified vs assumed

- [verified] The design, the field entry, the index rows, the STATUS pointer and
  the history entry are committed and pushed; `git status` clean and in sync
  with `origin/main`.
- [verified] `docs/09-sync.md` is 625 lines and byte-identical to what was
  written, despite riding into another agent's commit (`git diff HEAD` empty).
- [verified] FR83 is reachable from every documentation door: `docs/README.md`,
  `docs/STATUS.md`, `docs/07-field-requests.md`, `docs/history.md` and this
  file.
- [verified] The deployed daemon is `728b537a939e` and answers `make deployed`,
  serving the current embedded manual.
- [verified] The custom panel works both ways on the live desktop, not only in
  tests: it loads already holding its values, a click inside it reaches SQLite
  and moves the typed knob, a panel-only key survives a knob turn, and an
  agent's `update_assignment` moves an open panel with nobody touching the
  window. What was exercised and what was not is itemised in
  [docs/history.md](docs/history.md), session 38.
- [assumed] The bar note a panel gets for emitting the wrong event name. The
  routing is unit-tested; the sentence has never been on screen.
- [verified] Another agent held the desktop at write time, and the strip named
  its holder `timeout` rather than an agent name - the identity defect FR83's
  session key fixes.
- [verified] The design took an adversarial review; the three findings that
  forced design changes are recorded in `docs/history.md` session 39.
- [verified] No em-dashes, curly quotes or filler vocabulary in any file this
  session touched (checked by grep over each file).
- [assumed] That the document opened for Boris with `show_document` is actually
  on his screen. The call returned `shown:true`, but the app window was already
  open, so it may have landed in that window's Viewer surface rather than a new
  window; another agent held the desktop, so nothing was raised or
  screenshotted to confirm.
- [assumed] That no third agent has committed since this file was written.
  Re-run `git log --oneline -6` before trusting the shas above.
- [assumed] Everything in 09-sync.md about how the primitives will behave. It is
  a design; nothing in it has been built or measured. The claims about the
  client's tool-call idle cap and the need for progress notifications came from
  review, not from an experiment - the slice-0 spike is where they get tested.

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| Nothing removed this session | The session added docs only: `docs/09-sync.md` (new), FR83 in `docs/07-field-requests.md`, two `docs/README.md` index rows (08 was never listed), a STATUS queue pointer and a history entry |
| The previous handoff's "both streams landed, nothing in flight" framing | Corrected in place: three agents, and this file's Live-state section names what was still live at write time |
| FR83's pointer to the volatile `/tmp` ledger as "still on disk" | Rewritten to quote the ledger's substance, since the file will not survive a reboot |

## Map

1. [docs/09-sync.md](docs/09-sync.md) - FR83, the design Boris asked for. Read before any sync work.
2. [docs/STATUS.md](docs/STATUS.md) - current state, what works, known gaps, the queue.
3. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers used in commits; FR83 is the newest, "Mechanics discovered" is at the foot.
4. [docs/history.md](docs/history.md) - session-by-session record; this session is "Thirty-ninth".
5. [docs/08-assignments.md](docs/08-assignments.md) - the M12 design. Read before touching assignments.
6. [CLAUDE.md](CLAUDE.md) - traps that have cost sessions; read before touching the build or the daemon.
7. [docs/README.md](docs/README.md) - the full docs index in reading order.
