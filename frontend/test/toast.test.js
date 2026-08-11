import { describe, test, expect, beforeEach, afterEach } from "vitest";
import { mount, unmount, flushSync } from "svelte";
import Toast from "../src/surfaces/Toast.svelte";
import { Call, calls, reset, emit } from "./stubs/wailsio-runtime.js";
import { forget } from "../src/lib/trouble.svelte.js";

// The toast window is REUSED across items, and both defects here come from that.
//
// U-07: the clamp is measured with `scrollHeight`, a DOM read Svelte cannot track,
// so a new item's body landed in the same node with the previous item's answer still
// on screen - a long notice clipped with nothing saying so, or a short one offering
// to expand into nothing.
//
// U-08: once expanded, every click in the strip dismissed. The expanded body scrolls
// and is selectable, so clicking a scrollbar or placing a cursor destroyed the notice
// the human had just opened in order to read.

const SVC = "github.com/borismilner/agentbox/internal/webui.Bridge.";
const sent = (method) => calls.filter((c) => c.name === SVC + method);

function view(item = {}, extra = {}) {
  return {
    item: {
      id: "itm-1",
      kind: "notify",
      level: "info",
      title: "build done",
      body: "",
      identity: { agent: "release-bot", project: "checkout-api" },
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

// jsdom lays nothing out, so the one measurement this file is about has to be
// supplied: a body is "long" when it holds more text than three lines would.
function layout() {
  Object.defineProperty(Element.prototype, "scrollHeight", {
    configurable: true,
    get() {
      return (this.textContent ?? "").length > 60 ? 240 : 40;
    },
  });
  Object.defineProperty(Element.prototype, "clientHeight", {
    configurable: true,
    get() {
      return this.classList?.contains("open") ? 240 : 40;
    },
  });
}

async function settle() {
  flushSync();
  await new Promise((r) => requestAnimationFrame(r));
  flushSync();
}

let host = null;
let app = null;

function show(v) {
  host = document.createElement("div");
  document.body.appendChild(host);
  app = mount(Toast, { target: host });
  emit("agentbox:view", v);
  flushSync();
}

const short = "one line.";
const long = "a notice long enough to be clipped, ".repeat(6);

beforeEach(() => {
  reset();
  forget();
  layout();
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
});

describe("a reused toast measures the item it is showing (U-07)", () => {
  test("a long notice after a short one offers to expand", async () => {
    show(view({ id: "a" }, { bodyHtml: short }));
    await settle();
    expect(host.querySelector(".more"), "a short body should not offer to expand").toBeNull();

    emit("agentbox:view", view({ id: "b" }, { bodyHtml: long }));
    await settle();

    expect(host.querySelector(".more"), "the long body arrived with no way to read the rest").not.toBeNull();
  });

  test("a short notice after a long one stops offering", async () => {
    show(view({ id: "a" }, { bodyHtml: long }));
    await settle();
    expect(host.querySelector(".more")).not.toBeNull();

    emit("agentbox:view", view({ id: "b" }, { bodyHtml: short }));
    await settle();

    expect(host.querySelector(".more"), "it offered to expand into nothing").toBeNull();
  });
});

describe("an expanded toast is for reading, not for dismissing (U-08)", () => {
  // sticky is the level's own business (a warning waits, an info self-closes), so
  // it is a parameter here: the close button used to appear for sticky notices ONLY,
  // which is exactly the case that would hide this defect.
  async function expand(sticky = true) {
    show(view({ id: "a" }, { bodyHtml: long, sticky }));
    await settle();
    host.querySelector(".toast").click();
    await settle();
    expect(host.querySelector(".body.open"), "the first click should expand").not.toBeNull();
  }

  test("a click in the body does not take it away", async () => {
    await expand();

    host.querySelector(".body").click();
    await settle();

    expect(sent("Dismiss"), "clicking into the text dismissed the notice").toHaveLength(0);
  });

  test("a click anywhere in the strip does not take it away either", async () => {
    await expand();

    host.querySelector(".toast").click();
    await settle();

    expect(sent("Dismiss")).toHaveLength(0);
  });

  test("the close button is there instead, and it works", async () => {
    await expand(false);

    const x = host.querySelector("button.x");
    expect(x, "an expanded strip with no dismiss control is a trap").not.toBeNull();
    x.click();
    await settle();

    expect(sent("Dismiss")).toHaveLength(1);
  });

  test("and the keyboard still dismisses", async () => {
    await expand();

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await settle();

    expect(sent("Dismiss")).toHaveLength(1);
  });

  test("a strip with nothing hidden still dismisses on a click", async () => {
    show(view({ id: "a" }, { bodyHtml: short }));
    await settle();

    host.querySelector(".toast").click();
    await settle();

    expect(sent("Dismiss"), "the ordinary one-click dismissal is the whole gesture").toHaveLength(1);
  });
});

// The stub records calls; nothing here needs a reply.
Call.byName.set(SVC + "Dismiss", () => {});
