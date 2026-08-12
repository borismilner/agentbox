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
# Rather than print a command and leave the user to run it, install them - that is
# the difference between a download that works and a download that gives you
# homework. It is the only part that needs root, so it is also the only part that
# asks, and it asks once with the exact command it is about to run.
#
# --yes installs without asking (for scripts and images); --no-deps skips this
# entirely, for somebody who manages their own packages.
ASSUME_YES=${AGENTBOX_ASSUME_YES:-}
SKIP_DEPS=

for arg in "$@"; do
	case "$arg" in
		-y|--yes) ASSUME_YES=1 ;;
		--no-deps) SKIP_DEPS=1 ;;
		-h|--help)
			echo "usage: ./install.sh [-y|--yes] [--no-deps]"
			echo "  -y, --yes    install missing system libraries without asking"
			echo "      --no-deps  never touch system packages"
			exit 0 ;;
		*) echo "install.sh: unknown option $arg (try --help)" >&2; exit 2 ;;
	esac
done

# The package names differ per distribution and the runtime ones are not the -dev
# ones a build needs, so this is a table rather than one command.
deps_cmd() {
	if command -v apt-get >/dev/null 2>&1; then
		echo "apt-get install -y libgtk-4-1 libwebkitgtk-6.0-4"
	elif command -v dnf >/dev/null 2>&1; then
		echo "dnf install -y gtk4 webkitgtk6.0"
	elif command -v pacman >/dev/null 2>&1; then
		echo "pacman -S --needed --noconfirm gtk4 webkitgtk-6.0"
	elif command -v zypper >/dev/null 2>&1; then
		echo "zypper install -y gtk4 libwebkitgtk-6_0-4"
	fi
}

missing_libs() {
	command -v ldd >/dev/null 2>&1 || return 0
	ldd ./agentbox 2>/dev/null | awk '/not found/ {print $1}'
}

install_deps() {
	missing=$(missing_libs)
	[ -n "$missing" ] || return 0

	echo "this build needs shared libraries that are not installed here:"
	echo "$missing" | sed 's/^/  /'
	echo

	if [ -n "$SKIP_DEPS" ]; then
		echo "--no-deps: leaving them to you. The CLI works without them; the windows do not."
		echo
		return 0
	fi

	cmd=$(deps_cmd)
	if [ -z "$cmd" ]; then
		echo "unknown package manager - install GTK4 and WebKitGTK 6.0 by hand." >&2
		echo
		return 0
	fi

	sudo=
	if [ "$(id -u)" != 0 ]; then
		command -v sudo >/dev/null 2>&1 || {
			echo "no sudo here. Run this as root, or install by hand:  $cmd" >&2
			echo
			return 0
		}
		sudo=sudo
	fi

	if [ -z "$ASSUME_YES" ]; then
		# No tty means nobody is there to answer, so it must not hang waiting.
		if [ ! -t 0 ]; then
			echo "not a terminal, so not installing packages. Re-run with --yes, or:  $sudo $cmd" >&2
			echo
			return 0
		fi
		printf 'run "%s %s"? [Y/n] ' "$sudo" "$cmd"
		read -r reply
		case "$reply" in
			[Nn]*) echo "skipped. Install by hand when you are ready:  $sudo $cmd"; echo; return 0 ;;
		esac
	fi

	echo "--> $sudo $cmd"
	if command -v apt-get >/dev/null 2>&1; then
		$sudo apt-get update
	fi
	# Unquoted on purpose: $cmd is several words from the table above, not input.
	# shellcheck disable=SC2086
	$sudo $cmd || {
		echo "install.sh: installing the libraries failed. AgentBox is still installed;" >&2
		echo "  its CLI works, and the windows will once the libraries are there." >&2
	}

	still=$(missing_libs)
	if [ -n "$still" ]; then
		echo "still missing after the install:" >&2
		echo "$still" | sed 's/^/  /' >&2
	else
		echo "libraries present."
	fi
	echo
}

install_deps

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
