# ADR-0006: Sound via subprocess (pw-play chain), embedded assets

Status: proposed

## Context

FR13 wants short earcons. The host runs PipeWire (pipewire-pulse). The
in-process Go audio stack in 2026: ebitengine/oto v3 is ALSA-only on Linux,
needs cgo, and holds an audio device open from the daemon; gopxl/beep v2
sits on oto with the same footprint; libcanberra is GNOME-blessed but
upstream-frozen with stale cgo bindings.

## Decision

Spawn a player process per earcon: try `pw-play`, fall back to `paplay`,
then `aplay`; resolve the chain once at daemon start. Assets are OGG/WAV
embedded via go:embed, written once to `$XDG_RUNTIME_DIR/agentbox/sounds/`.
Concurrent earcons are coalesced (one player at a time; an `urgent` sound
replaces a playing lesser one).

## Alternatives

- oto/beep in-process: lower per-play latency, gapless mixing - none of
  which a sub-400 ms chime needs. Costs cgo, ALSA-layer dependency, and a
  daemon holding an audio handle 24/7.
- libcanberra/gsound: the only way to respect the freedesktop sound theme;
  frozen upstream, old bindings. Parked: a config option could later map
  classes to theme event names via `canberra-gtk-play`.

## Consequences

- Zero audio dependencies in the binary; the sound package is ~100 lines.
- 50-150 ms spawn latency per chime, imperceptible for notifications.
- If a future feature needs mixed or gapless audio, this ADR gets
  superseded rather than stretched.
