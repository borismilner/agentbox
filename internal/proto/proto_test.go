package proto

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

func validChoice() Item {
	return Item{
		Kind:     KindChoice,
		Title:    "Deploy?",
		Options:  []Option{{Label: "Yes"}, {Label: "No"}},
		Identity: Identity{Agent: "test"},
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Item)
		wantErr string
	}{
		{"valid choice", func(it *Item) {}, ""},
		{"valid notify", func(it *Item) { it.Kind = KindNotify; it.Options = nil }, ""},
		{"valid confirm", func(it *Item) { it.Kind = KindConfirm; it.Options = nil }, ""},
		{"valid text", func(it *Item) { it.Kind = KindText; it.Options = nil }, ""},
		{"unknown kind", func(it *Item) { it.Kind = "shout" }, "unknown kind"},
		{"no title", func(it *Item) { it.Title = "" }, "title is required"},
		{"no agent", func(it *Item) { it.Identity.Agent = "" }, "identity.agent is required"},
		{"one option", func(it *Item) { it.Options = it.Options[:1] }, "at least 2"},
		{"ten options", func(it *Item) {
			it.Options = nil
			for i := range 10 {
				it.Options = append(it.Options, Option{Label: fmt.Sprintf("o%d", i)})
			}
		}, "at most 9"},
		{"empty option label", func(it *Item) { it.Options[1].Label = "" }, "empty label"},
		{"default not an option", func(it *Item) { it.Default = "Maybe" }, "not one of the options"},
		{"default is an option", func(it *Item) { it.Default = "No" }, ""},
		{"bad level", func(it *Item) { it.Level = "loud" }, "unknown level"},
		{"negative timeout", func(it *Item) { it.TimeoutS = -1 }, ">= 0"},
		{"valid veto", func(it *Item) { it.Kind = KindVeto; it.Options = nil; it.TimeoutS = 15 }, ""},
		{"veto without window", func(it *Item) { it.Kind = KindVeto; it.Options = nil }, "positive timeout_s"},
		{"valid secret to-file", func(it *Item) { it.Kind = KindSecret; it.Options = nil; it.Sink = "/tmp/x" }, ""},
		{"valid secret stdout", func(it *Item) { it.Kind = KindSecret; it.Options = nil; it.Stdout = true }, ""},
		{"secret with no destination", func(it *Item) { it.Kind = KindSecret; it.Options = nil }, "sink"},
		{"valid diff", func(it *Item) { it.Kind = KindDiff; it.Options = nil; it.Diff = "@@ -1 +1 @@" }, ""},
		{"diff without a diff", func(it *Item) { it.Kind = KindDiff; it.Options = nil }, "need a diff"},
		{"valid notify actions", func(it *Item) {
			it.Kind = KindNotify
			it.Options = nil
			it.Actions = []Action{{Label: "Open", Exec: "xdg-open ."}}
		}, ""},
		{"actions on non-notify", func(it *Item) {
			it.Actions = []Action{{Label: "Open", Exec: "xdg-open ."}}
		}, "only allowed on notify"},
		{"too many actions", func(it *Item) {
			it.Kind = KindNotify
			it.Options = nil
			for range MaxActions + 1 {
				it.Actions = append(it.Actions, Action{Label: "a", Exec: "true"})
			}
		}, "at most"},
		{"action empty label", func(it *Item) {
			it.Kind = KindNotify
			it.Options = nil
			it.Actions = []Action{{Label: "", Exec: "true"}}
		}, "empty label"},
		{"action empty command", func(it *Item) {
			it.Kind = KindNotify
			it.Options = nil
			it.Actions = []Action{{Label: "Run", Exec: ""}}
		}, "empty command"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			it := validChoice()
			tc.mutate(&it)
			err := it.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && searchString(s, sub))
}

func searchString(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestBlocking(t *testing.T) {
	for kind, want := range map[Kind]bool{KindNotify: false, KindChoice: true, KindText: true, KindConfirm: true} {
		it := Item{Kind: kind}
		if it.Blocking() != want {
			t.Errorf("Blocking(%s) = %v, want %v", kind, it.Blocking(), want)
		}
	}
}

func pipeConns(t *testing.T) (*Conn, *Conn) {
	t.Helper()
	a, b := net.Pipe()
	ca, cb := NewConn(a), NewConn(b)
	t.Cleanup(func() { ca.Close(); cb.Close() })
	return ca, cb
}

func TestServeRecoversHandlerPanic(t *testing.T) {
	// A panicking handler must not crash the server goroutine; the caller gets
	// a CodeInternal error, and a later request on the same connection still
	// works (robustness: no silent hang, no process death).
	client, server := pipeConns(t)
	go server.Serve(context.Background(), func(_ context.Context, method string, _ json.RawMessage) (any, *RPCError) {
		if method == "boom" {
			panic("handler exploded")
		}
		return Result{ID: "ok"}, nil
	})

	var res Result
	err := client.Call(context.Background(), "boom", struct{}{}, &res)
	rpcErr, ok := err.(*RPCError)
	if !ok || rpcErr.Code != CodeInternal {
		t.Fatalf("panicking handler should return CodeInternal, got %v", err)
	}
	// The connection survived the panic.
	if err := client.Call(context.Background(), MethodNotify, struct{}{}, &res); err != nil {
		t.Fatalf("connection did not survive the panic: %v", err)
	}
	if res.ID != "ok" {
		t.Fatalf("follow-up call got %q, want ok", res.ID)
	}
}

func TestCallRoundTrip(t *testing.T) {
	client, server := pipeConns(t)
	go server.Serve(context.Background(), func(_ context.Context, method string, params json.RawMessage) (any, *RPCError) {
		if method != MethodNotify {
			return nil, &RPCError{Code: CodeMethodNotFound, Message: method}
		}
		var it Item
		if err := json.Unmarshal(params, &it); err != nil {
			return nil, &RPCError{Code: CodeInvalidParams, Message: err.Error()}
		}
		return Result{ID: "item-1", Answered: false}, nil
	})

	var res Result
	it := validChoice()
	if err := client.Call(context.Background(), MethodNotify, &it, &res); err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.ID != "item-1" {
		t.Fatalf("got ID %q, want item-1", res.ID)
	}
}

func TestCallServerError(t *testing.T) {
	client, server := pipeConns(t)
	go server.Serve(context.Background(), func(_ context.Context, _ string, _ json.RawMessage) (any, *RPCError) {
		return nil, &RPCError{Code: CodeItemNotFound, Message: "no such item"}
	})

	err := client.Call(context.Background(), MethodCancel, map[string]string{"id": "x"}, nil)
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("expected *RPCError, got %T: %v", err, err)
	}
	if rpcErr.Code != CodeItemNotFound {
		t.Fatalf("got code %d, want %d", rpcErr.Code, CodeItemNotFound)
	}
}

// A slow call must not block a fast call issued later on the same
// connection; this is what lets ask block for minutes while notify flows.
func TestConcurrentCallsInterleave(t *testing.T) {
	client, server := pipeConns(t)
	release := make(chan struct{})
	go server.Serve(context.Background(), func(_ context.Context, method string, _ json.RawMessage) (any, *RPCError) {
		if method == "slow" {
			<-release
		}
		return Result{ID: method}, nil
	})

	var wg sync.WaitGroup
	wg.Add(1)
	slowDone := make(chan error, 1)
	go func() {
		defer wg.Done()
		var r Result
		slowDone <- client.Call(context.Background(), "slow", nil, &r)
	}()

	var fast Result
	if err := client.Call(context.Background(), "fast", nil, &fast); err != nil {
		t.Fatalf("fast call while slow pending: %v", err)
	}
	select {
	case err := <-slowDone:
		t.Fatalf("slow call returned before release: %v", err)
	default:
	}
	close(release)
	wg.Wait()
	if err := <-slowDone; err != nil {
		t.Fatalf("slow call: %v", err)
	}
}

func TestCallContextCancel(t *testing.T) {
	client, server := pipeConns(t)
	go server.Serve(context.Background(), func(ctx context.Context, _ string, _ json.RawMessage) (any, *RPCError) {
		<-ctx.Done()
		return nil, &RPCError{Code: CodeShuttingDown, Message: "bye"}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := client.Call(ctx, MethodAsk, nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want deadline exceeded", err)
	}
}

func TestCallConnectionDrop(t *testing.T) {
	client, server := pipeConns(t)
	go func() {
		time.Sleep(20 * time.Millisecond)
		server.Close()
	}()
	err := client.Call(context.Background(), MethodAsk, nil, nil)
	if err == nil {
		t.Fatal("expected error after connection drop")
	}
}

func TestServeRejectsGarbage(t *testing.T) {
	a, b := net.Pipe()
	server := NewConn(b)
	t.Cleanup(func() { a.Close(); b.Close() })
	go server.Serve(context.Background(), func(_ context.Context, _ string, _ json.RawMessage) (any, *RPCError) {
		t.Error("handler must not run for garbage input")
		return nil, nil
	})

	go a.Write([]byte("not json at all\n"))
	buf := make([]byte, 4096)
	n, err := a.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var resp struct {
		Error *RPCError `json:"error"`
	}
	if err := json.Unmarshal(buf[:n], &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != CodeParse {
		t.Fatalf("got %+v, want parse error", resp.Error)
	}
}
