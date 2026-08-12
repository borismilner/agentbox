# From nothing to a card on your screen

> **In short.** On x86-64 Linux, download the release and run its `install.sh`.
> Anywhere else it builds from source: one target installs the system packages,
> one puts the binary in `~/.local/bin` with a systemd user unit, and one line
> registers it with your coding agent. From a clean machine to a card on your
> screen is four commands either way.
>
> **Read on if** you are putting it on a machine today. **Skip to**
> [[Safe on a work machine?|is-it-safe]], or [[Limits and non-goals|limits]].

## Download

On x86-64 Linux:

```sh
curl -fsSLO https://github.com/borismilner/agentbox/releases/latest/download/agentbox-linux-amd64.tar.gz
tar xzf agentbox-linux-amd64.tar.gz && cd agentbox-* && ./install.sh
claude mcp add --scope user agentbox -- ~/.local/bin/agentbox mcp
```

There is a **Windows** build too, `agentbox-windows-amd64.zip`, and it is marked
experimental for a reason worth reading before you rely on it. Every published
build is started on a Windows runner first - the daemon comes up, `status`
answers, an item is taken and it stops cleanly - so it is not a guess. But no
person has used it for real work, the tray icon does not load, and
`agentbox secret` cannot apply `0600` to the file it writes, so a secret written
on Windows should be treated as readable by any account on the machine.
`WINDOWS.md` in the zip is the full account. On **macOS**, build from source:
every package compiles there, but the binary does not link, because `systray` and
Wails both define an Objective-C class called `MenuItem`.

That URL always resolves to the newest release, on
[GitHub](https://github.com/borismilner/agentbox/releases/latest) or
[GitLab](https://gitlab.com/fu-bar/agentbox/-/releases). Only the newest one is
kept, so there is exactly one download and it is the current one. A release is
cut deliberately - a commit landing on `main` does not produce one - so the
download lags the tip of the tree on purpose.

The archive is one binary plus the desktop entry, the icon and the systemd unit,
which are not in the binary and are what a launcher and autostart need.
`install.sh` puts all four under `$HOME` and starts nothing: the first CLI call
spawns the daemon anyway.

It also checks for the two shared libraries in the table below and installs them
through your package manager (apt, dnf, pacman or zypper), which is the one part
that needs root - so it is the one part that asks, showing the exact command
first. `--yes` answers it in advance, `--no-deps` leaves system packages alone,
and with no terminal to ask it declines rather than hanging.

## Or build it

Required on macOS, on Windows, and on any Linux whose GTK4 and WebKitGTK differ
from the ones the release was built against (Ubuntu 24.04). Everything below runs
in the root of a checkout.

A build carries no version number of its own: what identifies it is the
toolchain's record of the commit it came from, which `agentbox status` reports
back to you. A downloaded release is that same stamp plus the tag it was cut at.

## Two shared libraries, a display server it can do without, and sound too

| What | Why it is needed | If it is missing |
|---|---|---|
| GTK4 and WebKitGTK 6.0 | every surface is a webview, so these are the UI | nothing renders, and `make doctor` prints MISSING |
| X11 | placing a card exactly, appearing above without taking focus, the global hotkeys, driving the desktop, the summon key | the window manager places the card instead and it takes focus; the hotkeys and driving say they cannot rather than pretending |
| Go, the version in `go.mod` | building | `make bootstrap` says which version and stops. It never installs Go for you |
| an audio player (`pw-play`, `paplay`, `aplay`, `afplay`, PowerShell) | playing an earcon | sound switches itself off. Cards, speech and everything else are unaffected |
| `piper` and one voice | the spoken line an agent attaches to a card | nothing is read out. AgentBox finds the voice unaided once it is there |
| `npm` | rebuilding the web UI from source | nothing. `frontend/dist` is committed so a machine without npm still builds |

<sub>GTK4/WebKitGTK and Go are the hard requirements on Linux (macOS uses the system
WebKit, Windows uses WebView2, and neither needs installing). Every other row
degrades, and each one degrades to exactly the feature it belongs to and nothing
else - X11's row included, which is the change that made
[[the limits page|limits]] shorter.</sub>

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

![make doctor on a finished install: thirteen rows naming go 1.26.1, npm, gtk4 plus webkitgtk present, the X display as :0, the speech engine and voice by path, the installed binary with its build hash and time, the daemon answering "daemon running, 0 pending", and the systemd unit enabled. Under it, agentbox status repeating the daemon's build](img/install-doctor.png)

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
message to the card, which is also why it needs X11. On a desktop without it the
binding is unnecessary rather than missing: the card takes focus when it appears,
so it is already answerable.

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
