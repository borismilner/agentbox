# STATUS

Updated: 2026-08-07, fifty-third session (the wiki, and every doc claim checked
against source).

**This file is the current state, and only that.** What each session shipped,
broke, learned and verified is in [history.md](history.md), which is the log; the
resume brief with live state and copy-pasteable commands is
[../HANDOFF.md](../HANDOFF.md); and what a feature is FOR, in a stranger's terms,
is the wiki in [wiki/](wiki/). A narrative of the last fourteen sessions used to
sit here as well as in history.md, and it is gone rather than shortened.

Where it stands: milestones M0 through M13 are done, FR83 (multi-agent
coordination) and FR95 (recording mode) are complete, and what remains is a
verification and refinement queue. The showcase re-record was dropped for good on
2026-08-06 (priority 2 below says what that means for `tools/showcase/`). Session
53 audited every factual claim in these docs against the code and found nine
config keys nothing reads, three wrong verb lists, two wrong defaults and a tool
count that had been wrong in three places; [wiki/FACTS.md](wiki/FACTS.md) is the
audited base, and it is what to quote from now on.

AgentBox is **deployed and live on this machine**: module
`github.com/borismilner/agentbox`, binary and CLI `agentbox`, socket `agentbox.sock`, config
`~/.config/agentbox/`, state `~/.local/state/agentbox/`, unit `agentbox.service` (enabled,
active, systemd owns the daemon), MCP server `agentbox` (user scope, tools
`mcp__agentbox__*`), artifact API `window.agentbox.emit`, wire methods `agentbox.v1.*`.

M12 (assignments) is done including its last piece: the custom HTML panel runs in
the artifact sandbox with a two-way channel - values out through
`emit("params", ...)` into `SetAssignmentParams`, values in through
`window.agentbox.params` and the `agentbox:params` event, and the daemon pokes open
surfaces (`agentbox:assignments`) on every mutation so an agent's edit lands in a
panel somebody is looking at. See [08-assignments.md](08-assignments.md).

**FR83 is complete: agents can see, find and wait for each other.** Presence and
discovery, named leases, stored signals and a compare-and-swap blackboard, all
deployed and verified live, with hooks that put every session on the board with no
tokens and no instruction. [09-sync.md](09-sync.md) is the reference and is not
summarised here, because each of the five slices found something the design had
wrong and every one was found by running the thing rather than reading the diff.

Two bugs that had been shipped for months fell out of building it, both fixed and
both worth remembering here rather than in one slice's record: `Conn.Serve` never
told a blocking handler that its caller had hung up, so **FR45's caller-gone
indicator had never once fired in the field**, and every blocking card was on an
undocumented 30-minute fuse (FR88), which meant the human's card stayed up and his
answer arrived at a caller that was already gone.

## Deployed, and reachable from every Claude session

AgentBox is not only a repo. `~/.local/bin/agentbox` holds the current build, and it is
registered as a **user-scope** MCP server named `agentbox` in `~/.claude.json`, so
all 39 tools are available in every Claude Code session in every project -
not just in this one. Sessions that were running at the rename hold dead
tools from the old registration and must restart. `~/.claude/CLAUDE.md`
carries an AgentBox section telling an agent what the tools are for and when an
interruption is worth its cost, which is the part a tool list cannot say.
The daemon autostarts with the desktop and outlives whatever summoned it.
A compatibility symlink at the old binary name covers stray scripts through
the transition.

`make deployed` reports the running daemon's own build, which is how a
deploy is checked rather than hoped about. `agentbox stats --since 30d` reads the
full pre-rename history (311 interruptions at the latest move), which is the
proof the state move lost nothing. The old config, state and cache dirs are
still on disk as a fallback; delete them once a few quiet days pass.

## What works (all verified live on GNOME 46 / X11)

- Daemon: single instance per user session (flock), unix socket with
  peer-UID check, auto-spawned by the first client call. `AGENTBOX_INSTANCE`
  isolates socket and state. `agentbox quit` stops it gracefully.
- Interactions: `agentbox notify|ask|input|confirm|veto|form`; answers by keys
  (1-9, y/n, s=stop a veto, Enter=default, r or / = free-text reply hatch,
  u undo, c copy, Esc defer/dismiss) or mouse; results on stdout, exit codes
  0/1/2/3/4 documented in `agentbox help`.
- Veto (FR22): `agentbox veto --in 15 --title T` shows an act-unless-stopped
  card with a live "proceeding in Ns" countdown; window elapses -> proceed
  (exit 0, `{vetoed:false}`), `s`/Stop -> vetoed (exit 1, `{vetoed:true}`).
  No undo grace (the countdown is the deliberation window). Window is
  authoritative in handleSubmit so it fires even while away/DND; daemon
  fills `[veto] default_window_s` (15) when the caller omits `--in`.
- Agent integration (M4): `agentbox mcp` is an MCP stdio server (official
  modelcontextprotocol/go-sdk v1.6.1) exposing 39 tools (the seven
  assignment tools since M12, the eight sync tools since FR83); each proxies to
  the daemon over the socket (auto-spawn). `agentbox docs` / `docs agent` /
  `docs setup` serve the embedded manual; `agentbox schema` prints the wire JSON
  Schema. Recipes in docs/recipes.md.
- Desktop driving: `internal/hand` synthesises real X11 input, reachable as
  `agentbox drive`, the `agentbox.v1.drive` RPC and the `drive_desktop` MCP tool - how
  the showcase answers its own cards with nobody at the keyboard. **A named
  window is a target lock** (FR77, session 33): `window TITLE` raises it,
  follows it if it moves, and checks before every click that the pointer is
  really over it and before every type/key that the keyboard is really in it
  (`QueryPointer` walked to the deepest child; `GetInputFocus`). A mismatch
  raises the target and looks once more, then fails the step naming what was
  there instead, so a window that closed, moved or was covered stops the
  script rather than sending keystrokes into another document. Menus,
  tooltips and the target's own dialogs count as the target (override-redirect
  and `WM_TRANSIENT_FOR`); `screen` gives the lock up and enforces nothing.
  `internal/hand/target.go`. Typing is
  layout-proof: every synthetic press locks the keyboard group it was
  planned against immediately before it goes down (`internal/hand/xkb.go`;
  per-stroke because GNOME re-asserts the human's group within about a
  millisecond), verified live with Hebrew active. Every surface has its own
  window title - card `agentbox`, toast `agentbox · toast`, app `agentbox · app`, progress,
  panel and viewer as before - so `"=agentbox"` means the card and nothing else.
- Action buttons (FR32): `agentbox notify ... --action "Label::command"`
  (repeat up to 3) puts buttons on a notify card; `notify_user` takes the
  same via its `actions` argument. Clicking (or the matching number key)
  runs the command via `sh -c` in the caller's cwd, off the UI goroutine;
  the verbatim command shows on hover (anti-spoof), output goes to the log,
  a non-zero exit raises an error card. Global kill switch `[actions]
  enabled = false` (live-reloadable); when off the buttons do not render and
  RunAction is a no-op. A notify card with buttons drops the whole-card
  click-to-dismiss so it cannot swallow a button click (Esc still dismisses).
- Markdown engine (M6, ADR-0008 + its 2026-07-25 amendment):
  `internal/webui/mdhtml.go` renders with goldmark (GFM + footnotes + GitHub
  alerts) straight to HTML, with chroma writing CSS classes rather than
  colours so code follows a theme change. Highlighting carries eleven token
  roles (2026-07-28): keyword, type, constant/builtin, function, string,
  escape/interpolation, number, tag/attribute/decorator, operator,
  punctuation, comment, plus generic tokens riding the status colours (a
  diff fence gets tinted add/remove lines). `[markdown] code_theme` offers
  auto | nord | gruvbox | github | onedark | dracula (owner runs onedark,
  picked 2026-07-28 via the tools/mockups/code-theme-picker.html artifact);
  palettes fall back role-by-role so a five-colour palette still renders
  the full set. Alerts become tinted panels,
  `chart` fences become themed SVG drawn in Go (bar/line/area/scatter/pie/
  doughnut, `chart.go`), mermaid fences are drawn by a bundled engine loaded
  on first use, code blocks get line numbers and a copy button, and links
  open in the desktop browser. One stylesheet (`.k-md` in
  `frontend/src/app.css`) serves the card, the session and the viewer.
- Math and images (M10 slice 4): TeX in `$...$`, `\(...\)`, `$$...$$`,
  `\[...\]` and a ```math fence, parsed in Go (`internal/webui/math.go`,
  with the money rule that keeps `$5 and $10` money) and typeset by KaTeX
  with `trust: false`; a refused formula shows its own TeX. Images are read
  from an absolute local path (or a path relative to a document AgentBox opened
  from disk), sniffed by magic number (PNG/JPEG/GIF/WebP), inlined as
  `data:` URIs under a 2 MB ceiling; a remote host renders as a placeholder
  saying why. `img-src 'self' data: blob:` enforces it (ADR-0010).
- Interactive artifacts (M10): agent HTML runs in a sandboxed iframe with
  React 19 and Tailwind v4 injected from AgentBox's own bundle, no network of any
  kind; `window.agentbox.emit` is the only way out (`await_artifact_event`,
  `read_artifact_events`, `agentbox artifact wait|read`). Code/preview toggle on
  every artifact; `[artifact] enabled` is a live, retroactive trust switch.
  ADR-0010 has the bargain and the probe:
  `agentbox show --artifact tools/artifact-probe.html` re-checks it in seconds.
  An artifact page (`agentbox show --artifact`) opens maximized, A+/A- scales the
  sandbox document's base FONT SIZE - the rem root, sourced from
  `[font] size_pt` via --k-size, owner's call 2026-07-28 replacing the
  earlier html zoom rule: A+ means bigger text, not a magnifier, and
  px-sized chrome holds still while rem-sized text scales (an artifact that
  sizes text in px has opted out). The level shows beside the buttons and
  clicks back to 100%. The block chrome - preview/code tabs, reload -
  renders only when an artifact sits inside a conversation (all 2026-07-28,
  from the FR58 mock rounds).
- Speech (M11): `internal/speech` holds one synthesiser open (engine
  contract: text line on stdin, raw s16le PCM on stdout; piper
  auto-detected, Kokoro via `[speech] command`). A `speak` field on the CLI
  and every item-creating MCP tool; the `speak` tool and `agentbox say "..."`
  (with `--wait`, which returns when the audio has finished) for a line with
  no card behind it. Idle hold, mute, DND and quiet hours all apply.
- Diff review (FR33, restructured by FR62): `agentbox review --title T
  --diff-file P` (or stdin), the `request_review` MCP tool, and the `diff`
  item kind. The card parses the patch into per-file sections with sticky
  headers, add/del stats and new/deleted/renamed badges; past one file a
  left rail lists the files as steps (click to jump, n/p keys, scroll-spy
  marks the current file, seen files go quiet). Cards open at 560px, 780px
  with the rail. A growing textarea takes an optional note (Enter is a
  newline, Esc returns the keyboard to the answer keys), and Approve /
  Request changes answer by a / r. Result is `{approved, reply:comment}`;
  CLI exits 0 approved, 1 changes, 3 unanswered.
- Viewer (FR36-38): `agentbox show FILE|-` (and the `show_document` MCP tool)
  opens a reading window on a 760px measure; `--watch` live-reloads on file
  change (mtime poll); `/` or Ctrl+F is find-in-page (matching blocks
  tinted + counted, Esc closes). On an artifact page the query is forwarded
  into the sandboxed frame, which searches its own text, paints hits via the
  CSS Highlight API and answers with the count - a running program is
  searchable like a document. `docs/sample.md` exercises every block.
- Secret (FR23): `agentbox secret --title T --to-file PATH [--stdout]` and the
  request_secret tool. Masked entry; the daemon writes the value to a 0600
  file and only returns it over the socket on explicit --stdout. The value
  is never logged or stored - history records that a secret was provided.
- Stats (FR35): `agentbox stats [--since 7d|24h|30d|0]` over history -
  interruptions/questions/answered, median time-to-answer, per-agent and
  per-day breakdown; `--json` for the raw object. Served by the daemon
  (agentbox.v1.stats) from store.Stats.
- Undo grace (FR28): answers hold 3 s (config `ask.undo_grace_s`, clamped
  0-5) behind an "Answered: X · sending in Ns · undo" strip; engine
  duration is wall-clock tested; lifecycle logged
  (item.grace_started -> item.answered | item.undone).
- Cards: dead-center on the monitor the pointer is on (RandR 1.5),
  undecorated, above-all (side X11 connection sets MOTIF hints, window
  type, position). Toasts top-center with drawn severity icons and a
  "closes in Ns" countdown. Identity pills in the agent's stable hue;
  footer shows waiting-agent dots (multi-agent legibility). Dark/light via
  desktop setting; font size via config.
- Queue: FIFO, urgent preempts, Esc defers, inbox click promotes;
  escalation (FR9) replays earcons at config cadence up to the cap;
  DND (FR11) via CLI/tray/config, urgent breaks through; pending items
  survive restarts (NFR7).
- Config: `~/.config/agentbox/config.toml`, defaults baked in, unknown keys
  warn-and-skip, live reload (the watcher polls at 400 ms).
- Store: SQLite, embedded migrations (at 0004: actions + cwd columns so
  notify cards keep their buttons across a restart; missed_while_away for
  FR44), exactly-once answers, level-based retention (below
  `history.keep_level` evicted after `retention_days`).
- Tray: the brand robot's head (FR80, session 34), rendered at 128px from
  `docs/img/logo.png` so a HiDPI panel has something to scale down from, masked
  to an ellipse to cut the speech bubble behind it, with a badge dot carrying
  state (none / blue / amber). Tooltip count,
  menu: Show/Hide AgentBox (a toggle that opens or hides the app window, its
  label following the window state from any source - tray, `agentbox app`, or the
  window's close button - via u.OnAppChange -> tray.SetAppOpen), DND
  checkbox, Quit AgentBox. AgentBox is tray-resident: the daemon and the `.desktop`
  launcher (Exec=agentbox daemon, with an "Open AgentBox window" action) start it
  minimized to the tray with no window forced open; the window is summoned
  from the tray. The window's CLOSE button (X) HIDES to the tray - it keeps
  the window state and any live Claude sessions (run's defer drops only the
  OS window, ShowApp reuses the kept views); only tray Quit / `agentbox quit` /
  SIGTERM actually end the sessions, via UI.ShutdownApp() wired into the
  daemon shutdown. Maintenance shutdown is graceful: those paths all drain
  through d.OnQuit, and `make deploy`/`make stop` quit gracefully (pkill
  only as a fallback for a wedged daemon).
- Fonts: three CSS stacks, each configurable (`[font] family`/`reading`/
  `mono`) - Cantarell then the system UI font for chrome, Bitstream Charter
  then Georgia for agent prose, JetBrains Mono then Fira Code for code.
  Nothing is embedded; the webview falls back through the system's fonts.
  Colour emoji render (verified 2026-07-25).
- Agent manual: docs/agent-manual.md is the exhaustive reference for an AI
  agent driving AgentBox (every MCP tool + CLI command, args, exit codes,
  identity, limits, patterns, anti-patterns). Linked from docs/README.md.
- App: `agentbox app` opens one frameless window (`internal/webui/app.go`) in the
  daemon process like every other window, with a left surface rail - **Home**,
  Session, Inbox, History, Viewer, Library, and Settings at the foot - and a
  permanent status strip. The rail badges the inbox pending count and
  working sessions separately. Progress is not a rail surface: it has its
  own window (FR21). `agentbox inbox` and the tray open the app on the inbox
  surface; `agentbox app --tab session|inbox|history|viewer|settings` picks the
  first one. The card/toast/veto model and the daemon are unchanged - this
  is the housing around them.
- Home surface (FR81, session 34): what the app opens on. Four tiles (waiting,
  agents working, interruptions today, reviews open) that are buttons rather
  than trivia, then what is waiting, what is running, the week's shape from the
  same stats query History renders, open reviews, and a row of doors into every
  other surface. Panels are individually switchable; the choice is per-machine
  view state in localStorage, not a config knob. `ShowApp("")` lands here - a
  caller that wants a named surface still names it (`agentbox inbox`, the tray).
- Assignments (M12/FR82, end to end as of session 35): work AgentBox hands a Claude
  agent on a schedule or on demand. `internal/assign` holds the model,
  `{{placeholder}}` substitution and the schedule grammar (`every 30m` |
  `daily 09:00` | `weekly mon 09:00`, empty = ad-hoc); migration 0007 holds
  `assignments` + `assignment_runs`; `internal/daemon/assignments.go` is the
  scheduler and `assignmentsrpc.go` the six wire methods; `internal/mcp/
  assignments.go` is the seven tools an agent authors with; `internal/webui/
  assignments.go` is the Runner and `assignpanel.go` the surface's bridge.
  A missed slot is counted and recorded, never caught up.
  - A run is an ordinary session, spawned with the assignment's model, dir and
    mode and `manual.Assignment()` as its brief. It does NOT take the human's
    selection, and its child is stopped when it finishes while the transcript
    stays - thirty daily runs must not be thirty idle `claude` processes.
  - The run's last assistant message is the summary; a fenced ```agentbox-data block
    inside it is lifted into the run's `data` column, which is what "collect
    statistics for later analysis" means in practice.
  - Refusals are shared: the panel writes through `Daemon.AssignmentSave`, the
    same entry point the MCP tools use, so an editor cannot have its own idea of
    a valid schedule. Every field is optional on an update - what you do not
    send, you do not change.
  - The custom HTML panel (session 37) runs in the artifact sandbox, marked
    `data-panel` so its emits route to `SetAssignmentParams` rather than to a
    waiting agent. Two-way: `emit("params", {...})` merges out (keys not sent
    survive, and turning a typed knob no longer erases panel-only keys);
    `window.agentbox.params` + the `agentbox:params` event carry changes in. The
    daemon's `AssignmentsChanged` poke (every save, param write, enable, delete,
    run start/finish/skip, any caller) replaced the surface's 3s poll. A no-spec
    assignment keeps its saved values at run time (launch uses `mergeParams`,
    not `assign.Merge`, which erased them). The typed knobs stay the way in
    that always works.
  See docs/08-assignments.md.
- Session surface (FR49): `internal/webui/sessions.go` runs Claude Code
  inside AgentBox. It drives a headless `claude` child over stream-json
  (internal/session: `claude -p --input-format stream-json --output-format
  stream-json --verbose --include-partial-messages`), parses the NDJSON
  event stream on a reader goroutine, and renders each turn through the M6
  engine as a scrollable conversation - prose, syntax-highlighted code with
  copy, tables, and compact tool-call chips with their results - with live
  token streaming (deltas accumulate off-frame, re-parsed on a 75ms
  throttle, reconciled by the authoritative `assistant` event). A multiline
  prompt sends on Send / Ctrl+Enter. Typography is live (A-/A+ scale
  0.7-1.8x, a Mono toggle), no restart. Several sessions run at once (a
  switcher with a working dot, "+ New", a Plan/Full permission selector
  fixed at spawn; default plan = read-only; Full maps to bypassPermissions;
  a mode switch starts a new child with `--resume`, keeping AgentBox's session
  id). Sessions are named by Claude's own first words, renameable, saved to
  disk as JSON + a markdown sibling, reopenable, findable. Verified live
  (2026-07-25, eighteenth session): a real `claude` child answered a typed
  prompt in the panel, streaming. In-flow interaction: the child is spawned
  with AgentBox's own MCP server (--mcp-config -> this binary's `mcp`) and a
  AGENTBOX_SESSION_ID that `agentbox mcp` stamps onto Identity.Session; UI.Present
  routes a session-tagged ask/confirm/notify to the surface's inline panel
  (answered through the same Resolver as cards) and suppresses the popup,
  while text/secret/form/diff/veto still pop a card.
- The drop-down panel (M10): `Ctrl+Alt+grave` (daemon-grabbed via XGrabKey,
  rebindable live) rolls a session console down from the top edge of the
  monitor the pointer is on; `agentbox panel [--show|--hide|--state]` is the
  fallback. It shows the same sessions the app window shows and it takes
  the keyboard - the `agentbox summon` exception. Esc rolls it up.
- Inline ask panel (FR49): a question from an agent running in the session
  surface is answered in its conversation, above the composer.
  `inlineRoutable` is the whole rule: session-tagged, a kind the panel can
  take (choice/confirm/notify), not urgent, and a host on screen. The panel
  does not take focus; a switcher row marks itself when its conversation is
  the one waiting; losing the host with a question pending re-presents it as
  a card (`rerouteAsk`).
  **Since 2026-08-06 "a host" means the app window OR the drop-down panel.**
  It was the app window alone, so a question asked while the panel was down
  opened a card over the very conversation it was about. Three things to know
  before touching this: the panel's reroute must run after `slide()`, which is
  what clears the open flag (asked earlier, routing still sees a host and the
  question stays in a panel that is not there); with both hosts open the
  question renders in both, and answering either resolves it; and opening a
  host does NOT pull an existing card back inline - the app has never done
  that, and the two are deliberately symmetric.
- Settings surface: a descriptor table (section → group → knob) drives the
  whole surface - control type, bounds, valid values and restart-need
  declared once, in Go. Reads the file fresh as the baseline (the file is
  what Save edits and the user may have hand-edited it); Save writes ONLY
  changed keys via the surgical, comment-preserving writer
  (internal/config/write.go: edit the key's line in place, else append;
  defaults never materialized; atomic temp+rename so the Watch poller never
  reads a torn file); refusals (bad enum, malformed hex, unparseable quiet
  hours) do not block the other keys; clamps share the loader's exported
  bounds. The live/restart split matches `SetPolicy`: only
  `dnd.start_in_dnd`, `history.*` and `log.*` carry an "applies on restart"
  tag - theme and font apply live. `ask.allow_reply` is omitted (the global
  flag is unwired). Since session 54 there is a `command` kind for argv
  arrays, which is what `[editor] command` and `[speech] command` needed and
  is why neither had a control before: they are edited as the line you would
  type, quotes holding a path together, and stored as arrays. Neither carries
  a restart tag - `applySpeech` hands the new argv to the Speaker on every
  config change, and the board reads the editor config when the click happens.
  It is not a shell (no globbing, variables or pipes): what is typed is exec'd
  directly. The surface edits the real `config.toml`, so point
  `XDG_CONFIG_HOME` at a scratch directory when poking at it - or type into a
  knob and press Revert, which is how session 54 exercised this one without
  touching Boris's file.
- Inbox (FR10): the inbox surface shows pending (click to summon) + history
  with identity dots and outcome chips. Live search box filters by title,
  body, agent, project, kind and state; footer shows today's interruption
  count. Keyboard triage (FR34): j/k move a cursor, the same shortcuts as
  cards answer the selected item (text/secret/form promote to their card; a
  notice dismisses with D/Backspace), c copies, `/` hands focus to search,
  Esc returns; dispatch is a pure function (triageFor) with table tests.
  UI.Present refreshes on every queue change, so it stays live whether or
  not it is the shown tab.
- History/Stats surface (FR35): the same stats query `agentbox stats` serves,
  rendered through the M6 engine - a summary table, a per-agent table, and
  a per-day bar chart - with a 24h/7d/30d/all segmented control, rebuilt on
  a throttle, never per frame.
- Runtime mute (FR47): `agentbox mute <agent>` / `agentbox unmute <agent>` /
  `agentbox mute --list` silence a flooding agent instantly - in memory, cleared
  on restart, the fast reaction the config `mute` (FR17, not yet wired) is
  too slow for. Muted items queue straight to the inbox (no card, no sound)
  via the same display gate as DND, but per-agent and including urgent;
  unmuting reveals whatever queued, with its earcon. Inbox rows and the
  tray tooltip badge muted agents. `agentbox.v1.mute` RPC.
- Missed-while-away (FR44): an info/success toast that auto-closes while
  the user is idle is recorded `missed_while_away` (migration 0004) and
  shown with a chip in the inbox, so the return-from-idle review separates
  toasts that flashed unseen from ones read. Idle comes from an X11
  MIT-SCREEN-SAVER monitor (`internal/presence/idle.go`) behind the daemon
  `Presence` interface; no X11 (Wayland/headless) reads as "present" (safe
  direction). Reuses `presence.idle_after_s`; 0 disables the marker.
- Caller-alive (FR45): a blocking card shows a caller dot by the identity
  pill - green live, red "caller disconnected", amber "awaiting reconnect"
  (restored after a restart, NFR7). State is derived from the waiters map
  plus a `gone` set. When the caller's socket drops the card
  auto-dismisses after a short grace (`cfg.CallerGone`, 4 s) or on any key;
  a daemon-teardown disconnect is told apart by a `closing` flag and stays
  a quiet cancel.
- Presence gate (FR29): all of `[presence]` is wired. hold_when_idle holds
  the chime on a card that arrives while the desktop is idle (the card
  still shows - idle gates sound, not visibility) and pauses escalation
  without spending its budget; on return, one summary chime sounds and the
  oldest pending card is current ("N waiting" in the footer).
  fullscreen_auto_dnd treats a focused fullscreen app as DND,
  respect_desktop_dnd treats GNOME's show-banners=false as DND; both share
  the manual-DND break-through rule and held items reveal when the
  suppression lifts. A 5 s poller drives the reveal/summary-chime edge
  while items are pending, and handleSubmit refreshes the signals on each
  arrival.
- Progress (FR21): `long_task | agentbox progress --title T` drives a live
  progress bar from stdin (lines are `NN`, `NN status`, or a bare status
  line; `--indeterminate` for a spinner), the `report_progress` MCP tool
  does the same for agents (start without id, reuse the returned id, `done`
  to finish), and completion emits a success/error toast. Reports render in
  a dedicated window kept OUT of the card queue, so they are non-blocking.
  The window is top-most like every surface (`x11.above` after the map;
  Boris's rule, 2026-07-26) while staying focus-free and in the taskbar. A
  CLI report is `hold`-tied to its connection and reaped with an
  "interrupted" toast if the pipe dies; an orphaned report is reaped by the
  poll tick after 15 min. There is no Progress tab: a report is always its own
  corner window, deliberately, because a bar that lives inside the app window is
  invisible whenever the app window is not the thing you are looking at.
- Focus policy (vision principle 3 / NFR5): cards map with
  `_NET_WM_USER_TIME=0` + `_NET_WM_STATE_ABOVE`, so on Mutter they pop
  above without taking the keyboard. `agentbox summon` (FR15) re-focuses the
  current card via a `_NET_ACTIVE_WINDOW` client message (pager source);
  bind it to a desktop shortcut. `internal/webui/x11.go` is the mechanism.
  Verified live: a card and a toast map without changing
  `_NET_ACTIVE_WINDOW`, and `agentbox summon` then focuses the card.
- Observability: JSONL event log (`agentbox logs --follow`), startup banner,
  `agentbox version` (commit, dirty flag, build time). Events cover the full item
  lifecycle plus agent.muted/unmuted, item.held_muted, missed_while_away,
  caller_gone, held_idle, presence.returned, progress.*, artifact.*.
- Error-logging hardening (owner: "100% robust, every error logged"): panic
  recovery wraps the daemon Handle (logs + returns CodeInternal), the proto
  dispatch goroutine, the server connection goroutine, the presence-poll
  goroutine, the tray goroutine and every UI window run loop; the server
  logs per-connection Serve errors; the inbox reload logs store failures;
  newID checks crypto/rand. The remaining best-effort ignores (X11 property
  writes, gsettings reads, fire-and-forget UI invalidates) are deliberate
  and commented.
- Deploying: `make deploy` stops the daemon, replaces the binary, restarts,
  and FAILS unless the daemon reports it is serving the build that was just
  installed (`agentbox status` prints the daemon's own build and says plainly
  when client and daemon differ). The replaced build is kept as `agentbox.prev`;
  `make rollback` is one command; `make deployed` is the check on its own.
  `make help` lists the lifecycle: build / test / check / run / stop /
  logs / deploy / deployed / rollback.
- Packaging (M7): `make install` puts the binary, an `agentbox.desktop` launcher,
  a 256px app icon, and a systemd `--user` service under `$HOME` (no
  root); `systemctl --user enable --now agentbox.service` autostarts the daemon
  and restarts it on failure. `make uninstall` reverses it. Files in
  `packaging/` (README there); icon generated by `tools/genicon`. The
  `.desktop` passes desktop-file-validate; the unit is bound to
  graphical-session.target and has no ExecStop (SIGTERM drains through the
  graceful path; the unit explains why). Wayland validation is deferred
  (owner not on Wayland); X11 is the supported session.

- **Review board (FR58/59/61/63/66/67, ADR-0012).** A walkthrough is a durable
  step-by-step review: the agent's spec plus the human's marks, in SQLite,
  outliving both sessions. `create_walkthrough` / `await_walkthrough` /
  `read_walkthrough --ack` / `list` / `delete`, and
  `agentbox walkthrough create --spec FILE | open ID | await [ID] | read ID [--ack] | list | delete ID`.
  The board renders per-line chroma from the daemon, derives every added/removed
  marking from the spec's diff manifest, and writes each verdict, note and
  line-anchored comment straight to the store. Submission is unclear-first, gated
  against a hollow unclear mark, and delivered exactly once whether or not an
  agent is waiting. **A walkthrough keeps the code it is about** (FR78, session
  33): creation captures every cited range from the working tree into
  `walkthrough_excerpts` (migration 0006), and the board prefers that capture,
  falling back to reading the file so everything stored earlier still renders.
  Without it a review was true only until the next checkout - and an edit that
  left the file long enough rendered different code under the same prose,
  silently. `agentbox walkthrough repair [ID]` recovers what the older ones never kept
  from `git cat-file blob <pinned>:<path>`, the pinned SHA's first actual use.
  What the board itself offers the reader is the teaching-surface section below.

The demo harness shows any surface without a daemon and without touching
the queue, store or inbox:

```
agentbox webui-demo                        # cards, one per kind
agentbox webui-demo notify                 # the toasts (success, sticky warning, bare info)
agentbox webui-demo viewer [FILE]          # the reader, watching docs/sample.md by default
agentbox webui-demo progress               # the FR21 window, three fake tasks
agentbox webui-demo ask                    # the inline panel: choice, confirm, notice, then a card fallback
agentbox webui-demo app [session|inbox|history|viewer|settings]
agentbox webui-demo artifact [FILE]        # a React artifact in the sandbox (M10); FILE runs your own
agentbox webui-demo panel                  # the drop-down console with a canned conversation
```

### The review board, as a teaching surface (FR58, FR61, FR63, FR66-71)

- **Steps** carry prose, code citations rendered with per-line chroma, numbered
  margin notes beside the lines they explain, comprehension checks with a
  reversible reveal, and commands with an expected result and a date. Prose is
  paragraphed (`"p": true` on a segment), a bound phrase lights its code region,
  and the note margin sits **outside** the code frame, on the step's grid. Each
  note carries the margin's rule itself, so the line runs the length of the
  annotations and no further, and the note under the pointer lights its own
  segment together with its badge in the gutter.
- **`lead` and `close`** (FR69) put text between two blocks and under the last
  one, so a step reads lead-in, code, takeaway rather than a wall of prose
  followed by a wall of code.
- **A glossary** (FR68) marks the first occurrence of each defined term per step
  with a quiet underline. Hover gives the one-line short after 220ms; click, or
  `g`, opens the drawer at the full entry. Bound phrases and code chips are
  never marked. AgentBox warns about a term no prose can reach.
- **Read-aloud per region** (FR66, reshaped by FR72; served over `MethodAloud`):
  the opening, each block's `lead`, the `close` and the checks each carry their
  own play control, and each is read as one utterance so the engine decides
  prosody over the whole of it. Play and stop only - pause and rewind needed the
  text split into passages, and the split was what lost the last words of a
  passage (the transport's positions were the FR66 truncation; the fix is
  confirmed by ear, 2026-07-31, not only by arithmetic). `a` reads the opening,
  Esc stops, moving step stops.
- **The hands-off strip** (FR74, FR75): while an agent has the desktop, one
  always-on-top strip sits at the top centre - asking (reason, countdown, Deny,
  Now) then driving (the activity line and how long since it changed). Its
  presence is the whole signal: on screen means hands off, gone means the desktop
  is the human's, which is why there is no idle state. It outranks every other
  window including AgentBox's own (notification type plus a keeper that restacks it).
  `agentbox control request|activity|release|state` from any shell, and
  `request_control` / `set_activity` / `release_control` over MCP (session 32).
  Both manuals now put an agent here before it reaches for a card of its own: a
  card is answered and gone while the driving goes on, which is the failure this
  replaces. The top-centre column (`topstack.go`) lays the strip and the toasts out
  so they cannot land on top of each other.
- **The header is also the window's title bar** (FR76, session 33): the board is
  frameless, so minimise, maximise/restore and close live in its header, and the
  whole strip is a drag handle (it could not be moved at all before). The
  maximise glyph reads its state back from the window rather than remembering
  it. Below about 1150px the title ellipses and the repo path and pinned sha
  drop out, so every control stays reachable down to the 700px minimum - before
  that the header simply overflowed and took the close button off the edge.
  Submit is an icon plus one verb; the title already says which review.
- **A library** (FR70): the app window's Library tab lists every stored review
  with its progress, puts one on the board, or deletes it behind a confirm.
  `l` from the board, `agentbox app --tab library`, or `agentbox walkthrough open` with no
  id for the most recent.
- **The authoring standard** an agent reads before writing one is
  `internal/manual/walkthrough.md`, embedded and served three ways (MCP resource
  `agentbox://standards/walkthrough`, the `walkthrough_standard` prompt, and
  `agentbox docs walkthrough`). 56 rules, contiguously numbered since session 49.
  It was tested against a real change that session - authored blind, then audited
  rule by rule - and five defects came out of it: a hole where rules 13-16 used to
  be, a rule demanding fields it never named, `tldr` and `domains` missing from
  every field reference an agent reaches, a domain count that said two things, and
  an instruction ("say what you did not verify next to the gate") the validator
  refuses. All five are fixed; the review that found them is `w5bd381d0590a`, and
  session 49 of [history.md](history.md) has the list. **The last finding closed
  on 2026-08-06** (session 52): the coverage rule has a validator. Every hunk of
  the diff is compared against every cited range at create and again on every
  read, the create result carries the arithmetic, and the uncovered hunks are
  named in a warning rather than counted in silence. (The other finding, that
  the standard never said what changes when the reader is the code's own author,
  was closed the same session by a paragraph under "The job".) Rule numbers are
  deliberately not quoted anywhere outside the standard itself: they have
  renumbered twice and every reference to one went stale silently.

## Tests

`make check` = gofmt + vet + `go test ./... -race` + `make test-js`; **21 Go
packages** (incl. `internal/hand`, `internal/session`, `internal/webui` and
`internal/speech`), **685 tests**, all green as of FR83's last slice (session 45;
the count is `grep -rh "^func Test"` top-level Go tests, 2026-08-04). The
frontend gained a runner on 2026-08-06 (session 52): node's own `--test`, no
framework, over `frontend/src/**/*.test.js` - `parseDiff` is the first module in
it, and the target says so and moves on when node is absent, exactly like the
build does with a committed dist. There is still no automated *visual* check
(see "Known gaps"); what a surface is *allowed to do* is tested, and so is the
HTML Go hands it.

`FuzzParse` (internal/change) is the first fuzz target, and it earned its keep
on the first run: a hunk header with twenty digits overflowed the hand-rolled
atoi and came back as a NEGATIVE line number, which then flowed into every span
and slice built from the geometry. Clamped, and the failing input is committed
as the corpus so `go test` replays it. The frontend parser was told the same lie and read
it differently - enormous rather than negative, so it swallowed every following
file into one body - and **both parsers now end a hunk when a body line is not a
body line at all** (' ', '+', '-', '\', or empty). Watched on screen in a review
card: a header claiming nine thousand lines no longer eats the file after it,
which still renders with its own count.

- **Policy** (`internal/webui/policy_test.go`): one invariant over every
  surface at once - nothing AgentBox hands a webview fetches from a host on its
  own - by running one hostile document (remote images; raw HTML using
  `src`, `srcset`, `poster`, `background`, `<object data>`, `xlink:href`, a
  stylesheet `<link>`, an `<iframe>`; CSS `@import` and `url()`; the same
  inside an `html` fence, an `artifact` fence, a table cell and an alert)
  through every producer of surface HTML: `encode`, `encodeTurns`,
  `RenderMarkdown`, `RenderMarkdownIn`, `RenderArtifact`, `ParseInline` and
  the viewer's load path. A link is exempt on purpose - it needs a click
  and AgentBox opens it in the desktop browser - and that exemption has its own
  test; a third asserts the fixture is genuinely hostile so the sweep
  cannot pass by rendering nothing. `frontend/policy_test.go` checks the
  other side against `dist` - the bytes `go:embed` puts in the binary, not
  the working tree: the image policy ships and names no host, the shipped
  policy matches its source (a stale bundle says "run `npm run build`"),
  the shipped document loads only bundled assets, and the artifact CSP
  survives into the bundle with every directive that closes a way out while
  `allow-scripts` is set and `allow-same-origin` appears nowhere. Then the
  trust model itself: **every `{@html}` in the tree is an allowlist entry
  naming the Go field behind it**, and `innerHTML` is confined to the three
  reviewed files that rework markup Go already produced. Verified by
  mutation: dropping the image renderer, enabling goldmark's raw HTML,
  loosening the source `img-src`, deleting the shipped policy meta tag,
  adding `{@html view.item.title}` to a card, and opening the sandbox with
  `allow-same-origin` each fail these tests.
- **Images**: every verdict has a test; `TestImageNeverEmitsARemoteSrc`
  states the invariant over every kind of destination both with and without
  a base directory, and the base's own rules (what it resolves, what it
  must not rescue, that it changes the reason and not the verdict, that it
  must be absolute) are pinned in `images_test.go` with the end-to-end
  through the viewer in `viewer_test.go`.
- **Encoders** (`encode_test.go`): the body arriving as HTML, a zero time
  becoming 0 rather than a 1970 countdown, one identity hue per queued item
  (same identity same colour, theme changing it), the FR32 kill switch
  travelling, an empty view encoding rather than panicking, and every FR45
  caller state plus an unknown one reading as "none". `cardHeight`: growth
  per kind, a fourth choice wrapping to a second row of buttons, and both
  ways to overflow (a 400-line paste stopping at the 12-line cap, a
  60-field form clamped to the configured maximum). `encodeTurns`: the four
  segment kinds, a user turn carrying its source beside the HTML while an
  assistant turn does not, model and cost, a system error turn, an empty
  turn encoding as a list and not null, and a 12 kB tool result cut before
  it reaches a window. Verified by mutation rather than by passing. These
  matter because nothing renders a surface offscreen - a field that quietly
  stopped being filled would show as an empty region on a real screen and
  nowhere else.
- **Inline ask routing**: the routing rule per kind (including the urgent
  carve-out and the conditions the Gio build never had: an untagged item, a
  session the surface is not showing, a closed app window), the question
  reaching only the session that asked it, the panel clearing when the item
  resolves, the controls a choice gets (digit keys, the default reading as
  primary, descriptions kept), a confirm's yes/no vocabulary staying in Go,
  a notice getting a dismiss verb plus its FR32 buttons only when actions
  are enabled, and the keystroke path (digit, Enter default, d dismiss,
  y/n, a digit past the options, an unmapped key, a key for another item,
  and a key arriving after the item is gone).
- **Toasts, viewer, progress, markdown**: which items get a toast and which
  a card (including the urgent carve-out), the severity glyph per level,
  sticky following the daemon's deadline, the toast's opening height and
  cap, the progress encoding (identity hue, the "Working" fallback, percent
  clamping, an empty set encoding as a list and not null) and its window
  sizing, the viewer's document load (file, inline, unreadable file,
  content fallback, empty state), the window title rule, the watch loop
  retiring with its window, and the markdown renderer: every alert kind and
  tone, a plain quote staying a quote, an unknown marker staying visible,
  the code block's badge/copy/line-number behaviour, the mermaid fence
  keeping escaped source, tables/task lists/footnotes, agent HTML being
  escaped, and the chart renderer (geometry per type, token-only colours,
  nice axis maxima, label thinning, the single-slice circle, and every
  undrawable spec falling back to source).
- **Target lock** (`internal/hand/target_test.go`, session 33): the rules that
  decide whether an event is about to land where the script aimed it, as a pure
  function over the window chain - the target itself, one of its children,
  another window entirely (refused, and named), a root-parented menu (allowed),
  the target's own modal (allowed), another window's modal (refused), and the
  label formatting a failure message depends on.
- **Captured source** (`internal/webui/boardpinned_test.go`,
  `internal/daemon/wtcapture_test.go`, session 33): the capture beats the file
  on disk, no capture still reads the file (the path every older walkthrough
  depends on), a capture survives the file being deleted, and neither present
  still errors honestly. On the daemon side: create keeps the cited lines and
  they do not follow a later edit, create warns rather than refusing when it
  cannot read, repair recovers from the pinned commit, repair leaves an existing
  capture alone, and repair says why when git cannot help. The git ones build a
  real repository, because the repair path shells out and a fake would prove
  nothing about the arguments.
- **Settings**: every knob readable and writable (a half-added knob fails
  the suite, not the user), the restart set matching what the daemon
  actually reads at startup, clamping, every refusal with its reason,
  reading values from the file rather than the defaults, loader warnings
  reaching the surface, a save into a hand-written temp config that
  preserves comments and untouched keys and materialises no defaults, a
  second save writing nothing, one refused key not blocking a good one,
  unknown knob ids ignored, the preview resolving pending values without
  touching the file, and the contrast lift and code-theme palettes.
- **Inbox + history**: the triage keymap per item kind (including the
  veto/diff carve-out from FR50's dismiss), the outcome chip for every
  state (a veto's "expired" reads as *proceeded*), pending-first ordering,
  today's interruption count, body flattening, the snapshot against a fake
  Source (hint only on pending rows, muted badge, identity hue, degrading
  to an empty surface when the store errors), dispatch (answer / veto /
  dismiss / promote, and the refusals: a resolved item, a key the kind
  ignores, an unknown id), the clipboard form, and the stats encoding
  (answered share of *questions*, peak day, per-day rate, empty window,
  duration formatting).
- **Session driving** (`internal/session`): the stream-json parser/turn
  assembly from NDJSON fixtures (no real claude), the partial-message
  streaming sequence + no-duplicate reconciliation, a stub-binary
  spawn/read, the driver args (mcp-config/allowed-tools/partial), and
  persistence (JSON round-trip, markdown export, save/load-latest).
- **Config writer** (`internal/config`): replace an existing key, append a
  key under a section, append a missing section, preserve comments +
  untouched keys, key disambiguation across sections, empty/missing file,
  two keys into one new section, literal formatting incl. the
  float-keeps-a-decimal rule, and a Write->Load round-trip with comments
  intact.
- **Daemon FR suites**: FR21 (create/update/clamp, done success+error
  toast, unknown-id rejected, held report reaped+warned on caller-gone,
  teardown stays quiet, stale reap) plus a CLI line-parser test. FR47 (mute
  gates display+sound, unmute reveals, muted blocking item answerable from
  inbox, list/unmute roundtrip). FR44 (idle->flagged, present->not,
  idle_after_s=0 disables, store round-trip). FR45 (live->gone
  auto-dismiss, gone dismisses on key, restored->awaiting, teardown not
  shown as gone). FR29 (idle holds the initial chime then one summary chime
  on return, idle pauses then resumes escalation, fullscreen holds a card
  and reveals on exit, desktop DND holds but urgent pierces), driven by a
  fakePresence and a direct PresencePoll call. Proto has
  TestServeRecoversHandlerPanic; MethodInbox/MethodApp open the app on the
  requested tab against a fakeUI.

## Known gaps (deliberate)

- **No offscreen rendering.** The Gio headless harness went with the
  cutover, and WebKitGTK has no equivalent here. How a surface *looks* is
  checked by driving a real window: start the daemon on a desktop session
  and screenshot with `python3 tools/uidrive/uidrive.py shot /tmp/x.png`.
  What remains untested is behaviour: no Svelte component is rendered by
  any test, and `buildDocument` (the sandbox document assembler) is not
  executed by one, because that needs a JS test runner and a DOM. A
  toolchain decision waiting to be made, not an oversight - vitest plus
  jsdom would be the cheapest version ("Do this next").
- Artifacts: while an agent is still streaming the turn an artifact sits
  in, the conversation re-renders its HTML each frame and the artifact
  restarts with it (a finished turn is stable, and an artifact in its own
  window never has this). Only react and react-dom are bundled, so an
  artifact importing recharts or lucide is told so in its own bar rather
  than served. An `html` fence is classified by content, which is a
  heuristic: markup with a `<script>` runs, a table does not. The
  interaction channel is verified through the CLI and unit tests, not yet
  through an MCP host.
- Session surface, deliberate scope cuts: the stream-json permission
  control_request protocol is not handled (so only the non-stalling modes
  plan/Full are offered, not default/acceptEdits); whether `plan` mode lets
  the agent call AgentBox's MCP ask/confirm tools is unconfirmed; a session's
  child is killed on app-window close (the conversation is saved and
  reopenable, but there is no keep-alive); the child runs in the daemon's
  cwd (no dir picker). Markdown re-parse during live streaming briefly
  holds the conversation lock (parse is sub-ms; revisit only if a frame
  stalls).
- Card bodies render markdown (M6) but the fixed card height is still
  estimated from raw text length, so a body with a code block/table/chart
  can overflow a card - long content belongs in `agentbox show`. No live
  countdown ring on card timeouts (static footer text).
- FR33 refinement: diff lines are coloured by add/remove/hunk, without
  per-token language highlighting inside the line (chroma-in-diff is a
  later refinement).
- M6 refinements not done: per-token highlighting inside diffs (above).
- The knobs that were documented and read by nothing are gone from
  06-configuration.md as of 2026-08-07, and what they described is now in that
  file's "Behaviour that is fixed, and has no knob" table instead: card and toast
  placement, the five-minute defer, the never-take-focus rule, the global
  `ask.allow_reply` (per-item `--strict` is the real lever), the per-agent and
  per-class earcon overrides (FR46, dropped as redundant), `[viewer]
  watch_default` and `[markdown] chart_palette`. An unknown key is a startup
  warning and then nothing, so a documented key nothing reads is worse than no
  documentation at all.
- Inbox triage mode (FR34) is built; the keyboard focus handoff between the
  triage cursor and the search box is unverified on a live desktop.
- A second answer during an undo grace is dropped by design.
- A veto queued behind another card (or deferred and re-shown) ticks its
  visible countdown from display, while the authoritative proceed timer
  runs from submission; cosmetic drift only, and only in that edge case.
- Presence gate (FR29) is wired but platform-limited and partly
  live-unverified. The daemon logic is unit-tested with a fake;
  `item.held_idle` and `presence.returned` have fired live. The signals:
  fullscreen is X11-only (_NET_WM_STATE_FULLSCREEN on the active window),
  desktop DND reads GNOME gsettings (show-banners) so non-GNOME desktops
  read as off, and the idle read is the X11 screensaver from FR44. Each
  absent signal degrades to "present" (no false suppression). The poll
  cadence is a fixed 5 s and shells out to gsettings each tick while items
  are pending; not yet measured on a busy desktop. Still unseen live: ONE
  summary chime on return rather than a backlog, fullscreen/desktop-DND
  holding a card and revealing on lift, urgent piercing ("Do this next").
- FR45 caller-alive is verified at the daemon level (unit tests cover
  live->gone->auto-dismiss, awaiting-reconnect, and the teardown
  distinction) but the behaviour on a live desktop is unseen. The
  disconnect signal relies on the per-connection context cancelling when
  the client socket closes (proto.Conn.Serve); confirm with a killed caller
  on a real session.
- The settings surface reads the config once, on mount, so an external edit
  to `config.toml` while it is open leaves its knobs stale - the theme
  changes under it (live theming works) but Mode still shows the old value
  and the footer still says "Saved values match the file". Reopening the
  window refreshes it. A push would have to merge against unsaved edits,
  which is why it is a note rather than a patch.
- Rounded frameless corners need an ARGB visual. `WindowIsTranslucent` +
  `BackgroundTypeTransparent` did not produce one under GTK4 here, so cards
  are square-cornered. Boris may want to drop the CSS radius so the edge
  reads as deliberate.
- The viewer has no per-document history: `agentbox show` replaces whatever the
  window was showing, and the app's viewer surface shows that same
  document.
- **A capture is taken from the working tree, and nothing checks that the tree
  matched the pinned SHA** (FR78, session 33). The spec says `pinned` is the
  commit the citations are true against, but creation captures what is on disk,
  which is what the authoring agent actually read - right when the tree is
  clean, and quietly a different thing when it is not. Comparing the two at
  create time and warning on a mismatch is the obvious next step; it was left
  out to keep create free of a git subprocess.
- **`MaxSpecBytes` is documented but never enforced on its own** (session 53).
  `Parse` checks the whole payload against `MaxSpecBytes+MaxDiffBytes` and then
  the diff against `MaxDiffBytes`, so a spec carrying no diff may be 3 MB - three
  times the 1 MB the constant reads as. The worst case measured is a 2.8 MB spec
  with a full 48-term glossary of near-miss spellings: **940 ms in `Parse`**,
  nearly all of it the glossary scan, which is O(prose x spellings). Bounded and
  one-shot on create, so it is a documented bound rather than a patch: the
  obvious check (`len(raw) - len(s.Diff)`) counts a heavily-escaped diff against
  the spec's half and would refuse honest specs at the extreme.
- **Nothing in the UI says whether a block came from the capture or the live
  file.** `wireCode.Pinned` travels on the wire and the surface ignores it. It
  matters the moment a margin note stops matching its code, because that is the
  first question to ask. Boris was offered this and has not decided.
- **A resolved review's diff, and any spoken line, still cannot be read back.**
  FR73 gave every inbox row a detail view that reads its item back whole, and
  building it found the limit of "everything needed is already stored": the
  store's items table has no `speak` and no `diff` column, so those two go with
  the card whatever the reader does. The detail deliberately carries no field
  for either rather than promising what the read behind it cannot deliver. Both
  are a schema change and their own field request.
- The inline ask panel holds one question at a time (all the daemon
  presents), and a question already in the panel does not move when you
  switch sessions - it stays with the conversation that asked, and the
  other row keeps its `?`. So the panel can be off screen while an agent
  waits, which is what the mark is for.
- Mermaid bakes its own theme into the SVG it produces, so a config theme
  change redraws every diagram (`markdown.svelte.js` listens for
  `agentbox:themed`). It is the one thing a CSS variable write cannot re-theme on
  its own.

## Build and run

```
sudo apt-get install -y gcc pkg-config libgtk-4-dev libwebkitgtk-6.0-dev
# plus node/npm to rebuild frontend/dist (dist is committed, so this is only
# needed when a .svelte source changes)
cd ~/me/projects/agentbox && make run      # build (frontend first) + (re)start daemon
./agentbox webui-demo                       # the surfaces, no daemon, nothing queued
```

The `wails3` CLI is **not** needed - the build is `go build`, and
`make build` rebuilds the frontend first when its sources changed.

Dependency note (2026-07-25): `BurntSushi/toml v1.6.0` is current and stays.
`go-toml/v2` makes an unknown key a hard error where AgentBox wants the
warn-and-continue list it has; `koanf`/`viper` have no writer at all, which
would cost the comment-preserving surgical save the settings surface
depends on.

## ADR ledger

| ADR | Decision | Status |
|-----|----------|--------|
| 0001 | standalone repo | accepted |
| 0002 | UI toolkit: Gio | superseded by 0009; the code is deleted |
| 0003 | unix socket + JSON-RPC, auto-spawn | implemented as written |
| 0004 | MCP-first agent API, official go-sdk | implemented (go-sdk v1.6.1, M4) |
| 0005 | SQLite app DB + managed migrations | implemented as written |
| 0006 | sound via pw-play subprocess chain | implemented as written |
| 0008 | markdown: goldmark + chroma + native charts | implemented (M6), amended for HTML at M9 |
| 0009 | UI toolkit: Wails v3 webview | implemented (M9); supersedes 0002 |
| 0010 | artifact sandbox | implemented (M10) |
| 0012 | walkthrough store + native review board (FR58/59/61) | proposed; slice 1 shipped 2026-07-29, slice 2 (handback) + the board round (FR63/66/67) shipped 2026-07-30 |

## Do this next

The handoff for the current session is [../HANDOFF.md](../HANDOFF.md) - read
that first; it carries the exact commands and the live state.

**As of session 55 (2026-08-06) session 54's queue is empty and nothing is
blocked on Boris.** Session 55 cleared it: `config.SplitArgv` got the fifth fuzz
target (two escaping defects, both fixed); the inline ask panel was looked at on
a real long body (it does not need FR84's fold, but it was pushing the composer
off the window and now does not); the drop-down panel became an ask host, which
is what that look turned up; and the showcase question was answered - the video
pipeline is deleted and the live half kept (priority 2 below). What remains is
living with `[flood]`'s defaults, which is a matter of using AgentBox rather than
a task anybody can sit down and do.

**As of session 54 (2026-08-06) FR30 is built.**
He cleared the whole queue in two exchanges at the start of that session: flood
control's shape and threshold, FR84's other half, the argv settings control, the
showcase dropped for good, and permission to take the daemon down for the last
unseen surface. All five shipped in the same session and every one of them was
exercised on the real desktop; session 54 in [history.md](history.md) says what
that turned up, which was four defects the tests could not see.

**Flood control (FR30) is deployed.** `internal/daemon/flood.go`, kind `stack`,
migration 0013, knobs in `[flood]` (`burst = 3`, `window_s = 10`; burst 0 is
off). Past its budget a session's items collapse into one warning-level stack
card that lists the burst newest first and drops nothing: every collapsed item is
stored and pending exactly as it would have been, so a question caught in a flood
still has its own item, says "waiting on you" in the list, and opens as a real
card by click or number key. Dismissing the stack takes the notifications and
keeps the questions.

Four things about it are worth knowing before touching it:

- **The budget is keyed on the session, not the agent name.** Every Claude
  session on this machine calls itself `claude`, so a name-keyed bucket would let
  the first looping agent collapse its innocent neighbours.
- **An open stack card keeps collecting whatever the bucket says.** The window
  refills underneath it otherwise, and a sustained loop gets a fresh budget every
  window - three cards per ten seconds on the defaults, eighteen a minute. The
  collapse ends when the human ends it.
- **Three doors retire an item** (the card's Esc, the inbox row, and
  `agentbox dismiss` / an agent retracting) and all three must sweep the stack.
  The sweep was first written on one of them, and the CLI then cleared a summary
  and left five invisible notifications pending behind it.
- **Promoting a row means the stack steps aside FIRST.** Written the other way
  round the stack card simply retakes the screen and the promoted item sits
  behind it reading "1 waiting" - which is what the number key did on the real
  desktop while every unit test passed, because they all had the stack queued
  rather than on screen.

**FR83, multi-agent sync (designed in [09-sync.md](09-sync.md)): all five slices
are finished, deployed and verified live as of session 45.** Presence, discovery,
the rider, the Agents surface, locks, signals and shared values - all four
primitives, and the composition the feature was for: park on a signal, take a
lock, claim a chunk. Plus the teaching that makes it the default: the hooks are in
`~/.claude/settings.json`, so every session on this machine is on the board whether
its model remembers anything or not. Every number the design guessed is now
measured: the client's tool-call idle cap at 1800s (`tools/idlecap-probe.sh`, which
is why `wait_max_s` is 1500), and a CLI hold's ceiling at 120s for a foreground call
and 600s with an explicit timeout, which is why `agentbox sync lock NAME -- CMD`
cannot be the naive wrap the design first described.

**All five** slices found something the design had wrong, and **every one was
found by running it rather than by reading the diff**: slice 1's four surface
defects, slice 2's CLI-wrap ceiling, slice 3's gap check, slice 4's roster-only
ownership check, and slice 5's un-shareable minted session key - which four
sessions had missed by reading a recipe instead of installing it. That is the
pattern worth carrying to the next feature, and slice 5 is its sharpest form: **a
door nobody has walked through is not a door.**

**The opened-row detail was watched on 2026-08-06 (session 53), and two of its
three empty answers turn out to be unreachable by ordinary use.** Recent items
paints as designed: twelve rows at the `agentDetailItems` cap, newest first, each
with kind, state and age, and the Signals block above it on both a real row and a
hook-only one. The other two are correct defensive code that no session will meet.
*"This session has left the board"* needs `found: false`, but `Agents.svelte`
closes the detail the instant the roster stops listing the row, so the daemon's
answer can only be seen in the sub-second race between the surface's roster copy
and the daemon's map, or when the bridge call itself throws. *"Nothing behind it
yet"* needs a row with no timeline, no signals and no items - and every row on the
board got there by announcing, which posts the signal that fills the block. Both
were tried; the hook-only row showed its meta and its one announce, as it should.

**What the same sitting showed is that the activity line was the problem, not the
blocks under it - and it is now fixed.** The PostToolUse hook in
`~/.claude/settings.json` wrote the raw Bash command through `cut -c1-70`, which
truncates every line and drops none, so a heredoc arrived whole: one opened row
rendered a Go test file as a single wrapping activity line and a commit message as
another, and Recent items sat two screens below the fold. Six handoffs carried this
as a wording preference. It was a legibility defect, and the fix Boris authorised
on 2026-08-06 collapses the command to one line before truncating:

    jq -r '.tool_input.command' | tr '\n' ' ' | tr -s ' ' | sed -E 's/^(.{80}).+$/\1…/'

Watched on the board with the last old-format entry still in the ring directly
above the new ones - three wrapped lines against one, and the Signals block back
in the first screenful. The ellipsis is what says a line was cut, which `cut` never
did.

**`webui-demo agents` was the last unseen surface and it was watched on
2026-08-06 (session 54), with nothing wrong on it.** All four areas render, the
orphan lock carries its `1 waiting` count and its Break lock button, shared values
show the abandoned claim beside the held one and the ownerless value, the wait
chain names its holder and its place in the queue, and every state badge the
roster can produce appears at least once (working, blocked, asking you, listening,
quiet, driving desktop, reporting a percentage, never announced, seen not
attached). An opened row paints all four blocks off the canned fixture - meta,
HOLDS, ACTIVITY, SIGNALS, RECENT ITEMS - and Break lock opens its inline confirm
("Reassigns the lock. It does not stop this agent."), which Keep dismisses. The
one thing to know before repeating it: the footer counts inbox sessions and the
header counts roster rows, so `2 sessions` under `10 agents` is two populations,
not a miscount.

Two mechanics cost the sitting more time than the looking did, and both are about
the harness rather than the app. `import -window` captures window-relative
coordinates while `xdotool` clicks in screen coordinates, so every click must be
offset by the window's absolute origin (`xwininfo -id ID`) or it lands on the rail
and silently changes surface. And a click only reaches the page once the window
holds focus: `xdotool windowactivate` first, then move and click.

**The field defects this feature turned up are all closed.** FR86 (a project named
after whatever directory the agent stood in) and FR85 (two identity hues that
disagreed) were fixed together in session 46, which is also where a third
divergence turned up inside FR85: the frontend hashed UTF-16 where Go hashes
UTF-8. FR87 (a daemon restart replaying a stale activity line) was fixed in session
42. All three are in [07-field-requests.md](07-field-requests.md). FR84 (a form
clipping sentence-length options) closed the same day: Boris picked a shape from a
live mock of three, and the chosen option is now spelled out under the control.
**FR84 is fully closed as of session 54.** Its other half - a long body pushing a
form's fields down a scrolling card - was the mock's third approach, and Boris
picked it on 2026-08-06. Past 240 characters of body the controls come first and
the reasoning folds behind one line ("?" or a click). A diff is never folded: its
body is the thing being judged, which is the one cost the mock named for this
shape.

Closing it uncovered a defect older than the feature. `.card` is
`min-height: 100%`, so measuring the shell measures the WINDOW: once a card had
grown it reported the window's height back to Go forever and could never shrink.
Nothing on a card changed height mid-life before, so it had never shown. The fix
is in two parts and the second is the one that is easy to miss - past the
window's height the content pushes the shell out and the observer fires, but
under it min-height pins the shell and there is nothing left to observe, so
anything that can make the card shorter has to ask for a measurement itself.

**The inline panel deliberately does NOT fold, and session 55 settled why.** The
card folds because a fixed-height window puts its fields out of reach; the
panel's body is bounded at 260px and its controls sit under that, so they never
move however long the question is. Looking at one on a real long body turned up
the opposite defect instead: `.askwrap` had no `min-height: 0`, and a flex item's
automatic minimum is its content, so the panel held its full height whatever the
window did. The transcript went to nothing and the overflow came off the bottom -
which is the composer. At 1000x520 the reply box was a clipped sliver with no
Send button, so a long question could be read and not answered. The panel now
yields, in two stages, and both were arrived at on screen rather than on paper:
its body absorbs the squeeze first (`min-height: 0`, 260px still the cap when
there is room) so the controls stay put, and past that the panel scrolls as a
unit. `.composer` never shrinks at all.

The second stage is not belt-and-braces, it is a defect the first version
shipped with: at 22pt in a 900x560 window the body reached zero and the controls
carried on past the panel's own edge, over the composer and the status bar.
Leaning on the automatic minimum instead of `min-height: 0` does not fix it - the
panel's min-content includes the body's text, so nothing shrinks at all and the
composer goes straight back off the window. Both wrong shapes were built and
looked at before the right one.

`webui-demo ask`'s second item is a long-bodied one, added so the case is looked
at rather than reasoned about, and `webui-demo ask panel` puts the same items in
the drop-down panel.

**The 2026-08-01 priority reset is spent:** it put the main panel and recurring
assignments (FR81/FR82) first, and both shipped by 2026-08-04. FR95 closed on
2026-08-06, so **no field request is open**; [../HANDOFF.md](../HANDOFF.md)
carries the current short order and the numbered list below is the long tail
behind it, kept for the items nothing else records.

1. **FR74's fullscreen marker was watched on 2026-08-06 and is half right.**
   Session 49 held the desktop, put a `gnome-terminal` fullscreen and looked. The
   marker itself is exactly what was designed: a `1920x4` amber line at `+0+0`,
   present the whole time the fullscreen window has focus and gone within a beat
   of leaving it. **The strip does not step aside**, though, so a film gets the
   620x62 card AND the line. It is not the arithmetic - `planMark` returns
   `step: true` on one monitor and `beat` does call `x.lower(strip)` - it is that
   the strip is a NOTIFICATION type window with `_NET_WM_STATE_ABOVE`, and Mutter
   layers notifications above a fullscreen window whatever the stacking order
   says. **This needs Boris's word before it is fixed** (07-field-requests.md,
   FR74): the failure is in the safe direction - the guarantee is over-kept, not
   broken - and the fix is to hide the strip rather than lower it, which risks
   taking the keyboard back on the remap. That would be a worse defect than a
   card over a film. **Boris decided on 2026-08-06: leave it, it is the safe
   direction.** What he asked for instead is FR94.
1b. **FR94 - take the keyboard back mid-run. SHIPPED 2026-08-06, session 50.**
   Mocked first, driven by Boris himself, and all three open questions settled at
   the mock - two of them against the recommendation, which is the argument for
   mocking. What exists and was exercised on the real desktop:
   `Ctrl+Alt+Escape` (or the strip's own Pause button) latches the desktop back
   to him mid-run; the strip inverts to green `PAUSED - YOURS` with the frozen
   activity line still readable rather than vanishing; a running
   `drive_desktop` parks at the end of its step, and between characters inside
   a `type`, instead of failing or queueing; the latch is desktop-wide, so a
   second agent's `request_control` waits for HIM rather than being handed the
   desktop when the parked run releases; and nothing an agent can call resumes
   it. Verified live: a parked drive held 21.45s with the pointer frozen, then
   ran its two steps to the exact pixel on resume. The decisions and their
   reasons are in the FR94 entry; the mock is
   [mocks/fr94-pause-resume.html](mocks/fr94-pause-resume.html).
1c. **FR95 - get the strip out of a screen recording. SHIPPED 2026-08-06,
   sessions 51 and 52.** Mocked, settled by Boris at the mock (all four
   recommendations taken), and built. What exists and was exercised on the real
   desktop: `Ctrl+Alt+Q` (fired end to end through XTEST) and `agentbox control
   quiet|loud` demote the strip to a `1920x4` amber line at `+0+0`; a
   kiosk-fullscreen window over the top edge covers it completely, with the
   marker still mapped behind it and reappearing when the window goes; the marker
   is `#4FB286` green while the desktop is paused; the mode is not persisted and
   a 30-minute fuse takes it back to loud. **Session 52 added the fourth answer:
   the card column goes quiet with the sign.** Cards queue and drain when it goes
   loud, the earcon still plays (this is not DND - DND holds the chime), the
   spoken line is held until the card is on screen, urgent waits but is first out,
   and the progress window closes with them. Watched on the deployed build: three
   held cards and no AgentBox window on screen, `control state` reading `3 cards
   waiting`, the urgent one first out with "2 waiting" in its footer. The mock is
   [mocks/fr95-recording-mode.html](mocks/fr95-recording-mode.html); the entry
   carries the four decisions, the measurements, and the silent race the delivery
   had to be hardened against.
2. ~~**The re-record.**~~ **Dropped by Boris on 2026-08-06, for good, and the
   video half is now deleted rather than frozen.** He settled the open question
   the same day: delete the video, keep what is reusable. Gone:
   `tools/showcase/record.sh`, `take.sh`, `verify.sh`, `docs/recording.md` and
   `docs/youtube.md` - in git history if ever wanted, and nothing left in the
   tree points at them. Kept, because each stands on its own without a camera:
   `deck.py` (generates `docs/agentbox-showcase.pptx`, `make deck`),
   `perform.py` (drives and narrates a LIVE demo; its recorder marks and its
   recorded-monitor check were stripped), `console.jsx` (a working sandbox
   artifact, the only one that calls `agentbox.emit`), `tour.md` (a document
   exercising every `show_document` block) and `docs/showcase.md`, reframed from
   frozen to live-only. The uploaded take stays up as it is, missing the slide-11
   progress bar and wearing the old brand. Numbers in the deck that describe the
   product (the "fourteen tools" it still claims) are stale - check one before
   saying it to a room, but correcting them is not owed work.
3. **Resize affordance for frameless surfaces** (owner, 2026-07-28, from the
   FR58 mock round): a maximized artifact window can only be resized through
   WM chords (Alt+F8, Super+middle-drag), which is undiscoverable. Wanted:
   edge grips on AgentBox's frameless windows or double-click-to-restore on AgentBox's
   own title bar, with "any size I want, full screen the default" as the
   acceptance line. Artifact windows already open maximized (d656eeb).
4. **The earcons.** Boris: "they are mechanical, I would rather have
   something pleasant to the ear." `tools/genearcons/main.go` synthesises
   them (sine plus one harmonic, exponential decay); the WAVs are committed.
   He has to hear each attempt, so do it with him in the room.
5. **Two cosmetic defects at slide 17**: the OnlyOffice presenter strip
   flashes, and the app window is caught mid-transition as a grey rectangle.
6. **A JS test runner** (vitest + jsdom, two devDependencies and an
   `npm test` the Makefile can call). First candidates: `buildDocument`
   producing a document whose CSP and injected runtimes are what the source
   says, `compile` handling JSX and a TypeScript annotation, and
   `markdown.svelte.js` leaving a block Go produced alone.
7. **Live-verify the rest of M4 on a desktop session**: ask a question
   through MCP from a project session and confirm the click result
   returns; confirm a Stop hook chimes (recipes.md). Smoke-tested so far:
   handshake + tools/list, and the deployed server answers `notify`.
8. **Live-verify the M5 calm FRs** (daemon-tested; FR44's marker is seen
   live): FR45 - run a blocking `agentbox ask`, kill the caller, check the card
   flips to "caller disconnected" then auto-dismisses; restart the daemon
   with a pending ask and check "awaiting reconnect". FR47 - `agentbox mute
   claude-code`, confirm its cards stop surfacing while the tray and inbox
   badge it, `agentbox unmute` reveals them.
9. **Live-verify the rest of the FR29 presence gate**: one summary chime
   on return rather than a backlog, a fullscreen app (or GNOME's DND
   switch) holding a card and revealing it on leaving, urgent piercing.
10. **Packaging leftovers**: autostart on a real login (needs a logout),
   `make uninstall` reversing cleanly, and no tray menu item has been
   clicked by hand since the cutover.
11. **Remaining direction**: session-surface follow-ups (inline approval
   via the stream-json control protocol so the agent can act under a
   prompting mode; session keep-alive across an app-window close; a
   working-directory picker for new sessions); M6 refinements (per-token
   highlighting inside diffs, non-default markdown/viewer knobs); M7
   Wayland (DEFERRED by owner - validate activation/raise, color-scheme
   portal, fractional scaling, and add the Wayland fullscreen signal;
   FR29 fullscreen + card placement are X11-only today).
12. **Watch items**: (user-reported) if an undo strip ever visibly exceeds
    its countdown again, `agentbox logs` has item.grace_started with grace_ms;
    diff that against the item.answered timestamp. (session 25) one
    `make deploy` run failed its `-race` test gate and the identical re-run
    passed; the failing package went uncaptured - if it recurs, keep the
    log.

## Environment facts (for resumability)

GNOME Shell 46.0 on X11 (Ubuntu 24.04, HiDPI), GTK 4.14.5, WebKitGTK 2.52.3,
PipeWire via pipewire-pulse, Go 1.26.1, node/npm for the frontend, Cantarell
fonts at /usr/share/fonts/opentype/cantarell, module
github.com/borismilner/agentbox, origin git@gitlab.com:fu-bar/agentbox.git (path
renamed on GitLab 2026-08-03; the old URL redirects), public mirror
github.com/borismilner/agentbox.

Git: everything is on `main`, pushed and in sync with `origin`. There is no
CI and there are no PRs; Boris pushes to `main` directly. The history was
restarted as fifteen subsystem commits at the rename (2026-08-03; history.md
keeps the lineage), so hashes recorded before that no longer resolve.

`~/.config/agentbox/config.toml` exists (moved from the old config dir at the
rename), which it did not for the first fifteen sessions. It turns speech on
and points `[speech] command` at the Kokoro engine. That matters for a live
check: running the daemon without pointing `XDG_CONFIG_HOME` at a scratch
directory means a daemon that talks. Point it at a scratch dir to test
defaults.

Latency (measured 2026-07-25, daemon → surface mounted, so not first
paint): a card that needs a new window is on screen in ~360-400 ms, well
inside M1's 1.5 s cold budget. A queued item arriving while a card window
is open reuses that window - no window creation, no bundle load, just an
event - so the 300 ms warm budget is met with room to spare.
