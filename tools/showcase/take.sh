#!/usr/bin/env bash
# One take, start to finish, in a single process: prepare the desk, put the deck
# into its slideshow, roll, perform all eighteen slides, cut, restore.
#
# It is one script because the marks have to bracket the performance with nothing
# else in between. Two shell commands with an agent's thinking time between them put
# thirteen seconds of silent title slide into the first rehearsal.
set -uo pipefail
cd "$(dirname "$0")/../.." || exit 1   # the repo root, wherever it is checked out

OUT="$HOME/Videos/agentbox-showcase-$(date +%Y%m%d-%H%M).mp4"
nap() { read -r -t "$1" _ < /dev/zero 2>/dev/null || true; }
# fullscreen_here prints the id of a fullscreen window that is actually on the monitor
# being recorded. "Some window somewhere is fullscreen" is not the same question, and
# answering that one instead is how a take once recorded an empty desktop while the
# slideshow ran on the other screen.
fullscreen_here() {
	local rx="$1" ry="$2" rw="$3" rh="$4" w x y ww hh
	for w in $(wmctrl -l | awk '{print $1}'); do
		case "$(xprop -id "$w" _NET_WM_STATE 2>/dev/null)" in *FULLSCREEN*) ;; *) continue ;; esac
		read -r x y ww hh <<<"$(xwininfo -id "$w" | awk '
			/Absolute upper-left X/ { x = $4 } /Absolute upper-left Y/ { y = $4 }
			/^  Width/ { w = $2 } /^  Height/ { h = $2 } END { print x, y, w, h }')"
		[ "$x" = "$rx" ] && [ "$y" = "$ry" ] && [ "$ww" = "$rw" ] && [ "$hh" = "$rh" ] || continue
		printf '%s ' "$w"
	done
}

# deck_window is the OnlyOffice window, resolved by its class and never by its title.
# `wmctrl -l | grep -i showcase | head -1` matched the *terminal* the take was started
# from - it was called "Record agentbox showcase demo" and it is the older window - so a
# take moved that terminal onto the recorded monitor, raised it over the deck and
# clicked the play button into it. A window title belongs to whoever set it.
deck_window() {
	local w
	for w in $(wmctrl -l | awk '{print $1}'); do
		case "$(xprop -id "$w" WM_CLASS 2>/dev/null)" in
			*DesktopEditors*|*ONLYOFFICE*) printf '%s\n' "$w"; return 0 ;;
		esac
	done
	return 1
}

# bail gives up *and puts the desk back*. prepare has already turned
# fullscreen_auto_dnd off and may have moved windows; an exit that leaves those in
# place means the next prepare finds the knob already off, does not back it up, and
# no later stop ever restores it.
bail() {
	echo "$*"
	./tools/showcase/record.sh abort >/dev/null 2>&1 || true
	exit 1
}

REHEARSE=1
[ "${1:-}" = "--no-rehearse" ] && REHEARSE=0

echo "=== ask first"
# The gate: nothing is touched until there is a real click. A take costs twenty
# minutes of somebody's screen, and starting one while they are typing wastes both.
agentbox confirm --title "Rehearse and record the showcase?" \
	--body "$(printf '%s\n\n%s\n\n%s' \
		'Two parts, about twenty-five minutes of this screen in total.' \
		'**Rehearsal** - every window, click and card, silent, about seven minutes. **Then the take** - about seventeen and a half minutes with the narration, and only if the rehearsal was clean.' \
		'Click **Yes** when you are ready and hands are off the keyboard.')" \
	--speak "Ready to rehearse and then record. About twenty five minutes of your screen in total. Click yes when you want me to start, and then hands off." \
	--agent claude --project agentbox --timeout 900
case $? in
	0) echo "confirmed" ;;
	1) echo "declined; nothing was changed"; exit 0 ;;
	*) echo "no answer; nothing was changed"; exit 0 ;;
esac
nap 2   # let the card leave the screen before the camera sees it

echo "=== prepare"
./tools/showcase/record.sh prepare || exit 1
read -r MON RW RH RX RY < "${XDG_RUNTIME_DIR:-/tmp}/agentbox-showcase-record/region"
echo "recording $MON ${RW}x${RH} at +$RX,+$RY"

echo "=== the deck into its slideshow, on that monitor"
# Ask first, touch second. A deck that is already showing fullscreen on the recorded
# monitor is the state we want, and moving or raising it can only lose it.
fs=$(fullscreen_here "$RX" "$RY" "$RW" "$RH")
if [ -z "$fs" ]; then
	id=$(deck_window) || bail "the deck is not open"
	# Put the editor on the recorded monitor before starting the show: OnlyOffice goes
	# fullscreen on whichever monitor its window is on.
	wmctrl -i -r "$id" -e "0,$RX,$RY,$RW,$((RH - 48))"
	wmctrl -i -a "$id"; nap 1.5
	# Its play button, bottom left of the editor's status bar. Ctrl+F5 works for a
	# human but not as a synthetic keystroke, which is why this is a click.
	printf 'screen\nclick %s %s\n' "$((RX + 23))" "$((RY + RH - 64))" | agentbox drive run - >/dev/null 2>&1
	# The slideshow is a *new* X window, and mapping it takes longer than any one
	# guess: 2.5s was not enough on 2026-07-25 and the run refused a stage that came
	# up a moment later. Poll instead.
	for _ in $(seq 1 20); do
		nap 1
		fs=$(fullscreen_here "$RX" "$RY" "$RW" "$RH")
		[ -n "$fs" ] && break
	done
fi
[ -n "$fs" ] || bail "no fullscreen window on $MON; refusing to record the wrong screen"
echo "fullscreen on $MON: $fs"
nap 2   # let the presenter's control strip fade before the camera rolls

# The rehearsal, with no camera and no voice: every window opened, every click landed,
# every card answered, and the slideshow still fullscreen at the end of each slide.
# It exists because the take of 22:33 lost its slideshow four minutes before the end
# and nothing noticed - seventeen and a half minutes of somebody's evening for a file
# that had to be thrown away. Seven minutes to protect eighteen is the trade.
if [ "$REHEARSE" = 1 ]; then
	echo "=== rehearse (no camera, no voice)"
	if ! .venv-deck/bin/python tools/showcase/perform.py --rehearse --park 1130,540; then
		echo "the rehearsal found problems; NOT recording"
		./tools/showcase/record.sh abort >/dev/null 2>&1
		./tools/showcase/record.sh stop >/dev/null 2>&1 || true
		agentbox notify --level error --title "The rehearsal failed" \
			--body "Nothing was recorded. The problems are in the terminal." \
			--speak "The rehearsal found problems, so I did not record anything." \
			--agent claude --project agentbox
		exit 1
	fi
	# The rehearsal ends on the last slide and with its own cards in the queue; the
	# performance sends Home itself, but give the screen a moment to settle.
	nap 3
	fs=$(fullscreen_here "$RX" "$RY" "$RW" "$RH")
	[ -n "$fs" ] || bail "the rehearsal left the deck out of fullscreen; not recording"
fi

echo "=== roll"
./tools/showcase/record.sh start --out "$OUT" || bail "the camera would not start"

echo "=== perform"
.venv-deck/bin/python tools/showcase/perform.py --park 1130,540 --marks
performed=$?

echo "=== cut and restore"
./tools/showcase/record.sh stop
if [ "$performed" != 0 ]; then
	echo "=== the performance stopped early: $OUT holds only what happened before that"
	agentbox notify --level warning --title "The take stopped early" \
		--body "Something went wrong mid-performance; the file holds only what came before it." \
		--speak "The take stopped early. The reason is in the terminal." \
		--agent claude --project agentbox
	exit 1
fi
echo "=== verify the film itself"
# The performance can report success and the film can still be unusable: the stage is
# what the camera saw, not what the script believed. This samples it.
if ./tools/showcase/verify.sh "$OUT" 10; then
	agentbox notify --level success --title "The showcase is recorded" \
		--body "$(basename "$OUT") - checked frame by frame, the deck was fullscreen throughout." \
		--speak "The showcase is recorded, and the film checks out." \
		--agent claude --project agentbox
else
	agentbox notify --level error --title "The take is not usable" \
		--body "The deck left fullscreen part way through. The timestamps are in the terminal." \
		--speak "The take is not usable. The deck left fullscreen part way through." \
		--agent claude --project agentbox
	exit 1
fi
echo "=== done: $OUT"
