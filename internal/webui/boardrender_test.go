package webui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	steps, _, err := renderSteps(fixtureSpec(t, nil), fixtureDiff, root, nil,
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
	steps, _, err := renderSteps(spec, "", root, nil, func(step, path, reason string) { missed = append(missed, reason) })
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
	steps, _, err = renderSteps(spec, "", root, nil, nil)
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
	steps, _, err = renderSteps(spec, "", root, nil, nil)
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
	steps, _, err := renderSteps(spec, "", root, nil, nil)
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
	steps, _, err := renderSteps(spec, "", root, nil, nil)
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
	steps, _, err := renderSteps(spec, "", root, nil, nil)
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
	steps, glossary, err := renderSteps(spec, "", root, nil, nil)
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
	steps, _, err := renderSteps(spec, "", root, nil, nil)
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
