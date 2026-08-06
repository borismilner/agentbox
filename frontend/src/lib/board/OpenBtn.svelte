<script>
  // Open this citation in the reader's editor (FR65). The sibling of CopyBtn in
  // a block header, and the reason the header has both: authoring rule 12 wants
  // every code reference to offer copy AND open, because the motion a reader
  // makes most on a file cited across eight steps is leaving the board to find
  // the line by hand.
  //
  // Unlike copying, this can fail for reasons the reader has to act on - no
  // editor configured, a program that is not on PATH - so the failure is words,
  // not a red glyph. It sits beside the button rather than in the board's error
  // banner: that banner REPLACES the review, which is the wrong trade for a
  // click that did not land.
  let { onOpen, title = "open in the editor" } = $props();

  let state = $state("idle"); // idle | done | failed
  let why = $state("");
  let clear;

  async function open() {
    clearTimeout(clear);
    why = "";
    try {
      await onOpen();
      // A cold IDE takes seconds to raise a window, so the click needs an
      // answer of its own; with the IDE already up the tick is the only sign
      // anything happened at all, because the editor takes the focus and the
      // board is behind it by the time it does.
      state = "done";
      clear = setTimeout(() => (state = "idle"), 1600);
    } catch (e) {
      state = "failed";
      why = String(e?.message ?? e).replace(/^Error:\s*/, "");
      clear = setTimeout(() => ((state = "idle"), (why = "")), 12000);
    }
  }
</script>

<button
  onclick={open}
  class:done={state === "done"}
  class:failed={state === "failed"}
  title={state === "failed" ? why : title}
  data-agentbox-find-exclude
>
  {#if state === "done"}
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
      <path d="m5 13 4 4L19 7" />
    </svg>
  {:else}
    <!-- Out of the frame and away: the mark for "this leaves here", which is
         what an editor launch is. Deliberately not a pencil - the board is not
         the thing doing the editing. -->
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
      <path d="M14 4h6v6" />
      <path d="M20 4 12 12" />
      <path d="M18 14v4a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4" />
    </svg>
  {/if}
  <span class="sr">open in the editor</span>
</button>
{#if state === "failed"}
  <span class="why" role="status">{why}</span>
{/if}

<style>
  /* Matched to CopyBtn's icon shape on purpose: three controls in one header
     that differ by glyph and not by size read as one set of actions. */
  button {
    display: inline-flex;
    align-items: center;
    margin-left: 0;
    background: none;
    border: 1px solid var(--k-edge);
    border-radius: 6px;
    color: var(--k-ink-2);
    padding: 3px 6px;
    cursor: pointer;
  }
  button:hover {
    border-color: var(--k-ink-3);
    color: var(--k-ink);
  }
  button svg {
    width: 1.05em;
    height: 1.05em;
  }
  button.done {
    color: var(--k-success);
    border-color: var(--k-success);
  }
  button.failed {
    color: var(--k-warning);
    border-color: var(--k-warning);
  }
  .why {
    font-size: 0.8em;
    color: var(--k-warning);
    text-wrap: pretty;
  }
  .sr {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }
</style>
