<script>
  import { bridge, on } from "../lib/bridge.js";
  import Session from "./Session.svelte";

  // The drop-down panel (M10): the session surface, reached with a hotkey instead
  // of a window. It shows the same conversations the app window shows - one
  // session list, two ways in - so the panel is a shortcut, not a second place
  // your work can live.
  //
  // Chrome is deliberately thin: a row of session chips, the project path, and
  // Esc to send it back up. Everything else about reading and replying belongs to
  // Session.svelte, which the app window uses too.

  let sessions = $state([]);
  let past = $state([]); // saved conversations, when the Load list is open
  let loading = $state(false);
  let arm = $state(""); // the session whose ✕ is armed

  const selected = $derived(sessions.find((s) => s.selected) ?? null);

  function toggleLoad() {
    loading = !loading;
    if (loading) bridge.savedSessions().then((list) => (past = list ?? []));
  }

  function reopen(p) {
    loading = false;
    bridge.reopenSession(p.path);
  }

  // A session's name: Claude picks one from its own first words, and a
  // double-click on the chip lets you overrule it with a label you will recognise
  // tomorrow. Empty hands it back to Claude's.
  let renaming = $state("");
  let draftName = $state("");

  function startRename(s) {
    renaming = s.id;
    draftName = s.title ?? "";
  }
  function commitRename(s) {
    if (renaming !== s.id) return;
    renaming = "";
    bridge.renameSession(s.id, draftName.trim());
  }
  function renameKey(e, s) {
    if (e.key === "Enter") {
      e.preventDefault();
      commitRename(s);
      return;
    }
    if (e.key === "Escape") {
      e.preventDefault();
      renaming = "";
    }
  }
  function focusNow(el) {
    el.focus();
    el.select();
  }

  // Closing kills a child. One click when it is idle; two when it is mid-turn.
  function close(s) {
    if (s.state === "working" && arm !== s.id) {
      arm = s.id;
      setTimeout(() => (arm = arm === s.id ? "" : arm), 3000);
      return;
    }
    arm = "";
    bridge.closeSession(s.id);
  }

  on("agentbox:sessions", (list) => (sessions = list ?? []));

  bridge.ready("panel");
  bridge.sessions().then((list) => (sessions = list ?? []));

  // The content fills the window, full stop. It used to be a fixed-height box
  // anchored to the viewport's bottom, measured from Go, so the height animation
  // could grow the window without reflowing anything. That number goes stale the
  // moment the panel moves to a monitor of a different size: the window was 960
  // tall on the portrait screen while the box was still the 540 it had been on the
  // wide one, leaving 420px of empty background above the header - which reads
  // exactly like a window somebody dragged out of place. Filling the window cannot
  // be stale. The animation is off by default now, and when it is on it pays for
  // itself with a reflow per frame.

  // Esc rolls the panel up. It only acts when nothing is being typed into, the
  // same guard the inline ask panel uses: Esc in the composer is for handing the
  // keyboard over, not for making the window disappear mid-sentence.
  function onKey(e) {
    if (e.key !== "Escape") return;
    const t = e.target;
    const typing = t && (t.tagName === "TEXTAREA" || t.tagName === "INPUT" || t.isContentEditable);
    if (typing) return;
    e.preventDefault();
    bridge.hidePanel();
  }
</script>

<svelte:window on:keydown={onKey} />

<div class="panel">
  <!-- Not draggable. It is pinned to the top edge of one monitor and re-pinned on
       every roll; a drag handle only lets you move it somewhere it will not stay,
       and dragging it down leaves a window whose content is anchored to a viewport
       bottom that is no longer where the panel is. Quake's console does not move
       either. -->
  <header>
    <span class="mark">agentbox</span>

    <div class="chips">
      {#each sessions as s (s.id)}
        <span class="chipwrap" class:on={s.selected}>
          <button
            class="chip"
            onclick={() => bridge.selectSession(s.id)}
            ondblclick={() => startRename(s)}
            title="{s.cwd}&#10;double-click to rename">
            <span class="idot" style="background: {s.hue}"></span>
            {#if renaming === s.id}
              <!-- Renaming is how you find this conversation tomorrow. Enter keeps
                   it, Escape abandons it, and an empty name gives it back to the
                   one Claude chose. -->
              <input
                class="rn"
                bind:value={draftName}
                onkeydown={(e) => renameKey(e, s)}
                onblur={() => commitRename(s)}
                onclick={(e) => e.stopPropagation()}
                use:focusNow />
            {:else}
              <span class="nm">{s.title}</span>
            {/if}
            {#if s.ask}
              <span class="mk asking" title="waiting on your answer">?</span>
            {:else if s.state === "working"}
              <span class="mk working" title="Claude is working">●</span>
            {/if}
          </button>
          <!-- Two clicks to close a session that is mid-turn, one when it is idle:
               closing kills the child, and a mis-click on a working agent is the
               expensive mistake. The conversation is saved either way. -->
          <button
            class="x"
            class:arm={arm === s.id}
            title={arm === s.id ? "Click again to close" : "Close this session"}
            onclick={() => close(s)}>{arm === s.id ? "sure?" : "✕"}</button>
        </span>
      {/each}
      <button class="chip new" onclick={() => bridge.newSession("", "")}>+ New</button>
      <button class="chip new" onclick={toggleLoad} title="Reopen a saved conversation">
        {loading ? "▾" : "▸"} Load
      </button>
    </div>

    <span class="spacer"></span>
    {#if selected}<span class="path">{selected.cwd}</span>{/if}
    <span class="fonts">
      <button class="fz" title="Smaller text" onclick={() => bridge.bumpFontSize(-1)}>A-</button>
      <button class="fz" title="Larger text" onclick={() => bridge.bumpFontSize(1)}>A+</button>
    </span>
    <button class="up" title="Roll up (Esc)" onclick={() => bridge.hidePanel()}>&#x2303;</button>
  </header>

  {#if loading}
    <div class="saved">
      {#if past.length === 0}
        <p class="none">Nothing saved yet. A conversation is written when you close it, and when agentbox stops.</p>
      {:else}
        {#each past as p (p.path)}
          <button class="row" onclick={() => reopen(p)}>
            <span class="when">{p.when}</span>
            <span class="prev">{p.preview || p.title || "(no prompt)"}</span>
            <span class="meta">{p.turns} turns{p.resume ? "" : " · read only"}</span>
          </button>
        {/each}
      {/if}
    </div>
  {/if}

  <main>
    {#if selected}
      <Session session={selected} />
    {:else}
      <div class="empty">
        <h2>No session yet</h2>
        <p>“+ New” starts Claude here. The panel shows the same sessions as the app window.</p>
      </div>
    {/if}
  </main>
</div>

<style>
  /* Fills the window, whatever size the window is. See the note in the script:
   * a measured, bottom-anchored box goes stale the moment the panel appears on a
   * monitor of a different size, and stale reads as "somebody dragged this". */
  .panel {
    position: fixed;
    inset: 0;
    display: grid;
    grid-template-rows: 40px 1fr;
    background: var(--k-ground);
    color: var(--k-ink);
    /* The panel hangs off the top edge of the screen, so only the bottom corners
     * are ever visible - and a hairline there sells the edge as deliberate. */
    border-bottom: 1px solid var(--k-edge);
  }

  header {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0 8px 0 14px;
    border-bottom: 1px solid var(--k-edge-soft);
    min-width: 0;
  }
  .mark {
    font-weight: 700;
    font-size: 0.78rem;
    letter-spacing: 0.02em;
  }
  .spacer {
    flex: 1;
  }
  .path {
    font-family: var(--k-font-mono);
    font-size: 0.68rem;
    color: var(--k-ink-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 34ch;
  }

  .chips {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    overflow-x: auto;
    scrollbar-width: none;
  }
  /* A chip is the session button plus its close button, sharing one pill so the ✕
   * reads as part of the session rather than as another session. */
  .chipwrap {
    display: flex;
    align-items: center;
    flex: 0 0 auto;
    border-radius: 999px;
    border: 1px solid var(--k-edge-soft);
    overflow: hidden;
  }
  .chipwrap.on {
    background: var(--k-surface-2);
    border-color: var(--k-edge);
  }
  .chipwrap.on .chip {
    color: var(--k-ink);
  }
  .chip {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: 0 0 auto;
    max-width: 22ch;
    padding: 3px 9px;
    border-radius: 999px;
    border: 1px solid var(--k-edge-soft);
    background: transparent;
    color: var(--k-ink-2);
    font-size: 0.74rem;
  }
  .chipwrap .chip {
    border: 0;
    border-radius: 0;
  }
  .chip:hover {
    background: var(--k-surface-2);
  }
  .chip.on {
    background: var(--k-surface-2);
    border-color: var(--k-edge);
    color: var(--k-ink);
  }
  .chip.new {
    color: var(--k-ink-3);
  }
  .x {
    flex: 0 0 auto;
    padding: 3px 7px 3px 3px;
    background: transparent;
    color: var(--k-ink-3);
    font-size: 0.66rem;
    line-height: 1;
  }
  .x:hover {
    color: var(--k-error);
  }
  .x.arm {
    color: var(--k-error);
    font-family: var(--k-font-mono);
  }

  .fonts {
    display: flex;
    gap: 2px;
    flex: 0 0 auto;
  }
  .fz {
    padding: 2px 6px;
    border-radius: 6px;
    background: transparent;
    color: var(--k-ink-3);
    font-size: 0.68rem;
    font-family: var(--k-font-mono);
  }
  .fz:hover {
    background: var(--k-surface-2);
    color: var(--k-ink);
  }

  /* The Load list hangs UNDER the header rather than sitting in the layout. The
   * panel is a two-row grid (header, conversation), and a third child took the
   * 1fr row for itself and pushed the conversation into an auto row off the bottom
   * edge - the list was in the DOM and nothing was visible. Absolute against the
   * fixed .panel keeps the grid a two-row grid and makes it behave like the
   * dropdown it looks like. */
  .saved {
    position: absolute;
    top: 40px;
    left: 0;
    right: 0;
    z-index: 5;
    max-height: 55%;
    overflow-y: auto;
    border-bottom: 1px solid var(--k-edge);
    background: var(--k-surface);
    box-shadow: 0 12px 28px rgb(0 0 0 / 40%);
  }
  .saved .none {
    margin: 0;
    padding: 10px 14px;
    color: var(--k-ink-3);
    font-size: 0.78rem;
  }
  .saved .row {
    display: flex;
    align-items: baseline;
    gap: 10px;
    width: 100%;
    padding: 6px 14px;
    background: transparent;
    text-align: left;
    font-size: 0.78rem;
    color: var(--k-ink-2);
  }
  .saved .row:hover {
    background: var(--k-surface-2);
  }
  .saved .when {
    flex: 0 0 auto;
    color: var(--k-ink-3);
    font-family: var(--k-font-mono);
    font-size: 0.72rem;
  }
  .saved .prev {
    flex: 1 1 auto;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--k-ink);
  }
  .saved .meta {
    flex: 0 0 auto;
    color: var(--k-ink-3);
    font-size: 0.72rem;
  }
  .rn {
    width: 16ch;
    padding: 0;
    border: 0;
    border-bottom: 1px solid var(--k-accent);
    background: transparent;
    color: var(--k-ink);
    font-size: inherit;
    font-family: inherit;
  }
  .nm {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .idot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex: 0 0 auto;
  }
  .mk {
    font-size: 0.66rem;
    flex: 0 0 auto;
  }
  .mk.working {
    color: var(--k-success);
  }
  .mk.asking {
    color: var(--k-warning);
    font-family: var(--k-font-mono);
  }

  .up {
    width: 26px;
    height: 24px;
    border-radius: 6px;
    color: var(--k-ink-3);
    font-size: 0.9rem;
  }
  .up:hover {
    background: var(--k-surface-2);
    color: var(--k-ink);
  }

  /* Session.svelte's conversation is `flex: 1; overflow-y: auto`, so its host has
   * to be a flex column with a bounded height - without that the stream simply
   * grows, nothing scrolls, and the composer ends up below the bottom edge.
   * --k-measure widens the reading column: the panel is far wider than the app
   * window's content area and a 700px column left it mostly empty. */
  main {
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
    --k-measure: var(--k-panel-measure, 980px);
  }
  .empty {
    height: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 6px;
    color: var(--k-ink-3);
  }
  .empty h2 {
    font-size: 0.95rem;
    color: var(--k-ink-2);
  }
  .empty p {
    font-size: 0.8rem;
  }
</style>
