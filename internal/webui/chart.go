package webui

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Charts (FR38): a ```chart fence carrying a JSON spec, drawn as inline SVG.
//
// The Gio build rasterised these through gonum/plot, which meant a theme change
// needed a re-render and a chart was a bitmap on a page of text. In a webview the
// better answer is an SVG that paints itself from the same CSS custom properties
// as everything else: it scales with the reading measure, it sharpens on a HiDPI
// screen, and it follows a light/dark switch without agentbox redrawing anything.
//
// The spec is deliberately the same one the Gio renderer accepted, so a document
// written for the old UI renders in the new one.

type chartSpec struct {
	Type   string        `json:"type"` // line | bar | scatter
	Title  string        `json:"title"`
	X      []string      `json:"x"`
	Series []chartSeries `json:"series"`
}

type chartSeries struct {
	Name   string    `json:"name"`
	Values []float64 `json:"values"`
}

// The series palette. Severity hues are deliberately excluded except at the end:
// a data series is not a warning, and a chart that looks like an error card is a
// chart that lies.
var seriesInks = []string{
	"var(--k-accent)",
	"var(--k-info)",
	"var(--k-success)",
	"var(--k-code-num)",
	"var(--k-code-fn)",
	"var(--k-warning)",
}

// chart geometry, in viewBox units; the SVG scales to whatever width it is given.
const (
	chW, chH   = 720.0, 300.0
	chL, chR   = 54.0, 14.0 // plot area insets
	chT, chB   = 34.0, 40.0
	chSteps    = 4  // horizontal gridlines
	chMaxLabel = 14 // x labels beyond this many are thinned
)

// renderChartSVG draws the spec. An unparseable or empty spec returns "", and the
// caller falls back to showing the source - the same rule the Gio renderer used,
// because a chart that cannot be drawn is still information the reader may need.
func renderChartSVG(spec string) string {
	var c chartSpec
	if err := json.Unmarshal([]byte(spec), &c); err != nil {
		return ""
	}
	if len(c.Series) == 0 {
		return ""
	}
	n := 0
	for _, s := range c.Series {
		if len(s.Values) > n {
			n = len(s.Values)
		}
	}
	if n == 0 {
		return ""
	}

	// A pie has no axes to share with the others, so it takes its own path.
	if t := strings.ToLower(c.Type); t == "pie" || t == "doughnut" || t == "donut" {
		return renderPieSVG(c, t != "pie")
	}

	lo, hi := bounds(c.Series)
	top := niceCeil(hi)
	bottom := 0.0
	if lo < 0 {
		bottom = -niceCeil(-lo)
	}
	if top == bottom {
		top = bottom + 1
	}

	plotW, plotH := chW-chL-chR, chH-chT-chB
	yOf := func(v float64) float64 {
		return chT + plotH - (v-bottom)/(top-bottom)*plotH
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<div class="k-chart"><svg viewBox="0 0 %g %g" role="img" preserveAspectRatio="xMidYMid meet"`, chW, chH)
	if c.Title != "" {
		fmt.Fprintf(&b, ` aria-label="%s"`, template(c.Title))
	}
	b.WriteString(`>`)

	if c.Title != "" {
		fmt.Fprintf(&b, `<text class="k-ch-title" x="%g" y="20">%s</text>`, chL, template(c.Title))
	}

	// Gridlines and their values first, so the data sits on top of them.
	for i := 0; i <= chSteps; i++ {
		v := bottom + (top-bottom)*float64(i)/chSteps
		y := yOf(v)
		fmt.Fprintf(&b, `<line class="k-ch-grid" x1="%g" y1="%.1f" x2="%g" y2="%.1f"/>`,
			chL, y, chW-chR, y)
		fmt.Fprintf(&b, `<text class="k-ch-tick" x="%g" y="%.1f" text-anchor="end">%s</text>`,
			chL-8, y+3.5, num(v))
	}
	// The zero line reads as an axis when the data crosses it.
	if bottom < 0 {
		fmt.Fprintf(&b, `<line class="k-ch-axis" x1="%g" y1="%.1f" x2="%g" y2="%.1f"/>`,
			chL, yOf(0), chW-chR, yOf(0))
	}

	switch strings.ToLower(c.Type) {
	case "bar", "column":
		drawBars(&b, c, n, plotW, yOf, bottom)
	case "scatter", "points":
		drawPoints(&b, c, n, plotW, yOf, false)
	case "area":
		drawArea(&b, c, n, plotW, yOf, bottom)
		drawPoints(&b, c, n, plotW, yOf, true)
	default:
		drawPoints(&b, c, n, plotW, yOf, true)
	}

	drawXLabels(&b, c, n, plotW, strings.EqualFold(c.Type, "bar"))
	drawLegend(&b, c)

	b.WriteString(`</svg></div>`)
	return b.String()
}

func bounds(series []chartSeries) (lo, hi float64) {
	first := true
	for _, s := range series {
		for _, v := range s.Values {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				continue
			}
			if first {
				lo, hi, first = v, v, false
				continue
			}
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	if first {
		return 0, 1
	}
	if hi < 0 {
		hi = 0
	}
	return lo, hi
}

// niceCeil rounds a maximum up to something a reader can divide by four in their
// head: 1, 2, 5 and 10 times a power of ten.
func niceCeil(v float64) float64 {
	if v <= 0 {
		return 1
	}
	mag := math.Pow(10, math.Floor(math.Log10(v)))
	for _, step := range []float64{1, 2, 2.5, 5, 10} {
		if v <= step*mag+1e-9 {
			return step * mag
		}
	}
	return 10 * mag
}

// num prints an axis value without trailing noise: whole numbers stay whole.
func num(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e7 {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(v, 'f', 2, 64), "0"), ".")
}

// drawBars groups the series inside each category slot, which is what makes two
// series comparable per category rather than stacked into one number.
func drawBars(b *strings.Builder, c chartSpec, n int, plotW float64, yOf func(float64) float64, base float64) {
	slot := plotW / float64(n)
	inner := slot * 0.72
	bw := inner / float64(len(c.Series))
	zero := yOf(base)
	if base < 0 {
		zero = yOf(0)
	}

	for si, s := range c.Series {
		ink := seriesInks[si%len(seriesInks)]
		for i, v := range s.Values {
			x := chL + float64(i)*slot + (slot-inner)/2 + float64(si)*bw
			y := yOf(v)
			h := zero - y
			if h < 0 { // a negative value grows downward from the zero line
				y, h = zero, -h
			}
			if h < 1 {
				h = 1
			}
			fmt.Fprintf(b, `<rect class="k-ch-bar" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"><title>%s%s</title></rect>`,
				x, y, math.Max(bw-1.5, 1), h, ink, label(s.Name), num(v))
		}
	}
}

// drawPoints draws a line series (line=true) or a scatter. A line still gets its
// points: on a seven-day chart the dots are what you actually read.
func drawPoints(b *strings.Builder, c chartSpec, n int, plotW float64, yOf func(float64) float64, line bool) {
	step := plotW
	if n > 1 {
		step = plotW / float64(n-1)
	}
	xOf := func(i int) float64 {
		if n == 1 {
			return chL + plotW/2
		}
		return chL + float64(i)*step
	}

	for si, s := range c.Series {
		ink := seriesInks[si%len(seriesInks)]
		if line && len(s.Values) > 1 {
			var pts strings.Builder
			for i, v := range s.Values {
				fmt.Fprintf(&pts, "%.1f,%.1f ", xOf(i), yOf(v))
			}
			fmt.Fprintf(b, `<polyline class="k-ch-line" points="%s" stroke="%s"/>`,
				strings.TrimSpace(pts.String()), ink)
		}
		for i, v := range s.Values {
			r := 2.6
			if !line {
				r = 3.4
			}
			fmt.Fprintf(b, `<circle class="k-ch-dot" cx="%.1f" cy="%.1f" r="%g" fill="%s"><title>%s%s</title></circle>`,
				xOf(i), yOf(v), r, ink, label(s.Name), num(v))
		}
	}
}

// drawArea fills under a line. With several series the fills are kept faint and
// drawn back to front, because an area chart that hides a series behind another
// is worse than the line chart it was trying to improve on.
func drawArea(b *strings.Builder, c chartSpec, n int, plotW float64, yOf func(float64) float64, base float64) {
	step := plotW
	if n > 1 {
		step = plotW / float64(n-1)
	}
	zero := yOf(math.Max(base, 0))

	for si, s := range c.Series {
		if len(s.Values) < 2 {
			continue
		}
		ink := seriesInks[si%len(seriesInks)]
		var pts strings.Builder
		fmt.Fprintf(&pts, "%.1f,%.1f ", chL, zero)
		for i, v := range s.Values {
			fmt.Fprintf(&pts, "%.1f,%.1f ", chL+float64(i)*step, yOf(v))
		}
		fmt.Fprintf(&pts, "%.1f,%.1f", chL+float64(len(s.Values)-1)*step, zero)
		fmt.Fprintf(b, `<polygon class="k-ch-area" points="%s" fill="%s"/>`, pts.String(), ink)
	}
}

// renderPieSVG draws the first series as slices, labelled with their share. The
// second series of a pie is meaningless, so it is ignored rather than guessed at.
func renderPieSVG(c chartSpec, doughnut bool) string {
	vals := c.Series[0].Values
	total := 0.0
	for _, v := range vals {
		if v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) {
			total += v
		}
	}
	if total <= 0 {
		return ""
	}

	const cx, cy, r = 190.0, 158.0, 108.0
	inner := 0.0
	if doughnut {
		inner = r * 0.56
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<div class="k-chart"><svg viewBox="0 0 %g %g" role="img" preserveAspectRatio="xMidYMid meet"`, chW, chH)
	if c.Title != "" {
		fmt.Fprintf(&b, ` aria-label="%s"`, template(c.Title))
	}
	b.WriteString(`>`)
	if c.Title != "" {
		fmt.Fprintf(&b, `<text class="k-ch-title" x="%g" y="20">%s</text>`, chL, template(c.Title))
	}

	angle := -math.Pi / 2 // start at twelve o'clock, where a reader starts
	for i, v := range vals {
		if v <= 0 {
			continue
		}
		share := v / total
		sweep := share * 2 * math.Pi
		ink := seriesInks[i%len(seriesInks)]
		name := ""
		if i < len(c.X) {
			name = c.X[i]
		}

		b.WriteString(slicePath(cx, cy, r, inner, angle, angle+sweep, ink,
			template(name), num(v), pct(share)))

		// The share sits on the slice when there is room for it, which is what
		// makes a pie readable without a legend lookup.
		if share > 0.06 {
			mid := angle + sweep/2
			lr := r * 0.66
			if doughnut {
				lr = (r + inner) / 2
			}
			fmt.Fprintf(&b, `<text class="k-ch-slice" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
				cx+lr*math.Cos(mid), cy+lr*math.Sin(mid)+4, pct(share))
		}
		angle += sweep
	}

	// Keys down the right, in slice order.
	y := 72.0
	for i, v := range vals {
		if v <= 0 {
			continue
		}
		name := "slice " + strconv.Itoa(i+1)
		if i < len(c.X) && c.X[i] != "" {
			name = c.X[i]
		}
		ink := seriesInks[i%len(seriesInks)]
		fmt.Fprintf(&b, `<rect x="352" y="%.1f" width="9" height="9" rx="2" fill="%s"/>`, y-9, ink)
		fmt.Fprintf(&b, `<text class="k-ch-legend" x="368" y="%.1f">%s</text>`, y, template(name))
		fmt.Fprintf(&b, `<text class="k-ch-tick" x="%g" y="%.1f" text-anchor="end">%s</text>`, chW-chR, y, num(v))
		y += 20
		if y > chH-8 {
			break
		}
	}

	b.WriteString(`</svg></div>`)
	return b.String()
}

// slicePath is one wedge (or ring segment for a doughnut). A single slice that is
// the whole pie cannot be drawn as an arc - the start and end points coincide - so
// it becomes a circle.
func slicePath(cx, cy, r, inner, from, to float64, ink, name, value, share string) string {
	title := "<title>" + strings.TrimSpace(name+" "+value+" ("+share+")") + "</title>"
	if to-from >= 2*math.Pi-1e-9 {
		if inner > 0 {
			return fmt.Sprintf(`<path class="k-ch-slice-shape" d="M %g %g m -%g 0 a %g %g 0 1 0 %g 0 a %g %g 0 1 0 -%g 0 M %g %g m -%g 0 a %g %g 0 1 1 %g 0 a %g %g 0 1 1 -%g 0" fill="%s" fill-rule="evenodd">%s</path>`,
				cx, cy, r, r, r, 2*r, r, r, 2*r, cx, cy, inner, inner, inner, 2*inner, inner, inner, 2*inner, ink, title)
		}
		return fmt.Sprintf(`<circle class="k-ch-slice-shape" cx="%g" cy="%g" r="%g" fill="%s">%s</circle>`, cx, cy, r, ink, title)
	}

	large := 0
	if to-from > math.Pi {
		large = 1
	}
	x1, y1 := cx+r*math.Cos(from), cy+r*math.Sin(from)
	x2, y2 := cx+r*math.Cos(to), cy+r*math.Sin(to)

	if inner <= 0 {
		return fmt.Sprintf(`<path class="k-ch-slice-shape" d="M %.1f %.1f L %.1f %.1f A %g %g 0 %d 1 %.1f %.1f Z" fill="%s">%s</path>`,
			cx, cy, x1, y1, r, r, large, x2, y2, ink, title)
	}
	ix2, iy2 := cx+inner*math.Cos(to), cy+inner*math.Sin(to)
	ix1, iy1 := cx+inner*math.Cos(from), cy+inner*math.Sin(from)
	return fmt.Sprintf(`<path class="k-ch-slice-shape" d="M %.1f %.1f A %g %g 0 %d 1 %.1f %.1f L %.1f %.1f A %g %g 0 %d 0 %.1f %.1f Z" fill="%s">%s</path>`,
		x1, y1, r, r, large, x2, y2, ix2, iy2, inner, inner, large, ix1, iy1, ink, title)
}

// pct prints a share: whole percentages stay whole, the rest keep one decimal.
func pct(share float64) string {
	v := share * 100
	if v == math.Trunc(v) {
		return strconv.FormatFloat(v, 'f', 0, 64) + "%"
	}
	return strconv.FormatFloat(v, 'f', 1, 64) + "%"
}

func drawXLabels(b *strings.Builder, c chartSpec, n int, plotW float64, bars bool) {
	if len(c.X) == 0 {
		return
	}
	// Every label when they fit, every other (or third) when they would collide.
	stride := 1
	for n/stride > chMaxLabel {
		stride++
	}
	slot := plotW / float64(n)
	step := plotW
	if n > 1 {
		step = plotW / float64(n-1)
	}
	for i := 0; i < n && i < len(c.X); i += stride {
		x := chL + float64(i)*step
		if bars {
			x = chL + float64(i)*slot + slot/2
		}
		fmt.Fprintf(b, `<text class="k-ch-tick" x="%.1f" y="%g" text-anchor="middle">%s</text>`,
			x, chH-chB+18, template(c.X[i]))
	}
}

// drawLegend puts the key on the title's line, right-aligned. It used to sit
// under the plot, where it crowded the x labels; up here it reads as part of the
// chart's heading and the bottom of the panel belongs to the axis alone.
func drawLegend(b *strings.Builder, c chartSpec) {
	named := 0
	for _, s := range c.Series {
		if s.Name != "" {
			named++
		}
	}
	if named == 0 || len(c.Series) < 2 {
		return
	}

	// Laid out right to left, so the last key ends at the plot's right edge.
	x := chW - chR
	for si := len(c.Series) - 1; si >= 0; si-- {
		s := c.Series[si]
		if s.Name == "" {
			continue
		}
		width := 13 + float64(len([]rune(s.Name)))*6.2
		x -= width
		ink := seriesInks[si%len(seriesInks)]
		fmt.Fprintf(b, `<rect x="%.1f" y="%.1f" width="8" height="8" rx="2" fill="%s"/>`, x, 11.0, ink)
		fmt.Fprintf(b, `<text class="k-ch-legend" x="%.1f" y="%.1f">%s</text>`, x+12, 19.0, template(s.Name))
		x -= 12
	}
}

func label(name string) string {
	if name == "" {
		return ""
	}
	return template(name) + ": "
}
