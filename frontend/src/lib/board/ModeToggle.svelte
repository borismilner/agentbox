<script>
  // The reading mode, as two exclusive states rather than one toggling label: a
  // lone chip reading "TL;DR" cannot say whether that is what you are looking at
  // or what you would get by pressing it, and the reader who opened straight
  // into the short version is exactly the one who needs that answered without
  // clicking to find out.
  //
  // Its own component because it needs to be unmistakably a control, and one
  // small stylesheet is easier to keep that way than another twenty lines in a
  // seven-hundred-line one.
  let { brief, onPick } = $props();
</script>

<div class="track" role="group" aria-label="reading mode">
  <button class={brief ? "seg on" : "seg"} aria-pressed={brief} onclick={() => onPick(true)} title="the TL;DR (t)">
    TL;DR
  </button>
  <button class={brief ? "seg" : "seg on"} aria-pressed={!brief} onclick={() => onPick(false)} title="the full text (t)">
    Full text
  </button>
</div>

<style>
  /* One track, two states, and the filled one is where you ARE. Sharing a track
     is what makes them read as a choice already made rather than as two separate
     things you could press. */
  .track {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 3px;
    border-radius: 999px;
    background: var(--k-surface-2, #1b1f27);
    border: 1px solid var(--k-edge, #2b323d);
  }
  .seg {
    margin: 0;
    padding: 4px 13px;
    border: 0;
    border-radius: 999px;
    background: transparent;
    color: var(--k-ink-3, #8b93a1);
    font: inherit;
    font-size: 0.78em;
    font-weight: 600;
    line-height: 1.3;
    white-space: nowrap;
    cursor: pointer;
    transition:
      background 160ms ease,
      color 160ms ease;
  }
  .seg:hover {
    color: var(--k-ink, #e6e9ef);
    background: color-mix(in srgb, currentColor 10%, transparent);
  }
  .seg:focus-visible {
    outline: 2px solid var(--k-info, #6ea8fe);
    outline-offset: 1px;
  }
  .seg.on,
  .seg.on:hover {
    color: var(--k-ground, #0d1117);
    background: var(--k-accent, #7c8cf8);
  }
  /* The fallbacks above are not belt and braces: a var() that resolves to
     nothing takes the whole declaration with it, and a control that has silently
     lost its background still reads as working to everything except the screen.
     This one did exactly that once already. */
  :global([data-motion="reduced"]) .seg,
  :global([data-motion="none"]) .seg {
    transition: none;
  }
</style>
