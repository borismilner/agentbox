package walkthrough

import (
	"strings"
	"testing"
)

// FR101's half of the spec: a step's body can hold a picture, a table and a
// callout beside its code, and every rule that keeps those from turning the board
// into a different board on every walkthrough.

// bodyStep builds a spec whose one step is a ground step carrying the blocks
// given. Ground rather than code, because these three are the blocks that do NOT
// need a citation and the point is that they are allowed without one.
func bodyStep(blocks ...map[string]any) map[string]any {
	m := good()
	m["steps"] = []map[string]any{{
		"id": "shape", "kind": "ground",
		"title": "The shape of the request path",
		"prose": []map[string]any{{"t": "One request, three hops."}},
		"body":  blocks,
	}}
	return m
}

const okSVG = `<svg viewBox="0 0 20 10"><rect x="1" y="1" width="8" height="8" fill="var(--k-surface-2)"/></svg>`

func TestBodyBlocksOnAGroundStep(t *testing.T) {
	raw := mustRaw(t, bodyStep(
		map[string]any{"lead": "The path, drawn.", "figure": map[string]any{"svg": okSVG, "caption": "one request, three hops"}},
		map[string]any{"table": map[string]any{
			"head":  []string{"hop", "cost"},
			"rows":  [][]string{{"the daemon", "0.4 ms"}, {"the store", "11 ms"}},
			"align": []string{"left", "right"},
		}},
		map[string]any{"callout": map[string]any{
			"tone":  "warn",
			"title": "The second hop is the one that fails",
			"prose": []map[string]any{{"t": "It retries three times and then gives up."}},
		}},
	))
	s, warnings, err := Parse(raw)
	if err != nil {
		t.Fatalf("a body of figure, table and callout was refused: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	blocks := s.Steps[0].Blocks()
	if len(blocks) != 3 {
		t.Fatalf("Blocks() returned %d, want 3", len(blocks))
	}
	if blocks[0].Figure == nil || blocks[1].Table == nil || blocks[2].Callout == nil {
		t.Errorf("the three forms did not survive the round trip: %+v", blocks)
	}
	for i := range blocks {
		if blocks[i].isCode() {
			t.Errorf("block %d reads as code", i)
		}
	}
}

// The older name still holds the blocks, because every stored walkthrough uses
// it. A spec that says both is the one thing refused: two arrays that cannot be
// interleaved with each other would make reading order unanswerable.
func TestBodyAndCodeAreTheSameArrayUnderTwoNames(t *testing.T) {
	m := good()
	steps := m["steps"].([]map[string]any)
	if got := mustParse(t, m).Steps[0].Blocks(); len(got) != 1 || got[0].Path == "" {
		t.Fatalf("code did not answer Blocks(): %+v", got)
	}
	steps[0]["body"] = steps[0]["code"]
	delete(steps[0], "code")
	if got := mustParse(t, m).Steps[0].Blocks(); len(got) != 1 || got[0].Path == "" {
		t.Fatalf("body did not answer Blocks(): %+v", got)
	}
	steps[0]["code"] = steps[0]["body"]
	if _, _, err := Parse(mustRaw(t, m)); err == nil || !strings.Contains(err.Error(), "same array under two names") {
		t.Errorf("both fields at once should refuse, got %v", err)
	}
}

func mustParse(t *testing.T, m map[string]any) *Spec {
	t.Helper()
	s, _, err := Parse(mustRaw(t, m))
	if err != nil {
		t.Fatalf("spec refused: %v", err)
	}
	return s
}

func TestBodyBlockTeachingErrors(t *testing.T) {
	cases := []struct {
		name  string
		block map[string]any
		want  string
	}{
		{"two forms in one block",
			map[string]any{"figure": map[string]any{"svg": okSVG}, "table": map[string]any{"head": []string{"a"}, "rows": [][]string{{"b"}}}},
			"exactly one of path"},
		{"an empty block", map[string]any{"lead": "nothing here"}, "this one is 0 of them"},
		{"a figure with both forms",
			map[string]any{"figure": map[string]any{"svg": okSVG, "src": "docs/a.png"}},
			"exactly one of svg"},
		{"a figure with neither", map[string]any{"figure": map[string]any{"caption": "x"}}, "exactly one of svg"},
		{"a remote image", map[string]any{"figure": map[string]any{"src": "https://evil.example/a.png"}}, "the board loads nothing over the network"},
		{"an escaping path", map[string]any{"figure": map[string]any{"src": "../../etc/passwd"}}, "repo_root is the jail"},
		{"an absolute path", map[string]any{"figure": map[string]any{"src": "/etc/hosts"}}, "repo_root is the jail"},
		{"a notes-carrying figure",
			map[string]any{"figure": map[string]any{"svg": okSVG}, "notes": []map[string]any{{"at": []int{1, 2}, "text": "x"}}},
			"takes no notes"},
		{"a headless table", map[string]any{"table": map[string]any{"rows": [][]string{{"a"}}}}, "needs a head"},
		{"a ragged table",
			map[string]any{"table": map[string]any{"head": []string{"a", "b"}, "rows": [][]string{{"only one"}}}},
			"renders as one with a hole in it"},
		{"a mis-aligned table",
			map[string]any{"table": map[string]any{"head": []string{"a", "b"}, "rows": [][]string{{"x", "y"}}, "align": []string{"middle", "left"}}},
			"it is left, right or center"},
		{"a table of paragraphs",
			map[string]any{"table": map[string]any{"head": []string{"a"}, "rows": [][]string{{strings.Repeat("x", MaxCellChars+1)}}}},
			"belongs in the prose or a note"},
		{"a fifth tone",
			map[string]any{"callout": map[string]any{"tone": "urgent", "prose": []map[string]any{{"t": "x"}}}},
			"a meaning that needs a fifth is prose"},
		{"a wordless callout", map[string]any{"callout": map[string]any{"tone": "note"}}, "must hold 1-"},
		{"a callout with a stale line number",
			map[string]any{"callout": map[string]any{"tone": "note", "prose": []map[string]any{{"t": "see xkb.go:118 for the guard"}}}},
			"literal line reference"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := Parse(mustRaw(t, bodyStep(tc.block)))
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("want an error containing %q, got %v", tc.want, err)
			}
		})
	}
}

// The two rules that keep the kinds meaning what they say: code is what the
// review's promise counts, and a picture is not code.
func TestCodeCitationsStayOnCodeSteps(t *testing.T) {
	m := bodyStep(map[string]any{"path": "internal/hand/xkb.go", "lines": []int{1, 4}})
	if _, _, err := Parse(mustRaw(t, m)); err == nil || !strings.Contains(err.Error(), "code citations belong on code steps only") {
		t.Errorf("a citation on a ground step should refuse, got %v", err)
	}

	m = good()
	steps := m["steps"].([]map[string]any)
	delete(steps[0], "code")
	delete(steps[0], "binds")
	steps[0]["prose"] = []map[string]any{{"t": "no code here"}}
	steps[0]["body"] = []map[string]any{{"figure": map[string]any{"svg": okSVG}}}
	if _, _, err := Parse(mustRaw(t, m)); err == nil || !strings.Contains(err.Error(), "is a ground step") {
		t.Errorf("a code step whose body draws only pictures should refuse, got %v", err)
	}
}

// A bound phrase lights a region of code. Pointing one at a figure has no region
// to light, and silently doing nothing is the failure mode this refuses.
func TestABindCannotPointAtAFigure(t *testing.T) {
	m := good()
	steps := m["steps"].([]map[string]any)
	steps[0]["body"] = []map[string]any{
		{"figure": map[string]any{"svg": okSVG}},
		{"path": "internal/hand/xkb.go", "lines": []int{118, 145}},
	}
	delete(steps[0], "code")
	steps[0]["binds"] = map[string]any{"planned": map[string]any{"lines": []int{120, 127}}}
	if _, _, err := Parse(mustRaw(t, m)); err == nil || !strings.Contains(err.Error(), "can only point at a citation or a snippet") {
		t.Errorf("a bind onto block 0 (a figure) should refuse, got %v", err)
	}
	// The same spec with the bind aimed at the citation is fine, which is what
	// makes the refusal above about the block's kind rather than about the bind.
	steps[0]["binds"] = map[string]any{"planned": map[string]any{"block": 1, "lines": []int{120, 127}}}
	mustParse(t, m)
}

// A citation is captured from git so the review says what it was written against;
// a figure has nothing to capture. The two must not be confused, because the
// capture walk is what keeps a stored walkthrough readable after a rebase.
func TestFiguresAreNotCitations(t *testing.T) {
	m := good()
	steps := m["steps"].([]map[string]any)
	steps[0]["body"] = []map[string]any{
		{"path": "internal/hand/xkb.go", "lines": []int{118, 145}},
		{"figure": map[string]any{"src": "docs/wiki/img/review.png"}},
	}
	delete(steps[0], "code")
	s := mustParse(t, m)
	cites := s.Citations()
	if len(cites) != 1 || cites[0].Path != "internal/hand/xkb.go" {
		t.Errorf("Citations() = %+v, want only the go file", cites)
	}
}

// A term defined in the glossary and said only inside a callout is reachable: the
// renderer marks callout prose, so the warning must not claim otherwise.
func TestAGlossaryTermIsReachableFromACallout(t *testing.T) {
	m := bodyStep(map[string]any{"callout": map[string]any{
		"tone":  "danger",
		"prose": []map[string]any{{"t": "The keepalive is what dies first."}},
	}})
	m["glossary"] = []map[string]any{{"term": "keepalive", "short": "the ping that says a session is alive"}}
	_, warnings, err := Parse(mustRaw(t, m))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range warnings {
		if strings.Contains(w, "keepalive") {
			t.Errorf("a term said in a callout was called unreachable: %s", w)
		}
	}
}

// The figure's own budget, so an author is told at authoring time rather than
// shipping a walkthrough whose picture arrives as a grey box.
func TestFigureSVGIsValidatedWhenTheSpecIsParsed(t *testing.T) {
	m := bodyStep(map[string]any{"figure": map[string]any{"svg": `<svg viewBox="0 0 1 1"><rect fill="#ff0000" width="1" height="1"/></svg>`}})
	_, _, err := Parse(mustRaw(t, m))
	if err == nil || !strings.Contains(err.Error(), "colours from the human's theme") {
		t.Errorf("a figure's svg was not validated by Parse: %v", err)
	}
}
