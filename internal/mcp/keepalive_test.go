package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borismilner/agentbox/internal/proto"
	abserver "github.com/borismilner/agentbox/internal/server"
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

// R-10. The ticker is the only thing holding a parked call up, so a notification
// that cannot be delivered is the call failing - and this used to be `_ =`, with a
// comment saying so and doing nothing about it. The human then answered a card
// whose caller had gone, and both sides believed they had done their part.
//
// This drives keepalive directly rather than through the middleware, and the reason
// is worth writing down: the go-sdk tears its own session down on a failed write
// (measured on v1.6.1 - a parked handler's context was already cancelled before
// this fix existed), so a test that watches the handler passes either way and pins
// nothing agentbox owns. What is asserted here is that the ticker itself reports the
// client as gone, which is the part that must survive a version bump.
func TestAKeepaliveThatCannotBeDeliveredEndsTheCall(t *testing.T) {
	var broken atomic.Bool
	srv := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	ct, st := sdk.NewInMemoryTransports()
	ss, err := srv.Connect(context.Background(), breakableTransport{inner: st, broken: &broken}, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "probe", Version: "0"}, nil)
	cs, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })

	broken.Store(true) // the pipe to the client goes while the call is parked
	gone := make(chan struct{})
	go keepalive(context.Background(), ss, "tok-gone", time.Millisecond, time.Millisecond,
		func() { close(gone) })
	select {
	case <-gone:
	case <-time.After(2 * time.Second):
		t.Fatal("a keepalive that could not be delivered was swallowed, which is R-10")
	}
}

// The dangerous state is the silent one: no progressToken means no ticker, so the
// client's own idle limit runs unopposed. A question that would have waited for
// ever is bounded instead, and ends as an outcome the agent can read (R-09/R-10).
func TestAWaitWithNoKeepaliveIsBounded(t *testing.T) {
	forever := &proto.Item{Kind: proto.KindChoice, Title: "Deploy?", TimeoutS: 0}
	capWaitWithoutKeepalive(context.Background(), forever)
	if forever.TimeoutS != noKeepaliveCap {
		t.Fatalf("timeout_s = %d, want the %ds ceiling", forever.TimeoutS, noKeepaliveCap)
	}

	// With a ticker, wait-forever means what it says: a question left open all night
	// and answered in the morning is the product working.
	covered := &proto.Item{Kind: proto.KindChoice, Title: "Deploy?", TimeoutS: 0}
	capWaitWithoutKeepalive(withKeepalive(context.Background()), covered)
	if covered.TimeoutS != 0 {
		t.Fatalf("timeout_s = %d, want 0: a covered wait is not bounded", covered.TimeoutS)
	}

	// A timeout the agent chose is never overwritten, in either direction.
	asked := &proto.Item{Kind: proto.KindChoice, Title: "Deploy?", TimeoutS: 4000}
	capWaitWithoutKeepalive(context.Background(), asked)
	if asked.TimeoutS != 4000 {
		t.Fatalf("timeout_s = %d, want the agent's own 4000", asked.TimeoutS)
	}

	// And a notify has nothing to wait for.
	note := &proto.Item{Kind: proto.KindNotify, Title: "done", TimeoutS: 0}
	capWaitWithoutKeepalive(context.Background(), note)
	if note.TimeoutS != 0 {
		t.Fatalf("a notify was given a %ds deadline", note.TimeoutS)
	}
}

// breakableTransport is a transport whose writes start failing on command: the
// pipe to the client going while a call is parked, which no in-memory transport
// does on its own. Read keeps working, so the only thing that fails is the
// notification - the failure R-10 is about.
type breakableTransport struct {
	inner  sdk.Transport
	broken *atomic.Bool
}

type breakableConn struct {
	inner  sdk.Connection
	broken *atomic.Bool
}

var errClientPipeGone = errors.New("test: pipe to the client is gone")

func (b breakableTransport) Connect(ctx context.Context) (sdk.Connection, error) {
	c, err := b.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return breakableConn{inner: c, broken: b.broken}, nil
}

func (c breakableConn) Read(ctx context.Context) (jsonrpc.Message, error) { return c.inner.Read(ctx) }
func (c breakableConn) Write(ctx context.Context, m jsonrpc.Message) error {
	if c.broken.Load() {
		return errClientPipeGone
	}
	return c.inner.Write(ctx, m)
}
func (c breakableConn) Close() error      { return c.inner.Close() }
func (c breakableConn) SessionID() string { return c.inner.SessionID() }

// The wiring, not the rule: s.call is the one place every blocking tool passes
// through, so that is where the ceiling has to be applied (R-10). A daemon that
// answers with the timeout it was SENT is the only way to see it from out here.
func TestTheCeilingReachesTheDaemon(t *testing.T) {
	s := &server{runtimeDir: echoTimeoutDaemon(t)}

	// No keepalive on this context, so a wait-forever ask must arrive bounded.
	res, out, err := s.ask(context.Background(), nil, askIn{Title: "Deploy?", Options: []string{"Yes", "No"}})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if res != nil && res.IsError {
		t.Fatalf("ask came back as a tool error: %s", resultText(res))
	}
	if out.Answer != strconv.Itoa(noKeepaliveCap) {
		t.Fatalf("the daemon was sent timeout_s=%s, want the %ds ceiling", out.Answer, noKeepaliveCap)
	}

	// And with one, the promise the tool's own description makes is kept.
	_, out, err = s.ask(withKeepalive(context.Background()), nil, askIn{Title: "Deploy?", Options: []string{"Yes", "No"}})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if out.Answer != "0" {
		t.Fatalf("a covered wait was sent timeout_s=%s, want 0 - wait forever means wait forever", out.Answer)
	}
}

// echoTimeoutDaemon accepts one connection per call and answers every ask with the
// timeout_s it was given, as the answer text. It exists because the ceiling is
// invisible from the tool's own return value: only the daemon sees what was sent.
func echoTimeoutDaemon(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sock := abserver.SocketPath(dir)
	if len(sock) > 100 {
		t.Skipf("socket path too long for this TMPDIR (%d bytes)", len(sock))
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				sc := bufio.NewScanner(conn)
				for sc.Scan() {
					var req struct {
						ID     any `json:"id"`
						Params struct {
							TimeoutS int `json:"timeout_s"`
						} `json:"params"`
					}
					if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
						return
					}
					reply, _ := json.Marshal(map[string]any{
						"jsonrpc": "2.0", "id": req.ID,
						"result": map[string]any{
							"id": "k1", "answered": true,
							"answer": strconv.Itoa(req.Params.TimeoutS), "outcome": proto.OutcomeAnswered,
						},
					})
					if _, err := conn.Write(append(reply, '\n')); err != nil {
						return
					}
				}
			}()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		<-done
		_ = os.Remove(sock)
	})
	return dir
}
