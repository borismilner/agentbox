# Handoff - AgentBox: FR83 slice 1 is live, slices 2-4 are next

*Written by session 40, which triaged FR83 with Boris, mocked the surface, built
and deployed slice 1, and found two shipped bugs on the way.*

**Written:** 2026-08-04 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean, in sync with origin/main
git log --oneline -8        # head: this handoff; then c73427f, 030e4b8, aa4b77e, 16e1d18, 23fc4d5
make deployed               # c73427f9b2a2 or newer, and NOT "(dirty)"
agentbox sync agents        # the roster, live - this is the feature
agentbox control state      # "no run: the desktop is the human's"
```

**Announce yourself before you touch anything.** That is now the standing rule in
Boris's global `~/.claude/CLAUDE.md`, and this repo is where it was built, so
breaking it here would be absurd:

```bash
export AGENTBOX_SESSION_KEY="$(head -c8 /dev/urandom | od -An -tx1 | tr -d ' \n')"
setsid agentbox sync attach --area repo:agentbox >/dev/null 2>&1 &
agentbox sync announce "<why this session exists>" --area repo:agentbox
agentbox sync activity "<what you are doing now>"     # and again as it changes
```

Your own `mcp__agentbox__announce` will not work if your mcp child predates the
deploy - the handshake fixes the tool list. The CLI above always works. To
exercise a tool as an agent would, speak stdio JSON-RPC to a fresh `agentbox mcp`
(recipe under "Mechanics discovered" in
[docs/07-field-requests.md](docs/07-field-requests.md)); a working probe is at
`/tmp/claude-1000/.../scratchpad/mcp_probe.py` if it survived, and it is 80 lines
to rewrite if not.

### The one thing Boris has not seen

**The Agents surface rendering real roster data.** The canned mock was walked and
clicked with him; the live board has not been looked at by anybody. His screen was
black when the daemon was deployed (the whole X root captured as solid black), and
waking it was not mine to do. So:

```bash
agentbox app --tab agents     # then LOOK at it, do not assume
```

Grouping by area, the state chips, the session key on each row, and the "not
everybody" notice all have real data behind them now and have never been on
screen together. Expect defects; the mock's five were all found by looking.

## What is built, and what is not

**Slice 1 of FR83 is deployed.** Read [docs/09-sync.md](docs/09-sync.md) before
touching any of it - the whole design is there, the obvious alternatives are in it
as rejected options with reasons, and the slice list says what each slice must
prove.

Built: the session key on `proto.Identity` (`SameSession`), the attach stream, the
roster (`internal/daemon/sync.go`), `announce` returning same-area peers with
`partial`, `set_activity` generalized to write the roster always, `list_agents`,
derived areas, `agentbox sync` (`cmd/agentbox/sync.go`), the Agents rail surface
(`frontend/src/surfaces/Agents.svelte`, fed by `internal/webui/agents.go`), and
all four teaching doors.

**Not built, in the order the design wants them:**

1. **The discovery rider** - slice 1's one missing piece. The daemon should
   piggyback a `sync` member on the JSON-RPC response envelope when the caller's
   area roster changed since its last call, and the child should append that line
   to the tool result. Without it, a mid-work agent only learns about company when
   it next asks. Pick this up first; everything else in slice 1 is done.
2. **Locks** (slice 2) - and note the measured ceiling below, which changes the
   Makefile wrap the design describes.
3. **Signals** (slice 3), **shared values** (slice 4).

The mock lives on as `agentbox webui-demo agents` over canned rows, including the
lock and orphan cases slice 2 will need. `internal/webui/agents.go` has a
`mockBreak` that slice 2 deletes.

## Measured numbers, so nobody re-guesses them

- **A CLI hold dies at 120s.** A foreground shell call from a Claude Code session
  is killed at exactly 120s (SIGTERM, exit 143); an explicit timeout caps at 600s.
  `make deploy` here runs longer than that, so `agentbox sync lock NAME -- CMD`
  from an agent's shell cannot be the naive wrap: `--ttl` is the normal path, and
  a wrapped hold must release on SIGTERM or every long command leaves an orphan.
  Folded into 09-sync.md.
- **The MCP client's tool-call idle cap is still unmeasured.** It is the last
  guessed number in the design (`wait_max_s = 1500`) and it only matters once
  something blocks - so it gates slice 2 and 3, not the rider. Probe it by parking
  a call past the cap against a fresh `agentbox mcp`, once with progress
  notifications and once without.

## Two shipped bugs found on the way, both fixed

- **`Conn.Serve` never told a blocking handler its caller had hung up** (`aa4b77e`).
  `cancel()` was registered before `wg.Wait()`, and defers run
  last-registered-first. **FR45's caller-gone indicator had therefore never fired
  in the field**, and its test cancels the context by hand rather than closing a
  socket, which is why nobody noticed. FR83's presence rests on this. The new test
  in `internal/proto/hangup_test.go` closes a real socket and fails on the old
  order.
- **A provisional row lived forever** (`c73427f`). Found by using the feature, not
  by reading it: a hook or CLI announce creates a row with no attach behind it,
  and only attached rows had an exit event. Boris's board would have filled up one
  row per session start.

## Deferred by Boris, explicitly, until FR83 is finished

- **FR84** - a form card clips sentence-length choice options mid-word, and the
  fields sit below the fold. He sent a screenshot; his words were "we'll have to
  think of a better visual approach". Mock it before building it.
- **FR85** - one agent, two identity colours. Go hashes `agent + " " + project`,
  the frontend hashes `agent + "\0" + project`, and four of five sampled
  identities differ between a card's pill and an inbox row. The literal NUL is
  also why `grep`/`rg` skip `frontend/src/lib/tokens.js` as a binary file, which
  is how the second implementation stayed invisible. Fix: one separator, pinned by
  a test over a fixed table of identities.

Then the older queue, unchanged: **FR73** (a card body must be readable after the
card closes), **FR65** (open a citation in the editor), **FR74's fullscreen
marker** (built session 34, never exercised, needs consent to drive), and the
**"Claude usage check" assignment retry** (`a0eff4b720959`, failed on a model
limit, not a defect).

## Live state (volatile - verify on resume)

- **Deployed:** `c73427f9b2a2`, verified by `make deployed`, clean stamp (not
  dirty). It holds every code commit and the current embedded manual.
- **Git:** clean, `main` pushed to `origin` (GitLab). This session's commits:
  `bb2c965` `5943fff` (triage + FR84/FR85 docs), `23fc4d5` (the mock), `16e1d18`
  (the session key), `aa4b77e` (the rpc fix), `030e4b8` (slice 1), `c73427f` (the
  provisional-row fix), then the docs and this handoff. GitLab push-mirrors to
  GitHub on its own; pushing `origin` is enough.
- **Two other repos were touched**, both committed and pushed:
  `~/me/laptop-setup` gained the desktop-verification package set
  (`playbooks/03-packages.md`) and, at last, a **tracked copy of
  `~/.claude/CLAUDE.md`** - `snapshot.sh` had always copied it, but a blanket
  `CLAUDE.md` rule in the global gitignore meant git had always skipped it. A
  repo-local negation fixes that. `~/.claude/CLAUDE.md` itself is edited in place
  and is not in any repo.
- **A row of mine may still be on the roster.** Session 40 announced itself with
  key `fr83session` and left a `setsid agentbox sync attach` running. If you see
  it and it is stale, kill it by pid - **never `pkill agentbox` or `pkill -f`**,
  which killed this session's own shell once today when used on `webui-demo`.
- **`partial: true` is expected and correct** while any session's mcp child
  predates the deploy. This session's own child did, so its items arrived without
  a key, which is exactly the case the design predicted.
- **Background jobs:** none besides that attach. **PRs:** none, ever.
- **Usage:** Boris asked to be kept under 95% of his weekly limit. Read it with
  `claude -p /usage 2>/dev/null | grep -E '^Current (session|week \(all models\))'`
  (he aliased it to `cu`). It was 84% when this was written; the week resets
  2026-08-05 05:00 Asia/Jerusalem.
- **Rename fallout still on disk, on purpose** (session 36): `~/.config/qq`,
  `~/.local/state/qq`, `~/.cache/qq`, `~/.local/share/qq` are fallback copies;
  `~/.local/bin/qq` is a compat symlink.

## Facts - verified vs assumed

- [verified] A fresh `agentbox mcp` child registers `announce` and `list_agents`,
  announces itself, and gets the other session's row back with its purpose and
  activity (`alone: false`, plus the note telling it to coordinate). Its row is
  gone within the grace when the child dies. Run through stdio JSON-RPC against
  the live daemon.
- [verified] `agentbox sync announce/activity/agents` all work against the
  deployed daemon, and the state chip moves `quiet` to `working` when an activity
  line lands.
- [verified] The whole suite passes `make check` (gofmt, vet, race) with the
  roster tests in it, and the two bug-fix tests fail on the old code - checked by
  reverting each and re-running, not by assuming.
- [verified] No em-dashes, curly quotes or filler vocabulary in any file this
  session touched.
- [assumed] **That the Agents surface looks right with real data.** Nobody has
  seen it. This is the top item above.
- [assumed] That the hook recipes in `docs/recipes.md` work as written. They are
  the right shape and the CLI underneath them is exercised, but the hooks
  themselves have never been installed in a real `settings.json`.
- [assumed] That the child's attach survives a daemon restart and replays its
  announce. The replay path is written and unit-tested at the roster end; the
  child's redial loop has not been watched through an actual restart.
- [verified] `set_activity` writes the roster whether or not the caller holds the
  desktop, and re-sending an unchanged line does not reset its age.

## Map

1. [docs/09-sync.md](docs/09-sync.md) - FR83, the design. Slice 1 built; read before any sync work.
2. [docs/STATUS.md](docs/STATUS.md) - current state, what works, known gaps, the queue.
3. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers; FR83, FR84, FR85 are the newest.
4. [docs/history.md](docs/history.md) - session-by-session; this session is "Fortieth".
5. [docs/agent-manual.md](docs/agent-manual.md) - the tool reference, mirrored from `internal/manual/agent.md`.
6. [docs/recipes.md](docs/recipes.md) - the hooks that keep the roster honest for nothing.
7. [CLAUDE.md](CLAUDE.md) - traps that have cost sessions; read before touching the build or the daemon.
