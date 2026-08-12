#!/bin/sh
# Install AgentBox from an extracted release tarball. Everything goes under $HOME,
# so this never needs root - and it never asks for it, which is the point: a script
# a stranger downloads should not be one that wants your password.
#
# This is the tarball's stand-in for `make install`. It does the same three things
# (binary, desktop entry, service unit) for somebody who has a download rather than
# a checkout, and it deliberately does not enable the service: starting a daemon is
# the user's decision, and the first CLI call spawns one anyway (ADR-0003).
set -eu

BINDIR=${BINDIR:-$HOME/.local/bin}
DESKTOPDIR=${DESKTOPDIR:-$HOME/.local/share/applications}
ICONDIR=${ICONDIR:-$HOME/.local/share/icons/hicolor/256x256/apps}
UNITDIR=${UNITDIR:-$HOME/.config/systemd/user}

cd "$(dirname "$0")"

if [ ! -x ./agentbox ]; then
	echo "install.sh: no agentbox binary beside this script - run it from the extracted tarball" >&2
	exit 1
fi

# The binary links GTK4 and WebKitGTK dynamically (the windows ARE a webview), so a
# missing runtime library is the one failure a download can hit that a build cannot.
# Say so here, with the fix, rather than letting the first card fail with a linker
# error the user has to interpret.
if command -v ldd >/dev/null 2>&1; then
	missing=$(ldd ./agentbox 2>/dev/null | awk '/not found/ {print "  " $1}')
	if [ -n "$missing" ]; then
		echo "install.sh: this build needs libraries that are not installed here:" >&2
		echo "$missing" >&2
		echo >&2
		echo "on Debian/Ubuntu:  sudo apt install libgtk-4-1 libwebkitgtk-6.0-4" >&2
		echo "on Fedora:         sudo dnf install gtk4 webkitgtk6.0" >&2
		echo "on Arch:           sudo pacman -S gtk4 webkitgtk-6.0" >&2
		echo >&2
		echo "installing anyway; the CLI works without them, the windows do not." >&2
	fi
fi

install -d "$BINDIR" "$DESKTOPDIR" "$ICONDIR" "$UNITDIR"
install -m 0755 ./agentbox "$BINDIR/agentbox"
install -m 0644 ./packaging/agentbox.desktop "$DESKTOPDIR/agentbox.desktop"
install -m 0644 ./packaging/agentbox.png "$ICONDIR/agentbox.png"
install -m 0644 ./packaging/agentbox.service "$UNITDIR/agentbox.service"

update-desktop-database "$DESKTOPDIR" 2>/dev/null || true
gtk-update-icon-cache -f -t "$HOME/.local/share/icons/hicolor" 2>/dev/null || true
systemctl --user daemon-reload 2>/dev/null || true

echo "installed $("$BINDIR/agentbox" version 2>/dev/null || echo agentbox) to $BINDIR"

case ":$PATH:" in
	*":$BINDIR:"*) ;;
	*) echo "note: $BINDIR is not on your PATH; add it to your shell profile" ;;
esac

cat <<EOF

next:
  agentbox notify --title hello --body "it works"  # raise a card (spawns the daemon)
  agentbox status                                  # daemon liveness and pending count
  systemctl --user enable --now agentbox.service   # optional: start on every login

to register it with Claude Code:
  claude mcp add --scope user agentbox -- $BINDIR/agentbox mcp
EOF
