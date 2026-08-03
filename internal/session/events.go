package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// wireEvent is the stream-json envelope: one JSON object per NDJSON line. Only
// the fields agentbox acts on are decoded; the rest of the (large) schema is
// ignored. Verified against Claude Code v2.1.177.
type wireEvent struct {
	Type      string          `json:"type"`    // system | assistant | user | result | stream_event | rate_limit_event
	Subtype   string          `json:"subtype"` // init | success | error | status | ...
	SessionID string          `json:"session_id"`
	Message   json.RawMessage `json:"message"` // assistant/user: the embedded API message
	Event     json.RawMessage `json:"event"`   // stream_event: the raw API streaming event

	// system/init
	Model string `json:"model"`
	Cwd   string `json:"cwd"`

	// assistant
	Err string `json:"error"` // e.g. "authentication_failed", null when fine

	// result
	IsError      bool    `json:"is_error"`
	Result       string  `json:"result"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

// wireMessage is the Anthropic API message carried by assistant/user events.
// Content is a string (a plain user prompt) or an array of content blocks.
type wireMessage struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
}

// wireBlock is one content block inside a message: text, thinking, tool_use or
// tool_result.
type wireBlock struct {
	Type string `json:"type"`

	Text     string `json:"text"`     // text
	Thinking string `json:"thinking"` // thinking

	// tool_use
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`

	// tool_result
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"` // string or [] of blocks
	IsError   bool            `json:"is_error"`
}

// assistantSegments turns an assistant message's content blocks into renderable
// segments, parsing prose markdown here (off the frame goroutine).
func assistantSegments(content json.RawMessage) []Segment {
	blocks := decodeBlocks(content)
	segs := make([]Segment, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) == "" {
				continue
			}
			segs = append(segs, Segment{Kind: SegText, Text: b.Text})
		case "thinking":
			if strings.TrimSpace(b.Thinking) == "" {
				continue
			}
			segs = append(segs, Segment{Kind: SegThinking, Text: b.Thinking})
		case "tool_use":
			segs = append(segs, Segment{
				Kind:      SegToolUse,
				ToolName:  b.Name,
				ToolID:    b.ID,
				ToolInput: summarizeInput(b.Input),
			})
		}
	}
	return segs
}

// decodeBlocks decodes a message's content, which is either a JSON string (a
// plain prompt) or an array of content blocks. A bare string yields a single
// text block.
func decodeBlocks(content json.RawMessage) []wireBlock {
	if len(content) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return []wireBlock{{Type: "text", Text: s}}
	}
	var blocks []wireBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil
	}
	return blocks
}

// blockText extracts the visible text of a tool_result's content (a string or
// an array of text blocks).
func blockText(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(content, &parts); err != nil {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		if p.Text != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

// inputKeys are the fields worth showing first in a tool-call summary, in
// priority order; most tools key their primary argument on one of these.
var inputKeys = []string{"command", "file_path", "path", "pattern", "url", "query", "prompt", "description"}

// summarizeInput renders a tool call's input as one compact line: the primary
// argument when there is an obvious one, else the whole object compacted. Long
// summaries are truncated so a tool row never dominates the conversation.
func summarizeInput(raw json.RawMessage) string {
	const cap = 160
	if len(raw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err == nil {
		for _, k := range inputKeys {
			if v, ok := m[k]; ok {
				return truncate(scalar(v), cap)
			}
		}
	}
	return truncate(string(compact(raw)), cap)
}

// scalar renders a JSON value as a plain string (unquoting a JSON string),
// for use in a one-line summary.
func scalar(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(compact(raw))
}

// compact removes insignificant whitespace from a JSON value; on error it
// returns the input unchanged.
func compact(raw json.RawMessage) []byte {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return raw
	}
	return buf.Bytes()
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// systemErrorTurn builds a system turn carrying an error notice.
func systemErrorTurn(format string, args ...any) Turn {
	msg := fmt.Sprintf(format, args...)
	return Turn{Role: RoleSystem, Err: msg, Segments: []Segment{{Kind: SegText, Text: msg}}}
}
