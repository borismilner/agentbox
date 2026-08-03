package daemon

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
	"github.com/borismilner/agentbox/internal/walkthrough"
)

// The walkthrough handlers (FR58/FR59). A walkthrough is not a queue item:
// like a progress report, a review must never wait behind a question, so it
// bypasses the card queue entirely - its own store tables, its own window.
// The blocking await lives in wthub.go; amend is a guard until the
// amendment round builds the real thing.

func invalid(format string, args ...any) *proto.RPCError {
	return &proto.RPCError{Code: proto.CodeInvalidParams, Message: fmt.Sprintf(format, args...)}
}

func (d *Daemon) walkthroughCreate(params json.RawMessage) (any, *proto.RPCError) {
	var req proto.WalkthroughCreate
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, invalid(`walkthrough_create wants {"id", "spec", "no_show"?, "identity"}`)
	}
	if !proto.ValidWalkthroughID(req.ID) {
		return nil, invalid("walkthrough id %q must be caller-minted: w plus 12 hex (proto.NewWalkthroughID)", req.ID)
	}
	spec, warnings, err := walkthrough.Parse(req.Spec)
	if err != nil {
		return nil, invalid("%s", err)
	}
	diff := spec.Diff
	spec.Diff = "" // the diff lives in its own column; the spec stays readable
	specJSON, err := json.Marshal(spec)
	if err != nil {
		return nil, &proto.RPCError{Code: proto.CodeInternal, Message: err.Error()}
	}
	w := &store.Walkthrough{
		ID:           req.ID,
		Title:        spec.Title,
		RepoRoot:     spec.RepoRoot,
		PinnedSHA:    spec.Pinned,
		BaseSHA:      spec.Base,
		ChangeKey:    spec.RepoRoot + "@" + spec.Base + ".." + spec.Pinned,
		Spec:         string(specJSON),
		Diff:         diff,
		CountedSteps: spec.CountedSteps(),
		Identity:     req.Identity,
	}
	if err := d.st.CreateWalkthrough(w); err != nil {
		if errors.Is(err, store.ErrSpecMismatch) {
			return nil, invalid("%s - mint a fresh id for new content", err)
		}
		return nil, &proto.RPCError{Code: proto.CodeInternal, Message: err.Error()}
	}
	// Capture what it cites, now, from the tree the authoring agent read. A
	// review that keeps only line numbers is true only until the next
	// checkout; this is what makes it readable afterwards (wtcapture.go).
	// Misses are warnings, never a failed create: a walk over work that is not
	// all on disk is still worth having, and the board still falls back to
	// reading the file.
	cites := spec.Citations()
	if len(cites) > 0 {
		ex, missed := captureFromWorktree(spec.RepoRoot, cites)
		if err := d.st.SaveExcerpts(req.ID, ex); err != nil {
			d.log.Warn("walkthrough.capture_failed", "component", "daemon", "wt_id", req.ID, "err", err.Error())
		}
		for _, m := range missed {
			warnings = append(warnings, "could not capture "+m+
				" - that block will be read from the working tree, so it goes stale when the file does")
		}
		d.log.Info("walkthrough.captured", "component", "daemon", "wt_id", req.ID,
			"ranges", len(ex), "cited", len(cites), "missed", len(missed))
	}
	d.log.Info("walkthrough.created", "component", "daemon", "wt_id", req.ID,
		"agent", req.Identity.Agent, "steps", len(spec.Steps), "counted", w.CountedSteps,
		"spec_bytes", len(specJSON), "diff_bytes", len(diff))
	if !req.NoShow {
		d.ui.ShowBoard(req.ID)
	}
	return proto.WalkthroughCreateResult{
		ID:  req.ID,
		Rev: 1,
		// Coverage arithmetic lands with the drift slice; until then it is
		// reported uncomputed, never guessed (FR61).
		Coverage: proto.CoverageReport{UncoveredHunks: []proto.HunkRef{}},
		Warnings: warnings,
	}, nil
}

func (d *Daemon) walkthroughRead(params json.RawMessage) (any, *proto.RPCError) {
	var req proto.WalkthroughRead
	if err := json.Unmarshal(params, &req); err != nil || req.ID == "" {
		return nil, invalid(`walkthrough_read wants {"id", "ack"?}`)
	}
	if req.Ack {
		if _, err := d.st.DeliverWalkthrough(req.ID, "read"); err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, &proto.RPCError{Code: proto.CodeInternal, Message: err.Error()}
		}
		// ErrNotFound here means nothing was waiting - ack is best-effort
		// by design, so a re-read after a lost race stays cheap.
	}
	st, rpcErr := d.walkthroughState(req.ID)
	if rpcErr != nil {
		return nil, rpcErr
	}
	return st, nil
}

// walkthroughState assembles the full picture: the agent's half, the
// human's half, and the last submission.
func (d *Daemon) walkthroughState(id string) (*proto.WalkthroughState, *proto.RPCError) {
	w, err := d.st.GetWalkthrough(id)
	if err != nil {
		if errors.Is(err, store.ErrWalkthroughNotFound) {
			return nil, &proto.RPCError{Code: proto.CodeItemNotFound, Message: "no walkthrough " + id}
		}
		return nil, &proto.RPCError{Code: proto.CodeInternal, Message: err.Error()}
	}
	marks, err := d.st.MarksFor(id)
	if err != nil {
		return nil, &proto.RPCError{Code: proto.CodeInternal, Message: err.Error()}
	}
	comments, err := d.st.CommentsFor(id)
	if err != nil {
		return nil, &proto.RPCError{Code: proto.CodeInternal, Message: err.Error()}
	}
	out := &proto.WalkthroughState{
		ID:          w.ID,
		Title:       w.Title,
		RepoRoot:    w.RepoRoot,
		Pinned:      w.PinnedSHA,
		Base:        w.BaseSHA,
		State:       w.State,
		Rev:         w.SpecRev,
		Spec:        json.RawMessage(w.Spec),
		Marks:       []proto.WalkthroughMark{},
		Comments:    []proto.WalkthroughComment{},
		Coverage:    proto.CoverageReport{UncoveredHunks: []proto.HunkRef{}},
		CreatedAtMS: w.CreatedAt.UnixMilli(),
		UpdatedAtMS: w.UpdatedAt.UnixMilli(),
	}
	if !w.SubmittedAt.IsZero() {
		out.SubmittedAtMS = w.SubmittedAt.UnixMilli()
	}
	if w.Payload != "" {
		out.Payload = json.RawMessage(w.Payload)
	}
	for _, m := range marks {
		out.Marks = append(out.Marks, proto.WalkthroughMark{
			StepID: m.StepID, Verdict: m.Verdict, Note: m.Note,
			Revealed: m.Revealed, CmdRuns: m.CmdRuns, Stale: m.Stale,
		})
	}
	for _, c := range comments {
		out.Comments = append(out.Comments, proto.WalkthroughComment{
			ID: c.ID, StepID: c.StepID, Path: c.Path, Side: c.Side,
			FromLine: c.FromLine, ToLine: c.ToLine, Exact: c.Exact,
			Body: c.Body, Adrift: c.Adrift, AtMS: c.CreatedAt.UnixMilli(),
		})
	}
	return out, nil
}

// walkthroughRepair recovers the source that reviews written before capture
// existed never kept. One id, or every walkthrough when none is named - which
// is the usual call, because they all have the same hole.
func (d *Daemon) walkthroughRepair(params json.RawMessage) (any, *proto.RPCError) {
	var req proto.WalkthroughRepair
	if len(params) > 0 {
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, invalid(`walkthrough_repair wants {"id"?} - omit the id to repair every walkthrough`)
		}
	}
	var ids []string
	if req.ID != "" {
		ids = []string{req.ID}
	} else {
		rows, err := d.st.ListWalkthroughs("", "", 1000)
		if err != nil {
			return nil, &proto.RPCError{Code: proto.CodeInternal, Message: err.Error()}
		}
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
	}
	out := proto.WalkthroughRepairResult{Repaired: []proto.WalkthroughRepairRow{}}
	for _, id := range ids {
		w, err := d.st.GetWalkthrough(id)
		if err != nil {
			if req.ID != "" {
				return nil, invalid("%s", err)
			}
			continue // a walkthrough deleted between the list and here is not an error
		}
		row, err := d.repairOne(w)
		if err != nil {
			row.Notes = append(row.Notes, err.Error())
		}
		// Silence for the ones that needed nothing: a repair run over a whole
		// library should say what it changed, not list everything it looked at.
		if row.Recovered > 0 || row.Missing > 0 || len(row.Notes) > 0 {
			out.Repaired = append(out.Repaired, row)
		}
		if row.Recovered > 0 {
			d.log.Info("walkthrough.repaired", "component", "daemon", "wt_id", id,
				"recovered", row.Recovered, "missing", row.Missing)
		}
	}
	return out, nil
}

func (d *Daemon) walkthroughList(params json.RawMessage) (any, *proto.RPCError) {
	var req proto.WalkthroughList
	if len(params) > 0 {
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, invalid(`walkthrough_list wants {"query"?, "state"?, "limit"?}`)
		}
	}
	rows, err := d.st.ListWalkthroughs(req.Query, req.State, req.Limit)
	if err != nil {
		return nil, &proto.RPCError{Code: proto.CodeInternal, Message: err.Error()}
	}
	if rows == nil {
		rows = []proto.WalkthroughSummary{}
	}
	return proto.WalkthroughListResult{Walkthroughs: rows}, nil
}

func (d *Daemon) walkthroughDelete(params json.RawMessage) (any, *proto.RPCError) {
	var req proto.WalkthroughDelete
	if err := json.Unmarshal(params, &req); err != nil || req.ID == "" {
		return nil, invalid(`walkthrough_delete wants {"id"}`)
	}
	deleted, err := d.st.DeleteWalkthrough(req.ID)
	if err != nil {
		return nil, &proto.RPCError{Code: proto.CodeInternal, Message: err.Error()}
	}
	if deleted {
		d.releaseGone(req.ID)
		d.log.Info("walkthrough.deleted", "component", "daemon", "wt_id", req.ID)
	}
	return proto.WalkthroughDeleteResult{Deleted: deleted}, nil
}

// walkthroughAmend is a guard, not yet an editor. The revision protocol is
// designed (ops by step id, expect_rev, stale flagging, the orphan shelf)
// but its board UX gets its own mock round before it ships, so this build
// refuses with directions. The submitted case is the one rule that must
// hold today: an unread handback is never overwritten.
func (d *Daemon) walkthroughAmend(params json.RawMessage) (any, *proto.RPCError) {
	var req proto.WalkthroughAmend
	if err := json.Unmarshal(params, &req); err != nil || req.ID == "" {
		return nil, invalid(`walkthrough_amend wants {"id", "expect_rev", "ops"}`)
	}
	w, err := d.st.GetWalkthrough(req.ID)
	if err != nil {
		if errors.Is(err, store.ErrWalkthroughNotFound) {
			return nil, &proto.RPCError{Code: proto.CodeItemNotFound, Message: "no walkthrough " + req.ID}
		}
		return nil, &proto.RPCError{Code: proto.CodeInternal, Message: err.Error()}
	}
	if w.State == store.WtSubmitted {
		return nil, invalid("walkthrough %s is submitted and unread - amending now would overwrite a handback nobody has seen; take the review first (await_walkthrough, or read with ack)", req.ID)
	}
	return nil, invalid("amendment is not in this build yet - create a fresh walkthrough for the revised content; %s keeps its marks and stays in the library", req.ID)
}

func (d *Daemon) walkthroughOpen(params json.RawMessage) (any, *proto.RPCError) {
	var req proto.WalkthroughOpen
	if err := json.Unmarshal(params, &req); err != nil || req.ID == "" {
		return nil, invalid(`walkthrough_open wants {"id"}`)
	}
	if _, err := d.st.GetWalkthrough(req.ID); err != nil {
		if errors.Is(err, store.ErrWalkthroughNotFound) {
			return nil, &proto.RPCError{Code: proto.CodeItemNotFound, Message: "no walkthrough " + req.ID}
		}
		return nil, &proto.RPCError{Code: proto.CodeInternal, Message: err.Error()}
	}
	d.ui.ShowBoard(req.ID)
	d.log.Info("walkthrough.opened", "component", "daemon", "wt_id", req.ID)
	return map[string]bool{"ok": true}, nil
}
