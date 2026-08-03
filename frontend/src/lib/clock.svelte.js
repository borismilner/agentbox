// A single shared 1Hz tick. Every countdown on screen reads the same store,
// so ten cards cost one timer and all of them change digit on the same frame.

import { onDestroy } from "svelte";

let subscribers = new Set();
let handle = null;

function pump() {
  const now = Date.now();
  for (const fn of subscribers) fn(now);
}

export function ticker() {
  const state = $state({ now: Date.now() });
  const fn = (now) => (state.now = now);
  subscribers.add(fn);
  if (!handle) handle = setInterval(pump, 1000);
  onDestroy(() => {
    subscribers.delete(fn);
    if (!subscribers.size && handle) {
      clearInterval(handle);
      handle = null;
    }
  });
  return state;
}

// "4:32" / "12s" - short enough to sit in a footer without wrapping.
export function remaining(deadlineMs, now) {
  if (!deadlineMs) return "";
  const s = Math.max(0, Math.round((deadlineMs - now) / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  return `${m}:${String(s % 60).padStart(2, "0")}`;
}
