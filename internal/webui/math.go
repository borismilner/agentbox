package webui

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Math (M10 slice 4). Neither goldmark nor its GFM extension knows about TeX, so
// the delimiters agents actually write are parsed here - the same call the alert
// transformer made, for the same reason: this is sixty lines of work, not a
// dependency.
//
// Four spellings arrive in practice, and all four are the same thing by the time
// they leave: `$$...$$` and `\[...\]` as display math, `$...$` and `\(...\)`
// inline, plus a ```math fence for an agent that would rather be explicit.
//
// Go's half stops at the TeX. KaTeX's layout engine is JavaScript, so what this
// emits is inert and carries the source as element text - exactly the arrangement
// mermaid uses, and for the same two reasons. An attribute would need a second
// escaping rule for a language made of backslashes and braces, and a formula
// agentbox cannot typeset is still a formula the reader can read, so the source is
// what stays on screen when KaTeX gives up.
//
// The one judgement call is the single dollar, because `$5 and $10` is money and
// must survive as money. A closing dollar therefore may not follow whitespace and
// may not be followed by a digit, and an opening one may not be followed by
// whitespace. That is pandoc's rule, and it is the reason `$` needs a parser
// rather than a regex.

var (
	kindMathBlock  = ast.NewNodeKind("AgentBoxMathBlock")
	kindMathInline = ast.NewNodeKind("AgentBoxMathInline")
)

// mathBlock is display math. It holds its TeX as line segments the way a fenced
// code block does, so nothing is copied until it is rendered.
type mathBlock struct {
	ast.BaseBlock
	opener string // "$$" or `\[`, kept for the unterminated case
	closer string
	closed bool
}

func (*mathBlock) Kind() ast.NodeKind              { return kindMathBlock }
func (*mathBlock) IsRaw() bool                     { return true }
func (n *mathBlock) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

// mathInline is math inside a line of prose.
type mathInline struct {
	ast.BaseInline
	tex     string
	display bool // `$$x$$` mid-sentence: the author asked for display size
}

func (*mathInline) Kind() ast.NodeKind              { return kindMathInline }
func (n *mathInline) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

// --- the inline parser ------------------------------------------------------

type mathInlineParser struct{}

// Trigger takes both delimiter openings. `\` is safe to claim: goldmark handles
// backslash escapes in its own scan loop rather than in an inline parser, and it
// only marks the next byte escaped once every parser registered for `\` has
// declined - so returning nil here leaves `\*` escaping exactly as it was.
func (mathInlineParser) Trigger() []byte { return []byte{'$', '\\'} }

func (mathInlineParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, _ := block.PeekLine()
	opener, closer := inlineDelims(line)
	if opener == "" {
		return nil
	}
	rest := line[len(opener):]
	end := mathClose(rest, closer)
	if end < 0 {
		return nil
	}
	tex := strings.TrimSpace(string(rest[:end]))
	if tex == "" {
		return nil
	}
	block.Advance(len(opener) + end + len(closer))
	return &mathInline{tex: tex, display: opener == "$$"}
}

// inlineDelims reads the opening delimiter off the start of a line.
func inlineDelims(line []byte) (opener, closer string) {
	switch {
	case bytes.HasPrefix(line, []byte("$$")):
		return "$$", "$$"
	case bytes.HasPrefix(line, []byte(`\(`)):
		return `\(`, `\)`
	case len(line) > 1 && line[0] == '$':
		// `$ 5` is a price with a space in front of it. Math never opens on blank.
		if isMathSpace(line[1]) {
			return "", ""
		}
		return "$", "$"
	}
	return "", ""
}

// mathClose finds the closing delimiter, or reports -1 if this run of text does
// not contain one. Dollar delimiters carry the money rules; `\)` needs none,
// because nothing else writes it.
func mathClose(rest []byte, closer string) int {
	for i := 0; i+len(closer) <= len(rest); i++ {
		if string(rest[i:i+len(closer)]) != closer {
			continue
		}
		if closer[0] == '$' {
			if backslashRun(rest, i)%2 == 1 {
				continue // `\$` is a literal dollar sign, not a delimiter
			}
			if i == 0 || isMathSpace(rest[i-1]) {
				continue // "...and $10" - a price, not the end of a formula
			}
			if next := i + len(closer); next < len(rest) && isDigit(rest[next]) {
				continue // `$5$10` is two prices
			}
		}
		return i
	}
	return -1
}

// backslashRun counts the backslashes immediately before i, so an escaped
// backslash in front of a delimiter does not read as escaping the delimiter.
func backslashRun(b []byte, i int) int {
	n := 0
	for i-n-1 >= 0 && b[i-n-1] == '\\' {
		n++
	}
	return n
}

func isMathSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
func isDigit(c byte) bool     { return c >= '0' && c <= '9' }

// --- the block parser -------------------------------------------------------

type mathBlockParser struct{}

func (mathBlockParser) Trigger() []byte { return []byte{'$', '\\'} }

func (mathBlockParser) Open(_ ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, segment := reader.PeekLine()
	pos := pc.BlockIndent()
	if pos < 0 || pos >= len(line) {
		return nil, parser.NoChildren
	}
	rest := line[pos:]

	var opener, closer string
	switch {
	case bytes.HasPrefix(rest, []byte("$$")):
		opener, closer = "$$", "$$"
	case bytes.HasPrefix(rest, []byte(`\[`)):
		opener, closer = `\[`, `\]`
	default:
		return nil, parser.NoChildren
	}

	body := rest[len(opener):]
	start := segment.Start + pos + len(opener)
	node := &mathBlock{opener: opener, closer: closer}

	if i := bytes.Index(body, []byte(closer)); i >= 0 {
		if !util.IsBlank(body[i+len(closer):]) {
			// `$$x$$ and then prose`: the line is a paragraph that happens to open
			// with math, so it belongs to the inline parser. Claiming it here would
			// eat everything after the closer.
			return nil, parser.NoChildren
		}
		node.Lines().Append(text.NewSegment(start, start+i))
		node.closed = true
		reader.AdvanceToEOL()
		return node, parser.NoChildren
	}

	if !util.IsBlank(body) {
		// `$$\begin{aligned}` - the equation starts on the opening line.
		seg := text.NewSegment(start, segment.Stop)
		seg.ForceNewline = true
		node.Lines().Append(seg)
	}
	reader.AdvanceToEOL()
	return node, parser.NoChildren
}

func (mathBlockParser) Continue(node ast.Node, reader text.Reader, _ parser.Context) parser.State {
	mb := node.(*mathBlock)
	if mb.closed {
		return parser.Close
	}
	line, segment := reader.PeekLine()
	if i := bytes.Index(line, []byte(mb.closer)); i >= 0 {
		if !util.IsBlank(line[:i]) {
			mb.Lines().Append(text.NewSegment(segment.Start, segment.Start+i))
		}
		mb.closed = true
		reader.AdvanceToEOL()
		return parser.Close
	}
	seg := segment
	seg.ForceNewline = true
	mb.Lines().Append(seg)
	reader.AdvanceToEOL()
	return parser.Continue | parser.NoChildren
}

func (mathBlockParser) Close(_ ast.Node, _ text.Reader, _ parser.Context) {}

// CanInterruptParagraph is what makes `prose\n$$\nx\n$$` work, which is how an
// agent writes it: no blank line before the equation.
func (mathBlockParser) CanInterruptParagraph() bool { return true }

// CanAcceptIndentedLine stays false so a four-space indent is still a code block.
func (mathBlockParser) CanAcceptIndentedLine() bool { return false }

// --- rendering --------------------------------------------------------------

type mathRenderer struct{}

func (m *mathRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindMathBlock, m.block)
	reg.Register(kindMathInline, m.inline)
}

func (m *mathRenderer) block(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	mb := node.(*mathBlock)
	tex := strings.Trim(linesText(node, source), "\n")
	if strings.TrimSpace(tex) == "" {
		return ast.WalkSkipChildren, nil
	}
	if !mb.closed {
		// A delimiter nobody closed. Hand the text back as prose: swallowing the
		// rest of a message into a formula would lose the message.
		w.WriteString("<p>" + template(mb.opener+"\n"+tex) + "</p>")
		return ast.WalkSkipChildren, nil
	}
	w.WriteString(mathHTML(tex, true))
	return ast.WalkSkipChildren, nil
}

func (m *mathRenderer) inline(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	mi := node.(*mathInline)
	w.WriteString(mathHTML(mi.tex, mi.display))
	return ast.WalkSkipChildren, nil
}

// mathHTML is the inert element a surface typesets. Display math is a block that
// may scroll sideways; inline math has to sit in the line it was written in.
func mathHTML(tex string, display bool) string {
	if display {
		return `<div class="k-math" data-display="1"><pre class="k-math-src">` + template(tex) + `</pre></div>`
	}
	return `<span class="k-math" data-display="0"><span class="k-math-src">` + template(tex) + `</span></span>`
}
