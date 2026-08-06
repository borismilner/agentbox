package walkthrough

import (
	"encoding/json"
	"maps"
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
				"tldr": map[string]any{
					"bottom": "Typing sets the keyboard group per stroke, so the desktop cannot swap it out from under a planned key.",
					"points": []string{
						"The desktop reverts the group within 1ms, which is why the lock is per press and not per call.",
						"Without it a planned key produced whatever layout happened to be live.",
					},
				},
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

// The TL;DR (2026-08-06). The board opens in it, so a step without one shows
// the reader nothing until they switch - which is the opposite of the point.
func TestTLDRRequiredOnCodeAndCheckSteps(t *testing.T) {
	for _, kind := range []string{"code", "check"} {
		m := good()
		st := m["steps"].([]map[string]any)[0]
		st["kind"] = kind
		if kind == "check" {
			delete(st, "code")
		}
		delete(st, "tldr")
		_, _, err := Parse(mustRaw(t, m))
		if err == nil {
			t.Fatalf("a %s step with no tldr was accepted", kind)
		}
		// A teaching error: the caller is a model that retries, and "tldr is
		// required" alone produces a summary of the summary.
		for _, want := range []string{"NOT the shortened version", "bottom", "points"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error does not teach %q: %v", want, err)
			}
		}
	}
}

func TestTLDRIsOptionalOnAGroundStep(t *testing.T) {
	m := good()
	st := m["steps"].([]map[string]any)[0]
	st["kind"] = "ground"
	delete(st, "code")
	delete(st, "binds")
	delete(st, "checks")
	delete(st, "tldr")
	st["prose"] = []map[string]any{{"t": "where we are before any of it"}}
	if _, _, err := Parse(mustRaw(t, m)); err != nil {
		t.Fatalf("a ground step without a tldr was refused: %v", err)
	}
}

func TestTLDRShapeIsEnforced(t *testing.T) {
	cases := []struct {
		name  string
		tldr  map[string]any
		wants string
	}{
		{"no bottom line", map[string]any{"points": []string{"a fact"}}, "bottom is required"},
		{"bottom over the cap", map[string]any{"bottom": strings.Repeat("x", MaxTLDRBottom+1), "points": []string{"a fact"}}, "one sentence"},
		{"too many points", map[string]any{"bottom": "b", "points": make([]string, MaxTLDRPoints+1)}, "the cap is"},
		{"an empty point", map[string]any{"bottom": "b", "points": []string{"a fact", "  "}}, "is empty"},
		{"a point over the cap", map[string]any{"bottom": "b", "points": []string{strings.Repeat("x", MaxTLDRPoint+1)}}, "paragraph wearing a bullet"},
		// A code step with a bottom line and nothing under it is the paragraph
		// summary this shape exists to refuse.
		{"no points at all", map[string]any{"bottom": "b"}, "at least one point"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := good()
			m["steps"].([]map[string]any)[0]["tldr"] = c.tldr
			_, _, err := Parse(mustRaw(t, m))
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), c.wants) {
				t.Fatalf("got %v, want it to say %q", err, c.wants)
			}
		})
	}
}

func TestTLDRSurvivesTheRoundTrip(t *testing.T) {
	s, _, err := Parse(mustRaw(t, good()))
	if err != nil {
		t.Fatal(err)
	}
	got := s.Step("xkb")
	if got.TLDR == nil || !strings.HasPrefix(got.TLDR.Bottom, "Typing sets") || len(got.TLDR.Points) != 2 {
		t.Fatalf("tldr = %+v", got.TLDR)
	}
}

// Domains (2026-08-06). The board walks one group at a time, which is what
// makes contiguity a rule rather than a preference.
func TestDomainsValid(t *testing.T) {
	m := good()
	m["domains"] = []map[string]any{{"id": "core", "title": "The guard", "blurb": "what stops the swap"}}
	m["steps"].([]map[string]any)[0]["domain"] = "core"
	s, _, err := Parse(mustRaw(t, m))
	if err != nil {
		t.Fatalf("a grouped spec was refused: %v", err)
	}
	if len(s.Domains) != 1 || s.Step("xkb").Domain != "core" {
		t.Fatalf("domains = %+v", s.Domains)
	}
}

func TestDomainsAreOptional(t *testing.T) {
	// A short walk needs no grouping, and that is the right answer rather than a
	// fallback: the rail it gets is the one it always had.
	if _, _, err := Parse(mustRaw(t, good())); err != nil {
		t.Fatalf("an ungrouped spec was refused: %v", err)
	}
}

func TestDomainTeachingErrors(t *testing.T) {
	two := func() map[string]any {
		m := good()
		first := m["steps"].([]map[string]any)[0]
		second := map[string]any{}
		maps.Copy(second, first)
		second["id"] = "second"
		m["steps"] = []map[string]any{first, second}
		return m
	}
	cases := []struct {
		name  string
		build func() map[string]any
		wants string
	}{
		{"a step names a domain nothing declares", func() map[string]any {
			m := good()
			m["steps"].([]map[string]any)[0]["domain"] = "ghost"
			return m
		}, "declares no domains"},
		{"an undeclared id with domains present", func() map[string]any {
			m := good()
			m["domains"] = []map[string]any{{"id": "core", "title": "Core"}}
			m["steps"].([]map[string]any)[0]["domain"] = "ghost"
			return m
		}, "is not declared"},
		{"a step left out of the grouping", func() map[string]any {
			m := two()
			m["domains"] = []map[string]any{{"id": "core", "title": "Core"}}
			m["steps"].([]map[string]any)[0]["domain"] = "core"
			return m
		}, "a review with a hole in it"},
		{"an empty domain", func() map[string]any {
			m := good()
			m["domains"] = []map[string]any{{"id": "core", "title": "Core"}, {"id": "spare", "title": "Spare"}}
			m["steps"].([]map[string]any)[0]["domain"] = "core"
			return m
		}, "no steps in it"},
		{"a domain with no title", func() map[string]any {
			m := good()
			m["domains"] = []map[string]any{{"id": "core"}}
			m["steps"].([]map[string]any)[0]["domain"] = "core"
			return m
		}, "needs a title"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := Parse(mustRaw(t, c.build()))
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), c.wants) {
				t.Fatalf("got %v, want it to say %q", err, c.wants)
			}
		})
	}
}

func TestDomainStepsMustBeConsecutive(t *testing.T) {
	// A B A. The board opens a domain when the reader enters it, so a domain that
	// is left and returned to would open twice and finish neither time.
	m := good()
	first := m["steps"].([]map[string]any)[0]
	mk := func(id, dom string) map[string]any {
		st := map[string]any{}
		maps.Copy(st, first)
		st["id"] = id
		st["domain"] = dom
		return st
	}
	m["domains"] = []map[string]any{{"id": "a", "title": "A"}, {"id": "b", "title": "B"}}
	m["steps"] = []map[string]any{mk("one", "a"), mk("two", "b"), mk("three", "a")}
	_, _, err := Parse(mustRaw(t, m))
	if err == nil {
		t.Fatal("an interleaved grouping was accepted")
	}
	if !strings.Contains(err.Error(), "must be consecutive") {
		t.Fatalf("got %v", err)
	}
}

// FuzzSpecParse guards the door every walkthrough comes through. The spec is a
// model's JSON, and Parse is the only thing standing between it and the
// subsystems that trust whatever it accepted - the worktree capture, the
// coverage arithmetic that now runs on every read, and the glossary marking
// that runs on every render. So this asserts the promises those callers make
// without checking, not just the absence of a panic.
func FuzzSpecParse(f *testing.F) {
	f.Add([]byte(``))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"version":1}`))
	f.Add(mustRawB(good()))
	f.Add(mustRawB(withDiff(good())))
	f.Add(mustRawB(withGlossary(good())))
	f.Add(mustRawB(withSnippet(good())))
	f.Add([]byte(`{"version":1,"title":"t","repo_root":"/r","pinned":"aaaaaaa",
		"out_of_scope":[{"paths":"**/*_test.go","reason":"tests"},{"path":"a.go","lines":[9,2],"reason":"inverted"}],
		"steps":[{"id":"g","kind":"ground","title":"G","prose":[{"t":"x"}]}]}`))

	f.Fuzz(func(t *testing.T, raw []byte) {
		s, _, err := Parse(raw)
		if err != nil {
			return
		}

		// The capture and the coverage arithmetic both read citations straight
		// off an accepted spec and slice files with them.
		for _, c := range s.Citations() {
			if c.Path == "" || c.From < 1 || c.To < c.From {
				t.Fatalf("accepted spec yielded an unusable citation: %+v", c)
			}
		}
		if n := s.CountedSteps(); n < 0 || n > len(s.Steps) {
			t.Fatalf("counted steps %d against %d steps", n, len(s.Steps))
		}
		for i := range s.Steps {
			if StepHash(&s.Steps[i]) == "" {
				t.Fatalf("step %q has no hash, so no amendment could ever go stale against it", s.Steps[i].ID)
			}
			for j := range s.Steps[i].Code {
				lo, hi := s.Steps[i].Code[j].lineBounds()
				if lo < 1 || hi < lo {
					t.Fatalf("step %q block %d bounds [%d,%d]", s.Steps[i].ID, j, lo, hi)
				}
			}
		}

		// The whole read path in one line: an accepted spec's own diff, its own
		// citations and its own scopes, through the arithmetic every open runs.
		cov := Cover(s.Diff, s.Citations(), s.OutOfScope)
		if cov.Covered+cov.OutOfScope+len(cov.Uncovered) != cov.Hunks {
			t.Fatalf("coverage parts do not add up on an accepted spec: %+v", cov)
		}

		// Glossary marking is a partition of the author's text, never a rewrite
		// of it: whatever the board shows must read back as what was written.
		idx := NewTermIndex(s.Glossary)
		for i := range s.Steps {
			seen := map[string]bool{}
			for _, segs := range [][]Seg{s.Steps[i].Prose, s.Steps[i].Close} {
				for _, seg := range segs {
					at := 0
					for _, m := range idx.Find(seg.T) {
						if m.From < at || m.To > len(seg.T) || m.From >= m.To {
							t.Fatalf("match %+v is out of order or out of bounds in %q", m, seg.T)
						}
						at = m.To
					}
					runs := idx.Split(seg.T, seen)
					if runs == nil {
						continue
					}
					var b strings.Builder
					for _, r := range runs {
						b.WriteString(r.T)
					}
					if b.String() != seg.T {
						t.Fatalf("glossary marking rewrote the text: %q became %q", seg.T, b.String())
					}
				}
			}
		}
	})
}

// mustRawB is mustRaw without a *testing.T, for seed corpora.
func mustRawB(m map[string]any) []byte {
	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return b
}

func withDiff(m map[string]any) map[string]any {
	m["diff"] = "diff --git a/internal/hand/xkb.go b/internal/hand/xkb.go\n" +
		"--- a/internal/hand/xkb.go\n+++ b/internal/hand/xkb.go\n" +
		"@@ -118,4 +118,5 @@\n context\n+added\n"
	return m
}

func withGlossary(m map[string]any) map[string]any {
	m["glossary"] = []map[string]any{
		{"term": "group lock", "short": "the per-stroke keyboard group guard", "also": []string{"lock"}},
	}
	return m
}

func withSnippet(m map[string]any) map[string]any {
	st := m["steps"].([]map[string]any)[0]
	st["code"] = []map[string]any{
		{"snippet": map[string]any{"lang": "go", "text": "one\ntwo\nthree", "added": []int{2},
			"del": []map[string]any{{"after": 1, "old": 7, "lines": []string{"gone"}}}}},
	}
	st["binds"] = map[string]any{"planned": map[string]any{"lines": []int{1, 2}}}
	return m
}
