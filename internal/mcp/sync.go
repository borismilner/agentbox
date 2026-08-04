package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borismilner/agentbox/internal/client"
	"github.com/borismilner/agentbox/internal/proto"
)

// The child's half of the roster (FR83, slice 1).
//
// One session is one mcp child, which is why the key is minted here and why the
// attach lives here: this process starts when the session starts and dies when
// it dies, so its connection is the truest statement of "this agent exists"
// available anywhere in the system.
//
// Two rules that are decisions rather than plumbing, both from the design:
//
//   - The attach never spawns the daemon. Dial with spawning disabled, or every
//     live session's redial loop resurrects the daemon that `make stop` just
//     stopped, and the working-copy workflow in CLAUDE.md loses the flock race
//     to a background reconnect.
//   - The attach is lazy. It connects on the first tool call, not at process
//     start, so opening a terminal does not raise a daemon, a tray and a
//     webview. The vision's rule stands: the first CALL auto-spawns, not the
//     first process.

// attachBackoff is how long the child waits between redials. A daemon restart
// should heal the roster without the agent's model ever knowing, and a second is
// far below human notice while costing nothing when the daemon is simply down.
const attachBackoff = time.Second

// errNoSpawn refuses the auto-spawn that Dial would otherwise do.
var errNoSpawn = errors.New("the attach never spawns the daemon")

type attachState struct {
	mu    sync.Mutex
	once  sync.Once
	live  bool
	last  proto.SyncAnnounceParams // replayed after a daemon restart
	haveL bool
	// activity is the latest line this session reported, which is NOT the one the
	// announce carried (FR87). Replaying the announce alone brought every row back
	// saying something that was true an hour ago with a fresh timestamp, and a
	// confident statement about the past is worse than an empty one.
	activity string
}

// rememberActivity keeps the newest line for the replay after a daemon restart.
func (s *server) rememberActivity(line string) {
	s.attach.mu.Lock()
	s.attach.activity = line
	s.attach.mu.Unlock()
}

// ensureAttached starts the presence connection on the first tool call. Safe to
// call from every handler; only the first one does anything.
func (s *server) ensureAttached() {
	if s.base == nil {
		return
	}
	s.attach.once.Do(func() { go s.attachLoop(s.base) })
}

// attachLoop holds one call open for the session's whole life, redialing when it
// drops. The call never returns anything useful: its being open is the point.
func (s *server) attachLoop(ctx context.Context) {
	cwd, _ := os.Getwd()
	p := proto.SyncAttachParams{
		Identity: s.id,
		Cwd:      cwd,
		// The AGENT's pid, not this child's. The child is an implementation
		// detail; what the human needs from a roster row is the process they
		// could go and look at, and what a later slice needs for orphan
		// liveness is the process that might still be doing the work.
		PID: os.Getppid(),
	}

	for ctx.Err() == nil {
		conn, err := client.Dial(ctx, s.runtimeDir, func() error { return errNoSpawn })
		if err != nil {
			// No daemon: wait and try again. This is the normal state of a
			// session whose human has not started AgentBox yet.
			if !sleepCtx(ctx, attachBackoff) {
				return
			}
			continue
		}

		s.attach.mu.Lock()
		s.attach.live = true
		replay, have := s.attach.last, s.attach.haveL
		// The announce is replayed with the LATEST activity, not the one it was
		// first called with: an hour-old line stamped as fresh is a lie the human
		// cannot see through, where a stale purpose is still true (FR87).
		if s.attach.activity != "" {
			replay.Activity = s.attach.activity
		}
		s.attach.mu.Unlock()

		// Replay the purpose the model already stated, so a daemon restart heals
		// the roster instead of demoting a working agent back to "no purpose
		// given" until it happens to speak again.
		if have {
			go func() {
				var res proto.SyncResult
				_ = conn.Call(ctx, proto.MethodSyncAnnounce, &replay, &res)
			}()
		}

		var res proto.SyncResult
		// Blocks until the daemon goes away or the session ends. Either way the
		// row is gone the moment this returns, which is the contract.
		_ = conn.Call(ctx, proto.MethodSyncAttach, &p, &res)
		conn.Close()

		s.attach.mu.Lock()
		s.attach.live = false
		s.attach.mu.Unlock()

		if !sleepCtx(ctx, attachBackoff) {
			return
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// syncCall makes one short sync call on its own connection, the way every other
// non-blocking tool does.
func (s *server) syncCall(ctx context.Context, method string, req, res any) error {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, err := client.Dial(dialCtx, s.runtimeDir, nil)
	cancel()
	if err != nil {
		return fmt.Errorf("cannot reach agentbox daemon: %w", err)
	}
	defer conn.Close()
	rider, err := conn.CallRidden(ctx, method, req, res)
	noteRider(ctx, rider)
	return err
}

type announceIn struct {
	Purpose  string   `json:"purpose" jsonschema:"one line saying what this session is FOR, in the human's terms - the headline of your row on their Agents board, e.g. 'porting the settings surface to the new theme'"`
	Activity string   `json:"activity,omitempty" jsonschema:"optional: what you are doing right now. set_activity carries this from then on"`
	Area     string   `json:"area,omitempty" jsonschema:"optional kind:scope tag refining where you work, e.g. subsystem:webui. The repo you are in is detected already; this only narrows searches, never what you are told about"`
	Tags     []string `json:"tags,omitempty" jsonschema:"optional extra kind:scope tags"`
}

type peerOut struct {
	Key      string `json:"key"`
	Agent    string `json:"agent"`
	Project  string `json:"project,omitempty"`
	Purpose  string `json:"purpose,omitempty"`
	Activity string `json:"activity,omitempty"`
	State    string `json:"state"`
	Cwd      string `json:"cwd,omitempty"`
}

type announceOut struct {
	OK    bool      `json:"ok"`
	Peers []peerOut `json:"peers"`
	// Alone is the answer to the question an agent actually has, and it is only
	// ever true when it is certainly true.
	Alone   bool   `json:"alone"`
	Partial bool   `json:"partial,omitempty"`
	Note    string `json:"note,omitempty"`
}

type listIn struct {
	Area    string `json:"area,omitempty" jsonschema:"optional: only agents in this area"`
	Project string `json:"project,omitempty" jsonschema:"optional: only agents on this project"`
}

type listOut struct {
	OK      bool      `json:"ok"`
	Agents  []peerOut `json:"agents"`
	Partial bool      `json:"partial,omitempty"`
	Note    string    `json:"note,omitempty"`
}

func peers(in []proto.SyncAgent) []peerOut {
	out := make([]peerOut, 0, len(in))
	for _, a := range in {
		out = append(out, peerOut{
			Key: a.Key, Agent: a.Agent, Project: a.Project,
			Purpose: a.Purpose, Activity: a.Activity, State: a.State, Cwd: a.Cwd,
		})
	}
	return out
}

// partialNote is the sentence that stops an agent concluding it is alone from a
// list that cannot see everybody.
const partialNote = "This roster is not everybody: at least one session predates sync and has no row. Do not conclude you are working alone."

func (s *server) announce(ctx context.Context, _ *sdk.CallToolRequest, in announceIn) (*sdk.CallToolResult, announceOut, error) {
	if strings.TrimSpace(in.Purpose) == "" {
		return errResult[announceOut](fmt.Errorf("announce needs a purpose: one line saying what this session is for, in the human's terms"))
	}
	s.ensureAttached()

	cwd, _ := os.Getwd()
	req := proto.SyncAnnounceParams{
		Identity: s.id, Purpose: in.Purpose, Activity: in.Activity,
		// The cwd is what the daemon derives the area from when the model names
		// none, and the announce may well arrive before this session's attach does
		// (FR90).
		Cwd: cwd, Area: in.Area, Tags: in.Tags,
	}
	// Remembered for replay: a daemon restart must not cost the human the
	// purpose the model already stated.
	s.attach.mu.Lock()
	s.attach.last, s.attach.haveL = req, true
	s.attach.mu.Unlock()

	var res proto.SyncResult
	if err := s.syncCall(ctx, proto.MethodSyncAnnounce, &req, &res); err != nil {
		return errResult[announceOut](err)
	}
	out := announceOut{OK: true, Peers: peers(res.Agents), Partial: res.Partial}
	out.Alone = len(res.Agents) == 0 && !res.Partial
	switch {
	case res.Partial:
		out.Note = partialNote
	case len(res.Agents) > 0:
		out.Note = fmt.Sprintf("%d other agent(s) share your area. You are not working alone: coordinate before editing a shared tree, and take a lock before touching a shared resource.", len(res.Agents))
	}
	return &sdk.CallToolResult{}, out, nil
}

func (s *server) listAgents(ctx context.Context, _ *sdk.CallToolRequest, in listIn) (*sdk.CallToolResult, listOut, error) {
	s.ensureAttached()
	req := proto.SyncListParams{Identity: s.id, Area: in.Area, Project: in.Project}
	var res proto.SyncResult
	if err := s.syncCall(ctx, proto.MethodSyncList, &req, &res); err != nil {
		return errResult[listOut](err)
	}
	out := listOut{OK: true, Agents: peers(res.Agents), Partial: res.Partial}
	if res.Partial {
		out.Note = partialNote
	}
	return &sdk.CallToolResult{}, out, nil
}

// The lock half (FR83 slice 2). Three tools, and the split is the house rule
// rather than taste: a blocking verb and a non-blocking verb never share one
// tool, because an agent reading a schema must be able to tell whether calling
// it will park its turn.
type lockIn struct {
	Name     string `json:"name" jsonschema:"the lock's name, in the kind:scope idiom the rest of agentbox uses - deploy:agentbox, repo:agentbox, vm:boris-vm. A name is a convention, not a registry: pick the one another agent reaching for the same resource would pick"`
	TimeoutS int    `json:"timeout_s,omitempty" jsonschema:"how long to wait for it, in seconds. 0 waits as long as the daemon allows a parked call. A timeout is a result, not an error: it comes back with the holder named, and re-arming is one call"`
	Note     string `json:"note,omitempty" jsonschema:"optional: what you are about to do with the resource, shown to the human and to whoever waits behind you"`
	// ReleaseOnDetach is offered to the model because only the model knows which
	// case it is in: a critical section that IS this session, or work it starts
	// that outlives it.
	ReleaseOnDetach bool `json:"release_on_detach,omitempty" jsonschema:"true if the hold should end the moment this session does. Leave false when you start work that outlives your session (a deploy, a long build): the hold then goes orphaned instead, and nobody else is given the resource until that work's process is gone"`
}

type unlockIn struct {
	Name string `json:"name" jsonschema:"the lock to release. Only the holder can release it"`
}

type lockOut struct {
	OK       bool   `json:"ok"`
	Name     string `json:"name"`
	Granted  bool   `json:"granted"`
	TimedOut bool   `json:"timed_out,omitempty"`
	Refused  bool   `json:"refused,omitempty"`
	Released bool   `json:"released,omitempty"`

	// The picture, so a refusal never needs a follow-up question.
	Holder     string `json:"holder,omitempty"`
	HolderKey  string `json:"holder_key,omitempty"`
	Purpose    string `json:"holder_purpose,omitempty"`
	Activity   string `json:"holder_activity,omitempty"`
	HolderNote string `json:"holder_note,omitempty"`
	HeldS      int    `json:"held_s,omitempty"`
	Queue      int    `json:"queue,omitempty"`
	Orphaned   bool   `json:"orphaned,omitempty"`
	WaitedS    int    `json:"waited_s,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Deadlock   string `json:"deadlock,omitempty"`
	Note       string `json:"note,omitempty"`
}

func lockResult(res proto.SyncLockResult) lockOut {
	out := lockOut{
		OK: res.OK, Name: res.Name, Granted: res.Granted, TimedOut: res.TimedOut,
		Refused: res.Refused, Released: res.Released, HolderNote: res.HolderNote,
		HeldS: int(res.HeldMS / 1000), Queue: res.Queue, Orphaned: res.Orphaned,
		WaitedS: int(res.WaitedMS / 1000), Reason: res.Reason,
		Deadlock: res.Deadlock, Note: res.Note,
	}
	if h := res.Holder; h != nil {
		out.Holder, out.HolderKey = h.Agent, h.Key
		out.Purpose, out.Activity = h.Purpose, h.Activity
	}
	return out
}

func (s *server) acquireLock(ctx context.Context, _ *sdk.CallToolRequest, in lockIn) (*sdk.CallToolResult, lockOut, error) {
	s.ensureAttached()
	req := proto.SyncLockParams{
		Identity: s.id, Name: in.Name, TimeoutS: in.TimeoutS, Note: in.Note,
		ReleaseOnDetach: in.ReleaseOnDetach,
		// The AGENT's pid, matching the attach: if this session dies holding the
		// lock, what decides whether the resource is really free is whether the
		// process that was using it is still there.
		PID: os.Getppid(),
	}
	var res proto.SyncLockResult
	// No dial deadline of its own: this call parks for as long as the caller is
	// patient, and the middleware keeps the client from giving up on it.
	if err := s.syncCall(ctx, proto.MethodSyncLock, &req, &res); err != nil {
		return errResult[lockOut](err)
	}
	return &sdk.CallToolResult{}, lockResult(res), nil
}

func (s *server) tryLock(ctx context.Context, _ *sdk.CallToolRequest, in lockIn) (*sdk.CallToolResult, lockOut, error) {
	s.ensureAttached()
	req := proto.SyncLockParams{
		Identity: s.id, Name: in.Name, Note: in.Note,
		ReleaseOnDetach: in.ReleaseOnDetach, PID: os.Getppid(),
	}
	var res proto.SyncLockResult
	if err := s.syncCall(ctx, proto.MethodSyncTryLock, &req, &res); err != nil {
		return errResult[lockOut](err)
	}
	return &sdk.CallToolResult{}, lockResult(res), nil
}

func (s *server) releaseLock(ctx context.Context, _ *sdk.CallToolRequest, in unlockIn) (*sdk.CallToolResult, lockOut, error) {
	s.ensureAttached()
	req := proto.SyncLockParams{Identity: s.id, Name: in.Name}
	var res proto.SyncLockResult
	if err := s.syncCall(ctx, proto.MethodSyncUnlock, &req, &res); err != nil {
		return errResult[lockOut](err)
	}
	return &sdk.CallToolResult{}, lockResult(res), nil
}

// The signal half (FR83 slice 3). Two tools for the same reason acquire and try
// are two: post never blocks and await always may, and an agent reading a schema
// has to be able to tell which one will park its turn.
type postIn struct {
	Topic string `json:"topic" jsonschema:"the topic this event is about, in the kind:scope idiom - tests:green, done:migration-3, build:failed. To message ONE agent, post to to:<its session key> (the key is on its roster row from announce or list_agents). A topic is a convention, not a registry: pick the name the agent waiting for this event would wait on"`
	Data  any    `json:"data,omitempty" jsonschema:"optional small JSON payload: the fact, the request or the hand-off. Keep it to what the receiver needs to act - a file path rather than a file"`
}

type postOut struct {
	OK    bool   `json:"ok"`
	Topic string `json:"topic"`
	// Seq is the signal's place in the one global sequence. It is also the cursor
	// to hand a peer: after_seq one below this delivers exactly this signal.
	Seq       int64  `json:"seq"`
	Delivered int    `json:"delivered"`
	Note      string `json:"note,omitempty"`
}

type awaitSignalIn struct {
	Topics []string `json:"topics" jsonschema:"the topics to wait on, any of which wakes you. An exact name waits for one event (tests:green); a name ending in * waits for a family (done:*). Use to:@me for signals addressed to this session - @me is your own session key"`
	// AfterSeq is offered to the model because resuming is the model's decision: it
	// is the difference between "tell me what happens next" and "tell me everything
	// I have not seen", and only the caller knows which it means.
	AfterSeq int64 `json:"after_seq,omitempty" jsonschema:"the cursor from your last await_signal or post_signal. Omit it to wait for what happens from now on; pass it to catch up on everything you have not seen, which may come back at once without waiting"`
	TimeoutS int   `json:"timeout_s,omitempty" jsonschema:"how long to wait, in seconds. 0 waits as long as the daemon allows a parked call. A timeout is a result, not an error: it comes back with your cursor unchanged, so waiting again misses nothing"`
}

type signalOut struct {
	Seq     int64  `json:"seq"`
	Topic   string `json:"topic"`
	Agent   string `json:"agent,omitempty"`
	Project string `json:"project,omitempty"`
	// Key is the poster's session key, which is what makes a reply possible:
	// post_signal to "to:<key>".
	Key  string `json:"key,omitempty"`
	Data any    `json:"data,omitempty"`
	AtMS int64  `json:"at_ms,omitempty"`
}

type awaitSignalOut struct {
	OK      bool        `json:"ok"`
	Signals []signalOut `json:"signals,omitempty"`
	Cursor  int64       `json:"cursor"`
	// Received says it plainly, so a model does not have to infer the difference
	// between an empty batch and a timeout from two other fields.
	Received  bool   `json:"received"`
	TimedOut  bool   `json:"timed_out,omitempty"`
	Gap       bool   `json:"gap,omitempty"`
	OldestSeq int64  `json:"oldest_seq,omitempty"`
	More      bool   `json:"more,omitempty"`
	Note      string `json:"note,omitempty"`
}

// expandTopics resolves @me to this session's own key, so `to:@me` becomes the
// session's private topic.
//
// The child does it rather than the daemon on purpose: a magic token the daemon
// understood would mean every stored topic and every pattern had to be read
// through it forever, and the whole point of the private topic is that it is an
// ordinary name. This way the daemon has one rule for topics and the convenience
// costs one string replacement.
func (s *server) expandTopics(in []string) []string {
	out := make([]string, 0, len(in))
	for _, t := range in {
		out = append(out, strings.ReplaceAll(t, "@me", s.id.Key))
	}
	return out
}

func (s *server) postSignal(ctx context.Context, _ *sdk.CallToolRequest, in postIn) (*sdk.CallToolResult, postOut, error) {
	s.ensureAttached()
	req := proto.SyncPostParams{Identity: s.id, Topic: strings.ReplaceAll(in.Topic, "@me", s.id.Key)}
	if in.Data != nil {
		b, err := json.Marshal(in.Data)
		if err != nil {
			return errResult[postOut](fmt.Errorf("data is not encodable as JSON: %w", err))
		}
		req.Data = b
	}
	var res proto.SyncPostResult
	if err := s.syncCall(ctx, proto.MethodSyncPost, &req, &res); err != nil {
		return errResult[postOut](err)
	}
	return &sdk.CallToolResult{}, postOut{OK: res.OK, Topic: res.Topic,
		Seq: res.Seq, Delivered: res.Delivered, Note: res.Note}, nil
}

func (s *server) awaitSignal(ctx context.Context, _ *sdk.CallToolRequest, in awaitSignalIn) (*sdk.CallToolResult, awaitSignalOut, error) {
	s.ensureAttached()
	req := proto.SyncAwaitParams{Identity: s.id, Topics: s.expandTopics(in.Topics),
		AfterSeq: in.AfterSeq, TimeoutS: in.TimeoutS}
	var res proto.SyncAwaitResult
	// No dial deadline of its own: this parks for as long as the caller is patient,
	// and the keep-alive middleware is what stops the client giving up on it.
	if err := s.syncCall(ctx, proto.MethodSyncAwait, &req, &res); err != nil {
		return errResult[awaitSignalOut](err)
	}
	out := awaitSignalOut{OK: res.OK, Cursor: res.Cursor, Received: len(res.Signals) > 0,
		TimedOut: res.TimedOut, Gap: res.Gap, OldestSeq: res.OldestSeq,
		More: res.More, Note: res.Note}
	for _, sig := range res.Signals {
		one := signalOut{Seq: sig.Seq, Topic: sig.Topic, Agent: sig.Agent,
			Project: sig.Project, Key: sig.Key, AtMS: sig.AtMS}
		if len(sig.Data) > 0 {
			// Handed back as the structure the poster sent rather than as a string of
			// JSON, so the receiving model reads a payload instead of parsing one. A
			// payload that will not decode travels as the raw text it was: the signal
			// arriving matters more than agentbox approving of its shape.
			var v any
			if err := json.Unmarshal(sig.Data, &v); err == nil {
				one.Data = v
			} else {
				one.Data = string(sig.Data)
			}
		}
		out.Signals = append(out.Signals, one)
	}
	return &sdk.CallToolResult{}, out, nil
}

// addSyncTools registers the roster family. Kept in one function beside the
// others so the tool list has one obvious place per feature.
func addSyncTools(srv *sdk.Server, s *server) {
	sdk.AddTool(srv, &sdk.Tool{
		Name: "announce",
		Description: "Say what this session is FOR, and find out who else is already working where you are. Non-blocking, and it should be your FIRST AgentBox call. " +
			"The human watches an Agents board of every live session: your purpose is the headline of your row, so write it as one line they would recognise, not as a restatement of your tools. " +
			"Returns the agents that share your area, each with its purpose and what it is doing right now - so you learn you are not alone before you touch anything. " +
			"If peers come back, coordinate: partition the work or wait, rather than editing the same tree at the same time. " +
			"Call it again if the mission changes. alone=true is only ever returned when it is certainly true; partial=true means the roster cannot see everybody, so never read it as being alone.",
	}, s.announce)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "list_agents",
		Description: "List the live agent sessions the daemon can see: identity, purpose, current activity and state. Non-blocking. " +
			"Use it to check for company before editing a shared tree, or to find the peer you want to coordinate with. " +
			"This is the same roster the human's Agents surface renders, so you and they never see different answers.",
	}, s.listAgents)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "acquire_lock",
		Description: "Take a named lock and BLOCK until it is yours, so two agents on this machine take turns over one resource instead of colliding. " +
			"Take one before anything shared: the deploy, a repo other sessions edit, the VM, an external service. Names are a convention in the kind:scope idiom - deploy:agentbox, repo:agentbox, vm:boris-vm - so pick the name another agent would pick. " +
			"Waiters queue in order and a grant says why it happened. A timeout is a RESULT, not an error: it comes back with the holder's purpose, what they are doing and how long they have held it, so you can decide whether to wait again, do something else, or go and coordinate with them. " +
			"Release it as soon as the work is done. Never hold a lock across a question to the human - that stalls every agent behind you on something only he can end. " +
			"You must announce before taking a lock: a lock the human cannot attribute to a purpose is one he cannot judge when it goes wrong.",
	}, s.acquireLock)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "try_lock",
		Description: "Take a named lock if it is free RIGHT NOW, and never wait. Returns granted, or refused with the full picture of who holds it and why. " +
			"Use it when you have something else useful to do; use acquire_lock when you cannot proceed without the resource.",
	}, s.tryLock)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "release_lock",
		Description: "Release a lock you hold, handing it to whoever is queued behind you. Non-blocking. " +
			"Release as soon as the protected work is finished rather than at the end of your session: everything behind you is stopped until you do.",
	}, s.releaseLock)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "post_signal",
		Description: "Tell the other agents on this machine that something happened - tests went green, a chunk is done, a build failed. Non-blocking. " +
			"Topics are names in the kind:scope idiom (tests:green, done:migration-3); post to to:<session key> to message ONE agent, whose key is on its roster row. " +
			"A signal is stored, so it is delivered whether or not anybody was waiting: a peer that was busy picks it up later by cursor, and a daemon restart does not lose it. " +
			"delivered=0 means nobody was parked on it, which is a fact rather than a failure. " +
			"Use this instead of leaving a peer to poll for a status you already know.",
	}, s.postSignal)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "await_signal",
		Description: "BLOCK until another agent (or agentbox) posts a matching signal, so waiting for a peer costs one parked call instead of a sleep-and-check loop that spends a turn per poll. " +
			"Topics: an exact name waits for one event, a name ending in * waits for a family (done:*), and to:@me waits for signals addressed to this session. " +
			"Everything matching since your cursor comes back in ONE batch, so a signal that fired while you were busy is not missed - pass the cursor you were last given as after_seq. " +
			"A timeout is a RESULT, not an error: the cursor comes back unchanged and waiting again misses nothing. gap=true means your cursor is older than what retention still holds, so the batch cannot be complete - treat what you were tracking as unknown rather than as not having happened. " +
			"Park here only for something you are genuinely waiting on. The composition worth knowing: await_signal on tests:green, then acquire_lock on the deploy.",
	}, s.awaitSignal)
}
