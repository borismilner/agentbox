<script>
  import { bridge } from "../lib/bridge.js";

  // History (FR35): how many interruptions arrived, who sent them, and
  // how fast they got answered. The point is not the numbers themselves - it is
  // noticing that one agent asks twice as often as the rest, or that half the
  // questions went unanswered, and doing something about it.
  //
  // The Gio version drew this through the markdown engine because a table and a
  // chart were expensive to lay out by hand. Here they are HTML, so the Go side
  // only ships numbers (internal/webui/stats.go).

  let { refresh = 0 } = $props();

  const WINDOWS = [
    { id: "24h", label: "24h" },
    { id: "7d", label: "7d" },
    { id: "30d", label: "30d" },
    { id: "all", label: "All" },
  ];

  let window_ = $state("7d");
  let stats = $state(null);
  let winEl = $state(null);

  // The only choice on this surface is which window it covers, and until now the
  // only way to make it was the pointer or four Tab stops (U-12). Four buttons
  // where exactly one is chosen is a radio group, so it answers to the arrows,
  // costs one Tab stop, and says which window is current out loud.
  //
  // The buttons are all on screen already, so the new one takes the focus by its
  // position without waiting for a repaint.
  function move(delta) {
    const at = WINDOWS.findIndex((w) => w.id === window_);
    const to = (at + delta + WINDOWS.length) % WINDOWS.length;
    window_ = WINDOWS[to].id;
    winEl?.querySelectorAll("button")[to]?.focus();
  }

  function onKey(e) {
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    if (e.key === "ArrowRight" || e.key === "ArrowDown") {
      e.preventDefault();
      move(1);
    } else if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
      e.preventDefault();
      move(-1);
    }
  }

  // Re-query on a window change and whenever the queue moved (refresh is bumped
  // by the app on every agentbox:inbox push), so an answer given a second ago is
  // already in the median.
  $effect(() => {
    const w = window_;
    void refresh;
    bridge.stats(w).then((s) => {
      if (s && s.window === w) stats = s;
    });
  });

  const days = $derived(stats?.byDay ?? []);
  const peak = $derived(Math.max(1, stats?.peak ?? 0));
  const agents = $derived(stats?.byAgent ?? []);
  // Label the tallest day only. A number on every bar is noise; the one that
  // matters is the worst day, and the rest are read off its height.
  const peakDay = $derived(days.find((d) => d.count === stats?.peak)?.day ?? "");
</script>

<section class="history">
  <header>
    <div class="line">
      <h1>History</h1>
      <div class="windows" role="radiogroup" aria-label="Time window" bind:this={winEl}>
        <!-- The keys sit on the buttons rather than on the group, because the
             chosen window is the only one in the Tab order and so the only one
             that can be holding the keyboard when an arrow arrives. -->
        {#each WINDOWS as w}
          <button
            role="radio"
            aria-checked={window_ === w.id}
            tabindex={window_ === w.id ? 0 : -1}
            class:on={window_ === w.id}
            onkeydown={onKey}
            onclick={() => (window_ = w.id)}>{w.label}</button
          >
        {/each}
      </div>
    </div>
  </header>

  <div class="scroll">
    {#if !stats}
      <p class="empty">Reading history…</p>
    {:else if stats.empty}
      <p class="empty">Nothing in the {stats.label}. A quiet desktop is the goal.</p>
    {:else}
      <!-- The same tile grammar as Home: the number first, in mono, with a
           lowercase word under it. Two dashboards that draw a count two ways
           read as two products. -->
      <div class="tiles">
        <div class="tile">
          <strong>{stats.total}</strong>
          <span class="l">interruptions</span>
          <span class="sub">{stats.perDay} · {stats.label}</span>
        </div>
        <div class="tile">
          <strong>{stats.questions}</strong>
          <span class="l">questions</span>
          <span class="sub">blocked an agent</span>
        </div>
        <div class="tile">
          <strong>{stats.answered}<em>{stats.questions ? ` · ${stats.answeredPct}%` : ""}</em></strong>
          <span class="l">answered</span>
          <span class="sub">{stats.questions - stats.answered} went unanswered</span>
        </div>
        <div class="tile">
          <strong>{stats.median}</strong>
          <span class="l">median answer</span>
          <span class="sub">card up to answered</span>
        </div>
      </div>

      <section class="panel">
        <h2>Interruptions per day</h2>
        <div class="chart" style="--peak: {peak}">
          {#each days as d (d.day)}
            <div class="col" title="{d.label}: {d.count}">
              <span class="plot">
                <span class="bar" class:top={d.day === peakDay} style="height: {(d.count / peak) * 100}%"></span>
              </span>
              <span class="tick">{d.label}</span>
            </div>
          {/each}
          {#if peakDay}
            <span class="peak">peak {stats.peak}</span>
          {/if}
        </div>
      </section>

      <section class="panel">
        <h2>By agent</h2>
        <table>
          <thead>
            <tr>
              <th>Agent</th>
              <th class="n">Total</th>
              <th class="n">Questions</th>
              <th class="n">Answered</th>
              <th class="n">Median</th>
            </tr>
          </thead>
          <tbody>
            {#each agents as a (a.agent)}
              <tr>
                <td><span class="idot" style="background: {a.hue}"></span>{a.agent}</td>
                <td class="n">{a.total}</td>
                <td class="n">{a.questions}</td>
                <td class="n">{a.answered}</td>
                <td class="n mono">{a.median}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </section>
    {/if}
  </div>
</section>

<style>
  .history {
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
    align-items: center;
    justify-content: space-between;
  }
  h1 {
    margin: 0;
    font-size: 1.12rem;
    font-weight: 700;
    letter-spacing: -0.01em;
  }

  .windows {
    display: flex;
    gap: 2px;
    padding: 2px;
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    background: var(--k-surface-2);
  }
  .windows button {
    padding: 3px 11px;
    border-radius: 6px;
    font-size: 0.75rem;
    color: var(--k-ink-3);
  }
  .windows button:hover {
    color: var(--k-ink-2);
  }
  .windows button.on {
    background: var(--k-surface-3);
    color: var(--k-ink);
  }

  .scroll {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 16px 18px 24px;
  }
  .empty {
    margin: 44px 0;
    text-align: center;
    font-family: var(--k-font-read);
    font-size: 0.9rem;
    color: var(--k-ink-3);
  }

  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(158px, 1fr));
    gap: 12px;
  }
  .tile {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 16px 18px;
    border: 1px solid var(--k-edge-soft);
    border-radius: 12px;
    background: var(--k-surface);
  }
  .tile strong {
    font-family: var(--k-font-mono);
    font-size: 1.55rem;
    font-weight: 500;
    line-height: 1.2;
    color: var(--k-ink);
    font-variant-numeric: tabular-nums;
  }
  .tile strong em {
    font-style: normal;
    font-size: 0.86rem;
    font-weight: 400;
    color: var(--k-ink-2);
  }
  .tile .l {
    font-size: 0.8rem;
    color: var(--k-ink-3);
  }
  .tile .sub {
    font-size: 0.72rem;
    color: var(--k-ink-3);
  }

  .panel {
    margin-top: 22px;
  }
  .panel h2 {
    margin: 0 0 10px;
    font-size: 0.76rem;
    font-weight: 600;
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--k-ink-3);
  }

  /* One series, so no legend: the heading names it. Bars wear the chrome accent
   * rather than a severity or identity hue - a day's count is neither. */
  .chart {
    position: relative;
    display: flex;
    align-items: stretch;
    gap: 2px;
    height: 178px;
    padding: 22px 14px 10px;
    border: 1px solid var(--k-edge-soft);
    border-radius: 10px;
    background: var(--k-surface-2);
  }
  .col {
    flex: 1 1 0;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 7px;
  }
  /* The plot row owns the height the bars scale against, so a full-height bar
   * cannot push its own label out of the box. */
  .plot {
    flex: 1;
    min-height: 0;
    display: flex;
    align-items: flex-end;
    justify-content: center;
    border-bottom: 1px solid var(--k-edge);
  }
  .bar {
    width: 100%;
    max-width: 38px;
    min-height: 2px;
    border-radius: 4px 4px 0 0;
    background: color-mix(in srgb, var(--k-accent) 52%, transparent);
    transition: background 120ms ease;
  }
  .col:hover .bar {
    background: color-mix(in srgb, var(--k-accent) 70%, transparent);
  }
  .bar.top {
    background: var(--k-accent);
  }
  .tick {
    font-family: var(--k-font-mono);
    font-size: 0.58rem;
    color: var(--k-ink-3);
    text-align: center;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .peak {
    position: absolute;
    top: 6px;
    right: 10px;
    font-family: var(--k-font-mono);
    font-size: 0.6rem;
    color: var(--k-ink-3);
  }

  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.82rem;
  }
  th,
  td {
    padding: 7px 10px;
    text-align: left;
    border-bottom: 1px solid var(--k-edge-soft);
  }
  th {
    font-size: 0.68rem;
    font-weight: 500;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--k-ink-3);
  }
  td {
    color: var(--k-ink-2);
  }
  td:first-child {
    color: var(--k-ink);
  }
  .n {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  .mono {
    font-family: var(--k-font-mono);
    font-size: 0.74rem;
  }
  tbody tr:last-child td {
    border-bottom: 0;
  }
  .idot {
    display: inline-block;
    width: 7px;
    height: 7px;
    border-radius: 50%;
    margin-right: 7px;
    vertical-align: 1px;
  }
</style>
