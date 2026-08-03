package webui

import (
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
)

func TestEncodeStats(t *testing.T) {
	st := proto.Stats{
		Total: 63, Questions: 41, Answered: 34, MedianAnswerMS: 14_200,
		ByAgent: []proto.AgentStat{
			{Agent: "claude-code", Total: 38, Questions: 25, Answered: 22, MedianAnswerMS: 11_800},
			{Agent: "codex", Total: 19, Questions: 13, Answered: 10, MedianAnswerMS: 0},
		},
		ByDay: []proto.DayCount{
			{Day: "2026-07-20", Count: 4},
			{Day: "2026-07-21", Count: 16},
			{Day: "2026-07-22", Count: 9},
		},
	}
	w := encodeStats(st, "7d", true)

	if w.Label != "last 7 days" || w.Window != "7d" {
		t.Fatalf("window = %q/%q", w.Window, w.Label)
	}
	// 34 of 41 questions answered: the percentage is of questions, not of every
	// interruption - a notify was never waiting on an answer.
	if w.AnsweredPct != 83 {
		t.Errorf("answeredPct = %d, want 83", w.AnsweredPct)
	}
	if w.Median != "14s" {
		t.Errorf("median = %q, want 14s", w.Median)
	}
	if w.Peak != 16 {
		t.Errorf("peak = %d, want 16", w.Peak)
	}
	if w.PerDay != "21.0/day" {
		t.Errorf("perDay = %q, want 21.0/day", w.PerDay)
	}
	if w.Empty {
		t.Error("63 interruptions is not empty")
	}
	if len(w.ByDay) != 3 || w.ByDay[1].Label != "Tue 21" {
		t.Errorf("byDay labels = %+v", w.ByDay)
	}
	if len(w.ByAgent) != 2 || w.ByAgent[0].Hue == "" {
		t.Errorf("byAgent = %+v, want identity hues", w.ByAgent)
	}
	if w.ByAgent[1].Median != "-" {
		t.Errorf("an agent with no answered question has no median, got %q", w.ByAgent[1].Median)
	}
}

func TestEncodeStatsEmptyWindow(t *testing.T) {
	w := encodeStats(proto.Stats{}, "24h", false)
	if !w.Empty || w.Label != "last 24h" {
		t.Fatalf("empty window = %+v", w)
	}
	if w.AnsweredPct != 0 || w.Median != "-" || w.PerDay != "-" {
		t.Errorf("no data must not divide by zero: %+v", w)
	}
	// The surface iterates these, so they are never nil.
	if w.ByAgent == nil || w.ByDay == nil {
		t.Error("byAgent/byDay must be empty slices, not nil")
	}
}

func TestStatsForFallsBackWithoutASource(t *testing.T) {
	u := testUI(&fakeResolver{}, nil)
	w := u.statsFor("30d")
	if !w.Empty || w.Window != "30d" || w.Label != "last 30 days" {
		t.Fatalf("no source = %+v, want an empty snapshot that keeps its window", w)
	}
	// An unknown window must not query an accidental range.
	if got := u.statsFor("everything").Window; got != "7d" {
		t.Errorf("unknown window = %q, want the 7d default", got)
	}
}

func TestSinceTimeAndWindowLabel(t *testing.T) {
	now := time.Now()
	if d := now.Sub(sinceTime("24h")); d < 23*time.Hour || d > 25*time.Hour {
		t.Errorf("24h window = %v", d)
	}
	if d := now.Sub(sinceTime("30d")); d < 29*24*time.Hour {
		t.Errorf("30d window = %v", d)
	}
	if sinceTime("all").After(time.Unix(1, 0)) {
		t.Error("all time must reach back past any history")
	}
	if d := now.Sub(sinceTime("nonsense")); d < 6*24*time.Hour || d > 8*24*time.Hour {
		t.Errorf("an unknown window should default to 7d, got %v", d)
	}
	if windowLabel("nonsense") != "last 7 days" {
		t.Error("unknown window label should default too")
	}
}

func TestFmtDurMS(t *testing.T) {
	tests := []struct {
		ms   int64
		want string
	}{
		{0, "-"},
		{-5, "-"},
		{400, "0s"},
		{600, "1s"},
		{14_200, "14s"},
		{59_400, "59s"},
		{60_000, "1m00s"},
		{96_000, "1m36s"},
		{3_600_000, "60m00s"},
	}
	for _, tc := range tests {
		if got := fmtDurMS(tc.ms); got != tc.want {
			t.Errorf("fmtDurMS(%d) = %q, want %q", tc.ms, got, tc.want)
		}
	}
}

func TestDayLabelFallsBackToTheRawDay(t *testing.T) {
	if got := dayLabel("2026-07-24"); got != "Fri 24" {
		t.Errorf("dayLabel = %q, want Fri 24", got)
	}
	if got := dayLabel("not-a-day"); got != "not-a-day" {
		t.Errorf("an unparseable day should pass through, got %q", got)
	}
}
