package mcp

import (
	"context"
	"fmt"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
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
			// The ticker rides its own context so it stops the moment the handler
			// returns, whether that is an answer, a timeout or an error.
			tickCtx, stop := context.WithCancel(ctx)
			defer stop()
			go keepalive(tickCtx, ss, token, first, every)
			return next(ctx, method, req)
		}
	}
}

// keepalive sends one progress notification per interval until ctx ends. The
// message is deliberately generic: the middleware does not know whether the
// call is waiting on the human, on a lock or on a signal, and a wrong guess in
// the client's status line would be worse than no detail.
func keepalive(ctx context.Context, ss *sdk.ServerSession, token any, first, every time.Duration) {
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
		// A failed notification is not worth reporting anywhere: the client has
		// hung up or the pipe is gone, and the call itself is about to fail with
		// something the caller can act on.
		_ = ss.NotifyProgress(ctx, &sdk.ProgressNotificationParams{
			ProgressToken: token,
			Progress:      waited.Seconds(),
			Message:       fmt.Sprintf("still waiting (%s)", waited),
		})
		t.Reset(every)
	}
}

// progressToken digs the token out of a request, or nil if the client sent none.
func progressToken(req sdk.Request) any {
	p, ok := req.GetParams().(sdk.RequestParams)
	if !ok {
		return nil
	}
	return p.GetProgressToken()
}
