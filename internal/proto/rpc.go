package proto

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// Method names. Versioned so a future v2 can coexist on the same socket.
const (
	MethodNotify   = "agentbox.v1.notify"
	MethodAsk      = "agentbox.v1.ask"
	MethodCancel   = "agentbox.v1.cancel"
	MethodList     = "agentbox.v1.list"
	MethodStatus   = "agentbox.v1.status"
	MethodInbox    = "agentbox.v1.inbox"
	MethodQuit     = "agentbox.v1.quit"
	MethodDnd      = "agentbox.v1.dnd"
	MethodStats    = "agentbox.v1.stats"
	MethodSummon   = "agentbox.v1.summon"
	MethodPanel    = "agentbox.v1.panel"
	MethodShow     = "agentbox.v1.show"
	MethodMute     = "agentbox.v1.mute"
	MethodProgress = "agentbox.v1.progress"
	MethodApp      = "agentbox.v1.app"
	MethodSpeak    = "agentbox.v1.speak"
	MethodDrive    = "agentbox.v1.drive"
	// MethodAloud is the human asking to hear a screen, and working the transport
	// while they do. Distinct from speak: that is an agent choosing to say one
	// line, this is a reading somebody drives.
	MethodAloud = "agentbox.v1.aloud"
	// MethodControl is an agent asking for the desktop and then saying what it is
	// doing with it (FR74). One strip on screen for the length of a run: while it
	// is there the desktop is the agent's, and when it goes it is the human's
	// again. That presence IS the signal, which is why there is no idle state.
	MethodControl = "agentbox.v1.control"
	// The artifact channel (M10): wait for the human to act in an artifact, or
	// take whatever they have done since you last looked.
	MethodArtifactWait = "agentbox.v1.artifact_wait"
	MethodArtifactRead = "agentbox.v1.artifact_read"
	// The walkthrough family (FR58/FR59): a durable, addressable review that
	// outlives the agent that created it. Types in walkthrough.go.
	MethodWalkthroughCreate = "agentbox.v1.walkthrough_create"
	MethodWalkthroughAmend  = "agentbox.v1.walkthrough_amend"
	MethodWalkthroughRead   = "agentbox.v1.walkthrough_read"
	MethodWalkthroughAwait  = "agentbox.v1.walkthrough_await"
	MethodWalkthroughList   = "agentbox.v1.walkthrough_list"
	MethodWalkthroughDelete = "agentbox.v1.walkthrough_delete"
	MethodWalkthroughOpen   = "agentbox.v1.walkthrough_open"
	MethodWalkthroughRepair = "agentbox.v1.walkthrough_repair"
	// The assignment family (M12/FR82): recurring work agentbox gives an agent, which
	// an agent may also author and edit. Types in assignment.go.
	MethodAssignmentList   = "agentbox.v1.assignment_list"
	MethodAssignmentRead   = "agentbox.v1.assignment_read"
	MethodAssignmentSave   = "agentbox.v1.assignment_save"
	MethodAssignmentDelete = "agentbox.v1.assignment_delete"
	MethodAssignmentRun    = "agentbox.v1.assignment_run"
	MethodAssignmentRuns   = "agentbox.v1.assignment_runs"

	// Sync (FR83). attach blocks for the session's whole life: the call is the
	// presence, so it is the only method here that is not a question.
	MethodSyncAttach   = "agentbox.v1.sync_attach"
	MethodSyncAnnounce = "agentbox.v1.sync_announce"
	MethodSyncActivity = "agentbox.v1.sync_activity"
	MethodSyncList     = "agentbox.v1.sync_list"
)

// ArtifactEvent is one thing the human did inside an artifact: the name and data
// an artifact passed to window.agentbox.emit, plus which artifact it came from
// (M10; internal/webui/artifact.go). Data is whatever the artifact sent, carried
// as raw JSON because it is the agent's own vocabulary and agentbox has no business
// having an opinion about its shape - only about its size.
type ArtifactEvent struct {
	ArtifactID string          `json:"artifact_id,omitempty"`
	Name       string          `json:"name"`
	Data       json.RawMessage `json:"data,omitempty"`
	AtMS       int64           `json:"at_ms,omitempty"`
}

// NewArtifactID mints the name an artifact's events will carry. The caller mints
// it rather than the daemon so it can start listening the instant it has asked for
// the window: an agent that has to go back and ask what it just showed is an agent
// that can miss the click that happened in between.
func NewArtifactID() (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "a" + hex.EncodeToString(b[:]), nil
}

// ArtifactWait blocks until an artifact emits. An empty ArtifactID waits on any
// artifact and an empty Names on any event, so an agent that showed one thing can
// wait with no bookkeeping. TimeoutS 0 waits as long as the caller does.
type ArtifactWait struct {
	ArtifactID string   `json:"artifact_id,omitempty"`
	Names      []string `json:"names,omitempty"`
	TimeoutS   int      `json:"timeout_s,omitempty"`
}

// ArtifactWaitResult reports one event, or that the window elapsed without one.
type ArtifactWaitResult struct {
	Received bool           `json:"received"`
	Event    *ArtifactEvent `json:"event,omitempty"`
	TimedOut bool           `json:"timed_out,omitempty"`
}

// ArtifactReadResult drains what arrived while nobody was waiting, newest value
// per event name (a dragged slider is one number, not forty).
type ArtifactReadResult struct {
	Events []ArtifactEvent `json:"events"`
}

// DriveRequest runs a synthetic-input script on the desktop: the pointer moves,
// buttons click, keys are pressed, as if the person at the keyboard did it. The
// script's grammar lives in internal/hand; it travels as one string because a
// sequence is the unit that matters - the whole thing is parsed before the first
// event is sent, so a typo cannot leave a button held down halfway through.
type DriveRequest struct {
	Script string  `json:"script"`
	Speed  float64 `json:"speed,omitempty"` // 1 = a hand's pace; higher is faster
	WPM    int     `json:"wpm,omitempty"`   // typing speed, default 300
}

// DriveResult reports how many steps ran.
type DriveResult struct {
	Steps int `json:"steps"`
}

// SpeakRequest is a line to read out loud, with no card behind it.
//
// Wait moves the answer from "queued" to "heard": the daemon replies when the
// audio has finished rather than when the engine has been handed the line. That is
// what a narrated sequence needs - the next line starts on the last word of this
// one instead of after a guess at how long the sentence took - and it is why the
// daemon measures the PCM (see internal/speech).
type SpeakRequest struct {
	Text string `json:"text"`
	Wait bool   `json:"wait,omitempty"`
}

// SpeakResult says the line was accepted, and whether the call waited for it. It
// is still silent if the human has speech off or is in quiet hours: agentbox reports
// what it did, not what it heard.
type SpeakResult struct {
	OK     bool `json:"ok"`
	Waited bool `json:"waited,omitempty"`
}

// The read-aloud controls. Start reads one region of a screen and replaces
// whatever was being read; stop takes the sound away; state asks and changes
// nothing, which is how a surface notices a reading ended on its own.
//
// There is no pause, resume or rewind, and that is the FR72 decision rather than
// an omission: all three needed the text split into passages, and splitting is
// what cost the speech both its prosody and its last words (speech.Utterance and
// speech.drainStart carry the why and the measurements). Per-region play is the
// control the reader actually wanted anyway.
const (
	AloudStart = "start"
	AloudStop  = "stop"
	AloudState = "state"
)

// AloudRequest is one action. Region names the part of the screen being read, so
// the surface can tell which of its controls to paint as playing; the daemon
// stores it and never interprets it. Text is read only by start.
type AloudRequest struct {
	Action string `json:"action"`
	Region string `json:"region,omitempty"`
	Text   string `json:"text,omitempty"`
}

// AloudResult is the reader's state after the action, so a surface can paint its
// controls from the answer rather than from a guess. Region is empty whenever
// nothing is playing.
type AloudResult struct {
	OK      bool   `json:"ok"`
	Playing bool   `json:"playing"`
	Region  string `json:"region,omitempty"`
}

// The control strip's verbs (FR74). Request blocks until the human grants or
// denies; activity updates the line while a run is live; release ends the run and
// takes the strip off the screen.
const (
	ControlRequest  = "request"
	ControlActivity = "activity"
	ControlRelease  = "release"
	ControlState    = "state"
)

// The states a live run can be in. There are only two, and that is a rule: the
// strip's presence is what tells the human to keep his hands off, so a state
// meaning "working, but you may touch things" would make presence ambiguous. An
// agent busy without needing the desktop shows nothing at all.
const (
	ControlAsking  = "asking"  // waiting to be granted the desktop
	ControlDriving = "driving" // granted; hands off
)

// ControlRequest is one verb. Reason says what the agent is about to do, in the
// human's terms, and is what the strip shows while asking. WindowS is how long
// the countdown runs before silence counts as consent; Activity is the line to
// show, on the activity verb.
// Sync (FR83): who else is here, what they are for, and what they are doing.
//
// SyncAttach is the odd one: it BLOCKS for the whole life of the session and
// never returns a useful result, because the call itself is the presence. Its
// context dying is the agent going away, which is the same insight FR45 had
// per-call, promoted to per-session.
type SyncAttachParams struct {
	Identity Identity `json:"identity"`
	// Cwd, PID and Area are what the child knows without asking its model
	// anything, which is what makes a rude agent visible instead of invisible.
	Cwd  string   `json:"cwd,omitempty"`
	PID  int      `json:"pid,omitempty"`
	Area string   `json:"area,omitempty"`
	Tags []string `json:"tags,omitempty"`
}

// SyncAnnounceParams states what a session is FOR. Purpose is the mandate;
// activity is optional here because set_activity carries it from then on.
type SyncAnnounceParams struct {
	Identity Identity `json:"identity"`
	Purpose  string   `json:"purpose"`
	Activity string   `json:"activity,omitempty"`
	Area     string   `json:"area,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

// SyncActivityParams updates the caller's roster line. It is the same verb the
// control strip already had, generalized: it writes the roster always, and the
// hands-off strip too when the caller happens to hold the desktop.
type SyncActivityParams struct {
	Identity Identity `json:"identity"`
	Activity string   `json:"activity"`
}

// SyncListParams filters the roster. Empty means everybody.
type SyncListParams struct {
	Identity Identity `json:"identity"`
	Area     string   `json:"area,omitempty"`
	Project  string   `json:"project,omitempty"`
}

// SyncAgent is one roster row on the wire.
type SyncAgent struct {
	Key     string `json:"key"`
	Agent   string `json:"agent"`
	Project string `json:"project,omitempty"`
	Session string `json:"session,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
	PID     int    `json:"pid,omitempty"`

	Area string   `json:"area,omitempty"`
	Tags []string `json:"tags,omitempty"`
	// AreaPath is where the area itself lives - the repo root, not this row's
	// cwd, and empty when the two have nothing to do with each other. A surface
	// that captions a group of agents with a path needs this rather than any one
	// member's cwd: agents in one repo sit in different subdirectories, and an
	// agent may declare an area it is not standing in.
	AreaPath string `json:"area_path,omitempty"`

	Purpose  string `json:"purpose,omitempty"`
	Activity string `json:"activity,omitempty"`

	// State is what the daemon observed, never what the agent claimed.
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`

	ActivitySinceMS int64 `json:"activity_since_ms"`
	AgeMS           int64 `json:"age_ms"`
}

// SyncResult answers announce and list alike: the caller's own peers.
//
// Partial is the honesty bit. A session whose mcp child predates this feature
// has no attach and no row, so a roster that omitted it while saying "you are
// alone" would be lying. When Partial is set, the list is not everybody, and
// nobody may conclude absence from it.
type SyncResult struct {
	OK      bool        `json:"ok"`
	Agents  []SyncAgent `json:"agents,omitempty"`
	Partial bool        `json:"partial,omitempty"`
	// Peers counts rows sharing the caller's area, excluding the caller. It is
	// separate from len(Agents) because a filtered list and "am I alone" are
	// different questions.
	Peers int `json:"peers"`
	// Note carries a teaching sentence for a refusal or a nudge, in the
	// self-teaching style the rest of the tools use.
	Note string `json:"note,omitempty"`
}

type ControlRequestParams struct {
	Action string `json:"action"`
	// Identity names the agent, and here it is load-bearing rather than
	// decoration: it is what the strip shows the human ("which of my sessions is
	// driving?") and what stops a second agent writing over the driver's activity
	// line. Same contract as everywhere else - the caller supplies it, the daemon
	// fills nothing in.
	Identity Identity `json:"identity"`
	Reason   string   `json:"reason,omitempty"`
	WindowS  int      `json:"window_s,omitempty"`
	Activity string   `json:"activity,omitempty"`
}

// ControlResult is the run's state after the verb. Live says whether a run is on
// screen at all.
//
// On request, exactly one of three things happened, and an agent must read them
// apart before touching anything:
//
//	Granted            - the desktop is yours until you release it.
//	Denied             - the human said no. Do not touch the desktop.
//	HeldBy non-empty   - another agent has it. Not yours, and nobody was asked.
//
// The third case exists because the desktop is one resource and several agents
// reach this daemon (FR74: "other AI agents should also be able to use this
// functionality, for example while they drive my debug chrome"). A second
// requester is refused rather than queued: a hidden queue means an agent is
// granted the mouse minutes after it asked, when it has moved on to something
// else, and two agents each believing they hold the pointer is the failure this
// whole feature exists to prevent. Retrying is the caller's business.
type ControlResult struct {
	OK       bool   `json:"ok"`
	Live     bool   `json:"live"`
	Granted  bool   `json:"granted,omitempty"`
	Denied   bool   `json:"denied,omitempty"`
	State    string `json:"state,omitempty"`
	Activity string `json:"activity,omitempty"`
	HeldBy   string `json:"held_by,omitempty"` // the agent holding it, when not you
	Reason   string `json:"reason,omitempty"`  // what that agent said it was doing
}

// ProgressUpdate creates or updates a live progress report (FR21). An empty
// ID mints a new report (the daemon returns it in ProgressResult.ID); a known
// ID updates that report in place. Percent is clamped 0..100; Indeterminate
// shows a spinner instead of a bar (use it when the fraction is unknown).
// Done finalizes the report: the progress card clears and one completion toast
// follows - success, or error when Error is non-empty. The progress display
// is non-blocking: it never enters the card queue, so reports run alongside
// questions and notifications.
type ProgressUpdate struct {
	ID            string   `json:"id,omitempty"`
	Title         string   `json:"title,omitempty"`
	Status        string   `json:"status,omitempty"` // one line under the bar
	Percent       int      `json:"percent,omitempty"`
	Indeterminate bool     `json:"indeterminate,omitempty"`
	Done          bool     `json:"done,omitempty"`
	Error         string   `json:"error,omitempty"` // Done + Error => the task failed
	Hold          bool     `json:"hold,omitempty"`  // tie the report to this connection: reap it if the connection drops before Done (used by the CLI, whose pipe lives for the task)
	Identity      Identity `json:"identity,omitzero"`
}

// ProgressResult returns the report's ID so the caller can update it.
type ProgressResult struct {
	ID string `json:"id"`
}

// ShowRequest opens the document viewer (FR36-38). The client resolves Path
// to an absolute path (the daemon's cwd differs); Content carries stdin or
// inline text when there is no file. Watch live-reloads the file on change.
//
// Artifact says the payload is interactive HTML rather than markdown (M10): the
// same window, but the content is run in a sandbox instead of rendered as prose.
// It rides Show rather than a method of its own so an artifact inherits the
// window, the title rule and Watch - an agent that writes app.html and shows it
// watched gets a live preview of its own work for free.
//
// ArtifactID names the artifact for the channel back (MethodArtifactWait): the
// caller mints it so it can wait on it the moment the tool returns.
type ShowRequest struct {
	Path       string `json:"path,omitempty"`
	Content    string `json:"content,omitempty"`
	Title      string `json:"title,omitempty"`
	Watch      bool   `json:"watch,omitempty"`
	Artifact   bool   `json:"artifact,omitempty"`
	ArtifactID string `json:"artifact_id,omitempty"`
}

// JSON-RPC 2.0 error codes plus the agentbox application range (1000+).
const (
	CodeParse          = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternal       = -32603
	CodeItemNotFound   = 1000
	CodeShuttingDown   = 1001
)

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("rpc %d: %s", e.Code, e.Message) }

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Handler serves one request. Blocking methods (ask) may take minutes; the
// connection serves other requests meanwhile.
type Handler func(ctx context.Context, method string, params json.RawMessage) (any, *RPCError)

// Conn frames JSON-RPC 2.0 as one JSON object per line over any stream.
// Safe for concurrent Calls; Serve dispatches each request on its own
// goroutine.
type Conn struct {
	rwc    io.ReadWriteCloser
	enc    *json.Encoder
	encMu  sync.Mutex
	nextID atomic.Int64

	mu      sync.Mutex
	pending map[int64]chan response
	readErr error
	reading bool
}

func NewConn(rwc io.ReadWriteCloser) *Conn {
	return &Conn{
		rwc:     rwc,
		enc:     json.NewEncoder(rwc),
		pending: make(map[int64]chan response),
	}
}

func (c *Conn) Close() error { return c.rwc.Close() }

func (c *Conn) send(v any) error {
	c.encMu.Lock()
	defer c.encMu.Unlock()
	return c.enc.Encode(v)
}

// Call sends a request and blocks until the response arrives, the context
// is done, or the connection fails.
func (c *Conn) Call(ctx context.Context, method string, params, result any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal params: %w", err)
	}
	id := c.nextID.Add(1)
	ch := make(chan response, 1)

	c.mu.Lock()
	if c.readErr != nil {
		err := c.readErr
		c.mu.Unlock()
		return err
	}
	c.pending[id] = ch
	if !c.reading {
		c.reading = true
		go c.readLoop()
	}
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := c.send(request{JSONRPC: "2.0", ID: &id, Method: method, Params: raw}); err != nil {
		return fmt.Errorf("send %s: %w", method, err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp, ok := <-ch:
		if !ok {
			c.mu.Lock()
			err := c.readErr
			c.mu.Unlock()
			return fmt.Errorf("connection closed during %s: %w", method, err)
		}
		if resp.Error != nil {
			return resp.Error
		}
		if result != nil {
			if err := json.Unmarshal(resp.Result, result); err != nil {
				return fmt.Errorf("unmarshal %s result: %w", method, err)
			}
		}
		return nil
	}
}

func (c *Conn) readLoop() {
	sc := bufio.NewScanner(c.rwc)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var resp response
		if err := json.Unmarshal(sc.Bytes(), &resp); err != nil || resp.ID == nil {
			continue
		}
		c.mu.Lock()
		ch, ok := c.pending[*resp.ID]
		c.mu.Unlock()
		if ok {
			ch <- resp
		}
	}
	err := sc.Err()
	if err == nil {
		err = io.EOF
	}
	c.mu.Lock()
	c.readErr = err
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.mu.Unlock()
}

// Serve reads requests until the stream closes, dispatching each to h on
// its own goroutine so a blocked ask does not starve the connection. It
// returns when the peer disconnects or ctx is done.
func (c *Conn) Serve(ctx context.Context, h Handler) error {
	ctx, cancel := context.WithCancel(ctx)
	// Order matters, and it is the whole of FR45 working or not working.
	//
	// Deferred calls run last-registered-first, so cancel() has to be registered
	// AFTER wg.Wait() to run BEFORE it. With the two the other way round, a
	// handler blocked on ctx.Done() - every blocking ask, and FR83's attach
	// stream - waits for a cancel that waits for the handler, and the peer
	// hanging up is never noticed at all: the goroutine leaks until daemon
	// shutdown, the card never auto-dismisses, and presence keyed to a call's
	// context can never expire. Cancel first, then wait for the woken handlers
	// to send whatever they still owe.
	var wg sync.WaitGroup
	defer wg.Wait()
	defer cancel()

	sc := bufio.NewScanner(c.rwc)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := make([]byte, len(sc.Bytes()))
		copy(line, sc.Bytes())

		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = c.send(response{JSONRPC: "2.0", Error: &RPCError{Code: CodeParse, Message: "invalid JSON"}})
			continue
		}
		if req.JSONRPC != "2.0" || req.Method == "" {
			_ = c.send(response{JSONRPC: "2.0", ID: req.ID, Error: &RPCError{Code: CodeInvalidRequest, Message: "expected jsonrpc 2.0 with a method"}})
			continue
		}
		wg.Add(1)
		go func(req request) {
			defer wg.Done()
			// Backstop: a handler panic must never crash the daemon or leave
			// the caller hanging. The handler is expected to recover and log
			// (it owns the logger); this guarantees a reply either way.
			defer func() {
				if r := recover(); r != nil && req.ID != nil {
					_ = c.send(response{JSONRPC: "2.0", ID: req.ID,
						Error: &RPCError{Code: CodeInternal, Message: "internal error handling " + req.Method}})
				}
			}()
			result, rpcErr := h(ctx, req.Method, req.Params)
			if req.ID == nil {
				return // notification semantics: no response
			}
			resp := response{JSONRPC: "2.0", ID: req.ID}
			if rpcErr != nil {
				resp.Error = rpcErr
			} else {
				raw, err := json.Marshal(result)
				if err != nil {
					resp.Error = &RPCError{Code: CodeInternal, Message: "marshal result: " + err.Error()}
				} else {
					resp.Result = raw
				}
			}
			_ = c.send(resp)
		}(req)
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		return err
	}
	return nil
}
