package walkthrough

import (
	"encoding/json"
	"strings"
	"testing"
)

// good returns a minimal valid spec, mutable per test.
func good() map[string]any {
	return map[string]any{
		"version":   1,
		"title":     "the twenty-fifth session",
		"repo_root": "/home/user/repo",
		"pinned":    "dd375a3cb2c7",
		"steps": []map[string]any{
			{
				"id": "xkb", "kind": "code",
				"title":   "The per-stroke group lock",
				"purpose": "Serves: typed text must be the planned text.",
				"prose": []map[string]any{
					{"t": "The fix is a guard that locks the "},
					{"t": "planned group", "bind": "planned"},
				},
				"code": []map[string]any{
					{"path": "internal/hand/xkb.go", "lines": []int{118, 145},
						"notes": []map[string]any{{"at": []int{121, 124}, "text": "decided once per call"}}},
				},
				"binds":  map[string]any{"planned": map[string]any{"lines": []int{120, 127}}},
				"checks": []map[string]any{{"q": "why per press?", "a": "the desktop reverts within 1ms"}},
			},
		},
	}
}

func mustRaw(t *testing.T, m map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseValid(t *testing.T) {
	s, warnings, err := Parse(mustRaw(t, good()))
	if err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if s.CountedSteps() != 1 || s.Step("xkb") == nil {
		t.Errorf("spec helpers wrong: counted=%d", s.CountedSteps())
	}
}

func TestTerminalCheckWarning(t *testing.T) {
	m := good()
	m["diff"] = "diff --git a/f b/f\n"
	_, warnings, err := Parse(mustRaw(t, m))
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "check") {
		t.Errorf("expected the terminal-check warning, got %v", warnings)
	}
}

func TestTeachingErrors(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(m map[string]any)
		want   string // substring of the teaching error
	}{
		{"bad version", func(m map[string]any) { m["version"] = 2 }, "version must be 1"},
		{"relative root", func(m map[string]any) { m["repo_root"] = "repo" }, "absolute"},
		{"bad pinned", func(m map[string]any) { m["pinned"] = "not-hex!" }, "pinned"},
		{"unknown field", func(m map[string]any) { m["surprise"] = true }, "unknown field"},
		{"dup step id", func(m map[string]any) {
			steps := m["steps"].([]map[string]any)
			m["steps"] = append(steps, steps[0])
		}, "twice"},
		{"bad kind", func(m map[string]any) {
			m["steps"].([]map[string]any)[0]["kind"] = "prose"
		}, "kind"},
		{"code on ground", func(m map[string]any) {
			m["steps"].([]map[string]any)[0]["kind"] = "ground"
			delete(m["steps"].([]map[string]any)[0], "purpose")
		}, "code blocks belong on code steps"},
		{"missing purpose", func(m map[string]any) {
			delete(m["steps"].([]map[string]any)[0], "purpose")
		}, "purpose is required"},
		{"unresolved bind", func(m map[string]any) {
			delete(m["steps"].([]map[string]any)[0], "binds")
		}, "no such entry"},
		{"bind outside block", func(m map[string]any) {
			m["steps"].([]map[string]any)[0]["binds"] = map[string]any{"planned": map[string]any{"lines": []int{100, 127}}}
		}, "outside block"},
		{"literal citation in prose", func(m map[string]any) {
			m["steps"].([]map[string]any)[0]["prose"] = []map[string]any{{"t": "the guard at xkb.go:121 decides"}}
		}, "bind the phrase"},
		{"literal line word in prose", func(m map[string]any) {
			m["steps"].([]map[string]any)[0]["prose"] = []map[string]any{{"t": "see line 121 for the guard"}}
		}, "bind the phrase"},
		{"diff status on citation", func(m map[string]any) {
			m["steps"].([]map[string]any)[0]["code"] = []map[string]any{
				{"path": "a.go", "lines": []int{1, 2}, "new": true}}
		}, "derives added and removed"},
		{"path and snippet", func(m map[string]any) {
			m["steps"].([]map[string]any)[0]["code"] = []map[string]any{
				{"path": "a.go", "lines": []int{1, 2}, "snippet": map[string]any{"text": "x"}}}
		}, "exactly one of path"},
		{"absolute block path", func(m map[string]any) {
			m["steps"].([]map[string]any)[0]["code"] = []map[string]any{
				{"path": "/etc/passwd", "lines": []int{1, 2}}}
		}, "repo-relative"},
		{"dotdot block path", func(m map[string]any) {
			m["steps"].([]map[string]any)[0]["code"] = []map[string]any{
				{"path": "../other/a.go", "lines": []int{1, 2}}}
		}, "repo-relative"},
		{"giant range", func(m map[string]any) {
			m["steps"].([]map[string]any)[0]["code"] = []map[string]any{
				{"path": "a.go", "lines": []int{1, 900}}}
		}, "cite the region"},
		{"note outside block", func(m map[string]any) {
			m["steps"].([]map[string]any)[0]["code"] = []map[string]any{
				{"path": "a.go", "lines": []int{10, 20},
					"notes": []map[string]any{{"at": []int{1, 2}, "text": "x"}}}}
		}, "outside the block"},
		{"scope without reason", func(m map[string]any) {
			m["out_of_scope"] = []map[string]any{{"paths": "docs/**"}}
		}, "reason is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := good()
			tc.mutate(m)
			// Repair collaterals: dropping purpose/binds on the base step is
			// only the point of some cases; others reuse the base step whose
			// binds reference code[0] that the case replaced.
			if tc.name == "diff status on citation" || tc.name == "path and snippet" ||
				tc.name == "absolute block path" || tc.name == "dotdot block path" ||
				tc.name == "giant range" || tc.name == "note outside block" {
				delete(m["steps"].([]map[string]any)[0], "binds")
				m["steps"].([]map[string]any)[0]["prose"] = []map[string]any{{"t": "plain"}}
			}
			_, _, err := Parse(mustRaw(t, m))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestProseFalsePositivesAllowed(t *testing.T) {
	for _, text := range []string{
		"measured at 14:02 on the wall clock",
		"a contrast of 13.8:1 on the ground",
		"1 of 200 immediate reads caught it",
	} {
		m := good()
		m["steps"].([]map[string]any)[0]["prose"] = []map[string]any{{"t": text}}
		delete(m["steps"].([]map[string]any)[0], "binds")
		if _, _, err := Parse(mustRaw(t, m)); err != nil {
			t.Errorf("prose %q wrongly rejected: %v", text, err)
		}
	}
}

func TestSnippetBlock(t *testing.T) {
	m := good()
	m["steps"].([]map[string]any)[0]["code"] = []map[string]any{
		{"snippet": map[string]any{"text": "one\ntwo\nthree", "added": []int{2},
			"del": []map[string]any{{"after": 1, "old": 40, "lines": []string{"gone"}}}},
			"label": "proposed change",
			"notes": []map[string]any{{"at": []int{2, 3}, "text": "why"}}},
	}
	m["steps"].([]map[string]any)[0]["binds"] = map[string]any{"planned": map[string]any{"lines": []int{1, 3}}}
	if _, _, err := Parse(mustRaw(t, m)); err != nil {
		t.Fatalf("snippet block rejected: %v", err)
	}
}

func TestStepHashDetectsChange(t *testing.T) {
	s, _, err := Parse(mustRaw(t, good()))
	if err != nil {
		t.Fatal(err)
	}
	h1 := StepHash(s.Step("xkb"))
	s.Step("xkb").Title = "renamed"
	if h2 := StepHash(s.Step("xkb")); h1 == h2 || h1 == "" {
		t.Error("StepHash must change when the step changes")
	}
}

func TestGlossaryValid(t *testing.T) {
	m := good()
	m["glossary"] = []map[string]any{
		{"term": "guard", "short": "the lock held for one keystroke", "body": "longer, for whoever wants it"},
	}
	s, warnings, err := Parse(mustRaw(t, m))
	if err != nil {
		t.Fatalf("valid glossary rejected: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(s.Glossary) != 1 || s.Glossary[0].Key() != "guard" {
		t.Errorf("glossary did not survive the parse: %+v", s.Glossary)
	}
}

// A term nothing says is effort the reader never sees, and the usual cause is
// a spelling mismatch - so the warning names the fix rather than the fault.
func TestGlossaryUnreachableTermWarns(t *testing.T) {
	m := good()
	m["glossary"] = []map[string]any{
		{"term": "XKB", "short": "the keyboard extension"},
	}
	_, warnings, err := Parse(mustRaw(t, m))
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "never appears") {
		t.Errorf("expected the unreachable-term warning, got %v", warnings)
	}
}

// A bound phrase is already a control; the renderer skips it, so the warning
// must agree or it would send an author chasing a mark that cannot exist.
func TestGlossaryBoundPhraseDoesNotCountAsReach(t *testing.T) {
	m := good()
	m["glossary"] = []map[string]any{
		{"term": "planned group", "short": "the layout the stroke was planned against"},
	}
	_, warnings, err := Parse(mustRaw(t, m))
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "never appears") {
		t.Errorf("expected the unreachable-term warning, got %v", warnings)
	}
}

func TestGlossaryTeachingErrors(t *testing.T) {
	cases := []struct {
		name  string
		gloss []map[string]any
		want  string
	}{
		{"no short", []map[string]any{{"term": "guard"}}, "short is required"},
		{"no term", []map[string]any{{"short": "x"}}, "term is required"},
		{"duplicate spelling", []map[string]any{
			{"term": "guard", "short": "a"},
			{"term": "Guard", "short": "b"},
		}, "already claimed"},
		{"alias collides with a term", []map[string]any{
			{"term": "guard", "short": "a"},
			{"term": "lock", "short": "b", "also": []string{"GUARD"}},
		}, "already claimed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := good()
			m["glossary"] = c.gloss
			_, _, err := Parse(mustRaw(t, m))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want an error containing %q", err, c.want)
			}
		})
	}
}

func TestLeadAndClose(t *testing.T) {
	m := good()
	st := m["steps"].([]map[string]any)[0]
	st["code"] = []map[string]any{{"path": "internal/hand/xkb.go", "lines": []int{118, 145},
		"lead": "What follows is the guard itself."}}
	st["binds"] = map[string]any{"planned": map[string]any{"lines": []int{120, 127}}}
	st["close"] = []map[string]any{{"t": "Take away: the lock is per stroke."}}
	if _, _, err := Parse(mustRaw(t, m)); err != nil {
		t.Fatalf("lead and close rejected: %v", err)
	}
}

func TestLeadAndCloseTeachingErrors(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(st map[string]any)
		want   string
	}{
		{"line number in a lead", func(st map[string]any) {
			st["code"].([]map[string]any)[0]["lead"] = "see line 120 below"
		}, "literal line reference"},
		{"close with no code", func(st map[string]any) {
			st["kind"] = "ground"
			delete(st, "purpose")
			delete(st, "code")
			delete(st, "binds")
			st["prose"] = []map[string]any{{"t": "no code here"}}
			st["close"] = []map[string]any{{"t": "nothing to close"}}
		}, "nothing to close"},
		{"unresolved bind in close", func(st map[string]any) {
			st["close"] = []map[string]any{{"t": "the guard", "bind": "nope"}}
		}, "close[0] binds"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := good()
			c.mutate(m["steps"].([]map[string]any)[0])
			_, _, err := Parse(mustRaw(t, m))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want an error containing %q", err, c.want)
			}
		})
	}
}

// The warning must agree with the renderer about which channels can carry a
// mark, or it sends the author chasing a mark that already exists.
func TestGlossaryReachableFromLeadAndClose(t *testing.T) {
	for _, where := range []string{"lead", "close"} {
		t.Run(where, func(t *testing.T) {
			m := good()
			st := m["steps"].([]map[string]any)[0]
			m["glossary"] = []map[string]any{{"term": "XKB", "short": "the keyboard extension"}}
			if where == "lead" {
				st["code"].([]map[string]any)[0]["lead"] = "XKB decides this per stroke."
			} else {
				st["close"] = []map[string]any{{"t": "Take away: XKB reverts within a millisecond."}}
			}
			_, warnings, err := Parse(mustRaw(t, m))
			if err != nil {
				t.Fatal(err)
			}
			if len(warnings) != 0 {
				t.Errorf("term is reachable from %s but agentbox warned: %v", where, warnings)
			}
		})
	}
}
