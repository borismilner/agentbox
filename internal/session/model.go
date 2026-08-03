// Package session drives a headless Claude Code child over the stream-json
// protocol (M8, FR49): it spawns `claude -p --input-format stream-json
// --output-format stream-json --verbose`, writes user prompts to its stdin,
// and parses the NDJSON event stream on a reader goroutine into a list of
// rendered turns. The JSON decode runs on that reader goroutine, never on the
// UI's own thread, so the shared app window stays responsive (the load-bearing
// rule from the M8 app shell). The package is UI-free - it hands the UI raw
// markdown and never renders any - so the parser is unit-testable with canned
// NDJSON, no real `claude`.
package session

import "time"

// Role labels a turn in the conversation.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system" // init banner and error notices
)

// SegKind discriminates the parts of a turn. A single assistant message can
// interleave prose, extended thinking and tool calls, so a turn is an ordered
// list of segments rather than one blob.
type SegKind int

const (
	SegText       SegKind = iota // markdown prose
	SegThinking                  // extended thinking, rendered muted
	SegToolUse                   // a tool call (name + input summary)
	SegToolResult                // a standalone tool result (when its call is unknown)
)

// Segment is one renderable part of a turn. Text/thinking carry the raw
// markdown source and the UI renders it (internal/webui/mdhtml.go); tool
// segments carry a name and compact summaries instead.
type Segment struct {
	Kind SegKind
	Text string // raw source for text/thinking; "" for tool segments

	// Tool call (SegToolUse) and result fields.
	ToolName  string
	ToolID    string
	ToolInput string // one-line summary of the call's input
	Result    string // tool result text, filled when it arrives
	HasResult bool   // a result has been attached to this call
	IsError   bool   // the tool call or result reported an error
}

// Turn is one role's contribution to the conversation. An assistant turn is a
// single complete `assistant` message; a user turn is one prompt we sent; a
// system turn carries the init banner or an error notice.
type Turn struct {
	Role     Role
	Segments []Segment
	Model    string  // assistant turn's model, when known
	CostUSD  float64 // per-response cost from the result event (last turn of a response)
	Err      string  // error text for a system error turn

	// At is when the turn appeared, for the clock the surface shows beside it. A
	// conversation you come back to needs to say when, not just what.
	At time.Time
	// Think is how long the model worked before the first word of its answer
	// arrived: the prompt going out to the first text of the reply, including
	// extended thinking and any tool calls it made on the way. Zero on a turn that
	// never produced prose, and on every user turn.
	Think time.Duration
}

// State is the driver's lifecycle phase.
type State int

const (
	StateIdle    State = iota // ready for a prompt (also the initial state)
	StateWorking              // a prompt is in flight, response streaming
	StateEnded                // the child exited cleanly (stdin closed)
	StateError                // the child failed to spawn, crashed, or reported an error
)

func (s State) String() string {
	switch s {
	case StateWorking:
		return "working"
	case StateEnded:
		return "ended"
	case StateError:
		return "error"
	default:
		return "idle"
	}
}
