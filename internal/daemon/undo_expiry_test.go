package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
)

// R-03. The expiry can fire and do nothing: the human answered inside the undo
// grace, so resolve() bounces off it. The handler then fell through to a bare
// `return <-wait, nil` - no deadline, no ctx branch - and stayed there.
//
// Two things followed, and they are separate defects with one cause:
//
//   - the one-shot timer was spent, so a subsequent UNDO put the item back
//     pending with no expiry at all, and the timeout_s the agent asked for had
//     quietly become unbounded;
//   - having left the select, the handler could not see its own caller drop, so
//     callerGone was never called and the card went on showing a live caller for
//     an agent that had gone.
//
// The second is the one that costs the human something: they spend a decision on
// a question nobody is left to read.

// answerThenUndoPastTheDeadline drives the exact sequence R-03 describes and
// leaves the item pending with its original window long gone.
func answerThenUndoPastTheDeadline(t *testing.T, d *Daemon, ui *fakeUI, ch chan proto.Result) *proto.Item {
	t.Helper()
	shown := waitForItem(t, ui)

	// Answer at 90% of the one-second window, so the grace outlives the expiry.
	time.Sleep(900 * time.Millisecond)
	d.Answer(shown.ID, "Yes")

	// Let the expiry fire and bounce off the grace.
	time.Sleep(250 * time.Millisecond)
	if msg := d.Undo(shown.ID); msg != "" {
		t.Fatalf("undo inside the grace refused: %q", msg)
	}
	// Nothing may have been delivered: the undo took the answer back.
	select {
	case res := <-ch:
		t.Fatalf("a result arrived after undo: %+v", res)
	case <-time.After(50 * time.Millisecond):
	}
	return shown
}

// The half that costs the human a decision.
func TestUndoPastTheDeadlineStillNoticesTheCallerGoing(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{
		UndoGrace: 400 * time.Millisecond, CallerGone: 10 * time.Second,
	})
	it := askItem()
	it.TimeoutS = 1
	it.Default = "No"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := askAsyncCtx(t, d, it, ctx)
	answerThenUndoPastTheDeadline(t, d, ui, ch)

	if c := ui.last().Caller; c != CallerLive {
		t.Fatalf("caller reads %v before the drop; the test is not set up", c)
	}

	// The caller's socket drops. The card must stop claiming somebody is waiting.
	cancel()
	deadline := time.Now().Add(3 * time.Second)
	for !ui.sawCaller(CallerGone) {
		if time.Now().After(deadline) {
			t.Fatal("the card still shows a live caller after the caller went: " +
				"the handler left its select when the expiry bounced off the undo grace")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The half that breaks the agent's own promise.
func TestUndoPastTheDeadlineRearmsTheExpiry(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{UndoGrace: 400 * time.Millisecond})
	it := askItem()
	it.TimeoutS = 1
	it.Default = "No"

	ch := askAsyncCtx(t, d, it, context.Background())
	answerThenUndoPastTheDeadline(t, d, ui, ch)

	// The question is the human's again, and the wait must still be bounded: a
	// fresh window from the undo, not none at all.
	select {
	case res := <-ch:
		if res.Answered || res.Answer != "No" || !res.DefaultApplied {
			t.Fatalf("result = %+v, want the timeout default applied on the re-armed window", res)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("nothing was delivered after the undo: the timer had been spent, " +
			"so timeout_s became unbounded and the agent waits forever")
	}
}

// The re-armed window must also be the one the CARD counts down, or the human
// watches a deadline that has already passed while the real one runs elsewhere.
func TestUndoPastTheDeadlineMovesTheCountdownToo(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{UndoGrace: 400 * time.Millisecond})
	it := askItem()
	it.TimeoutS = 1
	it.Default = "No"

	ch := askAsyncCtx(t, d, it, context.Background())
	shown := answerThenUndoPastTheDeadline(t, d, ui, ch)
	defer func() { <-ch }()

	d.mu.Lock()
	exp := d.expiries[shown.ID]
	d.mu.Unlock()
	if exp.Before(time.Now()) {
		t.Errorf("the card's countdown still shows %v, which is in the past: "+
			"the expiry was re-armed for the wait but not for the screen", exp)
	}
}

// The existing behaviour this must not disturb: an answer that rides its grace
// out uninterrupted is still the answer, and the expiry never steals it.
func TestGracedAnswerStillWinsAfterTheRearm(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{UndoGrace: 300 * time.Millisecond})
	it := askItem()
	it.TimeoutS = 1
	it.Default = "No"

	ch := askAsyncCtx(t, d, it, context.Background())
	shown := waitForItem(t, ui)
	time.Sleep(900 * time.Millisecond)
	d.Answer(shown.ID, "Yes")

	select {
	case res := <-ch:
		if !res.Answered || res.Answer != "Yes" || res.DefaultApplied {
			t.Fatalf("result = %+v, want the human's answer rather than the default", res)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the graced answer was never delivered")
	}
}

// R-08. The undo grace holds its outcome in MEMORY until finalizeGrace runs, and
// the default window is three seconds - which a deploy fits inside comfortably.
// A daemon that stopped in there lost the answer: the item was still pending, so
// it came back after the restart and the human was asked a second time, into a
// socket whose caller was long gone.
func TestShutdownShipsAnAnswerSittingInItsUndoWindow(t *testing.T) {
	// A grace long enough that nothing finalizes it on its own: the shutdown has
	// to be what ships it, or the test proves only that timers work.
	d, ui, _, st := newTestDaemon(t, Config{UndoGrace: 30 * time.Second})
	ch := askAsyncCtx(t, d, askItem(), context.Background())
	shown := waitForItem(t, ui)

	d.Answer(shown.ID, "Yes")
	// Still in its window, so nothing is in the store yet.
	waitForState(t, st, shown.ID, store.StatePending)

	d.BeginShutdown()

	select {
	case res := <-ch:
		if !res.Answered || res.Answer != "Yes" {
			t.Fatalf("result = %+v, want the answer the human had already given", res)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the answer was dropped by shutdown: the item comes back pending and " +
			"the human is asked a second time, into a caller that is gone")
	}
	waitForState(t, st, shown.ID, store.StateAnswered)
}

// Shutdown with nothing in a grace must stay harmless, and must not invent an
// answer for a question nobody has answered.
func TestShutdownWithNothingGracedIsQuiet(t *testing.T) {
	d, ui, _, st := newTestDaemon(t, Config{UndoGrace: 30 * time.Second})
	ctx := t.Context()
	askAsyncCtx(t, d, askItem(), ctx)
	shown := waitForItem(t, ui)

	// BeginShutdown only flags the teardown and ships a grace; releasing parked
	// callers is the server context's job, which is why nothing is awaited here.
	d.BeginShutdown()

	items, err := st.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.ID == shown.ID && it.State == store.StateAnswered {
			t.Fatalf("shutdown recorded an answer for a question nobody answered: %+v", it)
		}
	}
}
