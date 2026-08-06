package webui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/borismilner/agentbox/internal/store"
)

// The board must show what the review was written against, not what the file
// says now. These are the three states a citation can be in: captured, not
// captured, and captured for a file that is no longer on disk at all.

func pinnedSpec(t *testing.T, path string, from, to int) string {
	t.Helper()
	spec := map[string]any{
		"version": 1, "title": "t", "repo_root": "/x", "pinned": "abcdef1",
		"steps": []any{map[string]any{
			"id": "s1", "kind": "code", "title": "one",
			"purpose": "Serves: a test. Decided by: the test.",
			"prose":   []any{map[string]any{"t": "words"}},
			"code":    []any{map[string]any{"path": path, "lines": []int{from, to}}},
		}},
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// lineText pulls the plain text back out of the rendered chroma spans.
func lineText(html string) string {
	var b strings.Builder
	depth := 0
	for _, r := range html {
		switch {
		case r == '<':
			depth++
		case r == '>':
			depth--
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestCapturedSourceWinsOverTheFileOnDisk(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("what it says today\nsecond\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pinned := []store.Excerpt{{
		Path: "a.go", FromLine: 1, ToLine: 1,
		Text: "what it said when the review was written", Source: store.ExcerptWorktree,
	}}

	steps, _, _, err := renderSteps(pinnedSpec(t, "a.go", 1, 1), "", root, pinned, nil)
	if err != nil {
		t.Fatal(err)
	}
	code := steps[0].Codes[0]
	if code.Err != "" {
		t.Fatalf("render failed: %s", code.Err)
	}
	if !code.Pinned {
		t.Error("the block does not report that it came from the capture")
	}
	if got := lineText(code.Lines[0].HTML); !strings.Contains(got, "when the review was written") {
		t.Errorf("rendered the file on disk instead of the capture: %q", got)
	}
}

func TestWithoutACaptureTheFileIsStillRead(t *testing.T) {
	// Every walkthrough stored before capture existed depends on this path,
	// so it has to keep working exactly as it did.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("on disk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	steps, _, _, err := renderSteps(pinnedSpec(t, "a.go", 1, 1), "", root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	code := steps[0].Codes[0]
	if code.Err != "" {
		t.Fatalf("render failed: %s", code.Err)
	}
	if code.Pinned {
		t.Error("a block read from the working tree claims to be pinned")
	}
	if got := lineText(code.Lines[0].HTML); !strings.Contains(got, "on disk") {
		t.Errorf("did not read the file: %q", got)
	}
}

func TestACaptureSurvivesTheFileBeingDeleted(t *testing.T) {
	// The failure Boris hit: the file is gone from the working tree. With the
	// capture in hand the step renders anyway, which is the whole feature.
	root := t.TempDir() // deliberately empty
	pinned := []store.Excerpt{{
		Path: "cmd/gone/main.go", FromLine: 10, ToLine: 11,
		Text: "func main() {\n}", Source: store.ExcerptGit,
	}}
	steps, _, _, err := renderSteps(pinnedSpec(t, "cmd/gone/main.go", 10, 11), "", root, pinned, nil)
	if err != nil {
		t.Fatal(err)
	}
	code := steps[0].Codes[0]
	if code.Err != "" {
		t.Fatalf("a deleted file still broke the block: %s", code.Err)
	}
	if len(code.Lines) != 2 {
		t.Fatalf("rendered %d lines, want 2", len(code.Lines))
	}
	if code.Lines[0].N != 10 {
		t.Errorf("first line numbered %d, want the cited 10", code.Lines[0].N)
	}
	// A git-sourced capture is still a capture: the flag says which of the two
	// the reader is looking at, and it must not be blank just because the
	// source was recovered rather than taken at creation.
	if !code.Pinned {
		t.Error("a block served from a repaired capture does not report itself as pinned")
	}
}

func TestAMissingCaptureAndAMissingFileStillReportsHonestly(t *testing.T) {
	root := t.TempDir()
	var missed []string
	steps, _, _, err := renderSteps(pinnedSpec(t, "cmd/gone/main.go", 10, 11), "", root, nil,
		func(step, path, reason string) { missed = append(missed, reason) })
	if err != nil {
		t.Fatal(err)
	}
	if steps[0].Codes[0].Err == "" {
		t.Fatal("a block with neither a capture nor a file rendered as if it were fine")
	}
	if len(missed) != 1 {
		t.Errorf("renderMiss calls = %d, want 1", len(missed))
	}
}
