<script>
  import IdentityPill from "../lib/IdentityPill.svelte";
  import SeverityIcon from "../lib/SeverityIcon.svelte";
  import { bridge, on } from "../lib/bridge.js";
  import { markdown } from "../lib/markdown.svelte.js";
  import { ticker, remaining } from "../lib/clock.svelte.js";
  import { trouble, note, forget } from "../lib/trouble.svelte.js";

  // The toast (03-ui-ux.md): the rendering of a notify. Nobody has to answer it,
  // so it is a strip at the top of the screen rather than a card in the middle -
  // one glance, and the severity is legible before the words are.
  //
  // Which items land here, the icon, and whether it counts down or waits are all
  // decided in Go; this paints, times the countdown, and takes the click.

  let view = $state(null);
  let expanded = $state(false);
  let clamped = $state(false);
  let body = $state(null); // the body element, measured for the "more" affordance

  const clock = ticker();

  const item = $derived(view?.item ?? null);
  const level = $derived(item?.level || "info");
  const severity = $derived(
    { info: "var(--k-info)", success: "var(--k-success)", warning: "var(--k-warning)", error: "var(--k-error)", urgent: "var(--k-urgent)" }[level] ??
      "var(--k-info)",
  );
  // "6s" while it runs, "closing" once the deadline is up - the daemon owns the
  // timer that actually takes the strip away, and a countdown parked on "0s"
  // would read as a stuck window rather than one on its way out.
  const closesIn = $derived(remaining(view?.dismissAtMs, clock.now));
  const clockText = $derived(!view?.dismissAtMs ? "" : closesIn === "0s" ? "closing" : closesIn);
  const actions = $derived(view?.actionsEnabled ? (item?.actions ?? []) : []);

  // U-01, the toast's share of it. A dismiss that did not land left the strip on
  // screen looking exactly like one that had not been clicked yet, so the human
  // clicked again. The listeners are the card's, for the same reason.
  window.addEventListener("error", (e) => note("toast error: " + e.message));
  window.addEventListener("unhandledrejection", (e) => note("toast error: " + e.reason));

  // R-05, the same pull the card has: the pushed view can arrive before this
  // bundle mounts, and nothing buffers it or re-sends it. See Card.svelte for
  // the whole reasoning; a toast that misses its push is a strip with no text.
  function applyView(v) {
    const fresh = v?.item?.id !== item?.id;
    view = v;
    if (fresh) {
      expanded = false;
      forget();
    }
  }

  on("agentbox:view", applyView);

  bridge.ready("toast");

  bridge
    .view()
    .then((v) => {
      if (!view && v?.item) applyView(v);
    })
    .catch(() => {});

  // A strip has to be exactly as tall as what is in it, and it changes height
  // twice: once when the body lands, once if it is expanded. Measure and let Go
  // resize, same as the card.
  let shell = $state(null);
  $effect(() => {
    if (!shell) return;
    const report = () => bridge.fit(Math.ceil(shell.getBoundingClientRect().height));
    const ro = new ResizeObserver(report);
    ro.observe(shell);
    report();
    return () => ro.disconnect();
  });

  // "click to expand" only earns its place when there is something hidden: three
  // lines is the clamp (03-ui-ux.md), so compare the full height against it.
  $effect(() => {
    if (!body) return;
    clamped = !expanded && body.scrollHeight - body.clientHeight > 4;
  });

  function dismiss() {
    if (item) bridge.dismiss(item.id);
  }

  // The whole strip is one target, so a click reads as "I have seen it" - except
  // when there is more to read, where the first click opens it. Buttons stop the
  // click themselves; a notice with actions must not dismiss under the fingers of
  // someone reaching for one.
  function onClick() {
    if (clamped && !expanded) {
      expanded = true;
      return;
    }
    dismiss();
  }

  function onKey(e) {
    if (!item) return;
    if (e.key === "Escape" || e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      dismiss();
      return;
    }
    if (e.key === "c" && !e.ctrlKey) {
      e.preventDefault();
      bridge.copy(item.id);
    }
  }
</script>

<svelte:window on:keydown={onKey} />

{#if item}
  <!-- The strip is a click target, not a control: the keyboard path is the window
       handler above, because a toast maps without taking focus and cannot rely on
       having any. -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="toast"
    style="--sev: {severity}"
    bind:this={shell}
    onclick={onClick}
    title={clamped ? "click to expand" : "click to dismiss"}
  >
    <span class="rail"></span>
    <SeverityIcon glyph={view.glyph} size={20} />

    <div class="txt">
      <div class="top">
        <span class="title">{item.title}</span>
        <span class="spacer"></span>
        <IdentityPill agent={item.identity?.agent} project={item.identity?.project} session={item.identity?.session} compact />
        {#if view.sticky}
          <button
            class="x"
            title="dismiss"
            onclick={(e) => {
              e.stopPropagation();
              dismiss();
            }}>&#x2715;</button
          >
        {:else if clockText}
          <span class="clock">{clockText}</span>
        {/if}
      </div>

      {#if view.bodyHtml}
        <div class="body k-md selectable" class:open={expanded} bind:this={body} use:markdown={view.bodyHtml}>{@html view.bodyHtml}</div>
      {/if}

      {#if clamped}
        <span class="more">click to expand</span>
      {/if}

      <!-- U-01: what the last click actually did. It is inside the strip's own
           text column so the notice grows the window rather than sitting over
           the words it is about. -->
      {#if trouble.text}
        <div class="trouble" role="alert"><span class="bang">!</span>{trouble.text}</div>
      {/if}

      {#if actions.length}
        <div class="acts">
          {#each actions as a, i}
            <button
              class="act"
              title={a.exec}
              onclick={(e) => {
                e.stopPropagation();
                bridge.runAction(item.id, i);
              }}>{a.label}</button
            >
          {/each}
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .toast {
    position: relative;
    display: flex;
    align-items: flex-start;
    gap: 11px;
    padding: 12px 13px 12px 17px;
    background: var(--k-surface);
    border: 1px solid color-mix(in srgb, var(--sev) 34%, var(--k-edge));
    border-radius: var(--k-radius);
    /* The tint is what makes a toast read as its severity from across the room,
     * without the strip becoming a coloured block you cannot read text on. */
    background-image: linear-gradient(to right, color-mix(in srgb, var(--sev) 11%, transparent), transparent 62%);
    box-shadow: var(--k-shadow);
    text-align: left;
    cursor: pointer;
    animation: drop 120ms cubic-bezier(0.2, 0.9, 0.3, 1);
  }
  /* Shorter than a card's motion (03-ui-ux.md): 120ms in. */
  @keyframes drop {
    from {
      opacity: 0;
      transform: translateY(-7px);
    }
  }
  .toast:hover {
    border-color: color-mix(in srgb, var(--sev) 52%, var(--k-edge));
  }

  .rail {
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 3px;
    background: var(--sev);
  }

  .txt {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 5px;
  }
  .top {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .spacer {
    flex: 1;
  }
  /* The title always shows whole (03-ui-ux.md) - it wraps rather than clipping. */
  .title {
    font-size: 0.94rem;
    font-weight: 650;
    line-height: 1.3;
    letter-spacing: -0.005em;
  }
  .clock,
  .more {
    font-family: var(--k-font-mono);
    font-size: 0.62rem;
    color: var(--k-ink-3);
    white-space: nowrap;
  }
  .x {
    width: 20px;
    height: 20px;
    flex: 0 0 auto;
    border-radius: 5px;
    color: var(--k-ink-3);
    font-size: 0.62rem;
  }
  .x:hover {
    background: var(--k-surface-3);
    color: var(--k-ink);
  }

  .body {
    font-family: var(--k-font-read);
    font-size: 0.84rem;
    line-height: 1.5;
    color: var(--k-ink-2);
    overflow: hidden;
    overflow-wrap: anywhere;
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 3;
  }
  /* A px cap, not vh: the window is sized FROM this element, so a viewport-
   * relative height feeds back on itself - each measurement shrinks the window,
   * which shrinks the allowance, and an expanded body ends up smaller than the
   * clamped one it replaced. */
  .body.open {
    display: block;
    max-height: 210px;
    overflow-y: auto;
    -webkit-line-clamp: unset;
  }
  /* Styling comes from .k-md (app.css); a strip only tightens it. */
  .body :global(p),
  .body :global(ul),
  .body :global(ol) {
    margin-bottom: 0.5em;
  }
  .body :global(.k-code) {
    margin-bottom: 0.5em;
  }

  .acts {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 2px;
  }
  .act {
    padding: 4px 10px;
    border: 1px solid var(--k-edge);
    border-radius: calc(var(--k-radius) * 0.65);
    background: var(--k-surface-2);
    font-size: 0.8rem;
  }
  .act:hover {
    background: var(--k-surface-3);
  }

  /* U-01's line, same chroma as the card's so the two surfaces say a failure the
     same way. Fallbacks on every var(): one that resolves to nothing takes its
     whole declaration with it. */
  .trouble {
    display: flex;
    gap: 6px;
    margin-top: 4px;
    padding: 4px 7px;
    border-radius: 5px;
    background: color-mix(in srgb, var(--k-warning, #d9a441) 14%, transparent);
    color: var(--k-ink-1, #e8ecf3);
    font-size: 0.76rem;
    line-height: 1.35;
  }
  .trouble .bang {
    font-weight: 700;
    color: var(--k-warning, #d9a441);
  }
</style>
