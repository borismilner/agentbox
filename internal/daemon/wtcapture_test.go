package daemon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
)

// A review is only true while it can still show what it cited. These tests are
// the two halves of that: creation keeps the source, and a review created
// before it did gets the source back out of git.

func captureSpec(root string) map[string]any {
	return map[string]any{
		"version":   1,
		"title":     "one cited range",
		"repo_root": root,
		"pinned":    "0000000",
		"steps": []map[string]any{
			{"id": "s1", "kind": "code", "title": "The line",
				"purpose": "Serves: a test. Decided by: the test.",
				"prose":   []map[string]any{{"t": "the line in question"}},
				"code": []map[string]any{
					{"path": "a.go", "lines": []int{2, 3}}}},
		},
	}
}

func TestCreateKeepsTheCitedSource(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("one\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, rpcErr := d.Handle(context.Background(), proto.MethodWalkthroughCreate, mustJSON(t, map[string]any{
		"id": "w000000000010", "spec": captureSpec(root),
	})); rpcErr != nil {
		t.Fatalf("create: %v", rpcErr)
	}

	ex, err := d.st.ExcerptsFor("w000000000010")
	if err != nil {
		t.Fatal(err)
	}
	if len(ex) != 1 {
		t.Fatalf("captured %d ranges, want 1", len(ex))
	}
	if ex[0].Text != "two\nthree" {
		t.Errorf("captured %q, want the cited lines 2-3", ex[0].Text)
	}
	if ex[0].Source != store.ExcerptWorktree {
		t.Errorf("source = %q", ex[0].Source)
	}

	// The file changing underneath must not change what was captured - that is
	// the entire point of capturing it.
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("ONE\nTWO\nTHREE\nFOUR\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ex, err = d.st.ExcerptsFor("w000000000010")
	if err != nil {
		t.Fatal(err)
	}
	if ex[0].Text != "two\nthree" {
		t.Errorf("the capture followed the file: %q", ex[0].Text)
	}
}

func TestCreateWarnsWhenItCannotCapture(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	res, rpcErr := d.Handle(context.Background(), proto.MethodWalkthroughCreate, mustJSON(t, map[string]any{
		"id": "w000000000011", "spec": captureSpec(t.TempDir()), // empty tree: nothing to read
	}))
	if rpcErr != nil {
		t.Fatalf("a walkthrough over content that is not on disk must still be created: %v", rpcErr)
	}
	created := res.(proto.WalkthroughCreateResult)
	var found bool
	for _, w := range created.Warnings {
		if strings.Contains(w, "could not capture a.go") {
			found = true
		}
	}
	if !found {
		t.Errorf("no warning about the uncaptured citation: %v", created.Warnings)
	}
}

// gitRepo builds a real repository with one commit, and returns its root and
// the SHA. Real git, because the repair path shells out to it and a fake would
// prove nothing about the arguments it passes.
func gitRepo(t *testing.T, name, content string) (root, sha string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git on PATH")
	}
	root = t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q")
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", name)
	run("commit", "-q", "-m", "one")
	return root, run("rev-parse", "HEAD")
}

func TestRepairRecoversFromThePinnedCommit(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	root, sha := gitRepo(t, "a.go", "one\ntwo\nthree\nfour\n")

	// A walkthrough as it would have been stored before capture existed: the
	// row, with no excerpts behind it.
	spec := captureSpec(root)
	spec["pinned"] = sha
	if _, rpcErr := d.Handle(context.Background(), proto.MethodWalkthroughCreate, mustJSON(t, map[string]any{
		"id": "w000000000012", "spec": spec,
	})); rpcErr != nil {
		t.Fatalf("create: %v", rpcErr)
	}
	if err := d.st.SaveExcerpts("w000000000012", nil); err != nil {
		t.Fatal(err)
	}
	// And the working tree moves on, exactly as a checkout would move it.
	if err := os.Remove(filepath.Join(root, "a.go")); err != nil {
		t.Fatal(err)
	}

	res, rpcErr := d.Handle(context.Background(), proto.MethodWalkthroughRepair,
		mustJSON(t, map[string]any{"id": "w000000000012"}))
	if rpcErr != nil {
		t.Fatalf("repair: %v", rpcErr)
	}
	rows := res.(proto.WalkthroughRepairResult).Repaired
	if len(rows) != 1 || rows[0].Recovered != 1 || rows[0].Missing != 0 {
		t.Fatalf("repair reported %+v", rows)
	}

	ex, err := d.st.ExcerptsFor("w000000000012")
	if err != nil {
		t.Fatal(err)
	}
	if len(ex) != 1 || ex[0].Text != "two\nthree" {
		t.Fatalf("recovered %+v", ex)
	}
	if ex[0].Source != store.ExcerptGit {
		t.Errorf("source = %q, want git", ex[0].Source)
	}
}

func TestRepairLeavesAnExistingCaptureAlone(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	root, sha := gitRepo(t, "a.go", "one\ntwo\nthree\nfour\n")
	spec := captureSpec(root)
	spec["pinned"] = sha
	if _, rpcErr := d.Handle(context.Background(), proto.MethodWalkthroughCreate, mustJSON(t, map[string]any{
		"id": "w000000000013", "spec": spec,
	})); rpcErr != nil {
		t.Fatalf("create: %v", rpcErr)
	}
	res, rpcErr := d.Handle(context.Background(), proto.MethodWalkthroughRepair,
		mustJSON(t, map[string]any{"id": "w000000000013"}))
	if rpcErr != nil {
		t.Fatalf("repair: %v", rpcErr)
	}
	// Nothing to do, so nothing reported: a repair over the library should
	// name what it changed, not everything it looked at.
	if rows := res.(proto.WalkthroughRepairResult).Repaired; len(rows) != 0 {
		t.Errorf("repair touched a complete walkthrough: %+v", rows)
	}
	ex, _ := d.st.ExcerptsFor("w000000000013")
	if len(ex) != 1 || ex[0].Source != store.ExcerptWorktree {
		t.Errorf("the original capture was replaced: %+v", ex)
	}
}

func TestRepairSaysWhyWhenGitCannotHelp(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	root, _ := gitRepo(t, "a.go", "one\ntwo\nthree\nfour\n")
	spec := captureSpec(root)
	spec["pinned"] = "0000000000000000000000000000000000000000" // a commit this clone never had
	if _, rpcErr := d.Handle(context.Background(), proto.MethodWalkthroughCreate, mustJSON(t, map[string]any{
		"id": "w000000000014", "spec": spec,
	})); rpcErr != nil {
		t.Fatalf("create: %v", rpcErr)
	}
	if err := d.st.SaveExcerpts("w000000000014", nil); err != nil {
		t.Fatal(err)
	}
	res, rpcErr := d.Handle(context.Background(), proto.MethodWalkthroughRepair,
		mustJSON(t, map[string]any{"id": "w000000000014"}))
	if rpcErr != nil {
		t.Fatalf("repair: %v", rpcErr)
	}
	rows := res.(proto.WalkthroughRepairResult).Repaired
	if len(rows) != 1 || rows[0].Recovered != 0 || rows[0].Missing != 1 {
		t.Fatalf("repair reported %+v", rows)
	}
	if len(rows[0].Notes) == 0 || !strings.Contains(rows[0].Notes[0], "a.go") {
		t.Errorf("no usable reason for the miss: %v", rows[0].Notes)
	}
}
