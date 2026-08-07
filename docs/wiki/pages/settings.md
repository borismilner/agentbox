# Every default was chosen against a failure

> **In short.** Nearly nothing in AgentBox defaults to a taste. The settings
> surface puts 59 keys on seven pages with a live preview, and a save that names
> the exact lines it wrote into your `config.toml` rather than asking you to
> believe it left the rest alone.
>
> **Read on if** you are about to change something. **Skip to**
> [[What an agent can do|what-agents-can-do]], or [[Limits and non-goals|limits]].

The table below is not every key. It is the ones worth knowing about, each with the
reason its default is what it is, because a default with a reason is one you can
argue with.

## The dozen defaults you might actually change

| Knob | Default | Why that default |
|---|---|---|
| `sound.volume` | `0.4` | escalation replays a chime, so the first one does not have to carry everything. Too loud fails worse than too quiet |
| `ask.undo_grace_s` | `3` | long enough to catch a mis-click, short enough that the agent never notices. Clamped to 5 |
| `escalation.interval_s` and `count` | `60` and `5` | persistent for five minutes, then visible but silent. An ignored item should not become a metronome |
| `escalation.urgent_interval_s` | `20` | urgent replays three times as often as anything else, and that cadence is most of what makes it urgent |
| `toast.duration_s` | `6` | how long info and success stay up. A warning or an error gets no timer at all |
| `presence.idle_after_s` | `120` | a chime into an empty room spends the sound budget for nothing. One summary chime on your return carries the same information |
| `dnd.urgent_breaks_through` | `true` | urgent is the one level that pierces do not disturb. This is the switch that stops even that |
| `speech.enabled` | `false` | a voice is the most intrusive thing AgentBox can do, so it takes two deliberate acts: you turning it on, and an agent writing a line worth hearing |
| `speech.volume` | `0.7` | an earcon only has to be noticed, a sentence has to be understood, and one level for both makes one of them wrong |
| `artifact.enabled` | `true` | an artifact is agent-authored code, and you are entitled to say no to that on a given day. The switch is retroactive |
| `history.retention_days` | `30` | routine info traffic is noise after a month. Warnings, errors and anything you approved are the audit trail and stay |
| `font.size_pt` | `12` | one knob scales the whole interface, because text size is ergonomics rather than decoration |

<sub>The middle column is the shipped default, not a suggestion. All of these take
effect as you save them except `history.retention_days`, which is one of the five
that wait for a restart, named at the bottom of this page.</sub>

Flood control is the one whose number has a date on it. A burst of `3` inside `10`
seconds collapses into a single stack card, and those two numbers were picked on
2026-08-06 over a looser five in thirty: a retry loop trips the tight version almost
at once, which is the case it exists for, while two unrelated notices seconds apart
do not. It counts per session rather than per agent name, because every Claude
session on a machine calls itself `claude`, and a budget keyed on the name would let
the first looping agent silence its innocent neighbours.

## Seven pages, and a preview that cannot drift from the result

The surface lives in the app window: Appearance, Windows and panel, Sessions,
Sound, Interruptions, Presence and DND, and History and logs. Fifty-nine knobs, each
one a control typed to what it is: a switch, a number, a choice from a list, a
colour, a command line.

<!-- SHOT: the Appearance page with an unsaved change pending. Accent moved off the
default to Teal, so the preview's card, toast and code block are all already teal
while the panel around them is still wearing the saved theme. The footer must show
the pending key list before saving, and the identity pill in the preview must still
be its hashed hue rather than teal, because that is the paragraph three below. Crop
to the window, rail included. -->

Appearance is the page with the live preview, and the preview is the interesting
part of this whole page. It shows a card, a toast and a passage of agent prose with
a highlighted code block, all wearing the values you have changed but not yet saved.

What makes it worth trusting is where the colours come from. The pending values go
to Go, through the same theme resolver that dresses every real window, and the
tokens it returns are written by the same JavaScript that writes them onto a real
card. It is not a palette table copied into the settings page, which is the version
that drifts: two copies of the same colours disagree the first time one of them is
edited, and then the preview is a lie that renders correctly.

One thing in the preview deliberately ignores your accent. The identity pill keeps a
fixed hue, because an agent's colour is hashed from its name rather than configured,
and letting it follow the accent would advertise a knob that does not exist.

Previewing writes nothing. There is a test whose only job is to save the file's
bytes, preview a dozen changes and assert the bytes are unchanged.

## A save that names the lines it wrote

Editing somebody's hand-written config file in place is a thing a program has to
earn, so a save reports itself. It lists what it wrote, one line per key, in the
form `theme.accent = "#46b3a5"`, and the count in the footer says how many keys went
into which path.

Only differences are written, and the baseline is the file itself, re-read at the
moment you save rather than whatever the daemon is holding in memory. A value you
set back to what the file already said is not a change and does not appear in the
report.

Your comments survive, and so does every key you did not touch. The write patches
the existing line inside its section, or inserts the key under the section header,
or appends a new section at the end. A `# louder in the office` you once wrote
beside the volume is still there afterwards, and saving never materialises fifty
default keys you never set.

One bad value does not sink the rest either. A half-typed colour or an impossible
quiet-hours range is refused by name while the other keys write, and the file is
replaced atomically, so nothing reading it ever sees a torn version.

## Two colour knobs instead of twenty

There is one ground and one accent, and that is the whole palette you control.
Ground is a choice of three hand-tuned dark and light pairs; accent is a hex colour
with five presets and a picker. Everything else on screen is derived from those two:
the surfaces, the edges, the three levels of text, the padding and the gaps. Syntax
highlighting is the only other colour decision, and it is a named theme you pick
rather than a palette you assemble.

Two knobs that cannot produce an unreadable theme beat twenty that can.

The alternative was a colour per surface. Its failure mode is a card nobody can read
at 2am, with no obvious way back to a state that worked.

The colours that are not yours are not decoration. Severity hues are semantics: info
blue, success green, warning amber, error red, and urgent is error. An agent's
identity hue is a hash of its name and project across twelve stops, with the reds
left out on purpose, because red already means something on this screen.

## Behaviour with no knob at all

Some things have no key, and saying so is more honest than a table of settings that
do nothing. A card is always dead centre of the monitor your pointer is on. A card
never takes the keyboard, and there is no option to make one modal. Toasts are top
centre, one at a time, queued. <kbd>Esc</kbd> on a question requeues it for five
minutes and on a notification dismisses it. The six earcons are compiled into the
binary with no per-agent override, because `agentbox mute` is the direct lever when
one agent will not stop. Chart colours come from the active theme.

One more, and it is the one people ask about: a blocking call that names no timeout
waits forever. Callers opt into a timeout explicitly, because a question expiring
into a silent default is how wrong things ship.

> [!NOTE]
> An unknown key is a warning in the log at startup and then nothing at all, which
> means a knob that does not exist fails in silence rather than loudly. Older
> versions of the repo's own documentation invented several: a `[card]` section, a
> `[viewer]` section, `[sound.earcons]`, and a global free-text-reply switch that is
> parsed and read by nothing. If a setting seems to do nothing, check that the code
> reads it before you check anything else.

## Where the file is, and when a change takes hold

`~/.config/agentbox/config.toml`, or under `$XDG_CONFIG_HOME` if you set one. There
are 22 sections in it, and 77 keys the code reads. The settings surface is the
comfortable way in; the file is the complete way in, and hand-editing it is
supported rather than tolerated.

A change lands in about 400 milliseconds, from a poll on the file's timestamp. The
theme, the fonts, the reading measure, the window sizes and all three global hotkeys
rebind live, and open windows resize in place. An open card is the exception: it is
never resized under an answer somebody is reading, because moving the buttons out
from under a pointer is worse than an inconsistent window.

Five knobs need a restart, and the surface says so when you touch one:
`dnd.start_in_dnd`, `history.retention_days`, `history.keep_level`, `log.level` and
`log.retention_mb`. A number outside its range is clamped or refused rather than
honoured, so `panel.height_frac = 0.62` does not give you a taller panel. The clamp
is 0.2 to 0.5, and the value is reset with a warning.

**Next:** [[the ways to make it stay quiet|staying-quiet]], or
[[what an agent can reach|what-agents-can-do]].
