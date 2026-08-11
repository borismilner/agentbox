package webui

import (
	"strings"
	"testing"
	"time"
)

// The renderer feeds every surface, so these tests are the closest thing the port
// has to a check on what a card body, an agent turn and the viewer all look like.

func TestRenderAlerts(t *testing.T) {
	got := RenderMarkdown("> [!NOTE]\n> A neutral aside.\n")

	if !strings.Contains(got, `data-alert="note"`) {
		t.Fatalf("the marker was not recognised:\n%s", got)
	}
	if !strings.Contains(got, `data-tone="info"`) {
		t.Errorf("note should carry the info tone:\n%s", got)
	}
	if !strings.Contains(got, ">Note<") {
		t.Errorf("the panel should be titled:\n%s", got)
	}
	// The panel says "Note" in its own head; repeating the raw marker in the body
	// is exactly what the transformer exists to prevent.
	if strings.Contains(got, "[!NOTE]") {
		t.Errorf("the marker text leaked into the body:\n%s", got)
	}
	if !strings.Contains(got, "A neutral aside.") {
		t.Errorf("the body was lost:\n%s", got)
	}
	if strings.Contains(got, "<blockquote") {
		t.Errorf("an alert is a panel, not a quote:\n%s", got)
	}
}

func TestRenderAlertTones(t *testing.T) {
	cases := map[string]struct{ tone, title string }{
		"NOTE":      {"info", "Note"},
		"TIP":       {"success", "Tip"},
		"IMPORTANT": {"accent", "Important"},
		"WARNING":   {"warning", "Warning"},
		"CAUTION":   {"error", "Caution"},
		// GitHub accepts the lowercase form too.
		"warning": {"warning", "Warning"},
	}
	for marker, want := range cases {
		got := RenderMarkdown("> [!" + marker + "]\n> body\n")
		if !strings.Contains(got, `data-tone="`+want.tone+`"`) {
			t.Errorf("[!%s]: want tone %s\n%s", marker, want.tone, got)
		}
		if !strings.Contains(got, ">"+want.title+"<") {
			t.Errorf("[!%s]: want title %s\n%s", marker, want.title, got)
		}
	}
}

// Text on the marker's own line has to survive, and a plain quote has to stay a
// plain quote - the transformer must not treat every bracket as an alert.
func TestRenderAlertInlineAndPlain(t *testing.T) {
	inline := RenderMarkdown("> [!CAUTION] rewrites history\n> for everyone\n")
	if !strings.Contains(inline, "rewrites history") {
		t.Errorf("text after the marker was dropped:\n%s", inline)
	}
	if strings.Contains(inline, "[!CAUTION]") {
		t.Errorf("the marker leaked:\n%s", inline)
	}

	plain := RenderMarkdown("> Agents agentbox; you answer.\n")
	if !strings.Contains(plain, "<blockquote") {
		t.Errorf("a plain quote should stay a quote:\n%s", plain)
	}

	notAnAlert := RenderMarkdown("> [!SHOUTING]\n> body\n")
	if strings.Contains(notAnAlert, "k-alert") {
		t.Errorf("an unknown marker is not an alert:\n%s", notAnAlert)
	}
	if !strings.Contains(notAnAlert, "[!SHOUTING]") {
		t.Errorf("an unknown marker stays visible rather than vanishing:\n%s", notAnAlert)
	}
}

func TestRenderCodeBlock(t *testing.T) {
	got := RenderMarkdown("```go\nfmt.Println(\"agentbox\")\n```\n")

	if !strings.Contains(got, `data-lang="go"`) {
		t.Errorf("the language badge is missing:\n%s", got)
	}
	if !strings.Contains(got, "data-copy") {
		t.Errorf("every block gets a copy button:\n%s", got)
	}
	if !strings.Contains(got, "chroma") {
		t.Errorf("highlighting should be class-based chroma output:\n%s", got)
	}
	// Short blocks stay unnumbered: line numbers on a two-liner are noise.
	if strings.Contains(got, "lntable") {
		t.Errorf("a short block should not carry line numbers:\n%s", got)
	}
}

func TestRenderCodeBlockLineNumbers(t *testing.T) {
	var b strings.Builder
	b.WriteString("```python\n")
	for range lineNumberFrom + 2 {
		b.WriteString("x = 1\n")
	}
	b.WriteString("```\n")

	got := RenderMarkdown(b.String())
	if !strings.Contains(got, "lntable") {
		t.Errorf("a listing past %d lines should carry line numbers:\n%s", lineNumberFrom, got)
	}
}

// A mermaid fence keeps its source in the DOM: the surface draws the diagram and
// hides it, so a diagram that fails to draw degrades to something readable.
func TestRenderMermaid(t *testing.T) {
	got := RenderMarkdown("```mermaid\nflowchart LR\n  A[\"an agent\"] --> B\n```\n")

	if !strings.Contains(got, `class="k-mermaid"`) {
		t.Fatalf("no diagram wrapper:\n%s", got)
	}
	if !strings.Contains(got, "k-mermaid-src") {
		t.Errorf("the source should travel with it:\n%s", got)
	}
	if !strings.Contains(got, "flowchart LR") {
		t.Errorf("the source was mangled:\n%s", got)
	}
	// The quotes in a mermaid label are why the source is text and not an
	// attribute; they must arrive escaped but intact.
	if !strings.Contains(got, "&quot;an agent&quot;") {
		t.Errorf("label quotes should be escaped, not dropped:\n%s", got)
	}
	if strings.Contains(got, "chroma") {
		t.Errorf("a diagram is not a code block:\n%s", got)
	}
}

func TestRenderTablesListsAndFootnotes(t *testing.T) {
	got := RenderMarkdown(
		"| a | b |\n| :- | -: |\n| 1 | 2 |\n\n" +
			"- [x] done\n- [ ] open\n\n" +
			"text[^1]\n\n[^1]: the note\n")

	for _, want := range []string{"<table>", "text-align:right", `type="checkbox"`, "checked", "footnote"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderEscapesHTML(t *testing.T) {
	got := RenderMarkdown("a body from an agent: <script>alert(1)</script>\n")
	if strings.Contains(got, "<script>") {
		t.Fatalf("raw HTML from an agent must not reach the surface:\n%s", got)
	}
}

// R-17: a fence past the ceiling is emitted plain, and the ceiling is what keeps
// the render bounded. Measured through this function on go1.26.1: chroma's output
// is about nine times its input at every size and costs a microsecond per byte (two
// with no language, because Analyse runs), so 1 MB in one fence was a second of CPU
// and 9.4 MB of HTML, once per open window - and the session transcript re-renders
// on a 60ms cadence.
func TestBigFenceIsPlainRatherThanHighlighted(t *testing.T) {
	small := strings.Repeat("x := 1\n", 10)
	if got := codeBlockHTML(small, "go"); !strings.Contains(got, "chroma") {
		t.Errorf("an ordinary fence lost its highlighting: %q", got[:min(200, len(got))])
	}

	// Past the byte ceiling.
	big := strings.Repeat("x := 1 // padding to make this line long enough to count\n", 1200)
	if len(big) <= highlightMaxBytes {
		t.Fatalf("fixture is %d bytes, which is under the ceiling this test is about", len(big))
	}
	got := codeBlockHTML(big, "go")
	if strings.Contains(got, "chroma") {
		t.Error("a fence past the byte ceiling was still highlighted")
	}
	if !strings.Contains(got, "not highlighted") {
		t.Errorf("the block does not say why it is plain: %q", got[:min(300, len(got))])
	}
	// Nothing is truncated: the ceiling is on the amplification, not the content.
	if n := strings.Count(got, "x := 1"); n != 1200 {
		t.Errorf("plain block carries %d of 1200 lines", n)
	}
	if float64(len(got)) > 1.5*float64(len(big)) {
		t.Errorf("plain block is %d bytes for %d in, which is not a bound", len(got), len(big))
	}

	// Past the line ceiling with few bytes, which is the shape a byte count misses.
	lines := strings.Repeat("x\n", highlightMaxLines+1)
	if got := codeBlockHTML(lines, "go"); strings.Contains(got, "chroma") {
		t.Errorf("%d one-character lines were highlighted", highlightMaxLines+1)
	}
}

// The bound the entry asked for, stated as a test rather than as a measurement in
// a document: a megabyte in one fence renders inside a deadline and under a
// multiple of its own size.
func TestFenceRenderStaysBounded(t *testing.T) {
	src := strings.Repeat("func f() { return 1 } // and a comment to widen the line\n", 20000)
	if len(src) < 1<<20 {
		t.Fatalf("fixture is %d bytes, want at least a megabyte", len(src))
	}

	start := time.Now()
	got := codeBlockHTML(src, "go")
	took := time.Since(start)

	if took > 2*time.Second {
		t.Errorf("one fence took %s; before the ceiling a megabyte cost about a second and there was no ceiling", took)
	}
	if float64(len(got)) > 2*float64(len(src)) {
		t.Errorf("one fence produced %d bytes from %d, a %.1fx amplification", len(got), len(src), float64(len(got))/float64(len(src)))
	}
}
