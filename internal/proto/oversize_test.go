package proto

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"
)

// R-04. Item.Validate had no length check on anything an agent writes, so the
// only bound in the path was the wire line itself. Passing it produced no
// refusal at all: the daemon's scanner returned ErrTooLong, Serve returned, and
// the connection closed with NO response written. The agent got
// "connection closed during agentbox.v1.ask: EOF" - naming neither the size nor
// the field - and since nothing had been stored there was not even a history row
// to find afterwards. The message was simply gone.

func bigItem(field string, n int) *Item {
	it := Item{
		Kind: KindNotify, Title: "a title",
		Identity: Identity{Agent: "claude-code"},
	}
	pad := strings.Repeat("x", n)
	switch field {
	case "title":
		it.Title = pad
	case "body":
		it.Body = pad
	case "diff":
		it.Kind, it.Diff = KindDiff, pad
	case "speak":
		it.Speak = pad
	}
	return &it
}

func TestValidateRefusesOversizedFieldsByName(t *testing.T) {
	for _, tc := range []struct {
		field string
		max   int
	}{
		{"title", MaxTitleBytes},
		{"body", MaxBodyBytes},
		{"diff", MaxDiffBytes},
		{"speak", MaxSpeakBytes},
	} {
		t.Run(tc.field, func(t *testing.T) {
			if err := bigItem(tc.field, tc.max).Validate(); err != nil {
				t.Fatalf("exactly at the %d-byte limit was refused: %v", tc.max, err)
			}
			err := bigItem(tc.field, tc.max+1).Validate()
			if err == nil {
				t.Fatalf("one byte over the %d-byte limit was accepted", tc.max)
			}
			// The refusal has to be actionable. An agent that cannot see WHICH
			// field was too big cannot shorten it, and that was the whole
			// complaint about the EOF this replaces.
			if !strings.Contains(err.Error(), tc.field) {
				t.Errorf("refusal %q does not name the field", err)
			}
			if !strings.Contains(err.Error(), "limit") {
				t.Errorf("refusal %q does not mention a limit", err)
			}
		})
	}
}

// Every cap must leave room under the wire line, or an item that Validate
// accepts still cannot be sent - which is the same silence one layer down.
func TestTheCapsFitInsideTheWireLine(t *testing.T) {
	it := Item{
		Kind: KindDiff, Title: strings.Repeat("t", MaxTitleBytes),
		Body: strings.Repeat("b", MaxBodyBytes), Diff: strings.Repeat("d", MaxDiffBytes),
		Speak: strings.Repeat("s", MaxSpeakBytes), Identity: Identity{Agent: "claude-code"},
	}
	if err := it.Validate(); err != nil {
		t.Fatalf("a maximal item does not validate: %v", err)
	}
	line, err := json.Marshal(it)
	if err != nil {
		t.Fatal(err)
	}
	if len(line) >= MaxLineBytes {
		t.Fatalf("a maximal item marshals to %d bytes, at or over the %d-byte wire line: "+
			"Validate would accept something the connection cannot carry", len(line), MaxLineBytes)
	}
}

// The other half, and the one that survives anything that never went through
// Validate: a line past the wire cap gets a sentence rather than a dropped
// connection.
//
// Driven over a raw net.Pipe rather than through Call, because net.Pipe is
// unbuffered: a 4 MB write blocks until somebody consumes it, and the server
// stops consuming at exactly the moment under test. The writer is left parked on
// purpose and the response is read directly, which is the only way to see what
// Serve says on its way out.
func TestServeAnswersAnOversizedLineBeforeDropping(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	server := NewConn(b)
	served := make(chan error, 1)
	go func() {
		served <- server.Serve(context.Background(), func(context.Context, string, json.RawMessage) (any, *RPCError) {
			t.Error("the handler ran; an oversized line should never reach it")
			return nil, nil
		})
	}()

	// Never completes: the scanner gives up partway through and stops reading.
	go func() {
		line := append([]byte(`{"jsonrpc":"2.0","id":1,"method":"agentbox.v1.ask","params":"`), bytes.Repeat([]byte("x"), MaxLineBytes+1024)...)
		_, _ = a.Write(append(line, '"', '}', '\n'))
	}()

	_ = a.SetReadDeadline(time.Now().Add(15 * time.Second))
	dec := json.NewDecoder(a)
	var resp struct {
		ID    *int64    `json:"id"`
		Error *RPCError `json:"error"`
	}
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("Serve dropped the connection without answering: %v", err)
	}
	if resp.Error == nil {
		t.Fatalf("Serve answered %+v, want an error", resp)
	}
	if resp.ID != nil {
		t.Errorf("the refusal carries id %v; the line was never parsed, so it has no id to answer on", *resp.ID)
	}
	if !strings.Contains(resp.Error.Message, "wire limit") {
		t.Errorf("refusal says %q; want the wire limit named", resp.Error.Message)
	}
	if !strings.Contains(resp.Error.Message, "nothing was stored") {
		t.Errorf("refusal says %q; it must say the message was lost, which is what "+
			"decides whether the agent retries", resp.Error.Message)
	}
	<-served
}

// And the client half: a refusal with no id is what pending calls report, rather
// than the EOF that follows it a moment later. Without this the sentence Serve
// just went to the trouble of sending is read by nobody.
func TestAPendingCallPrefersARefusalOverTheEOF(t *testing.T) {
	client, server := pipeConns(t)

	// Serve, with a handler that parks. net.Pipe is unbuffered, so without
	// something reading, the client's own write blocks and the call fails at send
	// time - which is a different failure from the one under test. Parking rather
	// than answering leaves the call pending, which is the state a connection-level
	// refusal has to reach.
	reached := make(chan struct{})
	go server.Serve(context.Background(), func(ctx context.Context, _ string, _ json.RawMessage) (any, *RPCError) {
		close(reached)
		<-ctx.Done()
		return nil, nil
	})

	done := make(chan error, 1)
	go func() {
		var res Result
		done <- client.Call(context.Background(), MethodAsk, struct{}{}, &res)
	}()

	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("the request never reached the handler")
	}
	// Now answer the way Serve does when a line is past the wire cap.

	_ = server.send(response{JSONRPC: "2.0", Error: &RPCError{
		Code: CodeInvalidRequest, Message: "that request was over the wire limit; nothing was stored",
	}})
	server.Close()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("the call succeeded against a closed connection")
		}
		if strings.Contains(err.Error(), "EOF") {
			t.Errorf("the call reported %q, losing the refusal that arrived first", err)
		}
		if !strings.Contains(err.Error(), "wire limit") {
			t.Errorf("the call reported %q; want the peer's own sentence", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the call neither succeeded nor failed")
	}
}
