import { describe, test, expect, beforeEach, afterEach, vi } from "vitest";
import { mount, unmount, flushSync } from "svelte";
import Card from "../src/surfaces/Card.svelte";
import Toast from "../src/surfaces/Toast.svelte";
import { calls, reset, clearListeners, emit } from "./stubs/wailsio-runtime.js";

// U-14: `c` copied the item on four surfaces and was named on none of the two
// that write their own hints.
//
// The other two name it already, and from Go: the inbox row and the inline ask
// panel both render triageHint (internal/webui/inbox.go), which has ended in
// "c copy" since it was written and is asserted there by
// TestTriageHintStatesTheKeysThatWork. So this file is about the card and the
// toast, and about the rule the two of them now follow: a key is named where
// pressing it would do the thing, and nowhere else.
//
// Every test drives the real keystroke as well as reading the hint. A hint is a
// promise about a keystroke, and the two halves are worth nothing apart - the
// defect this file guards against is a line that says `c copy` on a surface
// where c does something else, which is what an unconditional hint would have
// produced on a card with a focused form field.

const SVC = "github.com/borismilner/agentbox/internal/webui.Bridge.";
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

let host = null;
let app = null;

function show(Surface, v) {
  host = document.createElement("div");
  document.body.appendChild(host);
  app = mount(Surface, { target: host });
  emit("agentbox:view", v);
  flushSync();
}

// The form card focuses its first field from a queueMicrotask, so a flushSync
// alone leaves the focus still in flight and the hint still on screen.
async function settle() {
  flushSync();
  await new Promise((r) => setTimeout(r, 0));
  flushSync();
}

// A keystroke from a given element, which is the part that matters: the card
// reads e.target to decide whether it is being typed into, so a key dispatched
// at the window would take the branch a focused field never does.
function key(k, from = window) {
  from.dispatchEvent(new KeyboardEvent("keydown", { key: k, bubbles: true }));
  flushSync();
}

const text = (sel) => host.querySelector(sel)?.textContent?.replace(/\s+/g, " ").trim() ?? "";

beforeEach(() => {
  reset();
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

describe("the card names the key that copies (U-14)", () => {
  test("the footer says so", () => {
    show(Card, view());
    expect(text("footer")).toContain("c copy");
  });

  test("and the key it names copies this item", () => {
    show(Card, view({ id: "itm-9" }));
    key("c");
    expect(sent("Copy")).toHaveLength(1);
    expect(sent("Copy")[0].args).toEqual(["itm-9"]);
  });

  test("it is on the kinds that have no other keys to spare, too", () => {
    show(Card, view({ kind: "notify" }));
    expect(text("footer")).toContain("c copy");
    key("c");
    expect(sent("Copy")).toHaveLength(1);
  });

  test("a field with the keyboard takes the line away, because c types a c there", async () => {
    show(Card, view({ kind: "form", fields: [{ key: "tag", label: "Tag", type: "text" }] }));
    await settle();

    const input = host.querySelector("input.text");
    expect(document.activeElement, "the form card focuses its first field").toBe(input);
    expect(text("footer"), "the card offered a key the field was about to swallow").not.toContain("c copy");

    key("c", input);
    expect(sent("Copy"), "c inside a field must reach the field, not the clipboard").toHaveLength(0);
  });

  test("the platform's own copy is left alone", () => {
    show(Card, view());
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "c", ctrlKey: true, bubbles: true }));
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "c", metaKey: true, bubbles: true }));
    flushSync();
    // The body is selectable, so a modified c belongs to whatever the reader has
    // selected. Taking it would put the whole item on the clipboard instead.
    expect(sent("Copy")).toHaveLength(0);
  });

  test("and the line comes back when the field gives the keyboard up", async () => {
    show(Card, view({ kind: "form", fields: [{ key: "tag", label: "Tag", type: "text" }] }));
    await settle();
    expect(text("footer")).not.toContain("c copy");

    host.querySelector("input.text").blur();
    flushSync();

    expect(text("footer")).toContain("c copy");
    key("c");
    expect(sent("Copy")).toHaveLength(1);
  });
});

describe("the toast names its keys only while it can hear them (U-14)", () => {
  // A toast is the product's one _NET_WM_WINDOW_TYPE_NOTIFICATION window
  // (internal/webui/x11.go): it maps without asking for focus, and a WM need
  // never give a notification the keyboard. Naming a key on a strip that cannot
  // receive it is worse than the silence, so the line is tied to focus.
  test("a strip without the keyboard stays as bare as it was", () => {
    vi.spyOn(document, "hasFocus").mockReturnValue(false);
    show(Toast, view({ kind: "notify" }));
    expect(host.querySelector(".keys"), "a strip that cannot hear a key must not advertise one").toBeNull();
  });

  test("a strip that already has it says so from the first frame", () => {
    vi.spyOn(document, "hasFocus").mockReturnValue(true);
    show(Toast, view({ kind: "notify" }));
    expect(text(".keys")).toBe("c copy · Esc dismiss");
  });

  test("the line arrives with the keyboard and leaves with it", () => {
    vi.spyOn(document, "hasFocus").mockReturnValue(false);
    show(Toast, view({ kind: "notify" }));

    window.dispatchEvent(new FocusEvent("focus"));
    flushSync();
    expect(text(".keys")).toBe("c copy · Esc dismiss");

    window.dispatchEvent(new FocusEvent("blur"));
    flushSync();
    expect(host.querySelector(".keys")).toBeNull();
  });

  test("a control inside the strip giving up focus is not the window losing it", () => {
    vi.spyOn(document, "hasFocus").mockReturnValue(true);
    show(Toast, view({ kind: "notify" }, { sticky: true }));
    const x = host.querySelector("button.x");
    expect(x, "a sticky strip carries the dismiss button this drives").not.toBeNull();

    x.focus();
    x.blur();
    flushSync();

    // focus and blur do not bubble, so the window listener must never see these.
    // If it did, tabbing off the ✕ would take the hint away while the strip
    // still had the keyboard.
    expect(text(".keys")).toBe("c copy · Esc dismiss");
  });

  test("both keys it names do what it says", () => {
    vi.spyOn(document, "hasFocus").mockReturnValue(true);
    show(Toast, view({ id: "itm-4", kind: "notify" }));
    expect(text(".keys")).toContain("c copy");

    key("c");
    expect(sent("Copy")).toHaveLength(1);
    expect(sent("Copy")[0].args).toEqual(["itm-4"]);

    key("Escape");
    expect(sent("Dismiss")).toHaveLength(1);
  });

  test("the strip never spends two lines on the two quiet affordances", async () => {
    // The one reader who gets both: a clipped body AND the keyboard. jsdom lays
    // nothing out, so the clamp is supplied the way toast.test.js supplies it.
    Object.defineProperty(Element.prototype, "scrollHeight", {
      configurable: true,
      get() {
        return (this.textContent ?? "").length > 60 ? 240 : 40;
      },
    });
    Object.defineProperty(Element.prototype, "clientHeight", { configurable: true, get: () => 40 });
    vi.spyOn(document, "hasFocus").mockReturnValue(true);

    show(Toast, view({ kind: "notify" }, { bodyHtml: "a notice long enough to be clipped, ".repeat(6) }));
    flushSync();
    await new Promise((r) => requestAnimationFrame(r));
    flushSync();

    expect(host.querySelector(".more"), "the clamp fixture did not take").not.toBeNull();
    const foot = host.querySelectorAll(".foot");
    expect(foot).toHaveLength(1);
    expect(foot[0].querySelector(".more")).not.toBeNull();
    expect(foot[0].querySelector(".keys")).not.toBeNull();
  });
});
