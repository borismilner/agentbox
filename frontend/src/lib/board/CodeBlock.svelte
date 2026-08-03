<script>
  // One code block: the daemon's per-line chroma HTML laid out into rows
  // this component owns. Structure rules paid for in earlier rounds:
  // border and radius live on the FRAME, scrolling on the inner scroller
  // (WebKitGTK paints a rounded scroll container's scrollbar over its own
  // border - the FR62 lesson); the hover comment peek renders BELOW the
  // scroller so hovering never shifts the lines under the pointer (round
  // 5); selection anchors walk data-ln, which only new-side rows carry, so
  // a comment's anchor is always a real new-file range.
  import Composer from "./Composer.svelte";
  import CopyBtn from "./CopyBtn.svelte";

  let { blk, root, stepId, comments, pend = $bindable(), blockIndex, lit = null, onComment, last = false } = $props();

  let scroller = $state(null);
  let hovCmt = $state(null); // {key, list} - only replaced when the SET changes
  let hovNote = $state(0); // note number under the pointer, 0 for none

  // Notes by the line they start on, so the gutter can carry a badge and the
  // margin can put the text beside the code it explains. Only new-side notes
  // are placed: an old-side note has no row of its own to sit against.
  const noteAt = $derived.by(() => {
    const m = new Map();
    for (const n of blk.notes ?? []) {
      if (n.side === "old") continue;
      m.set(n.from, [...(m.get(n.from) ?? []), n]);
    }
    return m;
  });

  const inLit = (n) => lit != null && n >= lit.from && n <= lit.to;

  const dels = $derived.by(() => {
    const m = new Map();
    for (const d of blk.dels ?? []) {
      m.set(d.after, [...(m.get(d.after) ?? []), d]);
    }
    return m;
  });

  function lnOf(node) {
    for (let el = node instanceof Element ? node : node.parentElement; el; el = el.parentElement) {
      if (el.dataset?.ln) return +el.dataset.ln;
    }
    return null;
  }

  function onMouseUp() {
    const s = window.getSelection();
    if (!s || s.isCollapsed || !scroller) return;
    let a = lnOf(s.anchorNode), b = lnOf(s.focusNode);
    if (a == null && b == null) return;
    if (a == null) a = b;
    if (b == null) b = a;
    const from = Math.min(a, b), to = Math.max(a, b);
    const row = scroller.querySelector(`[data-ln="${to}"]`);
    // The exact text comes from the selected rows' code, not from
    // selection.toString(): WebKitGTK includes user-select:none gutters in
    // toString(), which polluted the first field-tested anchor with line
    // numbers. The anchor means "these lines as they read", so take them.
    const exact = [...scroller.querySelectorAll(".row:not(.del)")]
      .filter((r) => +r.dataset.ln >= from && +r.dataset.ln <= to)
      .map((r) => r.querySelector(".txt")?.textContent ?? "")
      .join("\n")
      .trim()
      .slice(0, 400);
    pend = {
      blockIndex,
      path: blk.path,
      side: "new",
      from, to,
      exact,
      draft: "",
      top: row ? row.offsetTop + row.offsetHeight + 4 : 0,
    };
  }

  function onRowEnter(n) {
    const onLine = comments.filter((c) => n >= c.from && n <= c.to);
    if (!onLine.length) {
      if (hovCmt) hovCmt = null;
      return;
    }
    const key = onLine.map((c) => c.id).join(",");
    if (!hovCmt || hovCmt.key !== key) hovCmt = { key, list: onLine };
  }

  const commented = (n) => comments.some((c) => n >= c.from && n <= c.to);
</script>

<div class="frame" onmouseleave={() => (hovCmt = null)}>
  <div class="head">
    {#if blk.path}
      <CopyBtn text={root + "/" + blk.path} label={blk.path} />
      {#if !blk.err && blk.lines?.length}
        <span class="range mono">{blk.start}–{blk.start + blk.lines.length - 1}</span>
        {#if blk.new}<span class="new-chip">new</span>{/if}
        <!-- Which of the two sources these lines came from. The reader has to be
             able to ask it without leaving the block: when a note stops matching
             its code, "am I looking at what the author read, or at the file as it
             is now" is the first question, and only one of those two can drift. -->
        {#if blk.pinned}
          <span class="src-chip" title="rendered from the source captured when this review was written">captured</span>
        {:else}
          <span class="src-chip live" title="no capture was stored for this block, so it is read from the file as it is now - it may have changed since the review was written">live file</span>
        {/if}
        <span class="spacer"></span>
        <CopyBtn
          text={blk.path + ":" + blk.start + "-" + (blk.start + blk.lines.length - 1)}
          label="copy anchor"
          title="copy path:lines"
          shape="link"
          icon
        />
      {:else}
        <span class="spacer"></span>
      {/if}
    {:else}
      <span class="label">{blk.label || "snippet"}</span>
      <span class="spacer"></span>
    {/if}
    <CopyBtn
      text={() =>
        scroller
          ? [...scroller.querySelectorAll(".row:not(.del) .txt")].map((e) => e.textContent).join("\n")
          : ""}
      label="copy"
      title="copy this block"
      icon
    />
  </div>

  {#if blk.err}
    <div class="err">{blk.err}</div>
  {:else}
    <div class="scroller chroma mono" bind:this={scroller} onmouseup={onMouseUp} role="presentation">
      <!-- rows sits inside the scroller and is as wide as its widest line, so a
           row's background covers the whole line instead of stopping at the
           visible edge while the text scrolls on past it (FR67). -->
      <div class="rows">
      {#each blk.lines as l (l.n)}
        <div
          class="row"
          class:add={l.add}
          class:cmt={commented(l.n)}
          class:lit={inLit(l.n)}
          data-ln={l.n}
          onmouseenter={() => onRowEnter(l.n)}
          role="presentation"
        >
          <span class="no">{l.n}</span>
          <!-- The badge gets a column of its own so an annotated line's number
               sits in exactly the same place as an unannotated one's. -->
          <span class="mark">
            {#each noteAt.get(l.n) ?? [] as n (n.num)}
              <button
                class="badge"
                class:on={hovNote === n.num}
                onmouseenter={() => (hovNote = n.num)}
                onmouseleave={() => (hovNote = 0)}
                onclick={() => (hovNote = hovNote === n.num ? 0 : n.num)}
                title={n.text}
              >{n.num}</button>
            {/each}
          </span>
          <span class="txt">{@html l.html}</span>
        </div>
        {#each dels.get(l.n) ?? [] as d, di (di)}
          {#each d.lines as dl, dj (dj)}
            <div class="row del">
              <span class="no">{dl.n}</span>
              <span class="mark"></span>
              <span class="txt">{@html dl.html}</span>
            </div>
          {/each}
        {/each}
      {/each}
      </div>
      {#if pend && pend.blockIndex === blockIndex && !pend.stepLevel}
        <Composer
          anchorLabel={":" + pend.from + (pend.to !== pend.from ? "–" + pend.to : "") +
            " “" + pend.exact.slice(0, 70) + (pend.exact.length > 70 ? "…" : "") + "”"}
          top={pend.top}
          bind:pend
          onAdd={(body) => onComment({ path: pend.path, side: pend.side, from: pend.from, to: pend.to, exact: pend.exact }, body)}
        />
      {/if}
    </div>
    {#if hovCmt}
      <div class="peek">
        {#each hovCmt.list as c (c.id)}
          <div><span class="anchor mono">:{c.from}{c.to !== c.from ? "–" + c.to : ""}</span> {c.body}</div>
        {/each}
      </div>
    {/if}
  {/if}
</div>

<!-- The notes sit OUTSIDE the frame, as a sibling element: the step lays its
     children out on a grid, so this lands in the page's own margin column beside
     the block rather than inside its border. Keeping it out is what leaves the
     code its full width, and the numbered badge in the gutter is what ties an
     annotation to the line it explains. -->
{#if (blk.notes ?? []).some((n) => n.side !== "old")}
  <aside class="margin" class:tail={last}>
    {#each blk.notes as n (n.num)}
      {#if n.side !== "old"}
        <button
          class="note"
          class:on={hovNote === n.num}
          onmouseenter={() => (hovNote = n.num)}
          onmouseleave={() => (hovNote = 0)}
          onclick={() => (hovNote = hovNote === n.num ? 0 : n.num)}
        >
          <span class="note-num">{n.num}</span>
          <span class="note-text">{n.text}</span>
        </button>
      {/if}
    {/each}
  </aside>
{/if}

<style>
  .frame {
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    overflow: hidden;
    margin-bottom: 20px;
  }
  .head {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 16px;
    border-bottom: 1px solid var(--k-edge);
    background: var(--k-surface);
  }
  /* A CopyBtn carries margin-left:auto so it can end a row on its own. Two of
     them in one header each pushed themselves right, which split the free space
     between them and left the anchor control floating in the middle of the bar
     with nothing to belong to. Here the spacer decides the split: both icons
     land together at the end, and they differ by glyph, not by position. */
  .head :global(button) {
    margin-left: 0;
  }
  .head :global(button:first-child) {
    border: 0;
    padding: 0;
    font-size: 0.95em;
    color: var(--k-ink);
  }
  .mono {
    font-family: var(--k-font-mono);
  }
  .range {
    font-size: 0.8em;
    color: var(--k-ink-2);
  }
  .new-chip {
    border-radius: 999px;
    padding: 0 8px;
    font-size: 0.8em;
    background: color-mix(in srgb, var(--k-success) 15%, transparent);
    color: var(--k-success);
  }
  /* Quiet by design: this is provenance, not a status the reader has to act on.
     "captured" is the ordinary case and reads as a whisper; "live file" carries
     the warning hue because it is the only one of the two that can drift out
     from under the prose beside it. */
  .src-chip {
    border-radius: 999px;
    padding: 0 8px;
    font-size: 0.75em;
    letter-spacing: 0.02em;
    color: var(--k-ink-3);
    box-shadow: inset 0 0 0 1px var(--k-edge);
  }
  .src-chip.live {
    color: color-mix(in srgb, var(--k-warning) 70%, var(--k-ink-2));
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--k-warning) 40%, transparent);
  }
  .label {
    font-size: 0.95em;
    color: var(--k-ink);
    font-style: italic;
  }
  .spacer {
    margin-left: auto;
  }
  .err {
    padding: 14px 16px;
    color: var(--k-warning);
    font-size: 0.95em;
    background: var(--k-surface-2);
  }
  .scroller {
    position: relative;
    overflow: auto;
    max-height: min(60vh, 40em);
    padding: 8px 0;
    background: var(--k-surface-2);
    font-size: 1em;
  }
  /* max-content is the fix: rows are as wide as the longest line, so a row's
     background spans the whole line. min-width keeps short blocks full width. */
  .rows {
    width: max-content;
    min-width: 100%;
  }
  .row {
    display: flex;
    white-space: pre;
    line-height: 1.55;
    font-family: var(--k-font-mono);
  }
  .row:hover {
    background: color-mix(in srgb, var(--k-ink) 4%, transparent);
  }
  /* The gutter stays put horizontally: scrolling right to read the end of a long
     line must not take the line numbers away with it. Its background is opaque
     (mixed INTO the surface, not over it) because code slides underneath. */
  .no {
    width: 3.5em;
    text-align: right;
    padding-right: 14px;
    color: var(--k-ink-3);
    flex: none;
    user-select: none;
    border-left: 3px solid transparent;
    position: sticky;
    left: 0;
    z-index: 1;
    background: var(--k-surface-2);
  }
  .row.add .no {
    border-left-color: var(--k-success);
    background: color-mix(in srgb, var(--k-success) 7%, var(--k-surface-2));
  }
  .row.add .txt {
    background: color-mix(in srgb, var(--k-success) 7%, transparent);
  }
  .row.del .no {
    border-left-color: var(--k-error);
    color: color-mix(in srgb, var(--k-error) 60%, var(--k-ink-3));
    background: color-mix(in srgb, var(--k-error) 8%, var(--k-surface-2));
  }
  .row.del .txt {
    background: color-mix(in srgb, var(--k-error) 8%, transparent);
    opacity: 0.85;
  }
  .row.cmt .no {
    color: var(--k-accent);
    font-weight: 700;
  }
  /* A region a prose phrase points at, while the reader is on the phrase. */
  .row.lit .txt {
    background: color-mix(in srgb, var(--k-accent) 16%, transparent);
  }
  .row.lit .no {
    background: color-mix(in srgb, var(--k-accent) 16%, var(--k-surface-2));
    color: var(--k-accent);
  }
  .txt {
    flex: 1 0 auto;
    padding-right: 12px;
  }
  /* The badge's own column, always present, so a line with an annotation and a
     line without one put their numbers in exactly the same place. Sticky with
     the gutter, since the two read as one unit. */
  .mark {
    flex: none;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    width: 1.7em;
    user-select: none;
    position: sticky;
    left: 3.5em;
    z-index: 1;
    background: var(--k-surface-2);
  }
  .row.add .mark {
    background: color-mix(in srgb, var(--k-success) 7%, var(--k-surface-2));
  }
  .row.del .mark {
    background: color-mix(in srgb, var(--k-error) 8%, var(--k-surface-2));
  }
  .row.lit .mark {
    background: color-mix(in srgb, var(--k-accent) 16%, var(--k-surface-2));
  }
  /* The gutter's half of the pair, read the same way as the note it points at
     (see .note-num) but a shade stronger: this one has to be findable in dense
     colour, so its ring carries more of the accent than its fill does. */
  .badge {
    all: unset;
    cursor: pointer;
    display: grid;
    place-items: center;
    width: 1.45em;
    height: 1.45em;
    border-radius: 999px;
    font-size: 0.7em;
    font-weight: 600;
    color: color-mix(in srgb, var(--k-accent) 58%, var(--k-ink));
    background: color-mix(in srgb, var(--k-accent) 18%, transparent);
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--k-accent) 46%, transparent);
  }
  .badge.on {
    color: color-mix(in srgb, var(--k-accent) 30%, var(--k-ink));
    background: color-mix(in srgb, var(--k-accent) 30%, transparent);
    box-shadow: inset 0 0 0 1px var(--k-accent);
  }
  /* The margin lives outside the code frame, in the page's own margin: an
     annotation is the agent talking about the code, not part of it. No box of
     its own - a rule and the numbers are enough to bind it to the block. The
     rule belongs to the notes (below), not to this box: measured from the box it
     went missing exactly where the layout needed the box to have no height. */
  .margin {
    grid-column: 2;
    align-self: start;
    /* Aligns the first note with the first line of code: the head bar plus the
       scroller's own padding is what the annotations have to clear. */
    padding: 45px 0 0;
    margin-bottom: 20px;
  }
  /* On the last block of a step the notes stop setting the row's height, so
     the paragraph that closes the step starts where the CODE ends rather than
     where the annotations do. A tall stack of notes used to leave a hole of
     dead space under the block, which reads as a mistake. The notes simply
     carry on down the margin beside the closing text - nothing else in a step
     uses that column, and only the last block can do this: an earlier one
     would collide with the next block's notes. */
  .margin.tail {
    max-height: 0;
    overflow: visible;
    margin-bottom: 0;
  }
  /* No room for a margin: the notes go under the block, still numbered. */
  @media (max-width: 1180px) {
    .margin {
      grid-column: 1;
      padding: 0 0 4px;
      border-top: 1px solid var(--k-edge);
    }
    /* Stacked under the block, the notes need their height back or the text
       below them would be written over. */
    .margin.tail {
      max-height: none;
      overflow: visible;
      margin-bottom: 20px;
    }
  }
  /* Each note carries the margin's rule itself, so the line is exactly as long
     as the annotations are and consecutive notes draw one unbroken stroke. The
     alternative - one border down the aside - was measured against a box whose
     height the last-block rule deliberately zeroes, which left a 45px stub in
     open space beside notes with no rule at all. */
  .note {
    all: unset;
    cursor: pointer;
    display: flex;
    gap: 9px;
    padding: 6px 4px 6px 14px;
    border-left: 1px solid var(--k-edge);
    font-family: var(--k-font-read);
    font-size: 0.95em;
    line-height: 1.55;
    color: var(--k-ink-2);
    text-wrap: pretty;
  }
  .note:hover,
  .note.on,
  .note:focus-visible {
    color: var(--k-ink);
    background: color-mix(in srgb, var(--k-accent) 9%, transparent);
    border-left-color: color-mix(in srgb, var(--k-accent) 60%, transparent);
    border-radius: 0 6px 6px 0;
  }
  /* An index is there to be read at a glance, and the numeral is 11px: a dark
     numeral on a saturated fill is the one pairing that fails in both themes at
     that size. So the fill stays a wash and the numeral takes the accent pulled
     toward the page's own ink - which lightens in the dark theme and darkens in
     the light one from the same declaration. */
  .note-num {
    flex: none;
    display: grid;
    place-items: center;
    width: 1.55em;
    height: 1.55em;
    margin-top: 0.16em;
    border-radius: 999px;
    font-family: var(--k-font-mono);
    font-size: 0.74em;
    font-weight: 600;
    color: color-mix(in srgb, var(--k-accent) 62%, var(--k-ink));
    background: color-mix(in srgb, var(--k-accent) 15%, transparent);
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--k-accent) 34%, transparent);
  }
  .note.on .note-num,
  .note:hover .note-num {
    color: color-mix(in srgb, var(--k-accent) 34%, var(--k-ink));
    background: color-mix(in srgb, var(--k-accent) 26%, transparent);
    box-shadow: inset 0 0 0 1px var(--k-accent);
  }
  .peek {
    border-top: 1px solid var(--k-accent);
    background: var(--k-surface);
    padding: 8px 16px;
    font-family: var(--k-font-read);
    font-size: 1em;
  }
  .anchor {
    font-size: 0.8em;
    color: var(--k-accent);
  }
</style>
