# STATUS

Updated: 2026-08-04, forty-third session (FR83 slice 3 is COMPLETE: signals,
verified live and on screen, and its gap check was wrong until a live run caught
it). Session 42 the same day shipped locks and fixed the 30-minute fuse under
every blocking card; session 41 finished slice 1; session 40 built it; session 39
designed FR83.

AgentBox is **deployed and live on this machine**: module
`github.com/borismilner/agentbox`, binary and CLI `agentbox`, socket `agentbox.sock`, config
`~/.config/agentbox/`, state `~/.local/state/agentbox/`, unit `agentbox.service` (enabled,
active, systemd owns the daemon), MCP server `agentbox` (user scope, tools
`mcp__agentbox__*`), artifact API `window.agentbox.emit`, wire methods `agentbox.v1.*`.
Milestones M0 through M12 are done, **M12 (assignments) including its last
piece**: the custom HTML panel runs in the artifact sandbox with a two-way
channel - values out through `emit("params", ...)` into `SetAssignmentParams`,
values in through `window.agentbox.params` and the `agentbox:params` event, and
the daemon pokes open surfaces (`agentbox:assignments`) on every mutation so an
agent's edit lands in a panel somebody is looking at. See
[08-assignments.md](08-assignments.md).
FR81's visual pass over the remaining surfaces shipped in session 37, so the
rail speaks one language end to end.

**FR83's first slice is live: agents can see each other**
([09-sync.md](09-sync.md)). Every session can say what it is for and what it is
doing, find the peers sharing its repo, and appear on a new **Agents** rail
surface with a state chip the daemon derives rather than the agent claims. The
`~/.claude/CLAUDE.md` contract, the embedded manual and the hook recipes shipped
in the same commit as the tools, so every session on this machine is told to
announce itself. Two shipped bugs fell out of building it: `Conn.Serve` never
told a blocking handler its caller had hung up (so FR45's caller-gone indicator
had never fired), and the identity hue has two disagreeing implementations
(FR85, deferred).

**Slice 1 is finished, and the surface has now been seen.** Session 41 put four
real rows on the roster and looked at the board, which found four defects, all
fixed and re-checked on screen:

- A row never stopped saying `working`. State is derived at push time and every
  push is caused by a verb, so an idle board froze: it read "3 working" beside
  ages of 4m53s while the CLI called the same rows `quiet`. The roster ticks
  itself once a second now, and pushes only when the board would otherwise be
  wrong.
- `roster.Flush` had no caller anywhere, though its own comment said the daemon
  ticked it, so a push dropped inside the 250ms throttle waited for unrelated
  traffic - over a minute, in the field. The same tick delivers it.
- A group header was captioned with whichever member came first, which put
  `LAPTOP-SETUP` over the agentbox path. The caption is the area's own path now,
  and nothing at all when there is no honest answer.
- Every hook-created row was named `systemd`, because the prescribed attach is
  `setsid agentbox sync attach` and setsid reparents to init. Names now walk past
  shells and wrappers, fall back to `agent`, and are replaced when the session's
  own child announces. `AGENTBOX_AGENT` skips the guessing.

**The discovery rider is built and verified live.** When an agent's area gains or
loses a peer, one line rides back on the next tool result it gets - naming who
arrived, their purpose and their state - so an agent deep in a file finds out
without asking. Each arrival is reported once. `proto.Identity.Via` distinguishes
an mcp child from a shell, because a session's hooks call the CLI with that
session's key several times a minute and would otherwise eat every arrival before
the model saw it.

**Slice 2 is live: agents take turns.** Named advisory leases keyed to the
session key - `acquire_lock` parks in a FIFO queue, `try_lock` refuses at once,
`release_lock` hands it on - plus the CLI (`agentbox sync lock|unlock|locks|break`)
for Makefiles and hooks, the holds and waits on the Agents board, and Break lock
behind a two-step confirm. Verified against the deployed daemon by two live mcp
children and looked at on screen. Four things worth knowing:

- **A dead session does not free a live resource.** Its hold goes orphaned with
  the pid it recorded, and the next agent waits until that process is gone too.
  The board shows it as `LOCKS WITH NO LIVE HOLDER`, with "its pid N is still
  alive, so nobody gets this until it exits" - which is the only case where Break
  lock is the right button.
- **A deadlock is refused by name** at acquire time ("you asked for
  probe:repo, held by codex; codex waits on probe:deploy, held by you") and toasts
  the human. The two edges that cannot be refused - a holder parked on a question,
  a holder driving the desktop - warn instead.
- **`make deploy` could not use a sync lock.** It stops the daemon halfway, so a
  hold in the daemon's memory vanishes mid-install. It takes an flock instead; the
  one resource this daemon cannot arbitrate is the daemon.
- **Every blocking card was on a 30-minute fuse** (FR88), found while measuring
  the client's idle cap for the parked waits. Claude Code aborts a tool call
  silent for 1800s and nothing in the child ever spoke, so a card answered at
  minute 40 replied to a caller that was already gone. Fixed with a keep-alive
  ticker over every tool call. The last guessed number in the design is now
  measured: `tools/idlecap-probe.sh`.

**Slice 3 is live: agents wake each other.** `post_signal` and `await_signal`
(plus `agentbox sync post|await`), one global sequence as the only cursor, and
retention per topic and by age. A signal is stored, so it is delivered whether or
not anybody was listening: a peer that was busy picks it up later by cursor, and a
daemon restart loses nothing inside the window. `await_signal` returns everything
matching since the cursor in ONE batch, so three events that fired while an agent
was editing arrive together. Three topics the daemon posts itself -
`agents:<area>` on a join, announce or departure, `lock:<name>` when a lock
changes hands, and `to:<key>` for a message addressed to one session, which is how
direct messages ride the same rails with no mailbox subsystem. The composition is
the point: `await_signal(["tests:green"])` then `acquire_lock("deploy:agentbox")`
replaces a poll loop that spent a model turn per look. Three things worth knowing:

- **The gap check as designed was wrong, and only running it showed that.** "A
  cursor has fallen off the edge if it is below the oldest surviving sequence" is
  false for per-topic retention: one quiet topic's ancient row holds the global
  minimum down while the awaited topic is trimmed away underneath. Measured live
  with `signal_keep = 1` - a batch with two sequences missing came back reported as
  complete. Migration 0009 records what retention took, per topic, so a stale
  cursor is told (`gap: true`, plus the sequence a whole read starts from). Per
  topic and not globally, because `agents:<area>` is the chattiest topic here and a
  global watermark would cry gap at every unrelated read.
- **The `listening` chip finally has something behind it.** It had been on the
  surface since slice 1's mock with no daemon ever setting it. It sits below
  `blocked` deliberately - blocked is contention, listening is the feature working
  - and a listening row holds its state instead of decaying to `quiet`, so a parked
  agent does not look like a hung one. Photographed beside a row that had decayed.
- **A lock keeps two paths on purpose.** "The human broke your lock" still rides
  the discovery rider, because it is owed to the ex-holder personally; `lock:<name>`
  is for whoever is watching that lock and would rather be told it freed than park
  in `acquire_lock` again.

**A slice-1 defect surfaced while verifying slice 3** (FR90, fixed): only the
attach carried a cwd, and the attach is lazy, so an `announce` arriving first left
a row with **no area** - invisible to every area-filtered read, so another
session's `announce` could answer `alone: true` with a peer sitting in the same
repo. Its rider cursor was never initialized either, and `peersOf` with an empty
area returned every agent on the machine. The announce carries its cwd now, and an
unknown area answers "cannot say" rather than "everybody".

**What remains of FR83:** shared values - slice 4 in [09-sync.md](09-sync.md).

What else remains is the showcase re-record (decided, not yet scheduled) and the
verification and refinement queue.

This file is the current state. The session-by-session narrative - what each
session shipped, broke, learned and verified - is in [history.md](history.md).
The resume brief with live state and exact commands is [../HANDOFF.md](../HANDOFF.md).

## Deployed, and reachable from every Claude session

AgentBox is not only a repo. `~/.local/bin/agentbox` holds the current build, and it is
registered as a **user-scope** MCP server named `agentbox` in `~/.claude.json`, so
all 30 tools are available in every Claude Code session in every project -
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
  modelcontextprotocol/go-sdk v1.6.1) exposing 30 tools (the seven
  assignment tools included since M12); each proxies to
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
  take (choice/confirm/notify), not urgent, and the app window open. The
  panel does not take focus; a switcher row marks itself when its
  conversation is the one waiting; closing the app window with a question
  pending re-presents it as a card (`rerouteAsk`).
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
  flag is unwired). The surface edits the real `config.toml`, so point
  `XDG_CONFIG_HOME` at a scratch directory when poking at it.
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
  poll tick after 15 min. Reports fold into the app's Progress tab when
  `agentbox app` is open and fall back to a standalone window when it is closed;
  the set re-homes both ways.
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
  `agentbox docs walkthrough`).

## Tests

`make check` = gofmt + vet + `go test ./... -race`; **21 packages** (incl.
`internal/hand`, `internal/session`, `internal/webui`, `internal/speech` and
`frontend`), **568 tests**, all green as of the custom panel (session 38;
the count is `grep -c "^func Test"` top-level tests, 2026-08-04). There is no
automated *visual* check (see "Known gaps"); what a surface is *allowed to
do* is tested, and so is the HTML Go hands it.

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
- M6 refinements not done: per-token highlighting inside diffs (above), and
  the non-default `[markdown]`/`[viewer]` config knobs are not wired (their
  documented defaults already match the implementation).
- Config knobs documented but not wired: card/toast position (fixed at the
  chosen defaults), defer_minutes (Esc requeues untimed), ask.allow_reply
  global (per-item --strict works), `[agents."name"].earcon` and the
  `[sound.earcons]` per-class block (FR46 dropped as redundant; the M5
  roadmap note has the rationale).
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
- **Nothing in the UI says whether a block came from the capture or the live
  file.** `wireCode.Pinned` travels on the wire and the surface ignores it. It
  matters the moment a margin note stops matching its code, because that is the
  first question to ask. Boris was offered this and has not decided.
- The inbox surface has no per-item detail; a row shows title, identity,
  age and outcome, with the body only as a tooltip on resolved rows.
  Promote to the card to read a body. Queued as FR73 ("Do this next" 2) -
  no longer considered acceptable since a missed card lost its message.
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

**FR83, multi-agent sync (designed in [09-sync.md](09-sync.md)): slices 1, 2 and 3
are finished, deployed and verified live as of session 43.** Presence, discovery,
the rider, the Agents surface, locks, and signals. Every number the design guessed
is now measured: the client's tool-call idle cap at 1800s
(`tools/idlecap-probe.sh`, which is why `wait_max_s` is 1500), and a CLI hold's
ceiling at 120s for a foreground call and 600s with an explicit timeout, which is
why `agentbox sync lock NAME -- CMD` cannot be the naive wrap the design first
described. Next in the design's order: **slice 4, shared values** - the
compare-and-swap blackboard, whose change signals ride the hub that now exists.

**One defect found in the field on 2026-08-04 and still open:** FR86 (a project is
named after whatever directory the agent stood in, so an agent in `frontend/src`
reports project `src` and gets a second identity colour - fix it with FR85, they
are the same story). FR87 (a daemon restart replaying a stale activity line) was
fixed in session 42. Both are in [07-field-requests.md](07-field-requests.md).

**The 2026-08-01 priority reset is spent:** it put the main panel and recurring
assignments (FR81/FR82) first, and both shipped by 2026-08-04. [../HANDOFF.md](../HANDOFF.md)
carries the current short order (FR83 slice 4, then FR85 with FR86, FR89, FR84,
and the older FR73/FR65/FR74-marker queue); the numbered list below is the long
tail behind it, kept for the items nothing else records.

1. **FR74's last open piece: the fullscreen marker.** A fullscreen window may
   cover the strip, but a small always-visible marker must stay on top of it;
   needs FR29's `_NET_WM_STATE_FULLSCREEN` read, a second tiny window shape, and
   `keepOnTop` has to stop restacking the full strip once the marker exists. Do
   not skip the marker: a fully covered strip reads as "the desktop is yours"
   while an agent drives, which is the one wrong answer this feature can give.
   (The MCP control tools landed in session 32 - `request_control`,
   `set_activity`, `release_control` - and both manuals now route an agent to
   them before it invents a card of its own.)
2. **FR73 - a card body must be readable after the card closes.** The inbox row
   shows the body as a truncating tooltip and offers no detail view, so a card
   that timed out takes its message with it. Boris hit this on 2026-07-31: he
   missed a card, went to the inbox, and could not recover what it said.
   Nothing new needs storing - it is a reader.
3. **FR65 - open a citation in the editor**, the next board gap: an open
   button per code block, next to copy, running a configured editor command
   template. The JetBrains invocation is under "Mechanics discovered" in
   [07-field-requests.md](07-field-requests.md).
4. **The re-record.** Boris has said go on a from-scratch re-record and
   re-upload (the uploaded take is missing the slide-11 progress bar AND
   wears the old brand end to end); scheduling is his call, ~21 minutes of
   his screen. The blocker is gone: the top-most fix was seen working over a
   fullscreen stage on 2026-07-26 (session 24), and since session 25 the
   rehearsal itself asserts it - perform.py's `("above", title)` steps check
   `_NET_CLIENT_LIST_STACKING` for the progress bar, artifact, report, app
   window and panel, so a covered window fails the run instead of the film.
   Mechanics: `tools/showcase/take.sh`, preflight and traps in
   `docs/showcase.md`, the recorder in `docs/recording.md`. After a take,
   re-time the chapters in `docs/youtube.md` from the take's log, and listen
   to slide 1 in the rehearsal - the TTS spells out the product name where
   the narration names it. Before the camera rolls, refresh the tool count in
   `tools/showcase/deck.py` and the one-page argument in `docs/showcase.md`:
   both still say "fourteen tools" and the binary serves thirty.
5. **Resize affordance for frameless surfaces** (owner, 2026-07-28, from the
   FR58 mock round): a maximized artifact window can only be resized through
   WM chords (Alt+F8, Super+middle-drag), which is undiscoverable. Wanted:
   edge grips on AgentBox's frameless windows or double-click-to-restore on AgentBox's
   own title bar, with "any size I want, full screen the default" as the
   acceptance line. Artifact windows already open maximized (d656eeb).
6. **The earcons.** Boris: "they are mechanical, I would rather have
   something pleasant to the ear." `tools/genearcons/main.go` synthesises
   them (sine plus one harmonic, exponential decay); the WAVs are committed.
   He has to hear each attempt, so do it with him in the room.
7. **Two cosmetic defects at slide 17**: the OnlyOffice presenter strip
   flashes, and the app window is caught mid-transition as a grey rectangle.
8. **A JS test runner** (vitest + jsdom, two devDependencies and an
   `npm test` the Makefile can call). First candidates: `buildDocument`
   producing a document whose CSP and injected runtimes are what the source
   says, `compile` handling JSX and a TypeScript annotation, and
   `markdown.svelte.js` leaving a block Go produced alone.
9. **Live-verify the rest of M4 on a desktop session**: ask a question
   through MCP from a project session and confirm the click result
   returns; confirm a Stop hook chimes (recipes.md). Smoke-tested so far:
   handshake + tools/list, and the deployed server answers `notify`.
10. **Live-verify the M5 calm FRs** (daemon-tested; FR44's marker is seen
   live): FR45 - run a blocking `agentbox ask`, kill the caller, check the card
   flips to "caller disconnected" then auto-dismisses; restart the daemon
   with a pending ask and check "awaiting reconnect". FR47 - `agentbox mute
   claude-code`, confirm its cards stop surfacing while the tray and inbox
   badge it, `agentbox unmute` reveals them.
11. **Live-verify the rest of the FR29 presence gate**: one summary chime
   on return rather than a backlog, a fullscreen app (or GNOME's DND
   switch) holding a card and revealing it on leaving, urgent piercing.
12. **Packaging leftovers**: autostart on a real login (needs a logout),
   `make uninstall` reversing cleanly, and no tray menu item has been
   clicked by hand since the cutover.
13. **Remaining direction**: session-surface follow-ups (inline approval
   via the stream-json control protocol so the agent can act under a
   prompting mode; session keep-alive across an app-window close; a
   working-directory picker for new sessions); M6 refinements (per-token
   highlighting inside diffs, non-default markdown/viewer knobs); M7
   Wayland (DEFERRED by owner - validate activation/raise, color-scheme
   portal, fractional scaling, and add the Wayland fullscreen signal;
   FR29 fullscreen + card placement are X11-only today).
14. **Watch items**: (user-reported) if an undo strip ever visibly exceeds
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
