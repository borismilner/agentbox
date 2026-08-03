package session

import (
	"encoding/json"
	"strings"
	"time"
)

// parseThrottle bounds how often a streaming turn's markdown is re-parsed: text
// deltas arrive many times a second, but re-laying the document that often is
// wasteful. Block boundaries always force a final parse, so the throttle only
// affects mid-block smoothness, never correctness.
const parseThrottle = 75 * time.Millisecond

// blockBuf accumulates one content block of the in-flight assistant message as
// deltas arrive (--include-partial-messages).
type blockBuf struct {
	kind     SegKind
	text     string // accumulated text/thinking
	toolName string
	toolID   string
	toolJSON string // accumulated input_json_delta
}

// streamInner is the raw Claude API streaming event carried inside a
// stream_event envelope.
type streamInner struct {
	Type         string          `json:"type"`
	Index        int             `json:"index"`
	Delta        json.RawMessage `json:"delta"`
	ContentBlock json.RawMessage `json:"content_block"`
}

type streamDelta struct {
	Type        string `json:"type"` // text_delta | input_json_delta | thinking_delta
	Text        string `json:"text"`
	PartialJSON string `json:"partial_json"`
	Thinking    string `json:"thinking"`
}

type streamBlockStart struct {
	Type string `json:"type"` // text | tool_use | thinking
	ID   string `json:"id"`
	Name string `json:"name"`
}

// applyStreamEvent folds one stream_event into the in-flight assistant turn.
// The authoritative `assistant` event that follows message_stop reconciles the
// turn (see applyAssistant), so any delta-accumulation glitch is transient.
func (c *conversation) applyStreamEvent(ev wireEvent) {
	var in streamInner
	if err := json.Unmarshal(ev.Event, &in); err != nil {
		return
	}
	switch in.Type {
	case "message_start":
		c.beginStreaming()
	case "content_block_start":
		var b streamBlockStart
		_ = json.Unmarshal(in.ContentBlock, &b)
		c.startBlock(in.Index, b)
	case "content_block_delta":
		var d streamDelta
		_ = json.Unmarshal(in.Delta, &d)
		c.appendDelta(in.Index, d)
	case "content_block_stop":
		c.forceRebuild()
	case "message_stop":
		c.forceRebuild()
	}
}

// beginStreaming arms the delta accumulator for a new assistant message. It does
// NOT create the turn: rebuildActiveLocked does that the moment there is
// something to put in it, so a message that streams nothing renderable leaves no
// empty bubble behind (see the note on activeTurn).
func (c *conversation) beginStreaming() {
	c.mu.Lock()
	c.activeTurn = -1
	c.streamingActive = true
	c.streaming = nil
	c.lastParse = time.Time{}
	c.mu.Unlock()
	c.notify()
}

func (c *conversation) startBlock(index int, b streamBlockStart) {
	c.mu.Lock()
	if !c.streamingActive {
		c.mu.Unlock()
		return
	}
	c.ensureBlockLocked(index)
	bb := &c.streaming[index]
	switch b.Type {
	case "tool_use":
		bb.kind = SegToolUse
		bb.toolName = b.Name
		bb.toolID = b.ID
	case "thinking":
		bb.kind = SegThinking
	default:
		bb.kind = SegText
	}
	c.rebuildActiveLocked()
	c.mu.Unlock()
	c.notify()
}

func (c *conversation) appendDelta(index int, d streamDelta) {
	c.mu.Lock()
	if !c.streamingActive {
		c.mu.Unlock()
		return
	}
	c.ensureBlockLocked(index)
	bb := &c.streaming[index]
	switch d.Type {
	case "input_json_delta":
		bb.toolJSON += d.PartialJSON
	case "thinking_delta":
		bb.text += d.Thinking
	default: // text_delta
		bb.text += d.Text
	}
	rebuilt := false
	if now := time.Now(); now.Sub(c.lastParse) >= parseThrottle {
		c.lastParse = now
		c.rebuildActiveLocked()
		rebuilt = true
	}
	c.mu.Unlock()
	if rebuilt {
		c.notify()
	}
}

// forceRebuild re-parses the in-flight turn regardless of the throttle, at a
// block boundary or message end.
func (c *conversation) forceRebuild() {
	c.mu.Lock()
	if !c.streamingActive {
		c.mu.Unlock()
		return
	}
	c.lastParse = time.Now()
	c.rebuildActiveLocked()
	c.mu.Unlock()
	c.notify()
}

func (c *conversation) ensureBlockLocked(index int) {
	for len(c.streaming) <= index {
		c.streaming = append(c.streaming, blockBuf{kind: SegText})
	}
}

// rebuildActiveLocked rewrites the in-flight turn's segments from the
// accumulated block buffers, creating the turn on the first segment that is
// worth showing. Caller holds mu. markdown.Parse runs here, on the reader
// goroutine, never the frame goroutine.
func (c *conversation) rebuildActiveLocked() {
	if !c.streamingActive {
		return
	}
	segs := make([]Segment, 0, len(c.streaming))
	for _, b := range c.streaming {
		switch b.kind {
		case SegToolUse:
			segs = append(segs, Segment{Kind: SegToolUse, ToolName: b.toolName, ToolID: b.toolID, ToolInput: summarizeInput([]byte(b.toolJSON))})
		case SegThinking:
			if strings.TrimSpace(b.text) == "" {
				continue
			}
			segs = append(segs, Segment{Kind: SegThinking, Text: b.text})
		default:
			if strings.TrimSpace(b.text) == "" {
				continue
			}
			segs = append(segs, Segment{Kind: SegText, Text: b.text})
		}
	}
	if len(segs) == 0 {
		return // nothing to show yet, so no turn yet
	}
	if c.activeTurn < 0 || c.activeTurn >= len(c.turns) {
		c.turns = append(c.turns, Turn{Role: RoleAssistant, At: c.clock()})
		c.activeTurn = len(c.turns) - 1
	}
	c.turns[c.activeTurn].Segments = segs
	c.markThinkingLocked(segs)
}

// markThinkingLocked stamps how long the model worked before its first word, once
// per turn: the clock runs from the prompt going out to the first prose segment,
// so extended thinking and any tool calls it made on the way are all inside it.
// Caller holds mu.
func (c *conversation) markThinkingLocked(segs []Segment) {
	t := &c.turns[c.activeTurn]
	if t.Think != 0 || c.promptAt.IsZero() {
		return
	}
	for _, s := range segs {
		if s.Kind == SegText {
			t.Think = c.clock().Sub(c.promptAt)
			return
		}
	}
}
