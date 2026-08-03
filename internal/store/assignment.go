package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/borismilner/agentbox/internal/assign"
)

// Assignment storage (M12 / FR82). The definition round-trips through
// internal/assign, which owns what an assignment MEANS; this file owns only
// where the rows are.
//
// The JSON columns (params_spec, params, data) are marshalled here rather than
// by the caller so there is one place that decides what a malformed blob does.
// The answer is always the same: degrade to the empty value and keep the row
// readable. An assignment whose spec failed to parse must still list, still
// show its prompt and still be editable - the alternative is a row that can
// only be deleted, which is how a stored panel becomes a trap.

var assignmentIDRe = regexp.MustCompile(`^a[0-9a-f]{12}$`)

// NewAssignmentID mints an assignment id.
func NewAssignmentID() string { return mintID("a") }

// NewRunID mints an assignment-run id.
func NewRunID() string { return mintID("r") }

// ValidAssignmentID reports whether id has the minted shape.
func ValidAssignmentID(id string) bool { return assignmentIDRe.MatchString(id) }

func mintID(prefix string) string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not short-read on Linux; a time suffix keeps ids
		// unique rather than colliding on zeros if it ever does.
		return prefix + hex.EncodeToString(fmt.Appendf(nil, "%06x", time.Now().UnixNano()&0xffffff))[:12]
	}
	return prefix + hex.EncodeToString(b[:])
}

// Run is one execution of an assignment.
type Run struct {
	ID           string         `json:"id"`
	AssignmentID string         `json:"assignmentId"`
	StartedMS    int64          `json:"startedMs"`
	EndedMS      int64          `json:"endedMs,omitempty"`
	State        string         `json:"state"`   // running | ok | failed | skipped
	Trigger      string         `json:"trigger"` // schedule | manual | agent
	Params       map[string]any `json:"params,omitempty"`
	Summary      string         `json:"summary,omitempty"`
	Error        string         `json:"error,omitempty"`
	SessionID    string         `json:"sessionId,omitempty"`
	Data         string         `json:"data,omitempty"`
}

// Run states.
const (
	RunRunning = "running"
	RunOK      = "ok"
	RunFailed  = "failed"
	RunSkipped = "skipped"
)

// SaveAssignment inserts or replaces one. The caller owns the id; NextRunMS is
// the scheduler's column and is written as given, so a save from the panel does
// not silently re-arm a schedule the scheduler had already placed.
func (s *Store) SaveAssignment(a *assign.Assignment) error {
	if !ValidAssignmentID(a.ID) {
		return fmt.Errorf("assignment id %q is not the minted shape", a.ID)
	}
	if a.Name == "" {
		return errors.New("an assignment with no name cannot be found again")
	}
	now := time.Now().UnixMilli()
	if a.CreatedMS == 0 {
		a.CreatedMS = now
	}
	a.UpdatedMS = now

	spec, err := json.Marshal(a.Spec)
	if err != nil {
		return fmt.Errorf("params spec: %w", err)
	}
	params, err := json.Marshal(a.Params)
	if err != nil {
		return fmt.Errorf("params: %w", err)
	}
	if a.Params == nil {
		params = []byte("{}")
	}
	_, err = s.db.Exec(`
		INSERT INTO assignments
		  (id, name, description, prompt, params_spec, params, panel_html, model, mode,
		   dir, schedule, enabled, created_ms, updated_ms, last_run_ms, next_run_ms)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
		  name=excluded.name, description=excluded.description, prompt=excluded.prompt,
		  params_spec=excluded.params_spec, params=excluded.params,
		  panel_html=excluded.panel_html, model=excluded.model, mode=excluded.mode,
		  dir=excluded.dir, schedule=excluded.schedule, enabled=excluded.enabled,
		  updated_ms=excluded.updated_ms, last_run_ms=excluded.last_run_ms,
		  next_run_ms=excluded.next_run_ms`,
		a.ID, a.Name, a.Description, a.Prompt, string(spec), string(params),
		a.PanelHTML, a.Model, a.Mode, a.Dir, a.Schedule, boolInt(a.Enabled),
		a.CreatedMS, a.UpdatedMS, a.LastRunMS, a.NextRunMS)
	return err
}

const assignmentCols = `id, name, description, prompt, params_spec, params, panel_html,
	model, mode, dir, schedule, enabled, created_ms, updated_ms, last_run_ms, next_run_ms`

func scanAssignment(sc interface{ Scan(...any) error }) (*assign.Assignment, error) {
	var a assign.Assignment
	var spec, params string
	var enabled int
	if err := sc.Scan(&a.ID, &a.Name, &a.Description, &a.Prompt, &spec, &params,
		&a.PanelHTML, &a.Model, &a.Mode, &a.Dir, &a.Schedule, &enabled,
		&a.CreatedMS, &a.UpdatedMS, &a.LastRunMS, &a.NextRunMS); err != nil {
		return nil, err
	}
	a.Enabled = enabled != 0
	// Both blobs degrade to empty rather than failing the read: a spec an agent
	// wrote badly must leave the assignment editable, not unreachable.
	if err := json.Unmarshal([]byte(spec), &a.Spec); err != nil {
		a.Spec = nil
	}
	if err := json.Unmarshal([]byte(params), &a.Params); err != nil || a.Params == nil {
		a.Params = map[string]any{}
	}
	return &a, nil
}

// GetAssignment reads one. A missing id is (nil, nil), not an error: every
// caller has to handle "gone" anyway, since an agent may delete one mid-edit.
func (s *Store) GetAssignment(id string) (*assign.Assignment, error) {
	row := s.db.QueryRow(`SELECT `+assignmentCols+` FROM assignments WHERE id = ?`, id)
	a, err := scanAssignment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

// ListAssignments returns every assignment, newest first by creation. There is
// no paging: this is a list a person maintains by hand, and if it ever needs
// paging the design was wrong before the query was.
func (s *Store) ListAssignments() ([]*assign.Assignment, error) {
	rows, err := s.db.Query(`SELECT ` + assignmentCols + ` FROM assignments ORDER BY created_ms DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*assign.Assignment{}
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DueAssignments are the enabled, scheduled ones whose next run has arrived.
// Ad-hoc assignments carry next_run_ms = 0 and are excluded by it, which is why
// the scheduler needs no knowledge of the schedule grammar to skip them.
func (s *Store) DueAssignments(now int64) ([]*assign.Assignment, error) {
	rows, err := s.db.Query(`SELECT `+assignmentCols+`
		FROM assignments WHERE enabled = 1 AND next_run_ms > 0 AND next_run_ms <= ?
		ORDER BY next_run_ms`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*assign.Assignment{}
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetAssignmentSchedule writes just the scheduler's own columns, so arming the
// next run never races a concurrent edit of the prompt back over itself.
func (s *Store) SetAssignmentSchedule(id string, lastRunMS, nextRunMS int64) error {
	_, err := s.db.Exec(`UPDATE assignments SET last_run_ms = ?, next_run_ms = ? WHERE id = ?`,
		lastRunMS, nextRunMS, id)
	return err
}

// SetAssignmentParams writes just the values, which is what the panel saves.
func (s *Store) SetAssignmentParams(id string, params map[string]any) error {
	b, err := json.Marshal(params)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE assignments SET params = ?, updated_ms = ? WHERE id = ?`,
		string(b), time.Now().UnixMilli(), id)
	return err
}

// SetAssignmentEnabled is the pause switch. Disabling keeps the definition and
// the history; it is not a soft delete.
func (s *Store) SetAssignmentEnabled(id string, on bool) error {
	_, err := s.db.Exec(`UPDATE assignments SET enabled = ?, updated_ms = ? WHERE id = ?`,
		boolInt(on), time.Now().UnixMilli(), id)
	return err
}

// DeleteAssignment removes it and its whole run history (ON DELETE CASCADE).
func (s *Store) DeleteAssignment(id string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM assignments WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// StartRun records a run beginning and returns its id.
func (s *Store) StartRun(r *Run) error {
	if r.ID == "" {
		r.ID = NewRunID()
	}
	if r.StartedMS == 0 {
		r.StartedMS = time.Now().UnixMilli()
	}
	if r.State == "" {
		r.State = RunRunning
	}
	params, err := json.Marshal(r.Params)
	if err != nil || r.Params == nil {
		params = []byte("{}")
	}
	_, err = s.db.Exec(`INSERT INTO assignment_runs
		  (id, assignment_id, started_ms, ended_ms, state, trigger, params, summary, error, session_id, data)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.AssignmentID, r.StartedMS, r.EndedMS, r.State, r.Trigger,
		string(params), r.Summary, r.Error, r.SessionID, r.Data)
	return err
}

// FinishRun closes a run out. Both the summary and the data blob are written
// here rather than incrementally: a run that dies mid-way leaves a `running`
// row with the truth (it never finished) instead of a half-written result that
// reads like one.
func (s *Store) FinishRun(id, state, summary, errMsg, data string) error {
	_, err := s.db.Exec(`UPDATE assignment_runs
		SET ended_ms = ?, state = ?, summary = ?, error = ?, data = COALESCE(NULLIF(?, ''), data)
		WHERE id = ?`,
		time.Now().UnixMilli(), state, summary, errMsg, data, id)
	return err
}

// SetRunSession links a run to the conversation it is being carried out in, so
// the panel can offer to open it while it runs.
func (s *Store) SetRunSession(id, sessionID string) error {
	_, err := s.db.Exec(`UPDATE assignment_runs SET session_id = ? WHERE id = ?`, sessionID, id)
	return err
}

// RunsFor returns an assignment's runs, newest first.
func (s *Store) RunsFor(assignmentID string, limit int) ([]Run, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, assignment_id, started_ms, ended_ms, state, trigger,
		  params, summary, error, session_id, data
		FROM assignment_runs WHERE assignment_id = ? ORDER BY started_ms DESC LIMIT ?`,
		assignmentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		var r Run
		var params string
		if err := rows.Scan(&r.ID, &r.AssignmentID, &r.StartedMS, &r.EndedMS, &r.State,
			&r.Trigger, &params, &r.Summary, &r.Error, &r.SessionID, &r.Data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(params), &r.Params); err != nil {
			r.Params = nil
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReapRunningRuns marks every run still flagged `running` as failed. Called at
// startup: a run is a child process, so one that was running when the daemon
// died is not running now, and a row that says otherwise makes the panel claim
// work is in progress forever.
func (s *Store) ReapRunningRuns(reason string) (int, error) {
	res, err := s.db.Exec(`UPDATE assignment_runs SET state = ?, error = ?, ended_ms = ?
		WHERE state = ?`, RunFailed, reason, time.Now().UnixMilli(), RunRunning)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}
