<script>
  // The submission moment (FR58): what goes back to the agent, in one turn,
  // then a receipt - "the handback has no receipt" was the adversarial
  // pass's finding and this is where it is answered. The preview is
  // markdown, an export nicety; the real payload is assembled daemon-side
  // where the gate lives. A hollow unclear never gets this far quietly: the
  // modal jumps to the step instead, and the daemon refuses it again if
  // asked directly.
  import { bridge } from "../bridge.js";
  import CopyBtn from "./CopyBtn.svelte";

  let { review, marks, comments, onClose, onJump, onSubmitted } = $props();

  const counted = $derived(review.steps.filter((s) => s.kind === "code"));
  const understood = $derived(counted.filter((s) => marks[s.id]?.verdict === "understood").length);
  const unclear = $derived(counted.filter((s) => marks[s.id]?.verdict === "unclear"));
  const notReviewed = $derived(
    counted.filter((s) => !["understood", "unclear"].includes(marks[s.id]?.verdict)),
  );
  const hollow = $derived(unclear.find((s) => !(marks[s.id]?.note ?? "").trim()));

  let armed = $state(false); // zero-verdict submits only on the second, explicit click
  let busy = $state(false);
  let fail = $state("");
  let receipt = $state(null);

  const md = $derived.by(() => {
    const L = [`# Review: ${review.title}`, `pinned ${review.pinned}`, ""];
    L.push(`## Unclear (${unclear.length}) - answer these first`);
    for (const s of unclear) {
      const note = (marks[s.id]?.note ?? "").trim();
      L.push(`- **${s.title}**${note ? ` - ${note.replace(/\n+/g, " ")}` : ""}`);
    }
    L.push("", "## Steps");
    for (const s of review.steps) {
      const m = marks[s.id] ?? {};
      L.push(`### ${s.title} [${s.kind}] - ${m.verdict || "unread"}`);
      if ((m.note ?? "").trim()) for (const l of m.note.split("\n")) L.push(`> ${l}`);
      for (const c of comments.filter((c) => c.stepId === s.id)) {
        const anchor = c.path ? `${c.path}:${c.from}${c.to !== c.from ? "-" + c.to : ""} ` : "";
        L.push(`- ${anchor}${c.exact ? `“${c.exact}”` : ""}`.trimEnd());
        L.push(`  ${c.body}`);
      }
    }
    L.push("", `## Not reviewed (${notReviewed.length})`);
    for (const s of notReviewed) L.push(`- ${s.title}`);
    L.push("", "## Coverage", "not computed in this build");
    return L.join("\n");
  });

  async function submit() {
    if (hollow) {
      onJump(hollow.id);
      return;
    }
    if (understood + unclear.length === 0 && !armed) {
      armed = true;
      return;
    }
    busy = true;
    fail = "";
    try {
      const r = await bridge.boardSubmit(review.id);
      if (r.gate) {
        onJump(r.gate);
        return;
      }
      receipt = r;
      onSubmitted?.(r);
    } catch (e) {
      fail = String(e);
    } finally {
      busy = false;
    }
  }

  function hhmm(ms) {
    const d = new Date(ms);
    return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
<div class="veil" data-agentbox-find-exclude onclick={(e) => e.target === e.currentTarget && onClose()}>
  <div class="panel" role="dialog" aria-modal="true" aria-label="Submit review">
    {#if receipt}
      <div class="receipt">
        <div class="tick">✓</div>
        <div class="line">
          {receipt.delivered ? "Delivered to the agent" : "Saved for the agent"} · {hhmm(receipt.atMs)}
        </div>
        <div class="sub">
          {understood} understood · {unclear.length} unclear · {comments.length} comments
        </div>
        {#if !receipt.delivered}
          <div class="sub">no agent was waiting - the next one that reads or awaits this review takes it, exactly once</div>
        {/if}
        <button class="act" onclick={onClose}>Close</button>
      </div>
    {:else}
      <div class="head">
        <div class="title">What goes back to the agent, in one turn</div>
        <div class="tally">{unclear.length} unclear · {notReviewed.length} not reviewed</div>
      </div>
      <textarea readonly class="preview" value={md}></textarea>
      <div class="row">
        <button class="act" disabled={busy} onclick={submit}>
          {armed ? "No verdicts yet - submit anyway?" : "Submit to the agent"}
        </button>
        <span class="copy"><CopyBtn text={() => md} label="copy as markdown" /></span>
        <button class="quit" onclick={onClose}>Keep reviewing</button>
      </div>
      {#if fail}<div class="fail">{fail}</div>{/if}
      <div class="fine">
        Submission talks to the agent directly - AgentBox owns delivery. The clipboard is an export
        nicety, kept for pasting a review somewhere else.
      </div>
    {/if}
  </div>
</div>

<style>
  .veil {
    position: fixed;
    inset: 0;
    background: rgb(0 0 0 / 0.63);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 30;
  }
  .panel {
    width: min(760px, calc(100vw - 64px));
    max-height: 80vh;
    display: flex;
    flex-direction: column;
    background: var(--k-surface);
    border: 1px solid var(--k-edge);
    border-radius: 12px;
    padding: 24px;
  }
  .head {
    display: flex;
    align-items: baseline;
    margin-bottom: 8px;
  }
  .title {
    font-weight: 700;
    font-size: 1.125em;
  }
  .tally {
    margin-left: auto;
    color: var(--k-ink-2);
    font-size: 0.9em;
  }
  .preview {
    flex: 1;
    min-height: 16em;
    resize: none;
    background: var(--k-surface-2);
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    color: var(--k-ink);
    font-family: var(--k-font-mono);
    font-size: 0.85em;
    line-height: 1.5;
    padding: 10px 12px;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-top: 12px;
  }
  button {
    background: transparent;
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    color: var(--k-ink);
    font-size: 0.95em;
    padding: 6px 16px;
    cursor: pointer;
    transition: border-color 140ms ease, color 140ms ease;
  }
  button:disabled {
    opacity: 0.4;
    cursor: default;
  }
  button:focus-visible {
    outline: 2px solid var(--k-info);
  }
  .act {
    border-color: var(--k-info);
    color: var(--k-info);
  }
  .quit {
    margin-left: auto;
  }
  /* The copy control sits beside submit, an export next to the act
     (CopyBtn floats right by default). */
  .copy :global(button) {
    margin-left: 0;
  }
  .fail {
    margin-top: 8px;
    color: var(--k-error);
    font-size: 0.9em;
  }
  .fine {
    margin-top: 8px;
    color: var(--k-ink-3);
    font-size: 0.8em;
  }
  .receipt {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    padding: 40px 0;
  }
  .tick {
    color: var(--k-success);
    font-size: 2em;
  }
  .line {
    font-weight: 700;
    font-size: 1.125em;
  }
  .sub {
    color: var(--k-ink-2);
    font-size: 0.9em;
  }
  .receipt .act {
    margin-top: 12px;
  }
</style>
