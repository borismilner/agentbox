<script>
  import { bridge } from "../lib/bridge.js";
  import { trouble, forget } from "../lib/trouble.svelte.js";
  import { ticker } from "../lib/clock.svelte.js";
  import { markdown } from "../lib/markdown.svelte.js";
  import { parseDiff } from "../lib/diff.js";

  // The inbox (FR10): everything still waiting, then everything recent. Its
  // real job is triage (FR34) - after a meeting the backlog has to clear in
  // seconds, from the keyboard, without reading a manual. So the keys are the
  // same ones the card uses, the selected row states them out loud, and the
  // mouse is the fallback rather than the path.
  //
  // Which key does what to which item is decided in Go (Bridge.Triage), so this
  // surface and the card cannot drift apart about what "d" means.
  //
  // Its second job is reading (FR73). A row is a summary and truncates; opening
  // one asks Go for the whole item and shows it unabridged, because a card that
  // closed on its timer used to take its body with it. Nothing on this surface
  // may be the only copy of something an agent said.

  let { inbox } = $props();

  let query = $state("");
  let sel = $state(0);
  let typing = $state(false);
  let box = $state(null);
  let listEl = $state(null);

  // The opened row (FR73) and what Go said about it. `det` is kept beside `open`
  // rather than derived, because it arrives from a call: while it is in flight the
  // detail is open with nothing in it yet, and that is a state to paint.
  let open = $state(null);
  let det = $state(null);
  let detFor = $state(null); // the id `det` describes, so a stale reply is dropped

  const clock = ticker();

  // A resolved review's diff, parsed only when there is one. null rather than an
  // empty model so the template can ask one question: an item with no diff and a
  // diff that parsed to nothing both mean "no block here".
  const diffModel = $derived.by(() => {
    const m = det?.diff ? parseDiff(det.diff) : null;
    return m && m.files.length ? m : null;
  });

  const items = $derived(filter(inbox?.items ?? [], query));
  const pending = $derived(items.filter((i) => i.pending));
  const chosen = $derived(pending[Math.min(sel, pending.length - 1)] ?? null);

  // The queue moves on its own - an item resolves, another arrives - so the
  // selection is clamped against whatever is pending now, never a stale index.
  $effect(() => {
    if (sel > pending.length - 1) sel = Math.max(0, pending.length - 1);
  });

  $effect(() => {
    if (!chosen || !listEl) return;
    listEl.querySelector(`[data-id="${chosen.id}"]`)?.scrollIntoView({ block: "nearest" });
  });

  // The one place a detail is fetched, so opening a row and the queue moving under
  // an already-open one take the same path. Both cases are real: a row that LEFT
  // the list must not keep its detail open under whatever row now sits in that
  // place (the rule the Agents board learned the hard way), and a row that only
  // CHANGED - answered on its card, or triaged from the keyboard while open - has
  // to be re-read, or the detail goes on saying "waiting" and offering a card for
  // something already answered.
  $effect(() => {
    const rows = inbox?.items ?? [];
    if (!open) return;
    if (!rows.some((i) => i.id === open)) closeDetail();
    else load(open);
  });

  function closeDetail() {
    open = null;
    det = null;
    detFor = null;
  }

  async function load(id) {
    const got = await bridge.itemDetail(id);
    // The reply is only wanted if the row it describes is still the open one: two
    // fast clicks would otherwise paint the first answer under the second row.
    if (open !== id) return;
    det = got;
    detFor = id;
  }

  function toggleDetail(id) {
    if (open === id) {
      closeDetail();
      return;
    }
    open = id;
    det = null;
    detFor = null;
    // A failure that belonged to the row above is not news about this one.
    forget();
    // Clicking a pending row selects it too. Without this the keys still act on
    // wherever j/k was left, so reading one row and pressing d would dismiss
    // another - and a resolved row was inert before this, so nobody had reason to
    // click into the list and then type.
    const at = pending.findIndex((p) => p.id === id);
    if (at >= 0) sel = at;
    // The fetch is left to the effect above: `open` changing is what asks for it,
    // whether it changed from a click here or the row was refreshed under one.
  }

  function filter(list, q) {
    const needle = q.trim().toLowerCase();
    if (!needle) return list;
    return list.filter((i) =>
      [i.title, i.snippet, i.agent, i.project, i.kind, i.outcome].join(" ").toLowerCase().includes(needle),
    );
  }

  // TRIAGE_KEYS are the keys Go may act on. Listed here only so the surface
  // knows when to swallow the keystroke; the meaning stays on the Go side.
  const TRIAGE_KEYS = new Set(["1", "2", "3", "4", "5", "6", "7", "8", "9", "y", "n", "s", "d", "Enter", "Backspace"]);

  // What Go said about the last triage keystroke. `Triage` is the one method on
  // the whole answer path that returns anything (`internal/webui/inbox.go:588`),
  // and this surface used to throw it away: pressing a key that means nothing for
  // an item's kind, or pressing one on a row another window answered a moment ago,
  // looked exactly like pressing a key that worked. In a burst - where rows resolve
  // under the reader by design - that is not an edge case, it is Tuesday.
  let refused = $state(null); // {id, key}

  const KEY_NAMES = { Enter: "enter", Backspace: "backspace", " ": "space" };
  const keyName = (k) => KEY_NAMES[k] ?? k;

  async function triage(id, k) {
    // Clear first: a second press of a key that does nothing should re-state the
    // refusal rather than leave the first one sitting there looking stale.
    refused = null;
    // A call that never reached the daemon did nothing, which is the same thing
    // the reader needs to be told - and catching it is not optional now that this
    // is awaited: an unhandled rejection here would be U-01 all over again, in the
    // fix for U-03.
    const ok = await bridge.triage(id, k).catch(() => false);
    if (!ok) refused = { id, key: k };
  }

  function onKey(e) {
    const inField = e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement;

    if (e.key === "Escape") {
      if (inField || typing) {
        e.preventDefault();
        typing = false;
        box?.blur();
      } else if (open) {
        e.preventDefault();
        closeDetail();
      }
      return;
    }
    if (inField) return;

    // A held modifier means the keystroke was never triage. Nothing on this
    // surface uses one, and the meanings collide with the ones a desktop
    // already has: Ctrl+S is save everywhere, not snooze, and the shell walks
    // the rail with Ctrl+1..9 over whichever surface is in front. The copy key
    // below used to make this argument on its own; it holds for all of them.
    if (e.ctrlKey || e.metaKey || e.altKey) return;

    // A row is a real button, so Tab reaches it and Enter or Space opens its
    // detail - the only keyboard path to a resolved row, since triage's selection
    // only ever walks the pending run. Those two keys belong to the focused
    // button, not to the selected row: firing triage as well would answer one row
    // while opening another.
    if ((e.key === "Enter" || e.key === " ") && e.target instanceof HTMLButtonElement) return;

    if (e.key === "/") {
      e.preventDefault();
      typing = true;
      queueMicrotask(() => box?.focus());
      return;
    }
    if (e.key === "j" || e.key === "ArrowDown") {
      e.preventDefault();
      refused = null;
      sel = Math.min(sel + 1, Math.max(0, pending.length - 1));
      return;
    }
    if (e.key === "k" || e.key === "ArrowUp") {
      e.preventDefault();
      refused = null;
      sel = Math.max(sel - 1, 0);
      return;
    }
    if (!chosen) return;
    if (e.key === "c") {
      e.preventDefault();
      bridge.copyItem(chosen.id);
      return;
    }
    if (TRIAGE_KEYS.has(e.key)) {
      e.preventDefault();
      triage(chosen.id, e.key);
    }
  }

  // rel ages on the shared 1Hz tick, so "now" becomes "2m ago" without a
  // reload. Coarse on purpose: the exact second a question arrived never matters.
  function rel(msAt, now) {
    const d = Math.max(0, now - msAt);
    if (d < 60_000) return "now";
    if (d < 3_600_000) return `${Math.floor(d / 60_000)}m ago`;
    if (d < 86_400_000) return `${Math.floor(d / 3_600_000)}h ago`;
    return `${Math.floor(d / 86_400_000)}d ago`;
  }

  const sectionStart = (i) => i === 0 || items[i - 1].pending !== items[i].pending;
  const hint = $derived(
    typing ? "esc leaves search" : pending.length ? "j/k move · keys answer · / search" : "/ search",
  );
</script>

<svelte:window onkeydown={onKey} />

<section class="inbox">
  <header>
    <div class="line">
      <h1>Inbox</h1>
      {#if inbox?.pending}
        <span class="count">{inbox.pending} pending</span>
      {:else}
        <span class="count quiet">all quiet</span>
      {/if}
    </div>
    <div class="search" class:on={typing}>
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
        <circle cx="11" cy="11" r="6.5" /><path d="m16 16 4.5 4.5" />
      </svg>
      <input
        bind:this={box}
        bind:value={query}
        onfocus={() => (typing = true)}
        onblur={() => (typing = false)}
        placeholder="Search title, agent, kind…"
        spellcheck="false"
      />
      {#if query}
        <button class="clear" onclick={() => (query = "")} title="Clear">&#x2715;</button>
      {/if}
    </div>
  </header>

  <div class="list" bind:this={listEl}>
    {#if !(inbox?.items ?? []).length}
      <p class="empty">Nothing yet. When an agent has a question for you, it lands here.</p>
    {:else if !items.length}
      <p class="empty">No matches for “{query}”.</p>
    {:else}
      {#each items as it, i (it.id)}
        {#if sectionStart(i)}
          <span class="section">{it.pending ? "Pending" : "Recent"}</span>
        {/if}

        <div class="rowwrap" data-id={it.id} class:opened={open === it.id}>
          <button
            type="button"
            class="row {it.level}"
            class:pending={it.pending}
            class:on={!typing && chosen?.id === it.id}
            aria-expanded={open === it.id}
            onclick={() => toggleDetail(it.id)}
            title="Click to read it in full"
          >
            <span class="sev"></span>
            <span class="body">
              <span class="title">{it.title}</span>
              <span class="sub">
                <span class="idot" style="background: {it.hue}"></span>
                {it.agent}{it.project ? ` · ${it.project}` : ""} · {rel(it.createdMs, clock.now)}
                {#if it.muted}<em class="muted-badge">muted</em>{/if}
              </span>
            </span>
            <span class="outcome {it.tone}">{it.outcome}</span>
          </button>

          <!-- The hint is only honest while the keys are live: with the search
               box focused, "s stop" would type an s. -->
          {#if !typing && chosen?.id === it.id && refused?.id === it.id}
            <!-- Go declined the keystroke. It does not say which of the three
                 reasons it was, so neither does this: what the reader needs is
                 that nothing happened, which is the one thing the surface used
                 to leave them to guess. -->
            <span class="hint refused">{keyName(refused.key)} does nothing to this one</span>
          {:else if !typing && chosen?.id === it.id && it.hint}
            <span class="hint">{it.hint}</span>
          {/if}

          <!-- The detail (FR73). Nothing here is clamped, shortened or ellipsised:
               it exists because a card that closed on its timer used to be the
               only copy of what an agent said. -->
          {#if open === it.id}
            <div class="detail">
              {#if !det || detFor !== it.id}
                <p class="none">Reading it back…</p>
              {:else if !det.found}
                <p class="none">
                  This one has dropped out of the recent hundred, so there is nothing left here to
                  read back.
                </p>
              {:else}
                <div class="when">
                  <span>arrived <b>{det.createdAt}</b>, {rel(det.createdMs, clock.now)}</span>
                  {#if det.resolvedAt}
                    <span>
                      ended <b>{det.resolvedAt}</b>{det.took ? `, stood ${det.took}` : ""}
                    </span>
                  {/if}
                  <span class="grow"></span>
                  {#if det.pending}
                    <!-- Promoting stays one click from here: this is the row's own
                         detail, so the card is a button away rather than a
                         re-collapse and a second click on the row. -->
                    <button type="button" class="show" onclick={() => bridge.promote(det.id)}>
                      Show the card
                    </button>
                    <!-- U-01/U-02: the row can outlive the item behind it, which
                         is R-01's shape seen from here. A button that summons
                         nothing has to say so rather than look clicked. -->
                    {#if trouble.text}
                      <span class="trouble" role="alert">{trouble.text}</span>
                    {/if}
                  {/if}
                </div>

                {#if det.bodyHtml}
                  <div class="mdbody k-md selectable" use:markdown={det.bodyHtml}>{@html det.bodyHtml}</div>
                {:else}
                  <p class="none">This one was a title on its own; there was no body.</p>
                {/if}

                <!-- The diff a review asked about, parsed by the same lib/diff.js
                     the card used, so the change reads the way it read on screen.
                     No rail and no keyboard walk: this is a record being read
                     back, not a review being taken. Items raised before migration
                     0012 have no diff stored and simply show no block. -->
                {#if diffModel}
                  <div class="block">
                    <span class="label">The change it asked about <em class="stat">+{diffModel.add} −{diffModel.del}</em></span>
                    <div class="diff mono selectable">
                      {#each diffModel.files as f}
                        <div class="dfile">
                          <span class="dname">{f.name || "(unnamed)"}</span>
                          {#if f.badge}<em class="flag">{f.badge}</em>{/if}
                        </div>
                        {#each f.shown as l}
                          <div class="dline {l.cls}">{l.text}</div>
                        {/each}
                        {#if f.more > 0}
                          <div class="dline meta">… {f.more} more lines in this file</div>
                        {/if}
                      {/each}
                    </div>
                  </div>
                {/if}

                {#if det.speak}
                  <!-- What was said out loud when it arrived. Worth reading back
                       on its own: a reader who was in the room heard this and not
                       the title, and a reader who was not never had it at all. -->
                  <div class="block">
                    <span class="label">Said out loud</span>
                    <p class="gave spoken selectable">{det.speak}</p>
                  </div>
                {/if}

                {#if det.options?.length}
                  <div class="block">
                    <span class="label">It offered</span>
                    <ul class="opts">
                      {#each det.options as o}
                        <li class:chosen={o.chosen}>
                          <span class="lbl">{o.label}</span>
                          {#if o.default}<em class="flag">default</em>{/if}
                          {#if o.chosen}<em class="flag took">taken</em>{/if}
                          {#if o.desc}<span class="desc">{o.desc}</span>{/if}
                        </li>
                      {/each}
                    </ul>
                  </div>
                {/if}

                {#if det.answer || det.reply || det.values?.length}
                  <div class="block">
                    <span class="label">What went back</span>
                    {#if det.answer}<p class="gave selectable">{det.answer}</p>{/if}
                    {#if det.reply}<p class="gave typed selectable">{det.reply}</p>{/if}
                    {#if det.values?.length}
                      <dl class="vals selectable">
                        {#each det.values as v}
                          <dt>{v.label}</dt>
                          <dd>{v.value}</dd>
                        {/each}
                      </dl>
                    {/if}
                  </div>
                {/if}

                <dl class="meta selectable">
                  <dt>kind</dt>
                  <dd>{det.kind}</dd>
                  <dt>level</dt>
                  <dd>{det.level}</dd>
                  <dt>from</dt>
                  <dd>
                    <span class="idot" style="background: {det.hue}"></span>
                    {det.agent}{det.project ? ` · ${det.project}` : ""}{det.muted ? " (muted)" : ""}
                  </dd>
                  {#if det.session}
                    <dt>session</dt>
                    <dd class="mono">{det.session}</dd>
                  {/if}
                  <dt>id</dt>
                  <dd class="mono">{det.id}</dd>
                </dl>
              {/if}
            </div>
          {/if}
        </div>
      {/each}
    {/if}
  </div>

  <footer>
    <span>{hint}</span>
    <span
      >{inbox?.today ?? 0} interruption{(inbox?.today ?? 0) === 1 ? "" : "s"} today</span
    >
  </footer>
</section>

<style>
  .inbox {
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: 100%;
  }

  header {
    padding: 16px 18px 12px;
    border-bottom: 1px solid var(--k-edge-soft);
  }
  .line {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 12px;
  }
  h1 {
    margin: 0;
    font-size: 1.12rem;
    font-weight: 700;
    letter-spacing: -0.01em;
  }
  .count {
    font-family: var(--k-font-mono);
    font-size: 0.7rem;
    color: var(--k-info);
  }
  .count.quiet {
    color: var(--k-ink-3);
  }

  .search {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 10px;
    height: 34px;
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    background: var(--k-surface-2);
    color: var(--k-ink-3);
  }
  .search.on {
    border-color: color-mix(in srgb, var(--k-accent) 55%, var(--k-edge));
  }
  .search svg {
    width: 15px;
    height: 15px;
    flex: 0 0 auto;
  }
  .search input {
    flex: 1;
    min-width: 0;
    border: 0;
    outline: none;
    background: none;
    color: var(--k-ink);
    font: inherit;
    font-size: 0.84rem;
  }
  .search input::placeholder {
    color: var(--k-ink-3);
  }
  .clear {
    font-size: 0.7rem;
    color: var(--k-ink-3);
  }
  .clear:hover {
    color: var(--k-ink);
  }

  .list {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 4px 10px 14px;
  }
  .empty {
    margin: 44px 18px;
    text-align: center;
    font-family: var(--k-font-read);
    font-size: 0.9rem;
    color: var(--k-ink-3);
  }

  /* The label voice every surface shares (Home's h2): UI caps for headings,
   * mono only for data. */
  .section {
    display: block;
    padding: 14px 8px 5px;
    font-size: 0.76rem;
    font-weight: 600;
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--k-ink-3);
  }

  .rowwrap {
    display: flex;
    flex-direction: column;
  }
  .row {
    position: relative;
    display: flex;
    align-items: center;
    gap: 11px;
    width: 100%;
    text-align: left;
    padding: 9px 12px 9px 11px;
    border-radius: 9px;
    border: 1px solid transparent;
  }
  /* Every row opens now (FR73), so every row answers the pointer. Before this
   * a resolved row was inert and said so with the default cursor, which is
   * exactly the dead end the field request was about. */
  .row:hover {
    background: var(--k-surface-2);
  }
  .row.on {
    background: var(--k-surface-2);
    border-color: var(--k-edge);
  }
  .rowwrap.opened {
    margin: 4px 0 8px;
    border: 1px solid var(--k-edge);
    border-radius: 10px;
    background: var(--k-surface);
  }
  .rowwrap.opened .row {
    border-color: transparent;
    border-radius: 9px 9px 0 0;
  }

  /* The severity stripe, same device as the card's rail: level is a property of
   * the item, so it is stated in the one place saturation is allowed. */
  .sev {
    flex: 0 0 auto;
    width: 3px;
    align-self: stretch;
    min-height: 26px;
    border-radius: 2px;
    background: var(--k-ink-3);
  }
  .row.info .sev {
    background: var(--k-info);
  }
  .row.success .sev {
    background: var(--k-success);
  }
  .row.warning .sev {
    background: var(--k-warning);
  }
  .row.error .sev,
  .row.urgent .sev {
    background: var(--k-error);
  }
  .row:not(.pending) .sev {
    opacity: 0.42;
  }

  .body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }
  .title {
    font-size: 0.87rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .row:not(.pending) .title {
    color: var(--k-ink-2);
  }
  .sub {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 0.72rem;
    color: var(--k-ink-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .idot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex: 0 0 auto;
  }
  .muted-badge {
    font-style: normal;
    font-family: var(--k-font-mono);
    font-size: 0.6rem;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--k-warning);
  }

  .outcome {
    flex: 0 0 auto;
    max-width: 22ch;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.72rem;
    color: var(--k-ink-3);
  }
  .outcome.info {
    color: var(--k-info);
  }
  .outcome.success {
    color: var(--k-success);
  }
  .outcome.warning {
    color: var(--k-warning);
  }
  .outcome.error {
    color: var(--k-error);
  }

  .hint.refused {
    color: var(--k-warning, #d8a657);
  }
  .hint {
    padding: 0 12px 8px 27px;
    font-family: var(--k-font-mono);
    font-size: 0.68rem;
    color: var(--k-info);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* The detail (FR73). min-width: 0 is not decoration: this block holds a
   * rendered body, and a `1fr` grid track's automatic minimum is its content's
   * min-content width, so one unbreakable token in here would widen the whole
   * window. The shell pins its own track; this pins the block. */
  .detail {
    min-width: 0;
    padding: 4px 14px 14px 27px;
    border-top: 1px solid var(--k-edge-soft);
  }
  .none {
    margin: 10px 0 2px;
    font-family: var(--k-font-read);
    font-size: 0.84rem;
    color: var(--k-ink-3);
  }

  .when {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 14px;
    padding: 10px 0;
    font-family: var(--k-font-mono);
    font-size: 0.66rem;
    color: var(--k-ink-3);
  }
  .when b {
    font-weight: 500;
    color: var(--k-ink-2);
  }
  .grow {
    flex: 1;
  }
  .show {
    padding: 5px 11px;
    border: 1px solid color-mix(in srgb, var(--k-accent) 55%, var(--k-edge));
    border-radius: 7px;
    background: color-mix(in srgb, var(--k-accent) 16%, transparent);
    color: var(--k-ink);
    font-family: var(--k-font-ui);
    font-size: 0.74rem;
  }
  .show:hover {
    background: color-mix(in srgb, var(--k-accent) 28%, transparent);
  }
  /* U-01's line, worded and coloured the way the card says the same thing.
     Every var() carries a fallback: one that resolves to nothing takes its
     whole declaration with it, and a notice with no background still reads as
     fine to everything except the screen. */
  .trouble {
    padding: 4px 8px;
    border-radius: 6px;
    background: color-mix(in srgb, var(--k-warning, #d9a441) 16%, transparent);
    color: var(--k-ink, #dbe2ee);
    font-size: 0.74rem;
  }

  /* The body, and the one rule this whole surface exists for: no max-height, no
   * line clamp, no ellipsis. The list scrolls instead. */
  .mdbody {
    min-width: 0;
    margin: 4px 0 2px;
    font-family: var(--k-font-read);
    font-size: 0.88rem;
    line-height: 1.62;
    color: var(--k-ink);
  }

  .block {
    margin-top: 14px;
    min-width: 0;
  }
  .label {
    display: block;
    margin-bottom: 5px;
    font-size: 0.68rem;
    font-weight: 600;
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--k-ink-3);
  }
  .gave {
    margin: 0;
    font-family: var(--k-font-read);
    font-size: 0.86rem;
    color: var(--k-ink);
    overflow-wrap: anywhere;
  }
  .gave.typed {
    padding-left: 10px;
    border-left: 2px solid var(--k-edge);
    white-space: pre-wrap;
    color: var(--k-ink-2);
  }
  /* Marked as a quotation because it is one: these were words, not a value the
     reader chose. */
  .gave.spoken {
    padding-left: 10px;
    border-left: 2px solid color-mix(in srgb, var(--k-accent) 45%, transparent);
    font-style: italic;
    color: var(--k-ink-2);
  }

  /* The read-back diff. Scrolls in its own box on both axes: a detail sits inside
     a row in a list, and a long line of code must not widen the whole surface. */
  .diff {
    max-height: 22em;
    overflow: auto;
    border: 1px solid var(--k-edge);
    border-radius: 6px;
    background: var(--k-surface-2);
    font-size: 0.78rem;
    line-height: 1.5;
  }
  .stat {
    margin-left: 6px;
    font-style: normal;
    font-family: var(--k-font-mono);
    font-size: 0.9em;
    color: var(--k-ink-3);
  }
  .dfile {
    position: sticky;
    top: 0;
    display: flex;
    align-items: baseline;
    gap: 6px;
    padding: 4px 10px;
    background: var(--k-surface);
    border-bottom: 1px solid var(--k-edge);
    color: var(--k-ink-2);
  }
  .dname {
    overflow-wrap: anywhere;
  }
  .dline {
    padding: 0 10px;
    white-space: pre;
    color: var(--k-ink-2);
  }
  .dline.add {
    background: color-mix(in srgb, var(--k-success) 12%, transparent);
    color: var(--k-ink);
  }
  .dline.del {
    background: color-mix(in srgb, var(--k-error) 12%, transparent);
    color: var(--k-ink);
  }
  .dline.hunk {
    color: var(--k-accent);
    background: color-mix(in srgb, var(--k-accent) 8%, transparent);
  }
  .dline.meta {
    color: var(--k-ink-3);
    font-style: italic;
  }

  .opts {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .opts li {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 7px;
    font-size: 0.82rem;
    color: var(--k-ink-3);
  }
  .opts li.chosen .lbl {
    color: var(--k-ink);
    font-weight: 600;
  }
  .flag {
    font-style: normal;
    font-family: var(--k-font-mono);
    font-size: 0.6rem;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--k-ink-3);
  }
  .flag.took {
    color: var(--k-success);
  }
  .opts .desc {
    font-size: 0.76rem;
  }

  .vals,
  .meta {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: 3px 14px;
    margin: 0;
    font-size: 0.78rem;
  }
  .vals dt,
  .meta dt {
    font-family: var(--k-font-mono);
    font-size: 0.66rem;
    color: var(--k-ink-3);
    align-self: baseline;
  }
  .vals dd,
  .meta dd {
    margin: 0;
    min-width: 0;
    color: var(--k-ink-2);
    overflow-wrap: anywhere;
  }
  .vals dd {
    color: var(--k-ink);
    white-space: pre-wrap;
  }
  .meta {
    margin-top: 16px;
    padding-top: 12px;
    border-top: 1px solid var(--k-edge-soft);
  }
  .meta .mono {
    font-family: var(--k-font-mono);
    font-size: 0.7rem;
  }
  .meta .idot {
    display: inline-block;
    margin-right: 5px;
    vertical-align: baseline;
  }

  footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 14px;
    padding: 9px 18px;
    border-top: 1px solid var(--k-edge-soft);
    font-family: var(--k-font-mono);
    font-size: 0.64rem;
    color: var(--k-ink-3);
  }
</style>
