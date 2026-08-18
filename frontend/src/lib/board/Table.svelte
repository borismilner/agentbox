<script>
  // A table: the handful of measurements that carry an argument. Cells are text
  // and nothing else, so there is no markup to trust here - the only decisions
  // this component makes are alignment and rules.
  let { tbl } = $props();
  const align = (i) => tbl.align?.[i] || "left";
  // Numbers only line up when they are right-aligned AND monospaced in their
  // digits, which is what tabular-nums does to a proportional face. A column the
  // author aligned right is a column of numbers; that is what the alignment means.
  const numeric = (i) => align(i) === "right";
</script>

<figure class="tbl">
  <div class="scroll">
    <table>
      <thead>
        <tr>
          {#each tbl.head as h, i (i)}
            <th style:text-align={align(i)} class:num={numeric(i)}>{h}</th>
          {/each}
        </tr>
      </thead>
      <tbody>
        {#each tbl.rows as row, ri (ri)}
          <tr>
            {#each row as cell, ci (ci)}
              <td style:text-align={align(ci)} class:num={numeric(ci)}>{cell}</td>
            {/each}
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
  {#if tbl.caption}
    <figcaption>{tbl.caption}</figcaption>
  {/if}
</figure>

<style>
  /* The column, not a reading measure. 68ch of THIS font is about three quarters
     of the width the prose beside it occupies, because the prose is set larger
     and in the reading face - so a block capped that way arrives visibly
     narrower than the paragraph above it, and a table with three columns in it
     wraps every cell. Text wants a measure; evidence wants the column. */
  .tbl {
    margin: 4px 0 26px;
    max-width: 100%;
  }
  /* The table scrolls inside its own box rather than widening the step: a review
     whose page scrolls sideways has lost its reading column, and the column is
     what the whole surface is arranged around. */
  .scroll {
    overflow-x: auto;
    border: 1px solid var(--k-edge, #2a2f37);
    border-radius: 12px;
    background: var(--k-surface, #16181c);
  }
  /* 100% of the reading column, which the step's grid sizes from its prose. Two
     other widths were tried on screen and both were worse: 68ch of THIS font is
     three quarters of the prose's width and leaves the table visibly inset, and
     max-content pushes the table past the column so the last column is clipped
     and has to be scrolled to. A cell that wraps inside the reading measure is
     the better of the three, because the reader never has to scroll to finish a
     row. A table too wide for the column still scrolls, in the box below. */
  table {
    width: 100%;
    border-collapse: collapse;
    font-family: var(--k-font-ui, system-ui, sans-serif);
    font-size: 0.95em;
  }
  /* Rules, not boxes. A grid of borders draws forty lines to separate six
     numbers; one hairline under the head and one between rows is what the eye
     actually needs to stay on a row. */
  th {
    padding: 11px 16px;
    font-family: var(--k-font-mono, ui-monospace, monospace);
    font-size: 0.76em;
    font-weight: 600;
    letter-spacing: 0.07em;
    text-transform: uppercase;
    color: var(--k-ink-3, #8b93a1);
    white-space: nowrap;
    border-bottom: 1px solid var(--k-edge, #2a2f37);
    background: var(--k-surface-2, #1c2027);
  }
  td {
    padding: 10px 16px;
    color: var(--k-ink, #e6e9ef);
    line-height: 1.5;
    vertical-align: top;
    border-top: 1px solid var(--k-edge-soft, #23272f);
    text-wrap: pretty;
  }
  tbody tr:first-child td {
    border-top: 0;
  }
  tbody tr:hover td {
    background: color-mix(in srgb, var(--k-accent, #4f9cd8) 7%, transparent);
  }
  .num {
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
  figcaption {
    margin: 10px 2px 0;
    font-size: 0.88em;
    line-height: 1.5;
    color: var(--k-ink-3, #8b93a1);
    text-wrap: pretty;
  }
</style>
