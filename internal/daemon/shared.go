package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/borismilner/agentbox/internal/logging"
	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
)

// Shared values (FR83, slice 4): the blackboard, and the last of the four
// primitives. A lock says whose turn it is, a signal says something happened, and
// neither can say "chunk 7 is mine" - which is the fact a fanned-out job needs
// before it starts work a peer already started.
//
// What this file owes the design is four behaviours, each of which the earlier
// slices paid to learn:
//
//   - **The atomicity is in the SQL, not in this mutex.** Every CAS is one
//     statement (store/shared.go), so a write is safe against a concurrent writer
//     for a reason that survives a second daemon, a dev instance and a future
//     migration tool. The mutex here guards the observer fields and nothing else.
//   - **Ownership is RECORDED, then checked in two steps.** The trim/gap lesson,
//     applied to claims: a value cannot say whether the session that wrote it still
//     exists, and once that session's row is gone there is nothing left to ask. So the
//     owner goes into the row at write time - session key, agent name and pid - and a
//     read asks the roster first, then the process. The roster alone was wrong for
//     exactly one second per daemon restart, when it is empty and every live claim
//     read as abandoned; the pid is the only fact about an owner that outlives the
//     daemon, which is why the orphaned lock is pid-checked too.
//   - **A write posts through the signal hub and nowhere else.** "Waiting on a
//     value" is await_signal(["shared:claims/*"]). There is exactly one wake
//     mechanism in this design, and a second one here would be the drift the whole
//     feature exists to remove.
//   - **The signal is a doorbell, not a delivery.** shared:KEY carries the key, the
//     new version and the owner - never the value. Partly for the discipline the
//     signal hub already keeps, and partly because it could not be honest otherwise:
//     the value cap and the signal cap are the same number, so a maximum-size value
//     plus an envelope would be over the signal's limit and the wake would be dropped
//     rather than delivered small.
//
// Reads are ungated on purpose while writes are not. Visibility must not depend on
// good manners (the design's rule for the monitoring reads), but a claim the human
// cannot attribute to a purpose is a coordination he cannot follow when it goes
// wrong - the same line the locks and signals draw.

// sharedValueMax is the default cap on one value, matching signalDataMax so the two
// halves of the blackboard cannot disagree about what "small" means. Overridable
// from config, unlike the signal cap, because a value is state a workflow shapes
// while a signal is an event agentbox shapes.
const sharedValueMax = 16 << 10

// sharedStore is the persistence shared values need, narrowed to what this file
// calls. A daemon with no database refuses rather than keeping a map: durability is
// the feature here even more than it is for signals, since the acceptance case is
// literally "restart the daemon mid-drain and the claims survive".
type sharedStore interface {
	SharedGet(key string) (proto.SharedValue, bool, error)
	SharedList(prefix string, limit int) ([]proto.SharedValue, bool, error)
	SharedSet(key, value string, ifVersion *int64, owner, ownerAgent string, ownerPID int) (proto.SharedValue, bool, error)
	SharedDelete(key string, ifVersion *int64) (proto.SharedValue, bool, error)
	SharedCount() (int, error)
}

type shared struct {
	log *slog.Logger

	mu sync.Mutex
	st sharedStore
	// The observers, all three read outside this mutex when they are called: the
	// announce gate, whether a session is still on the roster, and the door onto the
	// signal hub. Reading another subsystem's state while holding your own lock is
	// what deadlocks this daemon on the first board repaint (see roster.snapshot).
	announcedFn func(key string) bool
	presentFn   func(key string) bool
	post        func(topic string, id proto.Identity, data any)
	// alive is the pid probe, swappable so a test can decide a process is gone
	// without killing anything. Same door the lock table's orphan check uses.
	alive func(pid int) bool
	// changed repaints the board. A write is the only thing that changes the
	// blackboard, so it is the only thing that has to trigger a repaint - the roster
	// tick would get there eventually, but "eventually" on a claim table is how a
	// human watches a stale board.
	changed func()

	maxBytes int
}

func newShared(log *slog.Logger) *shared {
	return &shared{log: log, maxBytes: sharedValueMax, alive: pidAlive}
}

// SetStore wires persistence, separately from the constructor for the reason every
// other subsystem does: a daemon assembles its store and its subsystems in that
// order.
func (sh *shared) SetStore(st sharedStore) {
	sh.mu.Lock()
	sh.st = st
	sh.mu.Unlock()
}

// SetObservers wires the announce gate, the liveness question and the signal door.
// All three are legitimately nil in a test.
func (sh *shared) SetObservers(announced, present func(key string) bool,
	post func(topic string, id proto.Identity, data any), changed func()) {
	sh.mu.Lock()
	sh.announcedFn, sh.presentFn, sh.post, sh.changed = announced, present, post, changed
	sh.mu.Unlock()
}

// SetMaxBytes applies the config cap. Zero leaves the default: a blackboard with no
// size limit is a memory leak with a JSON schema.
func (sh *shared) SetMaxBytes(n int) {
	sh.mu.Lock()
	if n > 0 {
		sh.maxBytes = n
	}
	sh.mu.Unlock()
}

// Handle is the one door for all three operations, which is what the design bought
// by folding them: none of them blocks, so a caller reading the schema still knows
// whether calling it parks its turn.
func (sh *shared) Handle(p proto.SyncSharedParams) (proto.SyncSharedResult, *proto.RPCError) {
	op := strings.TrimSpace(strings.ToLower(p.Op))
	key := strings.TrimSpace(p.Key)
	if key == "" {
		return proto.SyncSharedResult{}, &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: `shared needs a key. One key per item is the idiom - "claims/chunk-3", not one table under one hot key - because per-item first-writer-wins costs no retries where a global version makes every loser retry.`}
	}
	st := sh.store()
	if st == nil {
		return proto.SyncSharedResult{}, &proto.RPCError{Code: proto.CodeInternal,
			Message: "shared: this daemon has no database, so shared values cannot be stored. Coordinate through a lock or a signal instead."}
	}

	switch op {
	case proto.SharedOpGet:
		return sh.get(st, key)
	case proto.SharedOpSet:
		return sh.set(st, p, key)
	case proto.SharedOpDelete:
		return sh.del(st, p, key)
	default:
		return proto.SyncSharedResult{}, &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: fmt.Sprintf("shared: op %q is not one of get, set, delete.", p.Op)}
	}
}

// snapshot is the whole blackboard for the Agents surface, ownership marked, in key
// order. Bounded by the store's own list cap, so a runaway table cannot turn one
// repaint into a thousand rows on screen.
//
// One indexed read per throttled push, which is a different cost from the one slice
// 3 declined for recent signals: that would have been a read PER ROW on every
// snapshot, and this is one read for the whole payload. The repaint is throttled to
// four a second and already walks every roster row, so this is not the expensive
// part of it.
func (sh *shared) snapshot() []proto.SharedValue {
	st := sh.store()
	if st == nil {
		return nil
	}
	values, _, err := st.SharedList("", 0)
	if err != nil {
		sh.log.Warn(logging.EvSync, "component", "daemon", "sync", "shared_read_failed",
			"error", err.Error())
		return nil
	}
	return sh.markOwners(values)
}

// get reads one key, or a family when the key ends in the prefix wildcard. Ungated:
// looking is not coordinating.
func (sh *shared) get(st sharedStore, key string) (proto.SyncSharedResult, *proto.RPCError) {
	out := proto.SyncSharedResult{OK: true, Op: proto.SharedOpGet}
	if prefix, isPrefix := proto.ParseTopic(key); isPrefix {
		values, more, err := st.SharedList(prefix, 0)
		if err != nil {
			return proto.SyncSharedResult{}, &proto.RPCError{Code: proto.CodeInternal,
				Message: "could not read shared values: " + err.Error()}
		}
		out.Values = sh.markOwners(values)
		out.More = more
		out.Found = len(values) > 0
		switch {
		case more:
			out.Note = fmt.Sprintf("capped at %d keys under %s; there are more. Read a narrower prefix, or work through these first.", len(out.Values), prefix)
		case len(values) == 0:
			out.Note = fmt.Sprintf("no keys under %s. Nothing has been written there yet, which for a claim table means every item is still free.", prefix)
		default:
			if n := countGone(out.Values); n > 0 {
				out.Note = fmt.Sprintf("%d of these %d values is owned by a session that is no longer running (owner_gone). Its work was not finished by anybody: take it over by writing the key again with its current version as if_version.", n, len(out.Values))
			}
		}
		return out, nil
	}

	v, found, err := st.SharedGet(key)
	if err != nil {
		return proto.SyncSharedResult{}, &proto.RPCError{Code: proto.CodeInternal,
			Message: "could not read shared value: " + err.Error()}
	}
	out.Found = found
	if !found {
		// Version 0 is the answer, not an absence: it is the if_version a caller
		// passes to claim this key, so saying it here saves the round trip.
		out.Value = &proto.SharedValue{Key: key}
		out.Note = fmt.Sprintf("%s does not exist. Its version is 0, which is also the if_version that claims it: a set with if_version 0 succeeds only if nobody else got there first.", key)
		return out, nil
	}
	marked := sh.markOwners([]proto.SharedValue{v})
	out.Value = &marked[0]
	if out.Value.OwnerGone {
		out.Note = fmt.Sprintf("%s is owned by %s, whose session is no longer running. Nobody finished this work: take it over by setting the key with if_version %d.",
			key, ownerName(*out.Value), out.Value.Version)
	}
	return out, nil
}

// set writes one key under whatever CAS rule the caller stated.
func (sh *shared) set(st sharedStore, p proto.SyncSharedParams, key string) (proto.SyncSharedResult, *proto.RPCError) {
	if !store.SharedKeyValid(key) {
		return proto.SyncSharedResult{}, &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: `shared: a key cannot contain "*". A wildcard is something you READ with (a get on "claims/*" returns the family), so a key holding one would be unreachable by an exact get and matched by every prefix get by accident.`}
	}
	if err := sh.gate("shared set", p.Identity.Key); err != nil {
		return proto.SyncSharedResult{}, err
	}
	if len(p.Value) == 0 {
		return proto.SyncSharedResult{}, &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: `shared set needs a value. To remove a key use op "delete"; a value of null is a value, and it would leave the key claimed.`}
	}
	if maxBytes := sh.max(); len(p.Value) > maxBytes {
		return proto.SyncSharedResult{}, &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: fmt.Sprintf("shared set: the value is %d bytes, over the %d-byte cap. A shared value is coordination state - a claim, a counter, a pointer - so put the payload in a file and share the path.", len(p.Value), maxBytes)}
	}

	// Recorded at write time, never derived at read time: what makes an orphaned
	// claim reportable is that the row remembers who made it after that session is
	// gone from every live structure.
	var owner, ownerAgent string
	var ownerPID int
	if p.Own {
		owner, ownerAgent, ownerPID = p.Identity.Key, p.Identity.Agent, p.PID
	}
	v, applied, err := st.SharedSet(key, string(p.Value), p.IfVersion, owner, ownerAgent, ownerPID)
	if err != nil {
		code, msg := proto.CodeInternal, "could not write the shared value: "+err.Error()
		if errors.Is(err, store.ErrSharedFull) {
			code = proto.CodeInvalidParams
			msg = fmt.Sprintf("shared set: the blackboard already holds %d keys, which is the cap, so a NEW key is refused - the cap fails loudly rather than evicting somebody's claim, because a claim that vanishes hands one chunk to two agents. Delete the keys whose work is finished.", store.SharedKeyMax)
		}
		return proto.SyncSharedResult{}, &proto.RPCError{Code: code, Message: msg}
	}

	out := proto.SyncSharedResult{OK: true, Op: proto.SharedOpSet, Applied: applied}
	marked := sh.markOwners([]proto.SharedValue{v})
	out.Value = &marked[0]
	if !applied {
		out.Stale = true
		out.Note = sh.staleNote(key, p, *out.Value)
		sh.log.Info(logging.EvSync, "component", "daemon", "sync", "shared_refused",
			"key", key, "version", v.Version, "agent", p.Identity.Agent, "key_of", p.Identity.Key)
		return out, nil
	}

	sh.log.Info(logging.EvSync, "component", "daemon", "sync", "shared_set",
		"key", key, "version", v.Version, "bytes", len(p.Value),
		"owned", p.Own, "agent", p.Identity.Agent, "key_of", p.Identity.Key)
	// Outside every lock this file holds, and through the one hub: a peer parked on
	// shared:claims/* is woken by the same mechanism a peer parked on tests:green is.
	sh.emit(key, p.Identity, map[string]any{
		"key": key, "version": v.Version, "owner": v.Owner, "owner_agent": v.OwnerAgent,
	})
	sh.pushed()
	return out, nil
}

// del removes one key, optionally only at the version the caller last saw.
func (sh *shared) del(st sharedStore, p proto.SyncSharedParams, key string) (proto.SyncSharedResult, *proto.RPCError) {
	if err := sh.gate("shared delete", p.Identity.Key); err != nil {
		return proto.SyncSharedResult{}, err
	}
	if p.IfVersion != nil && *p.IfVersion == 0 {
		// 0 means "only if it does not exist", which is not a thing a delete can mean.
		// Refusing beats guessing: read as "unconditional" it deletes a key the caller
		// may have meant to protect.
		return proto.SyncSharedResult{}, &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: `shared delete: if_version 0 means "only if this key does not exist", which cannot be a delete. Omit if_version to delete whatever is there, or pass the version you last read to delete it only if nobody has touched it since.`}
	}
	v, applied, err := st.SharedDelete(key, p.IfVersion)
	if err != nil {
		return proto.SyncSharedResult{}, &proto.RPCError{Code: proto.CodeInternal,
			Message: "could not delete the shared value: " + err.Error()}
	}

	out := proto.SyncSharedResult{OK: true, Op: proto.SharedOpDelete, Applied: applied}
	marked := sh.markOwners([]proto.SharedValue{v})
	out.Value = &marked[0]
	if !applied {
		out.Stale = true
		if v.Version == 0 {
			out.Note = fmt.Sprintf("%s does not exist, so there was nothing to delete. If you were finishing a claim, somebody has already removed it.", key)
		} else {
			out.Note = fmt.Sprintf("%s is at version %d, not the version you asked to delete at, so somebody wrote it after you last read it. Read it again before deciding: the work you were about to declare finished may now be theirs.", key, v.Version)
		}
		return out, nil
	}

	sh.log.Info(logging.EvSync, "component", "daemon", "sync", "shared_deleted",
		"key", key, "version", v.Version, "agent", p.Identity.Agent, "key_of", p.Identity.Key)
	// A delete is a change like any other, and a waiter that only heard about writes
	// would park forever on a key that has been finished and removed.
	sh.emit(key, p.Identity, map[string]any{"key": key, "deleted": true, "version": v.Version})
	sh.pushed()
	return out, nil
}

// staleNote says what stopped the write and what to do about it, in the terms the
// caller's own request was in. A refusal that only says "no" costs a model turn to
// interpret; this one is meant to be actionable without a second call.
func (sh *shared) staleNote(key string, p proto.SyncSharedParams, cur proto.SharedValue) string {
	switch {
	case p.IfVersion != nil && *p.IfVersion == 0 && cur.Version > 0:
		who := ownerName(cur)
		if who == "" {
			return fmt.Sprintf("%s was already claimed (version %d) before your write, so it is not yours. That is the normal outcome of a race, not an error: move on to the next key.", key, cur.Version)
		}
		if cur.OwnerGone {
			return fmt.Sprintf("%s is already claimed by %s, whose session is no longer running - so the work was started and abandoned rather than finished. Take it over with if_version %d instead of claiming from empty.", key, who, cur.Version)
		}
		return fmt.Sprintf("%s is already claimed by %s, which is still running. That is the normal outcome of a race, not an error: move on to the next key.", key, who)
	case cur.Version == 0:
		return fmt.Sprintf("%s does not exist, so it cannot be at the version you asked for. Claim it with if_version 0 instead, which succeeds only if nobody beats you to it.", key)
	default:
		return fmt.Sprintf("%s is at version %d, not the version you wrote against, so somebody changed it after you read it. The current value and version are here: decide from them and write again with if_version %d.", key, cur.Version, cur.Version)
	}
}

// markOwners fills in OwnerGone by asking the roster, which is the half of ownership
// that cannot be stored. Done in one pass over a read so a prefix get costs one
// roster question per owned key rather than one per key.
//
// The observer is read under the mutex and CALLED outside it - the lock order this
// daemon's three sync subsystems all obey.
func (sh *shared) markOwners(in []proto.SharedValue) []proto.SharedValue {
	sh.mu.Lock()
	present, alive := sh.presentFn, sh.alive
	sh.mu.Unlock()
	out := make([]proto.SharedValue, 0, len(in))
	for _, v := range in {
		v.OwnerGone = v.Owner != "" && !sh.ownerLives(present, alive, v)
		out = append(out, v)
	}
	return out
}

// ownerLives is the two-step answer, and the ORDER is the whole point.
//
// The roster is asked first because it is the live truth: a session that is attached
// is doing the work whatever its pid looks like. The pid is asked only when the
// roster cannot answer, and that is the case a daemon restart creates - every row is
// gone for the second it takes each child to redial, and without this step every
// claim on the board read as abandoned in that window, which is an invitation to
// take over a chunk somebody is writing.
//
// A claim with no pid recorded (the CLI's honest zero) falls back to the roster's
// answer alone. That keeps a shell's write from manufacturing a false orphan the
// moment the shell exits, at the cost of the restart window for those keys only.
func (sh *shared) ownerLives(present func(key string) bool, alive func(pid int) bool, v proto.SharedValue) bool {
	if present != nil && present(v.Owner) {
		return true
	}
	if v.OwnerPID > 0 && alive != nil {
		return alive(v.OwnerPID)
	}
	return false
}

// emit puts the change on shared:<key>. Through the daemon's postSignal callback,
// which is the only wake mechanism in this design: "waiting on a value" is
// await_signal(["shared:claims/*"]) and nothing else.
func (sh *shared) emit(key string, id proto.Identity, data map[string]any) {
	sh.mu.Lock()
	post := sh.post
	sh.mu.Unlock()
	if post == nil {
		return
	}
	post(proto.SharedTopic(key), id, data)
}

// gate is the announce door, the same one the locks and signals use. Writes only:
// the reads stay open because visibility must not depend on good manners.
func (sh *shared) gate(tool, key string) *proto.RPCError {
	if key == "" {
		return &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: tool + " needs identity.key: one session, one key. The mcp child mints it; a CLI caller passes --key to act on behalf of a session."}
	}
	sh.mu.Lock()
	announced := sh.announcedFn
	sh.mu.Unlock()
	if announced != nil && !announced(key) {
		return &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: "announce first: call announce with what this session is FOR, then use shared values. A claim the human cannot attribute to a purpose is one he cannot judge when the work stalls."}
	}
	return nil
}

// pushed repaints the board, outside the mutex like every other observer call.
func (sh *shared) pushed() {
	sh.mu.Lock()
	changed := sh.changed
	sh.mu.Unlock()
	if changed != nil {
		changed()
	}
}

func (sh *shared) store() sharedStore {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	return sh.st
}

func (sh *shared) max() int {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	return sh.maxBytes
}

// countGone is how many of a batch are owned by sessions that have died, for the
// one sentence a reader needs to notice them.
func countGone(in []proto.SharedValue) int {
	n := 0
	for _, v := range in {
		if v.OwnerGone {
			n++
		}
	}
	return n
}

// ownerName is how a value's owner is named in a sentence: the agent, since the
// session key is a hex string nobody reads. Recording the name at write time is
// what makes this possible after the session is gone.
func ownerName(v proto.SharedValue) string {
	switch {
	case v.OwnerAgent != "":
		return v.OwnerAgent
	case v.Owner != "":
		return "session " + v.Owner
	}
	return ""
}

// sharedParams parses the one method's params, with the error naming the shape.
func sharedParams(params []byte) (proto.SyncSharedParams, *proto.RPCError) {
	const shape = `{"identity": {...}, "op": "get|set|delete", "key": "claims/chunk-3", "value"?: {...}, "if_version"?: N, "own"?: true}`
	var p proto.SyncSharedParams
	if len(params) == 0 {
		return p, &proto.RPCError{Code: proto.CodeInvalidParams, Message: "sync_shared wants " + shape}
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return p, &proto.RPCError{Code: proto.CodeInvalidParams, Message: "sync_shared wants " + shape + ": " + err.Error()}
	}
	return p, nil
}
