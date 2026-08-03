#!/usr/bin/env bash
# record.sh - capture the narrated showcase as one clean video, fit to upload.
#
# What makes a screen recording look amateur is not the encoder, it is the edges:
# three seconds of a terminal before the first slide, a mouse hunting for a menu at
# the end, somebody's chat window across the middle. So this script owns the edges.
# It records with pre-roll and post-roll, takes two marks while the performance
# runs, and cuts the finished file to exactly those marks with a fade at each end.
# The viewer's first frame is the first slide.
#
# Capture is ffmpeg: x11grab on one monitor at 60fps, encoded on the iGPU
# (h264_vaapi, qp 18) so the CPU stays free for the deck, the speech engine and the
# driving - measured at 59.5 of 60 fps with drops only in the first frames. Audio is
# the default sink's *monitor* source, which is the loopback of what came out of the
# speakers: agentbox's narration and its earcons, and no microphone anywhere in the
# graph, so the room stays out of the file.
#
# Volume is not something the recording depends on. PipeWire's monitor tap is
# pre-volume - measured, the same line captured at 10% and at 60% speaker volume
# peaks at -7.6 dB and -8.6 dB, which is line-to-line variation and not the knob -
# so the human can set the speakers anywhere, mute them even, and the capture is
# untouched. There is no need to route agentbox through a null sink, and no need to ask
# anybody to keep their volume up. The finished file is normalised to -14 LUFS,
# which is what YouTube plays back at.
#
# It also puts the desktop back. Three things change for a take and all three are
# written down first and restored after: any ordinary window sitting in the capture
# region is moved off it, [presence] fullscreen_auto_dnd is turned off (agentbox holds
# every card while a fullscreen app has focus, and the deck is a fullscreen app), and
# the pointer is parked. `stop` and `abort` both restore, and either is safe twice.
#
#   record.sh check                    what would break, before anything changes
#   record.sh prepare                  clear the frame, release the fullscreen hold
#   record.sh start [--out FILE]       begin capturing (roll before you perform)
#   record.sh mark in                  the presentation starts here
#   record.sh park                     pointer out of the way of the content
#   record.sh mark out                 and ends here
#   record.sh stop                     cut to the marks, normalise, restore, report
#   record.sh abort                    stop capturing, restore, keep the raw file
#   record.sh status
#
# Options: --monitor NAME (default: the widest connected monitor), --out FILE,
# --fade SECONDS (0.5), --fps (60), --qp (18), --crf (18).

set -euo pipefail

STATE="${XDG_RUNTIME_DIR:-/tmp}/agentbox-showcase-record"
CFG="${XDG_CONFIG_HOME:-$HOME/.config}/agentbox/config.toml"
AgentBox="${AGENTBOX_BIN:-$HOME/.local/bin/agentbox}"
FADE=0.5
FPS=60
QP=18
CRF=18
LUFS=-14
MONITOR=""
OUT=""

die() { printf 'record: %s\n' "$*" >&2; exit 1; }
note() { printf '  %s\n' "$*"; }
# pause without sleep, which some agent harnesses refuse in the foreground.
naptime() { read -r -t "$1" _ < /dev/zero 2>/dev/null || true; }

# --- the monitor to record ---------------------------------------------------

# monitors prints "name width height x y" per connected monitor. xrandr's
# --listmonitors is the right source rather than wmctrl -lG, which reports doubled
# coordinates on a scaled display; xwininfo and wmctrl -e agree with xrandr, and
# those are the two this script reads and writes with.
monitors() {
	xrandr --listmonitors | sed -n 's|^ *[0-9]*: *[+*]*\([A-Za-z0-9-]*\) *\([0-9]*\)/[0-9]*x\([0-9]*\)/[0-9]*+\([0-9]*\)+\([0-9]*\).*|\1 \2 \3 \4 \5|p'
}

# pick_monitor takes the widest by default. On this desk that is the 16:9 screen
# rather than the portrait primary, which is what a presentation wants: a capture of
# the whole root window would be mostly dead space, and nobody watches a portrait
# recording of a slide deck.
pick_monitor() {
	if [ -n "$MONITOR" ]; then
		monitors | awk -v n="$MONITOR" '$1 == n' | head -1
		return
	fi
	monitors | sort -k2,2nr | head -1
}

# sink_monitor is the loopback of the default output, and the only audio input this
# script ever opens.
sink_monitor() {
	local sink
	sink=$(pactl get-default-sink 2>/dev/null) || die "no PipeWire/Pulse server"
	printf '%s.monitor\n' "$sink"
}

geometry() {
	xwininfo -id "$1" 2>/dev/null | awk '
		/Absolute upper-left X/ { x = $4 }
		/Absolute upper-left Y/ { y = $4 }
		/^  Width/  { w = $2 }
		/^  Height/ { h = $2 }
		END { if (w != "") print x, y, w, h }'
}

# KEEP matches the windows that belong in the frame and must never be moved out of
# it: the presenter itself, and agentbox's own windows. Getting this wrong is not a
# cosmetic failure - the first full take moved the deck to the other monitor, the
# slideshow went fullscreen over there, and the camera recorded an empty desktop
# while the narration played to nobody.
KEEP='ONLYOFFICE|agentbox-showcase|LibreOffice|Impress|agentbox ·'

# in_frame lists ordinary windows overlapping the capture region: what a viewer
# would see if the deck were not covering them. DESKTOP and DOCK windows are skipped
# because they are the desktop itself, and shoving the wallpaper off screen would be
# a strange way to tidy up; KEEP windows are skipped because they are the show.
in_frame() {
	local rw="$1" rh="$2" rx="$3" ry="$4"
	wmctrl -l | while read -r id _ _ rest; do
		local type x y w h name
		type=$(xprop -id "$id" _NET_WM_WINDOW_TYPE 2>/dev/null || true)
		case "$type" in *DESKTOP*|*DOCK*) continue ;; esac
		printf '%s' "$rest" | grep -qE "$KEEP" && continue
		read -r x y w h <<<"$(geometry "$id")" || continue
		[ -n "${w:-}" ] || continue
		[ $(( x < rx + rw && x + w > rx && y < ry + rh && y + h > ry )) = 1 ] || continue
		name=$(printf '%s' "$rest" | cut -d' ' -f2-)
		printf '%s %s %s %s %s %s\n' "$id" "$x" "$y" "$w" "$h" "$name"
	done
}

# --- checks -----------------------------------------------------------------

cmd_check() {
	local ok=0 mon sinkmon w h
	echo "recorder:"
	command -v ffmpeg >/dev/null || { note "MISSING ffmpeg"; ok=1; }
	note "ffmpeg $(ffmpeg -version 2>/dev/null | head -1 | cut -d' ' -f3)"
	if [ -e /dev/dri/renderD128 ]; then
		note "hardware encode: h264_vaapi on /dev/dri/renderD128, ${FPS}fps qp $QP"
	else
		note "no /dev/dri/renderD128: falling back to libx264 (more CPU during the take)"
	fi

	echo "screen:"
	mon=$(pick_monitor) || true
	[ -n "$mon" ] || { note "MISSING monitor (xrandr found none)"; ok=1; return $ok; }
	# shellcheck disable=SC2086
	set -- $mon
	w=$(( $2 - $2 % 2 )); h=$(( $3 - $3 % 2 ))
	note "$1  ${w}x${h} at +$4,+$5"
	[ "$w" = "$2" ] && [ "$h" = "$3" ] || note "cropped to even dimensions for h264"

	echo "audio:"
	sinkmon=$(sink_monitor)
	note "$sinkmon"
	pactl list short sources | grep -q "	$sinkmon	" || { note "MISSING that source"; ok=1; }
	note "no microphone in the graph, and the tap is pre-volume: set the speakers anywhere"

	echo "agentbox:"
	[ -x "$AgentBox" ] || { note "MISSING $AgentBox"; ok=1; }
	"$AgentBox" status >/dev/null 2>&1 || { note "MISSING daemon (agentbox status fails)"; ok=1; }
	note "$("$AgentBox" status 2>&1 | head -1)"
	# GNOME's banner switch doubles as agentbox's desktop-DND signal, so banners off
	# would hold every card and the deck would run over silence.
	if [ "$(gsettings get org.gnome.desktop.notifications show-banners 2>/dev/null)" = "false" ]; then
		note "GNOME banners are off, which agentbox reads as DND: every card would be held"
		ok=1
	else
		note "GNOME banners on, so respect_desktop_dnd is not holding"
	fi
	if grep -qs '^[[:space:]]*fullscreen_auto_dnd[[:space:]]*=[[:space:]]*false' "$CFG"; then
		note "fullscreen_auto_dnd already false (prepare leaves it alone)"
	else
		note "fullscreen_auto_dnd is on: prepare turns it off, stop puts it back"
	fi

	echo "in the frame right now:"
	in_frame "$w" "$h" "$4" "$5" | while read -r id x y ww hh name; do
		note "$id  ${ww}x${hh} at +$x,+$y  $name"
	done
	[ "$(in_frame "$w" "$h" "$4" "$5" | wc -l)" = 0 ] && note "nothing, the frame is clear"

	echo "disk:"
	note "$(df -h --output=avail "$HOME" | tail -1 | tr -d ' ') free in $HOME  (a 1080p60 take of this deck runs well under 1 GB)"
	return $ok
}

# --- prepare and restore -----------------------------------------------------

cmd_prepare() {
	mkdir -p "$STATE"
	local mon w h x y
	mon=$(pick_monitor)
	# shellcheck disable=SC2086
	set -- $mon
	w=$(( $2 - $2 % 2 )); h=$(( $3 - $3 % 2 )); x=$4; y=$5
	printf '%s %s %s %s %s\n' "$1" "$w" "$h" "$x" "$y" > "$STATE/region"
	echo "region: $1 ${w}x${h} at +$x,+$y"

	# Windows in the frame go to the root's origin, which is the other monitor on any
	# desk where the recorded one is not at x=0. The old geometry is written down
	# first, and the move is verified rather than assumed - a window manager is free
	# to ignore the request, and a silent failure here is a chat window in the video.
	: > "$STATE/windows"
	while read -r id wx wy ww wh name; do
		[ -n "${id:-}" ] || continue
		printf '%s %s %s %s %s\n' "$id" "$wx" "$wy" "$ww" "$wh" >> "$STATE/windows"
		wmctrl -i -r "$id" -e "0,0,$wy,$ww,$wh" 2>/dev/null || true
		naptime 0.3
		local nx
		nx=$(geometry "$id" | cut -d' ' -f1)
		if [ "${nx:-0}" -lt "$x" ] 2>/dev/null; then
			echo "moved out of frame: $name"
		else
			echo "COULD NOT MOVE: $name (still at x=${nx:-?}; close or minimise it yourself)"
		fi
	done < <(in_frame "$w" "$h" "$x" "$y")
	[ -s "$STATE/windows" ] || echo "frame was already clear"

	# agentbox holds every card while a fullscreen app has focus (FR29), and the deck is
	# a fullscreen app, so the whole demo would be held back with nothing on screen.
	if grep -qs '^[[:space:]]*fullscreen_auto_dnd[[:space:]]*=[[:space:]]*false' "$CFG"; then
		echo "fullscreen_auto_dnd: already off, left alone"
	else
		cp -p "$CFG" "$STATE/config.toml.bak"
		if grep -qs '^\[presence\]' "$CFG"; then
			if grep -qs '^[[:space:]]*fullscreen_auto_dnd' "$CFG"; then
				sed -i 's|^[[:space:]]*fullscreen_auto_dnd.*|fullscreen_auto_dnd = false|' "$CFG"
			else
				sed -i '/^\[presence\]/a fullscreen_auto_dnd = false' "$CFG"
			fi
		else
			printf '\n[presence]\n# Written by tools/showcase/record.sh for one recording and removed again by\n# `record.sh stop`. A fullscreen deck must not hold back the cards it is there\n# to introduce.\nfullscreen_auto_dnd = false\n' >> "$CFG"
		fi
		echo "fullscreen_auto_dnd: off (config backed up, restored on stop)"
	fi

	# The keyboard group is no longer prepared here. agentbox locks the planned group
	# around every synthetic key press itself (internal/hand/xkb.go), so a second
	# layout cannot rewrite typed text no matter what is selected. The gsettings
	# write that used to sit in this spot was measured doing nothing: GNOME 46
	# ignores the deprecated `input-sources current` key in both directions.

	cmd_park
}

# park puts the pointer somewhere it hides nothing. It has to stay on the recorded
# monitor, because the pointer is what decides where a card lands. Left edge, half
# way down: the slide's own text starts about 7% in, cards are centred, toasts sit
# top-right, and - the reason it is not the bottom corner, which was tried first -
# a presenter's control strip lives along the bottom and a pointer resting there
# summons it into the recording. Call it after every click, so a demo of an
# interface is not a demo of an arrow sitting on top of one.
cmd_park() {
	[ -f "$STATE/region" ] || die "run prepare first"
	local name w h x y px py
	read -r name w h x y < "$STATE/region"
	px=$(( x + 50 )); py=$(( y + h / 2 ))
	printf 'screen\nmove %s %s\n' "$px" "$py" | "$AgentBox" drive run - >/dev/null 2>&1 \
		&& echo "pointer parked at $px,$py (on $name, out of the content)" \
		|| echo "pointer: agentbox drive refused; move it onto $name by hand"
}

restore() {
	if [ -s "$STATE/windows" ]; then
		while read -r id wx wy ww wh; do
			wmctrl -i -r "$id" -e "0,$wx,$wy,$ww,$wh" 2>/dev/null || true
		done < "$STATE/windows"
		echo "windows put back"
		: > "$STATE/windows"
	fi
	if [ -f "$STATE/config.toml.bak" ]; then
		cat "$STATE/config.toml.bak" > "$CFG"
		rm -f "$STATE/config.toml.bak"
		echo "config restored (the daemon reloads within a second)"
	fi
	# A stale input-source file may remain from a pre-fix run; it is dead weight
	# now (see prepare) and is simply dropped.
	rm -f "$STATE/input-source"
}

# --- capture ----------------------------------------------------------------

cmd_start() {
	[ -f "$STATE/region" ] || die "run prepare first"
	if [ -f "$STATE/pid" ] && kill -0 "$(cat "$STATE/pid")" 2>/dev/null; then
		die "already recording (pid $(cat "$STATE/pid"))"
	fi
	local name w h x y raw out
	read -r name w h x y < "$STATE/region"
	out="${OUT:-$HOME/Videos/agentbox-showcase-$(date +%Y%m%d-%H%M).mp4}"
	raw="$STATE/raw.mkv"
	mkdir -p "$(dirname "$out")"
	rm -f "$raw"
	printf '%s\n' "$out" > "$STATE/out"
	: > "$STATE/marks"

	# Matroska for the capture and mp4 only for the finished cut. A take that dies
	# halfway leaves a playable mkv, where an mp4 killed before its trailer is a
	# broken file - and an unattended seven-minute take is when that matters.
	local hw=(-vaapi_device /dev/dri/renderD128)
	local venc=(-vf format=nv12,hwupload -c:v h264_vaapi -qp "$QP")
	if [ ! -e /dev/dri/renderD128 ]; then
		hw=(); venc=(-c:v libx264 -preset veryfast -crf 20 -pix_fmt yuv420p)
	fi
	ffmpeg -hide_banner -loglevel warning -nostdin \
		"${hw[@]}" \
		-thread_queue_size 2048 -f x11grab -framerate "$FPS" -video_size "${w}x${h}" -i ":0.0+$x,$y" \
		-thread_queue_size 1024 -f pulse -ac 2 -ar 48000 -i "$(sink_monitor)" \
		"${venc[@]}" -c:a aac -b:a 320k \
		"$raw" >"$STATE/ffmpeg.log" 2>&1 &
	echo $! > "$STATE/pid"
	date +%s.%N > "$STATE/started"
	rolling
	echo "recording $name ${w}x${h}@${FPS} -> $raw"
	echo "audio: $(sink_monitor) (no microphone; capture level is independent of the speakers)"
	echo "finished file: $out"
	echo "next: mark in  ->  perform  ->  mark out  ->  stop"
}

# rolling waits for the first frames, so `mark in` cannot land before the capture
# exists.
rolling() {
	local pid tries=0
	pid=$(cat "$STATE/pid")
	while [ $tries -lt 100 ]; do
		kill -0 "$pid" 2>/dev/null || { tail -5 "$STATE/ffmpeg.log" >&2; die "ffmpeg died on startup"; }
		[ -s "$STATE/raw.mkv" ] && return 0
		tries=$((tries + 1))
		naptime 0.1
	done
	die "ffmpeg wrote nothing in 10s"
}

cmd_mark() {
	local which="${1:-}"
	case "$which" in in|out) ;; *) die "mark wants in or out" ;; esac
	[ -f "$STATE/started" ] || die "not recording"
	local at
	at=$(echo "$(date +%s.%N) - $(cat "$STATE/started")" | bc)
	printf '%s %s\n' "$which" "$at" >> "$STATE/marks"
	printf 'mark %s at %.1fs into the capture\n' "$which" "$at"
}

cmd_stop() {
	[ -f "$STATE/pid" ] || die "not recording"
	local pid raw out in_at out_at dur fo
	pid=$(cat "$STATE/pid")
	raw="$STATE/raw.mkv"
	out=$(cat "$STATE/out")

	# SIGINT is ffmpeg's clean stop: it flushes and writes the trailer. Anything
	# harder leaves a file that will not seek.
	if kill -0 "$pid" 2>/dev/null; then
		kill -INT "$pid" 2>/dev/null || true
		local tries=0
		while kill -0 "$pid" 2>/dev/null && [ $tries -lt 150 ]; do
			tries=$((tries + 1)); naptime 0.1
		done
		kill -0 "$pid" 2>/dev/null && kill -9 "$pid" 2>/dev/null || true
	fi
	rm -f "$STATE/pid"
	echo "capture stopped: $(du -h "$raw" | cut -f1) raw"

	in_at=$(awk '$1 == "in"  { print $2 }' "$STATE/marks" | head -1)
	out_at=$(awk '$1 == "out" { print $2 }' "$STATE/marks" | tail -1)
	: "${in_at:=0}"
	if [ -z "${out_at:-}" ]; then
		out_at=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$raw")
		echo "no out mark: cutting to the end of the capture"
	fi
	dur=$(echo "$out_at - $in_at" | bc)
	(( $(echo "$dur > 1" | bc) )) || die "marks are ${dur}s apart; the raw file is kept at $raw"
	fo=$(echo "$dur - $FADE" | bc)

	# Measure the loudness first, then apply it. Single-pass loudnorm adapts as it
	# goes, which pumps the first sentences of a long recording; two passes give one
	# constant gain, and -14 LUFS is what YouTube plays back at, so the upload is not
	# turned down on arrival.
	echo "measuring loudness"
	local m tp thresh offset
	m=$(ffmpeg -hide_banner -nostdin -ss "$in_at" -i "$raw" -t "$dur" \
		-af "loudnorm=I=$LUFS:TP=-1.5:LRA=11:print_format=json" -f null - 2>&1 | tr -d ' ' | tr -d '"')
	local mi mtp mlra mthresh moffset
	mi=$(printf '%s' "$m" | sed -n 's/.*input_i:\(-\?[0-9.]*\).*/\1/p' | tail -1)
	mtp=$(printf '%s' "$m" | sed -n 's/.*input_tp:\(-\?[0-9.]*\).*/\1/p' | tail -1)
	mlra=$(printf '%s' "$m" | sed -n 's/.*input_lra:\([0-9.]*\).*/\1/p' | tail -1)
	mthresh=$(printf '%s' "$m" | sed -n 's/.*input_thresh:\(-\?[0-9.]*\).*/\1/p' | tail -1)
	moffset=$(printf '%s' "$m" | sed -n 's/.*target_offset:\(-\?[0-9.]*\).*/\1/p' | tail -1)
	local norm="loudnorm=I=$LUFS:TP=-1.5:LRA=11"
	if [ -n "${mi:-}" ] && [ -n "${mtp:-}" ]; then
		note "narration measured at ${mi} LUFS, peak ${mtp} dBTP -> normalising to $LUFS LUFS"
		norm="loudnorm=I=$LUFS:TP=-1.5:LRA=11:measured_I=$mi:measured_TP=$mtp:measured_LRA=$mlra:measured_thresh=$mthresh:offset=$moffset:linear=true"
	else
		note "could not measure; falling back to a single pass"
	fi

	# One re-encode, and it earns its keep: it cuts the setup off the front and the
	# tidying off the back, fades each end so the video opens and closes on black,
	# and normalises the voice. x264 rather than the iGPU because this pass is
	# offline, and the settings are what YouTube asks for: high profile, yuv420p,
	# bt709 tagged rather than guessed, a keyframe every two seconds, faststart.
	echo "cutting ${in_at}s..${out_at}s (${dur}s), fade ${FADE}s, encoding for upload"
	ffmpeg -hide_banner -loglevel warning -nostdin -y \
		-ss "$in_at" -i "$raw" -t "$dur" \
		-vf "fade=t=in:st=0:d=$FADE,fade=t=out:st=$fo:d=$FADE" \
		-af "$norm,afade=t=in:st=0:d=$FADE,afade=t=out:st=$fo:d=$FADE" \
		-c:v libx264 -preset slow -crf "$CRF" -pix_fmt yuv420p -profile:v high -level 4.2 \
		-g $(( FPS * 2 )) -maxrate 20M -bufsize 40M \
		-color_primaries bt709 -color_trc bt709 -colorspace bt709 \
		-c:a aac -b:a 320k -ar 48000 -ac 2 \
		-movflags +faststart "$out"

	restore
	report "$out"
}

cmd_abort() {
	if [ -f "$STATE/pid" ]; then
		kill -INT "$(cat "$STATE/pid")" 2>/dev/null || true
		rm -f "$STATE/pid"
		echo "capture stopped; the raw file is kept at $STATE/raw.mkv"
	fi
	restore
}

report() {
	local f="$1" mean peak
	echo
	echo "$f"
	ffprobe -v error -show_entries format=duration,size,bit_rate \
		-show_entries stream=codec_name,width,height,avg_frame_rate,channels,sample_rate \
		-of default=noprint_wrappers=1 "$f" | sed 's/^/  /'
	# A recording with no sound is the failure that looks like a success, so the
	# level is reported rather than left to be discovered on YouTube.
	mean=$(ffmpeg -hide_banner -nostdin -i "$f" -af volumedetect -f null - 2>&1 | sed -n 's/.*mean_volume: \(.*\)/\1/p')
	peak=$(ffmpeg -hide_banner -nostdin -i "$f" -af volumedetect -f null - 2>&1 | sed -n 's/.*max_volume: \(.*\)/\1/p')
	echo "  narration: mean ${mean:-unknown}, peak ${peak:-unknown}"
}

cmd_status() {
	if [ -f "$STATE/pid" ] && kill -0 "$(cat "$STATE/pid")" 2>/dev/null; then
		printf 'recording, %.1fs so far\n' "$(echo "$(date +%s.%N) - $(cat "$STATE/started")" | bc)"
		[ -s "$STATE/marks" ] && sed 's/^/  mark /' "$STATE/marks"
	else
		echo "not recording"
	fi
	[ -s "$STATE/windows" ] && echo "windows still moved out of frame ($(wc -l < "$STATE/windows"))"
	[ -f "$STATE/config.toml.bak" ] && echo "config knob still changed (run stop or abort)"
	return 0
}

main() {
	local cmd="${1:-}"; shift || true
	local args=()
	while [ $# -gt 0 ]; do
		case "$1" in
			--monitor) MONITOR="$2"; shift 2 ;;
			--out) OUT="$2"; shift 2 ;;
			--fade) FADE="$2"; shift 2 ;;
			--fps) FPS="$2"; shift 2 ;;
			--qp) QP="$2"; shift 2 ;;
			--crf) CRF="$2"; shift 2 ;;
			*) args+=("$1"); shift ;;
		esac
	done
	case "$cmd" in
		check)   cmd_check ;;
		prepare) cmd_prepare ;;
		start)   cmd_start ;;
		mark)    cmd_mark "${args[0]:-}" ;;
		park)    cmd_park ;;
		stop)    cmd_stop ;;
		abort)   cmd_abort ;;
		status)  cmd_status ;;
		*) sed -n '3,46p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
	esac
}

main "$@"
