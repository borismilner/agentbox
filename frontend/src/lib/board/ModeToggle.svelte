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
  //
  // `available` is false when NOT ONE step in the review has a TL;DR. The control
  // then has to say so rather than offer a mode it cannot enter: a lit "TL;DR"
  // over a screen full of code is the control telling the reader something untrue
  // about what they are looking at, which is worse than having no control.
  let { brief, onPick, available = true } = $props();
</script>

<div class="track" class:empty={!available} role="group" aria-label="reading mode">
  <button
    class={brief && available ? "seg on" : "seg"}
    aria-pressed={brief && available}
    disabled={!available}
    onclick={() => onPick(true)}
    title={available ? "the TL;DR (t)" : "no step in this review has a TL;DR - it was written before they existed, or its author left them out"}
  >
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
  /* Offered and unavailable, which is different from not offered: the reader
     should be able to see that this review COULD have had a short version and
     does not, and hover to find out why. */
  .seg:disabled {
    cursor: default;
    color: var(--k-ink-3, #8b93a1);
    opacity: 0.45;
    background: transparent;
  }
  .track.empty {
    opacity: 0.9;
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
