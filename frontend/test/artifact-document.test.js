import { describe, test, expect } from "vitest";
import { buildDocument, SANDBOX } from "../src/lib/artifact-runtime.js";

// R-40, fix (3). `frontend/policy_test.go` checks the artifact sandbox's policy as
// text: the constants are in the source and in the shipped bundle. Its own header
// says what that cannot do - "asserting that `buildDocument` behaves would need a
// JS test runner and a DOM (it uses DOMParser)" - and until this file the assembler
// had never been executed by anything.
//
// The gap between the two is not academic. A policy can be present in the module
// and absent from the document: the meta tag is inserted with
// `insertAdjacentHTML("afterbegin")` into a head DOMParser produced from
// agent-authored markup, and every escape here is an agent's markup arriving in a
// place the constant is not. So these assertions are made against the built
// document, parsed, and never against the source string.

const parse = (html) => new DOMParser().parseFromString(html, "text/html");

const build = (source, extra = {}) => {
  const out = buildDocument({ source, runtime: "html", tokens: {}, ...extra });
  expect(out.html, `buildDocument returned no document: ${out.notes.join(" | ")}`).toBeTruthy();
  return { ...out, doc: parse(out.html) };
};

const csp = (doc) => doc.querySelector('meta[http-equiv="Content-Security-Policy"]')?.getAttribute("content") ?? "";

describe("the assembled document carries the policy", () => {
  test("the CSP meta is present and closes the network in every direction", () => {
    const { doc } = build("<p>hello</p>");
    const policy = csp(doc);
    for (const directive of [
      "default-src 'none'",
      "connect-src 'none'",
      "form-action 'none'",
      "frame-src 'none'",
      "child-src 'none'",
      "base-uri 'none'",
    ]) {
      expect(policy).toContain(directive);
    }
    // The one thing a reader of the constant cannot check: that no directive
    // anywhere in the assembled policy lets a host back in.
    expect(policy).not.toMatch(/https?:/);
    expect(policy).not.toContain("*");
  });

  test("the policy is the first thing in head, so it governs what follows", () => {
    const { doc } = build("<head><style>p{color:red}</style></head><body><p>hi</p></body>");
    const first = doc.head.firstElementChild;
    expect(first?.getAttribute("http-equiv")).toBe("Content-Security-Policy");
  });

  test("agent-authored head content cannot displace it", () => {
    // A document whose own head opens with a meta CSP of its own. The assembler
    // inserts afterbegin, so ours has to be the one in front.
    const { doc } = build(
      `<html><head><meta http-equiv="Content-Security-Policy" content="default-src *"></head><body>x</body></html>`,
    );
    const metas = [...doc.querySelectorAll('meta[http-equiv="Content-Security-Policy"]')];
    expect(metas.length).toBeGreaterThanOrEqual(1);
    expect(metas[0].getAttribute("content")).toContain("default-src 'none'");
    expect(doc.head.firstElementChild).toBe(metas[0]);
  });

  test("the sandbox is allow-scripts and nothing else", () => {
    // The opaque origin is the guarantee. allow-same-origin would hand the
    // artifact the surface around it, and this is the one string that decides it.
    expect(SANDBOX).toBe("allow-scripts");
    expect(SANDBOX).not.toContain("allow-same-origin");
  });
});

describe("what the assembler strips, checked on the output", () => {
  test("a remote script is removed and its host is named in a note", () => {
    const { doc, notes } = build(`<script src="https://cdn.example.com/chart.js"></script><p>x</p>`);
    expect(doc.querySelectorAll("script[src]").length).toBe(0);
    expect(notes.join(" ")).toContain("cdn.example.com");
  });

  test("react and tailwind CDN tags go quietly, because they are already inside", () => {
    const { doc, notes } = build(`<script src="https://unpkg.com/react@18/umd/react.production.min.js"></script>`);
    expect(doc.querySelectorAll("script[src]").length).toBe(0);
    expect(notes).toEqual([]);
  });

  test("a stylesheet link is removed", () => {
    const { doc } = build(`<link rel="stylesheet" href="https://fonts.example/x.css"><p>x</p>`);
    expect(doc.querySelectorAll('link[rel~="stylesheet"]').length).toBe(0);
  });

  test("a module script is compiled to a classic one, so the CSP's inline allowance covers it", () => {
    const { doc } = build(`<script type="module">const a = 1; export {}</script>`);
    const scripts = [...doc.querySelectorAll("script")].filter((s) => !s.id);
    const compiled = scripts.find((s) => (s.textContent || "").includes("const a = 1"));
    expect(compiled).toBeTruthy();
    expect(compiled.getAttribute("type")).toBe(null);
  });

  test("a script that will not compile leaves a note rather than a silent empty frame", () => {
    const { notes } = build(`<script type="text/babel">function ( {</script>`);
    expect(notes.join(" ")).toContain("did not compile");
  });
});

describe("the inlining cannot be broken out of", () => {
  // inlineScript escapes `</script` in everything it inlines. The bootstrap and
  // the runtimes are agentbox's own, but a react artifact's compiled source is the
  // agent's, and it travels the same path.
  test("a closing script sequence inside react source does not end the tag early", () => {
    const source = `export default function App() { const s = "</script><img src=x onerror=alert(1)>"; return null }`;
    const out = buildDocument({ source, runtime: "react", tokens: {} });
    expect(out.html).toBeTruthy();
    expect(out.html).not.toContain("</script><img");
    const doc = parse(out.html);
    expect(doc.querySelectorAll("img").length).toBe(0);
    expect(csp(doc)).toContain("default-src 'none'");
  });

  test("react source that will not compile returns no document and says why", () => {
    const out = buildDocument({ source: `export default function ( {`, runtime: "react", tokens: {} });
    expect(out.html).toBe(null);
    expect(out.notes.length).toBe(1);
  });
});

// U-18. An artifact that does not run used to look exactly like one that has not
// finished: the bar rendered, the badge rendered, the stage was blank and the error
// slot was empty. The cause is that a resource failure does not BUBBLE, so the
// bootstrap's `window.addEventListener("error", ...)` never saw it. Measured in
// Chrome against this document shape: of the three ways an artifact can fail to
// start, the bubble phase reported none of them, the capture phase reports two, and
// the third (a module import the policy blocks) reports to no listener at all,
// which is what the parent's silent-stage deadline covers.
//
// The bootstrap cannot be executed here - it is a string that runs inside a
// sandboxed opaque origin - so what this asserts is that the document ships with
// both listeners and that the capture one is the one taking a third argument.
describe("the bootstrap reports a resource that fails to load (U-18)", () => {
  const bootstrapOf = (html) => {
    const doc = parse(html);
    const first = doc.querySelector("script");
    expect(first, "the document carries no bootstrap").toBeTruthy();
    return first.textContent;
  };

  test("both phases are listened for, and the capture one reads the element", () => {
    const { html } = build("<p>hello</p>");
    const boot = bootstrapOf(html);

    const listeners = boot.match(/addEventListener\("error"/g) ?? [];
    expect(listeners.length, "one error listener only catches what bubbles").toBe(2);
    expect(boot, "the capture phase is what sees a resource error").toMatch(/addEventListener\("error"[\s\S]*?\}, true\)/);
    // The detail that makes the message worth reading rather than just present.
    expect(boot).toContain("failed to load");
    expect(boot).toMatch(/getAttribute\("src"\)/);
  });

  test("an exception is still reported once, not twice", () => {
    const boot = bootstrapOf(build("<p>hello</p>").html);
    // The capture handler returns early for anything that is not an element, so a
    // thrown error is reported by the bubble handler alone.
    expect(boot).toMatch(/if \(!el \|\| el === window \|\| !el\.tagName\) return/);
  });
});
