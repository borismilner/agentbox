// Bringing an artifact to life (M10). Go emits inert markup for one - the source,
// its highlighted twin, an empty stage (internal/webui/artifact.go) - and this is
// what puts a frame in the stage and runs it.
//
// It is the enforcement point for the trust switch: with [artifact] enabled off,
// this function creates no iframe, so nothing runs. Not hidden, not muted - never
// started. The switch arrives with the theme, so flipping it in settings takes
// effect on an open conversation.
//
// The runtime itself (React, Tailwind, the JSX transform, the document with its
// sandbox and its policy) is a separate chunk, imported the first time an
// artifact actually appears: a card asking a yes/no question must not pay for
// React.

import { bridge } from "./bridge.js";

let runtime = null;
let loading = null;

// The same bounds Go enforces on an emit (internal/webui/artifact.go).
const NAME_MAX = 64;
const DATA_MAX = 16 * 1024;
const PLAIN = /^[A-Za-z0-9._:-]+$/;

async function load() {
  if (runtime) return runtime;
  if (!loading) loading = import("./artifact-runtime.js").then((m) => (runtime = m));
  return loading;
}

const MIN_HEIGHT = 80;

// The reader's zoom, as far as the sandbox is concerned. It travels as a rule
// in the injected base style, so the artifact's own renderer re-lays-out and
// re-renders at the new size: glyphs stay crisp and hit-testing stays native.
// Mechanism four, by owner decision (2026-07-28): A+/A- moves the document's
// base FONT SIZE - the rem root - not a magnifier. The reader asking for A+
// means bigger text, not bigger everything; px-sized chrome holds still and
// rem-sized text scales. The base comes from --k-size, i.e. [font] size_pt,
// the same knob every other agentbox surface reads. The earlier mechanisms are
// kept on record: frame CSS zoom was evaluated only at build (A+ dead until
// reload) and drifted hit-testing; a transform composited a blurred bitmap;
// html{zoom} inside the frame worked but magnified boxes, not type.
// An artifact that sizes its text in px has opted out of scaling; agentbox's own
// surfaces and the board mock size text in rem.
let artZoom = 1;

function styleWithZoom() {
  if (!runtime) return null;
  let css = runtime.baseStyleFor(tokensNow());
  if (artZoom !== 1) {
    const base = parseFloat(tokensNow().size) || 14;
    css += `\nhtml { font-size: ${(base * artZoom).toFixed(2)}px; }`;
  }
  return css;
}

// setArtifactZoom applies the reader's A+/A- to every frame under root, live.
export function setArtifactZoom(root, z) {
  artZoom = z > 0 ? z : 1;
  const css = styleWithZoom();
  if (!css) return;
  for (const frame of root.querySelectorAll(".k-artifact-frame")) {
    frame.contentWindow?.postMessage({ from: "agentbox", type: "style", css }, "*");
  }
}

// The reader's find, the same way: the parent cannot walk an opaque origin's
// text, so the query travels into the frame, the frame searches itself and
// answers with a count (STATUS: "find cannot see into a running artifact").
// The query is remembered like the zoom is, so a reload re-marks it.
let artFind = "";
let foundCb = null;
let chordCb = null;

// onArtifactFound registers the one listener for counts coming back: the find
// bar of whichever surface is hosting the frame.
export function onArtifactFound(cb) {
  foundCb = cb;
}

// onArtifactChord registers the listener for keyboard chords the frame
// forwards out: once focus is inside the sandbox, Ctrl+F pressed there is
// the only way the surface ever hears about it.
export function onArtifactChord(cb) {
  chordCb = cb;
}

const CHORDS = new Set(["find", "find-next", "find-prev"]);

export function artifactFind(root, query) {
  artFind = String(query || "");
  for (const frame of root.querySelectorAll(".k-artifact-frame")) {
    frame.contentWindow?.postMessage({ from: "agentbox", type: "find", query: artFind }, "*");
  }
}

export function artifactFindJump(root, delta) {
  for (const frame of root.querySelectorAll(".k-artifact-frame")) {
    frame.contentWindow?.postMessage({ from: "agentbox", type: "find-jump", delta }, "*");
  }
}

function policy() {
  const root = document.documentElement;
  const css = getComputedStyle(root);
  const max = parseInt(css.getPropertyValue("--k-artifact-max"), 10);
  return {
    enabled: root.dataset.artifacts !== "off",
    max: Number.isFinite(max) && max > MIN_HEIGHT ? max : 640,
  };
}

// tokensNow reads the theme back off the document. The iframe cannot see agentbox's
// stylesheet (opaque origin, and rightly so), so its colours have to be handed
// to it as values.
function tokensNow() {
  const css = getComputedStyle(document.documentElement);
  const v = (name) => css.getPropertyValue(name).trim();
  return {
    mode: document.documentElement.dataset.mode || "dark",
    ground: v("--k-ground"),
    surface: v("--k-surface"),
    surface2: v("--k-surface-2"),
    surface3: v("--k-surface-3"),
    edge: v("--k-edge"),
    ink: v("--k-ink"),
    ink2: v("--k-ink-2"),
    ink3: v("--k-ink-3"),
    accent: v("--k-accent"),
    info: v("--k-info"),
    success: v("--k-success"),
    warning: v("--k-warning"),
    error: v("--k-error"),
    fontUI: v("--k-font-ui"),
    fontMono: v("--k-font-mono"),
    radius: v("--k-radius"),
    size: v("--k-size"),
  };
}

function note(block, text) {
  const el = block.querySelector(".k-artifact-note");
  if (el) el.textContent = text || "";
}

function sourceOf(block) {
  return block.querySelector(".k-artifact-src")?.textContent ?? "";
}

// frames maps a live iframe to the block it belongs to, so a message can be
// attributed to the artifact that sent it. The window identity is the check that
// matters: an opaque-origin document has origin "null", which names nothing.
const frames = new Map();

function onMessage(e) {
  const msg = e.data;
  if (!msg || msg.from !== "agentbox-artifact") return;
  const block = frames.get(e.source);
  if (!block) return; // not one of ours, or its frame is gone

  switch (msg.type) {
    case "height":
      fit(block, msg.height);
      break;
    case "error":
      note(block, String(msg.message ?? "").slice(0, 240));
      block.dataset.state = "error";
      break;
    case "ready":
      block.dataset.state = "running";
      break;
    case "event":
      emit(block, msg);
      break;
    case "found": {
      // Sent by agent code, so the numbers are clamped before anyone counts
      // on them; they only ever feed the find bar's n/m display.
      const matches = Math.max(0, Math.floor(Number(msg.matches) || 0));
      const at = Math.min(matches, Math.max(0, Math.floor(Number(msg.at) || 0)));
      foundCb?.({ matches, at });
      break;
    }
    case "chord":
      // Same sender, so only the vocabulary this side already speaks; the
      // worst a hostile artifact can do with it is open the find bar.
      if (CHORDS.has(msg.key)) chordCb?.(msg.key);
      break;
  }
}

// emit is the way out: what the human did inside the artifact, on its way to whatever
// agent is waiting for it (M10 slice 3). Everything is checked before it leaves
// this function, because the sender is agent-authored code in a sandbox and the
// daemon is not: a name that is not a short plain word, or a payload that will not
// serialise or is too big, is dropped here. Go checks the same things again -
// the surface checks so it can drop quietly, Go checks because Go must not be
// talked into anything.
function emit(block, msg) {
  const name = typeof msg.name === "string" ? msg.name.trim() : "";
  if (!name || name.length > NAME_MAX || !PLAIN.test(name)) return;

  let json = "";
  if (msg.data !== undefined && msg.data !== null) {
    try {
      json = JSON.stringify(msg.data) ?? "";
    } catch {
      return; // a cycle, or something that will not serialise: not a message
    }
    if (json.length > DATA_MAX) return;
  }

  // A parameter panel (M12) is not talking to an agent: nothing awaits it, and
  // an event routed to artifactEvent would queue for nobody. Its values go to
  // the surface hosting it, which writes them through SetAssignmentParams -
  // the same entry point the typed knobs use. One event name is the whole
  // vocabulary; anything else is named in the bar rather than swallowed,
  // because "I emitted and nothing happened" is the debugging session this
  // line saves the panel's author.
  if (block.dataset.panel === "1") {
    if (name !== "params") {
      note(block, `a panel speaks emit("params", {...}); "${name}" went nowhere`);
      return;
    }
    const values = msg.data;
    if (!values || typeof values !== "object" || Array.isArray(values)) {
      note(block, 'emit("params", ...) needs an object of values');
      return;
    }
    panelCb?.(block.dataset.artifactId || "", values);
    return;
  }

  bridge.artifactEvent(block.dataset.artifactId || "", name, json);
}

// The panel sink: one listener, the surface that hosts assignment panels. The
// same single-callback arrangement as onArtifactFound, for the same reason -
// only one surface ever hosts one.
let panelCb = null;

// onPanelParams registers the handler for values a parameter panel emits. The
// callback gets (assignmentId, values) with values already vetted: a plain
// object that serialises under the size cap.
export function onPanelParams(cb) {
  panelCb = cb;
}

// pushPanelParams hands the current values to every panel frame under root, and
// remembers them on the block so a frame that has not finished loading (or is
// reloaded later) still gets them - run()'s load listener replays them. This is
// the inbound half of the panel's two-way channel; the bootstrap turns the
// message into window.agentbox.params plus an "agentbox:params" event.
export function pushPanelParams(root, values) {
  let json;
  try {
    json = JSON.stringify(values ?? {});
  } catch {
    return;
  }
  for (const block of root.querySelectorAll('.k-artifact[data-panel="1"]')) {
    block.dataset.panelParams = json;
    const frame = block.querySelector(".k-artifact-frame");
    frame?.contentWindow?.postMessage({ from: "agentbox", type: "params", values: JSON.parse(json) }, "*");
  }
}

// filled is true when the artifact has a window to itself: the reader marks its
// container, the frame fills what is there, and how tall the artifact would like
// to be stops being anyone's business.
function filled(block) {
  return !!block.closest('[data-fill="1"]');
}

function fit(block, height) {
  const frame = block.querySelector(".k-artifact-frame");
  if (!frame || filled(block)) return;
  // What it asked for is remembered, so a change to the ceiling can be applied to
  // an artifact that is already on screen without asking it to measure again.
  if (height > 0) block.dataset.wanted = String(Math.ceil(height));
  const wanted = Number(block.dataset.wanted || height || 0);
  const { max } = policy();
  const h = Math.max(MIN_HEIGHT, Math.min(Math.ceil(wanted), max));
  frame.style.height = `${h}px`;
  block.dataset.clipped = wanted > max ? "1" : "";
}

window.addEventListener("message", onMessage);

async function run(block) {
  const stage = block.querySelector(".k-artifact-stage");
  if (!stage) return; // refused by Go (too big); its source is all there is

  const { enabled } = policy();
  if (!enabled) {
    block.dataset.view = "code";
    block.dataset.blocked = "1";
    note(block, "artifacts are turned off ([artifact] enabled)");
    return;
  }
  delete block.dataset.blocked;

  const mod = await load();
  const built = mod.buildDocument({
    source: sourceOf(block),
    runtime: block.dataset.runtime || "html",
    react: block.dataset.react === "1",
    tailwind: block.dataset.tailwind === "1",
    tokens: tokensNow(),
  });
  if (!built.html) {
    block.dataset.view = "code";
    block.dataset.state = "error";
    note(block, built.notes[0] || "this artifact could not be prepared");
    return;
  }
  note(block, built.notes[0] || "");

  const frame = document.createElement("iframe");
  frame.className = "k-artifact-frame";
  // The two attributes the whole arrangement rests on: allow-scripts WITHOUT
  // allow-same-origin (an opaque origin cannot reach agentbox), and an empty allow
  // list (no camera, no microphone, no geolocation delegated to it).
  frame.setAttribute("sandbox", mod.SANDBOX);
  frame.setAttribute("allow", "");
  frame.setAttribute("referrerpolicy", "no-referrer");
  frame.setAttribute("title", "interactive artifact");
  if (!filled(block)) {
    // A starting height, until the artifact says how tall it is. Filling a window
    // is the other case, and there the stylesheet owns the height - an inline
    // style here would win against it and leave a short frame in a tall window.
    frame.style.height = "220px";
  }
  frame.srcdoc = built.html;

  const previous = stage.querySelector(".k-artifact-frame");
  if (previous) {
    frames.delete(previous.contentWindow);
    previous.remove();
  }
  stage.append(frame);
  frames.set(frame.contentWindow, block);
  block.dataset.state = "loading";
  // contentWindow changes when the document commits, so the map is refreshed on
  // load as well: without this a reload's messages arrive from a window nobody
  // recognises and are dropped.
  // A zoom chosen before this artifact loaded is applied on load too, or a
  // reload would quietly reset the size.
  frame.addEventListener("load", () => {
    frames.set(frame.contentWindow, block);
    if (artZoom !== 1) {
      const css = styleWithZoom();
      if (css) frame.contentWindow?.postMessage({ from: "agentbox", type: "style", css }, "*");
    }
    // A find typed before this artifact (re)loaded is re-run in the fresh
    // document, or a reload would quietly blank the matches.
    if (artFind) frame.contentWindow?.postMessage({ from: "agentbox", type: "find", query: artFind }, "*");
    // A panel's values arrive whenever the surface pushes them, which can be
    // before the frame exists; replaying the remembered set here means load
    // order does not matter and a reload keeps its values.
    if (block.dataset.panel === "1" && block.dataset.panelParams) {
      try {
        frame.contentWindow?.postMessage(
          { from: "agentbox", type: "params", values: JSON.parse(block.dataset.panelParams) }, "*");
      } catch {
        // a torn dataset write is not worth a broken load
      }
    }
  });
}

function stop(block) {
  const frame = block.querySelector(".k-artifact-frame");
  if (!frame) return;
  frames.delete(frame.contentWindow);
  frame.remove();
}

// hydrate runs every artifact in a container that is not already running. Called
// from the markdown action, so a streaming agent turn hydrates the artifact it
// just finished writing and leaves the earlier ones alone.
export function hydrateArtifacts(root) {
  for (const block of root.querySelectorAll(".k-artifact:not([data-live])")) {
    block.dataset.live = "1";
    run(block);
  }
}

// onArtifactClick handles the chrome: the code/preview toggle and reload. It is
// called from the markdown action's click handler, which already owns clicks in
// rendered content, and reports whether the click was one of ours.
export function onArtifactClick(target) {
  const block = target.closest?.(".k-artifact");
  if (!block) return false;

  const tab = target.closest("[data-artifact-view]");
  if (tab) {
    block.dataset.view = tab.dataset.artifactView;
    return true;
  }
  if (target.closest("[data-artifact-run]")) {
    note(block, "");
    delete block.dataset.state;
    run(block);
    return true;
  }
  return false;
}

// refreshArtifacts applies a config change to what is already on screen, which is
// three separate promises kept:
//
//   * the trust switch is retroactive. Turn artifacts off and a frame that is
//     running right now is removed, not hidden - and turning them back on starts
//     the ones that were refused, without touching a code tab you chose yourself.
//   * a new ceiling re-clamps an artifact that is already tall.
//   * a theme change repaints rather than reloads. Rebuilding would be simpler
//     and wrong: it would throw away whatever the user had typed into it.
export function refreshArtifacts(root) {
  const { enabled } = policy();
  const css = styleWithZoom();

  for (const block of root.querySelectorAll(".k-artifact")) {
    if (!enabled) {
      stop(block);
      block.dataset.view = "code";
      block.dataset.blocked = "1";
      note(block, "artifacts are turned off ([artifact] enabled)");
      continue;
    }
    if (block.dataset.blocked === "1") {
      delete block.dataset.blocked;
      note(block, "");
      block.dataset.view = "preview";
      run(block);
      continue;
    }
    fit(block, 0);
    const frame = block.querySelector(".k-artifact-frame");
    if (frame && css) frame.contentWindow?.postMessage({ from: "agentbox", type: "style", css }, "*");
  }
}
