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
  // because the point is to know "every moment where we are, that nothing is stuck".
  //
  // And one latch across both (FR94): paused, the desktop is the human's again
  // mid-run. It is the inverted strip he picked at the mock - the same window,
  // green instead of amber, the label flipped and the frozen activity line still
  // readable so he can see what it goes back to. Presence never lapses, which is
  // why this is a repaint and not a hide: the strip staying up under a different
  // colour is how a glance still answers "whose desktop is this?".
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
  let base = $state({ at: Date.now(), since: 0, left: 0, held: 0 });
  $effect(() => {
    if (!run) return;
    base = {
      at: Date.now(),
      since: run.since_ms ?? 0,
      left: run.left_ms ?? 0,
      held: run.paused_ms ?? 0,
    };
  });
  $effect(() => {
    const tick = setInterval(() => (now = Date.now()), 200);
    return () => clearInterval(tick);
  });

  const elapsed = $derived(now - base.at);
  const paused = $derived(!!run?.paused);
  const asking = $derived(!paused && run?.state === "asking");
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
  const stale = $derived(!asking && !paused && ageMs > 45000);

  // How long he has held the desktop, and the one escalation the sign makes on
  // its own (FR94). Nothing ever auto-resumes - that was the whole point of his
  // answer - so past two minutes the strip stops saying "the desktop is yours"
  // and starts saying somebody is waiting on him. Green to amber, and the label
  // with it: the message has genuinely changed, and a counter quietly climbing
  // is not a thing anybody reads.
  const heldMs = $derived(base.held + elapsed);
  // Paused with nobody behind it: he latched an idle desktop, or the agent gave
  // up while he held it. Different sentence, different button.
  const idle = $derived(paused && !run?.waiting);
  // And no escalation at all in that case. The warm state's whole content is
  // "somebody is blocked on you", so with nobody parked it says a thing that is
  // not true - which is how it read on screen at 2m50s with the run released:
  // an amber AGENT WAITING over a line explaining that nothing is driving.
  const warm = $derived(paused && !idle && heldMs > 120000);

  function deny() {
    bridge.controlDeny(run?.id ?? "").catch(() => {});
  }
  function allow() {
    bridge.controlAllow(run?.id ?? "").catch(() => {});
  }
  function pause() {
    bridge.controlPause().catch(() => {});
  }
  function resume() {
    bridge.controlResume().catch(() => {});
  }
</script>

{#if run}
  <div class="strip" class:asking class:paused class:warm>
    <!-- The countdown is the background of the whole strip while asking: a bar
         somebody has to find is a bar somebody misses, and this one is the thing
         being decided. -->
    {#if asking}<div class="sweep" style="width: {pct}%"></div>{/if}

    <span class="dot" aria-hidden="true"></span>

    <span class="label">
      {#if paused}
        {warm ? "AGENT WAITING" : "PAUSED - YOURS"}
      {:else if asking}May I take the desktop?{:else}HANDS OFF{/if}
    </span>

    <span class="what" title={asking ? run.reason : run.activity || run.reason}>
      {#if idle}
        nothing is driving; agents are held off until you release it
      {:else if asking}
        {run.reason}
      {:else}
        {run.activity || run.reason}
      {/if}
    </span>

    <span class="who" title="{run.identity?.agent ?? ''}{run.identity?.project ? ' · ' + run.identity.project : ''}">
      {run.identity?.agent ?? ""}
    </span>

    {#if paused}
      <span class="count" title="how long you have had the desktop">{age(heldMs)}</span>
      <!-- Resume is the human's and only the human's, so it is a button here, on
           the hotkey and in his shell, and nowhere an agent can reach. -->
      <button class="btn resume" onclick={resume}>{idle ? "Allow agents" : "Resume"}</button>
    {:else if asking}
      <span class="count">{leftS}s</span>
      <button class="btn deny" onclick={deny}>Deny</button>
      <button class="btn allow" onclick={allow}>Now</button>
    {:else}
      <span class="count" class:stale title="since this line last changed">{age(ageMs)}</span>
      <!-- The discoverable way to pause. The hotkey is the fast one, and it has
           to be, because the pointer this button needs is the pointer an agent
           is currently moving. -->
      <button class="btn" onclick={pause} title="take the keyboard and mouse back without ending the run">
        Pause
      </button>
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
  /* Paused: the same window saying the opposite thing (FR94). Green is the only
     colour on the palette that means "go ahead" without meaning "finished", and
     the tint on the surface is what makes it read as a state rather than as the
     amber strip with a different border. Every var() carries a fallback - a
     var() that resolves to nothing takes its whole declaration with it, and a
     hands-off sign that has silently lost its background still reads as working
     to everything except the screen. */
  .strip.paused {
    border-color: var(--k-success, #4fb286);
    border-left-color: var(--k-success, #4fb286);
    background: color-mix(in srgb, var(--k-success, #4fb286) 10%, var(--k-surface, #161920));
  }
  /* Past two minutes the message is no longer "the desktop is yours" but
     "somebody is waiting on you", and the colour has to move with it or the
     counter is the only thing carrying it. */
  .strip.paused.warm {
    border-color: var(--k-warning, #d9a441);
    border-left-color: var(--k-warning, #d9a441);
    background: color-mix(in srgb, var(--k-warning, #d9a441) 12%, var(--k-surface, #161920));
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
  /* Still, and deliberately: the pulse is what says work is happening, and under
     the latch none is. */
  .paused .dot {
    background: var(--k-success, #4fb286);
    animation: none;
  }
  .paused.warm .dot {
    background: var(--k-warning, #d9a441);
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
  .paused .label {
    color: var(--k-success, #4fb286);
  }
  .paused.warm .label {
    color: var(--k-warning, #d9a441);
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
  /* Frozen, not gone: italic and dimmed says the line is what the run WILL go
     back to rather than what is happening, and keeping it there is what makes
     resuming an informed decision instead of a guess. */
  .paused .what {
    color: var(--k-ink-3, #69717e);
    font-style: italic;
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
  /* The one filled button on the strip. Everything else here is a border and a
     colour, because everything else is optional; this is the way out of a state
     the human is standing in, and it should look like the thing to press. */
  .btn.resume {
    border-color: var(--k-success, #4fb286);
    background: var(--k-success, #4fb286);
    color: var(--k-ground, #0f1116);
    font-weight: 600;
  }
  .paused.warm .btn.resume {
    border-color: var(--k-warning, #d9a441);
    background: var(--k-warning, #d9a441);
  }
  .btn.resume:hover,
  .btn.resume:focus-visible {
    color: var(--k-ground, #0f1116);
    filter: brightness(1.12);
  }
</style>
