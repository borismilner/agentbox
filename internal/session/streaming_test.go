package session

import (
	"strings"
	"testing"
)

// streamSeq is one assistant message delivered as partial-message events: a
// text block ("Hello") then a Bash tool call, followed by the authoritative
// assistant event and a result.
var streamSeq = []string{
	`{"type":"stream_event","event":{"type":"message_start","message":{"role":"assistant","content":[]}}}`,
	`{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}}`,
	`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hel"}}}`,
	`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}}`,
	`{"type":"stream_event","event":{"type":"content_block_stop","index":0}}`,
	`{"type":"stream_event","event":{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"Bash","input":{}}}}`,
	`{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"ls"}}}`,
	`{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"}"}}}`,
	`{"type":"stream_event","event":{"type":"content_block_stop","index":1}}`,
	`{"type":"stream_event","event":{"type":"message_stop"}}`,
	`{"type":"assistant","message":{"role":"assistant","model":"m","content":[{"type":"text","text":"Hello"},{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"ls"}}]}}`,
	`{"type":"result","subtype":"success","total_cost_usd":0.01}`,
}

func TestStreamingBuildsOneReconciledTurn(t *testing.T) {
	c := feed(streamSeq...)
	turns := c.snapshot()
	asst := 0
	for _, tn := range turns {
		if tn.Role == RoleAssistant {
			asst++
		}
	}
	if asst != 1 {
		t.Fatalf("assistant turns = %d, want 1 (streamed turn reconciled, not duplicated)", asst)
	}
	segs := turns[len(turns)-1].Segments
	if len(segs) != 2 {
		t.Fatalf("segments = %d, want 2 (%+v)", len(segs), segs)
	}
	if segs[0].Kind != SegText || segs[0].Text != "Hello" {
		t.Errorf("text segment = %+v", segs[0])
	}
	if segs[1].Kind != SegToolUse || segs[1].ToolInput != "ls" {
		t.Errorf("tool segment = %+v", segs[1])
	}
	if _, _, st, _ := c.info(); st != StateIdle {
		t.Errorf("state = %v, want idle", st)
	}
}

func TestStreamingShowsPartialMidFlight(t *testing.T) {
	// Up to the first block's stop, the live turn should already show "Hello"
	// even though the authoritative assistant event has not arrived.
	c := feed(streamSeq[:5]...)
	turns := c.snapshot()
	if len(turns) != 1 || turns[0].Role != RoleAssistant {
		t.Fatalf("want one in-flight assistant turn, got %+v", turns)
	}
	if len(turns[0].Segments) != 1 || !strings.Contains(turns[0].Segments[0].Text, "Hello") {
		t.Fatalf("in-flight text = %+v", turns[0].Segments)
	}
	if !c.streamingActive {
		t.Error("should still be streaming before the assistant event")
	}
}

// message_start alone opens NOTHING. It used to open an empty assistant turn, and
// the surface drew that turn's identity pill above the real reply - two agent
// bubbles for one answer, the first one blank, which is what Boris saw. A message
// that streams nothing renderable (an empty text block, a message that is only a
// tool call the surface shows elsewhere) must leave no trace.
func TestMessageStartAloneCreatesNoTurn(t *testing.T) {
	c := feed(streamSeq[0])
	if turns := c.snapshot(); len(turns) != 0 {
		t.Fatalf("message_start created %d turns, want none: %+v", len(turns), turns)
	}
	if !c.streamingActive {
		t.Error("the accumulator should be armed even with no turn yet")
	}

	// The turn appears with its first piece of content, not before.
	c.applyLine([]byte(streamSeq[1]))
	if turns := c.snapshot(); len(turns) != 0 {
		t.Fatalf("an empty content_block_start created a turn: %+v", turns)
	}
	c.applyLine([]byte(streamSeq[2]))
	turns := c.snapshot()
	if len(turns) != 1 || len(turns[0].Segments) == 0 {
		t.Fatalf("the first delta should create the turn, got %+v", turns)
	}
	if turns[0].At.IsZero() {
		t.Error("a turn with no timestamp cannot show a clock")
	}
}

// A whole message that streams only whitespace leaves nothing behind, and the
// authoritative assistant event that follows does not resurrect it.
func TestAnEmptyMessageLeavesNoBubble(t *testing.T) {
	c := feed(
		`{"type":"stream_event","event":{"type":"message_start"}}`,
		`{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"text"}}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"   "}}}`,
		`{"type":"stream_event","event":{"type":"message_stop"}}`,
		`{"type":"assistant","message":{"role":"assistant","model":"m","content":[{"type":"text","text":"  "}]}}`,
	)
	if turns := c.snapshot(); len(turns) != 0 {
		t.Fatalf("an all-whitespace message left %d turns: %+v", len(turns), turns)
	}
	if c.streamingActive {
		t.Error("the assistant event should have closed the streaming state")
	}
}
