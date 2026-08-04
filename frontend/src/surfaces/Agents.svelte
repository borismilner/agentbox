<script>
  // The Agents surface (FR83). Boris: "I should be able to monitor using the GUI
  // what exactly each such agent is doing at the moment in the most convenient
  // and informative way."
  //
  // The layout answers that in a fixed reading order, because the question is
  // always the same one and it should always be answered in the same place:
  // WHY the agent exists (the purpose, the headline), WHAT it is doing right
  // now (the activity and its age), and WHETHER it is stuck (the state chip,
  // and what it holds or waits on). Rows group by area, so two agents in one
  // repo sit visibly together - overlap is something Boris sees before either
  // agent does.
  //
  // The state chip is the daemon's word, not the agent's. An agent writes its
  // purpose and its activity and nothing else on the row: "asking you",
  // "blocked", "driving desktop" are facts the daemon observes, and a
  // self-report can colour a row but never define it.
  import { bridge, on } from "../lib/bridge.js";
  import { ticker } from "../lib/clock.svelte.js";

  let roster = $state(null); // null until the first payload: empty is a real answer
  let open = $state(null); // the expanded row's key
  let breaking = $state(null); // the lock name awaiting a break confirmation
  let flash = $state(null); // a row jumped to from a wait, briefly lit
  let err = $state("");

  const clock = ticker();

  // Ages arrive as ages, not timestamps, and are anchored to the moment the
  // payload landed - the arrangement the control strip proved. A wall-clock jump
  // then rebases everything on the next push instead of reading every agent as
  // quiet after a suspend.
  let base = $state(Date.now());

  function take(r) {
    roster = r ?? { agents: [], orphans: [], shared: [], partial: false };
    base = Date.now();
    // A row that went away must not keep the detail open under a different agent.
    if (open && !(roster.agents ?? []).some((a) => a.key === open)) open = null;
  }

  bridge
    .agents()
    .then(take)
    .catch((e) => (err = String(e)));
  on("agentbox:agents", take);

  const agents = $derived(roster?.agents ?? []);
  const orphans = $derived(roster?.orphans ?? []);
  // The blackboard (FR83 slice 4): global state, so its own block rather than a chip
  // on a row. Abandoned claims float to the top, because they are the only thing here
  // that is a problem - the rest is just work in progress.
  const shared = $derived(
    [...(roster?.shared ?? [])].sort((a, b) => (b.owner_gone ? 1 : 0) - (a.owner_gone ? 1 : 0)),
  );
  const abandoned = $derived(shared.filter((v) => v.owner_gone).length);

  // Grouped by area, in first-seen order, so the list does not reshuffle itself
  // every time an agent's state changes.
  const areas = $derived.by(() => {
    const out = [];
    const at = new Map();
    for (const a of agents) {
      let g = at.get(a.area);
      if (!g) {
        // The caption is the area's own path, never a member's cwd. Members of one
        // repo sit in different subdirectories, and an agent can declare an area it
        // is not standing in - both used to make this header state a falsehood
        // (LAPTOP-SETUP captioned with the agentbox path). Empty means say nothing.
        g = { key: a.area, label: a.area_label || a.area, cwd: "", rows: [] };
        at.set(a.area, g);
        out.push(g);
      }
      if (!g.cwd && a.area_path) g.cwd = a.area_path;
      g.rows.push(a);
    }
    return out;
  });

  const working = $derived(agents.filter((a) => a.state === "working").length);

  const elapsed = $derived(clock.now - base);

  // Control.svelte's grammar, extended for the one thing a roster has that a
  // strip does not: rows that have been quiet for an hour.
  function ago(sinceMs) {
    const total = Math.max(0, (sinceMs ?? 0) + elapsed);
    const s = Math.floor(total / 1000);
    if (s < 60) return `${s}s`;
    const m = Math.floor(s / 60);
    if (m < 60) return s % 60 ? `${m}m ${s % 60}s` : `${m}m`;
    const h = Math.floor(m / 60);
    return m % 60 ? `${h}h ${m % 60}m` : `${h}h`;
  }

  // The chip vocabulary, in the design's own words. The label is what Boris
  // reads; the class is what colours it.
  function chip(a) {
    switch (a.state) {
      case "asking":
        return { label: "asking you", cls: "ask" };
      case "driving":
        return { label: "driving desktop", cls: "drive" };
      case "blocked":
        return { label: `blocked: ${a.detail}`, cls: "block" };
      case "listening":
        return { label: `listening: ${a.detail}`, cls: "listen" };
      case "reporting":
        return { label: `reporting: ${a.detail}`, cls: "listen" };
      case "working":
        return { label: "working", cls: "work" };
      case "quiet":
        return { label: "quiet", cls: "dim" };
      // The headline already says "no purpose given" for this row, so the chip
      // says the other half of the fact rather than the same six words twice.
      case "unannounced":
        return { label: "never announced", cls: "dim" };
      case "detached":
        return { label: "seen, not attached", cls: "ghost" };
      default:
        return { label: a.state, cls: "dim" };
    }
  }

  function who(a) {
    return [a.agent, a.project].filter(Boolean).join(" · ");
  }

  // Clicking a wait goes to the holder's row. Two agents waiting on each other
  // is then a thing you follow, not a diagram you assemble in your head.
  function jump(key) {
    if (!key) return;
    open = key;
    flash = key;
    setTimeout(() => (flash = null), 1200);
    document.getElementById(`agent-${key}`)?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }

  function toggle(key) {
    breaking = null;
    open = open === key ? null : key;
  }

  async function breakLock(name) {
    breaking = null;
    try {
      const sentence = await bridge.breakLock(name);
      err = sentence || "";
    } catch (e) {
      err = String(e);
    }
  }
</script>

<section class="agents">
  <header>
    <div class="line">
      <h1>Agents</h1>
      <span class="count">
        {#if agents.length}
          {agents.length} agent{agents.length === 1 ? "" : "s"} · {areas.length} area{areas.length === 1 ? "" : "s"}
          {#if working}<span class="on"> · {working} working</span>{/if}
        {/if}
      </span>
    </div>
    {#if roster?.partial}
      <!-- FR61's rule applied to presence: "you are alone" must be true when it
           is said, or not said at all. -->
      <p class="partial">
        One or more sessions are older than sync and have no roster row of their own. This list is
        not everybody.
      </p>
    {/if}
  </header>

  <div class="scroll">
    {#if err}<div class="err">{err}</div>{/if}

    {#if roster === null}
      <p class="blank">Reading the roster…</p>
    {:else if agents.length === 0 && !orphans.length && !shared.length}
      <div class="blank">
        <p class="lead">No agents are attached.</p>
        <p>
          A session appears here as soon as it makes its first AgentBox call, and says what it is
          for the moment it calls <code>announce</code>.
        </p>
      </div>
    {:else}
      {#if orphans.length}
        <div class="area">
          <div class="head">
            <span class="label">Locks with no live holder</span>
            <span class="grow"></span>
            <span class="n">{orphans.length}</span>
          </div>
          <!-- Orphaned, not released. The session died; the work it started may
               not have. A lock handed to the next waiter while the first agent's
               deploy is still running is the failure the lock existed to
               prevent, so the grant waits on the recorded process. -->
          <div class="rows">
            {#each orphans as h (h.name)}
              <div class="row orphan">
                <div class="main plain">
                  <span class="hue warn"></span>
                  <span class="body">
                    <span class="l1"><span class="purpose mono">{h.name}</span></span>
                    <span class="l2">
                      <span class="who">{h.holder || "an agent that is gone"}</span>
                      <span class="act">
                        {#if h.pid_live}
                          its pid {h.pid} is still alive, so nobody gets this until it exits
                        {:else}
                          its pid {h.pid} is gone; the next waiter is granted on the next tick
                        {/if}
                      </span>
                      <span class="when">{ago(h.since_ms)}</span>
                    </span>
                  </span>
                  <span class="chips">
                    {#if h.waiters}<span class="chip block">{h.waiters} waiting</span>{/if}
                    {#if breaking === h.name}
                      <span class="confirm">
                        <span class="say">Reassigns the lock. It does not stop the process.</span>
                        <button class="danger" onclick={() => breakLock(h.name)}>Break</button>
                        <button class="keep" onclick={() => (breaking = null)}>Keep</button>
                      </span>
                    {:else}
                      <button class="ghost" onclick={() => (breaking = h.name)}>Break lock</button>
                    {/if}
                  </span>
                </div>
              </div>
            {/each}
          </div>
        </div>
      {/if}

      {#if shared.length}
        <div class="area">
          <div class="head">
            <span class="label">Shared values</span>
            <span class="grow"></span>
            <span class="n">{shared.length}</span>
            {#if abandoned}<span class="n warnn">· {abandoned} abandoned</span>{/if}
          </div>
          <!-- A claim outlives the session that made it, which is the point and the
               risk. An owner that is gone means work was started and never finished,
               and nothing else on this board says so: the lock table cannot, because
               a claim is not a lock, and the agent's row cannot, because the agent is
               gone. -->
          <div class="rows">
            {#each shared as v (v.key)}
              <div class="row" class:orphan={v.owner_gone}>
                <div class="main plain">
                  <span class="hue" class:warn={v.owner_gone}></span>
                  <span class="body">
                    <span class="l1">
                      <span class="purpose mono">{v.key}</span>
                      <span class="val mono">{v.value}</span>
                    </span>
                    <span class="l2">
                      {#if v.owner_gone}
                        <span class="who">{v.owner_name || "an agent that is gone"}</span>
                        <span class="act">is gone; this work stopped and nobody took it over</span>
                      {:else if v.owner_name}
                        <span class="who">{v.owner_name}</span>
                        <span class="act">holds it</span>
                      {:else}
                        <span class="act">no owner: shared state rather than a claim</span>
                      {/if}
                      <span class="when">v{v.version} · {ago(v.since_ms)}</span>
                    </span>
                  </span>
                  <span class="chips">
                    {#if v.owner_gone}<span class="chip block">owner gone</span>{/if}
                  </span>
                </div>
              </div>
            {/each}
          </div>
        </div>
      {/if}

      {#each areas as area (area.key)}
        <div class="area">
          <div class="head">
            <span class="label">{area.label}</span>
            {#if area.cwd}<span class="path">{area.cwd}</span>{/if}
            <span class="grow"></span>
            <span class="n">{area.rows.length}</span>
          </div>

          <div class="rows">
            {#each area.rows as a (a.key || a.agent + a.project)}
              {@const c = chip(a)}
              <div
                class="row"
                class:open={open === a.key}
                class:flash={flash === a.key}
                class:faint={a.state === "unannounced" || a.state === "detached"}
                id="agent-{a.key}"
              >
                <div
                  class="main"
                  role="button"
                  tabindex="0"
                  aria-expanded={open === a.key}
                  onclick={() => toggle(a.key)}
                  onkeydown={(e) => (e.key === "Enter" || e.key === " ") && (e.preventDefault(), toggle(a.key))}
                >
                  <span class="hue" style="background: {a.hue}"></span>

                  <span class="body">
                    <span class="l1">
                      {#if a.purpose}
                        <span class="purpose">{a.purpose}</span>
                      {:else}
                        <span class="purpose none">no purpose given</span>
                      {/if}
                    </span>

                    <span class="l2">
                      <span class="who">{who(a)}</span>
                      <!-- The key, on every row, because within one area the
                           agent and the project are usually identical and the
                           key is the only thing that tells two sessions apart.
                           That is the identity defect FR83 fixes, so the surface
                           may as well teach it. -->
                      {#if a.key}<span class="sess">{a.key}</span>{/if}
                      {#if a.session}<span class="sess">{a.session}</span>{/if}
                      {#each a.tags ?? [] as t}<span class="tag">{t}</span>{/each}
                      {#if a.activity}
                        <span class="act">{a.activity}</span>
                        <span class="when">{ago(a.activity_since_ms)}</span>
                      {:else if a.state === "detached"}
                        <span class="act quietly">last seen through an item</span>
                        <span class="when">{ago(a.age_ms)}</span>
                      {:else if !a.wait}
                        <!-- A blocked row says nothing here: its wait line below
                             is the activity, and "nothing reported" beside a
                             named wait would be a lie. -->
                        <span class="act quietly">nothing reported</span>
                      {/if}
                    </span>
                  </span>

                  <span class="chips">
                    {#each a.holds ?? [] as h}
                      <span class="lock" title="held for {ago(h.since_ms)}{h.note ? ' · ' + h.note : ''}">
                        {h.name}
                        <span class="lockage">{ago(h.since_ms)}</span>
                      </span>
                    {/each}
                    <span class="chip {c.cls}">{c.label}</span>
                  </span>
                </div>

                {#if a.wait}
                  <!-- The wait is its own line, not a chip: it carries a holder, a
                       queue place and an age, and it is the row's whole story
                       while it lasts. -->
                  <div class="wait">
                    <span class="arrow" aria-hidden="true">↳</span>
                    waiting on <b>{a.wait.lock}</b> for {ago(a.wait.since_ms)}, held by
                    <button class="holder" onclick={() => jump(a.wait.holder_key)} title="Go to the holder's row">
                      {a.wait.holder}
                    </button>
                    {#if a.wait.queue > 1}
                      <span class="place">place {a.wait.place} of {a.wait.queue}</span>
                    {/if}
                  </div>
                {/if}

                {#if open === a.key}
                  <div class="detail">
                    {#if a.pending}
                      <p class="pending">Parked on your answer: <b>{a.pending}</b></p>
                    {/if}

                    <dl class="meta">
                      {#if a.key}<dt>session key</dt>
                        <dd class="mono">{a.key}</dd>{/if}
                      {#if a.pid}<dt>pid</dt>
                        <dd class="mono">{a.pid}</dd>{/if}
                      <dt>directory</dt>
                      <dd class="mono">{a.cwd}</dd>
                      <dt>area</dt>
                      <dd class="mono">{a.area}</dd>
                      <dt>attached</dt>
                      <dd>{ago(a.age_ms)} ago</dd>
                    </dl>

                    {#if a.holds?.length}
                      <div class="block">
                        <span class="label">Holds</span>
                        {#each a.holds as h}
                          <div class="held">
                            <span class="mono">{h.name}</span>
                            <span class="dim">{ago(h.since_ms)}{h.note ? ` · ${h.note}` : ""}</span>
                            {#if h.waiters}<span class="dim">{h.waiters} waiting</span>{/if}
                            <span class="grow"></span>
                            {#if breaking === h.name}
                              <span class="confirm">
                                <span class="say">Reassigns the lock. It does not stop this agent.</span>
                                <button class="danger" onclick={() => breakLock(h.name)}>Break</button>
                                <button class="keep" onclick={() => (breaking = null)}>Keep</button>
                              </span>
                            {:else}
                              <button class="ghost" onclick={() => (breaking = h.name)}>Break lock</button>
                            {/if}
                          </div>
                        {/each}
                      </div>
                    {/if}

                    {#if a.timeline?.length}
                      <div class="block">
                        <span class="label">Activity</span>
                        {#each a.timeline as t}
                          <div class="tick">
                            <span class="tage">{ago(t.since_ms)}</span>
                            <span>{t.line}</span>
                          </div>
                        {/each}
                      </div>
                    {/if}

                    {#if a.signals?.length}
                      <div class="block">
                        <span class="label">Signals</span>
                        {#each a.signals as s}
                          <div class="tick">
                            <span class="tage">{ago(s.since_ms)}</span>
                            <span class="dir {s.dir}">{s.dir === "posted" ? "→" : "←"}</span>
                            <span class="mono">{s.topic}</span>
                            {#if s.data}<span class="dim mono">{s.data}</span>{/if}
                          </div>
                        {/each}
                      </div>
                    {/if}

                    {#if a.items?.length}
                      <div class="block">
                        <span class="label">Recent items</span>
                        {#each a.items as it}
                          <div class="tick">
                            <span class="tage">{ago(it.since_ms)}</span>
                            <span>{it.title}</span>
                            <span class="dim">{it.kind} · {it.state}</span>
                          </div>
                        {/each}
                      </div>
                    {/if}
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        </div>
      {/each}
    {/if}
  </div>
</section>

<style>
  .agents {
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
  }
  h1 {
    margin: 0;
    font-size: 1.12rem;
    font-weight: 700;
    letter-spacing: -0.01em;
  }
  .line .count {
    font-family: var(--k-font-mono);
    font-size: 0.7rem;
    color: var(--k-ink-3);
  }
  .line .on {
    color: var(--k-success);
  }
  .partial {
    margin: 9px 0 0;
    font-family: var(--k-font-read);
    font-size: 0.8rem;
    line-height: 1.45;
    color: var(--k-warning);
  }

  .scroll {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 14px 18px 24px;
  }
  .err {
    margin-bottom: 10px;
    font-size: 0.84rem;
    color: var(--k-error);
  }
  .blank {
    max-width: 480px;
    margin: 44px auto;
    text-align: center;
    font-family: var(--k-font-read);
    font-size: 0.9rem;
    color: var(--k-ink-3);
  }
  .blank .lead {
    margin: 0 0 6px;
    font-size: 1.05rem;
    color: var(--k-ink-2);
  }
  .blank p {
    margin: 0;
    line-height: 1.6;
  }
  .blank code {
    font-family: var(--k-font-mono);
    font-size: 0.82em;
    color: var(--k-ink-2);
  }

  /* One area is one group of agents that can collide with each other. The
   * heading is the section label voice the rest of the app uses. */
  .area {
    margin-bottom: 18px;
  }
  .head {
    display: flex;
    align-items: baseline;
    gap: 10px;
    padding: 0 2px 6px;
  }
  .label {
    font-size: 0.72rem;
    font-weight: 600;
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--k-ink-3);
  }
  .head .path {
    font-family: var(--k-font-mono);
    font-size: 0.7rem;
    color: var(--k-ink-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 42ch;
  }
  .grow {
    flex: 1;
  }
  .head .n {
    font-family: var(--k-font-mono);
    font-size: 0.7rem;
    color: var(--k-ink-3);
  }
  /* The count that is a problem rather than a fact: abandoned claims. It reads in
     the warning colour so a glance at the heading answers "is anything stuck?"
     without reading the rows. */
  .head .n.warnn {
    color: var(--k-warning);
  }
  /* A shared value's contents, beside its key. Bounded rather than wrapped: the cap
     is 16 KB and one long claim must not push the whole board's layout around. */
  .val {
    margin-left: 8px;
    color: var(--k-ink-2);
    font-size: 0.82rem;
    max-width: 40ch;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* The joined-rows frame every list in the app wears. */
  .rows {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--k-edge-soft);
    border-radius: 10px;
    overflow: hidden;
  }
  .row {
    border-top: 1px solid var(--k-edge-soft);
    background: var(--k-surface);
  }
  .row:first-child {
    border-top: 0;
  }
  .row:hover {
    background: var(--k-surface-2);
  }
  .row.open {
    background: var(--k-surface-2);
  }
  /* A row jumped to from somebody else's wait: lit long enough to find, not
   * long enough to become part of the design. */
  .row.flash {
    background: color-mix(in srgb, var(--k-accent) 12%, var(--k-surface));
  }
  /* Present but silent. Dim rather than hidden: an agent that never announced is
   * exactly what the human needs to see. */
  .row.faint .purpose,
  .row.faint .who {
    color: var(--k-ink-3);
  }

  .main {
    display: flex;
    align-items: flex-start;
    gap: 11px;
    padding: 10px 13px;
    cursor: pointer;
    text-align: left;
  }
  .main.plain {
    cursor: default;
  }
  .hue {
    flex: none;
    width: 7px;
    height: 7px;
    margin-top: 6px;
    border-radius: 50%;
  }
  .hue.warn {
    background: var(--k-warning);
  }
  .body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  /* Line one is the purpose and nothing else. It is the answer to "what is this
   * agent for", and it earns the row's biggest type. */
  .l1 {
    display: flex;
    min-width: 0;
  }
  .purpose {
    font-size: 0.9rem;
    line-height: 1.3;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .purpose.none {
    font-style: italic;
    color: var(--k-ink-3);
  }

  /* Line two is who and what-right-now, in the strip's grammar. */
  .l2 {
    display: flex;
    align-items: baseline;
    gap: 8px;
    min-width: 0;
    font-size: 0.74rem;
    color: var(--k-ink-3);
  }
  .who {
    flex: none;
    font-family: var(--k-font-mono);
    font-size: 0.7rem;
    color: var(--k-ink-2);
  }
  .sess {
    flex: none;
    font-family: var(--k-font-mono);
    font-size: 0.66rem;
    color: var(--k-ink-3);
  }
  .tag {
    flex: none;
    border: 1px solid var(--k-edge-soft);
    border-radius: 999px;
    padding: 0 6px;
    font-family: var(--k-font-mono);
    font-size: 0.62rem;
    color: var(--k-ink-3);
  }
  /* The activity and its age sit together, in the strip's own
   * `<activity> · 12s` grammar. Letting the activity take the whole row would
   * strand the age against the far edge, and the two are one reading. */
  .act {
    flex: 0 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--k-ink-2);
  }
  .act.quietly {
    font-style: italic;
    color: var(--k-ink-3);
  }
  .when {
    flex: none;
    font-variant-numeric: tabular-nums;
    font-size: 0.7rem;
    color: var(--k-ink-3);
  }

  .chips {
    flex: none;
    display: flex;
    align-items: center;
    gap: 6px;
    margin-top: 1px;
  }
  .chip {
    border: 1px solid var(--k-edge);
    border-radius: 999px;
    padding: 1px 9px;
    font-size: 0.68rem;
    white-space: nowrap;
    color: var(--k-ink-2);
  }
  .chip.work {
    border-color: color-mix(in srgb, var(--k-success) 45%, transparent);
    color: var(--k-success);
  }
  .chip.ask {
    border-color: color-mix(in srgb, var(--k-accent) 60%, transparent);
    background: color-mix(in srgb, var(--k-accent) 14%, transparent);
    color: var(--k-accent);
    animation: breathe 1.8s ease-in-out infinite;
  }
  @keyframes breathe {
    50% {
      opacity: 0.5;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .chip.ask {
      animation: none;
    }
  }
  .chip.drive,
  .chip.block {
    border-color: color-mix(in srgb, var(--k-warning) 50%, transparent);
    color: var(--k-warning);
  }
  .chip.listen {
    border-color: color-mix(in srgb, var(--k-info) 45%, transparent);
    color: var(--k-info);
  }
  .chip.dim {
    color: var(--k-ink-3);
  }
  /* Known only from item traffic: a dashed edge says the row is inferred, not
   * reported. */
  .chip.ghost {
    border-style: dashed;
    color: var(--k-ink-3);
  }

  /* A held lock rides the row itself, because "who has the deploy" is a
   * glance-level question. */
  .lock {
    display: inline-flex;
    align-items: baseline;
    gap: 6px;
    border: 1px solid color-mix(in srgb, var(--k-warning) 35%, transparent);
    border-radius: 6px;
    padding: 1px 7px;
    font-family: var(--k-font-mono);
    font-size: 0.66rem;
    color: var(--k-ink-2);
    white-space: nowrap;
  }
  .lockage {
    font-variant-numeric: tabular-nums;
    color: var(--k-ink-3);
  }

  .wait {
    display: flex;
    align-items: baseline;
    gap: 6px;
    padding: 0 13px 10px 31px;
    font-size: 0.76rem;
    color: var(--k-warning);
  }
  .wait .arrow {
    color: var(--k-ink-3);
  }
  .wait b {
    font-family: var(--k-font-mono);
    font-weight: 500;
  }
  .holder {
    border: 0;
    padding: 0;
    background: none;
    font: inherit;
    color: var(--k-ink);
    text-decoration: underline dotted;
    text-underline-offset: 2px;
    cursor: pointer;
  }
  .holder:hover,
  .holder:focus-visible {
    color: var(--k-accent);
  }
  .place {
    font-family: var(--k-font-mono);
    font-size: 0.68rem;
    color: var(--k-ink-3);
  }

  .detail {
    padding: 2px 13px 13px 31px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .pending {
    margin: 0;
    font-size: 0.8rem;
    color: var(--k-accent);
  }
  .meta {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 2px 12px;
    margin: 0;
    font-size: 0.76rem;
  }
  .meta dt {
    color: var(--k-ink-3);
  }
  .meta dd {
    margin: 0;
    color: var(--k-ink-2);
  }
  .mono {
    font-family: var(--k-font-mono);
    font-size: 0.94em;
  }
  .dim {
    color: var(--k-ink-3);
  }

  .block {
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .block .label {
    margin-bottom: 2px;
  }
  .tick {
    display: flex;
    align-items: baseline;
    gap: 9px;
    font-size: 0.78rem;
    color: var(--k-ink-2);
  }
  .tage {
    flex: none;
    min-width: 6ch;
    text-align: right;
    font-family: var(--k-font-mono);
    font-size: 0.68rem;
    font-variant-numeric: tabular-nums;
    color: var(--k-ink-3);
  }
  .dir.posted {
    color: var(--k-success);
  }
  .dir.received {
    color: var(--k-info);
  }

  .held {
    display: flex;
    align-items: center;
    gap: 9px;
    font-size: 0.78rem;
    color: var(--k-ink-2);
  }

  .ghost {
    border: 1px solid var(--k-edge);
    border-radius: 7px;
    padding: 2px 9px;
    font-size: 0.74rem;
    color: var(--k-ink-3);
    white-space: nowrap;
  }
  .ghost:hover {
    color: var(--k-ink);
    background: var(--k-surface-3);
  }

  /* Breaking a lock is unilateral, so the confirm says plainly what it does and
   * what it does not: it reassigns the lock, it does not stop the ex-holder. A
   * human who thinks otherwise has been handed the exact failure the orphan rule
   * exists to prevent. */
  .confirm {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .say {
    font-size: 0.74rem;
    color: var(--k-warning);
  }
  .danger,
  .keep {
    border: 1px solid var(--k-edge);
    border-radius: 7px;
    padding: 2px 9px;
    font-size: 0.76rem;
    color: var(--k-ink-2);
    white-space: nowrap;
  }
  .danger {
    color: var(--k-error);
    border-color: color-mix(in srgb, var(--k-error) 60%, transparent);
  }
  .danger:hover,
  .keep:hover {
    background: var(--k-surface-3);
  }
</style>
