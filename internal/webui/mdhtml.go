package webui

import (
	"bytes"
	"path/filepath"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Markdown rendering, second edition. The Gio build had to walk goldmark's AST
// into a layout tree because NFR1 banned an HTML intermediate; a webview removes
// that constraint, so the 600-line renderer collapses into goldmark's own HTML
// output plus the six things HTML does not give us for free:
//
//   - GitHub alerts (> [!NOTE] and friends) as tinted panels. Not part of GFM as
//     goldmark ships it, so the marker is recognised in an AST transformer here
//     rather than by adding a dependency for sixty lines of work.
//   - Code blocks with chroma classes, a language badge, a copy button, and line
//     numbers once a block is long enough to need them.
//   - ```chart fences drawn as SVG (chart.go).
//   - ```mermaid fences handed to the surface, which is where mermaid's layout
//     engine lives.
//   - TeX, in the four spellings agents write it in (math.go). Handed to the
//     surface for the same reason mermaid is: KaTeX is JavaScript.
//   - Images, which are the one thing here that is a security question rather
//     than a rendering one, and are answered in images.go.
//
// Highlighting stays server-side and class-based on purpose: the classes are
// coloured by the same --k-code-* tokens as everything else, so code follows a
// theme change with no second theme to keep in sync and no JS highlighter in the
// bundle.
//
// One renderer feeds every surface - a card body, an agent turn, the viewer - so
// the styling lives in one place too (app.css, .k-md). A table that renders in
// the reader and not in a card is the kind of drift this arrangement prevents.

var (
	mdOnce   sync.Once
	md       goldmark.Markdown
	former   *chromahtml.Formatter
	numbered *chromahtml.Formatter
	// inline emits the token spans with no <pre>, no <code> and no line wrappers,
	// for a single line that lives inside somebody else's element (HighlightInline).
	inline *chromahtml.Formatter
)

// lineNumberFrom is where a code block stops being a snippet and starts being a
// listing you refer to by line.
const lineNumberFrom = 10

func engine() goldmark.Markdown {
	mdOnce.Do(func() {
		former = chromahtml.New(
			chromahtml.WithClasses(true),
			chromahtml.TabWidth(4),
		)
		numbered = chromahtml.New(
			chromahtml.WithClasses(true),
			chromahtml.TabWidth(4),
			chromahtml.WithLineNumbers(true),
			chromahtml.LineNumbersInTable(true),
		)
		inline = chromahtml.New(
			chromahtml.WithClasses(true),
			chromahtml.TabWidth(4),
			chromahtml.PreventSurroundingPre(true),
		)
		md = goldmark.New(
			goldmark.WithExtensions(extension.GFM, extension.Footnote, extension.DefinitionList, extension.Typographer),
			goldmark.WithParserOptions(
				parser.WithASTTransformers(
					util.Prioritized(&alertTransformer{}, 100),
					util.Prioritized(&imageBaseTransformer{}, 110),
					util.Prioritized(&tableCapTransformer{}, 120),
				),
				parser.WithBlockParsers(util.Prioritized(&mathBlockParser{}, 100)),
				parser.WithInlineParsers(util.Prioritized(&mathInlineParser{}, 150)),
				parser.WithAttribute(),
			),
			goldmark.WithRendererOptions(
				html.WithHardWraps(),
				renderer.WithNodeRenderers(
					util.Prioritized(&codeRenderer{}, 1),
					util.Prioritized(&mathRenderer{}, 1),
					util.Prioritized(&imageRenderer{}, 1),
				),
			),
		)
	})
	return md
}

// RenderMarkdown turns an item body (or an agent turn) into HTML for a surface.
// Untrusted content never reaches this with raw HTML enabled - goldmark escapes
// it, since a card body can come from any agent.
//
// It renders without a base directory, which is the honest answer for everything
// that arrives over the socket: prose has no place of its own, so a relative
// image path in it has nothing to be relative to.
func RenderMarkdown(src string) string { return RenderMarkdownIn(src, "") }

// RenderMarkdownIn renders a document that does have a directory of its own, so a
// relative image path resolves against baseDir instead of being refused
// (images.go). Only a caller that knows where the source came from may pass one,
// and it must be absolute - the daemon's working directory is nobody's base.
func RenderMarkdownIn(src, baseDir string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	// A fresh context per render: it carries the base, and goldmark makes one
	// anyway when the option is absent, so nothing is shared that was not before.
	pc := parser.NewContext()
	if filepath.IsAbs(baseDir) {
		pc.Set(baseDirKey, filepath.Clean(baseDir))
	}
	var buf bytes.Buffer
	if err := engine().Convert([]byte(src), &buf, parser.WithContext(pc)); err != nil {
		// Never lose the message to a render failure: show it as plain text.
		return "<p>" + template(src) + "</p>"
	}
	return buf.String()
}

func template(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return r.Replace(s)
}

// --- code, charts and diagrams ----------------------------------------------

// codeRenderer replaces goldmark's fenced-code output with chroma's, and takes
// over the two fences that are pictures rather than code. It also renders the
// blockquotes the alert transformer marked.
type codeRenderer struct{}

func (c *codeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, c.render)
	reg.Register(ast.KindCodeBlock, c.render)
	reg.Register(ast.KindBlockquote, c.blockquote)
}

func (c *codeRenderer) render(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	var lang string
	if fenced, ok := node.(*ast.FencedCodeBlock); ok {
		lang = string(fenced.Language(source))
	}

	src := linesText(node, source)

	switch lang {
	case "math", "katex":
		// The explicit spelling. An agent that would rather not think about how
		// many dollars mean what can fence the TeX instead.
		w.WriteString(mathHTML(strings.Trim(src, "\n"), true))
		return ast.WalkSkipChildren, nil
	case "chart":
		// A chart that cannot be drawn falls back to its source: the numbers are
		// still information the reader may need.
		if svg := renderChartSVG(src); svg != "" {
			w.WriteString(svg)
			return ast.WalkSkipChildren, nil
		}
	case "mermaid":
		// The source travels as the element's text, not an attribute: mermaid
		// labels are full of quotes and an attribute would need a second escaping
		// rule to get wrong. The surface renders it and replaces this node; if the
		// diagram fails, the source stays on screen.
		w.WriteString(`<div class="k-mermaid"><pre class="k-mermaid-src">` + template(src) + `</pre></div>`)
		return ast.WalkSkipChildren, nil
	}

	// An artifact fence is markup to run rather than markup to read (artifact.go).
	if isArtifactFence(lang, src) {
		w.WriteString(artifactBlock(src, "", "", false))
		return ast.WalkSkipChildren, nil
	}

	w.WriteString(codeBlockHTML(src, lang))
	return ast.WalkSkipChildren, nil
}

// codeBlockHTML is one highlighted block: chroma's classes, a language badge, a
// copy button, and line numbers once it is long enough to be a listing. It
// returns a string rather than writing through because the artifact block needs
// the same thing for its code tab, and one code block in agentbox should look like
// every other one.
// HighlightInline colours a single line with the same lexers and the same
// --k-code-* tokens a fenced block gets, with no block chrome around it: no
// container, no copy button, no line numbers. A tool row is one line of shell, and
// it was the only code in a conversation that was rendered as flat grey text -
// which reads as agentbox not highlighting anything at all, because a tool call is
// most of what an agent shows you.
//
// Unknown language, or one chroma cannot lex, comes back escaped and unstyled
// rather than raw: this goes into the page as HTML.
func HighlightInline(src, lang string) string {
	engine()
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	lexer := lexers.Get(lang)
	if lexer == nil {
		return template(src)
	}
	iter, err := chroma.Coalesce(lexer).Tokenise(nil, src)
	if err != nil {
		return template(src)
	}
	var b strings.Builder
	if err := inline.Format(&b, styles(), iter); err != nil {
		return template(src)
	}
	return b.String()
}

// A fence past this is emitted as plain escaped text, with the block's corner
// label saying so (R-17).
//
// Measured on this tree's own Go source, through this very function, on go1.26.1:
// the HTML comes out about NINE times the input at every size, and the time is
// about a microsecond per byte with a language named and two without, because an
// unlabelled fence pays `lexers.Analyse` as well. So 64 kB costs 69ms and 620 kB
// of HTML, 1 MB costs a second and 9.4 MB, and nothing stopped the second case.
// (R-17's entry says the amplification is 16x. On this build it is 9x, which
// changes the arithmetic and not the defect.)
//
// 64 kB is about 2000 lines of Go: past what an agent means by "show me this
// file", and the point where colour is worth less than the window it costs. The
// line ceiling is there for the shape bytes miss - 30,000 one-character lines is a
// span and a table cell each, and cheap to write.
const (
	highlightMaxBytes = 64 << 10
	highlightMaxLines = 2000
)

func codeBlockHTML(src, lang string) string {
	engine() // the formatters are built with the parser; an artifact can arrive first
	// Before the lexer is chosen, because choosing it is half the cost.
	if len(src) > highlightMaxBytes || countRealLines(src) > highlightMaxLines {
		return plainBlockHTML(src, lang)
	}
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Analyse(src)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	f := former
	if countRealLines(src) >= lineNumberFrom {
		f = numbered
	}

	var b strings.Builder
	b.WriteString(`<div class="k-code" data-lang="` + template(lang) + `">`)
	// The copy button carries no text of its own; the surface reads the block's
	// own <pre> when it is clicked, so there is nothing here to keep in sync.
	b.WriteString(`<button class="k-copy" type="button" data-copy title="copy this block">copy</button>`)
	iter, err := lexer.Tokenise(nil, src)
	if err != nil {
		b.WriteString("<pre><code>" + template(src) + "</code></pre>")
	} else if err := f.Format(&b, styles(), iter); err != nil {
		b.WriteString("<pre><code>" + template(src) + "</code></pre>")
	}
	b.WriteString(`</div>`)
	return b.String()
}

// plainBlockHTML is the same block with no chroma in it: the code is whole, the
// colours are missing, and the corner label says which. It reuses `data-lang`
// because that attribute IS the block's caption already (`.k-code::before` in
// app.css), so being honest about it costs no stylesheet change and no rebuild -
// and a silent fallback would read as "this language has no highlighter", which is
// a different and wrong explanation.
func plainBlockHTML(src, lang string) string {
	label := "not highlighted, " + kb(len(src))
	if lang != "" {
		label = lang + " · " + label
	}
	return `<div class="k-code" data-lang="` + template(label) + `">` +
		`<button class="k-copy" type="button" data-copy title="copy this block">copy</button>` +
		"<pre><code>" + template(src) + "</code></pre>" +
		`</div>`
}

// linesText is the text of a raw block - a fence, an equation - joined back
// together out of the segments goldmark recorded.
func linesText(node ast.Node, source []byte) string {
	var b bytes.Buffer
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(source))
	}
	return b.String()
}

func countRealLines(s string) int {
	n := strings.Count(strings.TrimRight(s, "\n"), "\n")
	if s != "" {
		n++
	}
	return n
}

// styles returns a style whose only job is to name token types; the actual
// colours come from CSS because chroma is running in class mode.
func styles() *chroma.Style {
	s, err := chroma.NewStyle("agentbox", chroma.StyleEntries{})
	if err != nil {
		return chroma.MustNewStyle("agentbox-fallback", chroma.StyleEntries{})
	}
	return s
}

// --- tables -----------------------------------------------------------------

// mdMaxTableRows is how many body rows of one table are rendered (R-26).
//
// Nothing counted them. Measured through this renderer: 10,000 rows is 0.6 MB of
// HTML in 41ms and 170,000 rows - a 6.2 MB document, which readsource will hand
// over because its ceiling is 4 MB of BYTES and says nothing about structure - is
// 11 MB of HTML in 722ms, which is a million DOM nodes for a surface that renders
// prose. The row is the unit that matters rather than the byte, the same way the
// code fence needed a line ceiling next to its byte one.
//
// 2000 is deliberately the fence's line ceiling: past two thousand rows a table
// has stopped being something anybody reads down and become a data file, and a
// data file wants show_document on the file, not a card built out of it.
const mdMaxTableRows = 2000

// tableCapTransformer drops the rows past the ceiling and says so under the
// table. It runs on the AST rather than in a renderer because GFM's table
// renderer is the extension's, and taking it over to count rows would mean
// owning its alignment and its cell markup as well.
type tableCapTransformer struct{}

func (tableCapTransformer) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || n.Kind() != extast.KindTable {
			return ast.WalkContinue, nil
		}
		// The cells hold nothing this cares about, and a 170,000-row table is
		// half a million of them.
		rows, extra := 0, []ast.Node(nil)
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if c.Kind() != extast.KindTableRow {
				continue // the header, which is not a row anybody counts
			}
			rows++
			if rows > mdMaxTableRows {
				extra = append(extra, c)
			}
		}
		if len(extra) == 0 {
			return ast.WalkSkipChildren, nil
		}
		for _, row := range extra {
			n.RemoveChild(n, row)
		}
		if p := n.Parent(); p != nil {
			p.InsertAfter(p, n, noteNode(
				"first "+thousands(mdMaxTableRows)+" of "+thousands(rows)+" rows shown"))
		}
		return ast.WalkSkipChildren, nil
	})
}

// noteNode is one line of agentbox's own prose inside somebody else's document.
// It is emphasised rather than classed: a class needs a stylesheet rule to be
// visible at all, and a note the reader cannot see is the silence this replaces.
func noteNode(text string) ast.Node {
	p := ast.NewParagraph()
	em := ast.NewEmphasis(1)
	em.AppendChild(em, ast.NewString([]byte(text)))
	p.AppendChild(p, em)
	return p
}

// --- GitHub alerts ----------------------------------------------------------

// alertKinds is the GitHub set, in GitHub's order, with the word agentbox puts on
// the panel. The mapping is here rather than in CSS because "caution" reading as
// an error and "tip" reading as a success is an AgentBox decision about severity, not
// a styling detail.
var alertKinds = map[string]struct {
	title string
	tone  string
}{
	"note":      {"Note", "info"},
	"tip":       {"Tip", "success"},
	"important": {"Important", "accent"},
	"warning":   {"Warning", "warning"},
	"caution":   {"Caution", "error"},
}

// alertTransformer finds `> [!NOTE]`-style blockquotes and marks them, removing
// the marker text so the panel does not repeat its own name in the body.
type alertTransformer struct{}

func (alertTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	source := reader.Source()
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		bq, ok := n.(*ast.Blockquote)
		if !ok {
			return ast.WalkContinue, nil
		}
		para, ok := bq.FirstChild().(*ast.Paragraph)
		if !ok || para.Lines().Len() == 0 {
			return ast.WalkContinue, nil
		}

		// Read the marker off the SOURCE line, not off the first inline node:
		// goldmark's link parser splits `[!NOTE]` into `[`, `!NOTE`, `]` while
		// deciding it is not a link, so the AST has no node holding the whole
		// marker to match against.
		line := para.Lines().At(0)
		kind, width := alertMarker(string(line.Value(source)))
		if kind == "" {
			return ast.WalkContinue, nil
		}

		bq.SetAttributeString("data-alert", []byte(kind))
		trimMarker(para, line.Start+width)
		if para.FirstChild() == nil {
			bq.RemoveChild(bq, para)
		}
		return ast.WalkContinue, nil
	})
}

// alertMarker reads the `[!KIND]` prefix off a line, returning the lowercased
// kind and how many bytes of the line the marker (plus the space after it) takes.
func alertMarker(line string) (kind string, width int) {
	lead := len(line) - len(strings.TrimLeft(line, " \t"))
	s := line[lead:]
	if !strings.HasPrefix(s, "[!") {
		return "", 0
	}
	end := strings.IndexByte(s, ']')
	if end < 3 {
		return "", 0
	}
	name := strings.ToLower(strings.TrimSpace(s[2:end]))
	if _, ok := alertKinds[name]; !ok {
		return "", 0
	}
	after := s[end+1:]
	pad := len(after) - len(strings.TrimLeft(after, " \t"))
	return name, lead + end + 1 + pad
}

// trimMarker removes the marker's inline nodes from the paragraph. It works by
// source position rather than by node count, because how many nodes the marker
// became is goldmark's business: a node that ends inside the marker goes, one
// that straddles the boundary is shortened, and the first node past it stays.
func trimMarker(para *ast.Paragraph, stop int) {
	for {
		child := para.FirstChild()
		if child == nil {
			return
		}
		txt, ok := child.(*ast.Text)
		if !ok {
			return
		}
		switch {
		case txt.Segment.Stop <= stop:
			// Wholly inside the marker. Removing the last node of the line takes
			// its soft break with it, which is what makes the body start clean.
			para.RemoveChild(para, txt)
		case txt.Segment.Start < stop:
			txt.Segment = txt.Segment.WithStart(stop)
			return
		default:
			return
		}
	}
}

// blockquote renders a marked blockquote as a tinted panel and an ordinary one as
// an ordinary quote.
func (c *codeRenderer) blockquote(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	v, marked := node.AttributeString("data-alert")
	if !marked {
		if entering {
			w.WriteString("<blockquote>\n")
		} else {
			w.WriteString("</blockquote>\n")
		}
		return ast.WalkContinue, nil
	}

	kind := ""
	switch t := v.(type) {
	case []byte:
		kind = string(t)
	case string:
		kind = t
	}
	meta, ok := alertKinds[kind]
	if !ok {
		meta.title, meta.tone = strings.ToUpper(kind), "info"
	}

	if entering {
		w.WriteString(`<div class="k-alert" data-alert="` + kind + `" data-tone="` + meta.tone + `">`)
		w.WriteString(`<p class="k-alert-head">` + alertIcon(kind) + `<span>` + meta.title + `</span></p>`)
		w.WriteString(`<div class="k-alert-body">`)
	} else {
		w.WriteString(`</div></div>`)
	}
	return ast.WalkContinue, nil
}

// alertIcon draws the mark GitHub readers expect, in agentbox's line weight.
func alertIcon(kind string) string {
	const open = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">`
	switch kind {
	case "tip":
		return open + `<path d="M9.5 18h5"/><path d="M10 21h4"/><path d="M12 3a6 6 0 0 0-3.5 10.9V15h7v-1.1A6 6 0 0 0 12 3z"/></svg>`
	case "important":
		return open + `<path d="M20 4H4v12h4v4l5-4h7z"/><path d="M12 8v3"/><circle cx="12" cy="13.6" r="0.9" fill="currentColor" stroke="none"/></svg>`
	case "warning":
		return open + `<path d="M12 4.2 21 19.4H3z"/><path d="M12 10v4.2"/><circle cx="12" cy="17" r="0.9" fill="currentColor" stroke="none"/></svg>`
	case "caution":
		return open + `<circle cx="12" cy="12" r="9"/><path d="M9 9l6 6M15 9l-6 6"/></svg>`
	}
	return open + `<circle cx="12" cy="12" r="9"/><path d="M12 11v5.4"/><circle cx="12" cy="7.9" r="0.95" fill="currentColor" stroke="none"/></svg>`
}

// ParseInline is used where a surface wants one line of markdown (a toast body, a
// table cell) without a wrapping paragraph.
func ParseInline(src string) string {
	out := RenderMarkdown(src)
	out = strings.TrimSpace(out)
	out = strings.TrimPrefix(out, "<p>")
	out = strings.TrimSuffix(out, "</p>")
	return out
}
