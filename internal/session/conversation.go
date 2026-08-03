package session

import (
	"bufio"
	"encoding/json"
	"io"
	"sync"
	"time"
)

// conversation accumulates parsed turns from the event stream. It is the
// testable core of the driver: feed it lines (or a reader) and read back the
// turn list, no child process required. All state is guarded by mu, since the
// reader goroutine mutates it while the UI goroutine snapshots it.
type conversation struct {
	mu        sync.Mutex
	turns     []Turn
	sessionID string
	model     string
	state     State
	errMsg    string

	// Live-streaming state (--include-partial-messages): the in-flight
	// assistant message is built from deltas into turns[activeTurn] and then
	// reconciled by the authoritative `assistant` event.
	//
	// activeTurn is -1 until the message has produced something worth showing. It
	// used to be created at message_start, which is why a reply arrived with an
	// empty agent bubble above it: a message that streams nothing renderable (it
	// happens - an empty text block before a tool call, a message that is only
	// thinking) left a turn with no segments and agentbox drew its identity pill
	// anyway. A turn now appears when it has content.
	streamingActive bool
	activeTurn      int
	streaming       []blockBuf
	lastParse       time.Time

	// promptAt is when the last user prompt went out, which is where the thinking
	// clock starts.
	promptAt time.Time

	// now is the clock, overridable in tests.
	now func() time.Time

	// onUpdate fires after every mutation (off the lock) so the owner can wake
	// the UI. nil is fine (tests).
	onUpdate func()
}

func (c *conversation) snapshot() []Turn {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Turn, len(c.turns))
	copy(out, c.turns)
	return out
}

func (c *conversation) info() (sessionID, model string, st State, errMsg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID, c.model, c.state, c.errMsg
}

func (c *conversation) setState(s State) {
	c.mu.Lock()
	c.state = s
	c.mu.Unlock()
	c.notify()
}

// clock is the conversation's now, so a test can pin it.
func (c *conversation) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// append adds a turn (a system banner or error notice from the driver).
func (c *conversation) append(t Turn) {
	c.mu.Lock()
	if t.At.IsZero() {
		t.At = c.clock()
	}
	c.turns = append(c.turns, t)
	c.mu.Unlock()
	c.notify()
}

// addUserPrompt records a prompt we are about to send, so it shows immediately
// without waiting for a replay event, and moves to the working state.
func (c *conversation) addUserPrompt(text string) {
	c.mu.Lock()
	now := c.clock()
	c.turns = append(c.turns, Turn{
		Role:     RoleUser,
		Segments: []Segment{{Kind: SegText, Text: text}},
		At:       now,
	})
	c.promptAt = now
	c.state = StateWorking
	c.mu.Unlock()
	c.notify()
}

func (c *conversation) notify() {
	if c.onUpdate != nil {
		c.onUpdate()
	}
}

// consume reads NDJSON lines until EOF, applying each. Heavy parsing (JSON +
// markdown) happens here, on the caller's goroutine - the reader goroutine in
// production, the test goroutine in tests. ReadBytes (not bufio.Scanner) so a
// large assistant message never trips a token-size limit.
func (c *conversation) consume(r io.Reader) {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			c.applyLine(line)
		}
		if err != nil {
			return
		}
	}
}

// applyLine parses one NDJSON line and folds it into the turn list. Malformed
// or unrecognised lines are skipped (the schema is large and we decode only
// what we render).
func (c *conversation) applyLine(line []byte) {
	var ev wireEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return
	}
	switch ev.Type {
	case "system":
		if ev.Subtype == "init" {
			c.mu.Lock()
			if ev.SessionID != "" {
				c.sessionID = ev.SessionID
			}
			if ev.Model != "" {
				c.model = ev.Model
			}
			c.mu.Unlock()
			c.notify()
		}
	case "assistant":
		c.applyAssistant(ev)
	case "user":
		c.applyUser(ev)
	case "result":
		c.applyResult(ev)
	case "stream_event":
		c.applyStreamEvent(ev)
	}
}

func (c *conversation) applyAssistant(ev wireEvent) {
	var msg wireMessage
	if err := json.Unmarshal(ev.Message, &msg); err != nil {
		return
	}
	segs := assistantSegments(msg.Content)
	c.mu.Lock()
	if ev.Err != "" {
		c.state = StateError
		c.errMsg = ev.Err
		c.streamingActive = false
		c.streaming = nil
		c.turns = append(c.turns, systemErrorTurn("Claude reported an error: %s", ev.Err))
		c.mu.Unlock()
		c.notify()
		return
	}
	// Reconcile a streamed turn with the authoritative message: the deltas
	// built a live preview, the assistant event is the final word.
	if c.streamingActive && c.activeTurn >= 0 && c.activeTurn < len(c.turns) {
		c.turns[c.activeTurn].Model = msg.Model
		if len(segs) > 0 {
			c.turns[c.activeTurn].Segments = segs
			c.markThinkingLocked(segs)
		}
		c.streamingActive = false
		c.streaming = nil
		c.mu.Unlock()
		c.notify()
		return
	}
	if len(segs) == 0 {
		// Nothing renderable, and no turn was created for it either (the streamed
		// turn is created on its first segment): there is nothing to leave behind.
		c.streamingActive = false
		c.streaming = nil
		c.mu.Unlock()
		return
	}
	t := Turn{Role: RoleAssistant, Model: msg.Model, Segments: segs, At: c.clock()}
	if !c.promptAt.IsZero() {
		for _, s := range segs {
			if s.Kind == SegText {
				t.Think = c.clock().Sub(c.promptAt)
				break
			}
		}
	}
	c.turns = append(c.turns, t)
	c.streamingActive = false
	c.streaming = nil
	c.mu.Unlock()
	c.notify()
}

// applyUser folds an incoming user event. A plain-string content is a replay
// of a prompt we already showed, so it is ignored; a tool_result is attached
// to its originating tool call so the result reads under the call.
func (c *conversation) applyUser(ev wireEvent) {
	var msg wireMessage
	if err := json.Unmarshal(ev.Message, &msg); err != nil {
		return
	}
	blocks := decodeBlocks(msg.Content)
	changed := false
	c.mu.Lock()
	for _, b := range blocks {
		if b.Type != "tool_result" {
			continue
		}
		if c.attachResultLocked(b.ToolUseID, blockText(b.Content), b.IsError) {
			changed = true
		}
	}
	c.mu.Unlock()
	if changed {
		c.notify()
	}
}

// attachResultLocked finds the most recent tool call with the given id that is
// still awaiting a result and fills it in. Caller holds mu.
func (c *conversation) attachResultLocked(toolID, text string, isErr bool) bool {
	if toolID == "" {
		return false
	}
	for i := len(c.turns) - 1; i >= 0; i-- {
		for j := range c.turns[i].Segments {
			s := &c.turns[i].Segments[j]
			if s.Kind == SegToolUse && s.ToolID == toolID && !s.HasResult {
				s.Result = truncate(text, 240)
				s.HasResult = true
				s.IsError = isErr
				return true
			}
		}
	}
	return false
}

func (c *conversation) applyResult(ev wireEvent) {
	c.mu.Lock()
	if ev.IsError || ev.Subtype == "error" {
		c.state = StateError
		c.errMsg = ev.Result
		c.turns = append(c.turns, systemErrorTurn("Response failed: %s", ev.Result))
		c.mu.Unlock()
		c.notify()
		return
	}
	// Attribute the response cost to the last assistant turn, and go idle:
	// stdin stays open, so the child is ready for the next prompt.
	for i := len(c.turns) - 1; i >= 0; i-- {
		if c.turns[i].Role == RoleAssistant {
			c.turns[i].CostUSD = ev.TotalCostUSD
			break
		}
	}
	if c.state == StateWorking {
		c.state = StateIdle
	}
	c.mu.Unlock()
	c.notify()
}
