package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeStub writes an executable shell stub that emits the given stdout and
// returns its path, for exercising the real spawn/read path without `claude`.
func writeStub(t *testing.T, stdout string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "claude-stub")
	script := "#!/bin/sh\ncat <<'AgentBoxSTUB'\n" + stdout + "AgentBoxSTUB\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// waitState polls until the driver reaches want or the deadline passes.
func waitState(t *testing.T, d *Driver, want State) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if d.State() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("state = %v, never reached %v", d.State(), want)
}

func TestDriverSpawnAndRead(t *testing.T) {
	stub := writeStub(t, ""+
		`{"type":"system","subtype":"init","session_id":"sess-stub","model":"m"}`+"\n"+
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"stub reply"}]}}`+"\n"+
		`{"type":"result","subtype":"success","total_cost_usd":0.05}`+"\n")

	d := New(Config{Bin: stub}, nil, nil)
	if err := d.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(d.Kill)

	waitState(t, d, StateEnded)
	if d.SessionID() != "sess-stub" {
		t.Errorf("session id = %q, want sess-stub", d.SessionID())
	}
	turns := d.Turns()
	if len(turns) != 1 || turns[0].Segments[0].Text != "stub reply" {
		t.Fatalf("turns = %+v", turns)
	}
}

func TestDriverArgs(t *testing.T) {
	d := New(Config{Mode: "plan", Partial: true, MCPConfig: `{"mcpServers":{}}`, AllowedTools: []string{"mcp__agentbox", "Read"}}, nil, nil)
	got := strings.Join(d.args(), " ")
	for _, want := range []string{
		"-p", "--input-format stream-json", "--output-format stream-json", "--verbose",
		"--permission-mode plan", "--include-partial-messages",
		`--mcp-config {"mcpServers":{}}`, "--allowed-tools mcp__agentbox,Read",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("args missing %q\ngot: %s", want, got)
		}
	}
}

func TestDriverArgsMinimal(t *testing.T) {
	d := New(Config{}, nil, nil) // defaults: plan mode, no partial/mcp
	got := strings.Join(d.args(), " ")
	if strings.Contains(got, "--include-partial-messages") || strings.Contains(got, "--mcp-config") {
		t.Errorf("minimal args should omit partial/mcp flags: %s", got)
	}
	if !strings.Contains(got, "--permission-mode plan") {
		t.Errorf("default mode should be plan: %s", got)
	}
}

func TestDriverMissingBinary(t *testing.T) {
	d := New(Config{Bin: "definitely-not-a-real-binary-xyz"}, nil, nil)
	if err := d.Start(); err == nil {
		t.Fatal("Start should fail when the binary is missing")
	}
	if d.State() != StateError {
		t.Errorf("state = %v, want error", d.State())
	}
	turns := d.Turns()
	if len(turns) != 1 || turns[0].Role != RoleSystem {
		t.Fatalf("want a system error turn, got %+v", turns)
	}
}

// A session agentbox starts is told it is inside agentbox (manual.Session): the flag has
// to reach the child's argv, and it has to be absent when there is nothing to say,
// so a plain child stays plain.
func TestArgsCarryTheBriefing(t *testing.T) {
	d := New(Config{Brief: "you are inside agentbox"}, nil, nil)
	args := d.args()
	at := -1
	for i, a := range args {
		if a == "--append-system-prompt" {
			at = i
		}
	}
	if at < 0 {
		t.Fatalf("no --append-system-prompt in %v", args)
	}
	if at+1 >= len(args) || args[at+1] != "you are inside agentbox" {
		t.Errorf("the briefing did not follow the flag: %v", args)
	}

	for _, brief := range []string{"", "   \n\t"} {
		plain := New(Config{Brief: brief}, nil, nil).args()
		for _, a := range plain {
			if a == "--append-system-prompt" {
				t.Errorf("an empty briefing (%q) still passed the flag: %v", brief, plain)
			}
		}
	}
}

// agentbox's two words are not Claude's flag values. "full" passed straight through
// is not one of --permission-mode's choices, so the child exits 1 immediately -
// which is exactly what happened the first time Full became the default.
func TestPermissionModeTranslates(t *testing.T) {
	cases := map[string]string{
		"full":              "bypassPermissions",
		"plan":              "plan",
		"":                  "plan", // New() fills this in, but the mapping must be total
		"somethingelse":     "plan", // never pass an unknown value to the child
		"bypassPermissions": "plan", // Claude's own vocabulary is not agentbox's input
	}
	for in, want := range cases {
		if got := permissionMode(in); got != want {
			t.Errorf("permissionMode(%q) = %q, want %q", in, got, want)
		}
	}

	// And it reaches the argv, next to the flag.
	args := New(Config{Mode: "full"}, nil, nil).args()
	at := -1
	for i, a := range args {
		if a == "--permission-mode" {
			at = i
		}
	}
	if at < 0 || at+1 >= len(args) || args[at+1] != "bypassPermissions" {
		t.Errorf("a full session's argv does not carry bypassPermissions: %v", args)
	}
}
