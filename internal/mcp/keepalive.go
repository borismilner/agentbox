package mcp

import (
	"context"
	"fmt"
	"os"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borismilner/agentbox/internal/proto"
)

// The keep-alive ticker (FR83, FR88).
//
// An MCP client gives up on a tool call that goes quiet. Claude Code's stdio
// default is 1800s of silence, checked on a 30s poll, and a progress
// notification resets that clock - measured against the real client
// 2026-08-04 with tools/idlecap-probe.sh, and matching the client's own
// message: "sent no response or progress for Ns; aborting".
//
// Every blocking tool agentbox has is therefore on a 30-minute fuse without
// this: the human's card stays up, his answer arrives at a caller that is
// already gone, and the agent sees a transport error instead of an answer.
// That was shipped behaviour until this ticker, and it is also the mechanism
// FR83's parked waits (a lock, a signal) depend on, so it lives in the
// middleware and covers every tool at once rather than in each of them.
//
// Silence is only a problem for a call that parks, so nothing is sent until a
// call has already lasted keepaliveFirstAt. A fast tool - which is nearly all
// of them - never ticks at all.
const (
	keepaliveFirstAt  = 60 * time.Second
	keepaliveInterval = 60 * time.Second
)

// keepaliveMiddleware ticks progress notifications while a tool call is
// parked, so the client keeps waiting for the answer instead of aborting.
//
// It hooks tools/call only: the token comes from that request, and no other
// method of the protocol parks. A client that sends no progressToken gets
// nothing, because the spec ties a progress notification to a request that
// asked for one - Claude Code always asks (verified: `progressToken` on every
// tools/call it sends).
func keepaliveMiddleware(next sdk.MethodHandler) sdk.MethodHandler {
	return keepaliveMiddlewareEvery(keepaliveFirstAt, keepaliveInterval)(next)
}

// keepaliveMiddlewareEvery is keepaliveMiddleware with its two intervals
// given, so a test can watch a tick in milliseconds instead of minutes.
func keepaliveMiddlewareEvery(first, every time.Duration) sdk.Middleware {
	return func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}
			token := progressToken(req)
			ss, ok := req.GetSession().(*sdk.ServerSession)
			if token == nil || !ok {
				return next(ctx, method, req)
			}
			// R-10. Two things ride on this context. It stops the ticker the moment
			// the handler returns - answer, timeout or error - and it is what the
			// ticker cancels if the client stops taking notifications, so a question
			// nobody can answer any more stops being one (the daemon reads the cancel
			// as its caller going, retires the card and logs item.caller_gone).
			//
			// The SDK tears the session down on a failed write of its own accord,
			// measured on go-sdk v1.6.1 with a transport whose writes were broken
			// under a parked call: the handler's context was already cancelled. This
			// does not lean on that. A guarantee nobody in this tree owns is one that
			// can go away in a version bump without a single test noticing.
			callCtx, abort := context.WithCancel(withKeepalive(ctx))
			defer abort()
			go keepalive(callCtx, ss, token, first, every, abort)
			return next(callCtx, method, req)
		}
	}
}

// keepalive sends one progress notification per interval until ctx ends, and
// ends the call itself if one of them cannot be delivered. The message is
// deliberately generic: the middleware does not know whether the call is waiting
// on the human, on a lock or on a signal, and a wrong guess in the client's
// status line would be worse than no detail.
func keepalive(ctx context.Context, ss *sdk.ServerSession, token any, first, every time.Duration, gone func()) {
	started := time.Now()
	t := time.NewTimer(first)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		waited := time.Since(started).Round(time.Second)
		err := ss.NotifyProgress(ctx, &sdk.ProgressNotificationParams{
			ProgressToken: token,
			Progress:      waited.Seconds(),
			Message:       fmt.Sprintf("still waiting (%s)", waited),
		})
		if err != nil {
			// R-10. This used to be `_ =`, with a comment saying a failure was not
			// worth reporting because the call was about to fail anyway. It is the
			// opposite: this ticker is the only thing holding a parked call up, so its
			// failure IS the call failing, and swallowing it left the human answering
			// a card whose caller had gone. One line to the host's log, then end the
			// call so the daemon retires the card instead of waiting for an answer
			// nobody will receive.
			if ctx.Err() == nil {
				fmt.Fprintf(os.Stderr, "agentbox mcp: keepalive failed after %s (%v); treating the client as gone\n", waited, err)
				gone()
			}
			return
		}
		t.Reset(every)
	}
}

// keepaliveKey marks a context whose call has a ticker behind it (R-10).
//
// The absence of one is the dangerous state and it is silent: no progressToken or
// a session the SDK does not recognise means no notifications, so the client's
// 1800s idle fuse runs unopposed while the daemon happily waits for ever. A tool
// that parks reads this to decide whether it may promise "wait forever" at all.
type keepaliveKey struct{}

func withKeepalive(ctx context.Context) context.Context {
	return context.WithValue(ctx, keepaliveKey{}, true)
}

// hasKeepalive says whether this call's silence is being covered.
func hasKeepalive(ctx context.Context) bool {
	on, _ := ctx.Value(keepaliveKey{}).(bool)
	return on
}

// noKeepaliveCap is how long a question may wait when nothing is holding the
// client's idle fuse open (R-10). It sits under Claude Code's measured 1800s so
// the wait ends as an honest `expired` the agent can read and retry, rather than
// as an abandoned call whose card the human answers into nothing.
//
// The same shape as the sync family's ceiling, which was built this way from the
// start (a lock or signal park is capped at 25 minutes for exactly this reason).
// What is deliberately NOT done here is capping every wait: with a ticker running,
// `timeout_s: 0` means what it says, and a question left open all night to be
// answered in the morning is the product working.
const noKeepaliveCap = 1500 // seconds

// capWaitWithoutKeepalive bounds a wait-forever question raised by a client that
// asked for no progress notifications. Claude Code always asks (verified), so this
// changes nothing for the host agentbox is built around; it is the hosts nobody
// has tested against that would otherwise lose an answer silently.
func capWaitWithoutKeepalive(ctx context.Context, it *proto.Item) {
	if it == nil || it.TimeoutS != 0 || !it.Blocking() || hasKeepalive(ctx) {
		return
	}
	it.TimeoutS = noKeepaliveCap
	fmt.Fprintf(os.Stderr, "agentbox mcp: this client sends no progressToken, so a wait cannot be held open past "+
		"its idle limit; bounding %q at %ds instead of waiting for ever\n", it.Title, noKeepaliveCap)
}

// progressToken digs the token out of a request, or nil if the client sent none.
func progressToken(req sdk.Request) any {
	p, ok := req.GetParams().(sdk.RequestParams)
	if !ok {
		return nil
	}
	return p.GetProgressToken()
}
