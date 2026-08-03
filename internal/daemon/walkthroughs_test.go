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
				"prose":   []map[string]any{{"t": "the guard locks the group"}},
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
