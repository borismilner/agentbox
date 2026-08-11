<script module>
  // What agentbox is, in one column. A rail rather than a tab strip because with
  // several agents running, the state you glance at most (who is working, how
  // many are waiting) has to be permanently visible, not one click away.

  const items = [
    { id: "home", label: "Home" },
    { id: "session", label: "Session" },
    { id: "agents", label: "Agents" },
    { id: "assignments", label: "Assignments" },
    { id: "inbox", label: "Inbox" },
    { id: "history", label: "History" },
    { id: "viewer", label: "Viewer" },
    { id: "library", label: "Library" },
  ];
  const foot = { id: "settings", label: "Settings" };

  // The rail top to bottom, exported because the shell binds Ctrl+1..9 to the
  // same order (U-13). A second copy of the list in App.svelte would drift the
  // first time a surface is added here, and the shortcut would then open the
  // icon above or below the one it names.
  export const SURFACES = [...items, foot];
</script>

<script>
  // pending badges the inbox with items nobody has answered yet - the number
  // that decides whether you go and look. working marks the session icon while
  // an agent is mid-turn, and attached does the same for Agents: with several
  // agents running, that any of them is live is the thing worth a glance (FR83).
  // unsaved is how many settings knobs are changed and not yet written (U-10):
  // the edits now survive leaving the surface, and this is what says so from the
  // other eight.
  let { tab = $bindable("home"), pending = 0, working = 0, attached = 0, unsaved = 0 } = $props();

  // An icon button's accessible name is its only name, and the badge and the
  // live dot beside it are painted rather than written. A reader who cannot see
  // them hears the count here or nowhere, so the counts travel in the name.
  function spoken(it) {
    if (it.id === "inbox" && pending > 0) return `Inbox, ${pending} waiting`;
    if (it.id === "session" && working > 0) return `Session, ${working} working`;
    if (it.id === "agents" && attached > 0) return `Agents, ${attached} attached`;
    return it.label;
  }

  // The number in the tooltip is the shortcut, said where somebody looking for
  // it would already be pointing.
  const shortcut = (i) => `Ctrl+${i + 1}`;
</script>

<nav class="rail" aria-label="Surfaces">
  {#each items as it, i}
    <button
      class="btn"
      class:on={tab === it.id}
      title="{spoken(it)} · {shortcut(i)}"
      aria-label={spoken(it)}
      aria-keyshortcuts="Control+{i + 1}"
      aria-current={tab === it.id ? "page" : undefined}
      onclick={() => (tab = it.id)}
    >
      {#if it.id === "home"}
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
          <path d="M3 10.5 12 3l9 7.5" />
          <path d="M5 9.5V20a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V9.5" />
        </svg>
      {:else if it.id === "session"}
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
          <path d="M21 12a8 8 0 0 1-11.6 7.1L4 20l.9-5.1A8 8 0 1 1 21 12z" />
        </svg>
        {#if working > 0}<span class="live" title="{working} working"></span>{/if}
      {:else if it.id === "agents"}
        <!-- Two nodes and the line between them: this surface is about agents
             that can see each other, not about one agent talking. -->
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="7" cy="7" r="3.2" />
          <circle cx="17" cy="17" r="3.2" />
          <path d="m9.4 9.4 5.2 5.2" />
        </svg>
        {#if attached > 0}<span class="live" title="{attached} attached"></span>{/if}
      {:else if it.id === "assignments"}
        <!-- Work that happens on a clock: the hands say scheduled, the dot at
             the centre says something runs from it. -->
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="13" r="8" />
          <path d="M12 9.5V13l2.5 1.5" />
          <path d="M9 2.5h6" />
        </svg>
      {:else if it.id === "inbox"}
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
          <path d="M3 13h4l2 3h6l2-3h4" />
          <path d="M4.6 5.5 3 13v5a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-5l-1.6-7.5A2 2 0 0 0 17.4 4H6.6a2 2 0 0 0-2 1.5z" />
        </svg>
        {#if pending > 0}<span class="badge">{pending}</span>{/if}
      {:else if it.id === "history"}
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
          <path d="M4 20V10" /><path d="M10 20V4" /><path d="M16 20v-7" /><path d="M22 20H2" />
        </svg>
      {:else if it.id === "viewer"}
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
          <path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z" />
          <path d="M14 3v5h5" /><path d="M9 13h6" /><path d="M9 17h4" />
        </svg>
      {:else}
        <!-- Library: things agentbox is holding on to, stacked. -->
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
          <rect x="3" y="4" width="4" height="16" rx="1" />
          <rect x="9" y="4" width="4" height="16" rx="1" />
          <path d="m16.5 5.6 3.6 1 -3.2 12.3 -3.6-1z" />
        </svg>
      {/if}
    </button>
  {/each}

  <span class="grow"></span>

  <button
    class="btn"
    class:on={tab === foot.id}
    title={unsaved
      ? `${foot.label} · ${shortcut(items.length)} · ${unsaved} change${unsaved === 1 ? "" : "s"} not yet written`
      : `${foot.label} · ${shortcut(items.length)}`}
    aria-label={unsaved ? `${foot.label}, ${unsaved} not yet written` : foot.label}
    aria-keyshortcuts="Control+{items.length + 1}"
    aria-current={tab === foot.id ? "page" : undefined}
    onclick={() => (tab = foot.id)}
  >
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
      <path d="M4 7h10" /><path d="M18 7h2" /><path d="M4 17h4" /><path d="M12 17h8" />
      <circle cx="16" cy="7" r="2" /><circle cx="10" cy="17" r="2" />
    </svg>
    {#if unsaved > 0}<span class="unsaved"></span>{/if}
  </button>
</nav>

<style>
  .rail {
    grid-row: 2 / 4;
    border-right: 1px solid var(--k-edge-soft);
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 10px 0 12px;
    gap: 4px;
  }
  .grow {
    flex: 1;
  }
  .btn {
    position: relative;
    width: 36px;
    height: 34px;
    border-radius: 8px;
    display: grid;
    place-items: center;
    color: var(--k-ink-3);
  }
  .btn:hover {
    color: var(--k-ink-2);
    background: var(--k-surface-2);
  }
  .btn.on {
    color: var(--k-ink);
    background: var(--k-surface-2);
  }
  .btn.on::before {
    content: "";
    position: absolute;
    left: -7px;
    top: 8px;
    bottom: 8px;
    width: 2px;
    border-radius: 2px;
    background: var(--k-ink-2);
  }
  svg {
    width: 18px;
    height: 18px;
  }
  /* A dot, not a count: how many agents are mid-turn is a detail, that any of
   * them is working is the thing worth a glance. */
  .live {
    position: absolute;
    top: 4px;
    right: 4px;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--k-success);
    box-shadow: 0 0 0 2px var(--k-ground);
  }
  /* The same dot the settings surface puts beside a knob it is holding an edit
   * for, in the same accent, so the rail says the same thing from further away.
   * Not the inbox's badge: how many keys are pending is not a number anyone acts
   * on, and this is a reminder rather than a queue. The count is in the tooltip
   * for whoever wants it. */
  .unsaved {
    position: absolute;
    top: 4px;
    right: 4px;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--k-accent, #7c8cf8);
    box-shadow: 0 0 0 2px var(--k-ground, #0f1116);
  }
  .badge {
    position: absolute;
    top: 1px;
    right: 0;
    min-width: 15px;
    height: 15px;
    padding: 0 3px;
    border-radius: 8px;
    background: var(--k-warning);
    color: var(--k-ground);
    font-family: var(--k-font-mono);
    font-size: 0.58rem;
    font-weight: 500;
    line-height: 15px;
    text-align: center;
    border: 2px solid var(--k-ground);
  }
</style>
