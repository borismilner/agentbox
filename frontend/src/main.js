import { mount } from "svelte";
import "./app.css";
import { bridge, on, surfaceName } from "./lib/bridge.js";
import { applyTheme } from "./lib/tokens.js";
import Card from "./surfaces/Card.svelte";
import Toast from "./surfaces/Toast.svelte";
import Viewer from "./surfaces/Viewer.svelte";
import Progress from "./surfaces/Progress.svelte";
import App from "./surfaces/App.svelte";
import Panel from "./surfaces/Panel.svelte";
import Control from "./surfaces/Control.svelte";
import Mark from "./surfaces/Mark.svelte";

// Theme arrives before anything renders and again whenever the config
// changes, so every window re-themes together without a restart. The window event
// is for the things a CSS variable write cannot apply by itself: a mermaid diagram
// bakes its colours into the SVG and has to be redrawn, and an artifact lives in
// an opaque origin that cannot see this stylesheet at all, so its colours and the
// policy it runs under are handed to it (markdown.svelte.js).
on("agentbox:theme", (t) => {
  applyTheme(t);
  window.dispatchEvent(new CustomEvent("agentbox:themed"));
});

const surfaces = {
  card: Card,
  toast: Toast,
  viewer: Viewer,
  progress: Progress,
  app: App,
  panel: Panel,
  control: Control,
  mark: Mark,
};

const surface = surfaceName();

// The tokens have to be on :root before the first paint: on a light desktop a
// late theme means one dark frame, which on a frameless card reads as a flash.
// Wails serves Options.Flags over a runtime call rather than injecting them into
// the page, so the theme cannot be read synchronously - we ask Go for it and race
// the answer against a deadline, because chrome is worth a few milliseconds and a
// blocking question is not.
async function start() {
  await Promise.race([
    bridge
      .theme()
      .then(applyTheme)
      .catch(() => {}),
    new Promise((done) => setTimeout(done, 150)),
  ]);
  // The board is the one lazy surface: it is the largest thing in the
  // bundle and no card, toast or viewer should ever pay for it (NFR2).
  const Component =
    surface === "board" ? (await import("./surfaces/Board.svelte")).default : (surfaces[surface] ?? App);
  mount(Component, { target: document.getElementById("root"), props: { surface } });
}

start();
