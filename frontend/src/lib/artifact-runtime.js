// Assembling the document an artifact runs in (M10; internal/webui/artifact.go
// is the other half). Loaded on demand, so a card with a one-line question never
// pays for React.
//
// The whole contract in one place:
//
//   * The document goes into an iframe with sandbox="allow-scripts" and NO
//     allow-same-origin. That gives it an opaque origin: it cannot touch the
//     surface around it, read a cookie, or open a window.
//   * Its Content-Security-Policy is default-src 'none' with no network
//     directive of any kind. Not a CDN, not agentbox's own asset server, not a
//     fetch, not a websocket. Agent-authored markup must not be able to send
//     what you typed into it anywhere.
//   * Which means every library has to be IN the document: React and Tailwind
//     are injected as text from agentbox's own bundle (../artifact/generated/,
//     built by frontend/tools/build-runtime.mjs). agentbox works offline; so does
//     an artifact.
//   * The one way out is window.agentbox, defined in the bootstrap below: a named
//     channel, one message shape, validated on the other side. An artifact can ask
//     the agent to do something; it cannot do it itself.
//
// 'unsafe-inline' and 'unsafe-eval' are in that policy on purpose. The document
// IS agent code - refusing it eval would not make it less able to run what it
// already contains, and libraries that compile a template at runtime would
// break for nothing. What the policy is for is the network, and that is closed.

import { transform } from "sucrase";
import reactRuntime from "../artifact/generated/react-runtime.js?raw";
import tailwindRuntime from "../artifact/generated/tailwind-runtime.js?raw";

export const SANDBOX = "allow-scripts";

const CSP = [
  "default-src 'none'",
  "script-src 'unsafe-inline' 'unsafe-eval'",
  "style-src 'unsafe-inline'",
  "img-src data: blob:",
  "media-src data: blob:",
  "font-src data:",
  "connect-src 'none'",
  "form-action 'none'",
  "frame-src 'none'",
  "child-src 'none'",
  "base-uri 'none'",
].join("; ");

// Inlining JavaScript into a <script> element ends it early if the text contains
// the closing sequence. Escaping it inside the source is equivalent JavaScript
// wherever it can legally appear (a string, a regex, a comment).
function inlineScript(js) {
  return `<script>${js.replace(/<\/script/gi, "<\\/script")}</script>`;
}

// The bootstrap is the artifact's whole view of the world outside itself: how
// tall it is, that it broke, and window.agentbox.emit. It runs before anything else
// in the document, so an artifact that throws on its first line still reports.
const BOOTSTRAP = `(() => {
  const post = (msg) => {
    try { parent.postMessage(Object.assign({ from: "agentbox-artifact" }, msg), "*"); } catch (e) {}
  };
  let last = 0;
  const measure = () => {
    const b = document.body;
    if (!b) return 0;
    return Math.ceil(Math.max(b.scrollHeight, b.getBoundingClientRect().height));
  };
  const report = () => {
    const h = measure();
    if (h > 0 && Math.abs(h - last) > 2) { last = h; post({ type: "height", height: h }); }
  };
  window.__agentboxReport = report;
  window.agentbox = {
    // The named channel. Everything an artifact wants to tell the agent goes
    // through here; there is no other way out, by construction.
    emit(name, data) { post({ type: "event", name: String(name == null ? "" : name), data: data }); },
    resize: report,
  };
  const fail = (message) => post({ type: "error", message: String(message).slice(0, 400) });
  window.addEventListener("error", (e) => fail(e.message || e.error || "error"));
  window.addEventListener("unhandledrejection", (e) => {
    const r = e.reason;
    fail("unhandled rejection: " + ((r && r.message) || r));
  });
  // Find, forwarded from the surface: the parent cannot see into an opaque
  // origin, so the frame searches its own text and answers with the count.
  // Hits are painted with the CSS Highlight API, never by wrapping nodes in
  // <mark> the way the viewer marks its own document: these text nodes belong
  // to whatever framework rendered them, and splitting them makes the next
  // React render throw. The ranges go stale if the artifact re-renders; the
  // next keystroke or Enter in the find bar rebuilds them.
  let hits = [];
  let hitAt = 0;
  let findQuery = "";
  const canPaint = typeof Highlight === "function" && window.CSS && CSS.highlights;
  const paint = () => {
    if (!canPaint) return;
    CSS.highlights.set("agentbox-find", new Highlight(...hits));
    const on = hits[hitAt - 1];
    if (on) CSS.highlights.set("agentbox-find-current", new Highlight(on));
    else CSS.highlights.delete("agentbox-find-current");
  };
  const showHit = () => {
    const r = hits[hitAt - 1];
    const el = r && r.startContainer && r.startContainer.parentElement;
    // The element, not range geometry: scrollIntoView walks every nested
    // scroll container (a code panel scrolling inside a scrolling step) and
    // stays correct under the injected zoom.
    if (el) el.scrollIntoView({ block: "center", behavior: "smooth" });
  };
  const found = () => post({ type: "found", matches: hits.length, at: hitAt });
  // keep=true is the re-run after the artifact re-rendered: hold the position,
  // do not steal the scroll. Except when the matches went from none to some -
  // that is an arrival (the artifact navigated to where the text is), and the
  // reader is waiting to be taken to it.
  const findRun = (raw, keep) => {
    findQuery = String(raw || "");
    const wasEmpty = hits.length === 0;
    const prevAt = hitAt;
    hits = [];
    hitAt = 0;
    const needle = findQuery.trim().toLowerCase();
    if (needle.length >= 2 && document.body) {
      const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT, {
        acceptNode: (n) => {
          if (!n.nodeValue || !n.nodeValue.toLowerCase().includes(needle)) return NodeFilter.FILTER_REJECT;
          const p = n.parentElement;
          // Nothing invisible and nothing unpaintable: a hit that cannot be
          // seen or scrolled to only corrupts the count. An artifact can mark
          // its own find UI with data-agentbox-find-exclude so echoing the query
          // (a results strip, a search box) does not count as finding it.
          if (!p || p.closest("script,style,svg,textarea,[data-agentbox-find-exclude]") || !p.getClientRects().length) return NodeFilter.FILTER_REJECT;
          return NodeFilter.FILTER_ACCEPT;
        },
      });
      let node;
      while ((node = walker.nextNode())) {
        const text = node.nodeValue.toLowerCase();
        let idx = text.indexOf(needle);
        while (idx !== -1) {
          const r = document.createRange();
          r.setStart(node, idx);
          r.setEnd(node, idx + needle.length);
          hits.push(r);
          idx = text.indexOf(needle, idx + needle.length);
        }
      }
      if (hits.length) hitAt = keep ? Math.min(Math.max(prevAt, 1), hits.length) : 1;
    }
    paint();
    if (!keep || (wasEmpty && hits.length)) showHit();
    found();
  };
  const findJump = (delta) => {
    if (!hits.length) return found();
    hitAt = ((hitAt + (delta < 0 ? -1 : 1) - 1 + hits.length) % hits.length) + 1;
    paint();
    showHit();
    found();
  };
  // The ranges die with the nodes they anchor to. When the document mutates
  // under an active query - a framework re-render, the artifact switching
  // views - the search re-runs where the reader left it instead of silently
  // dropping its marks.
  let refindT = 0;
  const refind = () => {
    if (!findQuery.trim()) return;
    clearTimeout(refindT);
    refindT = setTimeout(() => findRun(findQuery, true), 150);
  };
  new MutationObserver(refind).observe(document.documentElement, { childList: true, subtree: true, characterData: true });
  // Once the reader clicks inside the artifact, every keystroke lands in this
  // document and the surface's find shortcuts go deaf. The chords are
  // forwarded out (capture phase, so artifact code cannot swallow them);
  // plain "/" only when not typing into the artifact.
  window.addEventListener("keydown", (e) => {
    const t = e.target;
    const editable = t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName || ""));
    if (((e.ctrlKey || e.metaKey) && (e.key === "f" || e.key === "F")) || (!editable && e.key === "/")) {
      e.preventDefault();
      post({ type: "chord", key: "find" });
    } else if (e.key === "F3") {
      e.preventDefault();
      post({ type: "chord", key: e.shiftKey ? "find-prev" : "find-next" });
    }
  }, true);
  // A theme change repaints the artifact instead of reloading it: reloading
  // would throw away whatever the user had typed into it.
  window.addEventListener("message", (e) => {
    if (e.source !== window.parent || !e.data || e.data.from !== "agentbox") return;
    if (e.data.type === "style") {
      const el = document.getElementById("__agentbox-base");
      if (el) el.textContent = String(e.data.css || "");
    }
    if (e.data.type === "find") findRun(e.data.query);
    if (e.data.type === "find-jump") findJump(e.data.delta);
  });
  const start = () => {
    report();
    // A ResizeObserver on the root element catches everything that changes the
    // document's height: a late image, a font, a React render, a click that
    // opens a panel. Polling would cost every artifact CPU forever.
    if (window.ResizeObserver) new ResizeObserver(report).observe(document.documentElement);
  };
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", start);
  else start();
  window.addEventListener("load", report);
  post({ type: "ready" });
})();`;

// The module shim: what an artifact's import statements resolve against, after
// the transform below has turned them into require calls. A missing library is
// named rather than swallowed, because "Recharts is not defined" three frames
// deep is a worse answer than "agentbox does not bundle recharts".
const REQUIRE_SHIM = `const __agentboxModules = globalThis.__agentboxArtifactModules || {};
  const require = (name) => {
    if (Object.prototype.hasOwnProperty.call(__agentboxModules, name)) return __agentboxModules[name];
    throw new Error('agentbox does not bundle "' + name + '" (react, react-dom and tailwind are available)');
  };
  const module = { exports: {} };
  const exports = module.exports;`;

// baseStyle gives the document agentbox's own colours and type as a starting point,
// so an artifact that styles nothing still looks like it belongs in the window it
// is in, and one that styles everything overrides it. The tokens are exposed as
// custom properties too: an artifact that wants to match the surface can.
export function baseStyleFor(tokens) {
  const t = tokens || {};
  const vars = Object.entries({
    "--k-ground": t.ground,
    "--k-surface": t.surface,
    "--k-surface-2": t.surface2,
    "--k-surface-3": t.surface3,
    "--k-edge": t.edge,
    "--k-ink": t.ink,
    "--k-ink-2": t.ink2,
    "--k-ink-3": t.ink3,
    "--k-accent": t.accent,
    "--k-info": t.info,
    "--k-success": t.success,
    "--k-warning": t.warning,
    "--k-error": t.error,
    "--k-font-ui": t.fontUI,
    "--k-font-mono": t.fontMono,
    "--k-radius": t.radius,
  })
    .filter(([, v]) => v)
    .map(([k, v]) => `${k}: ${v};`)
    .join(" ");

  return `:root { color-scheme: ${t.mode === "light" ? "light" : "dark"}; ${vars} }
html { font-size: ${t.size || "14px"}; }
html, body { margin: 0; padding: 0; }
body {
  background: ${t.surface || "transparent"};
  color: ${t.ink || "inherit"};
  font-family: ${t.fontUI || "system-ui, sans-serif"};
  font-size: 1rem;
  line-height: 1.5;
  -webkit-font-smoothing: antialiased;
}
::selection { background: ${t.accent || "#7c8cf8"}; color: ${t.ground || "#0f1116"}; }
::highlight(agentbox-find) { background-color: color-mix(in srgb, ${t.warning || "#d9a13b"} 34%, transparent); }
::highlight(agentbox-find-current) { background-color: color-mix(in srgb, ${t.warning || "#d9a13b"} 62%, transparent); }`;
}

// compile turns whatever the agent wrote into something a browser can run
// without a bundler: JSX and TypeScript syntax out, imports into require calls.
// The automatic JSX runtime is tried first because that is what current React
// code assumes; classic is the fallback for code that predates it.
export function compile(source) {
  const base = { transforms: ["jsx", "typescript", "imports"], production: true };
  try {
    return transform(source, { ...base, jsxRuntime: "automatic" }).code;
  } catch (err) {
    try {
      return transform(source, { ...base, jsxRuntime: "classic" }).code;
    } catch {
      throw err; // the first message is the honest one
    }
  }
}

// mountScript runs a compiled module and puts its default export on screen. An
// artifact that mounted itself is left alone: `createRoot(...).render(...)` in
// the artifact's own code is the claude.ai idiom too, and mounting twice would
// fight it.
function mountScript(code) {
  return `(() => {
  ${REQUIRE_SHIM}
  const fail = (e) => {
    const message = String((e && e.message) || e);
    parent.postMessage({ from: "agentbox-artifact", type: "error", message: message.slice(0, 400) }, "*");
  };
  try {
${code}
  } catch (e) { fail(e); return; }
  const host = document.getElementById("root");
  const Comp = module.exports.default || module.exports.App || globalThis.App;
  if (!host || host.childElementCount > 0) return;
  if (!Comp) { fail("this artifact exports no component (add: export default function App() ...)"); return; }
  try {
    globalThis.ReactDOM.createRoot(host).render(globalThis.React.createElement(Comp));
  } catch (e) { fail(e); return; }
  if (window.__agentboxReport) requestAnimationFrame(window.__agentboxReport);
})();`;
}

function head(spec, tokens) {
  const parts = [
    `<meta http-equiv="Content-Security-Policy" content="${CSP}">`,
    `<meta charset="utf-8">`,
    `<meta name="viewport" content="width=device-width, initial-scale=1">`,
    `<style id="__agentbox-base">${baseStyleFor(tokens)}</style>`,
    inlineScript(BOOTSTRAP),
  ];
  if (spec.react) parts.push(inlineScript(reactRuntime));
  if (spec.tailwind) parts.push(inlineScript(tailwindRuntime));
  return parts.join("\n");
}

// buildDocument is the whole assembly. It returns the document plus the notes
// worth showing above it: a blocked CDN, a source that would not compile. A
// note is not a failure - the artifact still runs - but silence about a library
// agentbox threw away would be a lie about why the frame is empty.
export function buildDocument({ source, runtime, react, tailwind, tokens }) {
  const spec = { react: react || runtime === "react", tailwind: !!tailwind };
  const notes = [];

  if (runtime === "react") {
    let code;
    try {
      code = compile(source);
    } catch (err) {
      return { html: null, notes: [`${err.message || err}`] };
    }
    return {
      html: `<!doctype html>
<html>
<head>
${head(spec, tokens)}
</head>
<body>
<div id="root"></div>
${inlineScript(mountScript(code))}
</body>
</html>`,
      notes,
    };
  }

  // A document (or a fragment: the parser normalises both into html/head/body,
  // so there is one path rather than two). Parsing here is inert - a document
  // from DOMParser runs nothing and fetches nothing - and gives us somewhere
  // exact to put the policy: the top of head, before anything it must govern.
  const doc = new DOMParser().parseFromString(source, "text/html");
  const hosts = new Set();

  for (const script of [...doc.querySelectorAll("script")]) {
    const type = (script.getAttribute("type") || "").toLowerCase();
    const src = script.getAttribute("src");
    if (src) {
      // Nothing can be fetched, so a CDN tag is dead weight. React and Tailwind
      // are already in the document; anything else the artifact will miss, and
      // it should say which.
      if (!/react|tailwind|babel/i.test(src)) {
        try {
          hosts.add(new URL(src, "https://cdn.invalid").host);
        } catch {
          hosts.add(src);
        }
      }
      script.remove();
      continue;
    }
    const needsCompile = type === "text/babel" || type === "text/jsx" || type === "module";
    if (!needsCompile) continue;
    try {
      const code = compile(script.textContent || "");
      script.textContent = `(() => {\n  ${REQUIRE_SHIM}\n${code}\n})();`;
      script.removeAttribute("type"); // a classic script now, and CSP allows it inline
    } catch (err) {
      notes.push(`script did not compile: ${err.message || err}`);
    }
  }
  for (const host of hosts) {
    notes.push(`${host} was not loaded (the sandbox has no network; react and tailwind are bundled)`);
  }

  for (const link of [...doc.querySelectorAll('link[rel~="stylesheet"], link[rel="preload"]')]) {
    link.remove(); // same reason, one fewer console error
  }

  doc.head.insertAdjacentHTML("afterbegin", head(spec, tokens));
  return { html: `<!doctype html>\n${doc.documentElement.outerHTML}`, notes };
}
