import { describe, test, expect, beforeEach, afterEach, vi } from "vitest";
import { mount, unmount, flushSync } from "svelte";
import Panel from "../src/surfaces/Panel.svelte";
import { calls, reset, emit, clearListeners } from "./stubs/wailsio-runtime.js";

// U-09: the panel ended an idle session on the FIRST click.
//
// Closing kills the child and takes an unsaved conversation with it, and the app
// window says exactly that about exactly the same session. The panel asked only
// when the agent was mid-turn, which made the weaker guard the one you meet by
// hotkey while doing something else.
//
// These tests drive the real button, not the close() function: the defect lived
// in the condition the click handler evaluates, so a test that calls close()
// directly would have passed against the broken build too.

const SVC = "github.com/borismilner/agentbox/internal/webui.Bridge.";
const closed = () => calls.filter((c) => c.name === SVC + "CloseSession");

let host = null;
let app = null;

function session(extra = {}) {
  return {
    id: "s-1",
    title: "port the settings surface",
    state: "idle",
    mode: "full",
    turns: 12,
    hue: "#8ab4f8",
    selected: true,
    ...extra,
  };
}

function panel(sessions) {
  host = document.createElement("div");
  document.body.appendChild(host);
  app = mount(Panel, { target: host });
  emit("agentbox:sessions", sessions);
  flushSync();
}

// The ✕ on a session chip. The chips carry other buttons (rename, + New, Load),
// so this selects by class rather than by position.
const xs = () => [...host.querySelectorAll("button.x")];

function click(el) {
  el.click();
  flushSync();
}

beforeEach(() => {
  reset();
  clearListeners();
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  if (app) unmount(app);
  host?.remove();
  app = null;
  host = null;
});

describe("the panel asks before it ends a session (U-09)", () => {
  test("one click on an IDLE session does not close it", () => {
    panel([session({ state: "idle" })]);
    const x = xs()[0];
    expect(x).toBeTruthy();

    click(x);

    expect(closed()).toHaveLength(0);
    expect(x.textContent.trim()).toBe("sure?");
  });

  test("the second click on an idle session closes it", () => {
    panel([session({ state: "idle" })]);
    click(xs()[0]);
    click(xs()[0]);

    expect(closed()).toHaveLength(1);
    expect(closed()[0].args).toEqual(["s-1"]);
  });

  test("a working session still takes two clicks", () => {
    panel([session({ state: "working" })]);
    click(xs()[0]);
    expect(closed()).toHaveLength(0);

    click(xs()[0]);
    expect(closed()).toHaveLength(1);
  });

  test("the armed button disarms itself, and the click after that is harmless", () => {
    panel([session({ state: "idle" })]);
    click(xs()[0]);
    expect(xs()[0].textContent.trim()).toBe("sure?");

    vi.advanceTimersByTime(3100);
    flushSync();

    expect(xs()[0].textContent.trim()).toBe("✕");
    click(xs()[0]);
    expect(closed()).toHaveLength(0);
  });

  test("the armed tooltip says what is about to be lost", () => {
    panel([session({ state: "idle" })]);
    click(xs()[0]);
    expect(xs()[0].title).toMatch(/unsaved conversation/);

    // and it names the other loss when the agent is mid-turn
    unmount(app);
    host.remove();
    panel([session({ id: "s-2", state: "working" })]);
    click(xs()[0]);
    expect(xs()[0].title).toMatch(/working here/);
  });

  test("arming one session does not arm another", () => {
    panel([session({ id: "s-1" }), session({ id: "s-2", selected: false })]);
    const [a, b] = xs();
    click(a);

    expect(b.textContent.trim()).toBe("✕");
    click(b);
    expect(closed()).toHaveLength(0);
    // arming b disarmed a, so a is back to one-click-does-nothing
    expect(xs()[0].textContent.trim()).toBe("✕");
  });
});
