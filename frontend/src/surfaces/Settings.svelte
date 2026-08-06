<script>
  import { bridge } from "../lib/bridge.js";
  import { applyThemeTo } from "../lib/tokens.js";

  // Settings. Two ideas from the reviewed design carry the whole surface.
  //
  // The preview is real: pending values go back to Go, which resolves them with
  // the same token builder every window uses, and the answer is applied to the
  // preview's subtree. So what you see before Save cannot drift from what you
  // get after it, and there is no palette duplicated in JavaScript.
  //
  // Save is honest: it lists the exact lines it wrote. agentbox edits your
  // config.toml in place - only the keys you changed, comments and untouched
  // keys intact - and the way to make that claim believable is to show it.

  let data = $state(null);
  let values = $state({});
  let base = $state({});
  let active = $state("appearance");
  let saving = $state(false);
  let note = $state("");
  let err = $state("");
  let written = $state([]);
  let previewEl = $state(null);

  const section = $derived(data?.sections?.find((s) => s.id === active) ?? null);
  const knobs = $derived(data ? data.sections.flatMap((s) => s.groups.flatMap((g) => g.knobs)) : []);
  const dirty = $derived(knobs.filter((k) => values[k.id] !== base[k.id]));
  const needsRestart = $derived(dirty.some((k) => k.restart));

  load();

  async function load() {
    const got = await bridge.settings();
    if (!got) return;
    data = got;
    values = Object.fromEntries(got.sections.flatMap((s) => s.groups.flatMap((g) => g.knobs.map((k) => [k.id, k.value]))));
    base = { ...values };
    err = got.err ?? "";
    repaint();
  }

  // The preview follows the controls at a debounce: dragging a slider should not
  // put a bridge call on every pixel.
  let pending = null;
  function repaint() {
    clearTimeout(pending);
    pending = setTimeout(async () => {
      const theme = await bridge.previewTheme(values).catch(() => null);
      if (theme && previewEl) applyThemeTo(previewEl, theme);
    }, 90);
  }

  function set(id, v) {
    values[id] = String(v);
    note = "";
    written = [];
    repaint();
  }

  function revert() {
    values = { ...base };
    note = "";
    written = [];
    err = "";
    repaint();
  }

  async function save() {
    saving = true;
    note = "";
    err = "";
    const res = await bridge.saveSettings(values).catch((e) => ({ err: String(e) }));
    saving = false;
    written = res?.written ?? [];
    note = res?.note ?? "";
    err = res?.err ?? "";
    if (!res?.err || written.length) await load();
  }

  // A short enum reads better as a segmented control; a long one belongs in a
  // select, where the options do not fight the panel for width.
  const isSegmented = (k) => k.kind === "enum" && k.enum.length <= 3 && k.enum.every((v) => v.length <= 12);
  const title = (s) => s.charAt(0).toUpperCase() + s.slice(1);
  const numeric = (k) => (k.kind === "float" ? Number(values[k.id]) : parseInt(values[k.id], 10));
  const shown = (k) => {
    const n = numeric(k);
    if (Number.isNaN(n)) return values[k.id];
    return k.unit ? `${n} ${k.unit}` : String(n);
  };
</script>

<section class="settings">
  <header>
    <div class="line">
      <h1>Settings</h1>
      {#if data}<span class="path" title={data.path}>{data.path}</span>{/if}
    </div>
    {#if data}
      <nav>
        {#each data.sections as s (s.id)}
          <button
            class:on={active === s.id}
            onclick={() => {
              active = s.id;
              note = "";
              written = [];
            }}>{s.title}</button
          >
        {/each}
      </nav>
    {/if}
  </header>

  {#if !data}
    <p class="empty">Reading {"~/.config/agentbox/config.toml"}…</p>
  {:else}
    <div class="body" class:split={section?.preview}>
      <div class="panel">
        {#if section?.blurb}<p class="blurb">{section.blurb}</p>{/if}

        {#each section?.groups ?? [] as g (g.title)}
          <div class="group">
            <span class="cap">
              {g.title}{#if g.caption}<em> · {g.caption}</em>{/if}
            </span>

            {#each g.knobs as k (k.id)}
              <div class="row" class:stacked={k.kind === "color"}>
                <span class="nm">
                  <span class="lbl">
                    {k.label}
                    {#if k.restart}<em class="tag" title="The daemon reads this at startup">restart</em>{/if}
                    {#if values[k.id] !== base[k.id]}<em class="dot" title="Changed, not yet written"></em>{/if}
                  </span>
                  {#if k.hint}<span class="hint">{k.hint}</span>{/if}
                </span>

                <span class="ctl">
                  {#if k.kind === "bool"}
                    <button
                      class="switch"
                      class:on={values[k.id] === "true"}
                      role="switch"
                      aria-checked={values[k.id] === "true"}
                      aria-label={k.label}
                      onclick={() => set(k.id, values[k.id] === "true" ? "false" : "true")}
                    ><span class="nub"></span></button>
                  {:else if isSegmented(k)}
                    <span class="seg">
                      {#each k.enum as opt}
                        <button aria-pressed={values[k.id] === opt} onclick={() => set(k.id, opt)}>{title(opt)}</button>
                      {/each}
                    </span>
                  {:else if k.kind === "enum"}
                    <select value={values[k.id]} onchange={(e) => set(k.id, e.currentTarget.value)}>
                      {#each k.enum as opt}<option value={opt}>{title(opt)}</option>{/each}
                    </select>
                  {:else if k.kind === "int" || k.kind === "float"}
                    <input
                      type="range"
                      min={k.min}
                      max={k.max}
                      step={k.step || 1}
                      value={numeric(k)}
                      oninput={(e) => set(k.id, e.currentTarget.value)}
                    />
                    <span class="val">{shown(k)}</span>
                  {:else if k.kind === "color"}
                    <span class="swatches">
                      {#each k.swatches ?? [] as sw}
                        <button
                          class="swatch"
                          style="background: {sw.hex}"
                          title={sw.name}
                          aria-label={sw.name}
                          aria-pressed={values[k.id] === sw.hex}
                          onclick={() => set(k.id, sw.hex)}
                        ></button>
                      {/each}
                      <input
                        type="color"
                        value={values[k.id] || "#7c8cf8"}
                        title="Custom accent"
                        oninput={(e) => set(k.id, e.currentTarget.value)}
                      />
                      {#if values[k.id]}
                        <button class="clear" title="Back to the theme's own accent" onclick={() => set(k.id, "")}>reset</button>
                      {/if}
                    </span>
                  {:else if k.kind === "command"}
                    <!-- An argv array, edited as the line you would type. Quotes
                         hold a word together because these take paths, which is
                         why the file stores an array and not a string. It is not
                         a shell: no globbing, no variables, no pipes, since what
                         is typed here is exec'd directly. -->
                    <input
                      type="text"
                      class="text mono"
                      value={values[k.id]}
                      placeholder="empty = let AgentBox decide"
                      spellcheck="false"
                      autocapitalize="off"
                      autocorrect="off"
                      oninput={(e) => set(k.id, e.currentTarget.value)}
                    />
                  {:else}
                    <input
                      type="text"
                      class="text"
                      list={k.suggest?.length ? `sg-${k.id}` : undefined}
                      value={values[k.id]}
                      placeholder={k.suggest?.length ? k.suggest[0] : ""}
                      spellcheck="false"
                      oninput={(e) => set(k.id, e.currentTarget.value)}
                    />
                    {#if k.suggest?.length}
                      <datalist id={`sg-${k.id}`}>
                        {#each k.suggest as s}<option value={s}></option>{/each}
                      </datalist>
                    {/if}
                  {/if}
                </span>
              </div>
            {/each}
          </div>
        {/each}

        {#if data.warnings?.length}
          <div class="group">
            <span class="cap">From the file</span>
            {#each data.warnings as w}<p class="warn">{w}</p>{/each}
          </div>
        {/if}
      </div>

      {#if section?.preview}
        <div class="preview" bind:this={previewEl}>
          <span class="cap">Live preview — card, toast, agent prose</span>

          <div class="pv-card">
            <span class="sev"></span>
            <div class="in">
              <div class="idline">
                <span class="pill"><span class="pd"></span>claude-code · grabbit</span>
                <span class="grow"></span>
                <span class="t">14:32</span>
              </div>
              <h3>Run DB migration on staging?</h3>
              <p class="pv-body">
                The diff adds a non-null column to <code>events</code>. Backfill takes about 4 minutes and
                holds a write lock for the last 30 seconds.
              </p>
              <div class="opts">
                <span class="opt primary"><span class="key">1</span>Run now</span>
                <span class="opt"><span class="key">2</span>Dry run</span>
                <span class="opt"><span class="key">3</span>Skip</span>
              </div>
            </div>
            <div class="foot"><span>expires in 4:32</span><span>default: Dry run</span><span class="grow"></span><span>2 waiting</span></div>
          </div>

          <div class="pv-toast">
            <span class="ic">✓</span>
            <span class="tx"><b>Build passed</b> · grabbit · 38s</span>
          </div>

          <div class="excerpt">
            <p>
              Resume works for <b>single-segment</b> downloads only — <code>resume.go</code> replays one
              offset as a <code>Range</code> header.
            </p>
            <pre><span class="com">// only seg[0] is persisted today</span>
<span class="kw">if</span> i &gt; <span class="num">0</span> &#123; <span class="kw">return</span> <span class="fn">errMultiSegmentResume</span> &#125;</pre>
          </div>
        </div>
      {/if}
    </div>

    <footer>
      <div class="state">
        {#if err}
          <span class="bad">{err}</span>
        {:else if note}
          <span class="ok">{note}</span>
        {:else if dirty.length}
          <span class="pend"
            >{dirty.length} key{dirty.length === 1 ? "" : "s"} to write{needsRestart ? " · one applies on restart" : ""}</span
          >
        {:else}
          <span class="idle">Saved values match the file.</span>
        {/if}
        {#if written.length}
          <span class="wrote">{written.join("  ·  ")}</span>
        {:else if dirty.length}
          <span class="wrote">{dirty.map((k) => k.id).join("  ·  ")}</span>
        {/if}
      </div>
      <button class="btn" disabled={!dirty.length || saving} onclick={revert}>Revert</button>
      <button class="btn primary" disabled={!dirty.length || saving} onclick={save}>{saving ? "Writing…" : "Save"}</button>
    </footer>
  {/if}
</section>

<style>
  .settings {
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: 100%;
  }

  header {
    padding: 16px 18px 0;
    border-bottom: 1px solid var(--k-edge-soft);
  }
  .line {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 14px;
  }
  h1 {
    margin: 0;
    font-size: 1.12rem;
    font-weight: 700;
    letter-spacing: -0.01em;
  }
  .path {
    font-family: var(--k-font-mono);
    font-size: 0.66rem;
    color: var(--k-ink-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 46ch;
  }
  nav {
    display: flex;
    gap: 2px;
    margin-top: 12px;
  }
  nav button {
    padding: 6px 12px;
    border-radius: 7px 7px 0 0;
    font-size: 0.8rem;
    color: var(--k-ink-3);
    border-bottom: 2px solid transparent;
  }
  nav button:hover {
    color: var(--k-ink-2);
  }
  nav button.on {
    color: var(--k-ink);
    border-bottom-color: var(--k-accent);
  }

  .body {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: 1fr;
    overflow: hidden;
  }
  .body.split {
    grid-template-columns: minmax(0, 1fr) minmax(340px, 46%);
  }
  .panel {
    min-width: 0;
    overflow-y: auto;
    padding: 16px 18px 26px;
    display: flex;
    flex-direction: column;
    gap: 22px;
  }
  .blurb {
    margin: 0;
    max-width: 68ch;
    font-family: var(--k-font-read);
    font-size: 0.9rem;
    line-height: 1.55;
    color: var(--k-ink-2);
  }

  /* A measure, not the full window: on a section with no preview beside it, a
   * label 900px from its control is two separate things on screen. */
  .group {
    display: flex;
    flex-direction: column;
    gap: 3px;
    max-width: 760px;
  }
  /* The shared label voice (Home's h2): UI caps for headings, mono for data. */
  .cap {
    font-size: 0.76rem;
    font-weight: 600;
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--k-ink-3);
    margin-bottom: 5px;
  }
  .cap em {
    font-style: normal;
    font-weight: 400;
    letter-spacing: 0.02em;
    text-transform: none;
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 18px;
    min-height: 34px;
    padding: 3px 0;
  }
  .row.stacked {
    align-items: flex-start;
  }
  .nm {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .lbl {
    display: flex;
    align-items: center;
    gap: 7px;
    font-size: 0.86rem;
  }
  .hint {
    font-size: 0.72rem;
    color: var(--k-ink-3);
  }
  .tag {
    font-style: normal;
    font-family: var(--k-font-mono);
    font-size: 0.56rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    padding: 1px 5px;
    border-radius: 4px;
    border: 1px solid color-mix(in srgb, var(--k-warning) 40%, transparent);
    color: var(--k-warning);
  }
  .dot {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: var(--k-accent);
  }

  .ctl {
    flex: 0 0 auto;
    display: flex;
    align-items: center;
    gap: 9px;
  }

  .switch {
    width: 34px;
    height: 19px;
    border-radius: 999px;
    background: var(--k-surface-3);
    border: 1px solid var(--k-edge);
    padding: 2px;
    display: flex;
    justify-content: flex-start;
  }
  .switch .nub {
    width: 13px;
    height: 13px;
    border-radius: 50%;
    background: var(--k-ink-3);
    transition: transform 120ms ease, background 120ms ease;
  }
  .switch.on {
    background: color-mix(in srgb, var(--k-accent) 30%, var(--k-surface-3));
    border-color: color-mix(in srgb, var(--k-accent) 55%, var(--k-edge));
  }
  .switch.on .nub {
    transform: translateX(15px);
    background: var(--k-accent);
  }

  .seg {
    display: inline-flex;
    gap: 2px;
    padding: 2px;
    border: 1px solid var(--k-edge);
    border-radius: 8px;
    background: var(--k-surface-2);
  }
  .seg button {
    padding: 2px 11px;
    border-radius: 6px;
    font-size: 0.75rem;
    color: var(--k-ink-3);
  }
  .seg button[aria-pressed="true"] {
    background: var(--k-surface-3);
    color: var(--k-ink);
  }
  .seg button:hover[aria-pressed="false"] {
    color: var(--k-ink-2);
  }

  select,
  .text {
    font: inherit;
    font-size: 0.78rem;
    color: var(--k-ink);
    background: var(--k-surface-2);
    border: 1px solid var(--k-edge);
    border-radius: 7px;
    padding: 3px 8px;
    max-width: 200px;
  }
  .text {
    width: 200px;
  }
  /* A command line is read a character at a time - a flag, a path, a placeholder
     - so it gets the mono face and more room than a font name needs. */
  .text.mono {
    width: 320px;
    max-width: 320px;
    font-family: var(--k-mono, ui-monospace, monospace);
    font-size: 0.74rem;
  }
  .text:focus,
  select:focus {
    outline: none;
    border-color: color-mix(in srgb, var(--k-accent) 55%, var(--k-edge));
  }

  input[type="range"] {
    width: 138px;
    accent-color: var(--k-accent);
  }
  .val {
    font-family: var(--k-font-mono);
    font-size: 0.7rem;
    color: var(--k-ink-3);
    font-variant-numeric: tabular-nums;
    min-width: 58px;
    text-align: right;
  }

  .swatches {
    display: flex;
    align-items: center;
    gap: 7px;
    flex-wrap: wrap;
  }
  .swatch {
    width: 22px;
    height: 22px;
    border-radius: 6px;
    border: 1px solid var(--k-edge);
  }
  .swatch[aria-pressed="true"] {
    box-shadow: 0 0 0 2px var(--k-surface), 0 0 0 3.5px var(--k-ink-2);
  }
  input[type="color"] {
    width: 22px;
    height: 22px;
    padding: 0;
    border: 1px solid var(--k-edge);
    border-radius: 6px;
    background: var(--k-surface-2);
  }
  .clear {
    font-family: var(--k-font-mono);
    font-size: 0.62rem;
    color: var(--k-ink-3);
  }
  .clear:hover {
    color: var(--k-ink);
  }

  .warn {
    margin: 0;
    font-size: 0.76rem;
    color: var(--k-warning);
  }
  .empty {
    margin: 44px auto;
    font-family: var(--k-font-read);
    color: var(--k-ink-3);
  }

  /* --- the preview ------------------------------------------------------- */

  .preview {
    border-left: 1px solid var(--k-edge-soft);
    background: var(--k-ground);
    padding: 16px 20px 24px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: var(--k-gap);
    color: var(--k-ink);
    font-family: var(--k-font-ui);
    font-size: var(--k-size);
  }
  .preview .cap {
    margin-bottom: 0;
  }

  .pv-card {
    position: relative;
    background: var(--k-surface);
    border: 1px solid var(--k-edge);
    border-radius: var(--k-radius);
    overflow: hidden;
  }
  .pv-card .sev {
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 3px;
    background: var(--k-warning);
  }
  .pv-card .in {
    padding: var(--k-pad) var(--k-pad) calc(var(--k-pad) - 2px) calc(var(--k-pad) + 4px);
    display: flex;
    flex-direction: column;
    gap: calc(var(--k-gap) * 0.55);
  }
  .idline {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .grow {
    flex: 1;
  }
  .idline .t {
    font-family: var(--k-font-mono);
    font-size: 0.72em;
    color: var(--k-ink-3);
  }
  /* The identity pill keeps a fixed hue: an agent's colour is hashed from its
   * name, not configured, so letting it follow the accent would advertise a
   * knob that does not exist. This is the hue claude-code lands on. */
  .preview {
    --pv-id: hsl(250 62% 68%);
  }
  .preview[data-mode="light"] {
    --pv-id: hsl(250 58% 42%);
  }
  .pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 2px 9px 2px 7px;
    border-radius: 999px;
    font-size: 0.78em;
    border: 1px solid color-mix(in srgb, var(--pv-id) 42%, transparent);
    background: color-mix(in srgb, var(--pv-id) 13%, transparent);
    color: color-mix(in srgb, var(--pv-id) 74%, var(--k-ink));
  }
  .pill .pd {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--pv-id);
  }
  .pv-card h3 {
    margin: 0;
    font-size: 1.14em;
    font-weight: 700;
    letter-spacing: -0.01em;
    line-height: 1.25;
  }
  .pv-body {
    margin: 0;
    font-family: var(--k-font-read);
    font-size: 0.97em;
    line-height: 1.55;
    color: var(--k-ink-2);
  }
  .opts {
    display: flex;
    gap: 7px;
    flex-wrap: wrap;
    margin-top: 2px;
  }
  .opt {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    padding: 5px 11px 5px 6px;
    border: 1px solid var(--k-edge);
    border-radius: calc(var(--k-radius) * 0.7);
    background: var(--k-surface-2);
    font-size: 0.88em;
  }
  .opt .key {
    font-family: var(--k-font-mono);
    font-size: 0.72em;
    min-width: 16px;
    height: 16px;
    line-height: 16px;
    text-align: center;
    border-radius: 4px;
    background: var(--k-surface-3);
    border: 1px solid var(--k-edge);
    color: var(--k-ink-3);
  }
  .opt.primary {
    border-color: color-mix(in srgb, var(--k-accent) 55%, transparent);
    background: color-mix(in srgb, var(--k-accent) 13%, var(--k-surface-2));
  }
  .pv-card .foot {
    display: flex;
    gap: 12px;
    align-items: center;
    padding: 7px calc(var(--k-pad) + 4px);
    border-top: 1px solid var(--k-edge-soft);
    background: color-mix(in srgb, var(--k-surface-3) 55%, transparent);
    font-family: var(--k-font-mono);
    font-size: 0.72em;
    color: var(--k-ink-3);
  }

  .pv-toast {
    display: flex;
    align-items: center;
    gap: 10px;
    background: var(--k-surface);
    border: 1px solid var(--k-edge);
    border-radius: var(--k-radius);
    padding: calc(var(--k-pad) * 0.62) var(--k-pad);
  }
  .pv-toast .ic {
    width: 17px;
    height: 17px;
    border-radius: 50%;
    border: 2px solid var(--k-success);
    display: grid;
    place-items: center;
    color: var(--k-success);
    font-size: 10px;
    flex: 0 0 auto;
  }
  .pv-toast .tx {
    font-size: 0.93em;
    color: var(--k-ink-2);
  }
  .pv-toast .tx b {
    color: var(--k-ink);
  }

  .excerpt {
    display: flex;
    flex-direction: column;
    gap: calc(var(--k-gap) * 0.5);
  }
  .excerpt p {
    margin: 0;
    font-family: var(--k-font-read);
    font-size: 1.02em;
    line-height: 1.62;
  }
  .excerpt code {
    font-family: var(--k-font-mono);
    font-size: 0.84em;
    background: var(--k-surface-2);
    border: 1px solid var(--k-edge-soft);
    border-radius: 4px;
    padding: 0.5px 4px;
  }
  .excerpt pre {
    margin: 0;
    padding: 10px 12px;
    overflow-x: auto;
    background: var(--k-surface-2);
    border: 1px solid var(--k-edge);
    border-radius: calc(var(--k-radius) * 0.8);
    font-family: var(--k-font-mono);
    font-size: 0.8em;
    line-height: 1.6;
  }
  .kw {
    color: var(--k-code-kw);
  }
  .num {
    color: var(--k-code-num);
  }
  .fn {
    color: var(--k-code-fn);
  }
  .com {
    color: var(--k-code-com);
    font-style: italic;
  }

  /* --- save bar ---------------------------------------------------------- */

  footer {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 9px 18px;
    border-top: 1px solid var(--k-edge-soft);
    background: var(--k-ground);
  }
  .state {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .state span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .state .ok {
    font-size: 0.76rem;
    color: var(--k-success);
  }
  .state .bad {
    font-size: 0.76rem;
    color: var(--k-error);
  }
  .state .pend {
    font-size: 0.76rem;
    color: var(--k-accent);
  }
  .state .idle {
    font-size: 0.76rem;
    color: var(--k-ink-3);
  }
  .state .wrote {
    font-family: var(--k-font-mono);
    font-size: 0.64rem;
    color: var(--k-ink-3);
  }
  .btn {
    padding: 4px 13px;
    border-radius: 7px;
    border: 1px solid var(--k-edge);
    background: var(--k-surface-2);
    font-size: 0.8rem;
    color: var(--k-ink-2);
  }
  .btn:hover:not(:disabled) {
    background: var(--k-surface-3);
    color: var(--k-ink);
  }
  .btn:disabled {
    opacity: 0.45;
    cursor: default;
  }
  .btn.primary {
    border-color: color-mix(in srgb, var(--k-accent) 55%, transparent);
    background: color-mix(in srgb, var(--k-accent) 16%, var(--k-surface-2));
    color: var(--k-ink);
  }
  .btn.primary:hover:not(:disabled) {
    background: color-mix(in srgb, var(--k-accent) 26%, var(--k-surface-2));
  }
</style>
