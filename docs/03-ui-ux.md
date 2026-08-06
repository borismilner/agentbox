# UI / UX

The item surfaces plus a tray menu: the toast (glanceable), the card (the
product), the inbox (support), the viewer (reading). This document is those four
and the language they share - focus policy, keyboard map, motion, sound, states.

The app window's own surfaces are each documented where their feature is, because
each was designed with it rather than here: the **Agents** board in
[09-sync.md](09-sync.md), the **assignments** surface in
[08-assignments.md](08-assignments.md), and the **review board** and walkthrough
library under FR58/FR59/FR61 in [07-field-requests.md](07-field-requests.md).

## Toast

The rendering of notify items: a compact strip at the top center of the
screen, with a drawn severity icon (circled i, check, warning triangle,
crossed circle, bell), severity accent and tint, mini identity chip, title,
optional body. The whole look reads as its severity at a glance. Text is
never clipped hard: the title always shows whole; the body wraps up to
three lines before "click to expand" takes over.

- info/success auto-dismiss (default 6 s); the timer pauses while the
  pointer is over it. warning/error stick until dismissed. urgent skips the
  toast and becomes a card with escalation.
- Click expands to a full card (whole body, links); Esc or click-away
  dismisses to history.
- Stacking: up to 3 toasts, newest on top; beyond that they collapse into a
  "+N more" collector that opens the inbox. No toast spam, ever.
- Same motion language as cards, shorter (120 ms in, 90 ms out).

As built (M9 slice 5), with three deliberate differences from the above:

- **No stacking.** The daemon presents one item at a time (a single queue with
  one current item), so two toasts cannot be on screen together; the second
  waits its turn. The "+N more" collector has nothing to collect and was not
  built. If the queue ever presents in parallel, this is where to start.
- **The countdown does not pause on hover.** The daemon owns the deadline and
  the timer that fires it (`setCurrentLocked`), which is what makes the toast
  honest while the user is away or in DND; a surface cannot pause it without
  the daemon growing a "hold" verb. Left alone deliberately.
- **A long body expands in place** rather than promoting to a card: the body is
  clamped to three lines with a "click to expand" affordance, and clicking
  grows the window. A click anywhere else dismisses, the way the Gio toast did;
  action buttons and the ✕ stop the click so nobody dismisses a notice while
  reaching for one of its actions. There is no click-away to catch - the strip
  never has focus, and a webview cannot see a click outside its own window.

## Card

Appears dead center of the monitor that has the pointer (both axes), above
all windows: the comfortable place to read and act on something that wants
a decision. Position is configurable for those who prefer a corner.

```
+--------------------------------------------------+
| (#) claude-code . devtool           14:32   Esc  |
|--------------------------------------------------|
| Run DB migration on staging?                     |
|                                                  |
| The diff adds a non-null column to events.       |
| Backfill will take about 4 minutes.              |
|                                                  |
|  [1] Run now     [2] Dry run     [3] Skip        |
|                                                  |
|  () 4:32 left . default: Dry run    2 waiting    |
+--------------------------------------------------+
```

Anatomy, top to bottom:

- Identity row: a tinted pill in the agent's deterministic hue (hashed
  from agent+project, red hues excluded) carrying a solid dot, agent name
  and project; copy and defer affordances on the right. The pill is loud
  on purpose: when agents alternate, who is asking must be unmissable.
- Severity accent: a 3 px bar on the left edge, colored by level. Urgent
  pulses gently; nothing else animates while idle.
- Title: one line, large. Body: full markdown (see Markdown rendering
  below); links open in the browser. Long bodies scroll inside the card;
  truly long content belongs in the viewer, and the card offers "Open in
  viewer" past ~40 lines.
- Answer zone: option buttons with their number shortcut printed on them, or
  a text field, or yes/no. Descriptions render as a second muted line.
  Choice and confirm cards also accept "/" to open a reply field (FR27):
  free text back to the agent instead of any listed option.
- Action buttons (FR32, notify items): caller-supplied, exec a local
  command on click; the command appears verbatim in the hover tooltip.
- Footer: remaining time when a timeout exists, the default answer, and
  "N waiting" with one identity-colored dot per queued item, so a second
  agent's pending question is visible while another agent's card is on
  screen.

After answering, the card collapses to an "Answered: Run now" strip with
an "undo · u" button and a live "sending in Ns" countdown (FR28, default
3 s, hard ceiling 5 s); the answer leaves for the agent only when the
countdown ends. Undo restores the card untouched.

## Focus policy (the load-bearing ergonomic decision)

Pop above, never grab. The card maps above all windows but does not take
keyboard focus; your keystrokes keep going wherever you were typing. To
answer: click the card, or press the summon shortcut (desktop keybinding ->
`agentbox summon`), which focuses it. Once focused, single keys work (1-9, y/n).
Callers may request `--focus grab` for true modals; default is never.

Rationale: a card that steals focus mid-typing can swallow a password or let
a stray keystroke answer a production question. The 300 ms it costs to press
the summon key is cheaper than one wrong "yes".

## Keyboard map (card focused)

| Key | Action |
|-----|--------|
| 1-9 | choose that option |
| y / n | confirm / reject (confirm cards) |
| Enter | accept default (shown in footer); submit text field |
| Ctrl+Enter | submit multiline text |
| Tab / arrows | move between options / form fields |
| / | reply instead: free text back to the agent (FR27) |
| u | undo, while the answered strip shows (FR28) |
| Esc | defer: requeue for 5 min, card hides |
| c | copy the whole item to the clipboard, agent-pasteable (FR43) |
| Ctrl+L | jump to next waiting item |
| Ctrl+I | open inbox |

Global (via desktop shortcut, suggested Ctrl+Alt+K): `agentbox summon`.

## Card variants

- Progress (FR21): thin card, title, progress bar or spinner, one status
  line, no answer zone. Updates stream in; completion collapses it into a
  success/error toast. Multiple progress cards stack compactly.
  As built (M9 slice 5): its own window rather than a card, because a report is
  not in the queue and must never wait behind a question. It opens on the first
  report, grows with the set, closes when the last one finishes, sits in the
  bottom-right corner (the middle of the screen belongs to whatever is asking
  something), and maps without taking the keyboard. Its ✕ hides the readout;
  the tasks keep running and the next report brings the window back.
- Veto (FR22): one big button carrying the countdown, "Stop - pushing to
  main in 0:12". Doing nothing is the answer; the button is the brake.
  Plays the question chime once, no escalation.
- Secret (FR23): masked field, paste allowed, hold-to-reveal toggle. The
  card states where the value goes ("written to .env.staging"); if the
  caller asked for stdout return, an amber warning says the value will
  enter the agent's context. Never echoed to history.
- Form (FR26): up to ~6 stacked fields, Tab moves through them, Enter on
  the last (or Ctrl+Enter anywhere) submits the lot. One round trip, one
  undo strip for the whole form.
- Diff (FR33): unified diff with syntax highlighting and a gutter, capped
  height with internal scroll; Approve / Reject buttons plus an optional
  comment line. Monospace, no markdown processing inside the diff.
- Inline ask panel (FR49, as built M9 slice 6): the one case where a question
  does not get a window. When the agent asking is one running in AgentBox's own
  session surface, the question renders in that conversation - directly above
  the composer, on the same measure, wearing the same severity rail a card
  would - because the context the question is about is what a card would cover.
  It takes choice, confirm and notify; text, secret, form, diff, veto and
  anything urgent still pop their card. It never takes the keyboard: the
  composer keeps focus, the answer keys act only when nothing is being typed
  (Esc in the composer hands the keyboard over), and the hint line says which
  of the two you are in. A non-selected session marks its switcher row instead,
  since the panel is only visible in the conversation it belongs to. If the app
  window closes with a question in the panel, the item gets its card - it is
  never left somewhere unreadable.

## Inbox

A plain list window: pending items on top (answer inline), then history with
outcome, identity, and timestamps. Filter box, level filter, per-agent
filter. DND toggle in the header. This window is allowed to behave like a
normal app window.

Triage mode (FR34): when several items are pending, j/k moves through them,
each rendered as a compact answerable row (same shortcuts as cards); answer
one and focus lands on the next. A queue built during a meeting clears in
seconds. The footer shows today's interruption count (FR35); `agentbox stats`
gives the full picture.

## Viewer

`agentbox show FILE|-` (and the MCP `show_document` tool): a reading-quality
window for whole documents. Content sits on a ~760 px measure regardless of
window size; generous line height; headings on a consistent scale. A normal
app window, behaves like one.

- `--watch`: re-render on file change, scroll position preserved. An agent
  iterating on a doc becomes a live preview.
- Keyboard: j/k and PgUp/PgDn scroll, "/" find-in-page with match count,
  Ctrl+= / Ctrl+- zoom, q closes.
- Per-code-block copy buttons work here as on cards.

As built (M9 slice 5): a frameless window carrying its own title bar (document
name, a "watching" badge, find, zoom, close) and a footer with the path, the
zoom level and the key hints - none of which a WM decoration could show. Find
wraps matches in the document and scrolls to them, with prev/next and an "n/m"
count; it skips charts and diagrams, because SVG has no `<mark>` and wrapping a
word inside one deletes it from the picture. `g` and `G` jump to the ends. The
same surface fills the app window's viewer slot without the chrome, showing
whatever document is open, so `agentbox show` and the app agree.

## Markdown rendering

The quality bar: better than GitHub's web rendering, in a native window
(stack in ADR-0008). Supported everywhere a body renders:

- GFM complete: tables with real column layout and alignment, task lists
  (interactive display, not editable), strikethrough, autolinks, footnotes
  that jump, horizontal rules.
- GitHub alerts (`> [!NOTE]`, `[!WARNING]`, ...) as tinted panels with the
  severity palette.
- Code blocks: chroma highlighting with a dark/light theme pair, language
  badge, copy button, horizontal scroll (never wrapped code), subtle line
  numbers for blocks over 10 lines.
- Math (FR39, M10 slice 4): TeX typeset by KaTeX, in all four spellings an
  agent might write - `$...$` and `\(...\)` inline, `$$...$$` and `\[...\]` as
  display, plus an explicit ```math fence. A formula KaTeX refuses shows its
  own source instead of vanishing. `$5 and $10` stays money: a closing dollar
  may not follow a space or precede a digit.
- Images (M10 slice 4), scaled to the measure and lazily decoded - but only
  ever from bytes AgentBox has read itself. See below.
- Charts (FR38): fenced `chart` blocks rendered as native vector plots in
  the theme palette; axes and legends follow the typography. Mermaid drawn
  by a bundled engine loaded on first use (ADR-0008 amendment).
- Typography: optical sizes per heading level, mono that aligns with the
  text baseline, tabular figures in tables.

### What an image may name (M10 slice 4)

Raw HTML never reaches a surface, so an `<img>` cannot be written by hand - but
`![alt](src)` is ordinary markdown, and left to goldmark it becomes a live
`<img src>` pointing anywhere. That is a request the surface would make on an
agent's behalf, to a host the human never saw, carrying whatever the agent put
in the path. So Go decides instead (`internal/webui/images.go`), on the same
principle as ADR-0010: bind the markup, not the agent.

- A local file is read by Go and inlined as a `data:` URI. The surface is handed
  bytes and never learns a path.
- The path must be absolute (`~` counts), unless the document is a file AgentBox
  opened itself. `agentbox show FILE` and the viewer's watch loop hand the renderer
  the file's own directory, so `![](out/chart.png)` in a document means what it
  means in an editor. Prose arriving over the socket - a card body, an agent's
  turn - gets no base and a relative path is refused: the daemon's working
  directory is not the agent's, and guessing would be wrong more often than
  right. Resolving adds a way to *name* a file, never a way to reach one an
  absolute path could not already: the joined path goes through the same stat,
  sniff and ceiling.
- The bytes must really be a PNG, JPEG, GIF or WebP, by magic number rather than
  by extension. SVG is deliberately absent: a vector picture from an agent is a
  `chart` or `mermaid` fence, both of which AgentBox draws itself.
- 2 MB ceiling, checked against the file size before anything is read. Encodings
  are cached by path, modification time and size, because a streaming turn
  re-renders every turn it contains ~16 times a second.
- Everything else - http, https, protocol-relative, a scheme AgentBox has never
  heard of - is not fetched. It renders as a marked placeholder carrying the alt
  text and the reason ("remote image not loaded", "needs an absolute path",
  "file not found"), with the destination in a `title` attribute for whoever is
  debugging - the resolved path, when there was a base, since that is the
  question being asked. A reader should know something was meant to be there.

`index.html` carries `img-src 'self' data: blob:` and no other directive, so the
rule is enforced rather than merely intended: an edit that ever emitted a remote
src still would not load.

## Artifacts (M10)

An artifact is agent-authored HTML AgentBox **runs**: a chart you hover, a slider you
drag, a control panel you click. It appears wherever markdown does (an `artifact`
fence in a conversation, and an `html` fence that is a document rather than markup
being explained) and in a window of its own from `show_artifact` or
`agentbox show --artifact FILE`.

As built:

- A bar above every artifact: an `interactive` badge, the title, which runtime it
  asked for (`react + tailwind`), any note worth reading (a blocked CDN, a compile
  error, an exception it threw), a **preview/code toggle**, and a reload button on
  hover. The code tab is the source that is running, character for character -
  reading what an agent wants to run is part of trusting it.
- Inline, it sizes itself to its content and stops at `[artifact] max_height_px`;
  in its own window it drops the reading measure and fills the window, because a
  program in a 760 px column is a program in a column.
- It cannot reach the network or the surface around it (ADR-0010). Its colours come
  from AgentBox's tokens, handed in as values, so an artifact that styles nothing still
  belongs in the window - and a theme change repaints it rather than reloading it,
  so whatever the human had typed into it survives.
- `[artifact] enabled = false` leaves the source and runs nothing, live and
  retroactively.
- What the human does in it reaches the agent through `window.agentbox.emit`, which is
  the only way out: `await_artifact_event` blocks on it, `read_artifact_events`
  drains it coalesced. A dragged slider is one value, not forty.

Known rough edge: while an agent is still streaming the turn an artifact is in, the
conversation re-renders its HTML each frame and the artifact restarts with it. Once
the turn is finished the block is stable. An artifact in its own window never has
this.

## Motion and visual language

- Card entry: 160 ms slide + fade from the top edge, ease-out. Exit: 120 ms
  fade. Respect the desktop reduced-motion setting: then instant.
- 8 px spacing grid, 12 px corner radius, one elevation shadow.
- System UI font (Cantarell on GNOME) via fontconfig; mono for code.
- Dark/light follows `org.gnome.desktop.interface color-scheme` live (read
  via the settings portal so it also works on Wayland later).
- Severity palette (AA contrast on both themes): info blue, success green,
  warning amber, error red, urgent red + attention hint. Identity hues avoid
  the severity hues.
- No icons-for-everything; text first, one glyph per severity.

## Sound design

Earcons bundled in the binary, all under 400 ms, quiet by default:

| Class | Character |
|-------|-----------|
| info / success | single soft pop / short tick |
| warning | two-tone descending |
| question | rising two-note chime (invites action) |
| error | low thud |
| urgent | three-note insistent figure, replayed by escalation |

Global volume and mute in config + tray; quiet hours window in config.
DND silences everything except urgent.

### Speech (M11)

An earcon says *something happened, and roughly how much it matters*. It cannot
say what. So an agent may attach one spoken line to anything it puts on screen,
and AgentBox reads it out **after** the chime: the level still arrives as a sound you
recognise without thinking, and the meaning arrives behind it as a sentence.

The division of labour is the whole design:

- **The agent writes the line.** AgentBox never reads a title or a body aloud on its
  own. A title is written to be taken in at a glance and makes poor speech, and
  reading them automatically would make every `agentbox notify` already in somebody's
  scripts start talking. So there is no heuristic: an item speaks if it carries a
  spoken line, and is silent if it does not, which is the default.
- **AgentBox owns when.** The line rides on the item, so it inherits every gate the
  chime has - held while the desktop is idle, dropped for a muted agent, suppressed
  by DND, silent in quiet hours (all of it, including urgent: a chirp at 3am and a
  voice at 3am are not the same event). Escalation repeats it, because that is what
  escalation is.
- **Quality is not a preference.** AgentBox uses the best voice installed and asks for
  the maximum resampler quality, since a synthesised voice is 22.05 kHz and a
  modern sink is 48 kHz. Details and the measurements in 06-configuration.md.

Off until a voice is configured (`[speech] enabled`). `agentbox say` and the `speak`
tool say a line with nothing on screen behind it - it never enters the queue, the
store or the inbox, because saying something out loud is not an interruption to be
triaged later.

## States

- Away (FR29): the idle monitor decides. While idle, chimes hold and
  escalation pauses; on return, one summary chime and the oldest pending
  card (with "N waiting"). No metronome in an empty room.
- Fullscreen / presenting (FR29): auto-DND while a fullscreen app is
  focused; nothing pops over a presentation or screen share, secret cards
  included. The desktop's own do-not-disturb is honored as DND.
- Recording (FR95): the hands-off strip drops to a 4px marker on the top edge
  and the whole top-centre column goes with it - cards queue, the progress
  window closes, urgent waits too and comes out first when it ends. The earcon
  still plays, which is what makes this different from DND: the picture is
  quiet, not the notification. Thirty-minute fuse, never persisted.
- Flooded (FR30): an agent over its rate limit collapses into one stack
  card ("claude-code: 14 items") plus a single warning toast; the items
  land in inbox triage.
- Multiple agents: queue is shared; identity chip disambiguates. No tabs in
  v1; Ctrl+L cycles.
- DND: tray icon dims; urgent still pops.
- Daemon restart: pending items re-presented from the store (NFR7).

## Delight

- First run: a greeting card introduces itself and demos the interaction
  (choose an option, hear the chime) so the first real card is familiar.
- Identity hues, the countdown ring, and the question chime are the
  personality budget: small, consistent, never cute at the cost of speed.

## Anti-goals

- No toast spam: notify items without escalation appear, wait, and slide to
  history; they never stack more than one card high.
- No focus stealing (above), no key grabbing while unfocused, no always-modal.
- No required setup: full experience with zero config; every critical aspect
  has a knob with a smart default (06-configuration.md), and knobs are added
  for real irritations, not preemptively.
