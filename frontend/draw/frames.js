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
//   width     the window, in CSS px. The frameless surfaces size themselves, so
//             this is the window Go would have opened, not a crop.
//   height    window height; "fit" measures the surface after it settles.
//   ground    how much desktop to leave around the surface, so its shadow reads
//   desk      {width, height, place} for the three frames whose subject is WHERE
//             they sit. The picture is then that piece of screen and the surface
//             sits in it, in an iframe the size of the window (see desk.html).
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

// The HTML the frames' documents are made of, rendered by Go from the markdown
// in draw/md/ (tools/wiki/drawhtml). Headings, tables, highlighted code, the
// artifact's chrome and its sandbox are all decisions internal/webui makes, so a
// fixture that wrote that HTML by hand would be drawing a renderer nobody ships.
import { HTML } from "./rendered.js";

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

// S3's queue. The field names are internal/webui/inbox.go's wire tags; `hint` is
// what triageHint() builds for that item's kind, so the keys under the selected
// row are the keys Go would really act on rather than a plausible list.
const row = (o) => ({
  id: o.id,
  kind: o.kind,
  level: o.level ?? "info",
  title: o.title,
  snippet: o.snippet ?? "",
  agent: o.agent,
  project: "checkout-api",
  hue: hue(o.agent),
  pending: !!o.pending,
  blocking: !!o.blocking,
  muted: false,
  outcome: o.outcome,
  tone: o.tone,
  hint: o.hint ?? "",
  createdMs: NOW - o.ago,
});

const MIN = 60_000;

const inbox = () => ({
  items: [
    // Pending, and first, because the surface orders pendingFirst. The selected
    // one is a confirm, so its hint is the confirm keymap: y, n, then the two
    // every kind carries.
    row({
      id: "q-cut", kind: "confirm", agent: "release-bot", pending: true, blocking: true,
      title: "Cut 2026.7.30 from the release branch?",
      snippet: "The changelog is written and the migration is reversible. Cutting tags the branch and starts the canary.",
      hint: "y yes · n no · d dismiss · c copy",
      outcome: "waiting", tone: "info", ago: 34 * MIN,
    }),
    row({
      id: "q-token", kind: "secret", agent: "oncall-helper", pending: true, blocking: true,
      title: "Staging token for the canary run",
      snippet: "It is read once, at the start of the run, and never enters the conversation.",
      hint: "enter open · d dismiss · c copy",
      outcome: "waiting", tone: "info", ago: 12 * MIN,
    }),
    // Then the hour you were not there for. The outcome column is the point of
    // the frame, so every tone the column can wear appears once.
    row({
      id: "q-region", kind: "choice", agent: "release-bot",
      title: "Where should 2026.7.30 go first?",
      snippet: "Tests are green and the changelog is written. The canary starts at 10% of live traffic wherever we begin.",
      outcome: "eu-west", tone: "success", ago: 47 * MIN,
    }),
    row({
      id: "q-diff", kind: "diff", agent: "oncall-helper",
      title: "Rotate the staging token before the canary",
      snippet: "One file, six lines: the token moves to the secrets path and the old constant goes.",
      outcome: "approved", tone: "success", ago: 58 * MIN,
    }),
    row({
      id: "q-veto", kind: "veto", agent: "test-runner",
      title: "Skipping the slow suite for this run",
      snippet: "It last failed in April and adds 22 minutes. Stop me within 30s if that is wrong.",
      outcome: "proceeded", tone: "success", ago: 71 * MIN,
    }),
    // Nobody answered this one, and it stays in frame on purpose.
    row({
      id: "q-changelog", kind: "choice", agent: "dependency-bot",
      title: "Should the changelog mention the index migration?",
      snippet: "It is invisible to callers but it is the only thing in this release that touches the write path.",
      outcome: "expired", tone: "muted", ago: 96 * MIN,
    }),
  ],
  pending: 2,
  today: 11,
  muted: [],
});

// S10's bars. FR21's shape: a long job reports rather than asks, so there is no
// answer zone and no place in the queue. The third is indeterminate, which is
// the state a task is in before it knows how much work it has.
const progress = () => [
  { id: "p-index", title: "Reindexing the search catalogue", status: "12,400 of 19,300 documents",
    percent: 64, indeterminate: false, agent: "release-bot", project: "checkout-api", hue: hue("release-bot") },
  { id: "p-backfill", title: "Backfill events.region", status: "shard 2 of 8",
    percent: 12, indeterminate: false, agent: "test-runner", project: "checkout-api", hue: hue("test-runner") },
  { id: "p-cdn", title: "Warming the CDN", status: "starting",
    percent: 0, indeterminate: true, agent: "oncall-helper", project: "checkout-api", hue: hue("oncall-helper") },
];

// S5 and S6. daemon.ControlState's wire tags; since_ms and paused_ms are ages
// the daemon derives rather than timestamps, so a strip that reopens mid-run
// does not restart the clock.
const driving = () => ({
  id: "run-1",
  state: "driving",
  reason: "rotating the staging secret in the console",
  activity: "renaming the staging secret in the console",
  identity: { agent: "oncall-helper", project: "checkout-api" },
  since_ms: 4_000,
});

const paused = () => ({
  ...driving(),
  paused: true,
  // Under two minutes: past that the strip escalates to AGENT WAITING in amber,
  // which is a third state and not what this pair is showing.
  paused_ms: 21_000,
  waiting: true,
});

// S11's console. internal/webui/sessions.go's wire tags: the selected session
// carries the whole conversation, and `ask` is the one field that rides a row
// whether it is selected or not, because a switcher row has to be able to say
// that this conversation is the one waiting on you.
const sessions = () => [
  {
    id: "s-canary",
    title: "checkout-api · rollback",
    project: "checkout-api",
    cwd: "~/work/checkout-api",
    mode: "full",
    state: "idle",
    hue: hue("claude-code"),
    turns: 3,
    selected: true,
    model: "claude-opus-5",
    conv: [
      {
        role: "user",
        segments: [
          {
            kind: "text",
            text: "Before we start the canary - is the rollback path actually fast enough to matter?",
            html: "<p>Before we start the canary - is the rollback path actually fast enough to matter?</p>",
          },
        ],
      },
      {
        role: "assistant",
        at: "14:38",
        think: "1m12s",
        model: "claude-opus-5",
        segments: [
          { kind: "tool", toolName: "read", toolInput: "internal/rollout/withdraw.go", hasResult: true },
          { kind: "text", html: HTML.turn },
        ],
      },
    ],
    ask: {
      id: "ask-1",
      kind: "choice",
      level: "info",
      glyph: "info",
      lead: "release-bot is asking",
      title: "Reorder withdraw() before the canary, or after it?",
      bodyHtml:
        "<p>Reordering is four lines and no behaviour change on the happy path. Doing it first means the canary is protected by the fix it is about to test.</p>",
      hue: hue("release-bot"),
      options: [
        { label: "Before", desc: "the canary runs with the fix in", key: "1", answer: "Before", primary: true },
        { label: "After", desc: "ship the canary on what is already reviewed", key: "2", answer: "After" },
      ],
      hint: "1 · 2 to answer, or keep typing",
      expiresAtMs: NOW + 231_000,
    },
  },
  {
    id: "s-suite",
    title: "checkout-api · flaky suite",
    project: "checkout-api",
    cwd: "~/work/checkout-api",
    mode: "plan",
    state: "working",
    hue: hue("claude-code", "checkout-api-suite"),
    turns: 7,
  },
];

// S15. A step's body: a figure, a table and a callout (FR101).
//
// The frame the photographer must not take. A real review board is a live
// walkthrough on Boris's own desktop, so photographing one puts a window over
// whatever he is working on - which is exactly what happened while this feature
// was being built, and it is why this fixture exists. It also draws BOTH themes,
// which no single sitting at a screen can do.
//
// The fixture carries no tldr on purpose: the board opens in TL;DR and this frame
// is about what is under it. A step with no TL;DR renders in full and the toggle
// disappears, which is the shape a reader of this frame should see.
function bodyReview() {
  const diagram = `<svg viewBox="0 0 700 150">
  <defs>
    <marker id="a" markerWidth="9" markerHeight="9" refX="8" refY="4.5" orient="auto" markerUnits="strokeWidth">
      <path d="M0 0L9 4.5L0 9z" fill="currentColor"/>
    </marker>
  </defs>
  <g stroke="var(--k-edge)" fill="var(--k-surface-2)" stroke-width="1">
    <rect x="4" y="30" width="150" height="60" rx="10"/>
    <rect x="206" y="30" width="150" height="60" rx="10"/>
    <rect x="408" y="30" width="150" height="60" rx="10"/>
    <rect x="610" y="30" width="86" height="60" rx="10"/>
  </g>
  <g stroke="var(--k-accent)" color="var(--k-accent)" stroke-width="1.6" fill="none">
    <line x1="156" y1="60" x2="198" y2="60" marker-end="url(#a)"/>
    <line x1="358" y1="60" x2="400" y2="60" marker-end="url(#a)"/>
    <line x1="560" y1="60" x2="602" y2="60" marker-end="url(#a)"/>
  </g>
  <g font-size="15" text-anchor="middle">
    <text x="79" y="58">the spec</text>
    <text x="281" y="58">SafeSVG</text>
    <text x="483" y="58">the wire</text>
    <text x="653" y="58">the board</text>
  </g>
  <g fill="var(--k-ink-3)" font-size="12.5" text-anchor="middle">
    <text x="79" y="78">figure.svg</text>
    <text x="281" y="78">allow-list</text>
    <text x="483" y="78">fig.svg</text>
    <text x="653" y="78">one img</text>
    <text x="79" y="112">what the agent wrote</text>
    <text x="483" y="112">bytes Go wrote</text>
    <text x="653" y="112">the theme sizes it</text>
  </g>
  <g fill="var(--k-warning)" font-size="12.5" text-anchor="middle">
    <text x="281" y="112">a literal colour stops here</text>
  </g>
</svg>`;
  return {
    id: "w8db488ad6750",
    title: "A walkthrough can draw, tabulate and warn",
    repo: "~/me/projects/agentbox",
    root: "/home/boris/me/projects/agentbox",
    pinned: "3dbd94fa638c",
    state: "open",
    rev: 1,
    pos: 0,
    revMs: NOW,
    marks: {},
    comments: [],
    steps: [
      {
        id: "shape",
        kind: "ground",
        title: "The path a picture takes",
        purpose: "",
        prose: [
          { t: "A figure is the only place a walkthrough carries markup rather than text, so the second station below is where every rule lives." },
        ],
        body: [
          {
            kind: "figure",
            lead: "The drawing takes its colours from the same tokens the page does, which is why it follows the theme rather than freezing in one.",
            figure: {
              svg: diagram,
              alt: "the spec's svg goes through SafeSVG, then the wire, then the board",
              caption: "Markup an agent writes is never filtered and passed on. It is refused, or re-composed element by element into bytes Go wrote itself.",
              wide: true,
            },
          },
          {
            kind: "table",
            lead: "The caps a body carries, so a step stays a step.",
            table: {
              head: ["what", "cap", "why that number"],
              rows: [
                ["blocks in one body", "14", "a diagram beside two citations needs the room"],
                ["svg bytes", "98304", "past that it is a screenshot, and src takes one of those"],
                ["table rows", "40", "past that it is data, and data belongs in a file the step cites"],
                ["image pixels", "40000000", "bytes of RGBA are width x height x 4, and the webview pays it"],
              ],
              align: ["left", "right", "left"],
              caption: "Each one is enforced in validation and stated in the error, so an author is told the number rather than guessing it.",
            },
          },
          {
            kind: "callout",
            callout: {
              tone: "warn",
              title: "A figure never declares a colour of its own",
              prose: [
                { t: "fill and stroke must be currentColor, a gradient defined in the same figure, or a --k-* token. A literal is refused with the list of tokens to use instead, because a drawing that hard-codes a palette looks right in one theme and wrong in the other." },
              ],
            },
          },
        ],
      },
    ],
  };
}

export const FRAMES = {
  // S1. The choice card. home.md and the-card.md.
  //
  // `2 waiting` is doing real work in this frame and DESIGN.md says so: it is the
  // proof that a second agent's question is visible while this one is up. The two
  // hues are what the daemon sends for the agents behind it - test-runner and
  // dependency-bot on checkout-api, the same fiction as S2.
  // The dark one, and its sibling below is the same fixture in the light theme.
  // Two frames rather than one because a theme is a fixture answer, and a drawing
  // that only ever existed in dark mode is half the claim.
  //
  // No desk on either of them, and that is not a style choice. The board is the
  // one LAZY surface (main.js imports it on demand), so data-drawn - which fires
  // two animation frames after the module loads - is set before the board's chunk
  // has painted. The readiness selector is what waits for the real thing, and
  // draw.py only passes one to a frame that has no desk, because a selector
  // cannot reach inside the desktop's iframe. A desk frame here photographs an
  // empty window; that is what the first run of this frame produced.
  s15: {
    out: "review-body.png",
    surface: "board",
    width: 1180,
    height: 900,
    ready: "aside.callout",
    calls: { Theme: {}, Ready: "", Board: bodyReview() },
  },

  s15l: {
    out: "review-body-light.png",
    surface: "board",
    width: 1180,
    height: 900,
    ready: "aside.callout",
    calls: { Theme: { mode: "light" }, Ready: "", Board: bodyReview() },
  },

  s1: {
    out: "card.png",
    surface: "card",
    width: 470,
    height: "fit",
    // On the desktop rather than on a rectangle of ground. A card is a
    // TRANSPARENT frameless window (webui.go), so what puts a shadow under it on
    // a real screen is the desktop, and a frame cropped flush to the card has
    // neither the shadow nor anything for it to fall on.
    desk: { width: 860, height: 560, place: "centre" },
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
    desk: { width: 1360, height: 880, place: "centre" },
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

  // S3. The inbox, after an hour away. nothing-gets-lost.md.
  //
  // The frame has to carry two things at once: that the pending rows take the
  // same keys the card takes, and that the day's resolved questions are still
  // readable. So the selected row states its keys out loud, and the outcome
  // column keeps a row nobody answered - DESIGN.md: "a wiki that only shows
  // answered rows is selling a fantasy".
  s3: {
    out: "inbox.png",
    surface: "app",
    query: "tab=inbox",
    width: 1180,
    height: 700,
    desk: { width: 1360, height: 860, place: "centre" },
    calls: {
      Theme: {},
      Ready: "",
      Sessions: [],
      // The shell pulls the roster whichever tab is in front, for the rail's dot.
      Agents: roster(),
      Inbox: inbox(),
    },
    events: { "agentbox:inbox": inbox() },
  },

  // S9. A toast. notifications.md.
  //
  // A warning, which is the level that does NOT count down: the countdown is
  // absent because a warning waits to be read, and the frame is evidence for
  // that sentence. Drawn on a desktop because where it sits - the top edge, the
  // same column the hands-off strip claims - is half of what it means.
  s9: {
    out: "toast.png",
    surface: "toast",
    width: 430, // window.toast_width
    height: "fit",
    desk: { width: 900, height: 330, place: "top" },
    calls: { Theme: {}, Ready: "" },
    events: {
      "agentbox:view": card(
        {
          id: "itm-toast",
          kind: "notify",
          level: "warning",
          title: "Two transitive dependencies moved to a yanked version",
          body: "Both are in the 2026.7.30 candidate. `left-pad@1.3.0` and `qs@6.5.2` were yanked upstream this morning.",
          identity: { agent: "dependency-bot", project: "checkout-api" },
        },
        {
          bodyHtml:
            "<p>Both are in the 2026.7.30 candidate. <code>left-pad@1.3.0</code> and <code>qs@6.5.2</code> were yanked upstream this morning.</p>",
          glyph: "warning",
          // No dismissAtMs at all, which is the point of the frame: the daemon
          // gives warning and error notices no dismiss deadline, so the strip
          // waits rather than counting down.
          sticky: true,
        },
      ),
    },
  },

  // S10. The progress window. notifications.md.
  //
  // The one frame where cropping tight would destroy the point: "bottom right,
  // never focused" is a claim about the screen, and a picture of the window
  // alone cannot make it. The corner inset is x11.corner's own - 28 from the
  // right, 52 from the bottom - rather than a number chosen to look right.
  s10: {
    out: "progress.png",
    surface: "progress",
    width: 400, // window.progress_width
    height: "fit",
    desk: { width: 900, height: 560, place: "corner" },
    calls: {
      Theme: {},
      Ready: "",
      Progress: progress(),
    },
    events: { "agentbox:progress": progress() },
  },

  // S5. The hands-off strip, driving. hands-off.md.
  //
  // The pair S5/S6 is the one place DESIGN.md allows two images on a page, and
  // the reason is that presence is the feature: one frame cannot show a state
  // change. Both are drawn over the SAME desktop, which is what makes the pair
  // legible as one screen changing rather than two screenshots.
  s5: {
    out: "hands-off.png",
    surface: "control",
    width: 620, // controlW
    height: 62, // controlH
    desk: { width: 1120, height: 360, place: "top" },
    calls: { Theme: {}, Ready: "", Control: driving() },
    events: { "agentbox:control": driving() },
  },

  // S6. The same strip, paused (FR94).
  //
  // Green, the label flipped, and the activity line still readable so a glance
  // says what the desktop goes back to. The counter is under two minutes on
  // purpose: past that the strip escalates to AGENT WAITING in amber, which is a
  // third state and not what this pair is about.
  s6: {
    out: "hands-off-paused.png",
    surface: "control",
    width: 620,
    height: 62,
    desk: { width: 1120, height: 360, place: "top" },
    calls: { Theme: {}, Ready: "", Control: paused() },
    events: { "agentbox:control": paused() },
  },

  // S7. An artifact running. documents-and-artifacts.md.
  //
  // The source is a React module (draw/md/canary.artifact.jsx) and Go decides
  // the rest: specFor reads `export default` and answers react, which brings
  // React and Tailwind into the sandbox, and the bar's runtime label follows
  // from that rather than from anything written here. The frame's claim is the
  // code toggle beside the badge, so the toggle has to be in it.
  s7: {
    out: "artifact.png",
    surface: "viewer",
    width: 900, // window.viewer_width
    // Shorter than the 780 Go opens, and a judgement like s2's: the stage flexes,
    // so a taller window is not more artifact, it is more empty stage under one.
    height: 470,
    // The one frame that needs telling when it is done. The stage's iframe is in
    // the DOM long before the program inside it has mounted, so "the page has
    // painted" is not the question - a frame with a box is.
    desk: { width: 1120, height: 640, place: "centre" },
    ready: ".k-artifact-frame",
    calls: {
      // artifactsEnabled is the trust switch, and it rides the theme. The
      // default is on ([artifact] enabled, config.go), so this is the product's
      // own answer rather than a fixture turning something on to get a picture.
      Theme: { artifactsEnabled: true },
      Ready: "",
      Document: {
        title: "How much traffic should the new build take?",
        html: HTML.canary,
        artifact: true,
        watch: false,
        empty: false,
        revMs: NOW,
      },
    },
  },

  // S8. The reading window. documents-and-artifacts.md.
  //
  // A table, a mermaid diagram and a highlighted code block in one frame,
  // because between them they are everything a terminal flattens. The watching
  // badge is lit: `--watch` is the claim the page makes right under it.
  s8: {
    out: "viewer.png",
    surface: "viewer",
    width: 900,
    height: 780, // window.viewer_height
    desk: { width: 1120, height: 940, place: "centre" },
    calls: {
      Theme: {},
      Ready: "",
      Document: {
        title: "release-2026.7.30.md",
        path: "~/checkout-api/reports/release-2026.7.30.md",
        html: HTML.report,
        watch: true,
        empty: false,
        revMs: NOW,
      },
    },
  },

  // S11. The drop-down console. sessions.md.
  //
  // The point is the inline ask sitting above the composer: the agent asking is
  // the one inside this window, so its question renders in the conversation it
  // is about instead of taking a card of its own - and it never takes the
  // composer's keyboard. Drawn over a desktop because a console rolled down over
  // somebody's work is the feature; cropped to the window it is just a chat.
  s11: {
    out: "panel.png",
    surface: "panel",
    // [panel] width_frac 0.74 and height_frac 0.5 of a 1920x1200 monitor, the
    // arithmetic in panel.sizeOn.
    width: 1420,
    height: 600,
    desk: { width: 1920, height: 700, place: "drop" },
    calls: { Theme: {}, Ready: "", Sessions: sessions() },
    events: { "agentbox:sessions": sessions() },
  },

  // S14. The diff review card. README.md.
  //
  // The one kind that is read before it is answered, so it renders the diff
  // rather than dumping it, and the keys are its own: a approves, r asks for
  // changes, and the reply box is the comment that travels back with either.
  // The change is the one the console frame is arguing about, so the two frames
  // are the same afternoon.
  s14: {
    out: "review.png",
    surface: "card",
    width: 470,
    height: "fit",
    desk: { width: 900, height: 700, place: "centre" },
    calls: { Theme: {}, Ready: "" },
    events: {
      "agentbox:view": card(
        {
          id: "itm-diff",
          kind: "diff",
          title: "Swap the router entry before draining",
          body: "Four lines, no behaviour change on the happy path.",
          identity: { agent: "release-bot", project: "checkout-api" },
          // Kept narrow on purpose. The card is 470px and scrolls a long line
          // rather than wrapping it, which is right on a desktop and reads as a
          // clipped picture on a page.
          diff: `--- a/internal/rollout/withdraw.go
+++ b/internal/rollout/withdraw.go
@@ -18,8 +18,9 @@ func (r *Rollout) withdraw(
-\tif err := r.drain(ctx, build); err != nil {
+\t// Swap first, or the drain feeds the
+\t// build being withdrawn.
+\tif err := r.point(ctx, r.previous); err != nil {
 \t\treturn err
 \t}
-\treturn r.point(ctx, r.previous)
+\treturn r.drain(ctx, build)
 }
`,
        },
        {
          bodyHtml: "<p>Four lines, no behaviour change on the happy path.</p>",
          expiresAtMs: NOW + 271_000,
        },
      ),
    },
  },

  // S13. The secret card. is-it-safe.md, where the page has been carrying a
  // SHOT: placeholder since the wiki was written.
  //
  // The destination is on screen BEFORE anything is typed, which is the whole
  // claim: the value goes to a 0600 file and the agent is handed the path. An
  // empty field is the honest frame for that sentence - a field with dots in it
  // would be a picture of the moment after the promise stopped mattering.
  s13: {
    out: "secret.png",
    surface: "card",
    width: 470,
    height: "fit",
    desk: { width: 860, height: 570, place: "centre" },
    calls: { Theme: {}, Ready: "" },
    events: {
      "agentbox:view": card(
        {
          id: "itm-secret",
          kind: "secret",
          title: "Staging token for the canary run",
          body: "It is read once, at the start of the run, and never enters the conversation.",
          identity: { agent: "oncall-helper", project: "checkout-api" },
          // The daemon mints this path per item; the shape is XDG_RUNTIME_DIR.
          sink: "/run/user/1000/agentbox/secrets/8f3c1d2a.secret",
        },
        {
          bodyHtml: "<p>It is read once, at the start of the run, and never enters the conversation.</p>",
          expiresAtMs: NOW + 174_000,
        },
      ),
    },
  },
};
