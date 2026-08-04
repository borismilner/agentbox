<script>
  // The Assignments surface (M12/FR82): the work agentbox does on its own.
  //
  // Master and detail rather than a list that expands, because the two
  // questions this surface answers are asked at different times. "Is anything
  // broken?" is answered by the column on the left at a glance - every
  // assignment, its kind, and how its last run went. "Why did Tuesday fail?" is
  // the panel on the right, and it wants the whole width.
  //
  // The knobs are drawn from a descriptor Go builds (assignpanel.go paramsFor),
  // not from the spec: a control's shape is decided in one place, so a knob type
  // added to assign.Param cannot half-exist because the frontend was not told.
  import { bridge, on } from "../lib/bridge.js";
  import { markdown } from "../lib/markdown.svelte.js";
  import { onPanelParams, pushPanelParams } from "../lib/artifact.svelte.js";

  let rows = $state(null); // null until the first load; empty is a real answer
  let err = $state("");
  let sel = $state(null); // the open assignment's id
  let open = $state(null); // its detail, from bridge.assignment
  let editing = $state(null); // the draft, or null when not editing
  let saved = $state(null); // the last save's warnings
  let asking = $state(false); // delete confirmation
  let busy = $state(false);
  let panelHost = $state(null); // the element the custom panel hydrates in

  const current = $derived((rows ?? []).find((r) => r.id === sel) ?? null);
  const knobs = $derived(open?.knobs ?? []);

  async function load(keep = true) {
    try {
      const res = await bridge.assignments();
      rows = res.assignments ?? [];
      err = res.err ?? "";
      if (!keep || !rows.some((r) => r.id === sel)) sel = rows[0]?.id ?? null;
      if (sel) await openOne(sel);
      else open = null;
    } catch (e) {
      err = String(e);
    }
  }

  async function openOne(id) {
    sel = id;
    editing = null;
    saved = null;
    asking = false;
    try {
      open = await bridge.assignment(id);
      err = open?.err ?? "";
    } catch (e) {
      err = String(e);
    }
  }

  load(false);

  // The daemon pokes on every assignment mutation, whoever made it - this
  // surface, an agent's update_assignment, the scheduler starting or finishing
  // a run - so the surface follows all of them without polling. Registered in
  // an effect because the app remounts a surface on every tab switch, and a
  // subscription with no unsubscribe would stack a refresh per visit.
  $effect(() => on("agentbox:assignments", () => refresh()));

  // The custom panel's two-way channel. Values go IN whenever they might have
  // changed: open holds fresh state after every load/refresh, and reading it
  // here makes the effect re-run on each one, so an agent's edit reaches a
  // panel somebody is looking at. pushPanelParams also remembers the values on
  // the block, so a frame still loading replays them when it is ready.
  $effect(() => {
    if (panelHost && open?.panelBlock) pushPanelParams(panelHost, currentValues());
  });

  // ...and values coming OUT are merged over everything stored and written
  // through the same entry point the knobs use. The panel emits what it
  // manages; keys it never mentions survive, the same rule update_assignment
  // follows. The id is checked twice: a frame outliving a selection switch
  // writes nowhere, and a switch during the write must not smear the reply
  // onto whatever is open now.
  $effect(() => {
    onPanelParams(async (id, patch) => {
      if (id !== sel) return;
      const values = { ...currentValues(), ...patch };
      const msg = await bridge.setAssignmentParams(id, JSON.stringify(values));
      if (id !== sel) return;
      if (msg) {
        err = msg;
        return;
      }
      err = "";
      for (const k of knobs) if (k.key && k.key in patch) k.value = patch[k.key];
      if (open?.assignment) open.assignment.params = values;
    });
    return () => onPanelParams(null);
  });

  async function refresh() {
    try {
      const res = await bridge.assignments();
      rows = res.assignments ?? [];
      if (sel) open = await bridge.assignment(sel);
    } catch (e) {
      err = String(e);
    }
  }

  function blank() {
    return {
      id: "",
      name: "",
      description: "",
      prompt: "",
      schedule: "",
      model: "",
      mode: "full",
      dir: "",
      spec: "[]",
      panel_html: "",
    };
  }

  function draftOf(a) {
    return {
      id: a.id,
      name: a.name ?? "",
      description: a.description ?? "",
      prompt: a.prompt ?? "",
      schedule: a.schedule ?? "",
      model: a.model ?? "",
      mode: a.mode || "full",
      dir: a.dir ?? "",
      spec: JSON.stringify(a.spec ?? [], null, 2),
      panel_html: a.panelHtml ?? "",
    };
  }

  function startNew() {
    editing = blank();
    saved = null;
    err = "";
  }

  async function save() {
    if (!editing) return;
    busy = true;
    try {
      const res = await bridge.saveAssignment({ ...editing });
      if (res.err) {
        err = res.err;
        return;
      }
      err = "";
      saved = res;
      const id = res.id;
      editing = null;
      await load();
      if (id) await openOne(id);
    } catch (e) {
      err = String(e);
    } finally {
      busy = false;
    }
  }

  // Everything the assignment's values are right now: what is stored, under
  // what the knobs show. Starting from the stored map is what keeps a value
  // only the custom panel manages from being erased by turning a knob.
  function currentValues() {
    const values = { ...(open?.assignment?.params ?? {}) };
    for (const k of knobs) if (k.key) values[k.key] = k.value;
    return values;
  }

  // A knob writes as it is turned. The values are the assignment's, not this
  // window's, so there is no Save button to forget: the next run uses what the
  // control says.
  async function setKnob(key, value) {
    if (!sel) return;
    const values = currentValues();
    values[key] = value;
    for (const k of knobs) if (k.key === key) k.value = value;
    const msg = await bridge.setAssignmentParams(sel, JSON.stringify(values));
    if (msg) err = msg;
    else if (open?.assignment) open.assignment.params = values;
  }

  async function act(fn) {
    busy = true;
    try {
      const msg = await fn();
      err = msg || "";
      await refresh();
    } catch (e) {
      err = String(e);
    } finally {
      busy = false;
    }
  }

  const run = () => act(() => bridge.runAssignment(sel));
  const toggle = (on) => act(() => bridge.enableAssignment(sel, on));

  async function remove() {
    asking = false;
    busy = true;
    try {
      const msg = await bridge.deleteAssignment(sel);
      err = msg || "";
      sel = null;
      await load(false);
    } finally {
      busy = false;
    }
  }

  // "in 4h" reads as a schedule; a timestamp reads as a log line.
  function until(ms) {
    if (!ms) return "";
    const s = (ms - Date.now()) / 1000;
    if (s < 0) return "due";
    if (s < 90) return "in under a minute";
    if (s < 3600) return `in ${Math.round(s / 60)}m`;
    if (s < 86400) return `in ${Math.round(s / 3600)}h`;
    return `in ${Math.round(s / 86400)}d`;
  }

  function ago(ms) {
    if (!ms) return "never";
    const s = Math.max(0, (Date.now() - ms) / 1000);
    if (s < 90) return "just now";
    if (s < 3600) return `${Math.round(s / 60)}m ago`;
    if (s < 86400) return `${Math.round(s / 3600)}h ago`;
    if (s < 7 * 86400) return `${Math.round(s / 86400)}d ago`;
    return new Date(ms).toLocaleDateString();
  }

  function took(r) {
    if (!r.ended_ms || !r.started_ms) return "";
    const s = Math.round((r.ended_ms - r.started_ms) / 1000);
    return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m${String(s % 60).padStart(2, "0")}s`;
  }
</script>

<section class="assignments">
  <aside class="list">
    <div class="head">
      <span class="label">Assignments</span>
      <button class="new" onclick={startNew}>+ New</button>
    </div>

    {#if rows === null}
      <p class="quiet">loading…</p>
    {:else if rows.length === 0}
      <p class="quiet">
        Nothing scheduled yet. An assignment is work AgentBox gives an agent on its own -
        write one here, or ask an agent to draft it with <code>create_assignment</code>.
      </p>
    {:else}
      {#each rows as r (r.id)}
        <button class="row" class:on={r.id === sel} class:off={!r.enabled} onclick={() => openOne(r.id)}>
          <span class="line1">
            <span class="name">{r.name}</span>
            {#if r.running}<span class="chip live">running</span>{/if}
            {#if !r.enabled}<span class="chip">paused</span>{/if}
          </span>
          <span class="line2">
            <span class="kind {r.kind}">{r.kind}</span>
            {#if r.schedule}<span class="mono">{r.schedule}</span>{/if}
            {#if r.enabled && r.next_run_ms}<span class="dim">{until(r.next_run_ms)}</span>{/if}
          </span>
          {#if r.last_state}
            <span class="line3 {r.last_state}">
              <span class="dot"></span>{r.last_state === "ok" ? "last run fine" : r.last_state}
              <span class="dim"> · {ago(r.last_run_ms)}</span>
            </span>
          {/if}
        </button>
      {/each}
    {/if}
  </aside>

  <div class="detail">
    {#if err}
      <div class="err">{err}</div>
    {/if}

    {#if editing}
      <header class="dhead">
        <h1>{editing.id ? "Edit assignment" : "New assignment"}</h1>
        <div class="acts">
          <button class="primary" onclick={save} disabled={busy}>Save</button>
          <button class="quietbtn" onclick={() => (editing = null)}>Cancel</button>
        </div>
      </header>

      <div class="form">
        <label class="f"><span>Name</span><input bind:value={editing.name} placeholder="Usage watch" /></label>
        <label class="f"><span>Description</span><input bind:value={editing.description} placeholder="what this is for, in one line" /></label>
        <label class="f wide">
          <span>Prompt</span>
          <textarea rows="9" bind:value={editing.prompt} spellcheck="false"
            placeholder="What the agent is asked to do. Use &#123;&#123;placeholders&#125;&#125; for the tunable parts."></textarea>
        </label>
        <label class="f">
          <span>Schedule</span>
          <input bind:value={editing.schedule} placeholder="empty, every 4h, daily 09:00, weekly mon 09:00" />
        </label>
        <label class="f"><span>Model</span><input bind:value={editing.model} placeholder="default" /></label>
        <label class="f">
          <span>Mode</span>
          <select bind:value={editing.mode}>
            <option value="full">full access</option>
            <option value="plan">plan (read-only)</option>
          </select>
        </label>
        <label class="f"><span>Directory</span><input bind:value={editing.dir} placeholder="home" /></label>
        <label class="f wide">
          <span>Knobs (JSON)</span>
          <textarea rows="7" class="mono" bind:value={editing.spec} spellcheck="false"></textarea>
          <small
            >One object per control: <code>key</code>, <code>type</code> (text, number, slider, toggle, enum,
            path, markdown), and <code>label</code>, <code>help</code>, <code>default</code>,
            <code>min</code>/<code>max</code>, <code>values</code> as the type needs them.</small
          >
        </label>
      </div>
    {:else if open?.assignment}
      {@const a = open.assignment}
      <header class="dhead">
        <div class="titles">
          <h1>{a.name}</h1>
          {#if a.description}<p class="sub">{a.description}</p>{/if}
        </div>
        <div class="acts">
          <button class="primary" onclick={run} disabled={busy || open.running}>
            {open.running ? "Running…" : "Run now"}
          </button>
          <button class="quietbtn" onclick={() => toggle(!current?.enabled)} disabled={busy}>
            {current?.enabled ? "Pause" : "Resume"}
          </button>
          <button class="quietbtn" onclick={() => (editing = draftOf(a))}>Edit</button>
          {#if asking}
            <button class="danger" onclick={remove}>Delete for good</button>
            <button class="quietbtn" onclick={() => (asking = false)}>Keep</button>
          {:else}
            <button class="quietbtn" onclick={() => (asking = true)}>Delete</button>
          {/if}
        </div>
      </header>

      <div class="facts">
        <span class="kind {open.kind}">{open.kind}</span>
        {#if a.schedule}<span class="mono">{a.schedule}</span>{/if}
        {#if current?.enabled && current?.next_run_ms}
          <span>next {until(current.next_run_ms)}</span>
        {:else if !current?.enabled}
          <span class="warn">paused</span>
        {/if}
        <span class="dim">{a.model || "default model"}</span>
        <span class="dim">{a.mode === "plan" ? "plan mode" : "full access"}</span>
      </div>

      {#if saved?.warnings?.length}
        <ul class="warns">
          {#each saved.warnings as w}<li>{w}</li>{/each}
        </ul>
      {/if}
      {#if open.unfilled?.length || open.problems?.length || open.unused?.length}
        <ul class="warns">
          {#each open.problems ?? [] as p}<li>{p}</li>{/each}
          {#if open.unfilled?.length}
            <li>
              the prompt asks for {open.unfilled.map((k) => `{{${k}}}`).join(", ")} and no knob fills it -
              the run would see the placeholder verbatim
            </li>
          {/if}
          {#if open.unused?.length}
            <li>the prompt never uses {open.unused.join(", ")}, so turning that knob changes nothing</li>
          {/if}
        </ul>
      {/if}

      {#if knobs.length}
        <h2>Parameters</h2>
        <div class="knobs">
          {#each knobs as k (k.key || k.label)}
            {#if k.type === "markdown"}
              <div class="prose" use:markdown={k.bodyHtml}></div>
            {:else}
              <div class="knob">
                <label class="klabel" for="k-{k.key}">{k.label}</label>
                <div class="kctl">
                  {#if k.type === "toggle"}
                    <input id="k-{k.key}" type="checkbox" checked={!!k.value} onchange={(e) => setKnob(k.key, e.currentTarget.checked)} />
                  {:else if k.type === "enum"}
                    <select id="k-{k.key}" value={k.value} onchange={(e) => setKnob(k.key, e.currentTarget.value)}>
                      {#each k.values ?? [] as v}<option value={v}>{v}</option>{/each}
                    </select>
                  {:else if k.type === "slider"}
                    <input
                      id="k-{k.key}"
                      type="range"
                      min={k.min ?? 0}
                      max={k.max ?? 100}
                      step={k.step ?? 1}
                      value={Number(k.value ?? 0)}
                      oninput={(e) => setKnob(k.key, Number(e.currentTarget.value))}
                    />
                    <span class="mono val">{k.value}{k.unit ?? ""}</span>
                  {:else if k.type === "number"}
                    <input
                      id="k-{k.key}"
                      type="number"
                      min={k.min ?? undefined}
                      max={k.max ?? undefined}
                      step={k.step ?? undefined}
                      value={Number(k.value ?? 0)}
                      onchange={(e) => setKnob(k.key, Number(e.currentTarget.value))}
                    />
                    {#if k.unit}<span class="dim">{k.unit}</span>{/if}
                  {:else if k.multiline}
                    <textarea id="k-{k.key}" rows="3" value={k.value ?? ""} onchange={(e) => setKnob(k.key, e.currentTarget.value)}></textarea>
                  {:else}
                    <input
                      id="k-{k.key}"
                      class:mono={k.type === "path"}
                      value={k.value ?? ""}
                      onchange={(e) => setKnob(k.key, e.currentTarget.value)}
                    />
                  {/if}
                </div>
                {#if k.help}<p class="khelp">{k.help}</p>{/if}
              </div>
            {/if}
          {/each}
        </div>
      {/if}

      {#if open.panelBlock}
        <h2>Custom panel</h2>
        <!-- The escape hatch, running: agent-authored controls in the artifact
             sandbox. Its emits go to setAssignmentParams (artifact.svelte.js
             routes on the block's data-panel mark), never to a waiting agent,
             and the knobs above stay the way in that always works. -->
        <div class="panelhost" bind:this={panelHost} use:markdown={open.panelBlock}>{@html open.panelBlock}</div>
      {/if}

      <h2>Prompt</h2>
      <pre class="prompt">{a.prompt}</pre>

      <h2>Runs</h2>
      {#if !open.runs?.length}
        <p class="quiet">It has not run yet. Run now is the fastest way to find out whether the prompt works.</p>
      {:else}
        <div class="runs">
          {#each open.runs as r (r.id)}
            <details class="run {r.state}">
              <summary>
                <span class="dot"></span>
                <span class="rstate">{r.state}</span>
                <span class="dim">{r.trigger}</span>
                <span class="dim">{ago(r.started_ms)}</span>
                {#if took(r)}<span class="dim mono">{took(r)}</span>{/if}
                <span class="rsum">{r.summary || r.error || ""}</span>
              </summary>
              <div class="rbody">
                {#if r.error}<p class="rerr">{r.error}</p>{/if}
                <!-- Only when the row above did not already say it: a one-line
                     report printed twice is the same sentence twice. -->
                {#if r.summary && (r.summary.includes("\n") || r.summary.length > 90)}
                  <pre class="rtext">{r.summary}</pre>
                {/if}
                {#if r.data}
                  <p class="rlabel">recorded</p>
                  <pre class="mono rdata">{r.data}</pre>
                {/if}
                {#if r.params && Object.keys(r.params).length}
                  <p class="rlabel">ran with</p>
                  <pre class="mono rdata">{JSON.stringify(r.params, null, 2)}</pre>
                {/if}
              </div>
            </details>
          {/each}
        </div>
      {/if}
    {:else if rows?.length === 0}
      <div class="blank">
        <h1>Work AgentBox does on its own</h1>
        <p>
          An assignment is a prompt AgentBox hands to a Claude agent on a schedule, or when you press Run.
          It runs as an ordinary session, so you can open it, read every step and take it over.
        </p>
        <p>
          The run can reach you while it happens - a notification when something matters, an urgent
          one when it cannot wait - and what it measures is kept, so a month of runs reads back as a
          series.
        </p>
        <button class="primary" onclick={startNew}>Write the first one</button>
      </div>
    {/if}
  </div>
</section>

<style>
  .assignments {
    display: grid;
    grid-template-columns: 268px 1fr;
    min-height: 0;
    height: 100%;
    overflow: hidden;
  }
  .list {
    border-right: 1px solid var(--k-edge-soft);
    padding: 14px 10px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 4px 8px;
  }
  .label {
    font-size: 0.78em;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--k-ink-3);
  }
  .new {
    background: none;
    border: 1px solid var(--k-edge);
    border-radius: 7px;
    color: var(--k-ink-2);
    cursor: pointer;
    font-size: 0.8em;
    padding: 3px 8px;
  }
  .new:hover {
    color: var(--k-ink);
    border-color: var(--k-accent);
  }
  .row {
    all: unset;
    cursor: pointer;
    display: flex;
    flex-direction: column;
    gap: 3px;
    border: 1px solid transparent;
    border-radius: 9px;
    padding: 7px 9px;
  }
  .row:hover {
    background: var(--k-surface-2);
  }
  .row.on {
    background: var(--k-surface-2);
    border-color: var(--k-edge);
  }
  .row.off .name {
    color: var(--k-ink-3);
  }
  .line1 {
    display: flex;
    align-items: baseline;
    gap: 6px;
  }
  .name {
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .line2,
  .line3 {
    display: flex;
    align-items: center;
    gap: 7px;
    font-size: 0.76em;
    color: var(--k-ink-2);
  }
  .kind {
    border: 1px solid var(--k-edge);
    border-radius: 5px;
    padding: 0 5px;
    font-size: 0.92em;
    color: var(--k-ink-3);
  }
  .kind.scheduled,
  .kind.periodic {
    color: var(--k-info);
    border-color: color-mix(in srgb, var(--k-info) 45%, transparent);
  }
  .chip {
    border: 1px solid var(--k-edge);
    border-radius: 5px;
    padding: 0 5px;
    font-size: 0.7em;
    color: var(--k-ink-3);
  }
  .chip.live {
    color: var(--k-success);
    border-color: color-mix(in srgb, var(--k-success) 50%, transparent);
  }
  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--k-ink-3);
    flex: none;
  }
  .ok .dot,
  .line3.ok .dot {
    background: var(--k-success);
  }
  .failed .dot,
  .line3.failed .dot {
    background: var(--k-error);
  }
  .skipped .dot,
  .line3.skipped .dot {
    background: var(--k-warning);
  }
  .running .dot,
  .line3.running .dot {
    background: var(--k-info);
  }

  .detail {
    padding: 18px 24px 30px;
    overflow-y: auto;
    min-width: 0;
  }
  .dhead {
    display: flex;
    align-items: flex-start;
    gap: 14px;
    margin-bottom: 10px;
  }
  .titles {
    flex: 1;
    min-width: 0;
  }
  h1 {
    font-size: 1.15em;
    margin: 0 0 2px;
  }
  h2 {
    font-size: 0.78em;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--k-ink-3);
    margin: 20px 0 8px;
  }
  .sub {
    margin: 0;
    color: var(--k-ink-2);
    font-size: 0.86em;
    max-width: 70ch;
  }
  .acts {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  .primary,
  .quietbtn,
  .danger {
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.85em;
    padding: 5px 11px;
    border: 1px solid var(--k-edge);
    background: none;
    color: var(--k-ink-2);
  }
  .primary {
    border-color: var(--k-accent);
    color: var(--k-ink);
  }
  .primary:hover:not(:disabled) {
    background: color-mix(in srgb, var(--k-accent) 18%, transparent);
  }
  .quietbtn:hover {
    color: var(--k-ink);
    border-color: var(--k-accent);
  }
  .danger {
    border-color: color-mix(in srgb, var(--k-error) 60%, transparent);
    color: var(--k-error);
  }
  button:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .facts {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
    font-size: 0.8em;
    color: var(--k-ink-2);
    padding-bottom: 4px;
  }
  .dim {
    color: var(--k-ink-3);
  }
  .warn {
    color: var(--k-warning);
  }
  .mono {
    font-family: var(--k-font-mono);
  }
  .err {
    color: var(--k-error);
    font-size: 0.88em;
    border: 1px solid color-mix(in srgb, var(--k-error) 40%, transparent);
    border-radius: 8px;
    padding: 6px 10px;
    margin-bottom: 12px;
  }
  .warns {
    margin: 12px 0 0;
    padding: 8px 10px 8px 26px;
    border: 1px solid color-mix(in srgb, var(--k-warning) 35%, transparent);
    border-radius: 8px;
    color: var(--k-ink-2);
    font-size: 0.84em;
  }
  .panelhost {
    max-width: 92ch;
  }
  .quiet {
    color: var(--k-ink-2);
    font-size: 0.88em;
    max-width: 60ch;
  }
  .quiet code,
  .blank code {
    font-family: var(--k-font-mono);
    font-size: 0.9em;
    background: var(--k-surface-2);
    border-radius: 4px;
    padding: 1px 5px;
  }

  /* Knobs: label, control, help. One column, because a parameter panel is read
     top to bottom and a two-column form makes the eye hunt. */
  .knobs {
    display: flex;
    flex-direction: column;
    gap: 12px;
    max-width: 62ch;
  }
  .knob {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .klabel {
    font-size: 0.85em;
    color: var(--k-ink);
  }
  .kctl {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .kctl input[type="range"] {
    flex: 1;
    max-width: 26em;
  }
  .val {
    font-size: 0.85em;
    color: var(--k-ink-2);
    min-width: 4em;
  }
  .khelp {
    margin: 0;
    font-size: 0.78em;
    color: var(--k-ink-3);
  }
  .prose {
    font-size: 0.85em;
    color: var(--k-ink-2);
    max-width: 62ch;
  }

  input,
  select,
  textarea {
    background: var(--k-surface-2);
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    color: var(--k-ink);
    padding: 5px 9px;
    font-size: 0.88em;
    font-family: inherit;
  }
  input[type="checkbox"] {
    width: 16px;
    height: 16px;
    padding: 0;
  }
  input[type="range"] {
    padding: 0;
    background: none;
    border: none;
  }
  input:focus,
  select:focus,
  textarea:focus {
    outline: none;
    border-color: var(--k-accent);
  }
  textarea {
    resize: vertical;
    line-height: 1.45;
  }

  .form {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px 16px;
    max-width: 78ch;
  }
  .f {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .f > span {
    font-size: 0.8em;
    color: var(--k-ink-2);
  }
  .f.wide {
    grid-column: 1 / -1;
  }
  .f small {
    color: var(--k-ink-3);
    font-size: 0.75em;
  }
  .f small code {
    font-family: var(--k-font-mono);
  }

  .prompt {
    margin: 0;
    white-space: pre-wrap;
    word-break: break-word;
    font-family: var(--k-font-mono);
    font-size: 0.82em;
    line-height: 1.5;
    color: var(--k-ink-2);
    background: var(--k-surface-2);
    border: 1px solid var(--k-edge);
    border-radius: 9px;
    padding: 10px 12px;
    max-width: 92ch;
  }

  .runs {
    display: flex;
    flex-direction: column;
    gap: 4px;
    max-width: 92ch;
  }
  .run {
    border: 1px solid var(--k-edge);
    border-radius: 9px;
    padding: 6px 10px;
  }
  .run summary {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
    font-size: 0.82em;
    color: var(--k-ink-2);
    list-style: none;
  }
  .run summary::-webkit-details-marker {
    display: none;
  }
  .rstate {
    text-transform: capitalize;
    color: var(--k-ink);
  }
  .rsum {
    color: var(--k-ink-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
  }
  .rbody {
    padding: 8px 0 4px 14px;
  }
  .rlabel {
    margin: 10px 0 3px;
    font-size: 0.72em;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    color: var(--k-ink-3);
  }
  .rtext,
  .rdata {
    margin: 0;
    white-space: pre-wrap;
    word-break: break-word;
    font-size: 0.82em;
    line-height: 1.5;
    color: var(--k-ink-2);
  }
  .rdata {
    font-family: var(--k-font-mono);
    font-size: 0.78em;
    background: var(--k-surface-2);
    border-radius: 7px;
    padding: 7px 9px;
  }
  .rerr {
    margin: 0 0 6px;
    color: var(--k-error);
    font-size: 0.84em;
  }

  .blank {
    max-width: 58ch;
    padding-top: 8vh;
  }
  .blank p {
    color: var(--k-ink-2);
    font-size: 0.9em;
    line-height: 1.6;
  }
  .blank .primary {
    margin-top: 6px;
  }
</style>
