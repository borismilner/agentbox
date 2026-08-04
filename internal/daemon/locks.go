package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/borismilner/agentbox/internal/logging"
	"github.com/borismilner/agentbox/internal/proto"
)

// Locks (FR83, slice 2): named, exclusive, advisory leases, so the agents
// sharing this machine take turns instead of asking Boris to sequence them.
//
// The names are a convention rather than a registry - `deploy:agentbox`,
// `repo:agentbox`, `vm:boris-vm` - and the manual teaches the idiom. What this
// file owes the design is three behaviours that are easy to get wrong:
//
//   - **A dead holder does not silently free a live resource.** An attach
//     dropping proves the mcp child died; it says nothing about the work. A
//     `make deploy` the session started is still running, and handing the lock
//     to the next agent then is the exact failure the lock exists to prevent.
//     So a hold whose session goes away becomes ORPHANED, with the pid it
//     recorded, and the next agent gets it only when that pid is gone too (or
//     when the human breaks it). `release_on_detach` opts a hold out, for when
//     the session IS the critical section.
//   - **Deadlock is refused at acquire time, by name.** The daemon sees the
//     whole wait-for graph, so an acquire that would close a cycle fails
//     immediately with both sides named. Cheap at this scale, and it turns the
//     worst failure mode of locks into an ordinary refusal.
//   - **A refusal carries the whole picture.** Holder, purpose, activity, how
//     long it has held, how many are queued - so the caller decides without a
//     second call. Reading a refusal must never require asking a follow-up
//     question (the house rule the control refusal already follows).
//
// Held state is memory only, on principle: a hold must not outlive the ability
// to observe its holder, so a daemon restart drops every lock and the first
// touch afterwards says so honestly rather than pretending to know.
const (
	// lockTickEvery is how often the table looks at itself: orphaned holds whose
	// pid may have died, ttl holds that may have expired, waits old enough to
	// warn about. Any state derived from elapsed time needs a clock of its own -
	// the lesson the roster's own tick paid for.
	lockTickEvery = time.Second

	// Reasons a waiter was granted, or a holder lost its hold. They travel to the
	// agent, so they are sentences it can act on rather than codes.
	LockReasonReleased   = "released"
	LockReasonHolderGone = "holder gone"
	LockReasonBroken     = "broken by the human"
	LockReasonExpired    = "ttl expired"
)

type lockHold struct {
	name     string
	key      string
	identity proto.Identity
	note     string

	// pid is whose life this hold follows once the session is gone. The mcp child
	// reports its own; a CLI wrap reports the wrapper's, which is the case the
	// orphan rule exists for. Zero means nothing can be probed, so the hold is
	// freed after the grace instead of waiting for a pid that will never die.
	pid   int
	since time.Time

	releaseOnDetach bool
	// expires is a ttl hold's deadline: the detached form, for a script that
	// cannot keep a process alive around its critical section.
	expires time.Time

	orphaned   bool
	orphanedAt time.Time
}

type lockWaiter struct {
	name     string
	key      string
	identity proto.Identity
	note     string
	pid      int
	since    time.Time

	releaseOnDetach bool

	// out is buffered so a grant never blocks the releasing caller. A waiter that
	// has already given up still receives it and releases immediately, which is
	// why the send side never has to know whether anybody is still listening.
	out chan lockGrant

	warnedLong  bool
	warnedHuman bool
}

// lockGrant is what a parked waiter wakes with.
type lockGrant struct {
	reason string
}

type locks struct {
	log *slog.Logger

	mu    sync.Mutex
	held  map[string]*lockHold
	queue map[string][]*lockWaiter

	// notices are lines owed to a session about a hold it lost without asking:
	// broken by the human, or reclaimed once its pid died. They ride the next
	// sync call that session makes (the discovery rider's envelope), so a working
	// agent finds out at its next touch of the platform rather than only if it
	// happens to touch that lock again.
	notices map[string][]string

	// The observers. announced gates the verbs, agentOf paints a holder in the
	// terms the human already reads, asking answers "is the holder parked on a
	// human question", warn is the one coordination toast that earns its
	// interruption, and changed repaints the board.
	announcedFn func(key string) bool
	agentOf     func(key string) (proto.SyncAgent, bool)
	askingFn    func() map[string]bool
	warn        func(title, body string)
	changed     func()

	// alive is the liveness probe, swappable so a test can kill a pid without
	// owning a process.
	alive func(pid int) bool

	waitWarn time.Duration
	grace    time.Duration

	stop chan struct{}
}

// lockParams parses any lock verb's params, with the error naming the shape.
func lockParams(params []byte, method string) (proto.SyncLockParams, *proto.RPCError) {
	var p proto.SyncLockParams
	if len(params) == 0 {
		return p, &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: method + ` wants {"identity": {...}, "name": "kind:scope"}`}
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return p, &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: method + ` wants {"identity": {...}, "name": "kind:scope", "timeout_s"?: N}: ` + err.Error()}
	}
	return p, nil
}

func newLocks(log *slog.Logger) *locks {
	return &locks{
		log:      log,
		held:     map[string]*lockHold{},
		queue:    map[string][]*lockWaiter{},
		notices:  map[string][]string{},
		alive:    pidAlive,
		waitWarn: 10 * time.Minute,
		grace:    5 * time.Second,
	}
}

// SetObservers wires what the table cannot see for itself. Every one of them is
// legitimately nil in a test.
func (l *locks) SetObservers(announced func(string) bool, agentOf func(string) (proto.SyncAgent, bool),
	asking func() map[string]bool, warn func(title, body string), changed func()) {
	l.mu.Lock()
	l.announcedFn, l.agentOf, l.askingFn, l.warn, l.changed = announced, agentOf, asking, warn, changed
	l.mu.Unlock()
}

// SetPolicy applies the two knobs that bound a wait and an orphan.
// A zero waitWarn is "never warn", which is what the config file's 0 means and
// therefore has to survive the trip. A zero grace is the only value that means
// "not configured", because an orphan freed with no grace at all would race the
// pid probe against the process it is probing.
func (l *locks) SetPolicy(waitWarn, grace time.Duration) {
	l.mu.Lock()
	if waitWarn >= 0 {
		l.waitWarn = waitWarn
	}
	if grace > 0 {
		l.grace = grace
	}
	l.mu.Unlock()
}

// pidAlive answers whether a process exists. EPERM counts as alive: the process
// is there, it just is not ours to signal, and treating that as death would free
// a resource somebody is still using.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// Acquire takes the lock or BLOCKS in a FIFO queue until it is granted, the
// timeout passes, or the caller goes away. A timeout is a result and not an
// error, carrying everything needed to decide what to do next.
//
// waitMax is the ceiling on any park - the client aborts a tool call that has
// been silent too long, so a wait that promised to sleep forever would be a lie
// the transport eventually tells. Hitting it returns timed_out, and re-arming is
// one call that misses nothing.
func (l *locks) Acquire(ctx context.Context, p proto.SyncLockParams, waitMax time.Duration) (proto.SyncLockResult, *proto.RPCError) {
	name, key, rpcErr := l.verb("acquire_lock", p)
	if rpcErr != nil {
		return proto.SyncLockResult{}, rpcErr
	}
	started := time.Now()

	res, waiter, rpcErr := l.tryOrQueue(ctx, p, name, key, true)
	if rpcErr != nil || waiter == nil {
		return res, rpcErr
	}

	// The park. Its ceiling is the smaller of what the caller asked for and what
	// the transport tolerates. A zero ceiling is a daemon nobody configured rather
	// than an instruction to give up at once, which is what taking it literally
	// would do to every acquire.
	if waitMax <= 0 {
		waitMax = 25 * time.Minute
	}
	wait := time.Duration(p.TimeoutS) * time.Second
	if wait <= 0 || wait > waitMax {
		wait = waitMax
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case grant := <-waiter.out:
		l.mu.Lock()
		l.grantedLocked(waiter)
		out := l.grantedPictureLocked(name)
		l.mu.Unlock()
		l.log.Info(logging.EvSync, "component", "daemon", "sync", "lock_acquired",
			"lock", name, "agent", p.Identity.Agent, "key", key, "why", grant.reason,
			"waited_ms", time.Since(started).Milliseconds())
		l.pushed()
		out.OK, out.Granted, out.Name = true, true, name
		out.Reason = grant.reason
		out.WaitedMS = time.Since(started).Milliseconds()
		return out, nil

	case <-timer.C:
		out := l.giveUp(waiter, name, key)
		out.TimedOut, out.WaitedMS = true, time.Since(started).Milliseconds()
		out.Note = fmt.Sprintf("not granted within %s. The queue kept your place only while you waited; re-arm with another acquire_lock, or use try_lock if you would rather do something else. Nothing about the holder has changed by your asking.", wait)
		return out, nil

	case <-ctx.Done():
		// The agent went away mid-wait. Leaving it queued would hand the lock to a
		// session that is not there, and the next waiter would sit behind a ghost.
		out := l.giveUp(waiter, name, key)
		out.WaitedMS = time.Since(started).Milliseconds()
		return out, nil
	}
}

// Try is the non-blocking form: granted now, or refused now with the same full
// picture a timeout carries. A separate verb because blocking and non-blocking
// never share one tool (the FR74 rule).
func (l *locks) Try(p proto.SyncLockParams) (proto.SyncLockResult, *proto.RPCError) {
	name, key, rpcErr := l.verb("try_lock", p)
	if rpcErr != nil {
		return proto.SyncLockResult{}, rpcErr
	}
	res, _, rpcErr := l.tryOrQueue(context.Background(), p, name, key, false)
	return res, rpcErr
}

// tryOrQueue is the shared front half of acquire and try: grant if it is free,
// refuse a deadlock by name, and otherwise either queue (acquire) or refuse
// (try). The returned waiter is non-nil only when the caller must park.
func (l *locks) tryOrQueue(ctx context.Context, p proto.SyncLockParams, name, key string, queue bool) (proto.SyncLockResult, *lockWaiter, *proto.RPCError) {
	now := time.Now()

	l.mu.Lock()
	held := l.held[name]

	// Already yours. No reentrancy counting in v1, so this is idempotent rather
	// than an error: an agent that takes the same lock twice across two calls
	// means to hold it, and refusing would only teach it to guess.
	if held != nil && held.key == key {
		held.note = firstNonEmpty(p.Note, held.note)
		held.orphaned = false
		// A re-acquire may also correct the pid, and the CLI wrap depends on it:
		// the hold has to be taken BEFORE the command starts, so the only pid it
		// can name at that moment is the wrapper's. Once the command is running,
		// the process whose life the resource really follows is that one - kill the
		// wrapper and the command survives, and the lock must not be handed on
		// while it does.
		if p.PID > 0 {
			held.pid = p.PID
		}
		out := l.pictureLocked(name, key)
		l.mu.Unlock()
		out.OK, out.Granted, out.Name, out.Reason = true, true, name, "already yours"
		out.Note = "you already held this lock; it is still yours and one release_lock frees it."
		return out, nil, nil
	}

	if held == nil {
		l.holdLocked(&lockHold{
			name: name, key: key, identity: p.Identity, note: p.Note,
			pid: p.PID, since: now, releaseOnDetach: p.ReleaseOnDetach,
			expires: expiryOf(now, p.TTLS),
		})
		out := l.grantedPictureLocked(name)
		l.mu.Unlock()
		l.log.Info(logging.EvSync, "component", "daemon", "sync", "lock_acquired",
			"lock", name, "agent", p.Identity.Agent, "key", key, "ttl_s", p.TTLS)
		l.pushed()
		out.OK, out.Granted, out.Name = true, true, name
		return out, nil, nil
	}

	// Held by somebody else. A wait that would close a cycle is refused here
	// rather than discovered by two agents sitting still forever.
	if chain := l.cycleLocked(key, name); chain != "" {
		out := l.pictureLocked(name, key)
		l.mu.Unlock()
		l.log.Warn(logging.EvSync, "component", "daemon", "sync", "deadlock_refused",
			"lock", name, "agent", p.Identity.Agent, "key", key, "chain", chain)
		out.OK, out.Refused, out.Name, out.Deadlock = true, true, name, chain
		out.Note = "refused: this would deadlock. " + chain + ". Release what you hold first, or coordinate with the other agent through its roster row."
		l.warnOf("Deadlock refused: "+name, chain)
		return out, nil, nil
	}

	if !queue {
		out := l.pictureLocked(name, key)
		l.mu.Unlock()
		out.OK, out.Refused, out.Name = true, true, name
		out.Note = "held by another agent. try_lock never waits: do something else and come back, or call acquire_lock with a timeout to queue."
		return out, nil, nil
	}

	w := &lockWaiter{
		name: name, key: key, identity: p.Identity, note: p.Note, pid: p.PID,
		since: now, releaseOnDetach: p.ReleaseOnDetach, out: make(chan lockGrant, 1),
	}
	l.queue[name] = append(l.queue[name], w)
	holderKey := held.key
	l.mu.Unlock()

	l.log.Info(logging.EvSync, "component", "daemon", "sync", "lock_waiting",
		"lock", name, "agent", p.Identity.Agent, "key", key, "held_by", holderKey)
	l.pushed()
	// A holder parked on a human answer cannot be refused - the card is already up
	// - so the chain is said out loud instead. This is the one moment a
	// coordination toast earns its interruption.
	l.warnHumanEdge(w, name, holderKey)
	return proto.SyncLockResult{}, w, nil
}

// Release frees a lock and hands it to the next waiter. Only the holder may
// release: a stray release from another agent would hand the resource over while
// the work is still running.
func (l *locks) Release(p proto.SyncLockParams) (proto.SyncLockResult, *proto.RPCError) {
	name, key, rpcErr := l.verb("release_lock", p)
	if rpcErr != nil {
		return proto.SyncLockResult{}, rpcErr
	}

	l.mu.Lock()
	held := l.held[name]
	if held == nil {
		// Also drop a queued wait of ours, so "release" is the one verb that
		// undoes an acquire whichever half of it we are in.
		dropped := l.dequeueLocked(name, key)
		l.mu.Unlock()
		out := proto.SyncLockResult{OK: true, Name: name}
		if dropped {
			out.Note = "you were queued for this lock, not holding it; your place in the queue is gone."
			l.pushed()
			return out, nil
		}
		out.Note = "nobody holds this lock, so there was nothing to release."
		return out, nil
	}
	if held.key != key {
		out := l.pictureLocked(name, key)
		l.mu.Unlock()
		out.OK, out.Name = true, name
		out.Note = "not yours to release: another agent holds it. Only the holder may release, so a stray call cannot free a resource somebody is still using."
		return out, nil
	}
	l.releaseLocked(name, LockReasonReleased)
	l.mu.Unlock()

	l.log.Info(logging.EvSync, "component", "daemon", "sync", "lock_released",
		"lock", name, "agent", p.Identity.Agent, "key", key)
	l.pushed()
	return proto.SyncLockResult{OK: true, Name: name, Released: true}, nil
}

// Break is the human taking a lock away from an agent, from the row detail in
// the Agents surface. It reassigns the lock; it does not stop the ex-holder,
// which is why the copy beside the button says so and why the ex-holder is told.
func (l *locks) Break(name string) proto.SyncLockResult {
	name = strings.TrimSpace(name)
	l.mu.Lock()
	held := l.held[name]
	if held == nil {
		l.mu.Unlock()
		return proto.SyncLockResult{OK: true, Name: name, Note: "nobody holds that lock."}
	}
	who := held.identity.Agent
	l.noticeLocked(held.key, fmt.Sprintf("sync: the human broke your lock %s. It has been handed to whoever was waiting. Your own work was NOT stopped - stop touching what the lock protects, or take it again.", name))
	l.releaseLocked(name, LockReasonBroken)
	l.mu.Unlock()

	l.log.Warn(logging.EvSync, "component", "daemon", "sync", "lock_broken", "lock", name, "was_held_by", who)
	l.pushed()
	return proto.SyncLockResult{OK: true, Name: name, Released: true, Reason: LockReasonBroken}
}

// SessionGone is the attach dropping: the mcp child is dead, so the row goes -
// but a hold does not, unless it asked to. See the orphan rule at the top.
func (l *locks) SessionGone(key string) {
	if key == "" {
		return
	}
	now := time.Now()
	var released, orphaned []string

	l.mu.Lock()
	for name, held := range l.held {
		if held.key != key {
			continue
		}
		if held.releaseOnDetach {
			l.releaseLocked(name, LockReasonHolderGone)
			released = append(released, name)
			continue
		}
		held.orphaned, held.orphanedAt = true, now
		orphaned = append(orphaned, name)
	}
	// A waiter that is gone must not keep its place: the next in line would sit
	// behind a session that will never take the grant.
	for name := range l.queue {
		l.dequeueLocked(name, key)
	}
	l.mu.Unlock()

	for _, name := range released {
		l.log.Info(logging.EvSync, "component", "daemon", "sync", "lock_released",
			"lock", name, "key", key, "why", "release_on_detach")
	}
	for _, name := range orphaned {
		l.log.Warn(logging.EvSync, "component", "daemon", "sync", "lock_orphaned", "lock", name, "key", key)
	}
	if len(released) > 0 || len(orphaned) > 0 {
		l.pushed()
	}
}

// TakeNotices hands over what this session is owed, once. Called by the rider,
// which is the only path that has a tool result to put a line on.
func (l *locks) TakeNotices(key string) []string {
	if key == "" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.notices[key]
	delete(l.notices, key)
	return out
}

// keepNotices puts lines back, for a caller that turned out to have nowhere to
// show them. Prepended: they were owed before whatever arrives next.
func (l *locks) keepNotices(key string, lines []string) {
	if key == "" || len(lines) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.notices[key] = append(lines, l.notices[key]...)
}

// Holds and waits, for the roster snapshot. One read under one lock so a row
// cannot show a hold and a wait from two different instants.
func (l *locks) rows() (map[string][]proto.SyncHold, map[string]proto.SyncWait) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	holds := make(map[string][]proto.SyncHold, len(l.held))
	for name, h := range l.held {
		holds[h.key] = append(holds[h.key], proto.SyncHold{
			Name: name, SinceMS: now.Sub(h.since).Milliseconds(),
			Orphaned: h.orphaned, PID: h.pid, PIDLive: l.alive(h.pid),
			Waiters: len(l.queue[name]), Note: h.note,
		})
	}
	for _, list := range holds {
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	}
	waits := map[string]proto.SyncWait{}
	for name, q := range l.queue {
		holderKey, holderAgent := "", ""
		if h := l.held[name]; h != nil {
			holderKey, holderAgent = h.key, h.identity.Agent
		}
		for i, w := range q {
			// One row shows one wait, and a session parked in a tool call can only
			// really have one. The first is the one it is closest to getting.
			if _, already := waits[w.key]; already {
				continue
			}
			waits[w.key] = proto.SyncWait{
				Name: name, SinceMS: now.Sub(w.since).Milliseconds(),
				Ahead: i, Queue: len(q), HolderKey: holderKey, HolderAgent: holderAgent,
			}
		}
	}
	return holds, waits
}

// Snapshot is every lock the daemon knows, for a CLI read and for the surface's
// pull on mount. Sorted by name so two reads of an unchanged table are equal.
func (l *locks) Snapshot() []proto.SyncLockState {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]proto.SyncLockState, 0, len(l.held))
	for name, h := range l.held {
		st := proto.SyncLockState{
			Name: name, HolderKey: h.key, HolderAgent: h.identity.Agent,
			Note: h.note, PID: h.pid, HeldMS: now.Sub(h.since).Milliseconds(),
			Orphaned: h.orphaned, Waiters: len(l.queue[name]),
		}
		if !h.expires.IsZero() {
			st.ExpiresInMS = max(h.expires.Sub(now).Milliseconds(), 0)
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// tick is the table's own clock: an orphan whose pid has died, a ttl that has
// passed, a wait old enough to be worth the human's attention. None of these
// are caused by a verb, so nothing else would ever notice them.
func (l *locks) tick() {
	now := time.Now()
	type freed struct{ name, why string }
	var free []freed
	var longWaits []*lockWaiter

	l.mu.Lock()
	for name, h := range l.held {
		switch {
		case !h.expires.IsZero() && now.After(h.expires):
			free = append(free, freed{name, LockReasonExpired})
		case h.orphaned && !l.alive(h.pid) && now.Sub(h.orphanedAt) >= l.grace:
			// The session is gone AND the work it recorded is gone. Only now is
			// handing the resource on safe.
			free = append(free, freed{name, LockReasonHolderGone})
		}
	}
	for _, f := range free {
		if h := l.held[f.name]; h != nil {
			why := "the holder's session ended and its process is gone"
			if f.why == LockReasonExpired {
				why = "its ttl ran out"
			}
			l.noticeLocked(h.key, fmt.Sprintf("sync: your lock %s was released - %s. Take it again if you still need it.", f.name, why))
		}
		l.releaseLocked(f.name, f.why)
	}
	if l.waitWarn > 0 {
		for _, q := range l.queue {
			for _, w := range q {
				if !w.warnedLong && now.Sub(w.since) >= l.waitWarn {
					w.warnedLong = true
					longWaits = append(longWaits, w)
				}
			}
		}
	}
	l.mu.Unlock()

	for _, f := range free {
		l.log.Info(logging.EvSync, "component", "daemon", "sync", "lock_released", "lock", f.name, "why", f.why)
	}
	for _, w := range longWaits {
		holder := "another agent"
		if a, ok := l.agent(w.key, w.name); ok {
			holder = a
		}
		l.log.Warn(logging.EvSync, "component", "daemon", "sync", "stall_warned",
			"lock", w.name, "agent", w.identity.Agent, "waited_s", int(now.Sub(w.since).Seconds()))
		l.warnOf(fmt.Sprintf("Waiting %s for lock %s", roundDur(now.Sub(w.since)), w.name),
			fmt.Sprintf("%s has been queued behind %s for %s. Contention, not progress: one of them may need you.",
				nameOr(w.identity.Agent, "an agent"), holder, roundDur(now.Sub(w.since))))
	}
	if len(free) > 0 {
		l.pushed()
	}
}

// Start begins the tick; Stop ends it. A table with no tick still answers every
// verb correctly - it only stops noticing the things time does.
func (l *locks) Start() {
	l.mu.Lock()
	if l.stop != nil {
		l.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	l.stop = stop
	l.mu.Unlock()

	go func() {
		t := time.NewTicker(lockTickEvery)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				l.tick()
			}
		}
	}()
}

func (l *locks) Stop() {
	l.mu.Lock()
	if l.stop != nil {
		close(l.stop)
		l.stop = nil
	}
	l.mu.Unlock()
}

// verb is the shared front door: a name, a key, and the announce gate.
//
// The gate is the design's answer to "how hard is the mandate": the sync verbs
// refuse an unannounced session with a teaching error, while presence and the
// monitoring reads never refuse - visibility must not depend on good manners,
// but taking a shared resource without saying who you are must not be possible.
func (l *locks) verb(tool string, p proto.SyncLockParams) (name, key string, rpcErr *proto.RPCError) {
	name = strings.TrimSpace(p.Name)
	key = strings.TrimSpace(p.Identity.Key)
	if name == "" {
		return "", "", &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: tool + ` needs a lock name. Use the kind:scope idiom the rest of agentbox uses - "deploy:agentbox", "repo:agentbox", "vm:boris-vm" - so two agents reaching for the same resource pick the same name.`}
	}
	if key == "" {
		return "", "", &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: tool + " needs identity.key: one session, one key. The mcp child mints it; a CLI caller passes --key to act on behalf of a session."}
	}
	l.mu.Lock()
	announced := l.announcedFn
	l.mu.Unlock()
	if announced != nil && !announced(key) {
		return "", "", &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: "announce first: call announce with what this session is FOR, then take the lock. A lock the human cannot attribute to a purpose is a lock he cannot judge when it goes wrong."}
	}
	return name, key, nil
}

// holdLocked records a hold. Callers hold l.mu.
func (l *locks) holdLocked(h *lockHold) { l.held[h.name] = h }

// releaseLocked frees a lock and hands it to the first waiter still there.
// Callers hold l.mu.
func (l *locks) releaseLocked(name, reason string) {
	delete(l.held, name)
	for len(l.queue[name]) > 0 {
		w := l.queue[name][0]
		l.queue[name] = l.queue[name][1:]
		if len(l.queue[name]) == 0 {
			delete(l.queue, name)
		}
		now := time.Now()
		l.holdLocked(&lockHold{
			name: name, key: w.key, identity: w.identity, note: w.note,
			pid: w.pid, since: now, releaseOnDetach: w.releaseOnDetach,
		})
		// Buffered, so this cannot block on a waiter that has stopped listening.
		// One that has given up releases what it was handed (see giveUp).
		w.out <- lockGrant{reason: reason}
		return
	}
}

// grantedLocked is the waiter's side of a grant: it already owns the hold the
// releaser created, so this only fills in what the waiter knows and the releaser
// did not. Callers hold l.mu.
func (l *locks) grantedLocked(w *lockWaiter) {
	if h := l.held[w.name]; h != nil && h.key == w.key {
		h.note = firstNonEmpty(w.note, h.note)
	}
}

// grantedPictureLocked is what a successful acquire answers with. Deliberately
// not the holder picture: on a grant the holder is the caller, and a result that
// described the caller to itself would read as though somebody else had it.
// The queue stays, because "two agents are waiting behind you" is worth knowing
// before starting something long. Callers hold l.mu.
func (l *locks) grantedPictureLocked(name string) proto.SyncLockResult {
	return proto.SyncLockResult{Name: name, Queue: len(l.queue[name])}
}

// giveUp takes a waiter out of the queue, and releases the lock if it was
// granted in the same breath. Without that second half a timeout racing a
// release would leave the lock held by an agent that has already moved on.
func (l *locks) giveUp(w *lockWaiter, name, key string) proto.SyncLockResult {
	l.mu.Lock()
	if !l.dequeueWaiterLocked(w) {
		// Not in the queue any more: either granted while we were timing out, or
		// already dropped. Check the channel rather than assume.
		select {
		case <-w.out:
			l.releaseLocked(name, LockReasonReleased)
			out := l.pictureLocked(name, key)
			l.mu.Unlock()
			l.log.Info(logging.EvSync, "component", "daemon", "sync", "lock_released",
				"lock", name, "key", key, "why", "granted after the waiter gave up")
			l.pushed()
			out.OK, out.Name = true, name
			out.Note = "granted just as you stopped waiting, so it was released again rather than held by a call that had moved on. Ask again if you still need it."
			return out
		default:
		}
	}
	out := l.pictureLocked(name, key)
	l.mu.Unlock()
	l.pushed()
	out.OK, out.Name = true, name
	return out
}

// dequeueLocked drops every wait a session has on one lock. Callers hold l.mu.
func (l *locks) dequeueLocked(name, key string) bool {
	q := l.queue[name]
	kept := q[:0]
	dropped := false
	for _, w := range q {
		if w.key == key {
			dropped = true
			continue
		}
		kept = append(kept, w)
	}
	if !dropped {
		return false
	}
	if len(kept) == 0 {
		delete(l.queue, name)
	} else {
		l.queue[name] = kept
	}
	return true
}

// dequeueWaiterLocked drops one specific wait. Callers hold l.mu.
func (l *locks) dequeueWaiterLocked(w *lockWaiter) bool {
	q := l.queue[w.name]
	for i, other := range q {
		if other == w {
			l.queue[w.name] = append(q[:i:i], q[i+1:]...)
			if len(l.queue[w.name]) == 0 {
				delete(l.queue, w.name)
			}
			return true
		}
	}
	return false
}

// pictureLocked is the full answer a refusal or a timeout carries: who holds it,
// what they are for, what they are doing, how long they have had it, how many
// are queued. Callers hold l.mu.
//
// It is deliberately generous. The alternative is an agent that asks a follow-up
// question to find out whether to wait, which costs a whole model turn to learn
// something the daemon already knew.
func (l *locks) pictureLocked(name, key string) proto.SyncLockResult {
	out := proto.SyncLockResult{Name: name}
	if h := l.held[name]; h != nil {
		out.HeldMS = time.Since(h.since).Milliseconds()
		out.HolderNote = h.note
		out.Orphaned = h.orphaned
		out.HolderPID = h.pid
		if l.agentOf != nil {
			if a, ok := l.agentOf(h.key); ok {
				out.Holder = &a
			}
		}
		if out.Holder == nil {
			// No roster row: a hold from a CLI caller, or a session whose child has
			// gone. Say what is known rather than nothing, so the caller can still
			// see who to go and ask.
			out.Holder = &proto.SyncAgent{Key: h.key, Agent: h.identity.Agent,
				Project: h.identity.Project, State: StateDetached}
		}
		if h.orphaned {
			out.Note = fmt.Sprintf("the holder's session is gone but its process (pid %d) is still running, so the lock is orphaned rather than free: whatever it was protecting may still be in progress. It frees itself when that pid dies, or the human can break it from the Agents surface.", h.pid)
		}
	}
	out.Queue = len(l.queue[name])
	for _, w := range l.queue[name] {
		if w.key == key {
			out.Queued = true
		}
	}
	return out
}

// cycleLocked answers whether key waiting on name would close a lock cycle, and
// names it if so. Callers hold l.mu.
//
// The walk is over lock edges only. Two other edges exist in the real wait-for
// graph - a session parked on a human answer, and one holding the desktop - and
// neither can be refused, because the human's card is already up. Those warn
// instead (warnHumanEdge).
func (l *locks) cycleLocked(key, name string) string {
	holder := l.held[name]
	if holder == nil {
		return ""
	}
	// Walk: I would wait on `name`, held by H. What is H waiting on? Who holds
	// that? Until we run out of edges, or come back to me.
	type step struct{ lock, key string }
	chain := []step{{name, holder.key}}
	seen := map[string]bool{holder.key: true}
	at := holder.key
	for {
		if at == key {
			// Read as a sentence, in the order the agent would have to undo it:
			// "you asked for deploy:agentbox, held by codex; codex waits on
			// repo:agentbox, held by you".
			parts := []string{fmt.Sprintf("you asked for %s, held by %s", chain[0].lock, l.nameOfLocked(chain[0].key))}
			for i := 1; i < len(chain); i++ {
				who := l.nameOfLocked(chain[i].key)
				if chain[i].key == key {
					who = "you"
				}
				parts = append(parts, fmt.Sprintf("%s waits on %s, held by %s",
					l.nameOfLocked(chain[i-1].key), chain[i].lock, who))
			}
			// No closing clause: the walk always ends at the caller, so the last
			// step already reads "... held by you" and saying it again only made the
			// refusal look like a third party was involved.
			return strings.Join(parts, "; ")
		}
		next, nextHolder := "", ""
		for lname, q := range l.queue {
			for _, w := range q {
				if w.key != at {
					continue
				}
				if h := l.held[lname]; h != nil {
					next, nextHolder = lname, h.key
					break
				}
			}
			if next != "" {
				break
			}
		}
		if next == "" || seen[nextHolder] {
			return ""
		}
		chain = append(chain, step{next, nextHolder})
		seen[nextHolder] = true
		at = nextHolder
	}
}

// nameOfLocked is the agent name behind a session key, for a message a human or
// a model reads. Callers hold l.mu.
func (l *locks) nameOfLocked(key string) string {
	if l.agentOf != nil {
		if a, ok := l.agentOf(key); ok {
			return nameOr(a.Agent, key)
		}
	}
	for _, h := range l.held {
		if h.key == key {
			return nameOr(h.identity.Agent, key)
		}
	}
	return key
}

// warnHumanEdge toasts when a waiter has queued behind a holder that is itself
// parked on a question to the human. It cannot be refused like a lock cycle, and
// it is the stall that will actually happen: the human is the one who can end it,
// so he is the one who is told.
func (l *locks) warnHumanEdge(w *lockWaiter, name, holderKey string) {
	l.mu.Lock()
	asking := l.askingFn
	l.mu.Unlock()
	if asking == nil || holderKey == "" {
		return
	}
	if !asking()[holderKey] {
		return
	}
	l.mu.Lock()
	if w.warnedHuman {
		l.mu.Unlock()
		return
	}
	w.warnedHuman = true
	holder := l.nameOfLocked(holderKey)
	l.mu.Unlock()

	l.log.Warn(logging.EvSync, "component", "daemon", "sync", "stall_warned",
		"lock", name, "agent", w.identity.Agent, "why", "holder is asking you")
	l.warnOf("Lock "+name+" waits on your answer",
		fmt.Sprintf("%s is queued for %s, which %s holds while waiting for you to answer its question. Answering it unblocks both.",
			nameOr(w.identity.Agent, "an agent"), name, holder))
}

// noticeLocked owes a session a line about a hold it lost. Callers hold l.mu.
func (l *locks) noticeLocked(key, line string) {
	if key == "" {
		return
	}
	l.notices[key] = append(l.notices[key], line)
}

func (l *locks) warnOf(title, body string) {
	l.mu.Lock()
	warn := l.warn
	l.mu.Unlock()
	if warn != nil {
		warn(title, body)
	}
}

func (l *locks) pushed() {
	l.mu.Lock()
	changed := l.changed
	l.mu.Unlock()
	if changed != nil {
		changed()
	}
}

// agent is the holder's name for a message, outside the lock.
func (l *locks) agent(waiterKey, name string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	h := l.held[name]
	if h == nil || h.key == waiterKey {
		return "", false
	}
	return l.nameOfLocked(h.key), true
}

func expiryOf(now time.Time, ttlS int) time.Time {
	if ttlS <= 0 {
		return time.Time{}
	}
	return now.Add(time.Duration(ttlS) * time.Second)
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func nameOr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func roundDur(d time.Duration) time.Duration {
	if d >= time.Minute {
		return d.Round(time.Minute)
	}
	return d.Round(time.Second)
}
