package daemon

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
)

// newTestSignals is the hub over a real store. A fake store would let the two
// halves of the cursor drift apart, and the cursor is the whole contract.
func newTestSignals(t *testing.T) *signals {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "agentbox.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	s := newSignals(slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.SetStore(st)
	// Announced by default: the gate has its own test, and every other test here is
	// about delivery.
	s.SetObservers(func(string) bool { return true }, nil)
	return s
}

func mustPost(t *testing.T, s *signals, key, topic, data string) proto.SyncPostResult {
	t.Helper()
	var raw json.RawMessage
	if data != "" {
		raw = json.RawMessage(data)
	}
	res, rpcErr := s.Post(proto.SyncPostParams{
		Identity: proto.Identity{Agent: "claude", Key: key}, Topic: topic, Data: raw})
	if rpcErr != nil {
		t.Fatalf("post %s: %s", topic, rpcErr.Message)
	}
	return res
}

// awaitInBackground runs an await in the background and hands back a channel with
// its result, so a test can post while the call is parked.
func awaitInBackground(t *testing.T, s *signals, key string, topics []string, after int64, timeoutS int) <-chan proto.SyncAwaitResult {
	t.Helper()
	out := make(chan proto.SyncAwaitResult, 1)
	go func() {
		res, rpcErr := s.Await(context.Background(), proto.SyncAwaitParams{
			Identity: proto.Identity{Agent: "claude", Key: key},
			Topics:   topics, AfterSeq: after, TimeoutS: timeoutS,
		}, 5*time.Second)
		if rpcErr != nil {
			t.Errorf("await: %s", rpcErr.Message)
		}
		out <- res
	}()
	return out
}

// waitParked blocks until the hub has n registered waiters, so a test posts into a
// hub that is actually listening rather than racing its own goroutine.
func waitParked(t *testing.T, s *signals, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		s.mu.Lock()
		have := len(s.waiters)
		s.mu.Unlock()
		if have >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d waiters parked", have, n)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestAwaitWakesOnAMatchingPost(t *testing.T) {
	s := newTestSignals(t)
	got := awaitInBackground(t, s, "waiter", []string{"tests:green"}, 0, 5)
	waitParked(t, s, 1)

	res := mustPost(t, s, "poster", "tests:green", `{"suite":"race"}`)
	if res.Delivered != 1 {
		t.Fatalf("one parked waiter should have been woken, delivered=%d", res.Delivered)
	}

	select {
	case out := <-got:
		if len(out.Signals) != 1 || out.Signals[0].Topic != "tests:green" {
			t.Fatalf("want one tests:green signal, got %+v", out.Signals)
		}
		if out.Cursor != out.Signals[0].Seq {
			t.Fatalf("the cursor should be the last delivered seq, got %d", out.Cursor)
		}
		if out.TimedOut {
			t.Fatal("a delivered signal is not a timeout")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the parked await never woke")
	}
}

// Fan-out is the one genuinely new behaviour in this hub: a signal is a broadcast
// by meaning, so two listeners on one topic both get it.
func TestAwaitFansOutToEveryMatchingWaiter(t *testing.T) {
	s := newTestSignals(t)
	first := awaitInBackground(t, s, "a", []string{"tests:green"}, 0, 5)
	second := awaitInBackground(t, s, "b", []string{"tests:*"}, 0, 5)
	waitParked(t, s, 2)

	res := mustPost(t, s, "poster", "tests:green", "")
	if res.Delivered != 2 {
		t.Fatalf("both waiters should have woken, delivered=%d", res.Delivered)
	}
	for i, ch := range []<-chan proto.SyncAwaitResult{first, second} {
		select {
		case out := <-ch:
			if len(out.Signals) != 1 {
				t.Fatalf("waiter %d got %d signals", i, len(out.Signals))
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("waiter %d never woke", i)
		}
	}
}

// The durability claim: a signal posted with nobody listening is picked up later by
// cursor. This is what makes "run the deploy when the tests are green" survive the
// two halves not overlapping.
func TestAwaitCatchesUpFromACursorWithoutParking(t *testing.T) {
	s := newTestSignals(t)
	mustPost(t, s, "poster", "done:one", "")
	mustPost(t, s, "poster", "done:two", "")

	// A one-second timeout would expire if this parked; it must return at once.
	res, rpcErr := s.Await(context.Background(), proto.SyncAwaitParams{
		Identity: proto.Identity{Agent: "claude", Key: "late"},
		Topics:   []string{"done:*"}, AfterSeq: 0, TimeoutS: 1,
	}, 5*time.Second)
	if rpcErr != nil {
		t.Fatalf("await: %s", rpcErr.Message)
	}
	// AfterSeq 0 means "from now on", so the backlog is deliberately NOT delivered.
	if !res.TimedOut || len(res.Signals) != 0 {
		t.Fatalf("after_seq 0 must mean from now on, got %+v", res)
	}

	res, rpcErr = s.Await(context.Background(), proto.SyncAwaitParams{
		Identity: proto.Identity{Agent: "claude", Key: "late"},
		Topics:   []string{"done:*"}, AfterSeq: 1, TimeoutS: 1,
	}, 5*time.Second)
	if rpcErr != nil {
		t.Fatalf("await: %s", rpcErr.Message)
	}
	if len(res.Signals) != 1 || res.Signals[0].Topic != "done:two" {
		t.Fatalf("a cursor of 1 should deliver the second signal at once, got %+v", res.Signals)
	}
}

// Three signals that fired while an agent was busy come back as ONE batch, which
// is the whole argument for a cursor over a callback: catching up costs one call
// rather than one per event.
func TestAwaitBatchesEverythingSinceTheCursor(t *testing.T) {
	s := newTestSignals(t)
	first := mustPost(t, s, "poster", "done:x", `{"chunk":1}`)
	mustPost(t, s, "poster", "done:x", `{"chunk":2}`)
	mustPost(t, s, "poster", "other", "")
	mustPost(t, s, "poster", "done:x", `{"chunk":3}`)

	res, rpcErr := s.Await(context.Background(), proto.SyncAwaitParams{
		Identity: proto.Identity{Agent: "claude", Key: "busy"},
		Topics:   []string{"done:x"}, AfterSeq: first.Seq, TimeoutS: 1,
	}, 5*time.Second)
	if rpcErr != nil {
		t.Fatalf("await: %s", rpcErr.Message)
	}
	if len(res.Signals) != 2 {
		t.Fatalf("want the two matching signals after the cursor in one batch, got %+v", res.Signals)
	}
	if res.TimedOut {
		t.Fatal("a batch that came back at once is not a timeout")
	}
	// The unmatched topic in between must not be in the batch, and must not stop the
	// cursor advancing past it.
	for _, sig := range res.Signals {
		if sig.Topic != "done:x" {
			t.Fatalf("an unmatched topic leaked into the batch: %+v", sig)
		}
	}
	if res.Cursor != 4 {
		t.Fatalf("the cursor should be the last delivered seq, got %d", res.Cursor)
	}
}

func TestAwaitTimeoutIsAResultWithTheCursorIntact(t *testing.T) {
	s := newTestSignals(t)
	mustPost(t, s, "poster", "other:topic", "")

	res, rpcErr := s.Await(context.Background(), proto.SyncAwaitParams{
		Identity: proto.Identity{Agent: "claude", Key: "waiter"},
		Topics:   []string{"tests:green"}, AfterSeq: 1, TimeoutS: 1,
	}, 5*time.Second)
	if rpcErr != nil {
		t.Fatalf("a timeout must not be an error: %s", rpcErr.Message)
	}
	if !res.TimedOut || !res.OK {
		t.Fatalf("want an ok timeout, got %+v", res)
	}
	if res.Cursor != 1 {
		t.Fatalf("the cursor must come back unchanged so re-arming misses nothing, got %d", res.Cursor)
	}
	if !strings.Contains(res.Note, "after_seq 1") {
		t.Fatalf("the note should tell the caller how to re-arm, got %q", res.Note)
	}
}

// The gap is the reason retention can be finite. A cursor below the oldest
// surviving signal must be told, or an agent reads a batch with a hole in it as a
// complete history.
func TestAwaitReportsAGapWhenTheCursorFellOffTheEdge(t *testing.T) {
	s := newTestSignals(t)
	for range 4 {
		mustPost(t, s, "poster", "done:x", "")
	}
	s.SetRetention(1, 0)
	s.trim(0)

	res, rpcErr := s.Await(context.Background(), proto.SyncAwaitParams{
		Identity: proto.Identity{Agent: "claude", Key: "stale"},
		Topics:   []string{"done:x"}, AfterSeq: 1, TimeoutS: 1,
	}, 5*time.Second)
	if rpcErr != nil {
		t.Fatalf("await: %s", rpcErr.Message)
	}
	if !res.Gap {
		t.Fatalf("a cursor of 1 against a table starting at 4 is a gap, got %+v", res)
	}
	if res.OldestSeq != 4 {
		t.Fatalf("the gap should name the oldest survivor, got %d", res.OldestSeq)
	}
	if !strings.Contains(res.Note, "cannot be complete") {
		t.Fatalf("the note must say the batch is incomplete, got %q", res.Note)
	}
	// A cursor that has NOT fallen off the edge is not a gap, however much was
	// trimmed before it.
	res, rpcErr = s.Await(context.Background(), proto.SyncAwaitParams{
		Identity: proto.Identity{Agent: "claude", Key: "fresh"},
		Topics:   []string{"done:x"}, AfterSeq: 4, TimeoutS: 1,
	}, 5*time.Second)
	if rpcErr != nil {
		t.Fatalf("await: %s", rpcErr.Message)
	}
	if res.Gap {
		t.Fatalf("a cursor at the oldest surviving signal is not a gap: %+v", res)
	}
}

func TestSignalVerbsRefuseAnUnannouncedSession(t *testing.T) {
	s := newTestSignals(t)
	s.SetObservers(func(string) bool { return false }, nil)

	_, rpcErr := s.Post(proto.SyncPostParams{
		Identity: proto.Identity{Agent: "rude", Key: "k"}, Topic: "tests:green"})
	if rpcErr == nil || !strings.Contains(rpcErr.Message, "announce first") {
		t.Fatalf("post should teach the gate, got %v", rpcErr)
	}
	_, rpcErr = s.Await(context.Background(), proto.SyncAwaitParams{
		Identity: proto.Identity{Agent: "rude", Key: "k"}, Topics: []string{"tests:green"}}, time.Second)
	if rpcErr == nil || !strings.Contains(rpcErr.Message, "announce first") {
		t.Fatalf("await should teach the gate, got %v", rpcErr)
	}
}

func TestPostRefusesAPatternAsATopic(t *testing.T) {
	s := newTestSignals(t)
	_, rpcErr := s.Post(proto.SyncPostParams{
		Identity: proto.Identity{Agent: "claude", Key: "k"}, Topic: "done:*"})
	if rpcErr == nil || !strings.Contains(rpcErr.Message, "exact topic") {
		t.Fatalf("a wildcard is something you wait on, not post to: %v", rpcErr)
	}
}

func TestPostRefusesAnOversizedPayload(t *testing.T) {
	s := newTestSignals(t)
	big := make([]byte, signalDataMax+1)
	for i := range big {
		big[i] = 'x'
	}
	_, rpcErr := s.Post(proto.SyncPostParams{
		Identity: proto.Identity{Agent: "claude", Key: "k"}, Topic: "t",
		Data: json.RawMessage(big)})
	if rpcErr == nil || !strings.Contains(rpcErr.Message, "cap") {
		t.Fatalf("an oversized payload should be refused with the cap named: %v", rpcErr)
	}
}

func TestAwaitNeedsATopic(t *testing.T) {
	s := newTestSignals(t)
	_, rpcErr := s.Await(context.Background(), proto.SyncAwaitParams{
		Identity: proto.Identity{Agent: "claude", Key: "k"}, Topics: []string{" ", ""}}, time.Second)
	if rpcErr == nil || !strings.Contains(rpcErr.Message, "at least one topic") {
		t.Fatalf("waiting on nothing must be refused rather than parked: %v", rpcErr)
	}
}

// A post with nobody listening says so, because fire-and-forget means zero is a
// real answer and a poster that cares should not have to assume.
func TestPostWithNoListenerSaysSo(t *testing.T) {
	s := newTestSignals(t)
	res := mustPost(t, s, "poster", "tests:green", "")
	if res.Delivered != 0 {
		t.Fatalf("nobody was parked, delivered=%d", res.Delivered)
	}
	if !strings.Contains(res.Note, "not a failure") {
		t.Fatalf("the note should say a stored signal is still delivered, got %q", res.Note)
	}
}

// The roster's listening chip is fed from here, and it has to appear while the call
// is parked and disappear when it ends - otherwise a board says an agent is
// listening long after it stopped.
func TestListensReportsParkedWaitersOnly(t *testing.T) {
	s := newTestSignals(t)
	if len(s.listens()) != 0 {
		t.Fatal("nothing is parked yet")
	}
	done := awaitInBackground(t, s, "listener", []string{"tests:green", "done:*"}, 0, 5)
	waitParked(t, s, 1)

	l := s.listens()
	got, ok := l["listener"]
	if !ok {
		t.Fatalf("the parked session should be listed, got %+v", l)
	}
	if len(got.Topics) != 2 || got.Topics[0] != "done:*" {
		t.Fatalf("topics should be sorted for a stable chip, got %+v", got.Topics)
	}
	mustPost(t, s, "poster", "tests:green", "")
	<-done

	deadline := time.Now().Add(2 * time.Second)
	for len(s.listens()) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("a finished await should stop being reported as listening")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// The state chip itself: listening outranks working, and it names the topics.
func TestDerivedStateListening(t *testing.T) {
	row := &rosterRow{announced: true, attached: true, activity: "editing", activityAt: time.Now()}
	listen := &proto.SyncListen{Topics: []string{"tests:green"}}
	state, detail := derivedState(row, "k", nil, "", nil, listen, time.Now())
	if state != StateListening || detail != "tests:green" {
		t.Fatalf("want listening on the topic, got %q / %q", state, detail)
	}
	// But a lock wait outranks it: blocked means somebody else is why, and that is
	// the one the human can act on.
	wait := &proto.SyncWait{Name: "deploy:agentbox"}
	state, _ = derivedState(row, "k", nil, "", wait, listen, time.Now())
	if state != StateBlocked {
		t.Fatalf("a lock wait should outrank listening, got %q", state)
	}
}

// An await whose caller goes away must take its registration with it, or the hub
// fans out to a listener that will never read and the board shows a phantom.
func TestAwaitDeregistersWhenTheCallerGoesAway(t *testing.T) {
	s := newTestSignals(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Await(ctx, proto.SyncAwaitParams{
			Identity: proto.Identity{Agent: "claude", Key: "goner"},
			Topics:   []string{"tests:green"},
		}, 5*time.Second)
	}()
	waitParked(t, s, 1)
	cancel()
	<-done

	if len(s.listens()) != 0 {
		t.Fatal("a cancelled await must leave no waiter behind")
	}
	if res := mustPost(t, s, "poster", "tests:green", ""); res.Delivered != 0 {
		t.Fatalf("nobody should be woken, delivered=%d", res.Delivered)
	}
}

// The built-in presence topic: a join, an announce and a departure are each a
// signal, so an agent that is genuinely idle can park on its area instead of
// polling the roster.
func TestRosterPostsPresenceSignals(t *testing.T) {
	r := newTestRoster()
	type posted struct {
		topic string
		data  map[string]any
	}
	seen := make(chan posted, 8)
	r.SetPost(func(topic string, _ proto.Identity, data any) {
		m, _ := data.(map[string]any)
		seen <- posted{topic, m}
	})

	i := id("claude", "agentbox", "k1")
	cancel := attached(t, r, i, "/tmp/x", "repo:agentbox")
	if _, rpcErr := r.Announce(proto.SyncAnnounceParams{
		Identity: i, Purpose: "slice 3", Area: "repo:agentbox"}); rpcErr != nil {
		t.Fatalf("announce: %s", rpcErr.Message)
	}
	cancel()

	want := []string{"join", "announce", "leave"}
	for _, event := range want {
		select {
		case got := <-seen:
			if got.topic != "agents:repo:agentbox" {
				t.Fatalf("presence should post on the area topic, got %q", got.topic)
			}
			if got.data["event"] != event {
				t.Fatalf("want event %q, got %v", event, got.data["event"])
			}
			if got.data["key"] != "k1" {
				t.Fatalf("the payload should name the session, got %v", got.data)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("no %s signal", event)
		}
	}
}

// A lock changing hands is announced on its own topic, so an agent that gave up
// queueing can be told the resource is free instead of parking in acquire again.
func TestLockReleasePostsALockSignal(t *testing.T) {
	l := newLocks(slog.New(slog.NewTextHandler(io.Discard, nil)))
	type posted struct {
		topic string
		data  map[string]any
	}
	seen := make(chan posted, 4)
	l.SetPost(func(topic string, poster proto.Identity, data any) {
		if poster.Agent != "agentbox" {
			t.Errorf("a lock event is the daemon's word, not an agent's: %+v", poster)
		}
		m, _ := data.(map[string]any)
		seen <- posted{topic, m}
	})

	held := proto.SyncLockParams{Identity: id("claude", "agentbox", "holder"), Name: "deploy:agentbox"}
	if res, rpcErr := l.Try(held); rpcErr != nil || !res.Granted {
		t.Fatalf("first acquire should be granted: %+v %v", res, rpcErr)
	}
	if _, rpcErr := l.Release(held); rpcErr != nil {
		t.Fatalf("release: %s", rpcErr.Message)
	}
	select {
	case got := <-seen:
		if got.topic != "lock:deploy:agentbox" {
			t.Fatalf("want the lock's own topic, got %q", got.topic)
		}
		if got.data["reason"] != LockReasonReleased {
			t.Fatalf("the payload should say why, got %v", got.data["reason"])
		}
		if got.data["was_held_by"] != "claude" {
			t.Fatalf("the payload should name the ex-holder, got %v", got.data["was_held_by"])
		}
		if got.data["free"] != true {
			t.Fatalf("with nobody queued the lock is free, got %v", got.data["free"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a release posted no lock signal")
	}
}

// emit is the built-in path: no gate, same delivery. It is what carries
// agents:<area> and lock:NAME.
func TestEmitDeliversWithoutTheGate(t *testing.T) {
	s := newTestSignals(t)
	// The waiter itself needs the gate open to park, so it parks first and the gate
	// shuts behind it: what this test is about is emit not consulting it at all.
	got := awaitInBackground(t, s, "waiter", []string{"agents:repo:agentbox"}, 0, 5)
	waitParked(t, s, 1)
	s.SetObservers(func(string) bool { return false }, nil)

	if err := s.emit("agents:repo:agentbox", proto.Identity{Agent: "agentbox"},
		json.RawMessage(`{"event":"join"}`)); err != nil {
		t.Fatalf("emit: %v", err)
	}
	select {
	case out := <-got:
		if len(out.Signals) != 1 || out.Signals[0].Topic != "agents:repo:agentbox" {
			t.Fatalf("want the built-in signal, got %+v", out.Signals)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("emit did not wake the waiter")
	}
}
