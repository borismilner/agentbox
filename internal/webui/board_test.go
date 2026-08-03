package webui

import (
	"errors"
	"strings"
	"testing"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
	"github.com/borismilner/agentbox/internal/walkthrough"
)

// fakeBoardStore records verbs; the wire snapshot renders from its data.
type fakeBoardStore struct {
	w         store.Walkthrough
	marks     []store.Mark
	comments  []store.Comment
	calls     []string
	deleted   []string
	submitErr error // what BoardSubmit returns; nil delivers
}

func (f *fakeBoardStore) BoardLibrary() ([]proto.WalkthroughSummary, error) {
	return []proto.WalkthroughSummary{{ID: f.w.ID, Title: f.w.Title, RepoRoot: f.w.RepoRoot}}, nil
}

func (f *fakeBoardStore) BoardDelete(id string) (bool, error) {
	f.deleted = append(f.deleted, id)
	return true, nil
}

func (f *fakeBoardStore) BoardData(id string) (store.Walkthrough, []store.Mark, []store.Comment, error) {
	return f.w, f.marks, f.comments, nil
}
func (f *fakeBoardStore) BoardVerdict(id, stepID, verdict string) error {
	f.calls = append(f.calls, "verdict:"+stepID+":"+verdict)
	return nil
}
func (f *fakeBoardStore) BoardNote(id, stepID, note string) error {
	f.calls = append(f.calls, "note:"+stepID)
	return nil
}
func (f *fakeBoardStore) BoardReveal(id, stepID string, revealed []int) error {
	f.calls = append(f.calls, "reveal:"+stepID)
	return nil
}
func (f *fakeBoardStore) BoardPos(id string, pos int) error {
	f.calls = append(f.calls, "pos")
	return nil
}
func (f *fakeBoardStore) BoardCommentAdd(id, stepID, path, side string, from, to int, exact, body string) (string, error) {
	f.calls = append(f.calls, "comment_add:"+stepID+":"+side+":"+exact)
	return "c000000000001", nil
}
func (f *fakeBoardStore) BoardCommentEdit(id, commentID, body string) error {
	f.calls = append(f.calls, "comment_edit:"+commentID)
	return nil
}
func (f *fakeBoardStore) BoardCommentDelete(id, commentID string) error {
	f.calls = append(f.calls, "comment_delete:"+commentID)
	return nil
}
func (f *fakeBoardStore) BoardSubmit(id string) (bool, int64, error) {
	f.calls = append(f.calls, "submit:"+id)
	if f.submitErr != nil {
		return false, 0, f.submitErr
	}
	return true, 1700000000000, nil
}

func boardTestUI(t *testing.T) (*UI, *fakeBoardStore) {
	t.Helper()
	u := confUI()
	root := fixtureRepo(t)
	f := &fakeBoardStore{w: store.Walkthrough{
		ID: "w000000000001", Title: "the session", RepoRoot: root,
		PinnedSHA: "dd375a3cb2c7", State: store.WtOpen, SpecRev: 1, Pos: 0,
		Spec: fixtureSpec(t, nil), Diff: fixtureDiff,
	}, marks: []store.Mark{{StepID: "s1", Verdict: "unclear", Note: "why?"}}}
	u.SetBoardStore(f)
	u.ShowBoard("w000000000001") // no app behind confUI: targets the id, opens nothing
	return u, f
}

func TestBridgeBoardSnapshot(t *testing.T) {
	u, _ := boardTestUI(t)
	br := &Bridge{ui: u}
	wb, err := br.Board()
	if err != nil {
		t.Fatal(err)
	}
	if wb.ID != "w000000000001" || wb.Pinned != "dd375a3cb2c7" || len(wb.Steps) != 1 {
		t.Errorf("snapshot: %+v", wb)
	}
	if wb.Root == "" || wb.Repo == "" {
		t.Error("root/repo missing from the wire")
	}
	if m := wb.Marks["s1"]; m.Verdict != "unclear" || m.Note != "why?" {
		t.Errorf("marks lost: %+v", wb.Marks)
	}
	if len(wb.Steps[0].Codes[0].Lines) == 0 {
		t.Error("code lines missing")
	}
}

func TestBridgeBoardVerbsValidate(t *testing.T) {
	u, f := boardTestUI(t)
	br := &Bridge{ui: u}

	if err := br.BoardVerdict("w000000000001", "s1", "maybe"); err == nil {
		t.Error("bad verdict accepted")
	}
	if err := br.BoardVerdict("w000000000001", "s1", "understood"); err != nil {
		t.Error(err)
	}
	if err := br.BoardNote("w000000000001", "s1", strings.Repeat("x", boardNoteMax+1)); err == nil {
		t.Error("oversize note accepted")
	}
	if _, err := br.BoardCommentAdd("w000000000001", "s1", "p", "sideways", 1, 2, "e", "b"); err == nil {
		t.Error("bad side accepted")
	}
	if _, err := br.BoardCommentAdd("w000000000001", "s1", "p", "new", 5, 2, "e", "b"); err == nil {
		t.Error("inverted range accepted")
	}
	if _, err := br.BoardCommentAdd("w000000000001", "s1", "p", "new", 1, 2, "e", "  "); err == nil {
		t.Error("empty comment accepted")
	}
	long := strings.Repeat("s", boardExactMax+50)
	if _, err := br.BoardCommentAdd("w000000000001", "s1", "p", "new", 1, 2, long, "words"); err != nil {
		t.Error(err)
	}
	// Only the good writes reached the store, and the oversized exact was
	// clipped rather than refused - the selection is context, not content.
	want := []string{"verdict:s1:understood", "comment_add:s1:new:" + long[:boardExactMax]}
	if len(f.calls) != len(want) || f.calls[0] != want[0] || f.calls[1] != want[1] {
		t.Errorf("store calls = %v", f.calls)
	}
}

// TestBridgeBoardSubmit pins the receipt mapping: a clean submit reports
// delivered/at, a gate refusal becomes a jump target rather than an error,
// and any other failure stays an error.
func TestBridgeBoardSubmit(t *testing.T) {
	u, f := boardTestUI(t)
	br := &Bridge{ui: u}

	r, err := br.BoardSubmit("w000000000001")
	if err != nil || !r.Delivered || r.AtMS == 0 || r.Gate != "" {
		t.Fatalf("clean submit: %+v %v", r, err)
	}

	f.submitErr = &walkthrough.GateError{StepID: "s1", Title: "The guard"}
	r, err = br.BoardSubmit("w000000000001")
	if err != nil {
		t.Fatalf("a gate refusal must not surface as an error: %v", err)
	}
	if r.Gate != "s1" || r.GateMsg == "" || r.Delivered {
		t.Errorf("gate receipt: %+v", r)
	}

	f.submitErr = errors.New("store on fire")
	if _, err = br.BoardSubmit("w000000000001"); err == nil {
		t.Error("a real failure must stay an error")
	}
}

func TestShowBoardRetargets(t *testing.T) {
	u, _ := boardTestUI(t)
	u.ShowBoard("w000000000002")
	u.board.mu.Lock()
	defer u.board.mu.Unlock()
	if u.board.id != "w000000000002" {
		t.Errorf("board id = %s", u.board.id)
	}
}
