package daemon

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/borismilner/agentbox/internal/proto"
)

func wtSpec() map[string]any {
	return map[string]any{
		"version":   1,
		"title":     "session twenty-five",
		"repo_root": "/home/user/repo",
		"pinned":    "dd375a3cb2c7",
		"steps": []map[string]any{
			{"id": "xkb", "kind": "code", "title": "The guard",
				"purpose": "Serves: typed text is planned text.",
				"tldr": map[string]any{
					"bottom": "The group is locked per stroke, so a planned key cannot be typed under a swapped layout.",
					"points": []string{"The desktop reverts within 1ms, so per call is not often enough."},
				},
				"prose": []map[string]any{{"t": "the guard locks the group"}},
				"code": []map[string]any{
					{"path": "internal/hand/xkb.go", "lines": []int{118, 145}}}},
		},
	}
}

func TestWalkthroughCreateReadDelete(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	ctx := context.Background()

	res, rpcErr := d.Handle(ctx, proto.MethodWalkthroughCreate, mustJSON(t, map[string]any{
		"id": "w000000000001", "spec": wtSpec(),
		"identity": proto.Identity{Agent: "claude-code"},
	}))
	if rpcErr != nil {
		t.Fatalf("create: %v", rpcErr)
	}
	created := res.(proto.WalkthroughCreateResult)
	if created.Rev != 1 || created.Coverage.Computed {
		t.Errorf("create result: %+v", created)
	}
	ui.mu.Lock()
	boards := len(ui.boards)
	ui.mu.Unlock()
	if boards != 1 {
		t.Errorf("create must show the board once, got %d", boards)
	}

	res, rpcErr = d.Handle(ctx, proto.MethodWalkthroughRead, mustJSON(t, map[string]any{"id": "w000000000001"}))
	if rpcErr != nil {
		t.Fatalf("read: %v", rpcErr)
	}
	st := res.(*proto.WalkthroughState)
	if st.Title != "session twenty-five" || st.State != "open" || st.Rev != 1 {
		t.Errorf("read state: %+v", st)
	}
	var spec map[string]any
	if err := json.Unmarshal(st.Spec, &spec); err != nil {
		t.Fatalf("spec does not parse back: %v", err)
	}
	if spec["diff"] != nil && spec["diff"] != "" {
		t.Errorf("diff must be stripped from the stored spec")
	}

	res, rpcErr = d.Handle(ctx, proto.MethodWalkthroughList, nil)
	if rpcErr != nil {
		t.Fatalf("list: %v", rpcErr)
	}
	if rows := res.(proto.WalkthroughListResult).Walkthroughs; len(rows) != 1 || rows[0].CountedSteps != 1 {
		t.Errorf("list: %+v", rows)
	}

	res, rpcErr = d.Handle(ctx, proto.MethodWalkthroughDelete, mustJSON(t, map[string]any{"id": "w000000000001"}))
	if rpcErr != nil || !res.(proto.WalkthroughDeleteResult).Deleted {
		t.Fatalf("delete: %v %v", res, rpcErr)
	}
	if _, rpcErr = d.Handle(ctx, proto.MethodWalkthroughRead, mustJSON(t, map[string]any{"id": "w000000000001"})); rpcErr == nil {
		t.Error("read after delete must fail")
	}
}

func TestWalkthroughCreateTeaches(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	ctx := context.Background()

	_, rpcErr := d.Handle(ctx, proto.MethodWalkthroughCreate, mustJSON(t, map[string]any{
		"id": "not-an-id", "spec": wtSpec(), "identity": proto.Identity{Agent: "a"},
	}))
	if rpcErr == nil || !strings.Contains(rpcErr.Message, "caller-minted") {
		t.Errorf("bad id: %v", rpcErr)
	}

	bad := wtSpec()
	bad["pinned"] = "nope"
	_, rpcErr = d.Handle(ctx, proto.MethodWalkthroughCreate, mustJSON(t, map[string]any{
		"id": "w000000000002", "spec": bad, "identity": proto.Identity{Agent: "a"},
	}))
	if rpcErr == nil || !strings.Contains(rpcErr.Message, "pinned") {
		t.Errorf("bad spec: %v", rpcErr)
	}
}

func TestWalkthroughOpenMissing(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	_, rpcErr := d.Handle(context.Background(), proto.MethodWalkthroughOpen,
		mustJSON(t, map[string]any{"id": "w0000000000ff"}))
	if rpcErr == nil || rpcErr.Code != proto.CodeItemNotFound {
		t.Errorf("open missing: %v", rpcErr)
	}
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if len(ui.boards) != 0 {
		t.Error("open of a missing walkthrough must not show a board")
	}
}

func TestWalkthroughNoShow(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	_, rpcErr := d.Handle(context.Background(), proto.MethodWalkthroughCreate, mustJSON(t, map[string]any{
		"id": "w000000000003", "spec": wtSpec(), "no_show": true,
		"identity": proto.Identity{Agent: "a"},
	}))
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if len(ui.boards) != 0 {
		t.Error("no_show must keep the board closed")
	}
}

// --- Coverage: the standard's rule 49, checked rather than trusted (FR61) -----

// wtSpecWithDiff is the fixture plus a manifest: two hunks, one of them under
// the step's citation (internal/hand/xkb.go:118-145) and one nowhere near it.
func wtSpecWithDiff() map[string]any {
	s := wtSpec()
	s["diff"] = `diff --git a/internal/hand/xkb.go b/internal/hand/xkb.go
--- a/internal/hand/xkb.go
+++ b/internal/hand/xkb.go
@@ -120,3 +120,4 @@ func lock() {
 	before()
+	locked()
 	after()
diff --git a/internal/webui/board.go b/internal/webui/board.go
--- a/internal/webui/board.go
+++ b/internal/webui/board.go
@@ -40,2 +40,3 @@ func paint() {
 	head()
+	rail()
`
	return s
}

func TestCreateCountsCoverageAndNamesWhatNoStepStandsOn(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	res, rpcErr := d.Handle(context.Background(), proto.MethodWalkthroughCreate, mustJSON(t, map[string]any{
		"id": "w000000000010", "spec": wtSpecWithDiff(),
		"identity": proto.Identity{Agent: "claude-code"},
	}))
	if rpcErr != nil {
		t.Fatalf("create: %v", rpcErr)
	}
	got := res.(proto.WalkthroughCreateResult).Coverage
	if !got.Computed {
		t.Fatal("a diff was passed and coverage came back uncomputed")
	}
	if got.Hunks != 2 || got.Covered != 1 || got.Uncovered != 1 {
		t.Errorf("coverage: %+v", got)
	}
	if len(got.UncoveredHunks) != 1 || got.UncoveredHunks[0].Path != "internal/webui/board.go" {
		t.Errorf("uncovered hunks: %+v", got.UncoveredHunks)
	}

	// And it says so in words, naming the hunk: "1 of 2 hunks" alone would send
	// the author back to diff the diff against their own steps by hand.
	warnings := res.(proto.WalkthroughCreateResult).Warnings
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "not cited by any step") && strings.Contains(w, "internal/webui/board.go:40-42") {
			found = true
		}
	}
	if !found {
		t.Errorf("no warning named the uncovered hunk: %q", warnings)
	}
}

func TestAStatedExclusionAnswersForItsHunks(t *testing.T) {
	spec := wtSpecWithDiff()
	spec["out_of_scope"] = []map[string]any{
		{"paths": "internal/webui/**", "reason": "the surface is unchanged in behaviour"},
	}
	d, _, _, _ := newTestDaemon(t, Config{})
	res, rpcErr := d.Handle(context.Background(), proto.MethodWalkthroughCreate, mustJSON(t, map[string]any{
		"id": "w000000000011", "spec": spec, "identity": proto.Identity{Agent: "claude-code"},
	}))
	if rpcErr != nil {
		t.Fatalf("create: %v", rpcErr)
	}
	created := res.(proto.WalkthroughCreateResult)
	if created.Coverage.OutOfScope != 1 || created.Coverage.Uncovered != 0 {
		t.Errorf("coverage: %+v", created.Coverage)
	}
	for _, w := range created.Warnings {
		if strings.Contains(w, "not cited by any step") {
			t.Errorf("a stated exclusion was warned about as a hole: %q", w)
		}
	}
}

func TestReadingAStoredWalkthroughRecomputesItsCoverage(t *testing.T) {
	// The board asks on every open, and the spec is the only copy of the truth -
	// so this is recomputed rather than stored, and a library review still says
	// what it accounted for.
	d, _, _, _ := newTestDaemon(t, Config{})
	ctx := context.Background()
	if _, rpcErr := d.Handle(ctx, proto.MethodWalkthroughCreate, mustJSON(t, map[string]any{
		"id": "w000000000012", "spec": wtSpecWithDiff(), "identity": proto.Identity{Agent: "claude-code"},
	})); rpcErr != nil {
		t.Fatalf("create: %v", rpcErr)
	}
	res, rpcErr := d.Handle(ctx, proto.MethodWalkthroughRead, mustJSON(t, map[string]any{"id": "w000000000012"}))
	if rpcErr != nil {
		t.Fatalf("read: %v", rpcErr)
	}
	got := res.(*proto.WalkthroughState).Coverage
	if !got.Computed || got.Hunks != 2 || got.Covered != 1 || got.Uncovered != 1 {
		t.Errorf("coverage on read: %+v", got)
	}
}
