<script>
  // The glossary drawer (FR68). Definitions belong out of the reading flow:
  // a term explained inline costs every reader the interruption and helps
  // only the ones who did not know it. So the prose carries a quiet mark and
  // this opens on demand - by clicking the mark, or with g for the whole
  // list.
  //
  // It overlays rather than taking a column, because the board's standing
  // rule is that nothing the reader hovers or opens may move the text under
  // them. Closing puts them back on the exact line they left.
  let { terms, openKey = null, onClose } = $props();

  let panel = $state(null);

  // Bring the asked-for entry into view, and let the reader's eye find it by
  // the highlight rather than by re-reading the list.
  $effect(() => {
    if (!openKey || !panel) return;
    const el = panel.querySelector(`[data-term="${CSS.escape(openKey)}"]`);
    if (el) el.scrollIntoView({ block: "center", behavior: "smooth" });
  });
</script>

<aside class="drawer" bind:this={panel} aria-label="Glossary">
  <div class="head">
    <span class="label">Glossary</span>
    <span class="hint">g closes · click any marked word</span>
    <button class="x" onclick={onClose} title="close (g or Esc)">✕</button>
  </div>
  <div class="list">
    {#each terms as t (t.key)}
      <div class="entry" class:open={t.key === openKey} data-term={t.key}>
        <div class="term">{t.term}</div>
        <div class="short">{t.short}</div>
        {#if t.body}<div class="body">{t.body}</div>{/if}
      </div>
    {/each}
  </div>
</aside>

<style>
  .drawer {
    position: absolute;
    top: 0;
    right: 0;
    bottom: 0;
    width: min(28em, 92vw);
    display: flex;
    flex-direction: column;
    background: var(--k-surface);
    border-left: 1px solid var(--k-edge);
    box-shadow: -16px 0 40px rgb(0 0 0 / 0.28);
    z-index: 5;
    animation: slide 140ms ease-out;
  }
  @keyframes slide {
    from {
      transform: translateX(12px);
      opacity: 0;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .drawer {
      animation: none;
    }
  }
  .head {
    display: flex;
    align-items: baseline;
    gap: 10px;
    padding: 10px 14px;
    border-bottom: 1px solid var(--k-edge);
    flex: none;
  }
  .label {
    font-weight: 700;
    font-size: 0.95em;
  }
  .hint {
    color: var(--k-ink-3);
    font-size: 0.78em;
    margin-right: auto;
  }
  .x {
    background: none;
    border: 0;
    color: var(--k-ink-3);
    cursor: pointer;
    font-size: 0.9em;
    padding: 0 2px;
  }
  .x:hover {
    color: var(--k-ink);
  }
  .list {
    overflow-y: auto;
    padding: 6px 0 24px;
  }
  .entry {
    padding: 12px 14px;
    border-left: 2px solid transparent;
  }
  .entry.open {
    border-left-color: var(--k-info);
    background: color-mix(in srgb, var(--k-info) 8%, transparent);
  }
  .term {
    font-weight: 700;
    font-size: 0.95em;
    margin-bottom: 3px;
  }
  .short {
    font-family: var(--k-font-read);
    font-size: 1.05em;
    line-height: 1.5;
  }
  .body {
    font-family: var(--k-font-read);
    font-size: 1em;
    line-height: 1.55;
    color: var(--k-ink-2);
    margin-top: 6px;
    white-space: pre-wrap;
  }
</style>
