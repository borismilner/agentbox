# AgentBox - agent session notes

Go 1.26 daemon + embedded Svelte webui. `make help` lists targets; the Makefile
comments explain the why. What follows is only what a session must know before
touching anything.

## Traps that have cost sessions

- **Never `pkill agentbox` or `pkill -f "agentbox daemon"`.** Every Claude session on the
  machine holds an `agentbox mcp` child; name-matching kills the tools out from under
  every agent, and the -f form has killed the invoking shell. `make stop` and
  the Makefile's kill-daemons macro do it right.
- **`frontend/dist` is committed on purpose** (go:embed; machines without npm
  must still build). Editing a `.svelte` file and running `go build` directly
  embeds the OLD ui with no warning. `make build` handles the rebuild; without
  npm it silently embeds whatever dist holds.
- **A replaced binary is not a deployed binary.** The daemon keeps serving the
  old code from an unlinked inode. Deploy only via `make deploy`, and trust
  only `make deployed` (it asks the running daemon its build, not the file).
- **`make run` displaces the deployed daemon** (single instance by flock) - the
  one Boris's live sessions reach him through. `AGENTBOX_INSTANCE=dev make run` runs
  beside it; `make restart-daemon` restores the deployed build.
- **A second AgentBox shows no windows while another is running.** `AGENTBOX_INSTANCE=dev`
  buys a second socket and a second store, not a second UI: the first process
  owns `org.wails.agentbox` on the session bus (`busctl --user list | grep agentbox`), so a
  later one is a remote GApplication instance whose window requests go to the
  holder. It logs `walkthrough.opened`, exits 0, and nothing appears - the same
  for `agentbox webui-demo`. To see a UI from a working copy you must have the
  desktop's only daemon: `make stop`, run your build, then `make restart-daemon`.
  Point it at a copy of the state (`XDG_STATE_HOME=/tmp/x`, plus
  `XDG_CONFIG_HOME` for a theme) so Boris's own reviews and settings are not
  what you are experimenting on.
- **Never click a list row at a coordinate read off an earlier screenshot.**
  Any queue change reflows the inbox - somebody answering a card two rows up
  collapses the Pending section and everything moves - so the row you measured
  is not the row you hit, and it is somebody else's item you just opened. Filter
  first (type into the search box) so the target is the only row on screen, then
  click. The same goes for the Agents board. There is no lock protecting this:
  the queue is shared with every other session on the machine.

## Rituals

- Quality gate: `make check` (gofmt + vet + race tests). A PostToolUse hook in
  `.claude/settings.json` already runs gofmt, go vet, and `go fix -diff` on
  every Go edit - keep the tree clean of modernization nags.
- Deploy: commit first (deploy warns on dirty builds, NFR14), then
  `make deploy`; it ends by running `deployed` itself. `make rollback` restores
  the previous build. Two agents deploying at once is now serialized by an flock
  (`make deploy` waits and says who holds it), not by anybody remembering. It is
  deliberately NOT an `agentbox sync lock`: the deploy stops the daemon the lock
  would live in.
- Shared resources other than the deploy: take a lock. `agentbox sync lock
  NAME --timeout 600 -- CMD` from a shell, `acquire_lock` from an agent, and
  `agentbox sync locks` to see who holds what.
- Git: no PRs, ever - Boris pushes `main` directly. Small reviewable commits,
  `type(scope): description`.
- UI changes are done only after exercising the real webui (cards, board,
  keyboard paths), not after reading the diff.

## Where things live

- `docs/00`-`07` - vision through field requests; `docs/07-field-requests.md`
  tracks FR numbers used in commits and handoffs.
- `docs/agent-manual.md` - the MCP tool reference (`agentbox docs agent` prints it).
- `docs/history.md` - per-session log; `docs/STATUS.md` - current state.
- `HANDOFF.md` (repo root) - assignment state, written by /handoff, read by
  /resume.
- Daemon runs as `agentbox.service` (systemd --user); config at
  `~/.config/agentbox/config.toml` is Boris's - never revert his settings.
