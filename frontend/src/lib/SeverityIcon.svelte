<script>
  // The drawn severity marks (03-ui-ux.md): circled i, check, warning triangle,
  // crossed circle, bell. Which mark a level wears is decided in Go
  // (severityGlyph) and arrives as a name - this only draws the shape, so the
  // card, the toast and the inbox cannot end up disagreeing about what a warning
  // looks like.

  let { glyph = "info", size = 20 } = $props();
</script>

<svg
  class={glyph}
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="1.7"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
>
  {#if glyph === "check"}
    <circle cx="12" cy="12" r="9" />
    <path d="M8 12.4l2.6 2.6L16 9.6" />
  {:else if glyph === "warning"}
    <path d="M12 4.2 21 19.4H3z" />
    <path d="M12 10v4.2" />
    <circle cx="12" cy="17" r="0.9" fill="currentColor" stroke="none" />
  {:else if glyph === "cross"}
    <circle cx="12" cy="12" r="9" />
    <path d="M9 9l6 6M15 9l-6 6" />
  {:else if glyph === "bell"}
    <path d="M18 15V10.5a6 6 0 1 0-12 0V15l-1.6 2.6h15.2z" />
    <path d="M9.6 20.4a2.6 2.6 0 0 0 4.8 0" />
  {:else}
    <circle cx="12" cy="12" r="9" />
    <path d="M12 11v5.4" />
    <circle cx="12" cy="7.9" r="0.95" fill="currentColor" stroke="none" />
  {/if}
</svg>

<style>
  svg {
    flex: 0 0 auto;
    color: var(--sev);
  }
</style>
