package proto

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"regexp"
)

// The walkthrough wire types (FR58/FR59). A walkthrough is a durable review:
// the agent submits a declarative spec (steps citing code by path and line
// range, plus the change's unified diff as the manifest), agentbox persists it,
// renders the board, and hands the human's whole review back in one turn.
// The spec's own schema lives in internal/walkthrough; it crosses the wire
// as raw JSON so the daemon is the one place that validates it.

// NewWalkthroughID mints a walkthrough id, caller-side for the same reason
// NewArtifactID is: the agent can await the review the instant create
// returns, and a create retried after a transport timeout is idempotent
// instead of a duplicate.
func NewWalkthroughID() (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "w" + hex.EncodeToString(b[:]), nil
}

var walkthroughIDRe = regexp.MustCompile(`^w[0-9a-f]{12}$`)

// ValidWalkthroughID reports whether id has the caller-minted shape.
func ValidWalkthroughID(id string) bool { return walkthroughIDRe.MatchString(id) }

// WalkthroughCreate stores a new walkthrough and, unless Show is false,
// opens the board. Spec is the version-1 spec object verbatim, diff included;
// the daemon extracts the diff to its own column.
type WalkthroughCreate struct {
	ID       string          `json:"id"`
	Spec     json.RawMessage `json:"spec"`
	NoShow   bool            `json:"no_show,omitempty"`
	Identity Identity        `json:"identity"`
}

// WalkthroughCreateResult reports what was stored and what agentbox computed.
// Warnings carry teaching notes that did not block creation ("no terminal
// check step"); hard failures are RPC errors instead.
type WalkthroughCreateResult struct {
	ID       string         `json:"id"`
	Rev      int            `json:"rev"`
	Coverage CoverageReport `json:"coverage"`
	Warnings []string       `json:"warnings,omitempty"`
}

// CoverageReport is agentbox's own arithmetic over the spec's citations vs its
// diff manifest (FR61: computed, not claimed). UncoveredHunks is always
// present, empty when clean - silence must never read as "covered".
type CoverageReport struct {
	Computed       bool      `json:"computed"`
	Hunks          int       `json:"hunks"`
	Covered        int       `json:"covered"`
	OutOfScope     int       `json:"out_of_scope"`
	Uncovered      int       `json:"uncovered"`
	UncoveredHunks []HunkRef `json:"uncovered_hunks"`
}

// HunkRef names one hunk of the change by its new-file range.
type HunkRef struct {
	Path string `json:"path"`
	From int    `json:"from"`
	To   int    `json:"to"`
	Kind string `json:"kind"` // add | del | mixed
}

// WalkthroughAmend revises a stored walkthrough by step id. Ops apply in
// one transaction; ExpectRev guards against clobbering a spec the caller
// has not seen (the error carries the current rev).
type WalkthroughAmend struct {
	ID        string          `json:"id"`
	ExpectRev int             `json:"expect_rev"`
	Ops       []WalkthroughOp `json:"ops"`
	Identity  Identity        `json:"identity"`
}

// WalkthroughOp is one amendment: replace or add carry a full step object,
// remove and move name a step id, add and move anchor After a step id
// ("" = front).
type WalkthroughOp struct {
	Op     string          `json:"op"` // replace | add | remove | move
	StepID string          `json:"step_id,omitempty"`
	After  string          `json:"after,omitempty"`
	Step   json.RawMessage `json:"step,omitempty"`
}

// WalkthroughAmendResult reports the new revision and which steps now sit
// stale under human marks.
type WalkthroughAmendResult struct {
	Rev           int            `json:"rev"`
	Coverage      CoverageReport `json:"coverage"`
	FlaggedSteps  []string       `json:"flagged_steps"`
	OrphanedSteps []string       `json:"orphaned_steps"`
	Warnings      []string       `json:"warnings,omitempty"`
}

// WalkthroughRead fetches full state without blocking. Ack takes a waiting
// submission - state moves submitted->delivered exactly once - so a next
// session can pick up a review its predecessor never saw.
type WalkthroughRead struct {
	ID  string `json:"id"`
	Ack bool   `json:"ack,omitempty"`
}

// WalkthroughMark is the human's per-step state.
type WalkthroughMark struct {
	StepID   string          `json:"step_id"`
	Verdict  string          `json:"verdict"` // "" | understood | unclear | seen
	Note     string          `json:"note,omitempty"`
	Revealed []int           `json:"revealed,omitempty"`
	CmdRuns  json.RawMessage `json:"cmd_runs,omitempty"`
	Stale    bool            `json:"stale,omitempty"`
}

// WalkthroughComment is one remark, anchored (path+side+lines+exact) or
// step-level (empty path).
type WalkthroughComment struct {
	ID       string `json:"id"`
	StepID   string `json:"step_id"`
	Path     string `json:"path,omitempty"`
	Side     string `json:"side,omitempty"` // new | old
	FromLine int    `json:"from_line,omitempty"`
	ToLine   int    `json:"to_line,omitempty"`
	Exact    string `json:"exact,omitempty"`
	Body     string `json:"body"`
	Adrift   bool   `json:"adrift,omitempty"`
	AtMS     int64  `json:"at_ms"`
}

// WalkthroughState is everything: the spec, the human's half, and the last
// submission if any.
type WalkthroughState struct {
	ID            string               `json:"id"`
	Title         string               `json:"title"`
	RepoRoot      string               `json:"repo_root"`
	Pinned        string               `json:"pinned"`
	Base          string               `json:"base,omitempty"`
	State         string               `json:"state"` // open | submitted | delivered
	Rev           int                  `json:"rev"`
	Spec          json.RawMessage      `json:"spec"`
	Marks         []WalkthroughMark    `json:"marks"`
	Comments      []WalkthroughComment `json:"comments"`
	Coverage      CoverageReport       `json:"coverage"`
	Payload       json.RawMessage      `json:"payload,omitempty"`
	CreatedAtMS   int64                `json:"created_at_ms"`
	UpdatedAtMS   int64                `json:"updated_at_ms"`
	SubmittedAtMS int64                `json:"submitted_at_ms,omitempty"`
}

// WalkthroughAwait blocks until the human submits. An empty ID waits on any
// walkthrough; TimeoutS 0 waits as long as the caller does. Identity names
// the taker in the delivery trail.
type WalkthroughAwait struct {
	ID       string   `json:"id,omitempty"`
	TimeoutS int      `json:"timeout_s,omitempty"`
	Identity Identity `json:"identity,omitzero"`
}

// WalkthroughAwaitResult carries the submission payload whole, or why not.
type WalkthroughAwaitResult struct {
	Submitted bool            `json:"submitted"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	TimedOut  bool            `json:"timed_out,omitempty"`
	Gone      bool            `json:"gone,omitempty"` // deleted while awaited
}

// WalkthroughList filters stored walkthroughs; Query matches title, spec
// content and cited paths.
type WalkthroughList struct {
	Query string `json:"query,omitempty"`
	State string `json:"state,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

// WalkthroughSummary is one library row, enough to recognise a review by
// (FR59): what, where, when, and how far the human got.
type WalkthroughSummary struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	RepoRoot      string `json:"repo_root"`
	Pinned        string `json:"pinned"`
	State         string `json:"state"`
	CountedSteps  int    `json:"counted_steps"`
	Understood    int    `json:"understood"`
	Unclear       int    `json:"unclear"`
	Comments      int    `json:"comments"`
	CreatedAtMS   int64  `json:"created_at_ms"`
	UpdatedAtMS   int64  `json:"updated_at_ms"`
	SubmittedAtMS int64  `json:"submitted_at_ms,omitempty"`
}

// WalkthroughListResult is the library, most recently touched first.
type WalkthroughListResult struct {
	Walkthroughs []WalkthroughSummary `json:"walkthroughs"`
}

// WalkthroughDelete removes a walkthrough and every annotation on it,
// permanently. An agent awaiting it is released with Gone.
type WalkthroughDelete struct {
	ID string `json:"id"`
}

// WalkthroughDeleteResult confirms the removal.
type WalkthroughDeleteResult struct {
	Deleted bool `json:"deleted"`
}

// WalkthroughRepair fills in the source a walkthrough never captured, from the
// blob at its pinned commit. An empty ID repairs every walkthrough that needs
// it - the case this exists for, since anything stored before capture existed
// has the same hole.
type WalkthroughRepair struct {
	ID string `json:"id"`
}

// WalkthroughRepairResult reports what was recovered, one row per walkthrough
// that needed anything.
type WalkthroughRepairResult struct {
	Repaired []WalkthroughRepairRow `json:"repaired"`
}

// WalkthroughRepairRow is one walkthrough's outcome. Notes carries the reason
// per range that could not be recovered, because "3 still missing" without the
// reason is not something anybody can act on.
type WalkthroughRepairRow struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Recovered int      `json:"recovered"`
	AlreadyOK int      `json:"already_captured"`
	Missing   int      `json:"missing"`
	Notes     []string `json:"notes,omitempty"`
}

// WalkthroughOpen (re)shows the board for a stored walkthrough.
type WalkthroughOpen struct {
	ID string `json:"id"`
}
