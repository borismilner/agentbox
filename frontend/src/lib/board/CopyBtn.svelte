<script>
  // One copy control: the daemon's clipboard (the webview refuses the async
  // one), confirming inline - no toast, no focus theft (FR56).
  //
  // `icon` turns it into a glyph with the label as its tooltip: in a block
  // header the words "copy" and "copy anchor" cost more width than they
  // carry, and the header is competing with the path for room. Rows that use
  // the label as content (the path itself) leave icon off.
  //
  // `shape` picks which glyph. Two icon buttons sit side by side in a block
  // header, and copying the lines is not copying a reference to them: drawn as
  // the same sheets-of-paper mark, the only thing telling them apart was a
  // tooltip you had to hover to read.
  import { bridge } from "../bridge.js";
  let { text, label = "copy", icon = false, shape = "copy", title = "" } = $props();
  let done = $state(false);
  function copy() {
    bridge.copyText(typeof text === "function" ? text() : text).catch(() => {});
    done = true;
    setTimeout(() => (done = false), 1200);
  }
</script>

<button onclick={copy} class:icon class:done title={title || label} data-agentbox-find-exclude>
  {#if icon}
    {#if done}
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <path d="m5 13 4 4L19 7" />
      </svg>
    {:else if shape === "link"}
      <!-- A reference to the lines, not the lines: the chain is the mark every
           other tool uses for "copy a link to this". -->
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <path d="M10 14 14 10" />
        <path d="M13.5 7.5 15 6a3.5 3.5 0 0 1 5 5l-1.5 1.5" />
        <path d="M10.5 16.5 9 18a3.5 3.5 0 0 1-5-5l1.5-1.5" />
      </svg>
    {:else}
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <rect x="9" y="9" width="11" height="11" rx="2" />
        <path d="M5 15V6a2 2 0 0 1 2-2h8" />
      </svg>
    {/if}
    <span class="sr">{label}</span>
  {:else}{done ? "copied ✓" : label}{/if}
</button>

<style>
  button {
    margin-left: auto;
    background: none;
    border: 1px solid var(--k-edge);
    border-radius: 6px;
    color: var(--k-ink-2);
    font-size: 0.85em;
    padding: 2px 10px;
    cursor: pointer;
  }
  button:hover {
    border-color: var(--k-ink-3);
    color: var(--k-ink);
  }
  button.icon {
    display: inline-flex;
    align-items: center;
    padding: 3px 6px;
  }
  button.icon svg {
    width: 1.05em;
    height: 1.05em;
  }
  button.icon.done {
    color: var(--k-success);
    border-color: var(--k-success);
  }
  /* The word stays for anything reading the page rather than looking at it. */
  .sr {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }
</style>
