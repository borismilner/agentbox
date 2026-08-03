package webui

// Which screen a window lands on.
//
// An X display with two monitors has ONE root window, and it is the bounding box
// of them both. On this laptop that is a 1080x1920 portrait panel at +0+0 beside
// a 1920x1080 one at +1080+0, so the root is 3000x1920 and "centre of the screen"
// is x=1500 - the seam between them, which is to say the wrong screen. Every card
// agentbox showed was centred there, and the drop-down panel rolled down across the
// join. The fix is not a smarter centre: it is to stop treating the root as a
// screen and place inside one monitor's rectangle instead.
//
// The maths lives here, apart from the X calls in x11.go, because it is the half
// worth testing: a display is not available to a test and a rectangle is.

// mon is one monitor's area, in root coordinates. A single-head display has one
// of these covering the whole root, which is why the old code was right until it
// was not.
type mon struct {
	x, y, w, h int
	primary    bool
}

func (m mon) contains(px, py int) bool {
	return px >= m.x && px < m.x+m.w && py >= m.y && py < m.y+m.h
}

func (m mon) usable() bool { return m.w > 0 && m.h > 0 }

// pickMon chooses the monitor a window should appear on.
//
// The pointer wins. It is the cheapest honest answer to "which screen is the
// person at" - it is where their hand is, it is what a window manager uses for
// the same decision, and it costs one round trip. The focused window is the other
// candidate and it is worse: a fullscreen deck can hold the focus on one screen
// while the work happens on the other, which is exactly the case that made this
// bug visible.
//
// Then the fallbacks, in order: the primary monitor when the pointer sits in a
// gap no monitor covers (a mismatched layout leaves those) or the pointer could
// not be read at all, the first monitor when nothing is marked primary, and the
// root when RandR has nothing to say - a server without the extension, or one
// older than the 1.5 that knows about monitors.
func pickMon(mons []mon, px, py int, havePtr bool, root mon) mon {
	if havePtr {
		for _, m := range mons {
			if m.usable() && m.contains(px, py) {
				return m
			}
		}
	}
	for _, m := range mons {
		if m.usable() && m.primary {
			return m
		}
	}
	for _, m := range mons {
		if m.usable() {
			return m
		}
	}
	return root
}

// centreIn is the top-left corner that centres a w x h window in m: a card takes
// the middle of the screen the person is on.
func centreIn(m mon, w, h int) (int, int) {
	return m.x + atLeast0((m.w-w)/2), m.y + atLeast0((m.h-h)/2)
}

// topCentreIn pins a toast under the monitor's top edge. Only x is centred; the
// inset is configuration ([window] toast_top_inset).
func topCentreIn(m mon, w, inset int) (int, int) {
	return m.x + atLeast0((m.w-w)/2), m.y + atLeast0(inset)
}

// cornerIn pins a window to the monitor's bottom-right, inset from both edges.
// This is where progress goes (FR21), and it has to be the bottom right of a
// monitor rather than of the root or it is off the far corner of the far screen.
func cornerIn(m mon, w, h, right, bottom int) (int, int) {
	return m.x + atLeast0(m.w-w-right), m.y + atLeast0(m.h-h-bottom)
}

// atLeast0 keeps a window from starting before its monitor's edge. A window wider
// or taller than the monitor is clipped on one side, which beats having half of it
// on a screen that is not there.
func atLeast0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
