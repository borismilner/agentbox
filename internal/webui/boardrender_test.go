package webui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/borismilner/agentbox/internal/walkthrough"
)

// fixtureRepo writes a small "repository" with one Go file whose lines are
// predictable, and returns its root.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	var b strings.Builder
	b.WriteString("package p\n\n")           // lines 1-2
	b.WriteString("// a multi-line story\n") // 3
	b.WriteString("func f() {\n")            // 4
	b.WriteString("\ts := `raw\n")           // 5: a raw string spanning lines,
	b.WriteString("still raw`\n")            // 6: the token-state test
	b.WriteString("\t_ = s\n")               // 7
	b.WriteString("}\n")                     // 8
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "f.go"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func fixtureSpec(t *testing.T, mutate func(m map[string]any)) string {
	t.Helper()
	m := map[string]any{
		"version": 1, "title": "t", "repo_root": "/x", "pinned": "dd375a3cb2c7",
		"steps": []map[string]any{
			{"id": "s1", "kind": "code", "title": "The block", "purpose": "p",
				"prose": []map[string]any{
					{"t": "the "}, {"t": "raw string", "bind": "raw"}},
				"binds": map[string]any{"raw": map[string]any{"lines": []int{5, 6}}},
				"code": []map[string]any{
					{"path": "pkg/f.go", "lines": []int{4, 8},
						"notes": []map[string]any{{"at": []int{5, 6}, "text": "state held"}}}},
				"checks": []map[string]any{{"q": "q?", "a": "a."}}},
		},
	}
	if mutate != nil {
		mutate(m)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

const fixtureDiff = `diff --git a/pkg/f.go b/pkg/f.go
--- a/pkg/f.go
+++ b/pkg/f.go
@@ -4,4 +4,5 @@
 func f() {
-	s := "old"
+	s := ` + "`raw" + `
+still raw` + "`" + `
 	_ = s
 }
`

func TestRenderStepsBasics(t *testing.T) {
	root := fixtureRepo(t)
	var missed []string
	steps, _, _, err := renderSteps(fixtureSpec(t, nil), fixtureDiff, root, nil,
		func(step, path, reason string) { missed = append(missed, step+":"+path) })
	if err != nil {
		t.Fatal(err)
	}
	if len(missed) != 0 {
		t.Fatalf("unexpected render misses: %v", missed)
	}
	st := steps[0]
	if st.Kind != "code" || len(st.Prose) != 2 || st.Prose[1].Bind != "raw" {
		t.Errorf("step shape: %+v", st)
	}
	if got := st.Binds["raw"]; len(got) != 3 || got[1] != 5 || got[2] != 6 {
		t.Errorf("binds: %v", st.Binds)
	}
	c := st.Codes[0]
	if c.Err != "" {
		t.Fatalf("unexpected block error: %s", c.Err)
	}
	if len(c.Lines) != 5 || c.Lines[0].N != 4 || c.Lines[4].N != 8 {
		t.Fatalf("line numbering: %+v", c.Lines)
	}
	// Diff-derived add flags: lines 5 and 6 were added, 4/7/8 were not.
	for _, l := range c.Lines {
		wantAdd := l.N == 5 || l.N == 6
		if l.Add != wantAdd {
			t.Errorf("line %d add = %v, want %v", l.N, l.Add, wantAdd)
		}
	}
	if c.New {
		t.Error("block is not all-new")
	}
	// The deletion renders after new line 4 with its OLD number.
	if len(c.Dels) != 1 || c.Dels[0].After != 4 || c.Dels[0].Lines[0].N != 5 {
		t.Errorf("dels: %+v", c.Dels)
	}
	if !strings.Contains(c.Dels[0].Lines[0].HTML, "old") {
		t.Errorf("del content lost: %q", c.Dels[0].Lines[0].HTML)
	}
	// Multi-line token state: line 6 continues the raw string opened on 5,
	// so its fragment must carry a string-literal class, not bare text.
	if !strings.Contains(c.Lines[2].HTML, "class=") || !strings.Contains(c.Lines[1].HTML, "class=") {
		t.Errorf("chroma classes missing: %q / %q", c.Lines[1].HTML, c.Lines[2].HTML)
	}
	if strings.Contains(c.Lines[1].HTML, "\n") {
		t.Errorf("line HTML carries a newline: %q", c.Lines[1].HTML)
	}
	// Notes numbered from 1 across the step.
	if len(c.Notes) != 1 || c.Notes[0].Num != 1 || c.Notes[0].From != 5 {
		t.Errorf("notes: %+v", c.Notes)
	}
}

func TestRenderHonestErrors(t *testing.T) {
	root := fixtureRepo(t)

	// Range past EOF: an honest error, not a truncation.
	spec := fixtureSpec(t, func(m map[string]any) {
		st := m["steps"].([]map[string]any)[0]
		st["code"] = []map[string]any{{"path": "pkg/f.go", "lines": []int{4, 99}}}
		delete(st, "binds")
		st["prose"] = []map[string]any{{"t": "plain"}}
	})
	var missed []string
	steps, _, _, err := renderSteps(spec, "", root, nil, func(step, path, reason string) { missed = append(missed, reason) })
	if err != nil {
		t.Fatal(err)
	}
	if got := steps[0].Codes[0].Err; !strings.Contains(got, "has 8 lines") {
		t.Errorf("past-EOF error: %q", got)
	}

	// Missing file.
	spec = fixtureSpec(t, func(m map[string]any) {
		st := m["steps"].([]map[string]any)[0]
		st["code"] = []map[string]any{{"path": "pkg/gone.go", "lines": []int{1, 2}}}
		delete(st, "binds")
		st["prose"] = []map[string]any{{"t": "plain"}}
	})
	steps, _, _, err = renderSteps(spec, "", root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := steps[0].Codes[0].Err; !strings.Contains(got, "cannot read") {
		t.Errorf("missing-file error: %q", got)
	}
	if len(missed) != 1 {
		t.Errorf("renderMiss calls = %d, want 1", len(missed))
	}

	// A crafted stored spec whose path escapes the root: the jail refuses
	// even though the validator upstream would too.
	spec = strings.Replace(fixtureSpec(t, nil), "pkg/f.go", "pkg/../../etc/passwd", 1)
	steps, _, _, err = renderSteps(spec, "", root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := steps[0].Codes[0].Err; !strings.Contains(got, "escapes") {
		t.Errorf("jail error: %q", got)
	}
}

func TestRenderSnippetAndAllNew(t *testing.T) {
	root := fixtureRepo(t)
	spec := fixtureSpec(t, func(m map[string]any) {
		st := m["steps"].([]map[string]any)[0]
		st["code"] = []map[string]any{
			{"snippet": map[string]any{"lang": "go", "text": "a := 1\nb := 2", "added": []int{1, 2},
				"del": []map[string]any{{"after": 0, "old": 10, "lines": []string{"gone := 9"}}}},
				"label": "proposed"},
		}
		delete(st, "binds")
		st["prose"] = []map[string]any{{"t": "plain"}}
	})
	steps, _, _, err := renderSteps(spec, "", root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := steps[0].Codes[0]
	if c.Path != "" || c.Label != "proposed" || c.Start != 1 {
		t.Errorf("snippet header: %+v", c)
	}
	if !c.New || !steps[0].AllNew {
		t.Error("fully-added snippet must be all-new")
	}
	if len(c.Dels) != 1 || c.Dels[0].After != 0 || c.Dels[0].Lines[0].N != 10 {
		t.Errorf("snippet dels: %+v", c.Dels)
	}
}

// boardLineHTML renders one hostile file through the board pipeline and
// joins every emitted line fragment, for the policy sweep.
func boardLineHTML(t *testing.T, hostile string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "h.md"), []byte(hostile), 0o644); err != nil {
		t.Fatal(err)
	}
	n := strings.Count(strings.TrimSuffix(hostile, "\n"), "\n") + 1
	spec := fixtureSpec(t, func(m map[string]any) {
		st := m["steps"].([]map[string]any)[0]
		st["code"] = []map[string]any{{"path": "h.md", "lines": []int{1, n}}}
		delete(st, "binds")
		st["prose"] = []map[string]any{{"t": "plain"}}
	})
	steps, _, _, err := renderSteps(spec, "", root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, l := range steps[0].Codes[0].Lines {
		b.WriteString(l.HTML)
		b.WriteString("\n")
	}
	if e := steps[0].Codes[0].Err; e != "" {
		t.Fatalf("hostile fixture failed to render: %s", e)
	}
	return b.String()
}

func TestRenderEscapesHostileContent(t *testing.T) {
	root := t.TempDir()
	hostile := "<img src=x onerror=alert(1)>\n<script>evil()</script>\n"
	if err := os.WriteFile(filepath.Join(root, "h.txt"), []byte(hostile), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := fixtureSpec(t, func(m map[string]any) {
		st := m["steps"].([]map[string]any)[0]
		st["code"] = []map[string]any{{"path": "h.txt", "lines": []int{1, 2}}}
		delete(st, "binds")
		st["prose"] = []map[string]any{{"t": "plain"}}
	})
	steps, _, _, err := renderSteps(spec, "", root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range steps[0].Codes[0].Lines {
		if strings.Contains(l.HTML, "<img") || strings.Contains(l.HTML, "<script") {
			t.Fatalf("hostile content crossed unescaped: %q", l.HTML)
		}
	}
}

// The glossary reaches the wire as entries plus pre-cut runs, and a bound
// phrase stays a bind: two controls under one phrase would mean two offers
// with one underline.
func TestRenderStepsGlossaryRuns(t *testing.T) {
	root := fixtureRepo(t)
	spec := fixtureSpec(t, func(m map[string]any) {
		m["glossary"] = []map[string]any{
			{"term": "raw string", "short": "a Go literal that spans lines"},
			{"term": "token", "short": "one unit of lexed source"},
		}
		m["steps"].([]map[string]any)[0]["prose"] = []map[string]any{
			{"t": "a token holds its token state"},
			{"t": "raw string", "bind": "raw"},
		}
	})
	steps, glossary, _, err := renderSteps(spec, "", root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(glossary) != 2 || glossary[0].Key != "raw string" || glossary[1].Short == "" {
		t.Fatalf("glossary on the wire: %+v", glossary)
	}
	runs := steps[0].Prose[0].Runs
	if len(runs) != 3 || runs[1].T != "token" || runs[1].Key != "token" {
		t.Fatalf("runs: %+v", runs)
	}
	if runs[2].Key != "" || !strings.Contains(runs[2].T, "token state") {
		t.Errorf("the second occurrence must stay plain text: %+v", runs[2])
	}
	if steps[0].Prose[0].T != "a token holds its token state" {
		t.Error("t must still carry the whole segment for find and read-aloud")
	}
	if steps[0].Prose[1].Runs != nil {
		t.Errorf("a bound phrase must not also be marked: %+v", steps[0].Prose[1].Runs)
	}
}

// A lead belongs to its block and the close belongs to the step, and both
// are prose as far as the glossary is concerned: the term memory runs
// prose -> leads -> close, in the order the reader meets them.
func TestRenderStepsLeadAndClose(t *testing.T) {
	root := fixtureRepo(t)
	spec := fixtureSpec(t, func(m map[string]any) {
		m["glossary"] = []map[string]any{{"term": "lexer", "short": "the thing that splits source into tokens"}}
		st := m["steps"].([]map[string]any)[0]
		st["prose"] = []map[string]any{{"t": "no terms up here"}}
		delete(st, "binds")
		st["code"] = []map[string]any{
			{"path": "pkg/f.go", "lines": []int{4, 8}, "lead": "The lexer holds its state across these lines."},
			{"path": "pkg/f.go", "lines": []int{1, 2}, "lead": "The same lexer again, so this one stays plain."},
		}
		st["close"] = []map[string]any{{"t": "And the lexer once more, still plain."}}
	})
	steps, _, _, err := renderSteps(spec, "", root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	st := steps[0]
	if st.Codes[0].Lead == "" || st.Codes[0].LeadRuns == nil {
		t.Fatalf("first lead should carry the mark: %+v", st.Codes[0])
	}
	if st.Codes[1].LeadRuns != nil {
		t.Errorf("second lead must stay plain: %+v", st.Codes[1].LeadRuns)
	}
	if len(st.Close) != 1 || st.Close[0].Runs != nil {
		t.Errorf("close: %+v", st.Close)
	}
}

// Domains reach the rail as index ranges, computed here because contiguity is a
// spec rule and Go is the side that enforces it.
func TestDomainRowsCarryRangesAndCounts(t *testing.T) {
	spec := &walkthrough.Spec{
		Domains: []walkthrough.Domain{
			{ID: "a", Title: "First", Blurb: "what it is about"},
			{ID: "b", Title: "Second"},
		},
		Steps: []walkthrough.Step{
			{ID: "s1", Kind: "code", Domain: "a"},
			{ID: "s2", Kind: "ground", Domain: "a"},
			{ID: "s3", Kind: "code", Domain: "b"},
			{ID: "s4", Kind: "check", Domain: "b"},
		},
	}
	got := domainRows(spec, make([]wireStep, 4))
	if len(got) != 2 {
		t.Fatalf("domains = %+v", got)
	}
	if got[0].From != 0 || got[0].To != 1 || got[0].Counted != 1 || got[0].Blurb != "what it is about" {
		t.Fatalf("first = %+v", got[0])
	}
	// Counted is over CODE steps only, the same number the header totals: a
	// ground step riding along must not make the group's progress unreachable.
	if got[1].From != 2 || got[1].To != 3 || got[1].Counted != 1 {
		t.Fatalf("second = %+v", got[1])
	}
}

func TestDomainRowsOnAnUngroupedSpec(t *testing.T) {
	if got := domainRows(&walkthrough.Spec{Steps: []walkthrough.Step{{ID: "s1"}}}, make([]wireStep, 1)); got != nil {
		t.Fatalf("an ungrouped spec produced %+v", got)
	}
	if got := domainRows(nil, nil); got != nil {
		t.Fatalf("a nil spec produced %+v", got)
	}
}

// A spec stored before the "no empty domain" rule existed can still hold one,
// and a group whose range is [-1,-1] would render as a heading over nothing.
func TestDomainRowsDropAGroupWithNothingInIt(t *testing.T) {
	spec := &walkthrough.Spec{
		Domains: []walkthrough.Domain{{ID: "a", Title: "First"}, {ID: "ghost", Title: "Empty"}},
		Steps:   []walkthrough.Step{{ID: "s1", Kind: "code", Domain: "a"}},
	}
	got := domainRows(spec, make([]wireStep, 1))
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("domains = %+v, want the empty one dropped", got)
	}
}

// The board reads a second caller-named path and its jail is lexical, so a symlink
// inside the root passes the prefix test: /dev/zero is reachable there too (R-16).
// R-30 owns the escape itself; this is only that the read ends.
func TestFileCacheRefusesADeviceThroughASymlink(t *testing.T) {
	if _, err := os.Stat("/dev/zero"); err != nil {
		t.Skip("no /dev/zero")
	}
	root := t.TempDir()
	if err := os.Symlink("/dev/zero", filepath.Join(root, "zero.go")); err != nil {
		t.Skip(err)
	}

	_, err := newFileCache(root, nil).lines("zero.go")
	if err == nil {
		t.Fatal("the board read a device as source")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("error = %q, want the rule that refused it", err)
	}
}
