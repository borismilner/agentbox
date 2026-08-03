// Command genicon renders the agentbox tray and application icons from the brand
// robot (docs/img/logo.png). Run via go:generate from internal/tray; the PNGs
// are committed, so a machine without this tool still builds.
//
// The tray slot is 24 logical pixels tall. Nothing in the full logo survives
// that - the speech bubble, the question mark and the robot's own body all turn
// to mush - so the tray icon is the HEAD alone, which is the part that still
// reads as a face when it is small: a rounded screen, two eyes, a smile.
// Measured by rendering the candidates and looking at them at 1:1, not by
// cropping to the bounding box and hoping.
//
// State rides a badge dot rather than a tint. A tint over a mint-green character
// reads as a rendering fault, and a desaturated one sitting in a row of colourful
// panel icons reads as disabled - so the robot is itself in all three, and the
// corner says whether anything is waiting.
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
	// height of every neighbouring icon, and soft. At 128 the host has something
	// to scale down from at any factor, and the robot fills its slot.
	tray = 128
	app  = 256 // hicolor application icon
)

// head is the robot's head in logo.png's coordinates. It is SQUARE and it cuts
// the two ear knobs, and both of those are the point.
//
// A tray slot is bounded by its height. The whole head is 204x152, so squaring it
// pads a quarter of the icon with transparency and the robot is drawn at 75% of
// the height every neighbouring icon uses - which is exactly what "the robot icon
// seems small" was. Cropping to the face instead spends the entire slot on the
// part that has to be legible, and the ear knobs are the first thing that stops
// being readable anyway.
//
// The antenna is excluded for the same reason: at panel size it is one stray
// pixel above the silhouette and reads as dirt.
var head = image.Rect(162, 170, 314, 322)

// badgeR is the status dot's radius as a fraction of the icon. It needs no
// margin to sit in: the head is masked to an ellipse inside a square, so the
// corners are already empty and the badge lands beside the face rather than on
// it. The robot itself runs edge to edge, because a tray icon that holds a
// margin its neighbours do not reads as the small one in the row.
const badgeR = 0.2

type variant struct {
	name  string
	badge color.NRGBA // zero alpha = no badge
}

// The robot is in full colour in all three: it is the brand, and a panel full of
// colourful icons reads a desaturated one as disabled rather than as quiet. The
// badge is the whole state channel - absent, blue, amber.
var variants = []variant{
	{name: "idle"},
	{name: "attn", badge: color.NRGBA{R: 0x46, G: 0x95, B: 0xEB, A: 0xFF}},
	{name: "urgent", badge: color.NRGBA{R: 0xE5, G: 0xA5, B: 0x0A, A: 0xFF}},
}

func main() {
	dir := "icons"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	src := filepath.Join("..", "..", "docs", "img", "logo.png")
	if len(os.Args) > 2 {
		src = os.Args[2]
	}
	logo, err := load(src)
	if err != nil {
		fail(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(err)
	}

	face := square(ellipse(sub(logo, head)))
	for _, v := range variants {
		img := resize(face, tray)
		if v.badge.A != 0 {
			dot(img, v.badge)
		}
		if err := write(filepath.Join(dir, v.name+".png"), img); err != nil {
			fail(err)
		}
	}

	// The application icon is the head as well. The whole logo was tried here -
	// robot, question mark, bubble - and in a switcher or dock the figure shrinks
	// to a sliver of the tile while the rest reads as clutter. The face fills the
	// square and stays legible, which is what a launcher icon is for.
	if err := write(filepath.Join(dir, "app-256.png"), resize(face, app)); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "genicon:", err)
	os.Exit(1)
}

func load(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := range b.Dy() {
		for x := range b.Dx() {
			r, g, bl, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if a == 0 {
				continue
			}
			// Back out the premultiplication color.RGBA() applies, or every edge
			// pixel darkens toward black as it fades.
			out.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r * 0xFF / a), G: uint8(g * 0xFF / a), B: uint8(bl * 0xFF / a),
				A: uint8(a >> 8),
			})
		}
	}
	return out, nil
}

func sub(src *image.NRGBA, r image.Rectangle) *image.NRGBA {
	r = r.Intersect(src.Bounds())
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := range r.Dy() {
		for x := range r.Dx() {
			out.SetNRGBA(x, y, src.NRGBAAt(r.Min.X+x, r.Min.Y+y))
		}
	}
	return out
}

// ellipse clears everything outside the largest ellipse the image holds. It is
// how the head is separated from the speech bubble behind it without a
// hand-drawn mask: a head is round, its corners are somebody else's picture.
// The edge is antialiased over one pixel, because a hard cut on a 200px source
// survives the downscale as a visible chord across the helmet.
func ellipse(src *image.NRGBA) *image.NRGBA {
	b := src.Bounds()
	rx, ry := float64(b.Dx())/2, float64(b.Dy())/2
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := range b.Dy() {
		for x := range b.Dx() {
			dx := (float64(x) + 0.5 - rx) / rx
			dy := (float64(y) + 0.5 - ry) / ry
			d := math.Hypot(dx, dy)
			if d >= 1 {
				continue
			}
			c := src.NRGBAAt(b.Min.X+x, b.Min.Y+y)
			// One source pixel of feather, expressed in the normalised radius.
			if soft := (1 - d) * math.Min(rx, ry); soft < 1 {
				c.A = uint8(float64(c.A) * soft)
			}
			out.SetNRGBA(x, y, c)
		}
	}
	return out
}

// square centres the image in a transparent square, so the resize below never
// changes its aspect - a squashed robot is worse than a small one.
func square(src *image.NRGBA) *image.NRGBA {
	b := src.Bounds()
	s := max(b.Dx(), b.Dy())
	out := image.NewNRGBA(image.Rect(0, 0, s, s))
	ox, oy := (s-b.Dx())/2, (s-b.Dy())/2
	for y := range b.Dy() {
		for x := range b.Dx() {
			out.SetNRGBA(ox+x, oy+y, src.NRGBAAt(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}

// resize is an area average - each destination pixel is the mean of the source
// box under it. For a downscale this large (194px of head into 24) it beats
// bilinear outright: bilinear samples four pixels and throws the other sixty
// away, which is how a detailed source turns into aliased noise.
//
// The colour mean is ALPHA-WEIGHTED and the alpha mean is not. Averaging colour
// without the weight lets fully transparent pixels - whose RGB is arbitrary -
// vote on the colour of the edge, which is where a dark halo around a soft edge
// comes from.
func resize(src *image.NRGBA, size int) *image.NRGBA {
	b := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	sx, sy := float64(b.Dx())/float64(size), float64(b.Dy())/float64(size)
	for py := range size {
		y0, y1 := int(float64(py)*sy), int(math.Ceil(float64(py+1)*sy))
		for px := range size {
			x0, x1 := int(float64(px)*sx), int(math.Ceil(float64(px+1)*sx))
			var r, g, bl, a, wsum, n float64
			for y := y0; y < y1 && y < b.Dy(); y++ {
				for x := x0; x < x1 && x < b.Dx(); x++ {
					c := src.NRGBAAt(b.Min.X+x, b.Min.Y+y)
					w := float64(c.A) / 255
					r += float64(c.R) * w
					g += float64(c.G) * w
					bl += float64(c.B) * w
					a += float64(c.A)
					wsum += w
					n++
				}
			}
			if n == 0 || wsum == 0 {
				continue
			}
			out.SetNRGBA(px, py, color.NRGBA{
				R: uint8(r / wsum), G: uint8(g / wsum), B: uint8(bl / wsum),
				A: uint8(a / n),
			})
		}
	}
	return out
}

// dot puts a status badge in the bottom-right corner: the count is in the
// tooltip, so all this has to carry is "there is something". It is drawn OVER
// the robot with a transparent gap around it, which is what keeps it readable
// against the head's own outline at this size.
func dot(img *image.NRGBA, col color.NRGBA) {
	b := img.Bounds()
	r := float64(b.Dx()) * badgeR
	cx, cy := float64(b.Dx())-r-0.5, float64(b.Dy())-r-0.5
	for y := range b.Dy() {
		for x := range b.Dx() {
			d := math.Hypot(float64(x)-cx, float64(y)-cy)
			switch {
			case d <= r:
				img.SetNRGBA(x, y, col)
			case d <= r+1.4:
				img.SetNRGBA(x, y, color.NRGBA{}) // the gap
			}
		}
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
