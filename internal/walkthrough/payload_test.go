package walkthrough

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/borismilner/agentbox/internal/proto"
)

func payloadSpec(t *testing.T) *Spec {
	t.Helper()
	spec, _, err := Parse([]byte(`{
		"version": 1, "title": "the walk", "repo_root": "/repo", "pinned": "dd375a3cb2c7",
		"steps": [
			{"id": "ground", "kind": "ground", "title": "Ground", "prose": [{"t": "context"}]},
			{"id": "one", "kind": "code", "title": "First", "purpose": "Serves: x.",
			 "tldr": {"bottom": "The one line that survives.", "points": ["A fact that stands alone."]},
			 "prose": [{"t": "read this"}],
			 "code": [{"path": "a/b.go", "lines": [1, 5]}],
			 "checks": [{"q": "why?", "a": "because"}, {"q": "how?", "a": "so"}]},
			{"id": "two", "kind": "code", "title": "Second", "purpose": "Serves: y.",
			 "tldr": {"bottom": "The one line that survives.", "points": ["A fact that stands alone."]},
			 "prose": [{"t": "then this"}],
			 "code": [{"path": "a/c.go", "lines": [10, 20]}]},
			{"id": "three", "kind": "code", "title": "Third", "purpose": "Serves: z.",
			 "tldr": {"bottom": "The one line that survives.", "points": ["A fact that stands alone."]},
			 "prose": [{"t": "finally"}],
			 "code": [{"path": "a/d.go", "lines": [3, 9]}]}
		]
	}`))
	if err != nil {
		t.Fatalf("fixture spec: %v", err)
	}
	return spec
}

func sub() Submission {
	return Submission{ID: "w000000000001", Title: "the walk", RepoRoot: "/repo",
		Pinned: "dd375a3cb2c7", SpecRev: 1, NowMS: 1700000000000}
}

func TestPayloadUnclearLeadsAndGates(t *testing.T) {
	spec := payloadSpec(t)
	marks := []proto.WalkthroughMark{
		{StepID: "one", Verdict: "understood", Note: "fine"},
		{StepID: "two", Verdict: "unclear", Note: "why is the guard here?", Revealed: []int{}},
	}
	p, err := BuildPayload(sub(), spec, marks, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(p.Unclear) != 1 || p.Unclear[0].StepID != "two" || p.Unclear[0].Note == "" {
		t.Errorf("unclear headline: %+v", p.Unclear)
	}
	if p.Tally.Understood != 1 || p.Tally.Unclear != 1 || p.Tally.NotReviewed != 1 {
		t.Errorf("tally: %+v", p.Tally)
	}
	if len(p.NotReviewed) != 1 || p.NotReviewed[0] != "three" {
		t.Errorf("not_reviewed: %v", p.NotReviewed)
	}

	// The headline set precedes the steps in the serialized form.
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if u, s := strings.Index(string(raw), `"unclear"`), strings.Index(string(raw), `"steps"`); u < 0 || s < 0 || u > s {
		t.Errorf("unclear must serialize before steps: unclear at %d, steps at %d", u, s)
	}

	// Hollow unclear refuses the whole submission and names the step.
	marks[1].Note = "  "
	if _, err := BuildPayload(sub(), spec, marks, nil); err == nil {
		t.Fatal("hollow unclear must refuse the submission")
	} else if ge, ok := err.(*GateError); !ok || ge.StepID != "two" {
		t.Errorf("gate error must name the step: %v", err)
	}
}

func TestPayloadUnsaidAndNullVerdicts(t *testing.T) {
	spec := payloadSpec(t)
	marks := []proto.WalkthroughMark{
		{StepID: "one", Verdict: "understood"}, // silent understood
		{StepID: "two", Verdict: "seen"},       // glanced, never judged
	}
	p, err := BuildPayload(sub(), spec, marks, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	steps := make(map[string]PayloadStep, len(p.Steps))
	for _, s := range p.Steps {
		steps[s.StepID] = s
	}
	if !steps["one"].Unsaid {
		t.Error("silent understood must carry unsaid")
	}
	if steps["two"].Verdict == nil || *steps["two"].Verdict != "seen" {
		t.Errorf("seen verdict must survive: %+v", steps["two"])
	}
	if steps["three"].Verdict != nil {
		t.Errorf("unjudged step must carry a null verdict, got %q", *steps["three"].Verdict)
	}
	// seen is not a judgment: the step still counts as not reviewed.
	if p.Tally.NotReviewed != 2 || len(p.NotReviewed) != 2 {
		t.Errorf("seen and unjudged both count not reviewed: %+v %v", p.Tally, p.NotReviewed)
	}

	// A raw round-trip keeps null (not "") for the unjudged verdict.
	raw, _ := json.Marshal(p)
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	for _, s := range back["steps"].([]any) {
		st := s.(map[string]any)
		if st["step_id"] == "three" && st["verdict"] != nil {
			t.Errorf("verdict must serialize as null, got %v", st["verdict"])
		}
	}
}

func TestPayloadAlwaysPresentSets(t *testing.T) {
	spec := payloadSpec(t)
	marks := []proto.WalkthroughMark{
		{StepID: "one", Verdict: "understood", Note: "ok"},
		{StepID: "two", Verdict: "understood", Note: "ok"},
		{StepID: "three", Verdict: "understood", Note: "ok"},
	}
	p, err := BuildPayload(sub(), spec, marks, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	raw, _ := json.Marshal(p)
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"not_reviewed", "unclear", "orphaned_comments"} {
		v, ok := back[key].([]any)
		if !ok {
			t.Errorf("%s must always be present as an array, got %T", key, back[key])
		} else if len(v) != 0 {
			t.Errorf("%s must be empty on a clean review: %v", key, v)
		}
	}
	if cov, ok := back["coverage"].(map[string]any); !ok || cov["computed"] != false || cov["uncovered_hunks"] == nil {
		t.Errorf("coverage must report uncomputed with a present hunk list: %v", back["coverage"])
	}
	if drift, ok := back["drift"].(map[string]any); !ok || drift["computed"] != false {
		t.Errorf("drift must report uncomputed: %v", back["drift"])
	}
}

func TestPayloadCommentsChecksAndOrphans(t *testing.T) {
	spec := payloadSpec(t)
	marks := []proto.WalkthroughMark{
		{StepID: "one", Verdict: "understood", Note: "ok", Revealed: []int{1}},
	}
	comments := []proto.WalkthroughComment{
		{ID: "c1", StepID: "one", Path: "a/b.go", Side: "new", FromLine: 2, ToLine: 3,
			Exact: "guard", Body: "why not a map?", AtMS: 1},
		{ID: "c2", StepID: "one", Body: "step-level remark", AtMS: 2},
		{ID: "c3", StepID: "vanished", Body: "left behind", AtMS: 3},
	}
	p, err := BuildPayload(sub(), spec, marks, comments)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var one *PayloadStep
	for i := range p.Steps {
		if p.Steps[i].StepID == "one" {
			one = &p.Steps[i]
		}
	}
	if one == nil || len(one.Comments) != 2 {
		t.Fatalf("step one comments: %+v", one)
	}
	if one.Comments[0].Text != "why not a map?" || one.Comments[0].Path != "a/b.go" {
		t.Errorf("anchored comment: %+v", one.Comments[0])
	}
	if one.Comments[1].Path != "" || one.Comments[1].Text != "step-level remark" {
		t.Errorf("step-level comment: %+v", one.Comments[1])
	}
	if len(one.Checks) != 2 || one.Checks[0].Revealed || !one.Checks[1].Revealed {
		t.Errorf("checks must carry revealed state: %+v", one.Checks)
	}
	if len(p.OrphanedComments) != 1 || p.OrphanedComments[0].StepID != "vanished" {
		t.Errorf("orphaned comments: %+v", p.OrphanedComments)
	}
	if p.Tally.Comments != 3 {
		t.Errorf("comment tally counts orphans too: %d", p.Tally.Comments)
	}
}
