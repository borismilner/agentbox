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
	MethodNotify = "agentbox.v1.notify"
	MethodAsk    = "agentbox.v1.ask"
	MethodCancel = "agentbox.v1.cancel"
	// Dismiss retires pending items without the mouse (FR89). Two callers, one
	// method: the human clearing his own queue from a terminal, and an agent taking
	// back something it posted and now knows to be noise. The identity decides which
	// - an agent may only ever touch its own, and only the human's own door may pass
	// `all`.
	MethodDismiss  = "agentbox.v1.dismiss"
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
	// Locks (FR83 slice 2). acquire BLOCKS in a FIFO queue; try never does, which
	// is why they are two methods and not one flag (the FR74 rule: a blocking and
	// a non-blocking verb never share a door). break_lock is the human's, from the
	// Agents surface, and locks is the read both the surface and the CLI use.
	MethodSyncLock      = "agentbox.v1.sync_lock"
	MethodSyncTryLock   = "agentbox.v1.sync_try_lock"
	MethodSyncUnlock    = "agentbox.v1.sync_unlock"
	MethodSyncBreakLock = "agentbox.v1.sync_break_lock"
	MethodSyncLocks     = "agentbox.v1.sync_locks"
	// Signals (FR83 slice 3). post never blocks and await always may, so they are
	// two methods for the same reason acquire and try are. The split matters more
	// here than anywhere: await is the design's one sanctioned way to spend a turn
	// doing nothing, and a caller has to be able to tell from the door.
	MethodSyncPost  = "agentbox.v1.sync_post"
	MethodSyncAwait = "agentbox.v1.sync_await"
	// Shared values (FR83 slice 4). One method for get, set and delete, because
	// none of the three blocks: the rule that splits acquire from try is about a
	// caller being able to tell from the door whether calling it parks the turn, and
	// here the answer is no for every operation.
	MethodSyncShared = "agentbox.v1.sync_shared"
)

// The three shared-value operations, as the wire spells them.
const (
	SharedOpGet    = "get"
	SharedOpSet    = "set"
	SharedOpDelete = "delete"
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
	// The human's own two verbs (FR94): pause latches the desktop back to him
	// mid-run without ending the run, resume hands it on. They are reachable from
	// the strip, the hotkey and the CLI, and deliberately NOT from MCP - an agent
	// that could resume its own pause would make the pause a suggestion.
	ControlPause  = "pause"
	ControlResume = "resume"
	// And his two for a recording (FR95). quiet demotes the strip to the 4px
	// marker and lets a window cover it; loud puts it back. Also his own rather
	// than an agent's, for a smaller reason than the pause: an agent that could
	// quieten the sign it is being watched by would be marking its own homework.
	ControlQuiet = "quiet"
	ControlLoud  = "loud"
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
//
// Cwd is here for the same reason the attach carries it: the area is DERIVED from
// where the caller stands, and an announce can arrive before that session's attach
// does - a hook announces on a session's behalf before its child has made a single
// tool call. A row with no area is invisible to every area-filtered read, so
// leaving it out made a session unfindable by exactly the peers it needed to
// coordinate with (FR90).
type SyncAnnounceParams struct {
	Identity Identity `json:"identity"`
	Purpose  string   `json:"purpose"`
	Activity string   `json:"activity,omitempty"`
	Cwd      string   `json:"cwd,omitempty"`
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

	// Holds and Waiting are this session's locks (FR83 slice 2). Two agents
	// waiting on each other is then a drawn edge on the surface rather than a
	// diagram the human assembles in his head.
	Holds   []SyncHold `json:"holds,omitempty"`
	Waiting *SyncWait  `json:"waiting,omitempty"`
	// Listening is what this session is parked on in await_signal (slice 3). It is
	// a state, not a problem: an agent waiting to be told is the shape this feature
	// exists to make possible, which is why nothing ever warns about it.
	Listening *SyncListen `json:"listening,omitempty"`

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

// SyncHold is one lock a session holds, as a roster row shows it.
type SyncHold struct {
	Name    string `json:"name"`
	SinceMS int64  `json:"since_ms"`
	// Orphaned says the holder's session is gone while the process it recorded
	// lives on, so the lock is neither safely free nor actively held. It is the
	// state that makes a half-finished deploy visible instead of invisible.
	Orphaned bool `json:"orphaned,omitempty"`
	PID      int  `json:"pid,omitempty"`
	// PIDLive is whether that process is still there. It is the difference
	// between an orphan the next waiter is about to be granted and one that is
	// protecting work still in flight, which is the only thing the human needs to
	// know before reaching for Break lock.
	PIDLive bool   `json:"pid_live,omitempty"`
	Waiters int    `json:"waiters,omitempty"`
	Note    string `json:"note,omitempty"`
}

// SyncWait is the lock a session is parked on. HolderKey lets the surface make
// the wait clickable: two agents waiting on each other should be one click apart,
// not a puzzle.
type SyncWait struct {
	Name        string `json:"name"`
	SinceMS     int64  `json:"since_ms"`
	Ahead       int    `json:"ahead,omitempty"` // how many are in front in the queue
	Queue       int    `json:"queue,omitempty"` // how many wait in total, this row included
	HolderKey   string `json:"holder_key,omitempty"`
	HolderAgent string `json:"holder_agent,omitempty"`
}

// SyncLockParams is acquire, try and release alike (FR83 slice 2).
//
// TimeoutS keeps the meaning it has in every shipped blocking tool: 0 waits as
// long as the caller does, bounded by the daemon's own ceiling on a parked call.
// PID and TTLS are filled by the child or the CLI rather than by a model: they
// are about which process the hold follows, which is a fact about the caller.
type SyncLockParams struct {
	Identity Identity `json:"identity"`
	Name     string   `json:"name"`
	TimeoutS int      `json:"timeout_s,omitempty"`
	Note     string   `json:"note,omitempty"`
	// ReleaseOnDetach frees the hold the moment the session's attach drops,
	// instead of orphaning it. True when the session IS the critical section;
	// false (the default) when it started work that outlives it.
	ReleaseOnDetach bool `json:"release_on_detach,omitempty"`
	TTLS            int  `json:"ttl_s,omitempty"`
	PID             int  `json:"pid,omitempty"`
}

// SyncLockResult is the answer to every lock verb. Exactly one of Granted,
// TimedOut, Refused and Released describes what happened; the rest of the fields
// are the picture the caller would otherwise have to ask a second question for.
type SyncLockResult struct {
	OK      bool   `json:"ok"`
	Name    string `json:"name,omitempty"`
	Granted bool   `json:"granted,omitempty"`
	// TimedOut is a result, not an error: the wait ended without a grant and
	// nothing about the lock changed. Re-arming is one call.
	TimedOut bool `json:"timed_out,omitempty"`
	Refused  bool `json:"refused,omitempty"`
	Released bool `json:"released,omitempty"`
	Queued   bool `json:"queued,omitempty"`

	// Holder and its companions are the whole picture: who has it, what they are
	// for, what they are doing, how long they have held it, how many wait behind.
	Holder     *SyncAgent `json:"holder,omitempty"`
	HolderNote string     `json:"holder_note,omitempty"`
	HolderPID  int        `json:"holder_pid,omitempty"`
	HeldMS     int64      `json:"held_ms,omitempty"`
	Queue      int        `json:"queue,omitempty"`
	Orphaned   bool       `json:"orphaned,omitempty"`

	// Reason says why a grant happened - released, holder gone, broken by the
	// human, ttl expired - so a waiter always learns why it won.
	Reason   string `json:"reason,omitempty"`
	WaitedMS int64  `json:"waited_ms,omitempty"`
	// Deadlock names the cycle an acquire would have closed.
	Deadlock string `json:"deadlock,omitempty"`
	Note     string `json:"note,omitempty"`
}

// SyncLockState is one lock as the surface and the CLI read it.
type SyncLockState struct {
	Name        string `json:"name"`
	HolderKey   string `json:"holder_key,omitempty"`
	HolderAgent string `json:"holder_agent,omitempty"`
	Note        string `json:"note,omitempty"`
	PID         int    `json:"pid,omitempty"`
	HeldMS      int64  `json:"held_ms"`
	ExpiresInMS int64  `json:"expires_in_ms,omitempty"`
	Orphaned    bool   `json:"orphaned,omitempty"`
	Waiters     int    `json:"waiters,omitempty"`
}

// DismissParams retires pending items (FR89). Exactly one of ID and All says
// which; Identity says who is asking.
//
// The asymmetry is deliberate and it is the whole safety model. An agent's call
// may only ever retire items IT posted, because withdrawing another agent's
// question would answer for it. `all` belongs to the human alone: it is his queue,
// and an agent that could empty it could hide a question it did not like.
type DismissParams struct {
	Identity Identity `json:"identity"`
	ID       string   `json:"id,omitempty"`
	All      bool     `json:"all,omitempty"`
	// Mine restricts a sweep to the caller's own items, which is what an agent's
	// retraction means when it does not name one. Ignored when ID is set.
	Mine bool `json:"mine,omitempty"`
	// Human marks the caller as the person rather than an agent, which is what
	// unlocks `all`. The CLI sets it; the MCP tool cannot.
	Human bool `json:"human,omitempty"`
}

// DismissResult says how many went, and names them so a caller can see it
// touched what it meant to.
type DismissResult struct {
	OK        bool     `json:"ok"`
	Dismissed int      `json:"dismissed"`
	IDs       []string `json:"ids,omitempty"`
	Note      string   `json:"note,omitempty"`
}

// SyncLocksResult is the whole lock table, for a read that is nobody's turn.
type SyncLocksResult struct {
	OK    bool            `json:"ok"`
	Locks []SyncLockState `json:"locks,omitempty"`
}

// Signals (FR83 slice 3): fire-and-forget events with durable pickup. The
// matching rule for Topic patterns is in topic.go.
//
// SyncPostParams posts one. Data is whatever the agent wants to say, carried as
// raw JSON because it is the agent's own vocabulary and agentbox has no business
// having an opinion about its shape - only about its size.
type SyncPostParams struct {
	Identity Identity        `json:"identity"`
	Topic    string          `json:"topic"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// SyncPostResult is the sequence number the signal got, which is also the cursor
// a waiter would use to read from just before it.
//
// Delivered counts the parked waiters woken. It is honest rather than
// reassuring: fire-and-forget means zero is a perfectly good answer (the signal
// is stored, and a later await picks it up by cursor), so a poster that cares
// whether anybody heard it can see the difference instead of assuming.
type SyncPostResult struct {
	OK        bool   `json:"ok"`
	Topic     string `json:"topic"`
	Seq       int64  `json:"seq"`
	Delivered int    `json:"delivered"`
	Note      string `json:"note,omitempty"`
}

// SyncAwaitParams parks until a matching signal arrives.
//
// AfterSeq is the cursor, and zero is not a cursor: seq starts at 1, so zero
// means "from now on" - the reading a caller with nothing to resume from wants.
// A real cursor means "everything I have not seen", which may come back as a
// batch before the call ever parks.
type SyncAwaitParams struct {
	Identity Identity `json:"identity"`
	Topics   []string `json:"topics"`
	AfterSeq int64    `json:"after_seq,omitempty"`
	TimeoutS int      `json:"timeout_s,omitempty"`
}

// Signal is one delivered event. Key is the poster's session key, which is what
// makes a reply addressable: answering is a post to "to:<key>".
type Signal struct {
	Seq     int64           `json:"seq"`
	Topic   string          `json:"topic"`
	Agent   string          `json:"agent,omitempty"`
	Project string          `json:"project,omitempty"`
	Key     string          `json:"key,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	AtMS    int64           `json:"at_ms,omitempty"`
}

// SyncAgentDetail is what OPENING a row on the Agents board asks for, and it is
// deliberately not part of the roster payload: the roster is pushed on every
// change (a lock taken, an activity line, a second) and carrying twenty ticks and
// twenty signals per agent in each push would pay for a hundred rows nobody has
// opened. Fetched per opened row instead, the same shape FR73's ItemDetail
// settled on for the inbox.
type SyncAgentDetail struct {
	Key string `json:"key"`
	// Found separates "this session ended" from "this session has done nothing
	// yet", which look identical when both come back empty.
	Found bool `json:"found"`
	// Timeline is the activity lines this session has moved past, oldest first.
	Timeline []SyncTick `json:"timeline,omitempty"`
	// Signals are the ones it posted and the ones it was handed, merged and newest
	// last. Direction is on each entry because the two answer different questions
	// and a row that mixed them silently would be worse than either alone.
	Signals []SyncSignalTick `json:"signals,omitempty"`
}

// SyncTick is one thing a session was doing, with how long ago it started. Ages
// travel as ages here for the same reason they do everywhere on this surface: a
// wall-clock time would be the one thing on the board that lies after a suspend.
type SyncTick struct {
	Line    string `json:"line"`
	SinceMS int64  `json:"since_ms"`
}

// SyncSignalTick is one signal on a session's row.
type SyncSignalTick struct {
	Topic   string `json:"topic"`
	Dir     string `json:"dir"` // posted | received
	SinceMS int64  `json:"since_ms"`
	Data    string `json:"data,omitempty"`
}

// SyncAwaitResult is a batch and the cursor to resume from.
//
// Gap is the honesty bit, and it is the reason retention can be finite at all. A
// cursor older than what the store still holds cannot be served completely, and a
// batch that silently skipped what retention ate is how two agents both come to
// own the same chunk of work. So the gap is reported with the oldest sequence
// that did survive, and the caller decides what to do about it (FR61's rule, on
// the wire).
//
// More says the batch was capped and another call gets the rest at once, without
// parking. A cap exists because a cursor from last week could otherwise return
// hundreds of signals into an agent's context in one result.
type SyncAwaitResult struct {
	OK      bool     `json:"ok"`
	Signals []Signal `json:"signals,omitempty"`
	// Cursor is where to resume: the seq of the last signal in this batch, or the
	// unchanged cursor when nothing arrived. Always safe to pass straight back as
	// AfterSeq.
	Cursor    int64  `json:"cursor"`
	TimedOut  bool   `json:"timed_out,omitempty"`
	Gap       bool   `json:"gap,omitempty"`
	OldestSeq int64  `json:"oldest_seq,omitempty"`
	More      bool   `json:"more,omitempty"`
	Note      string `json:"note,omitempty"`
}

// Shared values (FR83 slice 4): the compare-and-swap blackboard. Coordination
// that is neither a turn (a lock) nor an event (a signal) - a claim table, a "who
// is on which chunk" map, a progress counter.
//
// SyncSharedParams carries all three operations, because none of them blocks.
//
// IfVersion is a POINTER because zero is a real value here, and a meaningful one:
// versions start at 1, so 0 says "only if this key does not exist yet", which is
// the whole claim idiom - ten workers CAS-from-empty on claims/<chunk> and exactly
// one wins each key with no lock and no read-modify-write. Omitted means write
// unconditionally; N means "only if it is still at N". (SyncAwaitParams.AfterSeq
// folds "omitted" into 0 for the same reason inverted: there, nothing needs to say
// "must be zero", so a plain int is honest. Here something does.)
type SyncSharedParams struct {
	Identity Identity `json:"identity"`
	Op       string   `json:"op"`
	// Key is exact for set and delete. For get it may end in * and read the whole
	// family, which is the same prefix rule topics use - one idiom, not two.
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value,omitempty"`
	// IfVersion: nil unconditional, 0 must-not-exist, N must-be-N. A delete refuses
	// 0, because "delete it only if it is not there" is not a thing to mean.
	IfVersion *int64 `json:"if_version,omitempty"`
	// Own records this session as the value's owner, so a peer can tell a live claim
	// from one a dead session left behind. A counter wants no owner; a claim does.
	Own bool `json:"own,omitempty"`
	// PID is the owning AGENT's process, recorded with an owned write. It is the one
	// fact about an owner that survives the daemon dying, which is why it is here: the
	// roster is memory only, so for the second it takes every child to reattach after
	// a restart, nothing on the roster can tell a live claim from an abandoned one.
	// Zero means none was recorded - honest for a CLI caller, whose shell dies within
	// seconds and whose session the roster answers for anyway.
	PID int `json:"pid,omitempty"`
}

// SharedValue is one entry as every reader sees it.
//
// OwnerGone is the field the design is really about. A value's contents cannot say
// whether the session that wrote it is still alive, so the owner is recorded at
// write time and checked against the live roster at read time: an orphaned claim
// becomes visible after a death or a restart instead of leaving the table
// permanently un-drainable. OwnerAgent is recorded with it, so the report can name
// which agent left it rather than only that somebody did.
type SharedValue struct {
	Key        string          `json:"key"`
	Value      json.RawMessage `json:"value,omitempty"`
	Version    int64           `json:"version"`
	Owner      string          `json:"owner,omitempty"`
	OwnerAgent string          `json:"owner_agent,omitempty"`
	// OwnerPID is the process the owner was running as, checked when the roster
	// cannot answer. Zero means none was recorded.
	OwnerPID  int   `json:"owner_pid,omitempty"`
	OwnerGone bool  `json:"owner_gone,omitempty"`
	UpdatedMS int64 `json:"updated_ms,omitempty"`
}

// SyncSharedResult answers all three operations. Applied and Stale are the CAS
// pair: a refused write comes back with the value and version that stopped it, so
// one retry loop replaces lock-read-write-unlock and the caller never needs a
// second call to find out what it lost to.
type SyncSharedResult struct {
	OK bool   `json:"ok"`
	Op string `json:"op"`
	// Found and Value answer an exact get; Values and More answer a prefix get.
	Found  bool          `json:"found,omitempty"`
	Value  *SharedValue  `json:"value,omitempty"`
	Values []SharedValue `json:"values,omitempty"`
	More   bool          `json:"more,omitempty"`
	// Applied says the write or delete landed. Stale says it did not, because
	// somebody else got there first - which for a claim is a normal outcome and not
	// an error.
	Applied bool   `json:"applied,omitempty"`
	Stale   bool   `json:"stale,omitempty"`
	Note    string `json:"note,omitempty"`
}

// SyncListen is what a session is parked on, for its roster row. It is what makes
// "listening: tests:green" a fact the daemon observed rather than a claim, and it
// is deliberately never warned about: listening is the intended steady state.
type SyncListen struct {
	Topics  []string `json:"topics"`
	SinceMS int64    `json:"since_ms"`
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
	// FR94. Paused says the human has latched the desktop back to himself: the
	// run is alive and its place is kept, but nothing may drive until he resumes.
	// It is a property of the desktop rather than of the run, so it can be true
	// with no run live at all - a pre-emptive "not now" to every agent.
	Paused  bool `json:"paused,omitempty"`
	PausedS int  `json:"paused_s,omitempty"` // how long it has been latched
	// FR95. Quiet says the sign is demoted to the marker for a recording. Like
	// Paused it belongs to the desktop, so it can be armed before any run exists,
	// and QuietLeftS is what the fuse has left before it goes loud on its own.
	Quiet      bool `json:"quiet,omitempty"`
	QuietLeftS int  `json:"quiet_left_s,omitempty"`
	// QuietHeld is how many cards the demoted sign is holding back, so the state
	// line says what going loud is about to put on screen. Only meaningful while
	// Quiet: cards queue during a recording and drain when it ends.
	QuietHeld int `json:"quiet_held,omitempty"`
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

	// Sync is the discovery rider (FR83): one line about company that arrives on
	// the way back from an unrelated call, when the caller's area gained or lost
	// an agent since the last time it called anything. It rides the envelope
	// rather than the result so that every method carries it without every result
	// type knowing about it - and so an agent that is mid-task hears about a peer
	// at the moment it is about to act, instead of whenever it next thinks to ask.
	Sync string `json:"sync,omitempty"`
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
	connErr error // a refusal the peer sent with no id, preferred over a bare EOF
	reading bool
	rider   func(method string, params json.RawMessage) string
}

func NewConn(rwc io.ReadWriteCloser) *Conn {
	return &Conn{
		rwc:     rwc,
		enc:     json.NewEncoder(rwc),
		pending: make(map[int64]chan response),
	}
}

func (c *Conn) Close() error { return c.rwc.Close() }

// SetRider installs the function that decides what rides back on each envelope.
// Called after the handler, with the method and params of the request just
// served, so a peer that arrived DURING the call is still reported by it. An
// empty answer adds nothing. Set it before Serve.
func (c *Conn) SetRider(f func(method string, params json.RawMessage) string) {
	c.mu.Lock()
	c.rider = f
	c.mu.Unlock()
}

func (c *Conn) riderFn() func(string, json.RawMessage) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rider
}

func (c *Conn) send(v any) error {
	c.encMu.Lock()
	defer c.encMu.Unlock()
	return c.enc.Encode(v)
}

// Call sends a request and blocks until the response arrives, the context
// is done, or the connection fails.
func (c *Conn) Call(ctx context.Context, method string, params, result any) error {
	_, err := c.CallRidden(ctx, method, params, result)
	return err
}

// CallRidden is Call, and also hands back whatever the daemon rode along on the
// envelope (FR83's discovery rider). Every existing caller stays on Call: only
// the mcp child has anywhere to put the line, and only it needs to know.
func (c *Conn) CallRidden(ctx context.Context, method string, params, result any) (string, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("marshal params: %w", err)
	}
	id := c.nextID.Add(1)
	ch := make(chan response, 1)

	c.mu.Lock()
	if c.readErr != nil {
		err := c.readErr
		c.mu.Unlock()
		return "", err
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
		return "", fmt.Errorf("send %s: %w", method, err)
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case resp, ok := <-ch:
		if !ok {
			c.mu.Lock()
			err := c.readErr
			c.mu.Unlock()
			return "", fmt.Errorf("connection closed during %s: %w", method, err)
		}
		if resp.Error != nil {
			// The rider is dropped with a failed call on purpose: an agent being
			// told why its call failed does not also need news about company.
			return "", resp.Error
		}
		if result != nil {
			if err := json.Unmarshal(resp.Result, result); err != nil {
				return "", fmt.Errorf("unmarshal %s result: %w", method, err)
			}
		}
		return resp.Sync, nil
	}
}

// MaxLineBytes is the wire cap on one JSON-RPC line, in both directions. It was
// the ONLY bound on an item's size until R-04 put caps in Item.Validate, and it
// is deliberately the outer one: Validate refuses with a readable message naming
// the field, and this is what catches anything that never went through it.
//
// Raising it is the wrong fix for a payload that does not fit. That moves the
// cliff and leaves the same silence at the new edge.
const MaxLineBytes = 4 * 1024 * 1024

func (c *Conn) readLoop() {
	sc := bufio.NewScanner(c.rwc)
	sc.Buffer(make([]byte, 0, 64*1024), MaxLineBytes)
	for sc.Scan() {
		var resp response
		if err := json.Unmarshal(sc.Bytes(), &resp); err != nil {
			continue
		}
		if resp.ID == nil {
			// A connection-level refusal (R-04). The peer could not read the
			// request at all, so it has no id to answer on - but it said WHY, and
			// that sentence is worth more than the EOF that follows it. Every
			// pending call on this connection is about to fail; this is what they
			// report instead of "connection closed: EOF".
			if resp.Error != nil {
				c.mu.Lock()
				c.connErr = errors.New(resp.Error.Message)
				c.mu.Unlock()
			}
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
	if c.connErr != nil {
		err = c.connErr
	}
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
	sc.Buffer(make([]byte, 0, 64*1024), MaxLineBytes)
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
					// Only a successful call carries news (FR83). Computed here,
					// after the handler, so a peer who arrived during the call is
					// still in it.
					if rider := c.riderFn(); rider != nil {
						resp.Sync = rider(req.Method, req.Params)
					}
				}
			}
			_ = c.send(resp)
		}(req)
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			// R-04. The line was never parsed, so there is no id to answer on, and
			// the connection is about to go. Saying so first is the whole
			// difference between a refusal and a silence: what the agent used to
			// get was "connection closed during agentbox.v1.ask: EOF", which named
			// neither the size nor the field, and nothing was stored, so there was
			// not even a history row to find afterwards.
			_ = c.send(response{JSONRPC: "2.0", Error: &RPCError{
				Code: CodeInvalidRequest,
				Message: fmt.Sprintf("that request was over the %d-byte wire limit, so it was never read and nothing was stored; "+
					"put the large part in a file and show_document it", MaxLineBytes),
			}})
		}
		if !errors.Is(err, io.ErrClosedPipe) {
			return err
		}
	}
	return nil
}
