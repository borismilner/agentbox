// The wiki's frames, as data (FR99).
//
// One entry per frame in docs/wiki/DESIGN.md section 5. The copy here is that
// specification's copy, verbatim where it is quoted there, because the point of
// drawing rather than photographing is that the words on screen are the words the
// page author chose - not whatever a staging script could get a live daemon to
// produce.
//
// Each entry is:
//   out       the file name the wiki pages already reference, so a redrawn frame
//             replaces its photograph rather than arriving beside it
//   surface   which of main.js's surfaces to mount (the ?surface= value)
//   width     the viewport, in CSS px. The frameless surfaces size themselves,
//             so this is the window Go would have opened, not a crop.
//   height    viewport height; "fit" measures the surface after it settles.
//   ground    how much desktop to leave around the surface, so its shadow reads
//   calls     answers for bridge.* methods, keyed by the short method name
//   events    a payload per agentbox:* event, delivered once the surface subscribes
//
// The wire shapes are the daemon's, and the fixtures below carry the same fields
// internal/webui builds - which is the check that keeps a drawn frame honest. A
// field the surface reads and the daemon never sets would render here and never
// on a desktop, so anything added must be traceable to the Go side.

// Every countdown reads Date.now() through lib/clock.svelte.js, so the frames
// freeze it (index.html) and compute deadlines from the same instant. Without
// that a frame is a different picture every time it is drawn, and a re-run shows
// up as a diff nobody can review.
export const NOW = Date.UTC(2026, 6, 30, 10, 12, 0);

// The identity hues the daemon would compute for the agents queued behind a card.
// Taken from the product's own hash rather than written down as colours, because
// a hand-picked colour is exactly the kind of thing a drawing can get wrong and a
// photograph cannot: the dots would be a shade no real agent has. IdentityPill
// already derives its own, so only the loose ones (waitingHues, which Go sends
// rather than the surface deriving) need this.
import { identityHue } from "../src/lib/tokens.js";

const hue = (agent, project = "checkout-api") => identityHue(agent, project);

const card = (item, extra = {}) => ({
  item: {
    id: "itm-1",
    kind: "choice",
    level: "info",
    body: "",
    strict: false,
    ...item,
  },
  bodyHtml: "",
  waiting: 0,
  waitingHues: [],
  graced: false,
  gracedText: "",
  graceUntilMs: 0,
  dismissAtMs: 0,
  expiresAtMs: 0,
  actionsEnabled: false,
  caller: "live",
  glyph: "info",
  sticky: false,
  ...extra,
});

export const FRAMES = {
  // S1. The choice card. home.md and the-card.md.
  //
  // `2 waiting` is doing real work in this frame and DESIGN.md says so: it is the
  // proof that a second agent's question is visible while this one is up. The two
  // hues are what the daemon sends for the agents behind it - test-runner and
  // dependency-bot on checkout-api, the same fiction as S2.
  s1: {
    out: "card.png",
    surface: "card",
    width: 470,
    height: "fit",
    ground: 24,
    // Keyed by the Go method name, which is what bridge.js builds its FQN from.
    // An empty theme is deliberate and is the whole of FR99's honesty rule in one
    // field: app.css's own defaults then decide every colour, so the frame wears
    // the product's tokens rather than a palette written for a picture.
    calls: { Theme: {}, Ready: "" },
    events: {
      "agentbox:view": card(
        {
          title: "Where should 2026.7.30 go first?",
          body: "Tests are green and the changelog is written. The canary starts at 10% of live traffic wherever we begin.",
          identity: { agent: "release-bot", project: "checkout-api" },
          options: [
            { label: "eu-west", desc: "closest to the traffic peak" },
            { label: "us-east", desc: "quietest region right now" },
            { label: "Hold", desc: "stay on 2026.7.22" },
          ],
        },
        {
          bodyHtml:
            "<p>Tests are green and the changelog is written. The canary starts at 10% of live traffic wherever we begin.</p>",
          // "expires in 1:57", per the spec's footer.
          expiresAtMs: NOW + 117_000,
          waiting: 2,
          waitingHues: [hue("test-runner"), hue("dependency-bot")],
        },
      ),
    },
  },
};
