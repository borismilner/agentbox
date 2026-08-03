package webui

import (
	"testing"

	"github.com/borismilner/agentbox/internal/daemon"
	"github.com/borismilner/agentbox/internal/proto"
)

// pu is a UI with the default configuration behind it: the progress geometry these tests exercise
// is configuration now, not constants.
var pu = confUI()

func TestEncodeReports(t *testing.T) {
	rows := encodeReports([]daemon.ProgressState{
		{ID: "p1", Title: "Migrating events", Status: "412 of 1204 rows", Percent: 34,
			Identity: proto.Identity{Agent: "claude-code", Project: "grabbit"}},
		{ID: "p2", Percent: 140}, // a caller that cannot count
		{ID: "p3", Percent: -8},
		{ID: "p4", Indeterminate: true, Title: "Indexing"},
	}, true)

	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(rows))
	}
	if rows[0].Title != "Migrating events" || rows[0].Percent != 34 || rows[0].Status != "412 of 1204 rows" {
		t.Errorf("row 0 = %+v", rows[0])
	}
	if rows[0].Agent != "claude-code" || rows[0].Project != "grabbit" {
		t.Errorf("the caller's identity should reach the surface: %+v", rows[0])
	}
	if rows[0].Hue != IdentityHue("claude-code", "grabbit", true) {
		t.Errorf("hue = %q, want the shared identity hue", rows[0].Hue)
	}

	// A bar cannot be more than full or less than empty, whatever the caller says.
	if rows[1].Percent != 100 {
		t.Errorf("percent = %d, want clamped to 100", rows[1].Percent)
	}
	if rows[2].Percent != 0 {
		t.Errorf("percent = %d, want clamped to 0", rows[2].Percent)
	}

	// A report with no title is still a report.
	if rows[1].Title != "Working" {
		t.Errorf("title = %q, want the Working fallback", rows[1].Title)
	}
	if !rows[3].Indeterminate || rows[3].Title != "Indexing" {
		t.Errorf("row 3 = %+v", rows[3])
	}

	if got := encodeReports(nil, true); got == nil || len(got) != 0 {
		t.Errorf("an empty set must encode as an empty list, not null: %#v", got)
	}
}

// The window opens tight on one task and grows with the set, but never past the
// cap - a hundred reports must not produce a window taller than the screen.
func TestProgressHeight(t *testing.T) {
	one, three := pu.prog.progressHeight(1), pu.prog.progressHeight(3)
	if one >= three {
		t.Errorf("height(1)=%d height(3)=%d: more tasks need more room", one, three)
	}
	if pu.prog.progressHeight(0) != one {
		t.Error("an empty set should size like one row, not zero")
	}
	_, maxProgH := pu.progressGeom()
	if h := pu.prog.progressHeight(100); h != maxProgH {
		t.Errorf("height(100) = %d, want the %d cap", h, maxProgH)
	}
}
