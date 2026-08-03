# Platform notes

Facts verified 2026-06-12 against the live environment (GNOME Shell 46.0,
X11 session, PulseAudio protocol 35 via pipewire-pulse, Go 1.26.1) and web
research; sources in ADR-0002/0006.

## Pop-to-front

X11 (current target, Mutter):

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

Wayland (later, GNOME):

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
- Open question for the Wayland milestone (M7, deferred): "visible on top
  without focus" has no guaranteed
  Wayland equivalent of user-time-0. Validate on a Wayland session; worst
  case the card takes focus on map and we mitigate with a short input grace
  period (ignore keys for ~500 ms after map).

## System tray

- Stock GNOME has no StatusNotifierItem host in any release through GNOME 49.
  Ubuntu preinstalls the appindicator extension; Fedora does not. The tray
  can vanish at any shell upgrade.
- Therefore: tray is progressive enhancement (FR14). Every function must be
  reachable via CLI and the inbox window.
- Library: fyne.io/systray (v1.12.x, June 2026) - on Linux it speaks SNI
  directly over D-Bus (godbus, no GTK, no cgo on this path) and is
  toolkit-agnostic, so it pairs fine with the webview UI. getlantern/systray is
  unmaintained (last push July 2024).

## Sound

- Host runs PipeWire with pipewire-pulse. Playback: spawn
  `pw-play`, fall back to `paplay`, then `aplay` (ADR-0006). Spawn cost
  50-150 ms is fine for chimes.
- Earcons are embedded (go:embed) and written once per boot to
  `$XDG_RUNTIME_DIR/agentbox/sounds/` so the player gets file paths.

## Presence signals (FR29)

- Idle: `org.gnome.Mutter.IdleMonitor` over D-Bus (works on X11 and
  Wayland); fallback for non-GNOME later: org.freedesktop.ScreenSaver.
- Fullscreen: on X11, check `_NET_WM_STATE_FULLSCREEN` on the active
  window. No equivalent signal on Wayland for other clients' windows; an
  open question for the Wayland milestone, M7 (the knob works on X11 only
  today).
- Desktop DND: `org.gnome.desktop.notifications show-banners` via
  gsettings/portal, watched live.

## Autostart and lifetime

- Auto-spawn from the first client call (ADR-0003) means autostart is a
  convenience, not a requirement.
- Offer both: an XDG autostart .desktop entry and a systemd user unit
  (`agentbox.service`); document, do not install silently.

## Paths

| What | Where |
|------|-------|
| socket | `$XDG_RUNTIME_DIR/agentbox/agentbox.sock` (dir 0700) |
| extracted sounds | `$XDG_RUNTIME_DIR/agentbox/sounds/` |
| config | `~/.config/agentbox/config.toml` |
| history db | `$XDG_STATE_HOME/agentbox/agentbox.db` |
| logs | `$XDG_STATE_HOME/agentbox/log/` (slog, rotated by size) |

## Build and runtime footprint

- The webview UI (ADR-0009, superseding the Gio line here) needs `libgtk-4-dev`
  and `libwebkitgtk-6.0-dev` at build time (cgo) and links GTK4 + WebKitGTK at
  runtime, so it is one executable but no longer self-contained: the desktop has
  to have WebKitGTK. The Svelte bundle IS bundled - built to `frontend/dist`,
  committed, and embedded with `go:embed`, so a machine without npm still builds.
- sqlite (modernc) and the sound path stay cgo-free; the toolkit and the X11
  window plumbing are the only cgo surfaces.
- Dark/light: read `color-scheme` via the XDG settings portal (D-Bus), which
  works identically on X11 and Wayland; subscribe for live changes.
