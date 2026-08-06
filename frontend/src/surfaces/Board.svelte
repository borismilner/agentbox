<script>
  // The review board (FR58), slice 1: a stored walkthrough, walkable and
  // persistent. The daemon renders the spec (per-line chroma HTML plus
  // structured channels); this surface lays it out, collects verdicts,
  // closing notes and anchored comments, and writes every one of them
  // through the Board* keyhole as it happens. UX decisions come from the
  // mock's seven feedback rounds (tools/mockups/review-board.jsx history in
  // docs/07-field-requests.md) - the reading-room type scale, the worklist
  // rail, the verdict moment, Esc-discards, hover-never-shifts-layout.
  import { tick } from "svelte";
  import { bridge, on } from "../lib/bridge.js";
  import Glossary from "../lib/board/Glossary.svelte";
  import Rail from "../lib/board/Rail.svelte";
  import ModeToggle from "../lib/board/ModeToggle.svelte";
  import Step from "../lib/board/Step.svelte";
  import SubmitModal from "../lib/board/SubmitModal.svelte";

  let review = $state(null);
  let err = $state("");
  // A render error on a frameless window is otherwise a silent freeze; the
  // board wears it instead (and it reaches the daemon log via boardWrite
  // failures if the bridge is what broke).
  window.addEventListener("error", (e) => (err = "board error: " + e.message));
  window.addEventListener("unhandledrejection", (e) => (err = "board error: " + e.reason));
  let at = $state(0);
  let marks = $state({}); // stepId -> {verdict, note, revealed[]}
  let comments = $state([]);
  let zoom = $state(1);
  let pend = $state(null); // open composer, owned here so Esc precedence has one home

  // The window is frameless, so its buttons belong to this header. The board
  // opens maximised (board.go openWindow) and the window manager can change
  // that without telling the page, so the icon is read back from the window
  // instead of remembered - and a resize is the one signal both routes share.
  let maximised = $state(true);
  function syncMax() {
    bridge
      .isMaximisedSelf()
      .then((v) => (maximised = v))
      .catch(() => {});
  }
  syncMax();

  const counted = $derived((review?.steps ?? []).filter((s) => s.kind === "code"));
  const understood = $derived(counted.filter((s) => marks[s.id]?.verdict === "understood").length);
  const unclear = $derived(counted.filter((s) => marks[s.id]?.verdict === "unclear"));
  const step = $derived(review?.steps?.[at] ?? null);

  // Which group the reader is in, and where inside it. The board walks one
  // domain at a time; these are what the banner and the domain keys read.
  const domains = $derived(review?.domains ?? []);
  const domainAt = $derived(domains.findIndex((d) => at >= d.from && at <= d.to));
  const domain = $derived(domains[domainAt] ?? null);
  const inDomain = $derived(domain ? at - domain.from + 1 : 0);
  const ofDomain = $derived(domain ? domain.to - domain.from + 1 : 0);

  // [ and ] move by domain rather than by step, which is the navigation the
  // grouping exists to make possible: a reader who has decided a subject is not
  // theirs should not have to press → through five steps of it.
  function goDomain(delta) {
    if (!domains.length || domainAt < 0) return;
    const next = domains[Math.min(Math.max(domainAt + delta, 0), domains.length - 1)];
    if (next) goTo(next.from);
  }

  function seed(wb) {
    review = wb;
    marks = {};
    for (const [id, m] of Object.entries(wb.marks ?? {})) {
      marks[id] = { verdict: m.verdict ?? "", note: m.note ?? "", revealed: m.revealed ?? [] };
    }
    comments = wb.comments ?? [];
    at = Math.min(Math.max(wb.pos ?? 0, 0), (wb.steps?.length ?? 1) - 1);
  }

  bridge
    .board()
    .then(seed)
    .catch((e) => (err = String(e)));

  on("agentbox:board", (wb) => {
    if (!review || wb.revMs !== review.revMs) {
      const hold = at;
      seed(wb);
      at = Math.min(hold, wb.steps.length - 1);
    }
  });

  function mark(stepId) {
    return (marks[stepId] ??= { verdict: "", note: "", revealed: [] });
  }

  function setVerdict(stepId, v) {
    mark(stepId).verdict = v;
    bridge.boardVerdict(review.id, stepId, v).catch(() => {});
  }

  // The closing note debounces while typed and flushes on every edge that
  // could lose it: step change, blur, the page going away.
  let noteTimer = 0;
  let noteDirty = null; // stepId with unflushed text
  function setNote(stepId, text) {
    mark(stepId).note = text;
    noteDirty = stepId;
    clearTimeout(noteTimer);
    noteTimer = setTimeout(flushNote, 400);
  }
  function flushNote() {
    clearTimeout(noteTimer);
    if (!noteDirty || !review) return;
    bridge.boardNote(review.id, noteDirty, mark(noteDirty).note).catch(() => {});
    noteDirty = null;
  }
  window.addEventListener("pagehide", flushNote);
  window.addEventListener("visibilitychange", () => document.hidden && flushNote());

  function setRevealed(stepId, idx, open) {
    const m = mark(stepId);
    m.revealed = open ? [...m.revealed, idx] : m.revealed.filter((i) => i !== idx);
    bridge.boardReveal(review.id, stepId, m.revealed).catch(() => {});
  }

  let posTimer = 0;
  function goTo(i) {
    if (!review) return;
    flushNote();
    pend = null;
    at = Math.min(Math.max(i, 0), review.steps.length - 1);
    clearTimeout(posTimer);
    posTimer = setTimeout(() => bridge.boardPos(review.id, at).catch(() => {}), 300);
  }

  async function addComment(stepId, anchor, body) {
    const id = await bridge.boardCommentAdd(
      review.id, stepId, anchor.path ?? "", anchor.side ?? "new",
      anchor.from ?? 0, anchor.to ?? 0, anchor.exact ?? "", body,
    );
    comments = [...comments, { id, stepId, ...anchor, body, atMs: Date.now() }];
  }
  function editComment(id, body) {
    comments = comments.map((c) => (c.id === id ? { ...c, body } : c));
    bridge.boardCommentEdit(review.id, id, body).catch(() => {});
  }
  function deleteComment(id) {
    comments = comments.filter((c) => c.id !== id);
    bridge.boardCommentDelete(review.id, id).catch(() => {});
  }

  // FR65. The rejection is deliberately NOT caught here: OpenBtn shows it beside
  // the block it belongs to, and the board's own error state replaces the whole
  // review, which no failed click deserves.
  const onOpen = (path, line) => bridge.boardOpenInEditor(review.id, path, line);

  // Two reading modes for the whole review, not per step: a reader skimming is
  // skimming the review. It OPENS in brief, because the TL;DR is not the lossy
  // version - it is the same mastery laid out to be glanced at - so starting in
  // the full text makes every reader pay the long way in before they know
  // whether they need it. The choice lives for the window's life; it is a
  // reading posture, not an annotation, and nothing about it belongs in the
  // store beside the verdicts.
  let brief = $state(true);

  let noteFocus = $state(0); // bumped to ask the verdict box to focus the note
  let stepComposer = $state(0); // bumped to open the step-level composer
  let submit = $state(false); // the submit modal

  // The glossary drawer (FR68). null is closed; "" is open with nothing
  // singled out; a key is open at that entry. It is never opened for the
  // reader - a definition they did not ask for is an interruption - so every
  // path here starts with a click or a keypress.
  let glossaryAt = $state(null);
  const hasGlossary = $derived((review?.glossary ?? []).length > 0);
  // By key, so a marked word can show its one-liner on hover without asking
  // the daemon anything.
  const termsByKey = $derived(
    Object.fromEntries((review?.glossary ?? []).map((t) => [t.key, t])),
  );

  // The hover tip on a marked word, drawn here rather than in the step: a
  // fixed element rendered inside the scrolling column lands in that column's
  // paint order and the prose draws straight over it.
  let tip = $state(null); // {key, x, y, below}
  function onTermHover(key, rect) {
    if (!key || !rect || !termsByKey[key]) {
      tip = null;
      return;
    }
    const below = rect.top < 150;
    tip = {
      key,
      x: Math.min(Math.max((rect.left + rect.right) / 2, 190), window.innerWidth - 190),
      y: below ? rect.bottom + 8 : window.innerHeight - rect.top + 8,
      below,
    };
  }
  function openTerm(key) {
    glossaryAt = key;
  }
  function toggleGlossary() {
    if (!hasGlossary) return;
    glossaryAt = glossaryAt === null ? "" : null;
  }

  // A gate refusal lands here: show the offending step and put the keyboard
  // in its note, with the amber nudge (the same path the x key takes).
  async function jumpToStep(stepId) {
    submit = false;
    const i = review.steps.findIndex((s) => s.id === stepId);
    if (i < 0) return;
    goTo(i);
    await tick(); // the step must exist before the verdict box can hear the bump
    noteFocus++;
  }

  // Read-aloud (FR66, reshaped by FR72). A step is not one reading: it is a run
  // of prose, then a block, then the prose that hands over to the next block, and
  // a reader wants to hear one run, look at the code under it, and then ask for
  // the next. So each run is a region with its own control, and the voice reads
  // exactly the region that was pressed - never on into the next.
  //
  // Which region is playing is whatever the daemon last reported rather than a
  // guess made here; every call answers with its state.
  let readingRegion = $state(null);

  // The regions of a step, in the order they appear on the page. `intro` is the
  // title, the purpose and the prose above the first block; `lead:<n>` is the
  // handover paragraph above block n; `close` is the takeaway under the last
  // block; `checks` is the questions. Code blocks are left out on purpose -
  // reading punctuation and identifiers aloud is noise, and the prose is what
  // carries the argument.
  //
  // `lead` and `close` are here because they were missing: read-aloud was written
  // against the step shape that came before FR69 added them, so the takeaway - the
  // sentence a step exists to land - was never spoken at all.
  function paragraphs(segs) {
    const out = [];
    let para = [];
    for (const seg of segs ?? []) {
      if (seg.p && para.length) { out.push(para.join("")); para = []; }
      para.push(seg.t || seg.code || "");
    }
    if (para.length) out.push(para.join(""));
    return out;
  }

  function regionText(s, region) {
    if (!s) return "";
    if (region === "intro") {
      const out = [s.title];
      if (s.purpose) out.push(s.purpose);
      return out.concat(paragraphs(s.prose)).join("\n\n");
    }
    if (region === "close") return paragraphs(s.close).join("\n\n");
    if (region === "checks") return (s.checks ?? []).map((c) => c.q).join("\n\n");
    if (region.startsWith("lead:")) {
      const blk = (s.codes ?? [])[Number(region.slice(5))];
      return blk?.lead ?? "";
    }
    return "";
  }

  // One control per region, so pressing the one that is already playing stops it
  // and pressing any other one replaces it. There is no pause and no resume: both
  // needed the text split into passages, and the split is what cost the speech its
  // last words (FR72).
  async function aloud(region) {
    const stopping = region === null || readingRegion === region;
    const text = stopping ? "" : regionText(step, region);
    if (!stopping && !text.trim()) return;
    try {
      const res = await bridge.aloud(stopping ? "stop" : "start", region ?? "", text);
      readingRegion = res?.playing ? (res.region || region) : null;
    } catch {
      readingRegion = null;
    }
  }

  // A reading ends on its own, and nothing pushes that over the bridge, so the
  // control would stay painted as playing until the next press. Ask while a
  // reading is live and stop asking the moment it is not.
  $effect(() => {
    if (!readingRegion) return;
    const poll = setInterval(async () => {
      try {
        const res = await bridge.aloud("state", "", "");
        if (!res?.playing) readingRegion = null;
      } catch {
        readingRegion = null;
      }
    }, 700);
    return () => clearInterval(poll);
  });

  // Moving to another step stops the reading rather than leaving a voice reading
  // a page nobody is looking at any more.
  let lastRead = at;
  $effect(() => {
    if (at !== lastRead) {
      lastRead = at;
      if (readingRegion) aloud(null);
    }
  });

  function onKey(e) {
    if (submit) {
      // The modal owns the keyboard; Esc is "keep reviewing" (modal is last
      // in the Esc order - composers and pops close before it opens).
      if (e.key === "Escape") submit = false;
      return;
    }
    if (/INPUT|TEXTAREA/.test(e.target.tagName)) {
      if (e.key === "Escape") e.target.blur();
      return;
    }
    // A focused control owns the keys that ACTIVATE it. Without this the board
    // ran its own binding as well: Enter on a block header's button opened the
    // file and jumped to the next unread step, so the board moved under the
    // reader who pressed it. Found on FR65's open button, true of every button
    // on the surface since the shortcuts were added.
    if (e.target.tagName === "BUTTON" && (e.key === "Enter" || e.key === " ")) return;
    if (e.ctrlKey && (e.key === "=" || e.key === "+")) { zoom = Math.min(1.8, zoom + 0.1); e.preventDefault(); return; }
    if (e.ctrlKey && e.key === "-") { zoom = Math.max(0.7, zoom - 0.1); e.preventDefault(); return; }
    if (e.ctrlKey && e.key === "0") { zoom = 1; e.preventDefault(); return; }
    if (e.ctrlKey || e.metaKey || e.altKey || !review) return;
    switch (e.key) {
      case "ArrowRight": goTo(at + 1); break;
      case "ArrowLeft": goTo(at - 1); break;
      case "u": case "x": {
        if (step?.kind !== "code") return;
        setVerdict(step.id, e.key === "u" ? "understood" : "unclear");
        // Unclear must not ship hollow: an empty note gets the keyboard.
        if (e.key === "x" && !mark(step.id).note.trim()) noteFocus++;
        break;
      }
      case "Enter": {
        const after = review.steps.findIndex((s, j) => j > at && s.kind === "code" && !marks[s.id]?.verdict);
        const any = after !== -1 ? after : review.steps.findIndex((s) => s.kind === "code" && !marks[s.id]?.verdict);
        if (any !== -1) goTo(any);
        break;
      }
      case "n": if (step?.kind === "code") { noteFocus++; e.preventDefault(); } break;
      case "c": stepComposer++; e.preventDefault(); break;
      case "s": flushNote(); pend = null; submit = true; e.preventDefault(); break;
      // `a` reads the step's opening, which is where a reader starts; the rest of
      // the regions have their own buttons, because a key that walks between them
      // would be a position, and a position is what FR72 removed.
      case "a": aloud(readingRegion ?? "intro"); e.preventDefault(); break;
      case "t": brief = !brief; e.preventDefault(); break;
      case "[": goDomain(-1); e.preventDefault(); break;
      case "]": goDomain(1); e.preventDefault(); break;
      case "g": toggleGlossary(); e.preventDefault(); break;
      case "l": bridge.showLibrary().catch(() => {}); e.preventDefault(); break;
      // Esc already means "close the thing that is open"; a reading is one of
      // those, and it is the loudest, so it goes first. The drawer is last:
      // closing it must never be what happens instead of discarding a draft.
      case "Escape":
        if (readingRegion) aloud(null);
        else if (pend) pend = null;
        else glossaryAt = null;
        break;
      case "q": aloud(null); bridge.closeSelf(); break;
    }
  }
</script>

<svelte:window onkeydown={onKey} onresize={syncMax} />

<div class="board" style="font-size: calc(1rem * {zoom})">
  {#if err}
    <div class="load-error">{err}</div>
  {:else if review}
    <!-- The header is the window's title bar as well as its own: frameless
         means there is no other grip, and a window that cannot be moved is
         stuck wherever the placer put it. Chrome is not selectable text
         (app.css), so the whole strip drags and only the buttons opt out. -->
    <header data-agentbox-find-exclude style="--wails-draggable: drag">
      <div class="title">Review · {review.title}</div>
      <div class="mono repo">{review.repo}</div>
      <div class="mono chip pinned">pinned {review.pinned}</div>
      {#if hasGlossary}
        <!-- Icons, with the word and the key in the tooltip: a header chip
             competes with the title and the sha for width, and an open book
             says "definitions" faster than the word does. -->
        <button
          class="chip gloss icon"
          class:on={glossaryAt !== null}
          onclick={toggleGlossary}
          title="glossary - {review.glossary.length} terms (g)"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M12 6.5C10.5 5 8.5 4.5 4 5v13c4.5-.5 6.5 0 8 1.5" />
            <path d="M12 6.5C13.5 5 15.5 4.5 20 5v13c-4.5-.5-6.5 0-8 1.5" />
            <path d="M12 6.5v13" />
          </svg>
          <span class="n">{review.glossary.length}</span>
        </button>
      {/if}
      <ModeToggle {brief} onPick={(v) => (brief = v)} />
      <div class="pips">
        {#each counted as s (s.id)}
          <span
            class="pip"
            class:ok={marks[s.id]?.verdict === "understood"}
            class:warn={marks[s.id]?.verdict === "unclear"}
          ></span>
        {/each}
      </div>
      <div class="count" class:done={understood === counted.length && counted.length > 0}>
        {understood} of {counted.length} understood
        {#if unclear.length > 0}<span class="unclear-n"> · {unclear.length} unclear</span>{/if}
      </div>
      <!-- The word "review" is the one thing this header never has to say -
           the title already did. Icon plus one verb, sized like the chips
           beside it, and it keeps the accent because it is the way out. -->
      <button class="submit-btn" onclick={() => { flushNote(); pend = null; submit = true; }} title="submit the review (s)">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="m21 3-8 18-3-7.5L2.5 10.5z" />
          <path d="M21 3 10 13.5" />
        </svg>
        Submit
      </button>
      <button
        class="chip gloss icon"
        onclick={() => bridge.showLibrary().catch(() => {})}
        title="library - every stored review (l)"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <rect x="3" y="4" width="4" height="16" rx="1" />
          <rect x="9" y="4" width="4" height="16" rx="1" />
          <path d="m16.5 5.6 3.6 1 -3.2 12.3 -3.6-1z" />
        </svg>
      </button>

      <!-- Frameless, so the window's own buttons are drawn here or nowhere.
           They sit together at the end, in the order a title bar has them, so
           the eye finds them where it already looks. Closing loses nothing:
           marks, notes and comments are written through as they happen. -->
      <div class="wins">
        <button class="winbtn" onclick={() => bridge.minimiseSelf().catch(() => {})} title="minimise">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" aria-hidden="true">
            <path d="M6 17h12" />
          </svg>
        </button>
        <button
          class="winbtn"
          onclick={() => bridge.toggleMaximiseSelf().then(syncMax).catch(() => {})}
          title={maximised ? "restore down" : "maximise"}
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            {#if maximised}
              <rect x="4" y="9" width="11" height="11" rx="2" />
              <path d="M9 4.5h8A2.5 2.5 0 0 1 19.5 7v8" />
            {:else}
              <rect x="5" y="5" width="14" height="14" rx="2" />
            {/if}
          </svg>
        </button>
        <button class="winbtn close-btn" onclick={() => { flushNote(); aloud("stop"); bridge.closeSelf(); }} title="close the board (q) - your marks are already saved">✕</button>
      </div>
    </header>

    <div class="body">
      <Rail steps={review.steps} {marks} {at} go={goTo} domains={review.domains ?? []} />
      <div class="column">
          <!-- The domain banner. It is keyed on the DOMAIN, not on the step, so
               it animates once when the reader crosses into a new subject and
               sits still while they walk through it - which is the whole
               difference between an opening and a flicker on every arrow press. -->
          {#if domain}
            {#key domain.id}
              <div class="dbanner">
                <span class="dtitle">{domain.title}</span>
                <span class="dwhere">{inDomain} of {ofDomain}</span>
                {#if domain.blurb}<p class="dblurb">{domain.blurb}</p>{/if}
              </div>
            {/key}
          {/if}
        {#key step?.id}
          <Step
            {step}
            mark={marks[step?.id] ?? { verdict: "", note: "", revealed: [] }}
            comments={comments.filter((c) => c.stepId === step?.id)}
            root={review.root}
            isFirst={at === 0}
            isLast={at === review.steps.length - 1}
            {noteFocus}
            {stepComposer}
            bind:pend
            onVerdict={(v) => setVerdict(step.id, v)}
            onNote={(t) => setNote(step.id, t)}
            onReveal={(i, open) => setRevealed(step.id, i, open)}
            onComment={(anchor, body) => addComment(step.id, anchor, body)}
            onCommentEdit={editComment}
            onCommentDelete={deleteComment}
            onNav={(d) => goTo(at + d)}
            onAloud={aloud}
            {readingRegion}
            onTerm={hasGlossary ? openTerm : null}
            {onTermHover}
            {onOpen}
            {brief}
          />
        {/key}
      </div>
      {#if glossaryAt !== null}
        <Glossary terms={review.glossary} openKey={glossaryAt} onClose={() => (glossaryAt = null)} />
      {/if}
    </div>

    <footer data-agentbox-find-exclude>
      ← → step · u understood · x unclear · Enter next unread · n note · c comment · select code to comment · a read the opening aloud · t {brief ? "full text" : "TL;DR"}{#if domains.length} · [ ] domain{/if}{#if hasGlossary} · g glossary{/if} · l library · s submit · q close
    </footer>

    {#if tip}
      <div class="tip" style="left: {tip.x}px; {tip.below ? 'top' : 'bottom'}: {tip.y}px">
        <span class="tip-term">{termsByKey[tip.key].term}</span>
        <span class="tip-short">{termsByKey[tip.key].short}</span>
        <span class="tip-more">click for the rest</span>
      </div>
    {/if}

    {#if submit}
      <SubmitModal
        {review}
        {marks}
        {comments}
        onClose={() => (submit = false)}
        onJump={jumpToStep}
        onSubmitted={() => (review.state = "submitted")}
      />
    {/if}
  {/if}
</div>

<style>
  .board {
    height: 100vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    background: var(--k-ground);
    color: var(--k-ink);
  }
  /* The domain opening. It slides up and fades in once per domain, and the
     reader crossing from one subject to the next is the one moment on this
     surface where a movement is doing work: it says "new subject" before a word
     is read. Suppressed for anybody who asked for less motion - the banner is
     still there, it just arrives instead of moving. */
  .dbanner {
    margin: 0 0 22px;
    padding-bottom: 12px;
    border-bottom: 1px solid var(--k-edge);
    animation: domain-in 380ms cubic-bezier(0.22, 0.61, 0.36, 1);
  }
  .dtitle {
    font-family: var(--k-font-mono);
    font-size: 0.78em;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--k-accent);
  }
  .dwhere {
    margin-left: 10px;
    font-family: var(--k-font-mono);
    font-size: 0.76em;
    color: var(--k-ink-3);
  }
  .dbanner .dblurb {
    margin: 8px 0 0;
    max-width: 68ch;
    font-family: var(--k-font-read);
    font-size: 1em;
    line-height: 1.5;
    color: var(--k-ink-2);
    text-wrap: pretty;
  }
  @keyframes domain-in {
    from {
      opacity: 0;
      transform: translateY(-6px);
    }
    to {
      opacity: 1;
      transform: none;
    }
  }
  :global([data-motion="reduced"]) .dbanner,
  :global([data-motion="none"]) .dbanner {
    animation: none;
  }

  .load-error {
    margin: auto;
    color: var(--k-ink-2);
    font-size: 1.05em;
  }
  header {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 10px 20px;
    border-bottom: 1px solid var(--k-edge);
    flex: none;
  }
  /* The board opens maximised, but the restore size and the 700px minimum are
     sizes it is really used at, and a header that overflows takes the window
     buttons off the edge with it - the one control you cannot do without.
     So the title is the only part that gives up width, by ellipsis, and the
     things that can be read elsewhere leave before it comes to that. */
  .title {
    font-weight: 700;
    font-size: 1.1em;
    white-space: nowrap;
    min-width: 4em;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  header > :not(.title) {
    flex: none;
  }
  @media (max-width: 1150px) {
    header {
      gap: 12px;
    }
    .repo,
    .chip.pinned {
      display: none;
    }
  }
  @media (max-width: 820px) {
    .pips {
      display: none;
    }
    .count {
      margin-left: auto;
    }
  }
  .mono {
    font-family: var(--k-font-mono);
  }
  .repo {
    color: var(--k-ink-2);
    font-size: 0.9em;
  }
  .chip {
    border: 1px solid var(--k-edge);
    border-radius: 999px;
    padding: 2px 10px;
    font-size: 0.85em;
    color: var(--k-ink-2);
    white-space: nowrap;
  }
  .gloss {
    font-family: inherit;
    background: none;
    cursor: pointer;
  }
  /* Inside the drag strip a press that wanders a pixel would move the window
     instead of pressing the button, so every control opts out of the grip. */
  header button {
    --wails-draggable: none;
  }
  .chip.icon {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 4px 9px;
  }
  .chip.icon svg {
    width: 1.05em;
    height: 1.05em;
  }
  .chip.icon .n {
    font-size: 0.85em;
  }
  .gloss:hover,
  .gloss:focus-visible {
    color: var(--k-ink);
    border-color: var(--k-info);
  }
  .gloss.on {
    color: var(--k-info);
    border-color: var(--k-info);
  }
  .pips {
    margin-left: auto;
    display: flex;
    gap: 4px;
  }
  .pip {
    width: 1em;
    height: 0.375em;
    border-radius: 2px;
    background: var(--k-edge);
    transition: background 240ms ease;
  }
  .pip.ok {
    background: var(--k-success);
  }
  .pip.warn {
    background: var(--k-warning);
  }
  .count {
    font-size: 0.95em;
    white-space: nowrap;
  }
  .count.done {
    color: var(--k-success);
  }
  .submit-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    background: transparent;
    border: 1px solid var(--k-info);
    border-radius: 8px;
    color: var(--k-info);
    font-family: inherit;
    font-size: 0.95em;
    padding: 4px 12px;
    cursor: pointer;
    white-space: nowrap;
  }
  .submit-btn svg {
    width: 1.05em;
    height: 1.05em;
  }
  .submit-btn:focus-visible {
    outline: 2px solid var(--k-info);
  }
  /* Tight against each other and away from the rest: window buttons are one
     control, not three more things in the header's 16px rhythm. */
  .wins {
    display: flex;
    align-items: center;
    gap: 2px;
    margin-left: -6px;
  }
  .winbtn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: none;
    border: 0;
    border-radius: 6px;
    color: var(--k-ink-3);
    font-size: 1em;
    line-height: 1;
    padding: 5px 6px;
    cursor: pointer;
  }
  .winbtn svg {
    width: 1.05em;
    height: 1.05em;
  }
  .winbtn:hover,
  .winbtn:focus-visible {
    color: var(--k-ink);
    background: var(--k-edge);
  }
  /* The ✕ is a glyph where its neighbours are strokes, so it needs the size
     matched by hand; the red on hover is what every title bar has taught. */
  .close-btn {
    font-size: 0.95em;
  }
  .close-btn:hover,
  .close-btn:focus-visible {
    color: var(--k-error);
  }
  .unclear-n {
    color: var(--k-warning);
  }
  /* Relative because the glossary drawer overlays this area rather than
     taking a column from it: opening a definition must not reflow the
     paragraph the reader is in the middle of. */
  .body {
    position: relative;
    display: flex;
    min-height: 0;
    flex: 1;
  }
  .column {
    min-width: 0;
    flex: 1;
    overflow-y: auto;
    padding: 32px 40px;
  }
  /* Above the drawer and the composers, below the submit modal. Centred on
     the word and pointer-events: none - a tooltip you can hover is a tooltip
     that flickers as you move toward it. */
  .tip {
    position: fixed;
    transform: translateX(-50%);
    z-index: 20;
    pointer-events: none;
    max-width: 36ch;
    background: var(--k-surface);
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    box-shadow: 0 10px 28px rgb(0 0 0 / 0.45);
    padding: 8px 11px;
    display: flex;
    flex-direction: column;
    gap: 2px;
    animation: tipin 110ms ease-out;
  }
  @keyframes tipin {
    from {
      opacity: 0;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .tip {
      animation: none;
    }
  }
  .tip-term {
    font-weight: 700;
    font-size: 0.78em;
    color: var(--k-info);
  }
  .tip-short {
    font-family: var(--k-font-read);
    font-size: 0.95em;
    line-height: 1.45;
  }
  .tip-more {
    font-size: 0.72em;
    color: var(--k-ink-3);
    margin-top: 2px;
  }
  footer {
    height: 2.5em;
    display: flex;
    align-items: center;
    padding: 0 20px;
    border-top: 1px solid var(--k-edge);
    color: var(--k-ink-2);
    font-size: 0.85em;
    flex: none;
  }
</style>
