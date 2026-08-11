# Platform notes

AgentBox runs on Linux, macOS and Windows from one source tree. This file says
what each desktop gets, what it does not, and which of those differences you would
ever notice.

Facts verified 2026-08-11 on the machine AgentBox is developed on (GNOME Shell
46.0, X11 session, PulseAudio protocol 35 via pipewire-pulse, Go 1.26.1) and by
cross-compiling the other two from it. Earlier facts and their sources are in
ADR-0002/0006; the portability decision is [ADR-0013](decisions/ADR-0013-portable-by-default.md).

## What crosses, and what is an enhancement

The rule, from NFR10: **nothing that carries a message depends on a display
server.** Everything below that line is portable; everything above it makes the
same message land better on the desktop that can do it.

| Layer | Linux/X11 | Linux/Wayland | macOS | Windows |
|---|---|---|---|---|
| daemon, store, socket, protocol | yes | yes | yes | yes |
| CLI and MCP server, all tools | yes | yes | yes | yes |
| every surface appears and is answerable | yes | yes | yes | yes |
| earcons | pw-play/paplay/aplay | same | afplay | Media.SoundPlayer |
| speech | piper + a PCM player | same | piper + sox | piper + sox |
| open a file at a line | xdg-open | same | `open` | `cmd /c start` |
| **placement**: exact centre, top-centre column, the rolled panel | yes | WM's choice | WM's choice | WM's choice |
| **pop above without taking focus** | yes | see below | no | no |
| **global hotkey** for the panel | yes | no, says so | no, says so | no, says so |
| **pointer and keyboard driving** | yes | no, says so | no, says so | no, says so |
| peer-UID check on the socket (NFR8) | SO_PEERCRED | SO_PEERCRED | LOCAL_PEERCRED | directory ACL only |

Two things to read off that table. First, the top four rows are the product: a
message reaching a human and coming back answered never depends on any of the
rest. Second, the rows in bold are X11 calls, and they were kept whole rather than
reduced to what all three platforms share - that is the "never trade Linux
quality" half of the old non-goal, still in force.

### How "no placement layer" behaves

`dialX11` returns nil when there is no X11 (macOS, Windows, a Wayland session with
no Xwayland, or a Linux build made with `-tags nox11`). Twenty call sites in
`internal/webui` read that nil and take a documented degrade: the toolkit shows
the window, the window manager places and stacks it.

Measured, both ways, on 2026-08-11 with a toast on a 2560x1440 monitor at x=1440:

- With X11: `x=2505, y=48` - centred to the pixel (2720 - 430/2) with the
  top-bar inset applied.
- With `-tags nox11`: `x=2505, y=695` - the WM centred it vertically instead. The
  window is there, it carries its content, it is answerable. It is not where we
  would have put it.

The panel logs `webui.panel_unprepared` once on that path and reports its state
truthfully, which is the fix from R-12: it used to record itself open on a roll
that mapped nothing, and a question routed to it reached a surface nobody could
see. That defect is the reason `make check` now runs the whole suite through this
layer - the branch existed and was reachable from twenty places and had never once
been executed.

## Pop-to-front

X11 (Mutter):

- A newly mapped window with a fresh `_NET_WM_USER_TIME` takes focus under
  GNOME's default focus mode. With a stale/zero timestamp, Mutter instead
  sets demands-attention and shows the "window is ready" notification.
- To pop a card above everything WITHOUT taking keyboard focus (vision
  principle 3): set `_NET_WM_USER_TIME = 0` before mapping, plus
  `_NET_WM_STATE_ABOVE`. The card is visible on top; keystrokes stay where
  they were.
- `agentbox summon` focuses the card via a `_NET_ACTIVE_WINDOW` ClientMessage
  with a fresh timestamp (this is what xdotool does; works on Mutter).
- Polite escalation: `XUrgencyHint` / `_NET_WM_STATE_DEMANDS_ATTENTION`.

Wayland (GNOME):

- xdg-activation-v1 is the only sanctioned focus mechanism, and a background
  daemon cannot mint a valid token (it requires a recent input serial from a
  focused client). Denied activation degrades to demands-attention.
- Loophole that currently works: newly mapped windows still take focus under
  GNOME's default mode. Consequence for us: create a fresh window per card
  (window-per-prompt), never unhide a long-lived one. This is why the
  architecture has no persistent card window.
- GNOME will keep tightening (KDE already did in 2025). The sanctioned
  re-focus flow is a desktop notification whose action click hands us an
  activation token via the portal; plan that as the Wayland escalation path.
- "Visible on top without focus" has no guaranteed Wayland equivalent of
  user-time-0. Worst case the card takes focus on map, mitigated with a short
  input grace period (ignore keys for ~500 ms after map).

macOS and Windows: the toolkit's own always-on-top is what there is. A card
appears above and takes focus. That is a real difference from principle 3 and the
grace-period mitigation above is the same answer.

## System tray

- Stock GNOME has no StatusNotifierItem host in any release through GNOME 49.
  Ubuntu preinstalls the appindicator extension; Fedora does not. The tray
  can vanish at any shell upgrade.
- Therefore: tray is progressive enhancement (FR14). Every function must be
  reachable via CLI and the inbox window - which is also what makes the tray's
  absence on any platform a non-event.
- Library: fyne.io/systray (v1.12.x, June 2026) - on Linux it speaks SNI
  directly over D-Bus (godbus, no GTK, no cgo on this path) and is
  toolkit-agnostic, so it pairs fine with the webview UI. It has native macOS and
  Windows backends, both of which need cgo. getlantern/systray is unmaintained
  (last push July 2024).

## Sound

- Playback is a spawned player (ADR-0006); spawn cost 50-150 ms is fine for
  chimes. One list in preference order, resolved by `LookPath`, so the order does
  the platform selection and there is no build tag: `pw-play`, `paplay`, `aplay`,
  `afplay`, `powershell`. This machine still lands on pw-play, unchanged.
- `aplay` and PowerShell have no volume control, so the knob does nothing on
  those two and the earcon plays at asset level. Both are last resorts on their
  platform.
- Earcons are embedded (go:embed) and written once per boot to
  `$XDG_RUNTIME_DIR/agentbox/sounds/` so the player gets file paths.

## Speech

The one feature that is genuinely thinner off Linux. The pipeline is an engine
(piper) writing raw PCM into a player's stdin, which is what makes it 70 ms per
sentence instead of 2.5 s. Of the players above, only sox's `play` both exists off
Linux and reads raw PCM from a pipe - macOS's `afplay` cannot, which is why it is
a good earcon player and no use here.

So: speech works on macOS and Windows with piper and sox installed, and is silent
without them - the same fail-soft it already had on a Linux box with no piper.
Native `say` and Windows SAPI would be better and are a separate feature, because
they replace the engine contract rather than the player.

## Presence signals (FR29)

- Idle: `org.gnome.Mutter.IdleMonitor` over D-Bus (works on X11 and
  Wayland); fallback for non-GNOME later: org.freedesktop.ScreenSaver.
- Fullscreen: on X11, check `_NET_WM_STATE_FULLSCREEN` on the active
  window. There is no equivalent signal for other clients' windows on Wayland,
  macOS or Windows, so the knob is X11-only and reports "not fullscreen"
  elsewhere - which errs toward showing the human something.
- Desktop DND: `org.gnome.desktop.notifications show-banners` via
  gsettings/portal, watched live. Absent elsewhere, which reads as "no desktop
  DND", never as "suppress everything".

## Autostart and lifetime

- Auto-spawn from the first client call (ADR-0003) means autostart is a
  convenience, not a requirement - on every platform. `detach` is what makes it
  safe: `Setsid` on unix, `DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP` on
  Windows, so the daemon outlives whichever tool call happened to need it first.
- Linux: an XDG autostart `.desktop` entry and a systemd user unit
  (`agentbox.service`); document, do not install silently.
- macOS: a launchd `LaunchAgent` plist in `~/Library/LaunchAgents` is the
  equivalent. Not shipped yet; auto-spawn covers it.
- Windows: a shortcut in the per-user Startup folder, or a Task Scheduler entry
  at logon. Not shipped yet; auto-spawn covers it.

## Paths

One layout on every platform, and that is deliberate (ADR-0013): easier to
support, easier to document, and the one change in this area that could move
somebody's existing store. `os.UserHomeDir` is already correct on all three, and
the XDG variables override everywhere.

| What | Where |
|------|-------|
| socket | `$XDG_RUNTIME_DIR/agentbox/agentbox.sock` (dir 0700) |
| extracted sounds | `$XDG_RUNTIME_DIR/agentbox/sounds/` |
| config | `~/.config/agentbox/config.toml` |
| history db | `$XDG_STATE_HOME/agentbox/agentbox.db` |
| logs | `$XDG_STATE_HOME/agentbox/log/` (slog, rotated by size) |

`$XDG_RUNTIME_DIR` is unset on macOS and Windows, where it falls back to
`os.TempDir()/agentbox-<uid>`. That is per-user on both (`%TEMP%` already is), so
the 0700 guarantee survives the substitution.

## Build and runtime footprint

- The webview UI (ADR-0009, superseding the Gio line here) needs `libgtk-4-dev`
  and `libwebkitgtk-6.0-dev` at build time (cgo) and links GTK4 + WebKitGTK at
  runtime, so it is one executable but no longer self-contained: the desktop has
  to have WebKitGTK. On macOS the system WebKit is used and on Windows WebView2,
  both of which are present by default, so the footprint question is a Linux one.
  The Svelte bundle IS bundled - built to `frontend/dist`, committed, and embedded
  with `go:embed`, so a machine without npm still builds.
- sqlite (modernc) and the sound path stay cgo-free; the toolkit and the X11
  window plumbing are the only cgo surfaces. That is why `make cross` can compile
  the whole tree for Windows and everything-but-the-UI for macOS with one
  toolchain installed.
- Dark/light: read `color-scheme` via the XDG settings portal (D-Bus), which
  works identically on X11 and Wayland; subscribe for live changes. macOS and
  Windows have their own signal and are not wired to it yet, so a theme change
  there needs the mode set in config.

## Verifying a platform claim

Three commands, all of which `make check` runs:

```bash
make test-nox11   # the whole suite through the no-X11 placement layer
make cross        # windows/amd64 whole tree; darwin arm64+amd64 minus the native UI
make check        # both of the above, plus gofmt, vet, race tests and vitest
```

What none of them proves: that somebody has run the macOS build. The two packages
that link a native UI (`internal/webui`, `internal/tray`) need clang and a macOS
SDK, so they are verified by building on a Mac and nowhere else. The claim the
checks do make is precise - no platform-locked call remains outside a file named
for its platform, and the UI toolkit supports the target.
