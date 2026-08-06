# Architecture

One binary, `agentbox`, in three roles: thin client CLI, resident daemon, MCP
bridge. The roles share one client library and one wire protocol.

```
  agent / hook / script                Claude Code (or any MCP host)
        |                                        |
        | exec: agentbox ask / notify         | stdio MCP: agentbox mcp
        v                                        v
  +-------------------- internal/client ---------------------+
  | dial $XDG_RUNTIME_DIR/agentbox/agentbox.sock              |
  | if absent: spawn `agentbox daemon` detached, retry <= 2 s |
  +-----------------------------------------------------------+
                            | JSON-RPC 2.0, ndjson framing
                            v
                   agentbox daemon
        +---------+----------+--------+-------+
        | queue + | store    | sound  | tray  |
        | escalate| (sqlite) | player | (SNI) |
        +---------+----------+--------+-------+
                  | channel to UI main loop
                  v
            UI: webview surfaces (Wails v3 + WebKitGTK, ADR-0009)
```

## Subcommands

| Command | Role |
|---------|------|
| `agentbox daemon` | run resident daemon (UI, queue, socket server) |
| `agentbox notify --level warning --title T --body B` | fire-and-forget event |
| `agentbox ask --title T --option "Run now" --option "Skip" [--timeout 300 --default "Skip"]` | blocking choice; prints chosen label, exit 0; exit 3 on timeout |
| `agentbox input --title T [--multiline]` | blocking free text |
| `agentbox confirm --title T` | blocking yes/no; exit 0 yes, 1 no |
| `agentbox progress --title T` | progress card; reads `PCT status...` lines on stdin until EOF |
| `agentbox veto --in 15 --title "Pushing to main"` | act-unless-stopped; exit 0 proceed, 1 vetoed (2 stays the usage-error code) |
| `agentbox secret --title T --to-file PATH` | masked secret entry; `--stdout` opt-in returns the value |
| `agentbox form --field "choice:env:staging,prod" --field "text:tag" --field "bool:notify"` | multi-field card, one round trip; results as JSON |
| `agentbox diff --title T < changes.patch` | diff review; exit 0 approved, 1 rejected; comment on stdout |
| `agentbox stats [--since 7d]` | interruption insights from history |
| `agentbox show FILE\|- [--watch]` | render markdown in the viewer; watch reloads live |
| `agentbox inbox` | open/raise the inbox window |
| `agentbox summon` | raise current card (bind to a desktop shortcut) |
| `agentbox dnd on\|off\|status` | do-not-disturb |
| `agentbox status` | daemon liveness, pending count (also health check) |
| `agentbox logs [--follow --level err --since 1h --json]` | read the daemon log without knowing paths |
| `agentbox docs [TOPIC]` | embedded manual; `agent` = context-sized quickstart, `setup` = paste-ready snippets |
| `agentbox schema` | JSON Schema for the protocol and item kinds |
| `agentbox version` | version, git commit, dirty flag, build time |
| `agentbox mcp` | MCP stdio server proxying to the daemon |

All blocking commands also take `--json` to print the full result object.
Caller identity flags `--agent`, `--project`, `--session` with sane defaults
(executable name of parent, basename of cwd, empty).

Dependency policy: stdlib first. Subcommand dispatch by hand (no cobra).
Expected deps: the UI toolkit, an MCP SDK, modernc.org/sqlite, a TOML
parser, a tray library, and the markdown stack (goldmark, chroma -
ADR-0008; charts are SVG drawn in Go, gonum/plot left with the Gio UI).
Everything else needs a reason.

## Wire protocol

JSON-RPC 2.0 over the unix socket, one JSON object per line. Methods are
versioned with a prefix:

- `agentbox.v1.notify(item) -> {id}`
- `agentbox.v1.ask(item) -> {id, answer?, answered, default_applied}` (response
  is sent when resolved; the connection stays open while blocked)
- `agentbox.v1.cancel({id}) -> {ok}`
- `agentbox.v1.list({state}) -> {items}`
- `agentbox.v1.dnd({set?}) -> {enabled}`
- `agentbox.v1.summon() -> {ok}`

- `agentbox.v1.progress({id?, percent?, status?, done?}) -> {id}` (create on
  first call, update by id)

`item` carries: kind
(notify/choice/text/confirm/progress/veto/secret/form/diff), level, title,
body (full markdown, ADR-0008), options[], fields[] (form), diff text, actions[]
({label, exec} pairs, FR32), timeout_s, default, deliver_at (FR31), strict
(disables the FR27 reply hatch), sound, focus policy, identity {agent,
project, session}, attachment path, and for secret items a sink (file
path; stdout only by explicit flag). Secret values never touch the store;
only the event metadata does.

Ask results are one of `{answer}`, `{reply}` (FR27 free-text escape),
`{answered: false, ...}` (timeout/defer path). Delivery waits out the undo
grace (FR28); undo silently rearms the item, the caller never sees the
retracted answer.

Daemon is single-instance per user session (NFR12). The authoritative lock
is `flock` on `$XDG_RUNTIME_DIR/agentbox/daemon.lock`, taken before the socket
is touched; only the lock holder may remove a stale socket and bind. A
`agentbox daemon` that loses the lock prints "already running" and exits 0, so
the auto-spawn race (N clients spawning at once) converges: one daemon
wins, every client's connect-retry reaches it. Explicit opt-out:
`--instance NAME` (or `AGENTBOX_INSTANCE`) switches lock, socket and state
paths to a per-name directory; clients reach a named instance the same way.

## Daemon internals

- Item lifecycle: `pending -> answered | expired | cancelled`. Every
  transition is appended to the store before the caller is unblocked (NFR7).
- Queue: FIFO within severity; `urgent` preempts the displayed card; the
  preempted card returns to the head of the queue.
- Escalation engine: per-item timer replays the earcon every
  `escalation.interval` (default 60 s) up to `escalation.count` (default 5);
  `urgent` also sets the WM attention hint each cycle.
- Presence gate (FR29): subscribes to the desktop idle monitor, fullscreen
  state of the focused window, and the desktop DND setting (mechanisms in
  04-platform.md); gates sound and escalation, never visibility of already
  shown items.
- Scheduler (FR31): items with `deliver_at` sit in the store and enter the
  queue when due; they survive restarts like any pending item.
- Flood control (FR30, `internal/daemon/flood.go`): a sliding window per
  session key (defaults in 06-configuration.md); over the limit, new items merge
  into one stack item of kind `stack` and the collapse itself carries the
  warning - it is a warning-level card that says what happened, rather than a
  second card beside the first, because two cards for one flood is the noise
  this feature exists to end. Nothing is dropped: every collapsed item is stored
  and pending exactly as it would have been, and the stack card is a different
  way of SHOWING items that all still exist. A blocking call caught in a burst
  is opened from its row (which promotes the real item back onto the screen) or
  from inbox triage; dismissing the stack takes the notifications with it and
  deliberately leaves the questions pending.
- Action exec (FR32): runs `sh -c` as the user with the item's cwd; output
  goes to the daemon log, non-zero exit raises an error toast. No new
  privilege boundary (caller and clicker are the same user); the risk
  guarded against is a misleading label, hence the verbatim-command
  tooltip and the global kill switch.
- Store: the application database (ADR-0005, NFR15) - SQLite via modernc,
  auto-created at start, embedded forward-only migrations applied after
  the instance lock and before the socket binds. `items` + `transitions`
  first; every later persistence need is a new migration, never a side
  file. History queries power the inbox search.
- UI bridge: daemon logic never touches toolkit types; it sends display
  commands over a channel into the toolkit main loop, answers come back the
  same way. Keeps the core testable headless.

## MCP bridge

`agentbox mcp` speaks MCP over stdio and proxies to the daemon socket, so it
inherits auto-spawn. Tools (descriptions matter - the model reads them):

| MCP tool | Maps to |
|----------|---------|
| `notify_user` | notify |
| `ask_user` | choice or free text (options present -> choice) |
| `confirm_action` | confirm |
| `act_unless_stopped` | veto |
| `request_secret` | secret (file sink; the tool result is the path, not the value) |
| `ask_user_form` | form (typed fields, one round trip) |
| `request_review` | diff (result: approved + optional comment) |
| `show_document` | viewer (markdown content or file path) |

The shipped set has grown past this design table - `agentbox mcp` serves 30 tools
today (speech, desktop driving, artifacts, walkthroughs);
[agent-manual.md](agent-manual.md) is the current reference.

Registration for Claude Code (`.mcp.json`):

```json
{"mcpServers": {"agentbox": {"command": "agentbox", "args": ["mcp"]}}}
```

Hook recipes (no MCP needed): a Notification hook calls
`agentbox notify --level info ...`; a Stop hook calls
`agentbox notify --level success --title "Agent finished"`. Exact snippets are in
[recipes.md](recipes.md).

## Self-documentation

The binary is the manual (FR40-FR42). docs/ is embedded via go:embed at
build time, so documentation and behavior version together; `agentbox docs`
serves it. `agentbox docs agent` is a hand-written, context-budgeted page
(target under 2k tokens) that teaches an agent every capability with one
example each - it is the file an agent drops into its own instructions.
Misuse errors route through the same usage metadata that powers --help, so
the correct form with a concrete example is always in the error itself.
MCP tool descriptions are written to the same standard: they are the only
docs an MCP host shows the model.

## Observability

Logs are the primary debugging interface (NFR13); AgentBox's maintainers are
AI agents reading JSONL, not humans watching a terminal.

- slog JSON handler to `$XDG_STATE_HOME/agentbox/log/agentbox.jsonl`,
  size-rotated (retention in config). Default level info; debug adds full
  IPC payloads. Secret values are redacted unconditionally.
- Stable event vocabulary, one line per event: `item.created`,
  `item.displayed`, `item.answered`, `item.undone`, `item.expired`,
  `ipc.call`, `daemon.spawned`, `daemon.lock_lost`, `config.reloaded`,
  `render.failed`, `sound.failed`, `flood.triggered`, ... with keys ts,
  level, component, event, item_id, agent, err (wrapped chain), stack
  (panics only). Grep-able and machine-parseable without the source.
- Startup banner: version, commit, dirty flag, build time, Go version,
  session type, desktop, config snapshot. The first log line answers
  "what exactly is running, where".
- Panics in any goroutine are recovered at the top, logged with stack, and
  the daemon exits nonzero rather than limping with unknown state.
- Build provenance (NFR14) comes from `debug.ReadBuildInfo` (vcs.revision,
  vcs.time, vcs.modified) - embedded by the Go toolchain on any build from
  a git checkout, surfaced by `agentbox version` and `agentbox status --json`.

## Package layout

```
cmd/agentbox/  entry point, subcommand dispatch
internal/proto/   item types, JSON-RPC framing (no deps)
internal/client/  dial + auto-spawn, used by CLI and MCP bridge
internal/server/  socket listener, peer-cred check
internal/daemon/  queue, lifecycle, escalation (headless, fully testable)
internal/store/   sqlite app database, embedded migrations + runner
internal/sound/   earcon playback (ADR-0006), embedded assets
internal/webui/   the webview UI (ADR-0009): card, toast, app window, viewer,
                  progress, settings, inbox, history, session; markdown to HTML
internal/presence/ X11 idle / fullscreen / desktop-DND signals (FR29/FR44)
frontend/         Svelte surfaces, built to dist/ and embedded with go:embed
internal/tray/    StatusNotifierItem
internal/mcp/     MCP stdio server
assets/           sounds, icons (go:embed)
```
