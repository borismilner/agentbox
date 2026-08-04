package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The property slice 5 rests on: a hook and the model's own mcp child must derive
// the SAME session key, or one session shows up on the board as two rows.
//
// Their trees differ by exactly one shell. The mcp child's parent IS the agent;
// a hook's parent is the shell Claude Code ran the command through. So the walk is
// run over a real tree with both shapes in it, and the two answers are compared.
func TestAgentProcessAgreesAcrossAHookAndAChild(t *testing.T) {
	// `sh -c 'sleep ...'` without exec leaves the shell in place, which is the
	// hook's shape: agentbox -> sh -> agent.
	cmd := exec.Command("sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sh: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	shell := cmd.Process.Pid

	// This test binary stands in for the agent: its name is not a placeholder, so
	// the walk stops at it the way it stops at claude.
	self := os.Getpid()

	hookPid, hookName, ok := agentProcessFrom(shell)
	if !ok {
		t.Fatalf("hook-shaped walk from the shell (pid %d) found no agent", shell)
	}
	childPid, childName, ok := agentProcessFrom(self)
	if !ok {
		t.Fatalf("child-shaped walk from the agent (pid %d) found no agent", self)
	}
	if hookPid != self || childPid != self {
		t.Errorf("walks disagree: hook found %d, child found %d, want both %d", hookPid, childPid, self)
	}
	if hookName != childName {
		t.Errorf("names disagree: hook %q, child %q", hookName, childName)
	}

	// What the callers actually compare is the key, so compare that too.
	hookKey, err := procSessionKeyFor(hookPid)
	if err != nil {
		t.Fatalf("key for %d: %v", hookPid, err)
	}
	childKey, err := procSessionKeyFor(childPid)
	if err != nil {
		t.Fatalf("key for %d: %v", childPid, err)
	}
	if hookKey != childKey {
		t.Errorf("a hook and a child would write two rows: %q vs %q", hookKey, childKey)
	}
	if !strings.HasPrefix(hookKey, "proc-"+strconv.Itoa(self)+"-") {
		t.Errorf("key %q does not name the process it belongs to (pid %d)", hookKey, self)
	}
}

// A walk with no agent above it must answer false rather than name init. Under
// setsid the tree is cut, and a caller that belongs to no session has to be
// refused instead of being given somebody else's key.
func TestAgentProcessRefusesWhenTheTreeIsCut(t *testing.T) {
	if _, _, ok := agentProcessFrom(1); ok {
		t.Error("a walk starting at init claimed to find an agent")
	}
	if _, _, ok := agentProcessFrom(0); ok {
		t.Error("a walk starting at 0 claimed to find an agent")
	}
}

// The key is what a lock and a claim are owned by, so a recycled pid must not
// inherit them. The start time is what prevents it, and only the right stat field
// is a start time: field 21 is usually zero and field 23 is a memory size.
func TestProcStartTimeIsAStartTime(t *testing.T) {
	self, err := procStartTime(os.Getpid())
	if err != nil {
		t.Fatalf("start time of self: %v", err)
	}
	if self <= 0 {
		t.Fatalf("start time of self is %d, so the pid alone names the session", self)
	}

	// Ticks since boot, so it cannot exceed the uptime. This is what rules out
	// vsize, which would pass a "greater than zero" check with room to spare.
	up, err := os.ReadFile("/proc/uptime")
	if err != nil {
		t.Fatalf("read uptime: %v", err)
	}
	secs, err := strconv.ParseFloat(strings.Fields(string(up))[0], 64)
	if err != nil {
		t.Fatalf("parse uptime %q: %v", up, err)
	}
	if ticks := int64(secs) * 100; self > ticks+100 {
		t.Errorf("start time %d is past the uptime (%d ticks), so it is not a start time", self, ticks)
	}

	// A process started later started later. This is the ordering a recycled pid
	// would break if the field were a constant.
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	child, err := procStartTime(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("start time of child: %v", err)
	}
	if child < self {
		t.Errorf("child started at %d, before its parent at %d", child, self)
	}
}

// The key must not move while the session runs: it is read again by every hook
// and every tool call, and a key that drifts loses the row it wrote.
func TestInheritedSessionKeyIsStable(t *testing.T) {
	t.Setenv("AGENTBOX_SESSION_KEY", "")
	t.Setenv("AGENTBOX_SESSION_ID", "")
	first := inheritedSessionKey()
	if first == "" {
		t.Skip("no agent above this test process, so there is no key to be stable")
	}
	time.Sleep(10 * time.Millisecond)
	if second := inheritedSessionKey(); second != first {
		t.Errorf("key moved from %q to %q inside one process", first, second)
	}
}

// The environment still wins, in the documented order: an explicit key over
// AgentBox's own session id, and that over anything read off the process tree.
func TestInheritedSessionKeyPrefersWhatItIsTold(t *testing.T) {
	t.Setenv("AGENTBOX_SESSION_ID", "from-the-session-tab")
	t.Setenv("AGENTBOX_SESSION_KEY", "explicit")
	if got := inheritedSessionKey(); got != "explicit" {
		t.Errorf("got %q, want the explicit key", got)
	}
	t.Setenv("AGENTBOX_SESSION_KEY", "")
	if got := inheritedSessionKey(); got != "from-the-session-tab" {
		t.Errorf("got %q, want the session tab's id", got)
	}
	t.Setenv("AGENTBOX_SESSION_ID", "")
	if got := inheritedSessionKey(); strings.HasPrefix(got, "proc-") == (got == "") {
		t.Errorf("with nothing in the environment the key must come off the tree or be empty, got %q", got)
	}
}
