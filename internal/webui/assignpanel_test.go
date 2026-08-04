package webui

import (
	"strings"
	"testing"

	"github.com/borismilner/agentbox/internal/assign"
	"github.com/borismilner/agentbox/internal/proto"
)

// fakeAssignStore records what the bridge sends the daemon. What matters here is
// not that a save works - the daemon's own suite covers that - but that the
// editor's flat string map arrives as the right partial update.
type fakeAssignStore struct {
	saved     proto.AssignmentSave
	params    map[string]any
	ran       string
	panelHTML string
	err       *proto.RPCError
}

func (f *fakeAssignStore) AssignmentList() (proto.AssignmentListResult, *proto.RPCError) {
	return proto.AssignmentListResult{}, f.err
}

func (f *fakeAssignStore) AssignmentRead(req proto.AssignmentRead) (proto.AssignmentReadResult, *proto.RPCError) {
	return proto.AssignmentReadResult{
		Assignment: &assign.Assignment{
			ID: req.ID, Name: "Watch", Prompt: "Check {{window}}.", PanelHTML: f.panelHTML,
		},
		Kind: "ad-hoc",
	}, f.err
}

func (f *fakeAssignStore) AssignmentSave(req proto.AssignmentSave) (proto.AssignmentSaveResult, *proto.RPCError) {
	f.saved = req
	if f.err != nil {
		return proto.AssignmentSaveResult{}, f.err
	}
	return proto.AssignmentSaveResult{ID: "a1", Kind: "scheduled"}, nil
}

func (f *fakeAssignStore) DeleteAssignment(string) (bool, error)   { return true, nil }
func (f *fakeAssignStore) SetAssignmentEnabled(string, bool) error { return nil }
func (f *fakeAssignStore) RunAssignmentNow(id, _ string, _ map[string]any) (string, error) {
	f.ran = id
	return "r1", nil
}

func (f *fakeAssignStore) SetAssignmentParams(_ string, params map[string]any) error {
	f.params = params
	return nil
}

func panelBridge(t *testing.T) (*Bridge, *fakeAssignStore) {
	t.Helper()
	u := gateUI()
	store := &fakeAssignStore{}
	u.SetAssignmentStore(store)
	return &Bridge{ui: u}, store
}

// The editor's map is a partial update, exactly like an agent's. A field it did
// not put in the map must not travel as an empty string, or opening the editor
// on one tab and saving would blank everything the other tab holds.
func TestSaveAssignmentSendsOnlyTheKeysItWasGiven(t *testing.T) {
	b, store := panelBridge(t)

	res := b.SaveAssignment(map[string]string{"id": "a1", "prompt": "Check usage."})
	if res.Err != "" {
		t.Fatalf("save: %s", res.Err)
	}
	if store.saved.ID != "a1" || store.saved.Prompt == nil || *store.saved.Prompt != "Check usage." {
		t.Fatalf("save request: %+v", store.saved)
	}
	for name, got := range map[string]*string{
		"name": store.saved.Name, "schedule": store.saved.Schedule,
		"model": store.saved.Model, "dir": store.saved.Dir, "panel_html": store.saved.PanelHTML,
	} {
		if got != nil {
			t.Errorf("%s travelled as %q; the editor never sent it", name, *got)
		}
	}
	if store.saved.Spec != nil || store.saved.Enabled != nil {
		t.Errorf("spec or enabled were invented: %+v", store.saved)
	}

	// And an empty string IS a value when the key is there: it is how the
	// editor makes a scheduled assignment ad-hoc again.
	b.SaveAssignment(map[string]string{"id": "a1", "schedule": ""})
	if store.saved.Schedule == nil || *store.saved.Schedule != "" {
		t.Errorf("an explicit empty schedule did not survive: %v", store.saved.Schedule)
	}
}

func TestSaveAssignmentRefusesKnobsThatAreNotJSON(t *testing.T) {
	b, _ := panelBridge(t)
	res := b.SaveAssignment(map[string]string{"id": "a1", "spec": "{not json"})
	if res.Err == "" || !strings.Contains(res.Err, "JSON") {
		t.Errorf("err = %q, want a sentence naming the problem", res.Err)
	}
	if res := b.SaveAssignment(map[string]string{"id": "a1", "spec": ""}); res.Err != "" {
		t.Errorf("an empty knob list is a real answer, not a parse failure: %q", res.Err)
	}
}

// The surface draws from a descriptor, so everything a control needs has to be
// in it: the value merged from the defaults, and a markdown block's prose
// already rendered.
func TestParamsForBuildsDrawableKnobs(t *testing.T) {
	min, max := 0.0, 100.0
	knobs := paramsFor([]assign.Param{
		{Key: "window", Type: assign.TypeEnum, Values: []string{"24h", "7d"}, Default: "7d"},
		{Key: "threshold", Type: assign.TypeSlider, Min: &min, Max: &max, Unit: "%", Default: 80.0},
		{Type: assign.TypeMarkdown, Body: "Above **95** it goes urgent."},
	}, map[string]any{"threshold": 95.0})

	if len(knobs) != 3 {
		t.Fatalf("%d knobs", len(knobs))
	}
	if knobs[0].Label != "window" {
		t.Errorf("a knob with no label should fall back to its key, got %q", knobs[0].Label)
	}
	if knobs[0].Value != "7d" {
		t.Errorf("window = %v, want the default filled in", knobs[0].Value)
	}
	if knobs[1].Value != 95.0 || knobs[1].Unit != "%" {
		t.Errorf("threshold = %+v, want the stored value and its unit", knobs[1])
	}
	if knobs[2].Value != nil {
		t.Errorf("a markdown block carries no value, got %v", knobs[2].Value)
	}
	if !strings.Contains(knobs[2].BodyHTML, "<strong>95</strong>") {
		t.Errorf("markdown reached the surface unrendered: %q", knobs[2].BodyHTML)
	}
	if paramsFor(nil, nil) != nil {
		t.Error("no spec should draw no form")
	}
}

// A knob writes just the values. A slider that could rewrite a prompt would be
// a slider one bad payload away from destroying an assignment.
// The custom panel travels as an inert block only when there is one, marked and
// stamped with the assignment's id so the surface can route its values home. An
// assignment without one must not send an empty block the surface would try to
// hydrate.
func TestAssignmentCarriesItsPanelBlock(t *testing.T) {
	b, store := panelBridge(t)

	if got := b.Assignment("a1").PanelBlock; got != "" {
		t.Errorf("no panel_html, yet a block travelled: %q", got)
	}

	store.panelHTML = "export default function Panel() { return <input />; }"
	got := b.Assignment("a1").PanelBlock
	for _, want := range []string{`data-panel="1"`, `data-artifact-id="a1"`} {
		if !strings.Contains(got, want) {
			t.Errorf("panel block is missing %q:\n%s", want, got)
		}
	}
}

func TestSetAssignmentParamsWritesOnlyValues(t *testing.T) {
	b, store := panelBridge(t)
	if msg := b.SetAssignmentParams("a1", `{"window":"24h"}`); msg != "" {
		t.Fatalf("params: %s", msg)
	}
	if store.params["window"] != "24h" {
		t.Errorf("params = %v", store.params)
	}
	if msg := b.SetAssignmentParams("a1", "nope"); msg == "" {
		t.Error("a malformed values blob was accepted")
	}
}

// A build with no daemon says so rather than drawing an empty list, which reads
// as "you have no assignments" and is a different sentence entirely.
func TestThePanelSaysWhenThereIsNoDaemon(t *testing.T) {
	b := &Bridge{ui: gateUI()}
	if res := b.Assignments(); res.Err == "" {
		t.Error("an empty list with no error reads as 'you have none'")
	}
	if res := b.Assignment("a1"); res.Err == "" {
		t.Error("opening one with no daemon reported success")
	}
	for name, msg := range map[string]string{
		"save":   b.SaveAssignment(map[string]string{}).Err,
		"params": b.SetAssignmentParams("a1", "{}"),
		"enable": b.EnableAssignment("a1", true),
		"delete": b.DeleteAssignment("a1"),
		"run":    b.RunAssignment("a1"),
	} {
		if msg == "" {
			t.Errorf("%s reported success with no daemon behind it", name)
		}
	}
}

func TestRunAssignmentAsksTheDaemonToRunIt(t *testing.T) {
	b, store := panelBridge(t)
	if msg := b.RunAssignment("a1"); msg != "" {
		t.Fatalf("run: %s", msg)
	}
	if store.ran != "a1" {
		t.Errorf("ran %q, want a1", store.ran)
	}
}
