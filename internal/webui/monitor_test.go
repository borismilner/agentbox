package webui

import "testing"

// This laptop's actual layout, and the reason monitor.go exists: a portrait panel
// at the origin beside a landscape one, so the X root is 3000x1920 and its centre
// is on neither screen's middle.
var (
	portrait  = mon{x: 0, y: 0, w: 1080, h: 1920}
	landscape = mon{x: 1080, y: 0, w: 1920, h: 1080, primary: true}
	root      = mon{x: 0, y: 0, w: 3000, h: 1920, primary: true}
	both      = []mon{portrait, landscape}
)

// The pointer decides. This is the whole bug: with the pointer on the portrait
// screen, a card must not be placed using the landscape one - or the root.
func TestPickMonFollowsThePointer(t *testing.T) {
	if got := pickMon(both, 500, 900, true, root); got != portrait {
		t.Errorf("pointer at 500,900 picked %+v, want the portrait screen", got)
	}
	if got := pickMon(both, 2000, 500, true, root); got != landscape {
		t.Errorf("pointer at 2000,500 picked %+v, want the landscape screen", got)
	}
	// The seam belongs to the monitor that owns the column, not to whichever is
	// listed first: x=1080 is the landscape screen's first pixel.
	if got := pickMon(both, 1080, 10, true, root); got != landscape {
		t.Errorf("pointer at the seam picked %+v, want the landscape screen", got)
	}
	if got := pickMon(both, 1079, 10, true, root); got != portrait {
		t.Errorf("pointer one pixel left of the seam picked %+v, want the portrait screen", got)
	}
}

// The fallbacks, in order. Each of these happens: a pointer in the dead area of a
// mismatched layout (1080x1920 beside 1920x1080 leaves 840 rows of nothing under
// the landscape screen), a layout with nothing marked primary, and a server that
// cannot describe its monitors at all.
func TestPickMonFallbacks(t *testing.T) {
	if got := pickMon(both, 2000, 1500, true, root); got != landscape {
		t.Errorf("pointer in the gap picked %+v, want the primary screen", got)
	}
	if got := pickMon(both, 0, 0, false, root); got != landscape {
		t.Errorf("no pointer picked %+v, want the primary screen", got)
	}

	noPrimary := []mon{{x: 1080, y: 0, w: 1920, h: 1080}, portrait}
	if got := pickMon(noPrimary, 0, 0, false, root); got != noPrimary[0] {
		t.Errorf("nothing primary picked %+v, want the first monitor", got)
	}
	if got := pickMon(nil, 500, 900, true, root); got != root {
		t.Errorf("no RandR picked %+v, want the root", got)
	}
	// A monitor that is switched off reports a zero rect; placing in it would put
	// every window at its origin.
	off := []mon{{x: 0, y: 0, w: 0, h: 0, primary: true}, landscape}
	if got := pickMon(off, 0, 0, true, root); got != landscape {
		t.Errorf("a zero-sized monitor was picked (%+v)", got)
	}
}

// A card takes the middle of its monitor. The landscape numbers are the ones that
// were wrong on screen: a 470x260 card was at x=1265 (root centre, on the far
// screen) instead of x=1805.
func TestCentreInIsTheMonitorNotTheRoot(t *testing.T) {
	if x, y := centreIn(landscape, 470, 260); x != 1805 || y != 410 {
		t.Errorf("centre on the landscape screen = %d,%d, want 1805,410", x, y)
	}
	if x, y := centreIn(portrait, 470, 260); x != 305 || y != 830 {
		t.Errorf("centre on the portrait screen = %d,%d, want 305,830", x, y)
	}
	// Wider than the monitor: clipped on the right, never started off to the left
	// of the screen it belongs to.
	if x, y := centreIn(landscape, 2400, 1400); x != 1080 || y != 0 {
		t.Errorf("an oversized window = %d,%d, want the monitor's own origin", x, y)
	}
}

// A toast is centred across but pinned under the top edge, and the inset is
// measured from that monitor's top - which on a second screen is not y=0 of the
// root in the general case.
func TestTopCentreInPinsToTheMonitorTop(t *testing.T) {
	if x, y := topCentreIn(landscape, 470, 24); x != 1805 || y != 24 {
		t.Errorf("toast = %d,%d, want 1805,24", x, y)
	}
	below := mon{x: 0, y: 1080, w: 1920, h: 1080}
	if _, y := topCentreIn(below, 470, 24); y != 1104 {
		t.Errorf("toast on a stacked-below monitor at y = %d, want 1104", y)
	}
}

// Progress lives in the bottom-right of the monitor you are on. On the root it
// used to land in the far bottom corner of the far screen.
func TestCornerInIsTheMonitorsCorner(t *testing.T) {
	if x, y := cornerIn(landscape, 420, 172, 28, 52); x != 2552 || y != 856 {
		t.Errorf("corner = %d,%d, want 2552,856", x, y)
	}
	if x, y := cornerIn(portrait, 420, 172, 28, 52); x != 632 || y != 1696 {
		t.Errorf("corner on the portrait screen = %d,%d, want 632,1696", x, y)
	}
	// A window bigger than the monitor still starts inside it.
	if x, y := cornerIn(portrait, 2000, 3000, 28, 52); x != 0 || y != 0 {
		t.Errorf("an oversized progress window = %d,%d, want 0,0", x, y)
	}
}

// The panel is top-centre of one monitor: predictable, and the same place every
// time you hit the hotkey on a given screen. Its rectangle is a fraction of that
// monitor, so the portrait screen gets a portrait-shaped panel.
func TestPanelSizeAndOriginAreOneMonitors(t *testing.T) {
	u := gateUI()
	u.pan = newPanel(u)
	u.cfg.Panel.WidthFrac, u.cfg.Panel.HeightFrac = 0.8, 0.5

	w, h := u.pan.sizeOn(landscape)
	if w != 1536 || h != 540 {
		t.Errorf("panel on the landscape screen = %dx%d, want 1536x540", w, h)
	}
	if x := landscape.x + atLeast0((landscape.w-w)/2); x != 1272 {
		t.Errorf("panel origin x = %d, want 1272 (centred on that screen)", x)
	}

	w, h = u.pan.sizeOn(portrait)
	if w != 864 || h != 960 {
		t.Errorf("panel on the portrait screen = %dx%d, want 864x960", w, h)
	}
}
