package webui

import (
	"strings"
	"testing"
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
