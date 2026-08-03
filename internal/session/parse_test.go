package session

import (
	"strings"
	"testing"
)

// feed applies a set of NDJSON lines to a fresh conversation and returns it.
func feed(lines ...string) *conversation {
	c := &conversation{}
	for _, l := range lines {
		c.applyLine([]byte(l))
	}
	return c
}

func TestInitCapturesSessionAndModel(t *testing.T) {
	c := feed(`{"type":"system","subtype":"init","session_id":"sess-1","model":"claude-opus-4-8","cwd":"/tmp"}`)
	id, model, st, _ := c.info()
	if id != "sess-1" {
		t.Errorf("session id = %q, want sess-1", id)
	}
	if model != "claude-opus-4-8" {
		t.Errorf("model = %q, want claude-opus-4-8", model)
	}
	if st != StateIdle {
		t.Errorf("state = %v, want idle", st)
	}
	if len(c.snapshot()) != 0 {
		t.Errorf("init should not create a turn, got %d", len(c.snapshot()))
	}
}

func TestAssistantTextTurn(t *testing.T) {
	c := feed(`{"type":"assistant","message":{"role":"assistant","model":"m","content":[{"type":"text","text":"Hello **world**"}]}}`)
	turns := c.snapshot()
	if len(turns) != 1 {
		t.Fatalf("turns = %d, want 1", len(turns))
	}
	if turns[0].Role != RoleAssistant {
		t.Errorf("role = %v, want assistant", turns[0].Role)
	}
	if len(turns[0].Segments) != 1 || turns[0].Segments[0].Kind != SegText {
		t.Fatalf("want one text segment, got %+v", turns[0].Segments)
	}
	seg := turns[0].Segments[0]
	if !strings.Contains(seg.Text, "Hello") {
		t.Errorf("text = %q", seg.Text)
	}
	if seg.Text != "Hello **world**" {
		t.Errorf("prose should reach the UI as markdown source, got %q", seg.Text)
	}
}

func TestAssistantToolUseSegment(t *testing.T) {
	c := feed(`{"type":"assistant","message":{"role":"assistant","content":[` +
		`{"type":"text","text":"Listing files."},` +
		`{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"ls -la","description":"list"}}` +
		`]}}`)
	segs := c.snapshot()[0].Segments
	if len(segs) != 2 {
		t.Fatalf("segments = %d, want 2", len(segs))
	}
	tool := segs[1]
	if tool.Kind != SegToolUse || tool.ToolName != "Bash" || tool.ToolID != "toolu_1" {
		t.Fatalf("tool segment = %+v", tool)
	}
	if tool.ToolInput != "ls -la" {
		t.Errorf("input summary = %q, want the command", tool.ToolInput)
	}
}

func TestToolResultAttachesToCall(t *testing.T) {
	c := feed(
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"ls"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"a.txt\nb.txt","is_error":false}]}}`,
	)
	tool := c.snapshot()[0].Segments[0]
	if !tool.HasResult {
		t.Fatal("result should be attached to the call")
	}
	if !strings.Contains(tool.Result, "a.txt") {
		t.Errorf("result = %q", tool.Result)
	}
	if tool.IsError {
		t.Errorf("result should not be an error")
	}
}

func TestPlainUserReplayIgnored(t *testing.T) {
	// A replayed plain-string user message must not duplicate a prompt turn.
	c := feed(`{"type":"user","message":{"role":"user","content":"hello there"}}`)
	if n := len(c.snapshot()); n != 0 {
		t.Errorf("plain user replay created %d turns, want 0", n)
	}
}

func TestResultSuccessAttributesCostAndIdles(t *testing.T) {
	c := &conversation{}
	c.addUserPrompt("hi")
	c.applyLine([]byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi back"}]}}`))
	c.applyLine([]byte(`{"type":"result","subtype":"success","is_error":false,"total_cost_usd":0.0598}`))
	turns := c.snapshot()
	var asst *Turn
	for i := range turns {
		if turns[i].Role == RoleAssistant {
			asst = &turns[i]
		}
	}
	if asst == nil || asst.CostUSD != 0.0598 {
		t.Fatalf("cost not attributed to assistant turn: %+v", asst)
	}
	if _, _, st, _ := c.info(); st != StateIdle {
		t.Errorf("state = %v after result, want idle", st)
	}
}

func TestResultErrorSurfacesAndSetsError(t *testing.T) {
	c := feed(`{"type":"result","subtype":"error","is_error":true,"result":"context window exceeded"}`)
	_, _, st, msg := c.info()
	if st != StateError {
		t.Errorf("state = %v, want error", st)
	}
	if !strings.Contains(msg, "context window") {
		t.Errorf("error message = %q", msg)
	}
	turns := c.snapshot()
	if len(turns) != 1 || turns[0].Role != RoleSystem {
		t.Fatalf("want one system error turn, got %+v", turns)
	}
}

func TestAssistantErrorField(t *testing.T) {
	c := feed(`{"type":"assistant","error":"authentication_failed","message":{"role":"assistant","content":[]}}`)
	_, _, st, msg := c.info()
	if st != StateError || !strings.Contains(msg, "authentication_failed") {
		t.Errorf("state=%v msg=%q, want error + auth message", st, msg)
	}
}

func TestMalformedLinesSkipped(t *testing.T) {
	c := feed(
		`not json at all`,
		``,
		`{"type":`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"ok"}]}}`,
	)
	if n := len(c.snapshot()); n != 1 {
		t.Errorf("turns = %d, want 1 (malformed lines skipped)", n)
	}
}

func TestConsumeFullStream(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"s","model":"m"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"first"}]}}`,
		`{"type":"result","subtype":"success","total_cost_usd":0.01}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"second"}]}}`,
		`{"type":"result","subtype":"success","total_cost_usd":0.02}`,
	}, "\n") + "\n"
	c := &conversation{}
	c.consume(strings.NewReader(stream))
	turns := c.snapshot()
	if len(turns) != 2 {
		t.Fatalf("turns = %d, want 2", len(turns))
	}
	if turns[0].Segments[0].Text != "first" || turns[1].Segments[0].Text != "second" {
		t.Errorf("turn order wrong: %q, %q", turns[0].Segments[0].Text, turns[1].Segments[0].Text)
	}
}

func TestEncodeUserMessage(t *testing.T) {
	line, err := encodeUserMessage(`say "hi"` + "\n")
	if err != nil {
		t.Fatal(err)
	}
	got := string(line)
	want := `{"type":"user","message":{"role":"user","content":"say \"hi\"\n"}}` + "\n"
	if got != want {
		t.Errorf("encoded = %q\nwant      %q", got, want)
	}
}

func TestSummarizeInputFallback(t *testing.T) {
	// No known key -> compacted object, truncated.
	s := summarizeInput([]byte(`{"foo": 1, "bar": "two"}`))
	if !strings.Contains(s, "foo") {
		t.Errorf("summary = %q, want compacted object", s)
	}
}
