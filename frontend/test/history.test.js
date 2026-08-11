import { describe, test, expect, beforeEach, afterEach, vi } from "vitest";
import { mount, unmount, flushSync } from "svelte";
import History from "../src/surfaces/History.svelte";
import { calls, reset, Call, clearListeners } from "./stubs/wailsio-runtime.js";

// U-12 in docs/backlog/ux.md. History installed no key handler either. It has no
// list to walk and no search to open, which is what the entry prescribed for it;
// the one choice on the surface is which window the numbers cover, and that was
// four buttons reachable only by pointer or by four Tab stops, none of which
// said which window was the current one.

const SVC = "github.com/borismilner/agentbox/internal/webui.Bridge.";
const sent = (m) => calls.filter((c) => c.name === SVC + m);

const stats = (w) => ({
  window: w,
  label: w === "7d" ? "last 7 days" : w,
  total: 12,
  questions: 5,
  answered: 4,
  answeredPct: 80,
  median: "1m 20s",
  perDay: "1.7 a day",
  byAgent: [],
  byDay: [{ day: "2026-08-11", label: "Tue 11", count: 3 }],
  peak: 3,
  empty: false,
});

let host = null;
let app = null;

async function surface() {
  host = document.createElement("div");
  document.body.appendChild(host);
  app = mount(History, { target: host });
  await vi.waitFor(() => expect(host.querySelector(".tiles")).toBeTruthy());
  flushSync();
}

function key(k, over = {}) {
  const on = host.querySelector('.windows button[tabindex="0"]');
  on.dispatchEvent(new KeyboardEvent("keydown", { key: k, bubbles: true, ...over }));
  flushSync();
}

const windows = () => [...host.querySelectorAll(".windows button")];
const checked = () => windows().find((b) => b.getAttribute("aria-checked") === "true");

beforeEach(() => {
  reset();
  Call.byName.set(SVC + "Stats", (w) => stats(w));
});

afterEach(() => {
  if (app) unmount(app);
  host?.remove();
  app = host = null;
  clearListeners();
});

describe("the time window is a choice the keyboard can make", () => {
  test("the group says which window is current, and costs one Tab stop", async () => {
    await surface();
    expect(host.querySelector(".windows").getAttribute("role")).toBe("radiogroup");
    expect(checked().textContent).toBe("7d");
    expect(windows().map((b) => b.getAttribute("tabindex"))).toEqual(["-1", "0", "-1", "-1"]);
  });

  test("the arrows move it, and it asks Go for the new window", async () => {
    await surface();
    key("ArrowRight");

    expect(checked().textContent).toBe("30d");
    expect(document.activeElement).toBe(checked());
    await vi.waitFor(() => expect(sent("Stats").at(-1).args).toEqual(["30d"]));

    key("ArrowLeft");
    expect(checked().textContent).toBe("7d");
  });

  test("it wraps at both ends", async () => {
    await surface();
    key("ArrowRight");
    key("ArrowRight"); // 24h, 7d, 30d, All - and All is the last
    expect(checked().textContent).toBe("All");

    key("ArrowRight");
    expect(checked().textContent).toBe("24h");

    key("ArrowLeft");
    expect(checked().textContent).toBe("All");
  });

  test("a held modifier belongs to the shell", async () => {
    await surface();
    key("ArrowRight", { ctrlKey: true });
    expect(checked().textContent).toBe("7d");
  });
});
