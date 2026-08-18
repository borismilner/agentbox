package walkthrough

// A figure's inline SVG is the one place a walkthrough carries markup rather than
// text, and the board's standing rule is that the only HTML it ever injects is
// HTML Go produced (frontend/policy_test.go states the bargain). So the markup an
// agent writes is not filtered - it is re-written. SafeSVG parses it, keeps only
// what is on the allow-lists below, and emits bytes this file composed itself.
// Nothing the author wrote reaches the surface verbatim except attribute values
// that passed a pattern, and the text inside <text>, which is escaped.
//
// This deliberately disagrees with images.go, which refuses SVG outright for a
// markdown image and says why: a picture from an agent is a chart or a mermaid
// fence, and neither needs a parser for somebody else's XML. A walkthrough figure
// is the case that argument does not cover. The diagram IS the explanation - the
// request that led here was a flow diagram of a request path, which no chart
// fence can draw - and it has to be styled by the human's theme, which an <img>
// cannot be: an SVG inside an <img> is isolated from the page, so currentColor
// and var(--k-ink) resolve to nothing and the drawing is frozen in whichever
// palette the author happened to be looking at.
//
// Refusals are teaching errors rather than silent drops (vision principle 9): an
// author who is told "font-family is not allowed, size with font-size and let the
// theme choose the face" fixes the figure, while one whose attribute vanished
// ships a drawing that renders differently than it was written.

import (
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"
)

// Bounds on one figure's markup. The element and depth caps are about the
// surface: a drawing with four thousand nodes in it is a screenshot that forgot
// to be one, and the reader's browser lays out every node on every scroll.
const (
	MaxFigureSVG      = 96 << 10
	maxSVGElements    = 1200
	maxSVGDepth       = 24
	svgNamespace      = "http://www.w3.org/2000/svg"
	xlinkNamespace    = "http://www.w3.org/1999/xlink"
	maxSVGTextRunSize = 2000
)

// svgElements is what a diagram is made of: shapes, groups, text, and the
// definitions markers and gradients need. Everything absent is absent on
// purpose. <script> and <foreignObject> execute; <image> and <a> reach out of
// the page; <style> would let one figure restyle the board around it; <animate>
// and friends move a picture the reader did not ask to have move.
var svgElements = map[string]bool{
	"svg": true, "g": true, "defs": true, "title": true, "desc": true,
	"path": true, "rect": true, "circle": true, "ellipse": true,
	"line": true, "polyline": true, "polygon": true,
	"text": true, "tspan": true,
	"marker": true, "clipPath": true, "symbol": true, "use": true,
	"linearGradient": true, "radialGradient": true, "stop": true,
}

// svgAttrs is the attribute allow-list, applied to every element rather than per
// element: a geometry attribute on the wrong shape is ignored by the renderer,
// which is a drawing bug and not a safety one, and one list is a list a reader of
// this file can hold in their head.
var svgAttrs = map[string]bool{
	// structure and placement
	"id": true, "class": true, "transform": true, "viewBox": true,
	"preserveAspectRatio": true, "role": true, "aria-label": true, "aria-hidden": true,
	// geometry
	"x": true, "y": true, "x1": true, "y1": true, "x2": true, "y2": true,
	"cx": true, "cy": true, "r": true, "rx": true, "ry": true,
	"dx": true, "dy": true, "width": true, "height": true,
	"d": true, "points": true, "pathLength": true, "offset": true,
	// paint
	"fill": true, "fill-opacity": true, "fill-rule": true,
	"stroke": true, "stroke-width": true, "stroke-linecap": true,
	"stroke-linejoin": true, "stroke-dasharray": true, "stroke-dashoffset": true,
	"stroke-opacity": true, "stroke-miterlimit": true,
	"opacity": true, "color": true, "stop-color": true, "stop-opacity": true,
	"vector-effect": true, "shape-rendering": true,
	// text, without the face: the theme owns which font this is
	"text-anchor": true, "dominant-baseline": true, "alignment-baseline": true,
	"font-size": true, "font-weight": true, "font-style": true,
	"letter-spacing": true, "word-spacing": true, "textLength": true,
	// markers, clipping and gradients
	"marker-start": true, "marker-mid": true, "marker-end": true,
	"markerWidth": true, "markerHeight": true, "refX": true, "refY": true,
	"orient": true, "markerUnits": true, "clip-path": true, "clip-rule": true,
	"clipPathUnits": true, "gradientUnits": true, "gradientTransform": true,
	"spreadMethod": true, "fx": true, "fy": true,
}

// paintAttrs take a colour, which is where the theme rule bites: agent-authored
// markup consumes the --k-* tokens and never declares a colour of its own, so one
// figure cannot look right in light and wrong in dark. FR101 states the rule; this
// is where it is enforced rather than requested.
var paintAttrs = map[string]bool{
	"fill": true, "stroke": true, "stop-color": true, "color": true,
}

// refAttrs may only point inside this same figure.
var refAttrs = map[string]bool{
	"marker-start": true, "marker-mid": true, "marker-end": true, "clip-path": true,
}

var (
	reLocalRef  = regexp.MustCompile(`^url\(#[A-Za-z][A-Za-z0-9_.\-]{0,63}\)$`)
	reToken     = regexp.MustCompile(`^var\(\s*--k-[a-z0-9-]{1,32}\s*(?:,\s*[A-Za-z][A-Za-z0-9-]{0,31}\s*)?\)$`)
	reIdent     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.\-]{0,63}$`)
	reNumberish = regexp.MustCompile(`^[-+0-9eE.,%\s]+$`)
	// A path's d and a polygon's points are numbers, commands and separators.
	// Anything else in them is a sign the author is trying something this
	// allow-list does not cover, and it is refused rather than guessed at.
	rePathData  = regexp.MustCompile(`^[-+0-9eE.,\sMmZzLlHhVvCcSsQqTtAa]+$`)
	reTransform = regexp.MustCompile(`^[A-Za-z0-9\s(),.+\-eE]+$`)
)

// SafeSVG validates one figure's markup and returns the markup the board will
// inject. The output is composed here from names and values that passed the
// lists above, so it is Go's HTML in the sense the policy test means.
func SafeSVG(src string) (string, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return "", fmt.Errorf("svg is empty")
	}
	if len(src) > MaxFigureSVG {
		return "", fmt.Errorf("svg is %d bytes; the cap is %d - a drawing past that size is a screenshot, and a figure takes one of those in src", len(src), MaxFigureSVG)
	}
	dec := xml.NewDecoder(strings.NewReader(src))
	dec.Strict = true
	var out strings.Builder
	var stack []string
	elements, sawRoot := 0, false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("svg does not parse as XML: %w - a figure's svg must be well formed, with every tag closed", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			if !sawRoot {
				if name != "svg" {
					return "", fmt.Errorf("svg must start with an <svg> element, not <%s>", name)
				}
				sawRoot = true
			}
			if sp := t.Name.Space; sp != "" && sp != svgNamespace && sp != "svg" {
				return "", fmt.Errorf("<%s> is in namespace %q; a figure holds plain SVG only", name, sp)
			}
			if !svgElements[name] {
				return "", fmt.Errorf("<%s> is not allowed in a figure. Allowed: %s", name, strings.Join(sortedKeys(svgElements), ", "))
			}
			elements++
			if elements > maxSVGElements {
				return "", fmt.Errorf("svg holds more than %d elements - past that it is a picture of a drawing rather than a drawing, and src takes an image", maxSVGElements)
			}
			if len(stack) >= maxSVGDepth {
				return "", fmt.Errorf("svg nests more than %d deep", maxSVGDepth)
			}
			attrs, err := safeAttrs(name, t.Attr, len(stack) == 0)
			if err != nil {
				return "", err
			}
			out.WriteString("<" + name)
			out.WriteString(attrs)
			out.WriteString(">")
			stack = append(stack, name)
		case xml.EndElement:
			if len(stack) == 0 {
				return "", fmt.Errorf("svg closes </%s> that was never opened", t.Name.Local)
			}
			out.WriteString("</" + stack[len(stack)-1] + ">")
			stack = stack[:len(stack)-1]
		case xml.CharData:
			// Text only counts inside the elements that draw text; whitespace
			// between shapes is dropped rather than refused, because a formatted
			// drawing is how anybody writes one by hand.
			s := string(t)
			if strings.TrimSpace(s) == "" {
				continue
			}
			holder := ""
			if len(stack) > 0 {
				holder = stack[len(stack)-1]
			}
			switch holder {
			case "text", "tspan", "title", "desc":
				if len(s) > maxSVGTextRunSize {
					return "", fmt.Errorf("a text run inside the svg is %d characters; the cap is %d", len(s), maxSVGTextRunSize)
				}
				out.WriteString(html.EscapeString(s))
			default:
				return "", fmt.Errorf("the svg has text (%q) directly inside <%s>; put words in a <text> element", trim(s, 40), holder)
			}
		case xml.Comment, xml.ProcInst, xml.Directive:
			// A comment carries nothing the reader sees and a processing
			// instruction is a door; both are dropped.
		}
	}
	if len(stack) != 0 {
		return "", fmt.Errorf("svg leaves <%s> unclosed", stack[len(stack)-1])
	}
	if !sawRoot {
		return "", fmt.Errorf("svg holds no <svg> element")
	}
	return out.String(), nil
}

// safeAttrs vets one element's attributes and returns them re-composed. root
// carries the extra rule that only the outer <svg> has: it must say viewBox,
// which is what lets the board scale the drawing to the column instead of
// letting a fixed pixel size decide the layout.
func safeAttrs(el string, attrs []xml.Attr, root bool) (string, error) {
	var b strings.Builder
	sawViewBox := false
	for _, a := range attrs {
		name := a.Name.Local
		switch {
		case a.Name.Space == "xmlns" || name == "xmlns":
			continue // the board injects into HTML, where the namespace is implied
		case a.Name.Space == xlinkNamespace:
			return "", fmt.Errorf("<%s> uses xlink:%s; a figure never references anything outside itself", el, name)
		case a.Name.Space != "" && a.Name.Space != svgNamespace:
			return "", fmt.Errorf("<%s> has an attribute in namespace %q; a figure holds plain SVG only", el, a.Name.Space)
		}
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "on") {
			return "", fmt.Errorf("<%s> carries %s; a figure is a drawing and never handles events", el, name)
		}
		switch lower {
		case "style":
			return "", fmt.Errorf("<%s> carries a style attribute; set fill, stroke and font-size as attributes, and take colours from the --k-* tokens", el)
		case "font-family":
			return "", fmt.Errorf("<%s> sets font-family; the human's theme owns the face - size with font-size and leave the family alone", el)
		case "href", "src", "filter", "mask":
			return "", fmt.Errorf("<%s> carries %s, which is not allowed in a figure", el, name)
		}
		if name == "viewBox" {
			sawViewBox = true
		}
		if !svgAttrs[name] {
			return "", fmt.Errorf("<%s> carries %s, which is not on a figure's attribute list. Allowed: %s", el, name, strings.Join(sortedKeys(svgAttrs), ", "))
		}
		val := strings.TrimSpace(a.Value)
		if err := checkValue(el, name, val); err != nil {
			return "", err
		}
		b.WriteString(" " + name + `="` + html.EscapeString(val) + `"`)
	}
	if root && !sawViewBox {
		return "", fmt.Errorf("the outer <svg> needs a viewBox so the board can scale the drawing to the reader's column; width and height alone freeze it at one size")
	}
	if el == "use" {
		// <use> without a reference draws nothing, and the reference it would
		// need is href, which is refused above. Say so rather than emitting a
		// silent no-op.
		return "", fmt.Errorf("<use> needs href to mean anything, and a figure does not allow href - repeat the shape, or draw it in a <g>")
	}
	return b.String(), nil
}

// checkValue is where an attribute's meaning is enforced: a colour has to be a
// token, a reference has to be local, and everything else has to look like what
// its name says it is.
func checkValue(el, name, val string) error {
	if val == "" {
		return nil
	}
	if len(val) > 4000 {
		return fmt.Errorf("<%s> %s is %d characters; the cap is 4000", el, name, len(val))
	}
	if i := strings.IndexAny(val, "<>"); i >= 0 {
		return fmt.Errorf("<%s> %s contains %q", el, name, val[i:i+1])
	}
	low := strings.ToLower(val)
	if strings.Contains(low, "javascript:") || strings.Contains(low, "&#") {
		return fmt.Errorf("<%s> %s looks like an escape rather than a value: %q", el, name, trim(val, 40))
	}
	switch {
	case paintAttrs[name]:
		switch {
		case val == "none" || val == "transparent" || val == "currentColor":
			return nil
		case reToken.MatchString(val):
			return nil
		case reLocalRef.MatchString(val):
			return nil // a gradient defined in this same figure
		}
		return fmt.Errorf("<%s> %s is %q; a figure takes its colours from the human's theme, so %s must be none, currentColor, url(#id) for a gradient in this figure, or a token like var(--k-accent). The tokens are --k-ink, --k-ink-2, --k-ink-3, --k-accent, --k-edge, --k-edge-soft, --k-ground, --k-surface, --k-surface-2, --k-surface-3, --k-success, --k-warning, --k-error, --k-info and --k-urgent", el, name, val, name)
	case refAttrs[name]:
		if !reLocalRef.MatchString(val) {
			return fmt.Errorf("<%s> %s is %q; it must be url(#id) naming something defined in this same figure", el, name, val)
		}
		return nil
	case name == "d":
		if !rePathData.MatchString(val) {
			return fmt.Errorf("<%s> d holds something that is not path data: %q", el, trim(val, 60))
		}
		return nil
	case name == "points":
		if !reNumberish.MatchString(val) {
			return fmt.Errorf("<%s> points must be numbers and separators, got %q", el, trim(val, 60))
		}
		return nil
	case name == "transform" || name == "gradientTransform":
		if !reTransform.MatchString(val) {
			return fmt.Errorf("<%s> %s holds something that is not a transform list: %q", el, name, trim(val, 60))
		}
		return nil
	case name == "id" || name == "class" || name == "aria-label" || name == "role":
		if name == "aria-label" {
			return nil // words, and they are escaped on the way out
		}
		for _, part := range strings.Fields(val) {
			if !reIdent.MatchString(part) {
				return fmt.Errorf("<%s> %s is %q; use plain identifiers", el, name, trim(val, 40))
			}
		}
		return nil
	}
	// Everything left is a number, a length, a keyword or a list of them.
	if reNumberish.MatchString(val) {
		return nil
	}
	for _, part := range strings.Fields(strings.ReplaceAll(val, ",", " ")) {
		if !reIdent.MatchString(part) && !reNumberish.MatchString(part) {
			return fmt.Errorf("<%s> %s is %q, which is neither a number nor a keyword", el, name, trim(val, 40))
		}
	}
	return nil
}

func trim(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// A stable order, because the list is in an error message a model reads.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
