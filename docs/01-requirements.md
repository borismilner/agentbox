# Requirements

Tags: [must] v1 blocker, [should] v1 if cheap, [later] explicitly deferred.

## Functional

- FR1 [must] Notify: levels `info`, `success`, `warning`, `error`, `urgent`;
  title, body (markdown subset), source identity, optional sound override.
  Rendered as a toast (03-ui-ux.md): info/success auto-dismiss,
  warning/error stick until dismissed, urgent escalates to a card.
- FR2 [must] Ask, single choice: 2-9 options, each with label and optional
  description. Blocks the caller until answered, deferred past timeout, or
  cancelled.
- FR3 [must] Ask, free text: single-line and multi-line.
- FR4 [must] Confirm: yes/no with explicit phrasing supplied by the caller.
- FR5 [must] Timeout and default: every ask may carry `timeout_s` and an
  optional default answer. On timeout the caller gets
  `{answered: false, default_applied: true|false}`.
- FR6 [must] Cancel: a caller can cancel its own pending item (e.g. the user
  answered in the terminal instead). Answer delivery is exactly-once; answers
  arriving after timeout are recorded in history but not delivered.
- FR7 [must] Identity: every item carries agent name, project, and session id.
  The UI shows a colored identity chip (deterministic color from the hash of
  agent+project). Multiple concurrent agents are first-class.
- FR8 [must] Queue: pending items stack; the card shows "N more waiting".
  FIFO within severity, `urgent` preempts the displayed card.
- FR9 [must] Escalation: unanswered items replay their sound at a configurable
  interval up to a configurable count. `urgent` also sets the window manager
  attention hint.
- FR10 [must] History: every item and its outcome persisted in the app
  database for auditing; inbox window lists pending + recent, searchable.
  Retention: resolved items below a configurable importance level (default
  warning) are evicted after `retention_days` (default 30); items at or
  above it are kept indefinitely. Pending items are never evicted. Pruning
  runs at daemon start.
- FR11 [must] Do-not-disturb: queue silently; only `urgent` breaks through.
  Toggle from tray, CLI (`agentbox dnd on|off`) and inbox.
- FR12 [must] Transports: CLI subcommands; MCP stdio server (`agentbox mcp`);
  JSON-RPC over unix socket for direct integration. First client call
  auto-spawns the daemon.
- FR13 [must] Sounds: distinct short earcon per event class, bundled in the
  binary; global volume, mute, quiet hours.
- FR14 [should] Tray icon (StatusNotifierItem) with menu: open inbox, DND
  toggle, quit. The app must remain fully usable without a tray (stock GNOME
  has none).
- FR15 [should] Summon: `agentbox summon` raises/focuses the current card; meant
  to be bound to a desktop keyboard shortcut.
- FR16 [should] Image attachment: an ask/notify may reference a PNG path
  (e.g. a screenshot the agent took); shown scaled in the card.
- FR17 [must] Config file `~/.config/agentbox/config.toml` covering every
  critical behavior, live-reloaded, with defaults chosen per
  06-configuration.md. Per-agent overrides included.
- FR18 [later] Multi-select choice questions.
- FR19 [later] Subscribe API (server push) for companion surfaces.
- FR20 [later] Webhook relay for away-from-desk delivery.

Interactions that are hard or unsafe in a terminal (the reason AgentBox exists
beyond notifications):

- FR21 [should] Progress: a thin card for a long-running task, live percent
  or spinner plus status line, driven by repeated calls or a stdin stream
  (`long_task | agentbox progress --title "Migrating"`). Completion emits a
  success or error toast.
- FR22 [should] Veto (act-unless-stopped): the caller announces an action
  with a countdown window (default 15 s) and proceeds unless the user stops
  it. Result `{vetoed: bool}`. The terminal equivalent (a blocking confirm)
  punishes the common case where no answer is needed.
- FR23 [should] Secret input: masked entry whose value bypasses the agent
  transcript by default - written to a caller-named file (mode 0600);
  returning it on stdout requires an explicit opt-in flag and a visible
  warning on the card. History records that a secret was provided, never
  the value.
- FR24 [later] Native file/path picker question.
- FR25 [later] Clipboard handoff: agent puts text on the clipboard, user
  gets a confirmation toast.
- FR26 [should] Form card: one card with up to ~6 typed fields (choice,
  text, bool), answered in one round trip instead of N questions.
- FR27 [must] Reply-instead escape hatch: every choice/confirm card accepts
  free text as an alternative answer; result is `{reply}` instead of
  `{answer}`. Callers may disable with `--strict` for machine-parsed
  choices.
- FR28 [must] Answer undo grace: answers are delivered after a short grace
  window (default 3 s) during which an "Answered: X (undo)" strip allows
  taking it back. Delivery to the caller happens only after the grace
  expires (refines FR6 exactly-once).
- FR29 [should] Presence-aware delivery: when the user is idle (desktop
  idle monitor), chimes hold and escalation pauses; on return, one summary
  chime and the oldest pending card. Auto-DND while a fullscreen app is
  focused. Treat the desktop's own do-not-disturb as DND.
- FR30 [must] Flood control: per-agent rate limits; an agent exceeding them
  has its items collapsed into a single stack card plus one warning toast.
  Calm survives buggy callers. **Built 2026-08-06** (`internal/daemon/flood.go`,
  kind `stack`), with one deliberate departure from the line above: the warning
  is the stack card, which is warning-level and says why it appeared, rather
  than a second toast beside it. A flood answered with two cards would be the
  noise this requirement exists to remove.
- FR31 [should] Delayed items: `--in 30m` / `--at 15:00` on notify;
  scheduled through the same queue and store.
- FR32 [should] Action buttons: caller-supplied buttons on notify/toast
  items that exec a local command on click ("Open PR", "View log", "Jump to
  terminal"). The exact command is visible on hover; a config switch
  disables the feature globally.
- FR33 [should] Diff review card: unified diff rendered with syntax
  highlighting; approve / reject / comment; result
  `{approved, comment?}`.
- FR34 [should] Triage mode: keyboard-driven batch answering in the inbox
  for queues that built up while away.
- FR35 [should] Interruption insights: `agentbox stats` over the history
  (interruptions per agent/project/day, median time-to-answer); a one-line
  daily count in the inbox footer.

Rich content (terminals render markdown poorly; AgentBox renders it
brilliantly):

- FR36 [must] Full markdown rendering everywhere a body appears:
  CommonMark + GFM (tables, task lists, strikethrough, autolinks),
  footnotes, GitHub-style alerts, emoji shortcodes, inline images;
  code blocks syntax-highlighted with language badge and copy button.
  Stack and quality bar in ADR-0008.
- FR37 [must] Document viewer: `agentbox show FILE|-` opens a reading-quality
  window with the same engine; `--watch` re-renders on file change keeping
  scroll position. MCP tool `show_document`.
- FR38 [should] Charts: fenced `chart` blocks (line/bar/pie/scatter, data
  inline or CSV path) rendered natively in the theme palette. Mermaid
  blocks render opportunistically if mermaid-cli is installed; otherwise
  they show as highlighted code. (As built, M9: mermaid is drawn by a
  bundled engine loaded on first use - no external CLI, ADR-0008
  amendment.)
- FR39 [later] Math (LaTeX) rendering.

Self-teaching (the executable alone must be enough for an agent to use
everything, vision principle 9):

- FR40 [must] Embedded manual: the docs/ tree ships inside the binary
  (go:embed); `agentbox docs` lists topics, `agentbox docs TOPIC` prints one.
  `agentbox docs agent` prints a compact agent quickstart: every capability,
  CLI + MCP forms, JSON shapes, exit codes, and guidance on when to
  interrupt the user - sized to paste into an agent context (CLAUDE.md,
  AGENTS.md). `agentbox docs setup` emits ready-to-paste `.mcp.json` and hook
  snippets.
- FR41 [must] Teaching errors: misuse of any subcommand or RPC returns the
  correct usage form with a concrete example, not just "invalid flag".
  Exit codes are a stable, documented contract.
- FR42 [should] `agentbox schema`: JSON Schema for the wire protocol and all
  item kinds, so agents and integrations can validate programmatically.
- FR43 [must] Copy to clipboard: every card and toast has a copy button
  (and the `c` key) that puts the complete item on the clipboard in a
  plain-text, agent-pasteable format: id, kind, level, identity, title,
  body, options, default, timeout. Made for handing a notification back to
  an AI agent with nothing lost.

Calm and multi-agent refinements (each fills a gap in the features above):

- FR44 [should] Missed-while-away marker: info/success toasts that
  auto-expire while the user is idle (presence gate, FR29) are not lost.
  They are flagged "missed while away" in the inbox so the return-from-idle
  review separates toasts that flashed unseen from ones actually read.
  Reuses the FR29 idle signal and `presence.idle_after_s`; no new knob. Low
  toasts are not turned into blocking pending items - the flag is a history
  marker, not a fresh interruption.
- FR45 [should] Caller-alive indicator: while a blocking card is shown, a
  dot by the identity pill tracks the caller's socket connection - live, or
  "caller disconnected" when it drops (the answer would then only reach
  history, FR6). A disconnected card auto-dismisses shortly or on any key,
  so no thought is wasted answering a dead question. Items restored after a
  daemon restart (NFR7) show "awaiting reconnect" until the agent resubmits.
- FR46 [should] Per-agent sound signatures: a per-agent `earcon` override
  (06-configuration.md) gives each agent a distinct timbre within its
  severity class, so a multi-agent workflow is identifiable by ear before
  looking up. Severity still picks the earcon class; the override picks the
  timbre within it. A custom file path is the baseline; built-in
  pitch-shifted variants are an optional extra.
- FR47 [should] Runtime agent mute: `agentbox mute <agent>` / `unmute <agent>`
  / `mute --list` silence a flooding agent instantly - in memory, ephemeral,
  cleared on restart - without editing config. This is the quick reaction
  the config `mute` (FR17, ~2 s live reload) is too slow for; permanent
  policy still belongs in the file. Muted agents' items go straight to
  inbox; tray and inbox show a "(muted)" badge.
- FR48 [later] Queue peek: a key press (or hover on a waiting dot) previews
  the titles and kinds of queued items without cycling the displayed card
  (FR8). Read-only; dismisses on any other action. Queue reordering is out
  of scope - FIFO plus urgent-preempt stays the queue model.
- FR49 [later] Session surface (post-v1, roadmap M8): a tab that starts and
  drives a Claude Code session inside AgentBox, rendering the conversation
  through the FR36 engine (markdown, code, tables, charts) in a comfortable
  reading layout. Live font family and size, save/export the conversation,
  find-in-conversation search, and in-flow answering via the existing
  ask/confirm/veto/form cards. The terminal may run alongside; the goal is a
  graphically and functionally superior surface to it. This revisits the
  v1 non-goals (00-vision.md) on purpose and is sequenced last; M6 is a hard
  prerequisite.
- FR50 [must] Dismissing a question: a blocking card can be walked away
  from for good - shift+Esc on the card, d/backspace in inbox triage -
  resolving the ask as unanswered (Answered=false), exactly like a
  timeout. Plain Esc stays "later" (FR8 requeue), so one mistyped Esc
  never kills an agent's question. Motivation: voluntary prompts (a
  caller asking on the user's own initiative, e.g. nudge's idea capture)
  have no agent that must be answered; without a dismiss they re-queue
  forever. Veto and diff cards are excluded: walking away from those has
  action consequences, their existing keys stand.

## Non-functional

- NFR1 Single static Go binary. Native rendering; no webview, no browser
  engine, no separate runtime. CGO-free if the chosen toolkit allows it.
  Amended 2026-07-24 (ADR-0009, mirroring vision principle 6): still one
  executable, but the UI is a WebKitGTK webview - GTK4 and WebKitGTK are
  linked at runtime, traded deliberately for visual quality.
- NFR2 Warm path latency: CLI invocation to visible card under 300 ms.
  Daemon cold start to visible card under 1.5 s.
- NFR3 Resident daemon under 60 MB RSS idle.
- NFR4 Keyboard: every interaction reachable without a mouse.
- NFR5 Focus safety: cards never take keyboard focus implicitly (see
  vision principle 3); explicit `--focus grab` exists for callers that
  genuinely need a modal.
- NFR6 Dark/light follows the system setting live.
- NFR7 Crash safety: pending asks survive a daemon restart and are
  re-presented; callers see a reconnect, not a lost answer.
- NFR8 Security: socket directory mode 0700 under `$XDG_RUNTIME_DIR`; peer
  UID checked via SO_PEERCRED; no TCP listener anywhere.
- NFR9 Accessibility: labels for screen readers where the toolkit supports
  it; respect reduced-motion; all colors meet WCAG AA contrast.
- NFR10 X11 today, Wayland-ready: no X11-only mechanism may be load-bearing
  without a documented Wayland equivalent (see 04-platform.md).
- NFR11 Zero-config first run: an empty or absent config file gives the
  full intended experience; configuration tunes, never enables.
- NFR12 Single instance by default: one daemon per user session, enforced
  by an atomic lock. N agents auto-spawning at the same moment converge to
  one daemon; the losers exit cleanly and their clients connect to the
  winner. A second instance requires an explicit `--instance NAME` (own
  socket, own state, own history); nothing an agent does accidentally can
  create one.
- NFR13 Logs are the debugging interface (AgentBox is maintained by AI
  agents): every meaningful event - state transition, IPC call, spawn,
  config reload, render or sound failure - emits one structured JSON line
  with stable keys (component, event, item id, agent identity, outcome).
  Errors carry the full wrapped cause chain; panics carry a stack trace.
  The bar: an agent reading the log can reconstruct what happened and why
  without opening the source. Secret values are never logged at any level.
- NFR14 Build provenance: the binary embeds git commit, dirty flag and
  build timestamp (Go VCS build info, no ldflags ceremony). `agentbox
  version` prints them, `agentbox status --json` includes them, the daemon
  logs them at startup. A dirty flag in a release build is a defect.
- NFR15 Database lifecycle: the SQLite database is created automatically
  at first start; schema migrations are embedded in the binary, versioned,
  forward-only, applied transactionally at startup before the socket
  binds. A failed migration aborts startup losslessly with a teaching
  error; an older binary meeting a newer schema refuses to run. Details in
  ADR-0005.

## Open questions

All settled as ADRs: UI toolkit (ADR-0002, superseded by ADR-0009's
webview), sound playback (ADR-0006), the name (decided by Boris; its ADRs
were retired with the old names). The ADR ledger in STATUS.md tracks the
full set.
