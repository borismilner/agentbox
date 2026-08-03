<script>
  // The library (FR70): every stored review, with a way in and a way out.
  //
  // Reviews were durable from the day the store landed - spec, marks, notes,
  // comments and the step you were on, all write-through to SQLite - but the
  // only door was the CLI. Something you cannot see is something you assume
  // was not saved, which is exactly what happened the first time a board was
  // closed and reopened the next day.
  import { bridge } from "../lib/bridge.js";

  let rows = $state(null); // null until the first load, so empty is distinguishable
  let err = $state("");
  let query = $state("");
  let asking = $state(null); // id awaiting delete confirmation

  const shown = $derived(
    (rows ?? []).filter((r) => {
      const q = query.trim().toLowerCase();
      return !q || (r.title + " " + r.repo + " " + r.id).toLowerCase().includes(q);
    }),
  );

  async function load() {
    try {
      rows = await bridge.library();
      err = "";
    } catch (e) {
      err = String(e);
    }
  }
  load();

  async function open(id) {
    asking = null;
    try {
      await bridge.libraryOpen(id);
      await load(); // the board moved, so onBoard moved with it
    } catch (e) {
      err = String(e);
    }
  }

  async function remove(id) {
    asking = null;
    try {
      await bridge.libraryDelete(id);
      await load();
    } catch (e) {
      err = String(e);
    }
  }

  // Relative, because "yesterday" is what you remember about a review, not a
  // date. Anything older than a week gets the date instead - by then the
  // number of days stops meaning anything.
  function when(ms) {
    const s = Math.max(0, (Date.now() - ms) / 1000);
    if (s < 90) return "just now";
    if (s < 3600) return Math.round(s / 60) + "m ago";
    if (s < 86400) return Math.round(s / 3600) + "h ago";
    if (s < 7 * 86400) return Math.round(s / 86400) + "d ago";
    return new Date(ms).toLocaleDateString();
  }
</script>

<section class="library">
  <header>
    <div class="titles">
      <h1>Reviews</h1>
      <p class="sub">
        Saved as you walk them - verdicts, notes and comments, and the step you stopped on.
        Nothing here needs saving by hand.
      </p>
    </div>
    <input class="find" placeholder="filter" bind:value={query} />
    <button class="refresh" onclick={load} title="reload the list">↻</button>
  </header>

  {#if err}
    <div class="err">{err}</div>
  {/if}

  {#if rows === null}
    <div class="empty">loading…</div>
  {:else if rows.length === 0}
    <div class="empty">
      No reviews stored yet. An agent creates one with <code>create_walkthrough</code>,
      or <code>agentbox walkthrough create --spec review.json</code>.
    </div>
  {:else if shown.length === 0}
    <div class="empty">Nothing matches “{query}”.</div>
  {:else}
    <div class="rows">
      {#each shown as r (r.id)}
        <div class="row" class:on={r.onBoard} class:asked={asking === r.id}>
          <button class="main" onclick={() => open(r.id)} title="put this on the board">
            <div class="line1">
              <span class="title">{r.title}</span>
              {#if r.onBoard}<span class="chip live">on the board</span>{/if}
              {#if r.state !== "open"}<span class="chip">{r.state}</span>{/if}
            </div>
            <div class="line2">
              <span class="mono repo">{r.repo}</span>
              <span class="mono sha">{r.pinned.slice(0, 12)}</span>
              <span class="when">{when(r.updatedMs)}</span>
            </div>
          </button>

          <div class="progress" title="{r.understood} of {r.steps} understood">
            <div class="bar">
              <span style="width: {r.steps ? (100 * r.understood) / r.steps : 0}%"></span>
            </div>
            <span class="count">
              {r.understood} of {r.steps}
              {#if r.unclear > 0}<span class="warn"> · {r.unclear} unclear</span>{/if}
              {#if r.comments > 0}<span class="dim"> · {r.comments} comments</span>{/if}
            </span>
          </div>

          {#if asking === r.id}
            <div class="confirm">
              <span>delete for good?</span>
              <button class="danger" onclick={() => remove(r.id)}>delete</button>
              <button class="quiet" onclick={() => (asking = null)}>keep</button>
            </div>
          {:else}
            <button class="x" onclick={() => (asking = r.id)} title="delete this review">✕</button>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</section>

<style>
  .library {
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: 100%;
    padding: 18px 22px;
    overflow-y: auto;
  }
  header {
    display: flex;
    align-items: flex-start;
    gap: 14px;
    margin-bottom: 14px;
  }
  .titles {
    min-width: 0;
    flex: 1;
  }
  h1 {
    font-size: 1.15em;
    margin: 0 0 2px;
  }
  .sub {
    margin: 0;
    color: var(--k-ink-2);
    font-size: 0.85em;
    max-width: 60ch;
  }
  .find {
    background: var(--k-surface-2);
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    color: var(--k-ink);
    padding: 5px 10px;
    font-size: 0.9em;
    width: 14em;
  }
  .refresh {
    background: none;
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    color: var(--k-ink-2);
    cursor: pointer;
    padding: 4px 9px;
  }
  .refresh:hover {
    color: var(--k-ink);
    border-color: var(--k-accent);
  }
  .err {
    color: var(--k-error);
    font-size: 0.9em;
    margin-bottom: 10px;
  }
  .empty {
    color: var(--k-ink-2);
    font-size: 0.95em;
    padding: 24px 2px;
  }
  .empty code {
    font-family: var(--k-font-mono);
    font-size: 0.9em;
    background: var(--k-surface-2);
    border-radius: 4px;
    padding: 1px 5px;
  }
  .rows {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  /* A row is a div holding a button, not a button: the delete control is a
     control of its own and a button inside a button is not a thing. */
  .row {
    display: flex;
    align-items: center;
    gap: 12px;
    border: 1px solid var(--k-edge);
    border-radius: 10px;
    padding: 8px 10px 8px 12px;
  }
  .row:hover {
    border-color: var(--k-edge-soft, var(--k-accent));
  }
  .row.on {
    border-color: var(--k-info);
  }
  .row.asked {
    border-color: var(--k-warning);
  }
  .main {
    all: unset;
    cursor: pointer;
    min-width: 0;
    flex: 1;
  }
  .line1 {
    display: flex;
    align-items: baseline;
    gap: 8px;
  }
  .title {
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .line2 {
    display: flex;
    gap: 10px;
    margin-top: 2px;
    font-size: 0.8em;
    color: var(--k-ink-3);
  }
  .mono {
    font-family: var(--k-font-mono);
  }
  .repo {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 28ch;
  }
  .chip {
    border: 1px solid var(--k-edge);
    border-radius: 999px;
    padding: 1px 8px;
    font-size: 0.72em;
    color: var(--k-ink-2);
    white-space: nowrap;
  }
  .chip.live {
    border-color: var(--k-info);
    color: var(--k-info);
  }
  .progress {
    flex: none;
    width: 15em;
    text-align: right;
  }
  .bar {
    height: 4px;
    border-radius: 2px;
    background: var(--k-edge);
    overflow: hidden;
    margin-bottom: 3px;
  }
  .bar span {
    display: block;
    height: 100%;
    background: var(--k-success);
  }
  .count {
    font-size: 0.78em;
    color: var(--k-ink-2);
    white-space: nowrap;
  }
  .warn {
    color: var(--k-warning);
  }
  .dim {
    color: var(--k-ink-3);
  }
  .x {
    all: unset;
    cursor: pointer;
    color: var(--k-ink-3);
    padding: 2px 6px;
    border-radius: 6px;
  }
  .x:hover,
  .x:focus-visible {
    color: var(--k-error);
  }
  .confirm {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 0.82em;
    color: var(--k-warning);
    white-space: nowrap;
  }
  .danger,
  .quiet {
    border: 1px solid var(--k-edge);
    background: none;
    border-radius: 6px;
    cursor: pointer;
    font-size: 0.95em;
    padding: 2px 8px;
  }
  .danger {
    color: var(--k-error);
    border-color: var(--k-error);
  }
  .quiet {
    color: var(--k-ink-2);
  }
</style>
