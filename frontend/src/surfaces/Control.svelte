<script>
  // The hands-off strip (FR74). While this window is on screen the desktop
  // belongs to an agent; when it goes, it belongs to the human again. That
  // presence is the whole signal, which is why there is no idle state to render
  // and no close button to press: dismissing it would be lying to yourself about
  // whose desktop it is.
  //
  // Two states. `asking` carries the reason, a countdown and Deny, so the decision
  // happens where the question is rather than in a card beside it. `driving`
  // carries the activity line the agent keeps updating and the age of that line,
  // because Boris asked to know "every moment where we are, that nothing is stuck".
  import { bridge, on } from "../lib/bridge.js";

  let run = $state(null);

  bridge.control().then((st) => (run = st)).catch(() => {});
  on("agentbox:control", (st) => (run = st));

  // Both clocks tick here rather than arriving with every repaint: the daemon
  // sends state when state changes, and a countdown that only moved when
  // something happened would look frozen at exactly the moment it matters.
  //
  // They are anchored to the moment a payload arrived, not to a bare local clock,
  // so a window that opens mid-run shows the true age instead of starting at zero.
  let now = $state(Date.now());
  let base = $state({ at: Date.now(), since: 0, left: 0 });
  $effect(() => {
    if (!run) return;
    base = { at: Date.now(), since: run.since_ms ?? 0, left: run.left_ms ?? 0 };
  });
  $effect(() => {
    const tick = setInterval(() => (now = Date.now()), 200);
    return () => clearInterval(tick);
  });

  const elapsed = $derived(now - base.at);
  const asking = $derived(run?.state === "asking");
  const leftMs = $derived(Math.max(base.left - elapsed, 0));
  const ageMs = $derived(base.since + elapsed);

  // The countdown reads in whole seconds because that is how somebody deciding
  // reads it, and it never shows 0 while the answer is still open.
  const leftS = $derived(Math.max(Math.ceil(leftMs / 1000), 0));
  const pct = $derived(base.left > 0 ? Math.max((leftMs / base.left) * 100, 0) : 0);

  function age(ms) {
    const s = Math.floor(ms / 1000);
    if (s < 60) return `${s}s`;
    const m = Math.floor(s / 60);
    return s % 60 ? `${m}m ${s % 60}s` : `${m}m`;
  }

  // A line that has not changed for a while is the visible form of "stuck". It is
  // never an error - some steps genuinely take a minute - so it warns rather than
  // alarms, and only once it is past what any single step should need.
  const stale = $derived(!asking && ageMs > 45000);

  function deny() {
    bridge.controlDeny(run?.id ?? "").catch(() => {});
  }
  function allow() {
    bridge.controlAllow(run?.id ?? "").catch(() => {});
  }
</script>

{#if run}
  <div class="strip" class:asking>
    <!-- The countdown is the background of the whole strip while asking: a bar
         somebody has to find is a bar somebody misses, and this one is the thing
         being decided. -->
    {#if asking}<div class="sweep" style="width: {pct}%"></div>{/if}

    <span class="dot" aria-hidden="true"></span>

    <span class="label">
      {#if asking}May I take the desktop?{:else}HANDS OFF{/if}
    </span>

    <span class="what" title={asking ? run.reason : run.activity || run.reason}>
      {#if asking}
        {run.reason}
      {:else}
        {run.activity || run.reason}
      {/if}
    </span>

    <span class="who" title="{run.identity?.agent ?? ''}{run.identity?.project ? ' · ' + run.identity.project : ''}">
      {run.identity?.agent ?? ""}
    </span>

    {#if asking}
      <span class="count">{leftS}s</span>
      <button class="btn deny" onclick={deny}>Deny</button>
      <button class="btn allow" onclick={allow}>Now</button>
    {:else}
      <span class="count" class:stale title="since this line last changed">{age(ageMs)}</span>
    {/if}
  </div>
{/if}

<style>
  /* Amber, not red: red is an error and nothing here has gone wrong. Amber is the
     colour of "something is in progress that you should not walk into", which is
     exactly the message. */
  .strip {
    position: relative;
    overflow: hidden;
    display: flex;
    align-items: center;
    gap: 12px;
    height: 100vh;
    padding: 0 14px;
    box-sizing: border-box;
    font-family: var(--k-font-ui, system-ui);
    font-size: 13px;
    color: var(--k-ink);
    background: var(--k-surface);
    border: 1px solid var(--k-warning);
    border-left: 4px solid var(--k-warning);
    border-radius: 10px;
  }
  .strip.asking {
    border-color: var(--k-accent);
    border-left-color: var(--k-accent);
  }
  .sweep {
    position: absolute;
    inset: 0 auto 0 0;
    background: color-mix(in srgb, var(--k-accent) 14%, transparent);
    transition: width 200ms linear;
    pointer-events: none;
  }
  /* Everything above the sweep. */
  .dot,
  .label,
  .what,
  .who,
  .count,
  .btn {
    position: relative;
  }
  .dot {
    width: 9px;
    height: 9px;
    flex: none;
    border-radius: 999px;
    background: var(--k-warning);
    animation: pulse 1.8s ease-in-out infinite;
  }
  .asking .dot {
    background: var(--k-accent);
  }
  @keyframes pulse {
    50% {
      opacity: 0.35;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .dot {
      animation: none;
    }
    .sweep {
      transition: none;
    }
  }
  .label {
    flex: none;
    font-weight: 700;
    font-size: 11px;
    letter-spacing: 0.08em;
    color: var(--k-warning);
    white-space: nowrap;
  }
  .asking .label {
    color: var(--k-accent);
    letter-spacing: 0.01em;
    font-size: 12.5px;
  }
  /* The activity line takes the room, because it is the thing being read. One
     line, ellipsised: a strip that grows to fit its text would move under the
     eye every time the agent said something new. */
  .what {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--k-ink);
  }
  .who {
    flex: none;
    max-width: 12ch;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 11px;
    color: var(--k-ink-3);
  }
  .count {
    flex: none;
    font-variant-numeric: tabular-nums;
    font-size: 11px;
    color: var(--k-ink-3);
  }
  /* Past what any one step should take, the age is the warning - it says "look at
     this" without claiming anything has failed. */
  .count.stale {
    color: var(--k-warning);
    font-weight: 700;
  }
  .btn {
    flex: none;
    cursor: pointer;
    border-radius: 6px;
    padding: 4px 10px;
    font: inherit;
    font-size: 12px;
    background: none;
    border: 1px solid var(--k-edge);
    color: var(--k-ink-2);
  }
  .btn:hover,
  .btn:focus-visible {
    color: var(--k-ink);
  }
  .btn.deny {
    border-color: var(--k-error);
    color: var(--k-error);
  }
  .btn.deny:hover,
  .btn.deny:focus-visible {
    background: color-mix(in srgb, var(--k-error) 14%, transparent);
  }
  .btn.allow {
    border-color: var(--k-accent);
    color: var(--k-accent);
  }
  .btn.allow:hover,
  .btn.allow:focus-visible {
    background: color-mix(in srgb, var(--k-accent) 14%, transparent);
  }
</style>
