<script>
  // The verdict moment (round 7): Understood and Unclear rest in tints of
  // their own meaning and fill on selection; next lights action-blue once a
  // verdict exists - no auto-advance, the reader leaves when they leave.
  // The closing note is multiline, any length, a floor not a cap (round 4);
  // unclear without words gets the keyboard and an amber nudge - the
  // unclear set is the product and must not reach the agent hollow.
  let { kind, mark, isFirst, isLast, noteFocus, onVerdict, onNote, onNav } = $props();

  let ta = $state(null);
  let nudge = $state(false);

  let lastFocus = noteFocus;
  $effect(() => {
    if (noteFocus !== lastFocus) {
      lastFocus = noteFocus;
      nudge = mark.verdict === "unclear" && !mark.note.trim();
      ta?.focus();
    }
  });

  function grow(el) {
    el.style.height = "auto";
    el.style.height = el.scrollHeight + 2 + "px";
  }
</script>

{#if kind === "code"}
  <div class="verdict">
    <textarea
      rows="3"
      class:nudge
      placeholder="What you take from this step, before moving on. Any length. If nothing will come, the step is not done with you."
      value={mark.note}
      bind:this={ta}
      oninput={(e) => {
        onNote(e.target.value);
        grow(e.target);
        if (nudge && e.target.value.trim()) nudge = false;
      }}
      onfocus={(e) => grow(e.target)}
    ></textarea>
    {#if nudge}<div class="nudge-line" data-agentbox-find-exclude>unclear needs words - say what is unclear</div>{/if}
    <div class="row">
      <button
        class="v ok"
        class:on={mark.verdict === "understood"}
        onclick={() => onVerdict("understood")}
      >Understood</button>
      <button
        class="v warn"
        class:on={mark.verdict === "unclear"}
        onclick={() => onVerdict("unclear")}
      >Unclear – needs the agent</button>
      <div class="nav">
        <button disabled={isFirst} onclick={() => onNav(-1)}>← back</button>
        <button disabled={isLast} class:lit={!!mark.verdict} onclick={() => onNav(1)}>next →</button>
      </div>
    </div>
  </div>
{:else}
  <div class="nav bare">
    <button disabled={isFirst} onclick={() => onNav(-1)}>← back</button>
    <button disabled={isLast} onclick={() => onNav(1)}>next →</button>
  </div>
{/if}

<style>
  .verdict {
    margin-top: 32px;
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    background: var(--k-surface);
    padding: 20px;
  }
  textarea {
    width: 100%;
    resize: vertical;
    background: var(--k-surface-2);
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    color: var(--k-ink);
    font-family: var(--k-font-read);
    font-size: 1.15em;
    padding: 7px 10px;
    margin-bottom: 8px;
  }
  textarea::placeholder {
    color: var(--k-ink-3);
  }
  textarea:focus {
    outline: none;
    border-color: color-mix(in srgb, var(--k-info) 55%, transparent);
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--k-info) 20%, transparent);
  }
  textarea.nudge {
    border-color: var(--k-warning);
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--k-warning) 25%, transparent);
  }
  .nudge-line {
    color: var(--k-warning);
    font-size: 0.9em;
    margin: -4px 0 8px;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  button {
    background: transparent;
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    color: var(--k-ink);
    font-size: 0.95em;
    padding: 5px 14px;
    cursor: pointer;
    transition: border-color 140ms ease, background 140ms ease, color 140ms ease;
  }
  button:disabled {
    opacity: 0.4;
    cursor: default;
  }
  button:focus-visible {
    outline: 2px solid var(--k-info);
  }
  .v {
    font-size: 1em;
    padding: 8px 18px;
  }
  .v.ok {
    border-color: color-mix(in srgb, var(--k-success) 30%, transparent);
  }
  .v.ok.on {
    border-color: var(--k-success);
    color: var(--k-success);
    background: color-mix(in srgb, var(--k-success) 9%, transparent);
  }
  .v.warn {
    border-color: color-mix(in srgb, var(--k-warning) 30%, transparent);
  }
  .v.warn.on {
    border-color: var(--k-warning);
    color: var(--k-warning);
    background: color-mix(in srgb, var(--k-warning) 10%, transparent);
  }
  .nav {
    margin-left: auto;
    display: flex;
    gap: 8px;
  }
  .nav.bare {
    margin: 24px 0 0;
    justify-content: flex-start;
  }
  button.lit {
    border-color: var(--k-info);
    color: var(--k-info);
  }
</style>
