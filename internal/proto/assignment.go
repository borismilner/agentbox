package proto

import "github.com/borismilner/agentbox/internal/assign"

// The assignment wire types (M12 / FR82). An assignment is work agentbox gives an
// agent on a schedule or on demand; docs/08-assignments.md is the design.
//
// The CRUD is an MCP surface before it is a UI, because Boris asked for the
// agent to author assignments with him ("upon creation, the AI agent itself
// should help with generating the initial prompt and the configuration panel
// for it until the user is satisfied"). These types are therefore written for a
// model as much as for the panel: the save reports every problem with a spec at
// once, and distinguishes what it refused from what it merely warned about.

// AssignmentList asks for every assignment. There is no filter and no paging -
// this is a list a person maintains by hand.
type AssignmentList struct{}

// AssignmentSummary is one row: enough to recognise an assignment, decide
// whether it is healthy, and pick one to read in full.
type AssignmentSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind"` // ad-hoc | periodic | scheduled
	Schedule    string `json:"schedule,omitempty"`
	Enabled     bool   `json:"enabled"`
	Model       string `json:"model,omitempty"`
	Dir         string `json:"dir,omitempty"`
	Running     bool   `json:"running,omitempty"`
	LastRunMS   int64  `json:"last_run_ms,omitempty"`
	NextRunMS   int64  `json:"next_run_ms,omitempty"`
	LastState   string `json:"last_state,omitempty"`   // the newest run's state
	LastSummary string `json:"last_summary,omitempty"` // and what it reported
}

// AssignmentListResult is the whole list, newest first.
type AssignmentListResult struct {
	Assignments []AssignmentSummary `json:"assignments"`
}

// AssignmentRead fetches one definition whole. Runs caps the history returned
// with it; 0 means a small default, since an agent reading a definition
// usually wants to know how the last few went and never wants fifty.
type AssignmentRead struct {
	ID   string `json:"id"`
	Runs int    `json:"runs,omitempty"`
}

// AssignmentReadResult is the definition plus everything agentbox can work out about
// it that the row does not store. Placeholders and Problems are here so an
// agent asked to improve an assignment sees the same diagnosis the save would
// give it, before it writes anything.
type AssignmentReadResult struct {
	Assignment   *assign.Assignment `json:"assignment"`
	Kind         string             `json:"kind"`
	Running      bool               `json:"running"`
	Placeholders []string           `json:"placeholders,omitempty"` // {{keys}} the prompt refers to
	Unfilled     []string           `json:"unfilled,omitempty"`     // ...that no parameter fills
	Unused       []string           `json:"unused,omitempty"`       // parameters no placeholder reads
	Problems     []string           `json:"problems,omitempty"`     // spec faults, if the stored spec has any
	Runs         []AssignmentRun    `json:"runs,omitempty"`
}

// AssignmentSave creates (empty ID) or updates one. Every field is a pointer
// because an update must be able to change one thing: an agent rewriting a
// prompt has no business resending the schedule, and a plain string field
// cannot tell "leave it" from "make it empty". Params is the exception - a map
// is already absent-or-present, and its values are merged over what is stored
// rather than replacing them, so setting one knob does not clear the rest.
type AssignmentSave struct {
	ID          string          `json:"id,omitempty"`
	Name        *string         `json:"name,omitempty"`
	Description *string         `json:"description,omitempty"`
	Prompt      *string         `json:"prompt,omitempty"`
	Spec        *[]assign.Param `json:"spec,omitempty"`
	Params      map[string]any  `json:"params,omitempty"`
	PanelHTML   *string         `json:"panel_html,omitempty"`
	Model       *string         `json:"model,omitempty"`
	Mode        *string         `json:"mode,omitempty"`
	Dir         *string         `json:"dir,omitempty"`
	Schedule    *string         `json:"schedule,omitempty"`
	Enabled     *bool           `json:"enabled,omitempty"`
}

// AssignmentSaveResult reports what was stored and what agentbox noticed about it.
// Warnings never block a save (an unfilled placeholder is a prompt half
// written, which is a normal state to save in); anything that would make the
// assignment wrong rather than unfinished is an RPC error instead.
type AssignmentSaveResult struct {
	ID         string             `json:"id"`
	Created    bool               `json:"created"`
	Kind       string             `json:"kind"`
	NextRunMS  int64              `json:"next_run_ms,omitempty"`
	Warnings   []string           `json:"warnings,omitempty"`
	Assignment *assign.Assignment `json:"assignment,omitempty"`
}

// AssignmentDelete removes one and its whole run history.
type AssignmentDelete struct {
	ID string `json:"id"`
}

// AssignmentDeleteResult confirms the removal.
type AssignmentDeleteResult struct {
	Deleted bool `json:"deleted"`
}

// AssignmentRunNow starts a run outside the schedule. Overrides replace stored
// parameter values for this run only, which is what makes "try it with the
// threshold at 95" possible without editing the assignment.
type AssignmentRunNow struct {
	ID        string         `json:"id"`
	Trigger   string         `json:"trigger,omitempty"` // manual | agent; default agent
	Overrides map[string]any `json:"overrides,omitempty"`
}

// AssignmentRunNowResult names the run that started. It returns as soon as the
// run is recorded, not when it finishes: a run is a whole conversation, and an
// agent that asked for one should not be held for the minutes it takes.
type AssignmentRunNowResult struct {
	RunID string `json:"run_id"`
}

// AssignmentRuns is the history of one assignment, newest first.
type AssignmentRuns struct {
	ID    string `json:"id"`
	Limit int    `json:"limit,omitempty"`
}

// AssignmentRun is one execution. It mirrors store.Run on the wire so the
// transport does not drag the store into every package that reads a run.
type AssignmentRun struct {
	ID        string         `json:"id"`
	StartedMS int64          `json:"started_ms"`
	EndedMS   int64          `json:"ended_ms,omitempty"`
	State     string         `json:"state"`   // running | ok | failed | skipped
	Trigger   string         `json:"trigger"` // schedule | manual | agent
	Params    map[string]any `json:"params,omitempty"`
	Summary   string         `json:"summary,omitempty"`
	Error     string         `json:"error,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	Data      string         `json:"data,omitempty"`
}

// AssignmentRunsResult is the history.
type AssignmentRunsResult struct {
	Runs []AssignmentRun `json:"runs"`
}
