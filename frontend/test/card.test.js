import { describe, test, expect, beforeEach, afterEach } from "vitest";
import { mount, unmount, flushSync } from "svelte";
import Card from "../src/surfaces/Card.svelte";
import { calls, reset, clearListeners, emit } from "./stubs/wailsio-runtime.js";

// The first test that renders a surface (robustness.md R-40). Everything in
// docs/backlog/ux.md was found by reading, because until this file nothing in the
// repository executed a line of Svelte.
//
// What it asserts is deliberately the answer path and not the pixels. jsdom has no
// layout engine, so "is the button in the right place" is unanswerable here and
// stays a job for a human looking at a window. "Does pressing 1 answer the
// question" is answerable, was not being asked, and is what the product is.

const SVC = "github.com/borismilner/agentbox/internal/webui.Bridge.";
const sent = (method) => calls.filter((c) => c.name === SVC + method);

// The payload the daemon pushes, built from the Go structs it is serialised from:
// cardView in internal/webui/webui.go:56 wrapping proto.Item at
// internal/proto/types.go:178. Kept as one factory so a shape change breaks in one
// place rather than in every test.
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

// The card queues its re-measure with queueMicrotask, so a flushSync alone leaves
// the mount-time measurement still in flight. Draining matters: without this the
// U-06 test below counted that late arrival as a response to the select change and
// reported the defect as fixed. A macrotask is the drain to use - counting how many
// ticks a queueMicrotask and an await add up to is a guess that goes stale.
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

function show(v) {
  host = document.createElement("div");
  document.body.appendChild(host);
  app = mount(Card, { target: host });
  emit("agentbox:view", v);
  flushSync();
}

beforeEach(() => {
  reset();
  // jsdom implements neither, and the card measures itself with both. The stubs
  // are inert on purpose: what these tests assert is which state changes ASK for a
  // measurement, which is exactly the defect U-06 describes, and that question is
  // independent of any real height.
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

describe("a choice card", () => {
  test("renders the question, every option, and the number that answers it", () => {
    show(
      view({
        options: [
          { label: "staging", desc: "the usual first stop" },
          { label: "canary", desc: "5% of traffic" },
          { label: "straight to production", desc: "" },
        ],
        default: "canary",
      }),
    );

    expect(host.textContent).toContain("Where should 2026.7.30 go first?");
    const opts = [...host.querySelectorAll("button.opt")];
    expect(opts).toHaveLength(3);
    expect(opts.map((b) => b.querySelector("kbd").textContent)).toEqual(["1", "2", "3"]);
    expect(opts[1].textContent).toContain("canary");
    expect(opts[1].textContent).toContain("5% of traffic");
    // The default is the one the footer promises Enter will send, so it has to be
    // the one wearing the primary mark.
    expect(opts[1].classList.contains("primary")).toBe(true);
    expect(host.textContent).toContain("release-bot");
  });

  test("the number key answers with that option's label", () => {
    show(view({ options: [{ label: "staging" }, { label: "canary" }] }));
    key("2");
    expect(sent("Answer")).toHaveLength(1);
    expect(sent("Answer")[0].args).toEqual(["itm-1", "canary"]);
  });

  test("a number past the last option answers nothing at all", () => {
    show(view({ options: [{ label: "staging" }, { label: "canary" }] }));
    key("7");
    expect(sent("Answer")).toHaveLength(0);
  });

  test("Enter sends the default, and sends nothing when there is no default", () => {
    show(view({ options: [{ label: "staging" }], default: "staging" }));
    key("Enter");
    expect(sent("Answer")[0].args).toEqual(["itm-1", "staging"]);

    reset();
    key("Escape"); // clear the card's state between the two halves
    unmount(app);
    host.remove();
    app = host = null;
    clearListeners();

    show(view({ options: [{ label: "staging" }] }));
    key("Enter");
    expect(sent("Answer")).toHaveLength(0);
  });
});

// The bug Boris reported in his own words: "No matter how many times I press Esc,
// it pops back up." Deferring a notification puts a thing nobody can answer back on
// the queue, and escalation raises it again. Card.svelte:240-257 is the fix; this is
// the test it never had.
describe("Escape means different things and has to", () => {
  test("dismisses a notification rather than deferring it", () => {
    show(view({ kind: "notify", title: "deploy finished" }));
    key("Escape");
    expect(sent("Dismiss")).toHaveLength(1);
    expect(sent("Defer")).toHaveLength(0);
  });

  test("defers a question", () => {
    show(view({ options: [{ label: "staging" }] }));
    key("Escape");
    expect(sent("Defer")).toHaveLength(1);
    expect(sent("Dismiss")).toHaveLength(0);
  });

  test("shift+Escape forces dismiss on a question", () => {
    show(view({ options: [{ label: "staging" }] }));
    key("Escape", { shiftKey: true });
    expect(sent("Dismiss")).toHaveLength(1);
    expect(sent("Defer")).toHaveLength(0);
  });

  test("the header hint names whichever of the two this card does", () => {
    show(view({ kind: "notify" }));
    expect(host.querySelector(".hint").textContent.replace(/\s+/g, " ")).toContain("Esc dismiss");
  });
});

describe("a confirm card", () => {
  test("y and n both answer, and say which", () => {
    show(view({ kind: "confirm", title: "Run the migration?" }));
    key("y");
    expect(sent("Confirm")[0].args).toEqual(["itm-1", true]);
    key("n");
    expect(sent("Confirm")[1].args).toEqual(["itm-1", false]);
  });
});

// U-06 in docs/backlog/ux.md. The ResizeObserver can only see the card grow, and
// Card.svelte:186-193 compensates with a hand-written list of things that shrink
// it. formValues is not on that list, so choosing a short option after a long one
// removes the spelled-out line and leaves the window tall.
//
// test.fails is not a workaround: it records that this is what should happen and
// today does not. Fixing U-06 turns it red, which is the prompt to delete this
// comment and the marker with it.
describe("a form card that gets shorter", () => {
  test.fails("asks to be re-measured when a long choice is replaced by a short one", async () => {
    show(
      view({
        kind: "form",
        title: "Where does this go?",
        fields: [
          {
            key: "target",
            label: "Target",
            type: "choice",
            options: [
              "a deployment target whose name does not fit the closed select",
              "prod",
            ],
          },
        ],
      }),
    );

    // The long option is chosen first, so the spelled-out line is on screen.
    expect(host.querySelector(".spelled")).not.toBeNull();

    await settle();
    const before = sent("Fit").length;
    const select = host.querySelector("select");
    select.value = "prod";
    select.dispatchEvent(new Event("change", { bubbles: true }));
    select.dispatchEvent(new Event("input", { bubbles: true }));
    await settle();

    // The line is gone, so the card is shorter than the window it is in.
    expect(host.querySelector(".spelled")).toBeNull();
    expect(sent("Fit").length).toBeGreaterThan(before);
  });
});
