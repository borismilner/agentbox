package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/borismilner/agentbox/internal/logging"
	"github.com/borismilner/agentbox/internal/proto"
)

// Signals (FR83, slice 3): agents wake each other and message each other, so
// "run the deploy when the tests are green" is two calls instead of a poll loop
// with a model turn per iteration.
//
// The design's four primitives compose, and this is the one that makes the other
// three worth having: a lock is how two agents take turns, a signal is how one
// tells the other it is their turn. What this file owes the design is four
// behaviours that are each easy to get subtly wrong:
//
//   - **The store is the source of truth and the channel is only a doorbell.** A
//     waiter woken by a send does not read the payload off the channel; it
//     re-reads the store from its cursor. That is what makes a batch a batch - an
//     agent that was busy while three signals fired catches up in one call - and
//     it is why a buffer of one is enough for a hub that fans out.
//   - **Register before checking.** A waiter is in the map before the catch-up
//     read that decides whether to park, or a signal landing between the read and
//     the registration is delivered to nobody and waited for forever. The three
//     hubs already in this daemon do it in that order for the same reason.
//   - **A trimmed cursor is reported, never silently served.** Retention is finite,
//     so a cursor can fall off the edge. A batch with a hole in it is how two
//     agents both come to own one chunk of work, so the gap is said out loud with
//     the sequence a complete read starts from (FR61's rule, on the wire). It is
//     answered from what retention RECORDED taking, per topic - see gapAt for the
//     plausible version of this that shipped and was wrong.
//   - **Fan-out is by meaning.** Every waiter whose pattern matches wakes with the
//     same signal. This is the first multi-consumer hub in the daemon on purpose:
//     artifacts and walkthrough submissions are single-consumer hand-offs and stay
//     that way, because a review taken twice is a review answered twice.
//
// A parked await is deliberately NOT warned about, however long it lasts.
// Listening is the intended steady state, and a toast for it would train Boris to
// ignore the one that matters (a lock wait, which means contention).

// signalDataMax bounds one payload. Not a knob: a signal is a fact, a request or
// a hand-off between programs, and anything that wants a megabyte wants a file
// path instead. It matches shared_max_bytes so the two halves of the blackboard
// cannot disagree about what "small" means.
const signalDataMax = 16 << 10

// signalTrimEvery is how often retention runs. Age-based trimming is caused by
// nothing but time passing, so like every other elapsed-time fact in this daemon
// it needs a clock of its own rather than a verb to ride on.
const signalTrimEvery = time.Minute

// signalStore is the persistence signals need, narrowed to what this file calls.
// A daemon built without a store (most tests) has none, and the verbs then refuse
// with a teaching error rather than pretending to deliver: durability is the
// feature, so an in-memory fallback would be a lie with a happy path.
type signalStore interface {
	PostSignal(topic string, id proto.Identity, data string) (proto.Signal, error)
	SignalsSince(after int64, patterns []string, limit int) ([]proto.Signal, bool, error)
	SignalHighWater() (int64, error)
	SignalGap(after int64, patterns []string) (int64, error)
	TrimTopic(topic string, keepPerTopic int) (int, error)
	TrimSignals(keepPerTopic int, maxAge time.Duration) (int, error)
}

type signalWaiter struct {
	key      string
	identity proto.Identity
	topics   []string
	cursor   int64
	since    time.Time

	// bell is buffered so a post never blocks on a waiter that has already given
	// up. It carries no payload: the store does, and reading from the store is what
	// turns a wake into the whole batch.
	bell chan struct{}
}

type signals struct {
	log *slog.Logger

	mu      sync.Mutex
	st      signalStore
	waiters map[*signalWaiter]struct{}

	// The observers. announced gates the verbs the same way it gates the locks,
	// and changed repaints the board when a row starts or stops listening.
	announcedFn func(key string) bool
	changed     func()

	keep     int
	keepDays int

	// received is who HEARD what, per session key, newest last. The store cannot
	// answer this: sync_signals records the poster's session_key and nothing about
	// delivery, because a signal is fanned out by meaning to whoever happens to be
	// listening and the same row is read by every one of them. So the only place
	// the fact exists is here, at the moment a wait resolves.
	//
	// In memory and bounded, like the roster's activity ring and for the same
	// reason: it is a glance backwards over a live session, not an audit trail.
	received map[string][]receivedTick

	stop chan struct{}
}

// receivedTick is one signal a session was handed.
type receivedTick struct {
	topic string
	data  string
	at    time.Time
}

func newSignals(log *slog.Logger) *signals {
	return &signals{
		log:      log,
		waiters:  map[*signalWaiter]struct{}{},
		received: map[string][]receivedTick{},
		keep:     1000,
		keepDays: 7,
	}
}

// recordReceived notes a batch handed to one session. The data is kept short: an
// opened row shows the topic and a glimpse, and the payload itself is the
// caller's to read from the tool result.
func (s *signals) recordReceived(key string, sigs []proto.Signal) {
	if strings.TrimSpace(key) == "" || len(sigs) == 0 {
		return
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	ticks := s.received[key]
	for _, sig := range sigs {
		at := now
		if sig.AtMS > 0 {
			at = time.UnixMilli(sig.AtMS)
		}
		ticks = append(ticks, receivedTick{topic: sig.Topic, data: glimpse(string(sig.Data)), at: at})
	}
	if n := len(ticks); n > signalsKeep {
		ticks = append([]receivedTick(nil), ticks[n-signalsKeep:]...)
	}
	s.received[key] = ticks
}

// receivedBy answers what one session has been handed, oldest first.
func (s *signals) receivedBy(key string) []receivedTick {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]receivedTick(nil), s.received[key]...)
}

// forgetReceived drops a session's ring when its row leaves the roster, so a
// machine that has run all day is not holding the topics of sessions that ended
// hours ago.
func (s *signals) forgetReceived(key string) {
	s.mu.Lock()
	delete(s.received, key)
	s.mu.Unlock()
}

// glimpse is as much of a payload as a row can show without becoming a log.
func glimpse(data string) string {
	const max = 80
	data = strings.TrimSpace(data)
	if len(data) <= max {
		return data
	}
	return data[:max] + "…"
}

// SetStore wires persistence. Separate from the constructor because a daemon
// assembles its store and its subsystems in that order.
func (s *signals) SetStore(st signalStore) {
	s.mu.Lock()
	s.st = st
	s.mu.Unlock()
}

// SetObservers wires the announce gate and the board repaint. Both are
// legitimately nil in a test.
func (s *signals) SetObservers(announced func(key string) bool, changed func()) {
	s.mu.Lock()
	s.announcedFn, s.changed = announced, changed
	s.mu.Unlock()
}

// SetRetention applies the two knobs. Zero means "leave the default alone" for
// both: unlike the lock's wait warning, neither has a meaningful "off" - a signal
// table that grew forever would be a slow leak with no upper bound.
func (s *signals) SetRetention(keep, keepDays int) {
	s.mu.Lock()
	if keep > 0 {
		s.keep = keep
	}
	if keepDays > 0 {
		s.keepDays = keepDays
	}
	s.mu.Unlock()
}

// signalParams parses either signal verb's params, with the error naming the
// shape the caller got wrong.
func signalParams[T any](params []byte, method, shape string) (T, *proto.RPCError) {
	var p T
	if len(params) == 0 {
		return p, &proto.RPCError{Code: proto.CodeInvalidParams, Message: method + " wants " + shape}
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return p, &proto.RPCError{Code: proto.CodeInvalidParams, Message: method + " wants " + shape + ": " + err.Error()}
	}
	return p, nil
}

// Post stores one signal and wakes every waiter whose pattern matches it.
// Non-blocking, and it answers with the sequence number - which is also the
// cursor to read from just before, so a poster can tell a peer where to start.
func (s *signals) Post(p proto.SyncPostParams) (proto.SyncPostResult, *proto.RPCError) {
	topic := strings.TrimSpace(p.Topic)
	key := strings.TrimSpace(p.Identity.Key)
	if topic == "" {
		return proto.SyncPostResult{}, &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: `post_signal needs a topic. Use the kind:scope idiom the rest of agentbox uses - "tests:green", "done:migration-3", "to:<session key>" for one agent - so a peer waiting for this event picks the same name.`}
	}
	if strings.Contains(topic, proto.TopicPrefix) {
		// A wildcard is a thing you WAIT on, never a thing you post to. Storing one
		// would create a topic no exact-name waiter can ever match and that every
		// prefix waiter matches by accident.
		return proto.SyncPostResult{}, &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: `post_signal needs an exact topic, not a pattern: "*" belongs in await_signal's topics, where it means a prefix. Post to the one topic this event is about.`}
	}
	if err := s.gate("post_signal", key); err != nil {
		return proto.SyncPostResult{}, err
	}
	if len(p.Data) > signalDataMax {
		return proto.SyncPostResult{}, &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: fmt.Sprintf("post_signal data is %d bytes, over the %d-byte cap. A signal carries a fact, a request or a hand-off between programs; put anything bigger in a file and send the path.", len(p.Data), signalDataMax)}
	}

	st := s.store()
	if st == nil {
		return proto.SyncPostResult{}, errNoSignalStore("post_signal")
	}
	sig, err := st.PostSignal(topic, p.Identity, string(p.Data))
	if err != nil {
		return proto.SyncPostResult{}, &proto.RPCError{Code: proto.CodeInternal,
			Message: "could not store the signal: " + err.Error()}
	}

	delivered := s.deliver(sig)
	s.log.Info(logging.EvSync, "component", "daemon", "sync", "signal_posted",
		"topic", topic, "seq", sig.Seq, "agent", p.Identity.Agent, "key", key,
		"bytes", len(p.Data), "delivered", delivered)
	// Trimming here keeps the per-topic count honest without waiting for the tick,
	// and it costs one indexed delete on the topic just written rather than a sweep
	// of every other topic's history.
	s.trimTopic(topic)

	out := proto.SyncPostResult{OK: true, Topic: topic, Seq: sig.Seq, Delivered: delivered}
	if delivered == 0 {
		out.Note = fmt.Sprintf("stored as seq %d with nobody parked on %s. That is not a failure: a signal is delivered whether or not anyone was waiting, and a later await_signal with after_seq below %d picks it up.", sig.Seq, topic, sig.Seq)
	}
	return out, nil
}

// emit is the daemon's own door: store one signal and wake its listeners, with no
// announce gate and no topic validation, because the caller is this daemon rather
// than an agent. The built-in topics come through here - a join, an announce, a
// departure, a lock changing hands - and nothing else may.
//
// It shares every line of Post's delivery so a built-in signal and an agent's
// signal are indistinguishable to a waiter. That is the point: `agents:<area>` and
// `to:<key>` are conventions over one mechanism, not a second mechanism.
func (s *signals) emit(topic string, id proto.Identity, data json.RawMessage) error {
	st := s.store()
	if st == nil {
		return nil
	}
	if len(data) > signalDataMax {
		return fmt.Errorf("signal data is %d bytes, over the %d-byte cap", len(data), signalDataMax)
	}
	sig, err := st.PostSignal(strings.TrimSpace(topic), id, string(data))
	if err != nil {
		return err
	}
	delivered := s.deliver(sig)
	s.log.Info(logging.EvSync, "component", "daemon", "sync", "signal_posted",
		"topic", sig.Topic, "seq", sig.Seq, "agent", id.Agent, "key", id.Key,
		"bytes", len(data), "delivered", delivered, "builtin", true)
	// The built-in topics are the chattiest ones there are - one per attach,
	// announce and departure - so they cap themselves here rather than waiting a
	// minute for the tick.
	s.trimTopic(sig.Topic)
	return nil
}

// Await parks until a matching signal arrives, the timeout passes, or the caller
// goes away - and returns everything matching since the cursor in one batch.
//
// waitMax is the ceiling on the park, for the reason the lock's acquire has one:
// the client abandons a tool call it has heard nothing about for 1800s and does
// not tell the server it has, so a wait that promised to sleep forever would
// leave a park nothing outside this daemon can ever end. Hitting the ceiling
// returns timed_out WITH the cursor, and re-arming is one call that misses
// nothing.
func (s *signals) Await(ctx context.Context, p proto.SyncAwaitParams, waitMax time.Duration) (proto.SyncAwaitResult, *proto.RPCError) {
	key := strings.TrimSpace(p.Identity.Key)
	topics := cleanTopics(p.Topics)
	if len(topics) == 0 {
		return proto.SyncAwaitResult{}, &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: `await_signal needs at least one topic. An exact name waits for one event ("tests:green"); a name ending in * waits for a family ("done:*"). Waiting on nothing would park until the ceiling and tell you nothing.`}
	}
	if err := s.gate("await_signal", key); err != nil {
		return proto.SyncAwaitResult{}, err
	}
	st := s.store()
	if st == nil {
		return proto.SyncAwaitResult{}, errNoSignalStore("await_signal")
	}

	cursor := p.AfterSeq
	if cursor <= 0 {
		// No cursor means "from now on", so the caller must not be handed the
		// backlog it never asked about. The high-water mark is where now is.
		newest, err := st.SignalHighWater()
		if err != nil {
			return proto.SyncAwaitResult{}, &proto.RPCError{Code: proto.CodeInternal,
				Message: "could not read the signal cursor: " + err.Error()}
		}
		cursor = newest
	}

	// The catch-up read, before parking: a waiter that was busy while the signal it
	// needs fired must not wait for a second one.
	if out, rpcErr, done := s.batch(st, topics, cursor); done || rpcErr != nil {
		s.recordReceived(key, out.Signals)
		return out, rpcErr
	}

	w := &signalWaiter{key: key, identity: p.Identity, topics: topics,
		cursor: cursor, since: time.Now(), bell: make(chan struct{}, 1)}
	s.mu.Lock()
	s.waiters[w] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.waiters, w)
		s.mu.Unlock()
		s.pushed()
	}()
	// Registered, so now re-read: a signal that landed between the first read and
	// the registration rang nobody's bell.
	if out, rpcErr, done := s.batch(st, topics, cursor); done || rpcErr != nil {
		s.recordReceived(key, out.Signals)
		return out, rpcErr
	}
	s.log.Info(logging.EvSync, "component", "daemon", "sync", "signal_awaiting",
		"topics", strings.Join(topics, ","), "after_seq", cursor,
		"agent", p.Identity.Agent, "key", key)
	// The board shows this row as listening from here on, which is the state chip
	// the design promised and the reason a parked agent does not look hung.
	s.pushed()

	if waitMax <= 0 {
		waitMax = 25 * time.Minute
	}
	wait := time.Duration(p.TimeoutS) * time.Second
	if wait <= 0 || wait > waitMax {
		wait = waitMax
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()

	for {
		select {
		case <-w.bell:
			// A wake is only ever a hint that the store has something. Read it, and
			// if a trim took it back in between, keep waiting rather than returning an
			// empty batch that reads as "the event happened".
			if out, rpcErr, done := s.batch(st, topics, cursor); done || rpcErr != nil {
				s.recordReceived(key, out.Signals)
				return out, rpcErr
			}

		case <-timer.C:
			out := proto.SyncAwaitResult{OK: true, Cursor: cursor, TimedOut: true}
			out.Note = fmt.Sprintf("nothing on %s within %s. The cursor is unchanged, so calling await_signal again with after_seq %d misses nothing that happens in between.",
				strings.Join(topics, ", "), wait, cursor)
			out.Gap, out.OldestSeq = s.gapAt(st, topics, cursor)
			return out, nil

		case <-ctx.Done():
			// The caller went away. Nothing to clean up but the registration, which
			// the defer does.
			return proto.SyncAwaitResult{OK: true, Cursor: cursor}, nil
		}
	}
}

// batch reads everything matching above the cursor. The third return says the
// answer is ready: false means nothing matched and the caller should park.
func (s *signals) batch(st signalStore, topics []string, cursor int64) (proto.SyncAwaitResult, *proto.RPCError, bool) {
	// Zero lets the store apply its own cap, which keeps the batch size one
	// decision in one place rather than a number this file could drift from.
	sigs, more, err := st.SignalsSince(cursor, topics, 0)
	if err != nil {
		return proto.SyncAwaitResult{}, &proto.RPCError{Code: proto.CodeInternal,
			Message: "could not read signals: " + err.Error()}, true
	}
	if len(sigs) == 0 {
		return proto.SyncAwaitResult{}, nil, false
	}
	out := proto.SyncAwaitResult{OK: true, Signals: sigs, More: more,
		Cursor: sigs[len(sigs)-1].Seq}
	out.Gap, out.OldestSeq = s.gapAt(st, topics, cursor)
	switch {
	case out.Gap:
		out.Note = fmt.Sprintf("retention has taken signals on these topics above your cursor %d, so this batch cannot be complete - a read is whole again only from %d. Treat anything you were tracking by these signals as unknown rather than as not having happened.", cursor, out.OldestSeq)
	case more:
		out.Note = fmt.Sprintf("capped at %d signals; more are already waiting. Call await_signal again with after_seq %d and the next batch comes back without parking.", len(sigs), out.Cursor)
	}
	return out, nil, true
}

// gapAt answers whether retention took anything on THESE topics after this
// cursor, and from which sequence a read of them is complete again.
//
// The question is per topic and it is answered from a recorded watermark, not
// deduced from what survived. The deduction that reads as obvious - "the cursor is
// below the oldest surviving signal" - is what shipped first and it was wrong:
// retention is per topic, so one quiet topic's ancient row holds the global
// minimum down while the topic the caller actually asked about is trimmed out from
// under it. A live run found that inside the hour, which is the whole argument for
// running the thing rather than reading the diff.
func (s *signals) gapAt(st signalStore, topics []string, cursor int64) (bool, int64) {
	if cursor <= 0 {
		return false, 0
	}
	trimmedTo, err := st.SignalGap(cursor, topics)
	if err != nil || trimmedTo <= cursor {
		return false, 0
	}
	// One past the highest sequence that went: from here, a read of these topics is
	// complete.
	return true, trimmedTo + 1
}

// deliver rings the bell of every waiter whose pattern matches, and answers how
// many that was. Fan-out: one signal, every matching listener.
func (s *signals) deliver(sig proto.Signal) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	woken := 0
	for w := range s.waiters {
		if w.cursor >= sig.Seq {
			continue
		}
		if !proto.TopicsMatch(w.topics, sig.Topic) {
			continue
		}
		select {
		case w.bell <- struct{}{}:
		default:
			// Already ringing. One wake is enough: what the waiter does next is read
			// the store from its cursor, which picks up this signal and any other that
			// arrived while the bell was full.
		}
		woken++
	}
	return woken
}

// listens is the roster's observer: what each session is parked on. A session
// with two parked waits (rare, but two overlapping tool calls can do it) reports
// both topics under the age of the older one, because what the human wants from
// the chip is how long this agent has been listening.
func (s *signals) listens() map[string]proto.SyncListen {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]proto.SyncListen{}
	for w := range s.waiters {
		if w.key == "" {
			continue
		}
		have, seen := out[w.key]
		if !seen {
			out[w.key] = proto.SyncListen{Topics: append([]string(nil), w.topics...),
				SinceMS: now.Sub(w.since).Milliseconds()}
			continue
		}
		for _, t := range w.topics {
			if !slices.Contains(have.Topics, t) {
				have.Topics = append(have.Topics, t)
			}
		}
		have.SinceMS = max(have.SinceMS, now.Sub(w.since).Milliseconds())
		out[w.key] = have
	}
	for key, l := range out {
		slices.Sort(l.Topics)
		out[key] = l
	}
	return out
}

// Start runs retention's clock; Stop ends it. A subsystem with no tick still
// delivers every signal correctly - it only stops noticing that a week has
// passed, which is the one kind of trimming no verb can cause.
func (s *signals) Start() {
	s.mu.Lock()
	if s.stop != nil {
		s.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	s.stop = stop
	s.mu.Unlock()

	go func() {
		t := time.NewTicker(signalTrimEvery)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				safely(s.log, "signals.trim", func() { s.trim(time.Duration(s.keepDaysRead()) * 24 * time.Hour) })()
			}
		}
	}()
}

func (s *signals) Stop() {
	s.mu.Lock()
	if s.stop != nil {
		close(s.stop)
		s.stop = nil
	}
	s.mu.Unlock()
}

// trimTopic caps one topic's history, which is what a post owes: it is the only
// topic whose count can have changed.
func (s *signals) trimTopic(topic string) {
	st := s.store()
	if st == nil {
		return
	}
	s.mu.Lock()
	keep := s.keep
	s.mu.Unlock()
	n, err := st.TrimTopic(topic, keep)
	if err != nil {
		s.log.Warn(logging.EvSync, "component", "daemon", "sync", "signal_trim_failed",
			"topic", topic, "error", err.Error())
		return
	}
	if n > 0 {
		s.log.Info(logging.EvSync, "component", "daemon", "sync", "signals_trimmed",
			"topic", topic, "rows", n)
	}
}

// trim applies retention across every topic: the sweep by age, plus the per-topic
// count as a backstop for anything trimTopic missed. The tick's job, because
// nothing a caller does makes a signal older.
func (s *signals) trim(maxAge time.Duration) {
	st := s.store()
	if st == nil {
		return
	}
	s.mu.Lock()
	keep := s.keep
	s.mu.Unlock()
	n, err := st.TrimSignals(keep, maxAge)
	if err != nil {
		s.log.Warn(logging.EvSync, "component", "daemon", "sync", "signal_trim_failed", "error", err.Error())
		return
	}
	if n > 0 {
		s.log.Info(logging.EvSync, "component", "daemon", "sync", "signals_trimmed", "rows", n)
	}
}

func (s *signals) keepDaysRead() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.keepDays
}

// gate is the announce door, the same one the locks use: the sync verbs refuse a
// session that has not said what it is for, while presence and the monitoring
// reads never refuse. Posting a signal the human cannot attribute to a purpose is
// a coordination he cannot follow when it goes wrong.
func (s *signals) gate(tool, key string) *proto.RPCError {
	if key == "" {
		return &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: tool + " needs identity.key: one session, one key. The mcp child mints it; a CLI caller passes --key to act on behalf of a session."}
	}
	s.mu.Lock()
	announced := s.announcedFn
	s.mu.Unlock()
	if announced != nil && !announced(key) {
		return &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: "announce first: call announce with what this session is FOR, then use signals. A message the human cannot attribute to a purpose is one he cannot judge when it goes wrong."}
	}
	return nil
}

func (s *signals) store() signalStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st
}

func (s *signals) pushed() {
	s.mu.Lock()
	changed := s.changed
	s.mu.Unlock()
	if changed != nil {
		changed()
	}
}

// errNoSignalStore is what a daemon with no database says. Durable pickup IS the
// feature, so this refuses rather than falling back to an in-memory hub that
// would work until the first restart and then lose a hand-off silently.
func errNoSignalStore(tool string) *proto.RPCError {
	return &proto.RPCError{Code: proto.CodeInternal,
		Message: tool + ": this daemon has no database, so signals cannot be stored or replayed. Coordinate through a lock or the roster instead."}
}

// cleanTopics drops blanks and duplicates while keeping the caller's order, so a
// list that came out of a template with a hole in it does not silently become a
// wait on nothing.
func cleanTopics(in []string) []string {
	var out []string
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" || slices.Contains(out, t) {
			continue
		}
		out = append(out, t)
	}
	return out
}
