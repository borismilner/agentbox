package hand

import (
	"math"
	"math/rand"
	"strings"
	"time"
)

// This file is the half of the package with no X11 in it: given where the
// pointer is and where it should end up, produce the positions to send and when
// to send them. Keeping it pure is what makes the interesting part testable -
// the shape of a movement is a property of the numbers, not of the display.

// Pt is a pointer position in root (screen) coordinates.
type Pt struct{ X, Y int }

// Point is one position on the way somewhere: send it After waiting.
type Point struct {
	X, Y  int
	After time.Duration
}

// Motion is how a movement should feel. The zero value is a person's hand at an
// ordinary pace; Speed scales it (2 = twice as fast) and Rand adds the variation
// that stops a path from looking like a ruler. A nil Rand plans the same path
// every time, which is what the tests want and what a demo can rely on.
type Motion struct {
	Speed  float64 // 1 = human pace; higher is faster
	Rate   int     // positions per second (0 = 144)
	Jitter float64 // pixels of wobble on the way (0 = 0.7)
	Rand   *rand.Rand
}

const (
	defaultRate   = 144
	defaultJitter = 0.7
	minDuration   = 40 * time.Millisecond
)

func (m Motion) speed() float64 {
	if m.Speed <= 0 {
		return 1
	}
	return m.Speed
}

func (m Motion) rate() int {
	if m.Rate <= 0 {
		return defaultRate
	}
	return m.Rate
}

func (m Motion) jitter() float64 {
	if m.Jitter < 0 {
		return 0
	}
	if m.Jitter == 0 {
		return defaultJitter
	}
	return m.Jitter
}

// float in [lo, hi), from the motion's own source or the midpoint when it has none.
func (m Motion) between(lo, hi float64) float64 {
	if m.Rand == nil {
		return (lo + hi) / 2
	}
	return lo + m.Rand.Float64()*(hi-lo)
}

// MoveDuration is how long a hand takes to cross dist pixels. It is Fitts's law
// in the shape everyone actually uses: a fixed cost to start moving plus a
// logarithm, so a short hop is not instant and crossing two monitors does not
// take four seconds.
func MoveDuration(dist float64) time.Duration {
	if dist <= 0 {
		return minDuration
	}
	s := 0.11 + 0.20*math.Log2(1+dist/55)
	return time.Duration(s * float64(time.Second))
}

// PlanMove returns the positions that carry the pointer from from to to. The
// path is a shallow arc rather than a straight line and the pacing is the
// minimum-jerk curve a reaching arm follows (slow out, fast in the middle, slow
// into the target), because those two things together are the whole difference
// between "the mouse moved" and "somebody moved the mouse".
//
// The last position is exactly to, always: a demo that lands a pixel off because
// of jitter clicks the wrong thing.
func PlanMove(from, to Pt, m Motion) []Point {
	dx, dy := float64(to.X-from.X), float64(to.Y-from.Y)
	dist := math.Hypot(dx, dy)
	if dist < 1 {
		return []Point{{X: to.X, Y: to.Y, After: 0}}
	}

	dur := max(time.Duration(float64(MoveDuration(dist))/m.speed()), minDuration)
	n := max(int(dur.Seconds()*float64(m.rate())), 2)

	// The arc: one control point, pushed off the straight line at right angles.
	// The offset grows with distance but stops growing early, so a long sweep
	// curves like an arm and does not sail around the screen.
	bow := math.Min(dist*0.07, 90) * m.between(0.5, 1.0)
	if m.Rand != nil && m.Rand.Intn(2) == 0 {
		bow = -bow
	}
	nx, ny := -dy/dist, dx/dist // unit normal
	cx := float64(from.X) + dx/2 + nx*bow
	cy := float64(from.Y) + dy/2 + ny*bow

	step := dur / time.Duration(n)
	jit := m.jitter()
	out := make([]Point, 0, n)
	for i := 1; i <= n; i++ {
		s := minJerk(float64(i) / float64(n))
		x, y := quadBezier(float64(from.X), float64(from.Y), cx, cy, float64(to.X), float64(to.Y), s)
		if i < n && jit > 0 {
			x += m.between(-jit, jit)
			y += m.between(-jit, jit)
		}
		p := Point{X: int(math.Round(x)), Y: int(math.Round(y)), After: step}
		if i == n {
			p.X, p.Y = to.X, to.Y
		}
		out = append(out, p)
	}
	return out
}

// minJerk is the position profile of a hand reaching for something: zero
// velocity and zero acceleration at both ends.
func minJerk(t float64) float64 {
	t = clamp01(t)
	return t * t * t * (10 - 15*t + 6*t*t)
}

func quadBezier(x0, y0, cx, cy, x1, y1, t float64) (float64, float64) {
	u := 1 - t
	a, b, c := u*u, 2*u*t, t*t
	return a*x0 + b*cx + c*x1, a*y0 + b*cy + c*y1
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// Candidate is one window that could be the one a caller named. Win is the X id
// as a plain number, so this file stays free of X11 and testable without a
// display; it is what the caller locks onto once the choice is made.
type Candidate struct {
	Name  string
	Rect  Rect
	Order int // position in the window tree; later is nearer the top of the stack
	Win   uint32
}

// Choose picks the window a caller meant out of everything on screen. Every rule
// here is one that cost a wrong click:
//
//   - A leading "=" means the title must match exactly. Without it a substring
//     matches, which is convenient right up until a terminal whose title happens
//     to contain "agentbox" wins over the card.
//   - An exact match always beats a partial one, so "agentbox" finds the card and
//     not "agentbox - Release notes".
//   - agentbox keeps a 1x1 helper window called exactly "agentbox"; anything that small
//     is never what anybody meant, and the caller filters it out before here.
//   - Among equals, the biggest wins and then the topmost, which is the card in
//     front rather than the window behind it.
func Choose(cands []Candidate, want string) (Candidate, bool) {
	exactOnly := strings.HasPrefix(want, "=")
	want = strings.TrimSpace(strings.TrimPrefix(want, "="))
	if want == "" {
		return Candidate{}, false
	}
	lower := strings.ToLower(want)

	var best Candidate
	found := false
	bestExact := false
	for _, c := range cands {
		exact := strings.EqualFold(c.Name, want)
		if !exact {
			if exactOnly || !strings.Contains(strings.ToLower(c.Name), lower) {
				continue
			}
		}
		if !found || betterMatch(exact, c, bestExact, best) {
			best, bestExact, found = c, exact, true
		}
	}
	return best, found
}

func betterMatch(exact bool, c Candidate, bestExact bool, best Candidate) bool {
	if exact != bestExact {
		return exact
	}
	if a, b := c.Rect.W*c.Rect.H, best.Rect.W*best.Rect.H; a != b {
		return a > b
	}
	return c.Order > best.Order
}

// Keysyms agentbox needs by name. Latin-1 characters are their own keysym, which is
// why this list is only the keys that are not characters.
const (
	KeyReturn    = 0xff0d
	KeyTab       = 0xff09
	KeyEscape    = 0xff1b
	KeyBackSpace = 0xff08
)

// KeysymFor is the keysym a rune is typed with. Latin-1 maps to itself; anything
// above it takes the Unicode convention X has used for twenty years; the two
// whitespace characters that are keys rather than characters map to those keys.
func KeysymFor(r rune) uint32 {
	switch r {
	case '\n', '\r':
		return KeyReturn
	case '\t':
		return KeyTab
	}
	if r < 0x100 {
		return uint32(r)
	}
	return 0x01000000 + uint32(r)
}

// Layout is the keyboard as it is arranged right now: which keycode produces a
// keysym, and whether Shift has to be held to get it. It is read from the X
// server once, so typing follows the layout the human is actually using instead
// of assuming US QWERTY.
type Layout struct {
	byKeysym map[uint32]binding
}

type binding struct {
	Code  byte
	Shift bool
}

// NewLayout builds the table from an X keyboard mapping: keysyms is the flat
// list GetKeyboardMapping returns, per is how many entries each keycode has, and
// min is the first keycode it describes.
//
// The one subtlety is X's own shorthand: a keycode whose pair is (a, NoSymbol)
// means (a, A) - the upper case is implied. A table that took that literally
// would find no way to type a capital letter.
func NewLayout(min byte, per int, keysyms []uint32) *Layout {
	l := &Layout{byKeysym: make(map[uint32]binding, len(keysyms))}
	if per <= 0 {
		return l
	}
	for i := 0; i*per < len(keysyms); i++ {
		code := int(min) + i
		if code > 255 {
			break
		}
		plain := keysyms[i*per]
		var shifted uint32
		if per > 1 {
			shifted = keysyms[i*per+1]
		}
		if shifted == 0 && plain >= 'a' && plain <= 'z' {
			shifted = plain - 0x20
		}
		l.add(plain, binding{Code: byte(code)})
		l.add(shifted, binding{Code: byte(code), Shift: true})
	}
	return l
}

// add keeps the first keycode that can produce a keysym. Earlier keycodes are
// the main block; later ones are the numpad and second layout groups, which
// would type the same character in a way the human did not press.
func (l *Layout) add(ks uint32, b binding) {
	if ks == 0 {
		return
	}
	if _, seen := l.byKeysym[ks]; seen {
		return
	}
	l.byKeysym[ks] = b
}

// Keysym reports how to press one keysym.
func (l *Layout) Keysym(ks uint32) (code byte, shift bool, ok bool) {
	b, ok := l.byKeysym[ks]
	return b.Code, b.Shift, ok
}

// Rune reports how to type one character.
func (l *Layout) Rune(r rune) (code byte, shift bool, ok bool) {
	return l.Keysym(KeysymFor(r))
}

// Stroke is one key, pressed and released, After a pause.
type Stroke struct {
	Code  byte
	Shift bool
	After time.Duration
	Rune  rune // what it types, for diagnostics
}

// Typing is how fast and how evenly text is typed. WPM counts five-character
// words, the way typing tests do.
type Typing struct {
	WPM  int
	Rand *rand.Rand
}

const defaultWPM = 300

// PlanText turns text into strokes, and returns the characters this layout
// cannot type so the caller can say so rather than silently dropping them.
//
// The pauses are not uniform: a space costs a little more than a letter and a
// sentence ending costs more again, because that is where a person's hands
// actually hesitate. Text typed at a perfectly even interval reads as a machine
// even when everything else about it is right.
func PlanText(text string, l *Layout, t Typing) (strokes []Stroke, skipped []rune) {
	wpm := t.WPM
	if wpm <= 0 {
		wpm = defaultWPM
	}
	base := time.Duration(60_000/float64(wpm*5)) * time.Millisecond
	for _, r := range text {
		code, shift, ok := l.Rune(r)
		if !ok {
			skipped = append(skipped, r)
			continue
		}
		f := 1.0
		switch r {
		case ' ':
			f = 1.25
		case '.', ',', ';', ':', '!', '?':
			f = 1.6
		case '\n', '\r':
			f = 2.0
		}
		if t.Rand != nil {
			f *= 0.65 + t.Rand.Float64()*0.7
		}
		strokes = append(strokes, Stroke{
			Code: code, Shift: shift, Rune: r,
			After: time.Duration(float64(base) * f),
		})
	}
	return strokes, skipped
}
