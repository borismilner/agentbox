package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The discovery rider, child side (FR83).
//
// The daemon decides what an agent has not been told about its area and puts one
// line on the response envelope. This turns that line into something the model
// actually reads: an extra text block on the result of whatever tool it happened
// to call. No new tool, no parking, no hook - and it lands at the moment the
// agent is about to act, which is the moment the collision would have happened.
//
// The plumbing is a box in the context rather than a field on the server, because
// tool calls overlap: two of them in flight would otherwise take each other's
// news, and a line about a peer must arrive attached to the call it came back on.

type riderBox struct {
	line string
}

type riderKey struct{}

// withRiderBox puts a fresh box in ctx for one tool call.
func withRiderBox(ctx context.Context) (context.Context, *riderBox) {
	box := &riderBox{}
	return context.WithValue(ctx, riderKey{}, box), box
}

// noteRider records what came back on an envelope. Called by the call helpers,
// which is why they all go through CallRidden. A call made outside a tool handler
// finds no box and drops the line, which is correct: there is no result for it to
// ride on.
func noteRider(ctx context.Context, line string) {
	if line == "" {
		return
	}
	if box, ok := ctx.Value(riderKey{}).(*riderBox); ok {
		// Last one wins if a handler makes several calls. The rider is a statement
		// about the roster right now, so the freshest is the true one, and the
		// daemon's cursor means an earlier one said something this one repeats.
		box.line = line
	}
}

// riderMiddleware wraps every incoming request so that any news collected while
// serving it is appended to the tool result.
//
// It hooks tools/call only. A rider is a line for the model to read, and the
// other methods of the protocol (listing tools, reading resources) have no such
// audience - a line about a peer buried in a tool listing would be read by
// nobody and would still have cost the daemon its cursor.
func riderMiddleware(next sdk.MethodHandler) sdk.MethodHandler {
	return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
		if method != "tools/call" {
			return next(ctx, method, req)
		}
		ctx, box := withRiderBox(ctx)
		res, err := next(ctx, method, req)
		if err != nil || box.line == "" {
			return res, err
		}
		call, ok := res.(*sdk.CallToolResult)
		if !ok {
			return res, err
		}
		// Appended, never prepended: the answer the agent asked for comes first,
		// and the news about company after it.
		call.Content = append(call.Content, &sdk.TextContent{Text: box.line})
		return call, nil
	}
}
