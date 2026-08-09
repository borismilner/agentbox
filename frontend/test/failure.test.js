import { describe, test, expect, beforeEach, afterEach } from "vitest";
import { mount, unmount, flushSync } from "svelte";
import Card from "../src/surfaces/Card.svelte";
import Toast from "../src/surfaces/Toast.svelte";
import { Call, calls, reset, clearListeners, emit } from "./stubs/wailsio-runtime.js";
import { forget } from "../src/lib/trouble.svelte.js";

// U-01. The card made 26 calls to the daemon and awaited, caught and handled none
// of them, so a rejected call left the card exactly as it was: pressing the key
// looked the same as the daemon thinking about it. U-02 then gave the daemon a
// way to refuse in words, which is the second thing that has to reach a surface.
//
// These are the tests the audit named: mount the card with a bridge stub whose
// answer rejects, press the key, and assert something in the DOM says so.

const SVC = "github.com/borismilner/agentbox/internal/webui.Bridge.";
const sent = (method) => calls.filter((c) => c.name === SVC + method);
const stub = (method, fn) => Call.byName.set(SVC + method, fn);

function view(item = {}, extra = {}) {
  return {
    item: {
      id: "itm-1",
      kind: "choice",
      level: "info",
      title: "Where should 2026.7.30 go first?",
      body: "",
      identity: { agent: "release-bot", project: "checkout-api" },
      options: [{ label: "staging" }, { label: "canary" }],
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

// The call is a promise and the wrapper awaits it, so the notice lands a
// microtask after the keystroke rather than in the same frame. Draining is what
// separates "the surface never says anything" from "the test looked too early".
async function settle() {
  flushSync();
  await new Promise((r) => setTimeout(r, 0));
  flushSync();
}

function key(k, opts = {}) {
  window.dispatchEvent(new KeyboardEvent("keydown", { key: k, bubbles: true, ...opts }));
  flushSync();
}

let host = null;
let app = null;

function show(component, v) {
  host = document.createElement("div");
  document.body.appendChild(host);
  app = mount(component, { target: host });
  emit("agentbox:view", v);
  flushSync();
}

beforeEach(() => {
  reset();
  // The notice is one shared line per window, so a test that left one up would
  // hand it to the next test.
  forget();
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
  host?.remove();
  app = host = null;
  clearListeners();
});

describe("a card whose answer does not land", () => {
  test("a rejected call is said on the card instead of going to a console nobody reads", async () => {
    stub("Answer", () => {
      throw new Error("window is gone");
    });
    show(Card, view());

    key("1");
    await settle();

    const said = host.querySelector(".trouble");
    expect(said).not.toBeNull();
    expect(said.textContent).toContain("agentbox did not take that");
    // The reason the runtime gave is kept: "something went wrong" would send the
    // reader to a log this line exists to replace.
    expect(said.textContent).toContain("window is gone");
  });

  test("a refusal the daemon put in words is shown as the daemon wrote it", async () => {
    stub("Answer", () => "that item has already ended, so there was nothing left to answer.");
    show(Card, view());

    key("1");
    await settle();

    expect(host.querySelector(".trouble").textContent).toContain("already ended");
  });

  test("an answer that lands says nothing at all", async () => {
    show(Card, view());
    key("1");
    await settle();

    expect(sent("Answer")).toHaveLength(1);
    expect(host.querySelector(".trouble")).toBeNull();
  });

  test("a keystroke that works clears the last failure", async () => {
    stub("Answer", () => "that item has already ended, so there was nothing left to answer.");
    show(Card, view());
    key("1");
    await settle();
    expect(host.querySelector(".trouble")).not.toBeNull();

    Call.byName.clear();
    key("2");
    await settle();
    expect(host.querySelector(".trouble")).toBeNull();
  });

  test("a fresh question arrives with a clean slate", async () => {
    stub("Answer", () => "that item has already ended, so there was nothing left to answer.");
    show(Card, view());
    key("1");
    await settle();
    expect(host.querySelector(".trouble")).not.toBeNull();

    emit("agentbox:view", view({ id: "itm-2", title: "Next one" }));
    await settle();
    expect(host.querySelector(".trouble")).toBeNull();
  });

  test("the notice can be put away without answering the card", async () => {
    stub("Defer", () => "nothing else is waiting, so there is nothing to move this behind.");
    show(Card, view());

    key("Escape");
    await settle();
    expect(host.querySelector(".trouble")).not.toBeNull();

    host.querySelector(".trouble .shut").click();
    flushSync();
    expect(host.querySelector(".trouble")).toBeNull();
    // Putting the notice away is not answering: the question is still there.
    expect(host.textContent).toContain("Where should 2026.7.30 go first?");
  });

  test("the card re-measures when the notice appears, so it is not clipped off a frameless window", async () => {
    stub("Answer", () => "that item has already ended, so there was nothing left to answer.");
    show(Card, view());
    await settle();

    const before = sent("Fit").length;
    key("1");
    await settle();

    expect(host.querySelector(".trouble")).not.toBeNull();
    expect(sent("Fit").length).toBeGreaterThan(before);
  });

  // Every key on the card goes through the same wrapper, so the failure path is
  // the wrapper's and not each handler's. One case per verb the card owns.
  test.each([
    ["Escape", "Defer", { kind: "choice" }],
    ["Escape", "Dismiss", { kind: "notify" }],
    ["y", "Confirm", { kind: "confirm" }],
    ["Enter", "Veto", { kind: "veto" }],
    ["a", "Review", { kind: "diff", diff: "" }],
  ])("pressing %s on a %s that refuses says so", async (k, method, item) => {
    stub(method, () => "that item has already ended, so there was nothing left to answer.");
    show(Card, view(item));

    key(k);
    await settle();

    expect(sent(method).length).toBeGreaterThan(0);
    expect(host.querySelector(".trouble")).not.toBeNull();
  });

  test("a render error wears itself on the card rather than freezing it silently", async () => {
    show(Card, view());
    window.dispatchEvent(new ErrorEvent("error", { message: "boom" }));
    await settle();

    expect(host.querySelector(".trouble").textContent).toContain("boom");
  });
});

describe("a toast whose dismiss does not land", () => {
  test("says so instead of sitting there looking unclicked", async () => {
    stub("Dismiss", () => {
      throw new Error("window is gone");
    });
    show(Toast, view({ kind: "notify", title: "build done" }));

    key("Escape");
    await settle();

    expect(sent("Dismiss")).toHaveLength(1);
    expect(host.querySelector(".trouble").textContent).toContain("agentbox did not take that");
  });

  test("a dismiss that lands says nothing", async () => {
    show(Toast, view({ kind: "notify", title: "build done" }));
    key("Escape");
    await settle();

    expect(sent("Dismiss")).toHaveLength(1);
    expect(host.querySelector(".trouble")).toBeNull();
  });
});
