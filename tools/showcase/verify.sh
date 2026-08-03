#!/usr/bin/env bash
# Prove a finished take is usable without watching it.
#
#   tools/showcase/verify.sh ~/Videos/agentbox-showcase-YYYYMMDD-HHMM.mp4 [seconds]
#
# It samples the bottom strip of the frame every N seconds (10 by default) and reports
# any moment where the deck was not fullscreen. That strip is the whole test: a slide
# fills it with the deck's near-black background (mean brightness ~10 of 255), and the
# desktop taskbar underneath fills it with icons and white text (~38). There is no
# middle ground, so one number per sample is enough.
#
# It exists because the take of 2026-07-25 22:33 lost its slideshow at 13:04 of 17:27
# and nobody knew until the file was watched, by which time the evening was gone. This
# answers the same question in about twenty seconds.
set -uo pipefail

FILE="${1:-}"
STEP="${2:-10}"
THRESH=20          # anything brighter than this in the bottom strip is not a slide

[ -n "$FILE" ] || { echo "usage: $0 FILE.mp4 [sample-seconds]"; exit 2; }
[ -f "$FILE" ] || { echo "no such file: $FILE"; exit 2; }
for tool in ffprobe ffmpeg identify; do
	command -v "$tool" >/dev/null || { echo "$tool is missing"; exit 2; }
done

DUR=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$FILE" | cut -d. -f1)
[ -n "$DUR" ] && [ "$DUR" -gt 0 ] 2>/dev/null || { echo "no duration: the file is not finished writing"; exit 2; }
W=$(ffprobe -v error -select_streams v:0 -show_entries stream=width -of csv=p=0 "$FILE")
H=$(ffprobe -v error -select_streams v:0 -show_entries stream=height -of csv=p=0 "$FILE")
printf 'checking %s\n  %sx%s, %s:%02d, every %ss\n' "$(basename "$FILE")" "$W" "$H" \
	"$((DUR / 60))" "$((DUR % 60))" "$STEP"

TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
bad=0 samples=0 first=""
for ((t = 1; t < DUR; t += STEP)); do
	ffmpeg -v error -ss "$t" -i "$FILE" -frames:v 1 \
		-vf "crop=${W}:44:0:$((H - 44))" -f image2 -y "$TMP/s.png" 2>/dev/null || continue
	mean=$(identify -format '%[fx:int(mean*255)]' "$TMP/s.png" 2>/dev/null) || continue
	samples=$((samples + 1))
	if [ "${mean:-0}" -gt "$THRESH" ]; then
		bad=$((bad + 1))
		[ -n "$first" ] || first=$t
		printf '  NOT FULLSCREEN at %d:%02d (strip mean %s)\n' "$((t / 60))" "$((t % 60))" "$mean"
	fi
done

echo
if [ "$bad" = 0 ]; then
	echo "clean: $samples samples, the deck was fullscreen in every one of them"
	exit 0
fi
printf 'RUINED: %s of %s samples show the desktop, from %d:%02d onwards.\n' \
	"$bad" "$samples" "$((first / 60))" "$((first % 60))"
echo "Usable up to that point only. Do not upload it; find out what left fullscreen"
echo "(docs/recording.md, \"What the 22:33 take cost\") and record again."
exit 1
