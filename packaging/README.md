# Packaging

Desktop integration for AgentBox: a `.desktop` launcher, an application icon, and
a systemd `--user` service for autostart. Everything installs under `$HOME`
(no root), via the Makefile targets.

## Install

```
make install          # build + install binary, .desktop, icon, and the service unit
systemctl --user enable --now agentbox.service   # start now and on every login
```

`make install` runs three steps you can also run on their own:

- `make install-bin` - build and copy the binary to `~/.local/bin/agentbox`.
- `make install-desktop` - `agentbox.desktop` to `~/.local/share/applications`
  and `app-256.png` to `~/.local/share/icons/hicolor/256x256/apps/agentbox.png`,
  then refresh the desktop and icon caches.
- `make install-service` - `agentbox.service` to `~/.config/systemd/user` and
  `systemctl --user daemon-reload`.

## Autostart

The daemon auto-spawns on the first CLI/MCP call, so a service is optional. The
unit makes it start with the graphical session and restart on a crash:

```
systemctl --user enable --now agentbox.service
systemctl --user status agentbox.service
journalctl --user -u agentbox.service -f      # systemd's view; agentbox logs is richer
```

The unit is bound to `graphical-session.target`, so it has `DISPLAY` /
`WAYLAND_DISPLAY` and the session bus. If your login manager does not import
the graphical environment into the systemd user manager, add to your session
startup: `systemctl --user import-environment DISPLAY WAYLAND_DISPLAY`.

## Uninstall

```
make uninstall        # disables the service and removes binary, desktop, icon, unit
```

## Notes

- The launcher opens the inbox (`agentbox inbox`); AgentBox is primarily a daemon +
  CLI, so the menu entry is a convenience, not the main entry point.
- Wayland is not yet validated (roadmap M7); on Wayland the X11-only signals
  (idle, fullscreen auto-DND, card placement) degrade safely. X11 is the
  supported session today.
- The icon is generated from `tools/genicon` (committed PNGs); regenerate with
  `go run ./tools/genicon internal/tray/icons`.
