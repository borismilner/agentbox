package webui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// FR65. The surface names a review and a repo-relative path; Go owns the root.
// These are the cases where that distinction earns its keep.

func TestUnderRootResolvesACitation(t *testing.T) {
	got, err := underRoot("/repo", "pkg/images/client.go")
	if err != nil {
		t.Fatalf("underRoot: %v", err)
	}
	if got != "/repo/pkg/images/client.go" {
		t.Fatalf("path = %q", got)
	}
}

func TestUnderRootRefusesAnEscape(t *testing.T) {
	for _, rel := range []string{
		"../../etc/passwd",
		"pkg/../../outside.go",
		"/etc/passwd",
		"",
		"   ",
		"/repo-sibling/f.go", // absolute, and a prefix of the root's own string
	} {
		if got, err := underRoot("/repo", rel); err == nil {
			t.Fatalf("%q was accepted as %q", rel, got)
		}
	}
}

func TestUnderRootRefusesASiblingSharingThePrefix(t *testing.T) {
	// "/repo-sibling" starts with "/repo", so a prefix check without the
	// separator would let it through.
	if got, err := underRoot("/repo", "../repo-sibling/f.go"); err == nil {
		t.Fatalf("a sibling directory was accepted as %q", got)
	}
}

func TestOpenInEditorRunsTheConfiguredCommand(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the stub launcher is a shell script")
	}
	u, f := boardTestUI(t)

	// A launcher that records its argv, so what the editor was actually asked
	// for is asserted rather than assumed.
	dir := t.TempDir()
	out := filepath.Join(dir, "argv")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + out + "\n"
	if err := os.WriteFile(filepath.Join(dir, "fake-editor"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// No systemd-run on this PATH, so the launcher runs directly and this test
	// can wait for it; the cgroup escape is asserted in internal/editor.
	t.Setenv("PATH", dir)

	cfg := u.conf()
	cfg.Editor.Command = []string{"fake-editor", "{dir}", "--line", "{line}", "{file}"}
	u.SetConfig(cfg)

	br := &Bridge{ui: u}
	if err := br.BoardOpenInEditor("w000000000001", "pkg/images/client.go", 42); err != nil {
		t.Fatalf("open: %v", err)
	}

	// Started, not waited on - Start is what raises an editor and returning
	// before it has painted is the point - so poll for the trace it leaves.
	var got []byte
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(out); err == nil && len(b) > 0 {
			got = b
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got == nil {
		t.Fatal("the launcher never ran")
	}
	want := f.w.RepoRoot + "\n--line\n42\n" + filepath.Join(f.w.RepoRoot, "pkg/images/client.go") + "\n"
	if string(got) != want {
		t.Fatalf("the editor saw\n%q\nwant\n%q", got, want)
	}
}

func TestOpenInEditorRefusesAPathOutsideTheReview(t *testing.T) {
	u, _ := boardTestUI(t)
	// A launcher that would succeed if it were ever reached.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fake-editor"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	cfg := u.conf()
	cfg.Editor.Command = []string{"fake-editor", "{file}"}
	u.SetConfig(cfg)

	br := &Bridge{ui: u}
	err := br.BoardOpenInEditor("w000000000001", "../../../etc/passwd", 1)
	if err == nil {
		t.Fatal("a path outside the review was opened")
	}
	if !strings.Contains(err.Error(), "outside the repository") {
		t.Fatalf("err = %v, want it to say why", err)
	}
}

func TestOpenInEditorReportsAMissingEditor(t *testing.T) {
	u, _ := boardTestUI(t)
	t.Setenv("PATH", t.TempDir()) // nothing installed, not even xdg-open
	cfg := u.conf()
	cfg.Editor.Command = nil
	u.SetConfig(cfg)

	br := &Bridge{ui: u}
	err := br.BoardOpenInEditor("w000000000001", "pkg/images/client.go", 1)
	if err == nil {
		t.Fatal("an open succeeded with no editor on the machine")
	}
	// The message is what the reader is shown beside the block, so it has to
	// name the key that fixes it.
	if !strings.Contains(err.Error(), "editor.command") {
		t.Fatalf("err = %v, want it to name the config key", err)
	}
}

func TestOpenInEditorWithNoBoardStore(t *testing.T) {
	br := &Bridge{ui: confUI()}
	if err := br.BoardOpenInEditor("w000000000001", "f.go", 1); err == nil {
		t.Fatal("an open succeeded with no store wired")
	}
}
