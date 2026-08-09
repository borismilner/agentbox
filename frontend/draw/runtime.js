// The stand-in for @wailsio/runtime used when the wiki's frames are DRAWN
// rather than photographed (FR99).
//
// It is the sibling of test/stubs/wailsio-runtime.js and differs from it in one
// way that matters. That stub RECORDS calls so a test can assert what a keystroke
// sent; this one ANSWERS them, from a fixture, so a real surface renders real
// markup over content written for the page. Nothing here fakes a component: the
// bundle is the shipped bundle, the CSS is the shipped CSS, and the only
// substitution in the whole chain is the door to the daemon.
//
// What that buys, and what it costs, is FR99's own argument: exact content and
// exact composition with no desktop, no daemon and no XTEST - at the price that
// the picture is no longer evidence. The rule that keeps it honest is that a
// frame may only say things a real run would also say, which is why the fixtures
// live beside the wire shapes they imitate and why frames.js cites the surface
// each field is read by.

import { FRAMES } from "./frames.js";

const params = new URLSearchParams(location.search);
const frame = FRAMES[params.get("frame")] ?? {};

// Wails keys bound methods by full import path; bridge.js builds the same string.
const PKG = "github.com/borismilner/agentbox/internal/webui.Bridge.";
const short = (name) => (name.startsWith(PKG) ? name.slice(PKG.length) : name);

export const Call = {
  ByName(name, ...args) {
    const m = short(name);
    // How tall the frame should be, answered by the product rather than guessed
    // by the screenshot step. Every frameless surface measures its own laid-out
    // height and hands it to Go through fit(), which is the number Go sizes the
    // window to - so a drawn frame that photographs exactly this height is the
    // window a real run would have opened. Guessing it instead was the first
    // version, and it produced a card stretched down a 2000px viewport.
    // Go's method names, capitalised, because that is what bridge.js builds the
    // FQN from and what a fixture has to name to answer one.
    if (m === "Fit" || m === "FitProgress") {
      const h = Number(args[0]);
      if (Number.isFinite(h) && h > 0) document.documentElement.dataset.h = String(Math.ceil(h));
    }
    const answers = frame.calls ?? {};
    const a = answers[m];
    return Promise.resolve(typeof a === "function" ? a(...args) : a);
  },
};

// Surfaces subscribe on mount and are pushed to afterwards, so a fixture's
// events cannot be delivered until the surface is listening. Emitting on the
// microtask after On() is what makes the push arrive in the right order without
// the fixture having to know how many events a surface subscribes to.
const listeners = new Map();

export const Events = {
  On(event, fn) {
    const set = listeners.get(event) ?? new Set();
    set.add(fn);
    listeners.set(event, set);
    if (frame.events && event in frame.events) {
      queueMicrotask(() => fn({ data: frame.events[event] }));
    }
    return () => set.delete(fn);
  },
};

export const Window = {
  Close() {},
  Minimise() {},
  ToggleMaximise() {},
  IsMaximised: () => Promise.resolve(false),
};

// The screenshot step waits for this rather than for a timeout, so a frame that
// renders slowly is never photographed half-drawn. Set once the surface has had
// its events and the browser has painted twice - the second frame is when a
// ResizeObserver-driven layout (the card measures itself) has settled.
requestAnimationFrame(() =>
  requestAnimationFrame(() => {
    document.documentElement.dataset.drawn = "1";
  }),
);
