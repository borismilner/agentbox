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
