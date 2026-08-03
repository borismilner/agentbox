// Everything rendered markdown needs a browser for. The HTML itself comes from Go
// (internal/webui/mdhtml.go): headings, tables, alerts, highlighted code and
// charts arrive finished. Three things cannot be done there, and they are all
// here, in one action every surface applies to its container:
//
//   * mermaid diagrams, whose layout engine is JavaScript. Loaded on demand, from
//     the bundle - never from a CDN, because agentbox has to work offline.
//   * math, for the same reason: Go decided what is TeX (internal/webui/math.go),
//     KaTeX is what turns TeX into boxes, and KaTeX is JavaScript.
//   * interactive artifacts, which need a sandboxed iframe built around them
//     (artifact.svelte.js; the same on-demand rule, for the same reason).
//   * copy buttons on code blocks.
//   * links: a click inside a webview would navigate the card AWAY, which for a
//     frameless window with no back button is a one-way trip. Every link opens in
//     the desktop browser instead.

import { bridge } from "./bridge.js";
import { hydrateArtifacts, onArtifactClick, refreshArtifacts } from "./artifact.svelte.js";

let mermaidMod = null;
let mermaidLoading = null;

// The diagram has to agree with the surface it sits on, so mermaid is configured
// from the same CSS custom properties everything else reads.
function mermaidTheme() {
  const css = getComputedStyle(document.documentElement);
  const v = (name, fallback) => css.getPropertyValue(name).trim() || fallback;
  const ink = v("--k-ink", "#e5e8ee");
  const ink2 = v("--k-ink-2", "#98a0ad");
  const line = v("--k-edge", "#272c36");
  const surface = v("--k-surface-2", "#1c2028");
  const accent = v("--k-accent", "#7c8cf8");
  return {
    darkMode: document.documentElement.dataset.mode !== "light",
    background: "transparent",
    fontFamily: v("--k-font-ui", "sans-serif"),
    fontSize: "13px",
    primaryColor: surface,
    primaryTextColor: ink,
    primaryBorderColor: line,
    secondaryColor: v("--k-surface-3", "#22262f"),
    tertiaryColor: v("--k-ground", "#0f1116"),
    lineColor: ink2,
    textColor: ink,
    mainBkg: surface,
    nodeBorder: line,
    clusterBkg: "transparent",
    clusterBorder: line,
    titleColor: ink,
    edgeLabelBackground: v("--k-surface", "#161920"),
    actorBorder: line,
    actorBkg: surface,
    actorTextColor: ink,
    signalColor: ink2,
    signalTextColor: ink2,
    labelBoxBkgColor: surface,
    labelBoxBorderColor: line,
    labelTextColor: ink,
    loopTextColor: ink2,
    noteBkgColor: color(accent, surface),
    noteBorderColor: line,
    noteTextColor: ink,
    sectionBkgColor: surface,
    altSectionBkgColor: "transparent",
    gridColor: line,
    todayLineColor: v("--k-warning", "#d9a441"),
    pie1: accent,
    pie2: v("--k-info", "#4fa3e3"),
    pie3: v("--k-success", "#4fb286"),
    pie4: v("--k-code-num", "#d9a441"),
    pieTitleTextColor: ink,
    pieSectionTextColor: v("--k-ground", "#0f1116"),
  };
}

function color(hue, base) {
  return `color-mix(in srgb, ${hue} 16%, ${base})`;
}

async function mermaid() {
  if (mermaidMod) return mermaidMod;
  if (!mermaidLoading) {
    // A dynamic import so mermaid lands in its own chunk: a card that shows a
    // one-line question must not pay for a diagram engine it will never use.
    mermaidLoading = import("mermaid").then((m) => {
      mermaidMod = m.default ?? m;
      mermaidMod.initialize({
        startOnLoad: false,
        securityLevel: "strict", // agent-authored diagrams: no HTML labels, no click handlers
        theme: "base",
        themeVariables: mermaidTheme(),
        flowchart: { curve: "basis", htmlLabels: false, useMaxWidth: true },
        sequence: { useMaxWidth: true, actorFontFamily: "inherit" },
        gantt: { useMaxWidth: true },
      });
      return mermaidMod;
    });
  }
  return mermaidLoading;
}

let diagramSeq = 0;

async function drawDiagrams(root) {
  const blocks = [...root.querySelectorAll(".k-mermaid:not([data-state])")];
  if (!blocks.length) return;

  let mm;
  try {
    mm = await mermaid();
  } catch {
    for (const el of blocks) el.dataset.state = "failed";
    return;
  }

  for (const el of blocks) {
    const src = el.querySelector(".k-mermaid-src")?.textContent ?? "";
    if (!src.trim()) {
      el.dataset.state = "failed";
      continue;
    }
    el.dataset.state = "drawing";
    try {
      const { svg } = await mm.render(`k-mmd-${++diagramSeq}`, src);
      const holder = document.createElement("div");
      holder.className = "k-mermaid-svg";
      // The diagram source is agent-authored, so this HTML is untrusted input;
      // securityLevel "strict" above is what makes it safe to insert - mermaid
      // sanitises its own output through DOMPurify and refuses HTML labels and
      // click bindings in that mode. Do not relax that setting.
      holder.innerHTML = svg;
      el.append(holder);
      el.dataset.state = "drawn";
    } catch {
      // Keep the source visible: a diagram agentbox cannot draw is still a diagram
      // the reader can read, and a blank panel would say nothing at all.
      el.dataset.state = "failed";
    }
  }
}

// A theme change re-renders every diagram, because mermaid bakes its colours into
// the SVG it produces - the one thing in agentbox that a CSS variable write cannot
// re-theme on its own.
function redrawDiagrams(root) {
  mermaidMod = null;
  mermaidLoading = null;
  for (const el of root.querySelectorAll(".k-mermaid")) {
    el.querySelector(".k-mermaid-svg")?.remove();
    delete el.dataset.state;
  }
  drawDiagrams(root);
}

// --- math -------------------------------------------------------------------

let katexMod = null;
let katexLoading = null;

async function katex() {
  if (katexMod) return katexMod;
  if (!katexLoading) {
    // Its own chunk, like mermaid: a card asking a yes/no question must not pay
    // for a typesetting engine. The stylesheet rides along in the same chunk,
    // which is what pulls KaTeX's fonts into dist as local assets.
    katexLoading = Promise.all([import("katex"), import("katex/dist/katex.min.css")]).then(([m]) => {
      katexMod = m.default ?? m;
      return katexMod;
    });
  }
  return katexLoading;
}

async function typesetMath(root) {
  const blocks = [...root.querySelectorAll(".k-math:not([data-state])")];
  if (!blocks.length) return;

  let k;
  try {
    k = await katex();
  } catch {
    for (const el of blocks) el.dataset.state = "failed";
    return;
  }

  for (const el of blocks) {
    const tex = el.querySelector(".k-math-src")?.textContent ?? "";
    if (!tex.trim()) {
      el.dataset.state = "failed";
      continue;
    }
    const display = el.dataset.display === "1";
    try {
      // The TeX is agent-authored, so this is untrusted input and the options are
      // load-bearing. trust:false is the one that matters: it refuses \href,
      // \url, \includegraphics and the \html* family, which is what makes KaTeX's
      // output safe to insert. maxExpand bounds a macro that defines itself. Do
      // not relax either.
      const html = k.renderToString(tex, {
        displayMode: display,
        output: "html",
        throwOnError: true,
        strict: "ignore",
        trust: false,
        maxSize: 64,
        maxExpand: 1000,
      });
      const holder = document.createElement(display ? "div" : "span");
      holder.className = "k-math-out";
      holder.innerHTML = html;
      el.append(holder);
      el.dataset.state = "typeset";
    } catch {
      // A formula KaTeX will not accept is still a formula the reader can read:
      // the CSS puts the source back on screen for this state.
      el.dataset.state = "failed";
    }
  }
}

// Unlike a mermaid diagram, KaTeX output carries no colours of its own - it draws
// in currentColor - so a theme change needs nothing here.

function codeOf(button) {
  const block = button.closest(".k-code");
  if (!block) return "";
  // With line numbers, chroma puts the code in the second cell; without them the
  // <pre> is the block. Either way the numbers must not come along.
  const numbered = block.querySelector(".lntd:last-child");
  return (numbered ?? block.querySelector("pre"))?.innerText?.replace(/\n$/, "") ?? "";
}

async function copy(button) {
  const text = codeOf(button);
  if (!text) return;
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    // WebKitGTK refuses the async clipboard outside a secure context; the daemon
    // owns a clipboard that always works.
    bridge.copyText(text);
  }
  button.dataset.copied = "1";
  button.textContent = "copied";
  setTimeout(() => {
    delete button.dataset.copied;
    button.textContent = "copy";
  }, 1400);
}

// markdown is a Svelte action: use:markdown on the element holding rendered HTML.
// It re-runs when the html prop changes, which is what keeps a streaming agent
// turn's diagrams and copy buttons live.
export function markdown(node, html) {
  const onClick = (e) => {
    // The artifact chrome first: its own copy button is a code block's, so the
    // toggle and reload have to be claimed before the generic handlers.
    if (onArtifactClick(e.target)) {
      e.preventDefault();
      return;
    }
    const button = e.target.closest?.("[data-copy]");
    if (button) {
      e.preventDefault();
      copy(button);
      return;
    }
    const link = e.target.closest?.("a[href]");
    if (!link) return;
    const href = link.getAttribute("href") ?? "";
    if (href.startsWith("#")) return; // a footnote jump stays in the page
    e.preventDefault();
    bridge.openURL(link.href);
  };

  const onTheme = () => {
    redrawDiagrams(node);
    refreshArtifacts(node);
  };

  node.addEventListener("click", onClick);
  window.addEventListener("agentbox:themed", onTheme);
  drawDiagrams(node);
  typesetMath(node);
  hydrateArtifacts(node);

  return {
    update() {
      drawDiagrams(node);
      typesetMath(node);
      hydrateArtifacts(node);
    },
    destroy() {
      node.removeEventListener("click", onClick);
      window.removeEventListener("agentbox:themed", onTheme);
    },
  };
}
