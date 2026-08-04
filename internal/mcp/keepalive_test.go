package mcp

import (
	"context"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// parkServer is a server whose one tool blocks until the test lets it go, with
// the keep-alive intervals turned down from minutes to milliseconds. It returns
// the release as a function, and releases in cleanup as well: a t.Fatalf with
// the handler still parked deadlocks the session's Close, so a broken keep-alive
// would hang the suite instead of failing it (it did, once).
func parkServer(t *testing.T, first, every time.Duration, notes chan<- string) (*sdk.ClientSession, func()) {
	t.Helper()
	srv := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	srv.AddReceivingMiddleware(keepaliveMiddlewareEvery(first, every))

	release := make(chan struct{})
	sdk.AddTool(srv, &sdk.Tool{Name: "park", Description: "blocks like a card the human has not answered"},
		func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, struct{}, error) {
			select {
			case <-release:
			case <-ctx.Done():
				return nil, struct{}{}, ctx.Err()
			}
			return &sdk.CallToolResult{
				Content: []sdk.Content{&sdk.TextContent{Text: "answered"}},
			}, struct{}{}, nil
		})

	ct, st := sdk.NewInMemoryTransports()
	go func() { _, _ = srv.Connect(context.Background(), st, nil) }()
	client := sdk.NewClient(&sdk.Implementation{Name: "probe", Version: "0"}, &sdk.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *sdk.ProgressNotificationClientRequest) {
			select {
			case notes <- req.Params.Message:
			default:
			}
		},
	})
	cs, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	var once sync.Once
	stop := func() { once.Do(func() { close(release) }) }
	// Registered after Close, so it runs before it: cleanups are LIFO.
	t.Cleanup(stop)
	return cs, stop
}

// The whole point: a call that parks keeps saying it is alive, so the client's
// idle watchdog (1800s of silence on stdio, measured) never fires and the
// human's answer still has a caller to reach.
func TestAParkedCallKeepsTellingTheClientItIsAlive(t *testing.T) {
	notes := make(chan string, 16)
	cs, release := parkServer(t, 20*time.Millisecond, 20*time.Millisecond, notes)

	done := make(chan *sdk.CallToolResult, 1)
	go func() {
		p := &sdk.CallToolParams{Name: "park"}
		// What Claude Code does on every tools/call: ask for progress. Without a
		// token the spec allows no notification at all.
		p.SetProgressToken("tok-1")
		res, err := cs.CallTool(context.Background(), p)
		if err != nil {
			t.Errorf("call: %v", err)
		}
		done <- res
	}()

	// Two ticks prove a ticker rather than a single reminder.
	for i := range 2 {
		select {
		case msg := <-notes:
			if msg == "" {
				t.Errorf("tick %d carried no message; the client shows it in its status line", i+1)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("no keep-alive tick %d: a parked call would be aborted at the client's idle cap", i+1)
		}
	}

	release()
	select {
	case res := <-done:
		if res == nil || len(res.Content) == 0 {
			t.Fatalf("the answer did not come back: %#v", res)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the call never returned after the handler was released")
	}

	// And the ticking stops with the call, or every answered card would leave a
	// goroutine talking to the client forever.
	for len(notes) > 0 {
		<-notes
	}
	time.Sleep(80 * time.Millisecond)
	if n := len(notes); n != 0 {
		t.Errorf("%d ticks arrived after the call returned", n)
	}
}

func TestAFastCallNeverTicks(t *testing.T) {
	notes := make(chan string, 4)
	// first is longer than this call will take, which is the normal case for
	// nearly every tool: no notification should be sent at all.
	cs, release := parkServer(t, 2*time.Second, 20*time.Millisecond, notes)
	release()

	p := &sdk.CallToolParams{Name: "park"}
	p.SetProgressToken("tok-2")
	if _, err := cs.CallTool(context.Background(), p); err != nil {
		t.Fatalf("call: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	if n := len(notes); n != 0 {
		t.Errorf("a fast call sent %d progress notifications", n)
	}
}

func TestAClientThatAsksForNoProgressGetsNone(t *testing.T) {
	notes := make(chan string, 4)
	cs, release := parkServer(t, 10*time.Millisecond, 10*time.Millisecond, notes)

	done := make(chan struct{})
	go func() {
		defer close(done)
		// No SetProgressToken: a notification tied to no token is a protocol
		// error, so the middleware must stay silent rather than invent one.
		if _, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "park"}); err != nil {
			t.Errorf("call: %v", err)
		}
	}()
	time.Sleep(120 * time.Millisecond)
	if n := len(notes); n != 0 {
		t.Errorf("sent %d progress notifications for a call that asked for none", n)
	}
	release()
	<-done
}
