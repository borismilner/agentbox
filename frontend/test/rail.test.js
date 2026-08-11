import { describe, test, expect, beforeEach, afterEach } from "vitest";
import { mount, unmount, flushSync } from "svelte";
import App from "../src/surfaces/App.svelte";
import { calls, reset, Call, clearListeners, emit } from "./stubs/wailsio-runtime.js";

// U-13 in docs/backlog/ux.md. The rail was nine icon buttons reachable only by
// clicking or by tabbing through in order, and the current one was marked with a
// CSS class and nothing else - so the most repeated navigation in the app cost a
// pointer or up to nine Tab stops, and a screen reader was given nine similar
// buttons with no statement of which was active.
//
// The shell is mounted whole rather than the rail alone, because the shortcut
// lives in the shell and has to work while another surface holds the keyboard.
// The last test is the reason that matters: the inbox reads a bare digit as an
// answer to the selected question.

const SVC = "github.com/borismilner/agentbox/internal/webui.Bridge.";
const sent = (m) => calls.filter((c) => c.name === SVC + m);

let host = null;
let app = null;

function shell() {
  host = document.createElement("div");
  document.body.appendChild(host);
  app = mount(App, { target: host });
  flushSync();
}

const rail = () => host.querySelector("nav.rail");
const buttons = () => [...rail().querySelectorAll("button")];
const current = () => rail().querySelector('button[aria-current="page"]');
const named = (name) => buttons().find((b) => b.getAttribute("aria-label") === name);

function key(k, over = {}) {
  window.dispatchEvent(new KeyboardEvent("keydown", { key: k, bubbles: true, ...over }));
  flushSync();
}

function item(over = {}) {
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
    hint: "1 staging · 2 canary",
    createdMs: Date.now(),
    muted: false,
    ...over,
  };
}

const snapshot = (items) => ({ items, pending: items.filter((i) => i.pending).length, today: 1, muted: [] });

beforeEach(() => {
  reset();
  clearListeners();
  Element.prototype.scrollIntoView = () => {};
  // Every surface the rail can reach is mounted by these tests, so the calls
  // they make on the way in have to answer with the shape Go sends.
  Call.byName.set(SVC + "Library", () => []);
});

afterEach(() => {
  if (app) unmount(app);
  host?.remove();
  app = host = null;
  clearListeners();
});

describe("the rail says which surface is current", () => {
  test("the nav is named and exactly one button is marked", () => {
    shell();
    expect(rail().getAttribute("aria-label")).toBe("Surfaces");
    expect(rail().querySelectorAll("[aria-current]")).toHaveLength(1);
    expect(current().getAttribute("aria-label")).toBe("Home");
  });

  test("the mark follows a click", () => {
    shell();
    named("Library").click();
    flushSync();
    expect(current().getAttribute("aria-label")).toBe("Library");
    expect(rail().querySelectorAll("[aria-current]")).toHaveLength(1);
  });

  test("an icon button carries its own name and its shortcut", () => {
    shell();
    expect(buttons().map((b) => b.getAttribute("aria-label"))).toEqual([
      "Home",
      "Session",
      "Agents",
      "Assignments",
      "Inbox",
      "History",
      "Viewer",
      "Library",
      "Settings",
    ]);
    expect(buttons().map((b) => b.getAttribute("aria-keyshortcuts"))).toEqual([
      "Control+1",
      "Control+2",
      "Control+3",
      "Control+4",
      "Control+5",
      "Control+6",
      "Control+7",
      "Control+8",
      "Control+9",
    ]);
  });

  test("a count that is only painted is still said", () => {
    shell();
    expect(named("Inbox")).toBeTruthy();

    emit("agentbox:inbox", snapshot([item(), item({ id: "itm-2" }), item({ id: "itm-3" })]));
    flushSync();

    expect(named("Inbox, 3 waiting")).toBeTruthy();
    emit("agentbox:agents", { agents: [{ key: "a" }, { key: "b" }] });
    flushSync();
    expect(named("Agents, 2 attached")).toBeTruthy();
  });
});

describe("Ctrl+1..9 walks the rail", () => {
  test("the number is the icon's place, top to bottom", () => {
    shell();
    key("3", { ctrlKey: true });
    expect(current().getAttribute("aria-label")).toBe("Agents");

    key("9", { ctrlKey: true });
    expect(current().getAttribute("aria-label")).toBe("Settings");
    expect(current()).toBe(buttons().at(-1));

    key("1", { ctrlKey: true });
    expect(current().getAttribute("aria-label")).toBe("Home");
  });

  test("a bare digit belongs to the surface in front", () => {
    shell();
    key("3");
    expect(current().getAttribute("aria-label")).toBe("Home");
  });

  test("a second modifier is somebody else's chord", () => {
    shell();
    key("3", { ctrlKey: true, shiftKey: true });
    key("3", { ctrlKey: true, altKey: true });
    expect(current().getAttribute("aria-label")).toBe("Home");
  });

  test("switching surfaces from the keyboard does not answer the question under it", () => {
    Call.byName.set(SVC + "Triage", () => true);
    shell();
    emit("agentbox:inbox", snapshot([item()]));
    flushSync();

    key("5", { ctrlKey: true });
    expect(current().getAttribute("aria-label")).toBe("Inbox, 1 waiting");

    // The inbox is in front, with a pending question selected, and its own
    // handler sees this keystroke too: "1" is the first answer to that question.
    key("1", { ctrlKey: true });

    expect(sent("Triage")).toHaveLength(0);
    expect(current().getAttribute("aria-label")).toBe("Home");
  });
});
