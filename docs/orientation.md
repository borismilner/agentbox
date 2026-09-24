# AgentBox - orientation

Read this first. It says what AgentBox is, everything it offers, where each
piece lives, and where to go deeper. Written against commit `130d183`
(2026-09-24). The 39 MCP tools and the CLI commands below were read from the
registration code, not from other docs. When this file and the code disagree,
the code wins and this file gets fixed. `docs/wiki/FACTS.md` is the audited
fact base the wiki is written against.

## What AgentBox is, in five lines

A desktop interaction hub for AI agents: one Go binary and a resident daemon.
When an agent needs a human (a decision, a credential, an approval, or just to
say it finished) a card appears above every window without stealing focus,
a short sound says what kind of thing arrived, and the answer goes straight
back to the blocked call. It also lets several agents on one machine see and
coordinate with each other, and lets the human watch all of them live.
Local only: unix socket, no network listener, no cloud, no telemetry.

```
  Claude Code · scripts · hooks · cron
        │ MCP (agentbox mcp)      │ CLI (agentbox <cmd>, stable exit codes)
        └────────────┬────────────┘
                     ▼  unix socket
              ┌──────────────┐     store (SQLite): history, ledger,
              │   daemon     │◄──  walkthroughs, assignments, sync state
              └──────────────┘
       cards · toasts · panel · app window · progress · viewer · tray
       sound · speech · real X11 input
```

## The principles that decide every trade

1. Unmissable, not annoying. 2. Keyboard first: the median answer takes under
two seconds. 3. Pop above, never grab focus: a stolen keystroke that answers a
question by accident is the worst failure. 4. Agent-agnostic: anything that
can exec or speak MCP. 5. Local only. 6. One binary. 7. The card is the
product: visual quality is a requirement. 8. Configurable, never demanding.
9. Self-teaching: the manual is in the binary (`agentbox docs agent`).

## Everything it offers: the 39 MCP tools

Every tool has a CLI twin. Blocking calls return the answer and map the outcome
to a stable exit code: 0 answered or yes, 1 no or vetoed, 2 usage, 3 unanswered,
4 transport.

### Ask and tell (8)

| Tool | CLI | Blocks | What it does |
|---|---|---|---|
| `notify_user` | `notify` | no | toast with a level (info, success, warning, error, urgent), sound, optional spoken line |
| `retract` | `dismiss` | no | withdraw a card the agent no longer needs answered |
| `ask_user` | `ask`, `input` | yes | 2 to 9 numbered options, or free text; `r` opens a reply hatch |
| `confirm_action` | `confirm` | yes | yes or no |
| `act_unless_stopped` | `veto` | yes | proceeds after a countdown unless the human stops it |
| `ask_user_form` | `form` | yes | several fields at once |
| `request_secret` | `secret` | yes | the value lands in a 0600 file; the agent gets the path, never the value |
| `request_review` | `review` | yes | the patch in the card, coloured; Approve or Request changes with a note |

Answers sit behind a three-second undo strip before they are sent. Esc defers
a card to the inbox.

### Show things (5)

| Tool | CLI | Blocks | What it does |
|---|---|---|---|
| `show_document` | `show` | no | markdown with tables, code, mermaid, charts, LaTeX, local images; `--watch` re-renders on save |
| `report_progress` | `progress` | no | live bars in a corner window, outside the card queue; one toast at the end |
| `show_artifact` | `show --artifact` | no | a page the agent wrote (React 19 and Tailwind preloaded), in a sandbox with no network |
| `await_artifact_event` | `artifact wait` | yes | park until the human uses the page; a dragged slider returns one value |
| `read_artifact_events` | `artifact read` | no | take what the human already did |

### Voice and hands (4)

| Tool | CLI | Blocks | What it does |
|---|---|---|---|
| `speak` | `say` | optional (`--wait`) | one line through piper or Kokoro, after the earcon |
| `drive_desktop` | `drive` | while running | real X11 input: pointer on a human curve, click, drag, type on the live layout |
| `request_control` | `control request` | yes | take the desktop; a HANDS OFF strip shows who holds it; the human can pause you |
| `release_control` | `control release` | no | give it back |

### Coordinate with other agents: sync (9)

| Tool | What it does |
|---|---|
| `announce` | state this session's purpose; returns the peers already here |
| `set_activity` | one line saying what you are doing now; keeps your row fresh on the Agents board |
| `list_agents` | the live roster: purpose, activity, state |
| `acquire_lock`, `try_lock`, `release_lock` | named locks; a dead holder is detected; breaking a lock reassigns it and notifies |
| `post_signal`, `await_signal` | wake a peer, or park until woken; replaces poll loops |
| `shared` | a small blackboard: get, set, compare-and-swap |

### Walked code review (6)

`create_walkthrough`, `await_walkthrough`, `read_walkthrough`,
`list_walkthroughs`, `amend_walkthrough` (refuses in this build),
`delete_walkthrough`. A durable step-by-step review board: TL;DRs, domains,
`path:line` binds, a glossary; the whole review comes back in one turn. The
authoring standard ships as an MCP prompt, `walkthrough_standard`.

### Assignments: AgentBox summons an agent (7)

`create_assignment`, `update_assignment`, `delete_assignment`,
`list_assignments`, `read_assignment`, `run_assignment`, `assignment_runs`.
A piece of work given to a Claude agent on a schedule or on demand, with the
whole toolbox available while it runs. The inversion of everything else:
here AgentBox calls the agent. Design: `docs/08-assignments.md`.

## CLI-only commands

`daemon`, `status`, `quit`, `inbox`, `pending`, `app`, `panel`, `stats`,
`dnd`, `mute`, `unmute`, `summon`, `sync`, `logs`, `store`, `schema`, `docs`,
`version`, `mcp`, `webui-demo`. `agentbox help` lists them with flags.

## Surfaces the human sees

| Surface | What it is |
|---|---|
| Card | one question, numbered answers, undo strip, countdown |
| Toast | non-blocking news in a managed top-centre column |
| Progress | small corner window of live bars |
| Viewer | the reading window for documents |
| Panel | a session panel that rolls down on a hotkey |
| App window | a rail of surfaces: Home, Inbox, Agents, Assignments, Library, History, Settings, Session |
| Board | the walkthrough review board |
| Control, Mark | the HANDS OFF strip and the desktop-control overlay |
| Tray | an icon showing what waits |

Around them: an inbox of what arrived while away (pending first, then the
day's history, live search), a ledger and `agentbox stats` (which agent is
expensive), DND, per-agent mute, quiet hours, a presence gate that holds sound
while the human is away, escalation for ignored urgent items, six earcon
classes.

## Safety, stated plainly

- A secret goes from keyboard to a 0600 file. The log records that one was
  asked for, never the value.
- An artifact runs with no network at all: no fetch, XHR, websocket, remote
  image or storage. `window.agentbox.emit` is the only way out.
- An image in agent prose may name a local file only.
- Exactly one level pierces DND, and that can be switched off.

## Platforms

Developed on GNOME/mutter on X11, which gets exact placement (card dead centre,
toasts top centre) above other windows without taking focus. Builds and runs on
macOS and Windows; `make check` compiles both on every run. X11-only: the
global hotkey and `drive_desktop`, and each says so when asked. The deployed
`agentbox` binary is a client build with no GTK or WebKit; the daemon carries
the webview.

## Non-goals

Not a chat client. Not a terminal replacement. No remote or mobile delivery in
v1 (a relay is parked as bet B-1). No cloud, no account, no telemetry.

## Relationship to rig

rig (`~/me/projects/rig`) is the platform meant to host the in-house programs.
Its peers service is planned to replace AgentBox's sync tools at rig's M16
cutover, and AgentBox's GUI is planned to move into rig's window. Neither has
happened; AgentBox runs on its own. Walkthroughs, assignments, artifacts and
`request_review` were ruled out of rig and stay AgentBox's.

## Where things live

| Path | What |
|---|---|
| `cmd/agentbox/` | CLI dispatch and daemon entry; built full and client-only (`-tags noui`), see architecture |
| `internal/server`, `internal/daemon` | the daemon, the queue, routing |
| `internal/mcp` | the MCP server and every tool registration |
| `internal/proto`, `internal/client` | the wire types and the client |
| `internal/store` | SQLite: history, ledger, sync, walkthroughs |
| `internal/webui` | the Wails webview windows |
| `frontend/src/surfaces/` | the Svelte surfaces listed above |
| `internal/presence`, `internal/sound`, `internal/speech` | presence gate, earcons, TTS |
| `internal/hand`, `internal/hotkey`, `internal/tray` | X11 input, the global hotkey, the tray |
| `internal/walkthrough`, `internal/assign`, `internal/session` | review boards, assignments, agent sessions |
| `internal/config` | every knob; see `docs/06-configuration.md` |
| `internal/manual` | the embedded manual behind `agentbox docs agent` |
| `docs/wiki/pages/` | the wiki source: what each feature is for |
| `docs/decisions/` | ADRs |

## Build, run, test, deploy

```sh
make check       # gofmt, vet, race tests, the no-X11 path, the client-only build, macOS and Windows builds
make run         # rebuild and restart the daemon from this tree
make deploy      # install client + full build and restart; takes a lock
make deployed    # ask the RUNNING daemon which revision it is
make webui-demo  # every surface, no daemon
make doctor      # what is installed and what is missing
```

Read `make check`'s exit code from a file, not through a pipe. In a git
worktree, `make deployed` stamps main's revision.

## Where to go next

| Question | Go to |
|---|---|
| every argument and exit code of a tool | `docs/agent-manual.md` or `agentbox docs agent` |
| why a feature is shaped this way | `docs/wiki/pages/` |
| requirements and design | `docs/01-requirements.md`, `docs/02-architecture.md`, `docs/03-ui-ux.md` |
| integration snippets | `docs/recipes.md` |
| current state and next step | `docs/STATUS.md` (a link into the maintainer's notes) |
| what to build next | `docs/backlog/README.md` |
| what similar projects exist | `logbook/projects/agentbox/prior-art-2026-09-24.md` |
