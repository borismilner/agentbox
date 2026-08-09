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
//   query     any more of the query string the surface reads, e.g. tab=agents
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

// S2's roster. The field names are internal/webui/agents.go's wire tags, and the
// *_since_ms fields are DURATIONS rather than timestamps - Agents.svelte's ago()
// adds its own elapsed to them, which is what lets one payload keep counting.
const roster = () => ({
  agents: [
    {
      key: "k-release", agent: "release-bot", project: "checkout-api",
      hue: hue("release-bot"), area: "checkout-api", area_label: "checkout-api",
      purpose: "cutting release 2026.7.30",
      activity: "waiting on the region choice",
      state: "asking", activity_since_ms: 40_000, age_ms: 22 * 60_000,
      holds: [{ name: "deploy:checkout-api", since_ms: 6 * 60_000, waiters: 1 }],
      wait: null, tags: [], timeline: [],
    },
    {
      key: "k-tests", agent: "test-runner", project: "checkout-api",
      hue: hue("test-runner"), area: "checkout-api", area_label: "checkout-api",
      purpose: "running the pre-release suite",
      activity: "parked on the deploy lock",
      state: "blocked", activity_since_ms: 6 * 60_000, age_ms: 31 * 60_000,
      holds: [],
      // The whole point of the row: blocked names who it is blocked BY, which is
      // what "blocked" alone never told anybody.
      wait: {
        lock: "deploy:checkout-api", holder_key: "k-release", holder: "release-bot",
        since_ms: 6 * 60_000, place: 2, queue: 2,
      },
      tags: [], timeline: [],
    },
    {
      key: "k-deps", agent: "dependency-bot", project: "checkout-api",
      hue: hue("dependency-bot"), area: "checkout-api", area_label: "checkout-api",
      purpose: "auditing transitive dependencies",
      activity: "waiting for the suite to go green",
      // listening is the chip that looks like blocked and means the opposite:
      // parked in await_signal is the feature working.
      state: "listening", detail: "tests:green",
      activity_since_ms: 2 * 60_000 + 10_000, age_ms: 18 * 60_000,
      holds: [], wait: null, tags: [], timeline: [],
    },
    {
      // The dim row. DESIGN.md: "a board that only shows well-behaved agents is
      // not believable." No purpose, so the surface says so rather than guessing.
      key: "k-anon", agent: "claude", project: "checkout-api",
      hue: hue("claude"), area: "checkout-api", area_label: "checkout-api",
      purpose: "", activity: "",
      state: "unannounced", activity_since_ms: 0, age_ms: 4 * 60_000,
      holds: [], wait: null, tags: [], timeline: [],
    },
  ],
  orphans: [],
  shared: [
    { key: "release/region", value: "eu-west", version: 3, owner: "k-release",
      owner_name: "release-bot", owner_gone: false, since_ms: 40_000 },
    { key: "suite/shard-1", value: "running", version: 1, owner: "k-tests",
      owner_name: "test-runner", owner_gone: false, since_ms: 6 * 60_000 },
    // The abandoned claim, which is the reason the count is in the warning
    // colour: a claim whose owner died reads as abandoned rather than blocking
    // everybody else.
    { key: "suite/shard-2", value: "running", version: 1, owner: "k-gone",
      owner_name: "test-runner", owner_gone: true, since_ms: 14 * 60_000 },
  ],
  partial: false,
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

  // S2. The Agents board. agents-board.md and taking-turns.md.
  //
  // DESIGN.md calls this the single most persuasive frame in the wiki, and the
  // reason is the four chips read together: asking you, blocked, listening, and
  // a row with no purpose at all. Its caption is about the middle two - blocked
  // and listening look alike and mean opposite things - so a frame missing
  // either of them is the wrong frame, not a slightly worse one.
  //
  // This is also the frame the photographer could not get. The 2026-08-07
  // sitting produced seven roster rows where the page wants four, because
  // `sync lock` mints a session key per lock and every lock became a row. That
  // is not a product defect and not something a staging script can talk the
  // desktop out of; it is exactly what FR99 said drawing was for.
  s2: {
    out: "agents-board.png",
    surface: "app",
    query: "tab=agents",
    // The app window Go opens: config.Window.AppWidth / AppHeight defaults
    // (internal/config/config.go). The spec asks for the full window width
    // because the wait line naming the holder lives on the right.
    width: 1180,
    // Shorter than the 860 Go opens, and deliberately so. Four agents in an
    // 860px window leave 160px of empty board under the last row, which reads on
    // a wiki page as a screen with nothing in it - DESIGN.md's own warning about
    // photographing an empty inbox, arriving from the other direction. This is a
    // window height a human could drag to, not a crop that hides anything: the
    // footer strip is still pinned under the last row, where it lives.
    height: 720,
    // "Crop to the window ... exclude the desktop behind it", so no ground.
    ground: 0,
    calls: {
      Theme: {},
      Ready: "",
      // The shell pulls both of these on mount whichever tab is in front, and a
      // roster frame that left them undefined would paint a rail badge and a
      // status strip that no real window shows.
      Sessions: [],
      Inbox: { items: [], pending: 1, today: 6, muted: [] },
      Agents: roster(),
    },
    events: { "agentbox:agents": roster() },
  },
};
