<script>
  import { bridge, on } from "../lib/bridge.js";
  import { forget } from "../lib/trouble.svelte.js";
  import { endQuestion } from "../lib/endsession.js";
  import { unsavedKnobs } from "../lib/settingsdraft.svelte.js";
  import Home from "./Home.svelte";
  import Session from "./Session.svelte";
  import Agents from "./Agents.svelte";
  import Assignments from "./Assignments.svelte";
  import Inbox from "./Inbox.svelte";
  import History from "./History.svelte";
  import Settings from "./Settings.svelte";
  import Viewer from "./Viewer.svelte";
  import Library from "./Library.svelte";
  import Rail from "../lib/Rail.svelte";

  // The app shell: title bar, surface rail, session list, and whichever
  // surface is in front. agentbox as an application rather than a series of
  // transient cards.

  let tab = $state(new URLSearchParams(location.search).get("tab") || "home");
  let sessions = $state([]);
  let inbox = $state({ items: [], pending: 0, today: 0, muted: [] });
  let daemonUp = $state(true);

  const selected = $derived(sessions.find((s) => s.selected) ?? null);
  const working = $derived(sessions.filter((s) => s.state === "working").length);
  // Settings knobs changed and not yet written (U-10). The draft survives the
  // surface being swapped out, so leaving Settings no longer throws the edits
  // away - but work you can no longer see is work you forget you started, and
  // the rail is the one thing on screen whichever surface is in front. Same
  // arrangement as the pending and attached counts above it, and for the same
  // reason: a badge that only updates while you are looking at the surface it
  // describes would be worse than no badge.
  const unsaved = $derived(unsavedKnobs().length);

  // U-01's notice is one line per window, and this window has nine surfaces in
  // it. A failure that happened on the inbox is not something the sessions
  // surface should still be showing a minute later, so leaving a surface takes
  // its line with it.
  $effect(() => {
    void tab;
    forget();
  });

  // The shell owns the inbox snapshot rather than the surface, because the rail
  // badge and the status strip need the pending count whichever surface is in
  // front - and a badge that only updates while you are looking at the inbox
  // would be worse than no badge.
  let queueMoved = $state(0);

  // The session whose row is asking whether to end it. One at a time, and any
  // push clears it: a row that changed under the question should not carry a
  // confirmation aimed at what it used to say.
  let closing = $state(null);

  function close(id) {
    closing = null;
    bridge.closeSession(id);
  }

  on("agentbox:sessions", (list) => {
    sessions = list ?? [];
    if (closing && !sessions.some((s) => s.id === closing)) closing = null;
  });
  // The shell keeps the attached count for the same reason it keeps the pending
  // one: the rail's dot has to be right whichever surface is in front (FR83).
  let attached = $state(0);
  on("agentbox:agents", (r) => (attached = (r?.agents ?? []).length));
  bridge
    .agents()
    .then((r) => (attached = (r?.agents ?? []).length))
    .catch(() => {});

  on("agentbox:surface", (name) => (tab = name || "home"));
  on("agentbox:inbox", (v) => {
    if (v) inbox = v;
    queueMoved++;
  });

  bridge.ready("app");
  bridge.sessions().then((list) => (sessions = list ?? []));
  bridge.inbox().then((v) => v && (inbox = v));
</script>

<!-- The session list belongs to the session surface, so it collapses on the
     others: the inbox and history want every pixel of width for their rows, and
     a list of sessions beside a table of interruptions says nothing. -->
<div class="shell" class:solo={tab !== "session"}>
  <div class="titlebar" style="--wails-draggable: drag">
    <span class="brand"><span class="dot" class:up={daemonUp}></span><b>agentbox</b></span>
    <span class="spacer"></span>
    <button class="winbtn" title="Minimise" onclick={() => bridge.minimiseApp()}>&#x2013;</button>
    <button class="winbtn" title="Close to tray" onclick={() => bridge.hideApp()}>&#x2715;</button>
  </div>

  <Rail bind:tab pending={inbox.pending} {working} {attached} {unsaved} />

  <aside class="sessions">
    <div class="head">
      <span class="label">Sessions</span>
      <button class="new" onclick={() => bridge.newSession("", "plan")}>+ New</button>
    </div>

    <div class="list">
      {#each sessions as s (s.id)}
        <!-- A row is a div, not a button, because it holds one: the close ✕ is a
             control of its own and a button inside a button is not a thing. -->
        <div
          class="row"
          class:on={s.selected}
          class:asked={closing === s.id}
          role="button"
          tabindex="0"
          onclick={() => bridge.selectSession(s.id)}
          onkeydown={(e) => (e.key === "Enter" || e.key === " ") && bridge.selectSession(s.id)}
        >
          <span class="idot" style="background: {s.hue}"></span>
          <span class="txt">
            <span class="name">{s.title}</span>
            <span class="sub">{s.mode === "full" ? "full access" : "plan mode"} · {s.turns} turns</span>
          </span>

          {#if closing === s.id}
            <!-- Closing kills the agent and an unsaved conversation goes with it,
                 so the row asks rather than acting on the first click. The question
                 takes its own line: it has to be readable, and a 224px rail cannot
                 hold a sentence and two buttons on one. -->
            <span class="ask">
              <span class="q">{endQuestion(s)}</span>
              <span class="btns">
                <button class="end" onclick={(e) => (e.stopPropagation(), close(s.id))}>End session</button>
                <button class="keep" onclick={(e) => (e.stopPropagation(), (closing = null))}>Keep</button>
              </span>
            </span>
          {:else if s.ask}
            <!-- FR49: this conversation has a question in it. The panel is only
                 visible in the selected session, so the row has to say so. -->
            <span class="asking" title="waiting on your answer">?</span>
          {:else}
            <span class="state {s.state}">{s.state === "working" ? "working" : s.state === "error" ? "error" : s.state === "ended" ? "ended" : "idle"}</span>
          {/if}

          {#if closing !== s.id}
            <button
              class="x"
              title="End this session"
              aria-label="End session {s.title}"
              onclick={(e) => (e.stopPropagation(), (closing = s.id))}
            >&#x2715;</button>
          {/if}
        </div>
      {:else}
        <p class="empty">No sessions yet. <br />“+ New” starts Claude in this directory.</p>
      {/each}
    </div>

    <div class="grow"></div>
  </aside>

  <main>
    {#if tab === "home"}
      <Home bind:tab {inbox} {sessions} />
    {:else if tab === "session"}
      <Session session={selected} />
    {:else if tab === "agents"}
      <Agents />
    {:else if tab === "assignments"}
      <Assignments />
    {:else if tab === "inbox"}
      <Inbox {inbox} />
    {:else if tab === "history"}
      <History refresh={queueMoved} />
    {:else if tab === "viewer"}
      <!-- The same component `agentbox show` opens in its own window, minus the
           chrome: the app already has a title bar and a status strip, and the
           document should not bring a second set. -->
      <Viewer chrome={false} />
    {:else if tab === "library"}
      <Library />
    {:else if tab === "settings"}
      <Settings />
    {:else}
      <div class="stub">
        <h2>{tab}</h2>
        <p>This surface is next in the port.</p>
      </div>
    {/if}
  </main>

  <div class="status">
    <span class="up">● daemon up</span>
    <span>{sessions.length} session{sessions.length === 1 ? "" : "s"}</span>
    {#if inbox.pending}<span class="waiting">{inbox.pending} waiting</span>{/if}
    {#if selected}<span class="path">{selected.cwd}</span>{/if}
    <span class="spacer"></span>
    <span>webkitgtk-6.0</span>
  </div>
</div>

<style>
  .shell {
    height: 100%;
    display: grid;
    grid-template-columns: 54px 224px 1fr;
    grid-template-rows: 38px 1fr 30px;
    background: var(--k-ground);
    color: var(--k-ink);
  }
  /* Collapse the column rather than dropping the element, so main and the status
   * strip keep the grid columns they were placed in. */
  .shell.solo {
    grid-template-columns: 54px 0 1fr;
  }
  .shell.solo .sessions {
    display: none;
  }
  .shell.solo main {
    border-left: 0;
  }

  .titlebar {
    grid-column: 1 / -1;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 0 8px 0 14px;
    border-bottom: 1px solid var(--k-edge-soft);
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 0.78rem;
    color: var(--k-ink-2);
  }
  .brand b {
    color: var(--k-ink);
    font-weight: 700;
  }
  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--k-ink-3);
  }
  .dot.up {
    background: var(--k-success);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--k-success) 18%, transparent);
  }
  .spacer {
    flex: 1;
  }
  .winbtn {
    width: 26px;
    height: 24px;
    border-radius: 6px;
    color: var(--k-ink-3);
    font-size: 0.72rem;
  }
  .winbtn:hover {
    background: var(--k-surface-2);
    color: var(--k-ink);
  }

  .sessions {
    grid-row: 2 / 4;
    border-right: 1px solid var(--k-edge-soft);
    display: flex;
    flex-direction: column;
    min-height: 0;
  }
  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 12px 8px 14px;
  }
  /* The shared label voice (Home's h2), a step down for the narrow column. */
  .label {
    font-size: 0.72rem;
    font-weight: 600;
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--k-ink-3);
  }
  .new {
    font-size: 0.74rem;
    color: var(--k-ink-2);
    border: 1px solid var(--k-edge);
    border-radius: 6px;
    padding: 2px 8px;
    background: var(--k-surface-2);
  }
  .new:hover {
    background: var(--k-surface-3);
    color: var(--k-ink);
  }

  .list {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 0 8px;
    overflow-y: auto;
  }
  .row {
    position: relative;
    display: flex;
    align-items: flex-start;
    gap: 9px;
    text-align: left;
    padding: 8px 9px;
    border-radius: 8px;
    border: 1px solid transparent;
  }
  .row:hover {
    background: var(--k-surface-2);
  }
  .row.on {
    background: var(--k-surface-2);
    border-color: var(--k-edge);
  }
  .idot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    margin-top: 6px;
    flex: 0 0 auto;
  }
  .txt {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .name {
    font-size: 0.82rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .sub {
    font-size: 0.72rem;
    color: var(--k-ink-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .state {
    flex: 0 0 auto;
    margin-top: 2px;
    font-family: var(--k-font-mono);
    font-size: 0.62rem;
    color: var(--k-ink-3);
  }
  .state.working {
    color: var(--k-success);
  }
  .state.error {
    color: var(--k-error);
  }

  /* The ✕ stays out of the way until the pointer is on the row (or the row has
   * the keyboard), so the list reads as a list of sessions and not a list of
   * things to close. */
  /* Absolute, so an invisible control costs the title no width: it sits over the
   * state chip while the pointer is on the row, which is when the chip is the
   * least interesting thing there. */
  .x {
    position: absolute;
    top: 7px;
    right: 7px;
    width: 18px;
    height: 18px;
    line-height: 1;
    font-size: 0.66rem;
    border-radius: 5px;
    color: var(--k-ink-3);
    background: var(--k-surface-2);
    opacity: 0;
    transition: opacity 0.09s ease;
  }
  .row:hover .x,
  .row:focus-visible .x,
  .x:focus-visible {
    opacity: 1;
  }
  /* The chip and the ✕ want the same corner. While the pointer is on the row the
   * ✕ is what you came for, so the chip steps aside rather than being crowded -
   * and it is back the moment you leave, with no layout shift either way. */
  .row:hover .state,
  .row:hover .asking,
  .row:focus-visible .state,
  .row:focus-visible .asking {
    opacity: 0;
  }
  .state,
  .asking {
    transition: opacity 0.09s ease;
  }
  .x:hover {
    background: var(--k-surface-3);
    color: var(--k-ink);
  }
  .row.asked {
    border-color: var(--k-warning);
    flex-wrap: wrap;
  }
  .ask {
    flex: 1 0 100%;
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-top: 7px;
    padding-top: 7px;
    border-top: 1px solid var(--k-edge-soft);
  }
  .q {
    font-size: 0.72rem;
    line-height: 1.35;
    color: var(--k-warning);
  }
  .btns {
    display: flex;
    gap: 6px;
  }
  .btns button {
    font-size: 0.72rem;
    padding: 2px 9px;
    border-radius: 5px;
    border: 1px solid var(--k-edge);
    background: var(--k-surface-2);
    color: var(--k-ink-2);
  }
  .btns .end {
    border-color: var(--k-warning);
    color: var(--k-warning);
  }
  .btns button:hover {
    background: var(--k-surface-3);
  }
  .asking {
    flex: 0 0 auto;
    width: 16px;
    height: 16px;
    margin-top: 1px;
    display: grid;
    place-items: center;
    border-radius: 50%;
    background: color-mix(in srgb, var(--k-accent) 24%, transparent);
    border: 1px solid color-mix(in srgb, var(--k-accent) 55%, transparent);
    font-family: var(--k-font-mono);
    font-size: 0.62rem;
    color: var(--k-ink);
    animation: breathe 1.8s ease-in-out infinite;
  }
  @keyframes breathe {
    50% {
      opacity: 0.45;
    }
  }
  .empty {
    margin: 10px;
    font-family: var(--k-font-read);
    font-size: 0.82rem;
    line-height: 1.5;
    color: var(--k-ink-3);
  }
  .grow {
    flex: 1;
  }

  main {
    /* Placed explicitly: with the session list collapsed, auto-placement would
     * drop main into the zero-width column. */
    grid-column: 3 / 4;
    grid-row: 2 / 3;
    min-height: 0;
    /* And min-width, for the same reason in the other axis: a `1fr` track's
     * automatic minimum is its content's min-content width, so one unbreakable
     * token anywhere in a surface widened the whole window and pushed the right
     * edge of every row off screen. Found by opening a shared value whose JSON
     * had no spaces in it; nothing inside a surface can fix this from below. */
    min-width: 0;
    background: var(--k-surface);
    border-left: 1px solid var(--k-edge-soft);
    display: flex;
    flex-direction: column;
  }
  .stub {
    margin: auto;
    text-align: center;
    color: var(--k-ink-3);
  }
  .stub h2 {
    margin: 0 0 4px;
    font-size: 1rem;
    text-transform: capitalize;
    color: var(--k-ink-2);
  }
  .stub p {
    margin: 0;
    font-family: var(--k-font-read);
  }

  .status {
    grid-column: 3 / 4;
    grid-row: 3 / 4;
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 0 16px;
    border-top: 1px solid var(--k-edge-soft);
    background: var(--k-ground);
    font-family: var(--k-font-mono);
    font-size: 0.62rem;
    color: var(--k-ink-3);
  }
  .status .up {
    color: var(--k-success);
  }
  .status .waiting {
    color: var(--k-info);
  }
  .status .path {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 40ch;
  }
</style>
