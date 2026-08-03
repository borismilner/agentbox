package store

import (
	"testing"

	"github.com/borismilner/agentbox/internal/assign"
)

func newAssignment(t *testing.T) *assign.Assignment {
	t.Helper()
	lo, hi := 0.0, 100.0
	return &assign.Assignment{
		ID:     NewAssignmentID(),
		Name:   "Usage watch",
		Prompt: "Check Claude usage for {{window}}. Warn above {{critical}}%.",
		Spec: []assign.Param{
			{Key: "window", Type: assign.TypeEnum, Values: []string{"24h", "7d"}, Default: "7d"},
			{Key: "critical", Type: assign.TypeSlider, Min: &lo, Max: &hi, Default: float64(85)},
		},
		Params:   map[string]any{"window": "7d", "critical": float64(85)},
		Schedule: "every 4h",
		Model:    "claude-sonnet-5",
		Mode:     "plan",
		Enabled:  true,
	}
}

func TestAssignmentRoundTrip(t *testing.T) {
	s := openTemp(t)
	a := newAssignment(t)
	if err := s.SaveAssignment(a); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := s.GetAssignment(a.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if got.Name != a.Name || got.Prompt != a.Prompt || got.Schedule != "every 4h" ||
		got.Model != "claude-sonnet-5" || !got.Enabled {
		t.Fatalf("round trip lost a field: %+v", got)
	}
	if len(got.Spec) != 2 || got.Spec[0].Key != "window" || got.Spec[1].Min == nil || *got.Spec[1].Min != 0 {
		t.Fatalf("spec did not survive: %+v", got.Spec)
	}
	if got.Params["window"] != "7d" || got.Params["critical"] != float64(85) {
		t.Fatalf("params did not survive: %+v", got.Params)
	}
	if got.CreatedMS == 0 || got.UpdatedMS == 0 {
		t.Error("timestamps not stamped")
	}
}

func TestGetMissingAssignmentIsNotAnError(t *testing.T) {
	s := openTemp(t)
	got, err := s.GetAssignment("a000000000000")
	if err != nil || got != nil {
		t.Fatalf("got %v, %v - want nil, nil: every caller has to handle gone anyway", got, err)
	}
}

func TestSaveRefusesAnUnmintedIDAndANamelessAssignment(t *testing.T) {
	s := openTemp(t)
	if err := s.SaveAssignment(&assign.Assignment{ID: "nope", Name: "x"}); err == nil {
		t.Error("an id outside the minted shape was accepted")
	}
	if err := s.SaveAssignment(&assign.Assignment{ID: NewAssignmentID()}); err == nil {
		t.Error("an assignment with no name was accepted; it could never be found again")
	}
}

// The rule the whole custom-panel escape hatch rests on: a spec an agent wrote
// badly leaves the assignment READABLE and EDITABLE. A row that can only be
// deleted is how a stored panel becomes a trap (docs/08-assignments.md).
func TestABrokenSpecStillListsAndStillCarriesItsPrompt(t *testing.T) {
	s := openTemp(t)
	a := newAssignment(t)
	if err := s.SaveAssignment(a); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE assignments SET params_spec = ?, params = ? WHERE id = ?`,
		"{not json at all", "]]]", a.ID); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetAssignment(a.ID)
	if err != nil {
		t.Fatalf("a malformed blob failed the whole read: %v", err)
	}
	if got.Prompt != a.Prompt || got.Name != a.Name {
		t.Error("the definition was lost with the spec")
	}
	if got.Spec != nil {
		t.Error("a malformed spec should read as no spec")
	}
	if got.Params == nil {
		t.Error("params must never be nil; the panel writes into it")
	}
	if list, err := s.ListAssignments(); err != nil || len(list) != 1 {
		t.Errorf("list = %d, %v - one bad row must not break the list", len(list), err)
	}
}

func TestDueAssignmentsExcludeAdHocAndDisabled(t *testing.T) {
	s := openTemp(t)
	const now = 1_000_000

	due := newAssignment(t)
	due.NextRunMS = now - 1
	adhoc := newAssignment(t)
	adhoc.ID, adhoc.Schedule, adhoc.NextRunMS = NewAssignmentID(), "", 0
	off := newAssignment(t)
	off.ID, off.Enabled, off.NextRunMS = NewAssignmentID(), false, now-1
	later := newAssignment(t)
	later.ID, later.NextRunMS = NewAssignmentID(), now+1
	for _, a := range []*assign.Assignment{due, adhoc, off, later} {
		if err := s.SaveAssignment(a); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.DueAssignments(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != due.ID {
		ids := make([]string, len(got))
		for i, a := range got {
			ids[i] = a.ID
		}
		t.Fatalf("due = %v, want only %s", ids, due.ID)
	}
}

func TestRunsRecordWhatWasActuallyUsed(t *testing.T) {
	s := openTemp(t)
	a := newAssignment(t)
	if err := s.SaveAssignment(a); err != nil {
		t.Fatal(err)
	}

	r := &Run{AssignmentID: a.ID, Trigger: "schedule", Params: map[string]any{"window": "24h"}}
	if err := s.StartRun(r); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := s.FinishRun(r.ID, RunOK, "usage at 61%", "", `{"pct":61}`); err != nil {
		t.Fatalf("finish: %v", err)
	}

	// The definition moves on; the run keeps what it ran with.
	a.Params["window"] = "30d"
	if err := s.SaveAssignment(a); err != nil {
		t.Fatal(err)
	}

	runs, err := s.RunsFor(a.ID, 0)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs = %d, %v", len(runs), err)
	}
	got := runs[0]
	if got.Params["window"] != "24h" {
		t.Errorf("the run followed the definition (%v); it must keep its own values", got.Params)
	}
	if got.State != RunOK || got.Summary != "usage at 61%" || got.Data != `{"pct":61}` {
		t.Errorf("run did not round trip: %+v", got)
	}
	if got.EndedMS == 0 {
		t.Error("finish did not stamp an end")
	}
}

// A run is a child process. One flagged running when the daemon died is not
// running now, and a row that claims otherwise makes the panel show work in
// progress forever.
func TestReapMarksOrphanedRunsFailed(t *testing.T) {
	s := openTemp(t)
	a := newAssignment(t)
	if err := s.SaveAssignment(a); err != nil {
		t.Fatal(err)
	}
	live := &Run{AssignmentID: a.ID, Trigger: "manual"}
	done := &Run{AssignmentID: a.ID, Trigger: "manual"}
	if err := s.StartRun(live); err != nil {
		t.Fatal(err)
	}
	if err := s.StartRun(done); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishRun(done.ID, RunOK, "fine", "", ""); err != nil {
		t.Fatal(err)
	}

	n, err := s.ReapRunningRuns("the daemon restarted")
	if err != nil || n != 1 {
		t.Fatalf("reaped %d, %v - want exactly the orphan", n, err)
	}
	runs, _ := s.RunsFor(a.ID, 0)
	for _, r := range runs {
		if r.State == RunRunning {
			t.Fatal("a run survived the reap still claiming to be running")
		}
		if r.ID == done.ID && r.State != RunOK {
			t.Fatal("the reap rewrote a run that had already finished")
		}
	}
}

func TestDeleteTakesTheRunHistoryWithIt(t *testing.T) {
	s := openTemp(t)
	a := newAssignment(t)
	if err := s.SaveAssignment(a); err != nil {
		t.Fatal(err)
	}
	if err := s.StartRun(&Run{AssignmentID: a.ID, Trigger: "manual"}); err != nil {
		t.Fatal(err)
	}

	gone, err := s.DeleteAssignment(a.ID)
	if err != nil || !gone {
		t.Fatalf("delete = %v, %v", gone, err)
	}
	if runs, _ := s.RunsFor(a.ID, 0); len(runs) != 0 {
		t.Errorf("%d runs outlived their assignment", len(runs))
	}
	if again, _ := s.DeleteAssignment(a.ID); again {
		t.Error("deleting a gone assignment reported a deletion")
	}
}

func TestIDsAreDistinctAndValidated(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		id := NewAssignmentID()
		if !ValidAssignmentID(id) {
			t.Fatalf("minted %q, which its own validator rejects", id)
		}
		if seen[id] {
			t.Fatalf("collision on %q", id)
		}
		seen[id] = true
	}
	for _, bad := range []string{"", "a", "aXYZ", "r000000000000", "a00000000000", "a0000000000000"} {
		if ValidAssignmentID(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}
