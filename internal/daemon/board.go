package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
	"github.com/borismilner/agentbox/internal/walkthrough"
)

// The board's store seam (webui.BoardStore), satisfied structurally the way
// Source is: the surface's keyhole verbs land here, the daemon owns policy
// and the log. Every write is write-through to SQLite - NFR7 says a daemon
// restart loses nothing the human did.

// BoardData hands the board everything it renders from.
func (d *Daemon) BoardData(id string) (store.Walkthrough, []store.Mark, []store.Comment, error) {
	w, err := d.st.GetWalkthrough(id)
	if err != nil {
		return store.Walkthrough{}, nil, nil, err
	}
	marks, err := d.st.MarksFor(id)
	if err != nil {
		return store.Walkthrough{}, nil, nil, err
	}
	comments, err := d.st.CommentsFor(id)
	if err != nil {
		return store.Walkthrough{}, nil, nil, err
	}
	return *w, marks, comments, nil
}

// BoardVerdict records a verdict, fingerprinting the spec step as judged so
// a later amendment under the mark is detectable rather than silent.
func (d *Daemon) BoardVerdict(id, stepID, verdict string) error {
	w, err := d.st.GetWalkthrough(id)
	if err != nil {
		return err
	}
	var spec walkthrough.Spec
	if err := json.Unmarshal([]byte(w.Spec), &spec); err != nil {
		return fmt.Errorf("stored spec does not parse: %w", err)
	}
	st := spec.Step(stepID)
	if st == nil {
		return fmt.Errorf("walkthrough %s has no step %q", id, stepID)
	}
	if err := d.st.SetVerdict(id, stepID, verdict, walkthrough.StepHash(st)); err != nil {
		return err
	}
	d.log.Debug("walkthrough.mark", "component", "daemon", "wt_id", id, "step", stepID, "verdict", verdict)
	return nil
}

func (d *Daemon) BoardNote(id, stepID, note string) error {
	return d.st.SetNote(id, stepID, note)
}

func (d *Daemon) BoardReveal(id, stepID string, revealed []int) error {
	return d.st.SetRevealed(id, stepID, revealed)
}

func (d *Daemon) BoardPos(id string, pos int) error {
	return d.st.SetPos(id, pos)
}

// BoardCommentAdd mints the comment id daemon-side; nothing ever awaits a
// comment, so nobody needs it before it exists. The body never reaches the
// log (NFR13: events, not content).
func (d *Daemon) BoardCommentAdd(id, stepID, path, side string, from, to int, exact, body string) (string, error) {
	cid := "c" + newID()[1:13]
	c := &store.Comment{
		ID: cid, StepID: stepID, Path: path, Side: side,
		FromLine: from, ToLine: to, Exact: exact, Body: body,
	}
	if err := d.st.AddComment(id, c); err != nil {
		return "", err
	}
	d.log.Debug("walkthrough.comment", "component", "daemon", "wt_id", id, "step", stepID,
		"path", path, "from", from, "to", to, "action", "add")
	return cid, nil
}

func (d *Daemon) BoardCommentEdit(id, commentID, body string) error {
	if err := d.st.EditComment(commentID, body); err != nil {
		return err
	}
	d.log.Debug("walkthrough.comment", "component", "daemon", "wt_id", id, "comment", commentID, "action", "edit")
	return nil
}

func (d *Daemon) BoardCommentDelete(id, commentID string) error {
	if err := d.st.DeleteComment(commentID); err != nil {
		return err
	}
	d.log.Debug("walkthrough.comment", "component", "daemon", "wt_id", id, "comment", commentID, "action", "delete")
	return nil
}

// BoardSubmit assembles the handback and records the submission. The gate
// (an unclear verdict without words) refuses here, daemon-side - the modal
// checks too, but Go is the side that must not be talked into anything.
// delivered reports whether a parked agent took the review in the same
// instant; false means it landed as submitted for a later pickup.
func (d *Daemon) BoardSubmit(id string) (delivered bool, atMS int64, err error) {
	st, rpcErr := d.walkthroughState(id)
	if rpcErr != nil {
		return false, 0, errors.New(rpcErr.Message)
	}
	var spec walkthrough.Spec
	if err := json.Unmarshal(st.Spec, &spec); err != nil {
		return false, 0, fmt.Errorf("stored spec does not parse: %w", err)
	}
	now := time.Now().UnixMilli()
	p, err := walkthrough.BuildPayload(walkthrough.Submission{
		ID: st.ID, Title: st.Title, RepoRoot: st.RepoRoot, Pinned: st.Pinned,
		SpecRev: st.Rev, NowMS: now,
	}, &spec, st.Marks, st.Comments)
	if err != nil {
		return false, 0, err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return false, 0, err
	}
	if err := d.st.SubmitWalkthrough(id, string(raw)); err != nil {
		return false, 0, err
	}
	d.log.Info("walkthrough.submitted", "component", "daemon", "wt_id", id,
		"understood", p.Tally.Understood, "unclear", p.Tally.Unclear,
		"not_reviewed", p.Tally.NotReviewed, "comments", p.Tally.Comments)
	return d.offerSubmission(id, string(raw)), now, nil
}

// BoardLibrary is the stored-review list the board's own library panel reads
// (FR70). The same rows the CLI's `walkthrough list` prints - one source, so
// the two doors cannot disagree about what exists.
func (d *Daemon) BoardLibrary() ([]proto.WalkthroughSummary, error) {
	rows, err := d.st.ListWalkthroughs("", "", 200)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []proto.WalkthroughSummary{}
	}
	return rows, nil
}

// BoardDelete removes a review and everything annotated on it. Same path as
// the RPC verb, including releasing an agent parked on it: deleting from the
// panel must not leave a session waiting forever for a submission that can
// no longer happen.
func (d *Daemon) BoardDelete(id string) (bool, error) {
	deleted, err := d.st.DeleteWalkthrough(id)
	if err != nil {
		return false, err
	}
	if deleted {
		d.releaseGone(id)
		d.log.Info("walkthrough.deleted", "component", "daemon", "wt_id", id, "via", "board")
	}
	return deleted, nil
}
