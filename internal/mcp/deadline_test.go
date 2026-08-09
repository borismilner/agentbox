package mcp

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	abserver "github.com/borismilner/agentbox/internal/server"
)

// R-14. Only the DIAL was bounded. Once a connection existed, a tool whose own
// description promises it "returns at once, never blocks" would wait forever on
// a daemon that accepts the connection and then never answers - which is what a
// daemon wedged on its UI thread or on a store write looks like from out here,
// and the keepalive goes on reporting the call as healthy while it happens.
//
// The rest of the MCP tests deliberately point at a runtime dir with NO daemon
// (mcp_test.go), so every one of them exercises the dial deadline and none of
// them could ever have caught this: a daemon that is absent and a daemon that is
// wedged fail at different points, and only the first was bounded.

// deafDaemon listens on the real socket path, accepts every connection and then
// says nothing at all. It never reads or writes, which is the point: the client
// gets a connection and then silence.
func deafDaemon(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// A unix socket path is capped near 108 bytes and t.TempDir() under a long
	// TMPDIR can exceed it; the failure would look like "cannot reach daemon",
	// which is the answer this test is trying to distinguish from a timeout.
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
			// Hold it open and answer nothing. Closing would give the client a
			// clean EOF, which is a different failure with a different message.
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		<-done
		_ = os.Remove(sock)
	})
	return dir
}

// The budget a bounded tool must answer inside: its own cap plus room for the
// dial and the scheduler, and far under any wait a human would call "forever".
const testCap = 300 * time.Millisecond

func deaf(t *testing.T) *server {
	return &server{runtimeDir: deafDaemon(t), fastCap: testCap}
}

// The tool that names the promise in its own description.
func TestNotifyGivesUpOnADaemonThatNeverAnswers(t *testing.T) {
	s := deaf(t)
	start := time.Now()
	res, _, err := s.notify(t.Context(), nil, notifyIn{Title: "a milestone"})
	took := time.Since(start)

	if err != nil {
		t.Fatalf("notify returned a protocol error %v; want a tool error the model can read", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("notify came back as %+v; want IsError against a wedged daemon", res)
	}
	if took > 5*time.Second {
		t.Errorf("notify took %s; the cap was %s, so it was not bounded at all", took, testCap)
	}
	if txt := resultText(res); !strings.Contains(txt, "did not answer within") {
		t.Errorf("notify said %q; want a sentence saying the daemon did not answer", txt)
	}
}

// Every non-blocking tool, not just the one the audit named. A tool added to
// this family later and left on the unbounded helper is exactly how R-14 came
// back, so the list is the assertion.
func TestEveryNonBlockingToolIsBounded(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(*server, context.Context) (isErr bool, text string)
	}{
		{"notify_user", func(s *server, ctx context.Context) (bool, string) {
			r, _, _ := s.notify(ctx, nil, notifyIn{Title: "t"})
			return r != nil && r.IsError, resultText(r)
		}},
		{"retract", func(s *server, ctx context.Context) (bool, string) {
			r, _, _ := s.retract(ctx, nil, retractIn{})
			return r != nil && r.IsError, resultText(r)
		}},
		{"announce", func(s *server, ctx context.Context) (bool, string) {
			r, _, _ := s.announce(ctx, nil, announceIn{Purpose: "drawing the wiki's frames"})
			return r != nil && r.IsError, resultText(r)
		}},
		{"set_activity", func(s *server, ctx context.Context) (bool, string) {
			r, _, _ := s.controlActivity(ctx, nil, activityIn{Activity: "measuring a card"})
			return r != nil && r.IsError, resultText(r)
		}},
		{"list_agents", func(s *server, ctx context.Context) (bool, string) {
			r, _, _ := s.listAgents(ctx, nil, listIn{})
			return r != nil && r.IsError, resultText(r)
		}},
		{"show_document", func(s *server, ctx context.Context) (bool, string) {
			r, _, _ := s.showDocument(ctx, nil, showIn{Title: "t", Content: "# t"})
			return r != nil && r.IsError, resultText(r)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := deaf(t)
			start := time.Now()
			isErr, text := tc.run(s, t.Context())
			took := time.Since(start)

			if took > 5*time.Second {
				t.Fatalf("%s took %s against a wedged daemon; it is not bounded", tc.name, took)
			}
			if !isErr {
				t.Fatalf("%s did not report an error against a wedged daemon", tc.name)
			}
			if !strings.Contains(text, "did not answer within") {
				t.Errorf("%s said %q; want the deadline sentence", tc.name, text)
			}
		})
	}
}

// The other half of R-14's requirement, and the one worth more: the blocking
// family must keep the CALLER's context. A deadline applied to ask_user would
// cap how long a human is allowed to think, and the failure would read to the
// agent as the daemon dropping their answer.
func TestBlockingToolsKeepTheCallersOwnContext(t *testing.T) {
	s := deaf(t)
	// A caller's context that outlives the fast cap by a wide margin. If ask were
	// bounded like a notify it would return around testCap; it must instead sit
	// there until the caller's own deadline.
	ctx, cancel := context.WithTimeout(t.Context(), 4*testCap)
	defer cancel()

	start := time.Now()
	_, _, _ = s.ask(ctx, nil, askIn{Title: "Where should 2026.7.30 go first?"})
	took := time.Since(start)

	if took < 2*testCap {
		t.Errorf("ask_user gave up after %s, inside the non-blocking cap of %s: "+
			"a blocking tool has been bounded and a human's thinking time is now capped",
			took, testCap)
	}
}

// fastErr must not claim the daemon was slow when it was the caller who gave up.
// The two read identically to a model otherwise, and only one of them is
// AgentBox's fault.
func TestFastErrLeavesTheCallersOwnCancellationAlone(t *testing.T) {
	s := &server{fastCap: testCap}
	done, cancel := context.WithCancel(context.Background())
	cancel()

	if got := s.fastErr(done, context.DeadlineExceeded); got != context.DeadlineExceeded {
		t.Errorf("with the caller already done, fastErr rewrote the error to %v", got)
	}
	live := context.Background()
	got := s.fastErr(live, context.DeadlineExceeded)
	if !strings.Contains(got.Error(), "did not answer within") {
		t.Errorf("with a live caller, fastErr said %v; want the deadline sentence", got)
	}
	if errors.Is(got, context.DeadlineExceeded) {
		// Not a requirement so much as a note: the wrapped form would let a caller
		// branch on it. It does not today, and nothing needs it to.
		t.Log("fastErr wraps DeadlineExceeded; fine, but nothing relies on it")
	}
}

// resultText flattens a tool result's content to the string a model would read.
func resultText(r *sdk.CallToolResult) string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	for _, c := range r.Content {
		if t, ok := c.(*sdk.TextContent); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String()
}
