package daemon

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/borismilner/agentbox/internal/assign"
	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
)

// The assignment RPC surface (M12/FR82). assignments.go owns WHEN a thing runs;
// this file owns the six methods an agent or the panel reaches it through.
//
// It is a CRUD an agent uses as much as a human does - the agent is meant to
// author assignments with the user - so every refusal here is written to
// teach: the whole list of spec faults at once rather than the first, and a
// clear line between what was refused (a spec that would render wrong) and what
// was merely reported (a prompt still half written).

// runsInRead is how much history a read carries when the caller does not say.
// Enough to see whether the last few went well, not enough to bury the
// definition the caller actually asked for.
const runsInRead = 5

func (d *Daemon) assignmentList() (any, *proto.RPCError) {
	res, rpcErr := d.AssignmentList()
	if rpcErr != nil {
		return nil, rpcErr
	}
	return res, nil
}

// AssignmentList is the whole list with each one's last run folded in. Exported
// for the panel, which opens on exactly this.
func (d *Daemon) AssignmentList() (proto.AssignmentListResult, *proto.RPCError) {
	list, err := d.st.ListAssignments()
	if err != nil {
		return proto.AssignmentListResult{}, internalErr(err)
	}
	out := proto.AssignmentListResult{Assignments: make([]proto.AssignmentSummary, 0, len(list))}
	for _, a := range list {
		row := proto.AssignmentSummary{
			ID: a.ID, Name: a.Name, Description: a.Description,
			Kind: assign.Kind(a.Schedule), Schedule: a.Schedule, Enabled: a.Enabled,
			Model: a.Model, Dir: a.Dir, Running: d.AssignmentRunning(a.ID),
			LastRunMS: a.LastRunMS, NextRunMS: a.NextRunMS,
		}
		// One query per row. The list is a handful of assignments a person
		// maintains by hand, and the alternative (a join with a window
		// function) buys nothing at that size.
		if runs, err := d.st.RunsFor(a.ID, 1); err == nil && len(runs) > 0 {
			row.LastState, row.LastSummary = runs[0].State, runs[0].Summary
			if runs[0].Error != "" && runs[0].Summary == "" {
				row.LastSummary = runs[0].Error
			}
		}
		out.Assignments = append(out.Assignments, row)
	}
	return out, nil
}

func (d *Daemon) assignmentRead(params json.RawMessage) (any, *proto.RPCError) {
	var req proto.AssignmentRead
	if err := json.Unmarshal(params, &req); err != nil || req.ID == "" {
		return nil, invalid(`assignment_read wants {"id", "runs"?}`)
	}
	res, rpcErr := d.AssignmentRead(req)
	if rpcErr != nil {
		return nil, rpcErr
	}
	return res, nil
}

// AssignmentRead is one definition with agentbox's diagnosis of it and its recent
// runs. Exported for the panel: the editor shows the same warnings an agent
// gets, from the same arithmetic.
func (d *Daemon) AssignmentRead(req proto.AssignmentRead) (proto.AssignmentReadResult, *proto.RPCError) {
	a, err := d.st.GetAssignment(req.ID)
	if err != nil {
		return proto.AssignmentReadResult{}, internalErr(err)
	}
	if a == nil {
		return proto.AssignmentReadResult{}, invalid("no assignment %q; assignment_list has the ids", req.ID)
	}
	limit := req.Runs
	if limit == 0 {
		limit = runsInRead
	}
	res := proto.AssignmentReadResult{
		Assignment: a, Kind: assign.Kind(a.Schedule), Running: d.AssignmentRunning(a.ID),
		Placeholders: assign.Placeholders(a.Prompt),
		Problems:     assign.Validate(a.Spec),
	}
	// mergeParams, not assign.Merge: an assignment with no declared spec keeps
	// its values, and merging them through an empty one would report every
	// placeholder as unfilled while the run substitutes them perfectly well.
	_, res.Unfilled = assign.Render(a.Prompt, mergeParams(a.Spec, a.Params, nil))
	res.Unused = unusedKnobs(a.Spec, a.Prompt)
	if limit > 0 {
		runs, err := d.st.RunsFor(a.ID, limit)
		if err != nil {
			return proto.AssignmentReadResult{}, internalErr(err)
		}
		res.Runs = wireRuns(runs)
	}
	return res, nil
}

func (d *Daemon) assignmentSave(params json.RawMessage) (any, *proto.RPCError) {
	var req proto.AssignmentSave
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, invalid(`assignment_save wants {"id"?, "name"?, "prompt"?, "spec"?, "params"?, "schedule"?, ...}: %s`, err)
	}
	return d.AssignmentSave(req)
}

// AssignmentSave is create and update in one method, because they differ only
// in whether an id came in. Every field is optional: an agent improving a
// prompt sends a prompt, and everything it did not mention stays as the human
// left it.
//
// Exported because the panel goes through exactly this, not around it. An
// editor with its own idea of what a valid schedule is would be a second
// answer to the same question, and the two would drift.
func (d *Daemon) AssignmentSave(req proto.AssignmentSave) (proto.AssignmentSaveResult, *proto.RPCError) {
	a, created, rpcErr := d.loadForSave(req.ID)
	if rpcErr != nil {
		return proto.AssignmentSaveResult{}, rpcErr
	}
	setString(&a.Name, req.Name)
	setString(&a.Description, req.Description)
	setString(&a.Prompt, req.Prompt)
	setString(&a.PanelHTML, req.PanelHTML)
	setString(&a.Model, req.Model)
	setString(&a.Mode, req.Mode)
	setString(&a.Dir, req.Dir)
	setString(&a.Schedule, req.Schedule)
	if req.Spec != nil {
		a.Spec = *req.Spec
	}
	if req.Enabled != nil {
		a.Enabled = *req.Enabled
	}

	if strings.TrimSpace(a.Name) == "" {
		return proto.AssignmentSaveResult{}, invalid("an assignment needs a name; it is how a human finds it again")
	}
	if strings.TrimSpace(a.Prompt) == "" {
		return proto.AssignmentSaveResult{}, invalid("an assignment needs a prompt; it is the whole of what the agent is asked to do")
	}
	if a.Mode != "" && a.Mode != "plan" && a.Mode != "full" {
		return proto.AssignmentSaveResult{}, invalid("mode %q is not plan or full", a.Mode)
	}
	// A spec fault and an unparseable schedule are refused rather than warned
	// about: both would make the assignment behave differently from how it
	// reads, and an agent writing one can fix them in the same breath.
	if probs := assign.Validate(a.Spec); len(probs) > 0 {
		return proto.AssignmentSaveResult{}, invalid("the parameter spec has %d %s: %s",
			len(probs), plural(len(probs), "problem"), strings.Join(probs, "; "))
	}
	if _, err := assign.ParseSchedule(a.Schedule); err != nil {
		return proto.AssignmentSaveResult{}, invalid("%s", err)
	}

	dropped := droppedKeys(a.Spec, req.Params)
	a.Params = mergeParams(a.Spec, a.Params, req.Params)
	if err := d.SaveAssignment(a); err != nil {
		return proto.AssignmentSaveResult{}, internalErr(err)
	}
	return proto.AssignmentSaveResult{
		ID: a.ID, Created: created, Kind: assign.Kind(a.Schedule), NextRunMS: a.NextRunMS,
		Warnings: saveWarnings(a, dropped), Assignment: a,
	}, nil
}

// loadForSave is the definition a save starts from: a fresh one for a create,
// the stored one for an update. A new assignment arrives enabled, because the
// gesture that creates a scheduled thing is the gesture that wants it to run;
// pass enabled=false to author one quietly.
func (d *Daemon) loadForSave(id string) (*assign.Assignment, bool, *proto.RPCError) {
	if id == "" {
		return &assign.Assignment{ID: store.NewAssignmentID(), Enabled: true}, true, nil
	}
	a, err := d.st.GetAssignment(id)
	if err != nil {
		return nil, false, internalErr(err)
	}
	if a == nil {
		return nil, false, invalid("no assignment %q; omit the id to create one", id)
	}
	return a, false, nil
}

func (d *Daemon) assignmentDelete(params json.RawMessage) (any, *proto.RPCError) {
	var req proto.AssignmentDelete
	if err := json.Unmarshal(params, &req); err != nil || req.ID == "" {
		return nil, invalid(`assignment_delete wants {"id"}`)
	}
	deleted, err := d.DeleteAssignment(req.ID)
	if err != nil {
		return nil, internalErr(err)
	}
	return proto.AssignmentDeleteResult{Deleted: deleted}, nil
}

func (d *Daemon) assignmentRun(params json.RawMessage) (any, *proto.RPCError) {
	var req proto.AssignmentRunNow
	if err := json.Unmarshal(params, &req); err != nil || req.ID == "" {
		return nil, invalid(`assignment_run wants {"id", "overrides"?}`)
	}
	trigger := req.Trigger
	if trigger == "" {
		trigger = "agent"
	}
	runID, err := d.RunAssignmentNow(req.ID, trigger, req.Overrides)
	if err != nil {
		// Every way this refuses is the caller's to act on: no such
		// assignment, one already running, or a daemon with nothing to run it
		// with. None of them is an internal fault.
		return nil, invalid("%s", err)
	}
	return proto.AssignmentRunNowResult{RunID: runID}, nil
}

func (d *Daemon) assignmentRuns(params json.RawMessage) (any, *proto.RPCError) {
	var req proto.AssignmentRuns
	if err := json.Unmarshal(params, &req); err != nil || req.ID == "" {
		return nil, invalid(`assignment_runs wants {"id", "limit"?}`)
	}
	runs, err := d.AssignmentRuns(req.ID, req.Limit)
	if err != nil {
		return nil, internalErr(err)
	}
	return proto.AssignmentRunsResult{Runs: wireRuns(runs)}, nil
}

func internalErr(err error) *proto.RPCError {
	return &proto.RPCError{Code: proto.CodeInternal, Message: err.Error()}
}

func setString(dst *string, v *string) {
	if v != nil {
		*dst = *v
	}
}

func wireRuns(runs []store.Run) []proto.AssignmentRun {
	out := make([]proto.AssignmentRun, 0, len(runs))
	for _, r := range runs {
		out = append(out, proto.AssignmentRun{
			ID: r.ID, StartedMS: r.StartedMS, EndedMS: r.EndedMS, State: r.State,
			Trigger: r.Trigger, Params: r.Params, Summary: r.Summary, Error: r.Error,
			SessionID: r.SessionID, Data: r.Data,
		})
	}
	return out
}

// mergeParams folds the values a save carries over the stored ones, then over
// the spec.
//
// The no-spec case is not the same as an empty spec: an assignment whose knobs
// were never declared still substitutes {{key}} from its values, and running
// them through assign.Merge would erase every one of them - so a caller that
// sets a value on an undeclared knob would watch it vanish. Nothing declares
// what a knob is there, so nothing can declare a value stale either.
func mergeParams(spec []assign.Param, have, set map[string]any) map[string]any {
	out := map[string]any{}
	maps.Copy(out, have)
	maps.Copy(out, set)
	if len(spec) == 0 {
		return out
	}
	return assign.Merge(spec, out)
}

// droppedKeys are the values a caller sent that the spec has no knob for, and
// which the merge above therefore discards. Silently losing them is how an
// agent ends up debugging a parameter it believes it set.
func droppedKeys(spec []assign.Param, set map[string]any) []string {
	if len(spec) == 0 || len(set) == 0 {
		return nil
	}
	known := map[string]bool{}
	for _, p := range spec {
		if p.Input() && p.Key != "" {
			known[p.Key] = true
		}
	}
	var out []string
	for k := range set {
		if !known[k] {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

// unusedKnobs are the parameters no placeholder reads. A knob the prompt never
// mentions is a control that does nothing, which is worse than a missing one:
// the human turns it and the run does not change.
func unusedKnobs(spec []assign.Param, prompt string) []string {
	if len(spec) == 0 {
		return nil
	}
	used := map[string]bool{}
	for _, k := range assign.Placeholders(prompt) {
		used[k] = true
	}
	var out []string
	for _, p := range spec {
		if p.Input() && p.Key != "" && !used[p.Key] {
			out = append(out, p.Key)
		}
	}
	return out
}

// saveWarnings is everything agentbox noticed that did not stop the save. An
// assignment mid-authoring is allowed to be incomplete; it is not allowed to be
// incomplete quietly.
func saveWarnings(a *assign.Assignment, dropped []string) []string {
	var warns []string
	if _, missing := assign.Render(a.Prompt, a.Params); len(missing) > 0 {
		warns = append(warns, "the prompt refers to {{"+strings.Join(missing, "}}, {{")+
			"}} and no parameter fills it; the run would see the placeholder verbatim")
	}
	if unused := unusedKnobs(a.Spec, a.Prompt); len(unused) > 0 {
		warns = append(warns, "the prompt never uses "+strings.Join(unused, ", ")+
			", so turning that knob changes nothing")
	}
	if len(dropped) > 0 {
		warns = append(warns, "dropped "+strings.Join(dropped, ", ")+
			": the spec has no knob by that key")
	}
	// The rule that keeps the escape hatch from being a trapdoor
	// (docs/08-assignments.md): a custom panel that throws on load must never
	// be the only way in.
	if strings.TrimSpace(a.PanelHTML) != "" && len(a.Spec) == 0 {
		warns = append(warns, "a custom panel with no typed spec behind it: if the panel fails to load "+
			"there is nothing left to edit the values with")
	}
	return warns
}
