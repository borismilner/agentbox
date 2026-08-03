package mcp

import (
	"path/filepath"
	"testing"

	"github.com/borismilner/agentbox/internal/assign"
)

// What these pin is the shape of the request before it travels, without a
// daemon: the create/update asymmetry is the whole reason there are two input
// types, and getting it wrong is silent - an update would blank a field the
// human set and report success.

func TestUpdateSendsOnlyWhatItWasGiven(t *testing.T) {
	req, err := updateRequest(assignUpdateIn{AssignmentID: "a1", Prompt: new("go")})
	if err != nil {
		t.Fatal(err)
	}
	if req.ID != "a1" || req.Prompt == nil || *req.Prompt != "go" {
		t.Fatalf("request lost what it was given: %+v", req)
	}
	for name, got := range map[string]*string{
		"name": req.Name, "description": req.Description, "schedule": req.Schedule,
		"model": req.Model, "mode": req.Mode, "dir": req.Dir, "panel_html": req.PanelHTML,
	} {
		if got != nil {
			t.Errorf("%s travelled as %q; an unmentioned field must stay nil or the update clears it", name, *got)
		}
	}
	if req.Spec != nil || req.Enabled != nil || req.Params != nil {
		t.Errorf("spec/enabled/params were invented: %+v", req)
	}

	// An empty string is a real value on update: it is how a schedule is made
	// ad-hoc and how a custom panel is removed.
	req, err = updateRequest(assignUpdateIn{AssignmentID: "a1", Schedule: new("")})
	if err != nil {
		t.Fatal(err)
	}
	if req.Schedule == nil || *req.Schedule != "" {
		t.Errorf("an explicit empty schedule did not survive: %+v", req.Schedule)
	}

	if _, err := updateRequest(assignUpdateIn{Prompt: new("go")}); err == nil {
		t.Error("an update with no assignment_id was allowed to travel")
	}
}

func TestCreateOmitsWhatWasNotFilled(t *testing.T) {
	req, err := createRequest(assignCreateIn{Name: "Watch", Prompt: "Check {{window}}."})
	if err != nil {
		t.Fatal(err)
	}
	if req.ID != "" || *req.Name != "Watch" || *req.Prompt != "Check {{window}}." {
		t.Fatalf("request: %+v", req)
	}
	// Left out so the daemon's defaults stand: a mode of "" would be refused,
	// and an enabled of false would create it paused.
	if req.Mode != nil || req.Schedule != nil || req.Model != nil || req.Enabled != nil {
		t.Errorf("an unfilled field travelled anyway: %+v", req)
	}

	no := false
	req, err = createRequest(assignCreateIn{Name: "Watch", Prompt: "go", Schedule: "daily 09:00", Enabled: &no})
	if err != nil {
		t.Fatal(err)
	}
	if req.Schedule == nil || *req.Schedule != "daily 09:00" {
		t.Errorf("schedule: %+v", req.Schedule)
	}
	if req.Enabled == nil || *req.Enabled {
		t.Errorf("an explicit enabled=false was lost: %+v", req.Enabled)
	}
}

// The knobs arrive as objects (the schema a validating client can satisfy) and
// have to land as typed parameters, with a wrong field named here rather than
// discovered in the store.
func TestToSpecTypesTheKnobs(t *testing.T) {
	spec, err := toSpec([]map[string]any{
		{"key": "window", "type": "enum", "values": []string{"24h", "7d"}, "default": "7d"},
		{"key": "threshold", "type": "slider", "min": 0, "max": 100, "unit": "%"},
		{"type": "markdown", "body": "Above 95 this goes urgent."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(spec) != 3 {
		t.Fatalf("spec has %d knobs", len(spec))
	}
	if spec[0].Type != assign.TypeEnum || len(spec[0].Values) != 2 || spec[0].Default != "7d" {
		t.Errorf("enum: %+v", spec[0])
	}
	if spec[1].Min == nil || *spec[1].Max != 100 || spec[1].Unit != "%" {
		t.Errorf("slider: %+v", spec[1])
	}
	if spec[2].Input() || spec[2].Body == "" {
		t.Errorf("markdown should carry prose and no value: %+v", spec[2])
	}
	if probs := assign.Validate(spec); len(probs) > 0 {
		t.Errorf("a spec that should be valid was not: %v", probs)
	}

	if _, err := toSpec([]map[string]any{{"key": "x", "type": "slider", "min": "low"}}); err == nil {
		t.Error("a min that is not a number was accepted")
	}
}

// Every assignment tool refuses a missing id before dialling anything, so a
// daemonless caller gets the sentence rather than a transport error.
func TestAssignmentToolsRefuseAMissingIDWithoutADaemon(t *testing.T) {
	s := &server{runtimeDir: filepath.Join(t.TempDir(), "no-daemon")}
	for name, call := range map[string]func() (bool, error){
		"read_assignment": func() (bool, error) {
			res, _, err := s.assignRead(t.Context(), nil, assignReadIn{})
			return res != nil && res.IsError, err
		},
		"delete_assignment": func() (bool, error) {
			res, _, err := s.assignDelete(t.Context(), nil, assignDeleteIn{})
			return res != nil && res.IsError, err
		},
		"run_assignment": func() (bool, error) {
			res, _, err := s.assignRun(t.Context(), nil, assignRunIn{})
			return res != nil && res.IsError, err
		},
		"assignment_runs": func() (bool, error) {
			res, _, err := s.assignRuns(t.Context(), nil, assignRunsIn{})
			return res != nil && res.IsError, err
		},
	} {
		isErr, err := call()
		if err != nil || !isErr {
			t.Errorf("%s: isError = %v, err = %v; want a tool error the model can read", name, isErr, err)
		}
	}
}
