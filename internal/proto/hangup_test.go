package proto

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"
)

// Does a blocking handler learn that its caller went away?
//
// FR83's presence design rests entirely on this: the attach stream is one
// long-lived call whose context IS the session's liveness, so if a handler
// blocked on ctx.Done() never wakes when the peer closes, the roster can never
// drop a dead agent's row. FR45's caller-gone indicator rests on the same
// answer, and its own test cancels the context by hand rather than closing a
// socket, so it does not cover this.
func TestBlockingHandlerLearnsItsCallerHungUp(t *testing.T) {
	client, srv := net.Pipe()

	entered := make(chan struct{})
	cancelled := make(chan struct{})
	h := func(ctx context.Context, method string, params json.RawMessage) (any, *RPCError) {
		close(entered)
		<-ctx.Done()
		close(cancelled)
		return Result{}, nil
	}

	served := make(chan error, 1)
	go func() { served <- NewConn(srv).Serve(context.Background(), h) }()

	if _, err := client.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"agentbox.v1.ask","params":{}}` + "\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the handler was never called")
	}

	// The caller hangs up mid-question, which is a session being killed.
	client.Close()

	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("a blocking handler was NOT told its caller hung up: ctx stayed live " +
			"after the peer closed the socket, so presence keyed to a call's context " +
			"can never expire")
	}
	select {
	case <-served:
	case <-time.After(2 * time.Second):
		t.Fatal("Serve never returned after its peer went away")
	}
}
