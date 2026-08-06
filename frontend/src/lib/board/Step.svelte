<script>
  // One step of the walk: title, purpose, prose, its code blocks, the
  // comments already left here, comprehension checks with reversible
  // reveal, and the verdict box. Everything agent-authored renders as text
  // nodes; the only injected HTML on the whole board is a code line's
  // chroma spans, inside CodeBlock.
  import CodeBlock from "./CodeBlock.svelte";
  import Composer from "./Composer.svelte";
  import VerdictBox from "./VerdictBox.svelte";
  import CopyBtn from "./CopyBtn.svelte";
  import AloudBtn from "./AloudBtn.svelte";

  let {
    step, mark, comments, root, isFirst, isLast, noteFocus, stepComposer,
    pend = $bindable(),
    onVerdict, onNote, onReveal, onComment, onCommentEdit, onCommentDelete, onNav,
    onAloud, readingRegion = null, onTerm = null, onTermHover = null, onOpen = null,
  } = $props();

  // Prose arrives as inline segments, because a bound phrase has to sit
  // mid-sentence. `p` on a segment starts a new paragraph at it; grouping
  // here is what keeps a step from rendering as one wall (FR63).
  function group(segs) {
    const out = [];
    for (const seg of segs ?? []) {
      if (!out.length || seg.p) out.push([]);
      out[out.length - 1].push(seg);
    }
    return out;
  }
  // Two prose channels, same shape: what leads into the code, and what closes
  // it. The closing paragraph is the takeaway, and it belongs under the code it
  // is about rather than three blocks above it (FR69).
  const paras = $derived(group(step.prose));
  const closingParas = $derived(group(step.close));

  // The region a bound phrase points at, while the reader is on it. Held here
  // rather than in CodeBlock because the phrase and the code are in different
  // components and the prose is the one doing the pointing.
  let lit = $state(null); // {block, from, to}
  function litOf(name) {
    const b = step.binds?.[name];
    return b ? { block: b[0], from: b[1], to: b[2] } : null;
  }

  // `c` anywhere in the step opens a step-level composer: a remark with no
  // selection has to have somewhere to go (the adversarial pass, finding 1).
  let lastStepComposer = stepComposer;
  $effect(() => {
    if (stepComposer !== lastStepComposer) {
      lastStepComposer = stepComposer;
      pend = { stepLevel: true, draft: "" };
    }
  });

  // A marked word answers two different questions, so it has two responses.
  // Hovering asks "what is that?" and gets the one-line short, after a delay
  // so a pointer crossing the paragraph never sets it off. Clicking asks
  // "tell me properly" and opens the drawer, which is the only thing that
  // stays on screen.
  //
  // The tip itself is drawn by the surface, not here: rendered inside this
  // component it lands in the scrolling column's paint order and the prose
  // draws straight over it, tooltip or not.
  let tipTimer = 0;
  function showTip(event, key) {
    clearTimeout(tipTimer);
    const el = event.currentTarget;
    tipTimer = setTimeout(() => {
      const r = el.getBoundingClientRect();
      onTermHover?.(key, { left: r.left, right: r.right, top: r.top, bottom: r.bottom });
    }, 220);
  }
  function hideTip() {
    clearTimeout(tipTimer);
    onTermHover?.(null, null);
  }

  let editing = $state(null); // comment id being edited
  let editDraft = $state("");
</script>

<article>
  <div class="head">
    <h1>{step.title}</h1>
    {#if step.allNew}<span class="allnew">all of this is new</span>{/if}
    {#if onAloud}
      <span class="spacer"></span>
      <!-- The opening of the step: its title, its purpose and the prose above the
           first block. Every other region has its own control down the page. -->
      <AloudBtn
        region="intro"
        {readingRegion}
        {onAloud}
        label="the opening of this step (a)"
      />
    {/if}
  </div>
  {#if step.purpose}<div class="purpose">{step.purpose}</div>{/if}

  {#snippet segments(segs)}
    {#each segs as seg, i (i)}
      {#if seg.code}
        <code>{seg.code}</code>
      {:else if seg.bind && step.binds?.[seg.bind]}
        <button
          class="bindref"
          onmouseenter={() => (lit = litOf(seg.bind))}
          onmouseleave={() => (lit = null)}
          onfocus={() => (lit = litOf(seg.bind))}
          onblur={() => (lit = null)}
          title="the code this points at"
        >{seg.t}</button>
      {:else if seg.runs}
        <!-- Go cut this segment at its glossary marks, so the surface never
             counts characters. A marked word says only that a definition
             exists; it never pops one, because a definition the reader did
             not ask for is an interruption. -->
        {@render marked(seg.runs)}
      {:else}<span>{seg.t}</span>{/if}
    {/each}
  {/snippet}

  {#snippet paragraph(segs, extra)}
    <p class="prose {extra}">{@render segments(segs)}</p>
  {/snippet}

  {#snippet marked(runs)}
    {#each runs as r, j (j)}
      {#if r.g}
        <button
          class="term"
          onclick={() => { hideTip(); onTerm?.(r.g); }}
          onmouseenter={(e) => showTip(e, r.g)}
          onmouseleave={hideTip}
          onfocus={(e) => showTip(e, r.g)}
          onblur={hideTip}
        >{r.t}</button>
      {:else}<span>{r.t}</span>{/if}
    {/each}
  {/snippet}

  {#each paras as segs, pi (pi)}{@render paragraph(segs, "")}{/each}

  {#each step.codes ?? [] as blk, bi (bi)}
    <!-- The lead hands the reader into this block. Without it a step with two
         blocks stacks all its text above the first one and the seam between
         them is a wall of code with nothing to hold (FR69). -->
    {#if blk.lead}
      <p class="prose lead">
        {#if onAloud}<AloudBtn
            region={"lead:" + bi}
            {readingRegion}
            {onAloud}
            label="the paragraph above this block"
          />{/if}{#if blk.leadRuns}{@render marked(blk.leadRuns)}{:else}{blk.lead}{/if}
      </p>
    {/if}
    <CodeBlock
      {blk}
      {root}
      stepId={step.id}
      comments={comments.filter((c) => c.path === blk.path && blk.path !== "")}
      bind:pend
      blockIndex={bi}
      lit={lit && lit.block === bi ? lit : null}
      last={bi === (step.codes ?? []).length - 1}
      {onComment}
      {onOpen}
    />
  {/each}

  <!-- The takeaway under the last block. It carries its own control because it is
       the sentence a step exists to land, and until FR72 it was not read at all:
       read-aloud was written against the step shape from before FR69 added it. -->
  {#if closingParas.length && onAloud}
    <p class="prose close">
      <AloudBtn
        region="close"
        {readingRegion}
        {onAloud}
        label="the takeaway"
      />{@render segments(closingParas[0])}
    </p>
    {#each closingParas.slice(1) as segs, pi (pi)}{@render paragraph(segs, "close")}{/each}
  {:else}
    {#each closingParas as segs, pi (pi)}{@render paragraph(segs, "close")}{/each}
  {/if}

  {#if (step.codes ?? []).length > 0}
    <div class="legend" data-agentbox-find-exclude>
      <span><span class="bar add"></span> added in this diff</span>
      <span><span class="bar del"></span> removed (old line numbers)</span>
      <span class="right">select code to comment</span>
    </div>
  {/if}

  {#each step.cmds ?? [] as c, i (i)}
    <div class="cmd">
      <div class="cmd-head">
        <code class="mono">{c.cmd}</code>
        <CopyBtn text={c.cmd} />
      </div>
      <div class="cmd-body">
        {#if c.expect}expect: {c.expect}{/if}
        {#if c.recorded}<span class="recorded"> (recorded {c.recorded})</span>{/if}
      </div>
    </div>
  {/each}

  {#if pend?.stepLevel}
    <Composer
      anchorLabel="this step"
      bind:pend
      onAdd={(body) => onComment({}, body)}
    />
  {/if}

  {#each comments as c (c.id)}
    <div class="comment">
      <span class="anchor mono" title={c.path}>
        {#if c.path}:{c.from}{c.to !== c.from ? "–" + c.to : ""}{:else}step{/if}
      </span>
      {#if editing === c.id}
        <textarea
          class="edit"
          bind:value={editDraft}
          onkeydown={(e) => {
            if (e.key === "Enter" && e.ctrlKey) { onCommentEdit(c.id, editDraft.trim()); editing = null; }
            if (e.key === "Escape") editing = null;
          }}
        ></textarea>
      {:else}
        <span class="body">{c.body}</span>
      {/if}
      <button class="tiny" onclick={() => { editing = c.id; editDraft = c.body; }}>edit</button>
      <button class="tiny" onclick={() => onCommentDelete(c.id)}>✕</button>
    </div>
  {/each}

  {#if (step.checks ?? []).length > 0}
    <section class="checks">
      <div class="checks-label">
        Check yourself
        {#if onAloud}<AloudBtn
            region="checks"
            {readingRegion}
            {onAloud}
            label="the questions"
          />{/if}
      </div>
      {#each step.checks as c, i (i)}
        {@const open = mark.revealed.includes(i)}
        <div class="check">
          <button class="caret" onclick={() => onReveal(i, !open)} title={open ? "hide the answer" : "reveal the answer"}>
            {open ? "▾" : "▸"}
          </button>
          <div class="qa">
            <button class="q" onclick={() => onReveal(i, !open)}>{c.q}</button>
            {#if open}<div class="a">{c.a}</div>{/if}
          </div>
        </div>
      {/each}
    </section>
  {/if}

  <VerdictBox
    kind={step.kind}
    {mark}
    {isFirst}
    {isLast}
    {noteFocus}
    {onVerdict}
    {onNote}
    {onNav}
  />
  <div class="tail"></div>
</article>

<style>
  /* Two columns: everything the step says, and a margin for the annotations on
     its code. CodeBlock emits its frame and its notes as siblings, so a block
     and its notes land on the same grid row and stay aligned without either one
     measuring the other. The margin collapses when there is no room for it. */
  article {
    display: grid;
    grid-template-columns: minmax(0, 58em) minmax(0, 21em);
    column-gap: 26px;
    justify-content: center;
    align-items: start;
    animation: enter 180ms ease-out;
  }
  article > :global(*) {
    grid-column: 1;
    min-width: 0;
  }
  @media (max-width: 1180px) {
    article {
      grid-template-columns: minmax(0, 58em);
    }
  }
  @keyframes enter {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    article {
      animation: none;
    }
  }
  .head {
    display: flex;
    align-items: baseline;
    gap: 12px;
    margin-bottom: 4px;
  }
  h1 {
    font-size: 1.5em;
    font-weight: 700;
    margin: 0;
  }
  .allnew {
    border-radius: 999px;
    padding: 2px 10px;
    font-size: 0.8em;
    background: color-mix(in srgb, var(--k-success) 15%, transparent);
    color: var(--k-success);
    white-space: nowrap;
  }
  .purpose {
    color: var(--k-ink-2);
    font-size: 0.95em;
    margin-bottom: 24px;
  }
  /* Reading measure, not container width: past about 75 characters a line the
     eye loses its place on the return sweep, and this text is meant to be read
     rather than scanned. The last paragraph carries the gap to the code. */
  .prose {
    font-family: var(--k-font-read);
    font-size: 1.25em;
    line-height: 1.68;
    max-width: 68ch;
    margin: 0 0 0.9em;
    text-wrap: pretty;
  }
  .prose:last-of-type {
    margin-bottom: 26px;
  }
  /* Declared after the rule above so a lead that happens to be the last
     paragraph keeps its own small gap: a lead belongs to the block under it,
     and 26px of air would read as the end of a thought rather than a handover. */
  .prose.lead {
    margin: 24px 0 0.55em;
  }
  .prose.close {
    margin-top: 2px;
  }
  /* A bound phrase is text first: an underline it shares with no other span,
     and no colour of its own, so a paragraph does not turn into a link farm. */
  .bindref {
    all: unset;
    cursor: pointer;
    text-decoration: underline dotted color-mix(in srgb, var(--k-accent) 70%, transparent);
    text-underline-offset: 3px;
  }
  .bindref:hover,
  .bindref:focus-visible {
    text-decoration-style: solid;
    background: color-mix(in srgb, var(--k-accent) 12%, transparent);
    border-radius: 3px;
  }
  /* A defined term is marked quieter than a bound phrase and in a different
     ink, so the two never read as the same offer: a bind points at code in
     this step, a term points out of the review at a definition. */
  .term {
    all: unset;
    cursor: help;
    border-bottom: 1px dotted var(--k-ink-3);
  }
  .term:hover,
  .term:focus-visible {
    color: var(--k-info);
    border-bottom-color: var(--k-info);
  }
  .spacer {
    margin-left: auto;
  }
  /* A read control sits inline with the prose it reads, so it needs the gap a
     word would have had. The button itself is AloudBtn; this is only its
     relationship to the text beside it. */
  .prose :global(.aloud),
  .checks-label :global(.aloud) {
    margin-right: 0.45em;
  }
  .prose code {
    font-family: var(--k-font-mono);
    font-size: 0.8em;
    background: var(--k-surface-2);
    border-radius: 4px;
    padding: 1px 5px;
  }
  .legend {
    display: flex;
    flex-wrap: wrap;
    gap: 16px;
    margin: 8px 0 24px;
    font-size: 0.85em;
    color: var(--k-ink-2);
  }
  .legend .right {
    margin-left: auto;
  }
  .bar {
    display: inline-block;
    width: 3px;
    height: 0.9em;
    vertical-align: -2px;
    margin-right: 4px;
  }
  .bar.add {
    background: var(--k-success);
  }
  .bar.del {
    background: var(--k-error);
  }
  .mono {
    font-family: var(--k-font-mono);
  }
  .cmd {
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    overflow: hidden;
    margin-bottom: 12px;
  }
  .cmd-head {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 12px;
    background: var(--k-surface);
  }
  .cmd-head code {
    font-size: 0.95em;
  }
  .cmd-body {
    padding: 8px 12px;
    font-size: 0.95em;
    color: var(--k-ink-2);
  }
  .recorded {
    color: var(--k-ink-3);
  }
  .comment {
    display: flex;
    align-items: baseline;
    gap: 8px;
    border-left: 2px solid var(--k-accent);
    border-radius: 8px;
    padding: 8px 0 8px 16px;
    margin-bottom: 8px;
  }
  .anchor {
    font-size: 0.85em;
    color: var(--k-accent);
    flex: none;
  }
  .comment .body {
    font-family: var(--k-font-read);
    font-size: 1.15em;
    min-width: 0;
    flex: 1;
  }
  .comment .edit {
    flex: 1;
    font-family: var(--k-font-read);
    font-size: 1.05em;
    background: var(--k-surface-2);
    color: var(--k-ink);
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    padding: 6px 10px;
  }
  .tiny {
    background: none;
    border: 0;
    color: var(--k-ink-3);
    font-size: 0.8em;
    cursor: pointer;
    padding: 0 4px;
  }
  .tiny:hover {
    color: var(--k-ink);
  }
  .checks {
    margin: 32px 0 16px;
  }
  .checks-label {
    font-weight: 700;
    font-size: 0.9em;
    letter-spacing: 0.02em;
    color: var(--k-ink-2);
    margin-bottom: 8px;
  }
  .check {
    display: flex;
    gap: 8px;
    align-items: flex-start;
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    padding: 12px 16px;
    margin-bottom: 12px;
  }
  .caret {
    background: none;
    border: 0;
    color: var(--k-ink-2);
    cursor: pointer;
    padding: 2px 4px;
    font-size: 0.85em;
  }
  .qa {
    min-width: 0;
    flex: 1;
  }
  .q {
    display: block;
    width: 100%;
    text-align: left;
    background: none;
    border: 0;
    padding: 0;
    cursor: pointer;
    color: inherit;
    font-family: var(--k-font-read);
    font-size: 1.15em;
    line-height: 1.5;
  }
  .a {
    font-family: var(--k-font-read);
    font-size: 1.15em;
    color: var(--k-ink-2);
    margin-top: 6px;
    border-left: 2px solid var(--k-edge);
    padding-left: 10px;
    animation: reveal 160ms ease-out;
  }
  @keyframes reveal {
    from {
      opacity: 0;
    }
  }
  .tail {
    height: 32px;
  }
</style>
