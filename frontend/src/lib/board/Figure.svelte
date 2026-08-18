<script>
  // A figure: the drawing or the screenshot a step needs, in the reading column
  // with the prose (FR101). Two forms arrive, and neither one reaches out of the
  // page - Go re-composed the markup element by element (walkthrough.SafeSVG) and
  // turned any image into a data: URI through the same budget every other picture
  // on the surface passes. So this component draws what it is given and never
  // fetches, and the CSP in index.html is the second lock on that.
  //
  // The drawing is injected rather than rendered because an SVG the author wrote
  // is the whole point of the field; it is the board's second and last {@html},
  // and frontend/policy_test.go names both.
  let { fig } = $props();
</script>

<figure class="fig" class:wide={fig.wide}>
  <div class="art">
    {#if fig.err}
      <!-- A figure that cannot be shown says so where the picture would have
           been. A blank frame reads as a surface that broke; a stated absence
           reads as a review with one thing missing from it, which is the truth. -->
      <p class="miss">
        <span class="miss-label">no picture here</span>
        {fig.err}{#if fig.alt}. It was described as: {fig.alt}{/if}
      </p>
    {:else if fig.svg}
      <div class="draw" role="img" aria-label={fig.alt || fig.caption || "diagram"}>
        {@html fig.svg}
      </div>
    {:else}
      <img src={fig.src} alt={fig.alt || fig.caption || ""} />
    {/if}
  </div>
  {#if fig.caption}
    <figcaption>{fig.caption}</figcaption>
  {/if}
</figure>

<style>
  /* The frame is the same frame a code block wears, because a figure is evidence
     of the same kind and a second visual language would make the page louder
     without making it clearer. Every var() carries a fallback: a token that
     resolves to nothing takes its whole declaration with it, and a figure that
     has lost its background still reads as working to everything except the
     screen. */
  .fig {
    margin: 4px 0 26px;
    max-width: 68ch;
  }
  .fig.wide {
    max-width: none;
  }
  .art {
    display: flex;
    justify-content: center;
    padding: 20px 22px;
    border: 1px solid var(--k-edge, #2a2f37);
    border-radius: 12px;
    background: var(--k-surface, #16181c);
  }
  /* The drawing scales to the column it is in, which is why the spec insists on a
     viewBox: with one, this rule is all the sizing anybody needs. */
  .draw {
    width: 100%;
    line-height: 0;
  }
  .draw :global(svg) {
    display: block;
    width: 100%;
    height: auto;
    max-height: 66vh;
    /* The two defaults a diagram usually wants, so an author who says nothing
       still gets a drawing that reads: strokes follow the ink, fills stay out of
       the way. Anything the markup states wins over these. */
    color: var(--k-ink-2, #c8cdd6);
    font-family: var(--k-font-ui, system-ui, sans-serif);
  }
  .draw :global(text) {
    fill: var(--k-ink, #e6e9ef);
    font-size: 13px;
  }
  img {
    display: block;
    max-width: 100%;
    height: auto;
    max-height: 66vh;
    border-radius: 6px;
  }
  .miss {
    margin: 0;
    font-size: 0.9em;
    color: var(--k-ink-3, #8b93a1);
    text-align: center;
  }
  .miss-label {
    display: block;
    font-family: var(--k-font-mono, ui-monospace, monospace);
    font-size: 0.82em;
    letter-spacing: 0.06em;
    color: var(--k-warning, #d8a33a);
    margin-bottom: 4px;
  }
  /* The caption is what the picture is, not what to conclude from it - the
     conclusion is the takeaway under the block. Quiet, and left-aligned with the
     prose rather than centred, so the eye returns to the same margin it left. */
  figcaption {
    margin: 10px 2px 0;
    font-size: 0.88em;
    line-height: 1.5;
    color: var(--k-ink-3, #8b93a1);
    text-wrap: pretty;
  }
</style>
