<script>
  import { bridge } from "../lib/bridge.js";
  import { markdown } from "../lib/markdown.svelte.js";
  import IdentityPill from "../lib/IdentityPill.svelte";
  import AskPanel from "../lib/AskPanel.svelte";

  // Where an agent's work is read and answered. The rules that shape it:
  // prose caps at a reading measure and never reflows to the window width;
  // the mechanics of a turn (thinking, tool calls) collapse so the conclusion
  // gets the space; and the asymmetry between a user bubble and a full-width
  // agent turn replaces speaker labels entirely.

  let { session = null } = $props();

  let draft = $state("");
  let scroller = $state(null);
  let stick = $state(true);
  let open = $state({}); // expanded thinking blocks, keyed turn:seg

  const conv = $derived(session?.conv ?? []);
  const working = $derived(session?.state === "working");

  // Follow the stream, but stop following the moment the user scrolls up to
  // read something - nothing is more annoying than being yanked to the bottom
  // mid-sentence.
  function onScroll() {
    if (!scroller) return;
    const gap = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight;
    stick = gap < 60;
  }

  $effect(() => {
    conv.length;
    if (!scroller || !stick) return;
    queueMicrotask(() => (scroller.scrollTop = scroller.scrollHeight));
  });

  function send() {
    const text = draft.trim();
    if (!text || !session) return;
    draft = "";
    bridge.sendPrompt(session.id, text);
  }

  function onKey(e) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      send();
      return;
    }
    // Esc hands the keyboard back, which is how the ask panel's single-key
    // answers become reachable without ever taking focus from the composer.
    if (e.key === "Escape") {
      e.preventDefault();
      e.currentTarget.blur();
    }
  }

  const key = (t, s) => `${t}:${s}`;
</script>

{#if !session}
  <div class="blank">
    <h2>No session selected</h2>
    <p>Start one with “+ New”, or pick one from the list.</p>
  </div>
{:else}
  <div class="top">
    <div class="crumbs">
      <span class="proj">{session.project}</span>
      <span class="path">{session.cwd}</span>
    </div>
    <span class="spacer"></span>
    <!-- Buttons, not labels. These were two words with no handler, which is why
         Full was unreachable; switching replaces the child (--permission-mode is a
         spawn-time flag) and keeps the conversation. -->
    <div class="seg">
      <button
        class:on={session.mode === "plan"}
        title="Read-only: it can look and plan, not change anything"
        onclick={() => bridge.setSessionMode(session.id, "plan")}>Plan</button>
      <button
        class:on={session.mode === "full"}
        title="It can edit and run things"
        onclick={() => bridge.setSessionMode(session.id, "full")}>Full</button>
    </div>
    {#if session.model}<span class="model">{session.model}</span>{/if}
    {#if working}
      <button class="stop" onclick={() => bridge.stopSession(session.id)}>Stop</button>
    {/if}
  </div>

  <div class="convo" bind:this={scroller} onscroll={onScroll}>
    <div class="col">
      {#each conv as turn, ti}
        {#if turn.role === "user"}
          <div class="user">
            <div class="bubble selectable">
              <!-- The prompt as typed: Go sends the source for a user turn, because
                   de-tagging the rendered HTML left the entities behind. -->
              {#each turn.segments as seg}{seg.text ?? ""}{/each}
            </div>
          </div>
        {:else if turn.role === "system"}
          <div class="system selectable" class:bad={turn.err}>
            {turn.err || "session started"}
          </div>
        {:else}
          <div class="agent">
            <div class="who">
              <IdentityPill agent="claude-code" project={session.project} />
              <!-- When it arrived, and how long it worked before its first word.
                   Both formatted in Go (24 hour, and "4s"/"1m20s"). The cost is
                   off unless [session] show_cost asks for it: interesting once,
                   noise on every turn. -->
              {#if turn.at}<span class="when">{turn.at}</span>{/if}
              {#if turn.think}<span class="think-for" title="worked for this long before replying">{turn.think}</span>{/if}
              {#if session.showCost && turn.costUsd}
                <span class="cost">${turn.costUsd.toFixed(4)}</span>
              {/if}
            </div>

            {#each turn.segments as seg, si}
              {#if seg.kind === "thinking"}
                <button class="think" onclick={() => (open[key(ti, si)] = !open[key(ti, si)])}>
                  <span class="caret">{open[key(ti, si)] ? "▾" : "▸"}</span> Thinking
                </button>
                {#if open[key(ti, si)]}
                  <div class="thought k-md selectable" use:markdown={seg.html}>{@html seg.html}</div>
                {/if}
              {:else if seg.kind === "tool"}
                <!-- A log line, not a widget. A reply that ran five commands showed
                     five bordered pills stacked down the column, each one shouting
                     as loudly as the answer; what the reader wants is to scan what
                     it did and read what it said. A shell command is highlighted
                     here like any other code (toolHtml). -->
                <div class="tool" class:bad={seg.isError} title={seg.toolInput}>
                  <span class="k">{seg.toolName}</span>
                  {#if seg.toolHtml}
                    <span class="arg chroma">{@html seg.toolHtml}</span>
                  {:else}
                    <span class="arg">{seg.toolInput}</span>
                  {/if}
                  <span class="mark">{seg.isError ? "✗" : seg.hasResult ? "✓" : "…"}</span>
                </div>
              {:else if seg.kind === "result"}
                <pre class="result selectable">{seg.result}</pre>
              {:else}
                <div class="prose k-md selectable" use:markdown={seg.html}>{@html seg.html}</div>
              {/if}
            {/each}
          </div>
        {/if}
      {/each}

      {#if working}
        <div class="working"><span class="pulse"></span> working…</div>
      {/if}
    </div>
  </div>

  <!-- FR49: a question from this agent lands between the transcript and the
       composer, where the answer goes, instead of in a card over the window. -->
  {#if session.ask}
    <div class="askwrap">
      <div class="inner"><AskPanel ask={session.ask} /></div>
    </div>
  {/if}

  <div class="composer">
    <div class="inner">
      <div class="box">
        <textarea
          rows="2"
          bind:value={draft}
          onkeydown={onKey}
          placeholder="Reply to claude-code…"
          class="selectable"
        ></textarea>
        <div class="row">
          <span class="hint">Enter sends · Shift+Enter for a newline</span>
          <span class="spacer"></span>
          <button class="send" onclick={send} disabled={!draft.trim()}>Send</button>
        </div>
      </div>
    </div>
  </div>
{/if}

<style>
  /* The serif empty-state voice the viewer and the library use. */
  .blank {
    margin: auto;
    text-align: center;
    font-family: var(--k-font-read);
    font-size: 0.9rem;
    color: var(--k-ink-3);
  }
  .blank h2 {
    margin: 0 0 4px;
    font-size: 1.05rem;
    font-weight: 400;
    color: var(--k-ink-2);
  }
  .blank p {
    margin: 0;
  }

  .top {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 9px 16px;
    border-bottom: 1px solid var(--k-edge-soft);
  }
  .crumbs {
    display: flex;
    align-items: baseline;
    gap: 7px;
    min-width: 0;
  }
  .proj {
    font-size: 0.88rem;
    font-weight: 700;
    letter-spacing: -0.01em;
  }
  .path {
    font-family: var(--k-font-mono);
    font-size: 0.7rem;
    color: var(--k-ink-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .spacer {
    flex: 1;
  }
  .seg {
    display: flex;
    gap: 2px;
    padding: 2px;
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    background: var(--k-surface-2);
  }
  .seg button {
    font-size: 0.75rem;
    padding: 2px 10px;
    border-radius: 6px;
    color: var(--k-ink-3);
    background: transparent;
    border: 0;
  }
  .seg button:hover {
    color: var(--k-ink);
  }
  .seg button.on {
    background: var(--k-surface-3);
    color: var(--k-ink);
  }
  .model {
    font-family: var(--k-font-mono);
    font-size: 0.66rem;
    color: var(--k-ink-3);
  }
  .stop {
    font-size: 0.74rem;
    padding: 3px 10px;
    border: 1px solid color-mix(in srgb, var(--k-error) 45%, transparent);
    border-radius: 7px;
    color: var(--k-ink);
    background: color-mix(in srgb, var(--k-error) 14%, transparent);
  }

  .convo {
    flex: 1;
    overflow-y: auto;
    padding: 24px 0 8px;
    min-height: 0;
  }
  .col {
    /* The measure is a variable so a wide host can widen it: 700px is right for a
     * column beside the app window's rail, and leaves a drop-down panel two-thirds
     * empty. The panel sets --k-measure. */
    max-width: var(--k-measure, 700px);
    margin: 0 auto;
    padding: 0 28px;
    display: flex;
    flex-direction: column;
    gap: 20px;
  }

  .user {
    display: flex;
    justify-content: flex-end;
  }
  .bubble {
    max-width: 78%;
    background: var(--k-surface-2);
    border: 1px solid var(--k-edge);
    border-radius: 12px 12px 4px 12px;
    padding: 9px 13px;
    font-family: var(--k-font-read);
    font-size: 0.95rem;
    line-height: 1.55;
    white-space: pre-wrap;
  }

  .system {
    font-family: var(--k-font-mono);
    font-size: 0.68rem;
    color: var(--k-ink-3);
    text-align: center;
  }
  .system.bad {
    color: var(--k-error);
  }

  .agent {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .who {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .when,
  .think-for {
    font-family: var(--k-font-mono);
    font-size: 0.62rem;
    color: var(--k-ink-3);
  }
  .think-for::before {
    content: "· ";
  }
  .cost {
    font-family: var(--k-font-mono);
    font-size: 0.62rem;
    color: var(--k-ink-3);
  }

  .think {
    align-self: flex-start;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 2px 0 2px 12px;
    border-left: 2px solid var(--k-edge);
    font-family: var(--k-font-read);
    font-style: italic;
    font-size: 0.86rem;
    color: var(--k-ink-3);
  }
  .think:hover {
    color: var(--k-ink-2);
  }
  .caret {
    font-family: var(--k-font-mono);
    font-style: normal;
    font-size: 0.62rem;
  }
  .thought {
    border-left: 2px solid var(--k-edge);
    padding-left: 12px;
    font-family: var(--k-font-read);
    font-size: 0.9rem;
    line-height: 1.6;
    color: var(--k-ink-3);
  }

  /* What the agent DID, as a log: a run of these has to be scannable in one
   * glance and must not compete with what the agent SAID. They were bordered,
   * filled pills, one per tool call, so five commands looked like five
   * announcements. Now: a gutter rule, the tool name muted, the argument
   * highlighted when it is code, and the whole line one notch smaller than
   * prose. */
  .tool {
    display: flex;
    align-items: baseline;
    gap: 8px;
    max-width: 100%;
    margin: 0 0 1px;
    padding: 1px 0 1px 9px;
    border-left: 2px solid var(--k-edge);
    font-family: var(--k-font-mono);
    font-size: 0.7rem;
    color: var(--k-ink-2);
    line-height: 1.5;
  }
  .tool:hover {
    border-left-color: var(--k-edge-strong, var(--k-accent));
  }
  .tool.bad {
    border-left-color: var(--k-error);
  }
  .tool .k {
    flex: 0 0 auto;
    color: var(--k-ink-3);
  }
  .tool .arg {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .tool .mark {
    flex: 0 0 auto;
    color: color-mix(in srgb, var(--k-success) 70%, transparent);
  }
  .tool.bad .mark {
    color: var(--k-error);
  }

  .result {
    margin: 0;
    padding: 9px 11px;
    max-height: 220px;
    overflow: auto;
    background: var(--k-surface-2);
    border: 1px solid var(--k-edge);
    border-radius: calc(var(--k-radius) * 0.8);
    font-family: var(--k-font-mono);
    font-size: 0.72rem;
    line-height: 1.55;
    color: var(--k-ink-2);
  }

  /* Everything a heading, list, table, alert, chart or code block looks like is
   * in app.css (.k-md), shared with the card and the viewer. A turn keeps its own
   * reading font and a tighter rhythm - a conversation is read in scrolls, not
   * pages. */
  .prose {
    font-family: var(--k-font-read);
    font-size: 1rem;
    line-height: 1.66;
  }
  .prose :global(p),
  .prose :global(ul),
  .prose :global(ol),
  .prose :global(blockquote) {
    margin-bottom: 0.7em;
  }
  .prose :global(h1),
  .prose :global(h2),
  .prose :global(h3) {
    margin: 0.9em 0 0.35em;
  }
  .prose :global(h1) {
    font-size: 1.2rem;
  }
  .prose :global(h2) {
    font-size: 1.08rem;
    border-bottom: 0;
  }
  .prose :global(h3) {
    font-size: 0.98rem;
  }
  .prose :global(table) {
    font-size: 0.82rem;
  }
  .prose :global(th) {
    font-size: 0.68rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--k-ink-3);
    font-weight: 400;
    background: none;
  }
  .prose :global(th),
  .prose :global(td) {
    border: 0;
    border-bottom: 1px solid var(--k-edge-soft);
  }
  .prose :global(tbody tr:nth-child(even) td) {
    background: none;
  }

  .working {
    display: flex;
    align-items: center;
    gap: 8px;
    font-family: var(--k-font-mono);
    font-size: 0.68rem;
    color: var(--k-ink-3);
  }
  .pulse {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--k-accent);
    animation: breathe 1.4s ease-in-out infinite;
  }
  @keyframes breathe {
    50% {
      opacity: 0.25;
    }
  }

  /* The panel shares the composer's column, so the question, its answers and the
   * reply box all line up on the same measure.
   *
   * min-height: 0 is what lets it give way. A flex item's automatic minimum is
   * its content, so without this the panel keeps its full height whatever the
   * window does, the transcript goes to nothing, and the overflow comes off the
   * bottom - which is the composer. A long question would leave the human
   * looking at it with nowhere to type. The panel yields instead (its body
   * scrolls; see AskPanel), because a question you can still answer beats a
   * question you can read all of. */
  .askwrap {
    padding: 0 28px 10px;
    display: flex;
    min-height: 0;
  }
  .askwrap > .inner {
    display: flex;
    flex-direction: column;
    min-height: 0;
    width: 100%;
  }

  /* Never shrink: the reply box is the one thing on this surface that must stay
   * whole, and a clipped textarea reads as a broken window rather than a full
   * one. */
  .composer {
    border-top: 1px solid var(--k-edge-soft);
    padding: 12px 28px 14px;
    flex-shrink: 0;
  }
  .inner {
    max-width: var(--k-measure, 700px);
    margin: 0 auto;
  }
  .box {
    border: 1px solid var(--k-edge);
    border-radius: var(--k-radius);
    background: var(--k-surface-2);
    padding: 9px 11px;
    display: flex;
    flex-direction: column;
    gap: 7px;
  }
  .box:focus-within {
    border-color: color-mix(in srgb, var(--k-accent) 55%, transparent);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--k-accent) 14%, transparent);
  }
  textarea {
    width: 100%;
    border: 0;
    background: none;
    resize: none;
    font-family: var(--k-font-read);
    font-size: 0.95rem;
    line-height: 1.5;
    color: var(--k-ink);
    outline: none;
  }
  textarea::placeholder {
    color: var(--k-ink-3);
  }
  .row {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .hint {
    font-family: var(--k-font-mono);
    font-size: 0.62rem;
    color: var(--k-ink-3);
  }
  .send {
    padding: 4px 12px;
    border-radius: calc(var(--k-radius) * 0.7);
    border: 1px solid color-mix(in srgb, var(--k-accent) 55%, transparent);
    background: color-mix(in srgb, var(--k-accent) 16%, var(--k-surface-2));
    font-size: 0.8rem;
  }
  .send:disabled {
    opacity: 0.4;
    border-color: var(--k-edge);
    background: var(--k-surface-2);
    cursor: default;
  }
</style>
