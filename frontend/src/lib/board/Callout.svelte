<script>
  // A callout: one thing that must not be read as part of the flow. The trap, the
  // consequence, the thing the reader will otherwise carry away wrong.
  //
  // Its words are rendered by the step, not here, and passed in as a snippet -
  // prose can bind a phrase to a code region and mark a glossary term, and both
  // of those belong to the step that owns the binds. So this component owns the
  // frame and the tone, and reaches the borrowed markup with :global rules.
  import SeverityIcon from "../SeverityIcon.svelte";

  let { callout, body } = $props();

  // The four tones are the board's semantic colours, and they are the same four
  // an item's severity uses on every other surface: a warning on a card and a
  // warning in a walkthrough should not be two different colours of warning.
  const GLYPH = { note: "info", good: "check", warn: "warning", danger: "cross" };
</script>

<aside class="callout {callout.tone}">
  <span class="mark"><SeverityIcon glyph={GLYPH[callout.tone] ?? "info"} size={18} /></span>
  <div class="text">
    {#if callout.title}<p class="title">{callout.title}</p>{/if}
    {@render body?.()}
  </div>
</aside>

<style>
  /* One shape, four colours, and the colour is the only thing that changes: the
     tone is information, so it must not also change the density or the weight of
     what it holds, or two callouts of different tones stop being comparable. */
  .callout {
    --tone: var(--k-info, #4f9cd8);
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: 12px;
    align-items: start;
    margin: 4px 0 26px;
    max-width: 68ch;
    padding: 15px 18px 16px;
    border: 1px solid color-mix(in srgb, var(--tone) 32%, var(--k-edge, #2a2f37));
    border-left: 3px solid var(--tone);
    border-radius: 4px 12px 12px 4px;
    background: color-mix(in srgb, var(--tone) 8%, var(--k-surface, #16181c));
  }
  .callout.good {
    --tone: var(--k-success, #58a55c);
  }
  .callout.warn {
    --tone: var(--k-warning, #d8a33a);
  }
  .callout.danger {
    --tone: var(--k-error, #d05f5f);
  }
  .mark {
    display: flex;
    color: var(--tone);
    padding-top: 2px;
  }
  .title {
    margin: 0 0 6px;
    font-family: var(--k-font-ui, system-ui, sans-serif);
    font-size: 1em;
    font-weight: 700;
    letter-spacing: 0.01em;
    color: var(--k-ink, #e6e9ef);
  }
  /* The step rendered these paragraphs, so they carry its scope and not this
     one's. They are set a little tighter and a little smaller than the prose
     around the callout, which is what makes the box read as an aside rather than
     as the page continuing inside a border. */
  .text :global(p) {
    margin: 0 0 0.5em;
    font-family: var(--k-font-read, Georgia, serif);
    font-size: 1.08em;
    line-height: 1.55;
    color: var(--k-ink-2, #c8cdd6);
    text-wrap: pretty;
  }
  .text :global(p:last-child) {
    margin-bottom: 0;
  }
  .text :global(code) {
    font-family: var(--k-font-mono, ui-monospace, monospace);
    font-size: 0.82em;
    background: var(--k-surface-2, #1c2027);
    border-radius: 4px;
    padding: 1px 5px;
  }
</style>
