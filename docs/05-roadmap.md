# Roadmap

Each milestone ends with acceptance checks anyone can run.

Progress: M0 through M12 are DONE (M7's Wayland half stays deferred). STATUS.md
has the precise delta between this plan and what shipped; new work comes from
[07-field-requests.md](07-field-requests.md), which outranks the "Later /
parked" bucket below. The DONE milestones keep their plan and acceptance here;
the shipping narratives live in history.md (the "What M9/M10/M11 shipped"
sections and the dated entries).

## M0 - decisions closed [DONE 2026-06-12]

- ADR-0002 (toolkit) and the working-name decision (its ADR has since been
  retired) accepted; ADR-0006 (sound) at least proposed with a winner.
- Accept: every ADR in docs/decisions is `accepted` or explicitly parked.

## M1 - first card (spike + core) [DONE 2026-06-12]

- Toolkit spike: build the card mock from 03-ui-ux.md in Gio and in
  gotk4/libadwaita (ADR-0002), side by side, dark and light. Pick by
  screenshot comparison and code feel; record the loser's screenshots in
  the ADR.
- Then: `agentbox daemon` + `agentbox notify` end to end: socket, auto-spawn,
  card render, earcon, slide-to-history.
- Observability from day one (NFR13/14): JSON event log, startup banner,
  `agentbox version` with commit + build time, `agentbox logs`.
- Store bootstrap (NFR15): DB auto-created, migration runner, migration
  0001; startup order lock -> migrate -> socket -> UI.
- Accept: `agentbox notify --level warning --title test` from a terminal pops a
  card above all windows with sound in under 1.5 s cold, 300 ms warm.

## M2 - questions [DONE 2026-06-12]

- `agentbox ask` / `input` / `confirm` blocking round trip, timeout + default,
  cancel, exit codes, `--json`.
- `agentbox veto` countdown card with its exit-code contract.
- Reply-instead hatch (FR27), undo grace (FR28), `agentbox form` (FR26).
- Markdown v0 in card bodies: inline styles, lists, fenced code through the
  engine skeleton (the full engine is M6).
- Keyboard map complete; focus policy (pop above, never grab) verified.
- Accept: a script asks a 3-option question; answer arrives on stdout; a
  second pending question shows "1 waiting"; Esc defers; timeout applies
  default.

## M3 - memory and calm [DONE; inbox search/filters, agentbox veto and agentbox stats added 2026-06-13]

- SQLite store, inbox window with search/filters, DND, escalation engine,
  quiet hours, config file per 06-configuration.md with live reload.
- Toast stacking + collector and `agentbox demo` (the demo was **removed** in the
  sixteenth session, 2026-07-25 - `agentbox webui-demo` covers it without a daemon
  and without touching the queue). (FR21 `agentbox progress` was
  delivered later, 2026-06-13: the `agentbox progress` CLI, the `report_progress`
  MCP tool, and a non-blocking progress window. See the FR21 entry under the
  M8 section for how it folds into the future tabbed surface.)
- Presence gate (idle hold, fullscreen auto-DND, desktop DND), flood
  control, delayed items (`--in` / `--at`).
- Accept: kill -9 the daemon with two pending asks; restart re-presents both
  and the callers get answers (NFR7).

## M4 - agent integration [DONE 2026-06-13]

- `agentbox mcp` with notify_user / ask_user / confirm_action /
  act_unless_stopped / request_secret / ask_user_form (official go-sdk
  v1.6.1, ADR-0004); `agentbox secret` (FR23) lands here too. Claude Code
  `.mcp.json` and hook recipes in docs/recipes.md.
- Self-teaching binary (FR40-42): embedded manual (`agentbox docs`,
  `agentbox docs agent`, `agentbox docs setup`), teaching usage errors,
  `agentbox schema`.
- Accept: an agent given only the binary discovers and uses ask, veto and
  form via `agentbox docs agent` (the quickstart documents all three with
  examples, result shapes and exit codes). DONE.
- Accept: MCP handshake + tools/list verified end to end (all six tools,
  correct input schemas). A live "Claude Code asks through MCP and a Stop
  hook chimes" pass still needs a desktop session (headless CI cannot run
  the daemon).

## M5 - presence and polish [DONE 2026-06-13]

- Tray (SNI) with menu, autostart unit, summon shortcut docs, identity
  colors, motion polish, reduced-motion, attachment preview (FR16).
- Action buttons (FR32 DONE 2026-06-13), inbox triage mode (FR34 DONE
  2026-06-13), `agentbox stats` (FR35 done), diff review card (FR33 DONE
  2026-06-13).
- Calm/multi-agent refinements: missed-while-away marker (FR44 DONE
  2026-06-13), caller-alive indicator (FR45 DONE 2026-06-13), runtime agent
  mute `agentbox mute|unmute|mute --list` (FR47 DONE 2026-06-13). Per-agent
  sound signatures (FR46) dropped as redundant: agents are already told apart
  by the identity pill's hue and the waiting dots, and FR47 gives a direct
  lever to silence one - a distinct timbre per agent adds little for real
  plumbing cost.
- Presence gate completed (FR29 DONE 2026-06-13): on top of the FR44 idle
  monitor, hold_when_idle holds chimes and pauses escalation while idle then
  plays one summary chime on return; fullscreen_auto_dnd treats a focused
  fullscreen app as DND (X11 _NET_WM_STATE_FULLSCREEN); respect_desktop_dnd
  treats GNOME's show-banners=false as DND. Daemon logic unit-tested; the X11
  fullscreen read and the desktop-DND read are X11/GNOME-only and degrade to
  "present" elsewhere (Wayland fullscreen is an M7 gap).
- Accept: logout/login -> daemon up, tray present (with extension), DND
  toggle works from tray.

## M6 - markdown excellence [DONE 2026-06-13]

- Full engine per ADR-0008: GFM, alerts, footnotes, images, chroma code
  blocks with copy buttons, native charts (gonum/plot). Viewer (`agentbox
  show`) with `--watch` and find-in-page; `show_document` joins `agentbox mcp`.
- Golden-file tests: markdown in, layout tree out.
- Accept: a gnarly real-world README (tables, alerts, code, a chart) renders
  in dark and light; `agentbox show --watch` live-updates while the file is
  edited. Built and unit/golden-tested; `docs/sample.md` is the live-verify
  fixture (pixel-clean check needs a desktop session).
- Remaining refinement: wiring the non-default `[markdown]`/`[viewer]`
  config knobs (defaults already match). The rest of the original list
  shipped with M9: mermaid renders via a bundled engine, links open in the
  desktop browser, find gained prev/next.

## M7 - Wayland and packaging [packaging DONE 2026-06-13; Wayland DEFERRED]

- Packaging DONE 2026-06-13: `packaging/` has a systemd `--user` service
  (autostart + restart-on-failure), an `agentbox.desktop` launcher, and a 256px
  app icon (genicon now renders it alongside the tray variants). Makefile
  `install` / `install-bin` / `install-desktop` / `install-service` /
  `uninstall` targets install everything under `$HOME` with no root;
  `packaging/README.md` documents it. `.desktop` passes desktop-file-validate.
- Wayland DEFERRED (owner, 2026-06-13: not using Wayland now, revisit last).
  When picked up: validate activation/raise, the color-scheme portal, and
  fractional scaling on GNOME Wayland, and add the Wayland fullscreen signal
  (the FR29 fullscreen read and card placement are X11-only and degrade to
  "present"/WM-default on Wayland today).
- **The structural half landed early, on 2026-08-11 (FR100, ADR-0013), for a
  different reason.** Making the tree portable to macOS and Windows required the
  same seam Wayland needs - a placement layer that can be absent - so
  `internal/webui/x11_absent.go` now exists and `make check` runs the whole suite
  through it. A Wayland session takes exactly that path today: every surface
  appears and is answerable, placed by Mutter instead of by us. What is left of
  this milestone is the part that was always Wayland-specific rather than
  absence-specific: an activation token for re-focus, the fullscreen signal, and
  fractional scaling.
- Accept: same M1/M2 checks pass on a Wayland session (pending the Wayland
  work above).

## M8 - Claude Code session surface (post-v1) [DONE 2026-06-13]

A user-facing surface that starts and drives a Claude Code session in
AgentBox's own window, good enough to replace the terminal for a working
session - and, from 2026-06-13, the point where AgentBox becomes a real
desktop application: one window separating the
surfaces into clean tabs, with the session as one of them. The card/toast/
veto model and the daemon stay as they are; this is the housing around
them. It deliberately revisits two v1 non-goals (00-vision.md: "not a
terminal replacement", "not a chat client") as a post-v1 direction, and it
depends hard on M6 - the rendering engine is the whole point.

- Slice 1, the tabbed app shell, DONE 2026-06-13: one window housing inbox,
  history and progress; FR21's progress reports fold into a tab while the
  app is open and stand alone in their own window when it is not.
- Slice 2, the Claude session tab (FR49), DONE 2026-06-13: a headless
  `claude` child driven over stream-json, rendered through the M6 engine
  with live token streaming, multiple sessions, persistence and recall, and
  in-flow interaction - the child gets AgentBox's own MCP server and an
  AGENTBOX_SESSION_ID, and a session-tagged ask answers inline. Verified
  live against a real `claude` on 2026-07-25 (STATUS, Session surface).
- Slice 3, the settings tab, DONE 2026-06-13: a descriptor-driven editor
  over the surgical, comment-preserving config writer.
- The architectural requirement attached to all three: non-blocking
  concurrency - a running session, a long progress report and the inbox
  must stay responsive at once, which the single-window frame loop had to
  be designed around rather than given.

M8 shipped complete. What each surface does today is STATUS.md "What works"
(App, Session surface, Settings surface, Progress); the deliberately
deferred follow-ups - the stream-json control_request protocol so prompting
modes can approve inline, session keep-alive/`--resume` across a window
close, a working-directory picker - are queued there. The Gio build this
milestone first shipped in was replaced whole by M9; history.md's "Session
UI decisions (M8, live-tuned with the owner)" keeps the decisions that
survived the port.

## M9 - web UI port (Wails v3) [DONE 2026-07-25, started 2026-07-24]

The UI moved to a webview on 2026-07-24, so the agent-facing surfaces could
be genuinely beautiful and every element markedly more polished than its Gio
original. ADR-0009 records the decision and supersedes ADR-0002;
00-vision.md principle 6 is amended, not silently broken.

Architecture unchanged: `internal/webui` satisfies `daemon.Presenter` and
calls the same `Resolver` the Gio UI did, so daemon, queue, store,
protocol, MCP, sound and the session driver were untouched by the port.
Seven slices - shell + card, session surface, inbox + history, settings,
viewer + toasts + progress with the markdown renderer rebuilt behind them,
the inline ask panel, and the cutover that deleted `internal/ui`,
`internal/markdown` and the Gio dependencies. Slice by slice - including
the three defects the cutover surfaced and the GTK4/X11 mechanics that keep
vision principle 3 intact - in history.md, "What M9 shipped (the webview
port), in detail"; what each surface does today is STATUS.md "What works".

- Accept: MET on GNOME X11 with the daemon resident (2026-07-25). A card
  and a toast map above without taking the keyboard, `agentbox summon` then
  focuses the card, a key answers it and the FR28 grace delivers;
  `agentbox show --watch` re-renders on edit; a progress pipe fills its own
  window and reports "interrupted" when killed; the app window opens on
  every rail surface; a settings save reloads live, theme included, no
  restart. A new card window lands in ~360-400 ms (1.5 s budget); a queued
  item reuses the open window (300 ms budget). The session surface against
  a real `claude` was not part of the accept run; it was verified live the
  same day.

## M10 - the drop-down panel and live artifacts [DONE 2026-07-25]

Set 2026-07-25: a hotkey rolls a session panel down from the top of the
screen, quake-style, over everything; AgentBox renders everything the
official client renders and more - including interactive HTML the agent can
watch you use, with clicks and inputs reaching it in real time - and every
aspect of the application is configurable, live. Settled before code: the
daemon grabs the hotkey itself (no desktop configuration), the panel shows
the same sessions the app window shows, and artifacts get a sandboxed
runtime with React and Tailwind bundled, so a claude.ai-style artifact runs
as written.

Four slices, all DONE 2026-07-25: the panel (and with it every window
shape, the reading measure and the session defaults becoming live config,
the watcher at 400ms, the hotkey rebindable live), the artifact sandbox,
the interaction channel (blocking `await_artifact_event` and coalesced
`read_artifact_events`), and math + images. The mechanics - why the panel
pins at y=0 and animates its height, the sandbox bargain, the money rule,
the remote-image security fix - are in history.md, "What M10 shipped
(slices 1-4, 2026-07-25), in detail", and ADR-0010; the current behaviour
is STATUS.md "What works".

- Accept: a hotkey rolls the panel down over a fullscreen window in under
  200ms (done, slice 1); an agent shows an interactive artifact, the user
  clicks in it, and the agent acts on the click without the user touching
  the terminal - done and verified 2026-07-25: a real click in a sandboxed
  React artifact came out of a parked `agentbox artifact wait` as
  `run {"rows":500,"dryRun":false}`, exit 0.

## M11 - the voice [DONE 2026-07-25]

From 2026-07-25, AgentBox speaks, through the piper setup already on the
machine, so a notification carries its meaning to somebody who is not
looking at the screen. Settled before code: the agent writes the spoken
line (AgentBox never reads a title aloud), and the earcon still plays
first - the level rides the chime, the meaning rides the sentence.

Shipped as `internal/speech` - one synthesiser held open behind a
deliberately narrow engine contract (a line of text on stdin, raw s16le PCM
on stdout; piper satisfied as shipped, `[speech] command` takes any argv
that does) - a `speak` field riding the item so every gate the chime has
applies to the voice, and `agentbox say` / the `speak` tool for a line with
nothing on screen behind it. The measurements that shaped it (a voice loads
in ~2.5s while a sentence costs ~70ms, so no shelling out per line; the
resampler pinned to top quality because "as good as this machine can
manage" is not a preference) are in history.md, "What M11 shipped
(2026-07-25), in detail"; the current behaviour is STATUS.md "What works".

## M12 - assignments [DONE 2026-08-01]

Recurring AI work AgentBox runs on its own (FR81/FR82): the assignment
store, the scheduler, the runner that carries a run out as an ordinary
session, the seven authoring tools and the surface. Born as field requests
rather than planned here; the design and every decision behind it are
[08-assignments.md](08-assignments.md), the state is STATUS.md. One piece
open: the custom HTML parameter panel (stored and editable, not yet run in
the sandbox).

## M13 - multi-agent sync [DONE 2026-08-04]

Agents that can see, find and wait for each other, plus one surface where the
human watches all of them (FR83): the roster with purpose and activity, the
discovery rider, named locks with orphaning, signals with a global cursor, and
the compare-and-swap blackboard - then the teaching that makes it the default
rather than an option, which is hooks that put every session on the board with
no tokens. Born as a field request rather than planned here; the design and
every decision behind it are [09-sync.md](09-sync.md), the state is STATUS.md.
Nothing open.

## Later / parked

Field requests ([07-field-requests.md](07-field-requests.md)) outrank this
bucket: they come from sessions that hit a wall on real work.

- Multi-select questions (FR18), subscribe API (FR19), webhook relay for
  away delivery (FR20), native Mermaid, macOS port, packaging for distros.
  (FR39, math rendering, shipped in M10 slice 4.)
- Queue peek (FR48): preview queued titles/kinds without cycling.
