package store

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/borismilner/agentbox/internal/proto"
)

func testWalkthrough(id string) *Walkthrough {
	return &Walkthrough{
		ID:           id,
		Title:        "the twenty-fifth session",
		RepoRoot:     "/home/user/repo",
		PinnedSHA:    "dd375a3cb2c7",
		ChangeKey:    "/home/user/repo@..dd375a3cb2c7",
		Spec:         `{"version":1,"title":"t","steps":[]}`,
		Diff:         "diff --git a/f b/f\n",
		CountedSteps: 5,
		Identity:     proto.Identity{Agent: "claude-code", Project: "agentbox"},
	}
}

func TestWalkthroughRoundTrip(t *testing.T) {
	s := openTemp(t)
	w := testWalkthrough("w000000000001")
	if err := s.CreateWalkthrough(w); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetWalkthrough(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != w.Title || got.Spec != w.Spec || got.Diff != w.Diff ||
		got.State != WtOpen || got.SpecRev != 1 || got.CountedSteps != 5 ||
		got.Identity != w.Identity {
		t.Errorf("round trip mangled: %+v", got)
	}
	if _, err := s.GetWalkthrough("w0000000000ff"); !errors.Is(err, ErrWalkthroughNotFound) {
		t.Errorf("missing id: err = %v", err)
	}
}

func TestWalkthroughCreateIdempotent(t *testing.T) {
	s := openTemp(t)
	w := testWalkthrough("w000000000001")
	if err := s.CreateWalkthrough(w); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateWalkthrough(w); err != nil {
		t.Errorf("identical retry must succeed: %v", err)
	}
	other := testWalkthrough("w000000000001")
	other.Spec = `{"version":1,"title":"different","steps":[]}`
	if err := s.CreateWalkthrough(other); !errors.Is(err, ErrSpecMismatch) {
		t.Errorf("different spec on same id: err = %v", err)
	}
}

func TestMarksAndCommentsRoundTrip(t *testing.T) {
	s := openTemp(t)
	w := testWalkthrough("w000000000001")
	if err := s.CreateWalkthrough(w); err != nil {
		t.Fatal(err)
	}
	if err := s.SetVerdict(w.ID, "xkb", "understood", "hash1"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNote(w.ID, "xkb", "the guard is per press,\nnot per call"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevealed(w.ID, "xkb", []int{1}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetVerdict(w.ID, "titles", "unclear", "hash2"); err != nil {
		t.Fatal(err)
	}
	marks, err := s.MarksFor(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(marks) != 2 {
		t.Fatalf("marks = %d, want 2", len(marks))
	}
	xkb := marks[1] // ordered by step_id: titles, xkb
	if xkb.StepID != "xkb" || xkb.Verdict != "understood" || xkb.StepHash != "hash1" ||
		xkb.Note == "" || !reflect.DeepEqual(xkb.Revealed, []int{1}) {
		t.Errorf("xkb mark mangled: %+v", xkb)
	}

	c := &Comment{ID: "c000000000001", StepID: "xkb", Path: "internal/hand/xkb.go",
		FromLine: 134, ToLine: 134, Exact: "lockGroupUnchecked(0)", Body: "safe if the connection drops?"}
	if err := s.AddComment(w.ID, c); err != nil {
		t.Fatal(err)
	}
	if err := s.EditComment(c.ID, "still safe?"); err != nil {
		t.Fatal(err)
	}
	cs, err := s.CommentsFor(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].Body != "still safe?" || cs[0].Side != "new" || cs[0].FromLine != 134 {
		t.Errorf("comments mangled: %+v", cs)
	}
	if err := s.DeleteComment(c.ID); err != nil {
		t.Fatal(err)
	}
	if cs, _ = s.CommentsFor(w.ID); len(cs) != 0 {
		t.Errorf("comment survived deletion")
	}

	// Annotation writes on a missing walkthrough must refuse, not invent rows.
	if err := s.SetVerdict("w0000000000ff", "xkb", "understood", ""); !errors.Is(err, ErrWalkthroughNotFound) {
		t.Errorf("mark on missing walkthrough: err = %v", err)
	}
}

func TestWalkthroughSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentbox.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	w := testWalkthrough("w000000000001")
	if err := s.CreateWalkthrough(w); err != nil {
		t.Fatal(err)
	}
	if err := s.SetVerdict(w.ID, "xkb", "unclear", "h"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPos(w.ID, 3); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, err := s2.GetWalkthrough(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Pos != 3 {
		t.Errorf("pos = %d, want 3", got.Pos)
	}
	marks, err := s2.MarksFor(w.ID)
	if err != nil || len(marks) != 1 || marks[0].Verdict != "unclear" {
		t.Errorf("marks after reopen: %+v, %v", marks, err)
	}
}

func TestSubmitAndDeliverExactlyOnce(t *testing.T) {
	s := openTemp(t)
	w := testWalkthrough("w000000000001")
	if err := s.CreateWalkthrough(w); err != nil {
		t.Fatal(err)
	}
	if err := s.SubmitWalkthrough(w.ID, `{"version":1}`); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetWalkthrough(w.ID)
	if got.State != WtSubmitted || got.Payload == "" || got.SubmittedAt.IsZero() {
		t.Fatalf("after submit: %+v", got)
	}
	payload, err := s.DeliverWalkthrough(w.ID, "claude-code")
	if err != nil || payload != `{"version":1}` {
		t.Fatalf("deliver: %q, %v", payload, err)
	}
	if _, err := s.DeliverWalkthrough(w.ID, "claude-code"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delivery must lose: %v", err)
	}
	// The human says more after delivery: a resubmission is legal.
	if err := s.SubmitWalkthrough(w.ID, `{"version":2}`); err != nil {
		t.Fatal(err)
	}
	trail, err := s.WalkthroughTransitions(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"(new) -> open", "open -> submitted", "submitted -> delivered (delivered to claude-code)", "delivered -> submitted"}
	if !reflect.DeepEqual(trail, want) {
		t.Errorf("trail = %v, want %v", trail, want)
	}
}

func TestListWalkthroughs(t *testing.T) {
	s := openTemp(t)
	a := testWalkthrough("w000000000001")
	b := testWalkthrough("w000000000002")
	b.Title = "the viewer rework"
	b.Spec = `{"version":1,"steps":[{"id":"v","code":[{"path":"internal/webui/viewer.go"}]}]}`
	if err := s.CreateWalkthrough(a); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateWalkthrough(b); err != nil {
		t.Fatal(err)
	}
	if err := s.SetVerdict(b.ID, "v", "unclear", "h"); err != nil {
		t.Fatal(err)
	}

	all, err := s.ListWalkthroughs("", "", 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("list all: %+v, %v", all, err)
	}
	if all[0].ID != b.ID {
		t.Errorf("most recently touched first: got %s", all[0].ID)
	}
	if all[0].Unclear != 1 || all[0].CountedSteps != 5 {
		t.Errorf("counts: %+v", all[0])
	}

	byPath, err := s.ListWalkthroughs("viewer.go", "", 0)
	if err != nil || len(byPath) != 1 || byPath[0].ID != b.ID {
		t.Errorf("search by cited path: %+v, %v", byPath, err)
	}
	byState, err := s.ListWalkthroughs("", WtOpen, 0)
	if err != nil || len(byState) != 2 {
		t.Errorf("filter by state: %+v, %v", byState, err)
	}
}

func TestDeleteWalkthroughCascades(t *testing.T) {
	s := openTemp(t)
	w := testWalkthrough("w000000000001")
	if err := s.CreateWalkthrough(w); err != nil {
		t.Fatal(err)
	}
	if err := s.SetVerdict(w.ID, "xkb", "understood", "h"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddComment(w.ID, &Comment{ID: "c000000000001", StepID: "xkb", Body: "x"}); err != nil {
		t.Fatal(err)
	}
	deleted, err := s.DeleteWalkthrough(w.ID)
	if err != nil || !deleted {
		t.Fatalf("delete: %v %v", deleted, err)
	}
	if marks, _ := s.MarksFor(w.ID); len(marks) != 0 {
		t.Error("marks survived the cascade")
	}
	if cs, _ := s.CommentsFor(w.ID); len(cs) != 0 {
		t.Error("comments survived the cascade")
	}
	if deleted, _ = s.DeleteWalkthrough(w.ID); deleted {
		t.Error("second delete reported success")
	}
}
