package daemon

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
)

// newTestLocks is a lock table with every session announced, nobody asking, and
// a liveness probe the test drives itself: an orphan's release depends on a pid
// dying, and a test that had to kill a real process to check that would be
// timing-dependent for no reason.
func newTestLocks() (*locks, *lockProbe) {
	l := newLocks(slog.New(slog.NewTextHandler(io.Discard, nil)))
	p := &lockProbe{dead: map[int]bool{}}
	l.SetObservers(func(string) bool { return true }, nil, nil, p.warn, nil)
	l.alive = p.aliveFn
	l.grace = 0
	return l, p
}

type lockProbe struct {
	mu    sync.Mutex
	dead  map[int]bool
	warns []string
}

func (p *lockProbe) aliveFn(pid int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return pid > 0 && !p.dead[pid]
}

func (p *lockProbe) kill(pid int) {
	p.mu.Lock()
	p.dead[pid] = true
	p.mu.Unlock()
}

func (p *lockProbe) warn(title, body string) {
	p.mu.Lock()
	p.warns = append(p.warns, title+": "+body)
	p.mu.Unlock()
}

func (p *lockProbe) warned() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.warns...)
}

func lockAsk(name, key string) proto.SyncLockParams {
	return proto.SyncLockParams{Identity: id("claude", "agentbox", key), Name: name}
}

// acquireIn starts an acquire in the background and hands back a channel with its
// result, so a test can watch a queue from the outside.
func acquireIn(l *locks, p proto.SyncLockParams, waitMax time.Duration) <-chan proto.SyncLockResult {
	out := make(chan proto.SyncLockResult, 1)
	go func() {
		res, _ := l.Acquire(context.Background(), p, waitMax)
		out <- res
	}()
	return out
}

// waitForQueue blocks until n waiters are parked on name, so a test never races
// the goroutine it just started.
func waitForQueue(t *testing.T, l *locks, name string, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		l.mu.Lock()
		got := len(l.queue[name])
		l.mu.Unlock()
		if got == n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected %d waiters on %s, have %d", n, name, got)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestTwoAgentsRacingOneLockSerialize(t *testing.T) {
	// The whole point of the feature: the second agent waits instead of running
	// the deploy at the same time, and it is granted the moment the first is done.
	l, _ := newTestLocks()

	first, _ := l.Acquire(context.Background(), lockAsk("deploy:agentbox", "k1"), time.Minute)
	if !first.Granted {
		t.Fatalf("the first acquire was not granted: %+v", first)
	}

	second := acquireIn(l, lockAsk("deploy:agentbox", "k2"), time.Minute)
	waitForQueue(t, l, "deploy:agentbox", 1)
	select {
	case res := <-second:
		t.Fatalf("the second agent was granted a held lock: %+v", res)
	case <-time.After(50 * time.Millisecond):
	}

	if _, err := l.Release(lockAsk("deploy:agentbox", "k1")); err != nil {
		t.Fatalf("release: %v", err)
	}
	select {
	case res := <-second:
		if !res.Granted {
			t.Fatalf("the queued agent was not granted after the release: %+v", res)
		}
		if res.Reason != LockReasonReleased {
			t.Errorf("a waiter must learn WHY it won; got reason %q", res.Reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the queued agent was never granted the lock")
	}
}

func TestTheQueueIsFirstInFirstOut(t *testing.T) {
	l, _ := newTestLocks()
	l.Acquire(context.Background(), lockAsk("repo:agentbox", "k1"), time.Minute)

	second := acquireIn(l, lockAsk("repo:agentbox", "k2"), time.Minute)
	waitForQueue(t, l, "repo:agentbox", 1)
	third := acquireIn(l, lockAsk("repo:agentbox", "k3"), time.Minute)
	waitForQueue(t, l, "repo:agentbox", 2)

	l.Release(lockAsk("repo:agentbox", "k1"))
	select {
	case <-second:
	case res := <-third:
		t.Fatalf("the third agent jumped the queue: %+v", res)
	case <-time.After(2 * time.Second):
		t.Fatal("nobody was granted the lock")
	}
	// And the third only after the second is done, not before.
	select {
	case res := <-third:
		t.Fatalf("the third was granted while the second held it: %+v", res)
	case <-time.After(50 * time.Millisecond):
	}
	l.Release(lockAsk("repo:agentbox", "k2"))
	select {
	case res := <-third:
		if !res.Granted {
			t.Fatalf("the third was not granted: %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the third never got the lock")
	}
}

func TestARefusalCarriesTheWholePicture(t *testing.T) {
	// A refusal an agent cannot act on costs a whole model turn to ask the
	// follow-up question. So try_lock has to answer "should I wait?" by itself.
	l, _ := newTestLocks()
	l.SetObservers(func(string) bool { return true },
		func(key string) (proto.SyncAgent, bool) {
			return proto.SyncAgent{Key: key, Agent: "codex", Purpose: "deploying the mirror",
				Activity: "running make deploy", State: StateWorking}, true
		}, nil, nil, nil)

	l.Acquire(context.Background(), proto.SyncLockParams{
		Identity: id("codex", "agentbox", "k1"), Name: "deploy:agentbox", Note: "make deploy",
	}, time.Minute)

	res, err := l.Try(lockAsk("deploy:agentbox", "k2"))
	if err != nil {
		t.Fatalf("try: %v", err)
	}
	if res.Granted || !res.Refused {
		t.Fatalf("try_lock was granted a held lock: %+v", res)
	}
	if res.Holder == nil {
		t.Fatal("the refusal did not say who holds it")
	}
	if res.Holder.Purpose != "deploying the mirror" || res.Holder.Activity != "running make deploy" {
		t.Errorf("the holder's purpose and activity are the point of the picture: %+v", res.Holder)
	}
	if res.HolderNote != "make deploy" {
		t.Errorf("the hold's own note went missing: %q", res.HolderNote)
	}
	if res.HeldMS < 0 {
		t.Errorf("a hold age is part of deciding whether to wait: %d", res.HeldMS)
	}
	if res.Note == "" || !strings.Contains(res.Note, "acquire_lock") {
		t.Errorf("a refusal should teach the way out of it: %q", res.Note)
	}
}

func TestATimeoutIsAResultAndTheQueuePlaceGoesWithIt(t *testing.T) {
	l, _ := newTestLocks()
	l.Acquire(context.Background(), lockAsk("vm:boris-vm", "k1"), time.Minute)

	res, err := l.Acquire(context.Background(),
		proto.SyncLockParams{Identity: id("claude", "agentbox", "k2"), Name: "vm:boris-vm", TimeoutS: 1}, time.Minute)
	if err != nil {
		t.Fatalf("acquire returned an error rather than a result: %v", err)
	}
	if !res.TimedOut || res.Granted {
		t.Fatalf("expected a timed-out result: %+v", res)
	}
	if res.Holder == nil || res.Holder.Key != "k1" {
		t.Errorf("a timeout must still name the holder: %+v", res.Holder)
	}
	if !strings.Contains(res.Note, "re-arm") {
		t.Errorf("the timeout should say that re-arming is one call: %q", res.Note)
	}
	// The place in the queue is gone: the next release must not hand the lock to
	// a call that stopped waiting.
	waitForQueue(t, l, "vm:boris-vm", 0)
}

func TestWaitMaxCapsAPatientCaller(t *testing.T) {
	// timeout_s: 0 means "as long as I wait", and the ceiling is what keeps that
	// honest: the client aborts a silent call, so an unbounded park would end as a
	// transport error the agent cannot read instead of a timeout it can.
	l, _ := newTestLocks()
	l.Acquire(context.Background(), lockAsk("mirror:github", "k1"), time.Minute)

	started := time.Now()
	res, _ := l.Acquire(context.Background(),
		proto.SyncLockParams{Identity: id("claude", "agentbox", "k2"), Name: "mirror:github", TimeoutS: 0},
		300*time.Millisecond)
	if !res.TimedOut {
		t.Fatalf("a patient caller was not capped: %+v", res)
	}
	if took := time.Since(started); took > 2*time.Second {
		t.Errorf("the cap did not apply: waited %s", took)
	}
}

func TestTheVerbsRefuseAnUnannouncedSession(t *testing.T) {
	// The mandate with teeth: taking a shared resource without saying who you are
	// leaves the human a lock he cannot attribute. Reads stay open (that is the
	// roster's rule), but this is a write.
	l, _ := newTestLocks()
	l.SetObservers(func(string) bool { return false }, nil, nil, nil, nil)

	_, err := l.Acquire(context.Background(), lockAsk("deploy:agentbox", "k1"), time.Minute)
	if err == nil {
		t.Fatal("an unannounced session was allowed to take a lock")
	}
	if !strings.Contains(err.Message, "announce") {
		t.Errorf("the refusal must name the way out: %q", err.Message)
	}
	if _, err := l.Try(lockAsk("deploy:agentbox", "k1")); err == nil {
		t.Error("try_lock skipped the gate")
	}
	if _, err := l.Release(lockAsk("deploy:agentbox", "k1")); err == nil {
		t.Error("release_lock skipped the gate")
	}
}

func TestADeadChildDoesNotFreeALockItsWorkStillNeeds(t *testing.T) {
	// The failure this rule exists to prevent: the session's mcp child dies while
	// the `make deploy` it started keeps running, and the next agent is handed the
	// deploy lock in the middle of somebody else's deploy.
	l, probe := newTestLocks()
	l.Acquire(context.Background(), proto.SyncLockParams{
		Identity: id("claude", "agentbox", "k1"), Name: "deploy:agentbox", PID: 4242,
	}, time.Minute)

	waiter := acquireIn(l, lockAsk("deploy:agentbox", "k2"), 5*time.Second)
	waitForQueue(t, l, "deploy:agentbox", 1)

	l.SessionGone("k1")
	l.tick()
	select {
	case res := <-waiter:
		t.Fatalf("the lock was handed over while the holder's process was still running: %+v", res)
	case <-time.After(50 * time.Millisecond):
	}
	// And the roster can see WHY nothing is happening.
	holds, _ := l.rows()
	if len(holds["k1"]) != 1 || !holds["k1"][0].Orphaned {
		t.Fatalf("the hold should read as orphaned: %+v", holds["k1"])
	}

	probe.kill(4242)
	l.tick()
	select {
	case res := <-waiter:
		if !res.Granted {
			t.Fatalf("not granted after the holder's process died: %+v", res)
		}
		if res.Reason != LockReasonHolderGone {
			t.Errorf("the waiter should learn the holder was gone, got %q", res.Reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the lock was never freed after its recorded pid died")
	}
}

func TestReleaseOnDetachIsForWhenTheSessionIsTheCriticalSection(t *testing.T) {
	l, _ := newTestLocks()
	l.Acquire(context.Background(), proto.SyncLockParams{
		Identity: id("claude", "agentbox", "k1"), Name: "repo:agentbox",
		PID: 4242, ReleaseOnDetach: true,
	}, time.Minute)
	waiter := acquireIn(l, lockAsk("repo:agentbox", "k2"), 5*time.Second)
	waitForQueue(t, l, "repo:agentbox", 1)

	l.SessionGone("k1") // no pid probe, no grace: the hold asked to end with it
	select {
	case res := <-waiter:
		if !res.Granted {
			t.Fatalf("release_on_detach did not hand the lock over: %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("release_on_detach kept the lock after the session died")
	}
}

func TestADeadlockIsRefusedByNameRatherThanDiscovered(t *testing.T) {
	l, probe := newTestLocks()
	// A holds repo, B holds deploy. B asks for repo and queues. A then asks for
	// deploy, which would close the cycle.
	l.Acquire(context.Background(), lockAsk("repo:agentbox", "kA"), time.Minute)
	l.Acquire(context.Background(), lockAsk("deploy:agentbox", "kB"), time.Minute)
	acquireIn(l, lockAsk("repo:agentbox", "kB"), 5*time.Second)
	waitForQueue(t, l, "repo:agentbox", 1)

	res, err := l.Acquire(context.Background(), lockAsk("deploy:agentbox", "kA"), time.Minute)
	if err != nil {
		t.Fatalf("a deadlock should be a refusal, not an error: %v", err)
	}
	if !res.Refused || res.Granted {
		t.Fatalf("the cycle was not refused: %+v", res)
	}
	// Both locks, and in a sentence the agent can act on rather than a graph dump.
	for _, want := range []string{"deploy:agentbox", "repo:agentbox", "you asked for", "held by you"} {
		if !strings.Contains(res.Deadlock, want) {
			t.Errorf("the refusal must read as an instruction; %q lacks %q", res.Deadlock, want)
		}
	}
	if len(probe.warned()) == 0 {
		t.Error("a deadlock is the one coordination event worth telling the human about")
	}
}

func TestBreakingALockTellsTheAgentItLostIt(t *testing.T) {
	// Breaking reassigns the lock; it does not stop the ex-holder. So the
	// ex-holder has to find out, and it finds out on its next call of any kind -
	// not only if it happens to touch that lock again.
	l, _ := newTestLocks()
	l.Acquire(context.Background(), lockAsk("deploy:agentbox", "k1"), time.Minute)
	waiter := acquireIn(l, lockAsk("deploy:agentbox", "k2"), 5*time.Second)
	waitForQueue(t, l, "deploy:agentbox", 1)

	l.Break("deploy:agentbox")
	select {
	case res := <-waiter:
		if !res.Granted || res.Reason != LockReasonBroken {
			t.Fatalf("the waiter did not get the broken lock: %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("breaking the lock did not hand it to the waiter")
	}
	notices := l.TakeNotices("k1")
	if len(notices) == 0 {
		t.Fatal("the ex-holder was never told")
	}
	if !strings.Contains(notices[0], "NOT stopped") {
		t.Errorf("the notice must be honest about what breaking does not do: %q", notices[0])
	}
	if again := l.TakeNotices("k1"); len(again) != 0 {
		t.Errorf("a notice must be said once, got it twice: %v", again)
	}
}

func TestATTLHoldFreesItself(t *testing.T) {
	// --ttl is the normal path for a wrapped command, not a corner case: an
	// agent's shell kills a foreground call at 120s, so a long command's hold
	// cannot be tied to the call that took it.
	l, _ := newTestLocks()
	l.Acquire(context.Background(), proto.SyncLockParams{
		Identity: id("claude", "agentbox", "k1"), Name: "deploy:agentbox", TTLS: 1,
	}, time.Minute)

	l.mu.Lock()
	l.held["deploy:agentbox"].expires = time.Now().Add(-time.Millisecond)
	l.mu.Unlock()

	l.tick()
	if snap := l.Snapshot(); len(snap) != 0 {
		t.Fatalf("the expired hold is still there: %+v", snap)
	}
	notices := l.TakeNotices("k1")
	if len(notices) == 0 || !strings.Contains(notices[0], "ttl") {
		t.Errorf("the holder should be told its ttl ran out: %v", notices)
	}
}

func TestAGrantThatRacesATimeoutIsNotLeftHeld(t *testing.T) {
	// The nastiest ordering in the file: a release hands the lock to a waiter in
	// the same instant that waiter gives up. Getting this wrong leaves the lock
	// held by a call that has moved on, and nothing ever frees it.
	l, _ := newTestLocks()
	l.Acquire(context.Background(), lockAsk("deploy:agentbox", "k1"), time.Minute)

	w := &lockWaiter{
		name: "deploy:agentbox", key: "k2", identity: id("claude", "agentbox", "k2"),
		since: time.Now(), out: make(chan lockGrant, 1),
	}
	l.mu.Lock()
	l.queue["deploy:agentbox"] = append(l.queue["deploy:agentbox"], w)
	l.mu.Unlock()

	// The release grants it to w...
	l.Release(lockAsk("deploy:agentbox", "k1"))
	// ...and only then does w stop waiting.
	res := l.giveUp(w, "deploy:agentbox", "k2")
	if res.Granted {
		t.Fatalf("giveUp reported a grant to a caller that had gone: %+v", res)
	}
	if snap := l.Snapshot(); len(snap) != 0 {
		t.Fatalf("the lock is still held by a call that stopped waiting: %+v", snap)
	}
	if !strings.Contains(res.Note, "released again") {
		t.Errorf("the result should say what happened: %q", res.Note)
	}
}

func TestALongWaitWarnsTheHumanOnce(t *testing.T) {
	l, probe := newTestLocks()
	l.SetPolicy(10*time.Millisecond, 0)
	l.Acquire(context.Background(), lockAsk("deploy:agentbox", "k1"), time.Minute)
	acquireIn(l, lockAsk("deploy:agentbox", "k2"), 5*time.Second)
	waitForQueue(t, l, "deploy:agentbox", 1)

	time.Sleep(20 * time.Millisecond)
	l.tick()
	l.tick()
	warns := probe.warned()
	if len(warns) != 1 {
		t.Fatalf("expected exactly one warning, got %d: %v", len(warns), warns)
	}
	if !strings.Contains(warns[0], "deploy:agentbox") {
		t.Errorf("the warning must name the lock: %q", warns[0])
	}
}

func TestAHolderParkedOnAHumanAnswerWarnsWithTheChainNamed(t *testing.T) {
	// This cycle cannot be refused - the human's card is already up - so it is the
	// one that gets said out loud, to the only person who can end it.
	l, probe := newTestLocks()
	l.SetObservers(func(string) bool { return true },
		func(key string) (proto.SyncAgent, bool) {
			return proto.SyncAgent{Key: key, Agent: "codex"}, true
		},
		func() map[string]bool { return map[string]bool{"k1": true} }, probe.warn, nil)

	l.Acquire(context.Background(), lockAsk("deploy:agentbox", "k1"), time.Minute)
	acquireIn(l, lockAsk("deploy:agentbox", "k2"), time.Second)
	waitForQueue(t, l, "deploy:agentbox", 1)

	deadline := time.Now().Add(time.Second)
	for len(probe.warned()) == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	warns := probe.warned()
	if len(warns) == 0 {
		t.Fatal("nobody was told that the lock waits on an answer only the human can give")
	}
	if !strings.Contains(warns[0], "answer") {
		t.Errorf("the warning should say what would unblock it: %q", warns[0])
	}
}

func TestRetakingYourOwnLockIsIdempotent(t *testing.T) {
	l, _ := newTestLocks()
	l.Acquire(context.Background(), lockAsk("repo:agentbox", "k1"), time.Minute)
	res, _ := l.Acquire(context.Background(), lockAsk("repo:agentbox", "k1"), time.Minute)
	if !res.Granted {
		t.Fatalf("retaking a lock you hold should be granted, not refused: %+v", res)
	}
	// And one release is enough: no reentrancy counting in v1.
	l.Release(lockAsk("repo:agentbox", "k1"))
	if snap := l.Snapshot(); len(snap) != 0 {
		t.Fatalf("one release did not free it: %+v", snap)
	}
}

func TestOnlyTheHolderMayRelease(t *testing.T) {
	l, _ := newTestLocks()
	l.Acquire(context.Background(), lockAsk("deploy:agentbox", "k1"), time.Minute)
	res, _ := l.Release(lockAsk("deploy:agentbox", "k2"))
	if res.Released {
		t.Fatal("a second agent released somebody else's lock")
	}
	if !strings.Contains(res.Note, "not yours") {
		t.Errorf("the refusal should say why: %q", res.Note)
	}
	if snap := l.Snapshot(); len(snap) != 1 {
		t.Fatalf("the lock should still be held: %+v", snap)
	}
}

// The roster is where the human sees all of this, so a row has to carry the
// hold, the wait and a state that says it is stopped rather than working.
func TestARowShowsWhatItHoldsAndWhatItWaitsFor(t *testing.T) {
	r := newTestRoster()
	l, _ := newTestLocks()
	l.SetObservers(r.announced, r.agentOf, nil, nil, r.changed)
	r.SetLocks(l.rows)
	r.SetOnGone(l.SessionGone)

	stopA := attached(t, r, id("codex", "agentbox", "kA"), t.TempDir(), "repo:agentbox")
	defer stopA()
	stopB := attached(t, r, id("claude", "agentbox", "kB"), t.TempDir(), "repo:agentbox")
	defer stopB()
	r.Announce(proto.SyncAnnounceParams{Identity: id("codex", "agentbox", "kA"), Purpose: "deploying"})
	r.Announce(proto.SyncAnnounceParams{Identity: id("claude", "agentbox", "kB"), Purpose: "docs pass"})

	l.Acquire(context.Background(), lockAsk("deploy:agentbox", "kA"), time.Minute)
	acquireIn(l, lockAsk("deploy:agentbox", "kB"), 5*time.Second)
	waitForQueue(t, l, "deploy:agentbox", 1)

	rows, _ := r.snapshot()
	var a, b proto.SyncAgent
	for _, row := range rows {
		switch row.Key {
		case "kA":
			a = row
		case "kB":
			b = row
		}
	}
	if len(a.Holds) != 1 || a.Holds[0].Name != "deploy:agentbox" {
		t.Errorf("the holder's row does not show the hold: %+v", a.Holds)
	}
	if b.Waiting == nil || b.Waiting.Name != "deploy:agentbox" {
		t.Fatalf("the waiter's row does not show the wait: %+v", b.Waiting)
	}
	if b.Waiting.HolderKey != "kA" {
		t.Errorf("a wait must name the holder so the surface can link to it: %+v", b.Waiting)
	}
	if b.State != StateBlocked {
		t.Errorf("a parked agent must not read as working: state %q", b.State)
	}
	if !strings.Contains(b.Detail, "codex") {
		t.Errorf("blocked without a holder is a puzzle, not a state: %q", b.Detail)
	}
}
