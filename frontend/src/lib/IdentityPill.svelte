<script>
  import { identityHue } from "./tokens.js";

  let { agent = "", project = "", session = "", compact = false } = $props();
  const hue = $derived(identityHue(agent, project));
  const label = $derived([agent, project].filter(Boolean).join(" · ") || "unknown");
</script>

<span class="pill" class:compact style="--hue: {hue}" title={session ? `session ${session}` : label}>
  <span class="dot"></span>{label}
</span>

<style>
  .pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 2px 9px 2px 7px;
    border-radius: 999px;
    font-size: 0.74rem;
    letter-spacing: 0.01em;
    white-space: nowrap;
    border: 1px solid color-mix(in srgb, var(--hue) 42%, transparent);
    background: color-mix(in srgb, var(--hue) 13%, transparent);
    color: color-mix(in srgb, var(--hue) 74%, var(--k-ink));
  }
  .pill.compact {
    font-size: 0.68rem;
    padding: 1px 7px 1px 5px;
  }
  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--hue);
    flex: 0 0 auto;
  }
</style>
