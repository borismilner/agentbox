package mcp

import (
	"context"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borismilner/agentbox/internal/manual"
)

// The canonical way agentbox hands its standards to an agent: MCP resources.
//
// A tool description is the wrong place for a standard. It is read once, it is
// billed on every request whether or not the agent is about to write a review,
// and there is no room in it for twenty-five rules. Resources are the primitive
// the protocol has for exactly this - documents a client can list and read on
// demand - so an agent that is about to author a walkthrough can ask for the
// standard first, and one doing anything else never pays for it.
//
// The same content is `agentbox docs walkthrough` on the command line and is embedded
// in the binary, so the shell path, the MCP path and the documentation cannot
// drift: there is one copy.
const (
	uriWalkthrough = "agentbox://standards/walkthrough"
	uriAgent       = "agentbox://manual/agent"
	uriSchema      = "agentbox://schema/wire"
)

// addStandards registers the documents an agent may want mid-task. Every one is
// static and local: reading a resource here touches no daemon, no socket and no
// network, so it is safe to ask for at any point.
func addStandards(srv *sdk.Server) {
	text := func(uri, body string) sdk.ResourceHandler {
		return func(context.Context, *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
			return &sdk.ReadResourceResult{
				Contents: []*sdk.ResourceContents{{URI: uri, MIMEType: "text/markdown", Text: body}},
			}, nil
		}
	}

	srv.AddResource(&sdk.Resource{
		URI:      uriWalkthrough,
		Name:     "walkthrough-standard",
		Title:    "How to write an AgentBox walkthrough",
		MIMEType: "text/markdown",
		Description: "The standard for authoring a walkthrough (durable step-by-step code review): how to structure steps, " +
			"where the explanation goes versus the annotations, how to point prose at code without writing line numbers, " +
			"what coverage means, and how to close with a gate. READ THIS BEFORE CALLING create_walkthrough.",
	}, text(uriWalkthrough, manual.Walkthrough()))

	srv.AddResource(&sdk.Resource{
		URI:      uriAgent,
		Name:     "agent-manual",
		Title:    "agentbox agent manual",
		MIMEType: "text/markdown",
		Description: "Every AgentBox tool, when to reach for which, the full walkthrough spec reference, the CLI, and the " +
			"anti-patterns. The same content as `agentbox docs agent`.",
	}, text(uriAgent, manual.Agent()))

	srv.AddResource(&sdk.Resource{
		URI:      uriSchema,
		Name:     "wire-schema",
		Title:    "agentbox wire-protocol JSON Schema",
		MIMEType: "application/json",
		Description: "JSON Schema for AgentBox's Item/Result wire protocol. For building against AgentBox's socket directly rather " +
			"than through these tools.",
	}, func(context.Context, *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
		return &sdk.ReadResourceResult{
			Contents: []*sdk.ResourceContents{{URI: uriSchema, MIMEType: "application/json", Text: manual.Schema()}},
		}, nil
	})

	// A prompt as well as a resource, because the two are reached differently: a
	// resource is something an agent decides to read, a prompt is something the
	// human invokes (a slash command in most clients) to put the standard in
	// front of an agent that did not think to ask.
	srv.AddPrompt(&sdk.Prompt{
		Name:        "walkthrough_standard",
		Title:       "Load the walkthrough authoring standard",
		Description: "Put AgentBox's standard for writing a walkthrough into the conversation, before authoring one.",
	}, func(context.Context, *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
		return &sdk.GetPromptResult{
			Description: "AgentBox's walkthrough authoring standard",
			Messages: []*sdk.PromptMessage{{
				Role: "user",
				Content: &sdk.TextContent{Text: fmt.Sprintf(
					"Follow this standard for the walkthrough you are about to write. It is also readable any time as "+
						"the MCP resource %s, or `agentbox docs walkthrough`.\n\n%s", uriWalkthrough, manual.Walkthrough())},
			}},
		}, nil
	})
}
