<script>
  // The route rail: every station at once, in worklist order of attention -
  // UNREAD stations are the bright ones, understood goes quiet, the current
  // one is accented (round 6, the same inversion the diff card ships).
  //
  // With domains it becomes an accordion, and the reason is length rather than
  // taste: past a dozen steps a flat list is a wall you scan instead of a route
  // you follow. The domain holding the current step is open and lists its
  // stations; the others collapse to one line with their own progress, so the
  // shape of the whole review stays visible while only one part of it is
  // detailed. Exactly one is open, always - the one you are in.
  //
  // An ungrouped review renders exactly as it always did. That is not a
  // fallback, it is the right answer for a five-step walk.
  let { steps, marks, at, go, domains = [] } = $props();

  const grouped = $derived(domains.length > 0);
  const currentDomain = $derived(domains.findIndex((d) => at >= d.from && at <= d.to));

  // Which domain the reader has opened. It follows the current step, but a click
  // on a collapsed domain opens it WITHOUT moving there: looking ahead at what
  // is coming is a different act from going there, and a rail that navigated on
  // every peek would make the review impossible to survey.
  let peek = $state(null);
  const openAt = $derived(peek ?? currentDomain);
  // Following the step means dropping a peek the moment the reader actually
  // moves, or the rail stays open on a domain they have left.
  let lastAt = at;
  $effect(() => {
    if (at !== lastAt) {
      lastAt = at;
      peek = null;
    }
  });

  const glyph = (s) =>
    s.kind === "ground" ? "◇" : s.kind === "none" ? "∅" : s.kind === "check" ? "$" : String(counted(s) + 1);
  const counted = (s) => steps.filter((x) => x.kind === "code").findIndex((x) => x.id === s.id);
  const sub = (s) =>
    s.kind === "ground" ? "does not count" : s.kind === "none" ? "nothing to review" : s.kind === "check" ? "the gate" : "";

  // A domain's own progress, over what counts. The rail says "3 of 5" per group
  // rather than only in the header, because with one domain open the header's
  // total answers a question the reader is no longer asking.
  function done(d) {
    let n = 0;
    for (let i = d.from; i <= d.to; i++) {
      const s = steps[i];
      if (s?.kind === "code" && marks[s.id]?.verdict === "understood") n++;
    }
    return n;
  }
  const unclearIn = (d) => {
    for (let i = d.from; i <= d.to; i++) {
      const s = steps[i];
      if (s?.kind === "code" && marks[s.id]?.verdict === "unclear") return true;
    }
    return false;
  };
</script>

<nav class:grouped>
  {#if grouped}
    {#each domains as d, di (d.id)}
      {@const open = di === openAt}
      {@const n = done(d)}
      <section class="domain" class:open class:here={di === currentDomain}>
        <button
          class="dhead"
          onclick={() => (peek = open && di !== currentDomain ? null : di)}
          aria-expanded={open}
          title={open ? d.blurb || d.title : "look at " + d.title}
        >
          <span class="dmark" aria-hidden="true">{open ? "▾" : "▸"}</span>
          <span class="dname">{d.title}</span>
          <span class="dcount" class:full={n === d.counted && d.counted > 0} class:warn={unclearIn(d)}>
            {n}/{d.counted}
          </span>
        </button>

        <!-- The opening. Height is animated rather than display toggled, which
             is what makes it read as a drawer instead of a repaint; the blurb
             fades in behind it so the two do not arrive at once. -->
        <div class="drawer">
          <div class="dinner">
            {#if d.blurb}<p class="dblurb">{d.blurb}</p>{/if}
            {#each steps.slice(d.from, d.to + 1) as s, k (s.id)}
              {@render station(s, d.from + k, d.from + k === d.to)}
            {/each}
          </div>
        </div>
      </section>
    {/each}
  {:else}
    {#each steps as s, i (s.id)}
      {@render station(s, i, i === steps.length - 1)}
    {/each}
  {/if}
</nav>

{#snippet station(s, i, last)}
  {@const v = marks[s.id]?.verdict ?? ""}
  <button class="station" class:current={i === at} onclick={() => go(i)}>
    <div class="spine">
      <div class="circle" class:ok={v === "understood"} class:warn={v === "unclear"} class:muted={s.kind !== "code"}>
        {v === "understood" && s.kind === "code" ? "✓" : glyph(s)}
      </div>
      {#if !last}<div class="wire"></div>{/if}
    </div>
    <div class="label">
      <div class="name" class:quiet={v === "understood"}>{s.title}</div>
      {#if s.kind !== "code"}<div class="sub">{sub(s)}</div>{/if}
    </div>
  </button>
{/snippet}

<style>
  nav {
    width: 20em;
    flex: none;
    overflow-y: auto;
    border-right: 1px solid var(--k-edge);
    padding: 16px 20px;
    display: flex;
    flex-direction: column;
  }
  nav.grouped {
    padding: 12px 16px;
    gap: 2px;
  }
  .station {
    display: flex;
    gap: 12px;
    width: 100%;
    text-align: left;
    background: none;
    border: 0;
    margin: 0 -6px;
    padding: 2px 6px;
    border-radius: 8px;
    cursor: pointer;
    color: inherit;
    font: inherit;
    transition: background 120ms ease;
  }
  .station:hover {
    background: color-mix(in srgb, var(--k-ink) 5%, transparent);
  }
  .station:focus-visible {
    outline: 2px solid var(--k-info);
  }
  .spine {
    display: flex;
    flex-direction: column;
    align-items: center;
  }
  .circle {
    width: 2em;
    height: 2em;
    flex: none;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    border: 1px solid var(--k-edge);
    font-family: var(--k-font-mono);
    font-size: 0.95em;
    color: var(--k-ink);
  }
  .current .circle {
    border-color: var(--k-info);
    background: color-mix(in srgb, var(--k-info) 12%, transparent);
    font-weight: 700;
  }
  .circle.ok {
    border-color: var(--k-success);
    color: var(--k-success);
  }
  .circle.warn {
    border-color: var(--k-warning);
    color: var(--k-warning);
    background: color-mix(in srgb, var(--k-warning) 14%, transparent);
  }
  .circle.muted {
    color: var(--k-ink-2);
  }
  .wire {
    width: 1px;
    flex: 1;
    min-height: 20px;
    background: var(--k-edge);
  }
  .label {
    padding: 0.35em 0 1em;
    min-width: 0;
  }
  .name {
    font-size: 1.02em;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .current .name {
    font-weight: 700;
  }
  .name.quiet {
    color: var(--k-ink-2);
  }
  .sub {
    font-size: 0.82em;
    color: var(--k-ink-3);
  }

  /* --- domains ---------------------------------------------------------- */

  .domain {
    border-radius: 10px;
    /* The closed rows sit flat and the open one lifts, so the eye finds where
       it is without reading a single word. */
    transition: background 200ms ease;
  }
  .domain.open {
    background: color-mix(in srgb, var(--k-ink) 4%, transparent);
    padding-bottom: 4px;
  }
  .dhead {
    display: flex;
    align-items: baseline;
    gap: 8px;
    width: 100%;
    padding: 9px 10px;
    background: none;
    border: 0;
    border-radius: 10px;
    cursor: pointer;
    color: var(--k-ink-2);
    font: inherit;
    text-align: left;
    transition: color 160ms ease;
  }
  .dhead:hover {
    color: var(--k-ink);
  }
  .dhead:focus-visible {
    outline: 2px solid var(--k-info);
  }
  .domain.open .dhead {
    color: var(--k-ink);
    font-weight: 600;
  }
  /* The domain the reader is actually IN, as opposed to one they opened to look
     ahead at. Without this the two are indistinguishable and the rail stops
     answering "where am I". */
  .domain.here .dhead {
    color: var(--k-info);
  }
  .dmark {
    flex: none;
    width: 0.9em;
    font-size: 0.8em;
    color: var(--k-ink-3);
    transition: transform 200ms ease;
  }
  .dname {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .dcount {
    flex: none;
    font-family: var(--k-font-mono);
    font-size: 0.76em;
    color: var(--k-ink-3);
  }
  .dcount.full {
    color: var(--k-success);
  }
  .dcount.warn {
    color: var(--k-warning);
  }

  /* grid-template-rows 0fr -> 1fr is the height animation that does not need a
     measured pixel height, so a domain whose steps wrap to two lines opens just
     as smoothly as one whose steps do not. The inner element must own the
     overflow, or the rows collapse without hiding anything. */
  .drawer {
    display: grid;
    grid-template-rows: 0fr;
    transition:
      grid-template-rows 260ms cubic-bezier(0.22, 0.61, 0.36, 1),
      opacity 180ms ease;
    opacity: 0;
  }
  .domain.open .drawer {
    grid-template-rows: 1fr;
    opacity: 1;
  }
  .dinner {
    overflow: hidden;
    padding: 0 10px;
  }
  .domain.open .dinner {
    padding-top: 2px;
  }
  /* The line the domain opens with, and the reason a domain has a blurb at all:
     it is the moment the reader decides whether to pay attention. It arrives
     after the drawer rather than with it, so the opening reads as one movement
     and then a sentence. */
  .dblurb {
    margin: 0 0 10px;
    font-size: 0.86em;
    line-height: 1.5;
    color: var(--k-ink-2);
    text-wrap: pretty;
    opacity: 0;
    transition: opacity 200ms ease 120ms;
  }
  .domain.open .dblurb {
    opacity: 1;
  }
  /* Whoever asked for less motion gets none of this: the drawer still opens and
     closes, it just stops being a movement. theme.motion lands on the root as a
     data attribute (lib/tokens.js), so it is read here rather than threaded
     through as a prop that every caller would have to remember to pass. */
  :global([data-motion="reduced"]) .drawer,
  :global([data-motion="none"]) .drawer,
  :global([data-motion="reduced"]) .dblurb,
  :global([data-motion="none"]) .dblurb,
  :global([data-motion="reduced"]) .domain,
  :global([data-motion="none"]) .domain {
    transition: none;
  }
</style>
