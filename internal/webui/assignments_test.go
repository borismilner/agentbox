package webui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/daemon"
	"github.com/borismilner/agentbox/internal/session"
)

// The runner, end to end against a stub `claude`. What is worth pinning here is
// everything a real child would otherwise hide: that a run reports its last
// words, that a data block reaches the store instead of the human's summary,
// and that a run firing while somebody is typing does not move their selection.

// replyingClaude writes a stub that behaves like a child asked one question:
// it announces itself, waits for the prompt, answers with reply, reports the
// turn finished, and then holds stdin open the way a real session does.
func replyingClaude(t *testing.T, reply string) string {
	t.Helper()
	dir := t.TempDir()
	msg, err := json.Marshal(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"role": "assistant", "model": "m",
			"content": []map[string]any{{"type": "text", "text": reply}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	initLine := write("init.json",
		`{"type":"system","subtype":"init","session_id":"sess-run","model":"m","cwd":"/tmp"}`+"\n")
	replyLine := write("reply.json", string(msg)+"\n"+
		`{"type":"result","subtype":"success","total_cost_usd":0.01}`+"\n")

	path := filepath.Join(dir, "claude-stub")
	script := "#!/bin/sh\ncat " + initLine + "\nread -r line\ncat " + replyLine + "\ncat >/dev/null\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func runnerUI(t *testing.T, reply string) *UI {
	t.Helper()
	// A finished run saves its conversation, and session.SessionsDir reads the
	// environment: without this the suite writes into the human's own session
	// history every time it passes.
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	u := gateUI()
	u.pan = newPanel(u)
	u.cfg.Session.Binary = replyingClaude(t, reply)
	u.cfg.Session.Dir = t.TempDir()
	t.Cleanup(func() {
		for _, ls := range u.sess.list {
			ls.drv.Kill()
		}
	})
	return u
}

func TestARunReportsItsLastWordsAndItsData(t *testing.T) {
	u := runnerUI(t, "Usage is at 82% of the weekly cap.\n\n```agentbox-data\n{\"usage_pct\": 82}\n```\n\nNothing to do yet.")

	var gotSession string
	summary, data, err := u.RunAssignment(daemon.RunRequest{
		AssignmentID: "a1", RunID: "r1", Name: "Usage watch", Prompt: "Check usage.",
		Dir: t.TempDir(), Trigger: "manual",
		OnSession: func(id string) { gotSession = id },
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(summary, "82% of the weekly cap") || !strings.Contains(summary, "Nothing to do yet") {
		t.Errorf("summary = %q, want the prose either side of the data block", summary)
	}
	if strings.Contains(summary, "agentbox-data") || strings.Contains(summary, "usage_pct") {
		t.Errorf("the data block leaked into the human's summary: %q", summary)
	}
	if data != `{"usage_pct": 82}` {
		t.Errorf("data = %q, want the block's contents", data)
	}
	if gotSession == "" {
		t.Error("the run never named its session, so the panel cannot offer to open it")
	}
}

// A run is a session on purpose - but it is not the human's session. One that
// fires while the panel is open must not move them off what they were typing.
func TestARunDoesNotStealTheSelection(t *testing.T) {
	u := runnerUI(t, "done")
	u.sess.EnsureOne()
	mine := u.sess.selected
	if mine == "" {
		t.Fatal("no session to be stolen from")
	}

	if _, _, err := u.RunAssignment(daemon.RunRequest{
		AssignmentID: "a1", RunID: "r1", Name: "Nightly", Prompt: "Go.", Dir: t.TempDir(),
	}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if u.sess.selected != mine {
		t.Errorf("selected = %q, want the human's own session %q", u.sess.selected, mine)
	}
	if len(u.sess.list) != 2 {
		t.Fatalf("%d sessions, want the human's and the run's", len(u.sess.list))
	}
}

// The run's child is stopped when it is done - thirty daily runs must not leave
// thirty children idling - but the conversation stays, because reading what an
// assignment actually did is the whole reason a run is a session.
func TestAFinishedRunLeavesItsTranscriptAndNoChild(t *testing.T) {
	u := runnerUI(t, "all quiet")
	if _, _, err := u.RunAssignment(daemon.RunRequest{
		AssignmentID: "a1", RunID: "r1", Name: "Nightly", Prompt: "Go.", Dir: t.TempDir(),
	}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(u.sess.list) != 1 {
		t.Fatalf("%d sessions, want the run's to remain", len(u.sess.list))
	}
	ls := u.sess.list[0]
	if ls.name() != "Nightly" {
		t.Errorf("the chip says %q; a run should be labelled with its assignment", ls.name())
	}
	deadline := time.Now().Add(2 * time.Second)
	for ls.drv.State() != session.StateEnded && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if ls.drv.State() != session.StateEnded {
		t.Errorf("state = %v, want ended: the child outlived its run", ls.drv.State())
	}
	if len(ls.drv.Turns()) == 0 {
		t.Error("the transcript is empty, so there is nothing to open")
	}
}

// A child that exits without a word did not carry the assignment out. Recording
// that as a successful run with an empty summary is how an assignment quietly
// stops working for a month.
func TestARunThatSaysNothingIsAFailure(t *testing.T) {
	u := runnerUI(t, "")
	if _, _, err := u.RunAssignment(daemon.RunRequest{
		AssignmentID: "a1", RunID: "r1", Name: "Silent", Prompt: "Go.", Dir: t.TempDir(),
	}); err == nil {
		t.Fatal("a run that reported nothing was recorded as a success")
	}
}

// A session that cannot start is the run's failure, not a summary of nothing.
func TestARunThatCannotStartFails(t *testing.T) {
	u := gateUI()
	u.pan = newPanel(u)
	u.cfg.Session.Binary = filepath.Join(t.TempDir(), "not-claude")
	if _, _, err := u.RunAssignment(daemon.RunRequest{
		AssignmentID: "a1", RunID: "r1", Name: "Nightly", Prompt: "Go.", Dir: t.TempDir(),
	}); err == nil {
		t.Fatal("a run with no claude to spawn reported success")
	}
}

func TestSplitReportKeepsAHalfWrittenBlockVisible(t *testing.T) {
	for name, tc := range map[string]struct{ in, summary, data string }{
		"no block":     {"All quiet.", "All quiet.", ""},
		"only a block": {"```agentbox-data\n{\"n\":1}\n```", "", `{"n":1}`},
		"never closed": {"Report.\n```agentbox-data\n{\"n\":1}", "Report.\n```agentbox-data\n{\"n\":1}", ""},
		"no newline":   {"Report. ```agentbox-data", "Report. ```agentbox-data", ""},
		"empty":        {"", "", ""},
	} {
		summary, data := splitReport(tc.in)
		if summary != tc.summary || data != tc.data {
			t.Errorf("%s: summary = %q, data = %q; want %q / %q", name, summary, data, tc.summary, tc.data)
		}
	}
}

// The report is the agent's last words, not its last action: a run that ends on
// a tool call still reports the prose it wrote before it.
func TestLastAssistantTextSkipsWhatIsNotProse(t *testing.T) {
	turns := []session.Turn{
		{Role: session.RoleAssistant, Segments: []session.Segment{{Kind: session.SegText, Text: "First pass."}}},
		{Role: session.RoleUser, Segments: []session.Segment{{Kind: session.SegText, Text: "carry on"}}},
		{Role: session.RoleAssistant, Segments: []session.Segment{
			{Kind: session.SegThinking, Text: "hmm"},
			{Kind: session.SegText, Text: "The answer."},
			{Kind: session.SegToolUse, ToolName: "Bash", ToolInput: "ls"},
		}},
		{Role: session.RoleAssistant, Segments: []session.Segment{
			{Kind: session.SegToolUse, ToolName: "Bash", ToolInput: "ls"},
		}},
	}
	if got := lastAssistantText(turns); got != "The answer." {
		t.Errorf("report = %q, want the last prose the agent wrote", got)
	}
	if got := lastAssistantText(nil); got != "" {
		t.Errorf("an empty conversation reported %q", got)
	}
}
