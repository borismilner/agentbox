package webui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/borismilner/agentbox/internal/session"
)

// stubClaude writes an executable that behaves like a `claude` child doing
// nothing: it holds stdin open and says nothing, which is all these tests need
// from it. Returns its path.
func stubClaude(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claude-stub")
	if err := os.WriteFile(path, []byte("#!/bin/sh\ncat >/dev/null\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func sessionUI(t *testing.T) *UI {
	t.Helper()
	u := gateUI()
	u.pan = newPanel(u)
	u.cfg.Session.Binary = stubClaude(t)
	u.cfg.Session.Dir = t.TempDir()
	t.Cleanup(func() {
		for _, ls := range u.sess.list {
			ls.drv.Kill()
		}
	})
	return u
}

// The panel must not come down onto nothing. Rolling it down starts a session if
// there is none, and finds the same one every time after that - two hotkey presses
// must not leave two children running.
func TestEnsureOneStartsExactlyOneSession(t *testing.T) {
	u := sessionUI(t)

	u.sess.EnsureOne()
	if got := len(u.sess.list); got != 1 {
		t.Fatalf("%d sessions after the first roll, want 1", got)
	}
	first := u.sess.list[0].id

	u.sess.EnsureOne()
	if got := len(u.sess.list); got != 1 {
		t.Fatalf("%d sessions after the second roll, want the same 1", got)
	}
	if u.sess.list[0].id != first {
		t.Error("the second roll replaced the session instead of finding it")
	}
	if u.sess.selected != first {
		t.Errorf("selected = %q, want the session that exists (%q)", u.sess.selected, first)
	}
}

// A session started with no directory lands in [session] dir, resolved to a real
// path - not left as ".", which is what the switcher chip and the panel header
// would then have to show.
func TestStartResolvesTheConfiguredDirectory(t *testing.T) {
	u := sessionUI(t)
	want := u.cfg.Session.Dir

	id, err := u.sess.Start("", "plan")
	if err != nil {
		t.Fatal(err)
	}
	ls := u.sess.find(id)
	if ls == nil {
		t.Fatal("the session is not in the list")
	}
	if ls.cwd != want {
		t.Errorf("cwd = %q, want the configured %q", ls.cwd, want)
	}
	if ls.cwd == "." || ls.cwd == "" {
		t.Error("cwd was left relative, so nothing can display it")
	}
	if ls.name() != filepath.Base(want) {
		t.Errorf("name = %q, want the directory's name %q before anything is said", ls.name(), filepath.Base(want))
	}
}

// With no directory configured at all it is the daemon's own working directory,
// absolute - never the literal ".".
func TestStartFallsBackToTheDaemonsCwd(t *testing.T) {
	u := sessionUI(t)
	u.cfg.Session.Dir = ""
	wd, err := os.Getwd()
	if err != nil {
		t.Skip("no working directory")
	}

	id, err := u.sess.Start("", "plan")
	if err != nil {
		t.Fatal(err)
	}
	if got := u.sess.find(id).cwd; got != wd {
		t.Errorf("cwd = %q, want the daemon's %q", got, wd)
	}
}

// An explicit directory still wins over the config: the RPC and a future
// per-project launcher pass one.
func TestStartHonoursAnExplicitDirectory(t *testing.T) {
	u := sessionUI(t)
	explicit := t.TempDir()

	id, err := u.sess.Start(explicit, "full")
	if err != nil {
		t.Fatal(err)
	}
	ls := u.sess.find(id)
	if ls.cwd != explicit {
		t.Errorf("cwd = %q, want %q", ls.cwd, explicit)
	}
	if ls.mode != "full" {
		t.Errorf("mode = %q, want full", ls.mode)
	}
	if strings.Contains(ls.cwd, u.cfg.Session.Dir) {
		t.Error("the configured directory overrode the one that was asked for")
	}
}

// A session in a directory used to be called after the directory, so three
// sessions in one project were three chips reading the same word. Claude's own
// first words are the name; the human can overrule it with a label.
func TestSessionNamesItselfFromWhatWasSaid(t *testing.T) {
	ls := &liveSession{project: "agentbox"}
	if got := ls.name(); got != "agentbox" {
		t.Errorf("a session with nothing said = %q, want the project", got)
	}

	// Before the agent replies, the prompt stands in.
	ls.nameFromConversation([]session.Turn{
		{Role: session.RoleUser, Segments: []session.Segment{
			{Kind: session.SegText, Text: "Write a hello world program in 3 different languages"}}},
	})
	if got := ls.name(); got != "Write a hello world program in 3 different…" {
		t.Errorf("name from the prompt = %q", got)
	}

	// Claude's heading wins once it exists, and it is worked out once: a later call
	// must not rewrite the name under the human.
	fresh := &liveSession{project: "agentbox"}
	fresh.nameFromConversation([]session.Turn{
		{Role: session.RoleUser, Segments: []session.Segment{{Kind: session.SegText, Text: "go on then"}}},
		{Role: session.RoleAssistant, Segments: []session.Segment{
			{Kind: session.SegToolUse, ToolName: "Bash", ToolInput: "ls"},
			{Kind: session.SegText, Text: "**Plan: hello world in Python, Go, Rust** - the three toolchains here"}}},
	})
	if got := fresh.name(); got != "Plan: hello world in Python, Go, Rust" {
		t.Errorf("name from the reply = %q", got)
	}
	fresh.nameFromConversation([]session.Turn{
		{Role: session.RoleAssistant, Segments: []session.Segment{{Kind: session.SegText, Text: "something else entirely"}}},
	})
	if got := fresh.name(); got != "Plan: hello world in Python, Go, Rust" {
		t.Errorf("the name changed under the reader: %q", got)
	}

	// A label overrules both, and clearing it hands the name back.
	fresh.label = "the migration"
	if got := fresh.name(); got != "the migration" {
		t.Errorf("label = %q", got)
	}
	fresh.label = ""
	if got := fresh.name(); got != "Plan: hello world in Python, Go, Rust" {
		t.Errorf("clearing the label lost the automatic name: %q", got)
	}
}

// titleFrom is the only formatting rule here, so its edges are the test: markdown
// dress comes off, a sentence is a name and a paragraph is not, and 42 characters
// is what the switcher can show.
func TestTitleFromReducesMarkdownToAName(t *testing.T) {
	cases := map[string]string{
		"# Migrating the users table":      "Migrating the users table",
		"**Plan: two files** - and a test": "Plan: two files",
		"`agentbox say --wait` is next":    "agentbox say --wait is next",
		// A four-letter sentence is not a name, so the line stands: the sentence
		// cut only applies once there is enough in front of it to read.
		"Done. I removed the retry loop and the sleep":                              "Done. I removed the retry loop and the sle…",
		"A very long first line that runs on well past the measure a chip can show": "A very long first line that runs on well p…",
		"":         "",
		"   \n\n ": "",
	}
	for in, want := range cases {
		if got := titleFrom(in); got != want {
			t.Errorf("titleFrom(%q) = %q, want %q", in, got, want)
		}
	}
}
