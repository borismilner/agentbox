package webui

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/borismilner/agentbox/internal/assign"
	"github.com/borismilner/agentbox/internal/proto"
)

// The Assignments surface's half of the bridge (M12/FR82).
//
// Every write goes through the daemon's own AssignmentSave rather than around
// it, so the editor and an agent get the same refusals and the same warnings.
// A panel with its own idea of what a valid schedule is would be a second
// answer to the same question, and the two would drift the first time either
// changed.

// AssignmentStore is what the surface needs from the daemon. It is the same
// small keyhole BoardStore is, and for the same reason: the UI package must not
// depend on the daemon's internals to draw a list.
type AssignmentStore interface {
	AssignmentList() (proto.AssignmentListResult, *proto.RPCError)
	AssignmentRead(req proto.AssignmentRead) (proto.AssignmentReadResult, *proto.RPCError)
	AssignmentSave(req proto.AssignmentSave) (proto.AssignmentSaveResult, *proto.RPCError)
	DeleteAssignment(id string) (bool, error)
	SetAssignmentEnabled(id string, on bool) error
	SetAssignmentParams(id string, params map[string]any) error
	RunAssignmentNow(id, trigger string, overrides map[string]any) (string, error)
}

// SetAssignmentStore wires the daemon in after it exists, like SetBoardStore.
func (u *UI) SetAssignmentStore(as AssignmentStore) {
	u.mu.Lock()
	u.assignStore = as
	u.mu.Unlock()
}

func (u *UI) assignments() AssignmentStore {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.assignStore
}

// errNoStore is what every method here answers with in a build that has no
// daemon behind it (the demo harness). The surface shows the sentence rather
// than an empty list that reads like "you have no assignments".
var errNoStore = errors.New("this build has no daemon behind it, so it has no assignments")

// wireAssignments is what the surface opens on.
type wireAssignments struct {
	Assignments []proto.AssignmentSummary `json:"assignments"`
	Err         string                    `json:"err,omitempty"`
}

// Assignments lists them for the surface.
func (b *Bridge) Assignments() wireAssignments {
	as := b.ui.assignments()
	if as == nil {
		return wireAssignments{Assignments: []proto.AssignmentSummary{}, Err: errNoStore.Error()}
	}
	res, rpcErr := as.AssignmentList()
	if rpcErr != nil {
		return wireAssignments{Assignments: []proto.AssignmentSummary{}, Err: rpcErr.Message}
	}
	return wireAssignments{Assignments: res.Assignments}
}

// wireAssignment is one assignment open in the editor: the definition, agentbox's
// diagnosis of it, its knobs ready to draw, and its recent runs.
type wireAssignment struct {
	Assignment   *assign.Assignment    `json:"assignment,omitempty"`
	Kind         string                `json:"kind,omitempty"`
	Running      bool                  `json:"running"`
	Knobs        []wireParam           `json:"knobs,omitempty"`
	Placeholders []string              `json:"placeholders,omitempty"`
	Unfilled     []string              `json:"unfilled,omitempty"`
	Unused       []string              `json:"unused,omitempty"`
	Problems     []string              `json:"problems,omitempty"`
	Runs         []proto.AssignmentRun `json:"runs,omitempty"`
	Err          string                `json:"err,omitempty"`
}

// wireParam is one control, with everything the surface needs to draw it and
// nothing it has to work out. The descriptor arrangement the Settings surface
// uses: the shape of a control is decided in Go, so a knob type added to
// assign.Param cannot half-exist because the frontend was not told.
type wireParam struct {
	Key       string   `json:"key,omitempty"`
	Label     string   `json:"label"`
	Type      string   `json:"type"`
	Help      string   `json:"help,omitempty"`
	Unit      string   `json:"unit,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	Step      *float64 `json:"step,omitempty"`
	Values    []string `json:"values,omitempty"`
	Multiline bool     `json:"multiline,omitempty"`
	// BodyHTML carries a markdown block's prose, rendered here because every
	// other piece of markdown in agentbox is rendered here (mdhtml.go) and a second
	// renderer in the frontend would style it differently.
	BodyHTML string `json:"bodyHtml,omitempty"`
	Value    any    `json:"value,omitempty"`
}

// paramsFor pairs the spec with the values, in spec order. A knob with no value
// yet gets its default, so an assignment an agent has just written draws as a
// filled-in form rather than an empty one.
func paramsFor(spec []assign.Param, params map[string]any) []wireParam {
	if len(spec) == 0 {
		return nil
	}
	values := assign.Merge(spec, params)
	out := make([]wireParam, 0, len(spec))
	for _, p := range spec {
		k := wireParam{
			Key: p.Key, Label: p.Label, Type: p.Type, Help: p.Help, Unit: p.Unit,
			Min: p.Min, Max: p.Max, Step: p.Step, Values: p.Values, Multiline: p.Multiline,
		}
		if k.Label == "" {
			k.Label = p.Key
		}
		if p.Type == assign.TypeMarkdown {
			k.BodyHTML = RenderMarkdown(p.Body)
		} else {
			k.Value = values[p.Key]
		}
		out = append(out, k)
	}
	return out
}

// Assignment opens one, with more history than a read normally carries: this is
// the surface where somebody goes to find out why last Tuesday failed.
func (b *Bridge) Assignment(id string) wireAssignment {
	as := b.ui.assignments()
	if as == nil {
		return wireAssignment{Err: errNoStore.Error()}
	}
	res, rpcErr := as.AssignmentRead(proto.AssignmentRead{ID: id, Runs: 30})
	if rpcErr != nil {
		return wireAssignment{Err: rpcErr.Message}
	}
	return wireAssignment{
		Assignment: res.Assignment, Kind: res.Kind, Running: res.Running,
		Knobs:        paramsFor(res.Assignment.Spec, res.Assignment.Params),
		Placeholders: res.Placeholders, Unfilled: res.Unfilled, Unused: res.Unused,
		Problems: res.Problems, Runs: res.Runs,
	}
}

// wireAssignSaved is what the editor gets back: the id (a create has one only
// now), and everything agentbox noticed but did not refuse.
type wireAssignSaved struct {
	ID       string   `json:"id,omitempty"`
	Created  bool     `json:"created,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Err      string   `json:"err,omitempty"`
}

// SaveAssignment writes the editor's fields. Values is a flat string map for
// the same reason SaveSettings takes one - it is what crosses the Wails
// boundary without a generated type per form - and the two structured fields
// ride it as JSON text. A key that is absent is a field the editor did not
// touch, which is the same rule the MCP update follows.
func (b *Bridge) SaveAssignment(values map[string]string) wireAssignSaved {
	as := b.ui.assignments()
	if as == nil {
		return wireAssignSaved{Err: errNoStore.Error()}
	}
	req := proto.AssignmentSave{ID: values["id"]}
	for key, dst := range map[string]**string{
		"name": &req.Name, "description": &req.Description, "prompt": &req.Prompt,
		"panel_html": &req.PanelHTML, "model": &req.Model, "mode": &req.Mode,
		"dir": &req.Dir, "schedule": &req.Schedule,
	} {
		if v, ok := values[key]; ok {
			*dst = &v
		}
	}
	if v, ok := values["enabled"]; ok {
		on := v == "true" || v == "yes" || v == "1"
		req.Enabled = &on
	}
	if raw, ok := values["spec"]; ok {
		spec, err := parseSpecJSON(raw)
		if err != nil {
			return wireAssignSaved{Err: err.Error()}
		}
		req.Spec = &spec
	}
	if raw, ok := values["params"]; ok {
		params, err := parseParamsJSON(raw)
		if err != nil {
			return wireAssignSaved{Err: err.Error()}
		}
		req.Params = params
	}
	res, rpcErr := as.AssignmentSave(req)
	if rpcErr != nil {
		return wireAssignSaved{Err: rpcErr.Message}
	}
	return wireAssignSaved{ID: res.ID, Created: res.Created, Kind: res.Kind, Warnings: res.Warnings}
}

// parseSpecJSON reads the knobs as the editor's JSON field holds them. Empty
// means no knobs, which is a real answer and not a parse failure.
func parseSpecJSON(raw string) ([]assign.Param, error) {
	if strings.TrimSpace(raw) == "" {
		return []assign.Param{}, nil
	}
	var spec []assign.Param
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return nil, errors.New("the knobs are not a JSON array: " + err.Error())
	}
	return spec, nil
}

func parseParamsJSON(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		return nil, errors.New("the values are not a JSON object: " + err.Error())
	}
	return params, nil
}

// SetAssignmentParams saves just the knob values, which is what turning a
// control does. Separate from SaveAssignment because a slider must not be able
// to rewrite a prompt, and because the panel writes on every change.
func (b *Bridge) SetAssignmentParams(id, paramsJSON string) string {
	as := b.ui.assignments()
	if as == nil {
		return errNoStore.Error()
	}
	params, err := parseParamsJSON(paramsJSON)
	if err != nil {
		return err.Error()
	}
	if err := as.SetAssignmentParams(id, params); err != nil {
		return err.Error()
	}
	return ""
}

// EnableAssignment is the pause switch.
func (b *Bridge) EnableAssignment(id string, on bool) string {
	as := b.ui.assignments()
	if as == nil {
		return errNoStore.Error()
	}
	if err := as.SetAssignmentEnabled(id, on); err != nil {
		return err.Error()
	}
	return ""
}

// DeleteAssignment removes one with its history. The surface asks first.
func (b *Bridge) DeleteAssignment(id string) string {
	as := b.ui.assignments()
	if as == nil {
		return errNoStore.Error()
	}
	if _, err := as.DeleteAssignment(id); err != nil {
		return err.Error()
	}
	return ""
}

// RunAssignment is the Run button. It returns as soon as the run is recorded -
// the run itself is a session that takes minutes - so the surface polls rather
// than waiting.
func (b *Bridge) RunAssignment(id string) string {
	as := b.ui.assignments()
	if as == nil {
		return errNoStore.Error()
	}
	if _, err := as.RunAssignmentNow(id, "manual", nil); err != nil {
		return err.Error()
	}
	return ""
}
