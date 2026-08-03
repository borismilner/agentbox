package webui

import (
	"strings"
	"testing"
)

// A chart is drawn in Go and coloured by CSS, so what these tests check is the
// geometry being there and the colours being tokens rather than literals - the
// second is what lets a chart follow a light/dark switch with no re-render.

func TestChartBar(t *testing.T) {
	svg := renderChartSVG(`{"type":"bar","title":"Interruptions by day",
	  "x":["Mon","Tue","Wed"],
	  "series":[{"name":"asks","values":[4,7,3]},{"name":"vetoes","values":[1,2,0]}]}`)

	if svg == "" {
		t.Fatal("a valid bar spec should draw")
	}
	if n := strings.Count(svg, "<rect class=\"k-ch-bar\""); n != 6 {
		t.Errorf("bars = %d, want 6 (two series of three)", n)
	}
	if !strings.Contains(svg, "Interruptions by day") {
		t.Error("the title should be drawn")
	}
	for _, day := range []string{"Mon", "Tue", "Wed"} {
		if !strings.Contains(svg, ">"+day+"<") {
			t.Errorf("x label %q missing", day)
		}
	}
	// Both series named, so a key is needed to tell them apart.
	if !strings.Contains(svg, "k-ch-legend") {
		t.Error("two named series need a legend")
	}
	if !strings.Contains(svg, "var(--k-accent)") {
		t.Error("series colours must be tokens, so the chart re-themes with everything else")
	}
	if strings.Contains(svg, "#") {
		t.Errorf("no literal colours belong in the SVG:\n%s", svg)
	}
}

func TestChartLineAndArea(t *testing.T) {
	line := renderChartSVG(`{"type":"line","x":["a","b","c"],"series":[{"values":[1,4,2]}]}`)
	if !strings.Contains(line, "<polyline") {
		t.Errorf("a line chart needs a line:\n%s", line)
	}
	if n := strings.Count(line, "k-ch-dot"); n != 3 {
		t.Errorf("dots = %d, want 3: the points are what you read off a short series", n)
	}

	area := renderChartSVG(`{"type":"area","x":["a","b","c"],"series":[{"values":[1,4,2]}]}`)
	if !strings.Contains(area, "<polygon class=\"k-ch-area\"") {
		t.Errorf("an area chart needs a fill:\n%s", area)
	}

	scatter := renderChartSVG(`{"type":"scatter","x":["a","b"],"series":[{"values":[1,4]}]}`)
	if strings.Contains(scatter, "<polyline") {
		t.Errorf("a scatter has no line:\n%s", scatter)
	}
}

func TestChartPie(t *testing.T) {
	pie := renderChartSVG(`{"type":"pie","title":"By agent","x":["claude","codex","nudge"],
	  "series":[{"values":[38,19,6]}]}`)

	if n := strings.Count(pie, "k-ch-slice-shape"); n != 3 {
		t.Errorf("slices = %d, want 3", n)
	}
	// 38 of 63 is 60.3%: the share is what a reader takes from a pie, so it is
	// computed here rather than left to be eyeballed.
	if !strings.Contains(pie, "60.3%") {
		t.Errorf("expected the leading slice's share in:\n%s", pie)
	}
	for _, name := range []string{"claude", "codex", "nudge"} {
		if !strings.Contains(pie, name) {
			t.Errorf("key %q missing", name)
		}
	}

	doughnut := renderChartSVG(`{"type":"doughnut","x":["a","b"],"series":[{"values":[1,1]}]}`)
	if !strings.Contains(doughnut, "k-ch-slice-shape") {
		t.Errorf("a doughnut is a pie with a hole, not nothing:\n%s", doughnut)
	}

	// A single slice cannot be drawn as an arc (its ends coincide) - it is a
	// circle, and a chart that renders as nothing would be worse than the source.
	whole := renderChartSVG(`{"type":"pie","x":["all"],"series":[{"values":[5]}]}`)
	if !strings.Contains(whole, "<circle") {
		t.Errorf("a single slice should be a full circle:\n%s", whole)
	}
}

// An undrawable spec returns nothing, and the renderer falls back to showing the
// source: the numbers are still information the reader may need.
func TestChartFallsBack(t *testing.T) {
	cases := map[string]string{
		"not json":     `{nope`,
		"no series":    `{"type":"bar","x":["a"]}`,
		"empty series": `{"type":"bar","series":[]}`,
		"no values":    `{"type":"bar","series":[{"name":"x","values":[]}]}`,
		"empty pie":    `{"type":"pie","series":[{"values":[0,0]}]}`,
	}
	for name, spec := range cases {
		if got := renderChartSVG(spec); got != "" {
			t.Errorf("%s: expected no SVG, got:\n%s", name, got)
		}
	}

	// And the markdown path shows the source when the SVG comes back empty.
	md := RenderMarkdown("```chart\n{nope\n```\n")
	if !strings.Contains(md, "k-code") || !strings.Contains(md, "nope") {
		t.Errorf("an unparseable chart should render as its source:\n%s", md)
	}

	good := RenderMarkdown("```chart\n{\"type\":\"bar\",\"series\":[{\"values\":[1]}]}\n```\n")
	if !strings.Contains(good, "k-chart") || strings.Contains(good, "k-code") {
		t.Errorf("a good chart should replace the code block:\n%s", good)
	}
}

func TestChartScales(t *testing.T) {
	// Negative values need a zero line to read against.
	neg := renderChartSVG(`{"type":"bar","x":["a","b"],"series":[{"values":[-4,6]}]}`)
	if !strings.Contains(neg, "k-ch-axis") {
		t.Errorf("data crossing zero needs the zero line drawn:\n%s", neg)
	}

	// Axis maxima round to something divisible by four in your head.
	for spec, want := range map[string]string{
		`{"type":"bar","x":["a"],"series":[{"values":[7]}]}`:  ">10<",
		`{"type":"bar","x":["a"],"series":[{"values":[38]}]}`: ">50<",
		`{"type":"bar","x":["a"],"series":[{"values":[91]}]}`: ">100<",
	} {
		if got := renderChartSVG(spec); !strings.Contains(got, want) {
			t.Errorf("spec %s: want a %s gridline label\n%s", spec, want, got)
		}
	}

	// A long series thins its labels rather than overprinting them.
	var vals, xs []string
	for i := range 40 {
		vals = append(vals, "1")
		xs = append(xs, `"d`+string(rune('a'+i%26))+`"`)
	}
	long := renderChartSVG(`{"type":"line","x":[` + strings.Join(xs, ",") +
		`],"series":[{"values":[` + strings.Join(vals, ",") + `]}]}`)
	if n := strings.Count(long, "text-anchor=\"middle\""); n > chMaxLabel {
		t.Errorf("x labels = %d, want at most %d", n, chMaxLabel)
	}
}
