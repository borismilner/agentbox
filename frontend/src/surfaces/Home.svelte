<script>
  // Home (FR81). The app used to open on Session, and Session is empty until
  // you start one - so the first thing agentbox showed a person who opened it was a
  // blank column and a "+ New" button. Boris: "it goes straight to SESSIONS
  // which is empty and is not that important functionality. It should show
  // interesting information with configurable panels and along the way it
  // should let me interact with all the real functionalities."
  //
  // So this surface has one job with two halves: say something true about right
  // now, and be a door into every other surface. Nothing here is a dead end -
  // every tile and every row goes somewhere.
  //
  // Panels are configurable because a dashboard nobody can turn off is a
  // dashboard somebody stops reading. The choice lives in localStorage rather
  // than in config.toml: it is a per-machine view preference, not a setting
  // worth a knob in the settings surface, and the webview's origin is stable so
  // it survives a restart.
  import { bridge, on } from "../lib/bridge.js";

  let { tab = $bindable("home"), inbox = { items: [], pending: 0, today: 0, muted: [] }, sessions = [] } = $props();

  let stats = $state(null);
  let library = $state([]);
  let progress = $state([]);
  let now = $state(Date.now());

  const KEY = "agentbox.home.panels";
  const PANELS = [
    { id: "waiting", label: "Waiting for you" },
    { id: "work", label: "Work in progress" },
    { id: "rhythm", label: "Your week" },
    { id: "reviews", label: "Reviews" },
    { id: "doors", label: "Everything else" },
  ];

  let shown = $state(load());
  let customising = $state(false);

  function load() {
    try {
      const raw = JSON.parse(localStorage.getItem(KEY));
      if (Array.isArray(raw)) return new Set(raw);
    } catch {
      // A corrupt preference is not worth a message; the default view is fine.
    }
    return new Set(PANELS.map((p) => p.id));
  }
  function toggle(id) {
    const next = new Set(shown);
    next.has(id) ? next.delete(id) : next.add(id);
    shown = next;
    try {
      localStorage.setItem(KEY, JSON.stringify([...next]));
    } catch {
      // Private mode or a full quota: the view still works for this run.
    }
  }

  function refresh() {
    bridge.stats("7d").then((s) => (stats = s)).catch(() => {});
    bridge.library().then((r) => (library = r ?? [])).catch(() => {});
    bridge.progress().then((r) => (progress = r ?? [])).catch(() => {});
  }
  refresh();
  on("agentbox:progress", (r) => (progress = r ?? []));
  on("agentbox:inbox", () => refresh());

  $effect(() => {
    const t = setInterval(() => (now = Date.now()), 30_000);
    return () => clearInterval(t);
  });

  const hour = $derived(new Date(now).getHours());
  const greeting = $derived(hour < 5 ? "Still up" : hour < 12 ? "Good morning" : hour < 18 ? "Good afternoon" : "Good evening");

  const working = $derived(sessions.filter((s) => s.state === "working"));
  const waiting = $derived((inbox.items ?? []).filter((i) => i.pending));
  const recent = $derived((inbox.items ?? []).filter((i) => !i.pending).slice(0, 6));
  const openReviews = $derived(library.filter((r) => r.state === "open"));

  // The per-day bars come from the same stats query the History surface runs, so
  // the two can never disagree about what a week looked like.
  const days = $derived(stats?.byDay ?? []);
  const peak = $derived(Math.max(1, stats?.peak ?? 0));

  function ago(ms) {
    const s = Math.max(0, Math.floor((now - ms) / 1000));
    if (s < 60) return "just now";
    if (s < 3600) return `${Math.floor(s / 60)}m ago`;
    if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
    return `${Math.floor(s / 86400)}d ago`;
  }
</script>

<div class="home">
  <header>
    <div>
      <h1>{greeting}</h1>
      <p class="sub">
        {#if waiting.length}
          {waiting.length} {waiting.length === 1 ? "thing wants" : "things want"} an answer.
        {:else if working.length}
          Nothing waiting on you - {working.length} {working.length === 1 ? "agent is" : "agents are"} working.
        {:else}
          Nothing waiting, nothing running. The desk is clear.
        {/if}
      </p>
    </div>
    <button class="ghost" onclick={() => (customising = !customising)} aria-pressed={customising}>
      {customising ? "Done" : "Customise"}
    </button>
  </header>

  {#if customising}
    <!-- Deliberately plain: this is a preference, not a surface of its own, and
         it should read as something you are finished with in two seconds. -->
    <div class="customise">
      {#each PANELS as p}
        <label class="chip" class:on={shown.has(p.id)}>
          <input type="checkbox" checked={shown.has(p.id)} onchange={() => toggle(p.id)} />
          {p.label}
        </label>
      {/each}
    </div>
  {/if}

  <!-- The tiles are the glance. Each one is a button, because a number you
       cannot act on is trivia. -->
  <div class="tiles">
    <button class="tile" class:hot={waiting.length > 0} onclick={() => (tab = "inbox")}>
      <span class="n">{waiting.length}</span>
      <span class="l">waiting</span>
    </button>
    <button class="tile" onclick={() => (tab = "session")}>
      <span class="n">{working.length}{#if working.length}<i class="pulse"></i>{/if}</span>
      <span class="l">agents working</span>
    </button>
    <button class="tile" onclick={() => (tab = "history")}>
      <span class="n">{inbox.today ?? 0}</span>
      <span class="l">interruptions today</span>
    </button>
    <button class="tile" onclick={() => (tab = "library")}>
      <span class="n">{openReviews.length}</span>
      <span class="l">reviews open</span>
    </button>
  </div>

  {#if shown.has("waiting") && waiting.length}
    <section>
      <h2>Waiting for you <button class="more" onclick={() => (tab = "inbox")}>inbox →</button></h2>
      <div class="rows">
        {#each waiting.slice(0, 5) as it (it.id)}
          <button class="row" onclick={() => bridge.promote(it.id)}>
            <span class="hue" style="background: {it.hue}"></span>
            <span class="title">{it.title}</span>
            <span class="who">{it.agent}</span>
            <span class="when">{ago(it.createdMs)}</span>
          </button>
        {/each}
      </div>
    </section>
  {/if}

  {#if shown.has("work") && (working.length || progress.length)}
    <section>
      <h2>Work in progress</h2>
      <div class="rows">
        {#each working as s (s.id)}
          <button class="row" onclick={() => { bridge.selectSession(s.id); tab = "session"; }}>
            <span class="hue live"></span>
            <span class="title">{s.title || "Session"}</span>
            <span class="who">{s.project ?? ""}</span>
            <span class="when">working</span>
          </button>
        {/each}
        {#each progress as p (p.id)}
          <div class="row static">
            <span class="hue" style="background: {p.hue}"></span>
            <span class="title">{p.title}</span>
            <span class="bar"><i style="width: {Math.max(0, Math.min(100, p.percent ?? 0))}%"></i></span>
            <span class="when">{p.status || ""}</span>
          </div>
        {/each}
      </div>
    </section>
  {/if}

  {#if shown.has("rhythm") && days.length}
    <section>
      <h2>Your week <button class="more" onclick={() => (tab = "history")}>history →</button></h2>
      <div class="week">
        {#each days as d (d.day)}
          <div class="day" title="{d.label}: {d.count} interruptions">
            <span class="col" style="height: {Math.round(((d.count ?? 0) / peak) * 100)}%"></span>
            <span class="tick">{(d.label ?? "").slice(-2)}</span>
          </div>
        {/each}
      </div>
    </section>
  {/if}

  {#if shown.has("reviews") && library.length}
    <section>
      <h2>Reviews <button class="more" onclick={() => (tab = "library")}>library →</button></h2>
      <div class="rows">
        {#each library.slice(0, 4) as r (r.id)}
          <button class="row" onclick={() => bridge.libraryOpen(r.id)}>
            <span class="hue" style="background: var(--k-accent)"></span>
            <span class="title">{r.title}</span>
            <span class="who">{r.understood ?? 0}/{r.steps ?? 0} understood</span>
            <span class="when">{r.state}</span>
          </button>
        {/each}
      </div>
    </section>
  {/if}

  {#if shown.has("doors")}
    <section>
      <h2>Everything else</h2>
      <div class="doors">
        <button class="door" onclick={() => (tab = "session")}><b>New session</b><span>Run Claude inside AgentBox</span></button>
        <button class="door" onclick={() => (tab = "assignments")}><b>Assignments</b><span>Work AgentBox does on its own</span></button>
        <button class="door" onclick={() => (tab = "viewer")}><b>Viewer</b><span>Read a document</span></button>
        <button class="door" onclick={() => (tab = "history")}><b>History</b><span>What has interrupted you</span></button>
        <button class="door" onclick={() => (tab = "library")}><b>Library</b><span>Stored reviews</span></button>
        <button class="door" onclick={() => (tab = "settings")}><b>Settings</b><span>Theme, sound, quiet hours</span></button>
      </div>
    </section>
  {/if}

  {#if recent.length}
    <section class="quiet">
      <h2>Lately</h2>
      <div class="rows">
        {#each recent as it (it.id)}
          <div class="row static">
            <span class="hue" style="background: {it.hue}"></span>
            <span class="title">{it.title}</span>
            <span class="who">{it.agent}</span>
            <span class="when">{ago(it.createdMs)}</span>
          </div>
        {/each}
      </div>
    </section>
  {/if}
</div>

<style>
  .home {
    overflow: auto;
    padding: 26px 30px 40px;
    display: flex;
    flex-direction: column;
    gap: 26px;
  }
  header {
    display: flex;
    align-items: flex-start;
    gap: 16px;
  }
  h1 {
    margin: 0;
    font-family: var(--k-font-read);
    font-size: 1.7rem;
    font-weight: 500;
    letter-spacing: -0.01em;
    color: var(--k-ink);
  }
  .sub {
    margin: 5px 0 0;
    color: var(--k-ink-2);
    font-size: 0.95rem;
  }
  .ghost {
    margin-left: auto;
    border: 1px solid var(--k-edge);
    border-radius: 7px;
    padding: 5px 11px;
    font-size: 0.82rem;
    color: var(--k-ink-3);
    background: none;
    cursor: pointer;
  }
  .ghost:hover {
    color: var(--k-ink);
    border-color: var(--k-edge);
    background: var(--k-surface-2);
  }
  .customise {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    border: 1px solid var(--k-edge);
    border-radius: 999px;
    padding: 4px 12px;
    font-size: 0.82rem;
    color: var(--k-ink-3);
    cursor: pointer;
  }
  .chip.on {
    color: var(--k-ink);
    border-color: color-mix(in srgb, var(--k-accent) 45%, transparent);
    background: color-mix(in srgb, var(--k-accent) 10%, transparent);
  }
  .chip input {
    accent-color: var(--k-accent);
  }

  /* Tiles: a number big enough to read from across the desk, and a word telling
     you what it is. Buttons, because a count you cannot follow is trivia. */
  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 12px;
  }
  .tile {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
    border: 1px solid var(--k-edge-soft);
    border-radius: 12px;
    padding: 16px 18px;
    background: var(--k-surface);
    cursor: pointer;
    text-align: left;
    transition: border-color 120ms, transform 120ms;
  }
  .tile:hover {
    border-color: var(--k-edge);
    transform: translateY(-1px);
  }
  .tile .n {
    font-family: var(--k-font-mono);
    font-size: 1.8rem;
    line-height: 1.1;
    color: var(--k-ink);
    font-variant-numeric: tabular-nums;
  }
  .tile .l {
    font-size: 0.8rem;
    color: var(--k-ink-3);
  }
  /* Only the tile that means "somebody is waiting" gets colour. If they all
     did, none of them would say anything. */
  .tile.hot {
    border-color: color-mix(in srgb, var(--k-warning) 45%, transparent);
    background: color-mix(in srgb, var(--k-warning) 8%, var(--k-surface));
  }
  .tile.hot .n {
    color: var(--k-warning);
  }
  .pulse {
    display: inline-block;
    width: 7px;
    height: 7px;
    margin-left: 7px;
    vertical-align: middle;
    border-radius: 50%;
    background: var(--k-success);
    animation: pulse 1.8s ease-in-out infinite;
  }
  @keyframes pulse {
    50% {
      opacity: 0.3;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .pulse {
      animation: none;
    }
  }

  section {
    display: flex;
    flex-direction: column;
    gap: 9px;
  }
  section.quiet {
    opacity: 0.72;
  }
  h2 {
    display: flex;
    align-items: baseline;
    gap: 10px;
    margin: 0;
    font-size: 0.76rem;
    font-weight: 600;
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--k-ink-3);
  }
  .more {
    margin-left: auto;
    border: 0;
    background: none;
    padding: 0;
    font: inherit;
    letter-spacing: 0.02em;
    text-transform: none;
    color: var(--k-ink-3);
    cursor: pointer;
  }
  .more:hover {
    color: var(--k-accent);
  }

  .rows {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--k-edge-soft);
    border-radius: 10px;
    overflow: hidden;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 11px;
    width: 100%;
    padding: 9px 13px;
    border: 0;
    border-top: 1px solid var(--k-edge-soft);
    background: var(--k-surface);
    font: inherit;
    font-size: 0.9rem;
    color: var(--k-ink);
    text-align: left;
    cursor: pointer;
  }
  .row:first-child {
    border-top: 0;
  }
  .row.static {
    cursor: default;
  }
  .row:not(.static):hover {
    background: var(--k-surface-2);
  }
  .hue {
    flex: none;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--k-ink-3);
  }
  .hue.live {
    background: var(--k-success);
  }
  .title {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .who {
    flex: none;
    max-width: 16ch;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.78rem;
    color: var(--k-ink-3);
  }
  .when {
    flex: none;
    min-width: 6ch;
    text-align: right;
    font-size: 0.78rem;
    color: var(--k-ink-3);
    font-variant-numeric: tabular-nums;
  }
  .bar {
    flex: 1.2;
    height: 4px;
    border-radius: 3px;
    background: var(--k-surface-3, var(--k-edge-soft));
    overflow: hidden;
  }
  .bar i {
    display: block;
    height: 100%;
    background: var(--k-accent);
  }

  /* The week is the same query History renders, drawn small: a shape, not a
     chart. It is here to be recognised at a glance and clicked through. */
  .week {
    display: flex;
    align-items: flex-end;
    gap: 6px;
    height: 78px;
    padding: 10px 13px 6px;
    border: 1px solid var(--k-edge-soft);
    border-radius: 10px;
    background: var(--k-surface);
  }
  .day {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: flex-end;
    height: 100%;
    gap: 5px;
  }
  .col {
    width: 100%;
    max-width: 26px;
    min-height: 2px;
    border-radius: 3px 3px 0 0;
    background: color-mix(in srgb, var(--k-accent) 55%, transparent);
  }
  .day:hover .col {
    background: var(--k-accent);
  }
  .tick {
    font-family: var(--k-font-mono);
    font-size: 0.62rem;
    color: var(--k-ink-3);
  }

  .doors {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 10px;
  }
  .door {
    display: flex;
    flex-direction: column;
    gap: 3px;
    border: 1px solid var(--k-edge-soft);
    border-radius: 10px;
    padding: 13px 15px;
    background: var(--k-surface);
    text-align: left;
    cursor: pointer;
  }
  .door:hover {
    border-color: var(--k-edge);
    background: var(--k-surface-2);
  }
  .door b {
    font-weight: 500;
    font-size: 0.92rem;
    color: var(--k-ink);
  }
  .door span {
    font-size: 0.78rem;
    color: var(--k-ink-3);
  }
</style>
