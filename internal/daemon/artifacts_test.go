package daemon

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
)

func artifactDaemon(t *testing.T) *Daemon {
	t.Helper()
	d, _, _, _ := newTestDaemon(t, Config{})
	return d
}

func event(id, name, data string) proto.ArtifactEvent {
	return proto.ArtifactEvent{ArtifactID: id, Name: name, Data: json.RawMessage(data)}
}

func TestArtifactEventWaitsBothWays(t *testing.T) {
	d := artifactDaemon(t)

	// Buffered first, then waited on: an agent that was busy when the human clicked
	// must still hear about the click.
	d.ArtifactEvent(event("a1", "run", `{"rows":500}`))
	res := d.AwaitArtifact(context.Background(), proto.ArtifactWait{ArtifactID: "a1"})
	if !res.Received || res.Event == nil || res.Event.Name != "run" {
		t.Fatalf("buffered event not delivered: %+v", res)
	}
	if string(res.Event.Data) != `{"rows":500}` {
		t.Errorf("data = %s", res.Event.Data)
	}
	if res.Event.AtMS == 0 {
		t.Error("an event should be stamped")
	}

	// Waited on first, then published: the parked call wakes up.
	done := make(chan proto.ArtifactWaitResult, 1)
	go func() {
		done <- d.AwaitArtifact(context.Background(), proto.ArtifactWait{ArtifactID: "a1"})
	}()
	// Wait until the waiter is registered, so this tests delivery rather than the
	// buffer a second time.
	waitFor(t, "a waiter to park", func() bool {
		d.art.mu.Lock()
		defer d.art.mu.Unlock()
		return len(d.art.waiters) == 1
	})
	d.ArtifactEvent(event("a1", "cancel", ""))

	select {
	case res := <-done:
		if !res.Received || res.Event.Name != "cancel" {
			t.Fatalf("live event not delivered: %+v", res)
		}
		if len(res.Event.Data) != 0 {
			t.Errorf("an event with no payload should carry none, got %s", res.Event.Data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a published event never reached the waiting agent")
	}

	// Nothing is left over either way.
	if got := d.ReadArtifact(proto.ArtifactWait{}); len(got.Events) != 0 {
		t.Errorf("buffer should be empty, holds %+v", got.Events)
	}
}

func TestArtifactEventsCoalesceByName(t *testing.T) {
	d := artifactDaemon(t)

	// A dragged slider: many of one name, and only the last value matters.
	for _, rows := range []string{"100", "200", "900"} {
		d.ArtifactEvent(event("a1", "batch", `{"rows":`+rows+`}`))
	}
	d.ArtifactEvent(event("a1", "run", `null`))

	got := d.ReadArtifact(proto.ArtifactWait{ArtifactID: "a1"})
	if len(got.Events) != 2 {
		t.Fatalf("want 2 events after coalescing, got %d: %+v", len(got.Events), got.Events)
	}
	// Order is where each control was first touched, so the slider stays first.
	if got.Events[0].Name != "batch" || string(got.Events[0].Data) != `{"rows":900}` {
		t.Errorf("coalesced slider = %+v", got.Events[0])
	}
	if got.Events[1].Name != "run" {
		t.Errorf("second event = %+v", got.Events[1])
	}
	if left := d.ReadArtifact(proto.ArtifactWait{}); len(left.Events) != 0 {
		t.Error("read should drain what it returns")
	}
}

func TestArtifactWaitFilters(t *testing.T) {
	d := artifactDaemon(t)
	d.ArtifactEvent(event("other", "run", ""))
	d.ArtifactEvent(event("a1", "hover", ""))
	d.ArtifactEvent(event("a1", "run", ""))

	// An id and a name: neither the other artifact's event nor the wrong event.
	res := d.AwaitArtifact(context.Background(), proto.ArtifactWait{
		ArtifactID: "a1", Names: []string{"run"}, TimeoutS: 1,
	})
	if !res.Received || res.Event.ArtifactID != "a1" || res.Event.Name != "run" {
		t.Fatalf("filtered wait = %+v", res)
	}

	// What did not match is still there for whoever wants it.
	left := d.ReadArtifact(proto.ArtifactWait{})
	if len(left.Events) != 2 {
		t.Fatalf("the unmatched events should be untouched, got %+v", left.Events)
	}

	// An empty request hears anything, which is what an agent with one artifact
	// should not have to spell out.
	d.ArtifactEvent(event("whatever", "click", ""))
	if res := d.AwaitArtifact(context.Background(), proto.ArtifactWait{TimeoutS: 1}); !res.Received {
		t.Error("an unfiltered wait should take any event")
	}
}

func TestArtifactWaitTimesOutAndStopsWaiting(t *testing.T) {
	d := artifactDaemon(t)
	start := time.Now()
	res := d.AwaitArtifact(context.Background(), proto.ArtifactWait{ArtifactID: "a1", TimeoutS: 1})
	if res.Received || !res.TimedOut {
		t.Fatalf("want a timeout, got %+v", res)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Errorf("returned after %v, before its own window", elapsed)
	}

	d.art.mu.Lock()
	waiters := len(d.art.waiters)
	d.art.mu.Unlock()
	if waiters != 0 {
		t.Errorf("a timed-out wait left %d waiter(s) behind", waiters)
	}

	// And the click that comes later is buffered rather than handed to a waiter
	// that has gone: the next look finds it.
	d.ArtifactEvent(event("a1", "run", ""))
	if got := d.ReadArtifact(proto.ArtifactWait{}); len(got.Events) != 1 {
		t.Errorf("an event after a timeout should be kept, got %+v", got.Events)
	}
}

func TestArtifactWaitEndsWithItsCaller(t *testing.T) {
	d := artifactDaemon(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan proto.ArtifactWaitResult, 1)
	go func() { done <- d.AwaitArtifact(ctx, proto.ArtifactWait{}) }()
	waitFor(t, "a waiter to park", func() bool {
		d.art.mu.Lock()
		defer d.art.mu.Unlock()
		return len(d.art.waiters) == 1
	})
	cancel()

	select {
	case res := <-done:
		if res.Received || res.TimedOut {
			t.Errorf("an abandoned wait is neither answered nor timed out: %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelling the caller did not end the wait")
	}
	d.art.mu.Lock()
	waiters := len(d.art.waiters)
	d.art.mu.Unlock()
	if waiters != 0 {
		t.Errorf("an abandoned wait left %d waiter(s) behind", waiters)
	}
}

func TestArtifactBufferIsBounded(t *testing.T) {
	d := artifactDaemon(t)
	for i := range artifactBufferMax * 2 {
		d.ArtifactEvent(proto.ArtifactEvent{ArtifactID: "a1", Name: "e" + strconv.Itoa(i)})
	}
	got := d.ReadArtifact(proto.ArtifactWait{})
	if len(got.Events) != artifactBufferMax {
		t.Fatalf("buffer held %d, want the cap of %d", len(got.Events), artifactBufferMax)
	}
	// The oldest go, not the newest: what the human just did matters most.
	if got.Events[len(got.Events)-1].Name != "e"+strconv.Itoa(artifactBufferMax*2-1) {
		t.Errorf("the newest event was dropped: %+v", got.Events[len(got.Events)-1])
	}
}

// An event is one action, so it goes to one agent: two waiters must not both run
// the work behind a single click.
func TestArtifactEventGoesToOneWaiter(t *testing.T) {
	d := artifactDaemon(t)
	first := make(chan proto.ArtifactWaitResult, 1)
	second := make(chan proto.ArtifactWaitResult, 1)
	go func() { first <- d.AwaitArtifact(context.Background(), proto.ArtifactWait{TimeoutS: 2}) }()
	waitFor(t, "a waiter to park", func() bool {
		d.art.mu.Lock()
		defer d.art.mu.Unlock()
		return len(d.art.waiters) == 1
	})
	go func() { second <- d.AwaitArtifact(context.Background(), proto.ArtifactWait{TimeoutS: 2}) }()
	waitFor(t, "both waiters to park", func() bool {
		d.art.mu.Lock()
		defer d.art.mu.Unlock()
		return len(d.art.waiters) == 2
	})

	d.ArtifactEvent(event("a1", "run", ""))

	got := 0
	for _, ch := range []chan proto.ArtifactWaitResult{first, second} {
		select {
		case res := <-ch:
			if res.Received {
				got++
			}
		case <-time.After(3 * time.Second):
			t.Fatal("a waiter never returned")
		}
	}
	if got != 1 {
		t.Errorf("%d waiters received one click", got)
	}
}

func TestArtifactHandlerRoutes(t *testing.T) {
	d := artifactDaemon(t)
	d.ArtifactEvent(event("a1", "run", `{"ok":true}`))

	// An empty body is legal on both methods: any artifact, any event.
	res, rpcErr := d.Handle(context.Background(), proto.MethodArtifactWait, nil)
	if rpcErr != nil {
		t.Fatalf("wait: %v", rpcErr)
	}
	if got, ok := res.(proto.ArtifactWaitResult); !ok || !got.Received {
		t.Fatalf("wait returned %#v", res)
	}

	res, rpcErr = d.Handle(context.Background(), proto.MethodArtifactRead, []byte(`{}`))
	if rpcErr != nil {
		t.Fatalf("read: %v", rpcErr)
	}
	if got, ok := res.(proto.ArtifactReadResult); !ok || len(got.Events) != 0 {
		t.Fatalf("read returned %#v", res)
	}

	// And a malformed body is a parameter error, not a panic.
	if _, rpcErr = d.Handle(context.Background(), proto.MethodArtifactRead, []byte(`"nope"`)); rpcErr == nil {
		t.Error("a malformed artifact_read body should be rejected")
	}
}
