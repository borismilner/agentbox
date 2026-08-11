import { describe, test, expect, beforeEach, afterEach, vi } from "vitest";
import { mount, unmount, flushSync } from "svelte";
import Library from "../src/surfaces/Library.svelte";
import { calls, reset, Call, clearListeners } from "./stubs/wailsio-runtime.js";

// U-12 in docs/backlog/ux.md. The library is a list of rows with a search box
// above it and no key handler at all, so walking it and opening one was a
// pointer job. The keys are the inbox's on purpose: it is the other list of rows
// in the app and a reader should not have to learn two sets.

const SVC = "github.com/borismilner/agentbox/internal/webui.Bridge.";
const sent = (m) => calls.filter((c) => c.name === SVC + m);

function review(over = {}) {
  return {
    id: "wt-1",
    title: "resume replays one offset",
    repo: "~/src/grabbit",
    pinned: "9f2a1c4d5e6f",
    state: "open",
    steps: 8,
    understood: 3,
    unclear: 1,
    comments: 2,
    updatedMs: Date.now() - 60_000,
    onBoard: false,
    ...over,
  };
}

let host = null;
let app = null;

async function surface(rows) {
  Call.byName.set(SVC + "Library", () => rows);
  host = document.createElement("div");
  document.body.appendChild(host);
  app = mount(Library, { target: host });
  // The list is painted from Bridge.Library, so there is nothing to press until
  // that promise has landed.
  await vi.waitFor(() => expect(host.querySelector(".rows .row")).toBeTruthy());
  flushSync();
}

function key(k, over = {}, target = window) {
  target.dispatchEvent(new KeyboardEvent("keydown", { key: k, bubbles: true, ...over }));
  flushSync();
}

const rows = () => [...host.querySelectorAll(".row")];
const marked = () => host.querySelector(".row.on");
const box = () => host.querySelector(".search input");
const hint = () => host.querySelector(".count").textContent.replace(/\s+/g, " ").trim();

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

describe("the library answers the keyboard", () => {
  test("j and k walk the rows", async () => {
    await surface([review(), review({ id: "wt-2", title: "the second one" }), review({ id: "wt-3" })]);
    expect(marked().dataset.id).toBe("wt-1");

    key("j");
    expect(marked().dataset.id).toBe("wt-2");

    key("j");
    key("j"); // the end of the list is the end, not a wrap
    expect(marked().dataset.id).toBe("wt-3");

    key("k");
    expect(marked().dataset.id).toBe("wt-2");
  });

  test("the arrows do the same thing", async () => {
    await surface([review(), review({ id: "wt-2" })]);
    key("ArrowDown");
    expect(marked().dataset.id).toBe("wt-2");
    key("ArrowUp");
    expect(marked().dataset.id).toBe("wt-1");
  });

  test("enter puts the selected review back on the board", async () => {
    await surface([review(), review({ id: "wt-2" })]);
    key("j");
    key("Enter");

    expect(sent("LibraryOpen")).toHaveLength(1);
    expect(sent("LibraryOpen")[0].args).toEqual(["wt-2"]);
  });

  test("enter on a focused row is the button's, not the selection's", async () => {
    await surface([review(), review({ id: "wt-2" })]);
    key("j");

    // Tab reached the first row and Enter will press it. Opening the selection
    // as well would put two reviews on the board from one keystroke.
    key("Enter", {}, rows()[0].querySelector("button.main"));
    expect(sent("LibraryOpen")).toHaveLength(0);
  });

  test("/ reaches the search box and Esc leaves it", async () => {
    await surface([review()]);
    expect(hint()).toContain("/ search");

    key("/");
    expect(document.activeElement).toBe(box());
    expect(hint()).toContain("esc leaves search");

    key("Escape", {}, box());
    expect(document.activeElement).not.toBe(box());
  });

  test("the selection stays inside the filtered list", async () => {
    await surface([review(), review({ id: "wt-2", title: "the second one" })]);
    key("j");
    expect(marked().dataset.id).toBe("wt-2");

    box().focus();
    box().value = "resume";
    box().dispatchEvent(new Event("input", { bubbles: true }));
    flushSync();
    box().blur();
    flushSync();

    expect(rows()).toHaveLength(1);
    expect(marked().dataset.id).toBe("wt-1");

    key("Enter");
    expect(sent("LibraryOpen")[0].args).toEqual(["wt-1"]);
  });

  test("Esc calls off a delete that is waiting for its second click", async () => {
    await surface([review()]);
    host.querySelector("button.x").click();
    flushSync();
    expect(host.querySelector(".confirm")).toBeTruthy();

    key("Escape");

    expect(host.querySelector(".confirm")).toBeNull();
    expect(sent("LibraryDelete")).toHaveLength(0);
  });

  test("a held modifier is the shell's, not this surface's", async () => {
    await surface([review(), review({ id: "wt-2" })]);
    key("j", { ctrlKey: true });
    expect(marked().dataset.id).toBe("wt-1");
  });
});
