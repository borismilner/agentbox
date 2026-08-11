import { describe, test, expect, beforeEach, afterEach, vi } from "vitest";
import { mount, unmount, flushSync } from "svelte";
import Settings from "../src/surfaces/Settings.svelte";
import { calls, reset, Call, clearListeners } from "./stubs/wailsio-runtime.js";
import { draft } from "../src/lib/settingsdraft.svelte.js";

// U-12 in docs/backlog/ux.md. Settings is the sharpest of the four rail surfaces
// that installed no key handler at all: a dense form next to a Save button that
// is the whole point of it, with no way to write the file, take a change back,
// or move between the sections without the pointer.
//
// The keys are driven through the window, the way a reader's keyboard reaches
// them, rather than by calling save() and revert() - the defect was that nothing
// listened, so a test that calls them would have passed against the old build.

const SVC = "github.com/borismilner/agentbox/internal/webui.Bridge.";
const sent = (m) => calls.filter((c) => c.name === SVC + m);

function fixture() {
  return {
    path: "~/.config/agentbox/config.toml",
    sections: [
      {
        id: "appearance",
        title: "Appearance",
        preview: true,
        groups: [
          {
            title: "Theme",
            knobs: [
              { id: "ui.theme", label: "Theme", kind: "enum", value: "dark", default: "dark", enum: ["dark", "light"] },
              { id: "ui.scale", label: "Scale", kind: "int", value: "100", default: "100", min: 80, max: 140, step: 5, unit: "%" },
            ],
          },
        ],
      },
      {
        id: "sound",
        title: "Sound",
        groups: [
          {
            title: "Earcons",
            knobs: [
              { id: "sound.on", label: "Play earcons", kind: "bool", value: "true", default: "true" },
              { id: "sound.pack", label: "Pack", kind: "text", value: "default", default: "default" },
            ],
          },
        ],
      },
      {
        id: "desktop",
        title: "Desktop",
        groups: [{ title: "Placement", knobs: [{ id: "ui.corner", label: "Corner", kind: "text", value: "top-right", default: "top-right" }] }],
      },
    ],
  };
}

let host = null;
let app = null;

async function surface() {
  host = document.createElement("div");
  document.body.appendChild(host);
  app = mount(Settings, { target: host });
  // The first paint waits on Bridge.Settings, so nothing exists to press until
  // that promise has landed.
  await vi.waitFor(() => expect(host.querySelector(".tabs")).toBeTruthy());
  flushSync();
}

function key(k, over = {}, target = window) {
  target.dispatchEvent(new KeyboardEvent("keydown", { key: k, bubbles: true, ...over }));
  flushSync();
}

const tabs = () => [...host.querySelectorAll('.tabs button[role="tab"]')];
const chosen = () => tabs().find((b) => b.getAttribute("aria-selected") === "true");
const state = () => host.querySelector("footer .state").textContent.replace(/\s+/g, " ").trim();
const switchBtn = () => host.querySelector("button.switch");
const textBox = () => host.querySelector("input.text");
const slider = () => host.querySelector('input[type="range"]');

function dirty() {
  // The sound section owns the one control a click can dirty on its own.
  tabs()[1].click();
  flushSync();
  switchBtn().click();
  flushSync();
}

beforeEach(() => {
  reset();
  // The draft is module-level $state since U-10, so it OUTLIVES a mount and
  // leaks from one test into the next: without this, "with nothing pending
  // writes nothing" runs with the previous test's edit still pending and saves
  // it. Settings.svelte used to hold this state itself, which is why these
  // tests did not need the line when they were written.
  Object.assign(draft, { data: null, values: {}, base: {}, active: "appearance", saving: false, note: "", written: [], err: "" });
  Call.byName.set(SVC + "Settings", () => fixture());
  Call.byName.set(SVC + "SaveSettings", () => ({ written: ["sound.on = false"], note: "1 key written" }));
});

afterEach(() => {
  if (app) unmount(app);
  host?.remove();
  app = host = null;
  clearListeners();
});

describe("Ctrl+S writes the file", () => {
  test("a pending change is saved without touching the button", async () => {
    await surface();
    dirty();
    expect(state()).toContain("1 key to write");

    key("s", { ctrlKey: true });

    expect(sent("SaveSettings")).toHaveLength(1);
    expect(sent("SaveSettings")[0].args[0]["sound.on"]).toBe("false");
  });

  test("with nothing pending it writes nothing", async () => {
    await surface();
    key("s", { ctrlKey: true });
    expect(sent("SaveSettings")).toHaveLength(0);
  });

  test("it saves from inside a field too, since that is where the change was typed", async () => {
    await surface();
    tabs()[1].click();
    flushSync();

    const box = textBox();
    box.focus();
    box.value = "quiet";
    box.dispatchEvent(new Event("input", { bubbles: true }));
    flushSync();

    key("s", { ctrlKey: true }, box);
    expect(sent("SaveSettings")).toHaveLength(1);
    expect(sent("SaveSettings")[0].args[0]["sound.pack"]).toBe("quiet");
  });
});

describe("Esc takes a change back", () => {
  test("the pending values go back to the file's", async () => {
    await surface();
    dirty();

    key("Escape");

    expect(state()).toContain("Saved values match the file");
    expect(sent("SaveSettings")).toHaveLength(0);
  });

  test("in a field it leaves the field, and the typing survives", async () => {
    await surface();
    tabs()[1].click();
    flushSync();

    const box = textBox();
    box.focus();
    box.value = "quiet";
    box.dispatchEvent(new Event("input", { bubbles: true }));
    flushSync();
    expect(state()).toContain("1 key to write");

    key("Escape", {}, box);

    expect(document.activeElement).not.toBe(box);
    expect(state()).toContain("1 key to write");

    // and the second press, now that the field is left, reverts
    key("Escape");
    expect(state()).toContain("Saved values match the file");
  });
});

describe("the sections move under the arrows", () => {
  test("right and left walk the strip, and it wraps", async () => {
    await surface();
    expect(chosen().textContent).toBe("Appearance");

    key("ArrowRight");
    expect(chosen().textContent).toBe("Sound");

    key("ArrowRight");
    expect(chosen().textContent).toBe("Desktop");

    key("ArrowRight");
    expect(chosen().textContent).toBe("Appearance");

    key("ArrowLeft");
    expect(chosen().textContent).toBe("Desktop");
  });

  test("only the chosen tab is in the Tab order, and the panel says which tab it belongs to", async () => {
    await surface();
    expect(tabs().map((b) => b.getAttribute("tabindex"))).toEqual(["0", "-1", "-1"]);

    key("ArrowRight");
    expect(tabs().map((b) => b.getAttribute("tabindex"))).toEqual(["-1", "0", "-1"]);
    expect(host.querySelector(".panel").getAttribute("aria-labelledby")).toBe("tab-sound");
  });

  test("the keyboard lands on the tab that is now selected", async () => {
    await surface();
    tabs()[0].focus();

    key("ArrowRight", {}, tabs()[0]);
    expect(document.activeElement).toBe(chosen());

    // Pressed from inside the panel, where the control that had the focus is
    // about to be replaced along with the rest of the section.
    const sw = switchBtn();
    sw.focus();
    key("ArrowRight", {}, sw);

    expect(chosen().textContent).toBe("Desktop");
    expect(document.activeElement).toBe(chosen());
  });

  test("a control that reads the arrows itself keeps them", async () => {
    await surface();
    const range = slider();
    range.focus();

    key("ArrowRight", {}, range);

    expect(chosen().textContent).toBe("Appearance");
  });
});
