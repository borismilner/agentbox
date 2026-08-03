package manual

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/borismilner/agentbox/internal/proto"
)

func TestSchemaIsValidJSON(t *testing.T) {
	var v any
	if err := json.Unmarshal([]byte(Schema()), &v); err != nil {
		t.Fatalf("schema.json is not valid JSON: %v", err)
	}
}

// The schema must name every item kind, so it stays in sync when a new kind
// lands (it is the documented validation contract, FR42).
func TestSchemaListsEveryKind(t *testing.T) {
	for _, k := range []proto.Kind{
		proto.KindNotify, proto.KindChoice, proto.KindText,
		proto.KindConfirm, proto.KindVeto, proto.KindForm, proto.KindSecret,
	} {
		if !strings.Contains(Schema(), `"`+string(k)+`"`) {
			t.Errorf("schema.json does not list kind %q", k)
		}
	}
}

func TestAgentQuickstartCoversCommands(t *testing.T) {
	a := Agent()
	for _, want := range []string{"notify", "ask", "input", "confirm", "veto", "form", "Exit codes", "MCP"} {
		if !strings.Contains(a, want) {
			t.Errorf("agent quickstart is missing %q", want)
		}
	}
}

func TestTopics(t *testing.T) {
	if _, ok := Get("agent"); !ok {
		t.Fatal("agent topic not registered")
	}
	if len(Topics()) == 0 {
		t.Fatal("Topics() returned nothing")
	}
}

// The session briefing is what tells a child agentbox spawned that it can reach
// the human. An empty one would be a session that behaves like any other headless
// agent, which is the whole thing agentbox exists to fix - so this checks it says the
// three things that change behaviour, not just that a file exists.
func TestSessionBriefingSaysWhatChangesBehaviour(t *testing.T) {
	s := Session()
	if len(s) < 400 {
		t.Fatalf("the session briefing is %d bytes; that is not a briefing", len(s))
	}
	for _, want := range []string{
		"AgentBox session", // where it is
		"inline",           // its own questions come back in the conversation
		"notify_user",      // the non-blocking default
		"act_unless_stopped",
		"speak",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the briefing never mentions %q", want)
		}
	}
	if got, ok := Get("session"); !ok || got != s {
		t.Error("`agentbox docs session` does not print the briefing")
	}
	if !slices.Contains(Topics(), "session") {
		t.Errorf("session is not a listed topic: %v", Topics())
	}
}
