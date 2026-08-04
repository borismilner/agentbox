package daemon

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/borismilner/agentbox/internal/assign"
	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
)

// The assignment CRUD as an agent reaches it. What these pin is the half a
// scheduler test cannot: that an agent editing an assignment cannot destroy the
// human's settings by omission, and that everything it gets wrong comes back as
// a sentence it can act on.

func saveCall(t *testing.T, d *Daemon, req proto.AssignmentSave) (proto.AssignmentSaveResult, *proto.RPCError) {
	t.Helper()
	res, rpcErr := d.assignmentSave(mustJSON(t, req))
	if rpcErr != nil {
		return proto.AssignmentSaveResult{}, rpcErr
	}
	out, ok := res.(proto.AssignmentSaveResult)
	if !ok {
		t.Fatalf("save returned %T", res)
	}
	return out, nil
}

func mustSave(t *testing.T, d *Daemon, req proto.AssignmentSave) proto.AssignmentSaveResult {
	t.Helper()
	out, rpcErr := saveCall(t, d, req)
	if rpcErr != nil {
		t.Fatalf("save: %s", rpcErr.Message)
	}
	return out
}

func usageWatch() proto.AssignmentSave {
	return proto.AssignmentSave{
		Name:     new("Usage watch"),
		Prompt:   new("Check Claude usage over {{window}} and warn above {{threshold}}%."),
		Schedule: new("daily 09:00"),
		Model:    new("claude-opus-5"),
		Spec: &[]assign.Param{
			{Key: "window", Type: assign.TypeEnum, Values: []string{"24h", "7d"}, Default: "7d"},
			{Key: "threshold", Type: assign.TypeSlider, Min: new(0.0), Max: new(100.0), Default: 80.0},
		},
	}
}

// Every mutation pokes the surface, whoever asked: this is what lets an
// agent's update land in a panel somebody is looking at, instead of waiting
// for the next open (the agentbox:assignments event on the other end).
func TestEveryAssignmentMutationPokesTheSurface(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})

	created := mustSave(t, d, usageWatch())
	after := ui.assignmentPokes()
	if after == 0 {
		t.Fatal("a save never poked the surface")
	}
	for name, mutate := range map[string]func() error{
		"params":  func() error { return d.SetAssignmentParams(created.ID, map[string]any{"window": "24h"}) },
		"enabled": func() error { return d.SetAssignmentEnabled(created.ID, false) },
		"delete":  func() error { _, err := d.DeleteAssignment(created.ID); return err },
	} {
		before := ui.assignmentPokes()
		if err := mutate(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if ui.assignmentPokes() == before {
			t.Errorf("%s never poked the surface", name)
		}
	}
}

// The rule the whole update path exists for: an agent that sends a better
// prompt must not blank the schedule, the model or the knobs by not mentioning
// them. Everything a human tuned survives an edit it was not part of.
func TestAnUpdateOnlyChangesWhatItSends(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	created := mustSave(t, d, usageWatch())
	mustSave(t, d, proto.AssignmentSave{ID: created.ID, Params: map[string]any{"threshold": 95.0}})

	after := mustSave(t, d, proto.AssignmentSave{
		ID:     created.ID,
		Prompt: new("Check Claude usage over {{window}} and warn above {{threshold}}% - be brief."),
	})
	a := after.Assignment
	if a.Name != "Usage watch" || a.Schedule != "daily 09:00" || a.Model != "claude-opus-5" {
		t.Errorf("an unmentioned field was cleared: %+v", a)
	}
	if len(a.Spec) != 2 {
		t.Errorf("the spec was dropped by an edit that never mentioned it: %+v", a.Spec)
	}
	if a.Params["threshold"] != 95.0 || a.Params["window"] != "7d" {
		t.Errorf("params = %v, want the human's 95 and the default window", a.Params)
	}
	if !strings.HasSuffix(a.Prompt, "be brief.") {
		t.Errorf("the prompt did not take: %q", a.Prompt)
	}
}

// Setting one knob sets one knob. A params map that replaced rather than merged
// would make "turn the threshold up" quietly reset the window.
func TestParamsMergeRatherThanReplace(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	created := mustSave(t, d, usageWatch())

	mustSave(t, d, proto.AssignmentSave{ID: created.ID, Params: map[string]any{"window": "24h"}})
	out := mustSave(t, d, proto.AssignmentSave{ID: created.ID, Params: map[string]any{"threshold": 60.0}})

	if out.Assignment.Params["window"] != "24h" {
		t.Errorf("params = %v, want the window kept", out.Assignment.Params)
	}
	if out.Assignment.Params["threshold"] != 60.0 {
		t.Errorf("params = %v, want the new threshold", out.Assignment.Params)
	}
}

// An assignment whose knobs were never declared still substitutes its values.
// Running those through the spec merge would erase every one of them, so a
// caller that set a value would watch it vanish with nothing said.
func TestValuesSurviveWhenNoSpecDeclaresThem(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	out := mustSave(t, d, proto.AssignmentSave{
		Name:   new("Ad hoc"),
		Prompt: new("Summarise {{repo}}."),
		Params: map[string]any{"repo": "agentbox"},
	})
	if out.Assignment.Params["repo"] != "agentbox" {
		t.Fatalf("params = %v, want the value kept", out.Assignment.Params)
	}
	if len(out.Warnings) != 0 {
		t.Errorf("warnings = %v, want none: every placeholder has a value", out.Warnings)
	}

	// And the read has to agree with the save. Diagnosing through the spec
	// merge would report {{repo}} unfilled while the run substitutes it fine.
	raw, rpcErr := d.assignmentRead(mustJSON(t, proto.AssignmentRead{ID: out.ID}))
	if rpcErr != nil {
		t.Fatalf("read: %s", rpcErr.Message)
	}
	if unfilled := raw.(proto.AssignmentReadResult).Unfilled; len(unfilled) != 0 {
		t.Errorf("unfilled = %v, want none: the value is there", unfilled)
	}
}

// A value with no knob behind it is dropped by the merge - which is the
// documented rule - but never silently, because the alternative is an agent
// debugging a parameter it believes it set.
func TestADroppedValueIsReportedNotSwallowed(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	created := mustSave(t, d, usageWatch())

	out := mustSave(t, d, proto.AssignmentSave{ID: created.ID, Params: map[string]any{"nonesuch": 1}})
	if _, ok := out.Assignment.Params["nonesuch"]; ok {
		t.Error("a value with no knob was stored anyway")
	}
	if !warned(out.Warnings, "nonesuch") {
		t.Errorf("warnings = %v, want the dropped key named", out.Warnings)
	}
}

// Half-written is a normal state to save in; wrong is not. A spec fault and an
// unparseable schedule are refused, and the spec fault reports every problem at
// once so an agent can fix them in one pass.
func TestSaveRefusesWhatWouldRenderWrong(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	base := usageWatch()

	_, rpcErr := saveCall(t, d, proto.AssignmentSave{
		Name: base.Name, Prompt: base.Prompt,
		Spec: &[]assign.Param{
			{Key: "a", Type: "colour"},
			{Key: "b", Type: assign.TypeEnum},
			{Type: assign.TypeText},
		},
	})
	if rpcErr == nil {
		t.Fatal("a spec with three faults was accepted")
	}
	for _, want := range []string{"colour", "nothing to choose", "no key"} {
		if !strings.Contains(rpcErr.Message, want) {
			t.Errorf("refusal %q does not mention %q; an agent fixing one fault at a time needs all of them", rpcErr.Message, want)
		}
	}

	bad := usageWatch()
	bad.Schedule = new("every fortnight")
	if _, rpcErr := saveCall(t, d, bad); rpcErr == nil {
		t.Error("an unparseable schedule was accepted")
	}

	noPrompt := usageWatch()
	noPrompt.Prompt = new("   ")
	if _, rpcErr := saveCall(t, d, noPrompt); rpcErr == nil {
		t.Error("an assignment with no prompt was accepted")
	}
}

// The three things agentbox notices and does not refuse. Each of them is an
// assignment that would run and disappoint rather than fail.
func TestWarningsNameWhatIsUnfinished(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	out := mustSave(t, d, proto.AssignmentSave{
		Name:      new("Half written"),
		Prompt:    new("Check {{window}} and report to {{nobody}}."),
		PanelHTML: new("<div/>"),
		Spec: &[]assign.Param{
			{Key: "window", Type: assign.TypeText},
			{Key: "spare", Type: assign.TypeText},
		},
	})
	if !warned(out.Warnings, "nobody") {
		t.Errorf("warnings = %v, want the unfilled placeholder named", out.Warnings)
	}
	if !warned(out.Warnings, "spare") {
		t.Errorf("warnings = %v, want the knob the prompt never uses named", out.Warnings)
	}

	// A custom panel with no typed spec behind it is the one shape the design
	// rules out: if the panel fails to load there is no way left in.
	panelOnly := mustSave(t, d, proto.AssignmentSave{
		Name: new("Panel only"), Prompt: new("Go."), PanelHTML: new("<div/>"),
	})
	if !warned(panelOnly.Warnings, "panel") {
		t.Errorf("warnings = %v, want the panel-without-spec warning", panelOnly.Warnings)
	}
}

func warned(warnings []string, substr string) bool {
	return slices.ContainsFunc(warnings, func(w string) bool { return strings.Contains(w, substr) })
}

// A read is the same diagnosis a save gives, before anything is written: an
// agent asked to improve an assignment starts from agentbox's own opinion of it.
func TestReadCarriesTheDiagnosisAndTheHistory(t *testing.T) {
	d, _, _, st := newTestDaemon(t, Config{})
	created := mustSave(t, d, proto.AssignmentSave{
		Name:   new("Diagnose me"),
		Prompt: new("Check {{window}} and mail {{nobody}}."),
		Spec:   &[]assign.Param{{Key: "window", Type: assign.TypeText}, {Key: "spare", Type: assign.TypeText}},
	})
	if err := st.StartRun(&store.Run{AssignmentID: created.ID, Trigger: "manual", State: store.RunOK, Summary: "all quiet"}); err != nil {
		t.Fatal(err)
	}

	raw, rpcErr := d.assignmentRead(mustJSON(t, proto.AssignmentRead{ID: created.ID}))
	if rpcErr != nil {
		t.Fatalf("read: %s", rpcErr.Message)
	}
	res, ok := raw.(proto.AssignmentReadResult)
	if !ok {
		t.Fatalf("read returned %T", raw)
	}
	if res.Kind != "ad-hoc" {
		t.Errorf("kind = %q, want ad-hoc for an empty schedule", res.Kind)
	}
	if !slices.Equal(res.Placeholders, []string{"window", "nobody"}) {
		t.Errorf("placeholders = %v, want both in first-seen order", res.Placeholders)
	}
	if !slices.Equal(res.Unfilled, []string{"nobody"}) {
		t.Errorf("unfilled = %v, want the one nothing fills", res.Unfilled)
	}
	if !slices.Equal(res.Unused, []string{"spare"}) {
		t.Errorf("unused = %v, want the knob no placeholder reads", res.Unused)
	}
	if len(res.Runs) != 1 || res.Runs[0].Summary != "all quiet" {
		t.Errorf("runs = %+v, want the one run with its summary", res.Runs)
	}
}

// A create arms its own schedule, so an assignment made at 14:03 is due at
// 09:00 rather than waiting for a restart to notice it exists.
func TestCreateArmsTheSchedule(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	out := mustSave(t, d, usageWatch())
	if !out.Created || out.Kind != "scheduled" {
		t.Errorf("result = %+v, want a created scheduled assignment", out)
	}
	if out.NextRunMS == 0 {
		t.Error("a scheduled assignment was created unarmed and would never run")
	}
	if !out.Assignment.Enabled {
		t.Error("a new assignment arrived disabled")
	}

	quiet := usageWatch()
	quiet.Enabled = new(false)
	q := mustSave(t, d, quiet)
	if q.Assignment.Enabled || q.NextRunMS != 0 {
		t.Errorf("enabled=false was not honoured: %+v", q)
	}
}

// List is what the panel and an agent both open with, so it has to say how the
// last run went without a second call.
func TestListCarriesTheLastRun(t *testing.T) {
	d, _, _, st := newTestDaemon(t, Config{})
	created := mustSave(t, d, usageWatch())
	if err := st.StartRun(&store.Run{AssignmentID: created.ID, Trigger: "schedule", State: store.RunFailed, Error: "claude exited 1"}); err != nil {
		t.Fatal(err)
	}

	raw, rpcErr := d.assignmentList()
	if rpcErr != nil {
		t.Fatalf("list: %s", rpcErr.Message)
	}
	res := raw.(proto.AssignmentListResult)
	if len(res.Assignments) != 1 {
		t.Fatalf("list = %+v, want one row", res.Assignments)
	}
	row := res.Assignments[0]
	if row.LastState != store.RunFailed || row.LastSummary != "claude exited 1" {
		t.Errorf("row = %+v, want the failure and its reason", row)
	}
	if row.Kind != "scheduled" || !row.Enabled {
		t.Errorf("row = %+v, want a live scheduled assignment", row)
	}
}

// Every id-taking method refuses a stranger by name rather than 500ing or, in
// delete's case, cheerfully reporting success.
func TestMissingIdsAreRefusedNotGuessed(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	for name, call := range map[string]func() (any, *proto.RPCError){
		"read":   func() (any, *proto.RPCError) { return d.assignmentRead(mustJSON(t, proto.AssignmentRead{ID: "a000"})) },
		"update": func() (any, *proto.RPCError) { return d.assignmentSave(mustJSON(t, proto.AssignmentSave{ID: "a000"})) },
		"run":    func() (any, *proto.RPCError) { return d.assignmentRun(mustJSON(t, proto.AssignmentRunNow{ID: "a000"})) },
	} {
		if _, rpcErr := call(); rpcErr == nil {
			t.Errorf("%s accepted an id that does not exist", name)
		}
	}
	raw, rpcErr := d.assignmentDelete(mustJSON(t, proto.AssignmentDelete{ID: "a000"}))
	if rpcErr != nil {
		t.Fatalf("delete: %s", rpcErr.Message)
	}
	if raw.(proto.AssignmentDeleteResult).Deleted {
		t.Error("deleting nothing reported success")
	}
}

func TestDeleteTakesTheHistoryWithIt(t *testing.T) {
	d, _, _, st := newTestDaemon(t, Config{})
	created := mustSave(t, d, usageWatch())
	if err := st.StartRun(&store.Run{AssignmentID: created.ID, Trigger: "manual"}); err != nil {
		t.Fatal(err)
	}

	raw, rpcErr := d.assignmentDelete(mustJSON(t, proto.AssignmentDelete{ID: created.ID}))
	if rpcErr != nil {
		t.Fatalf("delete: %s", rpcErr.Message)
	}
	if !raw.(proto.AssignmentDeleteResult).Deleted {
		t.Fatal("delete reported nothing removed")
	}
	runs, err := st.RunsFor(created.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Errorf("%d runs outlived their assignment", len(runs))
	}
}

// The wire types have to survive the trip an MCP client actually makes them
// take. A pointer field that marshalled wrong would silently read as "leave it
// alone" and an update would do nothing at all.
func TestSaveRoundTripsThroughJSON(t *testing.T) {
	req := proto.AssignmentSave{ID: "a1", Prompt: new("go"), Enabled: new(false)}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var back proto.AssignmentSave
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.Name != nil || back.Schedule != nil {
		t.Errorf("an unsent field arrived set: %+v", back)
	}
	if back.Prompt == nil || *back.Prompt != "go" {
		t.Errorf("prompt = %v, want the value sent", back.Prompt)
	}
	if back.Enabled == nil || *back.Enabled {
		t.Errorf("enabled = %v, want an explicit false to survive", back.Enabled)
	}
}
