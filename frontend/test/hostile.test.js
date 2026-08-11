import { describe, test, expect, beforeEach, afterEach } from "vitest";
import { mount, unmount, flushSync } from "svelte";
import Card from "../src/surfaces/Card.svelte";
import { reset, clearListeners, emit } from "./stubs/wailsio-runtime.js";

// R-40, fix (2): mount the card with a payload written to hurt it. The two shapes
// are the ones the audit named, and each one is an open defect rather than a
// hypothetical, so most of what is below is `test.fails` - the marker that says
// this is what should happen and today does not. Fixing R-33 or R-26 turns its
// tests red, which is the prompt to delete the marker with the defect.
//
// jsdom has no layout engine, so nothing here asks where an element ended up. Both
// defects are answerable without one: R-33 is about which attributes survive into
// the document, and R-26 is about how much structure the card builds.

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
  window.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  window.requestAnimationFrame = (fn) => setTimeout(fn, 0);
});

afterEach(() => {
  if (app) unmount(app);
  if (host) host.remove();
  app = host = null;
  clearListeners();
});

// R-33. `parser.WithAttribute()` is on, and goldmark's global filter admits class,
// style, id, title, role, tabindex and any data-*. The payload below is what that
// option makes of one heading, quoted from the entry rather than re-derived: the
// markdown side is Go's business, and what reaches this surface is the html.
const OVERLAY = `<h1 style="position:fixed;inset:0;background:#000;z-index:99999" class="k-alert" data-tone="error">System notice</h1>`;

describe("a heading that wants to be system chrome (R-33)", () => {
  test("the body html reaches the card as it was produced", () => {
    // Not an assertion about the defect: it fixes the harness. If the card ever
    // stops rendering bodyHtml at all, every test below would pass for the wrong
    // reason, and this is the one that would say so.
    show(view({}, { bodyHtml: `<p>ordinary</p>` }));
    expect(host.querySelector(".body")?.textContent).toContain("ordinary");
  });

  test.fails("no element from the body can position itself over the controls", () => {
    show(view({}, { bodyHtml: OVERLAY }));
    const fixed = [...host.querySelectorAll(".body *")].filter(
      (el) => getComputedStyle(el).position === "fixed" || getComputedStyle(el).position === "absolute",
    );
    expect(fixed).toEqual([]);
  });

  test.fails("no element from the body can wear one of agentbox's own classes", () => {
    // The second half of R-33, and the half that is about trust rather than
    // pointer events: `k-alert` is the product's own alert styling, so a heading
    // wearing it reads as agentbox speaking rather than as an agent quoting.
    show(view({}, { bodyHtml: OVERLAY }));
    const chrome = [...host.querySelectorAll(".body *")].filter((el) =>
      [...el.classList].some((c) => c.startsWith("k-")),
    );
    expect(chrome).toEqual([]);
  });

  test("the controls are still in the document, whatever the body did", () => {
    // What is true today: the overlay covers the buttons on screen, it does not
    // remove them. Worth pinning, because a future fix that dropped the controls
    // instead of the attributes would satisfy the two tests above.
    show(view({}, { bodyHtml: OVERLAY }));
    expect(host.querySelectorAll(".opt").length).toBe(2);
  });
});

// R-26. DIFF_CAP bounds rendered LINES and deliberately not structure, so the card
// builds one section and one rail entry per file however many there are. 500 is far
// below the 200,000 the entry costs out at 5 MB, and it is not the interesting
// number: the point is that no bound exists, which 500 shows and 200,000 shows no
// better while costing the gate half a minute. Raise it locally to watch the
// surface die, not here - jsdom renders a card of 2,000 files in about seven
// seconds, and this suite is run on every `make check`.
const FILES = 500;
const bigDiff = () =>
  Array.from(
    { length: FILES },
    (_, i) =>
      `diff --git a/f${i}.txt b/f${i}.txt\n--- a/f${i}.txt\n+++ b/f${i}.txt\n@@ -1 +1 @@\n-old ${i}\n+new ${i}`,
  ).join("\n");

describe("a diff with no bound on its file count (R-26)", () => {
  test.fails("the card caps how many files it renders", () => {
    show(view({ kind: "diff", diff: bigDiff() }));
    // Less than every file, rather than less than some number: any cap at all
    // turns this red, and a number picked here would let a cap of 300 land
    // without anybody noticing it had.
    expect(host.querySelectorAll(".fsec").length).toBeLessThan(FILES);
  });

  test.fails("and says so when it truncates", () => {
    show(view({ kind: "diff", diff: bigDiff() }));
    expect(host.textContent).toMatch(/truncated|more files/i);
  });

  test("the render returns rather than hanging", () => {
    // The deadline half of R-40's fix (2). This one is not a `test.fails`: it
    // passes today at this size and exists so that a change which makes the
    // per-file work superlinear is caught by something other than a person
    // waiting. The bound is deliberately loose - it is a hang detector, not a
    // performance budget, and CI machines differ.
    const started = performance.now();
    show(view({ kind: "diff", diff: bigDiff() }));
    const elapsed = performance.now() - started;
    // Deliberately says nothing about how many sections there are: that is the
    // test above's business, and a count here would have to be edited by whoever
    // adds the cap for a reason that has nothing to do with hanging.
    expect(host.querySelector(".fsec")).not.toBeNull();
    expect(elapsed).toBeLessThan(10_000);
  });
});
