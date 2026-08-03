package assign

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func f(v float64) *float64 { return new(v) }

func TestRenderSubstitutesAndReportsWhatItCouldNot(t *testing.T) {
	tmpl := "Check usage for {{window}}. Warn above {{critical}}%. Keep stats: {{keep}}. Also {{nothing}}."
	got, missing := Render(tmpl, map[string]any{
		"window":   "7d",
		"critical": float64(85),
		"keep":     true,
	})
	want := "Check usage for 7d. Warn above 85%. Keep stats: yes. Also {{nothing}}."
	if got != want {
		t.Errorf("rendered\n got %q\nwant %q", got, want)
	}
	if !reflect.DeepEqual(missing, []string{"nothing"}) {
		t.Errorf("missing = %v, want [nothing]", missing)
	}
	// The unfilled slot stays IN the prompt. A clause that silently vanishes is
	// worse than one that visibly did not get its value.
	if !strings.Contains(got, "{{nothing}}") {
		t.Error("an unfilled placeholder was dropped from the prompt")
	}
}

// A value is data, not another template: an assignment prompt goes to a model,
// and a template language with corners has an injection in one of them.
func TestRenderDoesNotRecurse(t *testing.T) {
	got, _ := Render("run {{a}}", map[string]any{"a": "{{b}}", "b": "boom"})
	if got != "run {{b}}" {
		t.Fatalf("got %q, want the value left as data", got)
	}
}

func TestValueStringReadsAsASentence(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{float64(85), "85"},   // not 85.000000
		{float64(0.5), "0.5"}, // and not 0.500000 either
		{true, "yes"},         // not Go's true
		{false, "no"},
		{"7d", "7d"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := valueString(c.in); got != c.want {
			t.Errorf("valueString(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPlaceholdersInFirstSeenOrder(t *testing.T) {
	got := Placeholders("{{b}} then {{a}} then {{b}} again")
	if !reflect.DeepEqual(got, []string{"b", "a"}) {
		t.Fatalf("got %v, want [b a]", got)
	}
}

func TestValidateReportsEveryProblemAtOnce(t *testing.T) {
	probs := Validate([]Param{
		{Key: "a", Type: TypeText},
		{Key: "a", Type: TypeNumber},                       // duplicate
		{Key: "b", Type: "colour"},                         // unknown type
		{Type: TypeSlider, Key: "c"},                       // no bounds
		{Key: "d", Type: TypeEnum},                         // no values
		{Type: TypeMarkdown},                               // no body
		{Type: TypeText},                                   // no key
		{Key: "e", Type: TypeSlider, Min: f(9), Max: f(1)}, // inverted
	})
	if len(probs) != 7 {
		t.Fatalf("reported %d problems, want 7:\n%s", len(probs), strings.Join(probs, "\n"))
	}
}

// An agent rewriting the knobs must not be able to discard what Boris set. What
// still has a knob survives; what does not is gone, because keeping it would
// leave the prompt filled from something the panel cannot show or change.
func TestMergeKeepsSetValuesAcrossASpecRewrite(t *testing.T) {
	spec := []Param{
		{Key: "window", Type: TypeEnum, Values: []string{"24h", "7d"}, Default: "7d"},
		{Key: "critical", Type: TypeSlider, Min: f(0), Max: f(100), Default: float64(85)},
		{Type: TypeMarkdown, Body: "Some prose."},
	}
	got := Merge(spec, map[string]any{
		"window":  "24h",  // still exists: kept
		"dropped": "gone", // knob removed: not carried
	})
	want := map[string]any{"window": "24h", "critical": float64(85)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merged %v, want %v", got, want)
	}
}

func TestParseScheduleGrammar(t *testing.T) {
	ok := map[string]Schedule{
		"":                {},
		"every 30m":       {Every: 30 * time.Minute},
		"every 4h":        {Every: 4 * time.Hour},
		"every 1d":        {Every: 24 * time.Hour},
		"daily 09:00":     {Hour: 9, Daily: true},
		"weekly mon 07:5": {},
	}
	delete(ok, "weekly mon 07:5") // covered in the failure table
	for in, want := range ok {
		got, err := ParseSchedule(in)
		if err != nil {
			t.Errorf("ParseSchedule(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseSchedule(%q) = %+v, want %+v", in, got, want)
		}
	}
	if got, err := ParseSchedule("weekly Mon 09:30"); err != nil ||
		got != (Schedule{Hour: 9, Min: 30, Weekday: time.Monday, Weekly: true}) {
		t.Errorf("weekly = %+v, %v", got, err)
	}

	bad := []string{
		"every",    // no interval
		"every 5s", // below the one-minute floor: an expensive typo
		"every 0d",
		"every soon",
		"daily",
		"daily 25:00",
		"daily 9",
		"weekly 09:00", // no day
		"weekly xyz 09:00",
		"hourly",
	}
	for _, in := range bad {
		if _, err := ParseSchedule(in); err == nil {
			t.Errorf("ParseSchedule(%q) was accepted", in)
		}
	}
}

// Creating an assignment is not the same gesture as running it: an interval
// counts forward from now, so "every 1h" made at 14:03 first runs at 15:03.
func TestNextDoesNotFireOnCreation(t *testing.T) {
	now := time.Date(2026, 8, 1, 14, 3, 0, 0, time.UTC)
	s, _ := ParseSchedule("every 1h")
	if got := s.Next(now, time.Time{}); !got.Equal(now.Add(time.Hour)) {
		t.Fatalf("first run at %v, want %v", got, now.Add(time.Hour))
	}
}

func TestNextFromTheLastRun(t *testing.T) {
	now := time.Date(2026, 8, 1, 14, 3, 0, 0, time.UTC)
	last := now.Add(-20 * time.Minute)
	s, _ := ParseSchedule("every 30m")
	if got := s.Next(now, last); !got.Equal(last.Add(30 * time.Minute)) {
		t.Fatalf("next at %v, want %v", got, last.Add(30*time.Minute))
	}
}

func TestNextDailyAndWeeklyRollForward(t *testing.T) {
	// 14:03 on a Saturday.
	now := time.Date(2026, 8, 1, 14, 3, 0, 0, time.UTC)
	if now.Weekday() != time.Saturday {
		t.Fatalf("fixture drifted: %v", now.Weekday())
	}

	d, _ := ParseSchedule("daily 09:00")
	want := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC) // already past today
	if got := d.Next(now, time.Time{}); !got.Equal(want) {
		t.Errorf("daily next = %v, want %v", got, want)
	}

	d2, _ := ParseSchedule("daily 18:30")
	want2 := time.Date(2026, 8, 1, 18, 30, 0, 0, time.UTC) // still ahead today
	if got := d2.Next(now, time.Time{}); !got.Equal(want2) {
		t.Errorf("daily next = %v, want %v", got, want2)
	}

	w, _ := ParseSchedule("weekly mon 09:00")
	want3 := time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)
	if got := w.Next(now, time.Time{}); !got.Equal(want3) {
		t.Errorf("weekly next = %v, want %v", got, want3)
	}
}

// The laptop was shut for the weekend. Nothing catches up; the count is what
// the panel reports instead (Boris, 2026-08-01).
func TestMissedCountsSlotsRatherThanFiringThem(t *testing.T) {
	s, _ := ParseSchedule("every 1h")
	due := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	now := due.Add(3*time.Hour + 10*time.Minute)
	if got := s.Missed(due, now); got != 4 {
		t.Errorf("missed = %d, want 4", got)
	}
	if got := s.Missed(due, due.Add(-time.Minute)); got != 0 {
		t.Errorf("a slot not yet due counted as missed (%d)", got)
	}
	if got := s.Missed(time.Time{}, now); got != 0 {
		t.Errorf("an ad-hoc assignment reported %d missed runs", got)
	}
	// A machine off for a year says "a lot" rather than counting to 8760.
	if got := s.Missed(due, due.AddDate(1, 0, 0)); got != 999 {
		t.Errorf("cap = %d, want 999", got)
	}
}

func TestKindNamesTheTrigger(t *testing.T) {
	for in, want := range map[string]string{
		"":                 "ad-hoc",
		"  ":               "ad-hoc",
		"every 30m":        "periodic",
		"daily 09:00":      "scheduled",
		"weekly mon 09:00": "scheduled",
	} {
		if got := Kind(in); got != want {
			t.Errorf("Kind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDefaultsFillEveryInputAndNoProse(t *testing.T) {
	got := Defaults([]Param{
		{Key: "a", Type: TypeText},
		{Key: "b", Type: TypeToggle},
		{Key: "c", Type: TypeSlider, Min: f(10), Max: f(20)},
		{Key: "d", Type: TypeEnum, Values: []string{"x", "y"}},
		{Type: TypeMarkdown, Body: "prose"},
	})
	want := map[string]any{"a": "", "b": false, "c": float64(10), "d": "x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("defaults %v, want %v", got, want)
	}
}
