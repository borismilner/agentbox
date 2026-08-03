// Package manual is the binary's self-teaching content (FR40-42): an agent
// holding only the executable can learn every capability. Content is
// embedded so documentation and behavior version together.
package manual

import (
	_ "embed"
	"sort"
)

//go:embed agent.md
var agentMD string

//go:embed schema.json
var schemaJSON string

//go:embed session.md
var sessionMD string

//go:embed assignment.md
var assignmentMD string

//go:embed walkthrough.md
var walkthroughMD string

// topics maps `agentbox docs TOPIC` names to their content. `setup` is
// generated (it depends on the binary path), so it is not in here.
var topics = map[string]string{
	"agent":       agentMD,
	"assignment":  assignmentMD,
	"session":     sessionMD,
	"walkthrough": walkthroughMD,
}

// Agent returns the context-sized agent quickstart (FR40).
func Agent() string { return agentMD }

// Session is the briefing appended to the system prompt of a `claude` child
// agentbox spawns itself, so a session started from the panel or the app window
// knows agentbox is there and knows the etiquette without being told. It is not the
// full manual on purpose: it is the part that changes what the agent does, and it
// is short enough to hand over on every session.
func Session() string { return sessionMD }

// Assignment is the briefing for a run agentbox started by itself (M12/FR82): that
// nobody typed this prompt, that the final message is the report, and how to
// record data for later. It replaces the session brief rather than adding to
// it - half of what a panel session is told ("they are probably looking at
// you") is the opposite of true here.
func Assignment() string { return assignmentMD }

// Walkthrough is the standard for authoring a review kit: how to structure the
// steps, where the "why" goes, what coverage means, how to close. Served as an
// MCP resource as well as `agentbox docs walkthrough`, so an agent with no shell can
// ask for it before it writes one.
func Walkthrough() string { return walkthroughMD }

// Schema returns the wire-protocol JSON Schema (FR42).
func Schema() string { return schemaJSON }

// Get returns a topic's content by name.
func Get(name string) (string, bool) {
	s, ok := topics[name]
	return s, ok
}

// Topics lists the available `agentbox docs` topic names, sorted.
func Topics() []string {
	names := make([]string, 0, len(topics))
	for n := range topics {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
