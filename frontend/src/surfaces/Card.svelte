<script>
  import IdentityPill from "../lib/IdentityPill.svelte";
  import { bridge, on } from "../lib/bridge.js";
  import { markdown } from "../lib/markdown.svelte.js";
  import { ticker, remaining } from "../lib/clock.svelte.js";
  import { parseDiff } from "../lib/diff.js";

  // The card is the product (03-ui-ux.md): one blocking item, answerable in
  // under two seconds without the mouse. Everything here serves that.

  let view = $state(null);
  let draft = $state("");
  let replying = $state(false);
  let formValues = $state({});
  let comment = $state("");
  let field = $state(null); // bound input element

  // Focus the first field of a form without binding a ref per row.
  const first = (node, yes) => { if (yes) queueMicrotask(() => node.focus()); };

  const clock = ticker();

  const item = $derived(view?.item ?? null);
  const kind = $derived(item?.kind ?? "notify");
  const level = $derived(item?.level || "info");
  const graced = $derived(view?.graced ?? false);

  const severity = $derived(
    { info: "var(--k-info)", success: "var(--k-success)", warning: "var(--k-warning)", error: "var(--k-error)", urgent: "var(--k-urgent)" }[level] ??
      "var(--k-info)",
  );

  const expiresIn = $derived(remaining(view?.expiresAtMs, clock.now));
  const graceIn = $derived(remaining(view?.graceUntilMs, clock.now));
  const canReply = $derived(!item?.strict && (kind === "choice" || kind === "confirm"));

  // A diff is read before it is approved, so it is rendered rather than dumped:
  // headers become file sections and a rail lists them once there is more than
  // one. The parser lives in lib/diff.js because the inbox detail reads a
  // resolved review back with the same one - see the note there.
  const diffModel = $derived.by(() => parseDiff((item?.kind === "diff" && item?.diff) || ""));
  const diffMulti = $derived(diffModel.files.length > 1);
  const diffStat = $derived(`+${diffModel.add} −${diffModel.del}`);

  // A stack card (FR30) collapses a flooding agent's burst. Collapsed it shows
  // the newest few, because the point of the card is that the human does NOT
  // have to read all fourteen; expanded it shows everything, because "nothing
  // was dropped" is only a claim until he can see them.
  //
  // Newest first is deliberate and is the opposite of the stored order: what an
  // agent said last is what its burst is currently about, and the first line of
  // a retry loop is the least interesting of the fourteen.
  const STACK_PEEK = 4;
  let stackOpen = $state(false);
  const stackAll = $derived([...(item?.stack ?? [])].reverse());
  const shownStack = $derived(stackOpen ? stackAll : stackAll.slice(0, STACK_PEEK));
  const stackHidden = $derived(stackAll.length - shownStack.length);
  // Only rows still waiting count. A question answered through its own row is
  // not one the footer should keep warning about, and the daemon marks it the
  // moment it resolves - by any door, including the inbox.
  const stackAsks = $derived(stackAll.filter((e) => e.blocking && !e.done).length);

  const baseOf = (p) => p.split("/").pop();
  const dirOf = (p) => p.slice(0, p.length - baseOf(p).length);

  // Where the reader is in the diff. The rail mirrors it: the current file is
  // marked, files already scrolled through go quiet, untouched ones stay bright -
  // remaining work is what should catch the eye.
  let curFile = $state(0);
  let seenFiles = $state(new Set([0]));
  let pane = $state(null);
  let secs = $state([]);

  function spy() {
    if (!pane || secs.length < 2) return;
    const top = pane.scrollTop + 48;
    let i = 0;
    for (let k = 0; k < secs.length; k++) if (secs[k] && secs[k].offsetTop <= top) i = k;
    if (i !== curFile) {
      curFile = i;
      seenFiles = new Set(seenFiles).add(i);
    }
  }

  function jumpFile(i) {
    const s = secs[i];
    if (!s || !pane) return;
    curFile = i;
    seenFiles = new Set(seenFiles).add(i);
    pane.scrollTo({ top: s.offsetTop, behavior: "smooth" });
  }

  // The review note grows with its content - a change request is often more
  // than a line - up to four rows, then scrolls. WebKitGTK has no
  // field-sizing yet, so measure by hand; Fit resizes the window around it.
  // The few px of slack cover scrollHeight's integer truncation.
  function autogrow(node) {
    const fit = () => {
      node.style.height = "auto";
      node.style.height = Math.min(node.scrollHeight + 6, 104) + "px";
    };
    fit();
    node.addEventListener("input", fit);
    return { update: fit, destroy: () => node.removeEventListener("input", fit) };
  }

  on("agentbox:view", (v) => {
    const fresh = v?.item?.id !== item?.id;
    view = v;
    if (fresh) {
      draft = "";
      comment = "";
      replying = false;
      formValues = Object.fromEntries((v?.item?.fields ?? []).map(initialValue));
      curFile = 0;
      seenFiles = new Set([0]);
      secs = [];
    }
    queueMicrotask(() => field?.focus?.());
  });

  bridge.ready("card");

  // A frameless window has to be exactly as tall as its content. Measure
  // after every layout-affecting change and let Go resize; ResizeObserver
  // catches the reflow that markdown, a reply box or a form row causes.
  let shell = $state(null);
  $effect(() => {
    if (!shell) return;
    const report = () => bridge.fit(Math.ceil(shell.getBoundingClientRect().height));
    const ro = new ResizeObserver(report);
    ro.observe(shell);
    report();
    return () => ro.disconnect();
  });

  function choose(i) {
    const opt = item?.options?.[i];
    if (opt) bridge.answer(item.id, opt.label);
  }

  // SPELL_AT is where a choice option stops fitting the closed select (FR84).
  // internal/webui.spelledAt carries the same number, because Go sizes the window
  // the card opens at and has to leave room for the extra line; a test fails if
  // the two drift, which is the lesson FR85 charged for on the identity hue.
  const SPELL_AT = 34;

  // What a form field starts as. A choice with no default has to start on its
  // first option: bound to "", a <select> renders with nothing chosen, which reads
  // as a broken card and submits a value that was never on the menu.
  function initialValue(f) {
    if (f.type === "bool") return [f.key, f.default === "true" || f.default === true];
    if (f.type === "choice") return [f.key, f.default || f.options?.[0] || ""];
    return [f.key, f.default ?? ""];
  }

  // Every value crosses the bridge as a string, because the method behind it takes
  // map[string]string. A checkbox binds a real boolean, and one boolean in that map
  // failed the whole call - so a form with a checkbox in it could never be
  // submitted, and the card closed as if nothing had happened.
  function formStrings() {
    return Object.fromEntries(Object.entries(formValues).map(([k, v]) => [k, typeof v === "boolean" ? String(v) : String(v ?? "")]));
  }

  function onKey(e) {
    if (!item) return;
    const typing = e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement;

    if (e.key === "Escape") {
      e.preventDefault();
      // In a review's note, Esc hands the keyboard back (a/r answer again);
      // it takes a second Esc to defer. In the note Enter is a newline, so
      // blur is the only way back to the single-key answers.
      if (typing && kind === "diff") {
        e.target.blur();
        return;
      }
      if (replying) {
        replying = false;
        return;
      }
      // Esc DISMISSES a notification and defers everything else. Deferring is
      // "not now, ask me again", which is the right answer to a question and a
      // trap on a notification: there is nothing to answer, so the item stays
      // pending and escalation raises it again - every 20 seconds at urgent.
      // Boris, on two urgent notifies from another agent: "No matter how many
      // times I press Esc, it pops back up." He was pressing the only key the
      // card named, and it was the one that could not end this.
      //
      // ⇧Esc still forces dismiss on every kind, because a question you want off
      // the queue without answering it needs a key too.
      //
      // A stack card (FR30) goes with notify: it is a summary of a burst and not
      // a question, so deferring it would put a card nobody can answer back in
      // the queue. Dismissing takes the notifications inside it with it and
      // leaves the questions pending - the daemon's rule, not this surface's.
      if (e.shiftKey || kind === "notify" || kind === "stack") bridge.dismiss(item.id);
      else bridge.defer(item.id);
      return;
    }
    if (graced && (e.key === "u" || e.key === "U") && !typing) {
      e.preventDefault();
      bridge.undo(item.id);
      return;
    }
    if (typing) {
      const submit = kind === "text" && item.multiline ? e.key === "Enter" && (e.ctrlKey || e.metaKey) : e.key === "Enter" && !e.shiftKey;
      if (submit) {
        e.preventDefault();
        send();
      }
      return;
    }

    if (e.key === "/" && canReply) {
      e.preventDefault();
      replying = true;
      queueMicrotask(() => field?.focus?.());
      return;
    }
    if (e.key === "c" && !e.ctrlKey) {
      e.preventDefault();
      bridge.copy(item.id);
      return;
    }
    if (kind === "choice" && /^[1-9]$/.test(e.key)) {
      e.preventDefault();
      choose(Number(e.key) - 1);
      return;
    }
    // A stack card's rows answer to the number row too, so a question buried in
    // a flood is still reachable without the mouse. Only the rows on screen are
    // numbered: pressing 4 on a collapsed list must not open something the card
    // is not showing.
    if (kind === "stack" && /^[1-9]$/.test(e.key)) {
      e.preventDefault();
      const row = shownStack[Number(e.key) - 1];
      if (row && !row.done) bridge.openStacked(item.id, row.id);
      return;
    }
    if (kind === "stack" && (e.key === "e" || e.key === "E")) {
      e.preventDefault();
      stackOpen = !stackOpen;
      return;
    }
    if (kind === "confirm" && (e.key === "y" || e.key === "n")) {
      e.preventDefault();
      bridge.confirm(item.id, e.key === "y");
      return;
    }
    if (kind === "veto" && (e.key === "Enter" || e.key === " ")) {
      e.preventDefault();
      bridge.veto(item.id);
      return;
    }
    // A review is answerable without the mouse like everything else. The comment
    // box is deliberately not focused on arrival, so these keys work the moment
    // the card appears - the note is the exception, not the point.
    if (kind === "diff" && (e.key === "a" || e.key === "Enter")) {
      e.preventDefault();
      bridge.review(item.id, true, comment);
      return;
    }
    if (kind === "diff" && e.key === "r") {
      e.preventDefault();
      bridge.review(item.id, false, comment);
      return;
    }
    if (kind === "diff" && diffMulti && (e.key === "n" || e.key === "p")) {
      e.preventDefault();
      jumpFile(e.key === "n" ? Math.min(curFile + 1, diffModel.files.length - 1) : Math.max(curFile - 1, 0));
      return;
    }
    if (e.key === "Enter" && item.default) {
      e.preventDefault();
      bridge.answer(item.id, item.default);
    }
  }

  function send() {
    if (!item) return;
    if (replying) return bridge.reply(item.id, draft);
    if (kind === "text") return bridge.reply(item.id, draft);
    if (kind === "secret") return bridge.secret(item.id, draft);
    if (kind === "form") return bridge.answerForm(item.id, formStrings());
  }
</script>

<svelte:window on:keydown={onKey} />

{#if item}
  <div class="card" class:urgent={level === "urgent"} style="--sev: {severity}" bind:this={shell}>
    <span class="rail"></span>

    {#if graced}
      <!-- FR28: answered, but the answer has not shipped yet. -->
      <div class="strip">
        <span class="tick">✓</span>
        <span class="what selectable">{view.gracedText}</span>
        <span class="spacer"></span>
        <span class="sending">sending in {graceIn}</span>
        <button class="undo" onclick={() => bridge.undo(item.id)}>Undo <kbd>u</kbd></button>
      </div>
    {:else}
      <header>
        <IdentityPill agent={item.identity?.agent} project={item.identity?.project} session={item.identity?.session} />
        {#if view.caller === "gone"}<span class="caller" title="the caller disconnected; your answer reaches history only">caller gone</span>{/if}
        <span class="spacer"></span>
        <!-- The hint says what Esc will actually do on THIS card, which is not
             the same on all of them: on a notification it dismisses (there is
             nothing to answer, so deferring only brings it back), on anything
             else it defers and ⇧Esc is the way off the queue. Only "Esc defer"
             was ever written down, and on a notify that was the one key that
             could not end the thing. -->
        {#if kind === "notify" || kind === "stack"}
          <span class="hint"><kbd>Esc</kbd> dismiss</span>
        {:else}
          <span class="hint"><kbd>Esc</kbd> defer · <kbd>⇧Esc</kbd> dismiss</span>
        {/if}
      </header>

      <h1 class="selectable">{item.title}</h1>

      {#if view.bodyHtml}
        <div class="body k-md selectable" use:markdown={view.bodyHtml}>{@html view.bodyHtml}</div>
      {/if}

      <div class="answer">
        {#if kind === "choice" && !replying}
          <div class="opts">
            {#each item.options as opt, i}
              <button class="opt" class:primary={item.default === opt.label} onclick={() => choose(i)}>
                <kbd>{i + 1}</kbd>
                <span class="lbl">
                  {opt.label}
                  {#if opt.desc}<em>{opt.desc}</em>{/if}
                </span>
              </button>
            {/each}
          </div>
        {:else if kind === "confirm" && !replying}
          <div class="opts">
            <button class="opt primary" onclick={() => bridge.confirm(item.id, true)}><kbd>y</kbd><span class="lbl">Yes</span></button>
            <button class="opt" onclick={() => bridge.confirm(item.id, false)}><kbd>n</kbd><span class="lbl">No</span></button>
          </div>
        {:else if kind === "veto"}
          <button class="veto" onclick={() => bridge.veto(item.id)}>
            <span class="stop">Stop</span>
            <span class="sub">{item.title} in {expiresIn}</span>
          </button>
        {:else if kind === "form"}
          <div class="fields">
            {#each item.fields as f, i}
              <label class="fieldrow">
                <span class="name">{f.label || f.key}</span>
                {#if f.type === "bool"}
                  <input type="checkbox" bind:checked={formValues[f.key]} use:first={i === 0} />
                {:else if f.type === "choice"}
                  <select bind:value={formValues[f.key]} title={formValues[f.key]}>
                    {#each f.options ?? [] as o}<option value={o}>{o}</option>{/each}
                  </select>
                  <!-- FR84: a closed select clips its option at about 38 characters
                       with no wrap and no tooltip, so three sentence-length options
                       looked identical and the card was answerable only because the
                       body spelled them out. Boris picked this shape over a radio
                       list: the control stays compact and the chosen option is
                       spelled out under it, wrapped, whenever the line cannot hold
                       it. The ResizeObserver above regrows the window when the
                       selection changes. -->
                  {#if (formValues[f.key] ?? "").length > SPELL_AT}
                    <span class="spelled">{formValues[f.key]}</span>
                  {/if}
                {:else}
                  <input class="text" type={f.type === "secret" ? "password" : "text"} bind:value={formValues[f.key]} use:first={i === 0} />
                {/if}
              </label>
            {/each}
            <div class="row"><span class="spacer"></span><button class="go" onclick={send}>Submit</button></div>
          </div>
        {:else if kind === "diff"}
          <div class="review">
            <div class="rev" class:railed={diffMulti}>
              {#if diffMulti}
                <nav class="rfiles" aria-label="Changed files">
                  {#each diffModel.files as f, i}
                    <button class="rfile" class:cur={i === curFile} class:seen={seenFiles.has(i) && i !== curFile} title={f.name} onclick={() => jumpFile(i)}>
                      <span class="rname">{baseOf(f.name)}</span>
                      <span class="rmeta">
                        {#if seenFiles.has(i) && i !== curFile}<span class="tick">✓</span>{/if}
                        <span class="plus">+{f.add}</span>
                        <span class="minus">−{f.del}</span>
                      </span>
                    </button>
                  {/each}
                </nav>
              {/if}
              <div class="diff">
                <div class="dscroll selectable" bind:this={pane} onscroll={spy}>
                  {#each diffModel.files as f, i}
                    <section class="fsec" bind:this={secs[i]}>
                      {#if f.name || f.badge}
                        <div class="fhead">
                          <span class="fpath"><span class="dir">{dirOf(f.name)}</span>{baseOf(f.name)}</span>
                          {#if f.badge}<span class="fbadge {f.badge}">{f.badge}</span>{/if}
                          <span class="spacer"></span>
                          <span class="fstat"><span class="plus">+{f.add}</span> <span class="minus">−{f.del}</span></span>
                        </div>
                      {/if}
                      {#each f.shown as l}<div class="dl {l.cls}">{l.text}</div>{/each}
                      {#if f.more}<div class="dl meta">… {f.more} more lines in this file</div>{/if}
                    </section>
                  {/each}
                </div>
              </div>
            </div>
            <textarea class="text note" rows="1" placeholder="a note back (optional)" bind:value={comment} use:autogrow={comment}></textarea>
            <div class="row">
              <span class="hint">
                {diffStat}
                {#if diffMulti}· {curFile + 1}/{diffModel.files.length} files <kbd>n</kbd><kbd>p</kbd>{/if}
                · <kbd>a</kbd> approve <kbd>r</kbd> changes
              </span>
              <span class="spacer"></span>
              <button class="opt" onclick={() => bridge.review(item.id, false, comment)}><span class="lbl">Request changes</span></button>
              <button class="go" onclick={() => bridge.review(item.id, true, comment)}>Approve</button>
            </div>
          </div>
        {:else if kind === "secret"}
          <div class="secretbox">
            <input class="text" type="password" placeholder="value" bind:value={draft} bind:this={field} />
            <p class="sink">
              {#if item.stdout}<span class="warn">returned to the agent's context</span>{:else}written to <code>{item.sink}</code>{/if}
            </p>
            <div class="row"><span class="spacer"></span><button class="go" onclick={send}>Send</button></div>
          </div>
        {:else if kind === "stack"}
          <!-- FR30. Every row is a real pending item; clicking one puts it back
               on screen as its own card. A row that is a QUESTION says so, and
               says it in the strongest thing on the card, because a burst that
               swallowed something an agent is parked on is the one case where
               the collapse could cost more than it saves. -->
          <div class="stack">
            {#each shownStack as e, i}
              <button class="srow" class:ask={e.blocking && !e.done} class:done={e.done} disabled={e.done} onclick={() => bridge.openStacked(item.id, e.id)}>
                <kbd>{i + 1}</kbd>
                <span class="sdot" style="--sev: var(--k-{e.level || 'info'}, var(--k-info))"></span>
                <span class="slbl">{e.title}</span>
                {#if e.done}<span class="sdone">done</span>{:else if e.blocking}<span class="sask">waiting on you</span>{/if}
              </button>
            {/each}
            {#if stackHidden > 0}
              <button class="smore" onclick={() => (stackOpen = true)}>… {stackHidden} more <kbd>e</kbd></button>
            {:else if stackAll.length > STACK_PEEK}
              <button class="smore" onclick={() => (stackOpen = false)}>show fewer <kbd>e</kbd></button>
            {/if}
          </div>
        {:else if kind === "notify"}
          {#if view.actionsEnabled && item.actions?.length}
            <div class="opts">
              {#each item.actions as a, i}
                <button class="opt" title={a.exec} onclick={() => bridge.runAction(item.id, i)}><span class="lbl">{a.label}</span></button>
              {/each}
            </div>
          {/if}
        {:else}
          <!-- text, or a choice/confirm that opened its reply hatch -->
          <div class="textbox">
            {#if item.multiline}
              <textarea rows="4" bind:value={draft} bind:this={field} placeholder={replying ? "reply instead…" : "your answer"}></textarea>
            {:else}
              <input class="text" bind:value={draft} bind:this={field} placeholder={replying ? "reply instead…" : "your answer"} />
            {/if}
            <div class="row">
              <span class="hint">{item.multiline ? "Ctrl+Enter sends" : "Enter sends"}</span>
              <span class="spacer"></span>
              <button class="go" onclick={send}>Send</button>
            </div>
          </div>
        {/if}
      </div>

      <footer>
        {#if expiresIn && kind !== "veto"}<span>expires in {expiresIn}</span>{/if}
        {#if item.default}<span>default: {item.default}</span>{/if}
        {#if canReply && !replying}<span><kbd>/</kbd> reply</span>{/if}
        <!-- What Esc costs on a stack card, said before it is pressed: the
             notifications go, the questions do not. Without it, dismissing a
             card that says "14 notifications" looks like abandoning whatever
             agent is parked inside it. -->
        {#if kind === "stack"}
          <span
            >{#if stackAsks > 0}{stackAsks} still waiting for an answer; dismissing keeps {stackAsks === 1 ? "it" : "them"}{:else}dismissing clears all of them{/if}</span
          >
        {/if}
        <span class="spacer"></span>
        {#if view.waiting > 0}
          <span class="waiting">
            {view.waiting} waiting
            {#each view.waitingHues as h}<i style="background: {h}"></i>{/each}
          </span>
        {/if}
      </footer>
    {/if}
  </div>
{/if}

<style>
  .card {
    position: relative;
    min-height: 100%;
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 13px 16px 0 20px;
    background: var(--k-surface);
    border: 1px solid var(--k-edge);
    border-radius: var(--k-radius);
    box-shadow: var(--k-shadow);
    overflow: hidden;
    animation: rise 140ms cubic-bezier(0.2, 0.9, 0.3, 1);
  }
  @keyframes rise {
    from {
      opacity: 0;
      transform: translateY(6px) scale(0.985);
    }
  }

  .rail {
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 3px;
    background: var(--sev);
  }
  .urgent .rail {
    animation: pulse 1.8s ease-in-out infinite;
  }
  @keyframes pulse {
    50% {
      opacity: 0.35;
    }
  }

  header {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .spacer {
    flex: 1;
  }
  .hint,
  .caller {
    font-family: var(--k-font-mono);
    font-size: 0.66rem;
    color: var(--k-ink-3);
  }
  .caller {
    color: var(--k-warning);
  }

  kbd {
    font-family: var(--k-font-mono);
    font-size: 0.66rem;
    line-height: 1.35;
    padding: 1px 5px;
    border-radius: 4px;
    background: var(--k-surface-3);
    border: 1px solid var(--k-edge);
    color: var(--k-ink-3);
  }

  h1 {
    margin: 0;
    font-size: 1.12rem;
    font-weight: 700;
    letter-spacing: -0.01em;
    line-height: 1.25;
    text-wrap: balance;
  }

  .body {
    font-family: var(--k-font-read);
    font-size: 0.94rem;
    line-height: 1.55;
    color: var(--k-ink-2);
    /* A px cap, not vh: Fit sizes the window from this element's height, so a
     * viewport-relative cap would be measuring itself. */
    max-height: 420px;
    overflow-y: auto;
    overflow-wrap: anywhere;
  }
  /* Everything the body renders is styled by .k-md (app.css), shared with the
   * viewer and the session. A card only tightens the spacing: it is answering a
   * question, not laying out a page. */
  .body :global(p),
  .body :global(ul),
  .body :global(ol),
  .body :global(blockquote) {
    margin-bottom: 0.6em;
  }
  .body :global(h1),
  .body :global(h2),
  .body :global(h3) {
    margin-top: 0.9em;
    font-size: 1.05em;
  }
  .body :global(h2) {
    border-bottom: 0;
  }

  .answer {
    margin-top: 2px;
  }
  .opts {
    display: flex;
    flex-wrap: wrap;
    gap: 7px;
  }
  .opt {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 6px 12px 6px 7px;
    border: 1px solid var(--k-edge);
    border-radius: calc(var(--k-radius) * 0.72);
    background: var(--k-surface-2);
    font-size: 0.86rem;
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
    background: color-mix(in srgb, var(--k-accent) 12%, var(--k-surface-2));
  }
  .opt .lbl {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    line-height: 1.25;
  }
  .opt em {
    font-style: normal;
    font-size: 0.74rem;
    color: var(--k-ink-3);
  }

  /* FR30 stack card. Rows, not buttons in a row: this is a list to read down,
     and the burst it collapses can be fourteen long. Every var() carries a
     fallback because a var() that resolves to nothing takes its whole
     declaration with it, and a row that has silently lost its background still
     reads as working to everything except the screen. */
  .stack {
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .srow {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 5px 9px 5px 6px;
    border: 1px solid transparent;
    border-radius: calc(var(--k-radius, 8px) * 0.6);
    background: var(--k-surface-2, rgba(127, 127, 127, 0.08));
    font-size: 0.82rem;
    text-align: left;
    transition: background 90ms ease, border-color 90ms ease;
  }
  .srow:hover {
    background: var(--k-surface-3, rgba(127, 127, 127, 0.16));
    border-color: color-mix(in srgb, var(--k-ink-3, #888) 45%, var(--k-edge, #444));
  }
  .srow .sdot {
    flex: none;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--sev, var(--k-info, #6aa9ff));
  }
  .srow .slbl {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  /* A question inside a burst is the one row that must not read like the rest:
     an agent is parked on it right now. */
  .srow.ask {
    border-color: color-mix(in srgb, var(--k-warning, #d9a441) 45%, transparent);
    background: color-mix(in srgb, var(--k-warning, #d9a441) 10%, var(--k-surface-2, rgba(127, 127, 127, 0.08)));
  }
  .srow .sask {
    flex: none;
    font-size: 0.7rem;
    color: var(--k-warning, #d9a441);
  }
  /* A row that has been dealt with. It stays in the list - the burst is a record
     of what was sent, and a list that reflows under the pointer is how the wrong
     thing gets clicked - but it stops asking for anything. */
  .srow.done {
    opacity: 0.45;
    cursor: default;
  }
  .srow.done:hover {
    background: var(--k-surface-2, rgba(127, 127, 127, 0.08));
    border-color: transparent;
  }
  .srow .sdone {
    flex: none;
    font-size: 0.7rem;
    color: var(--k-ink-3, #8a8a8a);
  }
  .smore {
    align-self: flex-start;
    padding: 3px 6px;
    font-size: 0.76rem;
    color: var(--k-ink-3, #8a8a8a);
    background: none;
    border: none;
  }
  .smore:hover {
    color: var(--k-ink-1, #e8e8e8);
  }

  .veto {
    width: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 2px;
    padding: 12px;
    border-radius: calc(var(--k-radius) * 0.8);
    border: 1px solid color-mix(in srgb, var(--k-error) 55%, transparent);
    background: color-mix(in srgb, var(--k-error) 14%, var(--k-surface-2));
  }
  .veto:hover {
    background: color-mix(in srgb, var(--k-error) 22%, var(--k-surface-2));
  }
  .veto .stop {
    font-size: 1.05rem;
    font-weight: 700;
  }
  .veto .sub {
    font-size: 0.76rem;
    color: var(--k-ink-2);
  }

  .fields,
  .textbox,
  .secretbox,
  .review {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  /* The review pane: a rail of file steps beside the diff once there is more
   * than one file. Unseen files stay bright and the seen ones go quiet - the
   * remaining work is what should catch the eye. */
  .rev.railed {
    display: grid;
    grid-template-columns: 150px minmax(0, 1fr);
    gap: 8px;
    align-items: start;
  }
  .rfiles {
    display: flex;
    flex-direction: column;
    gap: 2px;
    max-height: 460px;
    overflow-y: auto;
  }
  .rfile {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 1px;
    width: 100%;
    padding: 5px 9px;
    border-left: 2px solid var(--k-edge);
    border-radius: 0 7px 7px 0;
    text-align: left;
    font-family: var(--k-font-mono);
    color: var(--k-ink-2);
    transition: background 90ms ease, color 90ms ease, border-color 90ms ease;
  }
  .rfile:hover {
    background: var(--k-surface-2);
    color: var(--k-ink);
  }
  .rfile.cur {
    border-left-color: var(--k-accent);
    background: color-mix(in srgb, var(--k-accent) 9%, transparent);
    color: var(--k-ink);
  }
  .rfile.seen {
    color: var(--k-ink-3);
  }
  .rname {
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.74rem;
  }
  .rmeta {
    display: flex;
    gap: 6px;
    font-size: 0.64rem;
  }
  .tick {
    color: var(--k-success);
  }
  .plus {
    color: var(--k-success);
  }
  .minus {
    color: var(--k-error);
  }

  /* The diff pane. Border and radius live on the outer box, scrolling on the
   * inner one: WebKitGTK's scrollbar paints over a rounded scroll container's
   * own bottom edge (field-tested: the note below lost its top border to it),
   * and splitting the two keeps the scrollbar inside the frame. The px cap is
   * for the same reason .body has one: Go sizes the window from the measured
   * height, so a viewport-relative cap would be measuring itself. Taller when
   * the rail is up: a multi-file review is a reading task, not a glance. */
  .diff {
    border: 1px solid var(--k-edge);
    border-radius: calc(var(--k-radius) * 0.7);
    background: var(--k-surface-2);
    overflow: hidden;
    font-family: var(--k-font-mono);
    font-size: 0.76rem;
    line-height: 1.5;
  }
  .dscroll {
    position: relative;
    max-height: 320px;
    overflow: auto;
    padding: 0 0 6px;
  }
  .railed .dscroll {
    max-height: 460px;
  }
  /* The anchors a reader navigates by, brightest structure on the pane: the
   * file header sticks while its section scrolls, so there is never a line on
   * screen whose file you cannot see. */
  .fhead {
    position: sticky;
    top: 0;
    z-index: 1;
    display: flex;
    align-items: center;
    gap: 8px;
    margin-top: 8px;
    padding: 5px 10px;
    background: var(--k-surface-3);
    border-top: 1px solid var(--k-edge-soft);
    border-bottom: 1px solid var(--k-edge-soft);
    font-size: 0.74rem;
    font-weight: 600;
    color: var(--k-ink);
  }
  .fsec:first-child .fhead {
    margin-top: 0;
    border-top: 0;
  }
  .fhead .fpath {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .fhead .dir {
    color: var(--k-ink-3);
    font-weight: 400;
  }
  .fhead .fstat {
    font-size: 0.68rem;
    font-weight: 500;
    white-space: nowrap;
  }
  .fbadge {
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 0.62rem;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--k-accent);
    background: color-mix(in srgb, var(--k-accent) 12%, transparent);
  }
  .fbadge.new {
    color: var(--k-success);
    background: color-mix(in srgb, var(--k-success) 12%, transparent);
  }
  .fbadge.deleted {
    color: var(--k-error);
    background: color-mix(in srgb, var(--k-error) 12%, transparent);
  }
  .dl {
    padding: 0 10px;
    white-space: pre;
    color: var(--k-ink-2);
  }
  .fsec:first-child > .dl:first-child {
    margin-top: 6px;
  }
  .dl.add {
    color: var(--k-success);
    background: color-mix(in srgb, var(--k-success) 12%, transparent);
  }
  .dl.del {
    color: var(--k-error);
    background: color-mix(in srgb, var(--k-error) 12%, transparent);
  }
  .dl.hunk {
    margin-top: 6px;
    color: var(--k-accent);
    background: color-mix(in srgb, var(--k-accent) 10%, transparent);
  }
  .dl.hunk:first-child {
    margin-top: 0;
  }
  .dl.meta {
    color: var(--k-ink-3);
  }
  .fieldrow {
    display: grid;
    grid-template-columns: 120px 1fr;
    align-items: center;
    gap: 10px;
  }
  .fieldrow .name {
    font-size: 0.8rem;
    color: var(--k-ink-2);
  }
  /* The chosen option in full, under the control that clipped it (FR84). In the
     value column, wrapped, and quiet enough to read as an echo of the select
     rather than as a second field. */
  .fieldrow .spelled {
    grid-column: 2;
    margin-top: -2px;
    font-size: 0.78rem;
    line-height: 1.4;
    color: var(--k-ink-3);
    overflow-wrap: anywhere;
  }
  input.text,
  textarea,
  select {
    width: 100%;
    font: inherit;
    font-size: 0.9rem;
    color: var(--k-ink);
    background: var(--k-surface-2);
    border: 1px solid var(--k-edge);
    border-radius: calc(var(--k-radius) * 0.7);
    padding: 7px 10px;
    resize: none;
  }
  textarea.note {
    overflow-y: auto;
    line-height: 1.4;
    margin-top: 2px;
    padding-top: 8px;
  }
  input.text:focus,
  textarea:focus {
    outline: none;
    border-color: color-mix(in srgb, var(--k-accent) 62%, transparent);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--k-accent) 16%, transparent);
  }
  .row {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .go {
    padding: 5px 12px;
    border-radius: calc(var(--k-radius) * 0.7);
    border: 1px solid color-mix(in srgb, var(--k-accent) 55%, transparent);
    background: color-mix(in srgb, var(--k-accent) 16%, var(--k-surface-2));
    font-size: 0.84rem;
  }
  .sink {
    margin: 0;
    font-size: 0.76rem;
    color: var(--k-ink-3);
  }
  .sink .warn {
    color: var(--k-warning);
  }

  footer {
    margin: auto -16px 0 -20px;
    padding: 7px 16px 7px 20px;
    display: flex;
    align-items: center;
    gap: 10px;
    white-space: nowrap;
    border-top: 1px solid var(--k-edge-soft);
    background: color-mix(in srgb, var(--k-surface-3) 55%, transparent);
    font-family: var(--k-font-mono);
    font-size: 0.66rem;
    color: var(--k-ink-3);
  }
  .waiting {
    display: inline-flex;
    align-items: center;
    gap: 5px;
  }
  .waiting i {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    display: inline-block;
  }

  .strip {
    display: flex;
    align-items: center;
    gap: 10px;
    height: 100%;
    padding-right: 4px;
  }
  .strip .tick {
    color: var(--k-success);
  }
  .strip .what {
    font-size: 0.94rem;
  }
  .strip .sending {
    font-family: var(--k-font-mono);
    font-size: 0.68rem;
    color: var(--k-ink-3);
  }
  .undo {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    border: 1px solid var(--k-edge);
    border-radius: calc(var(--k-radius) * 0.7);
    background: var(--k-surface-2);
    font-size: 0.82rem;
  }
  .undo:hover {
    background: var(--k-surface-3);
  }
</style>
