package mcp

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The rider only works if the line a call collected reaches the result of the
// tool that made the call. Both halves have been wrong once: the box lives in the
// context, so a handler served with a different context finds nothing, and the
// append needs the result to really be a CallToolResult.
func TestARiderCollectedDuringAToolCallIsAppendedToItsResult(t *testing.T) {
	srv := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	srv.AddReceivingMiddleware(riderMiddleware)

	type in struct {
		Line string `json:"line"`
	}
	sdk.AddTool(srv, &sdk.Tool{Name: "pretend", Description: "collects a rider"},
		func(ctx context.Context, _ *sdk.CallToolRequest, arg in) (*sdk.CallToolResult, struct{}, error) {
			// Exactly what a real handler's call helper does with what came back
			// on the envelope.
			noteRider(ctx, arg.Line)
			return &sdk.CallToolResult{
				Content: []sdk.Content{&sdk.TextContent{Text: "the answer"}},
			}, struct{}{}, nil
		})

	ct, st := sdk.NewInMemoryTransports()
	go func() { _, _ = srv.Connect(context.Background(), st, nil) }()
	client := sdk.NewClient(&sdk.Implementation{Name: "probe", Version: "0"}, nil)
	cs, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer cs.Close()

	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name: "pretend", Arguments: map[string]any{"line": "sync: 1 agent joined repo:agentbox"},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var texts []string
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			texts = append(texts, tc.Text)
		}
	}
	joined := strings.Join(texts, " | ")
	if !strings.Contains(joined, "the answer") {
		t.Errorf("the tool's own answer went missing: %q", joined)
	}
	if !strings.Contains(joined, "sync: 1 agent joined") {
		t.Errorf("the rider never reached the result: %q", joined)
	}
	// The answer first, the news after it.
	if i, j := strings.Index(joined, "the answer"), strings.Index(joined, "sync:"); i > j {
		t.Errorf("the rider was put before the answer: %q", joined)
	}
}

func TestACallOutsideAToolHandlerDropsItsRider(t *testing.T) {
	// The attach stream and the redial loop call the daemon with no tool result to
	// ride on. Dropping the line is correct; panicking or stashing it for whoever
	// calls next would not be.
	noteRider(context.Background(), "sync: nobody will read this")
}
