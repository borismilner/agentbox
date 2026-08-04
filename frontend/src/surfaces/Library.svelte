<script>
  // The library (FR70): every stored review, with a way in and a way out.
  //
  // Reviews were durable from the day the store landed - spec, marks, notes,
  // comments and the step you were on, all write-through to SQLite - but the
  // only door was the CLI. Something you cannot see is something you assume
  // was not saved, which is exactly what happened the first time a board was
  // closed and reopened the next day.
  import { bridge } from "../lib/bridge.js";
  import { ticker } from "../lib/clock.svelte.js";

  let rows = $state(null); // null until the first load, so empty is distinguishable
  let err = $state("");
  let query = $state("");
  let typing = $state(false);
  let box = $state(null);
  let asking = $state(null); // id awaiting delete confirmation

  const clock = ticker();

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
  // number of days stops meaning anything. Ages ride the shared 1Hz tick so
  // they stay honest while the window sits open.
  function when(ms, now) {
    const s = Math.max(0, (now - ms) / 1000);
    if (s < 90) return "just now";
    if (s < 3600) return Math.round(s / 60) + "m ago";
    if (s < 86400) return Math.round(s / 3600) + "h ago";
    if (s < 7 * 86400) return Math.round(s / 86400) + "d ago";
    return new Date(ms).toLocaleDateString();
  }
</script>

<section class="library">
  <header>
    <div class="line">
      <h1>Library</h1>
      {#if rows?.length}
        <span class="count">{rows.length} review{rows.length === 1 ? "" : "s"}</span>
      {/if}
    </div>
    <div class="tools">
      <div class="search" class:on={typing}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
          <circle cx="11" cy="11" r="6.5" /><path d="m16 16 4.5 4.5" />
        </svg>
        <input
          bind:this={box}
          bind:value={query}
          onfocus={() => (typing = true)}
          onblur={() => (typing = false)}
          placeholder="Search title, repo, id…"
          spellcheck="false"
        />
        {#if query}
          <button class="clear" onclick={() => (query = "")} title="Clear">&#x2715;</button>
        {/if}
      </div>
      <button class="ghost" onclick={load} title="Reload the list">Refresh</button>
    </div>
  </header>

  <div class="scroll">
    {#if err}
      <div class="err">{err}</div>
    {/if}

    {#if rows === null}
      <p class="blank">Reading the library…</p>
    {:else if rows.length === 0}
      <div class="blank">
        <p class="lead">No reviews stored yet.</p>
        <p>
          An agent creates one with <code>create_walkthrough</code>, or
          <code>agentbox walkthrough create --spec review.json</code>. Reviews save themselves as you
          walk them - verdicts, notes, comments, and the step you stopped on.
        </p>
      </div>
    {:else if shown.length === 0}
      <p class="blank">Nothing matches “{query}”.</p>
    {:else}
      <div class="rows">
        {#each shown as r (r.id)}
          <div class="row" class:asked={asking === r.id}>
            <button class="main" onclick={() => open(r.id)} title="Put this review on the board">
              <span class="hue"></span>
              <span class="body">
                <span class="line1">
                  <span class="title">{r.title}</span>
                  {#if r.onBoard}<span class="chip live">on the board</span>{/if}
                  {#if r.state !== "open"}<span class="chip">{r.state}</span>{/if}
                </span>
                <span class="line2">
                  <span class="repo">{r.repo}</span>
                  <span class="sha">{r.pinned.slice(0, 12)}</span>
                </span>
              </span>
            </button>

            {#if asking === r.id}
              <div class="confirm">
                <span>delete for good?</span>
                <button class="danger" onclick={() => remove(r.id)}>Delete</button>
                <button class="keep" onclick={() => (asking = null)}>Keep</button>
              </div>
            {:else}
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
              <span class="when">{when(r.updatedMs, clock.now)}</span>
              <button class="x" onclick={() => (asking = r.id)} title="Delete this review">&#x2715;</button>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  </div>
</section>

<style>
  .library {
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: 100%;
  }

  header {
    padding: 16px 18px 12px;
    border-bottom: 1px solid var(--k-edge-soft);
  }
  .line {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 12px;
  }
  h1 {
    margin: 0;
    font-size: 1.12rem;
    font-weight: 700;
    letter-spacing: -0.01em;
  }
  .line .count {
    font-family: var(--k-font-mono);
    font-size: 0.7rem;
    color: var(--k-ink-3);
  }
  .tools {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .search {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 10px;
    height: 34px;
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    background: var(--k-surface-2);
    color: var(--k-ink-3);
  }
  .search.on {
    border-color: color-mix(in srgb, var(--k-accent) 55%, var(--k-edge));
  }
  .search svg {
    width: 15px;
    height: 15px;
    flex: 0 0 auto;
  }
  .search input {
    flex: 1;
    min-width: 0;
    border: 0;
    outline: none;
    background: none;
    color: var(--k-ink);
    font: inherit;
    font-size: 0.84rem;
  }
  .search input::placeholder {
    color: var(--k-ink-3);
  }
  .clear {
    font-size: 0.7rem;
    color: var(--k-ink-3);
  }
  .clear:hover {
    color: var(--k-ink);
  }
  .ghost {
    border: 1px solid var(--k-edge);
    border-radius: 7px;
    padding: 5px 11px;
    font-size: 0.82rem;
    color: var(--k-ink-3);
  }
  .ghost:hover {
    color: var(--k-ink);
    background: var(--k-surface-2);
  }

  .scroll {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 16px 18px 24px;
  }
  .err {
    margin-bottom: 10px;
    font-size: 0.84rem;
    color: var(--k-error);
  }

  .blank {
    max-width: 480px;
    margin: 44px auto;
    text-align: center;
    font-family: var(--k-font-read);
    font-size: 0.9rem;
    color: var(--k-ink-3);
  }
  .blank .lead {
    margin: 0 0 6px;
    font-size: 1.05rem;
    color: var(--k-ink-2);
  }
  .blank p {
    margin: 0;
    line-height: 1.6;
  }
  .blank code {
    font-family: var(--k-font-mono);
    font-size: 0.82em;
    color: var(--k-ink-2);
  }

  /* The joined-rows container every list on Home uses: one frame, quiet
   * separators, the row lights on hover. */
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
    gap: 12px;
    padding: 9px 13px;
    border-top: 1px solid var(--k-edge-soft);
    background: var(--k-surface);
  }
  .row:first-child {
    border-top: 0;
  }
  .row:hover {
    background: var(--k-surface-2);
  }
  .row.asked {
    background: color-mix(in srgb, var(--k-warning) 7%, var(--k-surface));
  }

  .main {
    all: unset;
    cursor: pointer;
    min-width: 0;
    flex: 1;
    display: flex;
    align-items: center;
    gap: 11px;
  }
  /* Reviews carry no agent identity, so the dot wears the accent - the same
   * choice Home's review rows make. */
  .hue {
    flex: none;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--k-accent);
  }
  .body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }
  .line1 {
    display: flex;
    align-items: baseline;
    gap: 8px;
    min-width: 0;
  }
  .title {
    font-size: 0.9rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .line2 {
    display: flex;
    gap: 10px;
    font-family: var(--k-font-mono);
    font-size: 0.72rem;
    color: var(--k-ink-3);
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
    font-size: 0.68rem;
    color: var(--k-ink-2);
    white-space: nowrap;
  }
  .chip.live {
    border-color: color-mix(in srgb, var(--k-info) 45%, transparent);
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
    background: var(--k-surface-3);
    overflow: hidden;
    margin-bottom: 3px;
  }
  .bar span {
    display: block;
    height: 100%;
    background: var(--k-success);
  }
  .progress .count {
    font-size: 0.74rem;
    color: var(--k-ink-2);
    white-space: nowrap;
  }
  .warn {
    color: var(--k-warning);
  }
  .dim {
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

  /* The delete control stays out of the way until the pointer is on the row,
   * the same reveal the session list uses: a library is a list of reviews,
   * not a list of things to delete. */
  .x {
    flex: none;
    width: 18px;
    height: 18px;
    line-height: 1;
    font-size: 0.66rem;
    border-radius: 5px;
    color: var(--k-ink-3);
    opacity: 0;
    transition: opacity 0.09s ease;
  }
  .row:hover .x,
  .x:focus-visible {
    opacity: 1;
  }
  .x:hover,
  .x:focus-visible {
    color: var(--k-error);
  }

  .confirm {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 0.8rem;
    color: var(--k-warning);
    white-space: nowrap;
  }
  .danger,
  .keep {
    border: 1px solid var(--k-edge);
    border-radius: 7px;
    padding: 2px 9px;
    font-size: 0.78rem;
    color: var(--k-ink-2);
  }
  .danger {
    color: var(--k-error);
    border-color: color-mix(in srgb, var(--k-error) 60%, transparent);
  }
  .keep:hover,
  .danger:hover {
    background: var(--k-surface-2);
  }
</style>
