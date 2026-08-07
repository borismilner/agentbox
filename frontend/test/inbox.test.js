import { describe, test, expect, beforeEach, afterEach } from "vitest";
import { mount, unmount, flushSync } from "svelte";
import Inbox from "../src/surfaces/Inbox.svelte";
import { calls, reset, Call, clearListeners } from "./stubs/wailsio-runtime.js";

// U-03 in docs/backlog/ux.md, and the second surface anything has mounted.
//
// `Triage` is the one method on the whole answer path that returns a value
// (internal/webui/inbox.go:588-613 returns false when the item is gone, is no
// longer pending, or the key means nothing for its kind). The surface used to
// discard it, so a keystroke that did nothing looked exactly like one that worked.

const SVC = "github.com/borismilner/agentbox/internal/webui.Bridge.";
const sent = (m) => calls.filter((c) => c.name === SVC + m);

function row(over = {}) {
  return {
    id: "itm-1",
    title: "Where should 2026.7.30 go first?",
    snippet: "",
    agent: "release-bot",
    project: "checkout-api",
    kind: "choice",
    level: "info",
    pending: true,
    outcome: "waiting",
    tone: "info",
    hue: "hsl(205 62% 68%)",
    hint: "1 staging · 2 canary · d dismiss · c copy",
    createdMs: Date.now(),
    muted: false,
    ...over,
  };
}

const snapshot = (items) => ({ items, pending: items.filter((i) => i.pending).length, today: 3, muted: [] });

let host = null;
let app = null;

function show(items) {
  host = document.createElement("div");
  document.body.appendChild(host);
  app = mount(Inbox, { target: host, props: { inbox: snapshot(items) } });
  flushSync();
}

async function key(k) {
  window.dispatchEvent(new KeyboardEvent("keydown", { key: k, bubbles: true }));
  flushSync();
  // triage awaits the daemon, so the refusal lands after the microtask queue
  // drains. A macrotask is the only drain that is not a guess about how many
  // ticks the stub and the await add up to.
  await new Promise((r) => setTimeout(r, 0));
  flushSync();
}

const hint = () => host.querySelector(".hint")?.textContent?.replace(/\s+/g, " ").trim() ?? "";

beforeEach(() => {
  reset();
  Element.prototype.scrollIntoView = () => {};
});

afterEach(() => {
  if (app) unmount(app);
  host?.remove();
  app = host = null;
  clearListeners();
});

describe("triage", () => {
  test("sends the keystroke rather than a decision, so Go keeps the meaning", async () => {
    Call.byName.set(SVC + "Triage", () => true);
    show([row()]);
    await key("1");
    expect(sent("Triage")).toHaveLength(1);
    expect(sent("Triage")[0].args).toEqual(["itm-1", "1"]);
  });

  test("says so when Go declines the key", async () => {
    Call.byName.set(SVC + "Triage", () => false);
    show([row()]);
    expect(hint()).toContain("1 staging");

    await key("y");
    expect(hint()).toBe("y does nothing to this one");
  });

  test("stays quiet when Go accepts it", async () => {
    Call.byName.set(SVC + "Triage", () => true);
    show([row()]);
    await key("d");
    expect(hint()).toContain("1 staging");
    expect(hint()).not.toContain("does nothing");
  });

  test("names Enter and Backspace in words, not as key codes", async () => {
    Call.byName.set(SVC + "Triage", () => false);
    show([row()]);
    await key("Enter");
    expect(hint()).toBe("enter does nothing to this one");
  });

  test("a refusal does not follow the selection to another row", async () => {
    Call.byName.set(SVC + "Triage", () => false);
    show([row(), row({ id: "itm-2", title: "second", hint: "d dismiss" })]);
    await key("y");
    expect(hint()).toContain("does nothing");

    await key("j");
    // The hint now belongs to row two, which was never refused anything.
    expect(hint()).toBe("d dismiss");
  });

  test("a call that never reached the daemon reads as a key that did nothing", async () => {
    Call.byName.set(SVC + "Triage", () => {
      throw new Error("no such method");
    });
    show([row()]);
    await key("y");
    // The honest report: whatever went wrong, the keystroke did not answer
    // anything. The alternative is the silence U-01 is about.
    expect(hint()).toBe("y does nothing to this one");
  });

  test("pressing a dead key twice re-states it rather than leaving it stale", async () => {
    Call.byName.set(SVC + "Triage", () => false);
    show([row()]);
    await key("y");
    await key("y");
    expect(sent("Triage")).toHaveLength(2);
    expect(hint()).toBe("y does nothing to this one");
  });
});

describe("the list itself", () => {
  test("separates pending from recent and counts what is waiting", () => {
    show([row(), row({ id: "itm-2", title: "build passed", pending: false, outcome: "dismissed" })]);
    const sections = [...host.querySelectorAll(".section")].map((s) => s.textContent);
    expect(sections).toEqual(["Pending", "Recent"]);
    expect(host.textContent).toContain("1 pending");
  });

  test("an empty queue says so rather than showing an empty box", () => {
    show([]);
    expect(host.textContent).toContain("Nothing yet");
  });
});
