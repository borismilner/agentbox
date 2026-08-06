<script>
  import SeverityIcon from "./SeverityIcon.svelte";
  import { bridge } from "./bridge.js";
  import { markdown } from "./markdown.svelte.js";
  import { ticker, remaining } from "./clock.svelte.js";

  // The inline ask panel (FR49): the agent whose conversation this is has asked
  // something, and it is answered here rather than in a card over the window.
  //
  // Two rules shape it. It sits where the answer goes - directly above the
  // composer, so the question and the reply are in the same place. And it never
  // takes the keyboard: the composer keeps focus, keys act only when nothing is
  // being typed, and the hint says which of those you are in. A panel that
  // grabbed the keystroke mid-prompt could answer a production question with a
  // stray "1", which is the accident the whole focus policy exists to prevent.
  //
  // What a key means, which control is the default and what the panel calls
  // itself are all decided in Go (inline.go). This paints and forwards.

  let { ask } = $props();

  const clock = ticker();
  let typing = $state(false);

  const severity = $derived(
    { info: "var(--k-info)", success: "var(--k-success)", warning: "var(--k-warning)", error: "var(--k-error)", urgent: "var(--k-urgent)" }[ask.level] ??
      "var(--k-info)",
  );
  const expiresIn = $derived(remaining(ask.expiresAtMs, clock.now));

  function act(opt) {
    if (opt.verb === "dismiss") return bridge.dismiss(ask.id);
    bridge.answer(ask.id, opt.answer);
  }

  // The keys Go may act on. Listed only so the surface knows when to swallow the
  // keystroke - the meaning stays on the Go side, shared with inbox triage.
  const KEYS = new Set(["1", "2", "3", "4", "5", "6", "7", "8", "9", "y", "n", "d", "Enter", "Backspace"]);

  function onKey(e) {
    const inField = e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement;
    if (inField || e.ctrlKey || e.metaKey || e.altKey) return;
    if (e.key === "c") {
      e.preventDefault();
      bridge.copy(ask.id);
      return;
    }
    if (KEYS.has(e.key)) {
      e.preventDefault();
      bridge.askKey(ask.id, e.key);
    }
  }

  // Whether a field has the keyboard decides which affordance is honest, so it is
  // tracked rather than guessed: the keys are live only when it does not.
  function onFocus(e) {
    typing = e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement;
  }
</script>

<svelte:window onkeydown={onKey} onfocusin={onFocus} />

<div class="ask" style="--sev: {severity}; --hue: {ask.hue}">
  <span class="rail"></span>

  <div class="head">
    <SeverityIcon glyph={ask.glyph} size={15} />
    <span class="lead">{ask.lead}</span>
    <span class="spacer"></span>
    {#if ask.waiting}<span class="meta">{ask.waiting} more waiting</span>{/if}
    {#if expiresIn}<span class="meta">expires in {expiresIn}</span>{/if}
  </div>

  <h2 class="selectable">{ask.title}</h2>

  {#if ask.bodyHtml}
    <div class="body k-md selectable" use:markdown={ask.bodyHtml}>{@html ask.bodyHtml}</div>
  {/if}

  <div class="controls">
    {#each ask.options as opt}
      <button class="opt" class:primary={opt.primary} onclick={() => act(opt)}>
        {#if opt.key}<kbd>{opt.key}</kbd>{/if}
        <span class="lbl">
          {opt.label}
          {#if opt.desc}<em>{opt.desc}</em>{/if}
        </span>
      </button>
    {/each}

    <!-- FR32 action buttons ride along on a notice, by index: the surface never
         sees a command it could run, only the one it shows on hover. -->
    {#each ask.actions ?? [] as a}
      <button class="opt act" title={a.exec} onclick={() => bridge.runAction(ask.id, a.index)}>
        <span class="lbl">{a.label}</span>
      </button>
    {/each}
  </div>

  <div class="hint">
    {#if typing}
      the buttons answer while you are typing
    {:else}
      {ask.hint}
    {/if}
  </div>
</div>

<style>
  .ask {
    position: relative;
    padding: 11px 14px 9px 16px;
    border: 1px solid color-mix(in srgb, var(--hue) 40%, var(--k-edge));
    border-radius: var(--k-radius);
    background: color-mix(in srgb, var(--hue) 7%, var(--k-surface-2));
    display: flex;
    flex-direction: column;
    gap: 7px;
    animation: raise 160ms cubic-bezier(0.2, 0.9, 0.3, 1);
    /* Shrinkable, so a long question takes its space out of its own body rather
     * than out of the composer below it. Everything except the body keeps its
     * size, which is what makes the controls reachable at any window height. */
    min-height: 0;
  }
  @keyframes raise {
    from {
      opacity: 0;
      transform: translateY(8px);
    }
  }

  /* The same severity rail the card and the toast wear, so a warning reads as a
   * warning wherever it lands. */
  .rail {
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 3px;
    border-radius: var(--k-radius) 0 0 var(--k-radius);
    background: var(--sev);
  }

  .head {
    display: flex;
    align-items: center;
    gap: 7px;
  }
  .lead {
    font-family: var(--k-font-mono);
    font-size: 0.62rem;
    letter-spacing: 0.11em;
    text-transform: uppercase;
    color: var(--k-ink-3);
  }
  .spacer {
    flex: 1;
  }
  .meta {
    font-family: var(--k-font-mono);
    font-size: 0.64rem;
    color: var(--k-ink-3);
  }

  h2 {
    margin: 0;
    font-size: 0.98rem;
    font-weight: 700;
    letter-spacing: -0.01em;
    line-height: 1.3;
  }

  /* The only part of the panel that gives. 260px is the cap when there is room;
   * `flex: 0 1 auto` with min-height: 0 lets it go under that when there is not,
   * so the head, the title, the controls and the hint are never what gets cut.
   * This is why the panel does not need the card's FR84 fold: the card folds
   * because a fixed-height window pushes its fields out of reach, and here the
   * body is bounded instead - nothing is ever hidden from the reader, it just
   * scrolls where it sits. */
  .body {
    font-family: var(--k-font-read);
    font-size: 0.9rem;
    line-height: 1.55;
    color: var(--k-ink-2);
    flex: 0 1 auto;
    min-height: 0;
    max-height: 260px;
    overflow-y: auto;
    overflow-wrap: anywhere;
  }
  .body :global(p),
  .body :global(ul),
  .body :global(ol) {
    margin-bottom: 0.55em;
  }
  .body :global(p:last-child) {
    margin-bottom: 0;
  }

  .controls {
    display: flex;
    flex-wrap: wrap;
    gap: 7px;
    margin-top: 1px;
  }
  .opt {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 5px 11px 5px 7px;
    border: 1px solid var(--k-edge);
    border-radius: calc(var(--k-radius) * 0.72);
    background: var(--k-surface);
    font-size: 0.85rem;
    transition: transform 90ms ease, border-color 90ms ease, background 90ms ease;
  }
  .opt:hover {
    background: var(--k-surface-3);
    border-color: color-mix(in srgb, var(--k-ink-3) 45%, var(--k-edge));
    transform: translateY(-1px);
  }
  .opt:active {
    transform: translateY(0);
  }
  .opt.primary {
    border-color: color-mix(in srgb, var(--k-accent) 55%, transparent);
    background: color-mix(in srgb, var(--k-accent) 12%, var(--k-surface));
  }
  .opt.act {
    padding-left: 11px;
    font-size: 0.8rem;
    color: var(--k-ink-2);
  }
  .opt .lbl {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    line-height: 1.25;
  }
  .opt em {
    font-style: normal;
    font-size: 0.72rem;
    color: var(--k-ink-3);
  }
  kbd {
    font-family: var(--k-font-mono);
    font-size: 0.64rem;
    line-height: 1.35;
    padding: 1px 5px;
    border-radius: 4px;
    background: var(--k-surface-3);
    border: 1px solid var(--k-edge);
    color: var(--k-ink-3);
  }

  .hint {
    font-family: var(--k-font-mono);
    font-size: 0.62rem;
    color: var(--k-ink-3);
  }
</style>
