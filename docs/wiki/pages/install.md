# From nothing to a card on your screen

> **In short.** AgentBox builds from source: one target installs the system
> packages, one puts the binary in `~/.local/bin` with a systemd user unit, and
> one line registers it with your coding agent. From a clean machine to a card on
> your screen is four commands.
>
> **Read on if** you are putting it on a machine today. **Skip to**
> [[Safe on a work machine?|is-it-safe]], or [[Limits and non-goals|limits]].

Everything below runs in the root of a checkout. There is no package and no
release archive to download: the binary carries no version number, because no
tags exist and nothing is stamped into it. What identifies a build is the
toolchain's own record of the commit it came from, which `agentbox status` reports
back to you.

## Two shared libraries, an X display, and sound it can do without

| What | Why it is needed | If it is missing |
|---|---|---|
| GTK4 and WebKitGTK 6.0 | every surface is a webview, so these are the UI | nothing renders, and `make doctor` prints MISSING |
| X11 | placing a card, the global hotkeys, driving the desktop, the summon key | the parts that need a display say they cannot rather than pretending |
| Go, the version in `go.mod` | building | `make bootstrap` says which version and stops. It never installs Go for you |
| `pw-play`, `paplay` or `aplay` | playing an earcon | sound switches itself off. Cards, speech and everything else are unaffected |
| `piper` and one voice | the spoken line an agent attaches to a card | nothing is read out. AgentBox finds the voice unaided once it is there |
| `npm` | rebuilding the web UI from source | nothing. `frontend/dist` is committed so a machine without npm still builds |

<sub>The first three rows are hard requirements. The last three degrade, and each one
degrades to exactly the feature it belongs to and nothing else.</sub>

> [!NOTE]
> `make bootstrap` knows apt, dnf and pacman. On anything else it prints the four
> things it needs (a C compiler, `pkg-config`, the GTK4 and WebKitGTK 6.0
> development headers, and node with npm) and stops, because a Makefile cannot
> guess your distribution and neither can a wiki. `make doctor` is the honest
> check either way: it reports every dependency as present or MISSING and installs
> nothing at all.

## Four commands, and the two the Makefile will not run for you

```sh
make bootstrap    # system packages, a speech engine with a voice, and a config file
make install      # builds, then binary + desktop launcher + systemd user unit
systemctl --user enable --now agentbox.service
claude mcp add --scope user agentbox agentbox mcp
```

`make bootstrap` is the target that installs system packages, by way of `make deps`
underneath it, and every privileged command goes through a `sudo` you can read in
the recipe rather than being arranged quietly. It ends by running `make doctor` and
printing what to do next.

`make install` builds first, then writes three things: `~/.local/bin/agentbox`, a
`.desktop` launcher with an icon, and the unit file at
`~/.config/systemd/user/agentbox.service`.

The last two lines are yours because both of them change something outside the
repository. No Makefile target ever runs `systemctl --user enable`, and no
Makefile target registers the MCP server. A build tool that switched on a service
and edited your agent's configuration would be doing two things you did not ask
for while you were reading its output.

The service is a convenience rather than a requirement. Any CLI or MCP call
spawns the daemon if it is not running, so the unit exists to have it up from the
moment the desktop session starts.

## One line to hand the tools to your coding agent

```sh
claude mcp add --scope user agentbox agentbox mcp
```

User scope is the point of that line: the tools then exist in every project and
every session rather than in the one repository you happened to be in. The child
process it registers is `agentbox mcp`, a stdio server that holds no state of its
own and forwards to the same daemon over the same socket every other caller uses.
Any other MCP client works the same way, with `agentbox mcp` as the command and
stdio as the transport.

What appears on the other side is thirty-nine tools. If your agent's tool list
has them, it can already reach you: see [[the whole tool list|what-agents-can-do]].

## You are done when these four checks pass

- [ ] `make doctor` shows `gtk4 + webkitgtk` as present and names your display
- [ ] `agentbox status` answers with `daemon running, 0 pending` and a build line
- [ ] your coding agent lists the AgentBox tools
- [ ] a card appears when you run the command in the last section, and your summon
      key focuses it

```sh
make doctor       # every dependency, present or MISSING, and nothing installed
agentbox status   # what the running daemon is, not what the file on disk is
```

<!-- SHOT: the terminal output of make doctor on a finished install, then agentbox
status under it. Every row present, the X display named, the installed binary showing
a build, the daemon answering, the unit enabled. One row must honestly read
"missing (only needed for showcase screenshots)" so the frame is a real machine
rather than a staged one. Terminal only, no desktop, and no private paths in the
prompt. -->

Those two are different questions, and the second one matters after an upgrade. A
replaced binary is not a running binary: the daemon keeps serving the code it
started with until it restarts. `agentbox status` asks the daemon what build it is
and says so out loud when the client on disk is a newer one.

## The summon key is the one binding you make yourself

AgentBox grabs three hotkeys for itself. <kbd>Ctrl</kbd> + <kbd>Alt</kbd> +
<kbd>grave</kbd>, the backtick left of the 1, rolls the drop-down panel down or up.
<kbd>Ctrl</kbd> + <kbd>Alt</kbd> + <kbd>Esc</kbd> pauses or resumes a desktop
handover. <kbd>Ctrl</kbd> + <kbd>Alt</kbd> + <kbd>Q</kbd> toggles recording mode. All
three are rebindable in the config, and an empty string disables the grab entirely.

The summon key is not one of them. It is a shortcut you create in your desktop's
own keyboard settings, pointing at:

```sh
agentbox summon
```

<kbd>Ctrl</kbd> + <kbd>Alt</kbd> + <kbd>K</kbd> is the suggestion, and nothing
enforces it.

Do it before you need it. This is the binding that makes a card answerable at all: a
card is drawn above every window and never takes the keyboard, so until something
hands it focus, your single keystrokes go on reaching whatever you were typing in.
`agentbox summon` is that something, and what it sends is one `_NET_ACTIVE_WINDOW`
message to the card, which is also why it needs X11.

## The first card, from your own shell

```sh
agentbox notify --level success --title "AgentBox is installed" \
    --body "This is what an agent's message looks like on your screen."
```

That is a toast at the top of the screen, and it takes itself off again. For the
real thing, a question that blocks until you answer it:

```sh
agentbox ask --title "Deploy 2026.7.30?" \
    --option "Now" --option "Skip::wait for the next train" \
    --timeout 300 --default "Skip"
```

Press your summon key, then <kbd>1</kbd> or <kbd>2</kbd>. The command prints the
option you chose and exits, which is the whole contract an agent gets too. If you
answered and then changed your mind inside three seconds, <kbd>u</kbd> would have
taken it back.

**Next:** [[the card you just put on your own screen|the-card]], or
[[every knob, and why its default is what it is|settings]].
