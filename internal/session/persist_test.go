package session

import (
	"strings"
	"testing"
	"time"
)

func sampleTurns() []Turn {
	return []Turn{
		{Role: RoleUser, Segments: []Segment{{Kind: SegText, Text: "List the files."}}},
		{Role: RoleAssistant, Model: "claude-opus-4-8", CostUSD: 0.02, Segments: []Segment{
			{Kind: SegText, Text: "Here they are:"},
			{Kind: SegToolUse, ToolName: "Bash", ToolID: "toolu_1", ToolInput: "ls", Result: "a.txt\nb.txt", HasResult: true},
		}},
	}
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	data, err := Marshal(sampleTurns())
	if err != nil {
		t.Fatal(err)
	}
	got, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("turns = %d, want 2", len(got))
	}
	if got[0].Role != RoleUser || got[0].Segments[0].Text != "List the files." {
		t.Errorf("user turn = %+v", got[0])
	}
	a := got[1]
	if a.Role != RoleAssistant || a.Model != "claude-opus-4-8" || a.CostUSD != 0.02 {
		t.Errorf("assistant turn meta = %+v", a)
	}
	if len(a.Segments) != 2 {
		t.Fatalf("assistant segments = %d, want 2", len(a.Segments))
	}
	if a.Segments[0].Kind != SegText || a.Segments[0].Text != "Here they are:" {
		t.Errorf("prose segment lost its markdown source: %+v", a.Segments[0])
	}
	tool := a.Segments[1]
	if tool.Kind != SegToolUse || tool.ToolName != "Bash" || tool.ToolInput != "ls" || !tool.HasResult {
		t.Errorf("tool segment lost fields: %+v", tool)
	}
}

func TestToMarkdown(t *testing.T) {
	md := ToMarkdown(sampleTurns())
	for _, want := range []string{"## You", "## Claude", "List the files.", "**Bash**", "a.txt"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown export missing %q\n---\n%s", want, md)
		}
	}
}

func TestSaveAndLoadLatest(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	if _, err := saveAt(dir, []Turn{{Role: RoleUser, Segments: []Segment{{Kind: SegText, Text: "older"}}}}, Meta{}, base); err != nil {
		t.Fatal(err)
	}
	newer := base.Add(time.Hour)
	if _, err := saveAt(dir, sampleTurns(), Meta{}, newer); err != nil {
		t.Fatal(err)
	}
	turns, path, err := LoadLatest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, ".json") {
		t.Errorf("latest path = %q, want a .json", path)
	}
	if len(turns) != 2 || turns[0].Segments[0].Text != "List the files." {
		t.Fatalf("LoadLatest returned the wrong (older?) conversation: %+v", turns)
	}
}

func TestLoadLatestEmptyDir(t *testing.T) {
	if _, _, err := LoadLatest(t.TempDir()); err == nil {
		t.Error("LoadLatest should error on a dir with no saved sessions")
	}
}
