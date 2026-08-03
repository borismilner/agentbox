package hand

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

// A fake US-QWERTY-ish mapping: two keysyms per keycode, and the alphabetic keys
// spelled X's way (lower case with NoSymbol beside it) so the implied-upper-case
// rule is exercised rather than assumed.
func testLayout() *Layout {
	syms := []uint32{
		'a', 0, // keycode 10
		'b', 0, // 11
		'1', '!', // 12
		' ', 0, // 13
		'.', '>', // 14
		KeyReturn, 0, // 15
		0xffe1, 0, // 16 Shift_L
		'a', 0, // 17 a numpad twin: same keysym, later keycode
	}
	return NewLayout(10, 2, syms)
}

func TestPlanMoveEndsExactlyOnTarget(t *testing.T) {
	for _, tc := range []struct{ from, to Pt }{
		{Pt{0, 0}, Pt{1, 1}},
		{Pt{100, 100}, Pt{103, 97}},
		{Pt{1920, 1080}, Pt{40, 30}},
		{Pt{500, 500}, Pt{500, 900}},
	} {
		pts := PlanMove(tc.from, tc.to, Motion{})
		if len(pts) == 0 {
			t.Fatalf("PlanMove(%v -> %v) planned nothing", tc.from, tc.to)
		}
		last := pts[len(pts)-1]
		if last.X != tc.to.X || last.Y != tc.to.Y {
			t.Errorf("PlanMove(%v -> %v) ended at %d,%d, not on the target",
				tc.from, tc.to, last.X, last.Y)
		}
	}
}

func TestPlanMoveStandingStillIsOnePoint(t *testing.T) {
	pts := PlanMove(Pt{300, 300}, Pt{300, 300}, Motion{})
	if len(pts) != 1 || pts[0].After != 0 {
		t.Fatalf("moving nowhere planned %d points (%v); want one, immediately", len(pts), pts)
	}
}

// The pacing is the point: a hand accelerates, crosses the middle fast and
// arrives slowly. Equal steps would be a machine dragging the cursor.
func TestPlanMoveAcceleratesAndArrivesSlowly(t *testing.T) {
	pts := PlanMove(Pt{0, 0}, Pt{1200, 0}, Motion{Jitter: -1})
	if len(pts) < 20 {
		t.Fatalf("only %d points for a 1200px move", len(pts))
	}
	steps := make([]float64, 0, len(pts))
	prev := Pt{0, 0}
	for _, p := range pts {
		steps = append(steps, math.Hypot(float64(p.X-prev.X), float64(p.Y-prev.Y)))
		prev = Pt{p.X, p.Y}
	}
	first, last := steps[0], steps[len(steps)-1]
	mid := steps[len(steps)/2]
	if !(first < mid/2) {
		t.Errorf("first step %.2f is not much smaller than the middle step %.2f", first, mid)
	}
	if !(last < mid/2) {
		t.Errorf("last step %.2f is not much smaller than the middle step %.2f", last, mid)
	}
}

// It is an arc, not a ruler line, but a shallow one: a movement that bows a
// third of the way across the screen looks like a fly, not a hand.
func TestPlanMoveBowsGentlyOffTheStraightLine(t *testing.T) {
	from, to := Pt{100, 500}, Pt{1100, 500}
	pts := PlanMove(from, to, Motion{Jitter: -1})
	worst := 0.0
	for _, p := range pts {
		worst = max(worst, math.Abs(float64(p.Y-500)))
	}
	if worst < 2 {
		t.Errorf("the path deviates by only %.1fpx: that is a straight line", worst)
	}
	if worst > 100 {
		t.Errorf("the path deviates by %.1fpx from a 1000px line: too far", worst)
	}
}

func TestPlanMoveSpeedShortensTheWholeMovement(t *testing.T) {
	total := func(m Motion) time.Duration {
		var d time.Duration
		for _, p := range PlanMove(Pt{0, 0}, Pt{900, 400}, m) {
			d += p.After
		}
		return d
	}
	slow, fast := total(Motion{Speed: 1}), total(Motion{Speed: 3})
	if fast >= slow/2 {
		t.Errorf("speed 3 took %v against %v at speed 1; it should be much shorter", fast, slow)
	}
	if fast < minDuration {
		t.Errorf("speed 3 collapsed the movement to %v", fast)
	}
}

func TestMoveDurationGrowsWithDistanceButNotLinearly(t *testing.T) {
	short, long := MoveDuration(100), MoveDuration(1600)
	if long <= short {
		t.Fatalf("1600px (%v) is not slower than 100px (%v)", long, short)
	}
	if long > 8*short {
		t.Errorf("1600px took %v against 100px %v: that is nearly linear, not Fitts", long, short)
	}
}

func TestPlanMoveWithARandSourceVariesThePath(t *testing.T) {
	a := PlanMove(Pt{0, 0}, Pt{800, 600}, Motion{Rand: rand.New(rand.NewSource(1))})
	b := PlanMove(Pt{0, 0}, Pt{800, 600}, Motion{Rand: rand.New(rand.NewSource(2))})
	same := len(a) == len(b)
	if same {
		for i := range a {
			if a[i] != b[i] {
				same = false
				break
			}
		}
	}
	if same {
		t.Error("two seeds planned the identical path, so the variation does nothing")
	}
	// ...and no source means reproducible, which is what a demo relies on.
	c := PlanMove(Pt{0, 0}, Pt{800, 600}, Motion{})
	d := PlanMove(Pt{0, 0}, Pt{800, 600}, Motion{})
	if len(c) != len(d) {
		t.Fatal("two unsourced plans differ in length")
	}
	for i := range c {
		if c[i] != d[i] {
			t.Fatalf("unsourced plans differ at point %d: %v vs %v", i, c[i], d[i])
		}
	}
}

// The desktop this runs on is the reason these rules exist. Every case below was
// on screen at once while the driver was being written: agentbox's 1x1 helper window
// (filtered before Choose sees it), a card, a viewer, and a terminal whose title
// happened to contain the word "agentbox" because that was the task being worked on.
func TestChoosePicksTheWindowTheCallerMeant(t *testing.T) {
	card := Candidate{Name: "agentbox", Rect: Rect{725, 483, 470, 234}, Order: 4}
	viewer := Candidate{Name: "agentbox · Tour", Rect: Rect{300, 100, 1100, 800}, Order: 5}
	terminal := Candidate{Name: "boris@box: showcase agentbox script", Rect: Rect{0, 0, 1920, 1152}, Order: 2}
	all := []Candidate{terminal, card, viewer}

	// A substring match must not hand back the enormous terminal just because it
	// is enormous: the exact title wins first.
	if got, ok := Choose(all, "agentbox"); !ok || got != card {
		t.Errorf("Choose(\"agentbox\") = %+v (ok=%v), want the card", got, ok)
	}
	// The viewer is found by the part of its title that is its own.
	if got, ok := Choose(all, "Tour"); !ok || got != viewer {
		t.Errorf("Choose(\"Tour\") = %+v (ok=%v), want the viewer", got, ok)
	}
	// "=" refuses to settle for a substring at all.
	if got, ok := Choose([]Candidate{terminal, viewer}, "=agentbox"); ok {
		t.Errorf("Choose(\"=agentbox\") matched %+v with no window titled exactly agentbox", got)
	}
	if got, ok := Choose(all, "=agentbox"); !ok || got != card {
		t.Errorf("Choose(\"=agentbox\") = %+v (ok=%v), want the card", got, ok)
	}
	if _, ok := Choose(all, "no such window"); ok {
		t.Error("Choose matched a title that is not on screen")
	}
	if _, ok := Choose(all, "  "); ok {
		t.Error("Choose matched an empty title")
	}
}

func TestChooseAmongEqualsTakesTheBiggestThenTheTopmost(t *testing.T) {
	small := Candidate{Name: "agentbox", Rect: Rect{0, 0, 100, 100}, Order: 9}
	big := Candidate{Name: "agentbox", Rect: Rect{0, 0, 400, 400}, Order: 1}
	got, _ := Choose([]Candidate{small, big}, "agentbox")
	if got != big {
		t.Errorf("picked the small window over the big one: %+v", got)
	}
	lower := Candidate{Name: "agentbox", Rect: Rect{0, 0, 400, 400}, Order: 1}
	upper := Candidate{Name: "agentbox", Rect: Rect{0, 0, 400, 400}, Order: 7}
	if got, _ := Choose([]Candidate{upper, lower}, "agentbox"); got != upper {
		t.Errorf("picked the window behind: %+v", got)
	}
}

func TestKeysymForRunes(t *testing.T) {
	for r, want := range map[rune]uint32{
		'a': 'a', 'Z': 'Z', '1': '1', ' ': ' ',
		'\n': KeyReturn, '\r': KeyReturn, '\t': KeyTab,
		'é': 0xe9,       // Latin-1 is its own keysym
		'☃': 0x01002603, // and everything above it takes the Unicode convention
	} {
		if got := KeysymFor(r); got != want {
			t.Errorf("KeysymFor(%q) = %#x, want %#x", r, got, want)
		}
	}
}

func TestLayoutImpliesUpperCase(t *testing.T) {
	l := testLayout()
	code, shift, ok := l.Rune('A')
	if !ok {
		t.Fatal("cannot type A, so X's (a, NoSymbol) shorthand is not being read")
	}
	if !shift {
		t.Error("A does not hold shift")
	}
	lower, lowerShift, _ := l.Rune('a')
	if code != lower {
		t.Errorf("A is keycode %d but a is %d; they are the same key", code, lower)
	}
	if lowerShift {
		t.Error("a holds shift")
	}
}

func TestLayoutKeepsTheFirstKeycodeForAKeysym(t *testing.T) {
	// Keycode 17 in the fixture also produces 'a'. Typing must use the first one:
	// the later keycodes are numpads and second layout groups, which produce the
	// character in a way nobody pressed.
	code, _, _ := testLayout().Rune('a')
	if code != 10 {
		t.Errorf("a resolved to keycode %d, want the first one (10)", code)
	}
}

func TestLayoutReadsShiftedSymbols(t *testing.T) {
	l := testLayout()
	code, shift, ok := l.Rune('!')
	if !ok || !shift {
		t.Fatalf("! resolved to (%d, shift=%v, ok=%v); want the shifted 1 key", code, shift, ok)
	}
	if want, _, _ := l.Rune('1'); code != want {
		t.Errorf("! is keycode %d but 1 is %d", code, want)
	}
}

func TestPlanTextShiftsWhereItMustAndPausesLikeAPerson(t *testing.T) {
	l := testLayout()
	strokes, skipped := PlanText("ab A.", l, Typing{WPM: 300})
	if len(skipped) != 0 {
		t.Fatalf("the fixture layout could not type %q", string(skipped))
	}
	if len(strokes) != 5 {
		t.Fatalf("planned %d strokes for 5 characters", len(strokes))
	}
	if strokes[3].Rune != 'A' || !strokes[3].Shift {
		t.Errorf("the capital A did not hold shift: %+v", strokes[3])
	}
	if strokes[0].Shift {
		t.Errorf("the lower-case a held shift: %+v", strokes[0])
	}
	letter, space, period := strokes[0].After, strokes[2].After, strokes[4].After
	if space <= letter {
		t.Errorf("a space (%v) does not cost more than a letter (%v)", space, letter)
	}
	if period <= space {
		t.Errorf("a full stop (%v) does not cost more than a space (%v)", period, space)
	}
}

func TestPlanTextReportsWhatTheLayoutCannotType(t *testing.T) {
	strokes, skipped := PlanText("a☃b", testLayout(), Typing{})
	if string(skipped) != "☃" {
		t.Errorf("skipped %q, want the snowman", string(skipped))
	}
	if len(strokes) != 2 {
		t.Errorf("planned %d strokes; the two typeable characters should still be typed", len(strokes))
	}
}

func TestPlanTextWPMSetsThePace(t *testing.T) {
	sum := func(wpm int) time.Duration {
		var d time.Duration
		for _, s := range mustStrokes(t, "abab abab", wpm) {
			d += s.After
		}
		return d
	}
	slow, fast := sum(60), sum(600)
	if fast >= slow/5 {
		t.Errorf("600 wpm took %v against %v at 60 wpm", fast, slow)
	}
}

func mustStrokes(t *testing.T, text string, wpm int) []Stroke {
	t.Helper()
	strokes, skipped := PlanText(text, testLayout(), Typing{WPM: wpm})
	if len(skipped) != 0 {
		t.Fatalf("fixture layout cannot type %q", string(skipped))
	}
	return strokes
}
