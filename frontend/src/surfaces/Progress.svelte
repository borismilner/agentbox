<script>
  import { bridge, on } from "../lib/bridge.js";

  // Progress (FR21): what a long task looks like when it is not asking for
  // anything. Thin bars, no answer zone, no urgency - a readout you glance at and
  // then ignore. The window opens with the first report and closes with the last,
  // so an idle desktop never carries one.

  let reports = $state([]);

  on("agentbox:progress", (list) => (reports = list ?? []));

  bridge.ready("progress");
  bridge.progress().then((list) => (reports = list ?? []));

  // Frameless, so the window has to be exactly as tall as the bars in it: one
  // task is a strip, four are a panel.
  let shell = $state(null);
  $effect(() => {
    if (!shell) return;
    const report = () => bridge.fitProgress(Math.ceil(shell.getBoundingClientRect().height));
    const ro = new ResizeObserver(report);
    ro.observe(shell);
    report();
    return () => ro.disconnect();
  });
</script>

<div class="wrap" bind:this={shell}>
  <div class="bar" style="--wails-draggable: drag">
    <span class="label">Progress</span>
    {#if reports.length > 1}<span class="n">{reports.length} tasks</span>{/if}
    <span class="spacer"></span>
    <button class="x" title="hide (the tasks keep running)" onclick={() => bridge.closeSelf()}>&#x2715;</button>
  </div>

  <div class="list">
    {#each reports as r (r.id)}
      <div class="row" style="--hue: {r.hue}">
        <div class="top">
          <span class="title">{r.title}</span>
          <span class="spacer"></span>
          {#if r.indeterminate}
            <span class="spin" aria-label="working"></span>
          {:else}
            <span class="pct">{r.percent}%</span>
          {/if}
        </div>

        {#if r.indeterminate}
          <div class="track indet"><span class="sweep"></span></div>
        {:else}
          <div class="track"><span class="fill" style="width: {r.percent}%"></span></div>
        {/if}

        <div class="sub">
          {#if r.agent}<span class="who">{r.agent}{r.project ? ` · ${r.project}` : ""}</span>{/if}
          {#if r.status}<span class="status">{r.status}</span>{/if}
        </div>
      </div>
    {:else}
      <p class="empty">No active tasks.</p>
    {/each}
  </div>
</div>

<style>
  .wrap {
    min-height: 100%;
    display: flex;
    flex-direction: column;
    background: var(--k-ground);
    color: var(--k-ink);
    border: 1px solid var(--k-edge);
    border-radius: var(--k-radius);
    overflow: hidden;
  }

  .bar {
    display: flex;
    align-items: center;
    gap: 9px;
    height: 32px;
    padding: 0 6px 0 13px;
    border-bottom: 1px solid var(--k-edge-soft);
  }
  .label {
    font-family: var(--k-font-mono);
    font-size: 0.62rem;
    letter-spacing: 0.13em;
    text-transform: uppercase;
    color: var(--k-ink-3);
  }
  .n {
    font-family: var(--k-font-mono);
    font-size: 0.62rem;
    color: var(--k-ink-3);
  }
  .spacer {
    flex: 1;
  }
  .x {
    width: 22px;
    height: 22px;
    border-radius: 6px;
    color: var(--k-ink-3);
    font-size: 0.62rem;
  }
  .x:hover {
    background: var(--k-surface-2);
    color: var(--k-ink);
  }

  .list {
    display: flex;
    flex-direction: column;
    gap: 14px;
    padding: 13px 14px 15px;
    background: var(--k-surface);
    flex: 1;
    min-height: 0;
    overflow-y: auto;
  }

  .row {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .top {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .title {
    font-size: 0.86rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .pct {
    font-family: var(--k-font-mono);
    font-size: 0.68rem;
    color: var(--k-ink-2);
  }

  /* The bar carries the caller's identity hue, so two agents working at once are
   * told apart without reading either title. */
  .track {
    position: relative;
    height: 5px;
    border-radius: 999px;
    background: var(--k-surface-3);
    overflow: hidden;
  }
  .fill {
    display: block;
    height: 100%;
    border-radius: 999px;
    background: var(--hue);
    transition: width 220ms cubic-bezier(0.3, 0.8, 0.4, 1);
  }
  .sweep {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 34%;
    border-radius: 999px;
    background: var(--hue);
    animation: sweep 1.35s ease-in-out infinite;
  }
  @keyframes sweep {
    from {
      left: -34%;
    }
    to {
      left: 100%;
    }
  }
  .spin {
    width: 11px;
    height: 11px;
    border-radius: 50%;
    border: 1.6px solid color-mix(in srgb, var(--hue) 30%, transparent);
    border-top-color: var(--hue);
    animation: spin 780ms linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  /* Honour the motion knob: "reduced" means nothing loops on screen. */
  :global(html[data-motion="reduced"]) .sweep,
  :global(html[data-motion="reduced"]) .spin {
    animation: none;
  }

  .sub {
    display: flex;
    align-items: baseline;
    gap: 9px;
    font-size: 0.72rem;
    color: var(--k-ink-3);
  }
  .who {
    font-family: var(--k-font-mono);
    font-size: 0.64rem;
    white-space: nowrap;
  }
  .status {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .empty {
    margin: auto;
    font-family: var(--k-font-read);
    color: var(--k-ink-3);
  }
</style>
