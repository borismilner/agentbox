<script>
  // The control for one readable region (FR72). Play, and stop - there is no
  // pause, because pause needed the text split into passages and the split is
  // what cost the speech its last words.
  //
  // One of these sits with each run of prose rather than one at the top of the
  // step, because the reader's motion is to hear the paragraph above a block, read
  // the block, then ask for the next paragraph. A single control at the top can
  // only offer "read from here to the end", which is the thing being read over.
  let { region, readingRegion, onAloud, label } = $props();

  const on = $derived(readingRegion === region);
</script>

<button
  class="aloud"
  class:on
  onclick={() => onAloud(region)}
  aria-label={on ? `stop reading ${label}` : `read ${label} aloud`}
  title={on ? "stop reading (Esc)" : `read ${label} aloud`}
>
  <span class="glyph" aria-hidden="true">{on ? "■" : "▶"}</span>
</button>

<style>
  /* Quiet until it is wanted: the prose is what the eye should land on, so this
     is a small ring in the muted ink that fills in only on hover, focus, or while
     it is the one playing. */
  .aloud {
    all: unset;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1.55em;
    height: 1.55em;
    flex: none;
    border-radius: 999px;
    border: 1px solid var(--k-edge);
    color: var(--k-ink-3);
    font-size: 0.62em;
    line-height: 1;
    vertical-align: 0.32em;
    transition: color 120ms ease, border-color 120ms ease;
  }
  .aloud:hover,
  .aloud:focus-visible {
    color: var(--k-accent);
    border-color: var(--k-accent);
  }
  .aloud.on {
    color: var(--k-accent);
    border-color: var(--k-accent);
    background: color-mix(in srgb, var(--k-accent) 12%, transparent);
  }
  /* The play triangle sits optically low inside a circle; the stop square does
     not. Nudging only the one that needs it keeps both centred. */
  .glyph {
    display: block;
    transform: translateY(-0.02em);
  }
  @media (prefers-reduced-motion: reduce) {
    .aloud {
      transition: none;
    }
  }
</style>
