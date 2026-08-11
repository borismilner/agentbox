import { describe, test, expect, beforeEach, afterEach } from "vitest";
import { mount, unmount, flushSync } from "svelte";
import App from "../src/surfaces/App.svelte";
import { calls, Call, reset, emit, clearListeners } from "./stubs/wailsio-runtime.js";
import { draft } from "../src/lib/settingsdraft.svelte.js";

// U-10: leaving Settings with unsaved changes threw them away without a word.
//
// The shell renders one branch of an {#if} chain, so clicking any other rail
// icon destroyed <Settings /> and every pending edit with it - after the surface
// had spent three affordances saying it was holding them.
//
// These tests mount the whole shell and click the real rail, because the defect
// was in the seam between the two: Settings alone always looked correct, and a
// test that mounted it on its own could not see the surface being swapped out.
// They drive the controls a person drives (the slider the backlog names in its
// repro, the section tabs) rather than calling set().

const SVC = "github.com/borismilner/agentbox/internal/webui.Bridge.";
const reads = () => calls.filter((c) => c.name === SVC + "Settings");
const writes = () => calls.filter((c) => c.name === SVC + "SaveSettings");

// The shape Go sends (internal/webui/settings.go, wireSettings): two sections so
// there is a section tab to move, and a slider whose value is the pending edit.
const SCHEMA = () => ({
  path: "/home/b/.config/agentbox/config.toml",
  sections: [
    {
      id: "appearance",
      title: "Appearance",
      preview: true,
      groups: [
        {
          title: "Type",
          knobs: [
            { id: "ui.font_size", label: "Size", kind: "int", min: 11, max: 22, step: 1, unit: "px", value: "14" },
            { id: "ui.theme", label: "Theme", kind: "enum", enum: ["dark", "light", "auto"], value: "dark" },
          ],
        },
      ],
    },
    {
      id: "sound",
      title: "Sound",
      groups: [{ title: "Earcons", knobs: [{ id: "sound.enabled", label: "Earcons", kind: "bool", value: "true" }] }],
    },
  ],
});

let host = null;
let app = null;

// The draft outlives a mount on purpose, so it also outlives a test. Nothing in
// the app clears it; every test starts from the surface having never been read.
function blank() {
  Object.assign(draft, { data: null, values: {}, base: {}, active: "appearance", saving: false, note: "", written: [], err: "" });
}

// Svelte's effects flush synchronously; the daemon calls do not, and load() is
// three awaits deep. A handful of microtasks covers it either way.
async function settle() {
  for (let i = 0; i < 8; i++) await Promise.resolve();
  flushSync();
}

const rail = (name) => host.querySelector(`nav.rail button[title^="${name}"]`);
const slider = () => host.querySelector('.settings input[type="range"]');
// The section strip became a real tablist (U-12), so it is a div with
// role="tablist" rather than the nav this used to look for.
const sectionTab = (title) => [...host.querySelectorAll('.settings header [role="tablist"] button')].find((b) => b.textContent.trim() === title);
const footer = () => host.querySelector(".settings footer .state")?.textContent.replace(/\s+/g, " ").trim() ?? "";
const dots = () => host.querySelectorAll(".settings .dot").length;

function click(el) {
  el.click();
  flushSync();
}

// Drag the slider: an input event with a new value, which is what a real drag
// sends and what the surface listens for.
function drag(el, to) {
  el.value = String(to);
  el.dispatchEvent(new Event("input", { bubbles: true }));
  flushSync();
}

beforeEach(() => {
  reset();
  clearListeners();
  blank();
  Call.byName.set(SVC + "Settings", () => SCHEMA());
  host = document.createElement("div");
  document.body.appendChild(host);
  app = mount(App, { target: host });
  flushSync();
});

afterEach(() => {
  if (app) unmount(app);
  host?.remove();
  app = null;
  host = null;
});

async function openSettings() {
  click(rail("Settings"));
  await settle();
}

describe("pending settings survive leaving the surface (U-10)", () => {
  test("a knob changed and not saved is still pending after a trip to the inbox", async () => {
    await openSettings();
    expect(slider().value).toBe("14");

    drag(slider(), 19);
    expect(footer()).toMatch(/1 key to write/);
    expect(dots()).toBe(1);

    click(rail("Inbox"));
    expect(host.querySelector(".settings")).toBeNull();

    await openSettings();

    expect(slider().value).toBe("19");
    expect(footer()).toMatch(/1 key to write/);
    expect(dots()).toBe(1);
    // and nothing was written on the way out
    expect(writes()).toHaveLength(0);
  });

  test("coming back re-reads the file, and the edit survives the read", async () => {
    await openSettings();
    drag(slider(), 21);

    // Something else wrote config.toml while we were away. BumpFontSize is the
    // real one: Ctrl+= in a session writes the file this surface is editing.
    Call.byName.set(SVC + "Settings", () => {
      const s = SCHEMA();
      s.sections[1].groups[0].knobs[0].value = "false";
      return s;
    });

    click(rail("History"));
    await openSettings();

    expect(reads()).toHaveLength(2);
    // The edit is still pending, and it is the only thing pending: the key the
    // file moved came back from the file rather than from a stale draft, which
    // is what stops Save writing it back over the other change.
    expect(slider().value).toBe("21");
    expect(footer()).toMatch(/1 key to write/);
    click(sectionTab("Sound"));
    expect(host.querySelector('.settings button[role="switch"]').getAttribute("aria-checked")).toBe("false");
    expect(dots()).toBe(0);
  });

  test("with nothing pending, coming back is a plain re-read", async () => {
    await openSettings();
    expect(reads()).toHaveLength(1);

    click(rail("Inbox"));
    await openSettings();

    expect(reads()).toHaveLength(2);
    expect(footer()).toMatch(/Saved values match the file/);
  });

  test("the section you were reading is where you come back to", async () => {
    await openSettings();
    drag(slider(), 17);
    click(sectionTab("Sound"));
    expect(host.querySelector('.settings button[role="switch"]')).toBeTruthy();

    click(rail("Home"));
    await openSettings();

    expect(sectionTab("Sound").classList.contains("on")).toBe(true);
    expect(host.querySelector('.settings button[role="switch"]')).toBeTruthy();
  });

  test("the daemon swapping the surface from outside keeps the draft too", async () => {
    await openSettings();
    drag(slider(), 20);

    // `agentbox inbox`, the tray menu and the panel all reach ShowApp, which
    // pushes this event and swaps the surface with no click in this window
    // (internal/webui/app.go:38). A guard on the rail would never have seen it.
    emit("agentbox:surface", "inbox");
    flushSync();
    expect(host.querySelector(".settings")).toBeNull();

    await openSettings();
    expect(slider().value).toBe("20");
  });
});

describe("the rail says settings is holding an edit (U-10)", () => {
  const mark = () => rail("Settings").querySelector(".unsaved");

  test("no dot until something is changed", async () => {
    expect(mark()).toBeNull();
    await openSettings();
    expect(mark()).toBeNull();
  });

  test("the dot is up from another surface, and names the count", async () => {
    await openSettings();
    drag(slider(), 18);
    click(rail("Inbox"));

    expect(mark()).toBeTruthy();
    // The tooltip carries the shortcut too since U-13; the count is appended to it.
    expect(rail("Settings").title).toBe("Settings · Ctrl+9 · 1 change not yet written");
  });

  test("reverting takes the dot back down", async () => {
    await openSettings();
    drag(slider(), 18);
    click([...host.querySelectorAll(".settings footer button")].find((b) => b.textContent.trim() === "Revert"));

    expect(mark()).toBeNull();
    expect(rail("Settings").title).toBe("Settings · Ctrl+9");
  });
});
