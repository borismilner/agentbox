<script>
  // The fullscreen marker (FR74). Four pixels of amber along the top edge of the
  // screen an agent is driving, and nothing else - it exists because the strip
  // itself has to get out of the way of a fullscreen window, and the guarantee
  // may not lapse while it does. There is no text, no button and no state to
  // read: at this size the only thing it can say is "still hands off", and the
  // colour says it.
  //
  // It follows the strip's own vocabulary so the two are read as one thing: the
  // accent while an agent is still asking, amber once it is driving, green while
  // the human has it paused (FR95).
  //
  // That last one is the whole of Boris's answer to "a paused, recording desktop
  // has two things to say in four pixels": the colour carries one of them. There
  // is no second element to add at this size, and the vocabulary was already
  // there - this line has switched to the accent for asking since FR74.
  import { bridge, on } from "../lib/bridge.js";

  let run = $state(null);

  bridge.control().then((st) => (run = st)).catch(() => {});
  on("agentbox:control", (st) => (run = st));

  const paused = $derived(!!run?.paused);
  const asking = $derived(!paused && run?.state === "asking");
</script>

<div class="mark" class:asking class:paused></div>

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
  /* Paused: the desktop is his again, mid-run, and the sign says so in the one
     dimension four pixels have. Every var() carries a fallback - a var() that
     resolves to nothing takes its whole declaration with it, and a marker that
     has silently lost its background is a hands-off sign that is not there. */
  .mark.paused {
    background: var(--k-success, #4fb286);
  }
  /* The breathe stays under the latch, unlike the strip's dot, which stops. They
     say different things: the dot's pulse means work is happening and none is,
     while this fade is what stops four still pixels reading as a seam between two
     windows. A green seam would be no clearer than an amber one. */
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
