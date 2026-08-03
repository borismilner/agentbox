# ADR-0008: Markdown engine - goldmark + chroma, native renderer, native charts

Status: proposed

## Context

FR36-FR38: full markdown at reading-app quality, everywhere a body renders,
plus a document viewer. NFR1 bans webviews, so the easy path (render HTML)
is closed; the engine must produce toolkit widgets directly. The point of
the feature is that terminals render markdown poorly; a mediocre native
renderer would miss the point entirely.

## Decision

- Parse with goldmark: CommonMark-compliant, the de-facto Go standard
  (Hugo's parser), extension ecosystem covers GFM (tables, strikethrough,
  task lists, autolinks), footnotes, definition lists, GitHub alerts and
  emoji shortcodes.
- Render the goldmark AST directly to Gio layout in `internal/markdown`.
  No HTML intermediate. Real table layout with measured columns, alerts as
  tinted panels, the 760 px reading measure from 03-ui-ux.md.
- Code blocks: chroma (200+ languages, pure Go) with a dark/light style
  pair switched live with the theme.
- Charts: fenced `chart` blocks carry a small JSON spec
  (`{"type": "line|bar|pie|scatter", "data": ..., "title": ...}`, data
  inline or a CSV path); rendered natively via gonum/plot into the theme
  palette. Mermaid blocks: if mermaid-cli (`mmdc`) is installed, render
  through it to PNG and show the image; otherwise show highlighted mermaid
  source with a one-line hint. Reimplementing Mermaid natively is a
  project of its own and is explicitly out.
- Math (FR39) deferred; candidate go-latex/latex when it lands.

## Alternatives

- Webview/HTML: banned by NFR1, and rightly - it would drag a browser
  runtime into a notification daemon.
- glamour (charm's markdown renderer): targets ANSI terminals, the exact
  output quality we are escaping.
- Pre-rendering via headless chromium to images: heavy external dependency,
  fuzzy text on HiDPI, no selection or links.

## Consequences

- `internal/markdown` is the single biggest UI component; it gets its own
  milestone (M6) and golden-file tests (markdown in, layout tree out,
  rendered screenshots diffed in CI later).
- Dependencies grow by goldmark, chroma, gonum/plot - all pure Go, the
  cgo surface stays toolkit-only.
- The engine is the showcase: the M6 acceptance check is a gnarly
  real-world README with tables, alerts, code and a chart, pixel-clean in
  dark and light.

## Amendment, 2026-07-25 (M9 slice 5): second edition, for the webview

ADR-0009 replaced Gio with a webview, which removes the constraint this ADR
was written under. The parse side is unchanged - goldmark and chroma, same
extensions - but the render side is now HTML, and three of the decisions
above were made for a toolkit that no longer draws AgentBox:

- **No native layout tree.** goldmark's own HTML renderer replaces the
  600-line Gio renderer. `internal/webui/mdhtml.go` keeps only what HTML does
  not give for free: GitHub alerts (goldmark ships no extension for them, so
  an AST transformer recognises `[!NOTE]` and friends and a node renderer
  paints the panel), a language badge and copy button per code block, and
  line numbers past ten lines.
- **Highlighting is class-based**, not coloured at render time: chroma emits
  classes and the `--k-code-*` tokens colour them, so code follows a theme
  change with nothing re-rendered and no second palette to maintain.
- **Charts are SVG, drawn in Go** (`internal/webui/chart.go`) rather than
  rastered through gonum/plot. Same JSON spec, so documents written for the
  Gio build still render, plus `area` and `doughnut`. The colours are CSS
  tokens, which means a chart re-themes with the page, stays sharp on a HiDPI
  screen, and scales with the reading measure - none of which a bitmap did.
  gonum/plot went out of go.mod with `internal/ui` at the cutover, and
  `internal/markdown` - the Document model this ADR's first edition specified -
  is deleted with it: nothing renders a Document any more, so a session segment
  carries its markdown source and the UI decides how it reads.
- **Mermaid is real now.** The `mmdc` path in this ADR was never built and the
  fence rendered as source. Mermaid is a JavaScript layout engine and the UI
  is a webview, so mermaid ships **in the bundle** (not from a CDN: AgentBox
  has to work offline) and is imported on demand, in one lazy chunk, so a card
  that shows a one-line question never loads it. It is configured from the
  same CSS tokens as everything else, at `securityLevel: "strict"` - diagram
  source is agent-authored, and strict mode is what makes inserting mermaid's
  SVG output safe. A diagram that fails to draw leaves its source on screen.
- Two things HTML brought with it that had to be closed off: a link click
  would navigate the window away (a frameless card has no back button), so
  every link goes to the desktop browser through `Bridge.OpenURL`, which
  accepts http, https and mailto and nothing else; and raw HTML in an agent's
  body is escaped by goldmark, never rendered.

Cost: `frontend/dist` grows to ~3.4 MB because the diagram engine is
embedded, and the binary with it. That is the price of the feature and it is
paid once, in the binary, not per window.
