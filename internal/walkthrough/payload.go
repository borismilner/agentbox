package walkthrough

// The submission payload (FR58): the human's whole review, assembled once at
// submit and handed to the agent in one turn. The shape is written for a
// model reader - the unclear steps lead as a headline set, understood-but-
// silent is marked rather than blank, and not_reviewed is always present so
// silence can never read as "reviewed" (FR61's honesty rule applied to the
// handback).

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/borismilner/agentbox/internal/proto"
)

// GateError refuses a submission that would ship an unclear verdict without
// words. It names the step so the board can jump to it.
type GateError struct {
	StepID string
	Title  string
}

func (e *GateError) Error() string {
	return fmt.Sprintf("step %q (%s) is marked unclear but its note is empty - unclear is a question for the agent, and a question needs words; write what is unclear, or change the verdict", e.StepID, e.Title)
}

// Payload is submission format version 1. Field order is the reading order:
// the unclear headline set comes before the full walk.
type Payload struct {
	Version          int                  `json:"version"`
	WalkthroughID    string               `json:"walkthrough_id"`
	Title            string               `json:"title"`
	RepoRoot         string               `json:"repo_root"`
	Pinned           string               `json:"pinned"`
	SpecRev          int                  `json:"spec_rev"`
	SubmittedAtMS    int64                `json:"submitted_at_ms"`
	Tally            Tally                `json:"tally"`
	Unclear          []Headline           `json:"unclear"`
	Steps            []PayloadStep        `json:"steps"`
	NotReviewed      []string             `json:"not_reviewed"`
	Coverage         proto.CoverageReport `json:"coverage"`
	Drift            Drift                `json:"drift"`
	OrphanedComments []PayloadComment     `json:"orphaned_comments"`
}

// Tally is the one-line summary. NotReviewed counts code steps only - the
// steps the review's promise counts.
type Tally struct {
	Understood  int `json:"understood"`
	Unclear     int `json:"unclear"`
	NotReviewed int `json:"not_reviewed"`
	Comments    int `json:"comments"`
}

// Headline is one unclear step, pulled to the front: answer these first.
type Headline struct {
	StepID string `json:"step_id"`
	Title  string `json:"title"`
	Note   string `json:"note"`
}

// PayloadStep is the human's state on one step, in walk order. Verdict is
// null when the step was never judged - null and "" must stay
// distinguishable from a deliberate mark.
type PayloadStep struct {
	StepID       string           `json:"step_id"`
	Kind         string           `json:"kind"`
	Verdict      *string          `json:"verdict"`
	VerdictStale bool             `json:"verdict_stale,omitempty"`
	Note         string           `json:"note,omitempty"`
	Unsaid       bool             `json:"unsaid,omitempty"` // understood with no words (allowed; marked)
	Comments     []PayloadComment `json:"comments,omitempty"`
	Checks       []PayloadCheck   `json:"checks,omitempty"`
	CmdRuns      json.RawMessage  `json:"cmd_runs,omitempty"`
}

// PayloadComment is one remark. StepID is set only under orphaned_comments,
// where the parent step no longer says it.
type PayloadComment struct {
	StepID string `json:"step_id,omitempty"`
	Path   string `json:"path,omitempty"`
	Side   string `json:"side,omitempty"`
	From   int    `json:"from,omitempty"`
	To     int    `json:"to,omitempty"`
	Exact  string `json:"exact,omitempty"`
	Text   string `json:"text"`
	AtMS   int64  `json:"at_ms"`
	Adrift bool   `json:"adrift,omitempty"`
}

// PayloadCheck reports whether the human opened a check's answer - part of
// how far the step was actually read.
type PayloadCheck struct {
	Q        string `json:"q"`
	Revealed bool   `json:"revealed"`
}

// Drift is the citation-vs-repository arithmetic. Computed stays false until
// the coverage slice lands; the fields are present so the shape is stable
// and absence never has to be interpreted.
type Drift struct {
	Computed bool     `json:"computed"`
	Head     string   `json:"head,omitempty"`
	Moved    []string `json:"moved"`
	Stale    []string `json:"stale"`
}

// Submission is what the payload needs beyond the spec and the marks: the
// stored walkthrough's identity row and the submission instant.
type Submission struct {
	ID       string
	Title    string
	RepoRoot string
	Pinned   string
	SpecRev  int
	NowMS    int64
}

// BuildPayload assembles the handback from the spec and the human's marks
// and comments. It enforces the hard gate: an unclear verdict with an empty
// note refuses the whole submission and names the step. A review with no
// verdicts at all builds fine - whether to allow that is the surface's
// explicit-confirm decision, not a payload rule.
func BuildPayload(sub Submission, spec *Spec, marks []proto.WalkthroughMark, comments []proto.WalkthroughComment) (*Payload, error) {
	byStep := make(map[string]*proto.WalkthroughMark, len(marks))
	for i := range marks {
		byStep[marks[i].StepID] = &marks[i]
	}
	known := make(map[string]bool, len(spec.Steps))
	for i := range spec.Steps {
		known[spec.Steps[i].ID] = true
	}

	p := &Payload{
		Version:          1,
		WalkthroughID:    sub.ID,
		Title:            sub.Title,
		RepoRoot:         sub.RepoRoot,
		Pinned:           sub.Pinned,
		SpecRev:          sub.SpecRev,
		SubmittedAtMS:    sub.NowMS,
		Unclear:          []Headline{},
		Steps:            make([]PayloadStep, 0, len(spec.Steps)),
		NotReviewed:      []string{},
		Coverage:         proto.CoverageReport{UncoveredHunks: []proto.HunkRef{}},
		Drift:            Drift{Moved: []string{}, Stale: []string{}},
		OrphanedComments: []PayloadComment{},
	}
	p.Tally.Comments = len(comments)

	for i := range spec.Steps {
		st := &spec.Steps[i]
		m := byStep[st.ID]
		ps := PayloadStep{StepID: st.ID, Kind: st.Kind}
		note := ""
		if m != nil {
			note = strings.TrimSpace(m.Note)
			ps.Note = note
			ps.VerdictStale = m.Stale
			ps.CmdRuns = m.CmdRuns
			if m.Verdict != "" {
				v := m.Verdict
				ps.Verdict = &v
			}
		}
		switch {
		case ps.Verdict != nil && *ps.Verdict == "unclear":
			if note == "" {
				return nil, &GateError{StepID: st.ID, Title: st.Title}
			}
			p.Tally.Unclear++
			p.Unclear = append(p.Unclear, Headline{StepID: st.ID, Title: st.Title, Note: note})
		case ps.Verdict != nil && *ps.Verdict == "understood":
			p.Tally.Understood++
			ps.Unsaid = note == ""
		case st.Kind == "code":
			// "" and "seen" both mean the step was never judged.
			p.Tally.NotReviewed++
			p.NotReviewed = append(p.NotReviewed, st.ID)
		}
		for _, c := range comments {
			if c.StepID != st.ID {
				continue
			}
			ps.Comments = append(ps.Comments, PayloadComment{
				Path: c.Path, Side: c.Side, From: c.FromLine, To: c.ToLine,
				Exact: c.Exact, Text: c.Body, AtMS: c.AtMS, Adrift: c.Adrift,
			})
		}
		for ci, ch := range st.Checks {
			revealed := m != nil && slices.Contains(m.Revealed, ci)
			ps.Checks = append(ps.Checks, PayloadCheck{Q: ch.Q, Revealed: revealed})
		}
		p.Steps = append(p.Steps, ps)
	}

	for _, c := range comments {
		if known[c.StepID] {
			continue
		}
		p.OrphanedComments = append(p.OrphanedComments, PayloadComment{
			StepID: c.StepID, Path: c.Path, Side: c.Side, From: c.FromLine, To: c.ToLine,
			Exact: c.Exact, Text: c.Body, AtMS: c.AtMS, Adrift: c.Adrift,
		})
	}
	return p, nil
}
