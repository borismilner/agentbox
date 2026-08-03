package webui

import (
	"strings"
	"testing"
)

// Math is parsed here rather than by a dependency, so these tests are the whole
// specification of which dollars mean TeX. The money cases matter as much as the
// formulas: an agent writing about a bill must not have its prose typeset.

func TestMathInline(t *testing.T) {
	got := RenderMarkdown("the mass is $E = mc^2$ exactly")

	if !strings.Contains(got, `data-display="0"`) {
		t.Fatalf("inline math should not be display math:\n%s", got)
	}
	if !strings.Contains(got, `<span class="k-math-src">E = mc^2</span>`) {
		t.Errorf("the TeX should travel as element text:\n%s", got)
	}
	if !strings.Contains(got, "the mass is ") || !strings.Contains(got, " exactly") {
		t.Errorf("the sentence around the formula was lost:\n%s", got)
	}
	if strings.Contains(got, "$") {
		t.Errorf("the delimiters leaked into the output:\n%s", got)
	}
}

func TestMathLeavesMoneyAlone(t *testing.T) {
	cases := []string{
		"it costs $5 and $10 in total",
		"the invoice was $1,200 and the refund $300",
		"$5$10 is two prices",
		"a bare $ sign",
		"$ 5 has a space after the sign",
	}
	for _, src := range cases {
		got := RenderMarkdown(src)
		if strings.Contains(got, "k-math") {
			t.Errorf("%q was typeset as math:\n%s", src, got)
		}
	}
}

func TestMathIgnoresCodeSpansAndEscapes(t *testing.T) {
	got := RenderMarkdown("run `$HOME/bin` and `$PATH` first")
	if strings.Contains(got, "k-math") {
		t.Errorf("a shell variable in a code span is not math:\n%s", got)
	}

	// A backslash-escaped dollar is a dollar. goldmark's own escape handling does
	// this once every parser registered for `\` has declined, which is the whole
	// reason the inline parser may claim that trigger.
	got = RenderMarkdown(`costs \$5 and \$10`)
	if strings.Contains(got, "k-math") {
		t.Errorf("escaped dollars are not delimiters:\n%s", got)
	}
	if !strings.Contains(got, "$5") || !strings.Contains(got, "$10") {
		t.Errorf("the escaped dollars should render as dollars:\n%s", got)
	}
}

func TestMathDisplayBlock(t *testing.T) {
	got := RenderMarkdown("$$\n\\int_0^1 x^2 \\, dx = \\frac{1}{3}\n$$\n")

	if !strings.Contains(got, `data-display="1"`) {
		t.Fatalf("a $$ block is display math:\n%s", got)
	}
	if !strings.Contains(got, `\int_0^1 x^2 \, dx = \frac{1}{3}`) {
		t.Errorf("the TeX was mangled:\n%s", got)
	}
	if strings.Contains(got, "<br>") {
		t.Errorf("hard wraps must not reach inside an equation:\n%s", got)
	}
}

func TestMathDisplaySpellings(t *testing.T) {
	cases := map[string]string{
		"a $$ block":            "$$\nx^2\n$$\n",
		"a one-line $$ block":   "$$x^2$$\n",
		`a \[ \] block`:         "\\[\nx^2\n\\]\n",
		"a ```math fence":       "```math\nx^2\n```\n",
		"a ```katex fence":      "```katex\nx^2\n```\n",
		"TeX on the fence line": "$$x^2\n$$\n",
	}
	for name, src := range cases {
		got := RenderMarkdown(src)
		if !strings.Contains(got, `data-display="1"`) {
			t.Errorf("%s should be display math:\n%s", name, got)
		}
		if !strings.Contains(got, "x^2") {
			t.Errorf("%s lost its TeX:\n%s", name, got)
		}
		if strings.Contains(got, "k-code") {
			t.Errorf("%s rendered as a code block:\n%s", name, got)
		}
	}
}

func TestMathInterruptsParagraph(t *testing.T) {
	// How an agent actually writes it: no blank line before the equation.
	got := RenderMarkdown("The area is\n$$\n\\pi r^2\n$$\nwhich is the usual result.")

	if !strings.Contains(got, `data-display="1"`) {
		t.Fatalf("an equation should interrupt the paragraph:\n%s", got)
	}
	if !strings.Contains(got, "The area is") || !strings.Contains(got, "which is the usual result.") {
		t.Errorf("the prose on either side was lost:\n%s", got)
	}
}

func TestMathDisplayInsideALine(t *testing.T) {
	// `$$x$$` with prose after it on the same line is a paragraph, not a block.
	// The block parser has to decline it or it would swallow the rest of the line.
	got := RenderMarkdown("$$x^2$$ and then some prose")

	if !strings.Contains(got, "k-math") {
		t.Fatalf("the formula was lost:\n%s", got)
	}
	if !strings.Contains(got, "and then some prose") {
		t.Errorf("the prose after the closer was eaten:\n%s", got)
	}
}

func TestMathParenSpellingInline(t *testing.T) {
	got := RenderMarkdown(`so \(a^2 + b^2\) holds`)
	if !strings.Contains(got, `data-display="0"`) {
		t.Fatalf(`\( \) should be inline math:`+"\n%s", got)
	}
	if !strings.Contains(got, "a^2 + b^2") {
		t.Errorf("the TeX was lost:\n%s", got)
	}
}

func TestMathUnterminatedKeepsTheText(t *testing.T) {
	// A `$$` nobody closed. The rest of the message must survive: swallowing it
	// into a formula would lose what the agent said.
	got := RenderMarkdown("$$\nx^2 is what I meant\nand this is the next thought\n")

	if strings.Contains(got, "k-math") {
		t.Fatalf("an unterminated delimiter is not an equation:\n%s", got)
	}
	if !strings.Contains(got, "x^2 is what I meant") || !strings.Contains(got, "and this is the next thought") {
		t.Errorf("the text was swallowed:\n%s", got)
	}
}

func TestMathEscapesItsSource(t *testing.T) {
	// The TeX reaches the surface as element text, so it has to be escaped like
	// any other agent-authored string.
	got := RenderMarkdown("$a < b$ and $$c > d$$")
	if strings.Contains(got, "a < b") || strings.Contains(got, "c > d") {
		t.Errorf("angle brackets in TeX must be escaped:\n%s", got)
	}
	if !strings.Contains(got, "a &lt; b") {
		t.Errorf("the inline TeX should survive escaped:\n%s", got)
	}
}

func TestMathEmptyDelimitersAreNotMath(t *testing.T) {
	for _, src := range []string{"$$", "$$$$", "an empty $$ pair", "$$\n\n$$\n"} {
		got := RenderMarkdown(src)
		if strings.Contains(got, "k-math-src></") || strings.Contains(got, `<pre class="k-math-src"></pre>`) {
			t.Errorf("%q produced an empty formula:\n%s", src, got)
		}
	}
}
