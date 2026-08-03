<script>
  // The route rail: every station at once, in worklist order of attention -
  // UNREAD stations are the bright ones, understood goes quiet, the current
  // one is accented (round 6, the same inversion the diff card ships).
  let { steps, marks, at, go } = $props();

  const glyph = (s) =>
    s.kind === "ground" ? "◇" : s.kind === "none" ? "∅" : s.kind === "check" ? "$" : String(counted(s) + 1);
  const counted = (s) => steps.filter((x) => x.kind === "code").findIndex((x) => x.id === s.id);
  const sub = (s) =>
    s.kind === "ground" ? "does not count" : s.kind === "none" ? "nothing to review" : s.kind === "check" ? "the gate" : "";
</script>

<nav>
  {#each steps as s, i (s.id)}
    {@const v = marks[s.id]?.verdict ?? ""}
    <button class="station" class:current={i === at} onclick={() => go(i)}>
      <div class="spine">
        <div class="circle" class:ok={v === "understood"} class:warn={v === "unclear"} class:muted={s.kind !== "code"}>
          {v === "understood" && s.kind === "code" ? "✓" : glyph(s)}
        </div>
        {#if i < steps.length - 1}<div class="wire"></div>{/if}
      </div>
      <div class="label">
        <div class="name" class:quiet={v === "understood"}>{s.title}</div>
        {#if s.kind !== "code"}<div class="sub">{sub(s)}</div>{/if}
      </div>
    </button>
  {/each}
</nav>

<style>
  nav {
    width: 20em;
    flex: none;
    overflow-y: auto;
    border-right: 1px solid var(--k-edge);
    padding: 16px 20px;
    display: flex;
    flex-direction: column;
  }
  .station {
    display: flex;
    gap: 12px;
    width: 100%;
    text-align: left;
    background: none;
    border: 0;
    margin: 0 -6px;
    padding: 2px 6px;
    border-radius: 8px;
    cursor: pointer;
    color: inherit;
    font: inherit;
    transition: background 120ms ease;
  }
  .station:hover {
    background: color-mix(in srgb, var(--k-ink) 5%, transparent);
  }
  .station:focus-visible {
    outline: 2px solid var(--k-info);
  }
  .spine {
    display: flex;
    flex-direction: column;
    align-items: center;
  }
  .circle {
    width: 2em;
    height: 2em;
    flex: none;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    border: 1px solid var(--k-edge);
    font-family: var(--k-font-mono);
    font-size: 0.95em;
    color: var(--k-ink);
  }
  .current .circle {
    border-color: var(--k-info);
    background: color-mix(in srgb, var(--k-info) 12%, transparent);
    font-weight: 700;
  }
  .circle.ok {
    border-color: var(--k-success);
    color: var(--k-success);
  }
  .circle.warn {
    border-color: var(--k-warning);
    color: var(--k-warning);
    background: color-mix(in srgb, var(--k-warning) 14%, transparent);
  }
  .circle.muted {
    color: var(--k-ink-2);
  }
  .wire {
    width: 1px;
    flex: 1;
    min-height: 20px;
    background: var(--k-edge);
  }
  .label {
    padding: 0.35em 0 1em;
    min-width: 0;
  }
  .name {
    font-size: 1.02em;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .current .name {
    font-weight: 700;
  }
  .name.quiet {
    color: var(--k-ink-2);
  }
  .sub {
    font-size: 0.82em;
    color: var(--k-ink-3);
  }
</style>
