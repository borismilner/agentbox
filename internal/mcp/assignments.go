package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borismilner/agentbox/internal/assign"
	"github.com/borismilner/agentbox/internal/proto"
)

// The assignment tools (M12/FR82). An assignment is work agentbox gives an agent on
// a schedule or on demand; these seven tools are how an agent reads, writes and
// runs them.
//
// They exist as an MCP surface and not only as a panel because that is exactly
// the requirement: "upon creation, the AI agent itself should help with generating
// the initial prompt and the configuration panel for it until the user is
// satisfied. The AI agent should have full access to these so that it can help
// adjusting and improving assignments as we go along." So this is the authoring
// surface, and the descriptions below are the whole manual a model gets.

func addAssignmentTools(srv *sdk.Server, s *server) {
	sdk.AddTool(srv, &sdk.Tool{
		Name: "list_assignments",
		Description: "List the human's assignments: recurring or on-demand work agentbox gives a Claude agent on its own, each with a prompt, typed parameters and a schedule. " +
			"Shows what each one is, when it next runs, and how its last run went. Start here before creating one - an assignment that already exists usually wants editing, not a twin.",
	}, s.assignList)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "read_assignment",
		Description: "Read one assignment whole: prompt template, parameter spec and current values, schedule, model, and its recent runs with their summaries. " +
			"It also returns agentbox's own diagnosis - which {{placeholders}} nothing fills, which knobs the prompt never uses, and any fault in the stored spec - so you can improve it from the same picture the human sees.",
	}, s.assignRead)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "create_assignment",
		Description: "Create an assignment: work agentbox will carry out by launching a Claude agent, on a schedule or when asked. " +
			"Write the prompt as a template with {{placeholders}}, and declare a knob for each one so the human can retune it without editing prose. " +
			"The run gets agentbox's own tools, so say in the prompt how it should report - notify_user for something worth knowing, an urgent level when the number is bad, show_document for a summary worth reading. " +
			"It starts enabled; returns the assignment_id, and warnings for anything left half-written. " +
			"Author it WITH the human: propose the prompt and the knobs, run it once, and keep updating until they are satisfied.",
	}, s.assignCreate)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "update_assignment",
		Description: "Change an assignment. Only the fields you send change - send just a prompt and the schedule, knobs and values are left exactly as the human set them. " +
			"params merges over the stored values rather than replacing them, so setting one knob does not clear the others. " +
			"Refuses a spec that would render wrong or a schedule it cannot parse, and returns warnings for anything merely unfinished.",
	}, s.assignUpdate)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "delete_assignment",
		Description: "Delete an assignment permanently, with its whole run history. Prefer update_assignment with enabled=false to pause one: that keeps the definition and everything it has recorded.",
	}, s.assignDelete)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "run_assignment",
		Description: "Run an assignment now, outside its schedule. Non-blocking: it returns a run_id at once, because a run is a whole conversation - poll assignment_runs for the outcome. " +
			"overrides replace parameter values for this run only, which is how you try one at a different setting without editing what the human saved. " +
			"Use it after creating or editing one, so the human sees it work rather than waiting until 9am to find out it does not.",
	}, s.assignRun)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "assignment_runs",
		Description: "The run history of one assignment, newest first: when it ran, what triggered it, the parameter values it actually used, whether it succeeded, its summary, and any data it recorded for later analysis. " +
			"This is where a run's result comes back, and where a failing assignment shows you what to fix.",
	}, s.assignRuns)
}

type assignListOut struct {
	Assignments []proto.AssignmentSummary `json:"assignments"`
}

func (s *server) assignList(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, assignListOut, error) {
	var res proto.AssignmentListResult
	if err := s.callIntoFast(ctx, proto.MethodAssignmentList, proto.AssignmentList{}, &res); err != nil {
		return errResult[assignListOut](err)
	}
	return &sdk.CallToolResult{}, assignListOut{Assignments: res.Assignments}, nil
}

type assignReadIn struct {
	AssignmentID string `json:"assignment_id" jsonschema:"the id list_assignments returned"`
	Runs         int    `json:"runs,omitempty" jsonschema:"how many recent runs to include; default 5"`
}
type assignReadOut struct {
	Assignment any `json:"assignment" jsonschema:"the definition, its kind (ad-hoc, periodic, scheduled), agentbox's diagnosis of the prompt and spec, and the recent runs"`
}

func (s *server) assignRead(ctx context.Context, _ *sdk.CallToolRequest, in assignReadIn) (*sdk.CallToolResult, assignReadOut, error) {
	if in.AssignmentID == "" {
		return errResult[assignReadOut](fmt.Errorf("read_assignment needs an assignment_id; list_assignments finds one"))
	}
	var res json.RawMessage
	if err := s.callIntoFast(ctx, proto.MethodAssignmentRead, proto.AssignmentRead{
		ID: in.AssignmentID, Runs: in.Runs,
	}, &res); err != nil {
		return errResult[assignReadOut](err)
	}
	return &sdk.CallToolResult{}, assignReadOut{Assignment: decodeEventData(res)}, nil
}

// The create and update inputs differ in exactly one way, and it is the reason
// they are two types rather than one: create takes plain fields, because there
// is nothing behind it to preserve, while every update field is a pointer, so
// a nil is "leave it as the human set it" rather than "make it empty". An agent
// improving a prompt must not blank a schedule by not mentioning it.
type assignCreateIn struct {
	Name        string           `json:"name" jsonschema:"a short name the human will recognise in the list"`
	Prompt      string           `json:"prompt" jsonschema:"the whole instruction the agent will receive, as a template with {{placeholders}} for the tunable parts. Say what to do, what counts as worth interrupting for, and how to report - the human may not be watching"`
	Description string           `json:"description,omitempty" jsonschema:"one line on what this is for"`
	Spec        []map[string]any `json:"spec,omitempty" jsonschema:"the typed knobs, rendered by agentbox as a form: [{key, type, label?, help?, default?, min?, max?, step?, unit?, values?, multiline?, body?}]. type is text | number | slider | toggle | enum | path | markdown. enum needs values; slider needs min and max; markdown carries body instead of a key and renders as prose between the controls. Every key should appear as {{key}} in the prompt, and every {{key}} should have a knob"`
	Params      map[string]any   `json:"params,omitempty" jsonschema:"starting values by key; omit and each knob starts at its default"`
	PanelHTML   string           `json:"panel_html,omitempty" jsonschema:"a custom React/Tailwind panel for the parameters, run in agentbox's artifact sandbox (no network), reporting values through window.agentbox.emit. The escape hatch for controls the typed knobs cannot express - declare a spec as well, or a panel that fails to load leaves nothing to edit the values with"`
	Model       string           `json:"model,omitempty" jsonschema:"the Claude model the run uses; omit for the default"`
	Mode        string           `json:"mode,omitempty" jsonschema:"plan (read-only) or full; default full"`
	Dir         string           `json:"dir,omitempty" jsonschema:"absolute working directory for the run; omit for the human's home"`
	Schedule    string           `json:"schedule,omitempty" jsonschema:"when it runs. Empty = ad-hoc (only when a human or an agent asks). every 30m / every 4h / every 1d counts from the last run. daily 09:00 and weekly mon 09:00 are wall-clock. A missed slot is skipped and recorded, never caught up. One minute is the floor"`
	Enabled     *bool            `json:"enabled,omitempty" jsonschema:"omit to create it enabled; false to author one without arming its schedule"`
}

type assignUpdateIn struct {
	AssignmentID string            `json:"assignment_id" jsonschema:"the id list_assignments returned"`
	Name         *string           `json:"name,omitempty"`
	Prompt       *string           `json:"prompt,omitempty" jsonschema:"replaces the whole template; read_assignment first if you are editing rather than rewriting"`
	Description  *string           `json:"description,omitempty"`
	Spec         *[]map[string]any `json:"spec,omitempty" jsonschema:"replaces the knobs entirely. Values whose knob survives are kept, values whose knob is gone are dropped. Same shape as create_assignment's spec"`
	Params       map[string]any    `json:"params,omitempty" jsonschema:"values by key, merged over what is stored - send one knob to change one knob"`
	PanelHTML    *string           `json:"panel_html,omitempty" jsonschema:"the custom parameter panel; empty string removes it"`
	Model        *string           `json:"model,omitempty"`
	Mode         *string           `json:"mode,omitempty" jsonschema:"plan or full"`
	Dir          *string           `json:"dir,omitempty"`
	Schedule     *string           `json:"schedule,omitempty" jsonschema:"empty string makes it ad-hoc. every 30m / every 4h / every 1d, or daily 09:00 / weekly mon 09:00"`
	Enabled      *bool             `json:"enabled,omitempty" jsonschema:"false pauses it, keeping the definition and its history"`
}

type assignSaveOut struct {
	AssignmentID string   `json:"assignment_id"`
	Created      bool     `json:"created,omitempty"`
	Kind         string   `json:"kind" jsonschema:"ad-hoc, periodic or scheduled"`
	NextRunMS    int64    `json:"next_run_ms,omitempty" jsonschema:"when it is next due; absent for ad-hoc and paused ones"`
	Warnings     []string `json:"warnings,omitempty" jsonschema:"what agentbox noticed and did not refuse: unfilled placeholders, knobs the prompt never uses. Fix them before telling the human it is ready"`
}

// createRequest fills only what the caller filled. On create there is nothing
// behind the assignment to preserve, so an empty string is a field the caller
// left alone rather than one it wants blank - and leaving it out lets the
// daemon's own default (enabled, no mode) stand.
func createRequest(in assignCreateIn) (proto.AssignmentSave, error) {
	req := proto.AssignmentSave{Params: in.Params, Enabled: in.Enabled}
	req.Name, req.Prompt = &in.Name, &in.Prompt
	setIfNotEmpty(&req.Description, in.Description)
	setIfNotEmpty(&req.PanelHTML, in.PanelHTML)
	setIfNotEmpty(&req.Model, in.Model)
	setIfNotEmpty(&req.Mode, in.Mode)
	setIfNotEmpty(&req.Dir, in.Dir)
	setIfNotEmpty(&req.Schedule, in.Schedule)
	if in.Spec != nil {
		spec, err := toSpec(in.Spec)
		if err != nil {
			return proto.AssignmentSave{}, err
		}
		req.Spec = &spec
	}
	return req, nil
}

// updateRequest carries the pointers through untouched. Every nil here has to
// stay nil all the way to the daemon: that is what "only the fields you send
// change" is made of, and a well-meant default anywhere along the way would
// overwrite something the human set.
func updateRequest(in assignUpdateIn) (proto.AssignmentSave, error) {
	if in.AssignmentID == "" {
		return proto.AssignmentSave{}, fmt.Errorf("update_assignment needs an assignment_id; create_assignment makes a new one")
	}
	req := proto.AssignmentSave{
		ID: in.AssignmentID, Name: in.Name, Description: in.Description, Prompt: in.Prompt,
		Params: in.Params, PanelHTML: in.PanelHTML, Model: in.Model, Mode: in.Mode,
		Dir: in.Dir, Schedule: in.Schedule, Enabled: in.Enabled,
	}
	if in.Spec != nil {
		spec, err := toSpec(*in.Spec)
		if err != nil {
			return proto.AssignmentSave{}, err
		}
		req.Spec = &spec
	}
	return req, nil
}

func (s *server) assignCreate(ctx context.Context, _ *sdk.CallToolRequest, in assignCreateIn) (*sdk.CallToolResult, assignSaveOut, error) {
	req, err := createRequest(in)
	if err != nil {
		return errResult[assignSaveOut](err)
	}
	return s.assignSave(ctx, req)
}

func (s *server) assignUpdate(ctx context.Context, _ *sdk.CallToolRequest, in assignUpdateIn) (*sdk.CallToolResult, assignSaveOut, error) {
	req, err := updateRequest(in)
	if err != nil {
		return errResult[assignSaveOut](err)
	}
	return s.assignSave(ctx, req)
}

func (s *server) assignSave(ctx context.Context, req proto.AssignmentSave) (*sdk.CallToolResult, assignSaveOut, error) {
	var res proto.AssignmentSaveResult
	if err := s.callIntoFast(ctx, proto.MethodAssignmentSave, req, &res); err != nil {
		return errResult[assignSaveOut](err)
	}
	return &sdk.CallToolResult{}, assignSaveOut{
		AssignmentID: res.ID, Created: res.Created, Kind: res.Kind,
		NextRunMS: res.NextRunMS, Warnings: res.Warnings,
	}, nil
}

// toSpec turns the knobs as the model sent them into typed parameters. The tool
// takes objects rather than the Go type for the reason create_walkthrough takes
// one: a schema derived from the struct would describe pointers and free-form
// defaults in a way a validating client cannot satisfy (FR64). Re-marshalling
// costs one pass and catches a field of the wrong type here, where the error
// can name it, rather than in the store.
func toSpec(raw []map[string]any) ([]assign.Param, error) {
	if len(raw) == 0 {
		return []assign.Param{}, nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("spec does not re-encode: %w", err)
	}
	var spec []assign.Param
	if err := json.Unmarshal(b, &spec); err != nil {
		return nil, fmt.Errorf("spec: %w", err)
	}
	return spec, nil
}

func setIfNotEmpty(dst **string, v string) {
	if v != "" {
		*dst = &v
	}
}

type assignDeleteIn struct {
	AssignmentID string `json:"assignment_id" jsonschema:"the id list_assignments returned"`
}
type assignDeleteOut struct {
	Deleted bool `json:"deleted"`
}

func (s *server) assignDelete(ctx context.Context, _ *sdk.CallToolRequest, in assignDeleteIn) (*sdk.CallToolResult, assignDeleteOut, error) {
	if in.AssignmentID == "" {
		return errResult[assignDeleteOut](fmt.Errorf("delete_assignment needs an assignment_id"))
	}
	var res proto.AssignmentDeleteResult
	if err := s.callIntoFast(ctx, proto.MethodAssignmentDelete, proto.AssignmentDelete{
		ID: in.AssignmentID,
	}, &res); err != nil {
		return errResult[assignDeleteOut](err)
	}
	return &sdk.CallToolResult{}, assignDeleteOut{Deleted: res.Deleted}, nil
}

type assignRunIn struct {
	AssignmentID string         `json:"assignment_id" jsonschema:"the id list_assignments returned"`
	Overrides    map[string]any `json:"overrides,omitempty" jsonschema:"parameter values for this run only; the stored values are untouched"`
}
type assignRunOut struct {
	RunID string `json:"run_id" jsonschema:"pass the assignment_id to assignment_runs to see how it went"`
}

func (s *server) assignRun(ctx context.Context, _ *sdk.CallToolRequest, in assignRunIn) (*sdk.CallToolResult, assignRunOut, error) {
	if in.AssignmentID == "" {
		return errResult[assignRunOut](fmt.Errorf("run_assignment needs an assignment_id; list_assignments finds one"))
	}
	var res proto.AssignmentRunNowResult
	if err := s.callIntoFast(ctx, proto.MethodAssignmentRun, proto.AssignmentRunNow{
		ID: in.AssignmentID, Trigger: "agent", Overrides: in.Overrides,
	}, &res); err != nil {
		return errResult[assignRunOut](err)
	}
	return &sdk.CallToolResult{}, assignRunOut{RunID: res.RunID}, nil
}

type assignRunsIn struct {
	AssignmentID string `json:"assignment_id" jsonschema:"the id list_assignments returned"`
	Limit        int    `json:"limit,omitempty" jsonschema:"rows to return, newest first; default 50"`
}
type assignRunsOut struct {
	Runs []proto.AssignmentRun `json:"runs"`
}

func (s *server) assignRuns(ctx context.Context, _ *sdk.CallToolRequest, in assignRunsIn) (*sdk.CallToolResult, assignRunsOut, error) {
	if in.AssignmentID == "" {
		return errResult[assignRunsOut](fmt.Errorf("assignment_runs needs an assignment_id; list_assignments finds one"))
	}
	var res proto.AssignmentRunsResult
	if err := s.callIntoFast(ctx, proto.MethodAssignmentRuns, proto.AssignmentRuns{
		ID: in.AssignmentID, Limit: in.Limit,
	}, &res); err != nil {
		return errResult[assignRunsOut](err)
	}
	return &sdk.CallToolResult{}, assignRunsOut{Runs: res.Runs}, nil
}
