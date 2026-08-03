<script>
  import { bridge, on } from "../lib/bridge.js";
  import { markdown } from "../lib/markdown.svelte.js";
  import { setArtifactZoom, artifactFind, artifactFindJump, onArtifactFound, onArtifactChord } from "../lib/artifact.svelte.js";

  // The viewer (FR36-38): a reading window. The measure is fixed at 760px
  // whatever the window does, because a line of prose that stretches to 1600px is
  // unreadable no matter how nice the window looks.
  //
  // Go owns the document (reading the file, rendering it, re-reading it when it
  // changes). Everything here is reading comfort: the measure, find-in-page, the
  // zoom, and the scroll keys.

  let { chrome = true } = $props(); // false when hosted in the app window, which has its own title bar

  let doc = $state({ empty: true, title: "agentbox · viewer", html: "", watch: false, revMs: 0 });
  let finding = $state(false);
  let query = $state("");
  let matches = $state(0);
  let at = $state(0);
  let zoom = $state(1);
  let page = $state(null); // the scrolling element
  let article = $state(null); // the rendered document

  // A+/A- on an artifact page: the sandboxed document cannot see this page's
  // font-size, so the zoom is handed to the frame's own renderer instead
  // (artifact.svelte.js setArtifactZoom), which keeps text crisp at any size.
  $effect(() => {
    if (doc.artifact && article) setArtifactZoom(article, zoom);
  });
  let box = $state(null); // the find input

  on("agentbox:doc", (d) => {
    if (!d) return;
    // A watched file re-renders in place, so hold the scroll: an agent editing a
    // paragraph you are reading must not throw you back to the top (FR37).
    const keep = page?.scrollTop ?? 0;
    const reload = d.revMs !== doc.revMs && d.path && d.path === doc.path;
    doc = d;
    queueMicrotask(() => {
      if (reload && page) page.scrollTop = keep;
      if (query) mark();
    });
  });

  bridge.ready("viewer");
  bridge.document().then((d) => d && (doc = d));

  // The frame's answer to a forwarded find: its own match count and position.
  onArtifactFound((c) => {
    matches = c.matches;
    at = c.at;
  });

  // Chords pressed while focus is inside the frame: the sandbox forwards
  // them because this window would otherwise never hear a Ctrl+F again
  // after the first click into the artifact.
  onArtifactChord((key) => {
    if (key !== "find" && finding && matches) jump(key === "find-prev" ? -1 : 1);
    else openFind();
  });

  // --- find in page ---------------------------------------------------------
  // Wrapping the hits in <mark> beats a CSS highlight pass here: the matches have
  // to be scrollable-to, and a range that owns a DOM node is a range you can
  // scroll into view.

  function clearMarks() {
    if (!article) return;
    for (const m of [...article.querySelectorAll("mark.k-hit")]) {
      const text = document.createTextNode(m.textContent);
      m.replaceWith(text);
    }
    article.normalize();
  }

  function mark() {
    if (!article) return;
    // An artifact page has no text of its own to mark - the program lives in a
    // sandboxed frame. The query is forwarded there; the frame searches its own
    // document, scrolls to the hit, and answers with the count shown here.
    if (doc.artifact) {
      matches = 0;
      at = 0;
      artifactFind(article, query);
      return;
    }
    clearMarks();
    matches = 0;
    at = 0;
    const needle = query.trim().toLowerCase();
    if (needle.length < 2) return;

    // Walk text nodes only, so a hit inside highlighted code or a table cell is
    // wrapped without disturbing the structure around it. Charts and diagrams are
    // skipped: SVG has no <mark>, so wrapping a word inside one deletes it from
    // the picture - a chart legend reading "es" after searching for "vetoes" is
    // what that looks like. The hidden mermaid source is skipped for the same
    // reason a hidden match is worthless: it cannot be scrolled to.
    const walker = document.createTreeWalker(article, NodeFilter.SHOW_TEXT, {
      acceptNode: (n) => {
        if (!n.nodeValue.toLowerCase().includes(needle)) return NodeFilter.FILTER_REJECT;
        if (n.parentElement?.closest("svg, .k-mermaid-src")) return NodeFilter.FILTER_REJECT;
        return NodeFilter.FILTER_ACCEPT;
      },
    });
    const nodes = [];
    while (walker.nextNode()) nodes.push(walker.currentNode);

    for (const node of nodes) {
      let text = node;
      let idx = text.nodeValue.toLowerCase().indexOf(needle);
      while (idx !== -1) {
        const tail = text.splitText(idx);
        text = tail.splitText(needle.length);
        const hit = document.createElement("mark");
        hit.className = "k-hit";
        hit.textContent = tail.nodeValue;
        tail.replaceWith(hit);
        matches++;
        idx = text.nodeValue.toLowerCase().indexOf(needle);
      }
    }
    if (matches) jump(1);
  }

  function jump(delta) {
    if (doc.artifact) {
      if (article && matches) artifactFindJump(article, delta);
      return;
    }
    const hits = article?.querySelectorAll("mark.k-hit");
    if (!hits?.length) return;
    at = ((at + delta - 1 + hits.length) % hits.length) + 1;
    for (const h of hits) h.classList.remove("on");
    const target = hits[at - 1];
    target.classList.add("on");
    target.scrollIntoView({ block: "center", behavior: "smooth" });
  }

  function openFind() {
    finding = true;
    queueMicrotask(() => box?.focus());
  }

  function closeFind() {
    finding = false;
    query = "";
    if (doc.artifact && article) artifactFind(article, "");
    clearMarks();
    matches = 0;
    at = 0;
    page?.focus();
  }

  function onKey(e) {
    const typing = e.target instanceof HTMLInputElement;

    if (e.key === "Escape") {
      if (finding) {
        e.preventDefault();
        closeFind();
      }
      return;
    }
    if (e.key === "F3") {
      e.preventDefault();
      if (finding && matches) jump(e.shiftKey ? -1 : 1);
      else openFind();
      return;
    }
    if (typing) {
      if (e.key === "Enter") {
        e.preventDefault();
        e.shiftKey ? jump(-1) : jump(1);
      }
      return;
    }
    if (e.key === "/" || ((e.ctrlKey || e.metaKey) && e.key === "f")) {
      e.preventDefault();
      openFind();
      return;
    }
    if ((e.ctrlKey || e.metaKey) && (e.key === "=" || e.key === "+")) {
      e.preventDefault();
      zoom = Math.min(1.8, Math.round((zoom + 0.1) * 10) / 10);
      return;
    }
    if ((e.ctrlKey || e.metaKey) && e.key === "-") {
      e.preventDefault();
      zoom = Math.max(0.7, Math.round((zoom - 0.1) * 10) / 10);
      return;
    }
    if ((e.ctrlKey || e.metaKey) && e.key === "0") {
      e.preventDefault();
      zoom = 1;
      return;
    }
    if (!page) return;
    const line = 64;
    if (e.key === "j" || e.key === "ArrowDown") {
      e.preventDefault();
      page.scrollBy({ top: line });
    } else if (e.key === "k" || e.key === "ArrowUp") {
      e.preventDefault();
      page.scrollBy({ top: -line });
    } else if (e.key === "g") {
      e.preventDefault();
      page.scrollTo({ top: 0 });
    } else if (e.key === "G") {
      e.preventDefault();
      page.scrollTo({ top: page.scrollHeight });
    } else if (e.key === "q" && chrome) {
      e.preventDefault();
      bridge.closeSelf();
    }
  }

  const label = $derived(doc.title?.replace(/^agentbox · /, "") ?? "document");
</script>

<svelte:window on:keydown={onKey} />

<div class="viewer">
  {#if chrome}
    <div class="bar" style="--wails-draggable: drag">
      <span class="name">{label}</span>
      {#if doc.watch}<span class="watching" title="re-renders when the file changes">watching</span>{/if}
      <span class="spacer"></span>
      <button class="tool" title="find (/)" onclick={openFind}>find</button>
      <button class="tool" title="zoom out (Ctrl+-)" onclick={() => (zoom = Math.max(0.7, Math.round((zoom - 0.1) * 10) / 10))}>A&minus;</button>
      <!-- The level lives next to the buttons that change it, because the
           footer's copy of it is off the eye's path: a click at a clamp
           bound that changes nothing read as "zoom is broken". Clicking the
           number puts it back to 100%. -->
      {#if zoom !== 1}
        <button class="tool zoomlvl" title="back to 100% (Ctrl+0)" onclick={() => (zoom = 1)}>{Math.round(zoom * 100)}%</button>
      {/if}
      <button class="tool" title="zoom in (Ctrl+=)" onclick={() => (zoom = Math.min(1.8, Math.round((zoom + 0.1) * 10) / 10))}>A+</button>
      <button class="tool close" title="close (q)" onclick={() => bridge.closeSelf()}>&#x2715;</button>
    </div>
  {/if}

  {#if finding}
    <div class="find">
      <span class="lbl">Find</span>
      <input bind:this={box} bind:value={query} oninput={mark} placeholder={doc.artifact ? "search the running artifact" : "search this document"} spellcheck="false" />
      <span class="count">{query.trim().length < 2 ? "" : matches ? `${at}/${matches}` : "no matches"}</span>
      <button class="tool" title="previous (Shift+Enter)" onclick={() => jump(-1)}>&#x2191;</button>
      <button class="tool" title="next (Enter)" onclick={() => jump(1)}>&#x2193;</button>
      <button class="tool" title="close (Esc)" onclick={closeFind}>&#x2715;</button>
    </div>
  {/if}

  <!-- tabindex so the page itself can hold focus for the scroll keys -->
  <div class="page" class:filled={doc.artifact} bind:this={page} tabindex="-1">
    {#if doc.empty}
      <div class="blank">
        <p class="lead">Nothing open.</p>
        <p><code>agentbox show FILE</code> opens a document here. <code>--watch</code> keeps it live while an agent edits it.</p>
      </div>
    {:else}
      <!-- An artifact is a program, not prose: it drops the measure and takes the
           window (k-md-artifact in app.css), and its frame fills the space rather
           than sizing itself to its content. -->
      <article
        class="prose k-md selectable"
        class:k-md-artifact={doc.artifact}
        data-fill={doc.artifact ? "1" : null}
        bind:this={article}
        style="font-size: {zoom}rem"
        use:markdown={doc.html}>{@html doc.html}</article>
    {/if}
  </div>

  {#if chrome}
    <div class="foot">
      <span>{doc.path || "inline"}</span>
      <span class="spacer"></span>
      <span>{Math.round(zoom * 100)}%</span>
      <span>/ find · j k scroll · q close</span>
    </div>
  {/if}
</div>

<style>
  .viewer {
    height: 100%;
    display: flex;
    flex-direction: column;
    min-height: 0;
    background: var(--k-ground);
    color: var(--k-ink);
  }

  .bar {
    display: flex;
    align-items: center;
    gap: 8px;
    height: 38px;
    padding: 0 8px 0 14px;
    border-bottom: 1px solid var(--k-edge-soft);
  }
  .name {
    font-size: 0.8rem;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .watching {
    font-family: var(--k-font-mono);
    font-size: 0.6rem;
    padding: 1px 6px;
    border-radius: 999px;
    color: var(--k-success);
    border: 1px solid color-mix(in srgb, var(--k-success) 40%, transparent);
    background: color-mix(in srgb, var(--k-success) 12%, transparent);
  }
  .spacer {
    flex: 1;
  }
  .tool {
    padding: 3px 8px;
    border-radius: 6px;
    font-family: var(--k-font-mono);
    font-size: 0.66rem;
    color: var(--k-ink-3);
  }
  .tool:hover {
    background: var(--k-surface-2);
    color: var(--k-ink);
  }
  .tool.close:hover {
    color: var(--k-error);
  }
  /* Not ink-3 like its neighbours: the number only appears away from 100%,
   * and it is the state, not a verb. */
  .tool.zoomlvl {
    color: var(--k-accent);
    font-variant-numeric: tabular-nums;
  }

  .find {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 10px 6px 14px;
    border-bottom: 1px solid var(--k-edge-soft);
    background: var(--k-surface);
  }
  .find .lbl {
    font-family: var(--k-font-mono);
    font-size: 0.62rem;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--k-ink-3);
  }
  .find input {
    flex: 1;
    font: inherit;
    font-size: 0.84rem;
    color: var(--k-ink);
    background: var(--k-surface-2);
    border: 1px solid var(--k-edge);
    border-radius: 6px;
    padding: 4px 9px;
  }
  .find input:focus {
    outline: none;
    border-color: color-mix(in srgb, var(--k-accent) 60%, transparent);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--k-accent) 15%, transparent);
  }
  .find .count {
    font-family: var(--k-font-mono);
    font-size: 0.66rem;
    color: var(--k-ink-3);
    min-width: 7ch;
    text-align: right;
  }

  .page {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    background: var(--k-surface);
  }
  .page:focus {
    outline: none;
  }

  /* The measure: 760px of text, centered, whatever the window is doing. */
  .prose {
    max-width: 760px;
    margin: 0 auto;
    padding: 34px 28px 72px;
    font-family: var(--k-font-read);
    line-height: 1.72;
    color: var(--k-ink);
  }
  /* An artifact is the exception: no measure, no reading padding, and the page
   * becomes a flex column so the frame fills what is left of the window rather
   * than guessing a height. */
  .page.filled {
    display: flex;
    overflow: hidden;
  }
  .page.filled .prose {
    flex: 1;
    min-height: 0;
    /* min-width matters as much as min-height: without it, an artifact's code
     * view (one long unwrapped line is enough) sets this flex item's automatic
     * minimum to the widest line, the block grows past the window, and the
     * bar's preview/code tabs end up clipped off-screen with overflow hidden -
     * which reads as "I clicked code and there is no way back". */
    min-width: 0;
    max-width: none;
    padding: 10px;
  }
  .blank {
    max-width: 460px;
    margin: 22vh auto 0;
    text-align: center;
    font-family: var(--k-font-read);
    color: var(--k-ink-3);
  }
  .blank .lead {
    font-size: 1.05rem;
    color: var(--k-ink-2);
  }
  .blank code {
    font-family: var(--k-font-mono);
    font-size: 0.82em;
    color: var(--k-ink-2);
  }

  .foot {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 0 16px;
    height: 28px;
    border-top: 1px solid var(--k-edge-soft);
    background: var(--k-ground);
    font-family: var(--k-font-mono);
    font-size: 0.62rem;
    color: var(--k-ink-3);
  }
  .foot span:first-child {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 52ch;
  }

  /* --- the document itself -------------------------------------------------
   * Everything a heading, table, list, alert, chart or code block looks like is
   * in app.css (.k-md), shared with the card and the session so the reader and a
   * card body cannot disagree. What is left here belongs to the reading window
   * alone: the measure, the looser leading a whole document earns, and the find
   * marks. */

  .prose :global(h1) {
    font-size: 1.85em;
  }
  .prose :global(h2) {
    font-size: 1.4em;
  }
  .prose :global(h3) {
    font-size: 1.14em;
  }
  .prose :global(p),
  .prose :global(ul),
  .prose :global(ol),
  .prose :global(blockquote) {
    margin-bottom: 1.05em;
  }
  .prose :global(hr) {
    margin: 2em 0;
  }
  .prose :global(mark.k-hit) {
    background: color-mix(in srgb, var(--k-warning) 34%, transparent);
    color: inherit;
    border-radius: 2px;
  }
  .prose :global(mark.k-hit.on) {
    background: color-mix(in srgb, var(--k-warning) 62%, transparent);
    outline: 1px solid var(--k-warning);
  }
</style>
