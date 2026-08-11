package webui

import (
	"io"
	"log/slog"
	"testing"
)

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

// R-12. A roll that put nothing on screen must not report an open panel, because
// PanelOpen is one of the two inputs to ask routing: routeAsk is called with
// AppOpen() || PanelOpen(), and a routed question is answered by the session
// surface instead of a card. Claim a panel that never mapped and the question goes
// to a surface nobody can see, with no card made and nothing on screen to answer.
func TestSlideWithNoWindowReportsShut(t *testing.T) {
	u := &UI{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	u.pan = newPanel(u)

	// No window and no X11, which is what slide sees when the window went away
	// underneath the roll. It used to record open anyway, from the deferred
	// p.open = down.
	u.pan.slide(true)

	if u.pan.Open() {
		t.Error("panel reports open after a roll that mapped nothing")
	}
	if u.PanelOpen() {
		t.Error("PanelOpen() is true with nothing on screen: an ask would route to a surface nobody can see")
	}
	if u.pan.animating {
		t.Error("animating stayed set, so Toggle and Show would both refuse forever")
	}
}

// Rolling up is allowed to say shut whatever happened, and must: the window is
// hidden by Hide the moment slide returns, so a failed roll up is still shut.
func TestSlideUpAlwaysReportsShut(t *testing.T) {
	u := &UI{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	u.pan = newPanel(u)
	u.pan.open = true

	u.pan.slide(false)

	if u.pan.Open() {
		t.Error("panel still reports open after rolling up")
	}
}
