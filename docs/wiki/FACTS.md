# Verified facts, read from source

> **In a hurry:** every number and list in the wiki comes from here, and every
> line here was read out of the code rather than out of a document. Audited at
> commit `69230d4`, 2026-08-07. When a doc and this file disagree, the doc is
> wrong. When this file and the code disagree, fix this file.

Why it exists: the docs had drifted in nine measurable places, and one of them
(`app --tab stats|progress`) had drifted in the code's own help text too, so
three sources agreed with each other and all three were wrong. A wiki written
from those would have shipped the same errors to strangers. Anything the wiki
claims, it claims from a line in this file.

## Counts

| Thing | Number | Read from |
|---|---|---|
| MCP tools registered | 39 | `internal/mcp/mcp.go:56-197` (24), `internal/mcp/sync.go:589-649` (8), `internal/mcp/assignments.go:25-65` (7) |
| MCP resources | 3 | `internal/mcp/standards.go:42-92` |
| MCP prompts | 1 (`walkthrough_standard`) | `internal/mcp/standards.go:42-92` |
| CLI commands dispatched | 35 cases | `cmd/agentbox/main.go:100-176` |
| Notification levels | 5 | `internal/proto/types.go:70-76` |
| Earcon classes | 6 | `internal/sound/sound.go:29-36` |
| App window surfaces | 9 | `frontend/src/lib/Rail.svelte:12-21`, `frontend/src/surfaces/App.svelte:146-172` |
| Config sections read | 22 | `internal/config/config.go:17-234` |
| Go lines, non-test | 41,017 | `cloc`-equivalent count over the tree |
| Go lines, tests | 22,843 | same |
| Svelte lines | 13,158 in 32 files | same |
| Go version | 1.26 | `go.mod:3` |

The showcase deck still says fourteen tools. It is stale and must not be quoted.

## The 39 tools

Notify and ask (8): `notify_user`, `retract`, `ask_user`, `confirm_action`,
`act_unless_stopped`, `ask_user_form`, `request_review`, `report_progress`.

Documents and artifacts (4): `show_document`, `show_artifact`,
`await_artifact_event`, `read_artifact_events`.

Voice (1): `speak`. Secrets (1): `request_secret`.

Desktop (4): `drive_desktop`, `request_control`, `set_activity`,
`release_control`.

Coordination (8): `announce`, `list_agents`, `acquire_lock`, `try_lock`,
`release_lock`, `post_signal`, `await_signal`, `shared`.

Assignments (7): `list_assignments`, `read_assignment`, `create_assignment`,
`update_assignment`, `delete_assignment`, `run_assignment`, `assignment_runs`.

Walkthroughs (6): `create_walkthrough`, `await_walkthrough`, `read_walkthrough`,
`list_walkthroughs`, `amend_walkthrough`, `delete_walkthrough`.

`amend_walkthrough` is registered and always refuses
(`internal/daemon/walkthroughs.go:346`). Its description says so. Do not sell
amendment.

No tool is conditionally registered. `internal/mcp/mcp.go:50-54` records the
decision not to gate the sync family behind a flag: eight always-refusing
schemas in every session's context would be a kill switch that kills the feature
and keeps the cost. Conditionality lives one layer down, in the daemon: sync
writes need a store (`internal/daemon/shared.go:59`) and an `announce` first
(`internal/daemon/signals.go:602-604`), while reads never refuse.

## Severity, and what each level actually does

Five levels: `info`, `success`, `warning`, `error`, `urgent`
(`internal/proto/types.go:70-76`, default `info` at `:394-398`).

| Behaviour | Rule | Line |
|---|---|---|
| Auto-dismisses | only a `notify` at `info` or `success` | `internal/daemon/daemon.go:1501-1510` |
| Waits to be read | a `notify` at `warning` or `error`, no timer armed | `daemon.go:1501-1511` |
| Default dismiss time | 6000 ms | `daemon.go:230`, `internal/config/config.go:257` |
| Gets a full card, not a toast | anything blocking, and `urgent` even when it is a notify | `internal/webui/toast.go:23-25` |
| Escalates | blocking items, and urgent | `daemon.go:1526-1528` |
| Escalation cadence | 60000 ms, or 20000 ms for urgent, capped at 5 replays | `daemon.go:1529-1532`, `:1558` |
| Pierces do-not-disturb | urgent only, and that can be switched off too | `daemon.go:1345-1346` |
| Pierces quiet hours | urgent only | `internal/sound/sound.go:161-164` |
| Interrupts a playing sound | urgent kills the current player, everything else is dropped | `sound.go:166-172` |

Six earcon classes rather than five, because blocking-ness picks the sound as
well as the level: urgent wins first (`insist`), then anything blocking gets
`chime` whatever its level, then success `tick`, warning `twotone`, error `thud`,
default `pop` (`internal/sound/sound.go:29-36`, `:41-54`).

## Surfaces of the app window

Nine, in rail order: `home`, `session`, `agents`, `assignments`, `inbox`,
`history`, `viewer`, `library`, `settings`.

`agentbox app --tab` validates nothing at any layer. The string travels from the
flag (`cmd/agentbox/main.go:1848`) through the daemon (`internal/daemon/daemon.go:787-797`)
into a URL (`internal/webui/app.go:60`). An unknown value renders a stub reading
"This surface is next in the port" and exits 0, so `--tab banana` shows a page
titled banana.

## Global hotkeys

| Default | Action | Registered | Config |
|---|---|---|---|
| `Ctrl+Alt+grave` | roll the drop-down panel down or up | `cmd/agentbox/daemon.go:442-446` | `[panel] hotkey` |
| `Ctrl+Alt+Escape` | pause or resume the handover | `cmd/agentbox/daemon.go:454-465` | `[control] pause_hotkey` |
| `Ctrl+Alt+Q` | recording mode on or off | `cmd/agentbox/daemon.go:470-481` | `[control] quiet_hotkey` |

An empty string disables a grab, all three rebind on config reload
(`daemon.go:557-559`), and a combination with no modifier is rejected
(`internal/hotkey/hotkey.go:127`). `agentbox summon` is not a grab: you bind it
yourself.

## Security, with the line that enforces it

| Claim | Enforced at |
|---|---|
| No network listener of any kind | `internal/server/server.go:95` is `net.ListenUnix`, and the tree has zero non-test hits for `http.ListenAndServe`, `net.Listen("tcp`, `ListenTCP` or `http.Server` |
| Socket is per-user and mode 0700 | `internal/server/server.go:35-47`, `:76` |
| Every connection's peer UID is checked | `internal/server/server.go:118-123`, via `SO_PEERCRED` at `:152-171` |
| A secret file is 0600, and an existing file is tightened | `internal/daemon/daemon.go:1995`, `:1999` |
| A secret never crosses the socket unless asked for | `internal/daemon/daemon.go:1976-1977`, `:1985-1987` |
| A secret is never stored | `internal/store/store.go:73-76` keeps it in memory only; history records that one was provided, never the value |
| A secret gets no undo grace | `internal/daemon/daemon.go:1960-1962` |
| An artifact cannot reach the network | `frontend/src/lib/artifact-runtime.js:33-45`, `connect-src 'none'` with `default-src 'none'` |
| An artifact has an opaque origin | `artifact-runtime.js:31`, sandbox is `allow-scripts` with no `allow-same-origin` |
| An artifact gets no camera, microphone or location | `frontend/src/lib/artifact.svelte.js:325` sets `allow=""` |
| An image may name a local file and nothing else | `internal/webui/images.go:125-134` returns empty for `~`, `//` and any URI scheme |
| An image is typed by magic number, not extension, and SVG is excluded | `internal/webui/images.go:45-46`, `:73-81` |
| Raw HTML never reaches a surface | `internal/webui/images.go:25-27` |
| A drive script's keystrokes are never logged | `internal/mcp/mcp.go:126` |

## Build identity

There is no version number. No git tags exist, and the binary carries no
ldflags. Identity is the toolchain's VCS stamp: revision, build time, dirty flag
(`internal/version/version.go:19-47`). `agentbox status` reports the daemon's
build and then the client's if they differ (`cmd/agentbox/main.go:648-663`),
which is the check `make deploy` relies on (`Makefile:295`).

## Runtime requirements

GTK4 and WebKitGTK 6.0 shared libraries, X11. Audio needs one of `pw-play`,
`paplay` or `aplay`, and its absence disables sound rather than failing
(`internal/sound/sound.go:61-63`, `:91-93`). Speech needs `piper` and a voice,
auto-detected. `npm` is optional because `frontend/dist` is committed.

Direct dependencies: Wails v3 alpha (the webview shell), the official MCP Go
SDK, `modernc.org/sqlite` (pure Go), goldmark, chroma, BurntSushi/toml,
jezek/xgb, fyne.io/systray, golang.org/x/sys.

## Do not claim these

| Claim in the docs | Reality |
|---|---|
| `app --tab stats` or `--tab progress` | Neither is a surface. `docs/agent-manual.md:1002` says both, and so does the daemon's own error message at `internal/daemon/daemon.go:794` |
| Six sync tools | Eight. `docs/STATUS.md:172-173` says six, and its own total of 39 in the same sentence does not add up with it |
| Fourteen tools | Thirty-nine. The showcase deck still says fourteen |
| `agentbox sync status` | Never implemented. `docs/09-sync.md:499` documents it |
| Progress folds into a tab | Progress is always its own window (`internal/webui/progress.go:12-16`). `docs/STATUS.md:491-493` says it folds in and `:329` correctly says it does not |
| A `[card]` config section | No `Card` struct exists. `docs/06-configuration.md:128-135` documents five knobs, all inert |
| A `[sound.earcons]` section | No field. Earcons are compiled-in constants and embedded WAVs |
| A `[viewer]` section | No field |
| `[toast] position`, `[toast] max_stack`, `[ask] default_timeout_s`, `[ask] answer_on_summon`, `[markdown] chart_palette` | None exist. Unknown keys warn and are ignored (`internal/config/config.go:365-367`), so they fail in silence |
| `[ask] allow_reply` works | Parsed, then read by nothing. `internal/webui/settings.go:93` admits it |
| `panel.height_frac = 0.62` is the default | The default is 0.5 and the clamp is 0.2 to 0.5 (`config.go:295`, `:409`), so the documented value is rejected and reset |
| `session.default_mode = "plan"` is the default | It is `"full"` (`config.go:325`) |
| A `[sync] enabled` flag | Does not exist. `internal/mcp/mcp.go:51` refers to it anyway |
| Amendment of a walkthrough | Registered and always refuses |
| Wayland | X11 only. Placement, fullscreen detection, the target lock, hotkeys, driving and `summon` all need X11 |
| `Ctrl+L` on a card jumps to the next waiting item, `Ctrl+I` opens the inbox | Neither is bound. `docs/03-ui-ux.md:122` listed both in what reads as the built keymap |
| The inbox has a level filter and a per-agent filter | One search box, matching title, body, agent, project, kind and state at once |
| Toasts stack three high and collapse into a "+N more" collector | One item at a time. The collector was specified and never built, which 03-ui-ux.md does say a few lines later |
| Earcons are all under 400 ms | Five are. `insist` is 430, measured from the embedded WAVs |
| There is a surface for reviewing what arrived while you were away | There is a chip on the inbox row and nothing more |

## Wrong inside the code, not only the docs

- `internal/webui/app.go:11-17` says "one window, five surfaces", then lists
  seven. There are nine.
- `cmd/agentbox/main.go:60`, `main.go:1837` and `internal/daemon/daemon.go:794`
  print three different lists of `--tab` values and all three are wrong.
- `agentbox help` never mentions the `control` family or `webui-demo`, shows two
  of fourteen `sync` verbs, and six of seven `walkthrough` verbs.
- `internal/mcp/mcp.go:51` cites a config flag that was never built.

## What was checked and found correct

`docs/00-vision.md` holds up. Principle 5 (local only, unix socket, no network
listener) is true and understated: there is also a peer-UID check the vision does
not claim. Principle 6's webview amendment matches what the binary needs.
Principle 2 (keyboard first) is borne out by the key tables in `Card.svelte`,
`Inbox.svelte` and `Board.svelte`.
