import { describe, test, expect, afterEach } from "vitest";
import { mount, unmount, flushSync } from "svelte";
import Step from "../src/lib/board/Step.svelte";

// FR101: a step's body can hold a picture, a table and a callout beside its code,
// and the point of the whole increment is that those arrive on screen. Nothing
// mounted this component before, so every claim about how a walkthrough LOOKS was
// made by reading a diff (docs/backlog/robustness.md, R-40).
//
// The step is mounted alone rather than through the board, because the board
// needs a daemon snapshot and these blocks need only the wire model Go builds.

let host = null;
let app = null;

// Two mounts in one test is the only way to compare two shapes of the same block,
// so tearing down is a named step rather than only an afterEach.
function clean() {
  if (app) unmount(app);
  if (host) host.remove();
  app = host = null;
}

afterEach(clean);

const SVG = '<svg viewBox="0 0 40 20"><rect x="1" y="1" width="10" height="10" fill="var(--k-surface-2)"/><text x="4" y="16">hop</text></svg>';

function step(body, over = {}) {
  return {
    id: "s1",
    kind: "ground",
    title: "The shape of the request path",
    prose: [{ t: "One request, three hops." }],
    body,
    ...over,
  };
}

function show(s) {
  host = document.createElement("div");
  document.body.appendChild(host);
  app = mount(Step, {
    target: host,
    props: {
      step: s,
      mark: { verdict: "", note: "", revealed: [] },
      comments: [],
      root: "/repo",
      isFirst: true,
      isLast: true,
      noteFocus: 0,
      stepComposer: 0,
      pend: null,
      onVerdict: () => {},
      onNote: () => {},
      onReveal: () => {},
      onComment: () => {},
      onCommentEdit: () => {},
      onCommentDelete: () => {},
      onNav: () => {},
    },
  });
  flushSync();
  return host;
}

describe("a figure", () => {
  test("draws the markup Go re-composed, and says what it is", () => {
    const el = show(step([
      { kind: "figure", lead: "The path, drawn.", figure: { svg: SVG, caption: "three hops", alt: "the request path" } },
    ]));
    const fig = el.querySelector("figure");
    expect(fig).toBeTruthy();
    const svg = fig.querySelector("svg");
    expect(svg).toBeTruthy();
    // The drawing keeps its viewBox, which is what lets CSS size it to the column.
    expect(svg.getAttribute("viewBox")).toBe("0 0 40 20");
    expect(fig.querySelector("text").textContent).toBe("hop");
    expect(fig.querySelector("figcaption").textContent).toBe("three hops");
    // A drawing is a picture to a screen reader, not a pile of shapes.
    expect(fig.querySelector('[role="img"]').getAttribute("aria-label")).toBe("the request path");
    // The lead sits above it, the same way it sits above a code block.
    expect(el.textContent).toContain("The path, drawn.");
  });

  test("an image arrives as a data URI and nothing else", () => {
    const el = show(step([
      { kind: "figure", figure: { src: "data:image/png;base64,iVBORw0KGgo=", alt: "the board" } },
    ]));
    const img = el.querySelector("img");
    expect(img.getAttribute("src").startsWith("data:image/png")).toBe(true);
    expect(img.getAttribute("alt")).toBe("the board");
  });

  test("a figure that could not be read states the absence", () => {
    const el = show(step([
      { kind: "figure", figure: { err: "file not found", alt: "the board" } },
    ]));
    expect(el.querySelector("img")).toBeNull();
    expect(el.querySelector("svg")).toBeNull();
    expect(el.textContent).toContain("no picture here");
    expect(el.textContent).toContain("file not found");
    // The description survives the failure, because it is what the reader has left.
    expect(el.textContent).toContain("the board");
  });

  test("a wide figure crosses into the annotation margin, a plain one does not", () => {
    const wide = show(step([{ kind: "figure", figure: { svg: SVG, wide: true } }]));
    expect(wide.querySelector("figure").className).toContain("wide");
    clean();
    const plain = show(step([{ kind: "figure", figure: { svg: SVG } }]));
    expect(plain.querySelector("figure").className).not.toContain("wide");
  });
});

describe("a table", () => {
  test("renders the head, the rows and the alignment it was given", () => {
    const el = show(step([
      {
        kind: "table",
        table: {
          head: ["hop", "cost"],
          rows: [["the daemon", "0.4 ms"], ["the store", "11 ms"]],
          align: ["left", "right"],
          caption: "measured on the loaded machine",
        },
      },
    ]));
    const ths = [...el.querySelectorAll("th")].map((th) => th.textContent);
    expect(ths).toEqual(["hop", "cost"]);
    const rows = [...el.querySelectorAll("tbody tr")].map((tr) =>
      [...tr.querySelectorAll("td")].map((td) => td.textContent),
    );
    expect(rows).toEqual([["the daemon", "0.4 ms"], ["the store", "11 ms"]]);
    const cost = el.querySelectorAll("tbody tr td")[1];
    expect(cost.style.textAlign).toBe("right");
    // A right-aligned column is a column of numbers, and numbers only line up in
    // tabular figures.
    expect(cost.className).toContain("num");
    expect(el.querySelector("figcaption").textContent).toBe("measured on the loaded machine");
  });

  test("a cell's text is text, never markup", () => {
    const el = show(step([
      { kind: "table", table: { head: ["what"], rows: [['<img src=x onerror="boom()">']] } },
    ]));
    expect(el.querySelector("tbody img")).toBeNull();
    expect(el.querySelector("tbody td").textContent).toContain("<img");
  });
});

describe("a callout", () => {
  test("wears its tone, its title and its own words", () => {
    const el = show(step([
      {
        kind: "callout",
        callout: {
          tone: "danger",
          title: "The second hop is the one that fails",
          prose: [{ t: "It retries three times, then gives up." }],
        },
      },
    ]));
    const aside = el.querySelector("aside.callout");
    expect(aside.className).toContain("danger");
    expect(aside.textContent).toContain("The second hop is the one that fails");
    expect(aside.textContent).toContain("It retries three times");
    // The mark is the same drawn severity glyph every other surface uses.
    expect(aside.querySelector("svg")).toBeTruthy();
  });

  test("its paragraphs break where the author said they break", () => {
    const el = show(step([
      {
        kind: "callout",
        callout: {
          tone: "note",
          prose: [{ t: "First." }, { t: "Second.", p: true }],
        },
      },
    ]));
    const ps = [...el.querySelectorAll("aside.callout p")];
    expect(ps.length).toBe(2);
    expect(ps[0].textContent).toBe("First.");
    expect(ps[1].textContent).toBe("Second.");
  });
});

describe("the body as a sequence", () => {
  test("blocks render in the order they were written", () => {
    const el = show(step([
      { kind: "callout", callout: { tone: "note", title: "first", prose: [{ t: "a" }] } },
      { kind: "table", table: { head: ["h"], rows: [["r"]] } },
      { kind: "figure", figure: { svg: SVG } },
    ]));
    const kinds = [...el.querySelectorAll("aside.callout, table, figure")].map((n) =>
      n.tagName === "TABLE" ? "table" : n.tagName === "ASIDE" ? "callout" : "figure",
    );
    // The table is inside a figure element of its own, so it is listed twice by
    // that selector; what matters is that the callout comes first and the drawing
    // last.
    expect(kinds[0]).toBe("callout");
    expect(kinds[kinds.length - 1]).toBe("figure");
  });

  test("the diff legend appears only when the step shows code", () => {
    const noCode = show(step([{ kind: "figure", figure: { svg: SVG } }]));
    expect(noCode.textContent).not.toContain("added in this diff");
    clean();
    const withCode = show(step([
      { kind: "code", code: { path: "pkg/f.go", start: 4, lines: [{ n: 4, html: "func f() {" }] } },
    ], { kind: "code" }));
    expect(withCode.textContent).toContain("added in this diff");
  });
});
