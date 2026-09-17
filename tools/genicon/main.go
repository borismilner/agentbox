// Command genicon draws the agentbox tray and application icons. Run via
// go:generate from internal/tray; the PNGs are committed, so a machine without
// this tool still builds.
//
// # Why this is a wordmark and not the brand robot
//
// It used to crop the head out of docs/img/logo.png. rig ships the same robot,
// and the two icons sat next to each other in one panel strip at 22 logical
// pixels: identical silhouette, identical pale mint palette, indistinguishable
// without stopping to compare them. The tray icon is the only readout for
// "is it running", so two marks that need comparing is the same as no mark at
// all. Boris asked for "AB" on 2026-09-17.
//
// The test that decides every number below is a GLANCE at 22px in a row of
// other icons, not a diff at 256. Everything here is drawn from geometry rather
// than set in a typeface for that reason: at this size the counters are two or
// three pixels across, and a font's idea of a bold B puts them where the
// downscale closes them. Drawing the letters means the stroke, the tracking and
// the counters are chosen against the pixel grid. It also keeps this tool on
// the standard library, so it runs anywhere the repo does.
//
// # Why it reads as a sibling of rig rather than an accident
//
// Deep indigo against rig's pale mint, and a hard-edged filled tile against
// rig's round face. VALUE does more work than hue at 22px, so the two marks
// differ first in light-versus-dark and only then in colour: a pale green blob
// and a dark block with two white letters cannot be swapped by mistake.
//
// # Why the state badge is a bar and not the old corner dot
//
// The dot worked because a round face leaves its corners empty. A wordmark has
// no empty corner - the dot lands on the B and reads as a smudge. A bar along
// the bottom edge is 3 physical pixels at 22px, never touches a letter, and is
// easier to see than a dot at the same size.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

const (
	// tray is rendered far above the panel's logical 24px on purpose. A
	// StatusNotifierItem hands the host a bitmap and the host scales it, so a
	// 24px source on a HiDPI panel is drawn at 24 PHYSICAL pixels - half the
	// height of every neighbouring icon, and soft. At 128 the host has
	// something to scale down from at any factor.
	tray = 128
	app  = 256 // hicolor application icon

	// samples is the supersampling grid per output pixel. 8x8 is 64 coverage
	// samples, which is enough that a diagonal at 128px shows no stepping and
	// the 22px downscale has clean edges to work from.
	samples = 8
)

// The palette. Ratios are WCAG contrast, measured in tools/genicon by the
// numbers below rather than judged by eye:
//
//	glyph on tile          10.8:1   the letters, wherever the icon sits
//	tile on white panel    10.8:1   the block carries a light tray
//	glyph on #2D2D2D       13.8:1   the letters carry a dark tray, where the
//	                                indigo block has nothing to separate from
//
// That pairing is deliberate. No single flat colour can clear 4.5:1 against
// both a white and a near-black panel - the two requirements are arithmetically
// exclusive - so the icon carries two tones and lets whichever one has contrast
// do the work.
var (
	tileCol   = color.NRGBA{0x3A, 0x2E, 0x8C, 0xFF} // deep indigo
	glyphCol  = color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF} // the letters
	attnCol   = color.NRGBA{0x28, 0xD3, 0xE8, 0xFF} // cyan bar: something pending
	urgentCol = color.NRGBA{0xF5, 0xA5, 0x24, 0xFF} // amber bar: urgent
)

// Layout, in fractions of the icon's side. Changing one of these changes the
// 22px rendering, so re-run cmd/render at the small sizes before believing it.
const (
	corner = 0.215 // tile corner radius; a box, not a circle and not a square

	capTop = 0.235 // cap height top and bottom. The band is placed slightly
	capBot = 0.755 // above centre because optical centre is above geometric.

	stem  = 0.108 // stroke weight: 2.4 physical px at 22, which is the floor
	aWide = 0.440 // the A is wider than the B, as a triangle must be to balance
	aFlat = 0.165 // width of the A's cut apex - see letterA
	bWide = 0.315
	track = 0.028 // tighter than a text setting; the A's slanted right side
	// already opens a gap at the top that reads as space.

	barTop = 0.865 // state bar: 3 physical px at 22, 0.11 clear of the letters
)

type shape func(x, y float64) bool

func main() {
	dir := "icons"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(err)
	}

	for _, v := range []struct {
		name string
		bar  *color.NRGBA
	}{
		{"idle", nil},
		{"attn", &attnCol},
		{"urgent", &urgentCol},
	} {
		if err := write(filepath.Join(dir, v.name+".png"), render(tray, v.bar)); err != nil {
			fail(err)
		}
	}

	// The application icon is the same wordmark with no state on it: a launcher
	// or a switcher is not where "two items pending" belongs, and the .desktop
	// icon is installed by the Makefile rather than embedded, so it would go
	// stale the moment the count changed anyway.
	if err := write(filepath.Join(dir, "app-256.png"), render(app, nil)); err != nil {
		fail(err)
	}
}

// render draws one icon at the given side length. bar is nil for the plain
// mark, or the colour of the state bar along the bottom edge.
func render(size int, bar *color.NRGBA) *image.NRGBA {
	tile := roundedSquare(corner)
	glyph := wordmark()
	out := image.NewNRGBA(image.Rect(0, 0, size, size))

	step := 1 / (float64(size) * samples)
	for py := range size {
		for px := range size {
			var r, g, b, a float64
			for sy := range samples {
				for sx := range samples {
					x := (float64(px*samples+sx) + 0.5) * step
					y := (float64(py*samples+sy) + 0.5) * step
					if !tile(x, y) {
						continue
					}
					c := tileCol
					switch {
					case glyph(x, y):
						c = glyphCol
					case bar != nil && y >= barTop:
						c = *bar
					}
					r += float64(c.R)
					g += float64(c.G)
					b += float64(c.B)
					a++
				}
			}
			if a == 0 {
				continue
			}
			// The colour mean is over the COVERED samples only and the alpha is
			// over all of them. Letting uncovered samples vote on colour is how
			// a soft edge picks up a dark halo.
			n := float64(samples * samples)
			out.SetNRGBA(px, py, color.NRGBA{
				R: uint8(math.Round(r / a)),
				G: uint8(math.Round(g / a)),
				B: uint8(math.Round(b / a)),
				A: uint8(math.Round(a / n * 255)),
			})
		}
	}
	return out
}

// roundedSquare is the tile: full bleed, because an icon that holds a margin
// its neighbours do not reads as the small one in the row.
func roundedSquare(r float64) shape {
	return func(x, y float64) bool {
		dx := math.Abs(x-0.5) - (0.5 - r)
		dy := math.Abs(y-0.5) - (0.5 - r)
		if x < 0 || x > 1 || y < 0 || y > 1 {
			return false
		}
		if dx <= 0 || dy <= 0 {
			return true
		}
		return math.Hypot(dx, dy) <= r
	}
}

// wordmark composes the A and the B, centred on the icon by their combined
// bounding box.
func wordmark() shape {
	left := (1 - (aWide + track + bWide)) / 2
	a := letterA(left, left+aWide)
	b := letterB(left+aWide+track, left+aWide+track+bWide)
	return func(x, y float64) bool { return a(x, y) || b(x, y) }
}

// letterA is a trapezoid with a smaller trapezoid cut out of it and a crossbar
// put back.
//
// Two departures from a text face, and both are the 22px test rather than
// taste:
//
// The APEX IS CUT FLAT. A pointed A at this stroke weight closes its own
// counter long before 22px - measured at 0.8 physical pixels, which is dirt
// rather than a counter - and the point itself antialiases to a pale smudge.
// Cutting it at aFlat reopens the counter to 3.6px and, at this width, makes
// the apex stroke the same weight as the legs instead of half again as heavy.
//
// The CROSSBAR SITS LOW, at 60% of the cap height. That is what leaves the gap
// between the feet wide enough to survive the downscale; any lower and the two
// feet merge into a solid base.
func letterA(x0, x1 float64) shape {
	w := x1 - x0
	h := capBot - capTop
	mid := (x0 + x1) / 2

	// Horizontal width of a leg, so that its PERPENDICULAR width is stem. A leg
	// runs (w-aFlat)/2 across while it runs h down, so the correction is the
	// cosecant of its angle to the horizontal.
	legW := stem * math.Hypot((w-aFlat)/2/h, 1)

	const barLo, barHi = 0.60, 0.735 // crossbar, as a fraction of cap height

	// edges returns the outer left and right of the letter at height y.
	edges := func(y float64) (float64, float64) {
		t := (y - capTop) / h
		half := (aFlat + (w-aFlat)*t) / 2
		return mid - half, mid + half
	}

	return func(x, y float64) bool {
		if y < capTop || y > capBot {
			return false
		}
		l, r := edges(y)
		if x < l || x > r {
			return false
		}
		if x > l+legW && x < r-legW {
			return y >= capTop+barLo*h && y <= capTop+barHi*h
		}
		return true
	}
}

// letterB is a stem plus two bowls, each bowl an outer stadium with a smaller
// stadium cut out of it. The upper bowl is the smaller of the two, which is
// what stops a B reading as an 8.
func letterB(x0, x1 float64) shape {
	h := capBot - capTop
	const (
		hs   = 0.090 // horizontal stroke: the top and bottom of the bowls
		vs   = 0.095 // vertical stroke: the far side of each bowl
		hbar = 0.090 // the waist
	)
	// What is left after the three horizontals is the two counters.
	// The upper gets 46% of it; the lower takes whatever is left, which the
	// geometry below computes rather than restates.
	upH := (h - 2*hs - hbar) * 0.46

	waistTop := capTop + hs + upH
	waistBot := waistTop + hbar

	// Counter radius is held well under half the counter height on purpose. A
	// fully rounded counter is a dot once it is 2.5px across, and a dot carries
	// less of the letter than a slot does.
	const cr = 0.030

	upper := bowl(x0, x1, capTop, waistBot, hs, vs, cr)
	lower := bowl(x0, x1, waistTop, capBot, hs, vs, cr)

	return func(x, y float64) bool {
		if x >= x0 && x <= x0+stem && y >= capTop && y <= capBot {
			return true
		}
		return upper(x, y) || lower(x, y)
	}
}

// bowl is one lobe of the B: a stadium-ended rectangle with a smaller one
// removed. The left edge of both is square, because the stem is already there.
func bowl(x0, x1, y0, y1, hs, vs, cr float64) shape {
	outR := math.Min((y1-y0)/2, (x1-x0)*0.6)
	in := stadium(x0+stem, x1-vs, y0+hs, y1-hs, cr)
	out := stadium(x0, x1, y0, y1, outR)
	return func(x, y float64) bool { return out(x, y) && !in(x, y) }
}

// stadium is a rectangle with its RIGHT corners rounded by r.
func stadium(x0, x1, y0, y1, r float64) shape {
	r = math.Min(r, math.Min((x1-x0), (y1-y0)/2))
	return func(x, y float64) bool {
		if x < x0 || x > x1 || y < y0 || y > y1 {
			return false
		}
		if x <= x1-r {
			return true
		}
		cy := math.Min(math.Max(y, y0+r), y1-r)
		return math.Hypot(x-(x1-r), y-cy) <= r
	}
}

func write(path string, img *image.NRGBA) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	fmt.Println("wrote", path)
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "genicon:", err)
	os.Exit(1)
}
