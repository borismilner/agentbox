package webui

import "testing"

// The fullscreen marker's rule (FR74), as a function over rectangles. A display
// is not available to a test and a rectangle is - the same split monitor.go uses,
// and the reason the decision was pulled out of the keeper's goroutine.
func TestPlanMark(t *testing.T) {
	left := mon{x: 0, y: 0, w: 1080, h: 1920, primary: true}
	right := mon{x: 1080, y: 0, w: 1920, h: 1080}

	cases := []struct {
		name    string
		fs      bool
		fsM     mon
		stripM  mon
		want    markPlan
		because string
	}{
		{
			name: "nothing fullscreen", fs: false, stripM: left,
			want:    markPlan{},
			because: "the strip is legible on its own; a marker beside it says nothing new",
		},
		{
			name: "fullscreen over the strip", fs: true, fsM: left, stripM: left,
			want:    markPlan{mark: true, markMon: left, step: true},
			because: "this is the case the feature exists for: get out of the picture, leave the line",
		},
		{
			name: "fullscreen on the other monitor", fs: true, fsM: right, stripM: left,
			want:    markPlan{mark: true, markMon: right, step: false},
			because: "the strip covers nothing there, so hiding it would trade a legible sign for a thin one",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := planMark(c.fs, c.fsM, c.stripM)
			if got != c.want {
				t.Fatalf("planMark = %+v, want %+v - %s", got, c.want, c.because)
			}
		})
	}
}

// The marker goes where the fullscreen window is, never where the pointer is:
// mid-run the pointer is wherever the agent last moved it, which is the one
// signal that cannot be trusted while an agent drives.
func TestPlanMarkFollowsTheFullscreenWindowNotTheStrip(t *testing.T) {
	a := mon{x: 0, y: 0, w: 1080, h: 1920}
	b := mon{x: 1080, y: 0, w: 1920, h: 1080}
	if got := planMark(true, b, a); got.markMon != b {
		t.Fatalf("marker on %+v, want the fullscreen window's monitor %+v", got.markMon, b)
	}
}

// Recording mode's rule (FR95), as a function over three booleans. The windows are
// not available to a test and the decision is, which is the split planMark uses.
func TestPlanSurface(t *testing.T) {
	cases := []struct {
		name                     string
		quiet, wasQuiet, haveWin bool
		want                     surfacePlan
		because                  string
	}{
		{
			name: "first paint of a run", haveWin: false,
			want:    surfacePlan{open: true},
			because: "the strip has no window yet, which is what the whole switch was before FR95",
		},
		{
			name: "a repaint while loud", haveWin: true,
			want:    surfacePlan{},
			because: "an activity line changing must not close and reopen a window",
		},
		{
			name: "demoted mid-run", quiet: true, haveWin: true,
			want:    surfacePlan{demote: true},
			because: "he hit the key while an agent was driving",
		},
		{
			name: "a repaint while demoted", quiet: true, wasQuiet: true, haveWin: false,
			want:    surfacePlan{demote: true},
			because: "demote is idempotent, and asking for it again is what keeps the marker up",
		},
		{
			name: "a run STARTING while already demoted", quiet: true, wasQuiet: true,
			want:    surfacePlan{demote: true},
			because: "the mode is usually armed before any agent has asked for anything, and that run must come up demoted rather than loud",
		},
		{
			name: "loud again", quiet: false, wasQuiet: true,
			want:    surfacePlan{promote: true},
			because: "the marker goes and the strip comes back with the type, the ABOVE and the keeper",
		},
		{
			name: "loud again, with a strip somehow still up", quiet: false, wasQuiet: true, haveWin: true,
			want:    surfacePlan{promote: true},
			because: "promote is the one that also knows not to open a second window",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := planSurface(c.quiet, c.wasQuiet, c.haveWin)
			if got != c.want {
				t.Fatalf("planSurface(quiet=%v was=%v haveWin=%v) = %+v, want %+v - %s",
					c.quiet, c.wasQuiet, c.haveWin, got, c.want, c.because)
			}
		})
	}
}

// The one thing that must never happen: a quiet desktop asking for the strip. It
// is the failure the mode exists to remove, so it gets an assertion of its own
// rather than living inside the table above.
func TestPlanSurfaceNeverPutsTheStripBackWhileQuiet(t *testing.T) {
	for _, was := range []bool{false, true} {
		for _, win := range []bool{false, true} {
			p := planSurface(true, was, win)
			if p.open || p.promote {
				t.Errorf("quiet with was=%v haveWin=%v asked for the strip: %+v", was, win, p)
			}
		}
	}
}
