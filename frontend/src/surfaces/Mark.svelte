<script>
  // The fullscreen marker (FR74). Four pixels of amber along the top edge of the
  // screen an agent is driving, and nothing else - it exists because the strip
  // itself has to get out of the way of a fullscreen window, and the guarantee
  // may not lapse while it does. There is no text, no button and no state to
  // read: at this size the only thing it can say is "still hands off", and the
  // colour says it.
  //
  // It follows the strip's own vocabulary so the two are read as one thing: the
  // accent while an agent is still asking, amber once it is driving.
  import { bridge, on } from "../lib/bridge.js";

  let run = $state(null);

  bridge.control().then((st) => (run = st)).catch(() => {});
  on("agentbox:control", (st) => (run = st));

  const asking = $derived(run?.state === "asking");
</script>

<div class="mark" class:asking></div>

<style>
  /* The window IS the line, so the bar fills it: nothing here may rely on the
     window being taller than the paint, because it is exactly as tall. */
  .mark {
    width: 100vw;
    height: 100vh;
    background: var(--k-warning);
    animation: breathe 2.6s ease-in-out infinite;
  }
  .mark.asking {
    background: var(--k-accent);
  }
  /* A still line at this height could be a seam between two windows. The slow
     fade is what makes it read as something drawn on purpose - and it is slow
     enough not to pull the eye off whatever is playing underneath. */
  @keyframes breathe {
    50% {
      opacity: 0.45;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .mark {
      animation: none;
    }
  }
</style>
