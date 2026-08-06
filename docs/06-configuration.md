# Configuration

Principle: zero required setup. AgentBox is fully usable with no config file;
every critical behavior has a knob; every default is chosen so most days you
never open the file. Layering, lowest to highest precedence:

1. built-in defaults (below)
2. `~/.config/agentbox/config.toml`
3. per-item caller flags (`--sound`, `--focus`, `--timeout`, ...)

The daemon watches the file and applies changes live - a 400ms poll since
M10, chosen so tuning AgentBox is a conversation rather than a wait; invalid keys
are logged and skipped, never fatal. Appearance is included: since the M9
webview port a theme, font, window size or measure change is a CSS variable
write and a resize on every open window, so volume, placement and theme are
tuned by feel against whatever is on screen. Only the knobs the daemon
genuinely reads once - `dnd.start_in_dnd`, the `[history]` block and the
`[log]` block - carry an "applies on restart" tag.

You can edit the common knobs without opening the file: `agentbox app --tab
settings` shows them with the right control per type. Save writes only the keys
you changed, in place - it preserves your comments, formatting and any keys it
does not manage, and never writes out defaults. The surface does not manage
every key below; placement, per-agent overrides and the unwired knobs stay
file-only.

`AGENTBOX_THEME=dark|light` and `AGENTBOX_FONT_SCALE=0.5..3` override `theme.mode` and
`font.size_pt` for a quick experiment (env beats config; the scale is relative to
12 pt).

## The file, annotated with defaults and why

Since M10 the window shapes and the drop-down panel are knobs too, live like
the rest; even the panel's hotkey is re-grabbed on the spot.

```toml
[window]                     # every AgentBox window's shape; all live
card_width = 470
card_max_height = 760        # past this a card body scrolls inside the card
toast_width = 430
toast_max_height = 330
toast_top_inset = 48         # how far below the top edge a toast sits
app_width = 1180
app_height = 860
viewer_width = 900
viewer_height = 780
progress_width = 400
progress_max_height = 620
measure_px = 700             # the reading column: prose caps here, however wide
                             # the window is

[panel]                      # the drop-down session panel (M10)
hotkey = "Ctrl+Alt+grave"    # grabbed by the daemon on X11, so no desktop setup
                             # is needed; "" = no grab, use `agentbox panel`
width_frac = 0.74            # of the monitor it rolls down on, not of the X root
height_frac = 0.62
slide_ms = 0                 # 0 (the default) = the panel simply appears. Set a
                             # duration (150-300) to animate the roll: it runs on
                             # the clock, so a slow frame is dropped rather than
                             # stretched, but it is a window resize per frame and
                             # it does not read like a game console. theme.motion
                             # = "none" forces 0 whatever this says
measure_px = 980             # the panel is wider than the app window, so its
                             # reading column can be too

[control]                    # the hands-off strip (FR74) and its pause (FR94)
pause_hotkey = "Super+Escape" # takes the keyboard and mouse back mid-run, and
                             # the same key hands them on again. Grabbed by the
                             # daemon on X11; "" = no grab, use
                             # `agentbox control pause`. Resuming is the
                             # human's alone - no MCP tool can do it, or the
                             # pause would be a suggestion

[session]                    # the Claude child a session spawns (FR49)
default_mode = "plan"        # plan (read-only) | full; the prompting modes are
                             # not offered until AgentBox handles their protocol
binary = ""                  # empty = `claude` on the daemon's PATH
dir = ""                     # empty = the daemon's own working directory
```


```toml
# ~/.config/agentbox/config.toml
# Every value below is the default; an empty file behaves identically.

[sound]
enabled = true
volume = 0.4               # quiet: a chime should register, never startle
quiet_hours = ""           # e.g. "22:30-08:00"; empty = off

[sound.earcons]            # per-class override: file path or "none"
info = "builtin:pop"
success = "builtin:tick"
warning = "builtin:two-tone"
question = "builtin:chime"
error = "builtin:thud"
urgent = "builtin:insist"

[escalation]
interval_s = 60            # replay cadence for unanswered items
count = 5                  # then go silent, stay visible
urgent_interval_s = 20     # urgent insists harder

[card]
position = "center"        # center | top-left | top-right | bottom-left | bottom-right
monitor = "pointer"        # pointer | primary; pointer = where you look
                           # (width lives in [window] card_width, which is wired)
defer_minutes = 5          # Esc requeues for this long
focus = "never"            # never | grab; never = NFR5, a stolen keystroke
                           # answering a question is the worst failure
max_body_lines = 12        # longer bodies scroll inside the card

[toast]
position = "top-center"    # severity icon + tint; title never clipped
duration_s = 6             # info/success auto-dismiss; warning/error stick
max_stack = 3              # more collapse into a "+N more" collector

[ask]
default_timeout_s = 0      # 0 = wait forever unless the caller sets one
answer_on_summon = true    # summon focuses the card ready to answer
allow_reply = true         # "/" free-text escape on choice/confirm (FR27);
                           # callers can still force --strict per item
undo_grace_s = 3           # answered-strip window before delivery; 0 = off,
                           # clamped to 5 max (a long window holds answers
                           # hostage); the strip shows "sending in Ns"

[veto]
default_window_s = 15      # act-unless-stopped countdown when caller omits it

[presence]
hold_when_idle = true      # idle desktop: hold chimes, pause escalation
idle_after_s = 120
fullscreen_auto_dnd = true # nothing pops over presentations/screen shares
respect_desktop_dnd = true # GNOME's own DND switch counts as DND

[flood]
max_pending_per_agent = 10 # beyond this, items merge into one stack card
max_per_minute = 20        # token bucket per agent identity

[actions]
enabled = true             # caller-supplied action buttons (FR32);
                           # false hides them everywhere

[artifact]                 # agent-authored interactive HTML (M10, ADR-0010)
enabled = true             # the trust switch, live and retroactive: false removes
                           # a frame that is running right now and leaves its
                           # source, and nothing runs until it is true again
max_height_px = 640        # how tall one may grow inside a conversation; in a
                           # window of its own it fills the window instead

[speech]                   # reads an agent's spoken line out loud (M11)
enabled = false            # off until there is a voice to use; see below
command = []               # the engine argv. Empty means "find one": AgentBox looks
                           # for piper on PATH and the best voice it can find in
                           # ~/.local/share/piper-voices, ~/piper-voices and
                           # /usr/share/piper-voices, preferring English and the
                           # highest quality tier (high > medium > low > x_low).
                           # Set it to choose the voice and its prosody yourself:
                           #   command = ["piper", "--model",
                           #              "/home/you/piper-voices/en_US-lessac-high.onnx",
                           #              "--output-raw", "--length-scale", "1.1"]
                           # The contract is one line of text on stdin, raw
                           # s16le PCM on stdout - any engine that does that works
rate = 0                   # 0 = read it from the voice's own .onnx.json, which is
                           # what you want; a wrong rate is a chipmunk, not a
                           # subtle degradation
channels = 1               # every piper voice is mono
volume = 0.7               # 0-1, separate from [sound] volume: a sentence needs
                           # more level than a chime to stay intelligible
max_chars = 240            # a spoken line is a sentence or two; longer is cut on
                           # a word boundary
idle_timeout_s = 600       # release the engine after this long with nothing to
                           # say. A voice model is ~100MB resident, and the first
                           # line after a release pays the load again (~2.5s)
prewarm = false            # true loads the model at daemon start, so the first
                           # notification of the day is instant instead of late

[sync]                     # several agents taking turns, and one board to watch
                           # them on (FR83). Nothing here turns the roster on or
                           # off: presence and the Agents surface are not knobs
wait_max_s = 1500          # the ceiling on a PARKED tool call - a lock wait or
                           # a signal wait. It exists because the
                           # MCP client aborts a call it has heard nothing about
                           # for 1800s (measured, FR88), so a wait that promised
                           # more would be a lie the transport eventually tells.
                           # Hitting it returns a resumable timeout instead. A
                           # CLI hold is bounded by whatever runs the CLI, which
                           # is a different number entirely (120s from an agent's
                           # shell): the two must not be read as one
wait_warn_s = 600          # toast when a LOCK wait passes this; 0 disables. A
                           # signal wait never warns, because listening is the
                           # intended steady state and warning on it would train
                           # you to ignore the toast that matters
holder_gone_grace_s = 5    # a hold whose session died goes orphaned, not free.
                           # This is how long before its recorded pid being gone
                           # counts as proof the work is over. Short, because the
                           # pid check is the real evidence and this only keeps
                           # the probe from racing a process that is exiting
signal_keep = 1000         # how many signals each topic keeps, and:
signal_keep_days = 7       # how old any of them may get. Whichever bites first.
                           # Per topic rather than a global count so one chatty
                           # topic cannot evict another topic's only signal.
                           # Neither has an "off": what retention took is recorded
                           # per topic, so an agent whose cursor falls off the edge
                           # is TOLD (gap: true, plus the sequence a whole read
                           # starts from) rather than served a batch with a hole in
                           # it - which means finite retention costs honesty and
                           # not correctness, while unbounded growth would be a
                           # leak with no ceiling
shared_max_bytes = 16384   # the cap on ONE shared value - a claim, a counter, a
                           # pointer. Small by contract: the idiom for anything
                           # bigger is a file path. A knob where the signal
                           # payload cap is a constant, because a value is state
                           # a workflow shapes while a signal is an event agentbox
                           # shapes. There is no retention knob beside it on
                           # purpose: signals are history and may be forgotten, a
                           # claim is not, and trimming a claim table would hand
                           # one chunk of work to two agents. Values leave when an
                           # agent deletes them, and the ceiling on how many may
                           # exist refuses a new key rather than evicting somebody's

[markdown]
code_theme = "auto"        # auto | nord | gruvbox | github | onedark | dracula; auto follows the ground
chart_palette = "theme"    # chart colors come from the active theme

[viewer]
                           # the reading measure and the window size are wired, in
                           # [window] measure_px / viewer_width / viewer_height
watch_default = false      # `agentbox show --watch` opts in per call

[editor]                   # the open button on a cited code block (FR65)
command = []               # an argv TEMPLATE, not a command line, so a path with
                           # a space in it stays one argument. Placeholders are
                           # {dir} (the review's repo root), {file} (absolute),
                           # {line} and {column}, substituted inside a word as
                           # well as as a whole one:
                           #   ["goland", "{dir}", "--line", "{line}", "--column", "{column}", "{file}"]
                           #   ["code", "--goto", "{file}:{line}:{column}"]
                           #   ["kitty", "-e", "nvim", "+{line}", "{file}"]
                           # The order is per editor and unforgiving - GoLand
                           # rejects --line before the project directory - so an
                           # empty value does NOT guess an order: it looks for a
                           # launcher whose shape agentbox knows (the JetBrains
                           # family first, because with the project already open
                           # theirs routes to that window instead of a second
                           # one), then falls back to xdg-open, which opens the
                           # file and loses the line. $EDITOR is deliberately not
                           # consulted: it is usually a terminal editor and the
                           # daemon has no terminal to give it, so honouring it
                           # would be a click that silently does nothing. Name
                           # the terminal here instead, as in the third example

[theme]
mode = "auto"              # auto | dark | light; auto follows the desktop live
ground = "graphite"        # graphite | ink | slate; hand-tuned dark/light pairs
contrast = "normal"        # normal | high; lifts the muted inks and the hairlines
accent = ""                # empty = the theme's own; or any hex. Focus rings,
                           # links, primary action. Severity and identity hues
                           # are semantics, not decoration, and are not knobs.
density = "comfortable"    # comfortable | compact; padding and gaps
radius = 10                # corner radius in px, 0-24
motion = "full"            # full | reduced | none

[font]
size_pt = 12               # 6-36; every other size is relative to it
family = ""                # interface chrome; empty = Cantarell then the system
reading = ""               # agent prose; empty = the bundled serif
mono = ""                  # code, keycaps, numerals

[dnd]
start_in_dnd = false
urgent_breaks_through = true

[history]
retention_days = 30        # eviction age for items below keep_level
keep_level = "warning"     # this level and above is the audit trail: kept forever

[log]
level = "info"             # debug adds IPC payloads (secrets always redacted)
retention_mb = 50          # size-rotated JSONL

# Per-agent overrides match on the identity's agent field.
# [agents."claude-code"]
# volume = 0.6
# color = "#7aa2f7"        # pin the identity hue instead of the hash
# mute = false             # true = straight to inbox, no card, no sound
```

Note: the runtime mute (FR47, `agentbox mute <agent>`) is implemented; the
config `mute` above (FR17, file-persistent) and the per-agent `earcon`
override (FR46) are not. FR46 was dropped as redundant - agents are told
apart by the identity pill's hue and the waiting dots, and `agentbox mute` gives
a direct lever to silence one. The `[sound.earcons]` per-class block is also
unimplemented; the built-in earcons are used. All of `[presence]` is now wired
(FR29): `idle_after_s` drives both the missed-while-away marker (FR44) and the
idle chime hold; `hold_when_idle` holds chimes and pauses escalation while the
desktop is idle, then plays one summary chime on return; `fullscreen_auto_dnd`
treats a focused fullscreen app as DND; `respect_desktop_dnd` treats the
desktop's own do-not-disturb as DND. The two auto-DND knobs follow the same
break-through rule as the manual toggle, so urgent still pierces unless
`dnd.urgent_breaks_through` is off. Fullscreen and desktop DND are read via X11
and GNOME gsettings respectively; on a non-X11 / non-GNOME session they read as
"present" (no false suppression), and a Wayland client's fullscreen state is
not visible to AgentBox until M7.

## How the defaults were chosen

- volume 0.4 / quiet hours off: the failure mode of "too loud" is worse than
  "too quiet" because escalation replays; the first chime does not have to
  carry everything.
- focus "never": see vision principle 3. The grab option exists because some
  confirms are genuinely modal, but it is opt-in per caller, never ambient.
- card centered, toasts top-center, monitor pointer: a decision card sits
  where the eyes rest; a glanceable toast hangs at the top edge out of the
  work area; both on the monitor you are actually using.
- font size 12 pt, system family: matches the desktop; one knob scales the
  whole UI because text size is ergonomics, not decoration.
- defer 5 min: long enough to finish a thought, short enough that the agent
  is not abandoned.
- escalation 60 s x 5: persistent for five minutes, then visible but silent;
  an ignored item should not become a metronome.
- ask timeout 0: a blocking question with a silent default expiring is how
  wrong things ship; callers opt into timeouts explicitly.
- retention 30 days below warning, forever at and above: routine info
  traffic is noise after a month; warnings, errors and everything you
  explicitly approved are the audit trail and stay.
- undo grace 3 s: long enough to catch a misclick, short enough that the
  agent never notices. Forgiving answers are what make fast answering safe.
- idle hold on, 120 s: a chime into an empty room spends the sound budget
  for nothing; the summary chime on return carries the same information.
- flood limits 10 pending / 20 per minute: far above any sane agent, low
  enough that a looping one cannot turn the desktop into a slot machine.
- artifacts on, 640 px tall: an artifact is the M10 feature, so shipping it off
  by default would be shipping it in a drawer. The switch exists because it is
  agent-authored code and a person is entitled to say no to that on a given day,
  and it is retroactive so saying no is not a promise about the future only. The
  height is about a screenful of a conversation: enough for a real control panel,
  not enough for an artifact to push the transcript out of view.

- speech off by default, and never reading a title aloud: a voice is the most
  intrusive thing AgentBox can do, so it takes two deliberate acts - turning it on,
  and an agent writing a line worth hearing. Reading titles automatically was the
  alternative and it is worse: it makes every existing `agentbox notify` in a script
  start talking, and a title is written to be read at a glance, not heard.
- speech volume 0.7 against sound's 0.4: an earcon only has to be noticed, a
  sentence has to be understood, and the same level makes one of the two wrong.
- the engine held open, and released after ten minutes: measured with piper and
  en_US-lessac-high, loading the voice costs ~2.5s and a sentence costs ~70ms, so
  spawning per notification would make every one arrive three seconds after the
  thing it is about. Ten minutes is the other half of that bargain: an idle daemon
  should not sit on a hundred megabytes of voice model all night.
- the highest quality available, always, with no knob for it: AgentBox picks the best
  voice tier installed and asks pw-play for its maximum resampler quality (15 of
  15, against a default of 4). A piper voice is 22.05 kHz and a modern sink runs at
  48 kHz, so every utterance is resampled; the default is tuned for a stream that
  plays for an hour, and AgentBox's streams are one sentence long.

## Out of scope for the file

Keyboard remapping (fixed map in v1), per-project rules (per-agent covers
the real cases), theme editing beyond mode/accent. Each gets a knob only
when a real irritation shows up, not preemptively.
