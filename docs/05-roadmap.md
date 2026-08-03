# Roadmap

Each milestone ends with acceptance checks that a fresh session can run.

Progress: M0 through M11 are DONE (M7's Wayland half stays deferred by the
owner). STATUS.md has the precise delta between this plan and what shipped;
new work comes from [07-field-requests.md](07-field-requests.md), which
outranks the "Later / parked" bucket below.

## M0 - decisions closed [DONE 2026-06-12]

- ADR-0002 (toolkit) and the working-name decision (its ADR has since been
  retired) accepted by Boris; ADR-0006 (sound) at least proposed with a winner.
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
  the Gio daemon).

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
- Remaining refinements: mermaid via mmdc (source shows for now), interactive
  link clicks, scroll-to-next-match in find, and wiring the non-default
  `[markdown]`/`[viewer]` config knobs (defaults already match).

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
- Accept: same M1/M2 checks pass on a Wayland session (pending the Wayland
  work above).

## M8 - Claude Code session surface (post-v1) [DONE 2026-06-13]

A user-facing tab that starts and drives a Claude Code session in AgentBox's
own window, good enough to replace the terminal for a working session. It
reuses everything built by M6: full markdown, highlighted code, charts,
tables. The terminal may still run alongside (the session is not hidden),
but the AgentBox surface is where you read and act.

Owner direction (2026-06-13): M8 is also where AgentBox becomes a real
user-facing desktop application, not just transient cards and a tray. The
main window separates AgentBox's surfaces into clean tabs - inbox, history/stats,
settings - with the Claude Code session as one of them, so starting and
running an agent without a terminal is part of a coherent app rather than a
lone window. The card/toast/veto interaction model and the daemon stay as
they are; this is the housing around them.

Slice 1 - the tabbed app shell - DONE 2026-06-13: `agentbox app` opens one
window (internal/ui/app.go) with Inbox / History / Progress tabs, switched by
click or Ctrl+Tab / Ctrl+1..3. The inbox was refactored from a standalone
window into an inboxView tab; the History tab renders `store.Stats` through the
M6 engine (summary + per-agent tables, per-day bar chart, 24h/7d/30d/all
selector); the FR21 progress window folds into a Progress tab when the app is
open and falls back to a standalone window when closed. `agentbox inbox` and the
tray now open the app. The single-window event model is the foundation the
session tab slots into. The "non-blocking concurrency" worry was narrower than
feared: the daemon only serializes cards, and the other surfaces already ran
independently, so the real change was just collapsing them onto one window's
frame loop while keeping heavy work (the future session stream) off it. Two
requirements attach to the rest of M8:

- Agent progress reporting (FR21, DONE 2026-06-13; folded into the app shell
  2026-06-13): an agent reports live progress and the UI shows it - a bar where
  the fraction is known, a spinner where it is not - with a completion toast.
  Shipped as `agentbox progress` (stdin-driven), the `report_progress` MCP tool,
  and a non-blocking progress surface outside the card queue. As of slice 1 it
  renders in the app's Progress tab when `agentbox app` is open and a standalone
  window otherwise.
- Non-blocking concurrency: using one feature must never freeze another. A
  running session tab, a long progress report, and the inbox must all stay
  responsive at once; no single surface or blocking call may stall the others.
  Today the daemon serializes one card at a time (FIFO + urgent-preempt) and
  the UI is single-window - the tabbed app needs surfaces that run and render
  independently, so this is a real architectural change to scope in M8, not a
  free consequence of adding tabs.

NOTE: this deliberately revisits two v1 non-goals (00-vision.md: "not a
terminal replacement", "not a chat client"). It is a post-v1 direction, not
a v1 scope change; do M0-M7 first. Depends hard on M6 (the rendering engine
is the whole point) and reuses the identity, sound, DND and inbox machinery
already shipped.

Slice 2 - the Claude session tab (FR49) - DONE 2026-06-13: the Session tab
(internal/ui/session.go) starts a `claude` child and drives it headless via
`claude -p --input-format stream-json --output-format stream-json --verbose
--include-partial-messages` (internal/session) - not a PTY/terminal emulation.
The NDJSON event stream is parsed on a reader goroutine and each turn rendered
through the M6 engine (prose, code+copy, tables, charts, tool-call chips) as a
scrollable conversation, with live token streaming (deltas re-parsed off-frame
on a throttle, reconciled by the authoritative `assistant` event). Delivered:
- Conversation view laid out for comfortable reading (measure, spacing,
  scrollback), not terminal reflow.
- Live typography: A-/A+ size and a Mono toggle, applied to the conversation
  without restart (the global `[font]` knob stays restart-only).
- Multiple sessions: a switcher, "+ New", and a Plan/Full permission-mode
  selector fixed at spawn (default plan = read-only).
- Persistence and recall: Save writes JSON + a markdown sibling under the state
  dir, Reopen loads the latest, Find tints matching blocks in-conversation.
- In-flow interaction: the child is spawned with AgentBox's own MCP server and an
  AGENTBOX_SESSION_ID; a session-tagged ask/confirm/notify renders in the tab's
  inline panel (answered through the same Resolver as the cards) instead of a
  popup; text/secret/form/diff/veto still pop a card.
The headline acceptance is met: a session runs end to end inside AgentBox - prompt
in, streamed markdown rendered richly, font/size adjustable live, the
conversation saved and searched - with the terminal closed (verified offscreen
+ unit; the live run is pending a desktop, STATUS "Do this next"). Deferred to
follow-ups: the stream-json permission control_request protocol (so prompting
modes can approve inline), session keep-alive + `--resume` across an app-window
close, and a working-directory picker.

Slice 3 - the settings tab - DONE 2026-06-13: the Settings tab
(internal/ui/settings.go) is a descriptor-driven editor for the config knobs.
It reads the current `~/.config/agentbox/config.toml` as the baseline and, on Save,
writes only the keys the user changed via a surgical, comment-preserving writer
(internal/config/write.go) - it edits a key's line in place (or appends under
the section / a new section), never clobbering comments, formatting or untouched
keys, and never materializing defaults. No new RPC: the daemon's existing
`config.Watch` reloads the live knobs within ~400ms (2s until M10). The live/restart split matches
`SetPolicy`, so theme/font, history, `dnd.start_in_dnd` and the log knobs carry
an "applies on restart" tag (the UI theme and logger are built once at daemon
start). Steppers share the loader's clamp bounds (volume/font/undo-grace), enums
constrain to valid values, and an invalid `quiet_hours` is refused before
writing. Verified by unit tests (writer line-surgery + round-trip, Save
diff-vs-baseline, restart-tag set) and `TestShotSettings` offscreen; the live
desktop pass is in STATUS "Do this next".

With slices 1-3 shipped, **M8 is complete** (app shell + session tab +
settings). Open follow-ups: the session-tab live run (real `claude`), inline
approval via the stream-json control protocol, session keep-alive/`--resume`,
and a live theme/font rebuild path.

## M9 - web UI port (Wails v3) [DONE 2026-07-25, started 2026-07-24]

Owner direction (2026-07-24): move the UI to a webview so the agent-facing
surfaces can be genuinely beautiful, and make every element markedly more
polished than its Gio original. ADR-0009 records the decision and supersedes
ADR-0002; 00-vision.md principle 6 is amended, not silently broken.

Architecture is unchanged. `internal/webui` satisfies `daemon.Presenter` and
calls the same `Resolver` the Gio UI did, so daemon, queue, store, protocol,
MCP, sound and the session driver were untouched by the port.

Slice 1 - shell + card - DONE 2026-07-24: Wails v3 app, the `Bridge` service
(the only thing the webview can call), config-to-CSS-token theming that
applies live, goldmark+chroma markdown to HTML with class-based highlighting,
and the card surface for every kind (choice/confirm/text/form/veto/secret/
notify) with the full keyboard map. `internal/webui/x11.go` keeps vision
principle 3 intact under GTK4 - realize early, zero the user time, map with
`set_visible` not `present`, then claim stacking and placement by client
message. The frameless card measures itself and asks Go to fit the window.

Slice 2 - session surface - DONE 2026-07-24: the frameless app window with
its own title bar, the surface rail, the session switcher, and the
conversation (reading column, user bubbles right, agent turns full width,
thinking collapsed, tool chips, markdown tables, chroma code, follow-the-
stream scrolling that yields the moment you scroll up). Prompts go through
`Driver.Send`; pushes are coalesced to ~16Hz.

Slice 3 - inbox + history surfaces - DONE 2026-07-24: the inbox (FR10) with
pending above recent, a live filter, severity stripes, identity dots, outcome
chips and the muted / missed-while-away markers; keyboard triage (FR34/FR50)
where the surface forwards the keystroke and Go decides what it means, so the
card and the inbox cannot drift; and history (FR35) as summary tiles, a
per-day bar chart and a per-agent table over a 24h/7d/30d/all window. The Go
side reads through a `Source` interface copied from `internal/ui` (no
dependency between the two UIs) and pushes `agentbox:inbox` on every queue
change, so the rail badge and the status strip are right whichever surface is
in front. The session list collapses on the surfaces it does not belong to.
First tests for `internal/webui` landed with it (triage table, outcome
encoding, snapshot, dispatch, stats encoding).

Slice 4 - settings surface - DONE 2026-07-24: a descriptor-driven editor with
five sections. Appearance follows the reviewed sketch (mode, ground, contrast,
accent swatches plus a picker, density, radius, the three type roles, base size,
code theme, motion) beside a live preview of a card, a toast and agent prose;
the behaviour sections carry every knob the Gio settings tab edited, so the
cutover loses nothing. The preview asks Go to resolve the pending values with
the same token builder every window uses, so it cannot drift from what Save
gives you. Save writes only the keys whose value changed, through the existing
comment-preserving writer, and lists the exact lines it wrote; a refused value
(bad enum, malformed hex, unparseable quiet hours) stops its own key and nothing
else, while out-of-range numbers clamp. Theme and font no longer carry an
"applies on restart" tag - the token push applies them at once, and the daemon
re-themes without waiting for config.Watch. Two knobs the sketch needed were
added on the way: `theme.contrast` (new) and `markdown.code_theme` (documented
but unwired until now).

Slice 5 - viewer + toasts + progress - DONE 2026-07-25: the three surfaces that
are not the card and not the app window, plus the markdown renderer they all
lean on.

- Toasts: a notify gets a strip at the top of the screen instead of a card in
  the middle - severity mark, tint, mini identity chip, a body clamped to three
  lines that expands in place, and either a countdown or a "click to dismiss"
  depending on whether the daemon set a deadline. Urgent is the carve-out and
  still gets a card with escalation (03-ui-ux.md). Which treatment an item gets,
  the icon a level wears and whether the strip is sticky are decided in Go, so
  the card and the toast cannot drift; the surface paints and takes the click.
- Viewer (FR36-38): `agentbox show` opens a reading window on a 760px measure with
  its own title bar, a watch badge, find-in-page with match count and
  prev/next, zoom, and j/k/g/G/q keys. `--watch` reloads from Go on an mtime
  change and the surface holds the scroll. The same component fills the app
  window's viewer surface, minus the chrome, fed by the same Go-side document.
- Progress (FR21): its own window, opened by the first report and closed when
  the last one finishes, pinned bottom-right so it never covers the middle of
  the screen where cards land, and mapped without taking the keyboard - a task
  starting in the background must not interrupt typing.
- Markdown, second edition (ADR-0008 amendment): GitHub alerts as tinted
  panels, `chart` fences drawn as themed SVG in Go (bar, line, area, scatter,
  pie, doughnut), mermaid diagrams rendered by a bundled engine loaded on
  demand, copy buttons and line numbers on code blocks, task lists, footnotes
  and definition lists. One stylesheet (`.k-md`) serves the card, the session
  and the viewer, so a table that renders in the reader renders in a card too.
  Links open in the desktop browser instead of navigating the window away.
- One bug this slice had to fix before anything else worked: Wails quits when
  the last window closes, which for a tray-resident daemon whose windows are
  transient would mean answering the first question kills AgentBox.
  `DisableQuitOnLastWindowClosed` is now set.

Slice 6 - the inline ask panel (FR49) - DONE 2026-07-25: a question from an
agent running in AgentBox's own session surface is answered in that conversation,
between the transcript and the composer, instead of in a card over the window
the answer is being read in. The routing rule, the controls and the keyboard are
all Go (`internal/webui/inline.go`), and the keystroke path is literally the
inbox's table, so a digit cannot mean one thing in a conversation and another in
the inbox. What the panel takes: choice, confirm and notify. What keeps its card:
text and secret (they need a field), form (six of them), diff (the window), veto
(a countdown with consequences), and anything urgent - the same carve-out that
sends an urgent notice to a card instead of a toast.

Two decisions the Gio build never had to make, because there a session died with
its window:

- Routing asks whether the app window is open. A question put into a
  conversation nobody can see is an agent waiting forever.
- If the window closes while a question is in the panel, the item is
  re-presented and gets its card. Verified live: the panel's item reappears
  dead-center with the same options.

The panel never takes the keyboard. The composer keeps focus, single keys act
only when nothing is being typed, and the hint line says which of the two you are
in - so a stray "1" mid-prompt cannot answer a production question, which is the
same reason a card maps without focus.

Slice 7 - the cutover - DONE 2026-07-25: `agentbox daemon` builds
`webui.New(res, log, cfg)` and gives `u.Run()` the main goroutine, the tray hooks
carried over unchanged (`OnView`, `OnAppChange`, `ToggleApp`, `AppOpen`,
`ShutdownApp`), and `config.Watch` gained `u.SetTheme(c)` - the live-theming route
the Gio build had no equivalent for. The theme construction in
`cmd/agentbox/daemon.go` is gone: `webui.New` takes the config and resolves `auto`
off GNOME's `color-scheme` itself. The FR29/FR44 presence monitor moved rather
than went, to `internal/presence`: deleting it would have left every presence
signal reading "present", which is the safe direction and therefore silent.
`internal/ui` and `internal/markdown` are deleted along with gio, gio/x,
gonum/plot and the embedded fonts, and `make build` now rebuilds `frontend/dist`
when a frontend source changed.

Two bugs the cutover exposed, both fixed with tests:

- Any daemon start with an unresolved item in the store died on SIGSEGV.
  `daemon.New` restores the item and presents it while the daemon is still being
  constructed - before `Run` - and `application.InvokeSync` dereferences a
  platform application that `Run` has not created yet. A cold-start `agentbox show`,
  `agentbox app` or `agentbox progress` hit the same window, since the socket is served
  a moment before the loop comes up; all three were confirmed to land there. Window
  work that arrives early is now queued and replayed when the loop starts, keyed so
  a repeat replaces its predecessor instead of opening a second window.
- `agentbox show FILE --watch` silently dropped `--watch` (Go's flag package stops at
  the first positional), so FR37 live preview never ran for anyone who typed the
  flag last - which is the order the docs used. `show`, `mute` and `unmute` now
  parse flags around their positionals.

- Tests: the Go side of the inbox, history, settings, toast, viewer, progress,
  markdown, inline-panel and main-loop-gate paths is covered (including a guard
  that every declared knob is readable and writable, so a half-added knob fails
  the build rather than the user). The surfaces themselves still have no automated
  check - the Gio offscreen harness did not port and nothing replaces it - and the
  card and session encoders have no Go-side tests either.
- Accept: MET on GNOME X11 with the daemon resident (2026-07-25). A card and a
  toast map above without taking the keyboard (`_NET_ACTIVE_WINDOW` unchanged),
  `agentbox summon` then focuses the card, a key answers it and the FR28 grace
  delivers; `agentbox show --watch` re-renders on edit; an `agentbox progress` pipe fills
  its own window and reports "interrupted" when killed; the app window opens on
  every rail surface; the settings surface saves one key and the daemon reloads it
  within ~2 s, theme included, with no restart. A new card window is on screen in
  ~360-400 ms (1.5 s budget) and a queued item reuses the open window (300 ms
  budget). Not covered by the accept run: the session surface against a real
  `claude`, which spends tokens and is Boris's call.

## M10 - the drop-down panel and live artifacts [DONE 2026-07-25]

Owner direction (2026-07-25): a hotkey should roll a session panel down from the
top of the screen, quake-style, over everything; it should render everything the
official Claude desktop client renders and more - every chart and visual element
an agent might choose to show - **including interactive HTML the agent can watch
you use**, with your clicks and inputs reaching it in real time. And every aspect
of the application should be configurable, live, so tuning AgentBox is a conversation.

Decisions taken with Boris before code: the daemon grabs the hotkey itself (no
desktop configuration); the panel shows the same sessions the app window shows;
artifacts get a sandboxed runtime *with React and Tailwind bundled*, so a
claude.ai-style artifact runs as written.

Slice 1 - the panel - DONE 2026-07-25: `internal/hotkey` (an X11 key grab, live
re-bindable), `internal/webui/panel.go`, `frontend/src/surfaces/Panel.svelte`,
`agentbox panel [--show|--hide|--state]`, and the `[panel]` config section. Two
measurements shaped it, both recorded in panel.go: **Mutter clamps a managed
window to the work area**, so a fixed-size window cannot be parked above the top
edge and slid down, and **the window is not translucent** even with
`WindowIsTranslucent`, so the CSS-transform approach is out too. The panel
therefore pins itself at y=0 and animates its *height*, with the surface anchoring
its content to the bottom of the viewport at a fixed height - the content never
reflows, and the conversation rides the growing edge down. Frames must be
dispatched one per main-loop turn; a loop that blocks the UI thread produces two
visible sizes and nothing between.

Also in slice 1, because live tuning was part of the same ask: every window shape,
the reading measure, the panel's geometry and the session defaults are
configuration ([window], [panel], [session]), the config watcher hands the whole
config to the UI (`SetConfig`), the open windows resize as you edit, and the
watcher polls at 400ms so it feels like a conversation. The hotkey rebinds live.

Slice 2 - artifacts - DONE 2026-07-25. `show_artifact` (MCP), an `artifact` fence
in agent prose, and `agentbox show --artifact FILE [--watch]` run agent-authored HTML
in a **sandboxed iframe**: `allow-scripts` with no `allow-same-origin` (opaque
origin, no reach into the surface) under `default-src 'none'` with no network
directive at all. Because nothing can be fetched, **React 19 and Tailwind v4 are
injected as text** from AgentBox's own bundle plus a JSX/TypeScript transform that
runs in the surface, so an artifact written for claude.ai runs offline as written.
Every artifact has a code/preview toggle, and `[artifact] enabled` is a live trust
switch enforced where the iframe would be created. The whole bargain, and the probe
that verified it escapes nothing, is
[ADR-0010](decisions/ADR-0010-artifact-sandbox.md).

An `html` fence is the ambiguous case and is decided by content: a document, or
markup that brings behaviour with it, runs; a table an agent is explaining stays a
code block. `internal/webui/artifact.go` owns that call and is tested on it.

Slice 3 - the interaction channel - DONE 2026-07-25. `window.agentbox.emit(name,
data)` posts to the surface, which checks shape and size; Go checks again
(`Bridge.ArtifactEvent`), and `internal/daemon/artifacts.go` hands the event to
whichever agent is waiting. Two arrivals, both wanted: **blocking**
(`await_artifact_event`, and `show_artifact` returns the `artifact_id` to wait on)
which parks an agent on the human the way `ask_user` does, with the same timeout
and the same event log; and **coalesced** - an event that arrives with nobody
waiting is buffered by (artifact, name) with the newest winning, so a dragged
slider is one final value rather than forty, drained by `read_artifact_events`.
`agentbox artifact wait|read` is the same channel from a shell. Artifact events never
enter the item queue or the store: a slider drag is not an interruption.

Slice 4 - parity extras - DONE 2026-07-25: math and images, the two things the
official client rendered that AgentBox did not.

Math is parsed in Go (`internal/webui/math.go`) rather than by a dependency,
because the interesting part is sixty lines of judgement: `$$...$$`, `\[...\]`,
`$...$`, `\(...\)` and a ```math fence all become the same inert node carrying
its TeX as element text, and the single dollar carries pandoc's money rule so
`$5 and $10` survives as money. KaTeX typesets it in the surface for the reason
mermaid renders there - the layout engine is JavaScript - with `trust: false`,
which is what makes its output safe to insert. A formula KaTeX refuses shows its
own source. KaTeX came into the tree as one of mermaid's dependencies and is now
a direct one, in its own 259 kB chunk rather than mermaid's 3.1 MB one: math in
prose is much more common than a diagram.

Images turned out to be a security fix rather than a feature. Raw HTML is
dropped before rendering, but `![alt](https://host/p.gif?d=...)` was ordinary
markdown that goldmark turned into a live remote `<img src>` - a request the
surface made on an agent's behalf, to a host the human never saw, with whatever
the agent chose to put in the path. `internal/webui/images.go` now answers the
question ADR-0010 answered for artifacts: Go reads an absolute local path,
checks the bytes really are a PNG/JPEG/GIF/WebP by magic number, and inlines
them as a `data:` URI under a 2 MB ceiling; a relative path, a remote host or a
scheme AgentBox does not know renders as a marked placeholder that says why.
`index.html` carries `img-src 'self' data: blob:` so it is enforced and not
merely intended. Encodings are cached by (path, mtime, size) because
`encodeTurns` re-renders every turn on every stream push.

The follow-up that slice 4 left open is closed (2026-07-25): a relative image
path now resolves against the document's own directory when there *is* one.
`RenderMarkdownIn(src, baseDir)` is the entry point the viewer uses; the base
travels in goldmark's parser context to an AST transformer that rewrites relative
destinations, because one engine serves every surface and a NodeRenderer sees
only the node. Everything else is unchanged by design: prose over the socket
still has no base, and a resolved path faces the same stat, sniff and ceiling, so
this added a way to name a file and not a way to reach one.

Remaining:

- Accept: a hotkey rolls the panel down over a fullscreen window in under 200ms
  (done, slice 1); an agent shows an interactive artifact, the user clicks in it,
  and the agent acts on the click without the user touching the terminal - **done
  and verified 2026-07-25**: a real click in a sandboxed React artifact came out of
  a parked `agentbox artifact wait` as `run {"rows":500,"dryRun":false}`, exit 0.

## M11 - the voice [DONE 2026-07-25]

Owner direction (2026-07-25): AgentBox should be able to speak, through the piper
setup already on the machine, so a notification carries its meaning to somebody
who is not looking at the screen. Decided with Boris before code: the agent writes
the spoken line (AgentBox never reads a title aloud), and the earcon still plays
first, so the level is carried by the chime and the meaning by the sentence.

Two measurements shaped it, both against piper and en_US-lessac-high:

  * Loading the voice costs ~2.5s and synthesising a sentence costs ~70ms. Three
    sentences through one process cost the same as one. So `internal/speech` holds
    the engine open instead of shelling out per line - spawning per notification
    would make every notification arrive three seconds after the thing it is
    about. The other half of that bargain is `idle_timeout_s`: a voice model is
    ~100MB resident, and an idle daemon should not hold one all night.
  * A piper voice is 22.05 kHz and this laptop's sink is 48 kHz, so **every
    utterance is resampled**, and PipeWire's default resampler quality is 4 of 15.
    AgentBox pins it to 15 and prefers the highest voice tier installed. Neither is a
    knob: "as good as this machine can manage" is not a preference.

The engine contract is deliberately narrow so it is not a piper integration: a
process that reads one line of text on stdin and writes raw s16le PCM on stdout.
piper satisfies it as shipped, and `[speech] command` takes any argv that does.
With the command empty, AgentBox finds piper and the best voice itself, and reads the
sample rate out of the voice's own JSON rather than guessing - a wrong rate is a
chipmunk, not a subtle degradation.

The spoken line rides on the item (`proto.Item.Speak`), so it inherits every gate
the chime already had: idle hold, mute, DND, quiet hours, and escalation, which
says it again because that is what escalation is for. `Sounder` grew a `Speak`
method rather than becoming a second interface, so a new `Play` call site cannot
be added that silently forgets the voice. `speak` is also a tool and an
`agentbox say` for a line with nothing on screen behind it.

## Later / parked

Field requests ([07-field-requests.md](07-field-requests.md)) outrank this
bucket: they come from sessions that hit a wall on real work.

- Multi-select questions (FR18), subscribe API (FR19), webhook relay for
  away delivery (FR20), native Mermaid, macOS port, packaging for distros.
  (FR39, math rendering, shipped in M10 slice 4.)
- Queue peek (FR48): preview queued titles/kinds without cycling.
