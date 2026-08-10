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
- **`request_control` cannot protect a run that stops the daemon.** The HANDS OFF
  strip is a window the daemon draws, so `make stop` takes it down with everything
  else: you are granted the desktop, and the one signal telling Boris you hold it
  disappears at the exact moment you start driving. He is then looking at a card he
  did not raise with nothing on screen saying why, which is the confusion the strip
  exists to prevent - it happened on 2026-08-09 and he asked whether the session was
  stuck. This is the same shape as the deploy lock that cannot live inside the
  daemon the deploy stops. Until the strip outlives a restart, a check that stops
  the daemon must say in the turn, BEFORE `make stop`, what will be on screen and
  that the strip is about to vanish - and `set_activity` returning `{"live":false}`
  is how you know it already has.
- **A control can be fully styled in the stylesheet and unstyled on screen.**
  A segmented control in Board.svelte rendered as two bare words while its rules
  sat in the bundle with the right scope hash; moving it into its own component
  fixed it. Two habits came out of it: keep a control's CSS in the component that
  owns it rather than at the end of a seven-hundred-line stylesheet, and give
  every `var()` a fallback - a var() that resolves to nothing takes its whole
  declaration with it, so a control that has lost its background still reads as
  working to everything except the screen. Three build-deploy-look cycles went
  into believing the diff over the pixels.
- **`agentbox walkthrough open` on an ALREADY-OPEN board retargets the window
  without reloading the page.** After a frontend change, the surface you are
  looking at can be the old bundle even though the deploy succeeded. Close the
  window (or restart the daemon) before judging a UI change.
- **A commit message with an AI tell in it is refused, and the refusal kills the
  WHOLE command.** A guard hook scans commit and PR text (`harness` as a verb is
  one of the banned words) and blocks the Bash call before any of it runs - so a
  `python3 - <<PY ... PY && git add && git commit` one-liner leaves the file edit
  undone too, not just the commit. The symptom is a file that looks like the edit
  never applied, because it did not. Make the edit in its own call, or re-check
  the file after a blocked commit rather than assuming the edit survived.
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
- `tools/wiki/` - the wiki's own tooling, and the part a session will not guess:
  `draw.py` draws the pages' frames from fixtures (`frontend/draw/`), with
  `drawhtml` rendering their documents through the product's own markdown and
  artifact renderers and `shoot.mjs` capturing over CDP because a virtual clock
  photographs unfinished pages. `DRAW.md` is the runbook; `publish.sh` is the
  only path to the live wikis.
- `docs/wiki/` - the public wiki, live at
  https://gitlab.com/fu-bar/agentbox/-/wikis/home and mirrored to
  https://github.com/borismilner/agentbox/wiki. Read it when you need what a
  feature is FOR rather than how it works; `docs/wiki/FACTS.md` is the audited
  fact base (tool counts, severity behaviour, security guarantees, and the list
  of claims the older docs get wrong). Pages are written in `docs/wiki/pages/`
  and published by `tools/wiki/publish.sh`, never edited in a browser. Nothing
  mirrors wiki repos on its own, so a page changed here is not live until that
  script runs.
- `HANDOFF.md` (repo root) - assignment state, written by /handoff, read by
  /resume.
- Daemon runs as `agentbox.service` (systemd --user); config at
  `~/.config/agentbox/config.toml` is Boris's - never revert his settings.
