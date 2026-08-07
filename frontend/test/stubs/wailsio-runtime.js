// A stand-in for @wailsio/runtime, which only exists inside a Wails webview.
//
// bridge.js is the single door between every surface and the daemon, and it is
// built entirely out of these three objects. Aliasing the package here rather
// than mocking bridge.js keeps the real bridge in the test: a surface that calls
// the wrong method name, or hands it the wrong arguments, still fails.
//
// Every call is recorded instead of dispatched. `calls` is the assertion surface
// for "did pressing this key actually answer the question", which is the one
// thing no test in this repository could ask before.

export const calls = [];

export function reset() {
  calls.length = 0;
  Call.byName.clear();
}

export const Call = {
  // Name -> value or thrower, so a test can make one method reject and leave the
  // rest working. That is U-01's case, and it is the reason this is a map rather
  // than a single default.
  byName: new Map(),
  ByName(name, ...args) {
    calls.push({ name, args });
    const stub = Call.byName.get(name);
    if (typeof stub === "function") return Promise.resolve().then(() => stub(...args));
    return Promise.resolve(stub);
  },
};

// Surfaces subscribe on mount and are pushed to afterwards. `emit` is how a test
// plays the daemon: mount the component, then hand it a view.
const listeners = new Map();

export const Events = {
  On(event, fn) {
    const set = listeners.get(event) ?? new Set();
    set.add(fn);
    listeners.set(event, set);
    return () => set.delete(fn);
  },
};

export function emit(event, data) {
  for (const fn of listeners.get(event) ?? []) fn({ data });
}

export function clearListeners() {
  listeners.clear();
}

export const Window = {
  Close() {},
  Minimise() {},
  ToggleMaximise() {},
  IsMaximised: () => Promise.resolve(false),
};
