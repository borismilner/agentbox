package webui

import "testing"

// The roll's shape. A linear ramp is what "mechanical" means, so the curve has to
// be measurably ahead of linear in the first half and settle in the second: the
// panel should leave the top edge quickly and arrive gently.
func TestPanelEaseLeavesFastAndSettles(t *testing.T) {
	if got := panelEase(0); got != 0 {
		t.Errorf("panelEase(0) = %v, want 0: the roll must start shut", got)
	}
	if got := panelEase(1); got != 1 {
		t.Errorf("panelEase(1) = %v, want 1: the roll must land on the full height", got)
	}

	// Halfway through the time, most of the way down: cubic ease-out puts it at
	// 7/8. Linear or quadratic (0.5 and 0.75) both fail this.
	if got := panelEase(0.5); got < 0.86 || got > 0.89 {
		t.Errorf("panelEase(0.5) = %v, want ~0.875 (ahead of linear, and of quadratic)", got)
	}

	// Monotonic, or the panel visibly bounces.
	prev := -1.0
	for i := 0; i <= 100; i++ {
		f := float64(i) / 100
		e := panelEase(f)
		if e < prev {
			t.Fatalf("panelEase went backwards at f=%v: %v after %v", f, e, prev)
		}
		if e < 0 || e > 1 {
			t.Fatalf("panelEase(%v) = %v, outside 0..1", f, e)
		}
		prev = e
	}

	// Out of range is clamped rather than extrapolated: the clock loop can hand it
	// a fraction slightly over 1 on a frame that overran.
	if got := panelEase(1.4); got != 1 {
		t.Errorf("panelEase(1.4) = %v, want 1", got)
	}
	if got := panelEase(-0.2); got != 0 {
		t.Errorf("panelEase(-0.2) = %v, want 0", got)
	}
}

// With the animation off ([panel] slide_ms = 0, or theme.motion = "none") the
// panel still has to end up in the right state - open all the way, or shut.
func TestOpen01IsTheEndState(t *testing.T) {
	if got := open01(true); got != 1 {
		t.Errorf("open01(down) = %v, want 1", got)
	}
	if got := open01(false); got != 0 {
		t.Errorf("open01(up) = %v, want 0", got)
	}
}
