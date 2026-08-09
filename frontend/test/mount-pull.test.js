import { describe, test, expect, beforeEach, afterEach } from "vitest";
import { mount, unmount, flushSync } from "svelte";
import Card from "../src/surfaces/Card.svelte";
import Toast from "../src/surfaces/Toast.svelte";
import { Call, calls, reset, clearListeners, emit } from "./stubs/wailsio-runtime.js";

// R-05. The card and toast had no pull. Their first view arrived as a
// fire-and-forget Wails event with nothing to buffer it, and Go stopped waiting
// for the surface after two seconds and emitted regardless - a guess about
// WebKit startup on a loaded machine, measured nowhere. A bundle that mounted
// after that guess got nothing, and nothing re-sent it until the next queue
// change: the human heard the earcon and looked at an empty window over a live
// question that an agent was parked on.
//
// These tests are the case that could not be written before the vitest harness
// existed, and they are the exact shape R-40 was asked for: mount the component,
// never push anything, and assert the question is on screen anyway.

const SVC = "github.com/borismilner/agentbox/internal/webui.Bridge.";
const stub = (method, fn) => Call.byName.set(SVC + method, fn);
const sent = (method) => calls.filter((c) => c.name === SVC + method);

function view(item = {}, extra = {}) {
  return {
    item: {
      id: "itm-1",
      kind: "choice",
      level: "info",
      title: "Where should 2026.7.30 go first?",
      body: "",
      identity: { agent: "release-bot", project: "checkout-api" },
      options: [{ label: "eu-west" }, { label: "us-east" }],
      ...item,
    },
    bodyHtml: "",
    waiting: 0,
    waitingHues: [],
    graced: false,
    gracedText: "",
    graceUntilMs: 0,
    dismissAtMs: 0,
    expiresAtMs: 0,
    actionsEnabled: false,
    caller: "live",
    glyph: "info",
    sticky: false,
    ...extra,
  };
}

// The pull is a promise, so it resolves a microtask after mount rather than in
// the same frame. Draining is what separates "the surface never pulled" from
// "the test looked too early" - the same lesson U-01's tests recorded.
async function settle() {
  flushSync();
  await new Promise((r) => setTimeout(r, 0));
  flushSync();
}

let host = null;
let app = null;

function show(component) {
  host = document.createElement("div");
  document.body.appendChild(host);
  app = mount(component, { target: host });
}

beforeEach(() => {
  reset();
  clearListeners();
  // jsdom has neither, and both surfaces measure themselves on mount. The same
  // two stubs card.test.js and failure.test.js install, for the same reason.
  window.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  Element.prototype.getBoundingClientRect = () => ({
    height: 100, width: 400, top: 0, left: 0, right: 400, bottom: 100, x: 0, y: 0,
  });
});

afterEach(() => {
  if (app) unmount(app);
  if (host) host.remove();
  app = null;
  host = null;
});

describe.each([
  ["card", Card],
  ["toast", Toast],
])("%s", (name, Component) => {
  test("pulls the view on mount, so a push it never saw is not a blank window", async () => {
    stub("View", () => view());
    show(Component);
    await settle();

    expect(sent("View").length).toBe(1);
    expect(host.textContent).toContain("Where should 2026.7.30 go first?");
  });

  test("pulls AFTER saying it is ready, so the push stays the fast path", async () => {
    stub("View", () => view());
    show(Component);
    await settle();

    const order = calls.map((c) => c.name.slice(SVC.length));
    expect(order.indexOf("Ready")).toBeGreaterThanOrEqual(0);
    expect(order.indexOf("Ready")).toBeLessThan(order.indexOf("View"));
  });

  test("a push that arrived first wins; the pull does not overwrite it", async () => {
    // The real race: armCard emitted, the surface got it, and the pull answers a
    // moment later. Re-applying would reset a form the human has begun filling
    // in, so the later answer has to be dropped.
    // The deferred is built HERE, not inside the stub: the stub answers a
    // microtask after the call, so a resolver assigned in its body does not
    // exist yet at the point the test wants to resolve it.
    let resolvePull;
    const pending = new Promise((r) => (resolvePull = r));
    stub("View", () => pending);
    show(Component);
    emit("agentbox:view", view({ id: "pushed", title: "The pushed question" }));
    flushSync();

    resolvePull(view({ id: "pulled", title: "The pulled question" }));
    await settle();

    expect(host.textContent).toContain("The pushed question");
    expect(host.textContent).not.toContain("The pulled question");
  });

  test("an empty view is not painted as a card", async () => {
    // A window can outlive its item. Painting whatever came back would put an
    // empty shell on screen, which is the defect from the other direction.
    stub("View", () => ({ item: null }));
    show(Component);
    await settle();

    expect(host.textContent).not.toContain("Where should");
  });

  test("a pull that rejects leaves the surface alone rather than unhandled", async () => {
    // The window can be torn down between mount and answer. U-01 put a wrapper on
    // the answer path for exactly this; the pull is not on that path, so it
    // catches its own - and must not throw into the window.
    stub("View", () => {
      throw new Error("window went away");
    });
    show(Component);
    await settle();

    emit("agentbox:view", view({ title: "Still works" }));
    flushSync();
    expect(host.textContent).toContain("Still works");
  });
});
