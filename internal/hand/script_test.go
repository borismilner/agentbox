package hand

import (
	"strings"
	"testing"
)

func TestParseCoordSpellings(t *testing.T) {
	for _, tc := range []struct {
		in   string
		kind CoordKind
		val  float64
	}{
		{"400", CoordEdge, 400},
		{"-46", CoordFromEnd, 46},
		{"60%", CoordFrac, 0.6},
		{"-25%", CoordFracEnd, 0.25},
		{"center", CoordCenter, 0},
		{"MIDDLE", CoordCenter, 0},
		{"~", CoordPointer, 0},
		{"~+30", CoordPointer, 30},
		{"~-30", CoordPointer, -30},
	} {
		got, err := ParseCoord(tc.in)
		if err != nil {
			t.Errorf("ParseCoord(%q): %v", tc.in, err)
			continue
		}
		if got.Kind != tc.kind || got.Value != tc.val {
			t.Errorf("ParseCoord(%q) = %+v, want kind %v value %v", tc.in, got, tc.kind, tc.val)
		}
	}
	for _, bad := range []string{"", "left", "40px", "~x", "%"} {
		if _, err := ParseCoord(bad); err == nil {
			t.Errorf("ParseCoord(%q) was accepted", bad)
		}
	}
}

// The frame is a window somewhere on the screen, and this is the arithmetic every
// step depends on. A card's buttons are found with "-46", which has to mean 46
// pixels up from the window's own bottom edge whatever the window's height.
func TestCoordResolveInAWindowFrame(t *testing.T) {
	const origin, size, at = 100, 400, 250
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"0", 100},
		{"40", 140},
		{"-46", 454},
		{"50%", 300},
		{"-25%", 400},
		{"center", 300},
		{"~", 250},
		{"~+30", 280},
		{"~-30", 220},
	} {
		c, err := ParseCoord(tc.in)
		if err != nil {
			t.Fatalf("ParseCoord(%q): %v", tc.in, err)
		}
		if got := c.Resolve(origin, size, at); got != tc.want {
			t.Errorf("%q resolved to %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseScriptSkipsBlanksAndComments(t *testing.T) {
	steps, err := ParseScript("# a comment\n\n  \nmove center center\n# another\nclick\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 {
		t.Fatalf("parsed %d steps, want 2: %+v", len(steps), steps)
	}
	if steps[0].Op != OpMove || steps[1].Op != OpClick {
		t.Errorf("got %v then %v", steps[0].Op, steps[1].Op)
	}
	if steps[1].Line != 6 {
		t.Errorf("the click is reported on line %d, want 6", steps[1].Line)
	}
}

func TestParseScriptNamesTheLineThatIsWrong(t *testing.T) {
	_, err := ParseScript("move 1 1\nclick\nfly to the moon\n")
	if err == nil {
		t.Fatal("a script with an unknown step was accepted")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("error does not name line 3: %v", err)
	}
}

func TestParseScriptRefusesAnEmptyScript(t *testing.T) {
	if _, err := ParseScript("# nothing but a comment\n"); err == nil {
		t.Error("an empty script was accepted; running nothing is never what the caller meant")
	}
}

func TestParseStepClickForms(t *testing.T) {
	for _, tc := range []struct {
		in     string
		to     bool
		button byte
	}{
		{"click", false, 1},
		{"click right", false, 3},
		{"click 2", false, 2},
		{"click 10 20", true, 1},
		{"click 10 20 middle", true, 2},
		{"double center center", true, 1},
	} {
		st, err := ParseStep(tc.in)
		if err != nil {
			t.Errorf("ParseStep(%q): %v", tc.in, err)
			continue
		}
		if st.To != tc.to || st.Button != tc.button {
			t.Errorf("ParseStep(%q) = to:%v button:%d, want to:%v button:%d",
				tc.in, st.To, st.Button, tc.to, tc.button)
		}
	}
}

func TestParseStepTakesTheRestOfTheLineVerbatim(t *testing.T) {
	st, err := ParseStep(`type git commit -m "two  spaces and #hash"`)
	if err != nil {
		t.Fatal(err)
	}
	if st.Text != `git commit -m "two  spaces and #hash"` {
		t.Errorf("type reworded its own text: %q", st.Text)
	}
	win, err := ParseStep("window agentbox · Release notes")
	if err != nil {
		t.Fatal(err)
	}
	if win.Text != "agentbox · Release notes" {
		t.Errorf("window title came out as %q", win.Text)
	}
}

func TestParseStepKeysAndNumbers(t *testing.T) {
	st, err := ParseStep("key ctrl+alt+t Escape shift+Tab")
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Keys) != 3 || st.Keys[2] != "shift+Tab" {
		t.Errorf("keys parsed as %v", st.Keys)
	}
	if st, err = ParseStep("scroll -4"); err != nil || st.N != -4 {
		t.Errorf("scroll -4 parsed as %d (%v)", st.N, err)
	}
	if st, err = ParseStep("wait 250"); err != nil || st.N != 250 {
		t.Errorf("wait 250 parsed as %d (%v)", st.N, err)
	}
	if st, err = ParseStep("speed 2.5"); err != nil || st.F != 2.5 {
		t.Errorf("speed 2.5 parsed as %v (%v)", st.F, err)
	}
}

// Every step is validated at parse time, so Run can send its first event knowing
// the last one is spellable. Half a driven desktop is worse than none.
func TestParseStepRefusesWhatItCannotRun(t *testing.T) {
	for _, bad := range []string{
		"move 10",              // one coordinate
		"move 10 20 30",        // three
		"drag 1 2 3",           // three of four
		"scroll",               // no notches
		"scroll 0",             // a scroll of nothing
		"scroll two",           // not a number
		"wait",                 // no milliseconds
		"wait -5",              // backwards
		"speed 0",              // stopped
		"wpm nine",             // not a number
		"type",                 // nothing to type
		"key",                  // no combination
		"window",               // no title
		"screen now",           // takes nothing
		"click 10 20 sideways", // not a button
		"click 10 20 30 40",    // too many
	} {
		if _, err := ParseStep(bad); err == nil {
			t.Errorf("ParseStep(%q) was accepted", bad)
		}
	}
}
