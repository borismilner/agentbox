<script>
  // The comment composer. It opens where the eyes already are - just under
  // the selection, inside the code panel - or inline for a step-level
  // remark. Ctrl+Enter adds; Esc discards, written words included (round 5,
  // the owner's call: his Esc means "off my screen"; the Discard button
  // stays for the mouse).
  import { tick } from "svelte";

  let { anchorLabel, top = null, pend = $bindable(), onAdd } = $props();

  let box = $state(null);
  $effect(() => {
    if (box) tick().then(() => box?.scrollIntoView({ block: "nearest" }));
  });

  function add() {
    const body = pend.draft.trim();
    if (body) onAdd(body);
    pend = null;
  }
</script>

<div class="composer" class:floating={top !== null} style={top !== null ? `top: ${top}px` : ""} bind:this={box}>
  <div class="anchor" data-agentbox-find-exclude>{anchorLabel}</div>
  <!-- svelte-ignore a11y_autofocus -->
  <textarea
    rows="2"
    autofocus
    placeholder="Say it here. Ctrl+Enter adds it, Esc discards."
    bind:value={pend.draft}
    onkeydown={(e) => {
      if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
        e.preventDefault();
        add();
      }
      if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        pend = null;
      }
    }}
  ></textarea>
  <div class="row">
    <button onclick={add}>Add comment</button>
    <button onclick={() => (pend = null)}>Discard</button>
  </div>
</div>

<style>
  .composer {
    border: 1px solid var(--k-accent);
    border-radius: 8px;
    background: var(--k-surface);
    padding: 8px;
    margin: 8px 0;
    box-shadow: 0 8px 28px rgb(0 0 0 / 0.35);
  }
  .composer.floating {
    position: absolute;
    left: 64px;
    width: min(40em, 80%);
    z-index: 5;
    margin: 0;
  }
  .anchor {
    font-family: var(--k-font-mono);
    font-size: 0.85em;
    color: var(--k-accent);
    margin-bottom: 4px;
  }
  textarea {
    width: 100%;
    resize: vertical;
    background: var(--k-surface-2);
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    color: var(--k-ink);
    font-family: var(--k-font-read);
    font-size: 1.05em;
    padding: 7px 10px;
  }
  textarea:focus {
    outline: none;
    border-color: color-mix(in srgb, var(--k-info) 55%, transparent);
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--k-info) 20%, transparent);
  }
  .row {
    display: flex;
    gap: 8px;
    margin-top: 6px;
  }
  button {
    background: transparent;
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    color: var(--k-ink);
    font-size: 0.85em;
    padding: 4px 12px;
    cursor: pointer;
  }
  button:hover {
    border-color: var(--k-ink-3);
  }
</style>
