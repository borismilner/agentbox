package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
)

// Walkthrough states (FR58/FR59). A submission parks at submitted until
// exactly one agent takes it; delivered means the review reached its reader.
const (
	WtOpen      = "open"
	WtSubmitted = "submitted"
	WtDelivered = "delivered"
)

var ErrWalkthroughNotFound = errors.New("walkthrough not found")

// ErrSpecMismatch means a create retried an existing id with different
// content - a real conflict, not a lost-ack retry.
var ErrSpecMismatch = errors.New("walkthrough id exists with a different spec")

// Walkthrough is the stored object: the agent's half plus lifecycle state.
// The human's half lives in marks and comments.
type Walkthrough struct {
	ID           string
	Title        string
	RepoRoot     string
	PinnedSHA    string
	BaseSHA      string
	ChangeKey    string
	Spec         string // version-1 spec JSON, diff stripped
	SpecRev      int
	Diff         string
	CountedSteps int
	Pos          int
	State        string
	Identity     proto.Identity
	Payload      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	SubmittedAt  time.Time
	// The cited source as it was when the review was written. Loaded with the
	// walkthrough because everything that renders one needs it; empty for
	// anything stored before capture existed, which is what `repair` is for.
	Excerpts []Excerpt
}

// Mark is the human's state on one step.
type Mark struct {
	StepID    string
	Verdict   string
	Note      string
	Revealed  []int
	CmdRuns   json.RawMessage
	StepHash  string
	Stale     bool
	UpdatedAt time.Time
}

// Excerpt is one cited range, as the file read when the review was written.
// It is what makes a walkthrough readable after the working tree has moved on:
// the citation says which lines, this says what was on them.
type Excerpt struct {
	Path     string
	FromLine int
	ToLine   int
	Text     string
	Source   string // worktree | git
}

// Excerpt sources, which are not equally trustworthy: the working tree is what
// the authoring agent had in front of it; git is the blob at the pinned commit,
// recovered later for a review that was stored before any of this existed.
const (
	ExcerptWorktree = "worktree"
	ExcerptGit      = "git"
)

// Comment is one remark, anchored or step-level (empty Path).
type Comment struct {
	ID        string
	StepID    string
	Path      string
	Side      string
	FromLine  int
	ToLine    int
	Exact     string
	Body      string
	Adrift    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateWalkthrough persists a new walkthrough in state open. A retry with
// the same id and identical spec+diff succeeds silently (the first create
// won a race with a transport timeout); different content is refused.
func (s *Store) CreateWalkthrough(w *Walkthrough) error {
	if w.ID == "" {
		return errors.New("walkthrough has no ID")
	}
	now := time.Now().UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var spec, diff string
	err = tx.QueryRow(`SELECT spec, diff FROM walkthroughs WHERE id = ?`, w.ID).Scan(&spec, &diff)
	switch {
	case err == nil:
		if spec == w.Spec && diff == w.Diff {
			return nil // idempotent retry
		}
		return fmt.Errorf("%w: %s", ErrSpecMismatch, w.ID)
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("check walkthrough %s: %w", w.ID, err)
	}
	if _, err := tx.Exec(`INSERT INTO walkthroughs
		(id, title, repo_root, pinned_sha, base_sha, change_key, spec, spec_rev, diff, counted_steps,
		 pos, state, agent, project, session, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?, 0, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Title, w.RepoRoot, w.PinnedSHA, w.BaseSHA, w.ChangeKey, w.Spec, w.Diff, w.CountedSteps,
		WtOpen, w.Identity.Agent, w.Identity.Project, w.Identity.Session, now, now); err != nil {
		return fmt.Errorf("insert walkthrough %s: %w", w.ID, err)
	}
	if _, err := tx.Exec(`INSERT INTO walkthrough_transitions (walkthrough_id, from_state, to_state, at)
		VALUES (?, '', ?, ?)`, w.ID, WtOpen, now); err != nil {
		return fmt.Errorf("record creation of %s: %w", w.ID, err)
	}
	return tx.Commit()
}

// GetWalkthrough loads the stored object, without marks or comments.
func (s *Store) GetWalkthrough(id string) (*Walkthrough, error) {
	var w Walkthrough
	var payload sql.NullString
	var created, updated int64
	var submitted sql.NullInt64
	err := s.db.QueryRow(`SELECT id, title, repo_root, pinned_sha, base_sha, change_key, spec, spec_rev,
		diff, counted_steps, pos, state, agent, project, session, payload, created_at, updated_at, submitted_at
		FROM walkthroughs WHERE id = ?`, id).Scan(
		&w.ID, &w.Title, &w.RepoRoot, &w.PinnedSHA, &w.BaseSHA, &w.ChangeKey, &w.Spec, &w.SpecRev,
		&w.Diff, &w.CountedSteps, &w.Pos, &w.State, &w.Identity.Agent, &w.Identity.Project,
		&w.Identity.Session, &payload, &created, &updated, &submitted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrWalkthroughNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get walkthrough %s: %w", id, err)
	}
	w.Payload = payload.String
	w.CreatedAt = time.UnixMilli(created)
	w.UpdatedAt = time.UnixMilli(updated)
	if submitted.Valid {
		w.SubmittedAt = time.UnixMilli(submitted.Int64)
	}
	if w.Excerpts, err = s.ExcerptsFor(id); err != nil {
		return nil, err
	}
	return &w, nil
}

// SaveExcerpts replaces a walkthrough's captured source. Replace rather than
// merge: a repair that finds better text for a range must win over the miss it
// is repairing, and a partial capture must never leave half of an older one
// behind to be read as if it belonged.
func (s *Store) SaveExcerpts(wtID string, ex []Excerpt) error {
	if wtID == "" {
		return errors.New("no walkthrough id")
	}
	now := time.Now().UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM walkthrough_excerpts WHERE walkthrough_id = ?`, wtID); err != nil {
		return fmt.Errorf("clear excerpts for %s: %w", wtID, err)
	}
	for _, e := range ex {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO walkthrough_excerpts
			(walkthrough_id, path, from_line, to_line, text, source, captured_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			wtID, e.Path, e.FromLine, e.ToLine, e.Text, e.Source, now); err != nil {
			return fmt.Errorf("save excerpt %s:%d-%d: %w", e.Path, e.FromLine, e.ToLine, err)
		}
	}
	return tx.Commit()
}

// ExcerptsFor loads what was captured for a walkthrough. An empty result is
// the normal answer for anything created before this existed, and the board
// falls back to reading the working tree for those.
func (s *Store) ExcerptsFor(wtID string) ([]Excerpt, error) {
	rows, err := s.db.Query(`SELECT path, from_line, to_line, text, source
		FROM walkthrough_excerpts WHERE walkthrough_id = ? ORDER BY path, from_line`, wtID)
	if err != nil {
		return nil, fmt.Errorf("excerpts for %s: %w", wtID, err)
	}
	defer rows.Close()
	var out []Excerpt
	for rows.Next() {
		var e Excerpt
		if err := rows.Scan(&e.Path, &e.FromLine, &e.ToLine, &e.Text, &e.Source); err != nil {
			return nil, fmt.Errorf("scan excerpt: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListWalkthroughs returns library rows, most recently touched first
// (FR59). query matches title, spec content and the diff (cited paths live
// in both); state filters when non-empty.
func (s *Store) ListWalkthroughs(query, state string, limit int) ([]proto.WalkthroughSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	q := `SELECT w.id, w.title, w.repo_root, w.pinned_sha, w.state, w.counted_steps,
		(SELECT COUNT(*) FROM walkthrough_marks m WHERE m.walkthrough_id = w.id AND m.verdict = 'understood'),
		(SELECT COUNT(*) FROM walkthrough_marks m WHERE m.walkthrough_id = w.id AND m.verdict = 'unclear'),
		(SELECT COUNT(*) FROM walkthrough_comments c WHERE c.walkthrough_id = w.id),
		w.created_at, w.updated_at, w.submitted_at
		FROM walkthroughs w WHERE 1=1`
	var args []any
	if state != "" {
		q += ` AND w.state = ?`
		args = append(args, state)
	}
	if query != "" {
		q += ` AND (w.title LIKE ? OR w.spec LIKE ? OR w.diff LIKE ?)`
		pat := "%" + query + "%"
		args = append(args, pat, pat, pat)
	}
	q += ` ORDER BY w.updated_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list walkthroughs: %w", err)
	}
	defer rows.Close()
	var out []proto.WalkthroughSummary
	for rows.Next() {
		var r proto.WalkthroughSummary
		var submitted sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Title, &r.RepoRoot, &r.Pinned, &r.State, &r.CountedSteps,
			&r.Understood, &r.Unclear, &r.Comments, &r.CreatedAtMS, &r.UpdatedAtMS, &submitted); err != nil {
			return nil, err
		}
		r.SubmittedAtMS = submitted.Int64
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteWalkthrough removes a walkthrough and, by cascade, every annotation
// and transition on it. Permanent, from any state.
func (s *Store) DeleteWalkthrough(id string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM walkthroughs WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("delete walkthrough %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// touch bumps a walkthrough's updated_at inside the caller's transaction and
// doubles as the existence check for annotation writes.
func touch(tx *sql.Tx, id string, now int64) error {
	res, err := tx.Exec(`UPDATE walkthroughs SET updated_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrWalkthroughNotFound, id)
	}
	return nil
}

// SetVerdict records the human's verdict on a step. stepHash fingerprints
// the spec step being judged; a fresh verdict clears staleness because the
// human has now seen the step as it is.
func (s *Store) SetVerdict(wtID, stepID, verdict, stepHash string) error {
	now := time.Now().UnixMilli()
	return s.upsertMark(wtID, stepID, now, `
		INSERT INTO walkthrough_marks (walkthrough_id, step_id, verdict, step_hash, stale, updated_at)
		VALUES (?, ?, ?, ?, 0, ?)
		ON CONFLICT (walkthrough_id, step_id) DO UPDATE SET
			verdict = excluded.verdict, step_hash = excluded.step_hash, stale = 0, updated_at = excluded.updated_at`,
		wtID, stepID, verdict, stepHash, now)
}

// SetNote records the step's closing note.
func (s *Store) SetNote(wtID, stepID, note string) error {
	now := time.Now().UnixMilli()
	return s.upsertMark(wtID, stepID, now, `
		INSERT INTO walkthrough_marks (walkthrough_id, step_id, note, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (walkthrough_id, step_id) DO UPDATE SET
			note = excluded.note, updated_at = excluded.updated_at`,
		wtID, stepID, note, now)
}

// SetRevealed records which check answers are open - part of the step's
// state ("whether they were answered is part of the step's state", FR58).
func (s *Store) SetRevealed(wtID, stepID string, revealed []int) error {
	raw, err := json.Marshal(revealed)
	if err != nil {
		return err
	}
	if revealed == nil {
		raw = []byte("[]")
	}
	now := time.Now().UnixMilli()
	return s.upsertMark(wtID, stepID, now, `
		INSERT INTO walkthrough_marks (walkthrough_id, step_id, revealed, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (walkthrough_id, step_id) DO UPDATE SET
			revealed = excluded.revealed, updated_at = excluded.updated_at`,
		wtID, stepID, string(raw), now)
}

func (s *Store) upsertMark(wtID, stepID string, now int64, q string, args ...any) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := touch(tx, wtID, now); err != nil {
		return err
	}
	if _, err := tx.Exec(q, args...); err != nil {
		return fmt.Errorf("mark %s/%s: %w", wtID, stepID, err)
	}
	return tx.Commit()
}

// SetPos records where the human is, so a reopened review resumes there.
func (s *Store) SetPos(wtID string, pos int) error {
	res, err := s.db.Exec(`UPDATE walkthroughs SET pos = ?, updated_at = ? WHERE id = ?`,
		pos, time.Now().UnixMilli(), wtID)
	if err != nil {
		return fmt.Errorf("set pos %s: %w", wtID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrWalkthroughNotFound, wtID)
	}
	return nil
}

// MarksFor loads the human's per-step state.
func (s *Store) MarksFor(wtID string) ([]Mark, error) {
	rows, err := s.db.Query(`SELECT step_id, verdict, note, revealed, cmd_runs, step_hash, stale, updated_at
		FROM walkthrough_marks WHERE walkthrough_id = ? ORDER BY step_id`, wtID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Mark
	for rows.Next() {
		var m Mark
		var revealed, cmdRuns string
		var stale int
		var updated int64
		if err := rows.Scan(&m.StepID, &m.Verdict, &m.Note, &revealed, &cmdRuns, &m.StepHash, &stale, &updated); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(revealed), &m.Revealed); err != nil {
			return nil, fmt.Errorf("walkthrough %s mark %s has corrupt revealed: %w", wtID, m.StepID, err)
		}
		m.CmdRuns = json.RawMessage(cmdRuns)
		m.Stale = stale != 0
		m.UpdatedAt = time.UnixMilli(updated)
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddComment stores a remark. The id is the daemon's to mint.
func (s *Store) AddComment(wtID string, c *Comment) error {
	if c.ID == "" {
		return errors.New("comment has no ID")
	}
	now := time.Now().UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := touch(tx, wtID, now); err != nil {
		return err
	}
	side := c.Side
	if side == "" {
		side = "new"
	}
	if _, err := tx.Exec(`INSERT INTO walkthrough_comments
		(id, walkthrough_id, step_id, path, side, from_line, to_line, exact, body, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, wtID, c.StepID, c.Path, side, c.FromLine, c.ToLine, c.Exact, c.Body, now, now); err != nil {
		return fmt.Errorf("insert comment %s: %w", c.ID, err)
	}
	return tx.Commit()
}

// EditComment replaces a comment's body.
func (s *Store) EditComment(id, body string) error {
	res, err := s.db.Exec(`UPDATE walkthrough_comments SET body = ?, updated_at = ? WHERE id = ?`,
		body, time.Now().UnixMilli(), id)
	if err != nil {
		return fmt.Errorf("edit comment %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("comment %s: %w", id, ErrNotFound)
	}
	return nil
}

// DeleteComment removes a remark.
func (s *Store) DeleteComment(id string) error {
	_, err := s.db.Exec(`DELETE FROM walkthrough_comments WHERE id = ?`, id)
	return err
}

// CommentsFor loads a walkthrough's comments, oldest first.
func (s *Store) CommentsFor(wtID string) ([]Comment, error) {
	rows, err := s.db.Query(`SELECT id, step_id, path, side, from_line, to_line, exact, body, adrift, created_at, updated_at
		FROM walkthrough_comments WHERE walkthrough_id = ? ORDER BY created_at ASC`, wtID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Comment
	for rows.Next() {
		var c Comment
		var adrift int
		var created, updated int64
		if err := rows.Scan(&c.ID, &c.StepID, &c.Path, &c.Side, &c.FromLine, &c.ToLine, &c.Exact, &c.Body, &adrift, &created, &updated); err != nil {
			return nil, err
		}
		c.Adrift = adrift != 0
		c.CreatedAt = time.UnixMilli(created)
		c.UpdatedAt = time.UnixMilli(updated)
		out = append(out, c)
	}
	return out, rows.Err()
}

// SubmitWalkthrough records a submission: the assembled payload, state
// submitted, the receipt timestamp. Legal from open (first submission),
// submitted (resubmit replaces the unread payload) and delivered (the human
// said more after the agent read the last one).
func (s *Store) SubmitWalkthrough(id, payload string) error {
	now := time.Now().UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var from string
	if err := tx.QueryRow(`SELECT state FROM walkthroughs WHERE id = ?`, id).Scan(&from); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrWalkthroughNotFound, id)
		}
		return err
	}
	if _, err := tx.Exec(`UPDATE walkthroughs SET state = ?, payload = ?, submitted_at = ?, updated_at = ? WHERE id = ?`,
		WtSubmitted, payload, now, now, id); err != nil {
		return fmt.Errorf("submit %s: %w", id, err)
	}
	detail := ""
	if from == WtSubmitted {
		detail = "resubmitted"
	}
	if _, err := tx.Exec(`INSERT INTO walkthrough_transitions (walkthrough_id, from_state, to_state, detail, at)
		VALUES (?, ?, ?, ?, ?)`, id, from, WtSubmitted, detail, now); err != nil {
		return fmt.Errorf("record submission of %s: %w", id, err)
	}
	return tx.Commit()
}

// DeliverWalkthrough marks a waiting submission as taken, exactly once: the
// guarded UPDATE means two agents asking at the same instant cannot both
// win (the store.Resolve pattern). It returns the payload on the winning
// call and ErrNotFound on the losing one.
func (s *Store) DeliverWalkthrough(id, agent string) (string, error) {
	now := time.Now().UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE walkthroughs SET state = ?, updated_at = ? WHERE id = ? AND state = ?`,
		WtDelivered, now, id, WtSubmitted)
	if err != nil {
		return "", fmt.Errorf("deliver %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", fmt.Errorf("deliver %s: %w", id, ErrNotFound)
	}
	var payload sql.NullString
	if err := tx.QueryRow(`SELECT payload FROM walkthroughs WHERE id = ?`, id).Scan(&payload); err != nil {
		return "", err
	}
	if _, err := tx.Exec(`INSERT INTO walkthrough_transitions (walkthrough_id, from_state, to_state, detail, at)
		VALUES (?, ?, ?, ?, ?)`, id, WtSubmitted, WtDelivered, "delivered to "+agent, now); err != nil {
		return "", fmt.Errorf("record delivery of %s: %w", id, err)
	}
	return payload.String, tx.Commit()
}

// WalkthroughTransitions returns the audit trail, oldest first.
func (s *Store) WalkthroughTransitions(id string) ([]string, error) {
	rows, err := s.db.Query(`SELECT from_state, to_state, detail FROM walkthrough_transitions
		WHERE walkthrough_id = ? ORDER BY seq`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var from, to, detail string
		if err := rows.Scan(&from, &to, &detail); err != nil {
			return nil, err
		}
		if from == "" {
			from = "(new)"
		}
		line := from + " -> " + to
		if detail != "" {
			line += " (" + detail + ")"
		}
		out = append(out, line)
	}
	return out, rows.Err()
}
