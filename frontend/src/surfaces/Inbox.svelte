<script>
  import { bridge } from "../lib/bridge.js";
  import { ticker } from "../lib/clock.svelte.js";

  // The inbox (FR10): everything still waiting, then everything recent. Its
  // real job is triage (FR34) - after a meeting the backlog has to clear in
  // seconds, from the keyboard, without reading a manual. So the keys are the
  // same ones the card uses, the selected row states them out loud, and the
  // mouse is the fallback rather than the path.
  //
  // Which key does what to which item is decided in Go (Bridge.Triage), so this
  // surface and the card cannot drift apart about what "d" means.

  let { inbox } = $props();

  let query = $state("");
  let sel = $state(0);
  let typing = $state(false);
  let box = $state(null);
  let listEl = $state(null);

  const clock = ticker();

  const items = $derived(filter(inbox?.items ?? [], query));
  const pending = $derived(items.filter((i) => i.pending));
  const chosen = $derived(pending[Math.min(sel, pending.length - 1)] ?? null);

  // The queue moves on its own - an item resolves, another arrives - so the
  // selection is clamped against whatever is pending now, never a stale index.
  $effect(() => {
    if (sel > pending.length - 1) sel = Math.max(0, pending.length - 1);
  });

  $effect(() => {
    if (!chosen || !listEl) return;
    listEl.querySelector(`[data-id="${chosen.id}"]`)?.scrollIntoView({ block: "nearest" });
  });

  function filter(list, q) {
    const needle = q.trim().toLowerCase();
    if (!needle) return list;
    return list.filter((i) =>
      [i.title, i.snippet, i.agent, i.project, i.kind, i.outcome].join(" ").toLowerCase().includes(needle),
    );
  }

  // TRIAGE_KEYS are the keys Go may act on. Listed here only so the surface
  // knows when to swallow the keystroke; the meaning stays on the Go side.
  const TRIAGE_KEYS = new Set(["1", "2", "3", "4", "5", "6", "7", "8", "9", "y", "n", "s", "d", "Enter", "Backspace"]);

  function onKey(e) {
    const inField = e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement;

    if (e.key === "Escape") {
      if (inField || typing) {
        e.preventDefault();
        typing = false;
        box?.blur();
      }
      return;
    }
    if (inField) return;

    if (e.key === "/") {
      e.preventDefault();
      typing = true;
      queueMicrotask(() => box?.focus());
      return;
    }
    if (e.key === "j" || e.key === "ArrowDown") {
      e.preventDefault();
      sel = Math.min(sel + 1, Math.max(0, pending.length - 1));
      return;
    }
    if (e.key === "k" || e.key === "ArrowUp") {
      e.preventDefault();
      sel = Math.max(sel - 1, 0);
      return;
    }
    if (!chosen) return;
    if (e.key === "c" && !e.ctrlKey && !e.metaKey) {
      e.preventDefault();
      bridge.copyItem(chosen.id);
      return;
    }
    if (TRIAGE_KEYS.has(e.key)) {
      e.preventDefault();
      bridge.triage(chosen.id, e.key);
    }
  }

  // rel ages on the shared 1Hz tick, so "now" becomes "2m ago" without a
  // reload. Coarse on purpose: the exact second a question arrived never matters.
  function rel(msAt, now) {
    const d = Math.max(0, now - msAt);
    if (d < 60_000) return "now";
    if (d < 3_600_000) return `${Math.floor(d / 60_000)}m ago`;
    if (d < 86_400_000) return `${Math.floor(d / 3_600_000)}h ago`;
    return `${Math.floor(d / 86_400_000)}d ago`;
  }

  const sectionStart = (i) => i === 0 || items[i - 1].pending !== items[i].pending;
  const hint = $derived(
    typing ? "esc leaves search" : pending.length ? "j/k move · keys answer · / search" : "/ search",
  );
</script>

<svelte:window onkeydown={onKey} />

<section class="inbox">
  <header>
    <div class="line">
      <h1>Inbox</h1>
      {#if inbox?.pending}
        <span class="count">{inbox.pending} pending</span>
      {:else}
        <span class="count quiet">all quiet</span>
      {/if}
    </div>
    <div class="search" class:on={typing}>
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
        <circle cx="11" cy="11" r="6.5" /><path d="m16 16 4.5 4.5" />
      </svg>
      <input
        bind:this={box}
        bind:value={query}
        onfocus={() => (typing = true)}
        onblur={() => (typing = false)}
        placeholder="Search title, agent, kind…"
        spellcheck="false"
      />
      {#if query}
        <button class="clear" onclick={() => (query = "")} title="Clear">&#x2715;</button>
      {/if}
    </div>
  </header>

  <div class="list" bind:this={listEl}>
    {#if !(inbox?.items ?? []).length}
      <p class="empty">Nothing yet. When an agent has a question for you, it lands here.</p>
    {:else if !items.length}
      <p class="empty">No matches for “{query}”.</p>
    {:else}
      {#each items as it, i (it.id)}
        {#if sectionStart(i)}
          <span class="section">{it.pending ? "Pending" : "Recent"}</span>
        {/if}

        <div class="rowwrap" data-id={it.id}>
          <button
            type="button"
            class="row {it.level}"
            class:pending={it.pending}
            class:on={!typing && chosen?.id === it.id}
            onclick={() => it.pending && bridge.promote(it.id)}
            title={it.pending ? "Click to show the card" : it.snippet}
          >
            <span class="sev"></span>
            <span class="body">
              <span class="title">{it.title}</span>
              <span class="sub">
                <span class="idot" style="background: {it.hue}"></span>
                {it.agent}{it.project ? ` · ${it.project}` : ""} · {rel(it.createdMs, clock.now)}
                {#if it.muted}<em class="muted-badge">muted</em>{/if}
              </span>
            </span>
            <span class="outcome {it.tone}">{it.outcome}</span>
          </button>

          <!-- The hint is only honest while the keys are live: with the search
               box focused, "s stop" would type an s. -->
          {#if !typing && chosen?.id === it.id && it.hint}
            <span class="hint">{it.hint}</span>
          {/if}
        </div>
      {/each}
    {/if}
  </div>

  <footer>
    <span>{hint}</span>
    <span
      >{inbox?.today ?? 0} interruption{(inbox?.today ?? 0) === 1 ? "" : "s"} today</span
    >
  </footer>
</section>

<style>
  .inbox {
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
  .count {
    font-family: var(--k-font-mono);
    font-size: 0.7rem;
    color: var(--k-info);
  }
  .count.quiet {
    color: var(--k-ink-3);
  }

  .search {
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

  .list {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 4px 10px 14px;
  }
  .empty {
    margin: 44px 18px;
    text-align: center;
    font-family: var(--k-font-read);
    font-size: 0.9rem;
    color: var(--k-ink-3);
  }

  /* The label voice every surface shares (Home's h2): UI caps for headings,
   * mono only for data. */
  .section {
    display: block;
    padding: 14px 8px 5px;
    font-size: 0.76rem;
    font-weight: 600;
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--k-ink-3);
  }

  .rowwrap {
    display: flex;
    flex-direction: column;
  }
  .row {
    position: relative;
    display: flex;
    align-items: center;
    gap: 11px;
    width: 100%;
    text-align: left;
    padding: 9px 12px 9px 11px;
    border-radius: 9px;
    border: 1px solid transparent;
  }
  .row.pending:hover {
    background: var(--k-surface-2);
  }
  .row:not(.pending) {
    cursor: default;
  }
  .row.on {
    background: var(--k-surface-2);
    border-color: var(--k-edge);
  }

  /* The severity stripe, same device as the card's rail: level is a property of
   * the item, so it is stated in the one place saturation is allowed. */
  .sev {
    flex: 0 0 auto;
    width: 3px;
    align-self: stretch;
    min-height: 26px;
    border-radius: 2px;
    background: var(--k-ink-3);
  }
  .row.info .sev {
    background: var(--k-info);
  }
  .row.success .sev {
    background: var(--k-success);
  }
  .row.warning .sev {
    background: var(--k-warning);
  }
  .row.error .sev,
  .row.urgent .sev {
    background: var(--k-error);
  }
  .row:not(.pending) .sev {
    opacity: 0.42;
  }

  .body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }
  .title {
    font-size: 0.87rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .row:not(.pending) .title {
    color: var(--k-ink-2);
  }
  .sub {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 0.72rem;
    color: var(--k-ink-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .idot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex: 0 0 auto;
  }
  .muted-badge {
    font-style: normal;
    font-family: var(--k-font-mono);
    font-size: 0.6rem;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--k-warning);
  }

  .outcome {
    flex: 0 0 auto;
    max-width: 22ch;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.72rem;
    color: var(--k-ink-3);
  }
  .outcome.info {
    color: var(--k-info);
  }
  .outcome.success {
    color: var(--k-success);
  }
  .outcome.warning {
    color: var(--k-warning);
  }
  .outcome.error {
    color: var(--k-error);
  }

  .hint {
    padding: 0 12px 8px 27px;
    font-family: var(--k-font-mono);
    font-size: 0.68rem;
    color: var(--k-info);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 14px;
    padding: 9px 18px;
    border-top: 1px solid var(--k-edge-soft);
    font-family: var(--k-font-mono);
    font-size: 0.64rem;
    color: var(--k-ink-3);
  }
</style>
