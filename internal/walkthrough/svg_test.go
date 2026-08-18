package walkthrough

import (
	"strings"
	"testing"
)

// A figure's SVG is the only markup a walkthrough carries, so this file is the
// place the trust model is actually tested rather than asserted. Two halves: what
// a diagram is allowed to be, and every shape of "reaches out of the page" that
// has to come back as a refusal an author can act on.

const diagram = `<svg viewBox="0 0 240 80" xmlns="http://www.w3.org/2000/svg">
  <defs>
    <marker id="tip" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto">
      <path d="M0 0L8 4L0 8z" fill="currentColor"/>
    </marker>
  </defs>
  <g stroke="var(--k-edge)" fill="none">
    <rect x="4" y="20" width="90" height="40" rx="8" fill="var(--k-surface-2)"/>
    <line x1="98" y1="40" x2="140" y2="40" marker-end="url(#tip)" stroke="var(--k-accent)"/>
  </g>
  <text x="14" y="45" font-size="13" fill="var(--k-ink)">the daemon</text>
</svg>`

func TestSafeSVGKeepsADiagram(t *testing.T) {
	out, err := SafeSVG(diagram)
	if err != nil {
		t.Fatalf("a plain diagram was refused: %v", err)
	}
	for _, want := range []string{
		`<svg viewBox="0 0 240 80">`, // the namespace is implied in HTML and dropped
		`<marker id="tip"`,
		`marker-end="url(#tip)"`,
		`fill="var(--k-surface-2)"`,
		`<text x="14" y="45" font-size="13" fill="var(--k-ink)">the daemon</text>`,
		`</svg>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lost %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "xmlns") {
		t.Errorf("the namespace should be dropped, not carried:\n%s", out)
	}
}

// The output is composed here, character by character, which is the claim the
// board's injection policy rests on. Text inside <text> is the one place an
// author's bytes travel, and they arrive escaped.
func TestSafeSVGEscapesTheWordsItCarries(t *testing.T) {
	out, err := SafeSVG(`<svg viewBox="0 0 10 10"><text>a &lt; b &amp;&amp; c</text></svg>`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "< b") || !strings.Contains(out, "&lt; b") {
		t.Errorf("text was not escaped on the way out: %s", out)
	}
}

func TestSafeSVGRefusals(t *testing.T) {
	cases := []struct {
		name string
		svg  string
		want string
	}{
		{"a script element", `<svg viewBox="0 0 1 1"><script>fetch("//evil")</script></svg>`, "<script> is not allowed"},
		{"foreign objects", `<svg viewBox="0 0 1 1"><foreignObject><b>x</b></foreignObject></svg>`, "<foreignObject> is not allowed"},
		{"an event handler", `<svg viewBox="0 0 1 1"><rect onclick="x()" width="1" height="1"/></svg>`, "never handles events"},
		{"an inline style", `<svg viewBox="0 0 1 1"><rect style="fill:red" width="1" height="1"/></svg>`, "style attribute"},
		{"an external image", `<svg viewBox="0 0 1 1"><image href="https://evil.example/a.png"/></svg>`, "<image> is not allowed"},
		{"an xlink reference", `<svg xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 1 1"><use xlink:href="#x"/></svg>`, "never references anything outside itself"},
		{"a literal colour", `<svg viewBox="0 0 1 1"><rect fill="#0d6e75" width="1" height="1"/></svg>`, "takes its colours from the human's theme"},
		{"a named colour", `<svg viewBox="0 0 1 1"><rect fill="rebeccapurple" width="1" height="1"/></svg>`, "takes its colours from the human's theme"},
		{"a font family", `<svg viewBox="0 0 1 1"><text font-family="Georgia">x</text></svg>`, "the human's theme owns the face"},
		{"a remote paint server", `<svg viewBox="0 0 1 1"><rect fill="url(https://evil.example/p.svg#g)" width="1" height="1"/></svg>`, "takes its colours from the human's theme"},
		{"a remote marker", `<svg viewBox="0 0 1 1"><line marker-end="url(https://evil.example/m.svg#t)"/></svg>`, "url(#id) naming something defined in this same figure"},
		{"no viewBox", `<svg width="10" height="10"><rect width="1" height="1"/></svg>`, "needs a viewBox"},
		{"an animation", `<svg viewBox="0 0 1 1"><rect width="1" height="1"><animate attributeName="x" to="9"/></rect></svg>`, "<animate> is not allowed"},
		{"a style sheet", `<svg viewBox="0 0 1 1"><style>rect{fill:red}</style></svg>`, "<style> is not allowed"},
		{"words outside a text element", `<svg viewBox="0 0 1 1">the daemon</svg>`, "put words in a <text> element"},
		{"not svg at all", `<div>hello</div>`, "must start with an <svg> element"},
		{"broken xml", `<svg viewBox="0 0 1 1"><rect></svg>`, "does not parse as XML"},
		{"an anchor", `<svg viewBox="0 0 1 1"><a href="https://evil.example">x</a></svg>`, "<a> is not allowed"},
		{"a filter", `<svg viewBox="0 0 1 1"><rect filter="url(#f)" width="1" height="1"/></svg>`, "carries filter"},
		{"empty", "  ", "svg is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := SafeSVG(tc.svg)
			if err == nil {
				t.Fatalf("accepted, and emitted:\n%s", out)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("want an error containing %q, got %v", tc.want, err)
			}
		})
	}
}

// A refusal is only useful if it says what to write instead. This is the same
// rule the spec's other validation errors are held to (vision principle 9).
func TestSafeSVGColourRefusalNamesTheTokens(t *testing.T) {
	_, err := SafeSVG(`<svg viewBox="0 0 1 1"><rect fill="#123456" width="1" height="1"/></svg>`)
	if err == nil {
		t.Fatal("a literal colour was accepted")
	}
	for _, want := range []string{"currentColor", "var(--k-accent)", "--k-warning"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal never mentions %q: %v", want, err)
		}
	}
}

func TestSafeSVGBounds(t *testing.T) {
	big := "<svg viewBox=\"0 0 1 1\">" + strings.Repeat(`<rect width="1" height="1"/>`, 1400) + "</svg>"
	if _, err := SafeSVG(big); err == nil || !strings.Contains(err.Error(), "elements") {
		t.Errorf("an element flood was not bounded: %v", err)
	}
	huge := "<svg viewBox=\"0 0 1 1\"><text>" + strings.Repeat("x", MaxFigureSVG) + "</text></svg>"
	if _, err := SafeSVG(huge); err == nil || !strings.Contains(err.Error(), "the cap is") {
		t.Errorf("a byte flood was not bounded: %v", err)
	}
}
